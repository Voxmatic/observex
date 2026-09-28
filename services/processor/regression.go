// services/processor/regression.go
//
// Release Regression Detection Engine.
//
// How it works:
//
//  1. POST /v1/deployments from the API gateway (or CI pipeline) records a
//     DeploymentMarker. The processor stores it in-memory in deploymentsMap
//     and forwards it to the API gateway for PostgreSQL persistence.
//
//  2. A background goroutine (runRegressionAnalysis) checks every 60 seconds
//     for deployments that are:
//       - status = "pending"
//       - DeployedAt >= 30 minutes ago   (the "after" window is complete)
//
//  3. For each such deployment the engine reads native ObserveX metrics from
//     ClickHouse for:
//
//     BEFORE window: [DeployedAt - 30m, DeployedAt]
//     AFTER  window: [DeployedAt, DeployedAt + 30m]
//
//     Two metrics per service:
//       a) P99 latency from http_server_duration_ms samples
//
//       b) Error rate (%) from http_requests_total samples tagged outcome=error
//
//     The engine uses a bounded quantile/aggregate query per window.
//
//  4. Delta % = (after - before) / before * 100 (positive = worse for latency
//     and error rate, negative = better).
//
//  5. Decision rules:
//     - If P99 delta > +20% OR error-rate delta > +20% → REGRESSION
//       Creates a Problem (ProbRegression) and notifies the AI agent.
//     - If P99 delta < -10% AND error-rate delta < +5%  → IMPROVED
//     - Otherwise → OK
//
//  6. The result is written back to the API gateway at
//     PUT /v1/deployments/:id for PostgreSQL persistence and forwarded
//     to the frontend WebSocket hub.
//
// Insufficient data handling:
//   If native storage returns fewer than 3 data points for a window
//   the window is considered unreliable and the deployment is marked
//   status="ok" with a note in the problem detail. This prevents false
//   positives for brand-new services with sparse metrics.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/observex/platform/pkg/models"
)

// ── Constants ─────────────────────────────────────────────────────────────

const (
	// How long to wait after a deploy before analysing. Must collect a
	// full 30-minute "after" window.
	regressionWindowMin = 30

	// Minimum data points required in each window for a reliable comparison.
	// With step=60s over 30 minutes we expect up to 30 points.
	minDataPoints = 3

	// Regression thresholds (percentage change, positive = worse)
	latencyRegressionPct  = 20.0 // P99 latency worsened ≥ 20%
	errorRateRegressionPct = 20.0 // Error rate worsened ≥ 20 percentage points
	improvementPct        = 10.0  // Both improved ≥ 10%
)

// ── In-process deployment registry ───────────────────────────────────────
// Maps deployment ID → DeploymentMarker while pending analysis.
// Entries are removed after analysis completes.

type regressionEngine struct {
	p           *Processor
	metricStore nativeMetricStore

	mu          sync.Mutex
	deployments map[string]*models.DeploymentMarker // id → marker
}

func newRegressionEngine(p *Processor) *regressionEngine {
	return &regressionEngine{
		p:           p,
		metricStore: newNativeMetricStore(p),
		deployments: make(map[string]*models.DeploymentMarker),
	}
}

// recordDeployment is called when POST /v1/deployments is received.
func (e *regressionEngine) recordDeployment(d *models.DeploymentMarker) {
	e.mu.Lock()
	e.deployments[d.ID] = d
	e.mu.Unlock()
}

// run is the background analysis loop. Checks every minute for
// deployments whose 30-minute "after" window has elapsed.
func (e *regressionEngine) run() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		e.analyseReady()
	}
}

func (e *regressionEngine) analyseReady() {
	cutoff := time.Now().UTC().Add(-regressionWindowMin * time.Minute)

	e.mu.Lock()
	var ready []*models.DeploymentMarker
	for _, d := range e.deployments {
		if d.Status == models.DeployPending && d.DeployedAt.Before(cutoff) {
			ready = append(ready, d)
		}
	}
	e.mu.Unlock()

	for _, d := range ready {
		e.analyse(d)
	}
}

