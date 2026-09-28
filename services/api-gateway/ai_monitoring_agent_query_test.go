package main

// GO-8 tests for the AI agent's error-rate query URL. orgID must be quoted as
// a PromQL string literal and the whole expression URL-encoded, so no orgID
// can add label matchers, add query parameters or change the request target.

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	go8MetricURL   = "http://query-engine:8080"
	go8QueryPrefix = `sum by (service) (rate(http_requests_total{org=`
	go8QuerySuffix = `,status=~"5.."}[5m]))`
)

// go8EncodedQuery matches a raw query string made only of a single "query"
// parameter whose value is fully URL-encoded (no raw &, #, ?, quotes, braces,
// spaces or control characters).
var go8EncodedQuery = regexp.MustCompile(`^query=(?:[A-Za-z0-9_.~+-]|%[0-9A-F]{2})*$`)

var go8SpecialOrgIDs = []struct {
	name  string
	orgID string
}{
	{"DoubleQuoteMatcherInjection", `x",env=~".*`},
	{"BackslashAndQuote", `a\"b\\`},
	{"URLDelimiters", `a&query=up#frag?x=1`},
	{"PercentPlusSpace", `100% +org id%2F`},
	{"PromQLSyntax", `org"}[5m])) or vector(1) #`},
	{"ControlCharacters", "line1\nline2\t\x00end"},
	{"Unicode", "組織-é"},
	{"Empty", ""},
}

// go8DecodedQuery checks the request target and parameter structure of raw and
// returns the decoded PromQL expression.
func go8DecodedQuery(t *testing.T, raw string) string {
	t.Helper()
	if !strings.HasPrefix(raw, go8MetricURL+"/api/v1/query?query=") {
		t.Fatalf("URL does not start with %s/api/v1/query?query=", go8MetricURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	if u.Scheme != "http" || u.Host != "query-engine:8080" || u.Path != "/api/v1/query" || u.User != nil || u.Fragment != "" {
		t.Fatalf("target = scheme %q host %q path %q fragment %q, want http://query-engine:8080/api/v1/query", u.Scheme, u.Host, u.Path, u.Fragment)
	}
	if !go8EncodedQuery.MatchString(u.RawQuery) {
		t.Fatalf("raw query is not a single fully encoded query parameter: %q", u.RawQuery)
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		t.Fatalf("url.ParseQuery: %v", err)
	}
	if len(values) != 1 || len(values["query"]) != 1 {
		t.Fatalf("query parameters = %d keys (%d query values), want exactly one query parameter", len(values), len(values["query"]))
	}
	return values.Get("query")
}

func TestAgentErrorRateQueryURL_NormalOrgID(t *testing.T) {
	got := agentErrorRateQueryURL(go8MetricURL, "org-a")

	// Same URL the original URL-encoded template intended to build for org-a.
	const wantURL = "http://query-engine:8080/api/v1/query?query=sum+by+%28service%29+%28rate%28http_requests_total%7Borg%3D%22org-a%22%2Cstatus%3D~%225..%22%7D%5B5m%5D%29%29"
	if got != wantURL {
		t.Fatalf("URL = %q, want %q", got, wantURL)
	}
	const wantQuery = `sum by (service) (rate(http_requests_total{org="org-a",status=~"5.."}[5m]))`
	if q := go8DecodedQuery(t, got); q != wantQuery {
		t.Fatalf("decoded query = %q, want %q", q, wantQuery)
	}
}

func TestAgentErrorRateQueryURL_SpecialCharacterOrgIDs(t *testing.T) {
	for _, tc := range go8SpecialOrgIDs {
		t.Run(tc.name, func(t *testing.T) {
			q := go8DecodedQuery(t, agentErrorRateQueryURL(go8MetricURL, tc.orgID))
			if !strings.HasPrefix(q, go8QueryPrefix) || !strings.HasSuffix(q, go8QuerySuffix) || len(q) < len(go8QueryPrefix)+len(go8QuerySuffix) {
				t.Fatalf("decoded query %q does not keep the expected prefix and suffix", q)
			}
			literal := q[len(go8QueryPrefix) : len(q)-len(go8QuerySuffix)]
			// The org value must be exactly one escaped string literal ...
			if want := strconv.Quote(tc.orgID); literal != want {
				t.Fatalf("org literal = %s, want %s", literal, want)
			}
			// ... that decodes back to the original orgID (no extra matchers).
			unquoted, err := strconv.Unquote(literal)
			if err != nil {
				t.Fatalf("org literal %s is not a single string literal: %v", literal, err)
			}
			if unquoted != tc.orgID {
				t.Fatalf("org literal decodes to %q, want %q", unquoted, tc.orgID)
			}
		})
	}
}

func TestAgentErrorRateQueryURL_QueryStructure(t *testing.T) {
	orgIDs := []string{"org-a", "6f1c2a9e-3b7d-4c1e-9a52-0d8e7f6b5a43"}
	for _, tc := range go8SpecialOrgIDs {
		orgIDs = append(orgIDs, tc.orgID)
	}
	for _, orgID := range orgIDs {
		q := go8DecodedQuery(t, agentErrorRateQueryURL(go8MetricURL, orgID))
		want := go8QueryPrefix + strconv.Quote(orgID) + go8QuerySuffix
		if q != want {
			t.Errorf("decoded query = %q, want %q", q, want)
		}
	}
}
