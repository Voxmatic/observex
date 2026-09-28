package certexpiry

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// Fixed inputs. Nothing here reads a clock.
var (
	now       = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	horizon   = 720 * time.Hour // 30 days; supplied by the caller, never defaulted
	maxAge    = 15 * time.Minute
	observed  = now.Add(-5 * time.Minute)
	endpoint  = "api.example.com:443"
	checkID   = "chk-001"
	okParams  = Params{Horizon: horizon, MaxObservationAge: maxAge}
	baseObs   = Observation{CheckID: checkID, Endpoint: endpoint, NotAfter: now.Add(365 * 24 * time.Hour), ObservedAt: observed}
	oneSecond = time.Second
)

// obsRemaining returns an observation whose certificate expires exactly
// `remaining` after `now`.
func obsRemaining(remaining time.Duration) Observation {
	o := baseObs
	o.NotAfter = now.Add(remaining)
	return o
}

// ── Decision cases 1–8 ───────────────────────────────────────────────────────

func TestEvaluateHorizonDecision(t *testing.T) {
	cases := []struct {
		name      string
		remaining time.Duration
		want      bool
	}{
		{"1: far beyond horizon (365 days)", 365 * 24 * time.Hour, false},
		{"2: horizon + 1ns", horizon + 1, false},
		{"3: exactly the horizon (inclusive boundary)", horizon, true},
		{"4: horizon - 1ns", horizon - 1, true},
		{"5: 1ns remaining", 1, true},
		{"6: notAfter equals now", 0, true},
		{"7: expired by 1ns", -1, true},
		{"8: expired by 30 days", -30 * 24 * time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := obsRemaining(tc.remaining)
			f, found, err := Evaluate(now, obs, okParams)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if found != tc.want {
				t.Fatalf("found = %v, want %v (remaining %v, horizon %v)", found, tc.want, tc.remaining, horizon)
			}
			if !found {
				if !reflect.DeepEqual(f, Finding{}) {
					t.Fatalf("no finding, but a non-zero Finding was returned: %+v", f)
				}
				return
			}
			if f.Remaining != tc.remaining {
				t.Errorf("Remaining = %v, want %v", f.Remaining, tc.remaining)
			}
			if tc.remaining < 0 && f.Remaining >= 0 {
				t.Errorf("expired certificate reported non-negative remaining time %v", f.Remaining)
			}
		})
	}
}

// ── Unusable input, cases 9–16 (plus the documented zero-now choice) ─────────

func TestEvaluateUnusableInput(t *testing.T) {
	cases := []struct {
		name    string
		now     time.Time
		obs     Observation
		params  Params
		wantErr error
	}{
		{"9: zero notAfter", now, func() Observation { o := baseObs; o.NotAfter = time.Time{}; return o }(), okParams, ErrNoNotAfter},
		{"10: zero observedAt", now, func() Observation { o := baseObs; o.ObservedAt = time.Time{}; return o }(), okParams, ErrNoObservedAt},
		{"11: observation in the future", now, func() Observation { o := baseObs; o.ObservedAt = now.Add(oneSecond); return o }(), okParams, ErrObservationInFuture},
		{"12: observation stale by 1ns", now, func() Observation { o := baseObs; o.ObservedAt = now.Add(-maxAge - 1); return o }(), okParams, ErrObservationStale},
		{"14: empty endpoint", now, func() Observation { o := baseObs; o.Endpoint = ""; return o }(), okParams, ErrNoEndpoint},
		{"15a: zero horizon", now, baseObs, Params{Horizon: 0, MaxObservationAge: maxAge}, ErrHorizonNotSet},
		{"15b: negative horizon", now, baseObs, Params{Horizon: -time.Hour, MaxObservationAge: maxAge}, ErrHorizonNotSet},
		{"16a: zero max age", now, baseObs, Params{Horizon: horizon, MaxObservationAge: 0}, ErrMaxAgeNotSet},
		{"16b: negative max age", now, baseObs, Params{Horizon: horizon, MaxObservationAge: -time.Minute}, ErrMaxAgeNotSet},
		{"implementation choice: zero evaluation time", time.Time{}, baseObs, okParams, ErrNoEvaluationTime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, found, err := Evaluate(tc.now, tc.obs, tc.params)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if found {
				t.Error("found = true for unusable input")
			}
			if !reflect.DeepEqual(f, Finding{}) {
				t.Errorf("non-zero Finding returned with an error: %+v", f)
			}
		})
	}
}

