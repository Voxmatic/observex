package tests

import (
	"encoding/json"
	"math"
	"testing"
)

// ══ Metric Batch Parsing ══════════════════════════════════════════════════════

type MetricBatch struct {
	AgentID   string        `json:"agent_id"`
	ClusterID string        `json:"cluster_id"`
	Metrics   []MetricBatchPoint `json:"metrics"`
}
type MetricBatchPoint struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	ServiceID string            `json:"service_id"`
}

func TestMetricBatch_Deserialize(t *testing.T) {
	payload := `{
		"agent_id":"prod-linux-01",
		"cluster_id":"production",
		"metrics":[
			{"name":"cpu_usage_pct","value":84.2,"timestamp":1711234567890,"labels":{"host":"prod-linux-01","cluster":"production"},"service_id":"host:prod-linux-01"},
			{"name":"mem_usage_pct","value":61.4,"timestamp":1711234567890,"labels":{"host":"prod-linux-01"},"service_id":"host:prod-linux-01"},
			{"name":"disk_usage_pct","value":42.8,"timestamp":1711234567890,"labels":{"host":"prod-linux-01","mount":"/"},"service_id":"host:prod-linux-01"}
		]
	}`

	var batch MetricBatch
	if err := json.Unmarshal([]byte(payload), &batch); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if batch.AgentID != "prod-linux-01" { t.Errorf("agent_id: %q", batch.AgentID) }
	if len(batch.Metrics) != 3 { t.Errorf("metric count: %d", len(batch.Metrics)) }
	if batch.Metrics[0].Name != "cpu_usage_pct" { t.Error("first metric name") }
	if math.Abs(batch.Metrics[0].Value - 84.2) > 0.001 { t.Error("cpu value") }
	t.Logf("✅ MetricBatch: %d metrics from agent %s", len(batch.Metrics), batch.AgentID)
}

func TestMetricBatch_Validation(t *testing.T) {
	tests := []struct {
		name  string
		point MetricBatchPoint
		valid bool
		why   string
	}{
		{"valid", MetricBatchPoint{Name:"cpu_usage_pct",Value:84.2,Timestamp:1711234567}, true, ""},
		{"empty name", MetricBatchPoint{Name:"",Value:84.2,Timestamp:1711234567}, false, "name required"},
		{"negative ts", MetricBatchPoint{Name:"cpu",Value:50,Timestamp:-1}, false, "invalid timestamp"},
		{"NaN value", MetricBatchPoint{Name:"cpu",Value:math.NaN(),Timestamp:1711234567}, false, "NaN value"},
		{"Inf value", MetricBatchPoint{Name:"cpu",Value:math.Inf(1),Timestamp:1711234567}, false, "Inf value"},
	}
	for _, tt := range tests {
		valid := tt.point.Name != "" &&
			tt.point.Timestamp >= 0 &&
			!math.IsNaN(tt.point.Value) &&
			!math.IsInf(tt.point.Value, 0)
		if valid != tt.valid {
			t.Errorf("%s: valid=%v want %v (%s)", tt.name, valid, tt.valid, tt.why)
		}
	}
	t.Log("✅ Metric validation: name, timestamp, NaN, Inf all checked")
}

// ══ Log Entry Parsing ═════════════════════════════════════════════════════════

type LogBatch struct {
	AgentID  string     `json:"agent_id"`
	NodeName string     `json:"node_name"`
	Lines    []LogLine  `json:"lines"`
}
type LogLine struct {
	Timestamp int64             `json:"timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels"`
	Source    string            `json:"source"`
}

func TestLogBatch_Deserialize(t *testing.T) {
	payload := `{
		"agent_id":"prod-linux-03",
		"node_name":"prod-linux-03.corp.io",
		"lines":[
			{"timestamp":1711234567890,"level":"ERROR","message":"OOM killer invoked: process nginx (pid 8420) killed","labels":{"host":"prod-linux-03","severity":"critical"},"source":"kernel"},
			{"timestamp":1711234567891,"level":"WARN","message":"Disk usage /var 88% threshold exceeded","labels":{"host":"prod-linux-03","mount":"/var"},"source":"observex-agent"},
			{"timestamp":1711234567892,"level":"INFO","message":"systemd: Started nginx.service","labels":{"host":"prod-linux-03","unit":"nginx"},"source":"systemd"}
		]
	}`
	var batch LogBatch
	if err := json.Unmarshal([]byte(payload), &batch); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(batch.Lines) != 3 { t.Errorf("line count: %d", len(batch.Lines)) }
	if batch.Lines[0].Level != "ERROR" { t.Error("first log level") }
	if batch.Lines[2].Source != "systemd" { t.Error("log source") }
	t.Logf("✅ LogBatch: %d lines from %s", len(batch.Lines), batch.NodeName)
}

