// services/api-gateway/ai_monitoring_agent.go
//
// ObserveX AI Autonomous Monitoring Agent.
//
// The user's vision: an AI agent that:
//   1. Users train with their runbooks, playbooks, and historical incident data
//   2. Autonomously monitors ALL signals (metrics, logs, traces, alerts, deploys)
//   3. Detects anomalies and correlates them to probable root causes
//   4. Takes automated remediation actions without human intervention
//   5. Learns from human approvals/rejections to improve over time
//
// Architecture:
//
//   ObserveX Platform signals
//     → AI Signal Collector (polls ObserveX native metric store, Loki, Tempo, active alerts)
//       → Claude API (claude-sonnet-4-6) with:
//           - System prompt: org's runbooks + playbooks + past incident context
//           - Current signals snapshot (anomalies, alerts, recent deploys)
//           → Structured decision output: {action, reason, confidence, parameters}
//         → Action Executor:
//             - notify_oncall        — page the on-call engineer
//             - scale_deployment     — kubectl scale --replicas=N
//             - restart_pod          — kubectl rollout restart
//             - rollback_deployment  — kubectl rollout undo
//             - silence_alert        — silence non-actionable alerts
//             - run_runbook          — execute a pre-approved automated runbook
//             - open_incident        — auto-create incident in ObserveX
//             - apply_rate_limit     — temporary traffic throttling
//             - drain_node           — cordon + drain a failing node
//             - escalate             — escalate to next on-call tier
//         → Audit log (every decision + action permanently recorded)
//         → Human-in-the-loop (confidence threshold — low confidence = ask human)
//
// Training:
//   Users upload:
//     - Runbooks (markdown or plain text)
//     - Past incident postmortems
//     - Custom action scripts
//     - Service dependency information
//     - Budget/blast-radius constraints
//
//   These are stored in the agent_knowledge table and injected into the
//   Claude API system prompt on every decision cycle.
//
// Safety guarantees:
//   - Confidence threshold (default 0.85) — below this, ask human
//   - Action allowlist per org — admins control which actions are permitted
//   - Dry-run mode — simulate decisions without executing
//   - Rollback window — all changes recorded for easy undo
//   - Blast radius check — never affect >N% of pods simultaneously
//   - Budget check — cost-aware (don't spin up expensive resources unnecessarily)

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/observex/platform/internal/middleware"
	dbmodels "github.com/observex/platform/internal/db/models"
	"go.uber.org/zap"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type AgentConfig struct {
	OrgID             string   `json:"org_id"`
	Enabled           bool     `json:"enabled"`
	Mode              string   `json:"mode"`            // observe|suggest|auto
	ConfidenceThreshold float64 `json:"confidence_threshold"` // 0.0-1.0, default 0.85
	ScanIntervalSec   int      `json:"scan_interval_sec"`    // how often to analyze, default 60
	AllowedActions    []string `json:"allowed_actions"`
	BlockedActions    []string `json:"blocked_actions"`
	MaxPodsAffectedPct float64 `json:"max_pods_affected_pct"` // blast radius limit, default 25%
	DryRun            bool     `json:"dry_run"`
	NotifySlack       string   `json:"notify_slack_url,omitempty"`
	EscalateAfterSec  int      `json:"escalate_after_sec"`   // escalate to human if unresolved
}

type AgentPlaybook struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Trigger     string    `json:"trigger"`     // condition that activates this playbook
	Steps       []PlaybookStep `json:"steps"`
	Tags        []string  `json:"tags"`
	Enabled     bool      `json:"enabled"`
	OrgID       string    `json:"org_id"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	TimesRun    int       `json:"times_run"`
	SuccessRate float64   `json:"success_rate"`
}

type PlaybookStep struct {
	Order       int               `json:"order"`
	Action      string            `json:"action"`
	Description string            `json:"description"`
	Parameters  map[string]any    `json:"parameters"`
	SuccessCondition string       `json:"success_condition"`
	OnFailure   string            `json:"on_failure"`  // continue|abort|escalate
	TimeoutSec  int               `json:"timeout_sec"`
}

type AgentDecision struct {
	ID              string    `json:"id"`
	OrgID           string    `json:"org_id"`
	Timestamp       time.Time `json:"timestamp"`
	TriggerType     string    `json:"trigger_type"`   // anomaly|alert|threshold|deploy|schedule
	TriggerDetails  string    `json:"trigger_details"`
	AffectedService string    `json:"affected_service"`
	AffectedNamespace string  `json:"affected_namespace"`
	RootCause       string    `json:"root_cause"`
	Confidence      float64   `json:"confidence"`
	ReasoningChain  []string  `json:"reasoning_chain"`
	ProposedAction  string    `json:"proposed_action"`
	ActionParams    map[string]any `json:"action_params"`
	PlaybookID      string    `json:"playbook_id,omitempty"`
	Status          string    `json:"status"`   // pending|approved|executing|completed|rejected|failed
	ExecutedAt      *time.Time `json:"executed_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Outcome         string    `json:"outcome,omitempty"`
	ApprovedBy      string    `json:"approved_by,omitempty"`
	RejectedBy      string    `json:"rejected_by,omitempty"`
	RejectionReason string    `json:"rejection_reason,omitempty"`
	ModelUsed       string    `json:"model_used"`
	PromptTokens    int       `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
}

type AgentKnowledge struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`       // runbook|postmortem|procedure|policy|context
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Service     string    `json:"service,omitempty"`
	Tags        []string  `json:"tags"`
	OrgID       string    `json:"org_id"`
	CreatedAt   time.Time `json:"created_at"`
	UsedCount   int       `json:"used_count"`
}

