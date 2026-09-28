package main

// F6.1 probe endpoint tests that need no database. The accepted path, which
// writes results, is exercised against real PostgreSQL in f61_e2e_test.go.
// Credential values are never printed.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	evaluate "github.com/observex/platform/internal/evaluate/f61"
	intakef61 "github.com/observex/platform/internal/intake/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
)

const f61TestKey = "processor-f61-test-key-0123456789abcdef"

var f61T0 = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

// fakeRow scans a fixed row into any destination of the matching type.
type fakeRow struct {
	vals []any
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.vals) {
		return errors.New("scan arity")
	}
	for i, d := range dest {
		dv := reflect.ValueOf(d)
		v := reflect.ValueOf(r.vals[i])
		if dv.Kind() != reflect.Pointer || !v.Type().AssignableTo(dv.Elem().Type()) {
			return errors.New("unexpected scan target")
		}
		dv.Elem().Set(v)
	}
	return nil
}

// fakeRows is the minimal pgx.Rows the work listing needs.
type fakeRows struct {
	rows [][]any
	i    int
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool                                   { r.i++; return r.i <= len(r.rows) }
func (r *fakeRows) Scan(dest ...any) error                       { return fakeRow{vals: r.rows[r.i-1]}.Scan(dest...) }
func (r *fakeRows) Values() ([]any, error)                       { return r.rows[r.i-1], nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

// fakeDB answers the check lookup, the work listing and the revocation
// lookup. It has no transactions: anything that would write fails, and the
// test asserts no refusal ever got that far.
type fakeDB struct {
	mu      sync.Mutex
	checks  map[string][]any // f61CheckQuery column order
	revoked map[string]time.Time
	err     error
	queries []string
	args    [][]any
	begins  int
}

func (f *fakeDB) record(sql string, args []any) {
	f.queries = append(f.queries, sql)
	f.args = append(f.args, args)
}

func (f *fakeDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sql, args)
	if f.err != nil {
		return fakeRow{err: f.err}
	}
	if strings.Contains(sql, "f61_vantage_revocations") {
		if nb, ok := f.revoked[args[0].(string)+"|"+args[1].(string)]; ok {
			return fakeRow{vals: []any{pgtype.Timestamptz{Time: nb, Valid: true}}}
		}
		return fakeRow{err: pgx.ErrNoRows}
	}
	if v, ok := f.checks[args[0].(string)]; ok {
		return fakeRow{vals: v}
	}
	return fakeRow{err: pgx.ErrNoRows}
}

func (f *fakeDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(sql, args)
	if f.err != nil {
		return nil, f.err
	}
	var out [][]any
	for _, v := range f.checks { // deliberately unfiltered: the authorizer must filter
		out = append(out, v)
	}
	return &fakeRows{rows: out}, nil
}

func (f *fakeDB) Begin(context.Context) (pgx.Tx, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.begins++
	return nil, errors.New("fake database has no transactions")
}

func checkRow(id, org, ns, target, typ string, enabled bool, locations ...string) []any {
	return []any{id, org, ns, target, typ, enabled, locations, 60, 10}
}

func newFakeDB() *fakeDB {
	return &fakeDB{revoked: map[string]time.Time{}, checks: map[string][]any{
		"chk-a":     checkRow("chk-a", "org-a", "payments", "api.example.com:443", "ssl", true, "*"),
		"chk-b":     checkRow("chk-b", "org-b", "default", "b.example.com:443", "ssl", true, "*"),
		"chk-http":  checkRow("chk-http", "org-a", "default", "https://x", "http", true, "*"),
		"chk-eu":    checkRow("chk-eu", "org-a", "default", "eu.example.com:443", "ssl", true, "eu-west"),
		"chk-local": checkRow("chk-local", "org-a", "default", "l.example.com:443", "ssl", true, "local"),
		"chk-off":   checkRow("chk-off", "org-a", "default", "off.example.com:443", "ssl", false, "*"),
	}}
}

func f61Credential(t *testing.T, org string) (string, string) {
	t.Helper()
	out, err := probetoken.NewKey(f61TestKey).Issue(probetoken.IssueRequest{OrgID: org}, f61T0,
		bytes.NewReader(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential.Reveal(), out.VantageID
}

func f61TimeoutPayload(t *testing.T, checkID, endpoint string, observedAt time.Time) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"v": 1, "check_id": checkID, "endpoint": endpoint, "observed_at": observedAt.UTC().Format(time.RFC3339Nano),
		"duration_ns": 1000, "outcome": "timeout",
		"trust":  map[string]any{"chain_verified": false, "hostname_verified": false, "reason": ""},
		"detail": "timeout during dial",
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var f61Now = f61T0.Add(time.Minute)

func f61App(t *testing.T, key probetoken.Key, db *fakeDB) (*fiber.App, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	d := f61Deps{logger: zap.New(core), key: key, horizon: evaluate.DefaultHorizon, now: func() time.Time { return f61Now }}
	if db != nil {
		d.db = db
	}
	return newF61IntakeApp(d), logs
}

func f61Do(t *testing.T, app *fiber.App, method, path, auth string, body []byte, extra map[string]string) (int, map[string]any, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	rec := httptest.NewRecorder()
	for k, v := range resp.Header {
		rec.Header()[k] = v
	}
	rec.Body.Write(raw)
	return resp.StatusCode, m, rec
}

func f61Post(t *testing.T, app *fiber.App, checkID, auth string, body []byte, extra map[string]string) (int, map[string]any, *httptest.ResponseRecorder) {
	return f61Do(t, app, "POST", "/v1/synthetic/checks/"+checkID+"/tls-observations", auth, body, extra)
}

func TestF61IntakeRefusals(t *testing.T) {
	credA, _ := f61Credential(t, "org-a")
	credB, _ := f61Credential(t, "org-b")
	good := f61TimeoutPayload(t, "chk-a", "api.example.com:443", f61Now.Add(-time.Second))
	cases := []struct {
		name, check, auth string
		body              []byte
		status            int
		reason            string
	}{
		{"no credential", "chk-a", "", good, 401, "unauthenticated"},
		{"agent-style token", "chk-a", "Bearer oxat_a.b.c", good, 401, "unauthenticated"},
		{"credential for another org", "chk-a", "Bearer " + credB, good, 403, "forbidden"},
		{"missing check", "chk-zzz", "Bearer " + credA, good, 403, "forbidden"},
		{"not an ssl check", "chk-http", "Bearer " + credA, good, 403, "forbidden"},
		{"disabled check", "chk-off", "Bearer " + credA, good, 403, "forbidden"},
		{"check not assigned to this vantage", "chk-eu", "Bearer " + credA, good, 403, "forbidden"},
		{"check for the local executor only", "chk-local", "Bearer " + credA, good, 403, "forbidden"},
		{"payload for another check", "chk-a", "Bearer " + credA, f61TimeoutPayload(t, "chk-b", "api.example.com:443", f61Now), 409, "observation_check_mismatch"},
		{"payload for another endpoint", "chk-a", "Bearer " + credA, f61TimeoutPayload(t, "chk-a", "evil.example:443", f61Now), 409, "observation_check_mismatch"},
		{"observation older than the report window", "chk-a", "Bearer " + credA, f61TimeoutPayload(t, "chk-a", "api.example.com:443", f61Now.Add(-intakef61.MaxReportDelay-time.Second)), 400, "observation_time_out_of_range"},
		{"observation from the future", "chk-a", "Bearer " + credA, f61TimeoutPayload(t, "chk-a", "api.example.com:443", f61Now.Add(intakef61.ClockSkew+time.Second)), 400, "observation_time_out_of_range"},
		{"malformed payload", "chk-a", "Bearer " + credA, []byte("{"), 400, "malformed_observation"},
		{"oversized payload", "chk-a", "Bearer " + credA, make([]byte, tlscert.MaxWireBytes+1), 413, "payload_too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newFakeDB()
			app, logs := f61App(t, probetoken.NewKey(f61TestKey), db)
			status, body, rec := f61Post(t, app, tc.check, tc.auth, tc.body, nil)
			if status != tc.status || body["reason"] != tc.reason {
				t.Fatalf("status %d reason %v, want %d %q", status, body["reason"], tc.status, tc.reason)
			}
			if db.begins != 0 {
				t.Fatal("a refused report reached the result store")
			}
			if (status == 401) != (rec.Header().Get("WWW-Authenticate") == "Bearer") {
				t.Fatalf("WWW-Authenticate %q on status %d", rec.Header().Get("WWW-Authenticate"), status)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("refusal is cacheable")
			}
			f61AssertLogsClean(t, logs, credA, credB)
		})
	}
}

// The admitted path reaches the store with server-derived identity only, and
// a store failure is a 503 that reveals nothing.
func TestF61IntakeIgnoresCallerIdentityAndFailsClosedOnStore(t *testing.T) {
	db := newFakeDB()
	app, logs := f61App(t, probetoken.NewKey(f61TestKey), db)
	cred, _ := f61Credential(t, "org-a")
	spoof := map[string]string{"X-ObserveX-Org": "org-b", "X-ObserveX-Vantage": "external-eu", "X-ObserveX-Token": "x"}
	status, body, _ := f61Post(t, app, "chk-a", "Bearer "+cred, f61TimeoutPayload(t, "chk-a", "api.example.com:443", f61Now), spoof)
	if status != 503 || body["reason"] != "unavailable" || db.begins != 1 {
		t.Fatalf("status %d body %v begins %d", status, body, db.begins)
	}
	// The only lookups were by the path's check ID and by the credential's
	// org and vantage; no header value was ever used.
	for _, args := range db.args {
		for _, a := range args {
			if s, ok := a.(string); ok && (s == "org-b" || s == "external-eu") {
				t.Fatalf("a caller-supplied header reached a query: %v", db.args)
			}
		}
	}
	if db.queries[0] != f61CheckQuery && !strings.Contains(db.queries[0], "f61_vantage_revocations") {
		t.Fatalf("unexpected query: %v", db.queries)
	}
	f61AssertLogsClean(t, logs, cred)
}

// Missing, foreign, unassigned and disabled checks produce identical
// responses.
func TestF61IntakeDoesNotRevealForeignChecks(t *testing.T) {
	app, _ := f61App(t, probetoken.NewKey(f61TestKey), newFakeDB())
	cred, _ := f61Credential(t, "org-a")
	var first map[string]any
	for _, id := range []string{"chk-b", "chk-nope", "chk-eu", "chk-off"} {
		s, b, _ := f61Post(t, app, id, "Bearer "+cred, f61TimeoutPayload(t, id, "b.example.com:443", f61Now), nil)
		if s != 403 {
			t.Fatalf("%s: status %d", id, s)
		}
		if first == nil {
			first = b
		} else if !reflect.DeepEqual(first, b) {
			t.Fatalf("%s: body %v differs from %v", id, b, first)
		}
	}
}

func TestF61WorkListsOnlyThisVantagesChecks(t *testing.T) {
	db := newFakeDB()
	app, logs := f61App(t, probetoken.NewKey(f61TestKey), db)
	cred, vid := f61Credential(t, "org-a")
	status, _, rec := f61Do(t, app, "GET", f61WorkRoute, "Bearer "+cred, nil, map[string]string{"X-ObserveX-Org": "org-b"})
	if status != 200 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d", status)
	}
	var ws intakef61.WorkSet
	if err := json.Unmarshal(rec.Body.Bytes(), &ws); err != nil {
		t.Fatal(err)
	}
	if ws.VantageID != vid || len(ws.Assignments) != 1 || ws.Assignments[0].CheckID != "chk-a" ||
		ws.Assignments[0].Endpoint != "api.example.com:443" || ws.Assignments[0].IntervalSec != 60 ||
		ws.Assignments[0].PhaseSec != intakef61.Phase("chk-a", vid, 60) {
		t.Fatalf("work %+v", ws)
	}
	if len(db.args) < 2 || db.args[len(db.args)-1][0] != "org-a" {
		t.Fatalf("work not listed by the credential's org: %v", db.args)
	}
	for _, tc := range []struct{ auth, reason string }{{"", "unauthenticated"}, {"Bearer oxpt_nope", "unauthenticated"}} {
		if s, b, _ := f61Do(t, app, "GET", f61WorkRoute, tc.auth, nil, nil); s != 401 || b["reason"] != tc.reason {
			t.Fatalf("work with %q: %d %v", tc.auth, s, b)
		}
	}
	// A revoked vantage gets nothing, and a revocation lookup failure is a
	// 503, never an open door.
	db.revoked["org-a|"+vid] = f61T0.Add(time.Second)
	if s, b, _ := f61Do(t, app, "GET", f61WorkRoute, "Bearer "+cred, nil, nil); s != 401 || b["reason"] != "unauthenticated" {
		t.Fatalf("revoked: %d %v", s, b)
	}
	db.err = errors.New("connection refused")
	if s, b, _ := f61Do(t, app, "GET", f61WorkRoute, "Bearer "+cred, nil, nil); s != 503 || b["reason"] != "unavailable" {
		t.Fatalf("database failure: %d %v", s, b)
	}
	f61AssertLogsClean(t, logs, cred)
}

func TestF61UnconfiguredIsUnavailable(t *testing.T) {
	cred, _ := f61Credential(t, "org-a")
	body := f61TimeoutPayload(t, "chk-a", "api.example.com:443", f61Now)
	noKey, _ := f61App(t, probetoken.Key{}, newFakeDB())
	noDB, _ := f61App(t, probetoken.NewKey(f61TestKey), nil)
	badHorizon := newF61IntakeApp(f61Deps{logger: zap.NewNop(), key: probetoken.NewKey(f61TestKey), db: newFakeDB(), horizon: time.Hour, now: time.Now})
	for name, app := range map[string]*fiber.App{"no key": noKey, "no database": noDB, "invalid horizon": badHorizon} {
		if status, b, _ := f61Post(t, app, "chk-a", "Bearer "+cred, body, nil); status != 503 || b["reason"] != "unavailable" {
			t.Errorf("%s: intake status %d body %v", name, status, b)
		}
		if status, b, _ := f61Do(t, app, "GET", f61WorkRoute, "Bearer "+cred, nil, nil); status != 503 || b["reason"] != "unavailable" {
			t.Errorf("%s: work status %d body %v", name, status, b)
		}
	}
	// The dedicated app serves the two probe routes and nothing else.
	app, _ := f61App(t, probetoken.NewKey(f61TestKey), newFakeDB())
	for _, p := range []string{"/health", "/api/v1/services", "/v1/synthetic/checks/chk-a"} {
		if s, _, _ := f61Do(t, app, "GET", p, "Bearer "+cred, nil, nil); s != 404 && s != 405 {
			t.Errorf("%s answered %d on the probe listener", p, s)
		}
	}
}

func TestF61HorizonFromEnv(t *testing.T) {
	env := func(v string, ok bool) func(string) (string, bool) {
		return func(string) (string, bool) { return v, ok }
	}
	if d, err := f61HorizonFromEnv(env("", false)); err != nil || d != evaluate.DefaultHorizon {
		t.Fatalf("default: %v %v", d, err)
	}
	if d, err := f61HorizonFromEnv(env("336h", true)); err != nil || d != 336*time.Hour {
		t.Fatalf("336h: %v %v", d, err)
	}
	for _, v := range []string{"1h", "9000h", "thirty days", "-720h"} {
		if _, err := f61HorizonFromEnv(env(v, true)); err == nil {
			t.Errorf("%q accepted", v)
		}
	}
}

func TestPgLookupMapsRows(t *testing.T) {
	db := newFakeDB()
	l := pgSyntheticCheckLookup{db: db}
	r, err := l.LookupCheck(context.Background(), "chk-eu")
	if err != nil || r.Row.ID != "chk-eu" || r.Row.OrgID != "org-a" || r.Row.Target != "eu.example.com:443" ||
		r.Type != "ssl" || !r.Enabled || !reflect.DeepEqual(r.Locations, []string{"eu-west"}) || r.IntervalSec != 60 || r.TimeoutSec != 10 {
		t.Fatalf("got %+v, %v", r, err)
	}
	if _, err := l.LookupCheck(context.Background(), "nope"); !errors.Is(err, intakef61.ErrCheckNotFound) {
		t.Fatalf("missing row: %v", err)
	}
	db.err = errors.New("boom")
	if _, err := l.LookupCheck(context.Background(), "chk-a"); err == nil || errors.Is(err, intakef61.ErrCheckNotFound) {
		t.Fatalf("database error must not look like a missing row: %v", err)
	}
	for _, q := range []string{f61CheckQuery, f61WorkQuery} {
		if strings.Contains(q, "%") || !strings.Contains(q, "$1") {
			t.Fatal("queries must be parameterised")
		}
	}
}

func f61AssertLogsClean(t *testing.T, logs *observer.ObservedLogs, creds ...string) {
	t.Helper()
	for _, cred := range creds {
		secret := strings.TrimPrefix(cred, probetoken.TokenPrefix)
		for _, e := range logs.All() {
			b, _ := json.Marshal(e.ContextMap())
			if strings.Contains(e.Message+string(b), secret[:24]) {
				t.Fatal("a log entry contains the credential")
			}
		}
	}
}
