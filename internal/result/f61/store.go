// Package f61 persists F6.1 TLS certificate results (PROPOSED DEST-1): the
// latest observation of each vantage, the per-check verdict with its alert
// episode, and a transactional outbox of transitions. Schema:
// internal/db/migrations/009_f61_tls_certificates.sql.
//
// # Write path (processor)
//
// Record handles one accepted observation in one transaction:
//
//  1. serialise per check (transaction-scoped advisory lock);
//  2. upsert the vantage's observation only if it is strictly newer than the
//     stored one — a duplicate or out-of-order report changes nothing;
//  3. reload the check's stored observations and rebuild them from their wire
//     documents, bound to the check's current target and each row's vantage
//     (rows for a target the check no longer names are deleted);
//  4. evaluate (internal/evaluate/f61) and derive the alert episode;
//  5. upsert the result and, on a transition, append an outbox event;
//  6. prune old rows.
//
// Any error rolls the whole transaction back; the caller answers 503 and the
// probe's next report supersedes this one.
//
// # Read path (gateway)
//
// Every read takes the organization as an argument from the caller's
// authenticated context and filters by it in SQL. A check of another
// organization is indistinguishable from a missing one.
package f61

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	evaluate "github.com/observex/platform/internal/evaluate/f61"
	"github.com/observex/platform/internal/observe/tlscert"
	wire "github.com/observex/platform/internal/wire/f61"
)

// Retention (PROPOSED DEST-1).
const (
	ObservationRetention = 30 * 24 * time.Hour
	EventRetention       = 90 * 24 * time.Hour
	pruneBatch           = 200
)

// Alert states and event types.
const (
	AlertNone = "none"
	AlertOpen = "open"

	EventOpened    = "opened"
	EventEscalated = "escalated"
	EventResolved  = "resolved"
)

// Errors.
var (
	ErrNotFound = errors.New("result/f61: not found")
	ErrInput    = errors.New("result/f61: accepted observation is incomplete")
)

// DB is the part of pgx the store needs: *pgxpool.Pool and pgx.Conn satisfy it.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store reads and writes F6.1 results.
type Store struct{ db DB }

// New returns a Store over db.
func New(db DB) *Store { return &Store{db: db} }

// Accepted is one observation the intake has authenticated, authorized and
// decoded. Subject and IntervalSec come from the authoritative check row; the
// observation's vantage came from the verified credential.
type Accepted struct {
	Subject     wire.Subject
	IntervalSec int
	Observation tlscert.Observation
	ReceivedAt  time.Time
}

// Outcome reports what Record did.
type Outcome struct {
	// Stored is false when the observation was not newer than the stored one
	// for the same vantage; nothing else changed.
	Stored  bool
	Verdict evaluate.Verdict
	// Event is the outbox event written, if any.
	Event string
	// AlertState after the write.
	AlertState string
}

