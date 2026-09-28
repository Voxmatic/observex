-- ════════════════════════════════════════════════════════════════
--  ObserveX PostgreSQL — Migration 002
--  Full multi-tenancy: organisations, team members, namespace
--  permissions, invitations, and password reset tokens.
--
--  This migration EXTENDS 001 — it does not replace it.
--  All new tables and columns are additive.
-- ════════════════════════════════════════════════════════════════

-- ── Extensions (idempotent) ───────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "citext";      -- case-insensitive text for emails

-- ════════════════════════════════════════════════════════════════
--  ORGANISATIONS
--  Top-level tenant container. Every team, user, dashboard, SLO,
--  and alert rule belongs to an org. This enables true multi-tenancy
--  where org A cannot see org B's data.
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS orgs (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    name           TEXT NOT NULL UNIQUE,
    display_name   TEXT NOT NULL DEFAULT '',
    -- billing / tier info
    plan           TEXT NOT NULL DEFAULT 'free'
                   CHECK (plan IN ('free', 'pro', 'enterprise')),
    -- limits (0 = unlimited for enterprise)
    max_users      INT  NOT NULL DEFAULT 5,
    max_teams      INT  NOT NULL DEFAULT 3,
    max_dashboards INT  NOT NULL DEFAULT 20,
    -- ownership
    owner_id       TEXT NOT NULL,               -- references users.id (set post-create)
    -- branding / settings stored as JSONB for flexibility
    settings       JSONB NOT NULL DEFAULT '{}',
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orgs_name     ON orgs(name);
CREATE INDEX IF NOT EXISTS idx_orgs_owner_id ON orgs(owner_id);

-- ── Add org_id to existing tables ────────────────────────────────────────────
-- Adds org scoping to all tenant data tables. NULL = belongs to the default org.
-- In a new install, migration 001 creates the default org and backfills these.

DO $$ BEGIN
    -- teams
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='teams' AND column_name='org_id') THEN
        ALTER TABLE teams ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_teams_org_id ON teams(org_id);
    END IF;

    -- users
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='users' AND column_name='org_id') THEN
        ALTER TABLE users ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);
    END IF;

    -- dashboards
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='dashboards' AND column_name='org_id') THEN
        ALTER TABLE dashboards ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_dashboards_org_id ON dashboards(org_id);
    END IF;

    -- slos
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='slos' AND column_name='org_id') THEN
        ALTER TABLE slos ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_slos_org_id ON slos(org_id);
    END IF;

    -- alert_rules
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='alert_rules' AND column_name='org_id') THEN
        ALTER TABLE alert_rules ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_alert_rules_org_id ON alert_rules(org_id);
    END IF;

    -- api_keys
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='api_keys' AND column_name='org_id') THEN
        ALTER TABLE api_keys ADD COLUMN org_id TEXT REFERENCES orgs(id) ON DELETE CASCADE;
        CREATE INDEX IF NOT EXISTS idx_api_keys_org_id ON api_keys(org_id);
    END IF;
END $$;

-- ── Default organisation (for migration 001 data) ─────────────────────────────
INSERT INTO orgs (id, name, display_name, plan, max_users, max_dashboards, owner_id)
VALUES (
    'org-default',
    'default',
    'Default Organisation',
    'enterprise',   -- no limits on the default org
    0, 0,
    'usr-admin-default'
) ON CONFLICT (id) DO NOTHING;

-- Backfill org_id on all existing rows created by migration 001
UPDATE teams       SET org_id = 'org-default' WHERE org_id IS NULL;
UPDATE users       SET org_id = 'org-default' WHERE org_id IS NULL;
UPDATE dashboards  SET org_id = 'org-default' WHERE org_id IS NULL;
UPDATE slos        SET org_id = 'org-default' WHERE org_id IS NULL;
UPDATE alert_rules SET org_id = 'org-default' WHERE org_id IS NULL;
UPDATE api_keys    SET org_id = 'org-default' WHERE org_id IS NULL;

-- ════════════════════════════════════════════════════════════════
--  TEAM MEMBERS  (many-to-many: users ↔ teams)
--  Replaces the single team_id FK on users with a proper junction.
--  A user can belong to multiple teams within the same org.
--  The junction also carries a team-level role (can override org role).
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS team_members (
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- team role may be lower than user's org role (principle of least privilege)
    role       TEXT NOT NULL DEFAULT 'viewer'
               CHECK (role IN ('admin', 'editor', 'viewer')),
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id);
CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members(team_id);

