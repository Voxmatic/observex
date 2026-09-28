// services/query-engine/main.go
// ObserveX Query Engine — unified native query API for all telemetry backends.
// Metric reads use the ObserveX metric table in ClickHouse. Clients can use a
// selector such as host.cpu.usage{node="worker-1"}; query expressions are not
// passed through to a third-party metrics engine.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type Config struct {
	Port          string
	LokiURL       string
	TempoURL      string
	ClickHouseURL string
}

func loadConfig() Config {
	return Config{
		Port:          envOr("PORT", "9090"),
		LokiURL:       envOr("LOKI_URL", "http://loki:3100"),
		TempoURL:      envOr("TEMPO_URL", "http://tempo:3200"),
		ClickHouseURL: envOr("CLICKHOUSE_URL", "http://clickhouse:8123"),
	}
}

type QueryEngine struct {
	cfg    Config
	client *http.Client
	log    *zap.Logger
}

type QueryRequest struct {
	Type      string `json:"type"`
	Query     string `json:"query"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Step      string `json:"step"`
	Limit     int    `json:"limit"`
	Direction string `json:"direction"`
}

func (qe *QueryEngine) proxy(c *fiber.Ctx, method, targetURL string, body []byte) error {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(c.Context(), method, targetURL, bodyReader)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if contentType := c.Get("Content-Type"); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Query-Engine", "observex-native/1")
	resp, err := qe.client.Do(req)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "backend unavailable"})
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "failed to read backend response"})
	}
	c.Status(resp.StatusCode)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Set("Content-Type", contentType)
	}
	return c.Send(responseBody)
}

// ─── Native metrics ────────────────────────────────────────────────────────

var metricSelectorPattern = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.:-]*)(?:\{(.*)\})?$`)
var metricLabelPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*"([^"\\]*)"\s*$`)

type metricSelector struct {
	Name   string
	Labels map[string]string
}

type metricRow struct {
	Name      string  `json:"name"`
	ServiceID string  `json:"service_id"`
	Labels    string  `json:"labels"`
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

type metricSeries struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"`
}

func parseMetricSelector(raw string) (metricSelector, error) {
	raw = strings.TrimSpace(raw)
	match := metricSelectorPattern.FindStringSubmatch(raw)
	if match == nil {
		return metricSelector{}, fmt.Errorf("use a native metric selector such as host.cpu.usage{node=\"worker-1\"}")
	}
	selector := metricSelector{Name: match[1], Labels: map[string]string{}}
	if match[2] == "" {
		return selector, nil
	}
	parts := strings.Split(match[2], ",")
	if len(parts) > 32 {
		return metricSelector{}, fmt.Errorf("a metric selector may contain at most 32 labels")
	}
	for _, part := range parts {
		labelMatch := metricLabelPattern.FindStringSubmatch(part)
		if labelMatch == nil {
			return metricSelector{}, fmt.Errorf("labels must use key=\"value\" syntax")
		}
		selector.Labels[labelMatch[1]] = labelMatch[2]
	}
	return selector, nil
}

func quotedSQL(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func parseMetricTime(raw string, fallback time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if raw == "now" {
		return time.Now(), nil
	}
	if strings.HasPrefix(raw, "now-") {
		duration, err := time.ParseDuration(strings.TrimPrefix(raw, "now-"))
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative time %q", raw)
		}
		return time.Now().Add(-duration), nil
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Unix(int64(seconds), 0), nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be Unix seconds, RFC3339, now, or now-duration")
	}
	return parsed, nil
}

func metricWhere(selector metricSelector, start, end time.Time, orgID string) string {
	conditions := []string{
		"name = " + quotedSQL(selector.Name),
		fmt.Sprintf("timestamp >= toDateTime(%d)", start.Unix()),
		fmt.Sprintf("timestamp <= toDateTime(%d)", end.Unix()),
	}
	if orgID != "" {
		conditions = append(conditions, "JSONExtractString(labels, 'org') = "+quotedSQL(orgID))
	}
	labelNames := make([]string, 0, len(selector.Labels))
	for name := range selector.Labels {
		labelNames = append(labelNames, name)
	}
	sort.Strings(labelNames)
	for _, name := range labelNames {
		value := selector.Labels[name]
		switch name {
		case "service", "service_id":
			conditions = append(conditions, "service_id = "+quotedSQL(value))
		default:
			conditions = append(conditions, "JSONExtractString(labels, "+quotedSQL(name)+") = "+quotedSQL(value))
		}
	}
	return strings.Join(conditions, " AND ")
}

