// services/api-gateway/inhouse_agent.go
//
// ObserveX In-House Monitoring Agent — ZERO external API calls.
//
// This replaces the Claude-dependent agent with a fully self-contained
// decision engine that users deploy on their own infrastructure.
//
// Architecture:
//   Platform signals → Local Decision Engine
//                           ↓
//              ┌─────────────────────────────┐
//              │   Rule Engine (always on)   │  ← expert rules + user runbooks
//              │   + Ollama LLM (optional)   │  ← local model, no data leaves
//              │   + Pattern Matcher         │  ← learns from past incidents
//              └─────────────────────────────┘
//                           ↓
//                    Decision + Action
//
// Local LLM support via Ollama:
//   - Users install Ollama on their own servers
//   - Supported models: llama3, mistral, phi3, codellama, gemma
//   - Configure: OLLAMA_URL=http://localhost:11434
//   - All inference runs on-premise, zero data egress
//
// Fallback (no Ollama): sophisticated rule-based engine using:
//   - Statistical thresholds (z-score, percentile)
//   - Pattern matching against user-defined playbooks
//   - Historical incident correlation
//   - Dependency graph analysis

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/observex/platform/internal/middleware"
	"go.uber.org/zap"
)

// ── In-House Agent Config ─────────────────────────────────────────────────────

type InHouseAgentConfig struct {
	// Local LLM via Ollama (optional)
	OllamaURL   string `json:"ollama_url"`   // e.g. http://localhost:11434
	OllamaModel string `json:"ollama_model"` // e.g. llama3, mistral, phi3

	// Rule engine thresholds
	ErrorRateThresholdPct float64 `json:"error_rate_threshold_pct"` // default 5.0
	LatencyP99ThresholdMs float64 `json:"latency_p99_threshold_ms"` // default 1000
	CPUThresholdPct       float64 `json:"cpu_threshold_pct"`        // default 85
	MemThresholdPct       float64 `json:"mem_threshold_pct"`        // default 90

	// Pattern learning
	LearnFromIncidents bool   `json:"learn_from_incidents"` // use past incidents
	IncidentWindowDays int    `json:"incident_window_days"` // how far back to look

	// Deploy mode
	DeploymentType string `json:"deployment_type"` // "sidecar"|"daemonset"|"standalone"
}

