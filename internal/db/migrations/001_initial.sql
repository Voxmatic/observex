-- ════════════════════════════════════════════════════════════════
--  ObserveX PostgreSQL Schema
--  migrations/001_initial.sql
--
--  Run order: 001 → 002 → ...
--  The db/store/migrator.go reads these files in order and runs
--  each one inside a transaction. Already-applied migrations are
--  tracked in the schema_migrations table.
-- ════════════════════════════════════════════════════════════════

-- ── Migration tracking ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Extensions ────────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "pgcrypto";    -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";     -- trigram indexes for ILIKE search

-- ════════════════════════════════════════════════════════════════
--  TEAMS
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS teams (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    namespaces  TEXT[] NOT NULL DEFAULT '{}',  -- K8s namespaces this team owns
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_teams_name ON teams(name);

-- ════════════════════════════════════════════════════════════════
--  USERS
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    email         TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    password_hash TEXT NOT NULL,                -- bcrypt, cost 12
    role          TEXT NOT NULL DEFAULT 'viewer'
                  CHECK (role IN ('admin', 'editor', 'viewer')),
    team_id       TEXT REFERENCES teams(id) ON DELETE SET NULL,
    avatar_url    TEXT NOT NULL DEFAULT '',
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_users_email    ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_team_id  ON users(team_id);
CREATE INDEX IF NOT EXISTS idx_users_role     ON users(role);

-- Default admin user: admin@observex.io / observex
-- Password is bcrypt of "observex" with cost 12.
-- CHANGE THIS IN PRODUCTION.
INSERT INTO users (id, email, name, password_hash, role, is_active)
VALUES (
    'usr-admin-default',
    'admin@observex.io',
    'Admin',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBP36xl.5RRnTe',  -- "observex"
    'admin',
    TRUE
) ON CONFLICT (email) DO NOTHING;

-- ════════════════════════════════════════════════════════════════
--  SESSIONS
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS sessions (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       TEXT NOT NULL UNIQUE,           -- JWT token string (for revocation lookup)
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ip_address  TEXT NOT NULL DEFAULT '',
    user_agent  TEXT NOT NULL DEFAULT '',
    revoked     BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token      ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- ════════════════════════════════════════════════════════════════
--  API KEYS
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    key_hash     TEXT NOT NULL UNIQUE,          -- SHA-256(raw_key)
    key_prefix   TEXT NOT NULL,                 -- first 8 chars, shown in UI
    role         TEXT NOT NULL DEFAULT 'viewer'
                 CHECK (role IN ('admin', 'editor', 'viewer')),
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked      BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user_id  ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);

-- ════════════════════════════════════════════════════════════════
--  DASHBOARDS
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS dashboards (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    owner_id        TEXT NOT NULL REFERENCES users(id) ON DELETE SET DEFAULT,
    team_id         TEXT REFERENCES teams(id) ON DELETE SET NULL,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    widgets_json    JSONB NOT NULL DEFAULT '[]',        -- array of widget definitions
    variables_json  JSONB NOT NULL DEFAULT '[]',        -- dashboard variables
    time_range      TEXT NOT NULL DEFAULT '1h',
    auto_refresh_sec INT NOT NULL DEFAULT 0,
    is_template     BOOLEAN NOT NULL DEFAULT FALSE,
    is_public       BOOLEAN NOT NULL DEFAULT FALSE,     -- visible to all users
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dashboards_owner_id   ON dashboards(owner_id);
CREATE INDEX IF NOT EXISTS idx_dashboards_team_id    ON dashboards(team_id);
CREATE INDEX IF NOT EXISTS idx_dashboards_is_template ON dashboards(is_template) WHERE is_template = TRUE;
CREATE INDEX IF NOT EXISTS idx_dashboards_tags       ON dashboards USING gin(tags);
-- Full-text search on name and description
CREATE INDEX IF NOT EXISTS idx_dashboards_search ON dashboards
    USING gin(to_tsvector('english', name || ' ' || description));

-- Seed built-in dashboard templates
INSERT INTO dashboards (id, name, description, owner_id, is_template, is_public, tags, widgets_json)
VALUES
(
    'tpl-k8s-overview',
    'Kubernetes overview',
    'Cluster health, pods, nodes, namespaces',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['kubernetes','infra'],
    '[{"id":"w1","type":"stat","title":"Total pods","query":"count(kube_pod_info)","data_source":"metrics"},{"id":"w2","type":"stat","title":"Running pods","query":"count(kube_pod_status_phase{phase=\"Running\"})","data_source":"metrics"},{"id":"w3","type":"timeseries","title":"Node CPU %","query":"(1-avg(rate(node_cpu_idle[5m]))by(node))*100","data_source":"metrics"},{"id":"w4","type":"topology","title":"Cluster topology","data_source":"topology"}]'::JSONB
),
(
    'tpl-service-health',
    'Service health',
    'HTTP error rates, latency, throughput',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['http','apm'],
    '[{"id":"w1","type":"timeseries","title":"Request rate","query":"sum(rate(http_requests_total[1m]))by(service_name)","data_source":"metrics"},{"id":"w2","type":"timeseries","title":"Error rate %","query":"sum(rate(http_requests_total{status_code=~\"5..\"}[5m]))by(service_name)/sum(rate(http_requests_total[5m]))by(service_name)*100","data_source":"metrics"}]'::JSONB
),
(
    'tpl-ai-agent',
    'AI agent activity',
    'Auto-remediations, incidents, audit log',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['ai','remediations'],
    '[{"id":"w1","type":"alertlist","title":"Active problems","data_source":"problems"},{"id":"w2","type":"table","title":"Recent remediations","data_source":"remediations"}]'::JSONB
),
(
    'tpl-slo',
    'SLO dashboard',
    'Error budgets, burn rates, SLI trends',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['slo','reliability'],
    '[{"id":"w1","type":"slo","title":"All SLOs","data_source":"slos"},{"id":"w2","type":"timeseries","title":"Error budget burn rate","data_source":"slos"}]'::JSONB
),
(
    'tpl-queue',
    'Queue monitor',
    'Kafka / RabbitMQ consumer lag, throughput',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['kafka','rabbitmq'],
    '[{"id":"w1","type":"stat","title":"Max consumer lag","query":"max(kafka_consumer_group_lag)","data_source":"metrics"},{"id":"w2","type":"timeseries","title":"Consumer lag by group","query":"kafka_consumer_group_lag","data_source":"metrics"}]'::JSONB
),
(
    'tpl-database',
    'Database health',
    'Query latency, connections, slow queries',
    'usr-admin-default',
    TRUE, TRUE,
    ARRAY['postgres','database'],
    '[{"id":"w1","type":"stat","title":"Active connections","query":"pg_stat_activity_count","data_source":"metrics"},{"id":"w2","type":"timeseries","title":"Avg query time (ms)","query":"rate(pg_stat_statements_total_time_seconds[5m])/rate(pg_stat_statements_calls[5m])*1000","data_source":"metrics"}]'::JSONB
)
ON CONFLICT (id) DO NOTHING;

-- ════════════════════════════════════════════════════════════════
--  SLOs
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS slos (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    service_name TEXT NOT NULL,
    namespace    TEXT NOT NULL DEFAULT '',
    team_id      TEXT REFERENCES teams(id) ON DELETE SET NULL,
    kind         TEXT NOT NULL
                 CHECK (kind IN ('availability', 'latency', 'error_rate', 'throughput')),
    target       DOUBLE PRECISION NOT NULL
                 CHECK (target > 0 AND target <= 100),
    -- "window" is a reserved word in PostgreSQL and must be quoted. The
    -- unquoted form could not be applied on any supported PostgreSQL version,
    -- so no existing database contains it; the column name is unchanged.
    "window"     TEXT NOT NULL DEFAULT '30d'
                 CHECK ("window" IN ('7d', '30d', '90d')),
    sli_query    TEXT NOT NULL DEFAULT '',      -- PromQL to compute SLI value
    good_query   TEXT NOT NULL DEFAULT '',      -- optional: good events numerator
    total_query  TEXT NOT NULL DEFAULT '',      -- optional: total events denominator
    created_by   TEXT NOT NULL REFERENCES users(id) ON DELETE SET DEFAULT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_slos_service_name ON slos(service_name);
CREATE INDEX IF NOT EXISTS idx_slos_namespace    ON slos(namespace);
CREATE INDEX IF NOT EXISTS idx_slos_team_id      ON slos(team_id);

-- ════════════════════════════════════════════════════════════════
--  ALERT RULES
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS alert_rules (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    service_name TEXT NOT NULL DEFAULT '',
    namespace    TEXT NOT NULL DEFAULT '',
    team_id      TEXT REFERENCES teams(id) ON DELETE SET NULL,
    expr         TEXT NOT NULL,                -- PromQL expression
    threshold    DOUBLE PRECISION NOT NULL,
    operator     TEXT NOT NULL DEFAULT 'gt'
                 CHECK (operator IN ('gt', 'lt', 'gte', 'lte', 'eq')),
    duration     TEXT NOT NULL DEFAULT '5m',
    severity     TEXT NOT NULL DEFAULT 'MEDIUM'
                 CHECK (severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'LOW')),
    message      TEXT NOT NULL DEFAULT '',
    labels_json  JSONB NOT NULL DEFAULT '{}',
    channels     TEXT[] NOT NULL DEFAULT '{}',
    silenced     BOOLEAN NOT NULL DEFAULT FALSE,
    created_by   TEXT NOT NULL REFERENCES users(id) ON DELETE SET DEFAULT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_service ON alert_rules(service_name);
CREATE INDEX IF NOT EXISTS idx_alert_rules_ns      ON alert_rules(namespace);
CREATE INDEX IF NOT EXISTS idx_alert_rules_severity ON alert_rules(severity);
CREATE INDEX IF NOT EXISTS idx_alert_rules_silenced ON alert_rules(silenced);

-- ════════════════════════════════════════════════════════════════
--  AUDIT LOG
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS audit_log (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id      TEXT NOT NULL,                 -- NOT a FK — preserve log even if user deleted
    user_email   TEXT NOT NULL,
    action       TEXT NOT NULL,                 -- create, update, delete, login, logout, api_key_create
    resource     TEXT NOT NULL,                 -- dashboard, slo, alert_rule, user, team, session
    resource_id  TEXT NOT NULL DEFAULT '',
    ip_address   TEXT NOT NULL DEFAULT '',
    details      JSONB NOT NULL DEFAULT '{}',   -- before/after values, error info
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Partitioned by month to keep queries fast over large volumes
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id    ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_resource   ON audit_log(resource, resource_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at DESC);

-- ════════════════════════════════════════════════════════════════
--  UPDATED_AT triggers
--  Auto-updates the updated_at column on every UPDATE.
-- ════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['teams','users','dashboards','slos','alert_rules'] LOOP
        EXECUTE format(
            'DROP TRIGGER IF EXISTS trg_%s_updated_at ON %s;
             CREATE TRIGGER trg_%s_updated_at
             BEFORE UPDATE ON %s
             FOR EACH ROW EXECUTE FUNCTION set_updated_at();',
            t, t, t, t
        );
    END LOOP;
END;
$$;