// analyse runs the full before/after metric comparison for one deployment.
func (e *regressionEngine) analyse(d *models.DeploymentMarker) {
	log := e.p.logger.With(
		zap.String("deployment", d.ID),
		zap.String("service", d.ServiceName),
		zap.String("version", d.Version),
	)
	log.Info("analysing deployment for regression")

	before := timeRange{
		start: d.DeployedAt.Add(-regressionWindowMin * time.Minute),
		end:   d.DeployedAt,
	}
	after := timeRange{
		start: d.DeployedAt,
		end:   d.DeployedAt.Add(regressionWindowMin * time.Minute),
	}

	// ── Read P99 latency ────────────────────────────────────────────────

	p99Before, p99BeforeN, err := e.metricStore.durationP99(context.Background(), d.ServiceID, d.ServiceName, before.start, before.end)
	if err != nil {
		log.Warn("native metric query failed for P99 before window", zap.Error(err))
	}
	p99After, p99AfterN, err := e.metricStore.durationP99(context.Background(), d.ServiceID, d.ServiceName, after.start, after.end)
	if err != nil {
		log.Warn("native metric query failed for P99 after window", zap.Error(err))
	}

	// ── Read error rate ─────────────────────────────────────────────────

	errBefore, _, err := e.metricStore.requestErrorRate(context.Background(), d.ServiceID, d.ServiceName, before.start, before.end)
	if err != nil {
		log.Warn("native metric query failed for error-rate before window", zap.Error(err))
	}
	errAfter, _, err := e.metricStore.requestErrorRate(context.Background(), d.ServiceID, d.ServiceName, after.start, after.end)
	if err != nil {
		log.Warn("native metric query failed for error-rate after window", zap.Error(err))
	}

	// ── Compute deltas ──────────────────────────────────────────────────

	d.P99Before = round2(p99Before)
	d.P99After  = round2(p99After)
	d.ErrRateBefore = round2(errBefore)
	d.ErrRateAfter  = round2(errAfter)

	// delta% = (after - before) / before * 100; positive means worse
	d.P99DeltaPct     = deltaPct(p99Before, p99After)
	d.ErrRateDeltaPct = deltaPct(errBefore, errAfter)

	sufficientData := p99BeforeN >= minDataPoints && p99AfterN >= minDataPoints

	log.Info("regression analysis complete",
		zap.Float64("p99_before_ms", d.P99Before),
		zap.Float64("p99_after_ms",  d.P99After),
		zap.Float64("p99_delta_pct", d.P99DeltaPct),
		zap.Float64("err_before_pct", d.ErrRateBefore),
		zap.Float64("err_after_pct",  d.ErrRateAfter),
		zap.Float64("err_delta_pct",  d.ErrRateDeltaPct),
		zap.Bool("sufficient_data", sufficientData),
	)

	// ── Classify ────────────────────────────────────────────────────────

	if !sufficientData {
		// Not enough metric data — mark OK conservatively rather than
		// firing false positive regressions for new/low-traffic services.
		d.Status = models.DeployOK
		log.Info("insufficient data points, marking OK",
			zap.Int("p99_before_points", p99BeforeN),
			zap.Int("p99_after_points",  p99AfterN),
		)
	} else if d.P99DeltaPct >= latencyRegressionPct || d.ErrRateDeltaPct >= errorRateRegressionPct {
		d.Status = models.DeployRegression
		d.ProblemID = e.fireRegressionProblem(d)
	} else if d.P99DeltaPct <= -improvementPct && d.ErrRateDeltaPct <= 5 {
		d.Status = models.DeployImproved
	} else {
		d.Status = models.DeployOK
	}

	// ── Persist results ─────────────────────────────────────────────────

	// Forward updated marker to API gateway for PostgreSQL persistence
	go e.persistResult(d)

	// Remove from in-memory map
	e.mu.Lock()
	delete(e.deployments, d.ID)
	e.mu.Unlock()
}

// ── Regression problem ────────────────────────────────────────────────────

