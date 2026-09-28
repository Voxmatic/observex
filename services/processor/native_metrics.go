package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// nativeMetricStore is a narrow ClickHouse reader shared by processor modules.
// It accepts only SQL assembled from fixed metric names, time bounds, and
// escaped identifiers; untrusted query text never reaches ClickHouse here.
type nativeMetricStore struct {
	endpoint string
	client   *http.Client
}

func newNativeMetricStore(p *Processor) nativeMetricStore {
	return nativeMetricStore{
		endpoint: strings.TrimRight(envOr("CLICKHOUSE_URL", "http://clickhouse:8123"), "/"),
		client:   p.client,
	}
}

func nativeMetricQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (s nativeMetricStore) scalar(ctx context.Context, query string) (float64, uint64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/?database=observex", strings.NewReader(query))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, 0, fmt.Errorf("native metric store returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Data []struct {
			Value   float64 `json:"value"`
			Samples uint64  `json:"samples"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return 0, 0, err
	}
	if len(result.Data) == 0 {
		return 0, 0, nil
	}
	return result.Data[0].Value, result.Data[0].Samples, nil
}

func nativeMetricTimeFilter(start, end time.Time) string {
	return fmt.Sprintf("timestamp >= toDateTime(%d) AND timestamp <= toDateTime(%d)", start.Unix(), end.Unix())
}

func nativeServiceFilter(serviceID, serviceName string) string {
	filters := make([]string, 0, 2)
	if serviceID != "" {
		filters = append(filters, "service_id = "+nativeMetricQuote(serviceID))
	}
	if serviceName != "" {
		filters = append(filters, "JSONExtractString(labels, 'service_name') = "+nativeMetricQuote(serviceName))
	}
	if len(filters) == 0 {
		return "1"
	}
	return "(" + strings.Join(filters, " OR ") + ")"
}

func (s nativeMetricStore) durationP99(ctx context.Context, serviceID, serviceName string, start, end time.Time) (float64, int, error) {
	query := fmt.Sprintf(
		"SELECT quantileTDigest(0.99)(value) AS value, count() AS samples FROM metrics WHERE name = 'http_server_duration_ms' AND %s AND %s FORMAT JSON",
		nativeMetricTimeFilter(start, end), nativeServiceFilter(serviceID, serviceName),
	)
	value, samples, err := s.scalar(ctx, query)
	return value, int(samples), err
}

func (s nativeMetricStore) requestErrorRate(ctx context.Context, serviceID, serviceName string, start, end time.Time) (float64, int, error) {
	query := fmt.Sprintf(
		"SELECT if(sum(value) = 0, 0, 100 * sumIf(value, JSONExtractString(labels, 'outcome') = 'error') / sum(value)) AS value, count() AS samples FROM metrics WHERE name = 'http_requests_total' AND %s AND %s FORMAT JSON",
		nativeMetricTimeFilter(start, end), nativeServiceFilter(serviceID, serviceName),
	)
	value, samples, err := s.scalar(ctx, query)
	return value, int(samples), err
}

// eventRatePerMinute sums event-valued measurements over a bounded window.
// ObserveX writes LLM token usage as one value per request, rather than as a
// process-global mutable counter, so max-minus-min would undercount it.
func (s nativeMetricStore) eventRatePerMinute(ctx context.Context, name, labelName, labelValue string, window time.Duration) (float64, error) {
	if window <= 0 {
		window = 5 * time.Minute
	}
	start := time.Now().Add(-window)
	query := fmt.Sprintf(
		"SELECT sum(value) / %.6f AS value, count() AS samples FROM metrics WHERE name = %s AND %s AND JSONExtractString(labels, %s) = %s FORMAT JSON",
		window.Minutes(), nativeMetricQuote(name), nativeMetricTimeFilter(start, time.Now()), nativeMetricQuote(labelName), nativeMetricQuote(labelValue),
	)
	value, _, err := s.scalar(ctx, query)
	return value, err
}
