// Package tlscert observes the TLS certificates a live endpoint presents.
//
// It exists to supply the input of the F6.1 detector (internal/detect/certexpiry),
// whose rule is "notAfter minus now, per certificate, measured on the live
// production endpoint". This package measures; it decides nothing.
//
// # Observation is not trust
//
// The handshake used here sets InsecureSkipVerify so that the peer's chain can
// be read even when that chain is expired, untrusted or issued for another
// name — the cases the F6.1 pattern is about, and exactly the cases a verifying
// handshake aborts before exposing any certificate. Skipping verification in the
// handshake does NOT mean the certificate is accepted:
//
//   - the connection carries no application data, ever: the handshake completes,
//     ConnectionState is read, and the connection is closed;
//   - verification is then performed explicitly, in code, with
//     x509.Certificate.Verify and x509.Certificate.VerifyHostname, and its
//     outcome is recorded in the observation as data;
//   - nothing here changes TLS behaviour anywhere else in the product, and this
//     package exposes no way to weaken verification: Config carries no such
//     switch, and it reads no environment variable.
//
// A caller that wants to send traffic to an endpoint must do its own verified
// handshake. An Observation is evidence about what an endpoint served at a
// moment in time, nothing more.
//
// # Chain
//
// Every certificate the endpoint actually presents is observed, in wire order:
// leaf at position 0, then intermediates. Roots are not observed, because
// servers do not present them. The chain is never collapsed to its earliest
// expiry and intermediates are never dropped: the detector works per
// certificate, so this package reports per certificate.
//
// # Vantage
//
// An Observation records where it was taken. There is no external probe tier in
// ObserveX today, so a caller running inside the cluster must say so. This
// package will not pretend an in-cluster process is an external vantage point,
// and F6.1's requirement to verify "from outside… not from a single probe" is
// therefore NOT satisfied by an in-cluster caller.
//
// # Out of scope
//
// No severity, confidence, policy, remediation, action, incident, persistence,
// scheduling or knowledge lookup. No first-party imports. Standard library only.
package tlscert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Errors returned for a caller mistake. Environmental failures are never
// errors: they are typed Outcomes on the Observation.
var (
	ErrNoEndpoint = errors.New("tlscert: target has no endpoint")
	ErrNoTimeout  = errors.New("tlscert: target timeout must be greater than zero")
	ErrBadAddress = errors.New("tlscert: endpoint is not a valid host or host:port")
)

// DefaultPort is appended when the endpoint carries no port.
const DefaultPort = "443"

// Outcome classifies what happened, so a caller never has to parse an error
// string and a transport failure is never mistaken for a missing certificate.
type Outcome string

const (
	// Observed: the chain was read and explicit verification passed.
	Observed Outcome = "observed"
	// ObservedUntrusted: the chain was read and explicit verification failed.
	// The certificates are still valid observations; their notAfter is a fact.
	ObservedUntrusted Outcome = "observed_untrusted"
	// NoCertificates: the handshake completed but the peer presented none.
	NoCertificates Outcome = "no_certificates"
	// TransportFailure: connection refused, reset, DNS failure, no route.
	TransportFailure Outcome = "transport_failure"
	// Timeout: the probe budget was exhausted.
	Timeout Outcome = "timeout"
	// Malformed: the peer did not speak TLS, or the handshake was unusable.
	Malformed Outcome = "malformed"
)

// Reasons recorded on a failed verification.
const (
	ReasonExpired          = "expired"
	ReasonNotYetValid      = "not yet valid"
	ReasonUnknownAuthority = "unknown authority or incomplete chain"
	ReasonHostnameMismatch = "hostname mismatch"
	ReasonUnusableChain    = "chain could not be verified"
)

// Target is what to probe. Identity comes from the caller.
type Target struct {
	// CheckID identifies the probe configuration, for example a
	// synthetic_checks row id. It is carried through to the observation.
	CheckID string
	// Endpoint is "host" or "host:port"; DefaultPort is assumed when absent.
	Endpoint string
	// Timeout bounds the whole probe: dial plus handshake.
	Timeout time.Duration
}

// Vantage says where a probe ran. It is descriptive, never aspirational.
type Vantage struct {
	// Kind is the honest description of the location, for example
	// "in-cluster-processor". F6.1 asks for an external vantage point;
	// no value of this field makes an in-cluster caller external.
	Kind string
	// ID identifies the specific process or deployment.
	ID string
}

// TrustStatus is the result of verification performed explicitly after the
// chain was read. It is evidence, not a gate.
type TrustStatus struct {
	// ChainVerified reports x509.Certificate.Verify against the configured
	// roots, at the observation time.
	ChainVerified bool
	// HostnameVerified reports x509.Certificate.VerifyHostname for the
	// requested host.
	HostnameVerified bool
	// Reason is empty when both checks passed, otherwise a short description.
	Reason string
}

// Verified reports whether both checks passed.
func (t TrustStatus) Verified() bool { return t.ChainVerified && t.HostnameVerified }