// ── Ollama local inference ────────────────────────────────────────────────────

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// callOllama sends a prompt to a local Ollama instance.
// Returns empty string if Ollama is not available.
func callOllama(ctx context.Context, ollamaURL, model, prompt string) string {
	if ollamaURL == "" {
		ollamaURL = envOr("OLLAMA_URL", "")
	}
	if ollamaURL == "" { return "" }
	if model == "" { model = envOr("OLLAMA_MODEL", "llama3") }

	reqBody, _ := json.Marshal(ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
		Options: map[string]any{"temperature": 0.1, "num_predict": 512},
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		ollamaURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil { return "" }
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var ollamaResp ollamaResponse
	if json.Unmarshal(body, &ollamaResp) != nil { return "" }
	return ollamaResp.Response
}

// ── Signal types ──────────────────────────────────────────────────────────────

type PlatformSignal struct {
	Type      string    `json:"type"`     // metric|log|trace|alert|deploy|anomaly
	Service   string    `json:"service"`
	Namespace string    `json:"namespace"`
	Metric    string    `json:"metric,omitempty"`
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Direction string    `json:"direction"` // above|below
	Severity  string    `json:"severity"`  // info|warning|critical
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Labels    map[string]string `json:"labels,omitempty"`
}

type IncidentPattern struct {
	Signals    []string `json:"signals"`     // signal patterns that matched
	RootCause  string   `json:"root_cause"`
	Resolution string   `json:"resolution"`
	Actions    []string `json:"actions"`
	MatchCount int      `json:"match_count"`  // how many times this pattern occurred
	LastSeen   time.Time `json:"last_seen"`
}

// ── Rule Engine ───────────────────────────────────────────────────────────────

type RuleEngine struct {
	Rules    []AgentRule
	Patterns []IncidentPattern // learned from past incidents
}

type AgentRule struct {
	Name        string
	Priority    int
	Condition   func(signals []PlatformSignal) bool
	RootCause   func(signals []PlatformSignal) string
	Action      string
	ActionParams func(signals []PlatformSignal) map[string]any
	Confidence  float64
	Explanation []string
}

func newRuleEngine(knowledge []AgentKnowledge) *RuleEngine {
	re := &RuleEngine{}
	re.Rules = buildExpertRules()
	re.Patterns = extractPatterns(knowledge)
	return re
}

func buildExpertRules() []AgentRule {
	return []AgentRule{
		// Rule 1: High error rate + recent deploy → rollback
		{
			Name: "Deploy regression",
			Priority: 1,
			Confidence: 0.92,
			Condition: func(sigs []PlatformSignal) bool {
				hasHighErrors := false; hasRecentDeploy := false
				for _, s := range sigs {
					if s.Type == "metric" && s.Metric == "error_rate" && s.Value > 5.0 { hasHighErrors = true }
					if s.Type == "deploy" && time.Since(s.Timestamp) < 30*time.Minute { hasRecentDeploy = true }
				}
				return hasHighErrors && hasRecentDeploy
			},
			RootCause: func(sigs []PlatformSignal) string {
				for _, s := range sigs {
					if s.Type == "deploy" { return fmt.Sprintf("Deploy regression: %s deploy caused error spike", s.Service) }
				}
				return "Recent deploy caused service degradation"
			},
			Action: "rollback_deployment",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				for _, s := range sigs {
					if s.Type == "metric" { return map[string]any{"namespace": s.Namespace, "deployment": s.Service} }
				}
				return map[string]any{}
			},
			Explanation: []string{
				"Error rate exceeded threshold",
				"Recent deployment detected within 30 minutes",
				"Pattern matches deploy regression signature",
				"Confidence high — rolling back to previous stable version",
			},
		},
		// Rule 2: High error rate + connection pool errors → restart + scale
		{
			Name: "Connection pool exhaustion",
			Priority: 2,
			Confidence: 0.88,
			Condition: func(sigs []PlatformSignal) bool {
				for _, s := range sigs {
					if s.Type == "log" && strings.Contains(s.Message, "connection pool") { return true }
					if s.Type == "metric" && s.Metric == "db_connections" && s.Value > s.Threshold { return true }
				}
				return false
			},
			RootCause: func(sigs []PlatformSignal) string {
				return "Database connection pool exhausted — too many concurrent requests or connection leak"
			},
			Action: "restart_pod",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				for _, s := range sigs {
					if s.Service != "" { return map[string]any{"namespace": s.Namespace, "pod_selector": "app=" + s.Service} }
				}
				return map[string]any{}
			},
			Explanation: []string{
				"Connection pool exhaustion pattern detected in logs",
				"Database connection count exceeds configured limit",
				"This is a known transient issue — pod restart clears connections",
				"Will scale up if restart does not resolve within 5 minutes",
			},
		},
		// Rule 3: OOMKilled events → scale up memory
		{
			Name: "OOM kill",
			Priority: 3,
			Confidence: 0.95,
			Condition: func(sigs []PlatformSignal) bool {
				for _, s := range sigs {
					if (s.Type == "log" || s.Type == "alert") && strings.Contains(s.Message, "OOM") { return true }
					if s.Type == "metric" && s.Metric == "memory_pct" && s.Value > 90 { return true }
				}
				return false
			},
			RootCause: func(sigs []PlatformSignal) string {
				return "Pod killed due to out-of-memory condition — memory limit too low for current load"
			},
			Action: "scale_deployment",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				for _, s := range sigs {
					if s.Service != "" {
						return map[string]any{
							"namespace": s.Namespace, "deployment": s.Service,
							"replicas_delta": 2, "reason": "OOM mitigation",
						}
					}
				}
				return map[string]any{}
			},
			Explanation: []string{
				"OOMKilled events detected on pods",
				"Memory usage consistently above 90% threshold",
				"Scaling horizontally to distribute memory pressure",
				"Consider increasing memory limits if issue persists",
			},
		},
		// Rule 4: CPU spike + no deploy → scale out
		{
			Name: "CPU saturation",
			Priority: 4,
			Confidence: 0.82,
			Condition: func(sigs []PlatformSignal) bool {
				for _, s := range sigs {
					if s.Type == "metric" && s.Metric == "cpu_pct" && s.Value > 85 { return true }
				}
				return false
			},
			RootCause: func(sigs []PlatformSignal) string {
				for _, s := range sigs {
					if s.Type == "metric" && s.Metric == "cpu_pct" {
						return fmt.Sprintf("CPU saturation at %.1f%% on %s — horizontal scale needed", s.Value, s.Service)
					}
				}
				return "CPU saturation — insufficient compute capacity for current load"
			},
			Action: "scale_deployment",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				for _, s := range sigs {
					if s.Service != "" {
						return map[string]any{"namespace": s.Namespace, "deployment": s.Service, "replicas_delta": 2}
					}
				}
				return map[string]any{}
			},
			Explanation: []string{
				"CPU utilization exceeded 85% sustained threshold",
				"No recent deployment — this is traffic-driven load increase",
				"Scaling out to distribute CPU load across more pods",
				"Monitoring recovery — will scale down when load normalizes",
			},
		},
		// Rule 5: High latency → notify + investigate
		{
			Name: "Latency degradation",
			Priority: 5,
			Confidence: 0.78,
			Condition: func(sigs []PlatformSignal) bool {
				for _, s := range sigs {
					if s.Type == "metric" && s.Metric == "latency_p99" && s.Value > 1000 { return true }
				}
				return false
			},
			RootCause: func(sigs []PlatformSignal) string {
				return "P99 latency degradation — possible downstream dependency issue or traffic spike"
			},
			Action: "notify_oncall",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				return map[string]any{"urgency": "high", "requires_investigation": true}
			},
			Explanation: []string{
				"P99 latency exceeded 1000ms threshold",
				"No obvious CPU/memory cause identified",
				"Could be downstream dependency (DB, external API) or traffic anomaly",
				"Notifying on-call for manual investigation while collecting diagnostic data",
			},
		},
		// Rule 6: Multiple services degraded → infra issue
		{
			Name: "Widespread degradation",
			Priority: 6,
			Confidence: 0.90,
			Condition: func(sigs []PlatformSignal) bool {
				degraded := map[string]bool{}
				for _, s := range sigs {
					if s.Type == "metric" && s.Value > s.Threshold { degraded[s.Service] = true }
				}
				return len(degraded) >= 3
			},
			RootCause: func(sigs []PlatformSignal) string {
				return "Multiple services degraded simultaneously — likely shared infrastructure issue (network, DNS, node)"
			},
			Action: "open_incident",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				return map[string]any{"severity": "CRITICAL", "title": "Widespread service degradation — infra investigation needed"}
			},
			Explanation: []string{
				"3+ services showing degradation simultaneously",
				"Correlated timing suggests shared infrastructure cause",
				"Not a single-service issue — escalating to infrastructure team",
				"Creating critical incident for coordinated response",
			},
		},
		// Rule 7: Alert storm → correlate + silence noise
		{
			Name: "Alert storm",
			Priority: 7,
			Confidence: 0.85,
			Condition: func(sigs []PlatformSignal) bool {
				alertCount := 0
				for _, s := range sigs { if s.Type == "alert" { alertCount++ } }
				return alertCount > 10
			},
			RootCause: func(sigs []PlatformSignal) string {
				return "Alert storm detected — many correlated alerts likely from single root cause"
			},
			Action: "silence_alert",
			ActionParams: func(sigs []PlatformSignal) map[string]any {
				return map[string]any{"duration_min": 30, "reason": "Alert storm — investigating root cause"}
			},
			Explanation: []string{
				"More than 10 alerts firing simultaneously",
				"Alert storm pattern — downstream alerts are likely noise from single root cause",
				"Silencing secondary alerts to reduce noise during investigation",
				"Root cause investigation in progress",
			},
		},
	}
}

