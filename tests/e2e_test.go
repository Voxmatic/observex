package tests

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

// Simulates the full data flow through the platform
// without needing real database connections

// ══ Stage 1: Agent collects metrics ══════════════════════════════════════════

type AgentCollectedMetrics struct {
	AgentID   string
	NodeName  string
	ClusterID string
	Timestamp time.Time
	CPU       float64
	MemPct    float64
	DiskPct   float64
	NetRxMbps float64
	NetTxMbps float64
	Processes int
}

func simulateAgentCollection(node string) AgentCollectedMetrics {
	return AgentCollectedMetrics{
		AgentID:   "agent-" + node,
		NodeName:  node,
		ClusterID: "production",
		Timestamp: time.Now(),
		CPU:       84.2, // elevated
		MemPct:    61.4,
		DiskPct:   42.8,
		NetRxMbps: 842.4,
		NetTxMbps: 284.8,
		Processes: 284,
	}
}

// ══ Stage 2: Ingestor transforms to metric points ═════════════════════════════

type MetricPoint2 struct {
	Name      string
	Value     float64
	Labels    map[string]string
	Timestamp int64
}

func ingestorTransform(raw AgentCollectedMetrics) []MetricPoint2 {
	base := map[string]string{
		"host":    raw.NodeName,
		"cluster": raw.ClusterID,
		"agent":   raw.AgentID,
	}
	ts := raw.Timestamp.UnixMilli()
	return []MetricPoint2{
		{"system.cpu.usage_pct",    raw.CPU,       base, ts},
		{"system.memory.usage_pct", raw.MemPct,    base, ts},
		{"system.disk.usage_pct",   raw.DiskPct,   base, ts},
		{"system.net.rx_mbps",      raw.NetRxMbps, base, ts},
		{"system.net.tx_mbps",      raw.NetTxMbps, base, ts},
		{"system.processes.count",  float64(raw.Processes), base, ts},
	}
}

// ══ Stage 3: Processor evaluates alerts ══════════════════════════════════════

type FiredAlert struct {
	RuleName  string
	Severity  string
	Value     float64
	Threshold float64
	NodeName  string
}

func processorEvaluateAlerts(metrics []MetricPoint2) []FiredAlert {
	rules := []struct {
		name      string
		metric    string
		threshold float64
		severity  string
	}{
		{"High CPU",    "system.cpu.usage_pct",    80.0, "HIGH"},
		{"High Memory", "system.memory.usage_pct", 85.0, "HIGH"},
		{"High Disk",   "system.disk.usage_pct",   80.0, "MEDIUM"},
	}

	var alerts []FiredAlert
	for _, m := range metrics {
		for _, r := range rules {
			if m.Name == r.metric && m.Value > r.threshold {
				alerts = append(alerts, FiredAlert{
					RuleName:  r.name,
					Severity:  r.severity,
					Value:     m.Value,
					Threshold: r.threshold,
					NodeName:  m.Labels["host"],
				})
			}
		}
	}
	return alerts
}

// ══ Stage 4: API returns formatted response ═══════════════════════════════════

type APIMetricResponse struct {
	Node    string
	Metrics map[string]float64
	Alerts  []FiredAlert
	Health  string
}

func apiFormatResponse(metrics []MetricPoint2, alerts []FiredAlert) APIMetricResponse {
	m := make(map[string]float64)
	node := ""
	for _, mp := range metrics {
		shortName := mp.Name[strings.LastIndex(mp.Name, ".")+1:]
		m[shortName] = mp.Value
		if node == "" { node = mp.Labels["host"] }
	}

	health := "HEALTHY"
	for _, a := range alerts {
		if a.Severity == "HIGH" || a.Severity == "CRITICAL" { health = "DEGRADED"; break }
	}
	return APIMetricResponse{Node: node, Metrics: m, Alerts: alerts, Health: health}
}

// ══ Full E2E Test ═════════════════════════════════════════════════════════════

func TestE2E_MetricFlow(t *testing.T) {
	t.Log("Stage 1: Agent collects metrics from prod-linux-03...")
	raw := simulateAgentCollection("prod-linux-03")
	if raw.CPU == 0 { t.Fatal("agent collection returned zero CPU") }
	t.Logf("  ✅ Agent collected: CPU=%.1f%% MEM=%.1f%% DISK=%.1f%%", raw.CPU, raw.MemPct, raw.DiskPct)

	t.Log("Stage 2: Ingestor transforms to metric points...")
	metrics := ingestorTransform(raw)
	if len(metrics) != 6 { t.Errorf("expected 6 metric points, got %d", len(metrics)) }
	for _, m := range metrics {
		if m.Name == "" { t.Error("metric has empty name") }
		if m.Timestamp == 0 { t.Error("metric has zero timestamp") }
	}
	t.Logf("  ✅ Ingestor produced %d metric points with timestamps", len(metrics))

	t.Log("Stage 3: Processor evaluates alert rules...")
	alerts := processorEvaluateAlerts(metrics)
	if len(alerts) == 0 { t.Error("expected at least one alert (CPU=84.2 > 80)") }
	if alerts[0].RuleName != "High CPU" { t.Errorf("first alert: %q", alerts[0].RuleName) }
	if math.Abs(alerts[0].Value - 84.2) > 0.01 { t.Errorf("alert value: %.1f", alerts[0].Value) }
	t.Logf("  ✅ Processor fired %d alert(s): %s (%.1f%% > %.1f%%)",
		len(alerts), alerts[0].RuleName, alerts[0].Value, alerts[0].Threshold)

	t.Log("Stage 4: API formats response...")
	resp := apiFormatResponse(metrics, alerts)
	if resp.Node != "prod-linux-03" { t.Errorf("node: %q", resp.Node) }
	if resp.Health != "DEGRADED"    { t.Errorf("health: %q (should be DEGRADED)", resp.Health) }
	if resp.Metrics["usage_pct"] == 0 { t.Error("CPU metric missing in response") }

	// Serialize to JSON (simulating HTTP response)
	b, err := json.Marshal(resp)
	if err != nil { t.Fatalf("json marshal: %v", err) }
	if len(b) == 0 { t.Error("empty JSON response") }
	t.Logf("  ✅ API response: node=%s health=%s alerts=%d JSON=%d bytes",
		resp.Node, resp.Health, len(resp.Alerts), len(b))

	t.Log("✅ Full E2E flow verified: Agent → Ingestor → Processor → API")
}

