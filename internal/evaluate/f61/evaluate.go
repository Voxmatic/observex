// Package f61 evaluates the latest per-vantage TLS observations of one
// synthetic check into a single, deterministic verdict (F6.1-HORIZON-1 and
// F6.1-SPAN-1, PROPOSED in the increment-3 decision record).
//
// # How a verdict is made
//
//  1. The caller supplies the latest accepted observation of each vantage for
//     one subject, and the time of evaluation.
//  2. Observations older than newest−span are stale: they are listed, but they
//     take no part in the verdict. The rest form the correlation set, built
//     with the frozen internal/correlate/f61 and internal/compose/f61 exactly
//     as designed, fed by a replay prober over the stored observations.
//  3. Each in-span vantage is evaluated by the frozen adapter and detector
//     (every presented certificate, leaf and intermediates) and given a
//     status: expired, expiring, hostname_mismatch, ok, unreachable or
//     unknown.
//  4. The subject's status is the worst in-span vantage status. Vantages that
//     saw different leaf certificates are reported as a disagreement.
//
// # What it does not do
//
// It reads no clock, performs no I/O and keeps no state: the same inputs
// always give the same verdict. It never decides tenancy — the subject comes
// from the caller, who built it from the check row — and never invents a
// vantage: each observation already carries the vantage the processor bound
// it to from the verified credential.
package f61

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/observex/platform/internal/adapt/tlscertexpiry"
	compose "github.com/observex/platform/internal/compose/f61"
	correlate "github.com/observex/platform/internal/correlate/f61"
	"github.com/observex/platform/internal/detect/certexpiry"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// Policy defaults and bounds (PROPOSED HORIZON-1, SPAN-1).
const (
	// DefaultHorizon: a certificate within 30 days of expiry is "expiring".
	DefaultHorizon = 30 * 24 * time.Hour
	MinHorizon     = 24 * time.Hour
	MaxHorizon     = 365 * 24 * time.Hour
	// ClockSkew is tolerated between a vantage clock and the processor clock.
	ClockSkew = 30 * time.Second
	// SpanSlack is added to twice the interval to form the span.
	SpanSlack = 60 * time.Second
	// StaleAfterIntervals: a result not refreshed for this many intervals is
	// stale when read.
	StaleAfterIntervals = 3
)

// SpanFor is the correlation span for a check probed every interval by each
// vantage: two intervals (one missed report tolerated) plus slack.
func SpanFor(interval time.Duration) time.Duration { return 2*interval + SpanSlack }

// Policy is the server-owned evaluation policy.
type Policy struct {
	Horizon time.Duration
	Span    time.Duration
}

// Errors.
var (
	ErrPolicy         = errors.New("evaluate/f61: horizon or span outside the permitted range")
	ErrNoTime         = errors.New("evaluate/f61: evaluation time is required")
	ErrNoSubject      = errors.New("evaluate/f61: subject is incomplete")
	ErrForeign        = errors.New("evaluate/f61: observation does not belong to the subject")
	ErrUnidentified   = errors.New("evaluate/f61: observation carries no identified vantage")
	ErrFutureObserved = errors.New("evaluate/f61: observation is newer than the evaluation time allows")
)

// Validate refuses a policy the detector could not run with sensibly.
func (p Policy) Validate() error {
	if p.Horizon < MinHorizon || p.Horizon > MaxHorizon || p.Span <= 0 {
		return ErrPolicy
	}
	return nil
}

// Status is a vantage or subject status.
type Status string

const (
	StatusExpired          Status = "expired"
	StatusExpiring         Status = "expiring"
	StatusHostnameMismatch Status = "hostname_mismatch"
	StatusOK               Status = "ok"
	StatusUnreachable      Status = "unreachable"
	StatusUnknown          Status = "unknown"
	// StatusStale is used only for a vantage outside the span.
	StatusStale Status = "stale"
)

// rank orders statuses for the worst-case aggregate.
func rank(s Status) int {
	switch s {
	case StatusExpired:
		return 6
	case StatusExpiring:
		return 5
	case StatusHostnameMismatch:
		return 4
	case StatusOK:
		return 3
	case StatusUnreachable:
		return 2
	case StatusUnknown:
		return 1
	default:
		return 0
	}
}

// Definitive reports whether s is a statement about the certificate rather
// than about reachability.
func (s Status) Definitive() bool {
	return s == StatusExpired || s == StatusExpiring || s == StatusHostnameMismatch || s == StatusOK
}

// Alerting reports whether s opens (or keeps open) an expiry episode.
func (s Status) Alerting() bool { return s == StatusExpired || s == StatusExpiring }

