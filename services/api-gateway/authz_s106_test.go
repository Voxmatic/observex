package main

// S1-06 authorization tests: audited routes R1–R11, org scoping for
// /llm/ingest and /agents, fail-closed /remediations, comment delete errors,
// and internal token logging. Token and JWT values are never printed.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	dbmodels "github.com/observex/platform/internal/db/models"
	"github.com/observex/platform/internal/db/store"
	"github.com/observex/platform/internal/middleware"
	"github.com/observex/platform/internal/servicetoken"
	"github.com/observex/platform/pkg/models"
)

var s106JWTSecret = strings.Repeat("j", 40) + "-s106-test-jwt-secret"

// s106Users are the test identities returned by the AuthResolver seam.
var s106Users = map[string]store.AuthContext{
	"viewer-a":     {UserID: "viewer-a", Email: "viewer@a.test", UserEmail: "viewer@a.test", OrgID: "org-a", Role: dbmodels.RoleViewer, EffectiveRole: dbmodels.RoleViewer},
	"editor-a":     {UserID: "editor-a", Email: "editor@a.test", UserEmail: "editor@a.test", OrgID: "org-a", Role: dbmodels.RoleEditor, EffectiveRole: dbmodels.RoleEditor},
	"admin-a":      {UserID: "admin-a", Email: "admin@a.test", UserEmail: "admin@a.test", OrgID: "org-a", Role: dbmodels.RoleAdmin, EffectiveRole: dbmodels.RoleAdmin, IsOrgAdmin: true},
	"viewer-b":     {UserID: "viewer-b", Email: "viewer@b.test", UserEmail: "viewer@b.test", OrgID: "org-b", Role: dbmodels.RoleViewer, EffectiveRole: dbmodels.RoleViewer},
	"editor-b":     {UserID: "editor-b", Email: "editor@b.test", UserEmail: "editor@b.test", OrgID: "org-b", Role: dbmodels.RoleEditor, EffectiveRole: dbmodels.RoleEditor},
	"editor-noorg": {UserID: "editor-noorg", Email: "noorg@x.test", UserEmail: "noorg@x.test", OrgID: "", Role: dbmodels.RoleEditor, EffectiveRole: dbmodels.RoleEditor},
}

func s106Resolver(_ context.Context, userID string) (*store.AuthContext, error) {
	u, ok := s106Users[userID]
	if !ok {
		return nil, errors.New("unknown test user")
	}
	return &u, nil
}

func s106JWT(t *testing.T, userID string) string {
	t.Helper()
	claims := &middleware.Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s106JWTSecret))
	if err != nil {
		t.Fatal("failed to sign test JWT")
	}
	return signed
}

// newS106Gateway registers the real routes on a Gateway with no database. The
// AuthResolver seam supplies identities; handlers that reach a nil store panic
// and are turned into 500 by the recover middleware, which is fine for
// authorization checks.
func newS106Gateway(t *testing.T, ingestorURL string) (*Gateway, *fiber.App) {
	t.Helper()
	if ingestorURL == "" {
		ingestorURL = "http://ingestor.invalid:4318"
	}
	mw := middleware.New(middleware.Config{
		JWTSecret:    s106JWTSecret,
		Logger:       zap.NewNop(),
		AuthResolver: s106Resolver,
	})
	gw := &Gateway{
		cfg: Config{
			IngestorURL:    ingestorURL,
			QueryEngineURL: "http://query-engine.invalid:9090",
			AIAgentURL:     "http://ai-agent.invalid:8080",
			ProcessorURL:   "http://processor.invalid:8080",
		},
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 5 * time.Second},
		hub:    NewWSHub(),
		mw:     mw,
	}
	gw.hub.SetLogger(zap.NewNop())
	app := fiber.New()
	app.Use(recover.New())
	gw.registerRoutes(app)
	t.Cleanup(func() { _ = app.Shutdown() })
	return gw, app
}

