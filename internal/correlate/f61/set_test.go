package f61

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/observex/platform/internal/adapt/tlscertexpiry"
	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// Fixed times and caller-supplied bounds. Nothing here reads the real clock
// for a decision: the observer's clock is injected, the evaluation time is
// passed in, and the span is passed in.
var (
	fixedNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	span     = 5 * time.Minute
	params   = certexpiry.Params{Horizon: 720 * time.Hour, MaxObservationAge: 15 * time.Minute}
)

const (
	testHost     = "observex.test"
	testEndpoint = "observex.test:443"
	testCheckID  = "chk-91ab"
)

// subject is the approved envelope, built only by wire.SubjectFromCheck.
func subject(t *testing.T) wire.Subject {
	t.Helper()
	s, err := wire.SubjectFromCheck(wire.CheckRow{
		OrgID:     "org-7f3c",
		Namespace: "payments",
		ID:        testCheckID,
		Target:    testEndpoint,
	})
	if err != nil {
		t.Fatalf("SubjectFromCheck: %v", err)
	}
	return s
}

// ── fixtures: real handshakes over net.Pipe, no sockets ─────────────────────

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

func newCA(t *testing.T, cn string) *authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             fixedNow.Add(-365 * 24 * time.Hour),
		NotAfter:              fixedNow.Add(365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &authority{cert: cert, key: key, der: der}
}

func newLeaf(t *testing.T, issuer *authority, notAfter time.Time) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: testHost},
		NotBefore:    fixedNow.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{testHost},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer.cert, &key.PublicKey, issuer.key)
	if err != nil {
		t.Fatal(err)
	}
	return der, key
}

// pipeDial serves a TLS handshake on the far end of an in-memory pipe.
func pipeDial(t *testing.T, chain [][]byte, key *ecdsa.PrivateKey) tlscert.DialFunc {
	t.Helper()
	cfg := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: chain, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
	return func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			tc := tls.Server(server, cfg)
			_ = tc.Handshake()
			buf := make([]byte, 1)
			_, _ = tc.Read(buf)
			_ = tc.Close()
		}()
		return client, nil
	}
}

// observeLeaf mints a leaf, serves it and returns the observation plus the
// result of adapting it, both produced by the frozen packages themselves.
func observeLeaf(t *testing.T, v tlscert.Vantage, notAfter time.Time, opts ...func(*tlscert.Target)) (tlscert.Observation, tlscertexpiry.Result) {
	t.Helper()
	ca := newCA(t, "correlate-test-ca")
	der, key := newLeaf(t, ca, notAfter)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)

	p := tlscert.New(tlscert.Config{
		Dial:    pipeDial(t, [][]byte{der, ca.der}, key),
		Now:     func() time.Time { return fixedNow },
		Roots:   roots,
		Vantage: v,
	})
	target := tlscert.Target{CheckID: testCheckID, Endpoint: testEndpoint, Timeout: 5 * time.Second}
	for _, o := range opts {
		o(&target)
	}
	obs, err := p.Probe(context.Background(), target)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	res, err := tlscertexpiry.Adapt(fixedNow, obs, params)
	if err != nil {
		// A non-certificate observation legitimately yields no result.
		return obs, tlscertexpiry.Result{}
	}
	return obs, res
}

