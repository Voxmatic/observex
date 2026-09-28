// Package certexpiry implements the F6.1 detection rule from the approved
// ObserveX SRE knowledge catalog: "notAfter minus now, per certificate,
// measured on the live production endpoint"
// (F6.1 leading_indicators[0], method_fit deterministic-rule,
// prediction_class P1-deterministic).
//
// # What this package does
//
// One pure function, Evaluate, subtracts an observed certificate notAfter from
// a caller-supplied evaluation time and reports whether the remaining time is
// within a caller-supplied horizon. That is the whole rule. The arithmetic is
// exact, which is why the catalog classifies this pattern as deterministic.
//
// # What this package must not do, and does not do
//
//   - No I/O of any kind: no network, no TLS dialling, no files, no database,
//     no Kubernetes, no logging.
//   - No clock reads: the evaluation time is a parameter, so the same inputs
//     always produce the same output.
//   - No configuration or environment reads.
//   - No global mutable state.
//   - No action, remediation, automation class, severity, priority or
//     confidence score. F6.1 records an automation class for its remediations;
//     that is a property of a remediation and a matter for the policy layer,
//     never for a detector, and this package authorizes nothing.
//
// # No default horizon
//
// The F6.1 record contains no numeric threshold. Its escalation trigger reads
// "Any production certificate within the defined expiry horizon" — a horizon
// defined elsewhere. Params.Horizon is therefore required: Evaluate refuses to
// run without it rather than inventing one. In particular the existing
// processor's "daysLeft < 7" rule is a product choice with no catalog basis and
// is deliberately not inherited here.
//
// # Provenance
//
// The constants below bind this package to the approved catalog. They are
// compile-time values: the catalog is never parsed or interpreted at runtime by
// this package. knowledge_test.go asserts that each constant still matches the
// embedded catalog, so a catalog change that invalidates this rule fails the
// tests rather than silently changing behaviour.
package certexpiry

import (
	"errors"
	"time"

	"github.com/observex/platform/internal/knowledge"
)

// Provenance of the rule implemented here, from the approved catalog
// (KB-SEED-2). Verified against the embedded catalog by knowledge_test.go.
const (
	// PatternID is the catalog record this detector implements.
	PatternID = "F6.1"
	// PatternVersion is that record's version field.
	PatternVersion = "0.1.0"
	// PredictionClass is the class of the leading indicator implemented here
	// (F6.1 leading_indicators[0]).
	PredictionClass = "P1-deterministic"
	// IndicatorSignal is the exact indicator text this detector implements.
	IndicatorSignal = "notAfter minus now, per certificate, measured on the live production endpoint"
	// CatalogSHA256 is the approved catalog the provenance above came from.
	CatalogSHA256 = knowledge.CatalogSHA256
)

// Errors returned for input that cannot be evaluated. Each condition has its
// own error so callers and tests can tell them apart. An error means "no
// statement could be made"; it never means "no finding".
var (
	ErrHorizonNotSet       = errors.New("certexpiry: horizon must be greater than zero")
	ErrMaxAgeNotSet        = errors.New("certexpiry: maximum observation age must be greater than zero")
	ErrNoEvaluationTime    = errors.New("certexpiry: evaluation time is zero")
	ErrNoEndpoint          = errors.New("certexpiry: observation has no endpoint")
	ErrNoNotAfter          = errors.New("certexpiry: observation has no notAfter")
	ErrNoObservedAt        = errors.New("certexpiry: observation has no observation time")
	ErrObservationInFuture = errors.New("certexpiry: observation is newer than the evaluation time")
	ErrObservationStale    = errors.New("certexpiry: observation is older than the maximum observation age")
)

// Observation is one measurement taken at a live endpoint: the leaf
// certificate's notAfter, and when the measurement was taken. This package does
// not produce observations; a caller supplies them.
type Observation struct {
	// CheckID identifies the probe that took the measurement, for example a
	// synthetic_checks row id.
	CheckID string
	// Endpoint is the target as configured, for example "api.example.com:443".
	Endpoint string
	// NotAfter is the leaf certificate's notAfter, read from a completed
	// handshake. Go's x509 parser yields this as an instant in UTC.
	NotAfter time.Time
	// ObservedAt is when that handshake completed.
	ObservedAt time.Time
}

