// services/ingestor/main.go
// ObserveX Ingestor — receives telemetry from all OneAgents via HTTP.
// Routes: metrics -> ObserveX native storage, logs -> Loki, traces -> Tempo,
//         services/topology → PostgreSQL/Neo4j, events → ClickHouse.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/observex/platform/pkg/models"
)

// ═══════════════════════════════════════════════════════
//  CONFIG
// ═══════════════════════════════════════════════════════

type Config struct {
	Port              string
	LokiURL           string // http://loki:3100
	TempoURL          string // http://tempo:4317
	OTELCollectorURL  string // http://otel-collector:4318
	ClickHouseURL     string // http://clickhouse:8123
	ProcessorURL      string // http://processor:8080 — streams events for AI analysis
	// CORSOrigin is the Access-Control-Allow-Origin value for the /v1/rum endpoint.
	// Use "*" to allow any origin (dev) or your app domain for production.
	CORSOrigin        string
	AgentTokenSecret  string
	StaticAgentToken  string
	RequireAgentAuth bool
	AgentUpdateManifest string
}

func loadConfig() Config {
	secret := envOr("AGENT_TOKEN_SECRET", envOr("JWT_SECRET", ""))
	staticToken := joinStaticTokens(
		envOr("OBSERVEX_INGEST_TOKEN", ""),
		readSecretFile(envOr("OBSERVEX_INGEST_TOKEN_FILE", "")),
	)
	return Config{
		Port:               envOr("PORT", "4318"),
		LokiURL:            envOr("LOKI_URL", "http://loki:3100"),
		TempoURL:           envOr("TEMPO_URL", "http://tempo:3200"),
		OTELCollectorURL:   envOr("OTEL_COLLECTOR_URL", "http://otel-collector:4318"),
		ClickHouseURL:      envOr("CLICKHOUSE_URL", "http://clickhouse:8123"),
		ProcessorURL:       envOr("PROCESSOR_URL", "http://processor:8080"),
		CORSOrigin:         envOr("RUM_CORS_ORIGIN", "*"),
		AgentTokenSecret:   secret,
		StaticAgentToken:   staticToken,
		RequireAgentAuth:   envBool("REQUIRE_AGENT_AUTH", secret != "" || staticToken != ""),
		AgentUpdateManifest: envOr("OBSERVEX_AGENT_UPDATE_MANIFEST_JSON", ""),
	}
}

// ═══════════════════════════════════════════════════════
//  INGESTOR
// ═══════════════════════════════════════════════════════

type Ingestor struct {
	cfg     Config
	logger  *zap.Logger
	client  *http.Client

	// In-memory service registry (replicated to processor + query engine)
	svcMu    sync.RWMutex
	services map[string]*models.Service

	// Agent registry
	agentMu sync.RWMutex
	agents  map[string]*models.Agent

	// Metric write buffer (batched native writes to ClickHouse)
	metricBuf   []models.MetricPoint
	metricBufMu sync.Mutex
}

type AgentHeartbeat struct {
	AgentID        string          `json:"agent_id"`
	OrgID          string          `json:"org_id,omitempty"`
	NodeName       string          `json:"node_name"`
	ClusterName    string          `json:"cluster_name"`
	Environment    string          `json:"environment,omitempty"`
	HostGroup      string          `json:"host_group,omitempty"`
	NetworkZone    string          `json:"network_zone,omitempty"`
	MonitoringMode string          `json:"monitoring_mode,omitempty"`
	CollectionMode string          `json:"collection_mode,omitempty"`
	LogMonitoring  bool            `json:"log_monitoring,omitempty"`
	AutoUpdate     bool            `json:"auto_update,omitempty"`
	UpdateChannel  string          `json:"update_channel,omitempty"`
	UpdateState    string          `json:"update_state,omitempty"`
	TargetVersion  string          `json:"target_version,omitempty"`
	Version        string          `json:"version"`
	Status         string          `json:"status"`
	UptimeSec      int64           `json:"uptime_sec"`
	ServiceCount   int             `json:"service_count"`
	FlowCount      int             `json:"flow_count"`
	Features       map[string]bool `json:"features"`
	Capabilities   map[string]bool `json:"capabilities,omitempty"`
	ModuleStatus   map[string]string `json:"module_status,omitempty"`
	Timestamp      time.Time       `json:"timestamp"`
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := loadConfig()
	ing := &Ingestor{
		cfg:      cfg,
		logger:   logger,
		client:   &http.Client{Timeout: 10 * time.Second},
		services: make(map[string]*models.Service),
		agents:   make(map[string]*models.Agent),
	}

	// Start metric buffer flusher
	go ing.flushMetrics()

	app := fiber.New(fiber.Config{AppName: "ObserveX Ingestor"})
	app.Use(recover.New())
	app.Use(decodeGzipBody)
	// CORS required for the browser-based RUM SDK
	app.Use(func(c *fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", ing.cfg.CORSOrigin)
		c.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, X-RUM-App-ID, X-RUM-Token")
		if c.Method() == "OPTIONS" {
			return c.SendStatus(204)
		}
		return c.Next()
	})

	// Security events — from OneAgent eBPF security monitor
	// Protect SaaS ingest endpoints. Browser RUM remains separately keyed.
	app.Use(ing.agentAuthMiddleware())

	app.Post("/v1/security/events", ing.handleSecurityEvents)
	// Image scan results — from Trivy scanner
	app.Post("/v1/security/scans", ing.handleImageScanResults)

	// RUM — called from the browser JS SDK
	app.Post("/v1/rum", ing.handleRUM)
	app.Options("/v1/rum", func(c *fiber.Ctx) error { return c.SendStatus(204) })

	// Agent endpoints
	app.Post("/v1/agents/register", ing.handleAgentRegister)
	app.Post("/v1/agents/:id/heartbeat", ing.handleAgentHeartbeat)
	app.Get("/v1/agent/updates/manifest", ing.handleAgentUpdateManifest)

	// Service topology
	app.Post("/v1/services", ing.handleServiceUpsert)
	app.Post("/v1/topology/edges", ing.handleTopoEdge)

	// Telemetry
	app.Post("/v1/metrics/batch", ing.handleMetricsBatch)
	app.Post("/v1/logs/batch", ing.handleLogsBatch)
	app.Post("/v1/traces/batch", ing.handleTracesBatch)
	app.Post("/v1/profiles", ing.handleProfile)

	// OTEL HTTP receiver (compatible with OTEL SDK)
	app.Post("/v1/metrics", ing.handleOTELMetrics)
	app.Post("/v1/logs", ing.handleOTELLogs)
	app.Post("/v1/traces", ing.handleOTELTraces)

	// Internal reads (used by query engine)
	app.Get("/internal/services", ing.handleListServices)
	app.Get("/internal/agents", ing.handleListAgents)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "ts": time.Now().Unix()})
	})
	app.Get("/v1/self/metrics", ing.handleNativeStatus)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		logger.Info("shutting down gracefully...")
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()

	logger.Info("ingestor started", zap.String("port", cfg.Port))
	logger.Fatal("server error", zap.Error(app.Listen(":"+cfg.Port)))
}

