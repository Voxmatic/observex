-- ═══════════════════════════════════════════════════════════════════════════
-- Migration 006 — SSO Providers & Multi-cluster K8s
-- ═══════════════════════════════════════════════════════════════════════════

-- ── SSO / SAML / OIDC provider configurations ─────────────────────────────

CREATE TABLE IF NOT EXISTS sso_configs (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id          TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type            TEXT NOT NULL CHECK (type IN ('saml','oidc','github','google','azure')),

    -- Common
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    auto_provision  BOOLEAN NOT NULL DEFAULT TRUE,
    default_role    TEXT NOT NULL DEFAULT 'viewer' CHECK (default_role IN ('viewer','editor','admin')),

    -- Attribute mapping
    email_attr      TEXT NOT NULL DEFAULT 'email',
    name_attr       TEXT NOT NULL DEFAULT 'displayName',
    role_attr       TEXT NOT NULL DEFAULT '',           -- optional IdP group → role mapping

    -- SAML 2.0 fields
    idp_entity_id   TEXT NOT NULL DEFAULT '',
    idp_sso_url     TEXT NOT NULL DEFAULT '',
    idp_cert        TEXT NOT NULL DEFAULT '',          -- PEM X.509 certificate from IdP
    sp_entity_id    TEXT NOT NULL DEFAULT '',          -- our SP entity ID

    -- OIDC fields
    issuer          TEXT NOT NULL DEFAULT '',          -- OIDC discovery URL
    client_id       TEXT NOT NULL DEFAULT '',
    client_secret   BYTEA,                             -- encrypted with AES-256-GCM
    client_secret_iv BYTEA,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_sso_org_type UNIQUE (org_id, type)
);

CREATE INDEX IF NOT EXISTS idx_sso_configs_org ON sso_configs(org_id, enabled);

-- ── Multi-cluster Kubernetes registry ──────────────────────────────────────

CREATE TABLE IF NOT EXISTS k8s_clusters (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id          TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,                     -- used as cluster label in metrics
    display_name    TEXT NOT NULL DEFAULT '',
    region          TEXT NOT NULL DEFAULT '',
    provider        TEXT NOT NULL DEFAULT 'vanilla'
                        CHECK (provider IN ('eks','gke','aks','k3s','vanilla','openshift','rke2')),
    api_server_url  TEXT NOT NULL DEFAULT '',
    -- kubeconfig stored encrypted — never returned in list/get responses
    kubeconfig_enc  BYTEA,
    kubeconfig_iv   BYTEA,

    -- Live status (updated by K8s agent heartbeat)
    status          TEXT NOT NULL DEFAULT 'unknown'
                        CHECK (status IN ('active','unreachable','unknown','no_agent')),
    agent_version   TEXT NOT NULL DEFAULT '',
    node_count      INTEGER NOT NULL DEFAULT 0,
    pod_count       INTEGER NOT NULL DEFAULT 0,
    last_seen_at    TIMESTAMPTZ,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_cluster_org_name UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_k8s_clusters_org    ON k8s_clusters(org_id, status);
CREATE INDEX IF NOT EXISTS idx_k8s_clusters_status ON k8s_clusters(status, last_seen_at);

-- ── Function: update cluster heartbeat from agent ──────────────────────────
-- Called when the K8s agent posts a heartbeat via /v1/agents/:id/heartbeat
-- The agent sends cluster_name in its registration payload.

CREATE OR REPLACE FUNCTION update_cluster_heartbeat(
    p_org_id      TEXT,
    p_cluster_name TEXT,
    p_node_count  INTEGER,
    p_pod_count   INTEGER,
    p_version     TEXT
) RETURNS VOID AS $$
BEGIN
    UPDATE k8s_clusters
    SET    status       = 'active',
           node_count   = p_node_count,
           pod_count    = p_pod_count,
           agent_version = p_version,
           last_seen_at = NOW(),
           updated_at   = NOW()
    WHERE  org_id = p_org_id
    AND    name   = p_cluster_name;
END;
$$ LANGUAGE plpgsql;

-- ── Synthetic monitoring checks ────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS synthetic_checks (
    id                   TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    org_id               TEXT NOT NULL,
    name                 TEXT NOT NULL,
    type                 TEXT NOT NULL DEFAULT 'http'
                             CHECK (type IN ('http','tcp','dns','ssl','ping','grpc','websocket')),
    target               TEXT NOT NULL,
    interval_sec         INTEGER NOT NULL DEFAULT 60
                             CHECK (interval_sec BETWEEN 10 AND 3600),
    timeout_sec          INTEGER NOT NULL DEFAULT 10
                             CHECK (timeout_sec BETWEEN 1 AND 60),
    locations            TEXT[]  NOT NULL DEFAULT '{local}',
    enabled              BOOLEAN NOT NULL DEFAULT TRUE,
    expect_status        INTEGER NOT NULL DEFAULT 200,
    expect_body_contains TEXT    NOT NULL DEFAULT '',
    headers              JSONB   NOT NULL DEFAULT '{}',
    namespace            TEXT    NOT NULL DEFAULT 'default',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_synthetic_checks_org ON synthetic_checks(org_id, enabled);

-- ── Tier 3: SSO group mappings ─────────────────────────────────────────────────
-- Stores IdP group → ObserveX role + team mappings per SSO config.
ALTER TABLE sso_configs
  ADD COLUMN IF NOT EXISTS group_mappings JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS group_attr     TEXT  NOT NULL DEFAULT 'groups';

-- Status page config per org (slug, enabled flag, service list)
ALTER TABLE orgs
  ADD COLUMN IF NOT EXISTS status_page_config JSONB NOT NULL DEFAULT '{}'::jsonb;
