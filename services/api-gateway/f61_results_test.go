package main

// F6.1 result read API, vantage revocation and issuance guards. The database
// tests run against real PostgreSQL with every migration applied
// (OBSERVEX_TEST_POSTGRES_DSN) and skip without it.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/observex/platform/internal/db/dbtest"
	dbmodels "github.com/observex/platform/internal/db/models"
	"github.com/observex/platform/internal/db/store"
	evaluate "github.com/observex/platform/internal/evaluate/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
	resultf61 "github.com/observex/platform/internal/result/f61"
	wire "github.com/observex/platform/internal/wire/f61"
)

func init() {
	s106Users["f61-viewer-payments"] = store.AuthContext{UserID: "f61-viewer-payments", Email: "vp@a.test", UserEmail: "vp@a.test",
		OrgID: "org-a", Role: dbmodels.RoleViewer, EffectiveRole: dbmodels.RoleViewer,
		NamespaceAccess: map[string]dbmodels.Access{"payments": dbmodels.AccessRead}}
	s106Users["f61-admin-b"] = store.AuthContext{UserID: "f61-admin-b", Email: "admin@b.test", UserEmail: "admin@b.test",
		OrgID: "org-b", Role: dbmodels.RoleAdmin, EffectiveRole: dbmodels.RoleAdmin, IsOrgAdmin: true}
}

func f61GatewayRequest(t *testing.T, app *fiber.App, method, path, user string, body []byte) (int, map[string]any) {
	t.Helper()
	status, raw := s106Do(t, app, method, path, user, body, nil)
	var m map[string]any
	_ = json.Unmarshal([]byte(raw), &m)
	return status, m
}

func TestF61ResultRoutesWithoutDatabase(t *testing.T) {
	gw, app := newS106Gateway(t, "")
	gw.cfg.SyntheticProbeKey = probetoken.NewKey(spcKeyValue)
	gw.cfg.JWTSecret = s106JWTSecret
	gw.cfg.AgentTokenSecret = strings.Repeat("a", 40) + "-agent-secret"
	for _, p := range []string{"/api/v1/synthetic/tls-certificates", "/api/v1/synthetic/tls-certificates/events",
		"/api/v1/synthetic/checks/chk-a/tls-certificate"} {
		if s, _ := f61GatewayRequest(t, app, "GET", p, "", nil); s != 401 {
			t.Errorf("%s unauthenticated: %d", p, s)
		}
		if s, _ := f61GatewayRequest(t, app, "GET", p, "viewer-a", nil); s != 503 {
			t.Errorf("%s without a database: %d", p, s)
		}
	}
	revoke := []byte(`{"vantage_id":"x"}`)
	for user, want := range map[string]int{"": 401, "viewer-a": 403, "editor-a": 403, "admin-a": 503} {
		if s, _ := f61GatewayRequest(t, app, "POST", "/api/v1/synthetic/probe-credentials/revoke", user, revoke); s != want {
			t.Errorf("revoke as %q: %d, want %d", user, s, want)
		}
	}
	// Reserved zones are refused at issuance.
	for _, zone := range []string{"local", "*"} {
		s, m := f61GatewayRequest(t, app, "POST", "/api/v1/synthetic/probe-credentials", "admin-a", []byte(`{"network_zone":"`+zone+`"}`))
		if s != 400 || m["token"] != nil {
			t.Errorf("zone %q: %d %v", zone, s, m["error"])
		}
	}
	if s, _ := f61GatewayRequest(t, app, "POST", "/api/v1/synthetic/probe-credentials", "admin-a", []byte(`{"network_zone":"edge-eu"}`)); s != 201 {
		t.Errorf("ordinary zone: %d", s)
	}
}