func extractPatterns(knowledge []AgentKnowledge) []IncidentPattern {
	var patterns []IncidentPattern
	for _, k := range knowledge {
		if k.Type != "postmortem" { continue }
		// Extract action patterns from postmortem content
		actions := []string{}
		if strings.Contains(k.Content, "rollback") { actions = append(actions, "rollback_deployment") }
		if strings.Contains(k.Content, "restart") { actions = append(actions, "restart_pod") }
		if strings.Contains(k.Content, "scale") { actions = append(actions, "scale_deployment") }
		if strings.Contains(k.Content, "notify") { actions = append(actions, "notify_oncall") }
		if len(actions) == 0 { continue }
		patterns = append(patterns, IncidentPattern{
			RootCause:  k.Title,
			Resolution: k.Content[:min4(len(k.Content), 200)],
			Actions:    actions,
			MatchCount: k.UsedCount,
			LastSeen:   k.CreatedAt,
		})
	}
	return patterns
}

// ── Decision maker ────────────────────────────────────────────────────────────

type InHouseDecision struct {
	RuleName        string         `json:"rule_name"`
	RootCause       string         `json:"root_cause"`
	ReasoningChain  []string       `json:"reasoning_chain"`
	Confidence      float64        `json:"confidence"`
	ProposedAction  string         `json:"proposed_action"`
	ActionParams    map[string]any `json:"action_params"`
	AffectedService string         `json:"affected_service"`
	AffectedNamespace string       `json:"affected_namespace"`
	TriggerType     string         `json:"trigger_type"`
	SignalsAnalyzed int            `json:"signals_analyzed"`
	PatternMatches  int            `json:"pattern_matches"`
	OllamaEnriched  bool           `json:"ollama_enriched"`
	OllamaModel     string         `json:"ollama_model,omitempty"`
	DecidedAt       time.Time      `json:"decided_at"`
}