// observeTimeout produces a genuine target-side timeout observation: the far
// end of the pipe never speaks TLS, so the handshake exhausts the budget.
func observeTimeout(t *testing.T, v tlscert.Vantage) tlscert.Observation {
	t.Helper()
	p := tlscert.New(tlscert.Config{
		Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = server.Close(); _ = client.Close() })
			return client, nil
		},
		Now:     func() time.Time { return fixedNow },
		Vantage: v,
	})
	obs, err := p.Probe(context.Background(), tlscert.Target{
		CheckID: testCheckID, Endpoint: testEndpoint, Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if obs.Outcome != tlscert.Timeout {
		t.Fatalf("fixture did not produce a target timeout: outcome = %q", obs.Outcome)
	}
	return obs
}

func newSet(t *testing.T) Set {
	t.Helper()
	s, err := New(subject(t), span)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// ── 1. the Subject itself is the correlation key ────────────────────────────

func TestSubjectIsTheCorrelationKeyAndIsUsableAsAMapKey(t *testing.T) {
	sub := subject(t)
	s := newSet(t)
	if s.Subject() != sub {
		t.Fatalf("Subject() = %+v, want %+v", s.Subject(), sub)
	}

	// Comparable, therefore a map key with no string composition.
	byKey := map[wire.Subject]Set{sub: s}
	if _, ok := byKey[sub]; !ok {
		t.Fatal("the subject does not round-trip as a map key")
	}

	// Every field discriminates: five variants, five distinct keys.
	variants := []wire.CheckRow{
		{OrgID: "org-OTHER", Namespace: "payments", ID: testCheckID, Target: testEndpoint},
		{OrgID: "org-7f3c", Namespace: "OTHER", ID: testCheckID, Target: testEndpoint},
		{OrgID: "org-7f3c", Namespace: "payments", ID: "chk-OTHER", Target: testEndpoint},
		{OrgID: "org-7f3c", Namespace: "payments", ID: testCheckID, Target: "other.test:443"},
	}
	for _, row := range variants {
		other, err := wire.SubjectFromCheck(row)
		if err != nil {
			t.Fatal(err)
		}
		if other == sub {
			t.Errorf("subject %+v is not distinguished from %+v", other, sub)
		}
		if _, ok := byKey[other]; ok {
			t.Errorf("subject %+v collided with the original key", other)
		}
	}
}

func TestNewRejectsAnUnusableSubjectAndAnAbsentSpan(t *testing.T) {
	sub := subject(t)
	if _, err := New(wire.Subject{}, span); !errors.Is(err, ErrNoSubject) {
		t.Errorf("zero subject: err = %v, want ErrNoSubject", err)
	}
	partial := sub
	partial.OrgID = ""
	if _, err := New(partial, span); !errors.Is(err, ErrNoSubject) {
		t.Errorf("blank org: err = %v, want ErrNoSubject", err)
	}
	partial = sub
	partial.Namespace = "  "
	if _, err := New(partial, span); !errors.Is(err, ErrNoSubject) {
		t.Errorf("blank namespace: err = %v, want ErrNoSubject", err)
	}
	for _, bad := range []time.Duration{0, -time.Second} {
		if _, err := New(sub, bad); !errors.Is(err, ErrNoSpan) {
			t.Errorf("span %v: err = %v, want ErrNoSpan", bad, err)
		}
	}
}

// ── 2-5. tenancy and subject cannot vary inside one set ─────────────────────

func TestTenancyCannotVaryWithinASet(t *testing.T) {
	s := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{Kind: "test", ID: "v1"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obs, res, fixedNow); err != nil {
		t.Fatal(err)
	}
	obs2, res2 := observeLeaf(t, tlscert.Vantage{Kind: "test", ID: "v2"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obs2, res2, fixedNow); err != nil {
		t.Fatal(err)
	}
	for i, m := range s.Members() {
		if m.Subject != s.Subject() {
			t.Errorf("member %d carries subject %+v, want %+v", i, m.Subject, s.Subject())
		}
		if m.Subject.OrgID != "org-7f3c" || m.Subject.Namespace != "payments" {
			t.Errorf("member %d tenancy drifted: %+v", i, m.Subject)
		}
	}

	// There is no argument by which an org or a namespace could differ: the
	// only tenancy input is the set's own subject.
	for name, fn := range map[string]any{"AddReported": (*Set).AddReported, "AddUnavailable": (*Set).AddUnavailable} {
		ft := reflect.TypeOf(fn)
		for i := 0; i < ft.NumIn(); i++ {
			in := ft.In(i)
			if in == reflect.TypeOf(wire.Subject{}) {
				t.Errorf("%s accepts a Subject; the set's subject is the only source", name)
			}
			if in.Kind() == reflect.String {
				t.Errorf("%s accepts a bare string parameter %d; tenancy must not arrive that way", name, i)
			}
		}
	}
}

func TestObservationForAnotherCheckOrEndpointIsRefused(t *testing.T) {
	// A different CheckID.
	s := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{Kind: "test", ID: "v1"}, fixedNow.Add(240*time.Hour),
		func(tg *tlscert.Target) { tg.CheckID = "chk-SOMEONE-ELSE" })
	if err := s.AddReported(obs, res, fixedNow); !errors.Is(err, ErrSubjectMismatch) {
		t.Errorf("foreign check id: err = %v, want ErrSubjectMismatch", err)
	}
	if s.Len() != 0 {
		t.Errorf("a refused member was still added: len = %d", s.Len())
	}

	// A different Endpoint.
	s2 := newSet(t)
	obs2, res2 := observeLeaf(t, tlscert.Vantage{Kind: "test", ID: "v1"}, fixedNow.Add(240*time.Hour),
		func(tg *tlscert.Target) { tg.Endpoint = "elsewhere.test:443" })
	if err := s2.AddReported(obs2, res2, fixedNow); !errors.Is(err, ErrSubjectMismatch) {
		t.Errorf("foreign endpoint: err = %v, want ErrSubjectMismatch", err)
	}
	if s2.Len() != 0 {
		t.Errorf("a refused member was still added: len = %d", s2.Len())
	}
}

// ── 6-9. multiple vantages, no key role, no overwrite, order kept ───────────

func TestMultipleVantagesAreAllRetainedAndVantageIsNotPartOfTheKey(t *testing.T) {
	sub := subject(t)
	s := newSet(t)
	vantages := []tlscert.Vantage{
		{Kind: "external", ID: "frankfurt"},
		{Kind: "external", ID: "tokyo"},
		{Kind: "in-cluster-processor", ID: "proc-1"},
	}
	for _, v := range vantages {
		obs, res := observeLeaf(t, v, fixedNow.Add(240*time.Hour))
		if err := s.AddReported(obs, res, fixedNow); err != nil {
			t.Fatalf("vantage %v: %v", v, err)
		}
	}
	if s.Len() != len(vantages) {
		t.Fatalf("len = %d, want %d", s.Len(), len(vantages))
	}
	// One subject, one set: vantage changed nothing about the key.
	if s.Subject() != sub {
		t.Errorf("subject changed to %+v after adding vantages", s.Subject())
	}
	members := s.Members()
	for i, v := range vantages {
		if members[i].Vantage != v {
			t.Errorf("member %d vantage = %+v, want %+v (insertion order)", i, members[i].Vantage, v)
		}
		if members[i].Observation.Vantage != v {
			t.Errorf("member %d observation vantage = %+v, want %+v", i, members[i].Observation.Vantage, v)
		}
	}
}

func TestSameVantageTwiceIsNotDeduplicated(t *testing.T) {
	s := newSet(t)
	v := tlscert.Vantage{Kind: "external", ID: "frankfurt"}
	first, firstRes := observeLeaf(t, v, fixedNow.Add(240*time.Hour))
	second, secondRes := observeLeaf(t, v, fixedNow.Add(100*time.Hour))
	if err := s.AddReported(first, firstRes, fixedNow); err != nil {
		t.Fatal(err)
	}
	if err := s.AddReported(second, secondRes, fixedNow); err != nil {
		t.Fatal(err)
	}
	if s.Len() != 2 {
		t.Fatalf("len = %d, want 2: a repeat report must not overwrite", s.Len())
	}
	members := s.Members()
	firstLeaf, _ := members[0].Observation.Leaf()
	secondLeaf, _ := members[1].Observation.Leaf()
	if firstLeaf.NotAfter.Equal(secondLeaf.NotAfter) {
		t.Fatal("the two reports are indistinguishable; the fixture is wrong")
	}
	if !firstLeaf.NotAfter.Equal(fixedNow.Add(240 * time.Hour).UTC()) {
		t.Errorf("first report was overwritten: notAfter = %v", firstLeaf.NotAfter)
	}
	if !secondLeaf.NotAfter.Equal(fixedNow.Add(100 * time.Hour).UTC()) {
		t.Errorf("second report is not in position 1: notAfter = %v", secondLeaf.NotAfter)
	}
}

func TestInsertionOrderIsPreservedAcrossStatuses(t *testing.T) {
	s := newSet(t)
	obsA, resA := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "a"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obsA, resA, fixedNow); err != nil {
		t.Fatal(err)
	}
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "b"}, errors.New("probe did not report")); err != nil {
		t.Fatal(err)
	}
	obsC, resC := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "c"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obsC, resC, fixedNow); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id     string
		status MemberStatus
	}{{"a", StatusReported}, {"b", StatusUnavailable}, {"c", StatusReported}}
	members := s.Members()
	if len(members) != len(want) {
		t.Fatalf("len = %d, want %d", len(members), len(want))
	}
	for i, w := range want {
		if members[i].Vantage.ID != w.id || members[i].Status != w.status {
			t.Errorf("member %d = (%q, %q), want (%q, %q)",
				i, members[i].Vantage.ID, members[i].Status, w.id, w.status)
		}
	}
}

