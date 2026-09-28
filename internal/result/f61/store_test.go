package f61

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/observex/platform/internal/db/dbtest"
	evaluate "github.com/observex/platform/internal/evaluate/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

var t0 = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

const horizon = evaluate.DefaultHorizon

func leaf(t *testing.T, host string, notAfter time.Time) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: host},
		NotBefore: t0.Add(-400 * 24 * time.Hour), NotAfter: notAfter, DNSNames: []string{host}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func obsFor(t *testing.T, s wire.Subject, vantage string, at time.Time, der []byte) tlscert.Observation {
	t.Helper()
	m := map[string]any{"v": 1, "check_id": s.CheckID, "endpoint": s.Endpoint, "observed_at": at.Format(time.RFC3339Nano), "duration_ns": 1}
	if der == nil {
		m["outcome"], m["detail"] = "timeout", "timeout during dial"
		m["trust"] = map[string]any{"chain_verified": false, "hostname_verified": false, "reason": ""}
	} else {
		m["outcome"] = "observed_untrusted"
		m["trust"] = map[string]any{"chain_verified": false, "hostname_verified": true, "reason": tlscert.ReasonUnknownAuthority}
		m["detail"] = "observed 1 certificate(s); verification failed: " + tlscert.ReasonUnknownAuthority
		m["certificates"] = [][]byte{der}
	}
	data, _ := json.Marshal(m)
	o, err := tlscert.UnmarshalObservation(data, tlscert.Binding{CheckID: s.CheckID, Endpoint: s.Endpoint, Vantage: tlscert.Vantage{Kind: "synthetic-probe", ID: vantage}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func seedCheck(t *testing.T, pool *pgxpool.Pool, id, org, ns, target string, interval int) wire.Subject {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO synthetic_checks (id, org_id, name, type, target, namespace, interval_sec)
		VALUES ($1,$2,$3,'ssl',$4,$5,$6)`, id, org, "check "+id, target, ns, interval); err != nil {
		t.Fatal(err)
	}
	s, err := wire.SubjectFromCheck(wire.CheckRow{ID: id, OrgID: org, Namespace: ns, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func record(t *testing.T, st *Store, s wire.Subject, o tlscert.Observation, at time.Time) Outcome {
	t.Helper()
	out, err := st.Record(context.Background(), Accepted{Subject: s, IntervalSec: 300, Observation: o, ReceivedAt: at}, horizon)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	return out
}

func events(t *testing.T, st *Store, org string) []Event {
	t.Helper()
	ev, err := st.Events(context.Background(), org, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestLifecycleOpenEscalateResolve(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	s := seedCheck(t, pool, "chk-1", "org-a", "payments", "svc.example:443", 300)

	// ok: a result exists, no episode, no event.
	out := record(t, st, s, obsFor(t, s, "vtg_a", t0, leaf(t, "svc.example", t0.Add(90*24*time.Hour))), t0)
	if !out.Stored || out.Verdict.Status != evaluate.StatusOK || out.Event != "" || out.AlertState != AlertNone {
		t.Fatalf("ok: %+v", out)
	}
	// expiring: episode opens.
	at := t0.Add(5 * time.Minute)
	out = record(t, st, s, obsFor(t, s, "vtg_a", at, leaf(t, "svc.example", t0.Add(10*24*time.Hour))), at)
	if out.Event != EventOpened || out.AlertState != AlertOpen {
		t.Fatalf("expiring: %+v", out)
	}
	r, err := st.Get(context.Background(), "org-a", "chk-1", at)
	if err != nil || r.Status != "expiring" || r.EpisodeID == "" || r.OpenedAt == nil || r.AlertState != AlertOpen {
		t.Fatalf("stored expiring: %+v %v", r, err)
	}
	episode := r.EpisodeID
	// unreachable: status changes, episode stays open, no event.
	at = at.Add(5 * time.Minute)
	out = record(t, st, s, obsFor(t, s, "vtg_a", at, nil), at)
	if out.Verdict.Status != evaluate.StatusUnreachable || out.Event != "" || out.AlertState != AlertOpen {
		t.Fatalf("unreachable: %+v", out)
	}
	// expired: escalates once.
	at = at.Add(5 * time.Minute)
	out = record(t, st, s, obsFor(t, s, "vtg_a", at, leaf(t, "svc.example", at.Add(-time.Hour))), at)
	if out.Event != EventEscalated {
		t.Fatalf("expired: %+v", out)
	}
	// unreachable then expired again: no second escalation.
	at = at.Add(5 * time.Minute)
	record(t, st, s, obsFor(t, s, "vtg_a", at, nil), at)
	at = at.Add(5 * time.Minute)
	if out = record(t, st, s, obsFor(t, s, "vtg_a", at, leaf(t, "svc.example", at.Add(-time.Hour))), at); out.Event != "" {
		t.Fatalf("second escalation: %+v", out)
	}
	// renewed: resolves.
	at = at.Add(5 * time.Minute)
	out = record(t, st, s, obsFor(t, s, "vtg_a", at, leaf(t, "svc.example", at.Add(90*24*time.Hour))), at)
	if out.Event != EventResolved || out.AlertState != AlertNone {
		t.Fatalf("resolved: %+v", out)
	}
	r, _ = st.Get(context.Background(), "org-a", "chk-1", at)
	if r.Status != "ok" || r.ResolvedAt == nil || r.EpisodeID != "" || r.OpenedAt != nil {
		t.Fatalf("stored resolved: %+v", r)
	}
	ev := events(t, st, "org-a")
	if len(ev) != 3 || ev[0].Type != EventOpened || ev[1].Type != EventEscalated || ev[2].Type != EventResolved ||
		ev[0].EpisodeID != episode || ev[2].EpisodeID != episode || ev[1].StatusFrom != "unreachable" || ev[1].StatusTo != "expired" {
		t.Fatalf("events: %+v", ev)
	}
}

func TestDuplicatesAndOutOfOrderAreNoOps(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	s := seedCheck(t, pool, "chk-1", "org-a", "default", "svc.example:443", 300)
	newer := obsFor(t, s, "vtg_a", t0, leaf(t, "svc.example", t0.Add(10*24*time.Hour)))
	record(t, st, s, newer, t0)
	var version int64
	_ = pool.QueryRow(context.Background(), `SELECT version FROM f61_tls_results WHERE check_id='chk-1'`).Scan(&version)

	for _, o := range []tlscert.Observation{
		newer, // exact replay
		obsFor(t, s, "vtg_a", t0.Add(-time.Minute), leaf(t, "svc.example", t0.Add(-time.Hour))), // older, would be "expired"
	} {
		out := record(t, st, s, o, t0.Add(time.Minute))
		if out.Stored {
			t.Fatalf("not-newer observation was stored: %+v", out)
		}
	}
	var after int64
	var status string
	_ = pool.QueryRow(context.Background(), `SELECT version, status FROM f61_tls_results WHERE check_id='chk-1'`).Scan(&after, &status)
	if after != version || status != "expiring" || len(events(t, st, "org-a")) != 1 {
		t.Fatalf("state changed: version %d→%d status %s", version, after, status)
	}
}

func TestMultiVantageWorstCaseAndStaleness(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	s := seedCheck(t, pool, "chk-1", "org-a", "default", "svc.example:443", 300)
	record(t, st, s, obsFor(t, s, "vtg_a", t0, leaf(t, "svc.example", t0.Add(90*24*time.Hour))), t0)
	out := record(t, st, s, obsFor(t, s, "vtg_b", t0.Add(time.Minute), leaf(t, "svc.example", t0.Add(3*24*time.Hour))), t0.Add(time.Minute))
	if out.Verdict.Status != evaluate.StatusExpiring || !out.Verdict.Disagreement || out.Verdict.VantagesInSpan != 2 {
		t.Fatalf("two vantages: %+v", out.Verdict)
	}
	// Much later, only vtg_a reports: vtg_b is out of span and no longer counts.
	later := t0.Add(time.Hour)
	out = record(t, st, s, obsFor(t, s, "vtg_a", later, leaf(t, "svc.example", t0.Add(90*24*time.Hour))), later)
	if out.Verdict.Status != evaluate.StatusOK || out.Verdict.VantagesStale != 1 || out.Event != EventResolved {
		t.Fatalf("stale vantage: %+v event %q", out.Verdict, out.Event)
	}
	r, _ := st.Get(context.Background(), "org-a", "chk-1", later)
	if len(r.Vantages) != 2 || r.Vantages[1].Status != evaluate.StatusStale {
		t.Fatalf("detail: %+v", r.Vantages)
	}
	// Read-time staleness of the result itself.
	r, _ = st.Get(context.Background(), "org-a", "chk-1", later.Add(16*time.Minute))
	if !r.Stale {
		t.Fatal("a result not refreshed for three intervals must read as stale")
	}
}

func TestTenantIsolationOnRead(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	a := seedCheck(t, pool, "chk-a", "org-a", "default", "a.example:443", 300)
	b := seedCheck(t, pool, "chk-b", "org-b", "default", "b.example:443", 300)
	record(t, st, a, obsFor(t, a, "vtg_a", t0, leaf(t, "a.example", t0.Add(3*24*time.Hour))), t0)
	record(t, st, b, obsFor(t, b, "vtg_b", t0, leaf(t, "b.example", t0.Add(3*24*time.Hour))), t0)

	list, err := st.List(context.Background(), "org-a", "", t0)
	if err != nil || len(list) != 1 || list[0].CheckID != "chk-a" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err := st.Get(context.Background(), "org-a", "chk-b", t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign check must be not found: %v", err)
	}
	if _, err := st.Get(context.Background(), "org-a", "chk-none", t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing check: %v", err)
	}
	if ev := events(t, st, "org-a"); len(ev) != 1 || ev[0].CheckID != "chk-a" {
		t.Fatalf("events leaked across orgs: %+v", ev)
	}
	if _, err := st.List(context.Background(), "", "", t0); !errors.Is(err, ErrInput) {
		t.Fatal("an empty organization must be refused")
	}
	// The database refuses a row that claims an organization other than the check's.
	_, err = pool.Exec(context.Background(), `INSERT INTO f61_tls_results (check_id, org_id, namespace, endpoint, status,
		horizon_seconds, span_seconds, interval_sec, status_changed_at, evaluated_at, last_observed_at)
		VALUES ('chk-a','org-b','default','a.example:443','ok',1,1,300,now(),now(),now())
		ON CONFLICT (check_id) DO UPDATE SET org_id = EXCLUDED.org_id`)
	if err == nil {
		t.Fatal("a result row was allowed to move to another organization")
	}
}

func TestTargetChangeDiscardsOldObservations(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	s := seedCheck(t, pool, "chk-1", "org-a", "default", "old.example:443", 300)
	record(t, st, s, obsFor(t, s, "vtg_a", t0, leaf(t, "old.example", t0.Add(-time.Hour))), t0)
	if _, err := pool.Exec(context.Background(), `UPDATE synthetic_checks SET target='new.example:443' WHERE id='chk-1'`); err != nil {
		t.Fatal(err)
	}
	ns, _ := wire.SubjectFromCheck(wire.CheckRow{ID: "chk-1", OrgID: "org-a", Namespace: "default", Target: "new.example:443"})
	out := record(t, st, ns, obsFor(t, ns, "vtg_b", t0.Add(time.Minute), leaf(t, "new.example", t0.Add(90*24*time.Hour))), t0.Add(time.Minute))
	if out.Verdict.Status != evaluate.StatusOK || out.Verdict.VantagesInSpan != 1 {
		t.Fatalf("old-target observation still counted: %+v", out.Verdict)
	}
	var n int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM f61_tls_observations WHERE check_id='chk-1'`).Scan(&n)
	if n != 1 {
		t.Fatalf("obsolete rows kept: %d", n)
	}
}

func TestConcurrentReportsSerialise(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	s := seedCheck(t, pool, "chk-1", "org-a", "default", "svc.example:443", 300)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			at := t0.Add(time.Duration(i) * time.Second)
			o := obsFor(t, s, fmt.Sprintf("vtg_%02d", i), at, leaf(t, "svc.example", t0.Add(10*24*time.Hour)))
			if _, err := st.Record(context.Background(), Accepted{Subject: s, IntervalSec: 300, Observation: o, ReceivedAt: at}, horizon); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent record: %v", err)
	}
	r, _ := st.Get(context.Background(), "org-a", "chk-1", t0.Add(time.Minute))
	if r.VantagesInSpan != 16 || r.AlertState != AlertOpen || len(events(t, st, "org-a")) != 1 {
		t.Fatalf("after concurrency: in-span %d state %s events %d", r.VantagesInSpan, r.AlertState, len(events(t, st, "org-a")))
	}
}

func TestRevocation(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	st := New(pool)
	ctx := context.Background()
	if _, ok, err := st.RevokedBefore(ctx, "org-a", "vtg_a"); ok || err != nil {
		t.Fatal("no revocation expected")
	}
	nb, err := st.Revoke(ctx, "org-a", "vtg_a", t0, false, "rotated", "admin-1")
	if err != nil || !nb.Equal(t0) {
		t.Fatalf("revoke: %v %v", nb, err)
	}
	// An earlier bound never replaces a later one.
	nb, _ = st.Revoke(ctx, "org-a", "vtg_a", t0.Add(-time.Hour), false, "", "admin-1")
	if !nb.Equal(t0) {
		t.Fatalf("bound moved backwards: %v", nb)
	}
	nb, _ = st.Revoke(ctx, "org-a", "vtg_a", time.Time{}, true, "decommissioned", "admin-1")
	got, ok, _ := st.RevokedBefore(ctx, "org-a", "vtg_a")
	if !ok || !got.Equal(Decommissioned) || !nb.Equal(Decommissioned) {
		t.Fatalf("decommission: %v", got)
	}
	// Scoped by organization.
	if _, ok, _ := st.RevokedBefore(ctx, "org-b", "vtg_a"); ok {
		t.Fatal("revocation leaked across organizations")
	}
}

func TestRecordRefusesIncompleteInput(t *testing.T) {
	st := New(nil)
	if _, err := st.Record(context.Background(), Accepted{}, horizon); !errors.Is(err, ErrInput) {
		t.Fatalf("got %v", err)
	}
}