// ─── Agent registration ───────────────────────────────────────────────────────

func decodeGzipBody(c *fiber.Ctx) error {
	if !strings.EqualFold(c.Get("Content-Encoding"), "gzip") {
		return c.Next()
	}
	gr, err := gzip.NewReader(bytes.NewReader(c.Body()))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid gzip request body"})
	}
	defer gr.Close()
	body, err := io.ReadAll(gr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "failed to read gzip request body"})
	}
	c.Request().SetBodyRaw(body)
	c.Request().Header.Del("Content-Encoding")
	return c.Next()
}

func (ing *Ingestor) agentAuthMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		path := c.Path()
		if path == "/health" || path == "/v1/self/metrics" || path == "/v1/rum" || strings.HasPrefix(path, "/internal/") {
			return c.Next()
		}
		if !ing.cfg.RequireAgentAuth && ing.cfg.AgentTokenSecret == "" && ing.cfg.StaticAgentToken == "" {
			return c.Next()
		}

		token := extractAgentToken(c)
		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing agent token"})
		}

		for _, staticToken := range staticAgentTokens(ing.cfg.StaticAgentToken) {
			if subtle.ConstantTimeCompare([]byte(token), []byte(staticToken)) == 1 {
				if orgID := c.Get("X-ObserveX-Org"); orgID != "" {
					c.Locals("org_id", orgID)
				}
				return c.Next()
			}
		}

		if ing.cfg.AgentTokenSecret == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "agent token validation is not configured"})
		}
		claims, err := parseAgentJWT(token, ing.cfg.AgentTokenSecret)
		if err != nil {
			ing.logger.Warn("agent token rejected", zap.Error(err), zap.String("path", path))
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid agent token"})
		}
		if !agentTokenAllowsPath(claims, path) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "agent token scope denied"})
		}
		c.Locals("org_id", claims["org_id"])
		c.Locals("agent_scopes", claims["scopes"])
		return c.Next()
	}
}

func extractAgentToken(c *fiber.Ctx) string {
	auth := c.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if token := c.Get("X-ObserveX-Token"); token != "" {
		return strings.TrimSpace(token)
	}
	return ""
}

func parseAgentJWT(token, secret string) (jwt.MapClaims, error) {
	token = strings.TrimPrefix(token, "oxat_")
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing algorithm: %s", t.Method.Alg())
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	if orgID, _ := claims["org_id"].(string); orgID == "" {
		return nil, fmt.Errorf("missing org_id")
	}
	if typ, _ := claims["typ"].(string); typ != "observex_agent_install" {
		return nil, fmt.Errorf("invalid token type")
	}
	return claims, nil
}

func ingestOrgID(c *fiber.Ctx) string {
	if orgID, ok := c.Locals("org_id").(string); ok && orgID != "" {
		return orgID
	}
	return c.Get("X-ObserveX-Org")
}

func agentTokenAllowsPath(claims jwt.MapClaims, path string) bool {
	required := requiredAgentScope(path)
	if required == "" {
		return true
	}
	for _, scope := range agentScopes(claims["scopes"]) {
		if scope == "*" || scope == required {
			return true
		}
	}
	return false
}

func requiredAgentScope(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/agents"):
		return "agent:write"
	case strings.HasPrefix(path, "/v1/agent/updates"):
		return "agent:update"
	case strings.HasPrefix(path, "/v1/metrics"):
		return "metrics:write"
	case strings.HasPrefix(path, "/v1/logs"):
		return "logs:write"
	case strings.HasPrefix(path, "/v1/traces"):
		return "traces:write"
	case strings.HasPrefix(path, "/v1/services"), strings.HasPrefix(path, "/v1/topology"), strings.HasPrefix(path, "/v1/profiles"):
		return "topology:write"
	case strings.HasPrefix(path, "/v1/security"):
		return "security:write"
	default:
		return ""
	}
}

func agentScopes(v any) []string {
	switch scopes := v.(type) {
	case []string:
		return scopes
	case []any:
		out := make([]string, 0, len(scopes))
		for _, item := range scopes {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func (ing *Ingestor) handleAgentUpdateManifest(c *fiber.Ctx) error {
	raw := strings.TrimSpace(ing.cfg.AgentUpdateManifest)
	if raw == "" {
		return c.SendStatus(fiber.StatusNoContent)
	}
	var manifest struct {
		Version   string   `json:"version"`
		Channel   string   `json:"channel"`
		URL       string   `json:"url"`
		SHA256    string   `json:"sha256"`
		Signature string   `json:"signature"`
		Platforms []string `json:"platforms"`
	}
	if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
		ing.logger.Error("OBSERVEX_AGENT_UPDATE_MANIFEST_JSON is invalid", zap.Error(err))
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "agent update service is not configured"})
	}
	if manifest.Version == "" || manifest.URL == "" || manifest.SHA256 == "" || manifest.Signature == "" {
		ing.logger.Error("agent update manifest is missing signed release fields")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "agent update service is not configured"})
	}
	if requested := c.Query("channel"); requested != "" && manifest.Channel != "" && requested != manifest.Channel {
		return c.SendStatus(fiber.StatusNoContent)
	}
	c.Set("Cache-Control", "no-store")
	return c.JSON(manifest)
}

