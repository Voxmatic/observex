-- ════════════════════════════════════════════════════════════════
--  ObserveX PostgreSQL Schema
--  migrations/009_f61_tls_certificates.sql
--
--  F6.1 TLS certificate expiry: per-vantage observations, per-check results,
--  a transactional outbox of result transitions, and per-vantage credential
--  revocation. PROPOSED DEST-1 / REVOKE-1 (increment-3 decision record).
--
--  Idempotent: every statement can be re-applied safely.
--  Number 008 is intentionally left free for the A7/F-006 reconciliation.
--
--  Tenancy: every table carries org_id. Observation and result rows are tied
--  to their check by (check_id, org_id), so a row can never claim an
--  organization other than the check's; deleting a check deletes its rows.
-- ════════════════════════════════════════════════════════════════

-- (id, org_id) is trivially unique because id is the primary key; the index
-- exists only so that F6.1 rows can reference both columns.
CREATE UNIQUE INDEX IF NOT EXISTS uq_synthetic_checks_id_org ON synthetic_checks(id, org_id);

-- Latest accepted observation per (check, vantage). The payload is the
-- canonical tlscert wire document (v1); vantage identity is NOT in it and is
-- supplied from vantage_kind/vantage_id, which the processor took from the
-- verified credential.
CREATE TABLE IF NOT EXISTS f61_tls_observations (
    check_id       TEXT        NOT NULL,
    org_id         TEXT        NOT NULL,
    vantage_id     TEXT        NOT NULL,
    vantage_kind   TEXT        NOT NULL,
    observed_at    TIMESTAMPTZ NOT NULL,
    received_at    TIMESTAMPTZ NOT NULL,
    outcome        TEXT        NOT NULL,
    leaf_sha256    TEXT        NOT NULL DEFAULT '',
    leaf_not_after TIMESTAMPTZ,
    wire_version   SMALLINT    NOT NULL,
    payload        BYTEA       NOT NULL CHECK (octet_length(payload) <= 262144),
    PRIMARY KEY (check_id, vantage_id),
    FOREIGN KEY (check_id, org_id) REFERENCES synthetic_checks(id, org_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_f61_tls_observations_org ON f61_tls_observations(org_id, check_id);

-- One authoritative result per check.
CREATE TABLE IF NOT EXISTS f61_tls_results (
    check_id           TEXT        PRIMARY KEY,
    org_id             TEXT        NOT NULL,
    namespace          TEXT        NOT NULL,
    endpoint           TEXT        NOT NULL,
    status             TEXT        NOT NULL
                       CHECK (status IN ('expired','expiring','hostname_mismatch','ok','unreachable','unknown')),
    earliest_not_after TIMESTAMPTZ,
    horizon_seconds    BIGINT      NOT NULL CHECK (horizon_seconds > 0),
    span_seconds       BIGINT      NOT NULL CHECK (span_seconds > 0),
    interval_sec       INTEGER     NOT NULL,
    vantages_in_span   INTEGER     NOT NULL DEFAULT 0,
    vantages_evaluated INTEGER     NOT NULL DEFAULT 0,
    vantages_stale     INTEGER     NOT NULL DEFAULT 0,
    disagreement       BOOLEAN     NOT NULL DEFAULT FALSE,
    vantages           JSONB       NOT NULL DEFAULT '[]',
    alert_state        TEXT        NOT NULL DEFAULT 'none' CHECK (alert_state IN ('none','open')),
    episode_id         TEXT,
    episode_peak       TEXT        CHECK (episode_peak IS NULL OR episode_peak IN ('expiring','expired')),
    opened_at          TIMESTAMPTZ,
    resolved_at        TIMESTAMPTZ,
    status_changed_at  TIMESTAMPTZ NOT NULL,
    evaluated_at       TIMESTAMPTZ NOT NULL,
    last_observed_at   TIMESTAMPTZ NOT NULL,
    version            BIGINT      NOT NULL DEFAULT 1,
    CHECK ((alert_state = 'open') = (episode_id IS NOT NULL AND opened_at IS NOT NULL AND episode_peak IS NOT NULL)),
    FOREIGN KEY (check_id, org_id) REFERENCES synthetic_checks(id, org_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_f61_tls_results_org ON f61_tls_results(org_id, namespace, status);

-- Transactional outbox: written in the same transaction as the result change
-- it describes, read by cursor (id). At-least-once for any consumer.
CREATE TABLE IF NOT EXISTS f61_tls_result_events (
    id                 BIGSERIAL   PRIMARY KEY,
    org_id             TEXT        NOT NULL,
    namespace          TEXT        NOT NULL,
    check_id           TEXT        NOT NULL,
    episode_id         TEXT        NOT NULL,
    event_type         TEXT        NOT NULL CHECK (event_type IN ('opened','escalated','resolved')),
    status_from        TEXT        NOT NULL,
    status_to          TEXT        NOT NULL,
    earliest_not_after TIMESTAMPTZ,
    occurred_at        TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_f61_tls_result_events_org ON f61_tls_result_events(org_id, id);
CREATE INDEX IF NOT EXISTS idx_f61_tls_result_events_time ON f61_tls_result_events(occurred_at);

-- Per-vantage credential revocation: a probe credential whose issued-at is
-- before not_before is refused. not_before = 'infinity' decommissions the
-- vantage. Keyed by organization: a vantage ID is MAC-bound to one org.
CREATE TABLE IF NOT EXISTS f61_vantage_revocations (
    org_id     TEXT        NOT NULL,
    vantage_id TEXT        NOT NULL,
    not_before TIMESTAMPTZ NOT NULL,
    reason     TEXT        NOT NULL DEFAULT '' CHECK (length(reason) <= 256),
    revoked_by TEXT        NOT NULL DEFAULT '',
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, vantage_id)
);
