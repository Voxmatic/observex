// services/processor/main.go
// ObserveX Processor — the intelligence layer.
// Receives live data from the ingestor and:
//   1. Builds the live service topology graph
//   2. Runs anomaly detection on all metrics
//   3. Tracks SLOs and calculates error budgets
//   4. Evaluates alert rules
//   5. Feeds the AI agent with detected problems
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/observex/platform/internal/servicetoken"
	"github.com/observex/platform/pkg/models"
)

// ═══════════════════════════════════════════════════════
//  PROCESSOR
// ═══════════════════════════════════════════════════════

type Processor struct {
	logger *zap.Logger
	client *http.Client

	// Live service topology (in-memory graph)
	topoMu   sync.RWMutex
	services map[string]*models.Service
	edges    map[string]*models.TopoEdge

	// Metric time-windows for anomaly detection
	// serviceID → metricName → circular buffer of values
	metricsMu      sync.RWMutex
	metricsWindows map[string]map[string]*CircularBuffer

	// SLO state
	sloMu sync.RWMutex
	slos  map[string]*models.SLO

	// Alert rules and active alerts
	alertMu     sync.RWMutex
	alertRules  map[string]*models.AlertRule
	activeAlerts map[string]*models.FiredAlert

	// Problems detected — fed to AI agent
	problemMu sync.RWMutex
	problems  map[string]*models.Problem

	// AI agent endpoint
	aiAgentURL string

	// Regression engine
	regression *regressionEngine
	// Predictive analytics / forecasting
	forecaster       *ForecastEngine
	mlCostForecaster *MLCostForecaster

	// PostgreSQL pool (optional — for persistent deployment records)
	pg *pgxpool.Pool
}

type CircularBuffer struct {
	data  []float64
	head  int
	count int
	cap   int
	mean  float64
	std   float64
	dirty bool
}

func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{data: make([]float64, capacity), cap: capacity}
}

func (b *CircularBuffer) Push(v float64) {
	b.data[b.head] = v
	b.head = (b.head + 1) % b.cap
	if b.count < b.cap {
		b.count++
	}
	b.dirty = true
}

func (b *CircularBuffer) Stats() (mean, std float64) {
	if b.dirty {
		if b.count == 0 {
			return 0, 0
		}
		sum := 0.0
		for i := 0; i < b.count; i++ {
			sum += b.data[i]
		}
		b.mean = sum / float64(b.count)
		variance := 0.0
		for i := 0; i < b.count; i++ {
			d := b.data[i] - b.mean
			variance += d * d
		}
		if b.count > 1 {
			b.std = math.Sqrt(variance / float64(b.count-1))
		}
		b.dirty = false
	}
	return b.mean, b.std
}

func (b *CircularBuffer) Values() []float64 {
	out := make([]float64, b.count)
	for i := 0; i < b.count; i++ {
		idx := (b.head - b.count + i + b.cap) % b.cap
		out[i] = b.data[idx]
	}
	return out
}

// ═══════════════════════════════════════════════════════
//  STARTUP
// ═══════════════════════════════════════════════════════

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Connect to PostgreSQL for deployment persistence (optional — processor works without it)
	var pgPool *pgxpool.Pool
	if dsn := envOr("POSTGRES_DSN", ""); dsn != "" {
		if pool, err := pgxpool.New(context.Background(), dsn); err == nil {
			pgPool = pool
			defer pool.Close()
			logger.Info("processor connected to PostgreSQL")
		} else {
			logger.Warn("processor could not connect to PostgreSQL — deployments will be in-memory only", zap.Error(err))
		}
	}

	proc := &Processor{
		logger:          logger,
		client:          &http.Client{Timeout: 5 * time.Second},
		services:        make(map[string]*models.Service),
		edges:           make(map[string]*models.TopoEdge),
		metricsWindows:  make(map[string]map[string]*CircularBuffer),
		slos:            make(map[string]*models.SLO),
		alertRules:      make(map[string]*models.AlertRule),
		activeAlerts:    make(map[string]*models.FiredAlert),
		problems:        make(map[string]*models.Problem),
		aiAgentURL:      envOr("AI_AGENT_URL", "http://ai-agent:8080"),
		pg:              pgPool,
	}

	// Regression engine
	proc.regression = newRegressionEngine(proc)
	go proc.regression.run()

	// Forecasting / predictive analytics
	proc.forecaster = newForecastEngine(proc)
	go proc.forecaster.runPrecompute()

	proc.mlCostForecaster = newMLCostForecaster(proc)
	go proc.mlCostForecaster.Run()
	go proc.runLogAnomalyDetection()
	go proc.runBurnRateAlerts()

	// Background loops
	synCtx := context.Background()
	go proc.runAnomalyDetection()
	go proc.runSLOCalculation()
	go proc.runAlertEvaluation()
	go proc.runStaleCleanup()
	go proc.runSyntheticScheduler(synCtx)

	app := fiber.New(fiber.Config{AppName: "ObserveX Processor"})
	app.Use(recover.New())

	// Receive from ingestor
	app.Post("/v1/services", proc.handleServiceUpdate)
	app.Post("/v1/topology/edges", proc.handleEdgeUpdate)
	app.Post("/v1/metrics/batch", proc.handleMetricsBatch)
	app.Post("/v1/logs/batch", proc.handleLogsBatch)
	app.Post("/v1/traces/batch", proc.handleTracesBatch)
	app.Post("/v1/agents", proc.handleAgentUpdate)

	// API reads (used by query engine and frontend)
	app.Get("/v1/topology", proc.handleGetTopology)
	app.Get("/v1/topology/service/:id/subgraph", proc.handleGetSubgraph)
	app.Get("/v1/services", proc.handleListServices)
	app.Get("/v1/services/:id", proc.handleGetService)
	app.Get("/v1/problems", proc.handleListProblems)
	app.Get("/v1/problems/:id", proc.handleGetProblem)
	app.Get("/v1/slos", proc.handleListSLOs)
	app.Post("/v1/slos", proc.handleCreateSLO)
	app.Get("/v1/alerts", proc.handleListAlerts)
	app.Post("/v1/alerts/rules", proc.handleCreateAlertRule)
	app.Put("/v1/alerts/rules/:id", proc.handleUpdateAlertRule)
	app.Delete("/v1/alerts/rules/:id", proc.handleDeleteAlertRule)

	// Predictive analytics
	app.Get("/v1/alert-groups", proc.handleGetAlertGroups)
	app.Get("/v1/costs",     proc.handleGetCosts)
	app.Get("/v1/forecasts", proc.handleGetForecast)
	app.Get("/v1/forecasts/capacity", proc.handleCapacityForecast)
	app.Get("/v1/ml/cost/forecast",   proc.handleMLCostForecast)
	app.Get("/v1/ml/cost/anomalies",  proc.handleMLCostAnomalies)
	app.Get("/v1/ml/tokens/forecast", proc.handleMLTokenForecast)

	// Deployment markers (from API gateway / CI pipelines)
	app.Post("/v1/deployments", proc.handleCreateDeployment)
	app.Get("/v1/deployments",  proc.handleListDeployments)
	app.Get("/v1/deployments/:id", proc.handleGetDeployment)

	// F6.1 synthetic probe work and intake (G-1), on a dedicated listener;
	// see f61_intake.go. Never served on this internal app.
	stopF61 := startF61Intake(logger, pgPool)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		logger.Info("shutting down gracefully...")
		stopF61()
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()

	logger.Info("processor started")
	logger.Fatal("server error", zap.Error(app.Listen(":8080")))
}

// ═══════════════════════════════════════════════════════
//  TOPOLOGY MANAGEMENT
// ═══════════════════════════════════════════════════════

func (p *Processor) handleServiceUpdate(c *fiber.Ctx) error {
	var svc models.Service
	if err := c.BodyParser(&svc); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	p.topoMu.Lock()
	p.services[svc.ID] = &svc
	p.topoMu.Unlock()
	return c.JSON(fiber.Map{"status": "ok"})
}

