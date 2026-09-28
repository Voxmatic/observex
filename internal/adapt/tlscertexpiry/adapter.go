// Package tlscertexpiry adapts a TLS certificate observation
// (internal/observe/tlscert) into inputs for the F6.1 certificate-expiry
// detector (internal/detect/certexpiry), and calls that detector.
//
// It is the seam between two packages that must not know about each other:
// the observer records what an endpoint served and whether verification passed;
// the detector evaluates one certificate against a caller-supplied horizon.
// This package decides only which observed certificates are eligible inputs.
//
// # What it does not do
//
// No network access, no clock read (the evaluation time is a parameter), no
// re-verification of certificates, no persistence, no severity, no confidence,
// no policy, no remediation, no action. It never builds a Finding itself: every
// finding comes from certexpiry.Evaluate. It never invents a horizon or a
// freshness bound — both come from the caller's certexpiry.Params.
//
// # Eligibility, and the one boundary that is not settled
//
// The approved design (ObserveX-F6.1-Observation-Layer-Design-v0.1.0.md §4)
// authorizes these observed certificates as detector input: a valid certificate,
// one inside the horizon, one already expired, one whose issuer is untrusted,
// and one whose chain is incomplete. The notAfter of an expired or untrusted
// certificate is a fact about the endpoint, and F6.1 exists for exactly that
// case.
//
// The same table leaves one case OPEN: a certificate whose identity does not
// match the requested host ("the notAfter is real, but the certificate may not
// be the one that serves this name"). The F6.1 detector contract does not
// authorize it either. Rather than settle that question here, this adapter
// SKIPS such a certificate with SkipHostnameMismatch and preserves the
// observation unchanged, so the fact survives and the policy decision stays
// open for whoever owns it.
//
// Chain handling: every presented certificate becomes its own detector
// evaluation, in wire order. Nothing is collapsed to an earliest expiry, and
// intermediates are not dropped. Chain position and certificate identity live
// in this package's result, because the detector's Observation has no field for
// them and its contract is not widened to fit this adapter.
package tlscertexpiry

import (
	"errors"
	"fmt"
	"time"

	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
)

// Errors returned when an observation cannot yield detector input at all.
var (
	// ErrNotCertificateObservation is returned for an observation whose outcome
	// is environmental — transport failure, timeout, malformed peer, or no
	// certificate presented. Such an observation is never reinterpreted as a
	// certificate-expiry result.
	ErrNotCertificateObservation = errors.New("tlscertexpiry: observation did not observe a certificate")
	// ErrNoCertificates is returned when an otherwise successful observation
	// carries no certificates.
	ErrNoCertificates = errors.New("tlscertexpiry: observation carries no certificates")
)

// Status says what happened to one presented certificate.
type Status string

const (
	// StatusEvaluated: the certificate was passed to the detector. Found and
	// Finding carry the detector's answer.
	StatusEvaluated Status = "evaluated"
	// StatusSkipped: the certificate was not passed to the detector.
	// SkipReason says why.
	StatusSkipped Status = "skipped"
)

// Skip reasons. Each is a stated boundary, never a silent drop.
const (
	// SkipHostnameMismatch: the observed certificate failed hostname
	// verification. Whether such a certificate is eligible for F6.1 is an
	// unresolved question in the approved design (§4 case F); until it is
	// resolved this adapter does not feed it to the detector.
	SkipHostnameMismatch = "hostname verification failed; eligibility for F6.1 is unresolved in the approved design (observation design §4, case F)"
)

// CertificateResult is the outcome for one presented certificate. Position and
// identity are preserved here because the detector's Observation carries
// neither.
type CertificateResult struct {
	// Position in the presented chain; 0 is the leaf.
	Position int
	IsLeaf   bool
	// Identity of the certificate, copied from the observation.
	Subject      string
	Issuer       string
	SerialNumber string
	// NotAfter exactly as observed.
	NotAfter time.Time

	Status     Status
	SkipReason string

	// Finding and Found are the detector's answer; both are zero unless Status
	// is StatusEvaluated. Err is the detector's error for this certificate,
	// for example an unusable input or a missing horizon.
	Finding certexpiry.Finding
	Found   bool
	Err     error
}

