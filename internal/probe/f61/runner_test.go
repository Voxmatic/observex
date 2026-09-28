package f61

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
)

func TestPublicAddress(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "::1": false, "fe80::1": false,
		"fd00::1": false, "::ffff:10.0.0.1": false, "::ffff:8.8.8.8": true, "64:ff9b::a00:1": false,
		"224.0.0.1": false, "255.255.255.255": false, "198.18.0.1": false,
	} {
		if got := PublicAddress(netip.MustParseAddr(addr)); got != want {
			t.Errorf("PublicAddress(%s) = %v, want %v", addr, got, want)
		}
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func TestGuardedDialer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	res := fakeResolver{"internal.example": {netip.MustParseAddr("127.0.0.1")}}

	blocked := GuardedDialer{Resolver: res}
	for _, addr := range []string{"127.0.0.1:" + port, "internal.example:" + port} {
		if _, err := blocked.Dial(context.Background(), "tcp", addr); !errors.Is(err, ErrBlockedDestination) {
			t.Fatalf("%s: private destination not refused: %v", addr, err)
		}
	}
	allowed := GuardedDialer{Resolver: res, AllowPrivate: true}
	c, err := allowed.Dial(context.Background(), "tcp", "internal.example:"+port)
	if err != nil {
		t.Fatalf("allowed private dial: %v", err)
	}
	c.Close()

	// Through tlscert, a refused destination is a transport failure, and the
	// observation still names the configured endpoint.
	p := tlscert.New(tlscert.Config{Dial: blocked.Dial})
	obs, err := p.Probe(context.Background(), tlscert.Target{CheckID: "c", Endpoint: "internal.example:" + port, Timeout: time.Second})
	if err != nil || obs.Outcome != tlscert.TransportFailure || obs.Endpoint != "internal.example:"+port {
		t.Fatalf("observation %+v, %v", obs, err)
	}
}

// fakeProcessor serves work and records reports.
type fakeProcessor struct {
	mu        sync.Mutex
	work      WorkSet
	workCode  int
	acceptOK  string // credential accepted
	reports   atomic.Int64
	workCalls atomic.Int64
}

func (f *fakeProcessor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path == "/v1/synthetic/probe/work" {
		f.workCalls.Add(1) // every attempt, accepted or not
	}
	if r.Header.Get("Authorization") != "Bearer "+f.acceptOK {
		w.WriteHeader(401)
		return
	}
	if r.URL.Path == "/v1/synthetic/probe/work" {
		if f.workCode != 0 && f.workCode != 200 {
			w.WriteHeader(f.workCode)
			return
		}
		_ = json.NewEncoder(w).Encode(f.work)
		return
	}
	f.reports.Add(1)
	w.WriteHeader(202)
	_, _ = w.Write([]byte(`{"status":"accepted","stored":true,"evaluated":true,"result_status":"ok"}`))
}

// fixedProber returns a transport failure instantly: a valid observation.
type fixedProber struct{ calls atomic.Int64 }

func (p *fixedProber) Probe(_ context.Context, t tlscert.Target) (tlscert.Observation, error) {
	p.calls.Add(1)
	return tlscert.New(tlscert.Config{Dial: func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("refused")
	}}).Probe(context.Background(), t)
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

func newRunner(t *testing.T, fp *fakeProcessor, cred probetoken.Credential, clk *clock) (*Runner, *fixedProber) {
	t.Helper()
	srv := httptest.NewServer(fp)
	t.Cleanup(srv.Close)
	c, err := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: cred, AllowPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	pr := &fixedProber{}
	return &Runner{Client: c, Prober: pr, Now: clk.now}, pr
}

func TestRunnerDueSlots(t *testing.T) {
	cred := testCredential(t)
	base := time.Unix(1_800_000_000, 0).UTC() // a slot boundary for interval 60, phase 0
	fp := &fakeProcessor{acceptOK: cred.Reveal(), work: WorkSet{RefreshAfterSec: 60, Assignments: []Assignment{
		{CheckID: "a", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5, PhaseSec: 0},
		{CheckID: "b", Endpoint: "b.example", IntervalSec: 60, TimeoutSec: 5, PhaseSec: 30},
	}}}
	clk := &clock{t: base.Add(10 * time.Second)}
	r, pr := newRunner(t, fp, cred, clk)
	ctx := context.Background()

	if n := r.Step(ctx); n != 0 { // first sight: current slots skipped
		t.Fatalf("launched %d on first sight", n)
	}
	clk.set(base.Add(35 * time.Second)) // b's next slot (phase 30) has started
	if n := r.Step(ctx); n != 1 {
		t.Fatalf("launched %d, want 1 (b)", n)
	}
	r.Wait()
	if n := r.Step(ctx); n != 0 {
		t.Fatal("same slot launched twice")
	}
	clk.set(base.Add(61 * time.Second)) // a's next slot
	if n := r.Step(ctx); n != 1 {
		t.Fatalf("launched %d, want 1 (a)", n)
	}
	r.Wait()
	if pr.calls.Load() != 2 || fp.reports.Load() != 2 {
		t.Fatalf("probes %d reports %d", pr.calls.Load(), fp.reports.Load())
	}
}