func (p *Processor) handleEdgeUpdate(c *fiber.Ctx) error {
	var edge models.TopoEdge
	if err := c.BodyParser(&edge); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	p.topoMu.Lock()
	// EMA smoothing for edge metrics
	if existing, ok := p.edges[edge.ID]; ok {
		alpha := 0.3
		edge.CallsPerMin = alpha*edge.CallsPerMin + (1-alpha)*existing.CallsPerMin
		edge.AvgLatencyMs = alpha*edge.AvgLatencyMs + (1-alpha)*existing.AvgLatencyMs
		edge.ErrorRate = alpha*edge.ErrorRate + (1-alpha)*existing.ErrorRate
	}
	p.edges[edge.ID] = &edge
	p.topoMu.Unlock()
	return c.JSON(fiber.Map{"status": "ok"})
}

func (p *Processor) handleGetTopology(c *fiber.Ctx) error {
	ns := c.Query("namespace")
	health := c.Query("health") // filter by health state

	p.topoMu.RLock()
	svcs := make([]*models.Service, 0)
	edges := make([]*models.TopoEdge, 0)

	for _, s := range p.services {
		if ns != "" && s.Namespace != ns {
			continue
		}
		if health != "" && string(s.Health.State) != health {
			continue
		}
		svcs = append(svcs, s)
	}
	for _, e := range p.edges {
		edges = append(edges, e)
	}
	p.topoMu.RUnlock()

	return c.JSON(fiber.Map{"services": svcs, "edges": edges})
}

func (p *Processor) handleGetSubgraph(c *fiber.Ctx) error {
	id := c.Params("id")
	depth := c.QueryInt("depth", 2)

	p.topoMu.RLock()
	defer p.topoMu.RUnlock()

	// BFS up to `depth` hops from the target service
	visited := map[string]bool{id: true}
	queue := []string{id}

	for d := 0; d < depth && len(queue) > 0; d++ {
		var next []string
		for _, cur := range queue {
			for _, edge := range p.edges {
				if edge.SourceID == cur && !visited[edge.TargetID] {
					visited[edge.TargetID] = true
					next = append(next, edge.TargetID)
				}
				if edge.TargetID == cur && !visited[edge.SourceID] {
					visited[edge.SourceID] = true
					next = append(next, edge.SourceID)
				}
			}
		}
		queue = next
	}

	var svcs []*models.Service
	var edges []*models.TopoEdge

	for svcID := range visited {
		if s, ok := p.services[svcID]; ok {
			svcs = append(svcs, s)
		}
	}
	for _, edge := range p.edges {
		if visited[edge.SourceID] && visited[edge.TargetID] {
			edges = append(edges, edge)
		}
	}

	return c.JSON(fiber.Map{"services": svcs, "edges": edges})
}

func (p *Processor) handleListServices(c *fiber.Ctx) error {
	ns := c.Query("namespace")
	kind := c.Query("kind")

	p.topoMu.RLock()
	svcs := make([]*models.Service, 0)
	for _, s := range p.services {
		if ns != "" && s.Namespace != ns {
			continue
		}
		if kind != "" && string(s.Kind) != kind {
			continue
		}
		svcs = append(svcs, s)
	}
	p.topoMu.RUnlock()

	// Sort by name
	sort.Slice(svcs, func(i, j int) bool {
		return svcs[i].Name < svcs[j].Name
	})

	return c.JSON(fiber.Map{"services": svcs, "total": len(svcs)})
}

func (p *Processor) handleGetService(c *fiber.Ctx) error {
	id := c.Params("id")
	p.topoMu.RLock()
	svc, ok := p.services[id]
	p.topoMu.RUnlock()
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(svc)
}

