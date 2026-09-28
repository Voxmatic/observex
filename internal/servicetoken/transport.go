package servicetoken

import (
	"net/http"
	"net/url"
	"strings"
)

// Transport is an http.RoundTripper that adds the internal token header only
// to requests for one internal service origin. The scheme, host and port must
// all match the target URL; requests to any other origin (for example Slack,
// PagerDuty, identity providers or other internal services) are sent
// unchanged. The caller's request is never modified: the header is set on a
// clone.
type Transport struct {
	base   http.RoundTripper
	token  Token
	origin string
}

// NewTransport returns a Transport that sends token to targetURL's origin.
// base may be nil, in which case http.DefaultTransport is used. If targetURL
// has no scheme or host, or token is not configured, the header is never
// added.
func NewTransport(base http.RoundTripper, token Token, targetURL string) *Transport {
	t := &Transport{base: base, token: token}
	if u, err := url.Parse(strings.TrimSpace(targetURL)); err == nil {
		t.origin = originOf(u)
	}
	return t
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	if t.origin == "" || !t.token.Configured() || req.URL == nil || originOf(req.URL) != t.origin {
		return base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	t.token.SetHeader(clone.Header)
	return base.RoundTrip(clone)
}

// originOf returns "scheme://host:port" in lower case, with the default port
// filled in for http and https. It returns "" when scheme or host is missing.
func originOf(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if scheme == "" || host == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + host + ":" + port
}