func TestRunnerBoundsConcurrency(t *testing.T) {
	cred := testCredential(t)
	base := time.Unix(1_800_000_000, 0).UTC()
	var as []Assignment
	for _, id := range []string{"a", "b", "c", "d"} {
		as = append(as, Assignment{CheckID: id, Endpoint: id + ".example", IntervalSec: 60, TimeoutSec: 5})
	}
	fp := &fakeProcessor{acceptOK: cred.Reveal(), work: WorkSet{RefreshAfterSec: 600, Assignments: as}}
	clk := &clock{t: base.Add(time.Second)}
	r, _ := newRunner(t, fp, cred, clk)
	r.MaxInFlight = 2
	r.Step(context.Background())
	clk.set(base.Add(61 * time.Second))
	r.mu.Lock()
	r.inFlight = 1 // one probe already running
	r.mu.Unlock()
	if n := r.Step(context.Background()); n != 1 {
		t.Fatalf("launched %d with one slot free", n)
	}
	r.mu.Lock()
	r.inFlight--
	r.mu.Unlock()
	r.Wait()
	// Three deferred, two slots free: two now, the last on the step after.
	if n := r.Step(context.Background()); n != 2 {
		t.Fatalf("deferred assignments launched on the next step: %d, want 2", n)
	}
	r.Wait()
	if n := r.Step(context.Background()); n != 1 {
		t.Fatalf("last deferred assignment launched: %d, want 1", n)
	}
	r.Wait()
	if n := r.Step(context.Background()); n != 0 {
		t.Fatalf("slot launched twice: %d", n)
	}
}

func TestRunnerFailsClosed(t *testing.T) {
	cred := testCredential(t)
	base := time.Unix(1_800_000_000, 0).UTC()
	fp := &fakeProcessor{acceptOK: cred.Reveal(), work: WorkSet{RefreshAfterSec: 60, Assignments: []Assignment{
		{CheckID: "a", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5},
	}}}
	clk := &clock{t: base}
	r, _ := newRunner(t, fp, cred, clk)
	if r.Ready() {
		t.Fatal("ready before any work was fetched")
	}
	r.Step(context.Background())
	if len(r.Assignments()) != 1 || !r.Ready() {
		t.Fatal("work not loaded")
	}
	// The processor becomes unavailable: work is kept until the stale bound…
	fp.mu.Lock()
	fp.workCode = 503
	fp.mu.Unlock()
	clk.set(base.Add(2 * time.Minute))
	r.Step(context.Background())
	if len(r.Assignments()) != 1 {
		t.Fatal("work dropped before the stale bound")
	}
	// …and dropped after it.
	clk.set(base.Add(11 * time.Minute))
	r.Step(context.Background())
	if len(r.Assignments()) != 0 || r.Ready() {
		t.Fatal("stale work kept")
	}
	// A rejected credential drops work immediately.
	fp.mu.Lock()
	fp.workCode = 0
	fp.mu.Unlock()
	clk.set(base.Add(12 * time.Minute))
	r.Step(context.Background())
	if len(r.Assignments()) != 1 {
		t.Fatal("work not restored")
	}
	fp.mu.Lock()
	fp.acceptOK = "revoked"
	fp.mu.Unlock()
	clk.set(base.Add(14 * time.Minute))
	r.Step(context.Background())
	if len(r.Assignments()) != 0 || r.Ready() {
		t.Fatal("work kept after the credential was rejected")
	}
	// A rejected vantage backs off instead of retrying every step.
	calls := fp.workCalls.Load()
	for i := 1; i <= 14; i++ {
		clk.set(base.Add(14*time.Minute + time.Duration(i)*time.Second))
		r.Step(context.Background())
	}
	if got := fp.workCalls.Load() - calls; got != 0 {
		t.Fatalf("%d work requests within the backoff", got)
	}
	clk.set(base.Add(14*time.Minute + 16*time.Second))
	r.Step(context.Background())
	if got := fp.workCalls.Load() - calls; got != 1 {
		t.Fatalf("%d work requests after the backoff, want 1", got)
	}
}

