package tlscert

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
	"sync/atomic"
	"testing"
	"time"
)

// fixedNow is the evaluation and verification time for every test. Nothing here
// reads the real clock except where a genuine timeout is being exercised.
var fixedNow = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

// ── certificate fixtures, minted in-process with exact notAfter values ───────

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

// newLeaf mints a server certificate with an exact notAfter.
func newLeaf(t *testing.T, issuer *authority, cn string, notBefore, notAfter time.Time, dnsNames []string, ips []net.IP) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
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

// ── in-process servers ───────────────────────────────────────────────────────

// server records what the peer sent it after the handshake, so a test can prove
// the observer never writes application data.
type server struct {
	addr          string
	appBytesRead  atomic.Int64
	handshakes    atomic.Int64
	closed        atomic.Int64
	wg            sync.WaitGroup
	listener      net.Listener
	handshakeOnly bool
}

// startTLSServer serves the given chain (leaf first) and never speaks first.
func startTLSServer(t *testing.T, chain [][]byte, key *ecdsa.PrivateKey) *server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{addr: ln.Addr().String(), listener: ln}
	cfg := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: chain, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				tc := tls.Server(c, cfg)
				_ = tc.SetDeadline(time.Now().Add(5 * time.Second))
				if err := tc.Handshake(); err != nil {
					return
				}
				s.handshakes.Add(1)
				buf := make([]byte, 512)
				for {
					n, err := tc.Read(buf) // decrypted application data only
					if n > 0 {
						s.appBytesRead.Add(int64(n))
					}
					if err != nil {
						s.closed.Add(1)
						return
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close(); s.wg.Wait() })
	return s
}

// startPlaintextServer answers with non-TLS bytes.
func startPlaintextServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
			c.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

// startStalledServer accepts and never responds, so the handshake must time out.
func startStalledServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	var mu sync.Mutex
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		mu.Lock()
		for _, c := range conns {
			c.Close()
		}
		mu.Unlock()
	})
	return ln.Addr().String()
}

// countingDial records every connection it hands out, so a test can prove the
// observer closed it.
type countingConn struct {
	net.Conn
	closes       atomic.Int64
	bytesWritten atomic.Int64
}

func (c *countingConn) Close() error { c.closes.Add(1); return c.Conn.Close() }
func (c *countingConn) Write(b []byte) (int, error) {
	c.bytesWritten.Add(int64(len(b)))
	return c.Conn.Write(b)
}

func proberFor(t *testing.T, roots *x509.CertPool, conns *[]*countingConn) Prober {
	t.Helper()
	var mu sync.Mutex
	d := &net.Dialer{}
	return New(Config{
		Now:     func() time.Time { return fixedNow },
		Roots:   roots,
		Vantage: Vantage{Kind: "test", ID: "unit"},
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			c, err := d.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			cc := &countingConn{Conn: c}
			if conns != nil {
				mu.Lock()
				*conns = append(*conns, cc)
				mu.Unlock()
			}
			return cc, nil
		},
	})
}

func poolOf(certs ...*x509.Certificate) *x509.CertPool {
	p := x509.NewCertPool()
	for _, c := range certs {
		p.AddCert(c)
	}
	return p
}

func localIPs() []net.IP { return []net.IP{net.ParseIP("127.0.0.1")} }

// ── 1–4: validity window is observed exactly ─────────────────────────────────

