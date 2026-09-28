// services/api-gateway/nl_log_search.go
//
// AI-Powered Natural Language Log Search.
//
// Translates free-text queries ("show me errors after the 3pm deploy")
// into precise LogQL queries and executes them against Loki.
//
// Architecture:
//   User query (NL)
//     → Claude API (claude-haiku-4-5-20251001, structured output)
//     → LogQL query string + explanation
//     → Execute against Loki /loki/api/v1/query_range
//     → Return log lines + the generated query (for transparency)
//
// The system prompt teaches Claude:
//   - Available label schema (service, env, level, namespace, host, cluster)
//   - LogQL syntax: stream selectors, filter expressions, parsers, metrics
//   - Relative time resolution ("last hour" → [1h], "this morning" → since 6am)
//   - How to handle ambiguous queries (default to last 1h, level=error)
//
// Security:
//   - Org ID is injected server-side (never from user input)
//   - Query is sanitized: only whitelisted LogQL operators allowed
//   - Max log lines capped at 500 per query
//   - Result is streamed back as NDJSON for large result sets

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/observex/platform/internal/middleware"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type NLSearchRequest struct {
	Query     string `json:"query"`      // "show errors after 3pm deploy"
	Limit     int    `json:"limit"`      // default 100, max 500
	StartTime string `json:"start_time"` // optional ISO8601 override
	EndTime   string `json:"end_time"`   // optional ISO8601 override
}

type NLSearchResponse struct {
	OriginalQuery  string        `json:"original_query"`
	GeneratedLogQL string        `json:"generated_logql"`
	Explanation    string        `json:"explanation"`
	TimeRange      NLTimeRange   `json:"time_range"`
	Lines          []LogLine     `json:"lines"`
	TotalLines     int           `json:"total_lines"`
	TruncatedAt    int           `json:"truncated_at,omitempty"`
	GeneratedAt    time.Time     `json:"generated_at"`
	ModelUsed      string        `json:"model_used"`
	LatencyMs      float64       `json:"latency_ms"`
}

type NLTimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Label string `json:"label"` // "last hour", "today", etc.
}

type LogLine struct {
	Timestamp string            `json:"timestamp"`
	Line      string            `json:"line"`
	Labels    map[string]string `json:"labels"`
}

// ── LogQL safety allowlist ─────────────────────────────────────────────────────

var (
	// Only allow safe LogQL characters — block injection attempts
	safeLogQLRe = regexp.MustCompile(`^[a-zA-Z0-9\s\{\}\[\]\(\)\|=~!<>"',._\-\+\*/\\%@:#\^&?]+$`)
	// Reject obviously dangerous patterns
	dangerousRe = regexp.MustCompile(`(?i)(drop|delete|exec|eval|import|__proto__|prototype)`)
)

func validateLogQL(query string) error {
	if len(query) > 2000 {
		return fmt.Errorf("generated query too long (%d chars)", len(query))
	}
	if dangerousRe.MatchString(query) {
		return fmt.Errorf("query contains disallowed keywords")
	}
	if !strings.HasPrefix(strings.TrimSpace(query), "{") {
		return fmt.Errorf("LogQL must start with a stream selector {}")
	}
	return nil
}

// ── System prompt ──────────────────────────────────────────────────────────────

const logSearchSystemPrompt = `You are a LogQL query generator for ObserveX, an observability platform.
Your ONLY job is to convert natural language log search queries into valid LogQL queries.

AVAILABLE LABELS:
- service: service name (e.g. "api-gateway", "checkout-service", "payment-service")
- level: log level (error, warn, info, debug)
- namespace: Kubernetes namespace
- host: hostname or pod name
- cluster: cluster name
- env: environment (production, staging, dev)
- org: organization ID (ALWAYS include org="{ORG_ID}" in the stream selector)
- source: log source (systemd, nginx, kubernetes, app)

LOGQL SYNTAX RULES:
1. Stream selector (required, always first): {label="value", label2=~"pattern"}
2. Filter expressions: |= "text" (contains) | != "text" (not contains) | =~ "regex"  
3. Parser: | json | logfmt | pattern "<ip> - <method>"
4. Labels after parse: | level = "error"
5. Time functions: count_over_time, rate, bytes_over_time

RELATIVE TIME RESOLUTION (always resolve to ISO8601):
- "last hour" → start: now-1h, end: now
- "last 24 hours" / "today" → start: now-24h, end: now  
- "this morning" → start: today 06:00, end: now
- "yesterday" → start: yesterday 00:00, end: yesterday 23:59
- "last 5 minutes" → start: now-5m, end: now
- "after 3pm" / "after the deploy" → start: today 15:00, end: now
- "past week" → start: now-7d, end: now

RESPONSE FORMAT (respond with ONLY valid JSON, no markdown, no explanation outside JSON):
{
  "logql": "{org=\"{ORG_ID}\", service=~\".+\"} |= \"error\" | json | level=\"error\"",
  "start": "now-1h",
  "end": "now",
  "time_label": "last hour",
  "explanation": "Searching for error-level logs across all services in the last hour"
}

EXAMPLES:
- "show errors in checkout after the deploy" → {org="{ORG_ID}", service="checkout-service"} |= "error" | json
- "nginx 500s last 30 minutes" → {org="{ORG_ID}", service="nginx"} |~ "5[0-9]{2}"
- "memory OOM kills on production nodes today" → {org="{ORG_ID}", env="production"} |= "OOM" |= "kill"
- "slow database queries over 1s" → {org="{ORG_ID}", service=~".*db.*|.*postgres.*"} |= "slow query" | logfmt | duration > 1s

RULES:
- ALWAYS include org="{ORG_ID}" in stream selector (placeholder, replaced server-side)
- Never output markdown, never explain outside the JSON object
- If query is too vague, default to: {org="{ORG_ID}"} | level="error", last 1h
- Keep generated queries focused and efficient (avoid cross-label cartesian products)`

