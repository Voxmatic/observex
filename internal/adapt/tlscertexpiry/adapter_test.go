package tlscertexpiry

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
	"sync"
	"testing"
	"time"

	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
)

// Fixed times. Nothing in this package or its tests reads the real clock for a
// decision; the observer's clock is injected and the evaluation time is passed
// in.
var (
	fixedNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	horizon  = 720 * time.Hour // 30 days, supplied by the caller
	maxAge   = 15 * time.Minute
	params   = certexpiry.Params{Horizon: horizon, MaxObservationAge: maxAge}
)

// ── fixtures: real handshakes against in-process servers ────────────────────

type authority struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

func newCA(t *testing.T, cn string, parent *authority) *authority {
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
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &authority{cert: cert, key: key, der: der}
}

func newLeaf(t *testing.T, issuer *authority, cn string, notAfter time.Time, dnsNames []string, ips []net.IP) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    fixedNow.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer.cert, &key.PublicKey, issuer.key)
	if err != nil {
		t.Fatal(err)
	}
	return der, key
}

func startServer(t *testing.T, chain [][]byte, key *ecdsa.PrivateKey) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{Certificates: []tls.Certificate{{Certificate: chain, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				tc := tls.Server(c, cfg)
				_ = tc.SetDeadline(time.Now().Add(5 * time.Second))
				_ = tc.Handshake()
				buf := make([]byte, 64)
				for {
					if _, err := tc.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close(); wg.Wait() })
	return ln.Addr().String()
}

// observe probes a server and returns the observation the adapter consumes.
func observe(t *testing.T, addr string, roots *x509.CertPool) tlscert.Observation {
	t.Helper()
	p := tlscert.New(tlscert.Config{
		Now:     func() time.Time { return fixedNow },
		Roots:   roots,
		Vantage: tlscert.Vantage{Kind: "test", ID: "adapter"},
	})
	obs, err := p.Probe(context.Background(), tlscert.Target{CheckID: "chk-1", Endpoint: addr, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	return obs
}

func poolOf(certs ...*x509.Certificate) *x509.CertPool {
	p := x509.NewCertPool()
	for _, c := range certs {
		p.AddCert(c)
	}
	return p
}

func localIPs() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} }

// serveLeaf mints a leaf with an exact notAfter, serves it, and observes it.
func serveLeaf(t *testing.T, notAfter time.Time) (tlscert.Observation, *authority) {
	t.Helper()
	ca := newCA(t, "adapter-test-ca", nil)
	der, key := newLeaf(t, ca, "observex.test", notAfter, nil, localIPs())
	addr := startServer(t, [][]byte{der, ca.der}, key)
	return observe(t, addr, poolOf(ca.cert)), ca
}

// ── horizon decisions travel to the detector unchanged ──────────────────────

func TestEligibleCertificateIsEvaluatedByTheDetector(t *testing.T) {
	cases := []struct {
		name      string
		notAfter  time.Time
		wantFound bool
	}{
		{"valid, far beyond the horizon", fixedNow.Add(365 * 24 * time.Hour), false},
		{"inside the horizon", fixedNow.Add(240 * time.Hour), true},
		{"exactly at the horizon", fixedNow.Add(horizon), true},
		{"already expired", fixedNow.Add(-24 * time.Hour), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, _ := serveLeaf(t, tc.notAfter)
			res, err := Adapt(fixedNow, obs, params)
			if err != nil {
				t.Fatalf("Adapt: %v", err)
			}
			certs := res.Certificates()
			if len(certs) == 0 {
				t.Fatal("no per-certificate results")
			}
			leaf := certs[0]
			if leaf.Status != StatusEvaluated || leaf.Err != nil {
				t.Fatalf("leaf status = %q, err = %v", leaf.Status, leaf.Err)
			}
			if leaf.Found != tc.wantFound {
				t.Fatalf("Found = %v, want %v", leaf.Found, tc.wantFound)
			}
			// The observed notAfter reaches the detector byte-exact.
			if !leaf.NotAfter.Equal(tc.notAfter) {
				t.Errorf("result NotAfter = %v, want %v", leaf.NotAfter, tc.notAfter)
			}
			if !tc.wantFound {
				if leaf.Finding != (certexpiry.Finding{}) {
					t.Errorf("no finding expected, got %+v", leaf.Finding)
				}
				if len(res.Findings()) != 0 {
					t.Errorf("Findings() = %d, want 0", len(res.Findings()))
				}
				return
			}
			f := leaf.Finding
			if !f.NotAfter.Equal(tc.notAfter) || !f.ObservedAt.Equal(obs.ObservedAt) || !f.EvaluatedAt.Equal(fixedNow) {
				t.Errorf("timestamps not preserved: %+v (observation at %v)", f, obs.ObservedAt)
			}
			if f.Remaining != tc.notAfter.Sub(fixedNow) {
				t.Errorf("Remaining = %v, want %v", f.Remaining, tc.notAfter.Sub(fixedNow))
			}
			if f.Horizon != horizon {
				t.Errorf("Horizon = %v, want the caller's %v", f.Horizon, horizon)
			}
			if f.PatternID != certexpiry.PatternID || f.CatalogSHA256 != certexpiry.CatalogSHA256 {
				t.Errorf("provenance lost: %+v", f)
			}
			if f.CheckID != "chk-1" || f.Endpoint != obs.Endpoint {
				t.Errorf("subject lost: %q %q", f.CheckID, f.Endpoint)
			}
			if got := res.Findings(); len(got) != 1 {
				t.Errorf("Findings() = %d, want 1", len(got))
			}
		})
	}
}