func TestF61RevokeRequestDecoding(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"vantage_id":""}`, `{"vantage_id":"v","org_id":"org-b"}`, `{"vantage_id":"v"} {}`,
		`{"vantage_id":"v","reason":"` + strings.Repeat("r", 257) + `"}`, "{\"vantage_id\":\"v\",\"reason\":\"a\\u0007\"}"} {
		if _, err := decodeF61RevokeRequest([]byte(body)); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
	if r, err := decodeF61RevokeRequest([]byte(`{"vantage_id":"v","decommission":true,"reason":"host retired"}`)); err != nil || !r.Decommission {
		t.Fatalf("%+v %v", r, err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 400_000_000, time.UTC)
	if b := f61RevocationBound(now); !b.Equal(time.Date(2026, 9, 26, 12, 0, 1, 0, time.UTC)) {
		t.Fatalf("bound %v", b)
	}
	for _, l := range [][]string{{}, {""}, {" a"}, {"a\n"}, make([]string, 33)} {
		if validCheckLocations(l) {
			t.Errorf("locations %q accepted", l)
		}
	}
	if !validCheckLocations([]string{"*", "edge-eu"}) {
		t.Error("valid locations refused")
	}
}

// f61Observed builds an accepted-shape observation of a self-signed leaf.
func f61Observed(t *testing.T, s wire.Subject, vantage string, at, notAfter time.Time) tlscert.Observation {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "h"},
		NotBefore: at.Add(-24 * time.Hour), NotAfter: notAfter, DNSNames: []string{"h"}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"v": 1, "check_id": s.CheckID, "endpoint": s.Endpoint,
		"observed_at": at.Format(time.RFC3339Nano), "duration_ns": 1, "outcome": "observed_untrusted",
		"trust":        map[string]any{"chain_verified": false, "hostname_verified": true, "reason": tlscert.ReasonUnknownAuthority},
		"detail":       "observed 1 certificate(s); verification failed: " + tlscert.ReasonUnknownAuthority,
		"certificates": [][]byte{der}})
	o, err := tlscert.UnmarshalObservation(data, tlscert.Binding{CheckID: s.CheckID, Endpoint: s.Endpoint,
		Vantage: tlscert.Vantage{Kind: "synthetic-probe", ID: vantage}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func f61SeedResult(t *testing.T, pool *pgxpool.Pool, id, org, ns string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO synthetic_checks (id, org_id, name, type, target, namespace, locations)
		VALUES ($1,$2,$3,'ssl','h:443',$4,'{*}')`, id, org, "check "+id, ns); err != nil {
		t.Fatal(err)
	}
	s, err := wire.SubjectFromCheck(wire.CheckRow{ID: id, OrgID: org, Namespace: ns, Target: "h:443"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	out, err := resultf61.New(pool).Record(ctx, resultf61.Accepted{Subject: s, IntervalSec: 60,
		Observation: f61Observed(t, s, "v-"+org, now.Add(-time.Second), now.Add(10*24*time.Hour)), ReceivedAt: now}, evaluate.DefaultHorizon)
	if err != nil || out.Event != resultf61.EventOpened {
		t.Fatalf("seed %s: %+v %v", id, out, err)
	}
}

func TestF61ResultAPIAgainstPostgres(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	gw, app := newS106Gateway(t, "")
	gw.db = &store.DB{Pool: pool}
	f61SeedResult(t, pool, "chk-pay", "org-a", "payments")
	f61SeedResult(t, pool, "chk-def", "org-a", "default")
	f61SeedResult(t, pool, "chk-b", "org-b", "payments")

	list := func(user, query string) (int, []string) {
		s, m := f61GatewayRequest(t, app, "GET", "/api/v1/synthetic/tls-certificates"+query, user, nil)
		var ids []string
		if rs, ok := m["results"].([]any); ok {
			for _, r := range rs {
				ids = append(ids, r.(map[string]any)["check_id"].(string))
			}
		}
		return s, ids
	}
	for _, tc := range []struct {
		user, query string
		status      int
		want        string
	}{
		{"admin-a", "", 200, "chk-def,chk-pay"},
		{"admin-a", "?namespace=payments", 200, "chk-pay"},
		{"f61-viewer-payments", "", 200, "chk-pay"},
		{"f61-viewer-payments", "?namespace=payments", 200, "chk-pay"},
		{"f61-viewer-payments", "?namespace=default", 403, ""},
		{"viewer-a", "", 200, ""}, // no namespace access at all
		{"f61-admin-b", "", 200, "chk-b"},
		{"editor-noorg", "", 403, ""},
	} {
		s, ids := list(tc.user, tc.query)
		if got := strings.Join(sortedCopy(ids), ","); s != tc.status || got != tc.want {
			t.Errorf("%s %q: %d %q, want %d %q", tc.user, tc.query, s, got, tc.status, tc.want)
		}
	}

	// One result: the same 404 for another org's, an inaccessible and a
	// missing check.
	s, m := f61GatewayRequest(t, app, "GET", "/api/v1/synthetic/checks/chk-pay/tls-certificate", "f61-viewer-payments", nil)
	if s != 200 || m["status"] != string(evaluate.StatusExpiring) || m["alert_state"] != resultf61.AlertOpen || m["vantages"] == nil {
		t.Fatalf("get: %d %v", s, m)
	}
	var notFound map[string]any
	for _, tc := range []struct{ user, id string }{{"admin-a", "chk-b"}, {"f61-viewer-payments", "chk-def"}, {"admin-a", "chk-none"}} {
		s, m := f61GatewayRequest(t, app, "GET", "/api/v1/synthetic/checks/"+tc.id+"/tls-certificate", tc.user, nil)
		if s != 404 {
			t.Fatalf("%s %s: %d", tc.user, tc.id, s)
		}
		if notFound == nil {
			notFound = m
		} else if m["error"] != notFound["error"] {
			t.Fatalf("404 bodies differ: %v vs %v", m, notFound)
		}
	}

	// Events: org- and namespace-scoped, cursor over the whole page.
	events := func(user, query string) (int, []string, float64) {
		s, m := f61GatewayRequest(t, app, "GET", "/api/v1/synthetic/tls-certificates/events"+query, user, nil)
		var ids []string
		if es, ok := m["events"].([]any); ok {
			for _, e := range es {
				ids = append(ids, e.(map[string]any)["check_id"].(string)+":"+e.(map[string]any)["event_type"].(string))
			}
		}
		next, _ := m["next_after"].(float64)
		return s, ids, next
	}
	if s, ids, _ := events("admin-a", ""); s != 200 || strings.Join(sortedCopy(ids), ",") != "chk-def:opened,chk-pay:opened" {
		t.Fatalf("admin-a events: %d %v", s, ids)
	}
	if s, ids, _ := events("f61-viewer-payments", ""); s != 200 || strings.Join(ids, ",") != "chk-pay:opened" {
		t.Fatalf("viewer events: %d %v", s, ids)
	}
	if s, ids, _ := events("f61-admin-b", ""); s != 200 || strings.Join(ids, ",") != "chk-b:opened" {
		t.Fatalf("org-b events: %d %v", s, ids)
	}
	s1, page1, next := events("admin-a", "?limit=1")
	_, page2, _ := events("admin-a", "?limit=1&after="+strconv.FormatInt(int64(next), 10))
	if s1 != 200 || len(page1) != 1 || len(page2) != 1 || page1[0] == page2[0] {
		t.Fatalf("paging: %v then %v", page1, page2)
	}
	for _, q := range []string{"?after=-1", "?after=x", "?limit=0", "?limit=501"} {
		if s, _, _ := events("admin-a", q); s != 400 {
			t.Errorf("events %s: %d", q, s)
		}
	}

	// Locations are updatable (LOC-1), only within the caller's org.
	put := func(user, id, body string) int {
		s, _ := f61GatewayRequest(t, app, "PUT", "/api/v1/synthetic/checks/"+id, user, []byte(body))
		return s
	}
	locations := func(id string) []string {
		var l []string
		if err := pool.QueryRow(context.Background(), `SELECT locations FROM synthetic_checks WHERE id=$1`, id).Scan(&l); err != nil {
			t.Fatal(err)
		}
		return l
	}
	if s := put("editor-a", "chk-pay", `{"enabled":true,"locations":["edge-eu","*"]}`); s != 200 || strings.Join(locations("chk-pay"), ",") != "edge-eu,*" {
		t.Fatalf("update locations: %d %v", s, locations("chk-pay"))
	}
	if s := put("editor-a", "chk-pay", `{"enabled":true,"name":"renamed"}`); s != 200 || strings.Join(locations("chk-pay"), ",") != "edge-eu,*" {
		t.Fatalf("update without locations changed them: %d %v", s, locations("chk-pay"))
	}
	if s := put("editor-a", "chk-pay", `{"enabled":true,"locations":[""]}`); s != 400 {
		t.Fatalf("invalid locations: %d", s)
	}
	if put("editor-b", "chk-pay", `{"enabled":true,"locations":["x"]}`); strings.Join(locations("chk-pay"), ",") != "edge-eu,*" {
		t.Fatal("another org changed the check's locations")
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestF61RevokeAgainstPostgres(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	key := probetoken.NewKey(spcKeyValue)
	gw, _, issue := newSPCGateway(t, key)
	gw.db = &store.DB{Pool: pool}
	app := fiber.New()
	gw.registerRoutes(app)
	revoke := func(user, body string) (int, map[string]any) {
		return f61GatewayRequest(t, app, "POST", "/api/v1/synthetic/probe-credentials/revoke", user, []byte(body))
	}

	status, v1, _ := issue("POST", "admin-a", []byte(`{"network_zone":"edge-eu"}`))
	_, v2, _ := issue("POST", "admin-a", []byte(`{"network_zone":"edge-us"}`))
	if status != 201 || v1.VantageID == "" {
		t.Fatalf("issue: %d", status)
	}
	foreign, err := key.Issue(probetoken.IssueRequest{OrgID: "org-b"}, time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Refusals first: no row may be written by any of them.
	for _, tc := range []struct {
		user, body string
		want       int
	}{
		{"editor-a", `{"vantage_id":"` + v1.VantageID + `"}`, 403},
		{"admin-a", `{"vantage_id":"` + foreign.VantageID + `"}`, 404},
		{"admin-a", `{"vantage_id":"made-up"}`, 404},
		{"admin-a", `{"vantage_id":"` + v1.VantageID + `","org_id":"org-b"}`, 400},
		{"admin-a", `{"vantage_id":"` + v1.VantageID + `","not_before":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}`, 400},
	} {
		if s, m := revoke(tc.user, tc.body); s != tc.want {
			t.Fatalf("%s %s: %d %v", tc.user, tc.body, s, m)
		}
	}
	var rows int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM f61_vantage_revocations`).Scan(&rows)
	if rows != 0 {
		t.Fatalf("refused revocations wrote %d rows", rows)
	}

	// A revocation, then a decommission; the bound never moves back.
	s, m := revoke("admin-a", `{"vantage_id":"`+v1.VantageID+`","reason":"key exposure"}`)
	if s != 200 || m["decommissioned"] != false || m["not_before"] == nil {
		t.Fatalf("revoke: %d %v", s, m)
	}
	st := resultf61.New(pool)
	bound, revoked, err := st.RevokedBefore(context.Background(), "org-a", v1.VantageID)
	if err != nil || !revoked || bound.After(time.Now().Add(2*time.Second)) {
		t.Fatalf("bound %v %v %v", bound, revoked, err)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	if s, _ := revoke("admin-a", `{"vantage_id":"`+v1.VantageID+`","not_before":"`+past+`"}`); s != 200 {
		t.Fatalf("earlier bound: %d", s)
	}
	if b, _, _ := st.RevokedBefore(context.Background(), "org-a", v1.VantageID); !b.Equal(bound) {
		t.Fatalf("bound moved back: %v -> %v", bound, b)
	}
	if s, m := revoke("admin-a", `{"vantage_id":"`+v1.VantageID+`","decommission":true}`); s != 200 || m["decommissioned"] != true {
		t.Fatalf("decommission: %d %v", s, m)
	}
	// Rotation of a decommissioned vantage is refused.
	if s, r, _ := issue("POST", "admin-a", []byte(`{"vantage_id":"`+v1.VantageID+`","network_zone":"edge-eu"}`)); s != 409 || r.Token != "" {
		t.Fatalf("rotate decommissioned: %d", s)
	}

	// A bound in the past does not block rotation; a fresh bound does, until
	// it takes effect.
	if s, _ := revoke("admin-a", `{"vantage_id":"`+v2.VantageID+`","not_before":"`+past+`"}`); s != 200 {
		t.Fatalf("past revoke: %d", s)
	}
	if s, r, _ := issue("POST", "admin-a", []byte(`{"vantage_id":"`+v2.VantageID+`","network_zone":"edge-us"}`)); s != 201 || !r.Rotated {
		t.Fatalf("rotate after a past bound: %d", s)
	}
	future := time.Now().Add(time.Second)
	if _, err := st.Revoke(context.Background(), "org-a", v2.VantageID, future, false, "", "t"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if status, msg := gw.f61IssuanceRefusal(ctx, "org-a", v2.VantageID, future.Add(-time.Second)); status != 409 || !strings.Contains(msg, "retry after") {
		t.Fatalf("fresh bound: %d %q", status, msg)
	}
	if status, _ := gw.f61IssuanceRefusal(ctx, "org-a", v2.VantageID, future.Add(time.Second)); status != 0 {
		t.Fatalf("passed bound: %d", status)
	}
}