// CertificateView is one presented certificate as a vantage saw it.
type CertificateView struct {
	Position      int           `json:"position"`
	IsLeaf        bool          `json:"is_leaf"`
	Subject       string        `json:"subject"`
	Issuer        string        `json:"issuer"`
	SerialNumber  string        `json:"serial_number"`
	NotAfter      time.Time     `json:"not_after"`
	Remaining     time.Duration `json:"remaining_ns"`
	Evaluated     bool          `json:"evaluated"`
	WithinHorizon bool          `json:"within_horizon"`
}

// VantageVerdict is one vantage's contribution.
type VantageVerdict struct {
	VantageKind      string            `json:"vantage_kind"`
	VantageID        string            `json:"vantage_id"`
	ObservedAt       time.Time         `json:"observed_at"`
	Outcome          tlscert.Outcome   `json:"outcome"`
	Status           Status            `json:"status"`
	InSpan           bool              `json:"in_span"`
	Trusted          bool              `json:"trusted"`
	TrustReason      string            `json:"trust_reason,omitempty"`
	LeafSHA256       string            `json:"leaf_sha256,omitempty"`
	EarliestNotAfter time.Time         `json:"earliest_not_after,omitempty"`
	Certificates     []CertificateView `json:"certificates,omitempty"`
}

// Verdict is the subject-level evaluation.
type Verdict struct {
	Subject           wire.Subject
	Status            Status
	EvaluatedAt       time.Time
	Horizon           time.Duration
	Span              time.Duration
	EarliestNotAfter  time.Time // zero when no certificate was evaluated
	Remaining         time.Duration
	LastObservedAt    time.Time
	VantagesInSpan    int
	VantagesEvaluated int
	VantagesStale     int
	Disagreement      bool
	Vantages          []VantageVerdict // sorted by vantage ID
}

