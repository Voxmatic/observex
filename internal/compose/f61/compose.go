// Package f61 composes the five frozen F6.1 packages into one in-process path:
// an observation is taken by a prober the caller configured, adapted and
// evaluated against the caller's horizon, and admitted to the caller's
// correlation set.
//
// It exists to show that the pieces fit, and for no other reason. It adds no
// behaviour of its own: every decision along the path is made by the package
// that owns it — the observer decides what the endpoint served, the adapter
// decides which certificates are eligible, the detector decides whether a
// certificate is inside the horizon, and the correlation set decides what may
// be admitted. This package only carries values between them and reports what
// happened.
//
// # What this is NOT
//
// This is not external-vantage execution. It runs wherever its caller runs,
// with whatever prober its caller built, and it establishes no availability
// independence from the target. It creates no second vantage: one call
// produces at most one member, and a caller wanting several must call it
// several times with several probers. Nothing here may be described as
// external or multi-vantage.
//
// It also chooses nothing about deployment: it names no service, no scheduler,
// no location, no region and no destination. It does not persist, expose,
// emit, alert, remediate or decide. It introduces no severity, no confidence,
// no policy and no execution state.
//
// # Vantage
//
// The vantage on a reported member is exactly Observation.Vantage, as the
// prober set it. This package never reads it, never rewrites it and never
// substitutes anything for it. Input.UnavailableVantage exists only because a
// prober that fails returns no observation, and therefore no vantage, so an
// unavailable member would otherwise have nothing to be attributed to; it is
// never applied to an observation that exists.
//
// # Time
//
// No clock is read here. The evaluation time is the caller's and is passed
// straight through to the adapter, which passes it to the detector. The probe
// budget is the caller's. The correlation span is the set's, and was the
// caller's when the set was made.
package f61

