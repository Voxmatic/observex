// Package f61 is the authorization boundary for F6.1 synthetic probe intake.
//
// It makes one decision, in the processor (F6.1-INTAKE-1(a)): may the probe
// credential presented on this request report on this synthetic check, and if
// so, under which subject and from which vantage?
//
// # Order of checks
//
//  1. The credential is taken from the Authorization header value only, as
//     "Bearer <credential>". No other header is consulted: not
//     X-ObserveX-Token, and never X-ObserveX-Org. Authorize takes the header
//     value and a check ID and nothing else, so a caller-supplied organization
//     or vantage cannot be passed in.
//  2. The credential is verified by internal/probetoken: signature, typ, the
//     dedicated scope and nothing else (F6.1-INTAKE-1(b)), org_id, a vantage_id
//     minted and bound by the server, and lifetime. No check row is read for a
//     caller that fails this step.
//  3. The server-side synthetic_checks row is looked up by ID.
//  4. The subject is built from that row alone (wire/f61, F6.1-TEN-1); a row
//     whose tenancy cannot be resolved admits nothing (F6.1-TEN-2).
//  5. The credential's org_id must equal the row's org_id, byte for byte
//     (F6.1-INTAKE-1(c)).
//  6. The row must be an enabled "ssl" check, the only kind F6.1 observes.
//  7. The vantage is {Kind: VantageKind, ID: the credential's vantage_id}. It
//     is never read from a request body, a header or the probe's environment.
//
// # Errors
//
// Every refusal is one class, which a route maps with StatusCode and Reason:
// ErrUnauthenticated (401), ErrInvalidRequest (400), ErrForbidden (403),
// ErrConflict (409), ErrPayloadTooLarge (413) and ErrUnavailable (503);
// ErrConflict and ErrPayloadTooLarge arise only in Intake.Accept. A missing
// check and a check owned by another organization are both ErrForbidden, so
// the response does not reveal whether a check ID exists in another tenant;
// the detail sentinel (ErrCheckNotFound, ErrOrgMismatch, ...) is joined for
// logging and counting only. No error ever contains any part of the
// presented credential.
//
// # Payload
//
// Intake (intake.go) adds the payload to the decision: after Authorize, it
// decodes a tlscert wire document bound to the authoritative check ID, the
// row's target and the credential's vantage, and hands the result to a
// Receiver. The processor mounts it (services/processor/f61_intake.go).
//
// # What this package does not do
//
// It does not build a correlation set (F6.1-SPAN-1), evaluate
// (F6.1-HORIZON-1), persist or emit (F6.1-DEST-1), schedule or deliver work
// (F6.1-FANOUT-1), or implement replay defence. It keeps no vantage registry
// and holds no state.
package f61

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/observex/platform/internal/observe/tlscert"
	"github.com/observex/platform/internal/probetoken"
	wire "github.com/observex/platform/internal/wire/f61"
)

// VantageKind is the tlscert.Vantage.Kind of every vantage admitted here. It
// is a constant of the credential type, not something a probe or operator
// supplies. It says only that the observation came from a dedicated synthetic
// probe Deployment; it does not say the probe is external.
const VantageKind = "synthetic-probe"

// CheckTypeSSL is the synthetic_checks.type this boundary admits.
const CheckTypeSSL = "ssl"

// MaxCheckIDLength bounds a requested check ID.
const MaxCheckIDLength = 128

// Error classes. Every refusal wraps exactly one of them.
var (
	ErrUnauthenticated = errors.New("intake/f61: caller is not authenticated")
	ErrInvalidRequest  = errors.New("intake/f61: request is invalid")
	ErrForbidden       = errors.New("intake/f61: caller may not report on this check")
	ErrUnavailable     = errors.New("intake/f61: authorization could not be completed")
)

// Detail sentinels, joined to a class for logging and counting.
var (
	ErrNoCredential      = errors.New("intake/f61: no bearer credential in the Authorization header")
	ErrBadCheckID        = errors.New("intake/f61: check id is missing or invalid")
	ErrCheckNotFound     = errors.New("intake/f61: check not found")
	ErrLookupMismatch    = errors.New("intake/f61: lookup returned a different check")
	ErrTenancyUnresolved = errors.New("intake/f61: check tenancy cannot be resolved")
	ErrOrgMismatch       = errors.New("intake/f61: credential organization does not own the check")
	ErrNotF61Check       = errors.New("intake/f61: check is not an ssl check")
	ErrCheckDisabled     = errors.New("intake/f61: check is disabled")
	ErrNotAssigned       = errors.New("intake/f61: check is not assigned to this vantage (LOC-1)")
	ErrRevoked           = errors.New("intake/f61: credential was issued before the vantage's revocation bound")
)

