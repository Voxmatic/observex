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
	correlate "github.com/observex/platform/internal/correlate/f61"
	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// Fixed times and caller-supplied bounds. Nothing in this package or its tests
// reads the real clock for a decision.
var (
	fixedNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	horizon  = 720 * time.Hour // 30 days, the caller's
	maxAge   = 15 * time.Minute
	params   = certexpiry.Params{Horizon: horizon, MaxObservationAge: maxAge}
	span     = 5 * time.Minute
)

const (
	testHost     = "observex.test"
	testEndpoint = "observex.test:443"
	testCheckID  = "chk-91ab"
)

var testVantage = tlscert.Vantage{Kind: "in-cluster-processor", ID: "proc-1"}

func subject(t *testing.T) wire.Subject {
	t.Helper()
	s, err := wire.SubjectFromCheck(wire.CheckRow{
		OrgID: "org-7f3c", Namespace: "payments", ID: testCheckID, Target: testEndpoint,
	})
	if err != nil {
		t.Fatalf("SubjectFromCheck: %v", err)
	}
	return s
}

func newSet(t *testing.T) correlate.Set {
	t.Helper()
	s, err := correlate.New(subject(t), span)
	if err != nil {
		t.Fatalf("correlate.New: %v", err)
	}
	return s
}

// ── fixtures: real handshakes over net.Pipe, no sockets ─────────────────────

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

func newCA(t *testing.T) *authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "compose-test-ca"},
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

// serving builds a prober whose dialler hands back one end of an in-memory
// pipe with a TLS server on the other. The prober is the caller's, exactly as
// the composition expects.
func serving(t *testing.T, notAfter time.Time, v tlscert.Vantage) tlscert.Prober {
	t.Helper()
	ca := newCA(t)
	der, key := newLeaf(t, ca, notAfter)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	cfg := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der, ca.der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
	return tlscert.New(tlscert.Config{
		Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
			client, server := net.Pipe()
			go func() {
				tc := tls.Server(server, cfg)
				_ = tc.Handshake()
				buf := make([]byte, 1)
				_, _ = tc.Read(buf)
				_ = tc.Close()
			}()
			return client, nil
		},
		Now:     func() time.Time { return fixedNow },
		Roots:   roots,
		Vantage: v,
	})
}

// refusing builds a prober whose dialler fails. The observer reports that as a
// transport failure — an observation about the target, not a probe failure.
func refusing(t *testing.T, v tlscert.Vantage) tlscert.Prober {
	t.Helper()
	return tlscert.New(tlscert.Config{
		Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, errors.New("connection refused")
		},
		Now:     func() time.Time { return fixedNow },
		Vantage: v,
	})
}

// brokenProber stands in for a prober that cannot observe at all and returns
// an error with the zero Observation, exactly as tlscert.Probe does for an
// unusable target.
type brokenProber struct{ err error }

func (b brokenProber) Probe(context.Context, tlscert.Target) (tlscert.Observation, error) {
	return tlscert.Observation{}, b.err
}

// recordingProber captures the target it was handed.
type recordingProber struct {
	inner tlscert.Prober
	got   tlscert.Target
}

func (r *recordingProber) Probe(ctx context.Context, t tlscert.Target) (tlscert.Observation, error) {
	r.got = t
	return r.inner.Probe(ctx, t)
}

func input(p tlscert.Prober) Input {
	return Input{
		Prober: p, Timeout: 5 * time.Second, Params: params,
		EvaluatedAt: fixedNow, UnavailableVantage: testVantage,
	}
}

// ── the complete path, once ─────────────────────────────────────────────────

func TestValidObservationFlowsThroughTheWholeComposition(t *testing.T) {
	s := newSet(t)
	p := serving(t, fixedNow.Add(240*time.Hour), testVantage) // inside the horizon

	step, err := Observe(context.Background(), &s, input(p))
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if step.Status != correlate.StatusReported {
		t.Errorf("status = %q, want %q", step.Status, correlate.StatusReported)
	}
	if step.ProbeErr != nil || step.AdaptErr != nil {
		t.Errorf("unexpected errors: probe=%v adapt=%v", step.ProbeErr, step.AdaptErr)
	}
	if s.Len() != 1 {
		t.Fatalf("len = %d, want 1", s.Len())
	}

	m := s.Members()[0]
	if m.Status != correlate.StatusReported || !m.HasObservation() || !m.HasResult() {
		t.Fatalf("member did not complete the path: %+v", m.Status)
	}
	if m.Observation.Outcome != tlscert.Observed {
		t.Errorf("outcome = %q, want %q", m.Observation.Outcome, tlscert.Observed)
	}
	if !m.Observation.Trust.Verified() {
		t.Errorf("trust = %+v, want verified", m.Observation.Trust)
	}
	certs := m.Result.Certificates()
	if len(certs) != 2 {
		t.Fatalf("result covers %d certificates, want the leaf and its issuer", len(certs))
	}
	if !certs[0].Found {
		t.Error("the leaf is inside the horizon but produced no finding")
	}
}

