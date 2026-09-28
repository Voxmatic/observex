-- internal/db/migrations/003_deployments.sql
--
-- Deployment markers table.
-- Records every service deployment with version information and
-- stores the regression-analysis results once they are computed.
--
-- Apply with:
--   psql $DATABASE_URL -f 003_deployments.sql

-- ── Deployments ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS deployments (
    id              TEXT PRIMARY KEY,         -- UUID
    service_name    TEXT        NOT NULL,
    service_id      TEXT        NOT NULL DEFAULT '',
    namespace       TEXT        NOT NULL DEFAULT 'default',
    cluster_name    TEXT        NOT NULL DEFAULT 'default',
    version         TEXT        NOT NULL,
    prev_version    TEXT        NOT NULL DEFAULT '',
    deployed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deployed_by     TEXT        NOT NULL DEFAULT '',
    environment     TEXT        NOT NULL DEFAULT 'production',
    notes           TEXT        NOT NULL DEFAULT '',

    -- Regression analysis results
    status          TEXT        NOT NULL DEFAULT 'pending',   -- pending|ok|regression|improved
    analysed_at     TIMESTAMPTZ,

    -- Before/after P99 latency (ms)
    p99_before      DOUBLE PRECISION NOT NULL DEFAULT 0,
    p99_after       DOUBLE PRECISION NOT NULL DEFAULT 0,
    p99_delta_pct   DOUBLE PRECISION NOT NULL DEFAULT 0,      -- positive = worse

    -- Before/after error rate (%)
    err_rate_before DOUBLE PRECISION NOT NULL DEFAULT 0,
    err_rate_after  DOUBLE PRECISION NOT NULL DEFAULT 0,
    err_rate_delta_pct DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Link to a regression Problem (if one was created)
    problem_id      TEXT        NOT NULL DEFAULT '',

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Fast look-up by service + time for the timeline page
CREATE INDEX IF NOT EXISTS idx_deployments_service
    ON deployments(service_name, deployed_at DESC);

CREATE INDEX IF NOT EXISTS idx_deployments_status
    ON deployments(status)
    WHERE status IN ('pending', 'regression');

CREATE INDEX IF NOT EXISTS idx_deployments_deployed_at
    ON deployments(deployed_at DESC);

-- Auto-update updated_at
DO $$
BEGIN
    EXECUTE 'DROP TRIGGER IF EXISTS trg_deployments_updated_at ON deployments;
             CREATE TRIGGER trg_deployments_updated_at
             BEFORE UPDATE ON deployments
             FOR EACH ROW EXECUTE FUNCTION set_updated_at();';
END;
$$;

-- ── On-call schedules & escalation policies ───────────────────────────────────
-- Allows routing alerts to the right person at the right time.

CREATE TABLE IF NOT EXISTS oncall_schedules (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id      TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    timezone    TEXT NOT NULL DEFAULT 'UTC',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS oncall_rotations (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    schedule_id  TEXT NOT NULL REFERENCES oncall_schedules(id) ON DELETE CASCADE,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_time   TIMESTAMPTZ NOT NULL,
    end_time     TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_oncall_rotations_schedule ON oncall_rotations(schedule_id);
CREATE INDEX IF NOT EXISTS idx_oncall_rotations_active   ON oncall_rotations(schedule_id, start_time, end_time);

CREATE TABLE IF NOT EXISTS escalation_policies (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id      TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    -- JSON: [{"delay_min":0,"targets":["user:id1","schedule:id2"]},{"delay_min":15,...}]
    steps       JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Alert routing rules (which escalation policy handles which alerts)
CREATE TABLE IF NOT EXISTS alert_routes (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id      TEXT NOT NULL,
    -- Matchers: JSON array of {label, op, value} — e.g. [{label:"severity",op:"=",value:"CRITICAL"}]
    matchers    JSONB NOT NULL DEFAULT '[]',
    policy_id   TEXT NOT NULL REFERENCES escalation_policies(id) ON DELETE CASCADE,
    -- Inhibition: silence child alerts when parent fires
    inhibit_if  TEXT NOT NULL DEFAULT '', -- problem class that inhibits this route
    group_by    TEXT[] NOT NULL DEFAULT '{}', -- labels to group alerts by
    group_wait  INTERVAL NOT NULL DEFAULT '30 seconds',
    group_interval INTERVAL NOT NULL DEFAULT '5 minutes',
    repeat_interval INTERVAL NOT NULL DEFAULT '4 hours',
    priority    INT NOT NULL DEFAULT 0, -- higher = evaluated first
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_routes_org ON alert_routes(org_id, priority DESC);