// Verifier verifies a presented probe credential. probetoken.Key implements
// it; the interface exists so the boundary can be tested with no key.
type Verifier interface {
	Verify(presented string, now time.Time) (probetoken.Principal, error)
}

// CheckRecord is the server-side synthetic_checks row as the lookup read it.
type CheckRecord struct {
	// Row carries id, org_id, namespace and target.
	Row wire.CheckRow
	// Type is synthetic_checks.type.
	Type string
	// Enabled is synthetic_checks.enabled.
	Enabled bool
	// Locations is synthetic_checks.locations (LOC-1 selection).
	Locations []string
	// IntervalSec and TimeoutSec are the check's schedule.
	IntervalSec int
	TimeoutSec  int
}

// RevocationChecker reports the revocation bound of a vantage of an
// organization (PROPOSED REVOKE-1): a credential issued before it is refused.
type RevocationChecker interface {
	RevokedBefore(ctx context.Context, orgID, vantageID string) (time.Time, bool, error)
}

// CheckLookup reads one synthetic_checks row by ID. It returns an error
// wrapping ErrCheckNotFound when no row exists, and any other error when the
// row could not be read.
type CheckLookup interface {
	LookupCheck(ctx context.Context, checkID string) (CheckRecord, error)
}

// Admission is what an authorized request may report under.
type Admission struct {
	// Subject is built from the check row alone.
	Subject wire.Subject
	// Vantage is {VantageKind, credential vantage_id}.
	Vantage tlscert.Vantage
	// Declared is the operator declaration signed into the credential at
	// issuance. It is descriptive and unverified.
	Declared probetoken.Declaration
	// CredentialExpiresAt is when the presented credential stops being valid.
	CredentialExpiresAt time.Time
	// IntervalSec and TimeoutSec are the check's schedule, from the row.
	IntervalSec int
	TimeoutSec  int
}

// Authorizer makes the intake authorization decision.
type Authorizer struct {
	verifier    Verifier
	checks      CheckLookup
	revocations RevocationChecker
	now         func() time.Time
}

// New returns an Authorizer. It refuses a nil verifier, a verifier that
// reports it has no key, a nil lookup, a nil revocation checker or a nil
// clock.
func New(verifier Verifier, checks CheckLookup, revocations RevocationChecker, now func() time.Time) (*Authorizer, error) {
	if verifier == nil || checks == nil || revocations == nil || now == nil {
		return nil, errors.New("intake/f61: verifier, check lookup, revocation checker and clock are required")
	}
	if c, ok := verifier.(interface{ Configured() bool }); ok && !c.Configured() {
		return nil, fmt.Errorf("intake/f61: %w", probetoken.ErrKeyNotConfigured)
	}
	return &Authorizer{verifier: verifier, checks: checks, revocations: revocations, now: now}, nil
}

// Authenticate verifies the credential in authorization and applies the
// vantage's revocation bound. It is the first step of both Authorize and
// Work.
func (a *Authorizer) Authenticate(ctx context.Context, authorization string) (probetoken.Principal, error) {
	cred, ok := bearer(authorization)
	if !ok {
		return probetoken.Principal{}, refuse(ErrUnauthenticated, ErrNoCredential)
	}
	principal, err := a.verifier.Verify(cred, a.now())
	if err != nil {
		if errors.Is(err, probetoken.ErrKeyNotConfigured) || errors.Is(err, probetoken.ErrNoClock) {
			return probetoken.Principal{}, refuse(ErrUnavailable, err)
		}
		return probetoken.Principal{}, refuse(ErrUnauthenticated, err)
	}
	bound, revoked, err := a.revocations.RevokedBefore(ctx, principal.OrgID, principal.VantageID)
	if err != nil {
		// Fail closed: a revocation that cannot be read is not assumed absent.
		return probetoken.Principal{}, refuse(ErrUnavailable, err)
	}
	if revoked && principal.IssuedAt.Before(bound) {
		return probetoken.Principal{}, refuse(ErrUnauthenticated, ErrRevoked)
	}
	return principal, nil
}

