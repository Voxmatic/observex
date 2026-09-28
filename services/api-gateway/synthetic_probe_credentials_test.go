package main

// F6.1 synthetic probe credential issuance tests. Credential values are never
// printed; assertions compare them without logging them.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	intakef61 "github.com/observex/platform/internal/intake/f61"
	"github.com/observex/platform/internal/probetoken"
	wire "github.com/observex/platform/internal/wire/f61"
)

const spcKeyValue = "synthetic-probe-test-key-0123456789abcdef"

type spcResponse struct {
	Token       string            `json:"token"`
	TokenType   string            `json:"token_type"`
	OrgID       string            `json:"org_id"`
	VantageID   string            `json:"vantage_id"`
	VantageKind string            `json:"vantage_kind"`
	Scopes      []string          `json:"scopes"`
	Declared    map[string]string `json:"declared"`
	Rotated     bool              `json:"rotated"`
	Revocable   bool              `json:"revocable"`
	ExpiresIn   int               `json:"expires_in_sec"`
	Error       string            `json:"error"`
}

func newSPCGateway(t *testing.T, key probetoken.Key) (*Gateway, *observer.ObservedLogs, func(method, userID string, body []byte) (int, spcResponse, string)) {
	t.Helper()
	gw, app := newS106Gateway(t, "")
	core, logs := observer.New(zapcore.DebugLevel)
	gw.log = zap.New(core)
	gw.cfg.SyntheticProbeKey = key
	gw.cfg.JWTSecret = s106JWTSecret
	gw.cfg.AgentTokenSecret = strings.Repeat("a", 40) + "-agent-secret"
	do := func(method, userID string, body []byte) (int, spcResponse, string) {
		t.Helper()
		status, raw := s106Do(t, app, method, "/api/v1/synthetic/probe-credentials", userID, body, nil)
		var r spcResponse
		_ = json.Unmarshal([]byte(raw), &r)
		return status, r, raw
	}
	return gw, logs, do
}

func TestSPCRequiresOrgAdmin(t *testing.T) {
	_, _, do := newSPCGateway(t, probetoken.NewKey(spcKeyValue))
	for _, tc := range []struct {
		user string
		want int
	}{{"", 401}, {"viewer-a", 403}, {"editor-a", 403}, {"editor-noorg", 403}} {
		status, r, _ := do("POST", tc.user, nil)
		if status != tc.want || r.Token != "" {
			t.Errorf("user %q: status %d, want %d (token returned: %v)", tc.user, status, tc.want, r.Token != "")
		}
	}
}

func TestSPCRefusesWithoutDedicatedKey(t *testing.T) {
	cases := map[string]func(gw *Gateway){
		"not configured":     func(gw *Gateway) { gw.cfg.SyntheticProbeKey = probetoken.Key{} },
		"same as JWT secret": func(gw *Gateway) { gw.cfg.SyntheticProbeKey = probetoken.NewKey(s106JWTSecret) },
		"same as agent secret": func(gw *Gateway) {
			gw.cfg.SyntheticProbeKey = probetoken.NewKey(gw.cfg.AgentTokenSecret)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			gw, _, do := newSPCGateway(t, probetoken.NewKey(spcKeyValue))
			mutate(gw)
			status, r, _ := do("POST", "admin-a", nil)
			if status != 503 || r.Token != "" {
				t.Fatalf("status %d, want 503 with no token", status)
			}
		})
	}
}

