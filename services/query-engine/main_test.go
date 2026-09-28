package main

// S1-04 tests: the query engine has no route or branch that executes
// caller-supplied SQL. /query/events is removed, POST /query rejects the
// former "events" and "sql" types, and the remaining routes still reach only
// their fixed backends. Backends are local httptest stubs that record every
// request, so "nothing reached ClickHouse" is checked directly.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

type s104Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

type s104Backend struct {
	name string
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []s104Request
}

func newS104Backend(t *testing.T, name, response string) *s104Backend {
	t.Helper()
	b := &s104Backend{name: name}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.reqs = append(b.reqs, s104Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: string(body)})
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *s104Backend) requests() []s104Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]s104Request(nil), b.reqs...)
}

func (b *s104Backend) reset() {
	b.mu.Lock()
	b.reqs = nil
	b.mu.Unlock()
}

type s104Env struct {
	qe         *QueryEngine
	clickhouse *s104Backend
	loki       *s104Backend
	tempo      *s104Backend
}

func newS104Env(t *testing.T) *s104Env {
	t.Helper()
	env := &s104Env{
		clickhouse: newS104Backend(t, "clickhouse", `{"data":[]}`),
		loki:       newS104Backend(t, "loki", `{"status":"success","data":{"result":[]}}`),
		tempo:      newS104Backend(t, "tempo", `{"traces":[]}`),
	}
	env.qe = &QueryEngine{
		cfg: Config{
			Port:          "0",
			LokiURL:       env.loki.srv.URL,
			TempoURL:      env.tempo.srv.URL,
			ClickHouseURL: env.clickhouse.srv.URL,
		},
		client: &http.Client{Timeout: 5 * time.Second},
		log:    zap.NewNop(),
	}
	return env
}

func (env *s104Env) reset() {
	env.clickhouse.reset()
	env.loki.reset()
	env.tempo.reset()
}

func (env *s104Env) requireNoBackendRequests(t *testing.T) {
	t.Helper()
	for _, b := range []*s104Backend{env.clickhouse, env.loki, env.tempo} {
		if n := len(b.requests()); n != 0 {
			t.Fatalf("%s received %d request(s), want 0", b.name, n)
		}
	}
}

// do sends one request to a fresh app built by newApp and returns status and body.
func (env *s104Env) do(t *testing.T, method, target, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := newApp(env.qe).Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(respBody)
}

const s104MetricSelectPrefix = "SELECT name, service_id, labels, toUnixTimestamp(timestamp) AS timestamp, value FROM metrics WHERE name = 'up' AND "

func TestS104_RouteInventory(t *testing.T) {
	env := newS104Env(t)
	app := newApp(env.qe)

	// "/" entries are the recover and CORS middleware registered with app.Use.
	allowedPaths := map[string]bool{
		"/": true, "/health": true, "/query": true,
		"/query/range": true, "/query/instant": true, "/query/labels": true, "/query/series": true,
		"/query/logs": true, "/query/logs/labels": true, "/query/traces": true, "/query/traces/:id": true,
		"/api/v1/*": true,
	}
	// Only these paths may accept methods that carry a request body.
	bodyPaths := map[string]bool{"/": true, "/query": true, "/api/v1/*": true}

	seen := map[string]bool{}
	for _, methodRoutes := range app.Stack() {
		for _, route := range methodRoutes {
			lower := strings.ToLower(route.Path)
			if strings.Contains(lower, "events") || strings.Contains(lower, "sql") {
				t.Errorf("route %s %s must not exist", route.Method, route.Path)
			}
			if !allowedPaths[route.Path] {
				t.Errorf("unexpected route %s %s", route.Method, route.Path)
			}
			switch route.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				if !bodyPaths[route.Path] {
					t.Errorf("unexpected %s route on %s", route.Method, route.Path)
				}
			}
			seen[route.Method+" "+route.Path] = true
		}
	}

	required := []string{
		"GET /health", "POST /query",
		"GET /query/range", "GET /query/instant", "GET /query/labels", "GET /query/series",
		"GET /query/logs", "GET /query/logs/labels", "GET /query/traces", "GET /query/traces/:id",
		"GET /api/v1/*", "POST /api/v1/*",
	}
	for _, r := range required {
		if !seen[r] {
			t.Errorf("required route %s is missing", r)
		}
	}
}

func TestS104_QueryEventsRemoved(t *testing.T) {
	env := newS104Env(t)
	bodies := []string{"", `{}`, `{"sql":"SELECT 1"}`, `{"sql":"SELECT * FROM system.users","limit":10}`, `{"limit":50}`}
	methods := []string{http.MethodPost, http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		for _, body := range bodies {
			env.reset()
			status, _ := env.do(t, method, "/query/events", body)
			if status != http.StatusNotFound {
				t.Errorf("%s /query/events body %q: status %d, want 404", method, body, status)
			}
			env.requireNoBackendRequests(t)
		}
	}
}

