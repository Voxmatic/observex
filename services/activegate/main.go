// services/activegate/main.go
//
// ObserveX ActiveGate — proxy and concentrator tier.
//
// Mirrors Dynatrace's ActiveGate architecture (Images 1-4):
//
//   OneAgents / RUM / Synthetic / Remote Extensions
//          │  HTTPS :9999
//          ▼
//   ┌─────────────────────────────────────────────────┐
//   │               ActiveGate (this service)          │
//   │                                                   │
//   │  • Accepts telemetry from OneAgents on :9999     │
//   │  • Accepts RUM events (agentless, mobile)        │
//   │  • Accepts Synthetic results                     │
//   │  • Accepts remote extension data (SNMP, WMI)    │
//   │  • Buffers + compresses payloads                 │
//   │  • Forwards to ObserveX Cluster on :443/4318    │
//   │  • Acts as Environment ActiveGate OR             │
//   │    Cluster ActiveGate depending on GATE_MODE     │
//   └─────────────────────────────────────────────────┘
//          │  HTTPS :443 (cluster-internal :4318)
//          ▼
//   ObserveX Cluster (Ingestor nodes behind seedServerUrl LB)
//
// Deployment modes (GATE_MODE env var):
//   environment  — one gate per environment, proxies to single cluster endpoint
//   cluster      — multiple gates in an HA pool behind dnsEntryPoint LB
//
// Ports:
//   :9999  — agent-facing (OneAgents, synthetic probes, RUM beacons)
//   :9998  — health/metrics endpoint
//   :9997  — cluster-facing outbound (to seedServerUrl reverse proxy)
//
// Environment variables:
//   GATE_MODE           environment | cluster (default: environment)
//   GATE_ID             unique ID for this gate instance
//   CLUSTER_ENDPOINT    URL of the ObserveX cluster (seedServerUrl)
//                       e.g. http://observex-ingestor:4318
//   DNS_ENTRY_POINT     this gate's advertised address for agent auto-config
//   BUFFER_SIZE_MB      in-memory buffer before forwarding (default: 64)
//   COMPRESS            enable gzip compression on cluster writes (default: true)
//   LOG_LEVEL           debug | info | warn (default: info)

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	GateMode        string // environment | cluster
	GateID          string
	Port            string // agent-facing port (default 9999)
	ClusterEndpoint string // where to forward telemetry
	DNSEntryPoint   string // this gate's public address advertised to agents
	BufferSizeMB    int
	Compress        bool
	LogLevel        string
	AgentToken      string
	OrgID           string
	ClusterName     string
	Environment     string
	NetworkZone     string
}

func loadConfig() Config {
	bufMB := 64
	fmt.Sscanf(envOr("BUFFER_SIZE_MB", "64"), "%d", &bufMB)
	return Config{
		GateMode:        envOr("GATE_MODE", "environment"),
		GateID:          envOr("GATE_ID", fmt.Sprintf("gate-%d", time.Now().UnixNano()&0xFFFFF)),
		Port:            envOr("PORT", "9999"),
		ClusterEndpoint: envOr("CLUSTER_ENDPOINT", "http://observex-ingestor:4318"),
		DNSEntryPoint:   envOr("DNS_ENTRY_POINT", ""),
		BufferSizeMB:    bufMB,
		Compress:        envOr("COMPRESS", "true") == "true",
		LogLevel:        envOr("LOG_LEVEL", "info"),
		AgentToken:      envOr("OBSERVEX_TOKEN", envOr("AGENT_TOKEN", "")),
		OrgID:           envOr("OBSERVEX_ORG_ID", envOr("ORG_ID", "")),
		ClusterName:     envOr("CLUSTER_NAME", "default"),
		Environment:     envOr("OBSERVEX_ENV", envOr("ENVIRONMENT", "production")),
		NetworkZone:     envOr("OBSERVEX_NETWORK_ZONE", envOr("NETWORK_ZONE", "")),
	}
}

// ── Telemetry buffer ──────────────────────────────────────────────────────────