// 13: an observation exactly at the freshness bound is accepted.
func TestEvaluateObservationAgeBoundaryIsInclusive(t *testing.T) {
	obs := obsRemaining(time.Hour) // inside the horizon, so a finding is expected
	obs.ObservedAt = now.Add(-maxAge)
	f, found, err := Evaluate(now, obs, okParams)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v; want a finding at exactly MaxObservationAge", found, err)
	}
	if f.ObservationAge != maxAge {
		t.Errorf("ObservationAge = %v, want %v", f.ObservationAge, maxAge)
	}
}

// ── Representation and determinism, cases 17–20 ──────────────────────────────

// 17: the same instant expressed in a different zone yields the same finding.
func TestEvaluateIsTimezoneIndependent(t *testing.T) {
	kolkata, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Skipf("zone database unavailable: %v", err)
	}
	obs := obsRemaining(48 * time.Hour)

	shifted := obs
	shifted.NotAfter = obs.NotAfter.In(kolkata)
	shifted.ObservedAt = obs.ObservedAt.In(kolkata)

	utcFinding, foundUTC, errUTC := Evaluate(now, obs, okParams)
	zoneFinding, foundZone, errZone := Evaluate(now.In(kolkata), shifted, okParams)

	if errUTC != nil || errZone != nil || !foundUTC || !foundZone {
		t.Fatalf("both evaluations must produce findings: %v/%v, %v/%v", foundUTC, errUTC, foundZone, errZone)
	}
	if !reflect.DeepEqual(utcFinding, zoneFinding) {
		t.Fatalf("zone changed the finding:\n utc  = %+v\n zone = %+v", utcFinding, zoneFinding)
	}
	for name, ts := range map[string]time.Time{
		"NotAfter": zoneFinding.NotAfter, "ObservedAt": zoneFinding.ObservedAt, "EvaluatedAt": zoneFinding.EvaluatedAt,
	} {
		if ts.Location() != time.UTC {
			t.Errorf("%s is in %v, want UTC", name, ts.Location())
		}
	}
}

// 18: a monotonic reading on the evaluation time changes nothing.
func TestEvaluateIgnoresMonotonicReading(t *testing.T) {
	mono := time.Now()    // carries a monotonic reading
	wall := mono.Round(0) // same instant, monotonic stripped
	obs := Observation{CheckID: checkID, Endpoint: endpoint, NotAfter: wall.Add(time.Hour), ObservedAt: wall.Add(-time.Minute)}

	a, foundA, errA := Evaluate(mono, obs, okParams)
	b, foundB, errB := Evaluate(wall, obs, okParams)
	if errA != nil || errB != nil || !foundA || !foundB {
		t.Fatalf("both evaluations must produce findings: %v/%v, %v/%v", foundA, errA, foundB, errB)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("monotonic reading changed the finding:\n mono = %+v\n wall = %+v", a, b)
	}
}

// 19: identical inputs produce identical outputs, every time.
func TestEvaluateIsDeterministic(t *testing.T) {
	obs := obsRemaining(12 * time.Hour)
	first, found, err := Evaluate(now, obs, okParams)
	if err != nil || !found {
		t.Fatalf("setup: found = %v, err = %v", found, err)
	}
	for i := 0; i < 100; i++ {
		got, gotFound, gotErr := Evaluate(now, obs, okParams)
		if gotErr != nil || !gotFound || !reflect.DeepEqual(got, first) {
			t.Fatalf("iteration %d differed: %+v (found %v, err %v)", i, got, gotFound, gotErr)
		}
	}
}