func (e *regressionEngine) fireRegressionProblem(d *models.DeploymentMarker) string {
	// Build human-readable evidence
	var evidence []string
	if d.P99DeltaPct >= latencyRegressionPct {
		evidence = append(evidence, fmt.Sprintf(
			"P99 latency increased %.1f%% (%.0fms → %.0fms)",
			d.P99DeltaPct, d.P99Before, d.P99After,
		))
	}
	if d.ErrRateDeltaPct >= errorRateRegressionPct {
		evidence = append(evidence, fmt.Sprintf(
			"Error rate increased %.1f pp (%.2f%% → %.2f%%)",
			d.ErrRateDeltaPct, d.ErrRateBefore, d.ErrRateAfter,
		))
	}

	detail := fmt.Sprintf(
		"Deploy %s → %s at %s caused metric regression. "+
			"Comparing 30-minute windows before and after deployment.",
		d.PrevVersion, d.Version,
		d.DeployedAt.Format("15:04 UTC"),
	)

	probID := fmt.Sprintf("regression:%s:%s", d.ServiceName, d.ID)

	sev := models.SevHigh
	if d.P99DeltaPct >= 50 || d.ErrRateDeltaPct >= 50 {
		sev = models.SevCritical
	}

	prob := models.Problem{
		ID:          probID,
		Class:       models.ProbRegression,
		Severity:    sev,
		Title:       fmt.Sprintf("Regression detected after deploy %s (%s)", d.Version, d.ServiceName),
		Detail:      detail,
		RootCause:   fmt.Sprintf("Deploy of version %s at %s", d.Version, d.DeployedAt.Format(time.RFC3339)),
		ServiceName: d.ServiceName,
		Namespace:   d.Namespace,
		Deployment:  d.ServiceName,
		Metrics: map[string]float64{
			"p99_before_ms":      d.P99Before,
			"p99_after_ms":       d.P99After,
			"p99_delta_pct":      d.P99DeltaPct,
			"err_rate_before":    d.ErrRateBefore,
			"err_rate_after":     d.ErrRateAfter,
			"err_rate_delta_pct": d.ErrRateDeltaPct,
		},
		Evidence:   evidence,
		Confidence: regressionConfidence(d),
		Status:     "open",
		DetectedAt: time.Now(),
		UpdatedAt:  time.Now(),
	}

	e.p.problemMu.Lock()
	e.p.problems[prob.ID] = &prob
	e.p.problemMu.Unlock()

	go e.p.notifyAIAgent(&prob)

	e.p.logger.Warn("🔴 regression detected",
		zap.String("service", d.ServiceName),
		zap.String("version", d.Version),
		zap.Float64("p99_delta_pct", d.P99DeltaPct),
		zap.Float64("err_delta_pct", d.ErrRateDeltaPct),
	)

	return probID
}

func regressionConfidence(d *models.DeploymentMarker) float64 {
	// Higher confidence when both metrics regress simultaneously
	maxDelta := math.Max(d.P99DeltaPct/latencyRegressionPct, d.ErrRateDeltaPct/errorRateRegressionPct)
	conf := 0.70 + math.Min(0.28, (maxDelta-1)*0.15)
	return round2(conf)
}

// ── Persist result back to API gateway ───────────────────────────────────

func (e *regressionEngine) persistResult(d *models.DeploymentMarker) {
	gwURL := envOr("API_GATEWAY_URL", "http://api-gateway:3001")
	data, _ := json.Marshal(d)
	resp, err := e.p.client.Post(
		gwURL+"/api/v1/deployments/"+d.ID+"/result",
		"application/json",
		marshalReader(data),
	)
	if err != nil {
		e.p.logger.Warn("failed to persist regression result",
			zap.String("id", d.ID), zap.Error(err))
		return
	}
	resp.Body.Close()
}

// ── Native metric query helpers ───────────────────────────────────────────

type timeRange struct {
	start time.Time
	end   time.Time
}

// ── Handler ───────────────────────────────────────────────────────────────
// handleDeployments is registered on the processor's Fiber app.