func TestRefreshBackoff(t *testing.T) {
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i, w := range want {
		if got := refreshBackoff(i + 1); got != w {
			t.Errorf("refreshBackoff(%d) = %v, want %v", i+1, got, w)
		}
	}
	if refreshBackoff(1000) != 5*time.Minute {
		t.Error("backoff not capped")
	}
}

// A report rejected for the credential drops work and re-checks once at
// once, which is how a rotated Secret is picked up promptly.
func TestRejectedReportRechecksImmediately(t *testing.T) {
	cred := testCredential(t)
	base := time.Unix(1_800_000_000, 0).UTC()
	fp := &fakeProcessor{acceptOK: cred.Reveal(), work: WorkSet{RefreshAfterSec: 600, Assignments: []Assignment{
		{CheckID: "a", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5},
	}}}
	clk := &clock{t: base.Add(time.Second)}
	r, _ := newRunner(t, fp, cred, clk)
	r.Step(context.Background())
	fp.mu.Lock()
	fp.acceptOK = "revoked"
	fp.mu.Unlock()
	clk.set(base.Add(61 * time.Second))
	if n := r.Step(context.Background()); n != 1 {
		t.Fatalf("launched %d", n)
	}
	r.Wait()
	if len(r.Assignments()) != 0 {
		t.Fatal("work kept after a rejected report")
	}
	calls := fp.workCalls.Load()
	r.Step(context.Background())
	if fp.workCalls.Load() != calls+1 {
		t.Fatal("no immediate re-check after a rejected report")
	}
}

func TestRunnerReloadsRotatedCredential(t *testing.T) {
	oldCred := testCredential(t)
	rotated, err := probetoken.NewKey(strings.Repeat("k", 40)).Issue(probetoken.IssueRequest{OrgID: "org-a"},
		testNow.Add(time.Hour), bytes.NewReader(bytes.Repeat([]byte{2}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	newCred := rotated.Credential
	if oldCred.Reveal() == newCred.Reveal() {
		t.Fatal("rotated credential must differ")
	}
	fp := &fakeProcessor{acceptOK: newCred.Reveal(), work: WorkSet{RefreshAfterSec: 60, Assignments: []Assignment{
		{CheckID: "a", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5},
	}}}
	clk := &clock{t: time.Unix(1_800_000_000, 0)}
	r, _ := newRunner(t, fp, oldCred, clk)
	r.ReloadCredential = func() (probetoken.Credential, error) { return newCred, nil }
	r.Step(context.Background())
	if len(r.Assignments()) != 1 || fp.workCalls.Load() != 2 {
		t.Fatalf("rotated credential not picked up: assignments %d calls %d", len(r.Assignments()), fp.workCalls.Load())
	}
}

func TestWorkDropsMalformedAssignments(t *testing.T) {
	cred := testCredential(t)
	fp := &fakeProcessor{acceptOK: cred.Reveal(), work: WorkSet{RefreshAfterSec: 5, Assignments: []Assignment{
		{CheckID: "ok", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5, PhaseSec: 59},
		{CheckID: "", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5},
		{CheckID: "fast", Endpoint: "a.example", IntervalSec: 1, TimeoutSec: 5},
		{CheckID: "phase", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 5, PhaseSec: 60},
		{CheckID: "timeout", Endpoint: "a.example", IntervalSec: 60, TimeoutSec: 600},
	}}}
	srv := httptest.NewServer(fp)
	defer srv.Close()
	c, _ := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: cred, AllowPlaintext: true})
	ws, status, err := c.Work(context.Background())
	if err != nil || status != 200 || len(ws.Assignments) != 1 || ws.Assignments[0].CheckID != "ok" || ws.RefreshAfterSec != 60 {
		t.Fatalf("work %+v status %d err %v", ws, status, err)
	}
	other, err := probetoken.NewKey(strings.Repeat("x", 40)).Issue(probetoken.IssueRequest{OrgID: "org-a"}, testNow,
		bytes.NewReader(bytes.Repeat([]byte{3}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	bad, _ := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: other.Credential, AllowPlaintext: true})
	if _, _, err := bad.Work(context.Background()); !errors.Is(err, ErrCredentialRejected) {
		t.Fatalf("rejected credential: %v", err)
	}
	if strings.Contains(c.WorkURL(), "?") {
		t.Fatal("work URL carries a query")
	}
}