// bufferedPayload holds a forwarding unit: the path to POST to the cluster
// and the raw body (JSON array or NDJSON).
type bufferedPayload struct {
	path        string
	contentType string
	headers     map[string]string
	body        []byte
	receivedAt  time.Time
	compress    bool
}

// ActiveGate is the main service struct.
type ActiveGate struct {
	cfg    Config
	logger *zap.Logger
	client *http.Client

	// Metrics counters
	receivedTotal   atomic.Int64
	forwardedTotal  atomic.Int64
	droppedTotal    atomic.Int64
	bufferBytes     atomic.Int64

	// Buffer for async forwarding
	bufMu  sync.Mutex
	buffer []bufferedPayload

	// Connected agents registry (for admin UI)
	agentsMu sync.RWMutex
	agents   map[string]agentInfo
}

type agentInfo struct {
	AgentID     string    `json:"agent_id"`
	NodeName    string    `json:"node_name"`
	ClusterName string    `json:"cluster_name"`
	Version     string    `json:"version,omitempty"`
	LastSeen    time.Time `json:"last_seen"`
	BytesSent   int64     `json:"bytes_sent"`
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := loadConfig()

	gate := &ActiveGate{
		cfg:    cfg,
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		buffer: make([]bufferedPayload, 0, 1024),
		agents: make(map[string]agentInfo),
	}

	logger.Info("ObserveX ActiveGate starting",
		zap.String("mode", cfg.GateMode),
		zap.String("id", cfg.GateID),
		zap.String("agent_port", cfg.Port),
		zap.String("cluster_endpoint", cfg.ClusterEndpoint),
		zap.Int("buffer_mb", cfg.BufferSizeMB),
		zap.Bool("compress", cfg.Compress),
	)

	// Start background forwarder
	go gate.runForwarder()

	// Start metrics reporter
	go gate.runMetricsReporter()

	// ── Agent-facing HTTP server (port 9999) ──────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      fmt.Sprintf("ObserveX ActiveGate [%s]", cfg.GateMode),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		BodyLimit:    cfg.BufferSizeMB * 1024 * 1024,
	})
	app.Use(recover.New())

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		logger.Info("shutting down gracefully...")
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()

	// ── Agent auto-registration ────────────────────────────────────────────────
	app.Post("/v1/agents/register", gate.handleAgentRegister)
	app.Post("/v1/agents/:id/heartbeat", gate.handleAgentHeartbeat)
	// Update manifests are synchronously proxied so an Agent can use an
	// Environment ActiveGate as its only SaaS endpoint.
	app.Get("/v1/agent/updates/manifest", gate.handleAgentUpdateManifest)

	// ── Telemetry ingestion (proxied to cluster) ───────────────────────────────
	// Same API surface as the ObserveX ingestor so OneAgents need zero reconfiguration
	app.Post("/v1/metrics/batch",    gate.handleBuffer("/v1/metrics/batch", "application/json"))
	app.Post("/v1/logs/batch",       gate.handleBuffer("/v1/logs/batch", "application/json"))
	app.Post("/v1/traces/batch",     gate.handleBuffer("/v1/traces/batch", "application/json"))
	app.Post("/v1/services",         gate.handleBuffer("/v1/services", "application/json"))
	app.Post("/v1/topology/edges",   gate.handleBuffer("/v1/topology/edges", "application/json"))
	app.Post("/v1/security/events",  gate.handleBuffer("/v1/security/events", "application/json"))
	app.Post("/v1/security/scans",   gate.handleBuffer("/v1/security/scans", "application/json"))
	app.Post("/v1/rum",              gate.handleRUMBuffer)

	// ── Remote extension data sources (SNMP, WMI, DB Insights) ───────────────
	app.Post("/v1/extensions/:source", gate.handleExtension)

	// ── ActiveGate management API ─────────────────────────────────────────────
	app.Get("/health",       gate.handleHealth)
	app.Get("/v1/status",    gate.handleStatus)
	app.Get("/v1/agents",    gate.handleListAgents)
	app.Get("/v1/metrics",   gate.handleGateMetrics)
	app.Get("/v1/self/metrics", gate.handleNativeStatus)

	logger.Fatal("activegate server error", zap.Error(app.Listen(":"+cfg.Port)))
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleBuffer is the main proxy handler for all telemetry types.
// It reads the body, adds it to the buffer, and acknowledges immediately.
// The background forwarder drains the buffer to the cluster asynchronously.
func (g *ActiveGate) handleBuffer(clusterPath, ct string) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		body := make([]byte, len(c.Body()))
		copy(body, c.Body())

		if len(body) == 0 {
			return c.JSON(fiber.Map{"received": 0})
		}

		// Track agent if X-Agent-ID header is present
		if agentID := c.Get("X-Agent-ID"); agentID != "" {
			g.trackAgent(agentID, c.Get("X-Node-Name"), c.Get("X-Cluster"), int64(len(body)))
		}

		g.enqueue(bufferedPayload{
			path:        clusterPath,
			contentType: ct,
			headers:     forwardHeadersFromCtx(c),
			body:        body,
			receivedAt:  time.Now(),
			compress:    true,
		})

		g.receivedTotal.Add(1)
		return c.JSON(fiber.Map{"status": "buffered", "gate": g.cfg.GateID})
	}
}