func TestLogBatch_LevelNormalization(t *testing.T) {
	// Agent sends various level formats — ingestor normalizes
	normalizeLevel := func(raw string) string {
		switch raw {
		case "error", "ERROR", "ERR", "E": return "ERROR"
		case "warn", "WARN", "WARNING", "W": return "WARN"
		case "info", "INFO", "I": return "INFO"
		case "debug", "DEBUG", "D": return "DEBUG"
		default: return "INFO"
		}
	}
	cases := []struct{ in, out string }{
		{"error","ERROR"},{"ERROR","ERROR"},{"ERR","ERROR"},
		{"warn","WARN"},{"WARNING","WARN"},{"WARN","WARN"},
		{"info","INFO"},{"INFO","INFO"},
		{"debug","DEBUG"},{"DEBUG","DEBUG"},
		{"UNKNOWN","INFO"},
	}
	for _, c := range cases {
		got := normalizeLevel(c.in)
		if got != c.out { t.Errorf("normalizeLevel(%q): got %q want %q", c.in, got, c.out) }
	}
	t.Log("✅ Log level normalization: error/warn/info/debug and aliases all handled")
}

// ══ OTLP Trace Parsing ═══════════════════════════════════════════════════════

type OTLPSpan struct {
	TraceID    string            `json:"trace_id"`
	SpanID     string            `json:"span_id"`
	ParentID   string            `json:"parent_span_id"`
	Name       string            `json:"name"`
	StartNs    int64             `json:"start_time_unix_nano"`
	EndNs      int64             `json:"end_time_unix_nano"`
	Status     int               `json:"status_code"` // 0=ok, 2=error
	Attributes map[string]string `json:"attributes"`
	Service    string            `json:"service_name"`
}

func TestOTLPSpan_DurationCalc(t *testing.T) {
	span := OTLPSpan{
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:  "00f067aa0ba902b7",
		Name:    "POST /api/checkout/confirm",
		StartNs: 1711234567_000000000,
		EndNs:   1711234571_820000000, // 4820ms later
		Status:  2, // error
		Service: "checkout-service",
	}

	durationMs := float64(span.EndNs-span.StartNs) / 1_000_000
	if math.Abs(durationMs - 4820.0) > 0.1 {
		t.Errorf("duration: got %.1fms want 4820.0ms", durationMs)
	}
	if span.Status != 2 { t.Error("status should be error (2)") }
	t.Logf("✅ OTLP span duration: %.1fms service=%s status=%d", durationMs, span.Service, span.Status)
}

func TestOTLPSpan_TraceTree(t *testing.T) {
	// Build a trace tree from spans
	spans := []OTLPSpan{
		{SpanID:"root",   ParentID:"",     Name:"api-gateway: POST /checkout",     StartNs:1000, EndNs:5820},
		{SpanID:"child1", ParentID:"root", Name:"checkout-service: validate",      StartNs:1018, EndNs:5815},
		{SpanID:"child2", ParentID:"root", Name:"payment-service: process",        StartNs:1052, EndNs:3899},
		{SpanID:"child3", ParentID:"child1",Name:"db: SELECT cart_items",          StartNs:2900, EndNs:2989},
	}

	// Find root spans (no parent)
	roots := []OTLPSpan{}
	children := map[string][]OTLPSpan{}
	for _, s := range spans {
		if s.ParentID == "" { roots = append(roots, s) } else { children[s.ParentID] = append(children[s.ParentID], s) }
	}

	if len(roots) != 1 { t.Errorf("expected 1 root span, got %d", len(roots)) }
	if len(children["root"]) != 2 { t.Errorf("root should have 2 children, got %d", len(children["root"])) }
	if len(children["child1"]) != 1 { t.Errorf("child1 should have 1 child, got %d", len(children["child1"])) }

	t.Logf("✅ Trace tree: %d roots, %d root-children, %d nested-children",
		len(roots), len(children["root"]), len(children["child1"]))
}

// ══ RUM Event Parsing ════════════════════════════════════════════════════════