// Certificate is one certificate exactly as the endpoint presented it. Its
// mutable state (DER bytes, SAN list) is unexported and handed out only as
// copies, so an observation cannot be altered through a returned value.
type Certificate struct {
	// Position is the index in the presented chain: 0 is the leaf.
	Position int
	// IsLeaf is Position == 0.
	IsLeaf bool
	// IsCA is the certificate's basic-constraints CA flag.
	IsCA bool
	// Subject and Issuer are the RFC 2253 distinguished names.
	Subject string
	Issuer  string
	// SerialNumber is the serial in base 10.
	SerialNumber string
	// NotBefore and NotAfter are the validity bounds, in UTC.
	NotBefore time.Time
	NotAfter  time.Time

	der      []byte
	dnsNames []string
	ips      []string
}

// DER returns a copy of the certificate's DER encoding.
func (c Certificate) DER() []byte {
	if len(c.der) == 0 {
		return nil
	}
	out := make([]byte, len(c.der))
	copy(out, c.der)
	return out
}

// DNSNames returns a copy of the certificate's DNS subject alternative names.
func (c Certificate) DNSNames() []string { return cloneStrings(c.dnsNames) }

// IPAddresses returns a copy of the certificate's IP subject alternative names.
func (c Certificate) IPAddresses() []string { return cloneStrings(c.ips) }

// Observation is one probe of one endpoint at one moment, from one vantage.
type Observation struct {
	CheckID    string
	Endpoint   string
	ObservedAt time.Time     // when the handshake completed, UTC
	Duration   time.Duration // dial plus handshake
	Outcome    Outcome
	Trust      TrustStatus
	Vantage    Vantage
	// Detail is a short, bounded description for humans. Peer-supplied text is
	// never copied into it verbatim.
	Detail string

	certificates []Certificate
}

// Certificates returns a copy of the presented chain, in wire order. It is
// empty unless Outcome is Observed or ObservedUntrusted.
func (o Observation) Certificates() []Certificate {
	if o.certificates == nil {
		return nil
	}
	out := make([]Certificate, len(o.certificates))
	copy(out, o.certificates)
	return out
}

// Leaf returns the leaf certificate, if one was observed.
func (o Observation) Leaf() (Certificate, bool) {
	if len(o.certificates) == 0 {
		return Certificate{}, false
	}
	return o.certificates[0], true
}

// DialFunc matches net.Dialer.DialContext. Injecting it keeps tests offline.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Config configures a Prober. It deliberately has no field that can weaken or
// skip verification: verification always runs, and only its result varies.
type Config struct {
	// Dial connects to the endpoint. Defaults to a net.Dialer.
	Dial DialFunc
	// Now supplies the observation time and the verification time. Defaults to
	// time.Now. Injecting it makes verification reproducible.
	Now func() time.Time
	// Roots are the trust anchors used by explicit verification. Nil means the
	// host's system roots. Supplying roots narrows what is trusted; it cannot
	// disable the check.
	Roots *x509.CertPool
	// Vantage describes where this prober runs; see the package doc.
	Vantage Vantage
}

// Prober observes the certificates an endpoint presents.
type Prober interface {
	Probe(ctx context.Context, t Target) (Observation, error)
}

type prober struct {
	dial    DialFunc
	now     func() time.Time
	roots   *x509.CertPool
	vantage Vantage
}

// New returns a Prober. The zero Config is usable: it dials with a net.Dialer,
// reads the real clock and verifies against the system roots.
func New(cfg Config) Prober {
	p := &prober{dial: cfg.Dial, now: cfg.Now, roots: cfg.Roots, vantage: cfg.Vantage}
	if p.dial == nil {
		p.dial = (&net.Dialer{}).DialContext
	}
	if p.now == nil {
		p.now = time.Now
	}
	return p
}

// Probe performs one observation. Environmental failures are reported as
// Outcomes with a nil error; a non-nil error means the Target was unusable.
func (p *prober) Probe(ctx context.Context, t Target) (Observation, error) {
	if t.Endpoint == "" {
		return Observation{}, ErrNoEndpoint
	}
	if t.Timeout <= 0 {
		return Observation{}, ErrNoTimeout
	}
	host, addr, err := splitEndpoint(t.Endpoint)
	if err != nil {
		return Observation{}, err
	}

	obs := Observation{CheckID: t.CheckID, Endpoint: t.Endpoint, Vantage: p.vantage}
	start := p.now()

	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	raw, err := p.dial(ctx, "tcp", addr)
	if err != nil {
		return p.failed(obs, start, err, "dial"), nil
	}

	// The connection exists only to complete a handshake and read the peer's
	// certificates. No application data is written to it, and it is closed
	// here whatever happens next.
	conn := tls.Client(raw, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		// Observation only — see the package doc. Verification is performed
		// explicitly below and recorded as TrustStatus; this connection never
		// carries application data and is closed immediately.
		InsecureSkipVerify: true, // #nosec G402
	})
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}
	if err := conn.HandshakeContext(ctx); err != nil {
		return p.failed(obs, start, err, "handshake"), nil
	}

	certs := conn.ConnectionState().PeerCertificates
	obs.ObservedAt = p.now().UTC()
	obs.Duration = obs.ObservedAt.Sub(start.UTC())

	if len(certs) == 0 {
		obs.Outcome = NoCertificates
		obs.Detail = "handshake completed but the peer presented no certificate"
		return obs, nil
	}

	obs.certificates = describe(certs)
	obs.Trust = p.verify(certs, host, obs.ObservedAt)
	if obs.Trust.Verified() {
		obs.Outcome = Observed
		obs.Detail = fmt.Sprintf("observed %d certificate(s); verification passed", len(certs))
	} else {
		obs.Outcome = ObservedUntrusted
		obs.Detail = fmt.Sprintf("observed %d certificate(s); verification failed: %s", len(certs), obs.Trust.Reason)
	}
	return obs, nil
}

