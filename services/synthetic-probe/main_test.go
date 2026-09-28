package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	probef61 "github.com/observex/platform/internal/probe/f61"
	"github.com/observex/platform/internal/probetoken"
)

func testCred(t *testing.T) string {
	t.Helper()
	out, err := probetoken.NewKey(strings.Repeat("p", 40)).Issue(probetoken.IssueRequest{OrgID: "org-a"}, time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential.Reveal()
}

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func noFiles(string) ([]byte, error) { return nil, fs.ErrNotExist }

// A transport failure is a valid observation, so no TLS fixture is needed to
// exercise the command; the full path with certificates is covered in
// services/processor/f61_e2e_test.go.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

type intake struct {
	status int
	hits   atomic.Int64
	auth   atomic.Value
}

func (i *intake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	i.hits.Add(1)
	i.auth.Store(r.Header.Get("Authorization"))
	_, _ = io.Copy(io.Discard, r.Body)
	w.WriteHeader(i.status)
	if i.status == 202 {
		_, _ = io.WriteString(w, `{"status":"accepted","stored":true,"evaluated":true,"result_status":"unreachable"}`)
	} else {
		_, _ = io.WriteString(w, `{"reason":"forbidden"}`)
	}
}

func testDeps(env map[string]string, files map[string][]byte, out, errb io.Writer) deps {
	return deps{
		lookupEnv: envOf(env),
		readFile: func(p string) ([]byte, error) {
			if b, ok := files[p]; ok {
				return b, nil
			}
			return nil, fs.ErrNotExist
		},
		newProber: func(allowPrivate bool) tlscert.Prober {
			return tlscert.New(tlscert.Config{Roots: x509.NewCertPool(), Dial: probef61.GuardedDialer{AllowPrivate: allowPrivate}.Dial})
		},
		stdout: out, stderr: errb, listen: net.Listen,
	}
}

func runCmd(t *testing.T, args []string, env map[string]string) (int, result, string) {
	t.Helper()
	return runCmdFiles(t, args, env, nil)
}

func runCmdFiles(t *testing.T, args []string, env map[string]string, files map[string][]byte) (int, result, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(context.Background(), args, testDeps(env, files, &out, &errb))
	var r result
	if out.Len() > 0 {
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("stdout is not one JSON line: %q", out.String())
		}
	}
	return code, r, out.String() + errb.String()
}

func TestCommandReportsOnceAndExitsZeroWhenAccepted(t *testing.T) {
	cred := testCred(t)
	in := &intake{status: 202}
	srv := httptest.NewServer(in)
	defer srv.Close()
	addr := closedAddr(t)
	code, r, all := runCmd(t, []string{"-check-id", "chk-1", "-endpoint", addr, "-timeout", "2s", "-allow-plaintext", "-allow-private-targets"},
		map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: srv.URL})
	if code != 0 || !r.Accepted || !r.Stored || r.ResultStatus != "unreachable" || !r.Delivered || r.Status != 202 || r.Outcome != string(tlscert.TransportFailure) || r.CheckID != "chk-1" {
		t.Fatalf("code %d result %+v", code, r)
	}
	if in.hits.Load() != 1 || in.auth.Load() != "Bearer "+cred {
		t.Fatal("the report was not sent exactly once with the credential")
	}
	if strings.Contains(all, strings.TrimPrefix(cred, probetoken.TokenPrefix)) {
		t.Fatal("the credential was printed")
	}
}

func TestCommandExitsOneWhenRefusedOrUndelivered(t *testing.T) {
	cred := testCred(t)
	env := map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred}
	srv := httptest.NewServer(&intake{status: 403})
	defer srv.Close()
	code, r, all := runCmd(t, []string{"-check-id", "c", "-endpoint", closedAddr(t), "-processor-url", srv.URL, "-allow-plaintext"}, env)
	if code != 1 || r.Accepted || r.Status != 403 || r.Reason != "forbidden" || strings.Contains(all, strings.TrimPrefix(cred, probetoken.TokenPrefix)) {
		t.Fatalf("refused: code %d %+v", code, r)
	}
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	code, r, _ = runCmd(t, []string{"-check-id", "c", "-endpoint", closedAddr(t), "-processor-url", url, "-allow-plaintext"}, env)
	if code != 1 || r.Delivered || r.Error == "" {
		t.Fatalf("undelivered: code %d %+v", code, r)
	}
}

