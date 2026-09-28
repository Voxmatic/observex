package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/observex/platform/pkg/models"
)

// chLiterals splits a ClickHouse statement into its single-quoted string
// literals and a skeleton in which each literal is replaced by "?".
//
// Inside a literal, a backslash escapes the next character and a doubled
// quote (two single quotes) is a literal quote, following ClickHouse string
// literal rules.
// Any quote that ends a literal early changes the skeleton or the literal
// count, which is how the tests detect a breakout.
func chLiterals(stmt string) (lits []string, skeleton string, err error) {
	var sk, lit strings.Builder
	inLit := false
	for i := 0; i < len(stmt); i++ {
		c := stmt[i]
		if !inLit {
			if c == '\'' {
				inLit = true
				lit.Reset()
				sk.WriteByte('?')
				continue
			}
			sk.WriteByte(c)
			continue
		}
		switch {
		case c == '\\':
			if i+1 >= len(stmt) {
				return nil, "", fmt.Errorf("dangling backslash at end of statement")
			}
			i++
			switch stmt[i] {
			case 'n':
				lit.WriteByte('\n')
			case 't':
				lit.WriteByte('\t')
			case 'r':
				lit.WriteByte('\r')
			case '0':
				lit.WriteByte(0)
			default:
				lit.WriteByte(stmt[i])
			}
		case c == '\'' && i+1 < len(stmt) && stmt[i+1] == '\'':
			lit.WriteByte('\'')
			i++
		case c == '\'':
			inLit = false
			lits = append(lits, lit.String())
		default:
			lit.WriteByte(c)
		}
	}
	if inLit {
		return nil, "", fmt.Errorf("unterminated string literal")
	}
	return lits, sk.String(), nil
}

func TestChLiterals_DetectsQuoteOnlyEscapingBreakout(t *testing.T) {
	// The previous esc() escaped quotes but not backslashes. Confirm the
	// checker reports the resulting breakout, so passing tests below are
	// meaningful.
	quoteOnly := func(s string) string { return strings.ReplaceAll(s, "'", "\\'") }
	payload := `\'); DROP TABLE metrics; --`

	lits, skeleton, err := chLiterals("'" + quoteOnly(payload) + "'")
	if err == nil && len(lits) == 1 && skeleton == "?" && lits[0] == payload {
		t.Fatalf("checker did not detect breakout: lits=%q skeleton=%q", lits, skeleton)
	}
}

func TestEsc_Vectors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"NormalValue", "http_requests_total", "http_requests_total"},
		{"Empty", "", ""},
		{"SingleQuote", "O'Brien", `O\'Brien`},
		{"DoubledQuote", "''", `\'\'`},
		{"Backslash", `C:\temp`, `C:\\temp`},
		{"TrailingBackslash", `abc\`, `abc\\`},
		{"BackslashQuote", `\'`, `\\\'`},
		{"DoubleBackslashQuote", `\\'`, `\\\\\'`},
		{"SQLInjection", `'); DROP TABLE metrics; --`, `\'); DROP TABLE metrics; --`},
		{"SQLInjectionWithBackslash", `\'); DROP TABLE metrics; --`, `\\\'); DROP TABLE metrics; --`},
		{"Unicode", "héllo ✓", "héllo ✓"},
		{"NewlineUnchanged", "a\nb", "a\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := esc(tt.in)
			if got != tt.want {
				t.Fatalf("esc(%q) = %q, want %q", tt.in, got, tt.want)
			}
			lits, skeleton, err := chLiterals("'" + got + "'")
			if err != nil {
				t.Fatalf("literal did not parse: %v", err)
			}
			if skeleton != "?" || len(lits) != 1 {
				t.Fatalf("literal broke out: skeleton=%q literals=%q", skeleton, lits)
			}
			if lits[0] != tt.in {
				t.Fatalf("decoded literal = %q, want original %q", lits[0], tt.in)
			}
		})
	}
}

// captureTransport records the SQL sent to ClickHouse without any network
// access. It returns 200 OK so the write path completes normally.
type captureTransport struct {
	mu      sync.Mutex
	queries []string
	errs    []string
}

func (c *captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Fragment != "" {
		c.errs = append(c.errs, "query truncated by URL fragment: "+r.URL.Fragment)
	}
	if !strings.HasPrefix(r.URL.RawQuery, "query=") {
		c.errs = append(c.errs, "unexpected raw query: "+r.URL.RawQuery)
	}
	c.queries = append(c.queries, strings.TrimPrefix(r.URL.RawQuery, "query="))
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

func (c *captureTransport) only(t *testing.T) string {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.errs) > 0 {
		t.Fatalf("transport errors: %q", c.errs)
	}
	if len(c.queries) != 1 {
		t.Fatalf("got %d ClickHouse queries, want 1: %q", len(c.queries), c.queries)
	}
	return c.queries[0]
}

func newCaptureIngestor() (*Ingestor, *captureTransport) {
	ct := &captureTransport{}
	ing := &Ingestor{
		cfg:    Config{ClickHouseURL: "http://clickhouse.test:8123"},
		logger: zap.NewNop(),
		client: &http.Client{Transport: ct},
	}
	return ing, ct
}