// ── the resolved hostname boundary ──────────────────────────────────────────

func TestHostnameMismatchIsSkippedWithAnExplicitReason(t *testing.T) {
	ca := newCA(t, "adapter-test-ca", nil)
	notAfter := fixedNow.Add(24 * time.Hour) // well inside the horizon
	der, key := newLeaf(t, ca, "other.example", notAfter, []string{"other.example"}, nil)
	addr := startServer(t, [][]byte{der, ca.der}, key)
	obs := observe(t, addr, poolOf(ca.cert))

	if obs.Trust.HostnameVerified || !obs.Trust.ChainVerified {
		t.Fatalf("fixture: want a hostname-only failure, got %+v", obs.Trust)
	}

	res, err := Adapt(fixedNow, obs, params)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	leaf := res.Certificates()[0]
	if leaf.Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", leaf.Status, StatusSkipped)
	}
	if leaf.SkipReason != SkipHostnameMismatch {
		t.Errorf("SkipReason = %q", leaf.SkipReason)
	}
	if leaf.Found || leaf.Finding != (certexpiry.Finding{}) || leaf.Err != nil {
		t.Errorf("a skipped certificate must produce no finding and no error: %+v", leaf)
	}
	// The observation is preserved, not discarded or altered.
	if !leaf.NotAfter.Equal(notAfter) {
		t.Errorf("skipped certificate lost its notAfter: %v", leaf.NotAfter)
	}
	if res.Trust.HostnameVerified {
		t.Error("result must carry the observation's trust facts")
	}
	if len(res.Findings()) != 0 {
		t.Error("a skipped certificate produced a finding")
	}
}

// An untrusted issuer is authorized as detector input by the approved design;
// only the hostname case is unresolved.
func TestUntrustedIssuerIsStillEvaluated(t *testing.T) {
	served := newCA(t, "rogue-ca", nil)
	trusted := newCA(t, "trusted-ca", nil)
	notAfter := fixedNow.Add(48 * time.Hour)
	der, key := newLeaf(t, served, "observex.test", notAfter, nil, localIPs())
	addr := startServer(t, [][]byte{der, served.der}, key)
	obs := observe(t, addr, poolOf(trusted.cert))

	if obs.Outcome != tlscert.ObservedUntrusted || obs.Trust.ChainVerified || !obs.Trust.HostnameVerified {
		t.Fatalf("fixture: %+v %q", obs.Trust, obs.Outcome)
	}
	res, err := Adapt(fixedNow, obs, params)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	leaf := res.Certificates()[0]
	if leaf.Status != StatusEvaluated || !leaf.Found {
		t.Fatalf("untrusted issuer must still be evaluated: %+v", leaf)
	}
	if !leaf.Finding.NotAfter.Equal(notAfter) {
		t.Errorf("notAfter = %v, want %v", leaf.Finding.NotAfter, notAfter)
	}
}

// ── chain: one detector evaluation per presented certificate ────────────────

