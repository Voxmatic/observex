-- Generic durable state for feature modules that are not yet large enough
-- for dedicated relational tables. Each row is org-scoped and kind-scoped.

CREATE TABLE IF NOT EXISTS feature_state (
    org_id      TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    id          TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, kind, id)
);

CREATE INDEX IF NOT EXISTS idx_feature_state_kind
    ON feature_state(org_id, kind, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_feature_state_payload_gin
    ON feature_state USING GIN (payload);

DO $$
BEGIN
    DROP TRIGGER IF EXISTS trg_feature_state_updated_at ON feature_state;
    CREATE TRIGGER trg_feature_state_updated_at
        BEFORE UPDATE ON feature_state
        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
END $$;