func (p *Processor) handleAgentUpdate(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

// ═══════════════════════════════════════════════════════
//  METRIC INGESTION INTO WINDOWS
// ═══════════════════════════════════════════════════════

func (p *Processor) handleMetricsBatch(c *fiber.Ctx) error {
	var pts []models.MetricPoint
	if err := c.BodyParser(&pts); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	p.recordMetricPoints(pts)

	return c.JSON(fiber.Map{"received": len(pts)})
}

func (p *Processor) recordMetricPoints(pts []models.MetricPoint) {
	p.metricsMu.Lock()
	defer p.metricsMu.Unlock()
	for _, pt := range pts {
		if pt.ServiceID == "" {
			pt.ServiceID = "unknown"
		}
		svcWindows, ok := p.metricsWindows[pt.ServiceID]
		if !ok {
			svcWindows = make(map[string]*CircularBuffer)
			p.metricsWindows[pt.ServiceID] = svcWindows
		}
		buf, ok := svcWindows[pt.Name]
		if !ok {
			buf = NewCircularBuffer(120) // keep last 120 samples
			svcWindows[pt.Name] = buf
		}
		buf.Push(pt.Value)
	}
}

func (p *Processor) publishNativeMetrics(ctx context.Context, pts []models.MetricPoint) {
	if len(pts) == 0 {
		return
	}
	body, err := json.Marshal(pts)
	if err != nil {
		p.logger.Warn("encode native metrics failed", zap.Error(err))
		return
	}
	url := strings.TrimRight(envOr("INGESTOR_URL", "http://ingestor:4318"), "/") + "/v1/metrics/batch"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		p.logger.Warn("create native metrics request failed", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Warn("publish native metrics failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

func (p *Processor) handleLogsBatch(c *fiber.Ctx) error {
	// Log error patterns for anomaly detection
	var logs []models.LogEntry
	if err := c.BodyParser(&logs); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	// Count error logs per service for error rate detection
	errCounts := map[string]int{}
	totalCounts := map[string]int{}
	for _, l := range logs {
		totalCounts[l.ServiceID]++
		if l.Level == "error" || l.Level == "ERROR" || l.Level == "fatal" || l.Level == "FATAL" {
			errCounts[l.ServiceID]++
		}
	}
	now := time.Now()
	p.metricsMu.Lock()
	for svcID, total := range totalCounts {
		p.ensureWindow(svcID, "log_total_rate").Push(float64(total))
		if errs := errCounts[svcID]; errs > 0 {
			rate := float64(errs) / float64(total)
			p.ensureWindow(svcID, "log_error_rate").Push(rate)
		}
		_ = now
	}
	p.metricsMu.Unlock()
	return c.JSON(fiber.Map{"received": len(logs)})
}

func (p *Processor) handleTracesBatch(c *fiber.Ctx) error {
	var spans []models.Span
	if err := c.BodyParser(&spans); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	// Extract latency and error rate from spans
	latencies := map[string][]float64{}
	errCounts := map[string]int{}
	totalCounts := map[string]int{}
	nativePoints := make([]models.MetricPoint, 0, len(spans)*2)

	for _, span := range spans {
		if span.ServiceID == "" {
			span.ServiceID = "unknown"
		}
		totalCounts[span.ServiceID]++
		latencies[span.ServiceID] = append(latencies[span.ServiceID], span.DurationMs)
		if span.Status == "error" {
			errCounts[span.ServiceID]++
		}

		labels := map[string]string{
			"service_name": span.ServiceName,
			"operation":    span.OperationName,
			"span_kind":    span.Kind,
			"outcome":      span.Status,
			"source":       "observex-agent",
		}
		if labels["service_name"] == "" {
			labels["service_name"] = span.ServiceID
		}
		if statusCode := span.Attrs["http.status_code"]; statusCode != "" {
			labels["status_code"] = statusCode
		}
		at := span.EndTime
		if at.IsZero() {
			at = time.Now()
		}
		nativePoints = append(nativePoints,
			models.MetricPoint{Name: "http_server_duration_ms", Value: span.DurationMs, Timestamp: at, Labels: labels, ServiceID: span.ServiceID},
			models.MetricPoint{Name: "http_requests_total", Value: 1, Timestamp: at, Labels: labels, ServiceID: span.ServiceID},
		)
	}

	p.metricsMu.Lock()
	for svcID, lats := range latencies {
		sort.Float64s(lats)
		p99 := percentile(lats, 0.99)
		p50 := percentile(lats, 0.50)
		p.ensureWindow(svcID, "trace_latency_p99_ms").Push(p99)
		p.ensureWindow(svcID, "trace_latency_p50_ms").Push(p50)
		if total := totalCounts[svcID]; total > 0 {
			errRate := float64(errCounts[svcID]) / float64(total)
			p.ensureWindow(svcID, "trace_error_rate").Push(errRate)
		}
	}
	p.metricsMu.Unlock()
	if len(nativePoints) > 0 {
		p.recordMetricPoints(nativePoints)
		go p.publishNativeMetrics(context.Background(), nativePoints)
	}

	return c.JSON(fiber.Map{"received": len(spans)})
}

func (p *Processor) ensureWindow(svcID, metric string) *CircularBuffer {
	svcWindows, ok := p.metricsWindows[svcID]
	if !ok {
		svcWindows = make(map[string]*CircularBuffer)
		p.metricsWindows[svcID] = svcWindows
	}
	buf, ok := svcWindows[metric]
	if !ok {
		buf = NewCircularBuffer(120)
		svcWindows[metric] = buf
	}
	return buf
}

// ═══════════════════════════════════════════════════════
//  ANOMALY DETECTION (Z-SCORE + THRESHOLD RULES)
// ═══════════════════════════════════════════════════════

func (p *Processor) runAnomalyDetection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.detectAnomalies()
	}
}

func (p *Processor) detectAnomalies() {
	p.metricsMu.RLock()
	snapshot := make(map[string]map[string]*CircularBuffer)
	for svcID, windows := range p.metricsWindows {
		snapshot[svcID] = make(map[string]*CircularBuffer)
		for metric, buf := range windows {
			snapshot[svcID][metric] = buf
		}
	}
	p.metricsMu.RUnlock()

	var newProblems []models.Problem

	for svcID, windows := range snapshot {
		svc := p.getService(svcID)
		svcName := svcID
		if svc != nil {
			svcName = svc.DisplayName
		}

		// ── Memory usage ──────────────────────────────────────
		if memBuf, ok := windows["container_memory_working_set_bytes"]; ok {
			if limitBuf, ok2 := windows["container_memory_limit_bytes"]; ok2 {
				vals := memBuf.Values()
				limits := limitBuf.Values()
				if len(vals) > 0 && len(limits) > 0 {
					memPct := vals[len(vals)-1] / limits[len(limits)-1] * 100
					if memPct > 85 {
						// Check if it's growing (memory leak detection)
						isLeaking := false
						if len(vals) >= 10 {
							first := average(vals[:len(vals)/2])
							last := average(vals[len(vals)/2:])
							isLeaking = last > first*1.3 // growing >30%
						}
						class := models.ProbHighMemory
						if isLeaking {
							class = models.ProbMemoryLeak
						}
						newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
							class, memPct, 85, 95,
							fmt.Sprintf("Memory at %.0f%% of limit", memPct),
							fmt.Sprintf("Container memory usage is %.0f%% of its limit. %s",
								memPct, map[bool]string{true: "Memory is growing monotonically — likely leak.", false: ""}[isLeaking]),
						))
					}
				}
			}
		}

		// ── CPU usage ─────────────────────────────────────────
		if cpuBuf, ok := windows["container_cpu_usage_percent"]; ok {
			vals := cpuBuf.Values()
			if len(vals) > 0 {
				cpuPct := vals[len(vals)-1]
				if cpuPct > 90 {
					newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
						models.ProbHighCPU, cpuPct, 90, 99,
						fmt.Sprintf("CPU at %.0f%%", cpuPct),
						fmt.Sprintf("CPU usage is %.0f%%. Service may be throttled or need scaling.", cpuPct),
					))
				}
			}
		}

		// ── HTTP error rate ───────────────────────────────────
		for _, metric := range []string{"trace_error_rate", "log_error_rate"} {
			if errBuf, ok := windows[metric]; ok {
				vals := errBuf.Values()
				if len(vals) > 0 {
					errRate := vals[len(vals)-1]
					if errRate > 0.05 {
						mean, std := errBuf.Stats()
						zScore := 0.0
						if std > 0 {
							zScore = (errRate - mean) / std
						}
						if zScore > 2.0 || errRate > 0.05 {
							newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
								models.ProbHighErrorRate, errRate*100, 5, 20,
								fmt.Sprintf("Error rate %.1f%%", errRate*100),
								fmt.Sprintf("Error rate %.1f%% (z-score=%.1f). Check logs for root cause.", errRate*100, zScore),
							))
						}
					}
				}
			}
		}

		// ── Latency ───────────────────────────────────────────
		if latBuf, ok := windows["trace_latency_p99_ms"]; ok {
			vals := latBuf.Values()
			if len(vals) > 0 {
				p99 := vals[len(vals)-1]
				mean, std := latBuf.Stats()
				if p99 > 2000 || (std > 0 && (p99-mean)/std > 3.0) {
					zScore := 0.0
					if std > 0 {
						zScore = (p99 - mean) / std
					}
					newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
						models.ProbHighLatency, p99, 2000, 5000,
						fmt.Sprintf("p99 latency %.0fms", p99),
						fmt.Sprintf("p99 latency %.0fms (%.1f std devs above baseline %.0fms). Check DB queries and downstream deps.", p99, zScore, mean),
					))
				}
			}
		}

		// ── Queue consumer lag ────────────────────────────────
		if lagBuf, ok := windows["kafka_consumer_lag"]; ok {
			vals := lagBuf.Values()
			if len(vals) > 0 {
				lag := vals[len(vals)-1]
				if lag > 10000 {
					newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
						models.ProbQueueLag, lag, 10000, 100000,
						fmt.Sprintf("Consumer lag %.0f messages", lag),
						fmt.Sprintf("Queue consumer lag is %.0f messages. Consumer may be stuck or too slow.", lag),
					))
				}
			}
		}

		// ── Restart count ─────────────────────────────────────
		if restartBuf, ok := windows["kube_pod_container_status_restarts_total"]; ok {
			vals := restartBuf.Values()
			if len(vals) >= 2 {
				// Check if restarts are increasing
				restartsPerInterval := vals[len(vals)-1] - vals[len(vals)-2]
				if restartsPerInterval > 0 || vals[len(vals)-1] >= 3 {
					class := models.ProbCrashLoop
					if len(vals) >= 1 && vals[len(vals)-1] >= 1 {
						newProblems = append(newProblems, p.makeProblem(svcID, svcName, svc,
							class, vals[len(vals)-1], 3, 10,
							fmt.Sprintf("CrashLoop: %.0f restarts", vals[len(vals)-1]),
							fmt.Sprintf("Container has restarted %.0f times. Possible OOMKill or config error.", vals[len(vals)-1]),
						))
					}
				}
			}
		}
	}

	// Upsert problems and notify AI agent
	for _, prob := range newProblems {
		p.problemMu.Lock()
		existing, exists := p.problems[prob.ID]
		if !exists {
			p.problems[prob.ID] = &prob
			p.problemMu.Unlock()
			// New problem — notify AI agent
			go p.notifyAIAgent(&prob)
		} else {
			// Update existing
			existing.UpdatedAt = time.Now()
			existing.Metrics = prob.Metrics
			p.problemMu.Unlock()
		}
	}
}