func (ing *Ingestor) handleAgentRegister(c *fiber.Ctx) error {
	var agent models.Agent
	if err := c.BodyParser(&agent); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if orgID := ingestOrgID(c); orgID != "" {
		agent.OrgID = orgID
	}
	agent.LastSeen = time.Now()
	agent.Status = "active"

	ing.agentMu.Lock()
	ing.agents[agent.ID] = &agent
	ing.agentMu.Unlock()

	ing.logger.Info("agent registered",
		zap.String("id", agent.ID),
		zap.String("node", agent.NodeName),
	)

	// Forward to processor for awareness
	go ing.forwardToProcessor("/v1/agents", agent)

	return c.Status(201).JSON(fiber.Map{"status": "registered"})
}

func (ing *Ingestor) handleAgentHeartbeat(c *fiber.Ctx) error {
	id := c.Params("id")
	var hb AgentHeartbeat
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&hb); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if hb.AgentID == "" {
		hb.AgentID = id
	}
	if id == "" {
		id = hb.AgentID
	}
	if orgID := ingestOrgID(c); orgID != "" {
		hb.OrgID = orgID
	}
	now := time.Now()
	ing.agentMu.Lock()
	agent, ok := ing.agents[id]
	if !ok {
		agent = &models.Agent{ID: id, RegisteredAt: now}
		ing.agents[id] = agent
	}
	ing.applyAgentHeartbeat(agent, hb, now)
	ing.recordAgentUpMetric(agent, now)
	ing.agentMu.Unlock()
	return c.JSON(fiber.Map{"status": "ok"})
}

// ─── Service upsert ───────────────────────────────────────────────────────────

func (ing *Ingestor) applyAgentHeartbeat(agent *models.Agent, hb AgentHeartbeat, now time.Time) {
	if agent.ID == "" {
		agent.ID = hb.AgentID
	}
	if agent.RegisteredAt.IsZero() {
		agent.RegisteredAt = now
	}
	if hb.OrgID != "" {
		agent.OrgID = hb.OrgID
	}
	if hb.NodeName != "" {
		agent.NodeName = hb.NodeName
	}
	if hb.ClusterName != "" {
		agent.ClusterName = hb.ClusterName
	}
	if hb.Environment != "" {
		agent.Environment = hb.Environment
	}
	if hb.HostGroup != "" {
		agent.HostGroup = hb.HostGroup
	}
	if hb.NetworkZone != "" {
		agent.NetworkZone = hb.NetworkZone
	}
	if hb.MonitoringMode != "" {
		agent.MonitoringMode = hb.MonitoringMode
	}
	if hb.CollectionMode != "" {
		agent.CollectionMode = hb.CollectionMode
	}
	if hb.UpdateChannel != "" {
		agent.UpdateChannel = hb.UpdateChannel
	}
	if hb.UpdateState != "" {
		agent.UpdateState = hb.UpdateState
	}
	if hb.TargetVersion != "" {
		agent.TargetVersion = hb.TargetVersion
	}
	if hb.Version != "" {
		agent.Version = hb.Version
	}
	agent.LogMonitoring = hb.LogMonitoring
	agent.AutoUpdate = hb.AutoUpdate
	if hb.ModuleStatus != nil {
		agent.ModuleStatus = hb.ModuleStatus
	}
	if hb.Capabilities != nil {
		agent.Capabilities = hb.Capabilities
	} else if hb.Features != nil {
		agent.Capabilities = hb.Features
	}
	if agent.Capabilities != nil {
		agent.EBPFEnabled = agent.Capabilities["ebpf"]
	}
	if hb.Status != "" {
		agent.Status = hb.Status
	} else {
		agent.Status = "active"
	}
	agent.LastSeen = now
}

func (ing *Ingestor) recordAgentUpMetric(agent *models.Agent, ts time.Time) {
	labels := map[string]string{
		"agent_id": agent.ID,
		"node":     agent.NodeName,
		"cluster":  agent.ClusterName,
		"version":  agent.Version,
	}
	if agent.OrgID != "" {
		labels["org"] = agent.OrgID
	}
	if agent.Environment != "" {
		labels["environment"] = agent.Environment
	}
	if agent.HostGroup != "" {
		labels["host_group"] = agent.HostGroup
	}
	if agent.NetworkZone != "" {
		labels["network_zone"] = agent.NetworkZone
	}
	if agent.MonitoringMode != "" {
		labels["monitoring_mode"] = agent.MonitoringMode
	}
	if agent.CollectionMode != "" {
		labels["collection_mode"] = agent.CollectionMode
	}
	if agent.UpdateChannel != "" {
		labels["update_channel"] = agent.UpdateChannel
	}
	ing.metricBufMu.Lock()
	ing.metricBuf = append(ing.metricBuf, models.MetricPoint{
		Name:      "observex_agent_up",
		Value:     1,
		Timestamp: ts,
		Labels:    labels,
		ServiceID: agent.ID,
	})
	ing.metricBufMu.Unlock()
}

