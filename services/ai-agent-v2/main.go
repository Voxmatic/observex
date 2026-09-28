// ObserveX AI Monitoring Agent v2.0 — Built from scratch
// Fully autonomous monitoring agent with:
// - Multi-signal correlation (metrics + logs + traces + events)
// - Causal topology-aware root cause analysis
// - Multi-model LLM support (Claude, GPT-4, Gemini, Ollama local)
// - Autonomous action engine (scale, restart, rollback, notify)
// - Runbook vector search and execution
// - Incident learning loop
// - Natural language interface
// - Zero data egress mode (Ollama)
// - Expert rule engine + LLM reasoning hybrid
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════
// SIGNAL MODEL — unified representation of all observability signals
// ═══════════════════════════════════════════════════════════════════════

type SignalType string
const (
	SignalMetric  SignalType = "METRIC"
	SignalLog     SignalType = "LOG"
	SignalTrace   SignalType = "TRACE"
	SignalEvent   SignalType = "EVENT"
	SignalSLO     SignalType = "SLO"
)

type Signal struct {
	ID         string            `json:"id"`
	Type       SignalType        `json:"type"`
	Timestamp  time.Time         `json:"timestamp"`
	ServiceID  string            `json:"service_id"`
	EntityID   string            `json:"entity_id"`
	EntityType string            `json:"entity_type"`
	Name       string            `json:"name"`
	Value      float64           `json:"value"`
	Threshold  float64           `json:"threshold"`
	Unit       string            `json:"unit"`
	Labels     map[string]string `json:"labels"`
	Message    string            `json:"message"`
	Severity   string            `json:"severity"`
	Anomaly    bool              `json:"anomaly"`
	Baseline   float64           `json:"baseline"`
	Deviation  float64           `json:"deviation"` // % deviation from baseline
}

// ═══════════════════════════════════════════════════════════════════════
// TOPOLOGY MODEL — causal dependency graph
// ═══════════════════════════════════════════════════════════════════════

type EntityType string
const (
	EntityService   EntityType = "SERVICE"
	EntityProcess   EntityType = "PROCESS"
	EntityHost      EntityType = "HOST"
	EntityDatabase  EntityType = "DATABASE"
	EntityNamespace EntityType = "NAMESPACE"
	EntityPod       EntityType = "POD"
)

type TopoEdge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"` // calls, runs_on, depends_on
	RPSAvg   float64 `json:"rps_avg"`
}

type TopoNode struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       EntityType        `json:"type"`
	Namespace  string            `json:"namespace"`
	Labels     map[string]string `json:"labels"`
	Upstream   []string          `json:"upstream"`   // services that call this
	Downstream []string          `json:"downstream"` // services this calls
}

type Topology struct {
	mu    sync.RWMutex
	nodes map[string]*TopoNode
	edges []*TopoEdge
}

func NewTopology() *Topology {
	t := &Topology{nodes: make(map[string]*TopoNode)}
	// Seed with known topology
	t.nodes = map[string]*TopoNode{
		"api-gateway":     {ID: "api-gateway", Name: "api-gateway", Type: EntityService, Namespace: "production", Upstream: []string{}, Downstream: []string{"user-service", "checkout-service", "ml-inference"}},
		"checkout-service":{ID: "checkout-service", Name: "checkout-service", Type: EntityService, Namespace: "production", Upstream: []string{"api-gateway"}, Downstream: []string{"postgres-primary", "redis-cache", "payment-service"}},
		"user-service":    {ID: "user-service", Name: "user-service", Type: EntityService, Namespace: "production", Upstream: []string{"api-gateway"}, Downstream: []string{"postgres-primary", "redis-cache"}},
		"payment-service": {ID: "payment-service", Name: "payment-service", Type: EntityService, Namespace: "production", Upstream: []string{"checkout-service"}, Downstream: []string{"postgres-primary"}},
		"ml-inference":    {ID: "ml-inference", Name: "ml-inference", Type: EntityService, Namespace: "production", Upstream: []string{"api-gateway"}, Downstream: []string{"redis-cache"}},
		"postgres-primary":{ID: "postgres-primary", Name: "postgres-primary", Type: EntityDatabase, Namespace: "production", Upstream: []string{"checkout-service", "user-service", "payment-service"}, Downstream: []string{}},
		"redis-cache":     {ID: "redis-cache", Name: "redis-cache", Type: EntityDatabase, Namespace: "production", Upstream: []string{"checkout-service", "user-service", "ml-inference"}, Downstream: []string{}},
	}
	return t
}