import (
	"context"
	"errors"
	"time"

	"github.com/observex/platform/internal/adapt/tlscertexpiry"
	correlate "github.com/observex/platform/internal/correlate/f61"
	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// Errors returned when a composition cannot be attempted at all. Each one
// refuses; none substitutes a default. Errors raised by the packages being
// composed are returned as they are, never reclassified.
var (
	// ErrNoSet: no correlation set was supplied. The set carries the subject,
	// so without it there is nothing to observe on behalf of.
	ErrNoSet = errors.New("compose/f61: no correlation set")
	// ErrNoProber: no prober was supplied. This package does not build one:
	// how an endpoint is reached, and from where, is the caller's choice.
	ErrNoProber = errors.New("compose/f61: no prober")
	// ErrNoTimeout: the probe budget was not supplied, or was not positive.
	ErrNoTimeout = errors.New("compose/f61: probe timeout must be greater than zero")
	// ErrNoEvaluationTime: the evaluation time was not supplied. It is
	// required even when the probe fails, because a caller that cannot say
	// when it is evaluating is not ready to evaluate.
	ErrNoEvaluationTime = errors.New("compose/f61: no evaluation time")
)

// Input is everything one composition step needs. All of it is the caller's:
// this package supplies no default for any field.
type Input struct {
	// Prober takes the observation. It is used exactly once per call, and its
	// configuration — dialler, clock, roots and vantage — belongs entirely to
	// the caller.
	Prober tlscert.Prober

	// Timeout bounds the probe: dial plus handshake. It is passed to the
	// observer unchanged.
	Timeout time.Duration

	// Params is the caller's horizon and freshness bound, passed to the
	// adapter unchanged and validated by the detector, not here. A zero
	// Params is not refused by this package: the detector already refuses a
	// non-positive horizon per certificate, and pre-empting it here would
	// move that decision out of the package that owns it.
	Params certexpiry.Params

	// EvaluatedAt is the caller's evaluation time, passed to the adapter and
	// recorded on the member. No clock is read here.
	EvaluatedAt time.Time

	// UnavailableVantage attributes a member whose probe produced no
	// observation. It must be the vantage the Prober was configured with.
	// It is used ONLY on that path: it never replaces, overrides or fills in
	// Observation.Vantage, which is preserved exactly as the observer set it.
	// It may be left zero; the correlation set records an unavailable member
	// either way, and reports the missing identity through its own accessor.
	UnavailableVantage tlscert.Vantage
}

// Step reports what the composition did, so that outcomes which are not
// failures of admission stay distinguishable from one another.
//
// The errors here are transient values handed straight back to the caller for
// inspection with errors.Is; nothing in this package stores them.
type Step struct {
	// Status is the status of the member that was admitted, or the empty
	// string when nothing was admitted.
	Status correlate.MemberStatus

	// ProbeErr is the prober's error, when the prober could not produce an
	// observation. It is the reason recorded on the unavailable member.
	ProbeErr error

	// AdaptErr is the adapter's error, when the observation was sound but
	// carried nothing the adapter could evaluate — an environmental outcome,
	// or a certificate-free handshake. The observation is still admitted; it
	// simply has no result. This is not a detector error: per-certificate
	// detector errors live on the result, where the adapter puts them.
	AdaptErr error
}

// Observe runs one composition step and admits at most one member to set.
//
// The path is: the set's subject supplies the check and endpoint, the prober
// observes them, the adapter evaluates each presented certificate against the
// caller's parameters, and the set admits the result beside the observation.
//
// Three outcomes, none of which invents anything:
//
//   - The prober returns an error. No observation exists, so none is
//     manufactured: an unavailable member is recorded with the prober's error
//     as its cause, and no finding is produced.
//   - The prober observes, but the adapter cannot use the observation — a
//     transport failure, a timeout, a malformed peer, no certificate. The
//     observation is admitted as a reported member with no result, and the
//     adapter's error is returned on the Step.
//   - The prober observes and the adapter evaluates. The observation and the
//     result are admitted together with the caller's evaluation time.
//
// The returned error is an admission failure from the correlation set — an
// unidentified vantage, a subject mismatch, an observation outside the set's
// span — returned unchanged, so it can be matched against that package's
// sentinels. An admission failure adds nothing to the set.
func Observe(ctx context.Context, set *correlate.Set, in Input) (Step, error) {
	if set == nil {
		return Step{}, ErrNoSet
	}
	if in.Prober == nil {
		return Step{}, ErrNoProber
	}
	if in.Timeout <= 0 {
		return Step{}, ErrNoTimeout
	}
	if in.EvaluatedAt.IsZero() {
		return Step{}, ErrNoEvaluationTime
	}

	obs, probeErr := in.Prober.Probe(ctx, targetFor(set.Subject(), in.Timeout))
	if probeErr != nil {
		// No observation was produced. The zero Observation the prober
		// returned alongside the error is discarded, not recorded.
		if err := set.AddUnavailable(in.UnavailableVantage, probeErr); err != nil {
			return Step{ProbeErr: probeErr}, err
		}
		return Step{Status: correlate.StatusUnavailable, ProbeErr: probeErr}, nil
	}

	res, adaptErr := tlscertexpiry.Adapt(in.EvaluatedAt, obs, in.Params)
	if adaptErr != nil {
		// The observation is evidence about the endpoint whether or not a
		// certificate came with it, so it is admitted with no result and no
		// evaluation time — nothing was evaluated.
		if err := set.AddReported(obs, tlscertexpiry.Result{}, time.Time{}); err != nil {
			return Step{AdaptErr: adaptErr}, err
		}
		return Step{Status: correlate.StatusReported, AdaptErr: adaptErr}, nil
	}

	if err := set.AddReported(obs, res, in.EvaluatedAt); err != nil {
		return Step{}, err
	}
	return Step{Status: correlate.StatusReported}, nil
}

// targetFor builds the probe target from the subject the set is keyed by, so
// that what is observed is what the set is about. The check id and endpoint
// are copied unchanged; nothing else in the subject reaches the prober, which
// has no business knowing whose endpoint it is.
func targetFor(s wire.Subject, timeout time.Duration) tlscert.Target {
	return tlscert.Target{CheckID: s.CheckID, Endpoint: s.Endpoint, Timeout: timeout}
}