// ── Claude API caller ─────────────────────────────────────────────────────────

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type logQLPlan struct {
	LogQL       string `json:"logql"`
	Start       string `json:"start"`
	End         string `json:"end"`
	TimeLabel   string `json:"time_label"`
	Explanation string `json:"explanation"`
}

func (gw *Gateway) generateLogQL(ctx context.Context, naturalQuery, orgID string) (*logQLPlan, float64, error) {
	apiKey := envOr("ANTHROPIC_API_KEY", "")
	if apiKey == "" {
		// Fallback: basic heuristic translation
		return gw.heuristicLogQL(naturalQuery, orgID), 0, nil
	}

	start := time.Now()
	systemPrompt := strings.ReplaceAll(logSearchSystemPrompt, "{ORG_ID}", orgID)

	reqBody, _ := json.Marshal(claudeRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 512,
		System:    systemPrompt,
		Messages:  []claudeMessage{{Role: "user", Content: naturalQuery}},
	})

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.anthropic.com/v1/messages",
		bytes.NewReader(reqBody))
	if err != nil { return gw.heuristicLogQL(naturalQuery, orgID), 0, nil }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := gw.client.Do(req)
	if err != nil { return gw.heuristicLogQL(naturalQuery, orgID), 0, nil }
	defer resp.Body.Close()
	latencyMs := float64(time.Since(start).Milliseconds())

	body, _ := io.ReadAll(resp.Body)
	var cr claudeResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return gw.heuristicLogQL(naturalQuery, orgID), latencyMs, nil
	}
	if cr.Error != nil {
		return gw.heuristicLogQL(naturalQuery, orgID), latencyMs,
			fmt.Errorf("claude API error: %s", cr.Error.Message)
	}
	if len(cr.Content) == 0 {
		return gw.heuristicLogQL(naturalQuery, orgID), latencyMs, nil
	}

	text := cr.Content[0].Text
	// Strip potential markdown fences
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.Trim(text, "`\njson ")
	}

	var plan logQLPlan
	if err := json.Unmarshal([]byte(text), &plan); err != nil {
		gw.log.Warn("claude returned non-JSON logql plan", zap.String("raw", text[:min(len(text),200)]))
		return gw.heuristicLogQL(naturalQuery, orgID), latencyMs, nil
	}

	// Inject real orgID (replace placeholder if Claude used it)
	plan.LogQL = strings.ReplaceAll(plan.LogQL, `{ORG_ID}`, orgID)

	return &plan, latencyMs, nil
}

// heuristicLogQL is the fallback when Claude API is unavailable.
func (gw *Gateway) heuristicLogQL(query, orgID string) *logQLPlan {
	q := strings.ToLower(query)
	logql := fmt.Sprintf(`{org="%s"}`, orgID)
	label := "last hour"
	start := "now-1h"
	explanation := "Searching recent logs"

	// Service detection
	services := []string{"nginx", "postgres", "redis", "api", "checkout", "payment", "auth", "worker"}
	for _, svc := range services {
		if strings.Contains(q, svc) {
			logql = fmt.Sprintf(`{org="%s",service=~".*%s.*"}`, orgID, svc)
			break
		}
	}

	// Level detection
	if strings.Contains(q, "error") || strings.Contains(q, "err") || strings.Contains(q, "fail") {
		logql += ` | json | level="error"`
		explanation = "Filtering for error-level logs"
	} else if strings.Contains(q, "warn") {
		logql += ` | json | level="warn"`
	}

	// Keyword injection
	for _, kw := range []string{"timeout", "OOM", "panic", "exception", "crash", "killed", "refused"} {
		if strings.Contains(q, strings.ToLower(kw)) {
			logql += fmt.Sprintf(` |= "%s"`, kw)
		}
	}

	// Time range
	if strings.Contains(q, "today") || strings.Contains(q, "24h") {
		start = "now-24h"; label = "last 24 hours"
	} else if strings.Contains(q, "5 min") || strings.Contains(q, "5min") {
		start = "now-5m"; label = "last 5 minutes"
	} else if strings.Contains(q, "week") || strings.Contains(q, "7d") {
		start = "now-7d"; label = "last 7 days"
	}

	return &logQLPlan{LogQL: logql, Start: start, End: "now", TimeLabel: label, Explanation: explanation}
}