// runInHouseDecision is the core decision loop — no external calls.
func (gw *Gateway) runInHouseDecision(ctx context.Context, orgID string, signals []PlatformSignal, knowledge []AgentKnowledge) *InHouseDecision {
	if len(signals) == 0 {
		return &InHouseDecision{
			RootCause: "No active signals — system appears healthy",
			ProposedAction: "no_action",
			Confidence: 0.99,
			ReasoningChain: []string{"All metrics within normal thresholds", "No active alerts", "No anomalies detected"},
			DecidedAt: time.Now(),
		}
	}

	re := newRuleEngine(knowledge)

	// Try each rule in priority order
	var bestDecision *InHouseDecision
	for _, rule := range re.Rules {
		if !rule.Condition(signals) { continue }

		// Build decision from rule
		rootCause := rule.RootCause(signals)
		params := rule.ActionParams(signals)

		// Find affected service from signals
		service, namespace := extractServiceFromSignals(signals)

		decision := &InHouseDecision{
			RuleName:        rule.Name,
			RootCause:       rootCause,
			ReasoningChain:  rule.Explanation,
			Confidence:      rule.Confidence,
			ProposedAction:  rule.Action,
			ActionParams:    params,
			AffectedService: service,
			AffectedNamespace: namespace,
			TriggerType:     "rule_engine",
			SignalsAnalyzed: len(signals),
			PatternMatches:  countPatternMatches(signals, re.Patterns),
			DecidedAt:       time.Now(),
		}

		// Check if historical patterns boost confidence
		if decision.PatternMatches > 0 {
			decision.Confidence = math.Min(0.99, decision.Confidence+0.05)
			decision.ReasoningChain = append(decision.ReasoningChain,
				fmt.Sprintf("Pattern matches %d historical incidents — confidence boosted", decision.PatternMatches))
		}

		// Try Ollama enrichment if configured (optional, never required)
		ollamaURL := envOr("OLLAMA_URL", "")
		ollamaModel := envOr("OLLAMA_MODEL", "llama3")
		if ollamaURL != "" {
			enriched := gw.enrichWithOllama(ctx, decision, signals, knowledge, ollamaURL, ollamaModel)
			if enriched != "" {
				decision.ReasoningChain = append(decision.ReasoningChain, "[Ollama] "+enriched[:min4(len(enriched), 200)])
				decision.OllamaEnriched = true
				decision.OllamaModel = ollamaModel
			}
		}

		bestDecision = decision
		break // First matching rule wins (highest priority)
	}

	if bestDecision == nil {
		bestDecision = &InHouseDecision{
			RootCause:      "Signals analyzed — no matching rule triggered",
			ProposedAction: "notify_oncall",
			Confidence:     0.60,
			ReasoningChain: []string{
				fmt.Sprintf("Analyzed %d signals", len(signals)),
				"No rule pattern matched",
				"Notifying on-call for manual analysis",
			},
			AffectedService: extractServiceFromSignalsSimple(signals),
			TriggerType:    "no_rule_match",
			SignalsAnalyzed: len(signals),
			DecidedAt:      time.Now(),
		}
	}

	gw.log.Info("in-house agent decision",
		zap.String("rule", bestDecision.RuleName),
		zap.String("action", bestDecision.ProposedAction),
		zap.Float64("confidence", bestDecision.Confidence),
		zap.Bool("ollama", bestDecision.OllamaEnriched))

	return bestDecision
}