func assertLiterals(t *testing.T, stmt, wantSkeleton string, want []string) {
	t.Helper()
	lits, skeleton, err := chLiterals(stmt)
	if err != nil {
		t.Fatalf("statement did not parse: %v\n%s", err, stmt)
	}
	if skeleton != wantSkeleton {
		t.Fatalf("statement structure changed (possible breakout)\n got: %s\nwant: %s", skeleton, wantSkeleton)
	}
	if len(lits) != len(want) {
		t.Fatalf("got %d literals, want %d: %q", len(lits), len(want), lits)
	}
	for i := range want {
		if lits[i] != want[i] {
			t.Fatalf("literal %d = %q, want %q", i, lits[i], want[i])
		}
	}
}

var (
	testTime  = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	testStamp = "2026-01-02 03:04:05"

	// URL-safe payloads only: '#', '&', '%' and control characters are
	// affected by the separate URL transport issue (P-4 / S1-08b), which this
	// change does not address.
	injectionPayloads = []string{
		`'); DROP TABLE metrics; --`,
		`\'); DROP TABLE metrics; --`,
		`\\'); DROP TABLE metrics; --`,
		`O'Brien`,
		`abc\`,
		``,
	}
)

func TestWriteMetricsToClickHouse_NormalInputUnchanged(t *testing.T) {
	ing, ct := newCaptureIngestor()
	ing.writeMetricsToClickHouse([]models.MetricPoint{{
		Name:      "http_requests_total",
		Value:     1.5,
		ServiceID: "svc-1",
		Labels:    map[string]string{"method": "GET"},
		Timestamp: testTime,
	}})

	want := `INSERT INTO metrics (name, value, service_id, labels, timestamp) VALUES ('http_requests_total',1.5,'svc-1','{"method":"GET"}','2026-01-02 03:04:05')`
	if got := ct.only(t); got != want {
		t.Fatalf("statement changed\n got: %s\nwant: %s", got, want)
	}
}

func TestWriteMetricsToClickHouse_InjectionStaysInLiterals(t *testing.T) {
	for _, p := range injectionPayloads {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			labels := map[string]string{"k" + p: "v" + p}
			ing, ct := newCaptureIngestor()
			ing.writeMetricsToClickHouse([]models.MetricPoint{{
				Name:      p,
				Value:     1.5,
				ServiceID: "svc" + p,
				Labels:    labels,
				Timestamp: testTime,
			}})

			labelJSON, err := json.Marshal(labels)
			if err != nil {
				t.Fatal(err)
			}
			assertLiterals(t, ct.only(t),
				"INSERT INTO metrics (name, value, service_id, labels, timestamp) VALUES (?,1.5,?,?,?)",
				[]string{p, "svc" + p, string(labelJSON), testStamp})
		})
	}
}

func TestWriteServiceToClickHouse_InjectionStaysInLiterals(t *testing.T) {
	for _, p := range injectionPayloads {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			ing, ct := newCaptureIngestor()
			ing.writeServiceToClickHouse(&models.Service{
				ID:           "id" + p,
				Name:         "name" + p,
				Kind:         models.ServiceKind("kind" + p),
				Namespace:    "ns" + p,
				ClusterName:  "cluster" + p,
				NodeName:     "node" + p,
				Health:       models.Health{State: models.HealthState("state" + p), Score: 97.5},
				AgentID:      "agent" + p,
				DiscoveredAt: testTime,
				LastSeenAt:   testTime,
			})

			assertLiterals(t, ct.only(t),
				"INSERT INTO services (id, name, kind, namespace, cluster, node, health_state, health_score, agent_id, discovered_at, last_seen_at) VALUES (?,?,?,?,?,?,?,97.5,?,?,?)",
				[]string{"id" + p, "name" + p, "kind" + p, "ns" + p, "cluster" + p, "node" + p, "state" + p, "agent" + p, testStamp, testStamp})
		})
	}
}

func TestWriteEdgeToClickHouse_InjectionStaysInLiterals(t *testing.T) {
	for _, p := range injectionPayloads {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			ing, ct := newCaptureIngestor()
			ing.writeEdgeToClickHouse(&models.TopoEdge{
				ID:           "edge" + p,
				SourceID:     "src" + p,
				TargetID:     "dst" + p,
				Protocol:     "http" + p,
				CallsPerMin:  12,
				AvgLatencyMs: 3.5,
				ErrorRate:    0.25,
				UpdatedAt:    testTime,
			})

			assertLiterals(t, ct.only(t),
				"INSERT INTO topology_edges (id, source_id, target_id, protocol, calls_per_min, avg_latency_ms, error_rate, updated_at) VALUES (?,?,?,?,12,3.5,0.25,?)",
				[]string{"edge" + p, "src" + p, "dst" + p, "http" + p, testStamp})
		})
	}
}

func TestWriteProfileToClickHouse_InjectionStaysInLiterals(t *testing.T) {
	for _, p := range injectionPayloads {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			ing, ct := newCaptureIngestor()
			ing.writeProfileToClickHouse(&models.Profile{
				ServiceID:   "svc" + p,
				ProfileType: "cpu" + p,
				StartTime:   testTime,
				DurationSec: 30,
			})

			assertLiterals(t, ct.only(t),
				"INSERT INTO profiles (service_id, profile_type, start_time, duration_sec) VALUES (?,?,?,30)",
				[]string{"svc" + p, "cpu" + p, testStamp})
		})
	}
}
