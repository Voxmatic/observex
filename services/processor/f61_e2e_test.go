package main

// End-to-end F6.1 path in one test process, over real sockets and a real
// PostgreSQL with every migration applied (OBSERVEX_TEST_POSTGRES_DSN; the
// test skips without it):
//
//	TLS endpoint ──handshake── probe Runner (tlscert + internal/probe/f61)
//	       probe ──HTTPS, bearer probe credential── processor's dedicated TLS listener
//	   processor ──work, intake, revocation, results── PostgreSQL (migrations 001–009)
//
// The credential is issued with internal/probetoken, the code the gateway's
// issuance route calls; the revocation is written with the result store's
// Revoke, the code the gateway's revoke route calls. The gateway itself and
// Kubernetes are not part of this test.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/observex/platform/internal/db/dbtest"
	evaluate "github.com/observex/platform/internal/evaluate/f61"
	intakef61 "github.com/observex/platform/internal/intake/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	probef61 "github.com/observex/platform/internal/probe/f61"
	"github.com/observex/platform/internal/probetoken"
	resultf61 "github.com/observex/platform/internal/result/f61"
)

// f61CA is a throwaway certificate authority.
type f61CA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newF61CA(t *testing.T, name string, now time.Time) f61CA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(3 * 365 * 24 * time.Hour),
		IsCA: true, KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return f61CA{cert: cert, key: key, pool: pool}
}

// leaf issues a certificate for 127.0.0.1 valid until notAfter.
func (ca f61CA) leaf(t *testing.T, serial int64, now, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "f61-e2e"},
		NotBefore: now.Add(-time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key, Leaf: leaf}
}

// writePair writes a certificate and key as PEM files.
func writePair(t *testing.T, dir string, c tls.Certificate) (string, string) {
	t.Helper()
	keyDER, err := x509.MarshalECPrivateKey(c.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	var certPEM bytes.Buffer
	for _, d := range c.Certificate {
		_ = pem.Encode(&certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: d})
	}
	cf, kf := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	if err := os.WriteFile(cf, certPEM.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kf, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return cf, kf
}

// f61Endpoint serves whichever certificate is current.
func f61Endpoint(t *testing.T, cert *atomic.Pointer[tls.Certificate]) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return cert.Load(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				_ = c.(*tls.Conn).Handshake()
				_, _ = c.Read(make([]byte, 1))
			}(c)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

type f61Clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *f61Clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *f61Clock) advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	return c.t
}