// ── 10. the observer's own verdict survives unchanged ───────────────────────

func TestReportedObservationPreservesOutcomeAndTrust(t *testing.T) {
	s := newSet(t)
	v := tlscert.Vantage{Kind: "external", ID: "frankfurt"}
	obs, res := observeLeaf(t, v, fixedNow.Add(240*time.Hour))
	if obs.Outcome != tlscert.Observed || !obs.Trust.Verified() {
		t.Fatalf("fixture did not produce a verified observation: %q %+v", obs.Outcome, obs.Trust)
	}
	if err := s.AddReported(obs, res, fixedNow); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if m.Observation.Outcome != obs.Outcome {
		t.Errorf("Outcome = %q, want %q", m.Observation.Outcome, obs.Outcome)
	}
	if m.Observation.Trust != obs.Trust {
		t.Errorf("Trust = %+v, want %+v", m.Observation.Trust, obs.Trust)
	}
	if !m.Observation.ObservedAt.Equal(obs.ObservedAt) {
		t.Errorf("ObservedAt = %v, want %v", m.Observation.ObservedAt, obs.ObservedAt)
	}
	if got, want := len(m.Observation.Certificates()), len(obs.Certificates()); got != want {
		t.Errorf("chain length = %d, want %d", got, want)
	}
}