func TestEveryPresentedCertificateIsEvaluatedIndependently(t *testing.T) {
	root := newCA(t, "root-ca", nil)
	inter := newCA(t, "intermediate-ca", root)
	leafNotAfter := fixedNow.Add(120 * time.Hour) // inside the horizon
	der, key := newLeaf(t, inter, "observex.test", leafNotAfter, nil, localIPs())
	addr := startServer(t, [][]byte{der, inter.der}, key)
	obs := observe(t, addr, poolOf(root.cert))

	res, err := Adapt(fixedNow, obs, params)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	certs := res.Certificates()
	if len(certs) != 2 {
		t.Fatalf("results = %d, want 2 (leaf + intermediate)", len(certs))
	}

	leaf, intermediate := certs[0], certs[1]
	if leaf.Position != 0 || !leaf.IsLeaf || intermediate.Position != 1 || intermediate.IsLeaf {
		t.Fatalf("chain position or identity lost: %+v / %+v", leaf, intermediate)
	}
	if intermediate.Subject != inter.cert.Subject.String() || intermediate.SerialNumber != inter.cert.SerialNumber.String() {
		t.Errorf("intermediate identity lost: %+v", intermediate)
	}
	// Independent evaluations: the leaf is inside the horizon, the intermediate
	// outlives it, so exactly one finding is produced — the chain was not
	// collapsed to one verdict.
	if !leaf.Found {
		t.Error("leaf inside the horizon produced no finding")
	}
	if intermediate.Status != StatusEvaluated {
		t.Errorf("the intermediate was not evaluated: %+v", intermediate)
	}
	if intermediate.Found {
		t.Error("the intermediate is far from expiry and must not produce a finding")
	}
	if !intermediate.NotAfter.Equal(inter.cert.NotAfter.UTC()) {
		t.Errorf("intermediate notAfter = %v, want %v", intermediate.NotAfter, inter.cert.NotAfter.UTC())
	}
	if got := res.Findings(); len(got) != 1 || !got[0].NotAfter.Equal(leafNotAfter) {
		t.Errorf("Findings() = %+v, want only the leaf's", got)
	}
}

// Two certificates that are both inside the horizon produce two findings.
func TestMultipleCertificatesProduceIndependentFindings(t *testing.T) {
	root := newCA(t, "root-ca", nil)
	// An intermediate that itself expires soon.
	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	interNotAfter := fixedNow.Add(300 * time.Hour)
	interTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99), Subject: pkix.Name{CommonName: "short-lived-intermediate"},
		NotBefore: fixedNow.Add(-24 * time.Hour), NotAfter: interNotAfter,
		IsCA: true, KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true,
	}
	interDER, err := x509.CreateCertificate(rand.Reader, interTmpl, root.cert, &interKey.PublicKey, root.key)
	if err != nil {
		t.Fatal(err)
	}
	interCert, err := x509.ParseCertificate(interDER)
	if err != nil {
		t.Fatal(err)
	}
	inter := &authority{cert: interCert, key: interKey, der: interDER}

	leafNotAfter := fixedNow.Add(100 * time.Hour)
	der, key := newLeaf(t, inter, "observex.test", leafNotAfter, nil, localIPs())
	addr := startServer(t, [][]byte{der, inter.der}, key)
	obs := observe(t, addr, poolOf(root.cert))

	res, err := Adapt(fixedNow, obs, params)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	findings := res.Findings()
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2 (both certificates inside the horizon)", len(findings))
	}
	if !findings[0].NotAfter.Equal(leafNotAfter) || !findings[1].NotAfter.Equal(interNotAfter) {
		t.Errorf("findings are not per certificate, in wire order: %v / %v", findings[0].NotAfter, findings[1].NotAfter)
	}
	if findings[0].Remaining == findings[1].Remaining {
		t.Error("both findings carry the same remaining time; the chain was collapsed")
	}
}

// ── environmental observations are never reinterpreted ──────────────────────

func TestUnsuccessfulObservationsAreRejected(t *testing.T) {
	for _, outcome := range []tlscert.Outcome{
		tlscert.TransportFailure, tlscert.Timeout, tlscert.Malformed, tlscert.NoCertificates, tlscert.Outcome(""),
	} {
		t.Run(string(outcome), func(t *testing.T) {
			obs := tlscert.Observation{CheckID: "c", Endpoint: "host:443", Outcome: outcome, ObservedAt: fixedNow}
			res, err := Adapt(fixedNow, obs, params)
			if !errors.Is(err, ErrNotCertificateObservation) {
				t.Fatalf("err = %v, want ErrNotCertificateObservation", err)
			}
			if len(res.Certificates()) != 0 || len(res.Findings()) != 0 {
				t.Error("an environmental observation produced certificate results")
			}
		})
	}
}

// A successful outcome with no certificates is an error, not an empty success.
func TestObservedWithoutCertificates(t *testing.T) {
	obs := tlscert.Observation{CheckID: "c", Endpoint: "host:443", Outcome: tlscert.Observed, ObservedAt: fixedNow}
	if _, err := Adapt(fixedNow, obs, params); !errors.Is(err, ErrNoCertificates) {
		t.Fatalf("err = %v, want ErrNoCertificates", err)
	}
}

