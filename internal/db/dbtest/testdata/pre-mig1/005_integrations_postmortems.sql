-- ═══════════════════════════════════════════════════════════════════════════════
-- Migration 005 — Integrations, Postmortems, Network flows, Alert groups
-- ═══════════════════════════════════════════════════════════════════════════════

-- ── Integrations ──────────────────────────────────────────────────────────────
-- Stores encrypted integration configs (Slack, PagerDuty, OpsGenie, etc.)

CREATE TABLE IF NOT EXISTS integrations (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id      TEXT NOT NULL,
    type        TEXT NOT NULL,          -- slack|pagerduty|opsgenie|teams|aws|gcp|github|jira|otlp
    -- Config is encrypted with AES-256-GCM before storage
    -- Never store plaintext credentials
    config_enc  BYTEA NOT NULL,         -- encrypted JSON config blob
    config_iv   BYTEA NOT NULL,         -- AES-GCM initialization vector
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    last_tested_at  TIMESTAMPTZ,
    last_test_ok    BOOLEAN,
    last_test_msg   TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_integrations_org_type UNIQUE (org_id, type)
);

CREATE INDEX IF NOT EXISTS idx_integrations_org ON integrations(org_id, is_active);

-- ── Postmortems ───────────────────────────────────────────────────────────────

CREATE TYPE postmortem_status AS ENUM ('draft', 'review', 'published');

CREATE TABLE IF NOT EXISTS postmortems (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id          TEXT NOT NULL,
    incident_id     TEXT NOT NULL DEFAULT '',  -- links to problems table
    title           TEXT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'HIGH',
    status          postmortem_status NOT NULL DEFAULT 'draft',
    detected_at     TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    duration_min    INTEGER NOT NULL DEFAULT 0,
    impact          TEXT NOT NULL DEFAULT '',  -- customer impact description
    summary         TEXT NOT NULL DEFAULT '',  -- executive summary
    root_cause      TEXT NOT NULL DEFAULT '',
    timeline        JSONB NOT NULL DEFAULT '[]',
    -- 5-whys: [{level:1, why:"..."}, ...]
    whys            JSONB NOT NULL DEFAULT '[]',
    -- action items: [{id, what, who, by, done}]
    action_items    JSONB NOT NULL DEFAULT '[]',
    learnings       TEXT NOT NULL DEFAULT '',
    author_id       TEXT NOT NULL,
    reviewer_id     TEXT,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_postmortems_org    ON postmortems(org_id, status);
CREATE INDEX IF NOT EXISTS idx_postmortems_incident ON postmortems(incident_id);

-- ── Alert inhibition rules ────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS inhibit_rules (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id          TEXT NOT NULL,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    -- JSON label matchers: [{label:"class", op:"=", value:"NODE_NOT_READY"}]
    source_matchers JSONB NOT NULL DEFAULT '[]',
    target_matchers JSONB NOT NULL DEFAULT '[]',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inhibit_rules_org ON inhibit_rules(org_id, is_active);

-- ── Network flow snapshots ────────────────────────────────────────────────────
-- Periodic snapshots of inter-service network flows for trend analysis

CREATE TABLE IF NOT EXISTS network_flows (
    id              BIGSERIAL PRIMARY KEY,
    captured_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    src_service_id  TEXT NOT NULL,
    dst_service_id  TEXT NOT NULL,
    namespace       TEXT NOT NULL DEFAULT '',
    protocol        TEXT NOT NULL DEFAULT 'TCP',
    bytes_per_sec   DOUBLE PRECISION NOT NULL DEFAULT 0,
    packets_per_sec DOUBLE PRECISION NOT NULL DEFAULT 0,
    latency_ms      DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    established     INTEGER NOT NULL DEFAULT 0
) PARTITION BY RANGE (captured_at);

-- Create initial partition (current month)
CREATE TABLE IF NOT EXISTS network_flows_current
    PARTITION OF network_flows
    FOR VALUES FROM (DATE_TRUNC('month', NOW()))
           TO (DATE_TRUNC('month', NOW()) + INTERVAL '1 month');

CREATE INDEX IF NOT EXISTS idx_nf_captured ON network_flows(captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_nf_src_dst  ON network_flows(src_service_id, dst_service_id, captured_at DESC);

-- ── APM transaction stats ─────────────────────────────────────────────────────
-- Aggregated per-endpoint stats for APM page (updated every 5 minutes)

CREATE TABLE IF NOT EXISTS apm_transaction_stats (
    id              BIGSERIAL PRIMARY KEY,
    window_start    TIMESTAMPTZ NOT NULL,
    window_end      TIMESTAMPTZ NOT NULL,
    service_id      TEXT NOT NULL,
    service_name    TEXT NOT NULL,
    endpoint        TEXT NOT NULL,
    rpm             DOUBLE PRECISION NOT NULL DEFAULT 0,
    p50_ms          DOUBLE PRECISION NOT NULL DEFAULT 0,
    p99_ms          DOUBLE PRECISION NOT NULL DEFAULT 0,
    error_pct       DOUBLE PRECISION NOT NULL DEFAULT 0,
    apdex           DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    sample_count    BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_apm_window ON apm_transaction_stats(window_start DESC, service_name);

COMMENT ON TABLE integrations IS 'Encrypted integration configs for Slack, PagerDuty, AWS, etc.';
COMMENT ON TABLE postmortems IS 'Structured post-incident reviews with 5-whys and action items';
COMMENT ON TABLE network_flows IS 'Periodic inter-service network flow snapshots from eBPF/proc';
COMMENT ON TABLE apm_transaction_stats IS 'Aggregated per-endpoint APM metrics (5-min windows)';