func TestCertificateInsideTheHorizonProducesADetectorResult(t *testing.T) {
	s := newSet(t)
	notAfter := fixedNow.Add(240 * time.Hour)
	p := serving(t, notAfter, testVantage)

	if _, err := Observe(context.Background(), &s, input(p)); err != nil {
		t.Fatal(err)
	}
	leaf := s.Members()[0].Result.Certificates()[0]
	if !leaf.Found {
		t.Fatal("expected a finding for a certificate inside the horizon")
	}
	f := leaf.Finding
	if !f.NotAfter.Equal(notAfter.UTC()) {
		t.Errorf("finding NotAfter = %v, want %v", f.NotAfter, notAfter.UTC())
	}
	if !f.EvaluatedAt.Equal(fixedNow) {
		t.Errorf("finding EvaluatedAt = %v, want the caller's %v", f.EvaluatedAt, fixedNow)
	}
	if f.Horizon != horizon {
		t.Errorf("finding Horizon = %v, want the caller's %v", f.Horizon, horizon)
	}
	if f.CheckID != testCheckID || f.Endpoint != testEndpoint {
		t.Errorf("finding subject = %q/%q", f.CheckID, f.Endpoint)
	}
	if f.PatternID == "" || f.CatalogSHA256 == "" {
		t.Error("catalog provenance did not survive the composition")
	}
}

func TestCertificateOutsideTheHorizonYieldsNoFindingButStaysRepresentable(t *testing.T) {
	s := newSet(t)
	notAfter := fixedNow.Add(365 * 24 * time.Hour) // far beyond the horizon
	p := serving(t, notAfter, testVantage)

	step, err := Observe(context.Background(), &s, input(p))
	if err != nil {
		t.Fatal(err)
	}
	if step.Status != correlate.StatusReported || step.AdaptErr != nil {
		t.Fatalf("step = %+v", step)
	}
	m := s.Members()[0]
	if !m.HasResult() {
		t.Fatal("the observation was adapted, so a result must exist")
	}
	// No finding, and no error pretending to be one.
	if got := m.Result.Findings(); len(got) != 0 {
		t.Errorf("findings = %v, want none", got)
	}
	for i, c := range m.Result.Certificates() {
		if c.Found {
			t.Errorf("certificate %d produced a finding outside the horizon", i)
		}
		if c.Status != tlscertexpiry.StatusEvaluated {
			t.Errorf("certificate %d status = %q, want evaluated", i, c.Status)
		}
		if c.Err != nil {
			t.Errorf("certificate %d carries an error: %v", i, c.Err)
		}
	}
	// The observation itself is fully representable.
	if m.Observation.Outcome != tlscert.Observed || len(m.Observation.Certificates()) != 2 {
		t.Errorf("the observation was diminished: %q, %d certs",
			m.Observation.Outcome, len(m.Observation.Certificates()))
	}
	leaf, ok := m.Observation.Leaf()
	if !ok || !leaf.NotAfter.Equal(notAfter.UTC()) {
		t.Errorf("leaf notAfter = %v, want %v", leaf.NotAfter, notAfter.UTC())
	}
}

func TestExpiredCertificatePreservesObservedUntrustedSemantics(t *testing.T) {
	s := newSet(t)
	notAfter := fixedNow.Add(-24 * time.Hour) // already expired
	p := serving(t, notAfter, testVantage)

	if _, err := Observe(context.Background(), &s, input(p)); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if m.Observation.Outcome != tlscert.ObservedUntrusted {
		t.Errorf("outcome = %q, want %q", m.Observation.Outcome, tlscert.ObservedUntrusted)
	}
	if m.Observation.Trust.ChainVerified {
		t.Error("an expired certificate reported a verified chain")
	}
	if !m.Observation.Trust.HostnameVerified {
		t.Error("hostname verification is independent of expiry and should still pass")
	}
	if m.Observation.Trust.Reason != tlscert.ReasonExpired {
		t.Errorf("reason = %q, want %q", m.Observation.Trust.Reason, tlscert.ReasonExpired)
	}
	// An expired notAfter is a fact, and the detector treats it as inside
	// every horizon. The composition neither suppresses nor embellishes it.
	leaf := m.Result.Certificates()[0]
	if !leaf.Found {
		t.Fatal("an expired certificate produced no finding")
	}
	if leaf.Finding.Remaining >= 0 {
		t.Errorf("remaining = %v, want negative", leaf.Finding.Remaining)
	}
	if !m.Result.Trust.Verified() == false && m.Result.Outcome != tlscert.ObservedUntrusted {
		t.Errorf("the result did not carry the observation's verdict: %+v", m.Result.Outcome)
	}
}

