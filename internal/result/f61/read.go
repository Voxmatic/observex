package f61

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	evaluate "github.com/observex/platform/internal/evaluate/f61"
)

// Result is the authoritative per-check record as read by the API.
type Result struct {
	CheckID           string                    `json:"check_id"`
	Namespace         string                    `json:"namespace"`
	Endpoint          string                    `json:"endpoint"`
	Status            string                    `json:"status"`
	EarliestNotAfter  *time.Time                `json:"earliest_not_after,omitempty"`
	RemainingSeconds  *int64                    `json:"remaining_seconds,omitempty"`
	HorizonSeconds    int64                     `json:"horizon_seconds"`
	SpanSeconds       int64                     `json:"span_seconds"`
	IntervalSec       int                       `json:"interval_sec"`
	VantagesInSpan    int                       `json:"vantages_in_span"`
	VantagesEvaluated int                       `json:"vantages_evaluated"`
	VantagesStale     int                       `json:"vantages_stale"`
	Disagreement      bool                      `json:"disagreement"`
	AlertState        string                    `json:"alert_state"`
	EpisodeID         string                    `json:"episode_id,omitempty"`
	OpenedAt          *time.Time                `json:"opened_at,omitempty"`
	ResolvedAt        *time.Time                `json:"resolved_at,omitempty"`
	StatusChangedAt   time.Time                 `json:"status_changed_at"`
	EvaluatedAt       time.Time                 `json:"evaluated_at"`
	LastObservedAt    time.Time                 `json:"last_observed_at"`
	Stale             bool                      `json:"stale"`
	Vantages          []evaluate.VantageVerdict `json:"vantages,omitempty"`
}

// Event is one outbox entry.
type Event struct {
	ID               int64      `json:"id"`
	Namespace        string     `json:"namespace"`
	CheckID          string     `json:"check_id"`
	EpisodeID        string     `json:"episode_id"`
	Type             string     `json:"event_type"`
	StatusFrom       string     `json:"status_from"`
	StatusTo         string     `json:"status_to"`
	EarliestNotAfter *time.Time `json:"earliest_not_after,omitempty"`
	OccurredAt       time.Time  `json:"occurred_at"`
}

const resultColumns = `check_id, namespace, endpoint, status, earliest_not_after, horizon_seconds, span_seconds,
	interval_sec, vantages_in_span, vantages_evaluated, vantages_stale, disagreement, alert_state, episode_id,
	opened_at, resolved_at, status_changed_at, evaluated_at, last_observed_at`

func scanResult(row pgx.Row, withVantages bool, now time.Time) (Result, error) {
	var r Result
	var episode *string
	var vantages []byte
	dest := []any{&r.CheckID, &r.Namespace, &r.Endpoint, &r.Status, &r.EarliestNotAfter, &r.HorizonSeconds,
		&r.SpanSeconds, &r.IntervalSec, &r.VantagesInSpan, &r.VantagesEvaluated, &r.VantagesStale, &r.Disagreement,
		&r.AlertState, &episode, &r.OpenedAt, &r.ResolvedAt, &r.StatusChangedAt, &r.EvaluatedAt, &r.LastObservedAt}
	if withVantages {
		dest = append(dest, &vantages)
	}
	if err := row.Scan(dest...); err != nil {
		return Result{}, err
	}
	if episode != nil {
		r.EpisodeID = *episode
	}
	if withVantages && len(vantages) > 0 {
		if err := json.Unmarshal(vantages, &r.Vantages); err != nil {
			return Result{}, err
		}
	}
	if r.EarliestNotAfter != nil {
		s := int64(r.EarliestNotAfter.Sub(now) / time.Second)
		r.RemainingSeconds = &s
	}
	r.Stale = now.Sub(r.LastObservedAt) > time.Duration(evaluate.StaleAfterIntervals*r.IntervalSec)*time.Second
	return r, nil
}