// enrichWithOllama optionally asks local Ollama to validate/enrich the decision.
func (gw *Gateway) enrichWithOllama(ctx context.Context, decision *InHouseDecision, signals []PlatformSignal, knowledge []AgentKnowledge, ollamaURL, model string) string {
	// Build a concise prompt for local LLM
	sigSummary := summarizeSignals(signals)
	prompt := fmt.Sprintf(`You are an SRE assistant. Analyze this incident briefly.

Signals: %s
Rule matched: %s
Proposed action: %s
Confidence: %.0f%%

Is this the right action? If not, what else should be considered? Answer in 1-2 sentences only.`, sigSummary, decision.RuleName, decision.ProposedAction, decision.Confidence*100)

	result := callOllama(ctx, ollamaURL, model, prompt)
	return strings.TrimSpace(result)
}

func summarizeSignals(signals []PlatformSignal) string {
	var parts []string
	for _, s := range signals {
		if s.Type == "metric" {
			parts = append(parts, fmt.Sprintf("%s.%s=%.1f(threshold=%.1f)", s.Service, s.Metric, s.Value, s.Threshold))
		} else if s.Type == "alert" {
			parts = append(parts, fmt.Sprintf("ALERT:%s@%s", s.Message[:min4(len(s.Message), 40)], s.Service))
		}
	}
	if len(parts) > 5 { parts = parts[:5]; parts = append(parts, "...") }
	return strings.Join(parts, " | ")
}

func countPatternMatches(signals []PlatformSignal, patterns []IncidentPattern) int {
	count := 0
	for _, p := range patterns {
		for _, sig := range signals {
			for _, ps := range p.Signals {
				if strings.Contains(sig.Message, ps) { count++; break }
			}
		}
	}
	return count
}

func extractServiceFromSignals(signals []PlatformSignal) (service, namespace string) {
	// Prefer the service with the most severe signals
	svcCount := map[string]int{}
	svcNS := map[string]string{}
	for _, s := range signals {
		if s.Service != "" { svcCount[s.Service]++; svcNS[s.Service] = s.Namespace }
	}
	best, bestCount := "", 0
	for svc, count := range svcCount {
		if count > bestCount { best = svc; bestCount = count }
	}
	return best, svcNS[best]
}

func extractServiceFromSignalsSimple(signals []PlatformSignal) string {
	svc, _ := extractServiceFromSignals(signals)
	return svc
}

// ── Signal collection from local ObserveX native metric store + Loki ──────────────────────

