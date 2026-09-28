package servicetoken

import (
	"net/http"
	"strings"
	"testing"
)

type recordingRoundTripper struct {
	got []*http.Request
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = append(r.got, req)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
}

func configuredToken(t *testing.T) Token {
	t.Helper()
	tok := LoadFrom(fakeEnv{EnvVar: testTokenA}.lookup, fakeFiles{}.read)
	if !tok.Configured() {
		t.Fatal("test token not configured")
	}
	return tok
}

func TestTransport_AddsHeaderOnlyForTargetOrigin(t *testing.T) {
	const target = "http://query-engine:9090"
	tests := []struct {
		name       string
		url        string
		wantHeader bool
	}{
		{"SameOrigin", "http://query-engine:9090/api/v1/query?query=up", true},
		{"SameOriginUppercaseHost", "http://QUERY-ENGINE:9090/api/v1/query", true},
		{"SameOriginRootPath", "http://query-engine:9090", true},
		{"DifferentPort", "http://query-engine:9091/api/v1/query", false},
		{"DefaultPortDiffers", "http://query-engine/api/v1/query", false},
		{"DifferentScheme", "https://query-engine:9090/api/v1/query", false},
		{"DifferentHost", "http://ingestor:9090/internal/agents", false},
		{"IngestorService", "http://ingestor:4318/internal/agents", false},
		{"ExternalSlack", "https://hooks.slack.com/services/T000/B000/XXX", false},
		{"ExternalPagerDuty", "https://events.pagerduty.com/v2/enqueue", false},
		{"HostPrefixTrick", "http://query-engine.attacker.example:9090/", false},
		{"UserinfoTrick", "http://query-engine:9090@attacker.example:9090/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingRoundTripper{}
			tr := NewTransport(rec, configuredToken(t), target)
			req, err := http.NewRequest(http.MethodGet, tt.url, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if _, err := tr.RoundTrip(req); err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			if len(rec.got) != 1 {
				t.Fatalf("base transport called %d times, want 1", len(rec.got))
			}
			sent := rec.got[0].Header.Get(Header)
			if tt.wantHeader && sent == "" {
				t.Fatal("expected internal token header, none sent")
			}
			if !tt.wantHeader && sent != "" {
				t.Fatal("internal token header sent to a non-target origin")
			}
			if req.Header.Get(Header) != "" {
				t.Fatal("caller's request was modified")
			}
		})
	}
}

func TestTransport_DefaultPortsNormalised(t *testing.T) {
	rec := &recordingRoundTripper{}
	tr := NewTransport(rec, configuredToken(t), "https://qe.internal")
	req, _ := http.NewRequest(http.MethodGet, "https://qe.internal:443/x", nil)
	_, _ = tr.RoundTrip(req)
	if rec.got[0].Header.Get(Header) == "" {
		t.Fatal("explicit default port should match implicit default port")
	}
}

func TestTransport_NoHeaderWhenTokenNotConfigured(t *testing.T) {
	rec := &recordingRoundTripper{}
	tr := NewTransport(rec, Token{}, "http://query-engine:9090")
	req, _ := http.NewRequest(http.MethodGet, "http://query-engine:9090/api/v1/query", nil)
	_, _ = tr.RoundTrip(req)
	if rec.got[0].Header.Get(Header) != "" {
		t.Fatal("header sent although no token is configured")
	}
}

func TestTransport_NoHeaderWhenTargetInvalid(t *testing.T) {
	for _, target := range []string{"", "query-engine:9090", "::bad", "/relative/only"} {
		rec := &recordingRoundTripper{}
		tr := NewTransport(rec, configuredToken(t), target)
		req, _ := http.NewRequest(http.MethodGet, "http://query-engine:9090/api/v1/query", nil)
		_, _ = tr.RoundTrip(req)
		if rec.got[0].Header.Get(Header) != "" {
			t.Fatalf("header sent with invalid target %q", target)
		}
	}
}

func TestTransport_OverwritesSpoofedHeaderOnlyForTarget(t *testing.T) {
	rec := &recordingRoundTripper{}
	tr := NewTransport(rec, configuredToken(t), "http://query-engine:9090")

	req, _ := http.NewRequest(http.MethodGet, "http://query-engine:9090/q", nil)
	req.Header.Set(Header, "caller-supplied-value")
	_, _ = tr.RoundTrip(req)
	if !configuredToken(t).Matches(rec.got[0].Header.Get(Header)) {
		t.Fatal("target request should carry the configured token")
	}
	if strings.Contains(rec.got[0].Header.Get(Header), "caller-supplied") {
		t.Fatal("caller-supplied header value was forwarded")
	}
}