// Evaluate computes the verdict for subject from the latest observation of
// each vantage, at time now.
//
// Every observation must carry subject's check ID and endpoint and an
// identified vantage (the processor set it from the credential); none may be
// newer than now+ClockSkew. If a vantage appears twice, its newest
// observation is used.
func Evaluate(subject wire.Subject, latest []tlscert.Observation, now time.Time, p Policy) (Verdict, error) {
	if err := p.Validate(); err != nil {
		return Verdict{}, err
	}
	if now.IsZero() {
		return Verdict{}, ErrNoTime
	}
	if subject.OrgID == "" || subject.Namespace == "" || subject.CheckID == "" || subject.Endpoint == "" {
		return Verdict{}, ErrNoSubject
	}

	byVantage := map[string]tlscert.Observation{}
	var newest time.Time
	for _, o := range latest {
		if o.CheckID != subject.CheckID || o.Endpoint != subject.Endpoint {
			return Verdict{}, ErrForeign
		}
		if o.Vantage.Kind == "" || o.Vantage.ID == "" {
			return Verdict{}, ErrUnidentified
		}
		if o.ObservedAt.After(now.Add(ClockSkew)) {
			return Verdict{}, ErrFutureObserved
		}
		if prev, ok := byVantage[o.Vantage.ID]; !ok || o.ObservedAt.After(prev.ObservedAt) {
			byVantage[o.Vantage.ID] = o
		}
		if o.ObservedAt.After(newest) {
			newest = o.ObservedAt
		}
	}
	ids := make([]string, 0, len(byVantage))
	for id := range byVantage {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// A vantage clock up to ClockSkew ahead is tolerated by evaluating at the
	// later of the two times, so the detector never sees a future observation.
	evalAt := now.UTC()
	if newest.After(evalAt) {
		evalAt = newest.UTC()
	}

	v := Verdict{
		Subject:        subject,
		Status:         StatusUnknown,
		EvaluatedAt:    evalAt,
		Horizon:        p.Horizon,
		Span:           p.Span,
		LastObservedAt: newest.UTC(),
	}
	set, err := correlate.New(subject, p.Span)
	if err != nil {
		return Verdict{}, err
	}
	params := certexpiry.Params{Horizon: p.Horizon, MaxObservationAge: p.Span + ClockSkew}
	cutoff := newest.Add(-p.Span)

	stale := map[string]tlscert.Observation{}
	for _, id := range ids {
		o := byVantage[id]
		if o.ObservedAt.Before(cutoff) {
			stale[id] = o
			continue
		}
		step, err := compose.Observe(context.Background(), &set, compose.Input{
			Prober:             replay{obs: o},
			Timeout:            time.Second,
			Params:             params,
			EvaluatedAt:        evalAt,
			UnavailableVantage: o.Vantage,
		})
		if err != nil {
			return Verdict{}, err
		}
		_ = step
	}

	leaves := map[string]bool{}
	for _, m := range set.Members() {
		vv := vantageVerdict(m, evalAt)
		vv.InSpan = true
		v.VantagesInSpan++
		if vv.Status.Definitive() && vv.Status != StatusHostnameMismatch {
			v.VantagesEvaluated++
		}
		if vv.LeafSHA256 != "" {
			leaves[vv.LeafSHA256] = true
		}
		if rank(vv.Status) > rank(v.Status) {
			v.Status = vv.Status
		}
		if !vv.EarliestNotAfter.IsZero() && (v.EarliestNotAfter.IsZero() || vv.EarliestNotAfter.Before(v.EarliestNotAfter)) {
			v.EarliestNotAfter = vv.EarliestNotAfter
		}
		v.Vantages = append(v.Vantages, vv)
	}
	for _, id := range ids {
		o, ok := stale[id]
		if !ok {
			continue
		}
		v.VantagesStale++
		v.Vantages = append(v.Vantages, VantageVerdict{
			VantageKind: o.Vantage.Kind, VantageID: o.Vantage.ID, ObservedAt: o.ObservedAt.UTC(),
			Outcome: o.Outcome, Status: StatusStale, Trusted: o.Trust.Verified(), TrustReason: o.Trust.Reason,
			LeafSHA256: leafSHA256(o),
		})
	}
	sort.SliceStable(v.Vantages, func(i, j int) bool { return v.Vantages[i].VantageID < v.Vantages[j].VantageID })
	v.Disagreement = len(leaves) > 1
	if !v.EarliestNotAfter.IsZero() {
		v.Remaining = v.EarliestNotAfter.Sub(evalAt)
	}
	return v, nil
}

// vantageVerdict evaluates one member of the correlation set.
func vantageVerdict(m correlate.Member, evalAt time.Time) VantageVerdict {
	o := m.Observation
	vv := VantageVerdict{
		VantageKind: m.Vantage.Kind, VantageID: m.Vantage.ID, ObservedAt: o.ObservedAt.UTC(),
		Outcome: o.Outcome, Status: StatusUnreachable, Trusted: o.Trust.Verified(), TrustReason: o.Trust.Reason,
		LeafSHA256: leafSHA256(o),
	}
	if !m.HasResult() {
		// No certificate was observed: transport failure, timeout, malformed
		// peer or no certificates.
		return vv
	}
	evaluated, withinHorizon, expired, failed := 0, false, false, false
	for _, c := range m.Result.Certificates() {
		cv := CertificateView{
			Position: c.Position, IsLeaf: c.IsLeaf, Subject: c.Subject, Issuer: c.Issuer,
			SerialNumber: c.SerialNumber, NotAfter: c.NotAfter.UTC(), Remaining: c.NotAfter.Sub(evalAt),
			Evaluated: c.Status == tlscertexpiry.StatusEvaluated,
		}
		if cv.Evaluated {
			evaluated++
			if c.Err != nil {
				failed = true
			}
			if c.Found {
				cv.WithinHorizon = true
				withinHorizon = true
				if c.Finding.Remaining <= 0 {
					expired = true
				}
			}
			if vv.EarliestNotAfter.IsZero() || cv.NotAfter.Before(vv.EarliestNotAfter) {
				vv.EarliestNotAfter = cv.NotAfter
			}
		}
		vv.Certificates = append(vv.Certificates, cv)
	}
	switch {
	case evaluated == 0:
		vv.Status = StatusHostnameMismatch
	case expired:
		vv.Status = StatusExpired
	case withinHorizon:
		vv.Status = StatusExpiring
	case failed:
		vv.Status = StatusUnknown
	default:
		vv.Status = StatusOK
	}
	return vv
}

func leafSHA256(o tlscert.Observation) string {
	leaf, ok := o.Leaf()
	if !ok {
		return ""
	}
	sum := sha256.Sum256(leaf.DER())
	return hex.EncodeToString(sum[:])
}

// replay is a tlscert.Prober that returns one stored, already validated
// observation. It lets the frozen composition step run unchanged over
// observations that were taken by a remote probe.
type replay struct{ obs tlscert.Observation }

var errReplayTarget = errors.New("evaluate/f61: replay target does not match the stored observation")

func (r replay) Probe(_ context.Context, t tlscert.Target) (tlscert.Observation, error) {
	if t.CheckID != r.obs.CheckID || t.Endpoint != r.obs.Endpoint {
		return tlscert.Observation{}, errReplayTarget
	}
	return r.obs, nil
}