// Authorize decides whether the credential in authorization (the raw
// Authorization header value) may report on checkID. On any refusal it
// returns the zero Admission and an error wrapping one class.
func (a *Authorizer) Authorize(ctx context.Context, authorization, checkID string) (Admission, error) {
	principal, err := a.Authenticate(ctx, authorization)
	if err != nil {
		return Admission{}, err
	}

	if !validCheckID(checkID) {
		return Admission{}, refuse(ErrInvalidRequest, ErrBadCheckID)
	}
	record, err := a.checks.LookupCheck(ctx, checkID)
	if err != nil {
		if errors.Is(err, ErrCheckNotFound) {
			return Admission{}, refuse(ErrForbidden, ErrCheckNotFound)
		}
		return Admission{}, refuse(ErrUnavailable, err)
	}
	if record.Row.ID != checkID {
		return Admission{}, refuse(ErrUnavailable, ErrLookupMismatch)
	}

	subject, err := wire.SubjectFromCheck(record.Row)
	if err != nil {
		return Admission{}, refuse(ErrForbidden, errors.Join(ErrTenancyUnresolved, err))
	}
	if principal.OrgID != subject.OrgID {
		return Admission{}, refuse(ErrForbidden, ErrOrgMismatch)
	}
	if record.Type != CheckTypeSSL {
		return Admission{}, refuse(ErrForbidden, ErrNotF61Check)
	}
	if !record.Enabled {
		return Admission{}, refuse(ErrForbidden, ErrCheckDisabled)
	}
	// LOC-1: a vantage may report only on checks it would be given as work.
	if !Assigned(record.Locations, principal.Declared.NetworkZone) {
		return Admission{}, refuse(ErrForbidden, ErrNotAssigned)
	}

	return Admission{
		Subject:             subject,
		Vantage:             tlscert.Vantage{Kind: VantageKind, ID: principal.VantageID},
		Declared:            principal.Declared,
		CredentialExpiresAt: principal.ExpiresAt,
		IntervalSec:         record.IntervalSec,
		TimeoutSec:          record.TimeoutSec,
	}, nil
}

// Location vocabulary (PROPOSED LOC-1).
const (
	// LocationAll in synthetic_checks.locations assigns the check to every
	// vantage of the organization.
	LocationAll = "*"
	// LocationLocal keeps its existing meaning — the processor's in-process
	// runner — and never matches a probe vantage.
	LocationLocal = "local"
)

// ReservedZone reports whether zone may not be declared as a vantage's
// network_zone, because it has a fixed meaning in synthetic_checks.locations.
func ReservedZone(zone string) bool { return zone == LocationAll || zone == LocationLocal }

// Assigned is the LOC-1 rule, shared by work delivery and intake: a check is
// assigned to a vantage iff its locations contain LocationAll, or contain the
// vantage's declared network_zone exactly. A vantage with no zone, or with a
// reserved one, matches only LocationAll.
func Assigned(locations []string, zone string) bool {
	for _, l := range locations {
		if l == LocationAll {
			return true
		}
		if zone != "" && !ReservedZone(zone) && l == zone {
			return true
		}
	}
	return false
}

// HTTP status codes for the four classes. Literal values keep net/http out
// of this package.
const (
	statusBadRequest          = 400
	statusUnauthorized        = 401
	statusForbidden           = 403
	statusConflict            = 409
	statusPayloadTooLarge     = 413
	statusInternalServerError = 500
	statusServiceUnavailable  = 503
)

// StatusCode maps an Authorize error to the HTTP status a route returns.
// Anything that is not one of the classes is 500.
func StatusCode(err error) int {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return statusUnauthorized
	case errors.Is(err, ErrInvalidRequest):
		return statusBadRequest
	case errors.Is(err, ErrForbidden):
		return statusForbidden
	case errors.Is(err, ErrConflict):
		return statusConflict
	case errors.Is(err, ErrPayloadTooLarge):
		return statusPayloadTooLarge
	case errors.Is(err, ErrUnavailable):
		return statusServiceUnavailable
	default:
		return statusInternalServerError
	}
}

func refuse(class, detail error) error {
	return fmt.Errorf("%w: %w", class, detail)
}

// bearer accepts exactly "Bearer <credential>": a case-insensitive scheme,
// one space, and a credential with no whitespace.
func bearer(h string) (string, bool) {
	const scheme = "bearer "
	if len(h) <= len(scheme) || !strings.EqualFold(h[:len(scheme)], scheme) {
		return "", false
	}
	cred := h[len(scheme):]
	if strings.IndexFunc(cred, unicode.IsSpace) >= 0 {
		return "", false
	}
	return cred, true
}

func validCheckID(id string) bool {
	if id == "" || len(id) > MaxCheckIDLength {
		return false
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
