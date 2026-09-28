// Package f61 is the probe side of the F6.1 probe-to-processor path (G-1: a
// dedicated synthetic probe service, one Deployment per vantage).
//
// It does three things: it loads the vantage's probe credential, it reports
// one tlscert observation to the processor's intake route, and it combines
// the two with a single probe in ProbeAndReport.
//
// # What the probe does not decide
//
// The probe asserts nothing about who it is. It sends its credential, which
// the processor verifies, and an observation whose check ID and endpoint the
// processor compares with its own authoritative row; the vantage, organization,
// namespace and subject are all established by the processor. The observation
// payload carries no vantage at all.
//
// # What this package does not do
//
// It has no schedule, no loop and no list of checks. Which checks a vantage
// probes, and how often, is work delivery: F6.1-FANOUT-1 (and F6.1-LOC-1 if
// synthetic_checks.locations selects vantages), both undecided. It does not
// evaluate, correlate or keep anything.
//
// # Transport
//
// The credential is a bearer token. The client therefore:
//
//   - requires https unless AllowPlaintext is set explicitly; with plaintext
//     the credential crosses the network in clear, and whether that is
//     acceptable for a given deployment is a deployment decision (F6.1-EGRESS-1
//     and the processor's exposure), not something this package assumes;
//   - never follows a redirect, so the credential is sent to exactly the
//     configured origin;
//   - never puts the credential in a URL, an error or a log.
package f61

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
)

const (
	// CredentialEnvVar holds the credential value.
	CredentialEnvVar = "OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL"
	// CredentialFileEnvVar holds the path to a file containing the credential
	// (for example a mounted Secret). When set it always wins, with no
	// fallback to CredentialEnvVar.
	CredentialFileEnvVar = "OBSERVEX_SYNTHETIC_PROBE_CREDENTIAL_FILE"

	// DefaultRequestTimeout bounds one report when the caller supplies no
	// HTTP client.
	DefaultRequestTimeout = 15 * time.Second

	userAgent        = "observex-synthetic-probe/f61"
	maxResponseBytes = 64 << 10
)

// Errors. None contains the credential or the payload.
var (
	ErrNoCredential       = errors.New("probe/f61: no probe credential configured")
	ErrCredentialFile     = errors.New("probe/f61: probe credential file could not be read")
	ErrBadProcessorURL    = errors.New("probe/f61: processor URL is not a usable https (or explicitly allowed http) origin")
	ErrObservation        = errors.New("probe/f61: observation cannot be encoded")
	ErrTransport          = errors.New("probe/f61: report could not be delivered")
	ErrCredentialRejected = errors.New("probe/f61: processor rejected the credential")
	ErrForbidden          = errors.New("probe/f61: processor refused this check for this credential")
	ErrRejected           = errors.New("probe/f61: processor rejected the observation")
	ErrIntakeUnavailable  = errors.New("probe/f61: processor intake unavailable")
	ErrUnexpectedResponse = errors.New("probe/f61: unexpected response from processor")
)

// LoadCredential reads the probe credential from the process environment.
func LoadCredential() (probetoken.Credential, error) {
	return LoadCredentialFrom(os.LookupEnv, os.ReadFile)
}

// LoadCredentialFrom reads the credential using the given environment lookup
// and file reader. CredentialFileEnvVar wins when set to a non-blank path.
func LoadCredentialFrom(lookupEnv func(string) (string, bool), readFile func(string) ([]byte, error)) (probetoken.Credential, error) {
	if path, ok := lookupEnv(CredentialFileEnvVar); ok && strings.TrimSpace(path) != "" {
		data, err := readFile(strings.TrimSpace(path))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return probetoken.Credential{}, fmt.Errorf("%w: file does not exist", ErrCredentialFile)
			}
			return probetoken.Credential{}, ErrCredentialFile
		}
		return probetoken.ParseCredential(string(data))
	}
	if v, ok := lookupEnv(CredentialEnvVar); ok && strings.TrimSpace(v) != "" {
		return probetoken.ParseCredential(v)
	}
	return probetoken.Credential{}, ErrNoCredential
}

// ClientConfig configures a Client.
type ClientConfig struct {
	// ProcessorURL is the processor origin, optionally with a path prefix,
	// for example "https://processor.example:8443".
	ProcessorURL string
	// Credential is this vantage's probe credential.
	Credential probetoken.Credential
	// AllowPlaintext permits an http:// ProcessorURL. Off by default.
	AllowPlaintext bool
	// HTTPClient is optional. It is copied, never modified; redirects are
	// always disabled on the copy.
	HTTPClient *http.Client
	// RootCAs, when set and HTTPClient is nil, is the only trust anchor for
	// the processor's certificate (for example an internal CA). Nil means
	// the system roots.
	RootCAs *x509.CertPool
}

// Client reports observations to the processor and fetches work.
type Client struct {
	base *url.URL
	mu   sync.RWMutex
	cred probetoken.Credential
	http *http.Client
}