func (ing *Ingestor) handleServiceUpsert(c *fiber.Ctx) error {
	var svc models.Service
	if err := c.BodyParser(&svc); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if svc.Labels == nil {
		svc.Labels = map[string]string{}
	}
	if orgID := ingestOrgID(c); orgID != "" {
		svc.Labels["org"] = orgID
	}
	if agentID := c.Get("X-Agent-ID"); agentID != "" && svc.AgentID == "" {
		svc.AgentID = agentID
	}

	svc.LastSeenAt = time.Now()

	ing.svcMu.Lock()
	_, exists := ing.services[svc.ID]
	if !exists {
		svc.DiscoveredAt = time.Now()
	}
	ing.services[svc.ID] = &svc
	ing.svcMu.Unlock()

	// Write service to ClickHouse for persistence
	go ing.writeServiceToClickHouse(&svc)

	// Forward to processor (for topology building + AI analysis)
	go ing.forwardToProcessor("/v1/services", svc)

	return c.Status(200).JSON(fiber.Map{"id": svc.ID})
}

// ─── Topology edges ───────────────────────────────────────────────────────────

func (ing *Ingestor) handleTopoEdge(c *fiber.Ctx) error {
	var edge models.TopoEdge
	if err := c.BodyParser(&edge); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	edge.UpdatedAt = time.Now()

	// Write to ClickHouse for topology history
	go ing.writeEdgeToClickHouse(&edge)

	// Forward to processor for live topology map
	go ing.forwardToProcessor("/v1/topology/edges", edge)

	return c.JSON(fiber.Map{"status": "ok"})
}

// ─── Metrics batch ────────────────────────────────────────────────────────────

func (ing *Ingestor) handleMetricsBatch(c *fiber.Ctx) error {
	var pts []models.MetricPoint
	if err := c.BodyParser(&pts); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	ing.decorateMetricPoints(c, pts)

	ing.metricBufMu.Lock()
	ing.metricBuf = append(ing.metricBuf, pts...)
	shouldFlush := len(ing.metricBuf) >= 2000
	ing.metricBufMu.Unlock()

	if shouldFlush {
		go ing.doFlushMetrics()
	}

	return c.JSON(fiber.Map{"received": len(pts)})
}

func (ing *Ingestor) handleNativeStatus(c *fiber.Ctx) error {
	ing.agentMu.RLock()
	agentCount := len(ing.agents)
	ing.agentMu.RUnlock()

	ing.svcMu.RLock()
	serviceCount := len(ing.services)
	ing.svcMu.RUnlock()

	ing.metricBufMu.Lock()
	bufferedMetrics := len(ing.metricBuf)
	ing.metricBufMu.Unlock()

	return c.JSON(fiber.Map{
		"agents":                 agentCount,
		"services":               serviceCount,
		"metric_buffer_points":   bufferedMetrics,
		"collection_protocol":    "observex-native-json",
		"checked_at":             time.Now(),
	})
}

func (ing *Ingestor) decorateMetricPoints(c *fiber.Ctx, pts []models.MetricPoint) {
	orgID := ingestOrgID(c)
	agentID := c.Get("X-Agent-ID")
	node := c.Get("X-Node-Name")
	cluster := c.Get("X-Cluster")
	env := c.Get("X-ObserveX-Env")
	for i := range pts {
		if pts[i].Labels == nil {
			pts[i].Labels = map[string]string{}
		}
		if orgID != "" {
			pts[i].Labels["org"] = orgID
		}
		if agentID != "" {
			pts[i].Labels["agent_id"] = agentID
		}
		if node != "" {
			pts[i].Labels["node"] = node
		}
		if cluster != "" {
			pts[i].Labels["cluster"] = cluster
		}
		if env != "" {
			pts[i].Labels["environment"] = env
		}
	}
}

// flushMetrics periodically writes buffered native ObserveX metric points.
func (ing *Ingestor) flushMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ing.doFlushMetrics()
	}
}

func (ing *Ingestor) doFlushMetrics() {
	ing.metricBufMu.Lock()
	if len(ing.metricBuf) == 0 {
		ing.metricBufMu.Unlock()
		return
	}
	pts := ing.metricBuf
	ing.metricBuf = nil
	ing.metricBufMu.Unlock()

	ing.writeMetricsToClickHouse(pts)

	// Also forward to processor for anomaly detection
	go ing.forwardToProcessor("/v1/metrics/batch", pts)
}

func (ing *Ingestor) writeMetricsToClickHouse(pts []models.MetricPoint) {
	if len(pts) == 0 {
		return
	}

	var sb strings.Builder
	for _, pt := range pts {
		if pt.Timestamp.IsZero() {
			pt.Timestamp = time.Now()
		}
		labels, _ := json.Marshal(pt.Labels)
		sb.WriteString("INSERT INTO metrics (name, value, service_id, labels, timestamp) VALUES ")
		sb.WriteString(fmt.Sprintf("('%s',%g,'%s','%s','%s')",
			esc(pt.Name),
			pt.Value,
			esc(pt.ServiceID),
			esc(string(labels)),
			pt.Timestamp.Format("2006-01-02 15:04:05"),
		))
		if err := ing.clickhouseQueryErr(sb.String()); err != nil {
			ing.logger.Debug("clickhouse metric write error", zap.Error(err), zap.String("metric", pt.Name))
		}
		sb.Reset()
	}
}

// ─── Logs batch ───────────────────────────────────────────────────────────────

func (ing *Ingestor) handleLogsBatch(c *fiber.Ctx) error {
	var logs []models.LogEntry
	if err := c.BodyParser(&logs); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	go ing.writeLogsToLoki(logs)
	go ing.forwardToProcessor("/v1/logs/batch", logs)

	return c.JSON(fiber.Map{"received": len(logs)})
}