-- Migrate existing team_id FK data into the junction table
INSERT INTO team_members (team_id, user_id, role)
SELECT u.team_id, u.id, u.role
FROM users u
WHERE u.team_id IS NOT NULL
ON CONFLICT (team_id, user_id) DO NOTHING;

-- ════════════════════════════════════════════════════════════════
--  NAMESPACE PERMISSIONS
--  Fine-grained access control: which teams (and their users) can
--  read/write resources in a given Kubernetes namespace.
--
--  Rules:
--   - org admins implicitly have access to all namespaces in their org
--   - team editors/viewers can only access namespaces granted to their team
--   - access_level: 'read' | 'write' | 'admin'
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS namespace_permissions (
    id           TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id       TEXT NOT NULL REFERENCES orgs(id)   ON DELETE CASCADE,
    team_id      TEXT NOT NULL REFERENCES teams(id)  ON DELETE CASCADE,
    namespace    TEXT NOT NULL,                       -- K8s namespace name
    access_level TEXT NOT NULL DEFAULT 'read'
                 CHECK (access_level IN ('read', 'write', 'admin')),
    granted_by   TEXT NOT NULL REFERENCES users(id)  ON DELETE SET DEFAULT,
    granted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, team_id, namespace)
);

CREATE INDEX IF NOT EXISTS idx_ns_perms_org_team   ON namespace_permissions(org_id, team_id);
CREATE INDEX IF NOT EXISTS idx_ns_perms_namespace  ON namespace_permissions(org_id, namespace);

-- ════════════════════════════════════════════════════════════════
--  INVITATIONS
--  Org admins can invite new users or existing users to join a team.
--  Tokens expire after 7 days and are single-use.
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS invitations (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id         TEXT NOT NULL REFERENCES orgs(id)   ON DELETE CASCADE,
    team_id        TEXT          REFERENCES teams(id)  ON DELETE SET NULL,
    email          TEXT NOT NULL,                       -- invited email
    role           TEXT NOT NULL DEFAULT 'viewer'
                   CHECK (role IN ('admin', 'editor', 'viewer')),
    token_hash     TEXT NOT NULL UNIQUE,                -- SHA-256 of the invite token
    invited_by     TEXT NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
    accepted_at    TIMESTAMPTZ,
    accepted_by    TEXT          REFERENCES users(id)  ON DELETE SET NULL,
    expires_at     TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '7 days',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_invitations_email      ON invitations(email);
CREATE INDEX IF NOT EXISTS idx_invitations_token_hash ON invitations(token_hash);
CREATE INDEX IF NOT EXISTS idx_invitations_org_id     ON invitations(org_id);
CREATE INDEX IF NOT EXISTS idx_invitations_expires_at ON invitations(expires_at);

-- ════════════════════════════════════════════════════════════════
--  PASSWORD RESET TOKENS
--  Secure single-use tokens for the "forgot password" flow.
--  Tokens expire after 1 hour and are invalidated after use.
-- ════════════════════════════════════════════════════════════════
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,               -- SHA-256 of the reset token
    expires_at  TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '1 hour',
    used_at     TIMESTAMPTZ,                        -- NULL = not used yet
    ip_address  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pwd_reset_user_id    ON password_reset_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_pwd_reset_token_hash ON password_reset_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_pwd_reset_expires_at ON password_reset_tokens(expires_at);

-- ════════════════════════════════════════════════════════════════
--  ENHANCED AUDIT LOG
--  Adds org_id, resource_kind, and before/after JSONB snapshot
--  to support compliance-grade audit trails.
-- ════════════════════════════════════════════════════════════════
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='audit_log' AND column_name='org_id') THEN
        ALTER TABLE audit_log ADD COLUMN org_id TEXT DEFAULT 'org-default';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='audit_log' AND column_name='before_state') THEN
        ALTER TABLE audit_log ADD COLUMN before_state JSONB DEFAULT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='audit_log' AND column_name='after_state') THEN
        ALTER TABLE audit_log ADD COLUMN after_state JSONB DEFAULT NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_name='audit_log' AND column_name='user_agent') THEN
        ALTER TABLE audit_log ADD COLUMN user_agent TEXT DEFAULT '';
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_audit_log_org_id ON audit_log(org_id);

-- ════════════════════════════════════════════════════════════════
--  UPDATED_AT triggers for new tables
-- ════════════════════════════════════════════════════════════════
DO $$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['orgs'] LOOP
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
