// Package f61 correlates the per-vantage TLS observations of one synthetic
// check into an ordered set, without collapsing them.
//
// F6.1 asks for certificate expiry to be verified "from outside, on every
// region and endpoint, not from a single probe". The signal it is after is a
// disagreement: one vantage serving a renewed certificate while another still
// serves the old one. That signal only exists while the individual observations
// still exist, so this package's whole job is to hold them side by side.
//
// # What it does
//
// It keys a set by the approved subject envelope, appends one member per
// per-vantage report in the order the reports arrive, and hands back copies.
// That is all.
//
// # What it does not do
//
// It computes no agreement, no disagreement verdict, no quorum, no coverage,
// no "enough vantages", no earliest expiry and no summary of any kind. It does
// not observe, adapt, detect, persist, emit, schedule or authenticate. It reads
// no clock, no environment and no network, and it holds no package-level state.
// It takes no position on where a correlated set eventually goes.
//
// # Tenancy
//
// Tenancy enters only as a [wire.Subject], which the caller obtained from
// wire.SubjectFromCheck and which is therefore already fail-closed on the
// check row's org and namespace. This package offers no other way in: it never
// accepts an organization or a namespace as a separate argument, never derives
// one from an observation, a vantage or a result, and never constructs a
// Subject itself. Because the Subject is the key and every member is stamped
// with it, one set cannot hold two tenants' observations.
//
// A vantage is a location label, never a tenant. It takes no part in the key
// and no part in any decision this package makes.
//
// # Time
//
// No clock is read here. The evaluation time is the caller's, the observation
// time is the observer's, and the bound on how far apart observations may sit
// and still describe one moment is the caller's too — there is no default for
// it, in the same way the detector refuses to invent a horizon.
package f61

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/observex/platform/internal/adapt/tlscertexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// MaxProbeErrorMessage bounds the recorded description of a probe failure.
// A probe failure can carry text influenced by the endpoint being probed, so
// the description is sanitised and truncated rather than stored verbatim.
const MaxProbeErrorMessage = 256

// MemberStatus says whether a usable observation arrived from a vantage.
//
// It describes the REPORT, not the target. The target's condition is
// [tlscert.Observation.Outcome] and [tlscert.Observation.Trust], unchanged.
// The two are never interchangeable: a probe that never reported has no
// outcome at all, and in particular is not a tlscert.Timeout, which means the
// target did not complete a handshake within the probe's budget.
type MemberStatus string

const (
	// StatusReported: a usable observation arrived from this vantage. Read
	// Observation and, when HasResult reports true, Result.
	StatusReported MemberStatus = "reported"
	// StatusUnavailable: no usable observation arrived from this vantage —
	// the probe did not run, did not report, or reported something that was
	// not an observation. Cause says what was recorded about it. Observation
	// is the zero value and must not be read.
	StatusUnavailable MemberStatus = "unavailable"
)

// Errors returned when a set or a member cannot be formed. Each one refuses;
// none substitutes a default.
var (
	// ErrNoSubject: the set has no usable subject. A subject with any blank
	// field cannot be produced by wire.SubjectFromCheck, so this means the
	// caller bypassed it or used the zero Set.
	ErrNoSubject = errors.New("correlate/f61: set has no usable subject")
	// ErrNoSpan: the observation span was not supplied, or was not positive.
	// There is no default span.
	ErrNoSpan = errors.New("correlate/f61: observation span must be greater than zero")
	// ErrNoObservation: a reported member was offered the zero Observation.
	// A probe-level failure is recorded with AddUnavailable instead.
	ErrNoObservation = errors.New("correlate/f61: reported member carries no observation")
	// ErrNoObservationTime: the observation has no ObservedAt.
	ErrNoObservationTime = errors.New("correlate/f61: observation has no observation time")
	// ErrNoEvaluationTime: a result was supplied without the evaluation time
	// that produced it.
	ErrNoEvaluationTime = errors.New("correlate/f61: result supplied without an evaluation time")
	// ErrSubjectMismatch: the observation does not describe this set's check
	// and endpoint.
	ErrSubjectMismatch = errors.New("correlate/f61: observation does not match the set's subject")
	// ErrResultMismatch: the result does not describe the observation it was
	// offered with.
	ErrResultMismatch = errors.New("correlate/f61: result does not match its observation")
	// ErrNoCause: an unavailable member was offered no reason.
	ErrNoCause = errors.New("correlate/f61: unavailable member carries no cause")
	// ErrUnidentifiedVantage: a reported observation carries a vantage that
	// cannot identify where it was taken. Two such observations cannot be
	// shown to have come from independent vantages, which is the only reason
	// a set holds more than one, so the observation is refused rather than
	// counted. It is an invalid correlation input, not a probe failure.
	ErrUnidentifiedVantage = errors.New("correlate/f61: observation carries an unidentified vantage")
	// ErrOutsideSpan: accepting this observation would stretch the set's
	// observation times beyond the caller's span, so it was refused rather
	// than merged with evidence that may describe a different moment.
	ErrOutsideSpan = errors.New("correlate/f61: observation lies outside the set's span")
)