// ── failure semantics stay typed and distinguishable ────────────────────────

func TestUnavailableProbeIsRecordedAsUnavailableNotAsAnObservation(t *testing.T) {
	s := newSet(t)
	cause := errors.New("probe could not run")

	step, err := Observe(context.Background(), &s, input(brokenProber{err: cause}))
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if step.Status != correlate.StatusUnavailable {
		t.Errorf("status = %q, want %q", step.Status, correlate.StatusUnavailable)
	}
	if !errors.Is(step.ProbeErr, cause) {
		t.Errorf("ProbeErr = %v, want %v", step.ProbeErr, cause)
	}
	if step.AdaptErr != nil {
		t.Errorf("AdaptErr = %v, want nil", step.AdaptErr)
	}

	m := s.Members()[0]
	if m.Status != correlate.StatusUnavailable || m.HasObservation() {
		t.Fatalf("an unavailable probe became an observation: %+v", m.Status)
	}
	// The zero Observation the prober returned was discarded, not recorded.
	if m.Observation.Outcome != "" || m.Observation.Endpoint != "" || !m.Observation.ObservedAt.IsZero() {
		t.Errorf("an observation was manufactured: %+v", m.Observation)
	}
	if m.Observation.Outcome == tlscert.Timeout {
		t.Fatal("an unavailable probe was recorded as a target timeout")
	}
	// No finding exists.
	if m.HasResult() || len(m.Result.Findings()) != 0 || m.Result.Certificates() != nil {
		t.Error("a finding was manufactured for an unavailable probe")
	}
	if m.Cause.Message != cause.Error() {
		t.Errorf("cause = %q, want %q", m.Cause.Message, cause.Error())
	}
}

func TestTargetFailureRemainsAReportedObservation(t *testing.T) {
	s := newSet(t)
	step, err := Observe(context.Background(), &s, input(refusing(t, testVantage)))
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	// The probe ran; the target did not answer. That is a report, not a
	// probe failure, and the adapter legitimately cannot use it.
	if step.Status != correlate.StatusReported {
		t.Errorf("status = %q, want %q", step.Status, correlate.StatusReported)
	}
	if step.ProbeErr != nil {
		t.Errorf("ProbeErr = %v, want nil: the prober did report", step.ProbeErr)
	}
	if !errors.Is(step.AdaptErr, tlscertexpiry.ErrNotCertificateObservation) {
		t.Errorf("AdaptErr = %v, want ErrNotCertificateObservation", step.AdaptErr)
	}
	m := s.Members()[0]
	if !m.HasObservation() {
		t.Fatal("the observation was dropped")
	}
	if m.Observation.Outcome != tlscert.TransportFailure {
		t.Errorf("outcome = %q, want %q", m.Observation.Outcome, tlscert.TransportFailure)
	}
	if m.HasResult() {
		t.Error("a result was produced for an observation carrying no certificate")
	}
	if !m.EvaluatedAt.IsZero() {
		t.Errorf("EvaluatedAt = %v, want zero: nothing was evaluated", m.EvaluatedAt)
	}
	// The two failure classes are distinguishable at the Step.
	if step.ProbeErr != nil && step.AdaptErr != nil {
		t.Error("probe and adapt failures are being conflated")
	}
}

func TestUnidentifiedVantageIsRejectedByTheCorrelationContract(t *testing.T) {
	s := newSet(t)
	// The prober is configured with no vantage, so the observation it takes
	// carries none. The correlation layer owns that refusal.
	p := serving(t, fixedNow.Add(240*time.Hour), tlscert.Vantage{})

	step, err := Observe(context.Background(), &s, input(p))
	if !errors.Is(err, correlate.ErrUnidentifiedVantage) {
		t.Fatalf("err = %v, want correlate.ErrUnidentifiedVantage", err)
	}
	if step.Status != "" {
		t.Errorf("status = %q, want empty: nothing was admitted", step.Status)
	}
	if s.Len() != 0 {
		t.Errorf("len = %d, want 0", s.Len())
	}
	// The composition did not rescue it with UnavailableVantage.
	for _, m := range s.Members() {
		t.Errorf("a member was admitted despite the refusal: %+v", m.Vantage)
	}
}