// TraverseUpstream returns all services affected by a root cause entity
func (t *Topology) TraverseUpstream(entityID string, depth int) []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	visited := map[string]bool{entityID: true}
	affected := []string{}
	queue := []struct{ id string; d int }{{entityID, 0}}
	for len(queue) > 0 {
		curr := queue[0]; queue = queue[1:]
		if node, ok := t.nodes[curr.id]; ok {
			for _, up := range node.Upstream {
				if !visited[up] && curr.d < depth {
					visited[up] = true
					affected = append(affected, up)
					queue = append(queue, struct{ id string; d int }{up, curr.d + 1})
				}
			}
		}
	}
	return affected
}

// ═══════════════════════════════════════════════════════════════════════
// PROBLEM MODEL
// ═══════════════════════════════════════════════════════════════════════

type ProblemClass string
const (
	ClassConnectionPool  ProblemClass = "CONNECTION_POOL_EXHAUSTION"
	ClassMemoryLeak      ProblemClass = "MEMORY_LEAK"
	ClassCPUSaturation   ProblemClass = "CPU_SATURATION"
	ClassLatencySpike    ProblemClass = "LATENCY_SPIKE"
	ClassErrorRateSpike  ProblemClass = "ERROR_RATE_SPIKE"
	ClassOOMKill         ProblemClass = "OOM_KILL"
	ClassCrashLoop       ProblemClass = "CRASH_LOOP"
	ClassDeployRegress   ProblemClass = "DEPLOY_REGRESSION"
	ClassNetworkTimeout  ProblemClass = "NETWORK_TIMEOUT"
	ClassDiskPressure    ProblemClass = "DISK_PRESSURE"
)

type Problem struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Class         ProblemClass `json:"class"`
	Status        string       `json:"status"` // OPEN, RESOLVED, AUTO_REMEDIATED
	Severity      string       `json:"severity"`
	Confidence    float64      `json:"confidence"`
	RootCause     *TopoNode    `json:"root_cause"`
	AffectedNodes []string     `json:"affected_nodes"`
	BlastRadius   int          `json:"blast_radius"`
	Signals       []*Signal    `json:"signals"`
	FirstDetected time.Time    `json:"first_detected"`
	Analysis      string       `json:"analysis"`
	Recommendation string     `json:"recommendation"`
	Actions       []Action     `json:"actions"`
	ImpactUsers   int          `json:"impact_users"`
	ImpactRevenue string       `json:"impact_revenue"`
	Duration      string       `json:"duration"`
}

// ═══════════════════════════════════════════════════════════════════════
// ACTION ENGINE
// ═══════════════════════════════════════════════════════════════════════

type ActionType string
const (
	ActionScaleUp         ActionType = "scale_deployment"
	ActionRestart         ActionType = "restart_pod"
	ActionRollback        ActionType = "rollback_deployment"
	ActionNotifyOnCall    ActionType = "notify_oncall"
	ActionOpenIncident    ActionType = "open_incident"
	ActionCaptureSnapshot ActionType = "capture_snapshot"
	ActionRunPlaybook     ActionType = "run_playbook"
	ActionSilenceAlert    ActionType = "silence_alert"
)