func seedCheck(t *testing.T, pool *pgxpool.Pool, id, org, ns, target string, enabled bool, locations []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO synthetic_checks
		(id, org_id, name, type, target, namespace, enabled, locations, interval_sec, timeout_sec)
		VALUES ($1, $2, $3, 'ssl', $4, $5, $6, $7, 60, 5)`, id, org, "f61 "+id, target, ns, enabled, locations); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestF61EndToEndAgainstPostgres(t *testing.T) {
	pool := dbtest.NewDatabase(t) // skips without OBSERVEX_TEST_POSTGRES_DSN
	ctx := context.Background()
	clk := &f61Clock{t: time.Now().UTC().Truncate(time.Second)}
	base := clk.now()

	// The monitored endpoint: first a certificate expiring in 20 days.
	endpointCA := newF61CA(t, "f61-endpoint-ca", base)
	var current atomic.Pointer[tls.Certificate]
	expiring := endpointCA.leaf(t, 2, base, base.Add(20*24*time.Hour))
	current.Store(&expiring)
	endpoint := f61Endpoint(t, &current)

	seedCheck(t, pool, "chk-e2e", "org-a", "payments", endpoint, true, []string{"e2e"})
	seedCheck(t, pool, "chk-any", "org-a", "payments", endpoint, true, []string{"*"})
	seedCheck(t, pool, "chk-eu", "org-a", "default", endpoint, true, []string{"eu-west"})
	seedCheck(t, pool, "chk-local", "org-a", "default", endpoint, true, []string{"local"})
	seedCheck(t, pool, "chk-off", "org-a", "default", endpoint, false, []string{"*"})
	seedCheck(t, pool, "chk-foreign", "org-b", "default", endpoint, true, []string{"*"})

	// The processor's dedicated listener, with a real server certificate.
	processorCA := newF61CA(t, "f61-processor-ca", base)
	certFile, keyFile := writePair(t, t.TempDir(), processorCA.leaf(t, 3, base, base.Add(90*24*time.Hour)))
	ln, tlsOn, err := listenF61(f61ListenerConfig{addr: "127.0.0.1:0", certFile: certFile, keyFile: keyFile})
	if err != nil || !tlsOn {
		t.Fatalf("listener: %v tls=%v", err, tlsOn)
	}
	core, logs := observer.New(zapcore.DebugLevel)
	key := probetoken.NewKey(f61TestKey)
	app := newF61IntakeApp(f61Deps{logger: zap.New(core), key: key, db: pool, horizon: evaluate.DefaultHorizon, now: clk.now})
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })
	processorURL := "https://" + ln.Addr().String()

	// The vantage's credential, as the gateway would issue it.
	issued, err := key.Issue(probetoken.IssueRequest{OrgID: "org-a", Declared: probetoken.Declaration{NetworkZone: "e2e"}}, base, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := probetoken.ParseCredential(issued.Credential.Reveal() + "\n") // as read from its Secret file
	if err != nil {
		t.Fatal(err)
	}

	// Without the processor CA the probe refuses the listener: TLS is verified.
	untrusting, _ := probef61.NewClient(probef61.ClientConfig{ProcessorURL: processorURL, Credential: cred})
	if _, _, err := untrusting.Work(ctx); !errors.Is(err, probef61.ErrTransport) {
		t.Fatalf("unverified processor accepted: %v", err)
	}

	client, err := probef61.NewClient(probef61.ClientConfig{ProcessorURL: processorURL, Credential: cred, RootCAs: processorCA.pool})
	if err != nil {
		t.Fatal(err)
	}
	prober := tlscert.New(tlscert.Config{
		Dial:  probef61.GuardedDialer{AllowPrivate: true}.Dial, // the endpoint is on loopback
		Roots: endpointCA.pool, Now: clk.now,
	})
	var rotated atomic.Pointer[probetoken.Credential]
	runner := &probef61.Runner{Client: client, Prober: prober, Now: clk.now,
		ReloadCredential: func() (probetoken.Credential, error) {
			if c := rotated.Load(); c != nil {
				return *c, nil
			}
			return cred, nil
		}}
	store := resultf61.New(pool)

	// 1. Work: only the checks assigned to zone "e2e" (explicitly or "*").
	if n := runner.Step(ctx); n != 0 {
		t.Fatalf("launched %d on first sight", n)
	}
	got := map[string]probef61.Assignment{}
	for _, a := range runner.Assignments() {
		got[a.CheckID] = a
	}
	if len(got) != 2 || got["chk-e2e"].Endpoint != endpoint || got["chk-any"].Endpoint != endpoint ||
		got["chk-e2e"].IntervalSec != 60 || got["chk-e2e"].TimeoutSec != 5 ||
		got["chk-e2e"].PhaseSec != intakef61.Phase("chk-e2e", issued.VantageID, 60) {
		t.Fatalf("assignments %+v", got)
	}

	// 2. Next slot: both checks are probed and reported; the result opens.
	clk.advance(61 * time.Second)
	if n := runner.Step(ctx); n != 2 {
		t.Fatalf("launched %d, want 2", n)
	}
	runner.Wait()
	res, err := store.Get(ctx, "org-a", "chk-e2e", clk.now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != string(evaluate.StatusExpiring) || res.AlertState != resultf61.AlertOpen || res.Namespace != "payments" ||
		res.VantagesInSpan != 1 || res.EarliestNotAfter == nil || !res.EarliestNotAfter.Equal(expiring.Leaf.NotAfter.UTC()) {
		t.Fatalf("result after first report: %+v", res)
	}
	episode := res.EpisodeID

	// 3. The certificate is renewed; the next report resolves the episode.
	renewed := endpointCA.leaf(t, 4, base, base.Add(200*24*time.Hour))
	current.Store(&renewed)
	clk.advance(60 * time.Second)
	if n := runner.Step(ctx); n != 2 {
		t.Fatalf("launched %d, want 2", n)
	}
	runner.Wait()
	res, err = store.Get(ctx, "org-a", "chk-e2e", clk.now())
	if err != nil || res.Status != string(evaluate.StatusOK) || res.AlertState != resultf61.AlertNone {
		t.Fatalf("result after renewal: %+v %v", res, err)
	}
	events, err := store.Events(ctx, "org-a", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var e2e []resultf61.Event
	for _, e := range events {
		if e.CheckID == "chk-e2e" {
			e2e = append(e2e, e)
		}
	}
	if len(e2e) != 2 || e2e[0].Type != resultf61.EventOpened || e2e[1].Type != resultf61.EventResolved ||
		e2e[0].EpisodeID != episode || e2e[1].EpisodeID != episode {
		t.Fatalf("events %+v", e2e)
	}

	// 4. Replay defence: an older observation is acknowledged and ignored; one
	// outside the report window is refused.
	older := tlscert.New(tlscert.Config{Dial: probef61.GuardedDialer{AllowPrivate: true}.Dial, Roots: endpointCA.pool,
		Now: func() time.Time { return clk.now().Add(-90 * time.Second) }})
	_, receipt, err := probef61.ProbeAndReport(ctx, older, client, tlscert.Target{CheckID: "chk-e2e", Endpoint: endpoint, Timeout: 5 * time.Second})
	if err != nil || !receipt.Accepted || receipt.Stored {
		t.Fatalf("older report: %+v %v", receipt, err)
	}
	stale := tlscert.New(tlscert.Config{Dial: probef61.GuardedDialer{AllowPrivate: true}.Dial, Roots: endpointCA.pool,
		Now: func() time.Time { return clk.now().Add(-intakef61.MaxReportDelay - time.Minute) }})
	_, receipt, err = probef61.ProbeAndReport(ctx, stale, client, tlscert.Target{CheckID: "chk-e2e", Endpoint: endpoint, Timeout: 5 * time.Second})
	if !errors.Is(err, probef61.ErrRejected) || receipt.Status != 400 || receipt.Reason != "observation_time_out_of_range" {
		t.Fatalf("stale report: %+v %v", receipt, err)
	}

	// 5. Tenant isolation and location assignment: identical refusals, no rows.
	for _, id := range []string{"chk-foreign", "chk-eu", "chk-local", "chk-off", "chk-missing"} {
		_, receipt, err := probef61.ProbeAndReport(ctx, prober, client, tlscert.Target{CheckID: id, Endpoint: endpoint, Timeout: 5 * time.Second})
		if !errors.Is(err, probef61.ErrForbidden) || receipt.Status != 403 || receipt.Reason != "forbidden" {
			t.Fatalf("%s: %+v, %v", id, receipt, err)
		}
	}
	if _, err := store.Get(ctx, "org-b", "chk-e2e", clk.now()); !errors.Is(err, resultf61.ErrNotFound) {
		t.Fatalf("org-b sees org-a's result: %v", err)
	}
	if l, err := store.List(ctx, "org-b", "", clk.now()); err != nil || len(l) != 0 {
		t.Fatalf("org-b list: %v %v", l, err)
	}
	var foreignRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM f61_tls_observations WHERE check_id <> ALL($1)`,
		[]string{"chk-e2e", "chk-any"}).Scan(&foreignRows); err != nil || foreignRows != 0 {
		t.Fatalf("refused reports left %d observation rows (%v)", foreignRows, err)
	}

	// 6. The SSRF guard: without AllowPrivate a loopback target is never
	// dialled; the report is still made and the result says unreachable.
	clk.advance(time.Second) // strictly newer than chk-any's last report
	guarded := tlscert.New(tlscert.Config{Dial: probef61.GuardedDialer{}.Dial, Roots: endpointCA.pool, Now: clk.now})
	obs, receipt, err := probef61.ProbeAndReport(ctx, guarded, client, tlscert.Target{CheckID: "chk-any", Endpoint: endpoint, Timeout: 5 * time.Second})
	if err != nil || obs.Outcome != tlscert.TransportFailure || !receipt.Stored || receipt.ResultStatus != string(evaluate.StatusUnreachable) {
		t.Fatalf("guarded probe: %s %+v %v", obs.Outcome, receipt, err)
	}

	// 7. Revocation: credentials issued before the bound are refused on both
	// routes, the runner drops its work, and a re-issued credential restores it.
	bound := clk.advance(time.Second).Add(time.Second)
	if _, err := store.Revoke(ctx, "org-a", issued.VantageID, bound, false, "e2e rotation", "tester"); err != nil {
		t.Fatal(err)
	}
	clk.advance(2 * time.Second)
	_, receipt, err = probef61.ProbeAndReport(ctx, prober, client, tlscert.Target{CheckID: "chk-e2e", Endpoint: endpoint, Timeout: 5 * time.Second})
	if !errors.Is(err, probef61.ErrCredentialRejected) || receipt.Status != 401 {
		t.Fatalf("revoked credential report: %+v %v", receipt, err)
	}
	clk.advance(60 * time.Second) // work refresh due
	runner.Step(ctx)
	runner.Wait()
	if n := len(runner.Assignments()); n != 0 {
		t.Fatalf("runner kept %d assignments after revocation", n)
	}
	reissued, err := key.Issue(probetoken.IssueRequest{OrgID: "org-a", VantageID: issued.VantageID,
		Declared: probetoken.Declaration{NetworkZone: "e2e"}}, clk.now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rotated.Store(&reissued.Credential)
	clk.advance(16 * time.Second) // the runner backs off after a rejection
	runner.Step(ctx)
	if n := len(runner.Assignments()); n != 2 {
		t.Fatalf("re-issued credential: %d assignments", n)
	}

	// 8. Decommission: nothing issued for the vantage is ever accepted again.
	if _, err := store.Revoke(ctx, "org-a", issued.VantageID, time.Time{}, true, "decommissioned", "tester"); err != nil {
		t.Fatal(err)
	}
	later, _ := key.Issue(probetoken.IssueRequest{OrgID: "org-a", VantageID: issued.VantageID,
		Declared: probetoken.Declaration{NetworkZone: "e2e"}}, clk.advance(time.Hour), rand.Reader)
	client.SetCredential(later.Credential)
	if _, status, err := client.Work(ctx); !errors.Is(err, probef61.ErrCredentialRejected) || status != 401 {
		t.Fatalf("decommissioned vantage: %d %v", status, err)
	}

	for _, c := range []probetoken.Credential{cred, reissued.Credential, later.Credential} {
		f61AssertLogsClean(t, logs, c.Reveal())
	}
	for _, e := range logs.All() {
		if strings.Contains(e.Message, "WITHOUT TLS") {
			t.Fatal("plaintext warning on a TLS listener")
		}
	}
}