// handleRUMBuffer handles RUM beacons from browsers/mobile (agentless RUM).
// RUM beacons use a different CORS policy and simpler ack.
func (g *ActiveGate) handleRUMBuffer(c *fiber.Ctx) error {
	c.Set("Access-Control-Allow-Origin", "*")
	body := make([]byte, len(c.Body()))
	copy(body, c.Body())
	g.enqueue(bufferedPayload{path: "/v1/rum", contentType: "application/json", body: body, receivedAt: time.Now(), compress: true})
	g.receivedTotal.Add(1)
	return c.SendStatus(204) // RUM beacons expect 204 No Content
}

// handleExtension handles data from remote extension sources:
// SNMP devices, WMI counters, Database Insights agents.
func (g *ActiveGate) handleExtension(c *fiber.Ctx) error {
	source := c.Params("source") // snmp | wmi | db_insights | custom
	body := make([]byte, len(c.Body()))
	copy(body, c.Body())

	g.logger.Debug("extension data received",
		zap.String("source", source),
		zap.Int("bytes", len(body)),
	)

	// Extension data is re-wrapped as metrics before forwarding
	payload := map[string]interface{}{
		"source": source,
		"data":   json.RawMessage(body),
		"ts":     time.Now().Unix(),
		"gate":   g.cfg.GateID,
	}
	wrapped, _ := json.Marshal([]interface{}{payload})

	g.enqueue(bufferedPayload{
		path:        "/v1/extensions/batch",
		contentType: "application/json",
		headers:     g.defaultIdentityHeaders(),
		body:        wrapped,
		receivedAt:  time.Now(),
		compress:    true,
	})
	g.receivedTotal.Add(1)
	return c.JSON(fiber.Map{"received": 1, "source": source})
}