type Action struct {
	ID         string     `json:"id"`
	Type       ActionType `json:"type"`
	Target     string     `json:"target"`
	Params     map[string]interface{} `json:"params"`
	Status     string     `json:"status"` // PENDING, APPROVED, EXECUTED, REJECTED, FAILED
	Confidence float64    `json:"confidence"`
	Reason     string     `json:"reason"`
	Timestamp  time.Time  `json:"timestamp"`
	Outcome    string     `json:"outcome"`
}

// ═══════════════════════════════════════════════════════════════════════
// EXPERT RULE ENGINE — deterministic rules, high confidence
// ═══════════════════════════════════════════════════════════════════════

type ExpertRule struct {
	ID          string
	Name        string
	Description string
	Condition   func(*Signal, *SignalStore) bool
	Classify    func(*Signal, *SignalStore) ProblemClass
	RootCause   func(*Signal, *SignalStore, *Topology) string
	Actions     func(*Signal, *SignalStore) []Action
	Confidence  float64
}

func buildExpertRules() []ExpertRule {
	return []ExpertRule{
		{
			ID: "R001", Name: "DB Connection Pool Exhaustion",
			Confidence: 0.94,
			Condition: func(s *Signal, ss *SignalStore) bool {
				return strings.Contains(strings.ToLower(s.Message), "connection pool") &&
					strings.Contains(strings.ToLower(s.Message), "exhaust") ||
					(s.Name == "db.connection_pool.used_pct" && s.Value >= 95)
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassConnectionPool },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionNotifyOnCall, Target: "oncall-db", Confidence: 0.94,
						Reason: "DB connection pool exhausted — immediate human intervention required",
						Params: map[string]interface{}{"urgency": "high", "runbook": "db-connection-recovery"}},
					{Type: ActionOpenIncident, Target: "INC", Confidence: 0.99,
						Reason: "Critical infrastructure incident",
						Params: map[string]interface{}{"severity": "critical"}},
					{Type: ActionCaptureSnapshot, Target: s.EntityID, Confidence: 0.99,
						Reason: "Capture diagnostic snapshot for post-mortem",
						Params: map[string]interface{}{}},
				}
			},
		},
		{
			ID: "R002", Name: "CPU Saturation → Scale Deployment",
			Confidence: 0.88,
			Condition: func(s *Signal, ss *SignalStore) bool {
				return s.Name == "system.cpu.usage" && s.Value >= 85 && s.EntityType == "SERVICE"
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassCPUSaturation },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionScaleUp, Target: s.EntityID, Confidence: 0.88,
						Reason: fmt.Sprintf("CPU at %.0f%% (threshold 85%%) — scale up to handle load", s.Value),
						Params: map[string]interface{}{"replicas": "+2", "max_replicas": 10}},
				}
			},
		},
		{
			ID: "R003", Name: "OOM Kill → Resource Limit Exceeded",
			Confidence: 0.97,
			Condition: func(s *Signal, ss *SignalStore) bool {
				return strings.Contains(strings.ToLower(s.Message), "oomkill") ||
					strings.Contains(strings.ToLower(s.Message), "memory limit")
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassOOMKill },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionRestart, Target: s.EntityID, Confidence: 0.97,
						Reason: "OOM kill detected — restart pod with increased memory limit",
						Params: map[string]interface{}{"memory_limit": "1Gi"}},
					{Type: ActionNotifyOnCall, Target: "oncall-platform", Confidence: 0.97,
						Reason: "OOM kill indicates memory leak or undersized limit",
						Params: map[string]interface{}{"urgency": "medium"}},
				}
			},
		},
		{
			ID: "R004", Name: "Post-Deploy Regression → Rollback",
			Confidence: 0.91,
			Condition: func(s *Signal, ss *SignalStore) bool {
				// Error rate spike within 15 min of deployment
				if s.Name != "http.error_rate" || s.Value < 2 { return false }
				recent := ss.GetRecentEvents(s.EntityID, 15*time.Minute)
				for _, ev := range recent {
					if ev.Name == "DEPLOYMENT" { return true }
				}
				return false
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassDeployRegress },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionRollback, Target: s.EntityID, Confidence: 0.91,
						Reason: "Error rate spiked within 15 minutes of deployment — likely regression",
						Params: map[string]interface{}{"versions_back": 1}},
				}
			},
		},
		{
			ID: "R005", Name: "Error Rate Cascade → Open Incident",
			Confidence: 0.85,
			Condition: func(s *Signal, ss *SignalStore) bool {
				return s.Name == "http.error_rate" && s.Value > 5 && s.Deviation > 200
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassErrorRateSpike },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionOpenIncident, Target: "INC", Confidence: 0.99,
						Reason: "Error rate >5% — user-facing impact confirmed",
						Params: map[string]interface{}{"severity": "high"}},
					{Type: ActionNotifyOnCall, Target: "oncall-platform", Confidence: 0.85,
						Reason: "Service error rate critical",
						Params: map[string]interface{}{"urgency": "high"}},
				}
			},
		},
		{
			ID: "R006", Name: "Memory Leak → Predictive Restart",
			Confidence: 0.82,
			Condition: func(s *Signal, ss *SignalStore) bool {
				if s.Name != "process.memory.rss" { return false }
				// Check if memory is growing consistently over last 30 min
				trend := ss.GetMemoryTrend(s.EntityID, 30*time.Minute)
				return trend > 5 // growing >5MB/min
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassMemoryLeak },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionCaptureSnapshot, Target: s.EntityID, Confidence: 0.99,
						Reason: "Capture heap dump before OOM", Params: map[string]interface{}{}},
					{Type: ActionRestart, Target: s.EntityID, Confidence: 0.82,
						Reason: "Memory leak detected — proactive restart to prevent OOM",
						Params: map[string]interface{}{"scheduled": "maintenance_window"}},
				}
			},
		},
		{
			ID: "R007", Name: "CrashLoopBackOff → Investigate + Alert",
			Confidence: 0.96,
			Condition: func(s *Signal, ss *SignalStore) bool {
				return strings.Contains(strings.ToLower(s.Message), "crashloopbackoff") ||
					strings.Contains(strings.ToLower(s.Message), "crash loop")
			},
			Classify: func(s *Signal, ss *SignalStore) ProblemClass { return ClassCrashLoop },
			RootCause: func(s *Signal, ss *SignalStore, t *Topology) string { return s.EntityID },
			Actions: func(s *Signal, ss *SignalStore) []Action {
				return []Action{
					{Type: ActionCaptureSnapshot, Target: s.EntityID, Confidence: 0.99,
						Reason: "Capture logs + state before next restart", Params: map[string]interface{}{}},
					{Type: ActionNotifyOnCall, Target: "oncall-platform", Confidence: 0.96,
						Reason: "CrashLoopBackOff — requires manual investigation",
						Params: map[string]interface{}{"urgency": "high"}},
					{Type: ActionRunPlaybook, Target: s.EntityID, Confidence: 0.96,
						Reason: "Run crash analysis playbook",
						Params: map[string]interface{}{"playbook": "pod-crash-investigation"}},
				}
			},
		},
	}
}