func s106Do(t *testing.T, app *fiber.App, method, path, userID string, body []byte, headers map[string]string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userID != "" {
		req.Header.Set("Authorization", "Bearer "+s106JWT(t, userID))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: app.Test: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

type s106Route struct {
	id, method, path string
	adminOnly        bool
}

// Audited routes R1–R10 (R11 /ws is covered in ws_auth_test.go).
var s106Routes = []s106Route{
	{"R1", "POST", "/api/v1/postmortems", false},
	{"R2", "PUT", "/api/v1/postmortems/pm-1", false},
	{"R3", "DELETE", "/api/v1/postmortems/pm-1", false},
	{"R4", "POST", "/api/v1/integrations", true},
	{"R5", "DELETE", "/api/v1/integrations/int-1", true},
	{"R6", "POST", "/api/v1/integrations/int-1/test", true},
	{"R7", "DELETE", "/api/v1/problems/p-1/comments/c-1", false},
	{"R8", "POST", "/api/v1/llm/ingest", false},
	{"R9", "GET", "/api/v1/remediations", false},
	{"R10", "GET", "/api/v1/agents", false},
}

const (
	bodyEditorRequired = `{"error":"editor role required"}`
	bodyAdminRequired  = `{"error":"admin required"}`
)

func TestS106_AuditedRoutes_Unauthenticated401(t *testing.T) {
	_, app := newS106Gateway(t, "")
	for _, r := range append(s106Routes, s106Route{"R11", "GET", "/ws", false}) {
		t.Run(r.id, func(t *testing.T) {
			if status, _ := s106Do(t, app, r.method, r.path, "", nil, nil); status != fiber.StatusUnauthorized {
				t.Fatalf("no credentials: status = %d, want 401", status)
			}
			bad := map[string]string{"Authorization": "Bearer not-a-valid-jwt"}
			if status, _ := s106Do(t, app, r.method, r.path, "", nil, bad); status != fiber.StatusUnauthorized {
				t.Fatalf("invalid JWT: status = %d, want 401", status)
			}
		})
	}
}

func TestS106_AuditedRoutes_ViewerForbidden(t *testing.T) {
	_, app := newS106Gateway(t, "")
	for _, r := range s106Routes {
		t.Run(r.id, func(t *testing.T) {
			status, body := s106Do(t, app, r.method, r.path, "viewer-a", nil, nil)
			want := bodyEditorRequired
			if r.adminOnly {
				want = bodyAdminRequired
			}
			if status != fiber.StatusForbidden || body != want {
				t.Fatalf("viewer: status = %d body = %s, want 403 %s", status, body, want)
			}
		})
	}
}

func TestS106_AdminOnlyRoutes_EditorForbidden(t *testing.T) {
	_, app := newS106Gateway(t, "")
	for _, r := range s106Routes {
		if !r.adminOnly {
			continue
		}
		t.Run(r.id, func(t *testing.T) {
			status, body := s106Do(t, app, r.method, r.path, "editor-a", nil, nil)
			if status != fiber.StatusForbidden || body != bodyAdminRequired {
				t.Fatalf("editor: status = %d body = %s, want 403 %s", status, body, bodyAdminRequired)
			}
		})
	}
}

func TestS106_AuditedRoutes_AllowedRolesPassRoleCheck(t *testing.T) {
	ingestor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agents":[],"total":0}`))
	}))
	defer ingestor.Close()
	_, app := newS106Gateway(t, ingestor.URL)

	for _, r := range s106Routes {
		roles := []string{"editor-a", "admin-a"}
		if r.adminOnly {
			roles = []string{"admin-a"}
		}
		for _, role := range roles {
			t.Run(r.id+"_"+role, func(t *testing.T) {
				status, body := s106Do(t, app, r.method, r.path, role, nil, nil)
				if status == fiber.StatusUnauthorized || body == bodyEditorRequired || body == bodyAdminRequired {
					t.Fatalf("%s blocked by authentication/role check: status = %d body = %s", role, status, body)
				}
				if r.id == "R9" {
					want := `{"error":"remediation data is not organization-scoped"}`
					if status != fiber.StatusForbidden || body != want {
						t.Fatalf("R9 %s: status = %d body = %s, want 403 %s", role, status, body, want)
					}
				}
			})
		}
	}
}