// The lookup reads the migrated table; an injection-shaped ID is just an ID.
func TestF61LookupAgainstPostgres(t *testing.T) {
	pool := dbtest.NewDatabase(t)
	seedCheck(t, pool, "chk-pg", "org-a", "payments", "api.example.com:443", true, []string{"e2e", "*"})
	l := pgSyntheticCheckLookup{db: pool}
	r, err := l.LookupCheck(context.Background(), "chk-pg")
	if err != nil || r.Row.OrgID != "org-a" || r.Row.Namespace != "payments" || r.Row.Target != "api.example.com:443" ||
		r.Type != "ssl" || !r.Enabled || len(r.Locations) != 2 || r.IntervalSec != 60 || r.TimeoutSec != 5 {
		t.Fatalf("got %+v, %v", r, err)
	}
	for _, id := range []string{"chk-none", "x' OR '1'='1"} {
		if _, err := l.LookupCheck(context.Background(), id); !errors.Is(err, intakef61.ErrCheckNotFound) {
			t.Fatalf("%q: %v", id, err)
		}
	}
	list, err := l.ListChecks(context.Background(), "org-a")
	if err != nil || len(list) != 1 || list[0].Row.ID != "chk-pg" {
		t.Fatalf("list %+v %v", list, err)
	}
	if list, err := l.ListChecks(context.Background(), "org-b"); err != nil || len(list) != 0 {
		t.Fatalf("org-b list %+v %v", list, err)
	}
}