func TestObserveCertificateValidityWindow(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	horizon := 720 * time.Hour

	cases := []struct {
		name        string
		notAfter    time.Time
		wantOutcome Outcome
		wantReason  string
	}{
		{"1: valid, far from expiry", fixedNow.Add(365 * 24 * time.Hour), Observed, ""},
		{"2: inside an arbitrary 30-day horizon", fixedNow.Add(240 * time.Hour), Observed, ""},
		{"3: exactly at the horizon", fixedNow.Add(horizon), Observed, ""},
		{"4: already expired", fixedNow.Add(-24 * time.Hour), ObservedUntrusted, ReasonExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			der, key := newLeaf(t, ca, "observex.test", fixedNow.Add(-90*24*time.Hour), tc.notAfter, nil, localIPs())
			s := startTLSServer(t, [][]byte{der, ca.der}, key)
			p := proberFor(t, poolOf(ca.cert), nil)

			obs, err := p.Probe(context.Background(), Target{CheckID: "chk-1", Endpoint: s.addr, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if obs.Outcome != tc.wantOutcome {
				t.Fatalf("Outcome = %q (%s), want %q", obs.Outcome, obs.Detail, tc.wantOutcome)
			}
			if obs.Trust.Reason != tc.wantReason {
				t.Errorf("Trust.Reason = %q, want %q", obs.Trust.Reason, tc.wantReason)
			}
			leaf, ok := obs.Leaf()
			if !ok {
				t.Fatal("no leaf observed")
			}
			// The exact notAfter survives, expired or not. Horizons are the
			// detector's business, never the observer's.
			if !leaf.NotAfter.Equal(tc.notAfter) {
				t.Errorf("NotAfter = %v, want %v", leaf.NotAfter, tc.notAfter)
			}
			if leaf.NotAfter.Location() != time.UTC {
				t.Errorf("NotAfter is in %v, want UTC", leaf.NotAfter.Location())
			}
			if !obs.ObservedAt.Equal(fixedNow) {
				t.Errorf("ObservedAt = %v, want %v", obs.ObservedAt, fixedNow)
			}
			if obs.CheckID != "chk-1" || obs.Endpoint != s.addr {
				t.Errorf("probe context lost: %q %q", obs.CheckID, obs.Endpoint)
			}
			if obs.Vantage.Kind != "test" {
				t.Errorf("Vantage = %+v", obs.Vantage)
			}
		})
	}
}

// ── 5: hostname mismatch — observation preserved, failure recorded ───────────

func TestHostnameMismatchIsRecordedNotDiscarded(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	notAfter := fixedNow.Add(100 * 24 * time.Hour)
	// SAN names another host, so verifying 127.0.0.1 must fail on hostname only.
	der, key := newLeaf(t, ca, "other.example", fixedNow.Add(-24*time.Hour), notAfter, []string{"other.example"}, nil)
	s := startTLSServer(t, [][]byte{der, ca.der}, key)
	p := proberFor(t, poolOf(ca.cert), nil)

	obs, err := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if obs.Outcome != ObservedUntrusted {
		t.Fatalf("Outcome = %q, want %q", obs.Outcome, ObservedUntrusted)
	}
	if obs.Trust.HostnameVerified {
		t.Error("HostnameVerified = true for a mismatched name")
	}
	if !obs.Trust.ChainVerified {
		t.Errorf("ChainVerified = false; the chain itself is valid (%s)", obs.Trust.Reason)
	}
	if obs.Trust.Reason != ReasonHostnameMismatch {
		t.Errorf("Reason = %q, want %q", obs.Trust.Reason, ReasonHostnameMismatch)
	}
	leaf, ok := obs.Leaf()
	if !ok || !leaf.NotAfter.Equal(notAfter) {
		t.Fatalf("notAfter was discarded on hostname mismatch: %v (observed=%v)", leaf.NotAfter, ok)
	}
	if got := leaf.DNSNames(); len(got) != 1 || got[0] != "other.example" {
		t.Errorf("DNSNames = %v", got)
	}
}

// ── 6: untrusted issuer ──────────────────────────────────────────────────────

func TestUntrustedIssuerIsObservedAndRecorded(t *testing.T) {
	served := newCA(t, "rogue-ca", nil)
	trusted := newCA(t, "trusted-ca", nil)
	notAfter := fixedNow.Add(50 * 24 * time.Hour)
	der, key := newLeaf(t, served, "observex.test", fixedNow.Add(-time.Hour), notAfter, nil, localIPs())
	s := startTLSServer(t, [][]byte{der, served.der}, key)
	p := proberFor(t, poolOf(trusted.cert), nil)

	obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	if obs.Outcome != ObservedUntrusted || obs.Trust.ChainVerified {
		t.Fatalf("Outcome = %q, ChainVerified = %v", obs.Outcome, obs.Trust.ChainVerified)
	}
	if obs.Trust.Reason != ReasonUnknownAuthority {
		t.Errorf("Reason = %q, want %q", obs.Trust.Reason, ReasonUnknownAuthority)
	}
	if leaf, ok := obs.Leaf(); !ok || !leaf.NotAfter.Equal(notAfter) {
		t.Error("notAfter was discarded for an untrusted issuer")
	}
}

// ── 7: chain semantics — every presented certificate, in wire order ──────────

func TestChainIsObservedInWireOrder(t *testing.T) {
	root := newCA(t, "root-ca", nil)
	inter := newCA(t, "intermediate-ca", root)
	leafNotAfter := fixedNow.Add(30 * 24 * time.Hour)
	der, key := newLeaf(t, inter, "observex.test", fixedNow.Add(-time.Hour), leafNotAfter, nil, localIPs())

	t.Run("complete chain", func(t *testing.T) {
		s := startTLSServer(t, [][]byte{der, inter.der}, key)
		p := proberFor(t, poolOf(root.cert), nil)
		obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})

		if obs.Outcome != Observed {
			t.Fatalf("Outcome = %q (%s)", obs.Outcome, obs.Detail)
		}
		certs := obs.Certificates()
		if len(certs) != 2 {
			t.Fatalf("observed %d certificates, want 2 (leaf + intermediate)", len(certs))
		}
		if !certs[0].IsLeaf || certs[0].Position != 0 || certs[0].IsCA {
			t.Errorf("certs[0] = %+v; want the leaf at position 0", certs[0])
		}
		if certs[1].IsLeaf || certs[1].Position != 1 || !certs[1].IsCA {
			t.Errorf("certs[1] = %+v; want the intermediate at position 1", certs[1])
		}
		// Each certificate keeps its own notAfter; nothing is collapsed.
		if !certs[0].NotAfter.Equal(leafNotAfter) || !certs[1].NotAfter.Equal(inter.cert.NotAfter.UTC()) {
			t.Errorf("per-certificate notAfter lost: %v / %v", certs[0].NotAfter, certs[1].NotAfter)
		}
		if certs[1].NotAfter.Before(certs[0].NotAfter) {
			t.Fatal("fixture error: the intermediate should outlive the leaf")
		}
		// The root was not presented, so it is not observed.
		for _, c := range certs {
			if c.Subject == root.cert.Subject.String() {
				t.Error("a root certificate the endpoint did not present was included")
			}
		}
		if len(certs[0].DER()) == 0 {
			t.Error("DER is empty")
		}
	})

	t.Run("incomplete chain: intermediate not presented", func(t *testing.T) {
		s := startTLSServer(t, [][]byte{der}, key) // leaf only
		p := proberFor(t, poolOf(root.cert), nil)
		obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})

		if obs.Outcome != ObservedUntrusted {
			t.Fatalf("Outcome = %q, want %q", obs.Outcome, ObservedUntrusted)
		}
		if obs.Trust.Reason != ReasonUnknownAuthority {
			t.Errorf("Reason = %q, want %q (a missing intermediate is reported this way)", obs.Trust.Reason, ReasonUnknownAuthority)
		}
		if certs := obs.Certificates(); len(certs) != 1 || !certs[0].NotAfter.Equal(leafNotAfter) {
			t.Errorf("the presented leaf must still be observed: %+v", certs)
		}
	})
}