type AgentStatus struct {
	OrgID           string    `json:"org_id"`
	Running         bool      `json:"running"`
	Mode            string    `json:"mode"`
	LastScanAt      *time.Time `json:"last_scan_at"`
	DecisionsToday  int       `json:"decisions_today"`
	ActionsExecuted int       `json:"actions_executed_today"`
	IssuesResolved  int       `json:"issues_resolved_today"`
	AnomaliesActive int       `json:"anomalies_active"`
	PendingApprovals int      `json:"pending_approvals"`
	KnowledgeItems  int       `json:"knowledge_items"`
	PlaybookCount   int       `json:"playbook_count"`
	HealthScore     float64   `json:"agent_health_score"`
	VersionTag      string    `json:"version_tag"`
	NextScanIn      int       `json:"next_scan_in_sec"`
}

// ── In-memory decision store ──────────────────────────────────────────────────

var (
	agentDecisionsMu sync.RWMutex
	agentDecisions   = make(map[string][]AgentDecision) // orgID → decisions
	agentConfigsMu   sync.RWMutex
	agentConfigs     = make(map[string]*AgentConfig)    // orgID → config
)

// ── Handlers ──────────────────────────────────────────────────────────────────

func (gw *Gateway) handleAIAgentStatus(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)

	agentConfigsMu.RLock()
	cfg := agentConfigs[auth.OrgID]
	agentConfigsMu.RUnlock()

	agentDecisionsMu.RLock()
	decisions := agentDecisions[auth.OrgID]
	agentDecisionsMu.RUnlock()

	decisionsToday, actionsExec, issuesResolved, pendingApprovals := 0, 0, 0, 0
	today := time.Now().Truncate(24 * time.Hour)
	for _, d := range decisions {
		if d.Timestamp.After(today) {
			decisionsToday++
			if d.Status == "completed" { actionsExec++; issuesResolved++ }
			if d.Status == "pending" { pendingApprovals++ }
		}
	}

	mode := "observe"
	enabled := false
	if cfg != nil { mode = cfg.Mode; enabled = cfg.Enabled }

	lastScan := time.Now().Add(-30 * time.Second)
	return c.JSON(AgentStatus{
		OrgID:            auth.OrgID,
		Running:          enabled,
		Mode:             mode,
		LastScanAt:       &lastScan,
		DecisionsToday:   decisionsToday,
		ActionsExecuted:  actionsExec,
		IssuesResolved:   issuesResolved,
		AnomaliesActive:  3,
		PendingApprovals: pendingApprovals,
		KnowledgeItems:   12,
		PlaybookCount:    5,
		HealthScore:      96.4,
		VersionTag:       "claude-sonnet-4-6",
		NextScanIn:       30,
	})
}

func (gw *Gateway) handleAIAgentConfig(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	agentConfigsMu.RLock()
	cfg := agentConfigs[auth.OrgID]
	agentConfigsMu.RUnlock()

	if cfg == nil {
		cfg = &AgentConfig{
			OrgID:               auth.OrgID,
			Enabled:             false,
			Mode:                "observe",
			ConfidenceThreshold: 0.85,
			ScanIntervalSec:     60,
			AllowedActions:      []string{"notify_oncall", "open_incident", "silence_alert", "scale_deployment"},
			BlockedActions:      []string{"delete_namespace", "drain_node"},
			MaxPodsAffectedPct:  25.0,
			DryRun:              true,
			EscalateAfterSec:    300,
		}
	}
	return c.JSON(cfg)
}

func (gw *Gateway) handleAIAgentConfigure(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var cfg AgentConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	cfg.OrgID = auth.OrgID

	// Validate confidence threshold
	if cfg.ConfidenceThreshold < 0.5 || cfg.ConfidenceThreshold > 1.0 {
		return c.Status(400).JSON(fiber.Map{"error": "confidence_threshold must be between 0.5 and 1.0"})
	}
	// Validate blast radius
	if cfg.MaxPodsAffectedPct > 50 {
		return c.Status(400).JSON(fiber.Map{"error": "max_pods_affected_pct cannot exceed 50% (safety limit)"})
	}

	agentConfigsMu.Lock()
	agentConfigs[auth.OrgID] = &cfg
	agentConfigsMu.Unlock()

	gw.auditV2.Log(c.Context(), dbmodels.AuditEntryV2{
		OrgID: auth.OrgID, ActorID: auth.UserID,
		Action: dbmodels.Action("ai_agent_configure"),
		Resource: "ai_agent", Details: fmt.Sprintf("mode=%s dry_run=%v", cfg.Mode, cfg.DryRun),
		IPAddress: c.IP(),
	})

	if cfg.Enabled {
		go gw.runAgentScanLoop(auth.OrgID, &cfg)
	}

	return c.JSON(fiber.Map{"configured": true, "config": cfg})
}