// writeLogsToLoki uses Loki's HTTP push API
func (ing *Ingestor) writeLogsToLoki(logs []models.LogEntry) {
	if len(logs) == 0 {
		return
	}

	// Group logs by service (Loki stream per service)
	streams := map[string][]models.LogEntry{}
	for _, l := range logs {
		streams[l.ServiceID] = append(streams[l.ServiceID], l)
	}

	// Loki push API payload
	type lokiValue [2]string // [timestamp_ns_string, log_line]
	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values []lokiValue       `json:"values"`
	}
	type lokiPush struct {
		Streams []lokiStream `json:"streams"`
	}

	// Group by service+level for separate Loki streams (enables trace_id-level drill-down)
	type streamKey struct{ svcID, level, traceID string }
	streamMap := map[streamKey]*lokiStream{}

	var lokiStreams []lokiStream
	for _, log := range logs {
		traceID := ""
		if log.TraceID != "" {
			traceID = log.TraceID
		} else if log.Labels != nil {
			traceID = log.Labels["trace_id"]
		}
		key := streamKey{log.ServiceID, log.Level, traceID}
		if _, ok := streamMap[key]; !ok {
			ns := ""
			if log.Labels != nil {
				ns = log.Labels["namespace"]
			}
			st := &lokiStream{
				Stream: map[string]string{
					"service_id": log.ServiceID,
					"level":      log.Level,
					"namespace":  ns,
				},
			}
			// trace_id label enables Grafana derivedFields for 1-click trace→log linkage
			if traceID != "" {
				st.Stream["trace_id"] = traceID
			}
			streamMap[key] = st
		}
		msg := log.Message
		if log.Body != nil {
			if b, err := json.Marshal(log.Body); err == nil {
				msg = string(b)
			}
		}
		streamMap[key].Values = append(streamMap[key].Values, lokiValue{
			fmt.Sprintf("%d", log.Timestamp.UnixNano()),
			msg,
		})
	}
	for _, s := range streamMap {
		lokiStreams = append(lokiStreams, *s)
	}

	payload, err := json.Marshal(lokiPush{Streams: lokiStreams})
	if err != nil {
		return
	}

	resp, err := ing.client.Post(
		ing.cfg.LokiURL+"/loki/api/v1/push",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		ing.logger.Error("loki write failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

// ─── Traces batch ─────────────────────────────────────────────────────────────

func (ing *Ingestor) handleTracesBatch(c *fiber.Ctx) error {
	var spans []models.Span
	if err := c.BodyParser(&spans); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	go ing.writeTracesToTempo(spans)
	go ing.forwardToProcessor("/v1/traces/batch", spans)

	return c.JSON(fiber.Map{"received": len(spans)})
}

// writeTracesToTempo writes spans to Grafana Tempo via OTLP HTTP
func (ing *Ingestor) writeTracesToTempo(spans []models.Span) {
	if len(spans) == 0 {
		return
	}

	// Convert to Tempo-compatible JSON
	payload, _ := json.Marshal(spans)
	resp, err := ing.client.Post(
		ing.cfg.TempoURL+"/api/traces",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		ing.logger.Error("tempo write failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

// ─── Profiles ─────────────────────────────────────────────────────────────────

func (ing *Ingestor) handleProfile(c *fiber.Ctx) error {
	var profile models.Profile
	if err := c.BodyParser(&profile); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	// Store profile in ClickHouse
	go ing.writeProfileToClickHouse(&profile)
	return c.JSON(fiber.Map{"status": "ok"})
}

// ─── OTEL receivers (standard endpoints) ─────────────────────────────────────

func (ing *Ingestor) handleOTELMetrics(c *fiber.Ctx) error {
	if ing.isOTLPProtobuf(c) {
		return ing.proxyOTLP(c, "/v1/metrics")
	}
	return ing.handleMetricsBatch(c)
}

func (ing *Ingestor) handleOTELLogs(c *fiber.Ctx) error {
	if ing.isOTLPProtobuf(c) {
		return ing.proxyOTLP(c, "/v1/logs")
	}
	return ing.handleLogsBatch(c)
}

func (ing *Ingestor) handleOTELTraces(c *fiber.Ctx) error {
	if ing.isOTLPProtobuf(c) {
		return ing.proxyOTLP(c, "/v1/traces")
	}
	return ing.handleTracesBatch(c)
}

func (ing *Ingestor) isOTLPProtobuf(c *fiber.Ctx) bool {
	ct := strings.ToLower(c.Get("Content-Type"))
	return strings.Contains(ct, "application/x-protobuf") || strings.Contains(ct, "application/protobuf")
}

func (ing *Ingestor) proxyOTLP(c *fiber.Ctx, path string) error {
	req, err := http.NewRequest(c.Method(), ing.cfg.OTELCollectorURL+path, bytes.NewReader(c.Body()))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "invalid otlp request"})
	}
	req.Header.Set("Content-Type", c.Get("Content-Type"))
	if encoding := c.Get("Content-Encoding"); encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}

	resp, err := ing.client.Do(req)
	if err != nil {
		ing.logger.Error("otel collector proxy failed", zap.Error(err))
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "otel collector unavailable"})
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		if len(vals) > 0 {
			c.Set(k, vals[0])
		}
	}
	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

// ─── ClickHouse writes ────────────────────────────────────────────────────────

func (ing *Ingestor) writeServiceToClickHouse(svc *models.Service) {
	query := fmt.Sprintf(
		`INSERT INTO services (id, name, kind, namespace, cluster, node, health_state, health_score, agent_id, discovered_at, last_seen_at) VALUES ('%s','%s','%s','%s','%s','%s','%s',%g,'%s','%s','%s')`,
		esc(svc.ID), esc(svc.Name), esc(string(svc.Kind)), esc(svc.Namespace),
		esc(svc.ClusterName), esc(svc.NodeName), esc(string(svc.Health.State)),
		svc.Health.Score, esc(svc.AgentID),
		svc.DiscoveredAt.Format("2006-01-02 15:04:05"),
		svc.LastSeenAt.Format("2006-01-02 15:04:05"),
	)
	ing.clickhouseQuery(query)
}

func (ing *Ingestor) writeEdgeToClickHouse(edge *models.TopoEdge) {
	query := fmt.Sprintf(
		`INSERT INTO topology_edges (id, source_id, target_id, protocol, calls_per_min, avg_latency_ms, error_rate, updated_at) VALUES ('%s','%s','%s','%s',%g,%g,%g,'%s')`,
		esc(edge.ID), esc(edge.SourceID), esc(edge.TargetID), esc(edge.Protocol),
		edge.CallsPerMin, edge.AvgLatencyMs, edge.ErrorRate,
		edge.UpdatedAt.Format("2006-01-02 15:04:05"),
	)
	ing.clickhouseQuery(query)
}

