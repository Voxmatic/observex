package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// nativeMetricRow is the narrow shape returned from ObserveX's ClickHouse
// metric table. Queries in this package are assembled from fixed clauses only.
type nativeMetricRow struct {
	Name      string  `json:"name"`
	ServiceID string  `json:"service_id"`
	Labels    string  `json:"labels"`
	Node      string  `json:"node"`
	Endpoint  string  `json:"endpoint"`
	ServiceName string `json:"service_name"`
	Namespace string `json:"namespace"` // GO-7: selected as "namespace" by the APM services query
	Outcome   string  `json:"outcome"`
	Requests  float64 `json:"requests"`
	Errors    float64 `json:"errors"`
	P50       float64 `json:"p50"`
	P99       float64 `json:"p99"`
	Satisfied float64 `json:"satisfied"`
	Tolerating float64 `json:"tolerating"`
	Durations float64 `json:"durations"`
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

type nativeMetricSample struct {
	Timestamp int64
	Value     float64
}

func nativeMetricQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func nativeMetricLabel(name string) string {
	return "JSONExtractString(labels, " + nativeMetricQuote(name) + ")"
}

func nativeMetricWindow(raw string, fallback, maximum time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func nativeMetricWhereClause(name, orgID string, start, end time.Time, labels map[string]string) string {
	conditions := []string{"name = " + nativeMetricQuote(name)}
	if !start.IsZero() {
		conditions = append(conditions, fmt.Sprintf("timestamp >= toDateTime(%d)", start.Unix()))
	}
	if !end.IsZero() {
		conditions = append(conditions, fmt.Sprintf("timestamp <= toDateTime(%d)", end.Unix()))
	}
	if orgID != "" {
		conditions = append(conditions, nativeMetricLabel("org")+" = "+nativeMetricQuote(orgID))
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		conditions = append(conditions, nativeMetricLabel(key)+" = "+nativeMetricQuote(labels[key]))
	}
	return strings.Join(conditions, " AND ")
}

func (gw *Gateway) nativeMetricRows(ctx context.Context, query string) ([]nativeMetricRow, error) {
	target := strings.TrimRight(gw.cfg.ClickHouseURL, "/") + "/?database=observex"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewBufferString(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := gw.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("native metric store returned %d", resp.StatusCode)
	}
	var result struct {
		Data []nativeMetricRow `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (gw *Gateway) nativeMetricSamples(ctx context.Context, name, orgID string, start, end time.Time, labels map[string]string, aggregate bool) ([]nativeMetricSample, error) {
	where := nativeMetricWhereClause(name, orgID, start, end, labels)
	selectClause := "toUnixTimestamp(timestamp) AS timestamp, value"
	groupClause := ""
	if aggregate {
		selectClause = "toUnixTimestamp(timestamp) AS timestamp, sum(value) AS value"
		groupClause = " GROUP BY timestamp"
	}
	rows, err := gw.nativeMetricRows(ctx, "SELECT "+selectClause+" FROM metrics WHERE "+where+groupClause+" ORDER BY timestamp ASC LIMIT 10000 FORMAT JSON")
	if err != nil {
		return nil, err
	}
	samples := make([]nativeMetricSample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, nativeMetricSample{Timestamp: row.Timestamp, Value: row.Value})
	}
	return samples, nil
}

func (gw *Gateway) nativeNodeValues(ctx context.Context, name, orgID string, window time.Duration, rate bool) (map[string]float64, error) {
	end := time.Now()
	start := end.Add(-window)
	where := nativeMetricWhereClause(name, orgID, start, end, nil)
	node := nativeMetricLabel("node")
	var query string
	if rate {
		query = "SELECT node, sum(greatest(last_value - first_value, 0)) / " + strconv.FormatFloat(window.Seconds(), 'f', -1, 64) + " AS value FROM (" +
			"SELECT " + node + " AS node, labels, max(value) AS last_value, min(value) AS first_value FROM metrics WHERE " + where + " GROUP BY node, labels" +
			") GROUP BY node FORMAT JSON"
	} else {
		query = "SELECT node, sum(value) AS value FROM (" +
			"SELECT " + node + " AS node, labels, argMax(value, timestamp) AS value FROM metrics WHERE " + where + " GROUP BY node, labels" +
			") GROUP BY node FORMAT JSON"
	}
	rows, err := gw.nativeMetricRows(ctx, query)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64, len(rows))
	for _, row := range rows {
		if row.Node != "" {
			values[row.Node] = row.Value
		}
	}
	return values, nil
}

func nativeMetricMatrix(name string, labels map[string]string, samples []nativeMetricSample) fiber.Map {
	values := make([][]interface{}, 0, len(samples))
	for _, sample := range samples {
		values = append(values, []interface{}{sample.Timestamp, strconv.FormatFloat(sample.Value, 'f', -1, 64)})
	}
	return fiber.Map{
		"status": "success",
		"data": fiber.Map{
			"resultType": "matrix",
			"result": []fiber.Map{{
				"metric": func() map[string]string {
					out := map[string]string{"__name__": name}
					for key, value := range labels {
						out[key] = value
					}
					return out
				}(),
				"values": values,
			}},
			"source": "observex_native",
		},
	}
}

func nativeLatestSamples(samples []nativeMetricSample) nativeMetricSample {
	if len(samples) == 0 {
		return nativeMetricSample{}
	}
	return samples[len(samples)-1]
}

func nativePercentageSeries(total, available []nativeMetricSample) []nativeMetricSample {
	availableByTime := make(map[int64]float64, len(available))
	for _, sample := range available {
		availableByTime[sample.Timestamp] = sample.Value
	}
	result := make([]nativeMetricSample, 0, len(total))
	for _, sample := range total {
		free, ok := availableByTime[sample.Timestamp]
		if !ok || sample.Value <= 0 {
			continue
		}
		result = append(result, nativeMetricSample{Timestamp: sample.Timestamp, Value: (1 - free/sample.Value) * 100})
	}
	return result
}

func nativeCPUUsageSeries(total, idle []nativeMetricSample) []nativeMetricSample {
	idleByTime := make(map[int64]float64, len(idle))
	for _, sample := range idle {
		idleByTime[sample.Timestamp] = sample.Value
	}
	result := make([]nativeMetricSample, 0, len(total))
	var previousTotal, previousIdle float64
	var hasPrevious bool
	for _, sample := range total {
		idleValue, ok := idleByTime[sample.Timestamp]
		if !ok {
			continue
		}
		if hasPrevious {
			deltaTotal := sample.Value - previousTotal
			deltaIdle := idleValue - previousIdle
			if deltaTotal > 0 {
				result = append(result, nativeMetricSample{Timestamp: sample.Timestamp, Value: (1 - deltaIdle/deltaTotal) * 100})
			}
		}
		previousTotal, previousIdle, hasPrevious = sample.Value, idleValue, true
	}
	return result
}

func nativeCounterRateSeries(samples []nativeMetricSample, scale float64) []nativeMetricSample {
	result := make([]nativeMetricSample, 0, len(samples))
	for index := 1; index < len(samples); index++ {
		elapsed := samples[index].Timestamp - samples[index-1].Timestamp
		delta := samples[index].Value - samples[index-1].Value
		if elapsed <= 0 || delta < 0 {
			continue
		}
		result = append(result, nativeMetricSample{Timestamp: samples[index].Timestamp, Value: delta / float64(elapsed) * scale})
	}
	return result
}