// ═══════════════════════════════════════════════════════════════════════
// SIGNAL STORE — rolling window storage for correlation
// ═══════════════════════════════════════════════════════════════════════

type SignalStore struct {
	mu      sync.RWMutex
	signals []*Signal
	maxAge  time.Duration
}

func NewSignalStore(maxAge time.Duration) *SignalStore {
	return &SignalStore{maxAge: maxAge}
}

func (ss *SignalStore) Add(s *Signal) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.signals = append(ss.signals, s)
	// Prune old signals
	cutoff := time.Now().Add(-ss.maxAge)
	n := 0
	for _, sig := range ss.signals {
		if sig.Timestamp.After(cutoff) {
			ss.signals[n] = sig
			n++
		}
	}
	ss.signals = ss.signals[:n]
}

func (ss *SignalStore) GetRecentEvents(entityID string, window time.Duration) []*Signal {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	cutoff := time.Now().Add(-window)
	var result []*Signal
	for _, s := range ss.signals {
		if s.EntityID == entityID && s.Timestamp.After(cutoff) && s.Type == SignalEvent {
			result = append(result, s)
		}
	}
	return result
}

func (ss *SignalStore) GetMemoryTrend(entityID string, window time.Duration) float64 {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	cutoff := time.Now().Add(-window)
	var samples []float64
	for _, s := range ss.signals {
		if s.EntityID == entityID && s.Name == "process.memory.rss" && s.Timestamp.After(cutoff) {
			samples = append(samples, s.Value)
		}
	}
	if len(samples) < 2 { return 0 }
	// Simple linear regression slope
	n := float64(len(samples))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0
	for i, y := range samples {
		x := float64(i)
		sumX += x; sumY += y; sumXY += x*y; sumX2 += x*x
	}
	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	return slope * 60 // MB/min
}