// ProbeError is the recorded description of a probe-level failure.
//
// It is deliberately a bounded, sanitised string rather than the caller's
// error value: a member is copied to readers that may render or store it, and
// the text can be influenced by the probed endpoint. Callers that need to
// match a sentinel do so on their own error, before calling AddUnavailable.
type ProbeError struct {
	// Message is the sanitised cause, at most MaxProbeErrorMessage runes,
	// with control characters removed and whitespace collapsed.
	Message string
}

// Member is one vantage's report about the set's subject, in the order it
// arrived. A member is a value: copying it copies everything a reader may
// observe, and nothing a reader may write.
type Member struct {
	// Subject is the set's subject, stamped on every member so that a member
	// carries its own tenancy when it travels alone. It always equals the
	// set's subject.
	Subject wire.Subject

	// Status says whether a usable observation arrived. Read it before
	// anything else on this member.
	Status MemberStatus

	// Vantage is where this report came from, uniformly for both statuses.
	// For a reported member it is exactly Observation.Vantage, copied so that
	// the location is readable without a status check; it is never derived,
	// rewritten or defaulted, and it is always identified, because an
	// unidentified one is refused at admission. An unavailable member may
	// carry an unidentified vantage: nothing was observed, so nothing is
	// being attributed. This package does not judge whether a vantage is
	// genuinely external.
	Vantage tlscert.Vantage

	// Observation is the observer's report, unchanged. It is meaningful only
	// when Status is StatusReported; otherwise it is the zero value, whose
	// Outcome is the empty string and never tlscert.Timeout.
	Observation tlscert.Observation

	// Result is the adapter's per-certificate outcome for Observation,
	// unchanged, including each certificate's detector Finding in wire order.
	// It is meaningful only when HasResult reports true: an observation that
	// carries no certificate — a transport failure, a timeout — yields no
	// result.
	Result tlscertexpiry.Result

	// EvaluatedAt is the evaluation time the caller passed to the adapter,
	// normalised to UTC. The adapter's Result has nowhere to record it, so it
	// is kept here; it is zero when no result was produced.
	EvaluatedAt time.Time

	// Cause describes why no observation arrived. It is populated only when
	// Status is StatusUnavailable.
	Cause ProbeError
}

// HasObservation reports whether Observation is meaningful.
func (m Member) HasObservation() bool { return m.Status == StatusReported }

// HasResult reports whether Result is meaningful. A reported observation that
// presented no certificate has none.
func (m Member) HasResult() bool {
	return m.Status == StatusReported && m.Result.Outcome != ""
}

// VantageIdentified reports whether this member's vantage carries both halves
// of its identity. It is always true for a reported member, because
// AddReported refuses anything else; it can be false only for an unavailable
// member, where nothing was observed and nothing is being attributed.
func (m Member) VantageIdentified() bool { return identified(m.Vantage) }

// identified is the admission rule for a reported observation's vantage: both
// halves must carry content. Neither half is invented, normalised or derived,
// and no other field — not the subject's org, namespace, check or endpoint,
// not the observed host, not the environment, not this process — is ever used
// as a fallback identity. A vantage that does not identify itself is not
// identified.
func identified(v tlscert.Vantage) bool {
	return !blank(v.Kind) && !blank(v.ID)
}

// Set is the correlated observations of one subject, in arrival order.
//
// A Set is a mutable collection owned by whoever created it. It is NOT safe
// for concurrent use, and no synchronisation is claimed: a caller fanning a
// check out to several probes synchronises the adds itself, exactly as it
// supplies the clock itself. Do not copy a Set after adding to it — pass a
// pointer — because the copies would share one backing array. Reading a Set
// through its accessors is always safe, because each returns a copy.
type Set struct {
	subject wire.Subject
	span    time.Duration

	members []Member

	// earliest and latest bound the observation times of reported members.
	earliest time.Time
	latest   time.Time
}

// New starts an empty set for one subject.
//
// subject must be a subject wire.SubjectFromCheck produced; this package has
// no other way to obtain one and never builds one itself, so the check row
// remains the only source of tenancy.
//
// span is the caller's bound on how far apart two observations may sit and
// still be treated as describing one moment. There is no default and no
// constant for it anywhere in this package: an observation that would stretch
// the set beyond span is refused with ErrOutsideSpan rather than quietly
// merged with evidence from a different moment. What span ought to be is an
// unresolved policy question, and it is in this signature so that it stays
// visible.
func New(subject wire.Subject, span time.Duration) (Set, error) {
	if !usableSubject(subject) {
		return Set{}, ErrNoSubject
	}
	if span <= 0 {
		return Set{}, ErrNoSpan
	}
	return Set{subject: subject, span: span}, nil
}

// Subject returns the subject this set is keyed by. The Subject value is
// itself the correlation key: it is comparable, so callers may use it directly
// as a map key without composing a string.
func (s Set) Subject() wire.Subject { return s.subject }

// Span returns the caller-supplied observation span.
func (s Set) Span() time.Duration { return s.span }

// Len returns the number of members.
func (s Set) Len() int { return len(s.members) }