// The listener opens only with a certificate or the explicit plaintext switch.
func TestF61ListenerModes(t *testing.T) {
	if _, _, err := listenF61(f61ListenerConfig{addr: "127.0.0.1:0"}); !errors.Is(err, errF61NotStarted) {
		t.Fatalf("no configuration: %v", err)
	}
	for _, v := range []string{"1", "TRUE", "yes", " true"} {
		t.Setenv("OBSERVEX_F61_INTAKE_INSECURE_PLAINTEXT", v)
		if f61ListenerFromEnv().plaintext {
			t.Fatalf("%q enabled plaintext", v)
		}
	}
	t.Setenv("OBSERVEX_F61_INTAKE_INSECURE_PLAINTEXT", "true")
	if !f61ListenerFromEnv().plaintext {
		t.Fatal("exact \"true\" did not enable plaintext")
	}
	ln, tlsOn, err := listenF61(f61ListenerConfig{addr: "127.0.0.1:0", plaintext: true})
	if err != nil || tlsOn {
		t.Fatalf("plaintext: %v %v", tlsOn, err)
	}
	ln.Close()

	dir := t.TempDir()
	if _, _, err := listenF61(f61ListenerConfig{addr: "127.0.0.1:0", certFile: filepath.Join(dir, "none"), keyFile: filepath.Join(dir, "none")}); err == nil || errors.Is(err, errF61NotStarted) {
		t.Fatalf("missing certificate: %v", err)
	}

	// A configured certificate wins over the plaintext switch, and a renewed
	// pair is served without a restart.
	now := time.Now()
	ca := newF61CA(t, "f61-listener-ca", now)
	cf, kf := writePair(t, dir, ca.leaf(t, 10, now, now.Add(24*time.Hour)))
	ln, tlsOn, err = listenF61(f61ListenerConfig{addr: "127.0.0.1:0", certFile: cf, keyFile: kf, plaintext: true})
	if err != nil || !tlsOn {
		t.Fatalf("tls: %v %v", tlsOn, err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { _ = c.(*tls.Conn).Handshake(); c.Close() }(c)
		}
	}()
	serial := func() int64 {
		c, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: ca.pool, MinVersion: tls.VersionTLS12})
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.ConnectionState().PeerCertificates[0].SerialNumber.Int64()
	}
	if s := serial(); s != 10 {
		t.Fatalf("serial %d", s)
	}
	// The reloader behind the listener picks up a renewed pair.
	r, err := newCertReloader(cf, kf)
	if err != nil {
		t.Fatal(err)
	}
	leafSerial := func() int64 {
		c, _ := r.get(nil)
		l, err := x509.ParseCertificate(c.Certificate[0])
		if err != nil {
			t.Fatal(err)
		}
		return l.SerialNumber.Int64()
	}
	if s := leafSerial(); s != 10 {
		t.Fatalf("reloader serial %d", s)
	}
	writePair(t, dir, ca.leaf(t, 11, now, now.Add(48*time.Hour)))
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(cf, future, future); err != nil {
		t.Fatal(err)
	}
	if s := leafSerial(); s != 10 {
		t.Fatal("reloader re-read the files inside its check interval")
	}
	r.mu.Lock()
	r.checked = time.Time{} // the check interval has passed
	r.mu.Unlock()
	if s := leafSerial(); s != 11 {
		t.Fatalf("renewed pair not loaded: serial %d", s)
	}
	// A broken renewal keeps the previous pair.
	if err := os.WriteFile(cf, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	future = future.Add(time.Minute)
	_ = os.Chtimes(cf, future, future)
	r.mu.Lock()
	r.checked = time.Time{}
	r.mu.Unlock()
	if s := leafSerial(); s != 11 {
		t.Fatalf("broken renewal replaced the pair: serial %d", s)
	}
	// A plain HTTP client cannot talk to the TLS listener.
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("GET /v1/synthetic/probe/work HTTP/1.1\r\nHost: x\r\n\r\n"))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, _ := conn.Read(buf)
	conn.Close()
	if strings.Contains(string(buf[:n]), "HTTP/1.1 200") {
		t.Fatal("plaintext request served on the TLS listener")
	}
}