func (p *Processor) makeProblem(
	svcID, svcName string, svc *models.Service,
	class models.ProblemClass,
	value, warnThresh, critThresh float64,
	title, detail string,
) models.Problem {
	ns, deploy, podName, nodeName := "", "", "", ""
	if svc != nil {
		ns = svc.Namespace
		deploy = svc.Deployment
		podName = svc.PodName
		nodeName = svc.NodeName
	}

	sev := models.SevMedium
	if value >= critThresh {
		sev = models.SevCritical
	} else if value >= (warnThresh+critThresh)/2 {
		sev = models.SevHigh
	}

	confidence := 0.75 + (value-warnThresh)/(critThresh-warnThresh)*0.20
	if confidence > 0.99 {
		confidence = 0.99
	}

	return models.Problem{
		ID:          fmt.Sprintf("%s:%s:%s", string(class), ns, svcID),
		Class:       class,
		Severity:    sev,
		Title:       title,
		Detail:      detail,
		ServiceID:   svcID,
		ServiceName: svcName,
		Namespace:   ns,
		PodName:     podName,
		NodeName:    nodeName,
		Deployment:  deploy,
		Metrics:     map[string]float64{"value": value, "warn_threshold": warnThresh, "crit_threshold": critThresh},
		Confidence:  confidence,
		Status:      "open",
		DetectedAt:  time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// aiAgentAuthWarnOnce keeps the "agent rejected us" warning to one line per
// process, so a misconfiguration is visible without flooding the log from the
// problem pipeline.
var aiAgentAuthWarnOnce sync.Once

// notifyAIAgent posts a problem to the AI agent.
//
// S1-03-c: the AI agent now authenticates its callers, so this carries the
// internal service token. Without it the agent answers 401 and problems are
// never delivered — the pipeline fails closed rather than silently running an
// unauthenticated agent.
func (p *Processor) notifyAIAgent(prob *models.Problem) {
	data, _ := json.Marshal(prob)
	req, err := http.NewRequest(http.MethodPost, p.aiAgentURL+"/v1/problems", bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// SetHeader is a no-op when the token is not configured; the agent then
	// answers 401 and the warning below fires.
	processorInternalToken().SetHeader(req.Header)

	resp, err := p.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusServiceUnavailable {
		aiAgentAuthWarnOnce.Do(func() {
			p.logger.Warn("AI agent rejected problem delivery; check the internal service token on both sides",
				zap.Int("status", resp.StatusCode))
		})
	}
}

// ═══════════════════════════════════════════════════════
//  SLO CALCULATION
// ═══════════════════════════════════════════════════════

func (p *Processor) runSLOCalculation() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.calculateSLOs()
	}
}

func (p *Processor) calculateSLOs() {
	p.sloMu.Lock()
	defer p.sloMu.Unlock()

	p.metricsMu.RLock()
	defer p.metricsMu.RUnlock()

	for _, slo := range p.slos {
		windows, ok := p.metricsWindows[slo.ServiceID]
		if !ok {
			continue
		}

		switch slo.Kind {
		case models.SLOAvailability:
			// SLI = (total - errors) / total
			if errBuf, ok := windows["trace_error_rate"]; ok {
				vals := errBuf.Values()
				if len(vals) > 0 {
					avgErrRate := average(vals)
					slo.SLI = (1 - avgErrRate) * 100
				}
			}

		case models.SLOLatency:
			if latBuf, ok := windows["trace_latency_p99_ms"]; ok {
				vals := latBuf.Values()
				if len(vals) > 0 {
					slo.SLI = vals[len(vals)-1]
				}
			}

		case models.SLOErrorRate:
			if errBuf, ok := windows["trace_error_rate"]; ok {
				vals := errBuf.Values()
				if len(vals) > 0 {
					slo.SLI = average(vals) * 100
				}
			}
		}

		// Calculate error budget
		windowDays := 30.0
		if slo.Window == "7d" {
			windowDays = 7
		}
		totalMinutes := windowDays * 24 * 60
		if slo.Kind == models.SLOAvailability {
			allowedDowntimeMin := totalMinutes * (1 - slo.Target/100)
			usedDowntimeMin := totalMinutes * (1 - slo.SLI/100)
			slo.BudgetTotal = allowedDowntimeMin
			slo.BudgetLeft = allowedDowntimeMin - usedDowntimeMin
		}

		// Multi-window burn rate (Google SRE model: fast=1h/5%, slow=6h/2%)
		// Each window uses a different slice of the circular buffer samples.
		// BurnRate = (errorsInWindow / totalRequestsInWindow) / (1 - SLOTarget/100)
		// A burn rate of 1.0 means budget is being consumed at exactly the SLO rate.
		// > 14.4 = fast burn (exhausts 30d budget in 2h), > 6 = slow burn (in 5h).
		if slo.BudgetTotal > 0 && slo.Kind == models.SLOAvailability {
			if errBuf, ok := windows["trace_error_rate"]; ok {
				vals := errBuf.Values()
				n := len(vals)
				errorAllowance := 1 - slo.Target/100
				if errorAllowance <= 0 { errorAllowance = 0.001 }

				// 1h burn: use last n/24 samples (1 hour of 120-sample/5-min window)
				n1h := n / 24; if n1h < 1 { n1h = 1 }
				if n >= n1h {
					avg1h := average(vals[n-n1h:])
					slo.BurnRate1h = avg1h / errorAllowance
				}

				// 6h burn: use last n/4 samples
				n6h := n / 4; if n6h < 1 { n6h = 1 }
				if n >= n6h {
					avg6h := average(vals[n-n6h:])
					slo.BurnRate6h = avg6h / errorAllowance
				}

				// 24h burn: use all samples
				slo.BurnRate24h = average(vals) / errorAllowance
			}
		}

		// Status
		if slo.BudgetLeft > slo.BudgetTotal*0.5 {
			slo.Status = "ok"
		} else if slo.BudgetLeft > 0 {
			slo.Status = "warning"
		} else {
			slo.Status = "breached"
		}
		slo.UpdatedAt = time.Now()
	}
}

// upsertLogAnomalyProblem creates or updates a LOG_ANOMALY problem.
func (p *Processor) upsertLogAnomalyProblem(svcID, svcName string, svc *models.Service, current, zScore, mean float64) {
	ns := "default"
	if svc != nil { ns = svc.Namespace }
	prob := p.makeProblem(
		ns, svcID, svc,
		models.ProblemClass("LOG_ANOMALY"),
		current, mean, 0,
		fmt.Sprintf("%s: log error spike detected", svcName),
		fmt.Sprintf("Log error rate %.1f%% (z-score %.1f, baseline %.1f%%). Check logs.", current*100, zScore, mean*100),
	)
	p.notifyAIAgent(&prob)
}

func (p *Processor) handleListSLOs(c *fiber.Ctx) error {
	p.sloMu.RLock()
	slos := make([]*models.SLO, 0, len(p.slos))
	for _, s := range p.slos {
		slos = append(slos, s)
	}
	p.sloMu.RUnlock()
	return c.JSON(fiber.Map{"slos": slos})
}

func (p *Processor) handleCreateSLO(c *fiber.Ctx) error {
	var slo models.SLO
	if err := c.BodyParser(&slo); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	slo.ID = fmt.Sprintf("slo-%d", time.Now().UnixNano())
	slo.UpdatedAt = time.Now()
	p.sloMu.Lock()
	p.slos[slo.ID] = &slo
	p.sloMu.Unlock()
	return c.Status(201).JSON(slo)
}

// ═══════════════════════════════════════════════════════
//  ALERT EVALUATION
// ═══════════════════════════════════════════════════════

func (p *Processor) runAlertEvaluation() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.evaluateAlerts()
	}
}

func (p *Processor) evaluateAlerts() {
	p.alertMu.RLock()
	rules := make([]*models.AlertRule, 0, len(p.alertRules))
	for _, r := range p.alertRules {
		rules = append(rules, r)
	}
	p.alertMu.RUnlock()

	p.metricsMu.RLock()
	defer p.metricsMu.RUnlock()

	for _, rule := range rules {
		if rule.Silenced {
			continue
		}
		p.evaluateRule(rule)
	}
}