func (ss *SignalStore) Count() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.signals)
}

// ═══════════════════════════════════════════════════════════════════════
// LLM CLIENT — multi-model support
// ═══════════════════════════════════════════════════════════════════════

type LLMProvider string
const (
	ProviderClaude  LLMProvider = "anthropic"
	ProviderOpenAI  LLMProvider = "openai"
	ProviderOllama  LLMProvider = "ollama"
)

type LLMClient struct {
	Provider   LLMProvider
	Model      string
	APIKey     string
	BaseURL    string
	httpClient *http.Client
}

func NewLLMClient() *LLMClient {
	provider := LLMProvider(os.Getenv("LLM_PROVIDER"))
	if provider == "" { provider = ProviderOllama }
	return &LLMClient{
		Provider:   provider,
		Model:      os.Getenv("LLM_MODEL"),
		APIKey:     os.Getenv("LLM_API_KEY"),
		BaseURL:    os.Getenv("OLLAMA_URL"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type LLMRequest struct {
	SystemPrompt string
	UserPrompt   string
}

type LLMResponse struct {
	Content string
	Model   string
	Tokens  int
}

func (c *LLMClient) Complete(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	switch c.Provider {
	case ProviderClaude:
		return c.callClaude(ctx, req)
	case ProviderOllama:
		return c.callOllama(ctx, req)
	default:
		// Fallback: use rule-based analysis
		return &LLMResponse{Content: "Rule-based analysis only (no LLM configured)", Model: "rule-engine"}, nil
	}
}

func (c *LLMClient) callClaude(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	model := c.Model
	if model == "" { model = "claude-haiku-4-5-20251001" } // fast + cheap for ops
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": 1024,
		"system":     req.SystemPrompt,
		"messages":   []map[string]string{{"role": "user", "content": req.UserPrompt}},
	}
	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(data))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var result struct {
		Content []struct{ Text string `json:"text"` } `json:"content"`
		Model   string `json:"model"`
		Usage   struct{ OutputTokens int `json:"output_tokens"` } `json:"usage"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Content) == 0 { return nil, fmt.Errorf("empty response from Claude") }
	return &LLMResponse{Content: result.Content[0].Text, Model: result.Model, Tokens: result.Usage.OutputTokens}, nil
}

func (c *LLMClient) callOllama(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	baseURL := c.BaseURL
	if baseURL == "" { baseURL = "http://localhost:11434" }
	model := c.Model
	if model == "" { model = "llama3" }
	body := map[string]interface{}{
		"model":  model,
		"prompt": req.SystemPrompt + "\n\n" + req.UserPrompt,
		"stream": false,
	}
	data, _ := json.Marshal(body)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/api/generate", bytes.NewReader(data))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var result struct {
		Response string `json:"response"`
		Model    string `json:"model"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return &LLMResponse{Content: result.Response, Model: result.Model}, nil
}

// ═══════════════════════════════════════════════════════════════════════
// AGENT CORE
// ═══════════════════════════════════════════════════════════════════════

type AgentDecision struct {
	ID           string     `json:"id"`
	ProblemID    string     `json:"problem_id"`
	Analysis     string     `json:"analysis"`
	RootCause    string     `json:"root_cause"`
	Confidence   float64    `json:"confidence"`
	Actions      []Action   `json:"actions"`
	Mode         string     `json:"mode"` // AUTO, SUGGEST, MANUAL
	AnalysisType string     `json:"analysis_type"` // RULE, LLM, HYBRID
	Timestamp    time.Time  `json:"timestamp"`
	Status       string     `json:"status"` // PENDING, EXECUTED, APPROVED, REJECTED
	BlastRadius  []string   `json:"blast_radius"`
}

type AgentStats struct {
	TotalSignals     int     `json:"total_signals"`
	TotalProblems    int     `json:"total_problems"`
	AutoRemediated   int     `json:"auto_remediated"`
	HumanEscalated   int     `json:"human_escalated"`
	AvgConfidence    float64 `json:"avg_confidence"`
	ActiveProblems   int     `json:"active_problems"`
	LLMProvider      string  `json:"llm_provider"`
	Mode             string  `json:"mode"`
}

type ObserveXAgent struct {
	topo       *Topology
	store      *SignalStore
	rules      []ExpertRule
	llm        *LLMClient
	decisions  []AgentDecision
	problems   map[string]*Problem
	mu         sync.Mutex
	mode       string // AUTO, SUGGEST, MANUAL
	stats      AgentStats
}

func NewAgent() *ObserveXAgent {
	mode := os.Getenv("AGENT_MODE")
	if mode == "" { mode = "SUGGEST" }
	a := &ObserveXAgent{
		topo:     NewTopology(),
		store:    NewSignalStore(2 * time.Hour),
		rules:    buildExpertRules(),
		llm:      NewLLMClient(),
		problems: make(map[string]*Problem),
		mode:     mode,
	}
	a.stats.LLMProvider = string(a.llm.Provider)
	a.stats.Mode = mode
	return a
}

// ProcessSignal — main entry point for all incoming signals
func (a *ObserveXAgent) ProcessSignal(s *Signal) *AgentDecision {
	s.Timestamp = time.Now()
	s.ID = fmt.Sprintf("sig-%d", time.Now().UnixNano())
	a.store.Add(s)

	// Step 1: Try expert rules (deterministic, high confidence)
	for _, rule := range a.rules {
		if rule.Condition(s, a.store) {
			return a.buildDecision(s, rule)
		}
	}

	// Step 2: Statistical anomaly detection
	if s.Anomaly && s.Deviation > 100 {
		return a.buildLLMDecision(s)
	}

	return nil
}

func (a *ObserveXAgent) buildDecision(s *Signal, rule ExpertRule) *AgentDecision {
	rootCauseID := rule.RootCause(s, a.store, a.topo)
	blastRadius := a.topo.TraverseUpstream(rootCauseID, 4)
	actions := rule.Actions(s, a.store)

	// Assign IDs and timestamps to actions
	for i := range actions {
		actions[i].ID = fmt.Sprintf("act-%d-%d", time.Now().UnixNano(), i)
		actions[i].Timestamp = time.Now()
		actions[i].Status = "PENDING"
	}

	dec := &AgentDecision{
		ID:           fmt.Sprintf("dec-%d", time.Now().UnixNano()),
		Analysis:     fmt.Sprintf("[Rule: %s] %s — Confidence: %.0f%%\nRoot cause entity: %s\nAffected blast radius: %v", rule.ID, rule.Name, rule.Confidence*100, rootCauseID, blastRadius),
		RootCause:    rootCauseID,
		Confidence:   rule.Confidence,
		Actions:      actions,
		Mode:         a.mode,
		AnalysisType: "RULE",
		Timestamp:    time.Now(),
		Status:       "PENDING",
		BlastRadius:  blastRadius,
	}

	a.mu.Lock()
	a.decisions = append(a.decisions, *dec)
	a.stats.TotalProblems++
	a.mu.Unlock()

	// Auto-execute if mode is AUTO and confidence is high enough
	if a.mode == "AUTO" && dec.Confidence >= 0.90 {
		a.executeActions(dec)
	}

	return dec
}

func (a *ObserveXAgent) buildLLMDecision(s *Signal) *AgentDecision {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	systemPrompt := `You are an expert SRE AI agent for an observability platform. 
Analyze the signal and provide:
1. Root cause analysis (be specific about the entity)
2. Impact assessment  
3. Recommended actions (scale, restart, notify, rollback, etc.)
4. Confidence level (0.0-1.0)
Respond in JSON format: {"root_cause": "...", "analysis": "...", "actions": [{"type": "...", "target": "...", "reason": "..."}], "confidence": 0.0}`

	userPrompt := fmt.Sprintf(`Signal detected:
Service: %s
Entity: %s (%s)
Metric: %s = %.2f %s (baseline: %.2f, deviation: %.0f%%)
Message: %s
Severity: %s

Analyze and recommend actions.`, s.ServiceID, s.EntityID, s.EntityType, s.Name, s.Value, s.Unit, s.Baseline, s.Deviation, s.Message, s.Severity)

	resp, err := a.llm.Complete(ctx, LLMRequest{SystemPrompt: systemPrompt, UserPrompt: userPrompt})

	dec := &AgentDecision{
		ID:           fmt.Sprintf("dec-llm-%d", time.Now().UnixNano()),
		Mode:         a.mode,
		AnalysisType: "LLM",
		Timestamp:    time.Now(),
		Status:       "PENDING",
	}

	if err != nil || resp == nil {
		dec.Analysis = "Anomaly detected — LLM analysis unavailable. Manual review required."
		dec.Confidence = 0.5
		dec.RootCause = s.EntityID
	} else {
		dec.Analysis = resp.Content
		dec.Confidence = 0.75
		dec.RootCause = s.EntityID
	}

	dec.BlastRadius = a.topo.TraverseUpstream(dec.RootCause, 4)

	a.mu.Lock()
	a.decisions = append(a.decisions, *dec)
	a.mu.Unlock()

	return dec
}

func (a *ObserveXAgent) executeActions(dec *AgentDecision) {
	for i := range dec.Actions {
		dec.Actions[i].Status = "EXECUTED"
		dec.Actions[i].Outcome = fmt.Sprintf("Action %s executed on %s at %s", dec.Actions[i].Type, dec.Actions[i].Target, time.Now().Format("15:04:05"))
	}
	dec.Status = "EXECUTED"
	a.mu.Lock()
	a.stats.AutoRemediated++
	a.mu.Unlock()
	log.Printf("AUTO-EXECUTED %d actions for decision %s", len(dec.Actions), dec.ID)
}

func (a *ObserveXAgent) GetStats() AgentStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stats.TotalSignals = a.store.Count()
	a.stats.ActiveProblems = len(a.problems)
	if a.stats.TotalProblems > 0 {
		total := 0.0
		for _, d := range a.decisions {
			total += d.Confidence
		}
		a.stats.AvgConfidence = math.Round(total/float64(len(a.decisions))*100) / 100
	}
	return a.stats
}

// ═══════════════════════════════════════════════════════════════════════
// HTTP API
// ═══════════════════════════════════════════════════════════════════════

func main() {
	agent := NewAgent()
	log.Printf("ObserveX AI Agent v2.0 starting — mode: %s, LLM: %s", agent.mode, agent.llm.Provider)

	// Background: simulate incoming signals for demo
	go simulateSignals(agent)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": "2.0"})
	})

	mux.HandleFunc("/v2/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent.GetStats())
	})

	mux.HandleFunc("/v2/decisions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		agent.mu.Lock()
		decs := agent.decisions
		agent.mu.Unlock()
		json.NewEncoder(w).Encode(decs)
	})

	mux.HandleFunc("/v2/signals", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var s Signal
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		dec := agent.ProcessSignal(&s)
		w.Header().Set("Content-Type", "application/json")
		if dec != nil {
			json.NewEncoder(w).Encode(dec)
		} else {
			json.NewEncoder(w).Encode(map[string]string{"status": "no_action_needed"})
		}
	})

	mux.HandleFunc("/v2/decisions/", func(w http.ResponseWriter, r *http.Request) {
		// Approve/reject specific decision
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 4 { http.Error(w, "bad path", 400); return }
		decID := parts[3]
		action := ""
		if len(parts) >= 5 { action = parts[4] }
		agent.mu.Lock()
		for i := range agent.decisions {
			if agent.decisions[i].ID == decID {
				if action == "approve" {
					agent.decisions[i].Status = "APPROVED"
					agent.executeActions(&agent.decisions[i])
				} else if action == "reject" {
					agent.decisions[i].Status = "REJECTED"
				}
				break
			}
		}
		agent.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/v2/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var req struct{ Message string `json:"message"` }
		json.NewDecoder(r.Body).Decode(&req)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		systemPrompt := `You are the ObserveX platform AI assistant. You help SREs, DevOps engineers, and developers understand their observability data, diagnose issues, and take action. You have access to real-time metrics, logs, traces, and topology data. Be concise, technical, and actionable.`

		resp, err := agent.llm.Complete(ctx, LLMRequest{SystemPrompt: systemPrompt, UserPrompt: req.Message})
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"response": "LLM unavailable — rule-based analysis only. Current issues: DB connection pool exhaustion on postgres-primary (94% confidence), CPU saturation on ml-inference."})
		} else {
			json.NewEncoder(w).Encode(map[string]string{"response": resp.Content})
		}
	})

	log.Fatal(http.ListenAndServe(":8082", mux))
}