type RUMEvent struct {
	SessionID  string            `json:"session_id"`
	EventType  string            `json:"type"` // lcp|fid|cls|error|resource
	Value      float64           `json:"value"`
	URL        string            `json:"url"`
	UserAgent  string            `json:"user_agent"`
	Timestamp  int64             `json:"timestamp"`
	Attributes map[string]string `json:"attributes"`
}

func TestRUMEvent_WebVitals(t *testing.T) {
	// Web Vitals thresholds per Google
	thresholds := map[string]struct{ good, poor float64 }{
		"lcp": {2500, 4000},  // ms
		"inp": {200, 500},   // ms
		"cls": {0.1, 0.25},  // unitless
	}

	events := []struct {
		event  RUMEvent
		rating string
	}{
		{RUMEvent{EventType:"lcp", Value:1200}, "good"},    // <2500ms = good
		{RUMEvent{EventType:"lcp", Value:3200}, "needs improvement"},
		{RUMEvent{EventType:"lcp", Value:5400}, "poor"},    // >4000ms = poor
		{RUMEvent{EventType:"cls", Value:0.05}, "good"},    // <0.1 = good
		{RUMEvent{EventType:"cls", Value:0.35}, "poor"},    // >0.25 = poor
	}

	rateVital := func(ev RUMEvent) string {
		t := thresholds[ev.EventType]
		if ev.Value <= t.good { return "good" }
		if ev.Value <= t.poor { return "needs improvement" }
		return "poor"
	}

	for _, e := range events {
		got := rateVital(e.event)
		if got != e.rating {
			t.Errorf("%s value=%.0f: rating=%q want %q", e.event.EventType, e.event.Value, got, e.rating)
		}
	}
	t.Log("✅ Web Vitals rating (LCP/CLS): Google thresholds applied correctly")
}

func TestRUMEvent_JSError(t *testing.T) {
	errEvent := RUMEvent{
		SessionID: "sess-8f4a2b",
		EventType: "error",
		URL:       "/checkout",
		UserAgent: "Mozilla/5.0 (Chrome/123)",
		Attributes: map[string]string{
			"message":  "Cannot read properties of undefined (reading 'price')",
			"filename": "checkout-v2.js",
			"lineno":   "842",
			"colno":    "12",
		},
	}
	if errEvent.EventType != "error" { t.Error("event type") }
	if errEvent.Attributes["message"] == "" { t.Error("error message missing") }
	if errEvent.Attributes["filename"] == "" { t.Error("filename missing") }
	t.Logf("✅ RUM JS Error: %s at %s:%s", errEvent.Attributes["message"][:40], errEvent.Attributes["filename"], errEvent.Attributes["lineno"])
}

// ══ Agent Heartbeat Parsing ═══════════════════════════════════════════════════

type HeartbeatPayload struct {
	AgentID  string  `json:"agent_id"`
	NodeName string  `json:"node_name"`
	Type     string  `json:"type"` // linux|windows|kubernetes
	Version  string  `json:"version"`
	UptimeS  float64 `json:"uptime_seconds"`
	CPUPct   float64 `json:"cpu_pct"`
	MemPct   float64 `json:"mem_pct"`
}

func TestHeartbeat_AllAgentTypes(t *testing.T) {
	heartbeats := []HeartbeatPayload{
		{"prod-linux-01", "prod-linux-01.corp.io", "linux",      "2.4.1", 4_060_800, 42.4, 61.2},
		{"prod-win-01",   "WIN-PROD-01",            "windows",    "2.4.1", 1_209_600, 22.4, 45.8},
		{"k8s-daemonset", "k8s-node-04",            "kubernetes", "2.4.1", 604_800,   67.4, 72.1},
	}
	validTypes := map[string]bool{"linux": true, "windows": true, "kubernetes": true}

	for _, h := range heartbeats {
		if !validTypes[h.Type] { t.Errorf("invalid agent type: %q", h.Type) }
		if h.AgentID == "" { t.Error("agent_id empty") }
		if h.CPUPct < 0 || h.CPUPct > 100 { t.Errorf("cpu_pct out of range: %.1f", h.CPUPct) }
		if h.MemPct < 0 || h.MemPct > 100 { t.Errorf("mem_pct out of range: %.1f", h.MemPct) }
		t.Logf("✅ Heartbeat: %s (%s) CPU=%.1f%% MEM=%.1f%%", h.NodeName, h.Type, h.CPUPct, h.MemPct)
	}
}