func (ing *Ingestor) writeProfileToClickHouse(p *models.Profile) {
	query := fmt.Sprintf(
		`INSERT INTO profiles (service_id, profile_type, start_time, duration_sec) VALUES ('%s','%s','%s',%d)`,
		esc(p.ServiceID), esc(p.ProfileType),
		p.StartTime.Format("2006-01-02 15:04:05"), p.DurationSec,
	)
	ing.clickhouseQuery(query)
}

func (ing *Ingestor) clickhouseQuery(query string) {
	if err := ing.clickhouseQueryErr(query); err != nil {
		ing.logger.Debug("clickhouse write error", zap.Error(err))
	}
}

func (ing *Ingestor) clickhouseQueryErr(query string) error {
	resp, err := ing.client.Post(
		ing.cfg.ClickHouseURL+"/?query="+query,
		"text/plain",
		nil,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickhouse status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ─── Internal reads ───────────────────────────────────────────────────────────

func (ing *Ingestor) handleListServices(c *fiber.Ctx) error {
	ing.svcMu.RLock()
	svcs := make([]*models.Service, 0, len(ing.services))
	for _, s := range ing.services {
		svcs = append(svcs, s)
	}
	ing.svcMu.RUnlock()
	return c.JSON(fiber.Map{"services": svcs, "total": len(svcs)})
}

func (ing *Ingestor) handleListAgents(c *fiber.Ctx) error {
	ing.agentMu.RLock()
	agents := make([]*models.Agent, 0, len(ing.agents))
	for _, a := range ing.agents {
		agents = append(agents, a)
	}
	ing.agentMu.RUnlock()
	return c.JSON(fiber.Map{"agents": agents, "total": len(agents)})
}

// ─── Processor forwarding ─────────────────────────────────────────────────────

func (ing *Ingestor) forwardToProcessor(path string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	resp, err := ing.client.Post(
		ing.cfg.ProcessorURL+path,
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return // processor may not be up yet
	}
	resp.Body.Close()
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func sanitizeMetricName(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == ':' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

func sanitizeLabelName(name string) string {
	return sanitizeMetricName(name)
}

func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}

// esc escapes s for use inside a single-quoted ClickHouse string literal.
// Callers supply the surrounding quotes. Backslashes are escaped as well as
// single quotes, in a single pass, so input such as \' or a trailing \ cannot
// end the literal early.
func esc(s string) string {
	return clickHouseStringEscaper.Replace(s)
}

var clickHouseStringEscaper = strings.NewReplacer(`\`, `\\`, `'`, `\'`)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func readSecretFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func joinStaticTokens(tokens ...string) string {
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			parts = append(parts, token)
		}
	}
	return strings.Join(parts, ",")
}

func staticAgentTokens(tokenList string) []string {
	if tokenList == "" {
		return nil
	}
	fields := strings.FieldsFunc(tokenList, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, token := range fields {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		v = strings.ToLower(strings.TrimSpace(v))
		return v == "1" || v == "true" || v == "yes" || v == "on"
	}
	return fallback
}

// ── RUM handler ───────────────────────────────────────────────────────────────
// Accepts a batch of RUM events from the browser SDK.
// Each event is one of: page_view, vital, error, resource, navigation, custom.
// Events are translated to:
//   - MetricPoints  -> ObserveX native metrics (timing numbers, CLS, LCP, FID)
//   - LogEntries    → Loki (errors, page views with full context)
//
// POST /v1/rum
// Body: RUMBatch JSON
// Headers: X-RUM-App-ID (required), X-RUM-Token (optional, for rate limiting later)

// RUMBatch is the payload the JS SDK posts.
type RUMBatch struct {
	AppID     string     `json:"app_id"`
	SessionID string     `json:"session_id"`
	PageID    string     `json:"page_id"`
	URL       string     `json:"url"`
	UserAgent string     `json:"user_agent"`
	Events    []RUMEvent `json:"events"`
}

type RUMEvent struct {
	Type      string         `json:"type"`       // page_view | vital | error | resource | navigation | custom
	Timestamp int64          `json:"ts"`         // Unix ms
	Data      map[string]any `json:"data"`
}

func (ing *Ingestor) handleRUM(c *fiber.Ctx) error {
	var batch RUMBatch
	if err := c.BodyParser(&batch); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if batch.AppID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "app_id is required"})
	}
	if len(batch.Events) == 0 {
		return c.JSON(fiber.Map{"received": 0})
	}
	// Enforce a per-request event cap to prevent abuse
	if len(batch.Events) > 200 {
		batch.Events = batch.Events[:200]
	}

	var metrics []models.MetricPoint
	var logs    []models.LogEntry

	serviceID := "rum:" + batch.AppID
	baseLabels := map[string]string{
		"app_id":     batch.AppID,
		"session_id": batch.SessionID,
		"page_id":    batch.PageID,
		"url":        batch.URL,
		"source":     "rum",
	}

	for _, ev := range batch.Events {
		ts := time.UnixMilli(ev.Timestamp)
		if ts.IsZero() {
			ts = time.Now()
		}

		switch ev.Type {

		case "vital":
			// Web Vitals: LCP, FID/INP, CLS, FCP, TTFB
			name, _ := ev.Data["name"].(string)
			val, _  := ev.Data["value"].(float64)
			if name == "" || val < 0 {
				continue
			}
			labels := copyLabels(baseLabels)
			labels["vital"] = name
			if rating, ok := ev.Data["rating"].(string); ok {
				labels["rating"] = rating // "good" | "needs-improvement" | "poor"
			}
			metrics = append(metrics, models.MetricPoint{
				Name:      "rum_web_vital_ms",
				Value:     val,
				Timestamp: ts,
				Labels:    labels,
				ServiceID: serviceID,
			})

		case "page_view":
			// Route change or initial load
			loadMs, _ := ev.Data["load_ms"].(float64)
			domMs, _   := ev.Data["dom_ms"].(float64)
			ttfbMs, _  := ev.Data["ttfb_ms"].(float64)
			path, _    := ev.Data["path"].(string)

			labels := copyLabels(baseLabels)
			if path != "" {
				labels["path"] = path
			}
			metrics = append(metrics, models.MetricPoint{
				Name: "rum_page_view_total", Value: 1,
				Timestamp: ts, Labels: labels, ServiceID: serviceID,
			})

			if loadMs > 0 {
				metrics = append(metrics, models.MetricPoint{
					Name: "rum_page_load_ms", Value: loadMs,
					Timestamp: ts, Labels: labels, ServiceID: serviceID,
				})
			}
			if domMs > 0 {
				metrics = append(metrics, models.MetricPoint{
					Name: "rum_dom_content_loaded_ms", Value: domMs,
					Timestamp: ts, Labels: labels, ServiceID: serviceID,
				})
			}
			if ttfbMs > 0 {
				metrics = append(metrics, models.MetricPoint{
					Name: "rum_ttfb_ms", Value: ttfbMs,
					Timestamp: ts, Labels: labels, ServiceID: serviceID,
				})
			}

			// Also log the page view so it appears in log explorer
			logs = append(logs, models.LogEntry{
				Timestamp: ts,
				Level:     "info",
				Message:   fmt.Sprintf("page_view url=%s session=%s", batch.URL, batch.SessionID),
				ServiceID: serviceID,
				Labels:    mergeLabels(labels, map[string]string{"event_type": "page_view"}),
				Body:      ev.Data,
			})

		case "error":
			// JS errors and unhandled promise rejections
			msg, _   := ev.Data["message"].(string)
			stack, _ := ev.Data["stack"].(string)
			errType, _ := ev.Data["error_type"].(string)
			if msg == "" {
				msg = "unknown error"
			}
			labels := copyLabels(baseLabels)
			if errType != "" {
				labels["error_type"] = errType
			}
			// Count the error as a metric
			metrics = append(metrics, models.MetricPoint{
				Name: "rum_js_errors_total", Value: 1,
				Timestamp: ts, Labels: labels, ServiceID: serviceID,
			})
			// Full error detail goes to Loki
			body := map[string]any{
				"message": msg,
				"url":     batch.URL,
				"ua":      batch.UserAgent,
			}
			if stack != "" {
				body["stack"] = stack
			}
			logs = append(logs, models.LogEntry{
				Timestamp: ts,
				Level:     "error",
				Message:   fmt.Sprintf("js_error: %s", msg),
				ServiceID: serviceID,
				Labels:    mergeLabels(labels, map[string]string{"event_type": "error"}),
				Body:      body,
			})

		case "resource":
			// Slow resource loads (images, scripts, API calls)
			resURL, _   := ev.Data["url"].(string)
			durationMs, _ := ev.Data["duration_ms"].(float64)
			resType, _  := ev.Data["initiator_type"].(string)
			if durationMs <= 0 {
				continue
			}
			labels := copyLabels(baseLabels)
			if resType != "" {
				labels["initiator_type"] = resType
			}
			metrics = append(metrics, models.MetricPoint{
				Name: "rum_resource_duration_ms", Value: durationMs,
				Timestamp: ts,
				Labels:    mergeLabels(labels, map[string]string{"resource_url": truncate(resURL, 120)}),
				ServiceID: serviceID,
			})

		case "navigation":
			// SPA route changes
			fromPath, _ := ev.Data["from"].(string)
			toPath, _   := ev.Data["to"].(string)
			navMs, _    := ev.Data["duration_ms"].(float64)
			labels := copyLabels(baseLabels)
			labels["from_path"] = fromPath
			labels["to_path"]   = toPath
			if navMs > 0 {
				metrics = append(metrics, models.MetricPoint{
					Name: "rum_navigation_ms", Value: navMs,
					Timestamp: ts, Labels: labels, ServiceID: serviceID,
				})
			}

		case "custom":
			// App-defined events pushed via obs.track()
			evName, _ := ev.Data["name"].(string)
			val, hasVal := ev.Data["value"].(float64)
			if evName == "" {
				continue
			}
			labels := copyLabels(baseLabels)
			labels["event_name"] = evName
			if hasVal {
				metrics = append(metrics, models.MetricPoint{
					Name: "rum_custom_event", Value: val,
					Timestamp: ts, Labels: labels, ServiceID: serviceID,
				})
			} else {
				logs = append(logs, models.LogEntry{
					Timestamp: ts,
					Level:     "info",
					Message:   fmt.Sprintf("custom_event: %s", evName),
					ServiceID: serviceID,
					Labels:    labels,
					Body:      ev.Data,
				})
			}
		}
	}

	// Fanout asynchronously — never block the browser
	if len(metrics) > 0 {
		go func() {
			ing.metricBufMu.Lock()
			ing.metricBuf = append(ing.metricBuf, metrics...)
			ing.metricBufMu.Unlock()
		}()
	}
	if len(logs) > 0 {
		go ing.writeLogsToLoki(logs)
	}

	return c.JSON(fiber.Map{"received": len(batch.Events), "metrics": len(metrics), "logs": len(logs)})
}