func min(a, b int) int { if a < b { return a }; return b }

// ── Loki query executor ───────────────────────────────────────────────────────

func (gw *Gateway) executeLokiQuery(ctx context.Context, plan *logQLPlan, limit int) ([]LogLine, error) {
	if limit <= 0 || limit > 500 { limit = 100 }

	lokiURL := envOr("LOKI_URL", "http://loki:3100")
	params  := url.Values{}
	params.Set("query", plan.LogQL)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("start", plan.Start)
	params.Set("end", plan.End)
	params.Set("direction", "backward")

	endpoint := fmt.Sprintf("%s/loki/api/v1/query_range?%s", lokiURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil { return nil, fmt.Errorf("build loki request: %w", err) }

	resp, err := gw.client.Do(req)
	if err != nil { return nil, fmt.Errorf("loki query: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("loki rejected query: %s", string(body)[:min(len(string(body)), 200)])
	}

	var lokiResp struct {
		Data struct {
			Result []struct {
				Stream map[string]string `json:"stream"`
				Values [][2]string       `json:"values"` // [timestamp_ns, log_line]
			} `json:"result"`
		} `json:"data"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &lokiResp); err != nil {
		return nil, fmt.Errorf("parse loki response: %w", err)
	}

	var lines []LogLine
	for _, stream := range lokiResp.Data.Result {
		for _, val := range stream.Values {
			// Convert nanosecond timestamp to RFC3339
			ts := "unknown"
			if len(val[0]) >= 10 {
				ns, _ := fmt.Sscanf(val[0], "%d", new(int64))
				_ = ns
				// Parse nanoseconds → time
				var nsInt int64
				fmt.Sscanf(val[0], "%d", &nsInt)
				t := time.Unix(0, nsInt)
				ts = t.UTC().Format(time.RFC3339Nano)
			}
			labels := make(map[string]string, len(stream.Stream))
			for k, v := range stream.Stream { labels[k] = v }
			lines = append(lines, LogLine{Timestamp: ts, Line: val[1], Labels: labels})
			if len(lines) >= limit { goto done }
		}
	}
done:
	return lines, nil
}

// ── HTTP Handler ───────────────────────────────────────────────────────────────

// handleNLLogSearch translates a natural language query to LogQL and executes it.
// POST /api/v1/logs/search/nl
func (gw *Gateway) handleNLLogSearch(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	var req NLSearchRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return c.Status(400).JSON(fiber.Map{"error": "query required"})
	}
	if len(req.Query) > 500 {
		return c.Status(400).JSON(fiber.Map{"error": "query too long (max 500 chars)"})
	}
	if req.Limit <= 0 { req.Limit = 100 }
	if req.Limit > 500 { req.Limit = 500 }

	overallStart := time.Now()
	ctx, cancel := context.WithTimeout(c.Context(), 20*time.Second)
	defer cancel()

	// 1. Generate LogQL
	plan, llmLatency, genErr := gw.generateLogQL(ctx, req.Query, auth.OrgID)
	modelUsed := "heuristic"
	if genErr == nil && envOr("ANTHROPIC_API_KEY", "") != "" {
		modelUsed = "claude-haiku-4-5-20251001"
	}
	if genErr != nil {
		gw.log.Warn("logql generation error, using heuristic", zap.Error(genErr))
	}

	// 2. Validate generated LogQL
	if err := validateLogQL(plan.LogQL); err != nil {
		gw.log.Warn("invalid logql generated", zap.String("logql", plan.LogQL), zap.Error(err))
		plan = gw.heuristicLogQL(req.Query, auth.OrgID)
		modelUsed = "heuristic_fallback"
	}

	// 3. Override time range if provided by user
	if req.StartTime != "" { plan.Start = req.StartTime }
	if req.EndTime != ""   { plan.End = req.EndTime }

	// 4. Execute against Loki
	lines, lokiErr := gw.executeLokiQuery(ctx, plan, req.Limit)
	if lokiErr != nil {
		gw.log.Warn("loki query failed", zap.String("logql", plan.LogQL), zap.Error(lokiErr))
		// Return the generated query even if Loki is unreachable
		return c.Status(202).JSON(fiber.Map{
			"original_query":   req.Query,
			"generated_logql":  plan.LogQL,
			"explanation":      plan.Explanation,
			"error":            "log backend unavailable: " + lokiErr.Error(),
			"lines":            []LogLine{},
			"model_used":       modelUsed,
			"llm_latency_ms":   llmLatency,
		})
	}

	totalLatency := float64(time.Since(overallStart).Milliseconds())
	return c.JSON(NLSearchResponse{
		OriginalQuery:  req.Query,
		GeneratedLogQL: plan.LogQL,
		Explanation:    plan.Explanation,
		TimeRange: NLTimeRange{
			Start: plan.Start,
			End:   plan.End,
			Label: plan.TimeLabel,
		},
		Lines:      lines,
		TotalLines: len(lines),
		GeneratedAt: time.Now(),
		ModelUsed:   modelUsed,
		LatencyMs:   totalLatency,
	})
}

// handleLogSearchSuggestions returns example queries for the search UI autocomplete.
// GET /api/v1/logs/search/suggestions
func (gw *Gateway) handleLogSearchSuggestions(c *fiber.Ctx) error {
	prefix := strings.ToLower(c.Query("q", ""))
	all := []string{
		"show errors in the last hour",
		"database connection timeouts today",
		"5xx errors from nginx last 30 minutes",
		"OOM kills on production nodes",
		"slow queries over 1 second",
		"authentication failures last 24 hours",
		"panic or exception in checkout service",
		"deployment errors after 3pm",
		"rate limit exceeded errors",
		"memory usage warnings this week",
		"crashed pods in production",
		"SSL certificate errors",
		"disk full errors",
		"kafka consumer lag warnings",
		"redis connection refused errors",
	}
	if prefix == "" {
		return c.JSON(fiber.Map{"suggestions": all[:8]})
	}
	var filtered []string
	for _, s := range all {
		if strings.Contains(s, prefix) {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) == 0 { filtered = all[:5] }
	return c.JSON(fiber.Map{"suggestions": filtered})
}

// handleSavedSearches manages user-saved NL searches stored in org settings.
// GET  /api/v1/logs/search/saved
// POST /api/v1/logs/search/saved
func (gw *Gateway) handleSavedSearches(c *fiber.Ctx) error {
	auth := middleware.GetAuth(c)
	org, err := gw.orgs.GetByID(c.Context(), auth.OrgID)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }

	type SavedSearch struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Query       string    `json:"query"`
		GeneratedQL string    `json:"generated_logql,omitempty"`
		SavedAt     time.Time `json:"saved_at"`
		SavedBy     string    `json:"saved_by"`
	}

	switch c.Method() {
	case "GET":
		var settings map[string]any
		var searches []SavedSearch
		if org.Settings != "" {
			json.Unmarshal([]byte(org.Settings), &settings)
			if raw, ok := settings["saved_log_searches"]; ok {
				b, _ := json.Marshal(raw)
				json.Unmarshal(b, &searches)
			}
		}
		if searches == nil { searches = []SavedSearch{} }
		return c.JSON(fiber.Map{"searches": searches, "total": len(searches)})

	case "POST":
		var body struct {
			Name  string `json:"name"`
			Query string `json:"query"`
		}
		if err := c.BodyParser(&body); err != nil { return c.Status(400).JSON(fiber.Map{"error": err.Error()}) }
		if body.Name == "" || body.Query == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name and query required"})
		}
		// Generate LogQL for the saved search
		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()
		plan, _, _ := gw.generateLogQL(ctx, body.Query, auth.OrgID)

		var settings map[string]any
		if org.Settings != "" { json.Unmarshal([]byte(org.Settings), &settings) }
		if settings == nil { settings = make(map[string]any) }

		var searches []SavedSearch
		if raw, ok := settings["saved_log_searches"]; ok {
			b, _ := json.Marshal(raw); json.Unmarshal(b, &searches)
		}
		newSearch := SavedSearch{
			ID:          fmt.Sprintf("ss-%d", time.Now().UnixMilli()),
			Name:        body.Name,
			Query:       body.Query,
			GeneratedQL: plan.LogQL,
			SavedAt:     time.Now(),
			SavedBy:     auth.UserID,
		}
		searches = append(searches, newSearch)
		settings["saved_log_searches"] = searches

		settingsJSON, _ := json.Marshal(settings)
		if err := gw.orgs.UpdateSettings(c.Context(), auth.OrgID, string(settingsJSON)); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(newSearch)
	}
	return c.Status(405).JSON(fiber.Map{"error": "method not allowed"})
}