// 20: a returned finding is a value; mutating it cannot affect a later call.
func TestFindingIsIndependentValue(t *testing.T) {
	obs := obsRemaining(time.Hour)
	first, _, _ := Evaluate(now, obs, okParams)
	mutated := first
	mutated.Endpoint = "MUTATED"
	mutated.Remaining = 0
	mutated.PatternID = "MUTATED"

	second, found, err := Evaluate(now, obs, okParams)
	if err != nil || !found {
		t.Fatalf("second evaluation: found = %v, err = %v", found, err)
	}
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("mutating a returned finding affected a later call:\n first  = %+v\n second = %+v", first, second)
	}
}

// ── Evidence correctness ─────────────────────────────────────────────────────

func TestFindingEvidenceFields(t *testing.T) {
	obs := obsRemaining(240 * time.Hour) // 10 days, inside the 30-day horizon
	f, found, err := Evaluate(now, obs, okParams)
	if err != nil || !found {
		t.Fatalf("found = %v, err = %v", found, err)
	}
	if !f.NotAfter.Equal(obs.NotAfter) {
		t.Errorf("NotAfter = %v, want %v", f.NotAfter, obs.NotAfter)
	}
	if !f.ObservedAt.Equal(obs.ObservedAt) {
		t.Errorf("ObservedAt = %v, want %v", f.ObservedAt, obs.ObservedAt)
	}
	if !f.EvaluatedAt.Equal(now) {
		t.Errorf("EvaluatedAt = %v, want %v", f.EvaluatedAt, now)
	}
	if want := obs.NotAfter.Sub(now); f.Remaining != want {
		t.Errorf("Remaining = %v, want %v", f.Remaining, want)
	}
	if want := now.Sub(obs.ObservedAt); f.ObservationAge != want {
		t.Errorf("ObservationAge = %v, want %v", f.ObservationAge, want)
	}
	if f.Horizon != horizon {
		t.Errorf("Horizon = %v, want %v", f.Horizon, horizon)
	}
	if f.CheckID != checkID || f.Endpoint != endpoint {
		t.Errorf("subject = %q/%q, want %q/%q", f.CheckID, f.Endpoint, checkID, endpoint)
	}
	// Arithmetic must be exact, not rounded to days or seconds.
	odd := obsRemaining(horizon - 1)
	g, _, _ := Evaluate(now, odd, okParams)
	if g.Remaining != horizon-1 {
		t.Errorf("Remaining = %v, want exactly %v", g.Remaining, horizon-1)
	}
}

// The finding carries no severity, confidence, score or action — enforced
// structurally, so adding such a field breaks this test.
func TestFindingHasNoDecisionFields(t *testing.T) {
	want := []string{
		"PatternID", "PatternVersion", "CatalogSHA256", "PredictionClass",
		"CheckID", "Endpoint",
		"NotAfter", "ObservedAt", "EvaluatedAt", "Remaining", "ObservationAge", "Horizon",
	}
	tp := reflect.TypeOf(Finding{})
	got := make([]string, 0, tp.NumField())
	for i := 0; i < tp.NumField(); i++ {
		got = append(got, tp.Field(i).Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Finding fields = %v, want exactly %v", got, want)
	}
}

// There is no default horizon anywhere: the zero Params cannot evaluate, and
// no exported identifier offers a ready-made horizon or the processor's 7 days.
func TestNoDefaultHorizon(t *testing.T) {
	if _, found, err := Evaluate(now, baseObs, Params{}); found || !errors.Is(err, ErrHorizonNotSet) {
		t.Fatalf("zero Params: found = %v, err = %v; want ErrHorizonNotSet", found, err)
	}
	if _, found, err := Evaluate(now, obsRemaining(6*24*time.Hour), Params{MaxObservationAge: maxAge}); found || !errors.Is(err, ErrHorizonNotSet) {
		t.Fatalf("a certificate 6 days out must not fire without a horizon: found = %v, err = %v", found, err)
	}
}