func TestInputValidationRefusesRatherThanDefaults(t *testing.T) {
	s := newSet(t)
	p := serving(t, fixedNow.Add(240*time.Hour), testVantage)
	ctx := context.Background()

	if _, err := Observe(ctx, nil, input(p)); !errors.Is(err, ErrNoSet) {
		t.Errorf("nil set: err = %v, want ErrNoSet", err)
	}
	bad := input(p)
	bad.Prober = nil
	if _, err := Observe(ctx, &s, bad); !errors.Is(err, ErrNoProber) {
		t.Errorf("nil prober: err = %v, want ErrNoProber", err)
	}
	for _, d := range []time.Duration{0, -time.Second} {
		bad = input(p)
		bad.Timeout = d
		if _, err := Observe(ctx, &s, bad); !errors.Is(err, ErrNoTimeout) {
			t.Errorf("timeout %v: err = %v, want ErrNoTimeout", d, err)
		}
	}
	bad = input(p)
	bad.EvaluatedAt = time.Time{}
	if _, err := Observe(ctx, &s, bad); !errors.Is(err, ErrNoEvaluationTime) {
		t.Errorf("zero evaluation time: err = %v, want ErrNoEvaluationTime", err)
	}
	if s.Len() != 0 {
		t.Errorf("a refused call still admitted a member: len = %d", s.Len())
	}
}

// ── subject, vantage, time: nothing is derived ──────────────────────────────

func TestProbeTargetAndMemberComeFromTheSetsSubject(t *testing.T) {
	s := newSet(t)
	rec := &recordingProber{inner: serving(t, fixedNow.Add(240*time.Hour), testVantage)}

	if _, err := Observe(context.Background(), &s, input(rec)); err != nil {
		t.Fatal(err)
	}
	sub := s.Subject()
	if rec.got.CheckID != sub.CheckID || rec.got.Endpoint != sub.Endpoint {
		t.Errorf("target = %+v, want check %q endpoint %q", rec.got, sub.CheckID, sub.Endpoint)
	}
	if rec.got.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want the caller's 5s", rec.got.Timeout)
	}
	m := s.Members()[0]
	if m.Subject != sub {
		t.Errorf("member subject = %+v, want %+v", m.Subject, sub)
	}
	if m.Observation.CheckID != sub.CheckID || m.Observation.Endpoint != sub.Endpoint {
		t.Errorf("observation subject drifted: %q/%q", m.Observation.CheckID, m.Observation.Endpoint)
	}
}

func TestVantageComesOnlyFromTheObserverAndIsNeverFabricated(t *testing.T) {
	s := newSet(t)
	proberVantage := tlscert.Vantage{Kind: "in-cluster-processor", ID: "proc-REAL"}
	decoy := tlscert.Vantage{Kind: "external", ID: "DECOY"}

	in := input(serving(t, fixedNow.Add(240*time.Hour), proberVantage))
	in.UnavailableVantage = decoy // must never touch a successful observation

	if _, err := Observe(context.Background(), &s, in); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if m.Vantage != proberVantage {
		t.Errorf("member vantage = %+v, want the observer's %+v", m.Vantage, proberVantage)
	}
	if m.Observation.Vantage != proberVantage {
		t.Errorf("observation vantage = %+v, want %+v", m.Observation.Vantage, proberVantage)
	}
	if m.Vantage == decoy || m.Observation.Vantage == decoy {
		t.Fatal("UnavailableVantage replaced the observer's vantage")
	}

	// On the unavailable path it is used, because no observation exists.
	s2 := newSet(t)
	in2 := input(brokenProber{err: errors.New("no observation")})
	in2.UnavailableVantage = decoy
	if _, err := Observe(context.Background(), &s2, in2); err != nil {
		t.Fatal(err)
	}
	if got := s2.Members()[0].Vantage; got != decoy {
		t.Errorf("unavailable vantage = %+v, want %+v", got, decoy)
	}

	// No identity is invented anywhere: the detector's finding carries none.
	ft := reflect.TypeOf(certexpiry.Finding{})
	for _, banned := range []string{"Vantage", "OrgID", "Namespace", "ServiceID", "Tenant"} {
		if _, ok := ft.FieldByName(banned); ok {
			t.Errorf("certexpiry.Finding gained %q", banned)
		}
	}
}

