-- internal/db/migrations/003_comments_runbooks.sql
--
-- Adds two collaboration features:
--   1. runbook_url on alert_rules — links each alert to its runbook
--   2. incident_comments — threaded comments on Problem IDs
--
-- Both changes are backward-compatible and additive-only.
-- The runbook column is nullable so existing rules are unaffected.

-- ── 1. Runbook URL on alert rules ─────────────────────────────────────────────
-- Each alert rule can link to a runbook (Confluence, Notion, GitHub Wiki, etc.)
-- displayed when the alert fires so on-call engineers have immediate context.

DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'alert_rules' AND column_name = 'runbook_url'
    ) THEN
        ALTER TABLE alert_rules ADD COLUMN runbook_url TEXT NOT NULL DEFAULT '';
        COMMENT ON COLUMN alert_rules.runbook_url IS
            'URL of the runbook describing how to investigate and resolve this alert. '
            'Shown in Slack/PagerDuty notifications and the UI alert panel.';
    END IF;
END $$;

-- ── 2. Incident comments ───────────────────────────────────────────────────────
-- Threaded comments on any Problem. The problem_id is a logical identifier
-- (not a FK, because problems live in the processor's in-memory store).
-- Comments are scoped to the org for multi-tenancy.

CREATE TABLE IF NOT EXISTS incident_comments (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id      TEXT NOT NULL,
    problem_id  TEXT NOT NULL,          -- matches Problem.ID in the processor
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE SET DEFAULT,
    content     TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 10000),
    -- Editing: track last edit for audit trail
    edited      BOOLEAN NOT NULL DEFAULT FALSE,
    edited_at   TIMESTAMPTZ,
    -- Soft delete: keep for audit, hide from UI
    deleted     BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_incident_comments_problem
    ON incident_comments(org_id, problem_id)
    WHERE NOT deleted;

CREATE INDEX IF NOT EXISTS idx_incident_comments_user
    ON incident_comments(user_id);

-- updated_at trigger
DO $$
BEGIN
    DROP TRIGGER IF EXISTS trg_incident_comments_updated_at ON incident_comments;
    CREATE TRIGGER trg_incident_comments_updated_at
        BEFORE UPDATE ON incident_comments
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
END $$;
