package f61

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
)

var testNow = time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

func testCredential(t *testing.T) probetoken.Credential {
	t.Helper()
	out, err := probetoken.NewKey(strings.Repeat("k", 40)).Issue(probetoken.IssueRequest{OrgID: "org-a"}, testNow, bytes.NewReader(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential
}

func goldenObservation(t *testing.T) tlscert.Observation {
	t.Helper()
	data, err := os.ReadFile("../../observe/tlscert/testdata/observation-v1-observed-untrusted.json")
	if err != nil {
		t.Fatal(err)
	}
	obs, err := tlscert.UnmarshalObservation(data, tlscert.Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: tlscert.Vantage{Kind: "local", ID: "local"}})
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

func TestLoadCredentialFrom(t *testing.T) {
	cred := testCredential(t).Reveal()
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	files := func(m map[string]string) func(string) ([]byte, error) {
		return func(p string) ([]byte, error) {
			if v, ok := m[p]; ok {
				return []byte(v), nil
			}
			return nil, fs.ErrNotExist
		}
	}
	c, err := LoadCredentialFrom(env(map[string]string{CredentialFileEnvVar: "/s", CredentialEnvVar: "ignored"}), files(map[string]string{"/s": cred + "\n"}))
	if err != nil || c.Reveal() != cred {
		t.Fatalf("file credential: %v", err)
	}
	c, err = LoadCredentialFrom(env(map[string]string{CredentialEnvVar: cred}), files(nil))
	if err != nil || c.Reveal() != cred {
		t.Fatalf("env credential: %v", err)
	}
	if _, err := LoadCredentialFrom(env(map[string]string{CredentialFileEnvVar: "/missing", CredentialEnvVar: cred}), files(nil)); !errors.Is(err, ErrCredentialFile) {
		t.Fatalf("missing file must not fall back: %v", err)
	}
	if _, err := LoadCredentialFrom(env(nil), files(nil)); !errors.Is(err, ErrNoCredential) {
		t.Fatalf("no credential: %v", err)
	}
	if _, err := LoadCredentialFrom(env(map[string]string{CredentialEnvVar: "oxat_agent.token.value"}), files(nil)); !errors.Is(err, probetoken.ErrCredentialFormat) {
		t.Fatalf("agent token accepted as a probe credential: %v", err)
	}
}

func TestNewClientRefusesUnsafeTargets(t *testing.T) {
	cred := testCredential(t)
	bad := []ClientConfig{
		{ProcessorURL: "https://processor.example"},                               // no credential
		{ProcessorURL: "http://processor:8080", Credential: cred},                 // plaintext not allowed
		{ProcessorURL: "ftp://processor", Credential: cred, AllowPlaintext: true}, // scheme
		{ProcessorURL: "processor:8080", Credential: cred, AllowPlaintext: true},  // no scheme
		{ProcessorURL: "https://user:pw@processor", Credential: cred},             // userinfo
		{ProcessorURL: "https://processor/?x=1", Credential: cred},                // query
		{ProcessorURL: "https://processor/#f", Credential: cred},                  // fragment
		{ProcessorURL: "", Credential: cred},
	}
	for i, cfg := range bad {
		if _, err := NewClient(cfg); err == nil {
			t.Errorf("case %d: unsafe config accepted", i)
		}
	}
	c, err := NewClient(ClientConfig{ProcessorURL: "https://processor.example/prefix/", Credential: cred})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ObservationURL("a/b"); got != "https://processor.example/prefix/v1/synthetic/checks/a%2Fb/tls-observations" {
		t.Fatalf("ObservationURL = %q", got)
	}
	if _, err := NewClient(ClientConfig{ProcessorURL: "http://processor:8080", Credential: cred, AllowPlaintext: true}); err != nil {
		t.Fatalf("explicit plaintext refused: %v", err)
	}
}

type fakeIntake struct {
	status int
	body   string
	hits   atomic.Int64
	auth   atomic.Value
	path   atomic.Value
	sent   atomic.Value
}

func (f *fakeIntake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.hits.Add(1)
	f.auth.Store(r.Header.Get("Authorization"))
	f.path.Store(r.URL.EscapedPath())
	b, _ := io.ReadAll(r.Body)
	f.sent.Store(b)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.status)
	_, _ = io.WriteString(w, f.body)
}

func TestReportSendsCredentialAndServerBoundPayload(t *testing.T) {
	cred := testCredential(t)
	fi := &fakeIntake{status: 202, body: `{"status":"accepted","evaluated":false}`}
	srv := httptest.NewServer(fi)
	defer srv.Close()
	c, err := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: cred, AllowPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	obs := goldenObservation(t)
	r, err := c.Report(context.Background(), obs)
	if err != nil || !r.Accepted || r.Evaluated || r.Status != 202 {
		t.Fatalf("report: %+v, %v", r, err)
	}
	if fi.auth.Load().(string) != "Bearer "+cred.Reveal() {
		t.Fatal("credential not sent as a bearer token")
	}
	if fi.path.Load().(string) != "/v1/synthetic/checks/chk-golden-1/tls-observations" {
		t.Fatalf("path %q", fi.path.Load())
	}
	sent := fi.sent.Load().([]byte)
	if bytes.Contains(sent, []byte("vantage")) || bytes.Contains(sent, []byte(cred.Reveal())) {
		t.Fatal("payload carries a vantage or the credential")
	}
	back, err := tlscert.UnmarshalObservation(sent, tlscert.Binding{CheckID: "chk-golden-1", Endpoint: "observex.test:443", Vantage: tlscert.Vantage{Kind: "k", ID: "i"}})
	if err != nil || len(back.Certificates()) != 1 {
		t.Fatalf("payload does not decode: %v", err)
	}
}

func TestReportMapsRefusals(t *testing.T) {
	cred := testCredential(t)
	cases := []struct {
		status int
		body   string
		want   error
		reason string
	}{
		{401, `{"reason":"unauthenticated"}`, ErrCredentialRejected, "unauthenticated"},
		{403, `{"reason":"forbidden"}`, ErrForbidden, "forbidden"},
		{400, `{"reason":"inconsistent_observation"}`, ErrRejected, "inconsistent_observation"},
		{409, `{"reason":"observation_check_mismatch"}`, ErrRejected, "observation_check_mismatch"},
		{413, `{"reason":"payload_too_large"}`, ErrRejected, "payload_too_large"},
		{503, `{"reason":"unavailable"}`, ErrIntakeUnavailable, "unavailable"},
		{500, `oops`, ErrUnexpectedResponse, ""},
		{200, `{"status":"accepted"}`, ErrUnexpectedResponse, ""},
		{403, `{"reason":"<script>alert(1)</script>"}`, ErrForbidden, ""},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status, tc.reason), func(t *testing.T) {
			srv := httptest.NewServer(&fakeIntake{status: tc.status, body: tc.body})
			defer srv.Close()
			c, _ := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: cred, AllowPlaintext: true})
			r, err := c.Report(context.Background(), goldenObservation(t))
			if !errors.Is(err, tc.want) || r.Accepted || r.Reason != tc.reason || r.Status != tc.status {
				t.Fatalf("got %+v, %v", r, err)
			}
			if strings.Contains(err.Error(), cred.Reveal()) {
				t.Fatal("error contains the credential")
			}
		})
	}
}