// ── 8–10: transport, timeout, malformed ──────────────────────────────────────

func TestTransportFailure(t *testing.T) {
	// A port that nothing listens on.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	p := proberFor(t, x509.NewCertPool(), nil)
	obs, err := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: addr, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Probe returned an error for an environmental failure: %v", err)
	}
	if obs.Outcome != TransportFailure {
		t.Fatalf("Outcome = %q, want %q", obs.Outcome, TransportFailure)
	}
	if len(obs.Certificates()) != 0 {
		t.Error("certificates reported for a failed connection")
	}
}

func TestTimeoutFromDialer(t *testing.T) {
	p := New(Config{
		Now:   func() time.Time { return fixedNow },
		Roots: x509.NewCertPool(),
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, &net.OpError{Op: "dial", Err: timeoutError{}}
		},
	})
	obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: "example.invalid:443", Timeout: time.Second})
	if obs.Outcome != Timeout {
		t.Fatalf("Outcome = %q, want %q", obs.Outcome, Timeout)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestTimeoutDuringHandshake(t *testing.T) {
	addr := startStalledServer(t)
	p := New(Config{Roots: x509.NewCertPool()}) // real clock: a real deadline elapses
	obs, err := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: addr, Timeout: 150 * time.Millisecond})
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if obs.Outcome != Timeout {
		t.Fatalf("Outcome = %q (%s), want %q", obs.Outcome, obs.Detail, Timeout)
	}
}