// Params are supplied by the caller. The F6.1 record defines neither value, so
// neither has a default here and both must be greater than zero.
type Params struct {
	// Horizon is "the defined expiry horizon" of the catalog's escalation
	// trigger. A certificate whose remaining time is at or inside this horizon
	// produces a finding.
	Horizon time.Duration
	// MaxObservationAge is how old an observation may be and still describe the
	// endpoint. The catalog requires measurement at the serving endpoint; it
	// does not say how fresh, so the bound is the caller's.
	MaxObservationAge time.Duration
}

// Finding states that an observed certificate is at or inside the horizon.
// It carries the measurement, the arithmetic and the catalog provenance —
// nothing else. Every time field is normalised to UTC with any monotonic
// reading stripped, so two findings for the same instants are identical
// whatever zone or clock the inputs used.
type Finding struct {
	// Provenance, copied from the constants above; never computed.
	PatternID       string
	PatternVersion  string
	CatalogSHA256   string
	PredictionClass string

	// Subject of the measurement.
	CheckID  string
	Endpoint string

	// The measurement and the arithmetic.
	NotAfter       time.Time     // observed expiry
	ObservedAt     time.Time     // when it was measured
	EvaluatedAt    time.Time     // the evaluation time passed to Evaluate
	Remaining      time.Duration // NotAfter - EvaluatedAt; negative once expired
	ObservationAge time.Duration // EvaluatedAt - ObservedAt
	Horizon        time.Duration // the horizon this finding crossed
}

// Evaluate applies F6.1 to one observation at time now.
//
//	found == false, err == nil → no finding: the certificate is beyond the horizon
//	found == true,  err == nil → finding, with the evidence above
//	err != nil                 → the input cannot be evaluated; found is false
//	                             and the Finding is the zero value
//
// A finding is produced when the remaining time is at or inside the horizon,
// that is when NotAfter.Sub(now) <= Horizon. The boundary is inclusive, and an
// already-expired certificate (negative remaining time) is inside every
// horizon. Evaluate reads no clock, performs no I/O and mutates nothing.
func Evaluate(now time.Time, obs Observation, p Params) (Finding, bool, error) {
	// Parameters first: a missing horizon is a caller error, not a property of
	// the observation, and must never be defaulted.
	switch {
	case p.Horizon <= 0:
		return Finding{}, false, ErrHorizonNotSet
	case p.MaxObservationAge <= 0:
		return Finding{}, false, ErrMaxAgeNotSet
	case now.IsZero():
		// Not specified by the contract. Refusing is the smallest safe choice:
		// a zero evaluation time would silently place every certificate far
		// beyond any horizon.
		return Finding{}, false, ErrNoEvaluationTime
	case obs.Endpoint == "":
		return Finding{}, false, ErrNoEndpoint
	case obs.NotAfter.IsZero():
		return Finding{}, false, ErrNoNotAfter
	case obs.ObservedAt.IsZero():
		return Finding{}, false, ErrNoObservedAt
	}

	age := now.Sub(obs.ObservedAt)
	switch {
	case age < 0:
		return Finding{}, false, ErrObservationInFuture
	case age > p.MaxObservationAge:
		return Finding{}, false, ErrObservationStale
	}

	remaining := obs.NotAfter.Sub(now)
	if remaining > p.Horizon {
		return Finding{}, false, nil
	}

	return Finding{
		PatternID:       PatternID,
		PatternVersion:  PatternVersion,
		CatalogSHA256:   CatalogSHA256,
		PredictionClass: PredictionClass,

		CheckID:  obs.CheckID,
		Endpoint: obs.Endpoint,

		NotAfter:       obs.NotAfter.UTC(),
		ObservedAt:     obs.ObservedAt.UTC(),
		EvaluatedAt:    now.UTC(),
		Remaining:      remaining,
		ObservationAge: age,
		Horizon:        p.Horizon,
	}, true, nil
}