// Members returns a copy of the members, in the order they were added.
// Mutating the returned slice or its elements cannot affect the set.
func (s Set) Members() []Member {
	if s.members == nil {
		return nil
	}
	out := make([]Member, len(s.members))
	copy(out, s.members)
	return out
}

// AddReported records one vantage's observation, and the adapter result
// derived from it.
//
// obs is the observer's report, stored unchanged: its Outcome, Trust, Vantage,
// ObservedAt and presented chain are all preserved exactly as the observer
// produced them. res is the result of the caller's own tlscertexpiry.Adapt
// call on obs, or the zero Result when the observation carried no certificate
// and the adapter was not called; evaluatedAt is the time the caller passed to
// that call, and is required whenever a result is supplied.
//
// The observation must describe this set's check and endpoint, its vantage
// must identify itself, and the result must describe the observation, or the
// member is refused and nothing is appended. Members are appended: an
// identified vantage that reports twice yields two members, and nothing is
// overwritten or deduplicated — this package has no rule by which to decide
// that two reports are one.
func (s *Set) AddReported(obs tlscert.Observation, res tlscertexpiry.Result, evaluatedAt time.Time) error {
	if !usableSubject(s.subject) {
		return ErrNoSubject
	}
	// A zero Observation is not a report. Probe returns one alongside an
	// error when the target was unusable; that is AddUnavailable's case.
	if obs.Outcome == "" {
		return ErrNoObservation
	}
	if obs.CheckID != s.subject.CheckID || obs.Endpoint != s.subject.Endpoint {
		return ErrSubjectMismatch
	}
	// An observation that cannot say where it was taken cannot be shown to be
	// independent of any other member, so it is refused outright: not kept,
	// and not turned into an unavailable member either, because a probe that
	// reported is not a probe that failed.
	if !identified(obs.Vantage) {
		return ErrUnidentifiedVantage
	}
	if obs.ObservedAt.IsZero() {
		return ErrNoObservationTime
	}

	hasResult := res.Outcome != ""
	if hasResult {
		if res.CheckID != obs.CheckID || res.Endpoint != obs.Endpoint {
			return ErrResultMismatch
		}
		if evaluatedAt.IsZero() {
			return ErrNoEvaluationTime
		}
	}

	earliest, latest, err := s.spanWith(obs.ObservedAt)
	if err != nil {
		return err
	}

	m := Member{
		Subject:     s.subject,
		Status:      StatusReported,
		Vantage:     obs.Vantage,
		Observation: obs,
	}
	if hasResult {
		m.Result = res
		m.EvaluatedAt = evaluatedAt.UTC()
	}
	s.members = append(s.members, m)
	s.earliest, s.latest = earliest, latest
	return nil
}

// AddUnavailable records that a vantage produced no usable observation.
//
// cause is the probe-level failure — the error the observer returned, a
// transport failure reaching the probe, or the caller's own statement that the
// probe never reported. It is recorded as a bounded, sanitised description and
// is required: an unavailable member without a reason says nothing.
//
// No observation is manufactured. The member's Observation stays the zero
// value, so its Outcome is empty and can never be mistaken for a target
// timeout, and no detector finding exists or is produced.
func (s *Set) AddUnavailable(v tlscert.Vantage, cause error) error {
	if !usableSubject(s.subject) {
		return ErrNoSubject
	}
	if cause == nil {
		return ErrNoCause
	}
	msg := sanitise(cause.Error())
	if msg == "" {
		return ErrNoCause
	}
	s.members = append(s.members, Member{
		Subject: s.subject,
		Status:  StatusUnavailable,
		Vantage: v,
		Cause:   ProbeError{Message: msg},
	})
	return nil
}

// spanWith returns the set's observation bounds once t is included, or
// ErrOutsideSpan if including it would exceed the caller's span. Only reported
// members have an observation time, so unavailable members never constrain it.
func (s Set) spanWith(t time.Time) (earliest, latest time.Time, err error) {
	earliest, latest = s.earliest, s.latest
	if earliest.IsZero() {
		return t, t, nil
	}
	if t.Before(earliest) {
		earliest = t
	}
	if t.After(latest) {
		latest = t
	}
	if latest.Sub(earliest) > s.span {
		return time.Time{}, time.Time{}, ErrOutsideSpan
	}
	return earliest, latest, nil
}

// usableSubject reports whether every field of the subject carries content.
// It is a guard against a subject that did not come from SubjectFromCheck, not
// a second way to build one.
func usableSubject(s wire.Subject) bool {
	return !blank(s.OrgID) && !blank(s.Namespace) && !blank(s.ServiceID) &&
		!blank(s.CheckID) && !blank(s.Endpoint)
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// sanitise turns an error's text into a bounded single-line description:
// control characters become spaces, runs of whitespace collapse, and the
// result is truncated to MaxProbeErrorMessage runes.
func sanitise(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	runes := []rune(cleaned)
	if len(runes) > MaxProbeErrorMessage {
		return string(runes[:MaxProbeErrorMessage])
	}
	return cleaned
}