// Record applies one accepted observation. horizon is the server policy.
func (s *Store) Record(ctx context.Context, a Accepted, horizon time.Duration) (Outcome, error) {
	o := a.Observation
	if a.Subject.CheckID == "" || a.Subject.OrgID == "" || a.IntervalSec <= 0 || a.ReceivedAt.IsZero() ||
		o.CheckID != a.Subject.CheckID || o.Endpoint != a.Subject.Endpoint || o.Vantage.ID == "" || o.Vantage.Kind == "" {
		return Outcome{}, ErrInput
	}
	payload, err := tlscert.MarshalObservation(o)
	if err != nil {
		return Outcome{}, fmt.Errorf("%w: %w", ErrInput, err)
	}
	interval := time.Duration(a.IntervalSec) * time.Second
	policy := evaluate.Policy{Horizon: horizon, Span: evaluate.SpanFor(interval)}
	if err := policy.Validate(); err != nil {
		return Outcome{}, err
	}
	now := a.ReceivedAt.UTC()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Outcome{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('f61:' || $1, 0))`, a.Subject.CheckID); err != nil {
		return Outcome{}, err
	}

	var leafSHA string
	var leafNotAfter *time.Time
	if leaf, ok := o.Leaf(); ok {
		leafSHA = sha256Hex(leaf.DER())
		na := leaf.NotAfter
		leafNotAfter = &na
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO f61_tls_observations
		  (check_id, org_id, vantage_id, vantage_kind, observed_at, received_at, outcome, leaf_sha256, leaf_not_after, wire_version, payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (check_id, vantage_id) DO UPDATE SET
		  org_id = EXCLUDED.org_id, vantage_kind = EXCLUDED.vantage_kind, observed_at = EXCLUDED.observed_at,
		  received_at = EXCLUDED.received_at, outcome = EXCLUDED.outcome, leaf_sha256 = EXCLUDED.leaf_sha256,
		  leaf_not_after = EXCLUDED.leaf_not_after, wire_version = EXCLUDED.wire_version, payload = EXCLUDED.payload
		WHERE f61_tls_observations.observed_at < EXCLUDED.observed_at`,
		a.Subject.CheckID, a.Subject.OrgID, o.Vantage.ID, o.Vantage.Kind, o.ObservedAt.UTC(), now,
		string(o.Outcome), leafSHA, leafNotAfter, tlscert.WireVersion, payload)
	if err != nil {
		return Outcome{}, err
	}
	if tag.RowsAffected() == 0 {
		// Not newer than what this vantage already reported: idempotent no-op.
		if err := tx.Commit(ctx); err != nil {
			return Outcome{}, err
		}
		return Outcome{Stored: false}, nil
	}

	latest, err := loadObservations(ctx, tx, a.Subject)
	if err != nil {
		return Outcome{}, err
	}
	verdict, err := evaluate.Evaluate(a.Subject, latest, now, policy)
	if err != nil {
		return Outcome{}, err
	}

	prev, hasPrev, err := loadResultForUpdate(ctx, tx, a.Subject.CheckID)
	if err != nil {
		return Outcome{}, err
	}
	next, event := transition(prev, hasPrev, verdict, now)

	vantagesJSON, err := json.Marshal(verdict.Vantages)
	if err != nil {
		return Outcome{}, err
	}
	var earliest *time.Time
	if !verdict.EarliestNotAfter.IsZero() {
		e := verdict.EarliestNotAfter
		earliest = &e
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO f61_tls_results
		  (check_id, org_id, namespace, endpoint, status, earliest_not_after, horizon_seconds, span_seconds, interval_sec,
		   vantages_in_span, vantages_evaluated, vantages_stale, disagreement, vantages, alert_state, episode_id,
		   opened_at, resolved_at, status_changed_at, evaluated_at, last_observed_at, episode_peak, version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,1)
		ON CONFLICT (check_id) DO UPDATE SET
		  org_id = EXCLUDED.org_id, namespace = EXCLUDED.namespace, endpoint = EXCLUDED.endpoint, status = EXCLUDED.status,
		  earliest_not_after = EXCLUDED.earliest_not_after, horizon_seconds = EXCLUDED.horizon_seconds,
		  span_seconds = EXCLUDED.span_seconds, interval_sec = EXCLUDED.interval_sec,
		  vantages_in_span = EXCLUDED.vantages_in_span, vantages_evaluated = EXCLUDED.vantages_evaluated,
		  vantages_stale = EXCLUDED.vantages_stale, disagreement = EXCLUDED.disagreement, vantages = EXCLUDED.vantages,
		  alert_state = EXCLUDED.alert_state, episode_id = EXCLUDED.episode_id, opened_at = EXCLUDED.opened_at,
		  resolved_at = EXCLUDED.resolved_at, status_changed_at = EXCLUDED.status_changed_at,
		  evaluated_at = EXCLUDED.evaluated_at, last_observed_at = EXCLUDED.last_observed_at,
		  episode_peak = EXCLUDED.episode_peak, version = f61_tls_results.version + 1`,
		a.Subject.CheckID, a.Subject.OrgID, a.Subject.Namespace, a.Subject.Endpoint, string(verdict.Status), earliest,
		int64(horizon/time.Second), int64(policy.Span/time.Second), a.IntervalSec,
		verdict.VantagesInSpan, verdict.VantagesEvaluated, verdict.VantagesStale, verdict.Disagreement, vantagesJSON,
		next.AlertState, nullString(next.EpisodeID), next.OpenedAt, next.ResolvedAt, next.StatusChangedAt,
		verdict.EvaluatedAt, verdict.LastObservedAt, nullString(next.Peak)); err != nil {
		return Outcome{}, err
	}
	if event != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO f61_tls_result_events
			  (org_id, namespace, check_id, episode_id, event_type, status_from, status_to, earliest_not_after, occurred_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			a.Subject.OrgID, a.Subject.Namespace, a.Subject.CheckID, next.EventEpisode, event,
			next.StatusFrom, string(verdict.Status), earliest, now); err != nil {
			return Outcome{}, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM f61_tls_observations WHERE check_id = $1 AND observed_at < $2`,
		a.Subject.CheckID, now.Add(-ObservationRetention)); err != nil {
		return Outcome{}, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM f61_tls_result_events WHERE id IN
		(SELECT id FROM f61_tls_result_events WHERE occurred_at < $1 ORDER BY id LIMIT $2)`,
		now.Add(-EventRetention), pruneBatch); err != nil {
		return Outcome{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Outcome{}, err
	}
	return Outcome{Stored: true, Verdict: verdict, Event: event, AlertState: next.AlertState}, nil
}

// loadObservations rebuilds the check's stored observations. A row whose
// payload no longer decodes against the check's current target (the target
// was changed) is obsolete and is deleted rather than evaluated.
func loadObservations(ctx context.Context, tx pgx.Tx, s wire.Subject) ([]tlscert.Observation, error) {
	rows, err := tx.Query(ctx, `SELECT vantage_id, vantage_kind, payload FROM f61_tls_observations
		WHERE check_id = $1 AND org_id = $2 ORDER BY vantage_id`, s.CheckID, s.OrgID)
	if err != nil {
		return nil, err
	}
	type row struct {
		id, kind string
		payload  []byte
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.kind, &r.payload); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []tlscert.Observation
	for _, r := range all {
		o, err := tlscert.UnmarshalObservation(r.payload, tlscert.Binding{
			CheckID: s.CheckID, Endpoint: s.Endpoint, Vantage: tlscert.Vantage{Kind: r.kind, ID: r.id},
		})
		if errors.Is(err, tlscert.ErrWireBinding) {
			if _, err := tx.Exec(ctx, `DELETE FROM f61_tls_observations WHERE check_id = $1 AND vantage_id = $2`, s.CheckID, r.id); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// prevResult is the lifecycle part of a stored result.
type prevResult struct {
	Status          string
	AlertState      string
	EpisodeID       string
	Peak            string
	OpenedAt        *time.Time
	ResolvedAt      *time.Time
	StatusChangedAt time.Time
}

func loadResultForUpdate(ctx context.Context, tx pgx.Tx, checkID string) (prevResult, bool, error) {
	var p prevResult
	var episode, peak *string
	err := tx.QueryRow(ctx, `SELECT status, alert_state, episode_id, episode_peak, opened_at, resolved_at, status_changed_at
		FROM f61_tls_results WHERE check_id = $1 FOR UPDATE`, checkID).
		Scan(&p.Status, &p.AlertState, &episode, &peak, &p.OpenedAt, &p.ResolvedAt, &p.StatusChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return prevResult{AlertState: AlertNone}, false, nil
	}
	if err != nil {
		return prevResult{}, false, err
	}
	if episode != nil {
		p.EpisodeID = *episode
	}
	if peak != nil {
		p.Peak = *peak
	}
	return p, true, nil
}

// nextResult is the lifecycle part to write.
type nextResult struct {
	AlertState      string
	EpisodeID       string
	Peak            string
	OpenedAt        *time.Time
	ResolvedAt      *time.Time
	StatusChangedAt time.Time
	StatusFrom      string
	EventEpisode    string
}

// transition applies the episode rules (PROPOSED DEST-1 / SPAN-1 stability):
// an episode opens on expiring or expired, escalates on expiring→expired, and
// resolves only on a definitive ok. unreachable, unknown and hostname_mismatch
// neither open nor resolve an expiry episode.
func transition(prev prevResult, hasPrev bool, v evaluate.Verdict, now time.Time) (nextResult, string) {
	status := string(v.Status)
	n := nextResult{
		AlertState: prev.AlertState, EpisodeID: prev.EpisodeID, Peak: prev.Peak, OpenedAt: prev.OpenedAt, ResolvedAt: prev.ResolvedAt,
		StatusChangedAt: prev.StatusChangedAt, StatusFrom: prev.Status,
	}
	if n.AlertState == "" {
		n.AlertState = AlertNone
	}
	if !hasPrev || prev.Status != status {
		n.StatusChangedAt = now
	}
	if n.StatusFrom == "" {
		n.StatusFrom = "none"
	}
	switch {
	case n.AlertState == AlertNone && v.Status.Alerting():
		t := now
		n.AlertState, n.EpisodeID, n.OpenedAt, n.ResolvedAt = AlertOpen, uuid.NewString(), &t, nil
		n.Peak = status
		n.EventEpisode = n.EpisodeID
		return n, EventOpened
	case n.AlertState == AlertOpen && v.Status == evaluate.StatusExpired && n.Peak != string(evaluate.StatusExpired):
		// Escalation happens once per episode, however the status wanders
		// through unreachable or unknown in between.
		n.Peak = status
		n.EventEpisode = n.EpisodeID
		return n, EventEscalated
	case n.AlertState == AlertOpen && v.Status == evaluate.StatusOK:
		t := now
		n.EventEpisode = n.EpisodeID
		n.AlertState, n.EpisodeID, n.Peak, n.OpenedAt, n.ResolvedAt = AlertNone, "", "", nil, &t
		return n, EventResolved
	}
	return n, ""
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