// ── 11-13. an unavailable probe is not a target timeout ─────────────────────

func TestUnavailableProbeIsDistinctFromATargetTimeout(t *testing.T) {
	s := newSet(t)

	// A real target-side timeout: the probe ran and reported.
	timedOut := observeTimeout(t, tlscert.Vantage{Kind: "external", ID: "slow-target"})
	if err := s.AddReported(timedOut, tlscertexpiry.Result{}, time.Time{}); err != nil {
		t.Fatalf("a timeout observation is a report and must be accepted: %v", err)
	}

	// A probe-level failure: nothing was observed at all.
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "dead-probe"},
		errors.New("probe did not report")); err != nil {
		t.Fatal(err)
	}

	members := s.Members()
	target, probe := members[0], members[1]

	if target.Status != StatusReported {
		t.Errorf("target timeout status = %q, want %q", target.Status, StatusReported)
	}
	if target.Observation.Outcome != tlscert.Timeout {
		t.Errorf("target timeout outcome = %q, want %q", target.Observation.Outcome, tlscert.Timeout)
	}
	if !target.HasObservation() {
		t.Error("a target timeout is still an observation")
	}

	if probe.Status != StatusUnavailable {
		t.Errorf("unavailable status = %q, want %q", probe.Status, StatusUnavailable)
	}
	if probe.Observation.Outcome == tlscert.Timeout {
		t.Fatal("an unavailable probe was recorded as a target timeout")
	}
	if probe.Observation.Outcome != "" {
		t.Errorf("unavailable outcome = %q, want empty", probe.Observation.Outcome)
	}
	if probe.HasObservation() {
		t.Error("HasObservation is true for an unavailable member")
	}
	if probe.Cause.Message != "probe did not report" {
		t.Errorf("cause = %q", probe.Cause.Message)
	}
	if target.Status == probe.Status {
		t.Error("the two states are not distinguishable by status")
	}
}

func TestUnavailableMemberManufacturesNoObservationAndNoFinding(t *testing.T) {
	s := newSet(t)
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "dead"},
		errors.New("dial tcp: connection refused")); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]

	// No observation was invented.
	if m.Observation.Outcome != "" || m.Observation.Endpoint != "" || m.Observation.CheckID != "" {
		t.Errorf("an observation was manufactured: %+v", m.Observation)
	}
	if !m.Observation.ObservedAt.IsZero() {
		t.Errorf("ObservedAt = %v, want zero", m.Observation.ObservedAt)
	}
	if m.Observation.Certificates() != nil {
		t.Error("certificates were manufactured")
	}
	if m.Observation.Trust != (tlscert.TrustStatus{}) {
		t.Errorf("a trust status was manufactured: %+v", m.Observation.Trust)
	}

	// No finding exists or is produced.
	if m.HasResult() {
		t.Error("HasResult is true for an unavailable member")
	}
	if got := m.Result.Findings(); len(got) != 0 {
		t.Errorf("findings = %v, want none", got)
	}
	if got := m.Result.Certificates(); got != nil {
		t.Errorf("certificate results = %v, want none", got)
	}
	if !m.EvaluatedAt.IsZero() {
		t.Errorf("EvaluatedAt = %v, want zero", m.EvaluatedAt)
	}
}