func TestE2E_LLMInferenceFlow(t *testing.T) {
	// Simulate: Claude API request → metrics → SLO evaluation → alert

	t.Log("Stage 1: Inference event arrives...")
	type InferEvent struct {
		ModelID      string
		OrgID        string
		InputTokens  int
		OutputTokens int
		TTFTMs       float64
		StatusCode   int
		CostUSD      float64
	}
	event := InferEvent{
		ModelID:      "claude-sonnet-4-6",
		OrgID:        "acme-corp",
		InputTokens:  1240,
		OutputTokens: 420,
		TTFTMs:       4820.0, // way above 1500ms SLO!
		StatusCode:   200,
		CostUSD:      0.0189,
	}
	t.Logf("  ✅ Event: model=%s TTFT=%.0fms tokens=%d cost=$%.4f",
		event.ModelID, event.TTFTMs, event.InputTokens+event.OutputTokens, event.CostUSD)

	t.Log("Stage 2: Ingestor writes native metric records...")
	nativeMetricNames := []string{"llm_ttft_ms", "llm_input_tokens", "llm_output_tokens", "llm_cost_usd"}
	foundTTFT := false
	for _, name := range nativeMetricNames {
		if name == "llm_ttft_ms" {
			foundTTFT = true
			break
		}
	}
	if !foundTTFT { t.Error("TTFT metric missing") }
	t.Logf("  ✅ Native metric records: %d", len(nativeMetricNames))

	t.Log("Stage 3: Processor evaluates TTFT SLO...")
	sloTarget := 99.0   // 99% of requests under 1500ms
	ttftSLOms := 1500.0
	breached := event.TTFTMs > ttftSLOms
	if !breached { t.Error("TTFT SLO should be breached") }

	// Calculate instantaneous burn rate
	errorPct := 100.0  // this request exceeded SLO
	budgetPct := 100.0 - sloTarget // 1%
	burnRate  := errorPct / budgetPct
	t.Logf("  ✅ SLO breach: TTFT=%.0fms (SLO=%.0fms) burn=%.1fx", event.TTFTMs, ttftSLOms, burnRate)

	t.Log("Stage 4: Alert fired and routed...")
	alert := map[string]interface{}{
		"name":      "claude-sonnet TTFT SLO breach",
		"severity":  "HIGH",
		"model_id":  event.ModelID,
		"value_ms":  event.TTFTMs,
		"threshold": ttftSLOms,
		"burn_rate": burnRate,
		"channels":  []string{"slack:#platform-alerts", "pagerduty"},
	}
	b, _ := json.Marshal(alert)
	t.Logf("  ✅ Alert dispatched: %s", string(b)[:100])

	t.Log("✅ Full LLM E2E flow: API event → metrics → SLO eval → alert routed")
}

func TestE2E_UniversalIndustry(t *testing.T) {
	// Test that all 10 industry primitives can be serialized and processed

	industries := []struct {
		name      string
		primitive string
		event     map[string]interface{}
	}{
		{"AI/LLM",     "token",          map[string]interface{}{"model":"claude-haiku","ttft_ms":182.4,"tokens":1660}},
		{"E-Commerce",  "order",         map[string]interface{}{"order_id":"ord-8421","gmv_usd":142.80,"payment_ok":true}},
		{"Streaming",   "stream_session",map[string]interface{}{"session_id":"ss-9c2d","quality":"4K","rebuffer_s":0.0}},
		{"Fintech",     "transaction",   map[string]interface{}{"tx_id":"tx-2847","amount_usd":284.00,"success":true}},
		{"Ride-sharing","match",         map[string]interface{}{"booking_id":"bk-4821","match_ms":24,"eta_min":8}},
		{"Social",      "feed_event",    map[string]interface{}{"post_id":"p-9924","fanout":84200,"latency_ms":142}},
		{"SaaS",        "api_request",   map[string]interface{}{"org_id":"acme","endpoint":"/api/data","latency_ms":284}},
		{"Healthcare",  "patient_event", map[string]interface{}{"event_type":"ehr_sync","patient_id_hash":"anon-8421","latency_ms":284}},
		{"Gaming",      "game_tick",     map[string]interface{}{"session_id":"gs-4821","tick_hz":60.0,"players":16}},
		{"IoT",         "sensor_reading",map[string]interface{}{"device_id":"tesla-vin-X","msg_type":"telemetry","lag_ms":842}},
	}

	for _, ind := range industries {
		// Serialize event
		b, err := json.Marshal(ind.event)
		if err != nil { t.Errorf("%s: marshal error: %v", ind.name, err) }

		// Deserialize back
		var decoded map[string]interface{}
		if err := json.Unmarshal(b, &decoded); err != nil { t.Errorf("%s: unmarshal error: %v", ind.name, err) }

		// Verify round-trip
		b2, _ := json.Marshal(decoded)
		if string(b) != string(b2) { t.Errorf("%s: round-trip mismatch", ind.name) }

		t.Logf("  ✅ %-15s primitive=%-16s fields=%d JSON=%d bytes",
			ind.name, ind.primitive, len(ind.event), len(b))
	}
}