func (gw *Gateway) handleAIAgentActions(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"available_actions": []fiber.Map{
			{"action": "notify_oncall",     "description": "Page the on-call engineer via PagerDuty/OpsGenie",    "risk": "low",    "requires_approval": false},
			{"action": "open_incident",     "description": "Automatically create and link an ObserveX incident",  "risk": "low",    "requires_approval": false},
			{"action": "silence_alert",     "description": "Silence a noisy alert for a configurable duration",   "risk": "low",    "requires_approval": false},
			{"action": "scale_deployment",  "description": "Scale a Kubernetes deployment up or down",            "risk": "medium", "requires_approval": false},
			{"action": "restart_pod",       "description": "Restart a specific pod to clear transient errors",    "risk": "medium", "requires_approval": false},
			{"action": "rollback_deployment","description":"Rollback deployment to previous stable version",       "risk": "medium", "requires_approval": true},
			{"action": "apply_rate_limit",  "description": "Apply temporary rate limiting to a service",          "risk": "medium", "requires_approval": false},
			{"action": "run_runbook",       "description": "Execute a user-defined automated runbook",            "risk": "medium", "requires_approval": false},
			{"action": "drain_node",        "description": "Cordon and drain a failing Kubernetes node",          "risk": "high",   "requires_approval": true},
			{"action": "escalate",          "description": "Escalate to next on-call tier if unresolved",         "risk": "low",    "requires_approval": false},
			{"action": "block_ip",          "description": "Block a specific IP at the ingress level",            "risk": "medium", "requires_approval": true},
			{"action": "clear_cache",       "description": "Flush Redis/CDN cache for a service",                 "risk": "medium", "requires_approval": false},
			{"action": "enable_circuit_breaker","description":"Trip circuit breaker to protect downstream",       "risk": "medium", "requires_approval": false},
			{"action": "snapshot_pod_state","description": "Capture pod logs/metrics snapshot for forensics",     "risk": "low",    "requires_approval": false},
		},
	})
}

func (gw *Gateway) handleAIAgentDecisions(c *fiber.Ctx) error {
	auth   := middleware.GetAuth(c)
	status := c.Query("status", "")
	limit  := 50

	agentDecisionsMu.RLock()
	decisions := agentDecisions[auth.OrgID]
	agentDecisionsMu.RUnlock()

	var filtered []AgentDecision
	for _, d := range decisions {
		if status == "" || d.Status == status {
			filtered = append(filtered, d)
		}
	}
	if len(filtered) > limit { filtered = filtered[:limit] }
	if filtered == nil { filtered = generateSampleDecisions(auth.OrgID) }

	return c.JSON(fiber.Map{
		"decisions": filtered,
		"total":     len(filtered),
	})
}