func TestS104_UnifiedQuery_SQLTypesRejected(t *testing.T) {
	env := newS104Env(t)
	types := []string{"sql", "events", "SQL", "Events", "sQl", "EVENTS"}
	queries := []string{"SELECT 1", "DROP TABLE metrics", "SELECT * FROM system.users"}
	for _, typ := range types {
		for _, query := range queries {
			env.reset()
			body := `{"type":"` + typ + `","query":"` + query + `"}`
			status, respBody := env.do(t, http.MethodPost, "/query", body)
			if status != http.StatusBadRequest {
				t.Errorf("type %q query %q: status %d, want 400", typ, query, status)
			}
			if !strings.Contains(respBody, "unknown query type") {
				t.Errorf("type %q query %q: body %q, want unknown query type", typ, query, respBody)
			}
			env.requireNoBackendRequests(t)
		}
	}
}

func TestS104_UnifiedQuery_OtherTypesUnchanged(t *testing.T) {
	env := newS104Env(t)

	for _, typ := range []string{"", "metrics"} {
		t.Run("metrics_type_"+typ, func(t *testing.T) {
			env.reset()
			status, _ := env.do(t, http.MethodPost, "/query", `{"type":"`+typ+`","query":"up"}`)
			if status != http.StatusOK {
				t.Fatalf("status %d, want 200", status)
			}
			reqs := env.clickhouse.requests()
			if len(reqs) != 1 {
				t.Fatalf("clickhouse received %d request(s), want 1", len(reqs))
			}
			if reqs[0].Method != http.MethodPost || reqs[0].Query.Get("database") != "observex" {
				t.Fatalf("clickhouse request = %s %s ?%v, want POST with database=observex", reqs[0].Method, reqs[0].Path, reqs[0].Query)
			}
			if !strings.HasPrefix(reqs[0].Body, s104MetricSelectPrefix) || !strings.HasSuffix(reqs[0].Body, " FORMAT JSON") {
				t.Fatalf("clickhouse body %q is not the fixed metric statement", reqs[0].Body)
			}
			if n := len(env.loki.requests()) + len(env.tempo.requests()); n != 0 {
				t.Fatalf("loki/tempo received %d request(s), want 0", n)
			}
		})
	}

	t.Run("logs", func(t *testing.T) {
		env.reset()
		status, _ := env.do(t, http.MethodPost, "/query", `{"type":"logs","query":"{service_id=\"checkout\"}"}`)
		if status != http.StatusOK {
			t.Fatalf("status %d, want 200", status)
		}
		reqs := env.loki.requests()
		if len(reqs) != 1 || reqs[0].Path != "/loki/api/v1/query_range" || reqs[0].Query.Get("query") != `{service_id="checkout"}` {
			t.Fatalf("loki requests = %+v, want one query_range with the LogQL query", reqs)
		}
		if n := len(env.clickhouse.requests()) + len(env.tempo.requests()); n != 0 {
			t.Fatalf("clickhouse/tempo received %d request(s), want 0", n)
		}
	})

	t.Run("traces", func(t *testing.T) {
		env.reset()
		status, _ := env.do(t, http.MethodPost, "/query", `{"type":"traces","query":"checkout"}`)
		if status != http.StatusOK {
			t.Fatalf("status %d, want 200", status)
		}
		reqs := env.tempo.requests()
		if len(reqs) != 1 || reqs[0].Path != "/api/search" || reqs[0].Query.Get("q") != "checkout" {
			t.Fatalf("tempo requests = %+v, want one /api/search with q=checkout", reqs)
		}
		if n := len(env.clickhouse.requests()) + len(env.loki.requests()); n != 0 {
			t.Fatalf("clickhouse/loki received %d request(s), want 0", n)
		}
	})

	t.Run("unknown_type", func(t *testing.T) {
		env.reset()
		status, _ := env.do(t, http.MethodPost, "/query", `{"type":"foo","query":"x"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("status %d, want 400", status)
		}
		env.requireNoBackendRequests(t)
	})
}

func TestS104_NativeMetricRoutesUnchanged(t *testing.T) {
	env := newS104Env(t)

	for _, target := range []string{"/query/instant?query=up", "/query/range?query=up", "/api/v1/query?query=up", "/api/v1/query_range?query=up"} {
		t.Run(target, func(t *testing.T) {
			env.reset()
			status, _ := env.do(t, http.MethodGet, target, "")
			if status != http.StatusOK {
				t.Fatalf("status %d, want 200", status)
			}
			reqs := env.clickhouse.requests()
			if len(reqs) != 1 || !strings.HasPrefix(reqs[0].Body, s104MetricSelectPrefix) {
				t.Fatalf("clickhouse requests = %+v, want one fixed metric statement", reqs)
			}
		})
	}

	t.Run("health", func(t *testing.T) {
		env.reset()
		status, body := env.do(t, http.MethodGet, "/health", "")
		if status != http.StatusOK || !strings.Contains(body, `"query-engine"`) {
			t.Fatalf("health = %d %q, want 200 with service query-engine", status, body)
		}
		env.requireNoBackendRequests(t)
	})

	t.Run("unknown_compat_path", func(t *testing.T) {
		env.reset()
		status, _ := env.do(t, http.MethodGet, "/api/v1/unknown", "")
		if status != http.StatusNotFound {
			t.Fatalf("status %d, want 404", status)
		}
		env.requireNoBackendRequests(t)
	})
}