func TestUnavailableRequiresACauseAndBoundsIt(t *testing.T) {
	s := newSet(t)
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "x"}, nil); !errors.Is(err, ErrNoCause) {
		t.Errorf("nil cause: err = %v, want ErrNoCause", err)
	}
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "x"}, errors.New("   ")); !errors.Is(err, ErrNoCause) {
		t.Errorf("blank cause: err = %v, want ErrNoCause", err)
	}
	long := make([]byte, 4096)
	for i := range long {
		long[i] = 'x'
	}
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "x"}, errors.New(string(long))); err != nil {
		t.Fatal(err)
	}
	if err := s.AddUnavailable(tlscert.Vantage{Kind: "external", ID: "y"},
		errors.New("line one\nline two\ttabbed")); err != nil {
		t.Fatal(err)
	}
	members := s.Members()
	if n := len([]rune(members[0].Cause.Message)); n != MaxProbeErrorMessage {
		t.Errorf("message length = %d, want %d", n, MaxProbeErrorMessage)
	}
	if got, want := members[1].Cause.Message, "line one line two tabbed"; got != want {
		t.Errorf("sanitised message = %q, want %q", got, want)
	}
}

func TestZeroObservationIsRefusedAsAReport(t *testing.T) {
	s := newSet(t)
	if err := s.AddReported(tlscert.Observation{}, tlscertexpiry.Result{}, fixedNow); !errors.Is(err, ErrNoObservation) {
		t.Errorf("zero observation: err = %v, want ErrNoObservation", err)
	}
	if s.Len() != 0 {
		t.Errorf("len = %d, want 0", s.Len())
	}
}

// ── 14. positional finding provenance ───────────────────────────────────────

func TestPositionalFindingProvenanceIsPreserved(t *testing.T) {
	s := newSet(t)
	v := tlscert.Vantage{Kind: "external", ID: "frankfurt"}
	notAfter := fixedNow.Add(240 * time.Hour) // inside the horizon
	obs, res := observeLeaf(t, v, notAfter)
	if err := s.AddReported(obs, res, fixedNow); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if !m.HasResult() {
		t.Fatal("expected a result")
	}

	certs := m.Result.Certificates()
	if len(certs) < 2 {
		t.Fatalf("expected the leaf and its issuer, got %d", len(certs))
	}
	chain := m.Observation.Certificates()
	if len(chain) != len(certs) {
		t.Fatalf("result has %d entries for a chain of %d", len(certs), len(chain))
	}

	for i, c := range certs {
		if c.Position != i {
			t.Errorf("entry %d has Position %d", i, c.Position)
		}
		if c.IsLeaf != (i == 0) {
			t.Errorf("entry %d IsLeaf = %v", i, c.IsLeaf)
		}
		// The result entry describes the certificate at the same position.
		if !c.NotAfter.Equal(chain[i].NotAfter) || c.SerialNumber != chain[i].SerialNumber {
			t.Errorf("entry %d does not describe chain position %d", i, i)
		}
		if c.Found {
			// The finding belongs to this certificate, and carries the
			// subject's check and endpoint through from the observation.
			if !c.Finding.NotAfter.Equal(chain[i].NotAfter) {
				t.Errorf("entry %d finding notAfter = %v, want %v", i, c.Finding.NotAfter, chain[i].NotAfter)
			}
			if c.Finding.CheckID != m.Subject.CheckID {
				t.Errorf("entry %d finding CheckID = %q, want %q", i, c.Finding.CheckID, m.Subject.CheckID)
			}
			if c.Finding.Endpoint != m.Subject.Endpoint {
				t.Errorf("entry %d finding Endpoint = %q, want %q", i, c.Finding.Endpoint, m.Subject.Endpoint)
			}
		}
	}

	// The leaf is inside the horizon, so it produced a finding.
	if !certs[0].Found {
		t.Error("the leaf is inside the horizon but produced no finding")
	}
	// Provenance is reachable end to end without leaving the member.
	if m.Subject.OrgID == "" || m.Vantage.ID == "" || m.Observation.ObservedAt.IsZero() {
		t.Error("the subject -> vantage -> observation -> finding chain is incomplete")
	}
}