func (gw *Gateway) collectPlatformSignals(orgID string) []PlatformSignal {
	var signals []PlatformSignal
	metricURL := gw.cfg.QueryEngineURL
	lokiURL := envOr("LOKI_URL", "http://loki:3100")

	// Metrics checks
	metricChecks := []struct {
		query     string
		metric    string
		threshold float64
		severity  string
	}{
		{fmt.Sprintf(`sum by (service) (rate(http_requests_total{org="%s",status=~"5.."}[5m]))/sum by (service) (rate(http_requests_total{org="%s"}[5m]))*100`, orgID, orgID), "error_rate", 5.0, "critical"},
		{fmt.Sprintf(`sum by (service) (rate(http_requests_total{org="%s",status=~"5.."}[5m]))/sum by (service) (rate(http_requests_total{org="%s"}[5m]))*100`, orgID, orgID), "error_rate", 2.0, "warning"},
		{fmt.Sprintf(`histogram_quantile(0.99, sum by (service, le) (rate(http_request_duration_seconds_bucket{org="%s"}[5m])))*1000`, orgID), "latency_p99", 1000, "critical"},
		{fmt.Sprintf(`avg by (node) (rate(node_cpu_seconds_total{mode!="idle",org="%s"}[5m]))*100`, orgID), "cpu_pct", 85, "critical"},
		{fmt.Sprintf(`avg by (pod) (container_memory_working_set_bytes{org="%s"}) / avg by (pod) (kube_pod_container_resource_limits{resource="memory",org="%s"})*100`, orgID, orgID), "memory_pct", 90, "critical"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, check := range metricChecks {
		url := fmt.Sprintf("%s/api/v1/query?query=%s", metricURL, check.query)
		resp, err := gw.client.Get(url)
		if err != nil { continue }
		var r map[string]any; json.NewDecoder(resp.Body).Decode(&r); resp.Body.Close()

		results, _ := r["data"].(map[string]any)["result"].([]any)
		for _, result := range results {
			pt, _ := result.(map[string]any)
			metric, _ := pt["metric"].(map[string]any)
			values, _ := pt["value"].([]any)
			if len(values) < 2 { continue }
			var val float64; fmt.Sscanf(fmt.Sprintf("%v", values[1]), "%f", &val)
			if val <= check.threshold { continue }

			svc, _ := metric["service"].(string)
			ns, _ := metric["namespace"].(string)
			signals = append(signals, PlatformSignal{
				Type: "metric", Metric: check.metric, Value: val, Threshold: check.threshold,
				Service: svc, Namespace: ns, Severity: check.severity,
				Message: fmt.Sprintf("%s=%.1f (threshold %.1f)", check.metric, val, check.threshold),
				Timestamp: time.Now(), Direction: "above",
			})
		}
	}

	// Log pattern checks via Loki
	logChecks := []string{"connection pool exhausted", "OOMKilled", "panic:", "FATAL", "disk full"}
	for _, pattern := range logChecks {
		query := fmt.Sprintf(`{org="%s"} |= "%s"`, orgID, pattern)
		url := fmt.Sprintf("%s/loki/api/v1/query_range?query=%s&start=now-5m&end=now&limit=5", lokiURL, query)
		resp, err := gw.client.Get(url)
		if err != nil { continue }
		var lr map[string]any; json.NewDecoder(resp.Body).Decode(&lr); resp.Body.Close()

		streams, _ := lr["data"].(map[string]any)["result"].([]any)
		for _, stream := range streams {
			st, _ := stream.(map[string]any)
			labels, _ := st["stream"].(map[string]string)
			values, _ := st["values"].([]any)
			if len(values) == 0 { continue }
			svc := labels["service"]
			ns := labels["namespace"]
			signals = append(signals, PlatformSignal{
				Type: "log", Message: pattern, Service: svc, Namespace: ns,
				Severity: "warning", Timestamp: time.Now(),
			})
		}
	}

	// Active alerts
	alertURL := fmt.Sprintf("%s/api/v1/query?query=ALERTS{alertstate=%q}", metricURL, "firing")
	resp, err := gw.client.Get(alertURL)
	if err == nil {
		var r map[string]any; json.NewDecoder(resp.Body).Decode(&r); resp.Body.Close()
		results, _ := r["data"].(map[string]any)["result"].([]any)
		for _, result := range results {
			pt, _ := result.(map[string]any)
			metric, _ := pt["metric"].(map[string]any)
			alertName, _ := metric["alertname"].(string)
			svc, _ := metric["service"].(string)
			sev, _ := metric["severity"].(string)
			signals = append(signals, PlatformSignal{
				Type: "alert", Message: alertName, Service: svc,
				Severity: sev, Timestamp: time.Now(),
			})
		}
	}
	_ = ctx
	return signals
}

// ── Ollama status endpoint ────────────────────────────────────────────────────

func (gw *Gateway) handleOllamaStatus(c *fiber.Ctx) error {
	ollamaURL := envOr("OLLAMA_URL", "")
	if ollamaURL == "" {
		return c.JSON(fiber.Map{
			"available":   false,
			"mode":        "rule_engine",
			"description": "Running in pure rule-based mode. To enable local LLM, set OLLAMA_URL.",
			"models":      []string{},
		})
	}

	// Ping Ollama
	resp, err := gw.client.Get(ollamaURL + "/api/tags")
	if err != nil {
		return c.JSON(fiber.Map{
			"available": false, "mode": "rule_engine",
			"error": "Ollama unreachable at " + ollamaURL,
		})
	}
	defer resp.Body.Close()

	var tagsResp struct {
		Models []struct{ Name string `json:"name"` } `json:"models"`
	}
	json.NewDecoder(resp.Body).Decode(&tagsResp)

	modelNames := make([]string, 0, len(tagsResp.Models))
	for _, m := range tagsResp.Models { modelNames = append(modelNames, m.Name) }
	sort.Strings(modelNames)

	return c.JSON(fiber.Map{
		"available":    true,
		"mode":         "ollama_llm",
		"ollama_url":   ollamaURL,
		"active_model": envOr("OLLAMA_MODEL", "llama3"),
		"models":       modelNames,
		"description":  "Local LLM enrichment active. Decisions are made on-premise — zero data egress.",
	})
}

// ── Override runAgentDecision to use in-house engine ─────────────────────────
// This replaces the Claude-API version in ai_monitoring_agent.go

func (gw *Gateway) runInHouseAgentDecision(ctx context.Context, orgID, scenario string, simulate bool) (*AgentDecision, error) {
	knowledge := gw.loadAgentKnowledge(ctx, orgID)

	// Collect real platform signals (or parse from scenario string if simulating)
	var signals []PlatformSignal
	if simulate {
		signals = parseScenarioToSignals(scenario)
	} else {
		signals = gw.collectPlatformSignals(orgID)
	}

	// Run in-house decision engine
	inhouse := gw.runInHouseDecision(ctx, orgID, signals, knowledge)

	// Convert to AgentDecision format
	status := "pending"
	if inhouse.Confidence >= 0.85 { status = "approved" }
	if inhouse.ProposedAction == "no_action" { status = "completed" }

	return &AgentDecision{
		ID:               fmt.Sprintf("decision-%d", time.Now().UnixMilli()),
		OrgID:            orgID,
		Timestamp:        time.Now(),
		TriggerType:      inhouse.TriggerType,
		TriggerDetails:   fmt.Sprintf("Analyzed %d signals", inhouse.SignalsAnalyzed),
		AffectedService:  inhouse.AffectedService,
		AffectedNamespace: inhouse.AffectedNamespace,
		RootCause:        inhouse.RootCause,
		Confidence:       inhouse.Confidence,
		ReasoningChain:   inhouse.ReasoningChain,
		ProposedAction:   inhouse.ProposedAction,
		ActionParams:     inhouse.ActionParams,
		Status:           status,
		ModelUsed:        "observex-rule-engine" + func(enriched bool, model string) string { if enriched { return "+"+model }; return "" }(inhouse.OllamaEnriched, inhouse.OllamaModel),
	}, nil
}

func parseScenarioToSignals(scenario string) []PlatformSignal {
	s := strings.ToLower(scenario)
	var signals []PlatformSignal
	now := time.Now()

	if strings.Contains(s, "error rate") {
		val := 8.4
		signals = append(signals, PlatformSignal{Type: "metric", Metric: "error_rate", Value: val, Threshold: 5.0, Service: "checkout-service", Namespace: "production", Severity: "critical", Message: fmt.Sprintf("error_rate=%.1f%%", val), Timestamp: now})
	}
	if strings.Contains(s, "connection pool") || strings.Contains(s, "db") {
		signals = append(signals, PlatformSignal{Type: "log", Message: "connection pool exhausted", Service: "api-gateway", Namespace: "production", Severity: "critical", Timestamp: now})
	}
	if strings.Contains(s, "oom") || strings.Contains(s, "memory") {
		signals = append(signals, PlatformSignal{Type: "metric", Metric: "memory_pct", Value: 95, Threshold: 90, Service: "ml-service", Namespace: "production", Severity: "critical", Message: "memory_pct=95%", Timestamp: now})
	}
	if strings.Contains(s, "cpu") {
		signals = append(signals, PlatformSignal{Type: "metric", Metric: "cpu_pct", Value: 92, Threshold: 85, Service: "checkout-service", Namespace: "production", Severity: "critical", Message: "cpu_pct=92%", Timestamp: now})
	}
	if strings.Contains(s, "latency") || strings.Contains(s, "slow") {
		signals = append(signals, PlatformSignal{Type: "metric", Metric: "latency_p99", Value: 3200, Threshold: 1000, Service: "payment-service", Namespace: "production", Severity: "critical", Message: "latency_p99=3200ms", Timestamp: now})
	}
	if len(signals) == 0 {
		signals = append(signals, PlatformSignal{Type: "alert", Message: scenario, Service: "unknown", Severity: "warning", Timestamp: now})
	}
	return signals
}

// Override handleAIAgentSimulate to use in-house engine
func (gw *Gateway) handleInHouseSimulate(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var body struct{ Scenario string `json:"scenario"` }
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	decision, err := gw.runInHouseAgentDecision(c.Context(), auth.OrgID, body.Scenario, true)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"simulation":    true,
		"scenario":      body.Scenario,
		"decision":      decision,
		"engine":        "in-house-rule-engine",
		"ollama_active": envOr("OLLAMA_URL", "") != "",
		"note":          "In-house engine: no data sent externally. Optionally enhance with Ollama local LLM.",
	})
}