// SetCredential replaces the credential, for example after the vantage's
// Secret was rotated.
func (c *Client) SetCredential(cred probetoken.Credential) {
	if cred.Reveal() == "" {
		return
	}
	c.mu.Lock()
	c.cred = cred
	c.mu.Unlock()
}

func (c *Client) bearer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return "Bearer " + c.cred.Reveal()
}

// NewClient validates cfg and returns a Client.
func NewClient(cfg ClientConfig) (*Client, error) {
	if cfg.Credential.Reveal() == "" {
		return nil, ErrNoCredential
	}
	u, err := url.Parse(strings.TrimSpace(cfg.ProcessorURL))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrBadProcessorURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !cfg.AllowPlaintext {
			return nil, ErrBadProcessorURL
		}
	default:
		return nil, ErrBadProcessorURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""

	hc := &http.Client{Timeout: DefaultRequestTimeout}
	if cfg.RootCAs != nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: cfg.RootCAs}
		hc.Transport = tr
	}
	if cfg.HTTPClient != nil {
		copied := *cfg.HTTPClient
		hc = &copied
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: u, cred: cfg.Credential, http: hc}, nil
}

// Receipt is the processor's answer to one report.
type Receipt struct {
	// Status is the HTTP status.
	Status int
	// Accepted is true for 202: the report was received. It was either
	// stored, or ignored as not newer than one already held.
	Accepted bool
	// Stored is false for an acknowledged duplicate or out-of-order report.
	Stored bool
	// Evaluated is true when the processor recomputed the check's result.
	Evaluated bool
	// ResultStatus is the check's status after this report, when evaluated.
	ResultStatus string
	// Reason is the processor's refusal or ignore code, if any.
	Reason string
}

type intakeResponse struct {
	Status       string `json:"status"`
	Stored       bool   `json:"stored"`
	Evaluated    bool   `json:"evaluated"`
	ResultStatus string `json:"result_status"`
	Reason       string `json:"reason"`
}

// ObservationURL returns where an observation for checkID is reported.
func (c *Client) ObservationURL(checkID string) string {
	// base has no query or fragment (NewClient refuses them) and no trailing
	// slash, so the escaped segment can be appended as text.
	return c.base.String() + "/v1/synthetic/checks/" + url.PathEscape(checkID) + "/tls-observations"
}

// Report sends one observation. A nil error means the processor accepted it.
func (c *Client) Report(ctx context.Context, obs tlscert.Observation) (Receipt, error) {
	body, err := tlscert.MarshalObservation(obs)
	if err != nil {
		return Receipt{}, fmt.Errorf("%w: %w", ErrObservation, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ObservationURL(obs.CheckID), bytes.NewReader(body))
	if err != nil {
		return Receipt{}, ErrTransport
	}
	req.Header.Set("Authorization", c.bearer())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		// *url.Error names the method and URL; neither holds the credential.
		return Receipt{}, fmt.Errorf("%w: %w", ErrTransport, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))

	var parsed intakeResponse
	_ = json.Unmarshal(raw, &parsed)
	r := Receipt{Status: resp.StatusCode, Reason: safeReason(parsed.Reason)}
	switch resp.StatusCode {
	case http.StatusAccepted:
		r.Accepted, r.Stored, r.Evaluated, r.ResultStatus = true, parsed.Stored, parsed.Evaluated, safeReason(parsed.ResultStatus)
		return r, nil
	case http.StatusUnauthorized:
		return r, fmt.Errorf("%w (status %d, reason %q)", ErrCredentialRejected, r.Status, r.Reason)
	case http.StatusForbidden:
		return r, fmt.Errorf("%w (status %d, reason %q)", ErrForbidden, r.Status, r.Reason)
	case http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge:
		return r, fmt.Errorf("%w (status %d, reason %q)", ErrRejected, r.Status, r.Reason)
	case http.StatusServiceUnavailable:
		return r, fmt.Errorf("%w (status %d, reason %q)", ErrIntakeUnavailable, r.Status, r.Reason)
	default:
		return r, fmt.Errorf("%w (status %d)", ErrUnexpectedResponse, r.Status)
	}
}

// ProbeAndReport probes target once with prober and reports the result. If
// the target is unusable (Probe returns an error), nothing is sent.
func ProbeAndReport(ctx context.Context, prober tlscert.Prober, client *Client, target tlscert.Target) (tlscert.Observation, Receipt, error) {
	obs, err := prober.Probe(ctx, target)
	if err != nil {
		return tlscert.Observation{}, Receipt{}, err
	}
	receipt, err := client.Report(ctx, obs)
	return obs, receipt, err
}

// safeReason keeps only a short lower-case code from the response, so a
// hostile or broken server cannot inject text into the probe's output.
func safeReason(s string) string {
	if s == "" || len(s) > 64 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if !(s[i] >= 'a' && s[i] <= 'z' || s[i] == '_') {
			return ""
		}
	}
	return s
}