// ── Math helpers ──────────────────────────────────────────────────────────

func deltaPct(before, after float64) float64 {
	if before <= 0 {
		if after > 0 {
			return 100 // went from zero to non-zero
		}
		return 0
	}
	return round2((after - before) / before * 100)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func marshalReader(data []byte) io.Reader {
	return bytesReader(data)
}

type bytesReader []byte

func (b bytesReader) Read(p []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, io.EOF
	}
	n = copy(p, b)
	return n, nil
}

// ── HTTP handlers ──────────────────────────────────────────────────────────
// These are methods on Processor so they satisfy the Fiber handler signature
// and have access to the regression engine via p.regression.

func (p *Processor) handleCreateDeployment(c *fiber.Ctx) error {
	var d models.DeploymentMarker
	if err := c.BodyParser(&d); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate required fields
	if d.ServiceName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "service_name is required"})
	}
	if d.Version == "" {
		return c.Status(400).JSON(fiber.Map{"error": "version is required"})
	}

	// Set defaults
	if d.ID == "" {
		// 8-char hex prefix — unique enough for deployment IDs, human-readable
		d.ID = fmt.Sprintf("deploy-%x", time.Now().UnixNano()&0xFFFFFFFF)
	}
	if d.DeployedAt.IsZero() {
		d.DeployedAt = time.Now().UTC()
	}
	if d.Environment == "" {
		d.Environment = "production"
	}
	d.Status    = models.DeployPending
	d.CreatedAt = time.Now().UTC()
	d.UpdatedAt = d.CreatedAt

	// Enrich with topology data if possible
	if d.ServiceID == "" || d.Namespace == "" {
		p.topoMu.RLock()
		for svcID, svc := range p.services {
			if svc.Name == d.ServiceName || svc.DisplayName == d.ServiceName {
				if d.ServiceID == "" {
					d.ServiceID = svcID
				}
				if d.Namespace == "" {
					d.Namespace = svc.Namespace
				}
				break
			}
		}
		p.topoMu.RUnlock()
	}
	if d.ServiceID == "" {
		d.ServiceID = d.ServiceName
	}

	// Record in regression engine (also snapshots the "before" metrics via VM)
	p.regression.recordDeployment(&d)

	p.logger.Info("deployment marker created",
		zap.String("id", d.ID),
		zap.String("service", d.ServiceName),
		zap.String("version", d.Version),
		zap.String("env", d.Environment),
		zap.Time("deployed_at", d.DeployedAt),
		zap.String("analysis_at", d.DeployedAt.Add(regressionWindowMin*time.Minute).Format(time.RFC3339)),
	)

	return c.Status(201).JSON(&d)
}

func (p *Processor) handleListDeployments(c *fiber.Ctx) error {
	serviceFilter := c.Query("service")
	envFilter     := c.Query("environment")
	statusFilter  := c.Query("status")
	limit         := 50
	fmt.Sscanf(c.Query("limit", "50"), "%d", &limit)
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	p.regression.mu.Lock()
	all := make([]*models.DeploymentMarker, 0, len(p.regression.deployments))
	for _, d := range p.regression.deployments {
		if serviceFilter != "" && d.ServiceName != serviceFilter {
			continue
		}
		if envFilter != "" && d.Environment != envFilter {
			continue
		}
		if statusFilter != "" && string(d.Status) != statusFilter {
			continue
		}
		all = append(all, d)
	}
	p.regression.mu.Unlock()

	// Sort newest-first
	sort.Slice(all, func(i, j int) bool {
		return all[i].DeployedAt.After(all[j].DeployedAt)
	})

	total := len(all)
	if len(all) > limit {
		all = all[:limit]
	}

	return c.JSON(fiber.Map{
		"deployments": all,
		"total":       total,
	})
}

func (p *Processor) handleGetDeployment(c *fiber.Ctx) error {
	id := c.Params("id")

	p.regression.mu.Lock()
	d, ok := p.regression.deployments[id]
	p.regression.mu.Unlock()

	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": "deployment not found"})
	}
	return c.JSON(d)
}