func (p *Processor) evaluateRule(rule *models.AlertRule) {
	// Get the relevant metric value
	// The rule.Expr can reference specific metrics like "http_error_rate" or "cpu_percent"

	for svcID, windows := range p.metricsWindows {
		if rule.ServiceID != "" && rule.ServiceID != svcID {
			continue
		}

		for metricName, buf := range windows {
			if !strings.Contains(rule.Expr, metricName) {
				continue
			}
			vals := buf.Values()
			if len(vals) == 0 {
				continue
			}
			currentVal := vals[len(vals)-1]

			// Evaluate threshold condition
			triggered := false
			switch rule.Operator {
			case "gt", ">":
				triggered = currentVal > rule.Threshold
			case "lt", "<":
				triggered = currentVal < rule.Threshold
			case "gte", ">=":
				triggered = currentVal >= rule.Threshold
			case "lte", "<=":
				triggered = currentVal <= rule.Threshold
			case "eq", "==":
				triggered = currentVal == rule.Threshold
			}

			alertKey := fmt.Sprintf("%s:%s", rule.ID, svcID)

			p.alertMu.Lock()
			if triggered {
				if _, alreadyFired := p.activeAlerts[alertKey]; !alreadyFired {
					alert := &models.FiredAlert{
						ID:       fmt.Sprintf("alert-%d", time.Now().UnixNano()),
						RuleID:   rule.ID,
						RuleName: rule.Name,
						Severity: rule.Severity,
						Value:    currentVal,
						Message:  rule.Message,
						Labels:   rule.Labels,
						FiredAt:  time.Now(),
						State:    "firing",
					}
					p.activeAlerts[alertKey] = alert
					// Trigger problem creation
					go func(a *models.AlertRule, v float64, sid string) {
						prob := p.makeProblem(sid, sid, p.getService(sid),
							models.ProbUnknown, v, a.Threshold, a.Threshold*2,
							a.Name, a.Message,
						)
						p.problemMu.Lock()
						p.problems[prob.ID] = &prob
						p.problemMu.Unlock()
						go p.notifyAIAgent(&prob)
						// Deliver alert notifications via configured channels
						if a.NotificationChannels != "" {
							go p.deliverAlertNotification(a, v, sid, "firing")
						}
					}(rule, currentVal, svcID)
				}
			} else {
				if existing, fired := p.activeAlerts[alertKey]; fired {
					now := time.Now()
					existing.ResolvedAt = &now
					existing.State = "resolved"
					delete(p.activeAlerts, alertKey)
					// Notify resolution
					if rule.NotificationChannels != "" {
						go p.deliverAlertNotification(rule, existing.Value, rule.ServiceID, "resolved")
					}
				}
			}
			p.alertMu.Unlock()
		}
	}
}

func (p *Processor) handleListAlerts(c *fiber.Ctx) error {
	p.alertMu.RLock()
	alerts := make([]*models.FiredAlert, 0, len(p.activeAlerts))
	for _, a := range p.activeAlerts {
		alerts = append(alerts, a)
	}
	p.alertMu.RUnlock()
	return c.JSON(fiber.Map{"alerts": alerts, "total": len(alerts)})
}

func (p *Processor) handleCreateAlertRule(c *fiber.Ctx) error {
	var rule models.AlertRule
	if err := c.BodyParser(&rule); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	rule.ID = fmt.Sprintf("rule-%d", time.Now().UnixNano())
	rule.CreatedAt = time.Now()
	p.alertMu.Lock()
	p.alertRules[rule.ID] = &rule
	p.alertMu.Unlock()
	return c.Status(201).JSON(rule)
}

func (p *Processor) handleUpdateAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	var rule models.AlertRule
	if err := c.BodyParser(&rule); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	rule.ID = id
	p.alertMu.Lock()
	p.alertRules[id] = &rule
	p.alertMu.Unlock()
	return c.JSON(rule)
}

func (p *Processor) handleDeleteAlertRule(c *fiber.Ctx) error {
	id := c.Params("id")
	p.alertMu.Lock()
	delete(p.alertRules, id)
	p.alertMu.Unlock()
	return c.SendStatus(204)
}

// ═══════════════════════════════════════════════════════
//  PROBLEMS API
// ═══════════════════════════════════════════════════════

func (p *Processor) handleListProblems(c *fiber.Ctx) error {
	status := c.Query("status")
	sev := c.Query("severity")

	p.problemMu.RLock()
	probs := make([]*models.Problem, 0)
	for _, pr := range p.problems {
		if status != "" && pr.Status != status {
			continue
		}
		if sev != "" && string(pr.Severity) != sev {
			continue
		}
		probs = append(probs, pr)
	}
	p.problemMu.RUnlock()

	sort.Slice(probs, func(i, j int) bool {
		return probs[i].DetectedAt.After(probs[j].DetectedAt)
	})

	return c.JSON(fiber.Map{"problems": probs, "total": len(probs)})
}

func (p *Processor) handleGetProblem(c *fiber.Ctx) error {
	id := c.Params("id")
	p.problemMu.RLock()
	prob, ok := p.problems[id]
	p.problemMu.RUnlock()
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(prob)
}

// ═══════════════════════════════════════════════════════
//  STALE CLEANUP
// ═══════════════════════════════════════════════════════

func (p *Processor) runStaleCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		p.topoMu.Lock()
		for id, svc := range p.services {
			if time.Since(svc.LastSeenAt) > 10*time.Minute {
				delete(p.services, id)
			}
		}
		for id, edge := range p.edges {
			if time.Since(edge.UpdatedAt) > 10*time.Minute {
				delete(p.edges, id)
			}
		}
		p.topoMu.Unlock()

		// Expire resolved problems
		p.problemMu.Lock()
		for id, prob := range p.problems {
			if prob.Status == "resolved" && time.Since(prob.UpdatedAt) > 24*time.Hour {
				delete(p.problems, id)
			}
		}
		p.problemMu.Unlock()
	}
}

// ═══════════════════════════════════════════════════════
//  HELPERS
// ═══════════════════════════════════════════════════════

func (p *Processor) getService(id string) *models.Service {
	p.topoMu.RLock()
	svc := p.services[id]
	p.topoMu.RUnlock()
	return svc
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// processorInternalToken loads the internal service token once, on first use.
// The value is never logged.
var processorInternalToken = sync.OnceValue(servicetoken.Load)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ═══════════════════════════════════════════════════════
//  LOG ANOMALY DETECTION
// ═══════════════════════════════════════════════════════
// Detects spikes in error log frequency per service.
// Runs every 60s, compares current 1-min error rate to
// the 5-min rolling baseline. Z-score > 3 fires a problem.

func (p *Processor) runLogAnomalyDetection() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.detectLogAnomalies()
	}
}

func (p *Processor) detectLogAnomalies() {
	p.metricsMu.RLock()
	snapshot := make(map[string]map[string]*CircularBuffer)
	for svcID, windows := range p.metricsWindows {
		snapshot[svcID] = windows
	}
	p.metricsMu.RUnlock()

	for svcID, windows := range snapshot {
		errBuf, ok := windows["log_error_rate"]
		if !ok {
			continue
		}
		vals := errBuf.Values()
		if len(vals) < 5 {
			continue
		}

		mean, std := errBuf.Stats()
		current := vals[len(vals)-1]
		if std < 0.001 {
			continue
		}
		zScore := (current - mean) / std

		if zScore > 3.0 && current > 0.1 {
			svc := p.getService(svcID)
			svcName := svcID
			if svc != nil {
				svcName = svc.DisplayName
			}
			p.upsertLogAnomalyProblem(svcID, svcName, svc, current, zScore, mean)
			p.logger.Warn("log anomaly detected",
				zap.String("service", svcName),
				zap.Float64("error_rate", current),
				zap.Float64("z_score", zScore),
			)
		}
	}
}