func TestCallerSuppliedEvaluationTimeIsUsedAndRecorded(t *testing.T) {
	s := newSet(t)
	evaluatedAt := fixedNow.Add(90 * time.Second)
	in := input(serving(t, fixedNow.Add(240*time.Hour), testVantage))
	in.EvaluatedAt = evaluatedAt

	if _, err := Observe(context.Background(), &s, in); err != nil {
		t.Fatal(err)
	}
	m := s.Members()[0]
	if !m.EvaluatedAt.Equal(evaluatedAt) {
		t.Errorf("member EvaluatedAt = %v, want %v", m.EvaluatedAt, evaluatedAt)
	}
	if !m.Result.Certificates()[0].Finding.EvaluatedAt.Equal(evaluatedAt) {
		t.Errorf("the detector was given a different time: %v",
			m.Result.Certificates()[0].Finding.EvaluatedAt)
	}
	if m.EvaluatedAt.Equal(m.Observation.ObservedAt) {
		t.Error("the evaluation time collapsed into the observation time")
	}
	// The observation time is the observer's injected clock, untouched.
	if !m.Observation.ObservedAt.Equal(fixedNow) {
		t.Errorf("ObservedAt = %v, want the observer's %v", m.Observation.ObservedAt, fixedNow)
	}
}

// ── no mutation, no aliasing ────────────────────────────────────────────────

func TestInputIsNotMutatedAndStoredDataIsNotAliased(t *testing.T) {
	s := newSet(t)
	in := input(serving(t, fixedNow.Add(240*time.Hour), testVantage))
	before := in

	if _, err := Observe(context.Background(), &s, in); err != nil {
		t.Fatal(err)
	}
	if in != before {
		t.Errorf("Observe mutated its Input: %+v -> %+v", before, in)
	}

	m := s.Members()[0]
	certs := m.Observation.Certificates()
	certs[0].Subject = "forged"
	if again := s.Members()[0].Observation.Certificates(); again[0].Subject == "forged" {
		t.Error("observation certificates are aliased to the set")
	}
	results := m.Result.Certificates()
	results[0].SkipReason = "forged"
	if again := s.Members()[0].Result.Certificates(); again[0].SkipReason == "forged" {
		t.Error("result certificates are aliased to the set")
	}
	stolen := s.Members()
	stolen[0].Vantage = tlscert.Vantage{Kind: "forged", ID: "forged"}
	stolen[0].Subject.OrgID = "org-ATTACKER"
	if m2 := s.Members()[0]; m2.Vantage != testVantage || m2.Subject.OrgID != "org-7f3c" {
		t.Errorf("the set was mutated through Members(): %+v", m2.Vantage)
	}
	// The DER bytes are cloned by the frozen accessor, not shared.
	leaf, _ := m.Observation.Leaf()
	der := leaf.DER()
	if len(der) == 0 {
		t.Fatal("expected DER bytes")
	}
	der[0] ^= 0xFF
	again, _ := s.Members()[0].Observation.Leaf()
	if again.DER()[0] == der[0] {
		t.Error("certificate DER is aliased to the set")
	}
}

// ── several vantages are representable, and none is invented ────────────────

func TestSeveralCallsBuildSeveralMembersWithoutInventingIndependence(t *testing.T) {
	s := newSet(t)
	vantages := []tlscert.Vantage{
		{Kind: "in-cluster-processor", ID: "proc-1"},
		{Kind: "in-cluster-processor", ID: "proc-2"},
	}
	for _, v := range vantages {
		in := input(serving(t, fixedNow.Add(240*time.Hour), v))
		in.UnavailableVantage = v
		if _, err := Observe(context.Background(), &s, in); err != nil {
			t.Fatalf("vantage %+v: %v", v, err)
		}
	}
	if s.Len() != 2 {
		t.Fatalf("len = %d, want 2", s.Len())
	}
	members := s.Members()
	for i, v := range vantages {
		if members[i].Vantage != v {
			t.Errorf("member %d vantage = %+v, want %+v", i, members[i].Vantage, v)
		}
	}
	// Both are in-cluster, and nothing here says otherwise. This composition
	// makes no claim of externality or of independent availability.
	for i, m := range members {
		if m.Vantage.Kind != "in-cluster-processor" {
			t.Errorf("member %d kind = %q; the fixture is in-cluster", i, m.Vantage.Kind)
		}
	}
}
