package f61

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/observex/platform/internal/probetoken"
	wire "github.com/observex/platform/internal/wire/f61"
)

type fakeLister struct {
	rows []CheckRecord
	err  error
	org  string
}

func (f *fakeLister) ListChecks(_ context.Context, org string) ([]CheckRecord, error) {
	f.org = org
	return f.rows, f.err
}

func rec(id, org, ns, target string, locations []string, typ string, enabled bool, interval int) CheckRecord {
	return CheckRecord{Row: wire.CheckRow{ID: id, OrgID: org, Namespace: ns, Target: target}, Type: typ, Enabled: enabled,
		Locations: locations, IntervalSec: interval, TimeoutSec: 10}
}

func issueAt(t *testing.T, k probetoken.Key, org, zone string, at time.Time) (string, string) {
	t.Helper()
	out, err := k.Issue(probetoken.IssueRequest{OrgID: org, Declared: probetoken.Declaration{NetworkZone: zone}}, at, bytes.NewReader(bytes.Repeat([]byte{9}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential.Reveal(), out.VantageID
}

func TestAssignedRule(t *testing.T) {
	cases := []struct {
		locations []string
		zone      string
		want      bool
	}{
		{[]string{"*"}, "", true},
		{[]string{"*"}, "eu-west", true},
		{[]string{"eu-west"}, "eu-west", true},
		{[]string{"eu-west"}, "EU-WEST", false},
		{[]string{"eu-west"}, "us-east", false},
		{[]string{"eu-west"}, "", false},
		{[]string{"local"}, "local", false}, // local never matches a probe
		{[]string{"local"}, "eu-west", false},
		{nil, "eu-west", false},
		{[]string{}, "", false},
	}
	for _, tc := range cases {
		if got := Assigned(tc.locations, tc.zone); got != tc.want {
			t.Errorf("Assigned(%v, %q) = %v, want %v", tc.locations, tc.zone, got, tc.want)
		}
	}
	if !ReservedZone("local") || !ReservedZone("*") || ReservedZone("eu-west") {
		t.Fatal("reserved zones changed")
	}
}

func TestWorkReturnsOnlyAssignedChecksOfOwnOrg(t *testing.T) {
	k, l, _ := setup(t)
	a, _ := New(k, l, &fakeRevocations{}, func() time.Time { return t0.Add(time.Minute) })
	cred, vid := issueAt(t, k, "org-a", "eu-west", t0)
	lister := &fakeLister{rows: []CheckRecord{
		rec("c-zone", "org-a", "p", "a.example:443", []string{"eu-west"}, "ssl", true, 300),
		rec("c-all", "org-a", "p", "b.example", []string{"*"}, "ssl", true, 60),
		rec("c-other-zone", "org-a", "p", "c.example", []string{"us-east"}, "ssl", true, 60),
		rec("c-local", "org-a", "p", "d.example", []string{"local"}, "ssl", true, 60),
		rec("c-http", "org-a", "p", "https://e", []string{"*"}, "http", true, 60),
		rec("c-off", "org-a", "p", "f.example", []string{"*"}, "ssl", false, 60),
		rec("c-foreign", "org-b", "p", "g.example", []string{"*"}, "ssl", true, 60), // a broken lister must not leak
		rec("c-no-ns", "org-a", " ", "h.example", []string{"*"}, "ssl", true, 60),
		rec("c-bad-interval", "org-a", "p", "i.example", []string{"*"}, "ssl", true, 5),
	}}
	w, err := a.Work(context.Background(), "Bearer "+cred, lister)
	if err != nil {
		t.Fatal(err)
	}
	if lister.org != "org-a" || w.VantageID != vid || w.RefreshAfterSec != 60 || len(w.Assignments) != 2 {
		t.Fatalf("work: %+v (lister asked for %q)", w, lister.org)
	}
	if w.Assignments[0].CheckID != "c-all" || w.Assignments[1].CheckID != "c-zone" {
		t.Fatalf("assignments: %+v", w.Assignments)
	}
	for _, as := range w.Assignments {
		if as.PhaseSec < 0 || as.PhaseSec >= as.IntervalSec || as.PhaseSec != Phase(as.CheckID, vid, as.IntervalSec) {
			t.Fatalf("phase %+v", as)
		}
	}
}

func TestPhaseIsStableAndSpreads(t *testing.T) {
	if Phase("chk", "vtg_a", 300) != Phase("chk", "vtg_a", 300) {
		t.Fatal("phase is not stable")
	}
	seen := map[int]bool{}
	for _, v := range []string{"vtg_a", "vtg_b", "vtg_c", "vtg_d", "vtg_e", "vtg_f"} {
		seen[Phase("chk", v, 3600)] = true
	}
	if len(seen) < 4 {
		t.Fatal("phases do not spread across vantages")
	}
	if Phase("chk", "vtg_a", 0) != 0 {
		t.Fatal("zero interval must give phase 0")
	}
}

func TestRevocationIsEnforcedEverywhere(t *testing.T) {
	k, l, _ := setup(t)
	rev := &fakeRevocations{bounds: map[string]time.Time{}}
	a, _ := New(k, l, rev, func() time.Time { return t0.Add(2 * time.Hour) })
	old, vid := issueAt(t, k, "org-a", "", t0)
	newer, _ := probetoken.Key(k).Issue(probetoken.IssueRequest{OrgID: "org-a", VantageID: vid}, t0.Add(time.Hour), nil)

	rev.bounds["org-a/"+vid] = t0.Add(time.Hour) // rotation: everything issued before the new credential
	l.calls = 0
	if _, err := a.Authorize(context.Background(), "Bearer "+old, "chk-a"); !errors.Is(err, ErrUnauthenticated) || !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked credential accepted: %v", err)
	}
	if l.calls != 0 {
		t.Fatal("a check row was read for a revoked credential")
	}
	if _, err := a.Work(context.Background(), "Bearer "+old, &fakeLister{}); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked credential got work: %v", err)
	}
	if _, err := a.Authorize(context.Background(), "Bearer "+newer.Credential.Reveal(), "chk-a"); err != nil {
		t.Fatalf("credential issued at the bound refused: %v", err)
	}
	// Decommissioned: nothing passes.
	rev.bounds["org-a/"+vid] = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	if _, err := a.Authorize(context.Background(), "Bearer "+newer.Credential.Reveal(), "chk-a"); !errors.Is(err, ErrRevoked) {
		t.Fatalf("decommissioned vantage accepted: %v", err)
	}
	// Unreadable revocations fail closed.
	rev.err = errors.New("db down")
	if _, err := a.Authorize(context.Background(), "Bearer "+newer.Credential.Reveal(), "chk-a"); !errors.Is(err, ErrUnavailable) || StatusCode(err) != 503 {
		t.Fatalf("revocation failure did not fail closed: %v", err)
	}
}

func TestUnassignedCheckIsForbiddenLikeMissing(t *testing.T) {
	k, l, _ := setup(t)
	l.rows["chk-zone"] = rec("chk-zone", "org-a", "p", "api.example.com:443", []string{"us-east"}, "ssl", true, 300)
	a, _ := New(k, l, &fakeRevocations{}, func() time.Time { return t0.Add(time.Minute) })
	cred, _ := issueAt(t, k, "org-a", "eu-west", t0)
	_, e1 := a.Authorize(context.Background(), "Bearer "+cred, "chk-zone")
	_, e2 := a.Authorize(context.Background(), "Bearer "+cred, "chk-missing")
	if !errors.Is(e1, ErrNotAssigned) || StatusCode(e1) != StatusCode(e2) || Reason(e1) != Reason(e2) {
		t.Fatalf("unassigned %v vs missing %v", e1, e2)
	}
	if _, err := a.Work(context.Background(), "Bearer "+cred, &fakeLister{err: errors.New("db down")}); StatusCode(err) != 503 {
		t.Fatalf("work lister failure: %v", err)
	}
}

func TestReplayWindow(t *testing.T) {
	k, _, _, in := setupIntake(t) // intake clock: t0 + 1 min
	cred, _ := credential(t, k, "org-a", probetoken.Declaration{})
	now := t0.Add(time.Minute)
	for _, tc := range []struct {
		at   time.Time
		want bool
	}{
		{now, true},
		{now.Add(-MaxReportDelay), true},
		{now.Add(-MaxReportDelay - time.Second), false},
		{now.Add(ClockSkew), true},
		{now.Add(ClockSkew + time.Second), false},
	} {
		body := timeoutPayload(t, func(m map[string]any) { m["observed_at"] = tc.at.Format(time.RFC3339Nano) })
		_, _, err := in.Accept(context.Background(), "Bearer "+cred, "chk-a", body)
		if (err == nil) != tc.want {
			t.Fatalf("observed_at %v: err %v, want accepted=%v", tc.at, err, tc.want)
		}
		if err != nil && (Reason(err) != "observation_time_out_of_range" || StatusCode(err) != 400) {
			t.Fatalf("reason %q status %d", Reason(err), StatusCode(err))
		}
	}
}