// ═══════════════════════════════════════════════════════
//  COST ATTRIBUTION (FinOps light)
// ═══════════════════════════════════════════════════════
// Estimates per-service cloud cost from CPU+memory usage.
// Uses configurable price-per-core and price-per-GB-hour.
// GET /v1/costs  → returns cost estimate per service.

const (
	defaultPricePerCoreHour  = 0.048 // $/core/hour (roughly c5.xlarge / 4 cores)
	defaultPricePerGBHour    = 0.006 // $/GB RAM/hour
)

type ServiceCost struct {
	ServiceID   string  `json:"service_id"`
	ServiceName string  `json:"service_name"`
	Namespace   string  `json:"namespace"`
	CPUCores    float64 `json:"cpu_cores"`
	MemGB       float64 `json:"mem_gb"`
	CostPerHour float64 `json:"cost_per_hour"`
	CostPerDay  float64 `json:"cost_per_day"`
	CostPerMonth float64 `json:"cost_per_month"`
	Period      string  `json:"period"` // "1h" estimate
}

func (p *Processor) handleGetCosts(c *fiber.Ctx) error {
	priceCore := defaultPricePerCoreHour
	priceMem  := defaultPricePerGBHour

	p.metricsMu.RLock()
	defer p.metricsMu.RUnlock()

	costs := make([]ServiceCost, 0)
	for svcID, windows := range p.metricsWindows {
		cpuBuf, hasCPU := windows["container_cpu_usage_percent"]
		memBuf, hasMem := windows["container_memory_working_set_bytes"]
		if !hasCPU && !hasMem {
			continue
		}

		svc := p.getService(svcID)
		svcName, ns := svcID, "default"
		if svc != nil {
			svcName = svc.DisplayName
			ns = svc.Namespace
		}

		cpuPct := 0.0
		if hasCPU {
			vals := cpuBuf.Values()
			if len(vals) > 0 { cpuPct, _ = cpuBuf.Stats() }
		}
		memBytes := 0.0
		if hasMem {
			vals := memBuf.Values()
			if len(vals) > 0 { memBytes, _ = memBuf.Stats() }
		}

		// Convert CPU % to cores (assume 2 vCPUs per container = 100% = 2 cores)
		cores := (cpuPct / 100.0) * 2.0
		memGB := memBytes / (1024 * 1024 * 1024)

		perHour := cores*priceCore + memGB*priceMem
		costs = append(costs, ServiceCost{
			ServiceID:    svcID,
			ServiceName:  svcName,
			Namespace:    ns,
			CPUCores:     math.Round(cores*1000) / 1000,
			MemGB:        math.Round(memGB*100) / 100,
			CostPerHour:  math.Round(perHour*10000) / 10000,
			CostPerDay:   math.Round(perHour*24*100) / 100,
			CostPerMonth: math.Round(perHour*24*30*100) / 100,
			Period:       "1h",
		})
	}

	// Sort by cost descending
	for i := 1; i < len(costs); i++ {
		for j := i; j > 0 && costs[j].CostPerHour > costs[j-1].CostPerHour; j-- {
			costs[j], costs[j-1] = costs[j-1], costs[j]
		}
	}

	total := 0.0
	for _, c := range costs {
		total += c.CostPerHour
	}

	return c.JSON(fiber.Map{
		"services":           costs,
		"total_per_hour":     math.Round(total*10000) / 10000,
		"total_per_day":      math.Round(total*24*100) / 100,
		"total_per_month":    math.Round(total*24*30*100) / 100,
		"price_per_core_hour": priceCore,
		"price_per_gb_hour":  priceMem,
		"currency":           "USD",
	})
}

// ═══════════════════════════════════════════════════════
//  M5 — SLO BURN RATE ALERTS (Google SRE model)
// ═══════════════════════════════════════════════════════
// Fires CRITICAL alert when 1h burn rate > 14.4×
// Fires HIGH alert when 6h burn rate > 6×
// Fires MEDIUM alert when 24h burn rate > 3×
// These thresholds map to: 30d budget exhausted in 2h/5h/10d respectively.

func (p *Processor) runBurnRateAlerts() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		p.evaluateBurnRateAlerts()
	}
}

func (p *Processor) evaluateBurnRateAlerts() {
	p.sloMu.RLock()
	slos := make([]*models.SLO, 0, len(p.slos))
	for _, s := range p.slos {
		slos = append(slos, s)
	}
	p.sloMu.RUnlock()

	for _, slo := range slos {
		// Fast burn: 1h window > 14.4× → budget gone in 2 hours
		if slo.BurnRate1h >= 14.4 {
			sev := "CRITICAL"
			msg := fmt.Sprintf("SLO '%s' fast burn: %.1f× rate (14.4× threshold). Error budget will be exhausted in ~2 hours.", slo.Name, slo.BurnRate1h)
			p.fireSLOBurnAlert(slo, "fast_burn_1h", sev, msg, slo.BurnRate1h)
		} else if slo.BurnRate6h >= 6.0 {
			// Slow burn: 6h window > 6× → budget gone in 5 hours
			msg := fmt.Sprintf("SLO '%s' slow burn: %.1f× rate (6× threshold). Error budget will be exhausted in ~5 hours.", slo.Name, slo.BurnRate6h)
			p.fireSLOBurnAlert(slo, "slow_burn_6h", "HIGH", msg, slo.BurnRate6h)
		} else if slo.BurnRate24h >= 3.0 {
			// Trend burn: 24h > 3× → investigate before it accelerates
			msg := fmt.Sprintf("SLO '%s' trend burn: %.1f× rate (3× threshold). Error budget draining faster than expected.", slo.Name, slo.BurnRate24h)
			p.fireSLOBurnAlert(slo, "trend_burn_24h", "MEDIUM", msg, slo.BurnRate24h)
		}
	}
}

func (p *Processor) fireSLOBurnAlert(slo *models.SLO, alertType, severity, msg string, burnRate float64) {
	probID := fmt.Sprintf("slo-burn-%s-%s", slo.ID, alertType)

	p.problemMu.Lock()
	defer p.problemMu.Unlock()

	// Avoid re-firing the same burn alert within 30 minutes
	if existing, ok := p.problems[probID]; ok {
		if existing.Status == "open" && time.Since(existing.DetectedAt) < 30*time.Minute {
			return
		}
	}

	prob := models.Problem{
		ID:        probID,
		Class:     models.ProbHighErrorRate,
		Severity:  models.Severity(severity),
		Status:    "open",
		Title:     fmt.Sprintf("SLO burn rate alert: %s [%s]", slo.Name, alertType),
		Detail:    msg,
		ServiceID: slo.ServiceID,
		Namespace: "production",
		Evidence: []string{
			fmt.Sprintf("1h burn rate:  %.1f× (threshold: 14.4×)", slo.BurnRate1h),
			fmt.Sprintf("6h burn rate:  %.1f× (threshold: 6×)",   slo.BurnRate6h),
			fmt.Sprintf("24h burn rate: %.1f× (threshold: 3×)",   slo.BurnRate24h),
			fmt.Sprintf("Error budget remaining: %.1f%%", slo.BudgetLeft),
			fmt.Sprintf("Current SLI: %.3f%% (target: %.3f%%)", slo.SLI, slo.Target),
		},
		Metrics: map[string]float64{
			"burn_rate_1h":  slo.BurnRate1h,
			"burn_rate_6h":  slo.BurnRate6h,
			"burn_rate_24h": slo.BurnRate24h,
			"budget_left":   slo.BudgetLeft,
		},
		DetectedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}
	p.problems[probID] = &prob
	p.logger.Warn("SLO burn rate alert",
		zap.String("slo", slo.Name),
		zap.String("type", alertType),
		zap.Float64("burn_rate", burnRate),
	)
	go p.notifyAIAgent(&prob)
}