// Result is the outcome for one observation: one entry per presented
// certificate, in wire order.
type Result struct {
	CheckID    string
	Endpoint   string
	ObservedAt time.Time
	Outcome    tlscert.Outcome
	Trust      tlscert.TrustStatus

	certificates []CertificateResult
}

// Certificates returns a copy of the per-certificate results, in wire order.
func (r Result) Certificates() []CertificateResult {
	if r.certificates == nil {
		return nil
	}
	out := make([]CertificateResult, len(r.certificates))
	copy(out, r.certificates)
	return out
}

// Findings returns a copy of the findings the detector produced, in wire order.
func (r Result) Findings() []certexpiry.Finding {
	var out []certexpiry.Finding
	for _, c := range r.certificates {
		if c.Status == StatusEvaluated && c.Found {
			out = append(out, c.Finding)
		}
	}
	return out
}

// Adapt converts an observation into detector inputs and evaluates each one.
//
// now is the caller's evaluation time and is passed straight through to the
// detector; this package never reads a clock. p is the caller's horizon and
// freshness bound; this package never supplies a default for either.
//
// An environmental observation yields ErrNotCertificateObservation and no
// detector call. A per-certificate problem — an unusable input, a missing
// horizon — is reported on that certificate's result, not as a finding.
func Adapt(now time.Time, obs tlscert.Observation, p certexpiry.Params) (Result, error) {
	switch obs.Outcome {
	case tlscert.Observed, tlscert.ObservedUntrusted:
	default:
		return Result{}, fmt.Errorf("%w: outcome %q", ErrNotCertificateObservation, obs.Outcome)
	}

	certs := obs.Certificates()
	if len(certs) == 0 {
		return Result{}, ErrNoCertificates
	}

	res := Result{
		CheckID:    obs.CheckID,
		Endpoint:   obs.Endpoint,
		ObservedAt: obs.ObservedAt,
		Outcome:    obs.Outcome,
		Trust:      obs.Trust,
	}
	for _, c := range certs {
		res.certificates = append(res.certificates, adaptCertificate(now, certInput{
			CheckID:          obs.CheckID,
			Endpoint:         obs.Endpoint,
			ObservedAt:       obs.ObservedAt,
			HostnameVerified: obs.Trust.HostnameVerified,
			Position:         c.Position,
			IsLeaf:           c.IsLeaf,
			Subject:          c.Subject,
			Issuer:           c.Issuer,
			SerialNumber:     c.SerialNumber,
			NotAfter:         c.NotAfter,
		}, p))
	}
	return res, nil
}

// certInput is one presented certificate plus the observation context the
// detector needs. It exists so the conversion can be exercised directly.
type certInput struct {
	CheckID          string
	Endpoint         string
	ObservedAt       time.Time
	HostnameVerified bool

	Position     int
	IsLeaf       bool
	Subject      string
	Issuer       string
	SerialNumber string
	NotAfter     time.Time
}

// adaptCertificate applies the eligibility boundary and, when eligible, calls
// the detector. It copies timestamps unchanged: no rounding, no normalisation,
// no re-verification.
func adaptCertificate(now time.Time, in certInput, p certexpiry.Params) CertificateResult {
	out := CertificateResult{
		Position:     in.Position,
		IsLeaf:       in.IsLeaf,
		Subject:      in.Subject,
		Issuer:       in.Issuer,
		SerialNumber: in.SerialNumber,
		NotAfter:     in.NotAfter,
	}

	if !in.HostnameVerified {
		out.Status = StatusSkipped
		out.SkipReason = SkipHostnameMismatch
		return out
	}

	out.Status = StatusEvaluated
	out.Finding, out.Found, out.Err = certexpiry.Evaluate(now, certexpiry.Observation{
		CheckID:    in.CheckID,
		Endpoint:   in.Endpoint,
		NotAfter:   in.NotAfter,
		ObservedAt: in.ObservedAt,
	}, p)
	if out.Err != nil {
		out.Finding = certexpiry.Finding{}
		out.Found = false
	}
	return out
}
