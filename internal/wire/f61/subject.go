// Package f61 carries the tenant/scope identity of a synthetic check for the
// future F6.1 wiring layer.
//
// It exists to answer one question and no other: for a given synthetic check,
// which organization and namespace does it belong to, and what resource does it
// name? The answer is built from the check row alone.
//
// # Single source of identity
//
// Identity comes only from the server-side synthetic_checks row: org_id,
// namespace, id and target, read by the scheduler from the database. There is
// no second input. This package takes no caller-supplied organization, no
// request context, no headers, no labels, no authentication context, no
// environment and no process state, and it derives nothing from them. That is
// enforced by the shape of the API — SubjectFromCheck accepts a CheckRow and
// nothing else — not merely by convention.
//
// # Fail closed
//
// If any of the four row fields is empty or blank, no Subject is returned. The
// zero Subject is returned together with an error naming every missing field.
// There is no default organization, no default namespace, no placeholder
// endpoint and no partially populated Subject. A caller that cannot resolve
// tenancy is expected to do nothing, not to proceed with a guess.
//
// # What this package does not do
//
// It does not observe, adapt, detect, decide, persist, emit, schedule or call
// anything. It holds no state, reads no clock, opens no connection and imports
// no first-party package. It is not wired into runtime execution.
package f61

import (
	"errors"
	"strings"
)

// ServicePrefix is prepended to a check ID to form the ServiceID of a synthetic
// check's subject. It is the only value this package contributes that is not
// copied verbatim from the check row.
const ServicePrefix = "synthetic:"

// Errors reported when tenant/scope cannot be resolved from the check row.
// Every one of them means: produce no Subject.
var (
	// ErrMissingOrgID: synthetic_checks.org_id was empty or blank. It is never
	// defaulted and never taken from anywhere else.
	ErrMissingOrgID = errors.New("f61: check row has no org_id")
	// ErrMissingNamespace: synthetic_checks.namespace was empty or blank.
	ErrMissingNamespace = errors.New("f61: check row has no namespace")
	// ErrMissingCheckID: synthetic_checks.id was empty or blank.
	ErrMissingCheckID = errors.New("f61: check row has no id")
	// ErrMissingEndpoint: synthetic_checks.target was empty or blank.
	ErrMissingEndpoint = errors.New("f61: check row has no target")
)

// CheckRow is the server-side synthetic check row, and the only accepted source
// of subject identity. Each field is the column of the same meaning, read by
// the scheduler from synthetic_checks.
type CheckRow struct {
	// OrgID is synthetic_checks.org_id.
	OrgID string
	// Namespace is synthetic_checks.namespace.
	Namespace string
	// ID is synthetic_checks.id.
	ID string
	// Target is synthetic_checks.target.
	Target string
}

// Subject is the five-field identity envelope for one synthetic check.
//
// It carries identity and scope only. It deliberately carries no roles, no
// permissions, no authentication context, no vantage point, no certificate
// identity, no timestamps, no severity, no confidence, no remediation, no
// policy and no execution state. Those belong to other layers, and widening
// this envelope to hold them would make it something other than an identity.
type Subject struct {
	// OrgID is the owning organization, copied from the check row.
	OrgID string
	// Namespace is the scope within the organization, copied from the check row.
	Namespace string
	// ServiceID is ServicePrefix + CheckID.
	ServiceID string
	// CheckID is the synthetic check's identifier, copied from the check row.
	CheckID string
	// Endpoint is what the check targets, copied from the check row.
	Endpoint string
}

// SubjectFromCheck builds the subject envelope for one synthetic check.
//
// Every field is taken from row; ServiceID is ServicePrefix + row.ID. Values
// are copied byte for byte: nothing is trimmed, lower-cased, canonicalised or
// otherwise rewritten, because rewriting a tenant identifier is a way to make
// two distinct tenants collide.
//
// A blank field is treated as absent. On any absent field the zero Subject is
// returned with an error that reports all absent fields, so the caller sees the
// whole reason at once and never receives a partial subject.
func SubjectFromCheck(row CheckRow) (Subject, error) {
	var missing []error
	if blank(row.OrgID) {
		missing = append(missing, ErrMissingOrgID)
	}
	if blank(row.Namespace) {
		missing = append(missing, ErrMissingNamespace)
	}
	if blank(row.ID) {
		missing = append(missing, ErrMissingCheckID)
	}
	if blank(row.Target) {
		missing = append(missing, ErrMissingEndpoint)
	}
	if len(missing) > 0 {
		return Subject{}, errors.Join(missing...)
	}
	return Subject{
		OrgID:     row.OrgID,
		Namespace: row.Namespace,
		ServiceID: ServicePrefix + row.ID,
		CheckID:   row.ID,
		Endpoint:  row.Target,
	}, nil
}

// blank reports whether s carries no identifying content.
func blank(s string) bool {
	return strings.TrimSpace(s) == ""
}