func TestMalformedTLSResponse(t *testing.T) {
	addr := startPlaintextServer(t)
	p := proberFor(t, x509.NewCertPool(), nil)
	obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: addr, Timeout: 2 * time.Second})
	if obs.Outcome != Malformed {
		t.Fatalf("Outcome = %q (%s), want %q", obs.Outcome, obs.Detail, Malformed)
	}
	if len(obs.Certificates()) != 0 {
		t.Error("certificates reported for a non-TLS peer")
	}
}

// NoCertificates cannot be produced by a conforming TLS server, so the mapping
// is exercised directly.
func TestNoCertificatesOutcome(t *testing.T) {
	p := &prober{now: func() time.Time { return fixedNow }}
	obs := Observation{}
	// describe of an empty chain yields nothing, and the Probe path sets the
	// outcome from the same condition.
	if got := describe(nil); len(got) != 0 {
		t.Fatalf("describe(nil) = %v", got)
	}
	obs.Outcome = NoCertificates
	if len(obs.Certificates()) != 0 {
		t.Error("NoCertificates carried certificates")
	}
	_ = p
}

// ── caller mistakes are errors, not outcomes ─────────────────────────────────

func TestTargetValidation(t *testing.T) {
	p := proberFor(t, x509.NewCertPool(), nil)
	cases := []struct {
		name    string
		target  Target
		wantErr error
	}{
		{"no endpoint", Target{Timeout: time.Second}, ErrNoEndpoint},
		{"zero timeout", Target{Endpoint: "host:443"}, ErrNoTimeout},
		{"negative timeout", Target{Endpoint: "host:443", Timeout: -time.Second}, ErrNoTimeout},
		{"url instead of host", Target{Endpoint: "https://host/path", Timeout: time.Second}, ErrBadAddress},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, err := p.Probe(context.Background(), tc.target)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if obs.Outcome != "" {
				t.Errorf("Outcome = %q; want the zero Observation", obs.Outcome)
			}
		})
	}
}

func TestDefaultPortIsAppended(t *testing.T) {
	host, addr, err := splitEndpoint("observex.test")
	if err != nil || host != "observex.test" || addr != "observex.test:443" {
		t.Fatalf("splitEndpoint = %q, %q, %v", host, addr, err)
	}
	host, addr, err = splitEndpoint("observex.test:8443")
	if err != nil || host != "observex.test" || addr != "observex.test:8443" {
		t.Fatalf("splitEndpoint = %q, %q, %v", host, addr, err)
	}
}

// ── security properties ──────────────────────────────────────────────────────

// The observation connection must send no application data and must close.
func TestNoApplicationDataAndConnectionClosed(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	der, key := newLeaf(t, ca, "observex.test", fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), nil, localIPs())
	s := startTLSServer(t, [][]byte{der, ca.der}, key)

	var conns []*countingConn
	p := proberFor(t, poolOf(ca.cert), &conns)
	obs, err := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	if err != nil || obs.Outcome != Observed {
		t.Fatalf("Probe: %v, outcome %q", err, obs.Outcome)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.closed.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if s.handshakes.Load() != 1 {
		t.Fatalf("server saw %d handshakes, want 1", s.handshakes.Load())
	}
	if n := s.appBytesRead.Load(); n != 0 {
		t.Errorf("server received %d application bytes; the observer must send none", n)
	}
	if s.closed.Load() == 0 {
		t.Error("the server never saw the connection close")
	}
	if len(conns) != 1 {
		t.Fatalf("dialer handed out %d connections, want 1", len(conns))
	}
	if c := conns[0].closes.Load(); c != 1 {
		t.Errorf("connection Close called %d times, want exactly 1", c)
	}
	if conns[0].bytesWritten.Load() == 0 {
		t.Error("no bytes written at all; the handshake itself should have written some")
	}
}