func TestFindingCarriesNoIdentityFields(t *testing.T) {
	ft := reflect.TypeOf(certexpiry.Finding{})
	for _, banned := range []string{"Vantage", "OrgID", "Namespace", "ServiceID", "Subject", "Tenant"} {
		if _, ok := ft.FieldByName(banned); ok {
			t.Errorf("certexpiry.Finding gained an identity field %q", banned)
		}
	}
}

// ── 15. the caller's evaluation time ────────────────────────────────────────

func TestCallerSuppliedEvaluationTimeIsPreserved(t *testing.T) {
	s := newSet(t)
	evaluatedAt := fixedNow.Add(90 * time.Second)
	obs, res := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "f"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obs, res, evaluatedAt); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if !m.EvaluatedAt.Equal(evaluatedAt) {
		t.Errorf("EvaluatedAt = %v, want %v", m.EvaluatedAt, evaluatedAt)
	}
	if m.EvaluatedAt.Location() != time.UTC {
		t.Errorf("EvaluatedAt is not UTC: %v", m.EvaluatedAt.Location())
	}
	// It is distinct from the observation time, and neither is derived.
	if m.EvaluatedAt.Equal(m.Observation.ObservedAt) {
		t.Error("the evaluation time collapsed into the observation time")
	}
}

func TestResultWithoutAnEvaluationTimeIsRefused(t *testing.T) {
	s := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "f"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obs, res, time.Time{}); !errors.Is(err, ErrNoEvaluationTime) {
		t.Errorf("err = %v, want ErrNoEvaluationTime", err)
	}
	if s.Len() != 0 {
		t.Errorf("len = %d, want 0", s.Len())
	}
}

// ── temporal: the span is the caller's, and it refuses rather than merges ───

func TestObservationsOutsideTheSpanAreRefusedNotMerged(t *testing.T) {
	sub := subject(t)
	s, err := New(sub, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	near, nearRes := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "a"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(near, nearRes, fixedNow); err != nil {
		t.Fatal(err)
	}

	// A second observation from a clock two hours away cannot be part of the
	// same moment under a one-minute span.
	stale := near
	stale.ObservedAt = near.ObservedAt.Add(-2 * time.Hour)
	stale.Vantage = tlscert.Vantage{Kind: "external", ID: "b"}
	if err := s.AddReported(stale, tlscertexpiry.Result{}, time.Time{}); !errors.Is(err, ErrOutsideSpan) {
		t.Errorf("err = %v, want ErrOutsideSpan", err)
	}
	if s.Len() != 1 {
		t.Errorf("len = %d, want 1: stale evidence must not be merged", s.Len())
	}

	// Inside the span it is accepted, and each ObservedAt is kept as given.
	fresh := near
	fresh.ObservedAt = near.ObservedAt.Add(30 * time.Second)
	fresh.Vantage = tlscert.Vantage{Kind: "external", ID: "c"}
	if err := s.AddReported(fresh, tlscertexpiry.Result{}, time.Time{}); err != nil {
		t.Fatal(err)
	}
	members := s.Members()
	if !members[0].Observation.ObservedAt.Equal(near.ObservedAt) ||
		!members[1].Observation.ObservedAt.Equal(fresh.ObservedAt) {
		t.Error("observation times were not preserved as given")
	}
}

// ── vantage identity: identified or not admitted ────────────────────────────

// An observation that cannot say where it was taken is refused. Two such
// observations could not be shown to come from independent vantages, which is
// the only reason a set holds more than one.
func TestUnidentifiedVantageIsRefusedAtAdmission(t *testing.T) {
	cases := []struct {
		name    string
		vantage tlscert.Vantage
	}{
		{"empty kind", tlscert.Vantage{Kind: "", ID: "frankfurt"}},
		{"empty id", tlscert.Vantage{Kind: "external", ID: ""}},
		{"both empty", tlscert.Vantage{}},
		{"blank kind", tlscert.Vantage{Kind: "   ", ID: "frankfurt"}},
		{"blank id", tlscert.Vantage{Kind: "external", ID: "\t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSet(t)
			obs, res := observeLeaf(t, tc.vantage, fixedNow.Add(240*time.Hour))
			if obs.Outcome != tlscert.Observed {
				t.Fatalf("fixture produced %q; the observation itself must be sound", obs.Outcome)
			}

			err := s.AddReported(obs, res, fixedNow)
			if !errors.Is(err, ErrUnidentifiedVantage) {
				t.Fatalf("err = %v, want ErrUnidentifiedVantage", err)
			}
			if s.Len() != 0 {
				t.Errorf("Len = %d, want 0: a refused observation must not be appended", s.Len())
			}
			if m := s.Members(); len(m) != 0 {
				t.Errorf("Members = %+v, want none", m)
			}
			// It is an invalid input, not a probe failure: no unavailable
			// member is manufactured in its place.
			for _, m := range s.Members() {
				if m.Status == StatusUnavailable {
					t.Errorf("a rejected observation became an unavailable member: %+v", m)
				}
			}

			// The set is still usable afterwards.
			good, goodRes := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "frankfurt"}, fixedNow.Add(240*time.Hour))
			if err := s.AddReported(good, goodRes, fixedNow); err != nil {
				t.Fatalf("the set was left unusable by the rejection: %v", err)
			}
			if s.Len() != 1 {
				t.Errorf("Len = %d, want 1", s.Len())
			}
		})
	}
}

