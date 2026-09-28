package f61

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
)

// Runner is the probe service loop (PROPOSED FANOUT-1): it pulls work, runs
// each assignment once per due slot, and reports every observation.
//
// Due slots: for an assignment with interval I and phase P, slot n covers
// [n·I+P, (n+1)·I+P) in Unix seconds; the check is probed once per slot. On
// first sight of an assignment the current slot is skipped, so a restarted
// probe does not fire every check at once; the next slot boundary is at most
// one interval away and the correlation span tolerates one missed report.
//
// Fail closed: if work cannot be refreshed for StaleWorkAfter, or the
// processor rejects the credential, every assignment is dropped until work is
// fetched again. A probe never keeps probing on stale authorization.
type Runner struct {
	Client *Client
	Prober tlscert.Prober
	// Now defaults to time.Now.
	Now func() time.Time
	// MaxInFlight bounds concurrent probes (default 8).
	MaxInFlight int
	// StaleWorkAfter drops assignments when work has not been refreshed for
	// this long (default 10 minutes).
	StaleWorkAfter time.Duration
	// ReloadCredential, when set, is called after the processor rejects the
	// credential, so a rotated Secret is picked up without a restart.
	ReloadCredential func() (probetoken.Credential, error)
	// Log receives structured events. It never receives the credential.
	Log func(msg string, kv ...any)

	mu          sync.Mutex
	work        []Assignment
	lastSlot    map[string]int64
	ready       bool
	failures    int // consecutive failed refreshes, for backoff
	lastRefresh time.Time
	nextRefresh time.Time
	inFlight    int
	wg          sync.WaitGroup
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) log(msg string, kv ...any) {
	if r.Log != nil {
		r.Log(msg, kv...)
	}
}

// Run loops until ctx is cancelled, then waits for in-flight probes.
func (r *Runner) Run(ctx context.Context) error {
	if r.Client == nil || r.Prober == nil {
		return errors.New("probe/f61: runner needs a client and a prober")
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		r.Step(ctx)
		select {
		case <-ctx.Done():
			r.wg.Wait()
			return nil
		case <-t.C:
		}
	}
}

// Step performs one iteration: refresh work when due, then launch every
// assignment whose slot has started. It returns the number launched.
func (r *Runner) Step(ctx context.Context) int {
	now := r.now()
	r.mu.Lock()
	needRefresh := now.After(r.nextRefresh) || now.Equal(r.nextRefresh)
	r.mu.Unlock()
	if needRefresh {
		r.refresh(ctx, now)
	}

	max := r.MaxInFlight
	if max <= 0 {
		max = 8
	}
	launched := 0
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastSlot == nil {
		r.lastSlot = map[string]int64{}
	}
	for _, a := range r.work {
		slot := (now.Unix() - int64(a.PhaseSec)) / int64(a.IntervalSec)
		last, seen := r.lastSlot[a.CheckID]
		if !seen {
			r.lastSlot[a.CheckID] = slot // skip the current slot on first sight
			continue
		}
		if slot <= last || r.inFlight >= max {
			continue // not due, or deferred to the next step
		}
		r.lastSlot[a.CheckID] = slot
		r.inFlight++
		launched++
		r.wg.Add(1)
		go r.probe(ctx, a)
	}
	return launched
}

// Wait blocks until every launched probe has finished (used by tests and
// shutdown).
func (r *Runner) Wait() { r.wg.Wait() }

func (r *Runner) probe(ctx context.Context, a Assignment) {
	defer func() {
		r.mu.Lock()
		r.inFlight--
		r.mu.Unlock()
		r.wg.Done()
	}()
	obs, receipt, err := ProbeAndReport(ctx, r.Prober, r.Client, tlscert.Target{
		CheckID: a.CheckID, Endpoint: a.Endpoint, Timeout: time.Duration(a.TimeoutSec) * time.Second,
	})
	if err != nil {
		r.log("probe report failed", "check_id", a.CheckID, "outcome", string(obs.Outcome), "status", receipt.Status, "error", err.Error())
		if errors.Is(err, ErrCredentialRejected) {
			r.mu.Lock()
			r.dropWorkLocked("credential rejected")
			if r.failures == 0 {
				r.nextRefresh = time.Time{} // re-check (and reload) at once, then back off
			}
			r.mu.Unlock()
		}
		return
	}
	r.log("probe reported", "check_id", a.CheckID, "outcome", string(obs.Outcome), "stored", receipt.Stored,
		"result_status", receipt.ResultStatus)
}

func (r *Runner) refresh(ctx context.Context, now time.Time) {
	ws, status, err := r.Client.Work(ctx)
	if err != nil && errors.Is(err, ErrCredentialRejected) && r.ReloadCredential != nil {
		if cred, rerr := r.ReloadCredential(); rerr == nil {
			r.Client.SetCredential(cred)
			ws, status, err = r.Client.Work(ctx)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.failures++
		r.nextRefresh = now.Add(refreshBackoff(r.failures))
		stale := r.StaleWorkAfter
		if stale <= 0 {
			stale = 10 * time.Minute
		}
		switch {
		case errors.Is(err, ErrCredentialRejected):
			r.dropWorkLocked("credential rejected")
		case len(r.work) > 0 && now.Sub(r.lastRefresh) > stale:
			r.dropWorkLocked("work not refreshed within the stale bound")
		}
		r.log("work refresh failed", "status", status, "error", err.Error())
		return
	}
	r.work = ws.Assignments
	r.ready = true
	r.failures = 0
	r.lastRefresh = now
	r.nextRefresh = now.Add(time.Duration(ws.RefreshAfterSec) * time.Second)
	// Forget slots of checks no longer assigned.
	keep := map[string]bool{}
	for _, a := range ws.Assignments {
		keep[a.CheckID] = true
	}
	for id := range r.lastSlot {
		if !keep[id] {
			delete(r.lastSlot, id)
		}
	}
	if !ws.CredentialExpiresAt.IsZero() && ws.CredentialExpiresAt.Sub(now) < 24*time.Hour {
		r.log("probe credential expires soon", "expires_at", ws.CredentialExpiresAt.UTC().Format(time.RFC3339))
	}
	r.log("work refreshed", "assignments", len(ws.Assignments), "vantage_id", ws.VantageID)
}

func (r *Runner) dropWorkLocked(why string) {
	if len(r.work) > 0 {
		r.log("dropping all assignments", "reason", why)
	}
	r.work = nil
	r.ready = false
	r.lastSlot = map[string]int64{}
}

// refreshBackoff is the wait after n consecutive failed refreshes: 15 s,
// doubling, at most 5 minutes. A revoked or unreachable vantage therefore
// does not keep hammering the processor.
func refreshBackoff(n int) time.Duration {
	d := 15 * time.Second
	for i := 1; i < n && d < 5*time.Minute; i++ {
		d *= 2
	}
	if d > 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}

// Ready reports whether the runner holds work fetched within the stale bound
// with a credential the processor accepted. An empty assignment list is
// ready: the vantage simply has nothing to probe.
func (r *Runner) Ready() bool {
	now := r.now()
	stale := r.StaleWorkAfter
	if stale <= 0 {
		stale = 10 * time.Minute
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready && now.Sub(r.lastRefresh) <= stale
}

// Assignments returns a copy of the current assignments.
func (r *Runner) Assignments() []Assignment {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Assignment(nil), r.work...)
}