// ── detector errors surface per certificate, never as findings ──────────────

func TestDetectorErrorsAreReportedPerCertificate(t *testing.T) {
	base := certInput{
		CheckID: "chk-1", Endpoint: "observex.test:443", HostnameVerified: true,
		ObservedAt: fixedNow.Add(-time.Minute), NotAfter: fixedNow.Add(time.Hour),
		Position: 0, IsLeaf: true, Subject: "CN=observex.test",
	}
	cases := []struct {
		name    string
		in      certInput
		now     time.Time
		params  certexpiry.Params
		wantErr error
	}{
		{"observation too old", func() certInput { c := base; c.ObservedAt = fixedNow.Add(-maxAge - time.Second); return c }(), fixedNow, params, certexpiry.ErrObservationStale},
		{"observation in the future", func() certInput { c := base; c.ObservedAt = fixedNow.Add(time.Second); return c }(), fixedNow, params, certexpiry.ErrObservationInFuture},
		{"zero notAfter", func() certInput { c := base; c.NotAfter = time.Time{}; return c }(), fixedNow, params, certexpiry.ErrNoNotAfter},
		{"zero observedAt", func() certInput { c := base; c.ObservedAt = time.Time{}; return c }(), fixedNow, params, certexpiry.ErrNoObservedAt},
		{"no horizon", base, fixedNow, certexpiry.Params{MaxObservationAge: maxAge}, certexpiry.ErrHorizonNotSet},
		{"no maximum age", base, fixedNow, certexpiry.Params{Horizon: horizon}, certexpiry.ErrMaxAgeNotSet},
		{"zero evaluation time", base, time.Time{}, params, certexpiry.ErrNoEvaluationTime},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := adaptCertificate(tc.now, tc.in, tc.params)
			if got.Status != StatusEvaluated {
				t.Fatalf("status = %q; an eligible certificate must reach the detector", got.Status)
			}
			if !errors.Is(got.Err, tc.wantErr) {
				t.Fatalf("Err = %v, want %v", got.Err, tc.wantErr)
			}
			if got.Found || got.Finding != (certexpiry.Finding{}) {
				t.Errorf("a detector error produced a finding: %+v", got)
			}
			if !got.NotAfter.Equal(tc.in.NotAfter) {
				t.Error("the observation was not preserved on the error path")
			}
		})
	}
}

// The adapter supplies no horizon of its own: with empty Params nothing fires,
// however close to expiry the certificate is.
func TestNoDefaultHorizon(t *testing.T) {
	obs, _ := serveLeaf(t, fixedNow.Add(time.Hour)) // one hour from expiry
	res, err := Adapt(fixedNow, obs, certexpiry.Params{})
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	for _, c := range res.Certificates() {
		if c.Found {
			t.Fatalf("a finding was produced without a caller-supplied horizon: %+v", c)
		}
		if !errors.Is(c.Err, certexpiry.ErrHorizonNotSet) {
			t.Fatalf("Err = %v, want ErrHorizonNotSet", c.Err)
		}
	}
	if len(res.Findings()) != 0 {
		t.Error("Findings() is non-empty without a horizon")
	}
}

// ── determinism and value semantics ─────────────────────────────────────────

func TestAdaptIsDeterministicAndReturnsCopies(t *testing.T) {
	obs, _ := serveLeaf(t, fixedNow.Add(100*time.Hour))

	first, err := Adapt(fixedNow, obs, params)
	if err != nil {
		t.Fatalf("Adapt: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := Adapt(fixedNow, obs, params)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		a, b := got.Certificates(), first.Certificates()
		if len(a) != len(b) || a[0].Finding != b[0].Finding || a[0].Found != b[0].Found || a[0].Status != b[0].Status {
			t.Fatalf("iteration %d differed: %+v vs %+v", i, a[0], b[0])
		}
	}

	certs := first.Certificates()
	certs[0].Subject = "MUTATED"
	certs[0].Status = StatusSkipped
	if again := first.Certificates(); again[0].Subject == "MUTATED" || again[0].Status == StatusSkipped {
		t.Fatal("mutating a returned result changed the Result")
	}
	findings := first.Findings()
	if len(findings) > 0 {
		findings[0].Endpoint = "MUTATED"
		if first.Findings()[0].Endpoint == "MUTATED" {
			t.Fatal("mutating a returned finding changed the Result")
		}
	}
}