func TestSPCIssuesServerMintedVantageForAdminOrg(t *testing.T) {
	key := probetoken.NewKey(spcKeyValue)
	_, logs, do := newSPCGateway(t, key)
	body := []byte(`{"network_zone":"edge-eu","cluster_name":"probe-eu-1","environment":"production","host_group":"synthetic"}`)
	status, r, _ := do("POST", "admin-a", body)
	if status != 201 {
		t.Fatalf("status %d: %s", status, r.Error)
	}
	if r.OrgID != "org-a" || r.TokenType != "Bearer" || r.VantageKind != intakef61.VantageKind || r.Rotated || !r.Revocable {
		t.Fatalf("unexpected response fields: org=%q type=%q kind=%q rotated=%v revocable=%v", r.OrgID, r.TokenType, r.VantageKind, r.Rotated, r.Revocable)
	}
	if len(r.Scopes) != 1 || r.Scopes[0] != probetoken.Scope || r.ExpiresIn != int(probetoken.DefaultTTL.Seconds()) {
		t.Fatal("unexpected scopes or lifetime")
	}
	p, err := key.Verify(r.Token, time.Now())
	if err != nil {
		t.Fatalf("issued credential does not verify: %v", err)
	}
	if p.OrgID != "org-a" || p.VantageID != r.VantageID || p.Declared.ClusterName != "probe-eu-1" || p.Declared.NetworkZone != "edge-eu" {
		t.Fatal("verified principal does not match the issued vantage")
	}
	for _, e := range logs.All() {
		b, _ := json.Marshal(e.ContextMap())
		if strings.Contains(e.Message, r.Token) || strings.Contains(string(b), r.Token) || strings.Contains(string(b), probetoken.TokenPrefix) {
			t.Fatal("credential value appeared in a log entry")
		}
	}
	if logs.FilterMessage("synthetic probe credential issued").Len() != 1 {
		t.Fatal("issuance was not logged")
	}
}

func TestSPCRefusesCallerSuppliedTenantAndUnknownFields(t *testing.T) {
	_, _, do := newSPCGateway(t, probetoken.NewKey(spcKeyValue))
	for _, body := range []string{
		`{"org_id":"org-b"}`,
		`{"scopes":["metrics:write"]}`,
		`{"vantage_kind":"external"}`,
		`{"agent_id":"host-1"}`,
		`{"network_zone":"a"}{"org_id":"org-b"}`,
		`{"network_zone":`,
		`[]`,
		`{"ttl_hours":"24"}`,
		`{"ttl_hours":1.5}`,
	} {
		status, r, _ := do("POST", "admin-a", []byte(body))
		if status != 400 || r.Token != "" {
			t.Errorf("body %s: status %d, want 400 with no token", body, status)
		}
	}
}

func TestSPCLifetimeBounds(t *testing.T) {
	_, _, do := newSPCGateway(t, probetoken.NewKey(spcKeyValue))
	for _, tc := range []struct {
		body   string
		status int
		secs   int
	}{
		{`{}`, 201, int(probetoken.DefaultTTL.Seconds())},
		{`{"ttl_hours":0}`, 201, int(probetoken.DefaultTTL.Seconds())},
		{`{"ttl_hours":1}`, 201, 3600},
		{`{"ttl_hours":720}`, 201, 720 * 3600},
		{`{"ttl_hours":721}`, 400, 0},
		{`{"ttl_hours":-1}`, 400, 0},
		{`{"ttl_hours":9223372036854775807}`, 400, 0},
	} {
		status, r, _ := do("POST", "admin-a", []byte(tc.body))
		if status != tc.status || (status == 201 && r.ExpiresIn != tc.secs) {
			t.Errorf("body %s: status %d secs %d, want %d secs %d", tc.body, status, r.ExpiresIn, tc.status, tc.secs)
		}
	}
}

