package main

// D1 regression: the probe dials the processor's intake listener by the name
// the Helm chart renders, <intake Service>.<namespace>.svc, and verifies the
// listener certificate against that name. These tests run the real listener
// (listenF61, with the certificate reloader) and the real probe client
// (probef61.Client.Work). Only name resolution is replaced: the test's dialer
// sends that name to the listener's loopback address, as cluster DNS would to
// the Service. The client's TLS settings are the ones NewClient builds for a
// configured CA (TLS 1.2 minimum, verification on); nothing is relaxed.

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
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	evaluate "github.com/observex/platform/internal/evaluate/f61"
	probef61 "github.com/observex/platform/internal/probe/f61"
	"github.com/observex/platform/internal/probetoken"
)

// The name the chart renders for release "observex" in namespace "observex"
// (checked against the chart in deployments/helm/chartcheck).
const f61IntakeTLSName = "observex-processor-probe-intake.observex.svc"

// dnsLeaf issues a server certificate for the given DNS names.
func (ca f61CA) dnsLeaf(t *testing.T, serial int64, notBefore, notAfter time.Time, names ...string) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "f61-intake"},
		NotBefore: notBefore, NotAfter: notAfter, DNSNames: names,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key, Leaf: leaf}
}

// intakeWithCert starts the real TLS listener serving cert and the probe
// endpoints (fake database), and returns its address.
func intakeWithCert(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	certFile, keyFile := writePair(t, t.TempDir(), cert)
	ln, tlsOn, err := listenF61(f61ListenerConfig{addr: "127.0.0.1:0", certFile: certFile, keyFile: keyFile})
	if err != nil || !tlsOn {
		t.Fatalf("listener: %v tls=%v", err, tlsOn)
	}
	app := newF61IntakeApp(f61Deps{logger: zap.NewNop(), key: probetoken.NewKey(f61TestKey), db: newFakeDB(),
		horizon: evaluate.DefaultHorizon, now: func() time.Time { return f61Now }})
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	return ln.Addr().String()
}

// probeClient is the real probe client for https://<name>:<port>, trusting
// roots, with name resolution sent to addr.
func probeClient(t *testing.T, name, addr string, roots *x509.CertPool) *probef61.Client {
	t.Helper()
	_, port, _ := net.SplitHostPort(addr)
	cred, err := probetoken.ParseCredential(func() string { c, _ := f61Credential(t, "org-a"); return c }())
	if err != nil {
		t.Fatal(err)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		// Same TLS settings as probef61.NewClient builds for ClientConfig.RootCAs.
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(address)
			if host != name {
				return nil, errors.New("unexpected dial to " + address)
			}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	c, err := probef61.NewClient(probef61.ClientConfig{
		ProcessorURL: "https://" + net.JoinHostPort(name, port), Credential: cred,
		HTTPClient: &http.Client{Transport: transport, Timeout: 10 * time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestF61ProbeVerifiesIntakeServiceName(t *testing.T) {
	now := time.Now()
	ca := newF61CA(t, "f61-intake-ca", now)
	other := newF61CA(t, "f61-untrusted-ca", now)
	valid := func(names ...string) tls.Certificate {
		return ca.dnsLeaf(t, 10, now.Add(-time.Hour), now.Add(24*time.Hour), names...)
	}
	cases := []struct {
		name string
		cert tls.Certificate
		want string // "" = success; otherwise a fragment of the verification error
	}{
		{"certificate for the chart's intake name", valid(f61IntakeTLSName), ""},
		{"certificate for the short Service name only (the D1 mismatch)", valid("observex-processor-probe-intake"),
			"certificate is valid for observex-processor-probe-intake, not " + f61IntakeTLSName},
		{"certificate for a different host", valid("processor.other.svc"), "not " + f61IntakeTLSName},
		{"certificate from an untrusted CA", other.dnsLeaf(t, 11, now.Add(-time.Hour), now.Add(24*time.Hour), f61IntakeTLSName),
			"unknown authority"},
		{"expired certificate", ca.dnsLeaf(t, 12, now.Add(-48*time.Hour), now.Add(-time.Hour), f61IntakeTLSName),
			"expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr := intakeWithCert(t, tc.cert)
			ws, status, err := probeClient(t, f61IntakeTLSName, addr, ca.pool).Work(context.Background())
			if tc.want == "" {
				if err != nil || status != 200 || len(ws.Assignments) == 0 {
					t.Fatalf("work over TLS: status %d, %d assignments, err %v", status, len(ws.Assignments), err)
				}
				return
			}
			if err == nil || !errors.Is(err, probef61.ErrTransport) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want a verification failure containing %q, got status %d err %v", tc.want, status, err)
			}
		})
	}
}

// The certificate carrying the chart's intake name also verifies with the
// plain crypto/x509 check the probe's TLS stack performs.
func TestF61IntakeNameVerifiesAgainstSAN(t *testing.T) {
	now := time.Now()
	ca := newF61CA(t, "f61-intake-ca", now)
	leaf := ca.dnsLeaf(t, 20, now.Add(-time.Hour), now.Add(time.Hour), f61IntakeTLSName).Leaf
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: f61IntakeTLSName, Roots: ca.pool}); err != nil {
		t.Fatalf("SAN %v does not verify for %s: %v", leaf.DNSNames, f61IntakeTLSName, err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "observex-processor-probe-intake", Roots: ca.pool}); err == nil {
		t.Fatal("a .svc-only certificate unexpectedly verified for the short name")
	}
}