func TestCommandUsageAndConfigurationErrors(t *testing.T) {
	cred := testCred(t)
	env := map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: "https://processor.example"}
	cases := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"no check id", []string{"-endpoint", "a.example"}, env},
		{"no endpoint", []string{"-check-id", "c"}, env},
		{"timeout too long", []string{"-check-id", "c", "-endpoint", "a.example", "-timeout", "61s"}, env},
		{"url as endpoint", []string{"-check-id", "c", "-endpoint", "https://a.example/"}, env},
		{"extra argument", []string{"-check-id", "c", "-endpoint", "a.example", "more"}, env},
		{"no credential", []string{"-check-id", "c", "-endpoint", "a.example"}, map[string]string{processorURLEnvVar: "https://processor.example"}},
		{"plaintext without opt-in", []string{"-check-id", "c", "-endpoint", "a.example", "-processor-url", "http://processor:8080"}, env},
		{"no processor url", []string{"-check-id", "c", "-endpoint", "a.example"}, map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if code, _, all := runCmd(t, tc.args, tc.env); code != 2 || strings.Contains(all, strings.TrimPrefix(cred, probetoken.TokenPrefix)) {
				t.Fatalf("code %d, want 2 (output leaked credential: %v)", code, strings.Contains(all, cred))
			}
		})
	}
}

// countingListener accepts and closes connections, counting them.
func countingListener(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			n.Add(1)
			c.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String(), &n
}

// The SSRF guard: a loopback target is not dialled unless private targets
// are allowed; the observation is still reported.
func TestCommandGuardsPrivateTargets(t *testing.T) {
	cred := testCred(t)
	srv := httptest.NewServer(&intake{status: 202})
	defer srv.Close()
	target, dials := countingListener(t)
	env := map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: srv.URL}
	code, r, _ := runCmd(t, []string{"-check-id", "c", "-endpoint", target, "-allow-plaintext"}, env)
	if code != 0 || r.Outcome != string(tlscert.TransportFailure) || dials.Load() != 0 {
		t.Fatalf("guarded: code %d %+v dials %d", code, r, dials.Load())
	}
	env[allowPrivateEnvVar] = "yes" // only exactly "true" relaxes the guard
	if runCmd(t, []string{"-check-id", "c", "-endpoint", target, "-allow-plaintext"}, env); dials.Load() != 0 {
		t.Fatal("a non-\"true\" value relaxed the guard")
	}
	env[allowPrivateEnvVar] = "true"
	if code, _, _ := runCmd(t, []string{"-check-id", "c", "-endpoint", target, "-allow-plaintext"}, env); code != 0 || dials.Load() != 1 {
		t.Fatalf("allowed: code %d dials %d", code, dials.Load())
	}
}

// The processor's certificate is verified against the configured roots.
func TestCommandVerifiesProcessorCertificate(t *testing.T) {
	cred := testCred(t)
	in := &intake{status: 202}
	srv := httptest.NewTLSServer(in)
	defer srv.Close()
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	env := map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: srv.URL}
	args := []string{"-check-id", "c", "-endpoint", closedAddr(t), "-allow-private-targets"}

	if code, r, _ := runCmd(t, args, env); code != 1 || r.Delivered || in.hits.Load() != 0 {
		t.Fatalf("unknown processor CA: code %d %+v hits %d", code, r, in.hits.Load())
	}
	env[processorCAEnvVar] = "/etc/observex/processor-ca.crt"
	files := map[string][]byte{"/etc/observex/processor-ca.crt": caPEM}
	if code, r, _ := runCmdFiles(t, args, env, files); code != 0 || !r.Accepted || in.hits.Load() != 1 {
		t.Fatalf("configured CA: code %d %+v", code, r)
	}
	for name, f := range map[string]map[string][]byte{"missing CA file": nil, "no PEM in CA file": {"/etc/observex/processor-ca.crt": []byte("junk")}} {
		if code, _, _ := runCmdFiles(t, args, env, f); code != 2 {
			t.Errorf("%s: code %d, want 2", name, code)
		}
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

type workProcessor struct {
	accept string
	calls  atomic.Int64
}

func (p *workProcessor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/synthetic/probe/work" {
		w.WriteHeader(404)
		return
	}
	p.calls.Add(1)
	if r.Header.Get("Authorization") != "Bearer "+p.accept {
		w.WriteHeader(401)
		return
	}
	_, _ = io.WriteString(w, `{"vantage_id":"v","assignments":[],"refresh_after_sec":60}`)
}