// No other field stands in for a vantage: a fully populated subject, a real
// endpoint and a sound observation still do not make one identified.
func TestNoFieldSubstitutesForVantageIdentity(t *testing.T) {
	s := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{}, fixedNow.Add(240*time.Hour))
	if obs.CheckID == "" || obs.Endpoint == "" || obs.ObservedAt.IsZero() {
		t.Fatal("fixture is not carrying the fields that must NOT be used as a fallback")
	}
	if err := s.AddReported(obs, res, fixedNow); !errors.Is(err, ErrUnidentifiedVantage) {
		t.Fatalf("err = %v, want ErrUnidentifiedVantage", err)
	}
}

// Identified vantages are unaffected: two of them stay independently
// represented, and one may still report more than once.
func TestIdentifiedVantagesAreUnaffected(t *testing.T) {
	s := newSet(t)
	a := tlscert.Vantage{Kind: "external", ID: "frankfurt"}
	b := tlscert.Vantage{Kind: "external", ID: "tokyo"}
	for _, v := range []tlscert.Vantage{a, b, a} {
		obs, res := observeLeaf(t, v, fixedNow.Add(240*time.Hour))
		if err := s.AddReported(obs, res, fixedNow); err != nil {
			t.Fatalf("vantage %+v: %v", v, err)
		}
	}
	members := s.Members()
	if len(members) != 3 {
		t.Fatalf("len = %d, want 3", len(members))
	}
	for i, want := range []tlscert.Vantage{a, b, a} {
		if members[i].Vantage != want {
			t.Errorf("member %d vantage = %+v, want %+v", i, members[i].Vantage, want)
		}
		if !members[i].VantageIdentified() {
			t.Errorf("member %d is not identified", i)
		}
		if members[i].Status != StatusReported {
			t.Errorf("member %d status = %q", i, members[i].Status)
		}
	}
	if members[0].Vantage == members[1].Vantage {
		t.Error("two distinct vantages are not independently represented")
	}
	if members[0].Vantage != members[2].Vantage {
		t.Error("a repeat from one identified vantage was not retained as such")
	}
}

// An unavailable member needs no observation at all, so it needs no vantage
// on one — and it stays distinct from a reported observation that was refused
// for having no identity.
func TestUnavailableMemberNeedsNoVantageBearingObservation(t *testing.T) {
	s := newSet(t)
	if err := s.AddUnavailable(tlscert.Vantage{}, errors.New("probe did not report")); err != nil {
		t.Fatalf("an unidentified vantage must not block an unavailable member: %v", err)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}
	m := s.Members()[0]
	if m.Status != StatusUnavailable {
		t.Errorf("status = %q, want %q", m.Status, StatusUnavailable)
	}
	if m.VantageIdentified() {
		t.Error("VantageIdentified is true for a zero vantage")
	}
	if m.HasObservation() {
		t.Error("an unavailable member carries an observation")
	}
	if m.Observation.Vantage != (tlscert.Vantage{}) {
		t.Errorf("a vantage was manufactured on the observation: %+v", m.Observation.Vantage)
	}
	if m.Cause.Message != "probe did not report" {
		t.Errorf("cause = %q", m.Cause.Message)
	}

	// The two outcomes are not the same thing: one is a recorded member, the
	// other never enters the set.
	s2 := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{}, fixedNow.Add(240*time.Hour))
	if err := s2.AddReported(obs, res, fixedNow); !errors.Is(err, ErrUnidentifiedVantage) {
		t.Fatalf("err = %v, want ErrUnidentifiedVantage", err)
	}
	if s2.Len() != 0 {
		t.Errorf("Len = %d, want 0", s2.Len())
	}
	if s.Len() == s2.Len() {
		t.Error("an unavailable probe and an unidentified report are indistinguishable")
	}
}