// The client never follows a redirect, so the credential goes only to the
// configured origin.
func TestReportDoesNotFollowRedirects(t *testing.T) {
	cred := testCredential(t)
	elsewhere := &fakeIntake{status: 202, body: `{"status":"accepted"}`}
	other := httptest.NewServer(elsewhere)
	defer other.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	c, _ := NewClient(ClientConfig{ProcessorURL: redirector.URL, Credential: cred, AllowPlaintext: true})
	if _, err := c.Report(context.Background(), goldenObservation(t)); !errors.Is(err, ErrUnexpectedResponse) {
		t.Fatalf("redirect: %v", err)
	}
	if elsewhere.hits.Load() != 0 {
		t.Fatal("the redirect target received the report")
	}
}

func TestReportTransportFailureDoesNotLeakCredential(t *testing.T) {
	cred := testCredential(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, _ := NewClient(ClientConfig{ProcessorURL: url, Credential: cred, AllowPlaintext: true})
	_, err := c.Report(context.Background(), goldenObservation(t))
	if !errors.Is(err, ErrTransport) || strings.Contains(err.Error(), cred.Reveal()) {
		t.Fatalf("got %v", err)
	}
}

func TestProbeAndReportSendsNothingForAnUnusableTarget(t *testing.T) {
	fi := &fakeIntake{status: 202, body: `{}`}
	srv := httptest.NewServer(fi)
	defer srv.Close()
	c, _ := NewClient(ClientConfig{ProcessorURL: srv.URL, Credential: testCredential(t), AllowPlaintext: true})
	_, _, err := ProbeAndReport(context.Background(), tlscert.New(tlscert.Config{}), c, tlscert.Target{CheckID: "c", Endpoint: "https://x/", Timeout: time.Second})
	if !errors.Is(err, tlscert.ErrBadAddress) || fi.hits.Load() != 0 {
		t.Fatalf("got %v, hits %d", err, fi.hits.Load())
	}
	// An unencodable observation (no check ID) is not sent either.
	if _, err := c.Report(context.Background(), tlscert.Observation{}); !errors.Is(err, ErrObservation) || fi.hits.Load() != 0 {
		t.Fatalf("got %v", err)
	}
}