func TestSPCRotationKeepsVantageWithinOrgOnly(t *testing.T) {
	key := probetoken.NewKey(spcKeyValue)
	_, _, do := newSPCGateway(t, key)
	status, first, _ := do("POST", "admin-a", nil)
	if status != 201 {
		t.Fatalf("issue: %d", status)
	}
	body, _ := json.Marshal(map[string]string{"vantage_id": first.VantageID})
	status, second, _ := do("POST", "admin-a", body)
	// Within one second the re-issued credential may be byte-identical (same
	// claims, second-granularity iat); what rotation guarantees is the vantage.
	if status != 201 || !second.Rotated || second.VantageID != first.VantageID || second.Token == "" {
		t.Fatalf("rotation: status %d rotated %v same id %v", status, second.Rotated, second.VantageID == first.VantageID)
	}
	// A vantage minted for org-b cannot be re-issued by org-a's admin.
	foreign, err := key.NewVantageID("org-b", bytes.NewReader(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign, "my-probe", strings.ToUpper(first.VantageID)} {
		body, _ := json.Marshal(map[string]string{"vantage_id": id})
		if status, r, _ := do("POST", "admin-a", body); status != 400 || r.Token != "" {
			t.Errorf("vantage_id %q: status %d, want 400", id, status)
		}
	}
}

// A probe credential is not a user session, even when a misconfigured
// deployment signs it with a key equal to the session secret.
func TestSPCProbeCredentialIsNotASession(t *testing.T) {
	_, app := newS106Gateway(t, "")
	sessionKey := probetoken.NewKey(s106JWTSecret)
	out, err := sessionKey.Issue(probetoken.IssueRequest{OrgID: "org-a"}, time.Now(), bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	cred := out.Credential.Reveal()
	for _, presented := range []string{cred, strings.TrimPrefix(cred, probetoken.TokenPrefix)} {
		req := httptest.NewRequest("GET", "/api/v1/org", nil)
		req.Header.Set("Authorization", "Bearer "+presented)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("probe credential accepted as a session: status %d", resp.StatusCode)
		}
	}
}

type spcLookup map[string]intakef61.CheckRecord

func (l spcLookup) LookupCheck(_ context.Context, id string) (intakef61.CheckRecord, error) {
	r, ok := l[id]
	if !ok {
		return intakef61.CheckRecord{}, intakef61.ErrCheckNotFound
	}
	return r, nil
}

// End to end within the process: a credential issued over HTTP is admitted by
// the intake boundary for its own org's check, under the row's subject and the
// issued vantage, and refused for another org's check.
func TestSPCIssuedCredentialPassesIntakeBoundary(t *testing.T) {
	key := probetoken.NewKey(spcKeyValue)
	_, _, do := newSPCGateway(t, key)
	status, r, _ := do("POST", "admin-a", []byte(`{"network_zone":"edge-eu"}`))
	if status != 201 {
		t.Fatalf("issue: %d", status)
	}
	lookup := spcLookup{
		"chk-a": {Row: wire.CheckRow{ID: "chk-a", OrgID: "org-a", Namespace: "payments", Target: "api.example.com"}, Type: "ssl", Enabled: true,
			Locations: []string{"edge-eu"}, IntervalSec: 60, TimeoutSec: 10},
		"chk-b": {Row: wire.CheckRow{ID: "chk-b", OrgID: "org-b", Namespace: "default", Target: "b.example.com"}, Type: "ssl", Enabled: true,
			Locations: []string{"*"}, IntervalSec: 60, TimeoutSec: 10},
	}
	a, err := intakef61.New(key, lookup, spcNoRevocations{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adm, err := a.Authorize(context.Background(), "Bearer "+r.Token, "chk-a")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if adm.Subject.OrgID != "org-a" || adm.Subject.Namespace != "payments" || adm.Vantage.ID != r.VantageID ||
		adm.Vantage.Kind != intakef61.VantageKind || adm.Declared.NetworkZone != "edge-eu" {
		t.Fatalf("unexpected admission: %+v", adm)
	}
	if _, err := a.Authorize(context.Background(), "Bearer "+r.Token, "chk-b"); !errors.Is(err, intakef61.ErrForbidden) || !errors.Is(err, intakef61.ErrOrgMismatch) {
		t.Fatalf("cross-org report was not refused: %v", err)
	}
}

// spcNoRevocations: no vantage has a revocation bound.
type spcNoRevocations struct{}

func (spcNoRevocations) RevokedBefore(context.Context, string, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