// Routes next to the audited ones keep their existing middleware.
func TestS106_UnchangedNeighbourRoutes_ViewerNotBlockedByRole(t *testing.T) {
	_, app := newS106Gateway(t, "")
	for _, path := range []string{"/api/v1/postmortems", "/api/v1/integrations"} {
		t.Run(path, func(t *testing.T) {
			status, body := s106Do(t, app, "GET", path, "viewer-a", nil, nil)
			if status == fiber.StatusUnauthorized || status == fiber.StatusForbidden {
				t.Fatalf("viewer GET %s: status = %d body = %s; read route should be unchanged", path, status, body)
			}
		})
	}
}

func TestS106_RemediationsApproveRejectUnchanged(t *testing.T) {
	_, app := newS106Gateway(t, "")
	for _, path := range []string{"/api/v1/remediations/r-1/approve", "/api/v1/remediations/r-1/reject"} {
		status, body := s106Do(t, app, "POST", path, "viewer-a", nil, nil)
		if status != fiber.StatusForbidden || body != bodyEditorRequired {
			t.Fatalf("viewer POST %s: status = %d body = %s; existing editor check should be unchanged", path, status, body)
		}
	}
}

// ── R8 /llm/ingest org scoping ───────────────────────────────────────────────

func TestS106_LLMIngest_IgnoresBodyOrgID(t *testing.T) {
	var (
		mu     sync.Mutex
		points []models.MetricPoint
	)
	ingestor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/metrics/batch" {
			var batch []models.MetricPoint
			_ = json.NewDecoder(r.Body).Decode(&batch)
			mu.Lock()
			points = append(points, batch...)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ingestor.Close()
	gw, app := newS106Gateway(t, ingestor.URL)
	go gw.hub.Run()

	clientA := &wsClient{orgID: "org-a", send: make(chan []byte, 8), topics: map[string]bool{}}
	clientB := &wsClient{orgID: "org-b", send: make(chan []byte, 8), topics: map[string]bool{}}
	gw.hub.register <- clientA
	gw.hub.register <- clientB
	waitForClients(t, gw.hub, 2)

	body := []byte(`{"provider":"anthropic","model":"test-model","input_tokens":3,"output_tokens":4,"status_code":500,"org_id":"org-b","error":"boom"}`)
	status, resp := s106Do(t, app, "POST", "/api/v1/llm/ingest", "editor-a", body, nil)
	if status != fiber.StatusAccepted {
		t.Fatalf("status = %d body = %s, want 202", status, resp)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(points) == 0 {
		t.Fatal("no metrics were written")
	}
	for _, p := range points {
		if p.Labels["org"] != "org-a" {
			t.Fatalf("metric %s written with org label %q, want org-a", p.Name, p.Labels["org"])
		}
	}

	select {
	case msg := <-clientA.send:
		var ev wsEvent
		if err := json.Unmarshal(msg, &ev); err != nil || ev.Type != "llm_error" || ev.OrgID != "org-a" {
			t.Fatalf("org-a client got unexpected event %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("org-a client did not receive llm_error")
	}
	select {
	case msg := <-clientB.send:
		t.Fatalf("org-b client received another org's event: %s", msg)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestS106_LLMIngest_NoOrgForbidden(t *testing.T) {
	_, app := newS106Gateway(t, "")
	status, body := s106Do(t, app, "POST", "/api/v1/llm/ingest", "editor-noorg", []byte(`{"provider":"x","org_id":"org-a"}`), nil)
	if status != fiber.StatusForbidden || body != `{"error":"organization required"}` {
		t.Fatalf("status = %d body = %s, want 403 organization required", status, body)
	}
}

// ── R10 /agents org filtering ────────────────────────────────────────────────

func TestS106_Agents_FilteredToCallerOrg(t *testing.T) {
	var sawInternalHeader bool
	ingestor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(servicetoken.Header) != "" {
			sawInternalHeader = true
		}
		if r.URL.Path != "/internal/agents" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agents":[` +
			`{"id":"a1","org_id":"org-a","node_name":"node-a1","status":"online"},` +
			`{"id":"b1","org_id":"org-b","node_name":"node-b1"},` +
			`{"id":"x1","node_name":"node-no-org"},` +
			`{"id":"a2","org_id":"org-a","node_name":"node-a2"}` +
			`],"total":4}`))
	}))
	defer ingestor.Close()
	gw, app := newS106Gateway(t, ingestor.URL)
	// Same client wiring as main(): the token transport targets the query engine only.
	token := servicetoken.LoadFrom(
		func(k string) (string, bool) {
			if k == servicetoken.EnvVar {
				return strings.Repeat("g", servicetoken.MinLength) + "-gateway-test", true
			}
			return "", false
		},
		func(string) ([]byte, error) { return nil, io.EOF },
	)
	gw.client = &http.Client{Timeout: 5 * time.Second, Transport: servicetoken.NewTransport(nil, token, gw.cfg.QueryEngineURL)}

	status, body := s106Do(t, app, "GET", "/api/v1/agents", "editor-a", nil, nil)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d body = %s, want 200", status, body)
	}
	var got struct {
		Agents []map[string]any `json:"agents"`
		Total  int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 2 || len(got.Agents) != 2 {
		t.Fatalf("got %d agents (total %d), want 2: %s", len(got.Agents), got.Total, body)
	}
	for _, a := range got.Agents {
		if a["org_id"] != "org-a" {
			t.Fatalf("agent from another org or without org returned: %v", a)
		}
	}
	if got.Agents[0]["node_name"] != "node-a1" || got.Agents[0]["status"] != "online" {
		t.Fatalf("agent fields not preserved: %v", got.Agents[0])
	}
	if sawInternalHeader {
		t.Fatal("internal token header was sent to the ingestor")
	}

	status, body = s106Do(t, app, "GET", "/api/v1/agents", "editor-b", nil, nil)
	if status != fiber.StatusOK || !strings.Contains(body, `"b1"`) || strings.Contains(body, `"a1"`) {
		t.Fatalf("org-b: status = %d body = %s, want only b1", status, body)
	}
}