func (qe *QueryEngine) nativeRows(query string) ([]metricRow, error) {
	target := strings.TrimRight(qe.cfg.ClickHouseURL, "/") + "/?database=observex"
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := qe.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("native metric store returned %d", resp.StatusCode)
	}
	var result struct {
		Data []metricRow `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func metricIdentity(row metricRow) (string, map[string]string) {
	labels := map[string]string{"__name__": row.Name}
	if row.ServiceID != "" {
		labels["service_id"] = row.ServiceID
	}
	if row.Labels != "" {
		var stored map[string]string
		if json.Unmarshal([]byte(row.Labels), &stored) == nil {
			for key, value := range stored {
				labels[key] = value
			}
		}
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+labels[key])
	}
	return strings.Join(parts, "\x00"), labels
}

func metricValue(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (qe *QueryEngine) metricRange(selector metricSelector, start, end time.Time, orgID string) (fiber.Map, error) {
	where := metricWhere(selector, start, end, orgID)
	rows, err := qe.nativeRows("SELECT name, service_id, labels, toUnixTimestamp(timestamp) AS timestamp, value FROM metrics WHERE "+where+" ORDER BY timestamp ASC LIMIT 100000 FORMAT JSON")
	if err != nil {
		return nil, err
	}
	groups := map[string]*metricSeries{}
	for _, row := range rows {
		key, labels := metricIdentity(row)
		series := groups[key]
		if series == nil {
			series = &metricSeries{Metric: labels}
			groups[key] = series
		}
		series.Values = append(series.Values, []interface{}{row.Timestamp, metricValue(row.Value)})
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]metricSeries, 0, len(keys))
	for _, key := range keys {
		result = append(result, *groups[key])
	}
	return fiber.Map{"status": "success", "data": fiber.Map{"resultType": "matrix", "result": result, "source": "observex_native"}}, nil
}

func (qe *QueryEngine) metricInstant(selector metricSelector, at time.Time, orgID string) (fiber.Map, error) {
	start := at.Add(-90 * 24 * time.Hour)
	where := metricWhere(selector, start, at, orgID)
	rows, err := qe.nativeRows("SELECT name, service_id, labels, toUnixTimestamp(timestamp) AS timestamp, value FROM metrics WHERE "+where+" ORDER BY timestamp DESC LIMIT 100000 FORMAT JSON")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	result := make([]fiber.Map, 0)
	for _, row := range rows {
		key, labels := metricIdentity(row)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, fiber.Map{"metric": labels, "value": []interface{}{row.Timestamp, metricValue(row.Value)}})
	}
	return fiber.Map{"status": "success", "data": fiber.Map{"resultType": "vector", "result": result, "source": "observex_native"}}, nil
}

func metricError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"status": "error", "error": err.Error()})
}

// metricOrgID is supplied by the authenticated API gateway. The query engine is
// an internal service; a missing value is retained for local operator use only.
func metricOrgID(c *fiber.Ctx) string {
	return strings.TrimSpace(c.Get("X-ObserveX-Org"))
}

func (qe *QueryEngine) handleMetricsRange(c *fiber.Ctx) error {
	selector, err := parseMetricSelector(c.Query("query"))
	if err != nil {
		return metricError(c, err)
	}
	end, err := parseMetricTime(c.Query("end"), time.Now())
	if err != nil {
		return metricError(c, err)
	}
	start, err := parseMetricTime(c.Query("start"), end.Add(-time.Hour))
	if err != nil {
		return metricError(c, err)
	}
	if start.After(end) {
		return metricError(c, fmt.Errorf("start must not be after end"))
	}
	result, err := qe.metricRange(selector, start, end, metricOrgID(c))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
	}
	return c.JSON(result)
}

func (qe *QueryEngine) handleMetricsInstant(c *fiber.Ctx) error {
	selector, err := parseMetricSelector(c.Query("query"))
	if err != nil {
		return metricError(c, err)
	}
	at, err := parseMetricTime(c.Query("time"), time.Now())
	if err != nil {
		return metricError(c, err)
	}
	result, err := qe.metricInstant(selector, at, metricOrgID(c))
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
	}
	return c.JSON(result)
}

func (qe *QueryEngine) handleMetricsLabels(c *fiber.Ctx) error {
	orgID := metricOrgID(c)
	where := "timestamp >= now() - INTERVAL 7 DAY"
	if orgID != "" {
		where += " AND JSONExtractString(labels, 'org') = " + quotedSQL(orgID)
	}
	rows, err := qe.nativeRows("SELECT name, service_id, labels, toUnixTimestamp(timestamp) AS timestamp, value FROM metrics WHERE "+where+" ORDER BY timestamp DESC LIMIT 10000 FORMAT JSON")
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
	}
	labels := map[string]bool{"__name__": true, "service_id": true}
	for _, row := range rows {
		var stored map[string]string
		if json.Unmarshal([]byte(row.Labels), &stored) == nil {
			for key := range stored {
				labels[key] = true
			}
		}
	}
	result := make([]string, 0, len(labels))
	for label := range labels {
		result = append(result, label)
	}
	sort.Strings(result)
	return c.JSON(fiber.Map{"status": "success", "data": result, "source": "observex_native"})
}

func (qe *QueryEngine) handleMetricsSeries(c *fiber.Ctx) error {
	raw := c.Query("match[]", c.Query("query", ""))
	if raw == "" {
		return metricError(c, fmt.Errorf("match[] must contain a native metric selector"))
	}
	selector, err := parseMetricSelector(raw)
	if err != nil {
		return metricError(c, err)
	}
	end := time.Now()
	start, err := parseMetricTime(c.Query("start"), end.Add(-time.Hour))
	if err != nil {
		return metricError(c, err)
	}
	if requestedEnd, err := parseMetricTime(c.Query("end"), end); err == nil {
		end = requestedEnd
	} else {
		return metricError(c, err)
	}
	if start.After(end) {
		return metricError(c, fmt.Errorf("start must not be after end"))
	}
	rows, err := qe.nativeRows("SELECT name, service_id, labels, toUnixTimestamp(max(timestamp)) AS timestamp, argMax(value, timestamp) AS value FROM metrics WHERE "+metricWhere(selector, start, end, metricOrgID(c))+" GROUP BY name, service_id, labels LIMIT 5000 FORMAT JSON")
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
	}
	result := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		_, labels := metricIdentity(row)
		result = append(result, labels)
	}
	return c.JSON(fiber.Map{"status": "success", "data": result, "source": "observex_native"})
}

// ─── Logs and traces ───────────────────────────────────────────────────────

func (qe *QueryEngine) handleLogsQuery(c *fiber.Ctx) error {
	params := url.Values{}
	params.Set("query", c.Query("query", "{service_id!=\"\"}"))
	params.Set("start", c.Query("start", fmt.Sprintf("%d", time.Now().Add(-time.Hour).UnixNano())))
	params.Set("end", c.Query("end", fmt.Sprintf("%d", time.Now().UnixNano())))
	params.Set("limit", c.Query("limit", "100"))
	params.Set("direction", c.Query("direction", "backward"))
	return qe.proxy(c, http.MethodGet, qe.cfg.LokiURL+"/loki/api/v1/query_range?"+params.Encode(), nil)
}

func (qe *QueryEngine) handleLogsLabels(c *fiber.Ctx) error {
	return qe.proxy(c, http.MethodGet, qe.cfg.LokiURL+"/loki/api/v1/labels", nil)
}

func (qe *QueryEngine) handleTracesSearch(c *fiber.Ctx) error {
	params := url.Values{}
	if query := c.Query("q"); query != "" {
		params.Set("q", query)
	}
	if service := c.Query("service"); service != "" {
		params.Set("tags", "service.name="+service)
	}
	params.Set("limit", c.Query("limit", "20"))
	if start := c.Query("start"); start != "" {
		params.Set("start", start)
	}
	if end := c.Query("end"); end != "" {
		params.Set("end", end)
	}
	return qe.proxy(c, http.MethodGet, qe.cfg.TempoURL+"/api/search?"+params.Encode(), nil)
}

func (qe *QueryEngine) handleTraceByID(c *fiber.Ctx) error {
	return qe.proxy(c, http.MethodGet, qe.cfg.TempoURL+"/api/traces/"+url.PathEscape(c.Params("id")), nil)
}

// ─── Unified dispatch ──────────────────────────────────────────────────────

func (qe *QueryEngine) handleUnifiedQuery(c *fiber.Ctx) error {
	var request QueryRequest
	if err := c.BodyParser(&request); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if request.Query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query required"})
	}
	switch strings.ToLower(request.Type) {
	case "metrics", "":
		selector, err := parseMetricSelector(request.Query)
		if err != nil {
			return metricError(c, err)
		}
		if request.Start != "" || request.End != "" || request.Step != "" {
			end, err := parseMetricTime(request.End, time.Now())
			if err != nil {
				return metricError(c, err)
			}
			start, err := parseMetricTime(request.Start, end.Add(-time.Hour))
			if err != nil {
				return metricError(c, err)
			}
			result, err := qe.metricRange(selector, start, end, metricOrgID(c))
			if err != nil {
				return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
			}
			return c.JSON(result)
		}
		result, err := qe.metricInstant(selector, time.Now(), metricOrgID(c))
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"status": "error", "error": "native metric store unavailable"})
		}
		return c.JSON(result)
	case "logs":
		params := url.Values{"query": {request.Query}, "direction": {"backward"}}
		if request.Start != "" {
			params.Set("start", request.Start)
		}
		if request.End != "" {
			params.Set("end", request.End)
		}
		if request.Limit > 0 {
			params.Set("limit", strconv.Itoa(request.Limit))
		}
		return qe.proxy(c, http.MethodGet, qe.cfg.LokiURL+"/loki/api/v1/query_range?"+params.Encode(), nil)
	case "traces":
		params := url.Values{"q": {request.Query}}
		if request.Limit > 0 {
			params.Set("limit", strconv.Itoa(request.Limit))
		}
		return qe.proxy(c, http.MethodGet, qe.cfg.TempoURL+"/api/search?"+params.Encode(), nil)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("unknown query type %q", request.Type)})
	}
}

// api/v1 compatibility is deliberately limited to native selector reads. This
// lets older dashboards migrate without retaining a foreign metrics backend.
func (qe *QueryEngine) handleCompatibility(c *fiber.Ctx) error {
	switch c.Path() {
	case "/api/v1/query":
		return qe.handleMetricsInstant(c)
	case "/api/v1/query_range":
		return qe.handleMetricsRange(c)
	case "/api/v1/labels":
		return qe.handleMetricsLabels(c)
	case "/api/v1/series":
		return qe.handleMetricsSeries(c)
	default:
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"status": "error", "error": "native metrics endpoint not found"})
	}
}

// newApp builds the HTTP application and registers every query-engine route.
// S1-04: no route or query type executes caller-supplied SQL.
func newApp(qe *QueryEngine) *fiber.App {
	app := fiber.New(fiber.Config{AppName: "observex-query-engine", ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second})
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*"}))
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "query-engine", "metric_store": "observex_native", "version": "2.5.0"})
	})
	app.Post("/query", qe.handleUnifiedQuery)
	app.Get("/query/range", qe.handleMetricsRange)
	app.Get("/query/instant", qe.handleMetricsInstant)
	app.Get("/query/labels", qe.handleMetricsLabels)
	app.Get("/query/series", qe.handleMetricsSeries)
	app.Get("/query/logs", qe.handleLogsQuery)
	app.Get("/query/logs/labels", qe.handleLogsLabels)
	app.Get("/query/traces", qe.handleTracesSearch)
	app.Get("/query/traces/:id", qe.handleTraceByID)
	app.All("/api/v1/*", qe.handleCompatibility)
	return app
}

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()
	cfg := loadConfig()
	qe := &QueryEngine{cfg: cfg, client: &http.Client{Timeout: 30 * time.Second}, log: log}
	app := newApp(qe)
	log.Info("query-engine listening", zap.String("port", cfg.Port), zap.String("metric_store", "ClickHouse native"), zap.String("loki", cfg.LokiURL), zap.String("tempo", cfg.TempoURL))
	log.Fatal("server error", zap.Error(app.Listen(":"+cfg.Port)))
}