// verify performs the checks the handshake deliberately skipped. Its result is
// recorded, never acted on.
func (p *prober) verify(certs []*x509.Certificate, host string, at time.Time) TrustStatus {
	leaf := certs[0]

	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	st := TrustStatus{}
	_, chainErr := leaf.Verify(x509.VerifyOptions{
		Roots:         p.roots, // nil = system roots
		Intermediates: intermediates,
		CurrentTime:   at,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	st.ChainVerified = chainErr == nil
	hostErr := leaf.VerifyHostname(host)
	st.HostnameVerified = hostErr == nil

	var reasons []string
	if chainErr != nil {
		reasons = append(reasons, chainReason(chainErr, leaf, at))
	}
	if hostErr != nil {
		reasons = append(reasons, ReasonHostnameMismatch)
	}
	st.Reason = strings.Join(reasons, " and ")
	return st
}

// chainReason classifies a verification failure. The validity window is decided
// from the certificate's own dates rather than from error text, so the
// classification does not depend on any message format.
func chainReason(err error, leaf *x509.Certificate, at time.Time) string {
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
		if at.Before(leaf.NotBefore) {
			return ReasonNotYetValid
		}
		return ReasonExpired
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return ReasonUnknownAuthority
	}
	var hostErr x509.HostnameError
	if errors.As(err, &hostErr) {
		return ReasonHostnameMismatch
	}
	return ReasonUnusableChain
}

// describe converts parsed certificates into immutable observations, in wire
// order. Nothing is filtered, reordered or summarised.
func describe(certs []*x509.Certificate) []Certificate {
	out := make([]Certificate, 0, len(certs))
	for i, c := range certs {
		ips := make([]string, 0, len(c.IPAddresses))
		for _, ip := range c.IPAddresses {
			ips = append(ips, ip.String())
		}
		der := make([]byte, len(c.Raw))
		copy(der, c.Raw)
		out = append(out, Certificate{
			Position:     i,
			IsLeaf:       i == 0,
			IsCA:         c.IsCA,
			Subject:      c.Subject.String(),
			Issuer:       c.Issuer.String(),
			SerialNumber: c.SerialNumber.String(),
			NotBefore:    c.NotBefore.UTC(),
			NotAfter:     c.NotAfter.UTC(),
			der:          der,
			dnsNames:     cloneStrings(c.DNSNames),
			ips:          ips,
		})
	}
	return out
}

// failed classifies an environmental failure into a typed Outcome.
func (p *prober) failed(obs Observation, start time.Time, err error, stage string) Observation {
	obs.ObservedAt = p.now().UTC()
	obs.Duration = obs.ObservedAt.Sub(start.UTC())
	obs.Outcome = classify(err, stage)
	obs.Detail = string(obs.Outcome) + " during " + stage
	return obs
}

func classify(err error, stage string) Outcome {
	if err == nil {
		return TransportFailure
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Timeout
	}
	// os.ErrDeadlineExceeded and every net timeout satisfy net.Error.Timeout.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return Timeout
	}
	var recErr tls.RecordHeaderError
	if errors.As(err, &recErr) {
		return Malformed
	}
	if stage == "handshake" {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || isConnReset(err) {
			return TransportFailure
		}
		return Malformed
	}
	return TransportFailure
}

func isConnReset(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && strings.Contains(strings.ToLower(opErr.Error()), "reset")
}

// splitEndpoint returns the host for verification and the dial address.
func splitEndpoint(endpoint string) (host, addr string, err error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", "", ErrNoEndpoint
	}
	// A host or host:port only: no scheme, no path, no credentials, no spaces.
	if strings.ContainsAny(endpoint, "/@ \t\r\n") {
		return "", "", ErrBadAddress
	}
	if h, port, splitErr := net.SplitHostPort(endpoint); splitErr == nil {
		if h == "" || port == "" || !isNumeric(port) {
			return "", "", ErrBadAddress
		}
		return h, endpoint, nil
	}
	return endpoint, net.JoinHostPort(endpoint, DefaultPort), nil
}

func isNumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