func TestS106_Agents_FailClosed(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantBody   string
	}{
		{"UpstreamError", http.StatusInternalServerError, `{"error":"boom","agents":[{"id":"a1","org_id":"org-a"}]}`, fiber.StatusInternalServerError, `{"error":"agents service error"}`},
		{"MalformedJSON", http.StatusOK, `{"agents":[`, fiber.StatusBadGateway, `{"error":"invalid agents response"}`},
		{"AgentsFieldMissing", http.StatusOK, `{"items":[{"id":"a1","org_id":"org-a"}]}`, fiber.StatusBadGateway, `{"error":"invalid agents response"}`},
		{"AgentNotObject", http.StatusOK, `{"agents":["org-a"]}`, fiber.StatusBadGateway, `{"error":"invalid agents response"}`},
		{"EmptyList", http.StatusOK, `{"agents":[],"total":0}`, fiber.StatusOK, `{"agents":[],"total":0}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingestor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ingestor.Close()
			_, app := newS106Gateway(t, ingestor.URL)
			status, body := s106Do(t, app, "GET", "/api/v1/agents", "editor-a", nil, nil)
			if status != tt.wantStatus || body != tt.wantBody {
				t.Fatalf("status = %d body = %s, want %d %s", status, body, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

func TestS106_Agents_NoOrgForbidden(t *testing.T) {
	_, app := newS106Gateway(t, "")
	status, body := s106Do(t, app, "GET", "/api/v1/agents", "editor-noorg", nil, nil)
	if status != fiber.StatusForbidden || body != `{"error":"organization required"}` {
		t.Fatalf("status = %d body = %s, want 403 organization required", status, body)
	}
}

func TestFilterAgentsByOrg(t *testing.T) {
	in := []byte(`{"agents":[{"id":"a","org_id":"org-a"},{"id":"b","org_id":"org-b"},{"id":"c"},{"id":"d","org_id":""}]}`)
	out, err := filterAgentsByOrg(in, "org-a")
	if err != nil || len(out) != 1 || !strings.Contains(string(out[0]), `"id":"a"`) {
		t.Fatalf("filterAgentsByOrg = %s, %v", out, err)
	}
	if out, err := filterAgentsByOrg(in, ""); err != nil || len(out) != 0 {
		t.Fatalf("empty org must match nothing, got %s, %v", out, err)
	}
}

// ── R7 comment delete error mapping ─────────────────────────────────────────

func TestS106_CommentDeleteErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"NotFound", store.ErrCommentNotFound, fiber.StatusNotFound, `{"error":"comment not found"}`},
		{"WrappedNotFound", fmt.Errorf("delete: %w", store.ErrCommentNotFound), fiber.StatusNotFound, `{"error":"comment not found"}`},
		{"DatabaseErrorHidden", errors.New(`ERROR: relation "incident_comments" does not exist (SQLSTATE 42P01)`), fiber.StatusInternalServerError, `{"error":"failed to delete comment"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Delete("/x", func(c *fiber.Ctx) error { return commentDeleteError(c, tt.err) })
			resp, err := app.Test(httptest.NewRequest("DELETE", "/x", nil), -1)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tt.wantStatus || string(b) != tt.wantBody {
				t.Fatalf("status = %d body = %s, want %d %s", resp.StatusCode, b, tt.wantStatus, tt.wantBody)
			}
		})
	}
}