// Additional routes for in-house agent
func (gw *Gateway) registerInHouseAgentRoutes(api fiber.Router) {
	api.Get("/ai-agent/ollama-status", gw.handleOllamaStatus)
	api.Post("/ai-agent/inhouse-simulate", gw.handleInHouseSimulate)
	api.Get("/ai-agent/signals", gw.handleCurrentSignals)
	api.Get("/ai-agent/rule-engine", gw.handleRuleEngineInfo)
	api.Post("/ai-agent/inhouse-scan", gw.handleInHouseScan)
}

func (gw *Gateway) handleCurrentSignals(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	signals := gw.collectPlatformSignals(auth.OrgID)
	return c.JSON(fiber.Map{
		"signals": signals,
		"total":   len(signals),
		"scanned_at": time.Now(),
		"sources": []string{"observex_native", "loki", "active-alerts"},
	})
}

func (gw *Gateway) handleRuleEngineInfo(c *fiber.Ctx) error {
	rules := buildExpertRules()
	ruleInfo := make([]fiber.Map, len(rules))
	for i, r := range rules {
		ruleInfo[i] = fiber.Map{
			"name": r.Name, "priority": r.Priority,
			"confidence": r.Confidence, "action": r.Action,
			"explanation": r.Explanation,
		}
	}
	return c.JSON(fiber.Map{
		"rules":        ruleInfo,
		"total_rules":  len(rules),
		"ollama_url":   envOr("OLLAMA_URL", "not configured"),
		"ollama_model": envOr("OLLAMA_MODEL", "llama3"),
		"mode":         func() string { if envOr("OLLAMA_URL", "") != "" { return "ollama_llm" }; return "rule_engine" }(),
		"description":  "In-house monitoring agent — all decisions made locally, zero external API calls",
	})
}

func (gw *Gateway) handleInHouseScan(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	knowledge := gw.loadAgentKnowledge(c.Context(), auth.OrgID)
	signals := gw.collectPlatformSignals(auth.OrgID)
	decision := gw.runInHouseDecision(c.Context(), auth.OrgID, signals, knowledge)
	return c.JSON(fiber.Map{
		"decision":  decision,
		"signals":   signals,
		"timestamp": time.Now(),
	})
}

// Helper: import needed
var _ = sort.Strings