// List returns the organization's results, optionally for one namespace
// ("" or "all" = every namespace), ordered by urgency then check ID.
func (s *Store) List(ctx context.Context, orgID, namespace string, now time.Time) ([]Result, error) {
	if orgID == "" {
		return nil, ErrInput
	}
	q := `SELECT ` + resultColumns + ` FROM f61_tls_results WHERE org_id = $1`
	args := []any{orgID}
	if namespace != "" && namespace != "all" {
		q += ` AND namespace = $2`
		args = append(args, namespace)
	}
	q += ` ORDER BY CASE status WHEN 'expired' THEN 0 WHEN 'expiring' THEN 1 WHEN 'hostname_mismatch' THEN 2
		WHEN 'unknown' THEN 3 WHEN 'unreachable' THEN 4 ELSE 5 END, earliest_not_after NULLS LAST, check_id LIMIT 1000`
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		r, err := scanResult(rows, false, now)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get returns one check's result with its per-vantage view. A check of
// another organization is ErrNotFound, exactly like a missing one.
func (s *Store) Get(ctx context.Context, orgID, checkID string, now time.Time) (Result, error) {
	if orgID == "" || checkID == "" {
		return Result{}, ErrNotFound
	}
	r, err := scanResult(s.db.QueryRow(ctx, `SELECT `+resultColumns+`, vantages FROM f61_tls_results
		WHERE org_id = $1 AND check_id = $2`, orgID, checkID), true, now)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	return r, err
}

// Events returns up to limit outbox events of the organization with id >
// after, oldest first.
func (s *Store) Events(ctx context.Context, orgID string, after int64, limit int) ([]Event, error) {
	if orgID == "" {
		return nil, ErrInput
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `SELECT id, namespace, check_id, episode_id, event_type, status_from, status_to,
		earliest_not_after, occurred_at FROM f61_tls_result_events WHERE org_id = $1 AND id > $2 ORDER BY id LIMIT $3`,
		orgID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Namespace, &e.CheckID, &e.EpisodeID, &e.Type, &e.StatusFrom, &e.StatusTo,
			&e.EarliestNotAfter, &e.OccurredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RevokedBefore returns the revocation bound for a vantage of an
// organization, and whether one exists (PROPOSED REVOKE-1). A credential
// issued before the bound is refused.
func (s *Store) RevokedBefore(ctx context.Context, orgID, vantageID string) (time.Time, bool, error) {
	var nb pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `SELECT not_before FROM f61_vantage_revocations WHERE org_id = $1 AND vantage_id = $2`,
		orgID, vantageID).Scan(&nb)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return fromTimestamptz(nb), true, nil
}

// Revoke records that credentials of the vantage issued before notBefore are
// refused. A later bound replaces an earlier one; an earlier one never
// replaces a later one, so a revocation cannot be undone by accident.
func (s *Store) Revoke(ctx context.Context, orgID, vantageID string, notBefore time.Time, decommission bool, reason, actor string) (time.Time, error) {
	if orgID == "" || vantageID == "" || len(reason) > 256 {
		return time.Time{}, ErrInput
	}
	var bound any = notBefore.UTC()
	if decommission {
		bound = "infinity"
	}
	var nb pgtype.Timestamptz
	err := s.db.QueryRow(ctx, `
		INSERT INTO f61_vantage_revocations (org_id, vantage_id, not_before, reason, revoked_by, revoked_at)
		VALUES ($1, $2, $3::timestamptz, $4, $5, NOW())
		ON CONFLICT (org_id, vantage_id) DO UPDATE SET
		  not_before = GREATEST(f61_vantage_revocations.not_before, EXCLUDED.not_before),
		  reason = EXCLUDED.reason, revoked_by = EXCLUDED.revoked_by, revoked_at = NOW()
		RETURNING not_before`, orgID, vantageID, bound, reason, actor).Scan(&nb)
	if err != nil {
		return time.Time{}, err
	}
	return fromTimestamptz(nb), nil
}

// Decommissioned is the bound returned for a vantage revoked with
// not_before = 'infinity': every credential is before it.
var Decommissioned = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

func fromTimestamptz(v pgtype.Timestamptz) time.Time {
	switch v.InfinityModifier {
	case pgtype.Infinity:
		return Decommissioned
	case pgtype.NegativeInfinity:
		return time.Time{}
	}
	return v.Time.UTC()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