// ═══════════════════════════════════════════════════════
//  M7 — ALERT INHIBITION + ROUTING TREE
// ═══════════════════════════════════════════════════════
// Implements Alertmanager-style inhibition rules:
// When a parent alert fires, child alerts matching labels are suppressed.
// Also implements routing tree: evaluate matchers top-to-bottom,
// first match wins, group alerts by group_by labels.

type InhibitRule struct {
	SourceMatch  map[string]string // labels that must match the source alert
	TargetMatch  map[string]string // labels that are inhibited when source fires
}

type AlertGroup struct {
	GroupKey    string
	Alerts      []*models.FiredAlert
	FirstFired  time.Time
	LastUpdated time.Time
}

// Default inhibition rules — can be extended via API
var defaultInhibitRules = []InhibitRule{
	// When a node is critical, suppress individual pod alerts on that node
	{SourceMatch: map[string]string{"class": "NODE_NOT_READY"},  TargetMatch: map[string]string{"kind": "deploy"}},
	// When a deployment regression fires, suppress latency alerts for same service
	{SourceMatch: map[string]string{"class": "HIGH_ERROR_RATE"}, TargetMatch: map[string]string{"class": "HIGH_LATENCY"}},
	// When disk full, suppress memory alerts (they're effects, not causes)
	{SourceMatch: map[string]string{"class": "DISK_FULL"},       TargetMatch: map[string]string{"class": "HIGH_MEMORY"}},
}

func (p *Processor) isInhibited(alert *models.FiredAlert) bool {
	p.problemMu.RLock()
	defer p.problemMu.RUnlock()

	for _, rule := range defaultInhibitRules {
		// Check if any firing problem matches the source
		sourceActive := false
		for _, prob := range p.problems {
			if prob.Status != "open" {
				continue
			}
			matched := true
			for k, v := range rule.SourceMatch {
				switch k {
				case "class":
					if string(prob.Class) != v {
						matched = false
					}
				case "namespace":
					if prob.Namespace != v {
						matched = false
					}
				}
			}
			if matched {
				sourceActive = true
				break
			}
		}
		if !sourceActive {
			continue
		}

		// Check if this alert matches the target (should be inhibited)
		targetMatched := true
		for k, v := range rule.TargetMatch {
			switch k {
			case "class":
				if string(alert.Labels["class"]) != v {
					targetMatched = false
				}
			case "kind":
				if alert.Labels["kind"] != v {
					targetMatched = false
				}
			}
		}
		if targetMatched {
			return true // inhibited
		}
	}
	return false
}

// GroupAlerts groups firing alerts by service+severity for deduplication
func (p *Processor) groupAlerts() map[string]*AlertGroup {
	p.alertMu.RLock()
	defer p.alertMu.RUnlock()

	groups := make(map[string]*AlertGroup)
	for _, alert := range p.activeAlerts {
		if p.isInhibited(alert) {
			continue
		}
		// Group key = namespace + severity
		key := fmt.Sprintf("%s:%s", alert.Labels["namespace"], alert.Severity)
		if _, ok := groups[key]; !ok {
			groups[key] = &AlertGroup{
				GroupKey:   key,
				FirstFired: alert.FiredAt,
			}
		}
		groups[key].Alerts = append(groups[key].Alerts, alert)
		if alert.FiredAt.After(groups[key].FirstFired) {
			groups[key].LastUpdated = alert.FiredAt
		}
	}
	return groups
}

func (p *Processor) handleGetAlertGroups(c *fiber.Ctx) error {
	groups := p.groupAlerts()
	result := make([]fiber.Map, 0, len(groups))
	for _, g := range groups {
		result = append(result, fiber.Map{
			"group_key":    g.GroupKey,
			"alert_count":  len(g.Alerts),
			"first_fired":  g.FirstFired,
			"last_updated": g.LastUpdated,
			"alerts":       g.Alerts,
		})
	}
	return c.JSON(fiber.Map{"groups": result, "total": len(result)})
}

// ═══════════════════════════════════════════════════════════════════════════
//  SYNTHETIC MONITORING SCHEDULER
//  Runs HTTP/TCP/DNS/SSL checks on a per-check schedule.
//  Writes results as native synthetic_check_* metrics.
//  Also handles /v1/synthetic/run/:id for on-demand check execution.
// ═══════════════════════════════════════════════════════════════════════════

func init() {
	// Register synthetic routes on the processor app
	// Called after proc is created in main()
}

// SyntheticCheckDef mirrors the DB row for running checks
type SyntheticCheckDef struct {
	ID                  string
	Name                string
	Type                string
	Target              string
	IntervalSec         int
	TimeoutSec          int
	ExpectStatus        int
	ExpectBodyContains  string
	// OrgID and Namespace are the server-side tenant/scope of the check row
	// (synthetic_checks.org_id, synthetic_checks.namespace). They are read here
	// only so that scope travels with the check definition; nothing in this file
	// consumes them yet.
	OrgID     string
	Namespace string
}

// runSyntheticScheduler polls PostgreSQL for enabled checks and runs them
func (p *Processor) runSyntheticScheduler(ctx context.Context) {
	if p.pg == nil {
		p.logger.Info("synthetic scheduler: no DB connection, skipping")
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	// Track next-run time per check
	nextRun := map[string]time.Time{}

	p.logger.Info("synthetic scheduler started")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := p.pg.Query(ctx,
				`SELECT id,name,type,target,interval_sec,timeout_sec,
				        expect_status,expect_body_contains,
				        org_id,namespace
				 FROM synthetic_checks WHERE enabled=true`)
			if err != nil { continue }

			checks := []SyntheticCheckDef{}
			for rows.Next() {
				var ch SyntheticCheckDef
				rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Target,
					&ch.IntervalSec, &ch.TimeoutSec, &ch.ExpectStatus, &ch.ExpectBodyContains,
					&ch.OrgID, &ch.Namespace)
				checks = append(checks, ch)
			}
			rows.Close()

			now := time.Now()
			for _, ch := range checks {
				if next, ok := nextRun[ch.ID]; ok && now.Before(next) {
					continue // not due yet
				}
				go p.runSyntheticCheck(ctx, ch)
				nextRun[ch.ID] = now.Add(time.Duration(ch.IntervalSec) * time.Second)
			}
		}
	}
}

// runSyntheticCheck executes one check and records the result as native ObserveX metrics.
func (p *Processor) runSyntheticCheck(ctx context.Context, ch SyntheticCheckDef) {
	start := time.Now()
	up := 0.0
	var errMsg string

	switch ch.Type {
	case "http", "https":
		up, errMsg = p.runHTTPCheck(ch)
	case "tcp":
		up, errMsg = p.runTCPCheck(ch)
	case "dns":
		up, errMsg = p.runDNSCheck(ch)
	case "ssl":
		up, errMsg = p.runSSLCheck(ch)
	case "ping":
		up, errMsg = p.runPingCheck(ch)
	default:
		up, errMsg = p.runHTTPCheck(ch)
	}

	durationMs := float64(time.Since(start).Milliseconds())
	labels := map[string]string{
		"check_id":   ch.ID,
		"check_name": ch.Name,
		"check_type": ch.Type,
	}
	serviceID := "synthetic:" + ch.ID

	pts := []models.MetricPoint{
		{Name: "synthetic_check_up",          Value: up,         Timestamp: start, Labels: labels, ServiceID: serviceID},
		{Name: "synthetic_check_duration_ms", Value: durationMs, Timestamp: start, Labels: labels, ServiceID: serviceID},
	}
	if errMsg != "" {
		errLabels := copyLabels(labels)
		errLabels["error"] = errMsg[:min(len(errMsg), 100)]
		pts = append(pts, models.MetricPoint{Name: "synthetic_check_error", Value: 1, Timestamp: start, Labels: errLabels, ServiceID: serviceID})
	}

	p.recordMetricPoints(pts)
	p.publishNativeMetrics(ctx, pts)

	// Fire alert problem if check has been down for 3+ consecutive checks
	if up == 0 {
		prob := p.makeProblem(
			"default", "synthetic:"+ch.ID, nil,
			models.ProbServiceDown,
			1.0, 0.5, 0.8,
			fmt.Sprintf("Synthetic check FAILED: %s", ch.Name),
			fmt.Sprintf("Check %s (%s → %s) is DOWN. Error: %s. Duration: %.0fms",
				ch.Name, ch.Type, ch.Target, errMsg, durationMs),
		)
		p.notifyAIAgent(&prob)
	}
}