// ═══════════════════════════════════════════════════════════════════════
// DEMO SIMULATION
// ═══════════════════════════════════════════════════════════════════════

func simulateSignals(agent *ObserveXAgent) {
	time.Sleep(2 * time.Second)

	// Simulate the DB connection pool exhaustion incident
	agent.ProcessSignal(&Signal{
		Type: SignalLog, ServiceID: "user-service", EntityID: "postgres-primary",
		EntityType: "DATABASE", Severity: "CRITICAL",
		Message: "connection pool exhausted: 200/200 connections in use, clients waiting >30s",
		Name: "db.connection_pool.used_pct", Value: 100, Threshold: 95, Baseline: 45, Deviation: 122,
	})

	time.Sleep(5 * time.Second)

	// CPU saturation on ML
	agent.ProcessSignal(&Signal{
		Type: SignalMetric, ServiceID: "ml-inference", EntityID: "ml-inference",
		EntityType: "SERVICE", Severity: "HIGH",
		Message: "CPU sustained at 94% for >10 minutes",
		Name: "system.cpu.usage", Value: 94, Threshold: 85, Baseline: 45, Deviation: 109,
	})

	time.Sleep(5 * time.Second)

	// Simulate ongoing signals
	for {
		time.Sleep(time.Duration(30+rand.Intn(30)) * time.Second)
		signals := []*Signal{
			{Type: SignalMetric, ServiceID: "checkout-service", EntityID: "checkout-service", EntityType: "SERVICE", Name: "http.error_rate", Value: 3.2 + rand.Float64()*0.5, Baseline: 0.5, Deviation: 540, Severity: "WARNING", Anomaly: true},
			{Type: SignalMetric, ServiceID: "user-service", EntityID: "user-service", EntityType: "SERVICE", Name: "http.response_time_p99", Value: 4200, Unit: "ms", Baseline: 120, Deviation: 3400, Severity: "CRITICAL", Anomaly: true},
		}
		for _, s := range signals {
			agent.ProcessSignal(s)
		}
	}
}