// ── Internal token logging ──────────────────────────────────────────────────

func TestS106_LogInternalTokenStatus_NeverLogsValue(t *testing.T) {
	secret := strings.Repeat("s", servicetoken.MinLength) + "-log-test-secret"
	shortFileContent := "short-file-content"
	const path = "/run/secrets/observex_internal_token"

	tests := []struct {
		name      string
		env       map[string]string
		files     map[string]string
		wantLevel zapcore.Level
		wantField map[string]string
	}{
		{
			name:      "ConfiguredFromEnv",
			env:       map[string]string{servicetoken.EnvVar: secret},
			wantLevel: zapcore.InfoLevel,
			wantField: map[string]string{"source": "env"},
		},
		{
			name:      "ConfiguredFromFile",
			env:       map[string]string{servicetoken.FileEnvVar: path, servicetoken.EnvVar: "ignored-" + secret},
			files:     map[string]string{path: secret + "\n"},
			wantLevel: zapcore.InfoLevel,
			wantField: map[string]string{"source": "file"},
		},
		{
			name:      "FileTooShort_NoFallback",
			env:       map[string]string{servicetoken.FileEnvVar: path, servicetoken.EnvVar: secret},
			files:     map[string]string{path: shortFileContent},
			wantLevel: zapcore.ErrorLevel,
			wantField: map[string]string{"source": "file", "reason": "too_short", "path": path},
		},
		{
			name:      "NotSet",
			env:       map[string]string{},
			wantLevel: zapcore.ErrorLevel,
			wantField: map[string]string{"source": "none", "reason": "not_set"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := servicetoken.LoadFrom(
				func(k string) (string, bool) { v, ok := tt.env[k]; return v, ok },
				func(p string) ([]byte, error) {
					if v, ok := tt.files[p]; ok {
						return []byte(v), nil
					}
					return nil, io.EOF
				},
			)
			core, logs := observer.New(zapcore.DebugLevel)
			logInternalTokenStatus(zap.New(core), token)

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("got %d log entries, want 1", len(entries))
			}
			e := entries[0]
			if e.Level != tt.wantLevel {
				t.Fatalf("level = %s, want %s", e.Level, tt.wantLevel)
			}
			fields := e.ContextMap()
			for k, v := range tt.wantField {
				if fields[k] != v {
					t.Fatalf("field %q = %v, want %q", k, fields[k], v)
				}
			}
			rendered := e.Message + fmt.Sprint(fields)
			for _, s := range []string{secret, strings.Repeat("s", servicetoken.MinLength), shortFileContent} {
				if strings.Contains(rendered, s) {
					t.Fatal("log entry contains a token value or token file content")
				}
			}
		})
	}
}

func waitForClients(t *testing.T, hub *WSHub, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for hub.ConnectedCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("hub has %d clients, want %d", hub.ConnectedCount(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
