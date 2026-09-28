package f61

import (
	"context"
	"errors"
	"time"

	"github.com/observex/platform/internal/observe/tlscert"
)

// Replay window (PROPOSED REPLAY-1): an observation is accepted only if it was
// taken at most MaxReportDelay before receipt and at most ClockSkew after it.
// Together with the receiver's strictly-newer rule per (check, vantage), a
// captured payload can neither overwrite newer state nor be replayed later.
const (
	MaxReportDelay = 10 * time.Minute
	ClockSkew      = 30 * time.Second
)

// ErrObservationTime: ObservedAt is outside the replay window.
var ErrObservationTime = errors.New("intake/f61: observation time is outside the accepted window")

// Additional error classes used by Accept.
var (
	// ErrPayloadTooLarge (413): the body exceeds tlscert.MaxWireBytes or the
	// chain limits. Checked before anything else, so no work is spent on it.
	ErrPayloadTooLarge = errors.New("intake/f61: observation payload is too large")
	// ErrConflict (409): the observation is for a different check or endpoint
	// than the authoritative row the caller was authorized for.
	ErrConflict = errors.New("intake/f61: observation does not match the check")
)

// Receiver takes an admitted observation. It is called only after the caller
// was authenticated and authorized and the payload decoded and validated, and
// it receives the Subject and Vantage established server-side, never anything
// taken from the payload.
type Receiver interface {
	Receive(ctx context.Context, adm Admission, obs tlscert.Observation, receivedAt time.Time) (Receipt, error)
}

// Receipt is what the receiver did with an accepted observation.
type Receipt struct {
	// Stored is false when the observation was not newer than the one already
	// held for this vantage: acknowledged, nothing changed (idempotent).
	Stored bool `json:"stored"`
	// Evaluated is true when a verdict was computed.
	Evaluated bool `json:"evaluated"`
	// Status is the check's status after this observation, when evaluated.
	Status string `json:"result_status,omitempty"`
}

// Intake is the complete processor-side decision for one reported
// observation: size, authentication, authorization, decode, hand-off.
type Intake struct {
	authz *Authorizer
	recv  Receiver
	now   func() time.Time
}

// NewIntake returns an Intake. Both arguments are required.
func NewIntake(authz *Authorizer, recv Receiver) (*Intake, error) {
	if authz == nil || recv == nil {
		return nil, errors.New("intake/f61: authorizer and receiver are required")
	}
	return &Intake{authz: authz, recv: recv, now: authz.now}, nil
}

// Accept handles one request: authorization is the raw Authorization header
// value, checkID the check named by the request path, body the wire payload.
//
// Order: the size limit; then Authorize (so nothing is parsed for a caller
// that is not authenticated and authorized for this check); then
// tlscert.UnmarshalObservation bound to the authoritative check ID, the row's
// target and the credential's vantage; then the Receiver. On any refusal the
// Receiver is not called and the error wraps exactly one class.
func (in *Intake) Accept(ctx context.Context, authorization, checkID string, body []byte) (Admission, Receipt, error) {
	if len(body) > tlscert.MaxWireBytes {
		return Admission{}, Receipt{}, refuse(ErrPayloadTooLarge, tlscert.ErrWireTooLarge)
	}
	adm, err := in.authz.Authorize(ctx, authorization, checkID)
	if err != nil {
		return Admission{}, Receipt{}, err
	}
	obs, err := tlscert.UnmarshalObservation(body, tlscert.Binding{
		CheckID:  adm.Subject.CheckID,
		Endpoint: adm.Subject.Endpoint,
		Vantage:  adm.Vantage,
	})
	if err != nil {
		switch {
		case errors.Is(err, tlscert.ErrWireTooLarge):
			return Admission{}, Receipt{}, refuse(ErrPayloadTooLarge, err)
		case errors.Is(err, tlscert.ErrWireBinding):
			return Admission{}, Receipt{}, refuse(ErrConflict, err)
		default:
			return Admission{}, Receipt{}, refuse(ErrInvalidRequest, err)
		}
	}
	receivedAt := in.now().UTC()
	if obs.ObservedAt.Before(receivedAt.Add(-MaxReportDelay)) || obs.ObservedAt.After(receivedAt.Add(ClockSkew)) {
		return Admission{}, Receipt{}, refuse(ErrInvalidRequest, ErrObservationTime)
	}
	receipt, err := in.recv.Receive(ctx, adm, obs, receivedAt)
	if err != nil {
		return Admission{}, Receipt{}, refuse(ErrUnavailable, err)
	}
	return adm, receipt, nil
}

// Reason is a stable, machine-readable code for a refusal, safe to return to
// the caller. A missing check and another organization's check share
// "forbidden"; nothing distinguishes them.
func Reason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrPayloadTooLarge):
		return "payload_too_large"
	case errors.Is(err, ErrConflict):
		return "observation_check_mismatch"
	case errors.Is(err, ErrInvalidRequest):
		switch {
		case errors.Is(err, ErrBadCheckID):
			return "invalid_check_id"
		case errors.Is(err, ErrObservationTime):
			return "observation_time_out_of_range"
		case errors.Is(err, tlscert.ErrWireVersion):
			return "unsupported_wire_version"
		case errors.Is(err, tlscert.ErrWireMalformed):
			return "malformed_observation"
		case errors.Is(err, tlscert.ErrWireInvalid):
			return "inconsistent_observation"
		default:
			return "invalid_request"
		}
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "internal_error"
	}
}