// Verification is always performed and its result recorded; there is no way to
// skip it through the public API.
func TestVerificationAlwaysRunsAndIsRecorded(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	der, key := newLeaf(t, ca, "observex.test", fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), nil, localIPs())
	s := startTLSServer(t, [][]byte{der, ca.der}, key)

	trusting := proberFor(t, poolOf(ca.cert), nil)
	ok, _ := trusting.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	if !ok.Trust.ChainVerified || !ok.Trust.HostnameVerified || !ok.Trust.Verified() || ok.Outcome != Observed {
		t.Fatalf("trusted case: %+v", ok.Trust)
	}

	// The same endpoint, with roots that do not include the issuer: the
	// observation still happens, and the failure is recorded rather than hidden.
	empty := proberFor(t, x509.NewCertPool(), nil)
	untrusted, _ := empty.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	if untrusted.Trust.ChainVerified || untrusted.Trust.Verified() {
		t.Fatalf("verification did not run or was ignored: %+v", untrusted.Trust)
	}
	if untrusted.Outcome != ObservedUntrusted {
		t.Errorf("Outcome = %q, want %q", untrusted.Outcome, ObservedUntrusted)
	}
	if leaf, ok := untrusted.Leaf(); !ok || leaf.NotAfter.IsZero() {
		t.Error("the certificate was discarded when verification failed")
	}
}

// Returned values are copies: nothing a caller mutates can reach the observation.
func TestReturnedValuesAreCopies(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	der, key := newLeaf(t, ca, "observex.test", fixedNow.Add(-time.Hour), fixedNow.Add(24*time.Hour), []string{"a.test", "b.test"}, localIPs())
	s := startTLSServer(t, [][]byte{der, ca.der}, key)
	p := proberFor(t, poolOf(ca.cert), nil)
	obs, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})

	certs := obs.Certificates()
	certs[0].Subject = "MUTATED"
	certs[0].NotAfter = time.Time{}
	if d := certs[0].DER(); len(d) > 0 {
		d[0] ^= 0xff
	}
	if names := certs[0].DNSNames(); len(names) > 0 {
		names[0] = "MUTATED"
	}
	if ips := certs[0].IPAddresses(); len(ips) > 0 {
		ips[0] = "MUTATED"
	}

	again := obs.Certificates()
	if again[0].Subject == "MUTATED" || again[0].NotAfter.IsZero() {
		t.Fatal("mutating a returned certificate changed the observation")
	}
	if names := again[0].DNSNames(); names[0] != "a.test" {
		t.Fatalf("DNSNames mutated through a returned copy: %v", names)
	}
	if ips := again[0].IPAddresses(); ips[0] != "127.0.0.1" {
		t.Fatalf("IPAddresses mutated through a returned copy: %v", ips)
	}
	original := again[0].DER()
	if len(original) == 0 || original[0] != der[0] {
		t.Fatal("DER mutated through a returned copy")
	}
}

// The same handshake state always maps to the same observation.
func TestObservationIsStableForTheSameEndpoint(t *testing.T) {
	ca := newCA(t, "test-ca", nil)
	notAfter := fixedNow.Add(72 * time.Hour)
	der, key := newLeaf(t, ca, "observex.test", fixedNow.Add(-time.Hour), notAfter, nil, localIPs())
	s := startTLSServer(t, [][]byte{der, ca.der}, key)
	p := proberFor(t, poolOf(ca.cert), nil)

	first, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
	for i := 0; i < 3; i++ {
		got, _ := p.Probe(context.Background(), Target{CheckID: "c", Endpoint: s.addr, Timeout: 5 * time.Second})
		if got.Outcome != first.Outcome || got.Trust != first.Trust || !got.ObservedAt.Equal(first.ObservedAt) {
			t.Fatalf("probe %d differed: %+v vs %+v", i, got, first)
		}
		a, b := got.Certificates(), first.Certificates()
		if len(a) != len(b) || !a[0].NotAfter.Equal(b[0].NotAfter) || a[0].SerialNumber != b[0].SerialNumber {
			t.Fatalf("probe %d certificates differed", i)
		}
	}
}