// ── RUM helpers ───────────────────────────────────────────────────────────────

func copyLabels(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeLabels(base, extra map[string]string) map[string]string {
	dst := copyLabels(base)
	for k, v := range extra {
		dst[k] = v
	}
	return dst
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ── Security events handler ───────────────────────────────────────────────
// Accepts runtime threat detection events from OneAgent's eBPF security
// monitor and image scan results from the Trivy scanner.
// Events are fanned out to:
//   - Loki (full event as JSON log line for investigation)
//   - ObserveX native metrics (severity counters for dashboards + alerting)

func (ing *Ingestor) handleSecurityEvents(c *fiber.Ctx) error {
	var events []models.SecurityEvent
	if err := c.BodyParser(&events); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if len(events) == 0 {
		return c.JSON(fiber.Map{"received": 0})
	}

	var metrics []models.MetricPoint
	var logs    []models.LogEntry
	now := time.Now()

	for i := range events {
		ev := &events[i]
		if ev.Timestamp.IsZero() {
			ev.Timestamp = now
		}

		// Metric: count by severity + event_type
		metrics = append(metrics, models.MetricPoint{
			Name:  "security_events_total",
			Value: 1,
			Timestamp: ev.Timestamp,
			Labels: map[string]string{
				"severity":   ev.Severity,
				"event_type": ev.EventType,
				"node":       ev.NodeName,
				"cluster":    ev.ClusterName,
				"namespace":  ev.Namespace,
			},
			ServiceID: "security-monitor",
		})

		// Log: full event serialized to JSON for Loki
		body := map[string]any{
			"pid":          ev.PID,
			"ppid":         ev.PPID,
			"uid":          ev.UID,
			"process":      ev.ProcessName,
			"path":         ev.Path,
			"args":         ev.Args,
			"dst_ip":       ev.DstIP,
			"dst_port":     ev.DstPort,
			"pod":          ev.PodName,
			"image":        ev.ContainerImage,
			"mitre_tactic": ev.MitreTactic,
			"mitre_id":     ev.MitreTechID,
		}

		level := "warn"
		if ev.Severity == "CRITICAL" || ev.Severity == "HIGH" {
			level = "error"
		}

		logs = append(logs, models.LogEntry{
			Timestamp: ev.Timestamp,
			Level:     level,
			Message:   fmt.Sprintf("[%s] %s: %s", ev.Severity, ev.EventType, ev.Description),
			ServiceID: "security-monitor:" + ev.NodeName,
			Labels: map[string]string{
				"severity":   ev.Severity,
				"event_type": ev.EventType,
				"node":       ev.NodeName,
				"namespace":  ev.Namespace,
				"pod":        ev.PodName,
				"cluster":    ev.ClusterName,
				"source":     "ebpf-security",
			},
			Body: body,
		})
	}

	if len(metrics) > 0 {
		go func() {
			ing.metricBufMu.Lock()
			ing.metricBuf = append(ing.metricBuf, metrics...)
			ing.metricBufMu.Unlock()
		}()
	}
	if len(logs) > 0 {
		go ing.writeLogsToLoki(logs)
	}

	return c.JSON(fiber.Map{"received": len(events)})
}

func (ing *Ingestor) handleImageScanResults(c *fiber.Ctx) error {
	var results []models.ImageScanResult
	if err := c.BodyParser(&results); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if len(results) == 0 {
		return c.JSON(fiber.Map{"received": 0})
	}

	var metrics []models.MetricPoint
	var logs    []models.LogEntry
	now := time.Now()

	for _, r := range results {
		if r.ScannedAt.IsZero() {
			r.ScannedAt = now
		}

		// Risk score metric
		metrics = append(metrics,
			models.MetricPoint{
				Name: "image_scan_risk_score", Value: r.RiskScore,
				Timestamp: r.ScannedAt,
				Labels: map[string]string{
					"image": r.Image, "cluster": r.ClusterName, "namespace": r.Namespace,
				},
				ServiceID: "trivy-scanner",
			},
			models.MetricPoint{
				Name: "image_scan_vulns_critical", Value: float64(r.Critical),
				Timestamp: r.ScannedAt,
				Labels: map[string]string{"image": r.Image},
				ServiceID: "trivy-scanner",
			},
			models.MetricPoint{
				Name: "image_scan_vulns_high", Value: float64(r.High),
				Timestamp: r.ScannedAt,
				Labels: map[string]string{"image": r.Image},
				ServiceID: "trivy-scanner",
			},
		)

		// Log summary and each CRITICAL/HIGH finding
		logMsg := fmt.Sprintf("image scan: %s — C:%d H:%d M:%d L:%d score:%.1f",
			r.Image, r.Critical, r.High, r.Medium, r.Low, r.RiskScore)
		logs = append(logs, models.LogEntry{
			Timestamp: r.ScannedAt,
			Level:     map[bool]string{true: "error", false: "warn"}[r.Critical > 0],
			Message:   logMsg,
			ServiceID: "trivy-scanner",
			Labels: map[string]string{
				"source": "trivy", "image": r.Image, "cluster": r.ClusterName,
			},
		})

		// One log line per CRITICAL/HIGH CVE for easy searching
		for _, v := range r.Vulnerabilities {
			if v.Severity != models.VulnCritical && v.Severity != models.VulnHigh {
				continue
			}
			logs = append(logs, models.LogEntry{
				Timestamp: r.ScannedAt,
				Level:     map[models.VulnSeverity]string{models.VulnCritical: "error", models.VulnHigh: "warn"}[v.Severity],
				Message:   fmt.Sprintf("%s %s in %s@%s (fix: %s)", v.Severity, v.VulnID, v.PkgName, v.InstalledVersion, v.FixedVersion),
				ServiceID: "trivy-scanner",
				Labels: map[string]string{
					"source":   "trivy",
					"cve":      v.VulnID,
					"severity": string(v.Severity),
					"image":    r.Image,
					"pkg":      v.PkgName,
				},
				Body: map[string]any{
					"vuln_id": v.VulnID, "pkg": v.PkgName,
					"installed": v.InstalledVersion, "fixed": v.FixedVersion,
					"cvss": v.CVSS, "title": v.Title,
				},
			})
		}
	}

	if len(metrics) > 0 {
		go func() {
			ing.metricBufMu.Lock()
			ing.metricBuf = append(ing.metricBuf, metrics...)
			ing.metricBufMu.Unlock()
		}()
	}
	if len(logs) > 0 {
		go ing.writeLogsToLoki(logs)
	}

	return c.JSON(fiber.Map{"received": len(results)})
}