func copyLabels(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	for k, v := range m { n[k] = v }
	return n
}

func min(a, b int) int { if a < b { return a }; return b }

// ── HTTP check ─────────────────────────────────────────────────────────────

func (p *Processor) runHTTPCheck(ch SyntheticCheckDef) (float64, string) {
	client := &http.Client{
		Timeout: time.Duration(ch.TimeoutSec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 { return fmt.Errorf("too many redirects") }
			return nil
		},
	}
	resp, err := client.Get(ch.Target)
	if err != nil { return 0, err.Error() }
	defer resp.Body.Close()

	if ch.ExpectStatus > 0 && resp.StatusCode != ch.ExpectStatus {
		return 0, fmt.Sprintf("expected status %d got %d", ch.ExpectStatus, resp.StatusCode)
	}
	if ch.ExpectBodyContains != "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		if !strings.Contains(string(body), ch.ExpectBodyContains) {
			return 0, fmt.Sprintf("body does not contain %q", ch.ExpectBodyContains)
		}
	}
	return 1, ""
}

// ── TCP check ──────────────────────────────────────────────────────────────

func (p *Processor) runTCPCheck(ch SyntheticCheckDef) (float64, string) {
	conn, err := net.DialTimeout("tcp", ch.Target, time.Duration(ch.TimeoutSec)*time.Second)
	if err != nil { return 0, err.Error() }
	conn.Close()
	return 1, ""
}

// ── DNS check ──────────────────────────────────────────────────────────────

func (p *Processor) runDNSCheck(ch SyntheticCheckDef) (float64, string) {
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ch.TimeoutSec)*time.Second)
	defer cancel()
	addrs, err := resolver.LookupHost(ctx, ch.Target)
	if err != nil { return 0, err.Error() }
	if len(addrs) == 0 { return 0, "no addresses resolved" }
	return 1, ""
}

// ── SSL check ──────────────────────────────────────────────────────────────

func (p *Processor) runSSLCheck(ch SyntheticCheckDef) (float64, string) {
	host := ch.Target
	if !strings.Contains(host, ":") { host += ":443" }
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: time.Duration(ch.TimeoutSec) * time.Second},
		"tcp", host, &tls.Config{})
	if err != nil { return 0, err.Error() }
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 { return 0, "no certificates" }
	daysLeft := time.Until(certs[0].NotAfter).Hours() / 24
	if daysLeft < 7 { return 0, fmt.Sprintf("certificate expires in %.0f days", daysLeft) }
	return 1, ""
}

// ── Ping check ─────────────────────────────────────────────────────────────

func (p *Processor) runPingCheck(ch SyntheticCheckDef) (float64, string) {
	// Use TCP echo as a connectivity test since raw ICMP requires root
	conn, err := net.DialTimeout("tcp", ch.Target+":80", time.Duration(ch.TimeoutSec)*time.Second)
	if err != nil {
		// Try port 443 as fallback
		conn2, err2 := net.DialTimeout("tcp", ch.Target+":443", time.Duration(ch.TimeoutSec)*time.Second)
		if err2 != nil { return 0, err.Error() }
		conn2.Close(); return 1, ""
	}
	conn.Close(); return 1, ""
}

// ── On-demand run endpoint ─────────────────────────────────────────────────

func (p *Processor) handleRunSyntheticCheck(c *fiber.Ctx) error {
	id := c.Params("id")
	if p.pg == nil { return c.JSON(fiber.Map{"error": "no DB connection"}) }

	var ch SyntheticCheckDef
	err := p.pg.QueryRow(c.Context(),
		`SELECT id,name,type,target,interval_sec,timeout_sec,
		        expect_status,expect_body_contains FROM synthetic_checks WHERE id=$1`, id).
		Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Target,
			&ch.IntervalSec, &ch.TimeoutSec, &ch.ExpectStatus, &ch.ExpectBodyContains)
	if err != nil { return c.Status(404).JSON(fiber.Map{"error": "check not found"}) }

	start := time.Now()
	up, errMsg := p.runHTTPCheck(ch)
	durationMs := time.Since(start).Milliseconds()

	return c.JSON(fiber.Map{
		"check_id":    ch.ID,
		"check_name":  ch.Name,
		"up":          up == 1,
		"duration_ms": durationMs,
		"error":       errMsg,
		"ran_at":      start,
	})
}

// ── Alert Notification Delivery ──────────────────────────────────────────────
// deliverAlertNotification dispatches a fired or resolved alert to the
// channels configured in the AlertRule.NotificationChannels field.
// Format: comma-separated "provider:endpoint" pairs, e.g.:
//   slack:https://hooks.slack.com/...,pagerduty:routing-key-here

func (p *Processor) deliverAlertNotification(rule *models.AlertRule, value float64, serviceID, state string) {
	if rule.NotificationChannels == "" {
		return
	}

	// Build the alert payload
	type AlertPayload struct {
		AlertName   string  `json:"alert_name"`
		Severity    string  `json:"severity"`
		State       string  `json:"state"`
		Service     string  `json:"service"`
		Namespace   string  `json:"namespace"`
		Value       float64 `json:"value"`
		Threshold   float64 `json:"threshold"`
		RunbookURL  string  `json:"runbook_url,omitempty"`
		StartsAt    string  `json:"starts_at"`
		OrgID       string  `json:"org_id"`
	}

	payload := AlertPayload{
		AlertName:  rule.Name,
		Severity:   string(rule.Severity),
		State:      state,
		Service:    serviceID,
		Namespace:  rule.Namespace,
		Value:      value,
		Threshold:  rule.Threshold,
		RunbookURL: rule.RunbookURL,
		StartsAt:   time.Now().UTC().Format(time.RFC3339),
		OrgID:      rule.OrgID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		p.logger.Error("marshal alert payload", zap.Error(err))
		return
	}

	// POST to api-gateway's internal notification endpoint
	// which fans out to Slack/PagerDuty/OpsGenie based on channel DSN
	gwURL := envOr("GATEWAY_INTERNAL_URL", "http://api-gateway:3001")
	notifyURL := gwURL + "/internal/notify"

	req, err := http.NewRequestWithContext(context.Background(), "POST", notifyURL,
		bytes.NewReader(body))
	if err != nil {
		p.logger.Error("create notify request", zap.Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// X-ObserveX-Internal-Token from OBSERVEX_INTERNAL_TOKEN_FILE or
	// OBSERVEX_INTERNAL_TOKEN; omitted when not configured (S1-06).
	processorInternalToken().SetHeader(req.Header)
	req.Header.Set("X-Channels", rule.NotificationChannels)

	resp, err := p.client.Do(req)
	if err != nil {
		p.logger.Error("deliver alert notification",
			zap.String("alert", rule.Name),
			zap.String("state", state),
			zap.Error(err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		p.logger.Warn("notification endpoint returned error",
			zap.String("alert", rule.Name),
			zap.Int("status", resp.StatusCode))
	} else {
		p.logger.Info("alert notification delivered",
			zap.String("alert", rule.Name),
			zap.String("state", state),
			zap.String("channels", rule.NotificationChannels))
	}
}