// ── copy safety ─────────────────────────────────────────────────────────────

func TestMembersCannotBeMutatedThroughTheReturnedSlice(t *testing.T) {
	s := newSet(t)
	obs, res := observeLeaf(t, tlscert.Vantage{Kind: "external", ID: "frankfurt"}, fixedNow.Add(240*time.Hour))
	if err := s.AddReported(obs, res, fixedNow); err != nil {
		t.Fatal(err)
	}

	stolen := s.Members()
	stolen[0].Vantage = tlscert.Vantage{Kind: "forged", ID: "attacker"}
	stolen[0].Status = StatusUnavailable
	stolen[0].Subject.OrgID = "org-ATTACKER"
	stolen[0].EvaluatedAt = time.Time{}
	stolen = append(stolen, Member{Vantage: tlscert.Vantage{ID: "injected"}})
	_ = stolen

	m := s.Members()[0]
	if m.Vantage.ID != "frankfurt" || m.Status != StatusReported {
		t.Errorf("the set was mutated through Members(): %+v", m)
	}
	if m.Subject.OrgID != "org-7f3c" {
		t.Errorf("tenancy was mutated through Members(): %q", m.Subject.OrgID)
	}
	if s.Len() != 1 {
		t.Errorf("len = %d, want 1: append through Members() reached the set", s.Len())
	}

	// Certificate data stays owned: the frozen accessors already copy.
	certs := m.Observation.Certificates()
	if len(certs) == 0 {
		t.Fatal("expected certificates")
	}
	certs[0].Subject = "forged"
	if again := s.Members()[0].Observation.Certificates(); again[0].Subject == "forged" {
		t.Error("certificate data was mutated through a returned copy")
	}
	results := m.Result.Certificates()
	if len(results) == 0 {
		t.Fatal("expected certificate results")
	}
	results[0].SkipReason = "forged"
	if again := s.Members()[0].Result.Certificates(); again[0].SkipReason == "forged" {
		t.Error("result data was mutated through a returned copy")
	}
}

func TestEmptySetReturnsNoMembers(t *testing.T) {
	s := newSet(t)
	if s.Len() != 0 {
		t.Errorf("len = %d, want 0", s.Len())
	}
	if m := s.Members(); m != nil {
		t.Errorf("Members() = %v, want nil", m)
	}
	if s.Span() != span {
		t.Errorf("Span() = %v, want %v", s.Span(), span)
	}
}

// ── destination independence ────────────────────────────────────────────────

func TestNoDestinationAssumptionsInTheExportedShape(t *testing.T) {
	banned := []string{
		"ID", "RecordID", "PersistedAt", "StoredAt", "Problem", "ProblemID",
		"Severity", "Confidence", "Priority", "Remediation", "Action", "Policy",
		"Status_", "Acknowledged", "ResolvedAt", "Row", "Table", "Migration",
	}
	for _, tp := range []reflect.Type{reflect.TypeOf(Set{}), reflect.TypeOf(Member{}), reflect.TypeOf(ProbeError{})} {
		for _, name := range banned {
			if f, ok := tp.FieldByName(name); ok && f.IsExported() {
				t.Errorf("%s exposes %q, which assumes a destination", tp.Name(), name)
			}
		}
	}
	// The set exposes no identifier of its own: the subject identifies it.
	if _, ok := reflect.TypeOf(Set{}).FieldByName("ID"); ok {
		t.Error("Set mints an identifier; the subject is the identity")
	}
	// Member's exported fields are exactly the agreed six.
	want := []string{"Subject", "Status", "Vantage", "Observation", "Result", "EvaluatedAt", "Cause"}
	mt := reflect.TypeOf(Member{})
	var got []string
	for i := 0; i < mt.NumField(); i++ {
		if mt.Field(i).IsExported() {
			got = append(got, mt.Field(i).Name)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Member exported fields = %v, want exactly %v", got, want)
	}
}