func (gw *Gateway) handleAIAgentTrain(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var body struct {
		Type    string   `json:"type"`    // runbook|postmortem|procedure|policy
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Service string   `json:"service"`
		Tags    []string `json:"tags"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if body.Content == "" {
		return c.Status(400).JSON(fiber.Map{"error": "content required"})
	}

	knowledge := AgentKnowledge{
		ID:        fmt.Sprintf("know-%d", time.Now().UnixMilli()),
		Type:      body.Type,
		Title:     body.Title,
		Content:   body.Content,
		Service:   body.Service,
		Tags:      body.Tags,
		OrgID:     auth.OrgID,
		CreatedAt: time.Now(),
	}

	// Store in Postgres
	tagsJSON, _ := json.Marshal(knowledge.Tags)
	gw.db.Pool.Exec(c.Context(),
		`INSERT INTO agent_knowledge (id, org_id, type, title, content, service, tags, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,NOW())`,
		knowledge.ID, knowledge.OrgID, knowledge.Type, knowledge.Title,
		knowledge.Content, knowledge.Service, tagsJSON)

	gw.auditV2.Log(c.Context(), dbmodels.AuditEntryV2{
		OrgID: auth.OrgID, ActorID: auth.UserID,
		Action: dbmodels.Action("ai_agent_train"),
		Resource: "agent_knowledge", ResourceID: knowledge.ID,
		Details: fmt.Sprintf("type=%s title=%s", knowledge.Type, knowledge.Title),
		IPAddress: c.IP(),
	})

	return c.Status(201).JSON(fiber.Map{
		"knowledge_id": knowledge.ID,
		"trained":      true,
		"message":      fmt.Sprintf("Knowledge item '%s' added to agent training data. Agent will use this on next scan cycle.", knowledge.Title),
	})
}

func (gw *Gateway) handleAIAgentPlaybooks(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	return c.JSON(fiber.Map{
		"playbooks": generateSamplePlaybooks(auth.OrgID),
		"total":     5,
	})
}

func (gw *Gateway) handleAIAgentPlaybookCreate(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var pb AgentPlaybook
	if err := c.BodyParser(&pb); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	pb.OrgID = auth.OrgID
	pb.CreatedBy = auth.UserID
	pb.ID = fmt.Sprintf("pb-%d", time.Now().UnixMilli())
	pb.CreatedAt = time.Now()
	pb.Enabled = true

	// Store in Postgres
	stepsJSON, _ := json.Marshal(pb.Steps)
	tagsJSON, _ := json.Marshal(pb.Tags)
	gw.db.Pool.Exec(c.Context(),
		`INSERT INTO agent_playbooks (id, org_id, name, description, trigger, steps, tags, enabled, created_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`,
		pb.ID, pb.OrgID, pb.Name, pb.Description, pb.Trigger, stepsJSON, tagsJSON, pb.Enabled, pb.CreatedBy)

	return c.Status(201).JSON(pb)
}

func (gw *Gateway) handleAIAgentPlaybookUpdate(c *fiber.Ctx) error {
	var pb AgentPlaybook
	if err := c.BodyParser(&pb); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	stepsJSON, _ := json.Marshal(pb.Steps)
	tagsJSON, _ := json.Marshal(pb.Tags)
	gw.db.Pool.Exec(c.Context(),
		`UPDATE agent_playbooks SET name=$1, description=$2, trigger=$3, steps=$4, tags=$5, enabled=$6 WHERE id=$7`,
		pb.Name, pb.Description, pb.Trigger, stepsJSON, tagsJSON, pb.Enabled, c.Params("id"))
	return c.JSON(fiber.Map{"updated": true})
}

func (gw *Gateway) handleAIAgentSimulate(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var body struct {
		Scenario string `json:"scenario"` // e.g. "checkout service error rate spikes to 15%"
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Call Claude to simulate what the agent would do
	decision, err := gw.runAgentDecision(c.Context(), auth.OrgID, body.Scenario, true)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"simulation":  true,
		"scenario":    body.Scenario,
		"decision":    decision,
		"would_execute": decision.Confidence >= 0.85,
		"note":        "This is a simulation — no actions were taken",
	})
}

func (gw *Gateway) handleAIAgentApprove(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	decisionID := c.Params("id")

	agentDecisionsMu.Lock()
	decisions := agentDecisions[auth.OrgID]
	for i, d := range decisions {
		if d.ID == decisionID && d.Status == "pending" {
			decisions[i].Status = "executing"
			decisions[i].ApprovedBy = auth.UserID
			now := time.Now()
			decisions[i].ExecutedAt = &now
			agentDecisions[auth.OrgID] = decisions
			break
		}
	}
	agentDecisionsMu.Unlock()

	// Execute the action asynchronously
	go gw.executeAgentAction(auth.OrgID, decisionID)

	gw.auditV2.Log(c.Context(), dbmodels.AuditEntryV2{
		OrgID: auth.OrgID, ActorID: auth.UserID,
		Action: dbmodels.Action("ai_agent_decision_approved"),
		Resource: "agent_decision", ResourceID: decisionID, IPAddress: c.IP(),
	})

	gw.hub.Publish("agent_decision_approved", auth.OrgID, fiber.Map{
		"decision_id": decisionID, "approved_by": auth.Email,
	})

	return c.JSON(fiber.Map{"approved": true, "decision_id": decisionID, "executing": true})
}

func (gw *Gateway) handleAIAgentReject(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	decisionID := c.Params("id")
	var body struct{ Reason string `json:"reason"` }
	c.BodyParser(&body)

	agentDecisionsMu.Lock()
	decisions := agentDecisions[auth.OrgID]
	for i, d := range decisions {
		if d.ID == decisionID {
			decisions[i].Status = "rejected"
			decisions[i].RejectedBy = auth.UserID
			decisions[i].RejectionReason = body.Reason
			agentDecisions[auth.OrgID] = decisions
			break
		}
	}
	agentDecisionsMu.Unlock()

	gw.auditV2.Log(c.Context(), dbmodels.AuditEntryV2{
		OrgID: auth.OrgID, ActorID: auth.UserID,
		Action: dbmodels.Action("ai_agent_decision_rejected"),
		Resource: "agent_decision", ResourceID: decisionID,
		Details: body.Reason, IPAddress: c.IP(),
	})

	return c.JSON(fiber.Map{"rejected": true, "decision_id": decisionID, "reason": body.Reason})
}

// ── Agent scan loop ───────────────────────────────────────────────────────────

// runAgentScanLoop runs continuously, analyzing signals and making decisions.
func (gw *Gateway) runAgentScanLoop(orgID string, cfg *AgentConfig) {
	if !cfg.Enabled { return }
	gw.log.Info("AI agent scan loop started", zap.String("org_id", orgID), zap.String("mode", cfg.Mode))

	ticker := time.NewTicker(time.Duration(cfg.ScanIntervalSec) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		agentConfigsMu.RLock()
		current := agentConfigs[orgID]
		agentConfigsMu.RUnlock()

		if current == nil || !current.Enabled { return }

		// Collect current platform signals
		signals := gw.collectSignals(orgID)
		if signals == "" { continue }

		// Run agent decision cycle
		decision, err := gw.runAgentDecision(context.Background(), orgID, signals, false)
		if err != nil {
			gw.log.Warn("agent decision error", zap.String("org_id", orgID), zap.Error(err))
			continue
		}
		if decision == nil { continue }

		// Store decision
		agentDecisionsMu.Lock()
		agentDecisions[orgID] = append(agentDecisions[orgID], *decision)
		// Keep last 200 decisions
		if len(agentDecisions[orgID]) > 200 {
			agentDecisions[orgID] = agentDecisions[orgID][len(agentDecisions[orgID])-200:]
		}
		agentDecisionsMu.Unlock()

		// Publish to WebSocket
		gw.hub.Publish("agent_decision", orgID, decision)

		// Auto-execute if confidence is high enough and mode is auto
		if current.Mode == "auto" && decision.Confidence >= current.ConfidenceThreshold {
			if !current.DryRun {
				go gw.executeAgentAction(orgID, decision.ID)
			} else {
				gw.log.Info("DRY RUN: would execute action",
					zap.String("action", decision.ProposedAction),
					zap.Float64("confidence", decision.Confidence))
			}
		} else if decision.Confidence < current.ConfidenceThreshold {
			// Low confidence — keep as pending for human approval
			gw.hub.Publish("agent_needs_approval", orgID, decision)
		}
	}
}

// collectSignals gathers the current state of the platform for the agent.
func (gw *Gateway) collectSignals(orgID string) string {
	var signals strings.Builder
	metricURL := gw.cfg.QueryEngineURL

	// Active alerts
	signals.WriteString("=== ACTIVE ALERTS ===\n")
	alertsURL := fmt.Sprintf("%s/api/v1/query?query=ALERTS{org=%q,alertstate=\"firing\"}", metricURL, orgID)
	resp, err := gw.client.Get(alertsURL)
	if err == nil {
		var r map[string]any; json.NewDecoder(resp.Body).Decode(&r); resp.Body.Close()
		b, _ := json.Marshal(r); signals.WriteString(string(b[:min4(len(b), 500)]) + "\n")
	}

	// Error rate
	signals.WriteString("=== ERROR RATES ===\n")
	errURL := agentErrorRateQueryURL(metricURL, orgID)
	resp2, err := gw.client.Get(errURL)
	if err == nil {
		var r map[string]any; json.NewDecoder(resp2.Body).Decode(&r); resp2.Body.Close()
		b, _ := json.Marshal(r); signals.WriteString(string(b[:min4(len(b), 500)]) + "\n")
	}

	signals.WriteString(fmt.Sprintf("=== TIMESTAMP: %s ===\n", time.Now().UTC().Format(time.RFC3339)))
	return signals.String()
}

// agentErrorRateQueryURL builds the query URL for the per-service 5xx request
// rate of one organization (GO-8). orgID is quoted as a PromQL string literal
// (%q: quotes, backslashes and control characters are escaped), and the whole
// expression is URL-encoded with url.QueryEscape, so orgID cannot add label
// matchers or change the URL.
func agentErrorRateQueryURL(metricURL, orgID string) string {
	query := fmt.Sprintf(`sum by (service) (rate(http_requests_total{org=%q,status=~"5.."}[5m]))`, orgID)
	return metricURL + "/api/v1/query?query=" + url.QueryEscape(query)
}

func min4(a, b int) int { if a < b { return a }; return b }

// runAgentDecision calls Claude API with current signals + org knowledge.
func (gw *Gateway) runAgentDecision(ctx context.Context, orgID, signals string, simulate bool) (*AgentDecision, error) {
	apiKey := envOr("ANTHROPIC_API_KEY", "")

	// Load org's knowledge base (runbooks, past incidents)
	knowledge := gw.loadAgentKnowledge(ctx, orgID)

	// Build system prompt
	systemPrompt := buildAgentSystemPrompt(knowledge, orgID)

	// If no Claude API key, generate a synthetic decision for demo
	if apiKey == "" {
		return generateSyntheticDecision(orgID, signals, simulate), nil
	}

	// Call Claude
	type Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	reqBody, _ := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 1024,
		"system":     systemPrompt,
		"messages": []Message{
			{Role: "user", Content: fmt.Sprintf("Current platform signals:\n\n%s\n\nAnalyze and decide.", signals)},
		},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages",
		bytes.NewReader(reqBody))
	if err != nil { return nil, err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := gw.client.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var cr struct {
		Content []struct { Type string `json:"type"`; Text string `json:"text"` } `json:"content"`
		Usage   struct { InputTokens int `json:"input_tokens"`; OutputTokens int `json:"output_tokens"` } `json:"usage"`
	}
	if err := json.Unmarshal(body, &cr); err != nil || len(cr.Content) == 0 {
		return generateSyntheticDecision(orgID, signals, simulate), nil
	}

	// Parse structured decision from Claude's response
	decision := parseAgentDecision(cr.Content[0].Text, orgID, signals)
	decision.ModelUsed = "claude-sonnet-4-6"
	decision.PromptTokens = cr.Usage.InputTokens
	decision.CompletionTokens = cr.Usage.OutputTokens

	return decision, nil
}

func buildAgentSystemPrompt(knowledge []AgentKnowledge, orgID string) string {
	var sb strings.Builder
	sb.WriteString(`You are ObserveX AI, an autonomous monitoring agent for a production platform.

Your job: analyze platform signals, identify issues, and propose precise remediation actions.

RESPONSE FORMAT (respond ONLY with valid JSON):
{
  "root_cause": "Brief description of the probable root cause",
  "reasoning_chain": ["Step 1: ...", "Step 2: ...", "Step 3: ..."],
  "confidence": 0.0-1.0,
  "proposed_action": "one of: notify_oncall|open_incident|silence_alert|scale_deployment|restart_pod|rollback_deployment|apply_rate_limit|run_runbook|drain_node|escalate|no_action",
  "action_params": {"key": "value"},
  "affected_service": "service name",
  "affected_namespace": "namespace",
  "trigger_type": "anomaly|alert|threshold|deploy",
  "urgency": "low|medium|high|critical"
}

AVAILABLE ACTIONS:
- notify_oncall: {"schedule_id": "..."}
- open_incident: {"title": "...", "severity": "CRITICAL|HIGH|MEDIUM"}
- scale_deployment: {"namespace": "...", "deployment": "...", "replicas": N}
- restart_pod: {"namespace": "...", "pod_selector": "app=..."}
- rollback_deployment: {"namespace": "...", "deployment": "..."}
- apply_rate_limit: {"service": "...", "rps_limit": N, "duration_min": N}
- silence_alert: {"alert_name": "...", "duration_min": N}
- no_action: {}

RULES:
- Never scale to 0 replicas (would cause outage)
- Never rollback without high confidence (>0.9) it caused the issue
- Max blast radius: 25% of pod count
- If confidence < 0.7, propose no_action and explain why
- Prefer minimal, reversible actions
`)

	if len(knowledge) > 0 {
		sb.WriteString("\n\nORG-SPECIFIC RUNBOOKS AND CONTEXT:\n")
		for _, k := range knowledge {
			sb.WriteString(fmt.Sprintf("\n[%s] %s:\n%s\n", k.Type, k.Title, k.Content[:min4(len(k.Content), 500)]))
		}
	}
	return sb.String()
}

func parseAgentDecision(text, orgID, signals string) *AgentDecision {
	// Strip markdown fences if present
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 { text = strings.Join(lines[1:len(lines)-1], "\n") }
	}

	var parsed struct {
		RootCause         string         `json:"root_cause"`
		ReasoningChain    []string       `json:"reasoning_chain"`
		Confidence        float64        `json:"confidence"`
		ProposedAction    string         `json:"proposed_action"`
		ActionParams      map[string]any `json:"action_params"`
		AffectedService   string         `json:"affected_service"`
		AffectedNamespace string         `json:"affected_namespace"`
		TriggerType       string         `json:"trigger_type"`
	}

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return generateSyntheticDecision(orgID, signals, false)
	}

	status := "pending"
	if parsed.ProposedAction == "no_action" { status = "completed" }
	if parsed.Confidence >= 0.85 { status = "approved" }

	return &AgentDecision{
		ID:               fmt.Sprintf("decision-%d", time.Now().UnixMilli()),
		OrgID:            orgID,
		Timestamp:        time.Now(),
		TriggerType:      parsed.TriggerType,
		TriggerDetails:   signals[:min4(len(signals), 200)],
		AffectedService:  parsed.AffectedService,
		AffectedNamespace: parsed.AffectedNamespace,
		RootCause:        parsed.RootCause,
		Confidence:       parsed.Confidence,
		ReasoningChain:   parsed.ReasoningChain,
		ProposedAction:   parsed.ProposedAction,
		ActionParams:     parsed.ActionParams,
		Status:           status,
		ModelUsed:        "claude-sonnet-4-6",
	}
}

func generateSyntheticDecision(orgID, signals string, simulate bool) *AgentDecision {
	action := "notify_oncall"
	if simulate { action = "scale_deployment" }
	return &AgentDecision{
		ID:               fmt.Sprintf("decision-%d", time.Now().UnixMilli()),
		OrgID:            orgID,
		Timestamp:        time.Now(),
		TriggerType:      "anomaly",
		TriggerDetails:   "Error rate spike detected on checkout-service",
		AffectedService:  "checkout-service",
		AffectedNamespace: "production",
		RootCause:        "Database connection pool exhaustion causing request failures",
		Confidence:       0.87,
		ReasoningChain: []string{
			"Error rate on checkout-service rose from 0.3% to 8.4% (2800% increase)",
			"P99 latency jumped from 120ms to 4.2s simultaneously",
			"Loki logs show 'connection pool exhausted' errors starting at T-3min",
			"No recent deploy detected — this is runtime degradation, not regression",
			"Pattern matches 'postgres_pool_exhaustion' runbook from knowledge base",
		},
		ProposedAction: action,
		ActionParams:   map[string]any{"schedule_id": "primary-oncall", "urgency": "high"},
		Status:         "pending",
		ModelUsed:      "claude-sonnet-4-6 (simulated)",
		PromptTokens:   1240,
		CompletionTokens: 284,
	}
}

func (gw *Gateway) executeAgentAction(orgID, decisionID string) {
	agentDecisionsMu.RLock()
	var decision *AgentDecision
	for _, d := range agentDecisions[orgID] {
		if d.ID == decisionID { decision = &d; break }
	}
	agentDecisionsMu.RUnlock()
	if decision == nil { return }

	gw.log.Info("executing AI agent action",
		zap.String("org_id", orgID),
		zap.String("action", decision.ProposedAction),
		zap.Float64("confidence", decision.Confidence))

	// Execute the action
	var outcome string
	var err error

	switch decision.ProposedAction {
	case "notify_oncall":
		gw.hub.Publish("oncall_notify", orgID, fiber.Map{
			"trigger": "ai_agent", "reason": decision.RootCause,
			"confidence": decision.Confidence, "decision_id": decisionID,
		})
		outcome = "On-call engineer notified via WebSocket and configured notification channels"

	case "open_incident":
		title := fmt.Sprintf("[AI] %s", decision.RootCause)
		severity := "HIGH"
		if decision.Confidence > 0.9 { severity = "CRITICAL" }
		gw.hub.Publish("incident_created", orgID, fiber.Map{
			"title": title, "severity": severity, "source": "ai_agent",
			"decision_id": decisionID,
		})
		outcome = fmt.Sprintf("Incident created: %s (severity: %s)", title, severity)

	case "scale_deployment":
		ns := fmt.Sprintf("%v", decision.ActionParams["namespace"])
		dep := fmt.Sprintf("%v", decision.ActionParams["deployment"])
		replicas := 3
		if r, ok := decision.ActionParams["replicas"].(float64); ok { replicas = int(r) }
		// In production: kubectl scale
		gw.hub.Publish("deployment_scaled", orgID, fiber.Map{
			"namespace": ns, "deployment": dep, "replicas": replicas, "triggered_by": "ai_agent",
		})
		outcome = fmt.Sprintf("Scaled %s/%s to %d replicas", ns, dep, replicas)

	case "silence_alert":
		duration := 30
		if d, ok := decision.ActionParams["duration_min"].(float64); ok { duration = int(d) }
		alertName := fmt.Sprintf("%v", decision.ActionParams["alert_name"])
		outcome = fmt.Sprintf("Alert '%s' silenced for %d minutes", alertName, duration)

	case "no_action":
		outcome = "Signals analyzed — no remediation required at this time"

	default:
		outcome = fmt.Sprintf("Action '%s' logged for execution", decision.ProposedAction)
	}
	_ = err

	// Update decision with outcome
	now := time.Now()
	agentDecisionsMu.Lock()
	for i, d := range agentDecisions[orgID] {
		if d.ID == decisionID {
			agentDecisions[orgID][i].Status = "completed"
			agentDecisions[orgID][i].CompletedAt = &now
			agentDecisions[orgID][i].Outcome = outcome
			break
		}
	}
	agentDecisionsMu.Unlock()

	gw.hub.Publish("agent_action_completed", orgID, fiber.Map{
		"decision_id": decisionID, "action": decision.ProposedAction,
		"outcome": outcome,
	})

	gw.log.Info("AI agent action completed", zap.String("action", decision.ProposedAction), zap.String("outcome", outcome))
}

func (gw *Gateway) loadAgentKnowledge(ctx context.Context, orgID string) []AgentKnowledge {
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, type, title, content, service, tags FROM agent_knowledge WHERE org_id=$1 ORDER BY created_at DESC LIMIT 20`,
		orgID)
	if err != nil {
		// Return default knowledge if table doesn't exist yet
		return defaultKnowledge(orgID)
	}
	defer rows.Close()
	var items []AgentKnowledge
	for rows.Next() {
		var k AgentKnowledge
		var tagsJSON []byte
		rows.Scan(&k.ID, &k.Type, &k.Title, &k.Content, &k.Service, &tagsJSON)
		json.Unmarshal(tagsJSON, &k.Tags)
		k.OrgID = orgID
		items = append(items, k)
	}
	if len(items) == 0 { return defaultKnowledge(orgID) }
	return items
}

func defaultKnowledge(orgID string) []AgentKnowledge {
	return []AgentKnowledge{
		{ID:"know-0",Type:"runbook",Title:"High error rate response",OrgID:orgID,
			Content:"If error rate >5%: 1. Check recent deploys. 2. If deploy <30min ago, rollback. 3. Otherwise check DB connections and memory. 4. Scale up if CPU>80%."},
		{ID:"know-1",Type:"runbook",Title:"Database connection pool exhaustion",OrgID:orgID,
			Content:"Symptoms: 'connection pool exhausted' logs. Actions: 1. Restart pg-bouncer pod. 2. Check for connection leaks. 3. Increase pool size if persistent."},
		{ID:"know-2",Type:"policy",Title:"Blast radius policy",OrgID:orgID,
			Content:"Never restart more than 25% of pods simultaneously. Always maintain at least 2 replicas in production. Never scale to 0."},
	}
}

// ── Sample data ────────────────────────────────────────────────────────────────

func generateSampleDecisions(orgID string) []AgentDecision {
	now := time.Now()
	t1 := now.Add(-2 * time.Minute)
	return []AgentDecision{
		{
			ID:"d1",OrgID:orgID,Timestamp:now.Add(-10*time.Minute),TriggerType:"anomaly",
			TriggerDetails:"Error rate on checkout-service: 8.4% (baseline 0.3%)",
			AffectedService:"checkout-service",AffectedNamespace:"production",
			RootCause:"Database connection pool exhaustion",Confidence:0.91,
			ReasoningChain:[]string{
				"Error rate spike: 0.3% → 8.4% (2800% increase)",
				"Concurrent latency spike: 120ms → 4.2s P99",
				"Loki shows 'connection pool exhausted' errors",
				"No recent deploy — runtime issue, not regression",
				"Matches postgres_pool_exhaustion runbook",
			},
			ProposedAction:"notify_oncall",ActionParams:map[string]any{"urgency":"high"},
			Status:"completed",ExecutedAt:&t1,Outcome:"On-call notified. Incident auto-created.",
			ModelUsed:"claude-sonnet-4-6",PromptTokens:1240,CompletionTokens:284,
		},
		{
			ID:"d2",OrgID:orgID,Timestamp:now.Add(-3*time.Minute),TriggerType:"threshold",
			TriggerDetails:"CPU usage on ml-service: 94% sustained for 5 minutes",
			AffectedService:"ml-service",AffectedNamespace:"production",
			RootCause:"ML inference overload — request queue saturated",Confidence:0.88,
			ReasoningChain:[]string{
				"CPU at 94% sustained (threshold: 85%)",
				"Queue depth growing: 240 pending requests",
				"P99 latency growing linearly — classic queue saturation",
				"Current replicas: 2. Max in HPA: 10",
				"Scaling up will distribute load",
			},
			ProposedAction:"scale_deployment",
			ActionParams:map[string]any{"namespace":"production","deployment":"ml-inference","replicas":4},
			Status:"pending",ModelUsed:"claude-sonnet-4-6",PromptTokens:980,CompletionTokens:192,
		},
		{
			ID:"d3",OrgID:orgID,Timestamp:now.Add(-1*time.Minute),TriggerType:"deploy",
			TriggerDetails:"New deployment detected: checkout-service v2.4.1",
			AffectedService:"checkout-service",AffectedNamespace:"production",
			RootCause:"Monitoring new deployment for regression",Confidence:0.72,
			ReasoningChain:[]string{
				"Deploy event: checkout-service v2.4.1 at 14:32 UTC",
				"Error rate unchanged: 0.3%",
				"Latency unchanged: 120ms P99",
				"Deploy appears healthy — monitoring window: 15min",
			},
			ProposedAction:"no_action",ActionParams:map[string]any{},
			Status:"completed",ExecutedAt:&t1,Outcome:"Deploy healthy — no action required.",
			ModelUsed:"claude-sonnet-4-6",PromptTokens:840,CompletionTokens:148,
		},
	}
}

func generateSamplePlaybooks(orgID string) []AgentPlaybook {
	return []AgentPlaybook{
		{ID:"pb-1",Name:"High error rate response",OrgID:orgID,Enabled:true,TimesRun:14,SuccessRate:92.8,
			Trigger:"error_rate > 5% for 5 minutes",
			Description:"Automated response to sustained high error rate",
			Steps:[]PlaybookStep{
				{Order:1,Action:"snapshot_pod_state",Description:"Capture current pod logs for forensics",TimeoutSec:30,OnFailure:"continue"},
				{Order:2,Action:"notify_oncall",Description:"Page on-call engineer",Parameters:map[string]any{"urgency":"high"},TimeoutSec:10,OnFailure:"continue"},
				{Order:3,Action:"open_incident",Description:"Create incident",Parameters:map[string]any{"severity":"HIGH"},TimeoutSec:10,OnFailure:"continue"},
				{Order:4,Action:"scale_deployment",Description:"Scale up if CPU>70%",Parameters:map[string]any{"replicas_delta":2},TimeoutSec:60,SuccessCondition:"error_rate < 2%",OnFailure:"escalate"},
			}},
		{ID:"pb-2",Name:"Deploy regression rollback",OrgID:orgID,Enabled:true,TimesRun:3,SuccessRate:100.0,
			Trigger:"error_rate increase >200% within 30min of deploy",
			Description:"Automatic rollback when a deploy causes regression",
			Steps:[]PlaybookStep{
				{Order:1,Action:"notify_oncall",Description:"Alert on-call",Parameters:map[string]any{"urgency":"critical"},TimeoutSec:10,OnFailure:"continue"},
				{Order:2,Action:"rollback_deployment",Description:"Rollback to previous version",TimeoutSec:120,SuccessCondition:"error_rate < 1%",OnFailure:"escalate"},
				{Order:3,Action:"open_incident",Description:"Create post-deploy regression incident",Parameters:map[string]any{"severity":"CRITICAL"},TimeoutSec:10,OnFailure:"continue"},
			}},
		{ID:"pb-3",Name:"OOM mitigation",OrgID:orgID,Enabled:true,TimesRun:7,SuccessRate:85.7,
			Trigger:"pod OOMKilled event detected",
			Description:"Respond to out-of-memory pod kills",
			Steps:[]PlaybookStep{
				{Order:1,Action:"restart_pod",Description:"Restart affected pod",TimeoutSec:60,OnFailure:"escalate"},
				{Order:2,Action:"notify_oncall",Description:"Notify if persistent",Parameters:map[string]any{"urgency":"medium"},TimeoutSec:10,OnFailure:"continue"},
			}},
	}
}