func TestServeFetchesWorkAndReportsHealth(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		cred := testCred(t)
		wp := &workProcessor{accept: cred}
		if !accepted {
			wp.accept = "someone-else"
		}
		srv := httptest.NewServer(wp)
		var errb syncBuffer
		d := testDeps(map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: srv.URL,
			allowPlaintextEnvVar: "true", healthAddrEnvVar: "127.0.0.1:0"}, nil, io.Discard, &errb)
		healthAddr := make(chan string, 1)
		d.listen = func(network, addr string) (net.Listener, error) {
			ln, err := net.Listen(network, addr)
			if err == nil {
				healthAddr <- ln.Addr().String()
			}
			return ln, err
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan int, 1)
		go func() { done <- run(ctx, []string{"serve"}, d) }()
		addr := <-healthAddr
		deadline := time.Now().Add(5 * time.Second)
		for wp.calls.Load() == 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		get := func(path string) int {
			resp, err := http.Get("http://" + addr + path)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		wantReady := 503
		if accepted {
			wantReady = 200
		}
		var ready int
		for time.Now().Before(deadline) {
			if ready = get("/readyz"); ready == wantReady {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if wp.calls.Load() == 0 || get("/healthz") != 200 || ready != wantReady {
			t.Fatalf("accepted=%v: work calls %d readyz %d", accepted, wp.calls.Load(), ready)
		}
		cancel()
		if code := <-done; code != 0 {
			t.Fatalf("serve exit %d", code)
		}
		srv.Close()
		if strings.Contains(errb.String(), strings.TrimPrefix(cred, probetoken.TokenPrefix)) {
			t.Fatal("the credential was logged")
		}
		if !strings.Contains(errb.String(), `"msg":"synthetic probe serving"`) {
			t.Fatalf("no structured start log: %s", errb.String())
		}
	}
}

func TestServeConfigurationErrors(t *testing.T) {
	cred := testCred(t)
	base := map[string]string{"OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL": cred, processorURLEnvVar: "https://processor.example:8443",
		healthAddrEnvVar: "off"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range base {
			m[a] = b
		}
		if v == "" {
			delete(m, k)
		} else {
			m[k] = v
		}
		return m
	}
	cases := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{"no credential", []string{"serve"}, with("OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL", "")},
		{"no processor url", []string{"serve"}, with(processorURLEnvVar, "")},
		{"plaintext without opt-in", []string{"serve"}, with(processorURLEnvVar, "http://processor:8443")},
		{"plaintext opt-in not exactly true", []string{"serve"}, func() map[string]string {
			m := with(processorURLEnvVar, "http://processor:8443")
			m[allowPlaintextEnvVar] = "1"
			return m
		}()},
		{"max in flight zero", []string{"serve", "-max-in-flight", "0"}, base},
		{"max in flight too large", []string{"serve"}, with(maxInFlightEnvVar, "65")},
		{"max in flight not a number", []string{"serve"}, with(maxInFlightEnvVar, "many")},
		{"missing CA file", []string{"serve"}, with(processorCAEnvVar, "/nope")},
		{"extra argument", []string{"serve", "more"}, base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := run(context.Background(), tc.args, testDeps(tc.env, nil, &out, &errb)); code != 2 {
				t.Fatalf("code %d, want 2", code)
			}
			if strings.Contains(out.String()+errb.String(), strings.TrimPrefix(cred, probetoken.TokenPrefix)) {
				t.Fatal("the credential was printed")
			}
		})
	}
}