// handleAgentRegister records a newly connected agent.
func (g *ActiveGate) handleAgentRegister(c *fiber.Ctx) error {
	var req struct {
		AgentID     string `json:"agent_id"`
		NodeName    string `json:"node_name"`
		ClusterName string `json:"cluster_name"`
		Version     string `json:"version"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	g.trackAgent(req.AgentID, req.NodeName, req.ClusterName, 0)

	// Also forward registration to cluster
	g.enqueue(bufferedPayload{
		path:        "/v1/agents/register",
		contentType: "application/json",
		headers:     forwardHeadersFromCtx(c),
		body:        c.Body(),
		receivedAt:  time.Now(),
		compress:    true,
	})

	return c.JSON(fiber.Map{
		"status":           "registered",
		"gate_id":          g.cfg.GateID,
		"gate_mode":        g.cfg.GateMode,
		"cluster_endpoint": g.cfg.ClusterEndpoint,
		"dns_entry_point":  g.cfg.DNSEntryPoint,
	})
}

func (g *ActiveGate) handleAgentHeartbeat(c *fiber.Ctx) error {
	agentID := c.Params("id")
	g.trackAgent(agentID, "", "", 0)
	g.enqueue(bufferedPayload{
		path:        "/v1/agents/" + agentID + "/heartbeat",
		contentType: "application/json",
		headers:     forwardHeadersFromCtx(c),
		body:        c.Body(),
		receivedAt:  time.Now(),
		compress:    true,
	})
	return c.JSON(fiber.Map{"ok": true})
}

// handleAgentUpdateManifest proxies the signed update manifest without putting
// it through the asynchronous telemetry buffer. The Agent verifies both the
// manifest signature and artifact digest before it stages any update.
func (g *ActiveGate) handleAgentUpdateManifest(c *fiber.Ctx) error {
	path := "/v1/agent/updates/manifest"
	if query := string(c.Request().URI().QueryString()); query != "" {
		path += "?" + query
	}

	status, body, headers, err := g.forwardClusterRequest(http.MethodGet, path, forwardHeadersFromCtx(c), nil)
	if err != nil {
		g.logger.Warn("update manifest proxy failed", zap.Error(err))
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "update manifest unavailable"})
	}
	for _, key := range []string{"Content-Type", "Cache-Control", "ETag", "Last-Modified"} {
		if value := headers.Get(key); value != "" {
			c.Set(key, value)
		}
	}
	c.Set("Cache-Control", "no-store")
	return c.Status(status).Send(body)
}

func (g *ActiveGate) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":   "ok",
		"gate_id":  g.cfg.GateID,
		"mode":     g.cfg.GateMode,
		"buffered": len(g.buffer),
	})
}

func (g *ActiveGate) handleStatus(c *fiber.Ctx) error {
	g.agentsMu.RLock()
	agentCount := len(g.agents)
	g.agentsMu.RUnlock()

	g.bufMu.Lock()
	bufLen := len(g.buffer)
	g.bufMu.Unlock()

	return c.JSON(fiber.Map{
		"gate_id":          g.cfg.GateID,
		"mode":             g.cfg.GateMode,
		"cluster_endpoint": g.cfg.ClusterEndpoint,
		"dns_entry_point":  g.cfg.DNSEntryPoint,
		"connected_agents": agentCount,
		"buffer_depth":     bufLen,
		"received_total":   g.receivedTotal.Load(),
		"forwarded_total":  g.forwardedTotal.Load(),
		"dropped_total":    g.droppedTotal.Load(),
	})
}

func (g *ActiveGate) handleListAgents(c *fiber.Ctx) error {
	g.agentsMu.RLock()
	agents := make([]agentInfo, 0, len(g.agents))
	for _, a := range g.agents {
		agents = append(agents, a)
	}
	g.agentsMu.RUnlock()
	return c.JSON(fiber.Map{"agents": agents, "gate_id": g.cfg.GateID})
}

func (g *ActiveGate) handleGateMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"gate_id":         g.cfg.GateID,
		"received_total":  g.receivedTotal.Load(),
		"forwarded_total": g.forwardedTotal.Load(),
		"dropped_total":   g.droppedTotal.Load(),
		"buffer_bytes":    g.bufferBytes.Load(),
	})
}

func (g *ActiveGate) handleNativeStatus(c *fiber.Ctx) error {
	g.agentsMu.RLock()
	agentCount := len(g.agents)
	g.agentsMu.RUnlock()

	g.bufMu.Lock()
	bufferDepth := len(g.buffer)
	g.bufMu.Unlock()

	return c.JSON(fiber.Map{
		"gate_id":         g.cfg.GateID,
		"mode":            g.cfg.GateMode,
		"connected_agents": agentCount,
		"buffer_depth":    bufferDepth,
		"received_total":  g.receivedTotal.Load(),
		"forwarded_total": g.forwardedTotal.Load(),
		"dropped_total":   g.droppedTotal.Load(),
		"buffer_bytes":    g.bufferBytes.Load(),
		"collection_protocol": "observex-native-json",
	})
}

// ── Buffer + async forwarder ──────────────────────────────────────────────────

func (g *ActiveGate) enqueue(p bufferedPayload) {
	maxBytes := int64(g.cfg.BufferSizeMB) * 1024 * 1024
	if g.bufferBytes.Load()+int64(len(p.body)) > maxBytes {
		g.droppedTotal.Add(1)
		g.logger.Warn("buffer full, dropping payload",
			zap.Int("payload_bytes", len(p.body)),
			zap.Int64("buffer_bytes", g.bufferBytes.Load()),
		)
		return
	}
	g.bufMu.Lock()
	g.buffer = append(g.buffer, p)
	g.bufMu.Unlock()
	g.bufferBytes.Add(int64(len(p.body)))
}

// runForwarder drains the buffer to the cluster every 500ms.
// Payloads that share the same path are NOT merged (to preserve ordering)
// but are sent concurrently per-path for throughput.
func (g *ActiveGate) runForwarder() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		g.flush()
	}
}

func (g *ActiveGate) flush() {
	g.bufMu.Lock()
	if len(g.buffer) == 0 {
		g.bufMu.Unlock()
		return
	}
	toSend := g.buffer
	g.buffer = make([]bufferedPayload, 0, 1024)
	g.bufMu.Unlock()

	// Group by path for batch forwarding
	grouped := make(map[string][]bufferedPayload)
	for _, p := range toSend {
		grouped[p.path] = append(grouped[p.path], p)
	}

	var wg sync.WaitGroup
	for path, payloads := range grouped {
		wg.Add(1)
		go func(path string, payloads []bufferedPayload) {
			defer wg.Done()
			for _, p := range payloads {
				if err := g.forwardToCluster(p); err != nil {
					g.logger.Debug("forward failed, re-queueing",
						zap.String("path", path),
						zap.Error(err),
					)
					g.enqueue(p) // re-queue on failure
					g.droppedTotal.Add(1)
				} else {
					g.forwardedTotal.Add(1)
					g.bufferBytes.Add(-int64(len(p.body)))
				}
			}
		}(path, payloads)
	}
	wg.Wait()
}

func (g *ActiveGate) forwardToCluster(p bufferedPayload) error {
	var body io.Reader
	var encoding string

	if p.compress && g.cfg.Compress && len(p.body) > 1024 {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write(p.body)
		gz.Close()
		body = &buf
		encoding = "gzip"
	} else {
		body = bytes.NewReader(p.body)
	}

	url := strings.TrimRight(g.cfg.ClusterEndpoint, "/") + p.path
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", p.contentType)
	g.applyIdentityHeaders(req)
	for k, v := range p.headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("X-ActiveGate-ID", g.cfg.GateID)
	req.Header.Set("X-ActiveGate-Mode", g.cfg.GateMode)
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Client error — don't retry
		g.logger.Warn("cluster rejected payload",
			zap.String("path", p.path),
			zap.Int("status", resp.StatusCode),
		)
		return nil
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("cluster error %d", resp.StatusCode)
	}
	return nil
}

func (g *ActiveGate) forwardRawToCluster(path string, headers map[string]string, body []byte) (int, []byte, error) {
	status, response, _, err := g.forwardClusterRequest(http.MethodPost, path, headers, body)
	return status, response, err
}

func (g *ActiveGate) forwardClusterRequest(method, path string, headers map[string]string, body []byte) (int, []byte, http.Header, error) {
	url := strings.TrimRight(g.cfg.ClusterEndpoint, "/") + path
	var requestBody io.Reader
	if len(body) > 0 {
		requestBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, url, requestBody)
	if err != nil {
		return 0, nil, nil, err
	}
	g.applyIdentityHeaders(req)
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("X-ActiveGate-ID", g.cfg.GateID)
	req.Header.Set("X-ActiveGate-Mode", g.cfg.GateMode)

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, resp.Header, err
	}
	return resp.StatusCode, respBody, resp.Header, nil
}

func (g *ActiveGate) applyIdentityHeaders(req *http.Request) {
	if g.cfg.AgentToken != "" {
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Bearer "+g.cfg.AgentToken)
		}
		if req.Header.Get("X-ObserveX-Token") == "" {
			req.Header.Set("X-ObserveX-Token", g.cfg.AgentToken)
		}
	}
	if g.cfg.OrgID != "" && req.Header.Get("X-ObserveX-Org") == "" {
		req.Header.Set("X-ObserveX-Org", g.cfg.OrgID)
	}
	if g.cfg.ClusterName != "" && req.Header.Get("X-Cluster") == "" {
		req.Header.Set("X-Cluster", g.cfg.ClusterName)
	}
	if g.cfg.Environment != "" && req.Header.Get("X-ObserveX-Env") == "" {
		req.Header.Set("X-ObserveX-Env", g.cfg.Environment)
	}
	if g.cfg.NetworkZone != "" && req.Header.Get("X-Network-Zone") == "" {
		req.Header.Set("X-Network-Zone", g.cfg.NetworkZone)
	}
}

func (g *ActiveGate) defaultIdentityHeaders() map[string]string {
	headers := map[string]string{}
	if g.cfg.AgentToken != "" {
		headers["Authorization"] = "Bearer " + g.cfg.AgentToken
		headers["X-ObserveX-Token"] = g.cfg.AgentToken
	}
	if g.cfg.OrgID != "" {
		headers["X-ObserveX-Org"] = g.cfg.OrgID
	}
	if g.cfg.ClusterName != "" {
		headers["X-Cluster"] = g.cfg.ClusterName
	}
	if g.cfg.Environment != "" {
		headers["X-ObserveX-Env"] = g.cfg.Environment
	}
	if g.cfg.NetworkZone != "" {
		headers["X-Network-Zone"] = g.cfg.NetworkZone
	}
	return headers
}

func forwardHeadersFromCtx(c *fiber.Ctx) map[string]string {
	headers := map[string]string{}
	for _, name := range []string{
		"Authorization",
		"X-ObserveX-Token",
		"X-ObserveX-Org",
		"X-Agent-ID",
		"X-Node-Name",
		"X-Cluster",
		"X-ObserveX-Env",
		"X-Host-Group",
		"X-Network-Zone",
		"X-Monitoring-Mode",
		"X-Collection-Mode",
		"X-Agent-Version",
		"Content-Encoding",
		"Content-Type",
		"User-Agent",
	} {
		if v := c.Get(name); v != "" {
			headers[name] = v
		}
	}
	return headers
}

// ── Agent tracking ────────────────────────────────────────────────────────────

func (g *ActiveGate) trackAgent(agentID, nodeName, clusterName string, bytes int64) {
	if agentID == "" {
		return
	}
	g.agentsMu.Lock()
	a, ok := g.agents[agentID]
	if !ok {
		a = agentInfo{AgentID: agentID, NodeName: nodeName, ClusterName: clusterName}
	}
	if nodeName != "" {
		a.NodeName = nodeName
	}
	if clusterName != "" {
		a.ClusterName = clusterName
	}
	a.LastSeen = time.Now()
	a.BytesSent += bytes
	g.agents[agentID] = a
	g.agentsMu.Unlock()
}

// ── Background metrics reporter ───────────────────────────────────────────────

func (g *ActiveGate) runMetricsReporter() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		g.agentsMu.RLock()
		agentCount := len(g.agents)
		g.agentsMu.RUnlock()

		g.logger.Info("activegate stats",
			zap.String("gate_id", g.cfg.GateID),
			zap.String("mode", g.cfg.GateMode),
			zap.Int("connected_agents", agentCount),
			zap.Int64("received_total", g.receivedTotal.Load()),
			zap.Int64("forwarded_total", g.forwardedTotal.Load()),
			zap.Int64("dropped_total", g.droppedTotal.Load()),
		)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
