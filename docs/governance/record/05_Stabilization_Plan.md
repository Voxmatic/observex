# 05 — ObserveX Proposed Stabilization Plan

**Scope:** Stage 1 (critical containment) plus the first Stage 2 cleanup items. The plan is ordered so each item is small, independently revertible and testable.

**Item status labels**

| Status | Meaning |
|---|---|
| ✅ **Implemented & verified** | Done in this session and validated here |
| 🟡 **Ready — awaiting approval** | Fully specified; changes behavior, defaults or schema |
| 🟠 **Ready — needs Go toolchain** | Low-risk and needs no decision, but I can't compile or test Go in this environment, so I haven't changed Go code |

> **Why I haven't modified Go code:** Go module downloads (`proxy.golang.org`) are blocked by this environment's egress policy, so I can't compile or test Go changes here. Shipping untested Go edits to a security-critical auth path would break the rule that every change must include tests. To unblock me, either allow `proxy.golang.org` and `sum.golang.org` in your network egress settings, or run the validation commands below in your environment and share the output.

---

## S0 — Implemented in this session (✅)

Delivered as `observex-stabilization-S0.patch` (apply with `git apply`; verified with `git apply --check`). It touches 2 files and doesn't change any runtime behavior of backend services.

### S0-01 · RUM SDK test suite never exits (F-103)
- **Change:** Test harness only. It tracks `setInterval` handles created during the run and clears them in an `after()` hook. **The SDK source is unchanged.**
- **File:** `sdk/rum/observex-rum.test.js`
- **Risk:** None to production. The worst case is a test-only regression.
- **Validation (run here):**
  - Before: `timeout 60 node --test observex-rum.test.js` → killed at 60 s (exit 124).
  - After: exits in 0.3 s, `# tests 28 · # pass 28 · # fail 0`.
- **Rollback:** Revert the patch hunk.

### S0-02 · Password-change API client contract mismatch (F-006-adjacent, C3)
- **Change:** `auth.changePassword` now sends `old_password` (the backend contract, `internal/db/models/models.go:69-72`) instead of `current_password`. **The backend API is unchanged**, so no other consumer is affected.
- **File:** `frontend/src/lib/api.ts`
- **Risk:** Very low. No UI screen calls this function today (`grep` shows only the definition).
- **Validation (run here):** `tsc --noEmit` passes; `npm run build` succeeds.
- **Rollback:** Revert the patch hunk.
- **Note:** A contract test (baseline A2) should replace this manual check in Stage 2.

---

## Stage 1 pack — proposed changes

For each item:
1. Proposed change
2. Affected files/components
3. Risks
4. Validation steps
5. Approval needed

### S1-01 · SAML containment (F-001) — 🟡 approval (pack)
1. In `handleSAMLCallbackSecure`:
   - Reject with 403 when `RelayState` is empty.
   - Reject with 403 when no row exists with `type='saml' AND enabled=true`.
   - Reject with 403 when `idp_cert` is empty. Remove the "allow without cert" branch.
   - Look up users by `(email, org_id)`, and refuse login when the user's org differs from the SAML org.
   - Stop returning the `detail` verification error to clients.
2. `services/api-gateway/main.go` (≈4284-4366, 4000-4031)
3. **Risk:** Anyone currently relying on cert-less SAML "setup mode" loses it. Setup must configure the cert first. The digest/wrapping weakness remains until Stage 3 (D-06), so SSO should also be disabled by default (S1-02).
4. Validation:
   - Go unit tests with fixture SAML responses: unsigned response with no cert → 403; empty RelayState → 403; valid-org, cross-org user → 403.
   - `go vet`, `go test ./services/api-gateway/...`.
   - Manual curl proof of the original exploit returning 403.
5. Pack approval.

### S1-02 · SSO feature flag, default off (F-001, F-014) — 🟡 approval
1. `SSO_ENABLED` env (default `false`). All `/api/auth/sso/*` routes return `404 {"error":"sso disabled"}` when it's off.
2. `services/api-gateway/main.go` route registration (≈376-382 region of `registerRoutes`)
3. **Risk:** Any org using SSO today loses it. Given F-006 (SSO config can't be saved) and F-014, working SSO deployments are unlikely, but **please confirm there are none**.
4. Validation: route test with the flag off/on.
5. **Approval required.**

### S1-03 · Remediation safe defaults and internal auth (F-002, F-011) — 🟡 approval
1. Go `ai-agent`:
   - `DRY_RUN` defaults to `true` unless `DRY_RUN=false` is set explicitly.
   - Enforce `RequireApproval`: planned actions in that list get status `pending_approval` and don't execute.
   - `ALLOWED_NAMESPACES` empty → deny all (instead of allow all).
   - Require `X-Internal-Token` (constant-time compare, fail-closed if unset) on every route except `/health`.
   - Approve/reject return `501 Not Implemented` instead of fake success.

   Python agent:
   - Default mode `suggest`.
   - Token required on `/mode`, `/decisions/*/approve|reject`, `/signals`, `/chat`.

   Gateway: forward `X-Internal-Token` on proxied calls.
2. `services/ai-agent/main.go`, `ai-agent-python/src/server.py`, `ai-agent-python/src/agent.py` (default), `services/api-gateway/main.go` (`proxy`), processor → agent client, Helm/Compose env
3. **Risk:** Remediation stops acting automatically, which is intentional. Anyone depending on auto-actions must opt in explicitly. The processor must be configured with the token or problem delivery fails (visible as 403 in logs).
4. Validation:
   - Go tests: default config → dry-run; empty allowlist → deny; missing token → 403; rollback class → pending.
   - pytest: default mode suggest; unauthenticated `/mode` → 401.
   - Compose smoke: processor delivers a problem with the token.
5. **Approval required** (changes default product behavior).

### S1-04 · Query engine raw SQL removal and CORS (F-003, F-017) — 🟡 approval (pack)
1. Remove the `"events","sql"` case in `handleUnifiedQuery` and ignore the `sql` field in `handleEventsQuery` (the built-in bounded query stays). Set CORS from `ALLOWED_ORIGINS` with no wildcard default. Require `X-Internal-Token`.
2. `services/query-engine/main.go` (458-470, 524-526, 556)
3. **Risk:** Verified that neither the frontend nor the gateway calls these endpoints. External scripts using them (unknown) would break.
4. Validation: tests confirming `type:"sql"` → 400 and a `sql` body is ignored; `go vet`/`go test` in the module.
5. Pack approval.

### S1-05 · Fail fast on weak or default secrets (F-005) — 🟡 approval
1. At startup (gateway, ingestor, activegate), refuse to start if `JWT_SECRET` or `AGENT_TOKEN_SECRET` is empty, shorter than 32 bytes, or equal to a known default string. Refuse if `INTEGRATION_ENC_KEY` isn't 32 bytes. Refuse if `AGENT_TOKEN_SECRET == JWT_SECRET`. Keep a `OBSERVEX_DEV_INSECURE=true` escape hatch that logs a loud warning (dev only). Remove Helm inline secret defaults so `required` works. Update Compose to generate secrets through an `.env` template.
2. `services/api-gateway/main.go` (loadConfig, encKey), `services/ingestor/main.go` (loadConfig), `deployments/helm/observex/values.yaml`, `templates/secrets.yaml`, `docker-compose.yml`, `.env.example`, README
3. **Risk:** Existing dev and prod installs with default secrets won't start after upgrade, which is intentional. **Rotating `INTEGRATION_ENC_KEY` makes existing encrypted integration configs unreadable.** Installs that ran on the zero key need a one-time re-encryption migration (included: decrypt with zero key, encrypt with new key, only if `REENCRYPT_FROM_ZERO_KEY=true`).
4. Validation: config unit tests (table-driven); `helm template` fails without secrets; Compose starts with generated `.env`.
5. **Approval required.**

### S1-06 · Authorization gaps and fail-closed internal middleware (F-010, F-011, F-047) — 🟡 approval (pack)
1.
   - `InternalOnly()` denies when the token is unset.
   - Postmortem writes → `RequireEditor`; integrations create/delete/test → `RequireAdmin`; `DELETE /problems/:id/comments/:cid` → `RequireEditor` plus author-or-admin check; `/llm/ingest` → `RequireEditor` and ignore body `OrgID`; `/remediations` and `/agents` → `RequireEditor`.
   - `/ws` requires a valid session/JWT on upgrade and sets `orgID` from auth.
2. `internal/middleware/middleware.go`, `services/api-gateway/main.go` (routes 484-640, `handleLLMIngest`, `handleWebSocket`)
3. **Risk:** Viewers lose write abilities they shouldn't have had. The frontend may need to hide those buttons (cosmetic; the API returns 403).
4. Validation: a route-level authorization test table (viewer/editor/admin × route → expected status); WS upgrade without a token → 401.
5. Pack approval.

### S1-07 · Ingest body limits and org header trust (F-015, F-004 partial) — 🟡 approval (limit value)
1. Cap decompressed body at `INGEST_MAX_BODY_BYTES` (default 16 MiB **[A]**) → 413. Set the Fiber `BodyLimit` explicitly. Stop honoring `X-ObserveX-Org` for static tokens (use the configured `OBSERVEX_ORG_ID`).
2. `services/ingestor/main.go` (205-221, 238-243, `fiber.New`)
3. **Risk:** Agents sending batches larger than the limit get 413. **Need confirmation of maximum agent batch size.** Multi-org use of one static token stops working, which is intentional.
4. Validation: gzip bomb fixture → 413 with bounded memory (test asserts reader limit); static token plus a forged header → org forced to the configured value.
5. **Approval required for the limit value.**

### S1-08 · ClickHouse insert escaping (F-012) — 🟠 needs Go toolchain
1. Replace `esc()` with an escaper that handles `\` then `'` (ClickHouse string literal rules), and add unit tests with injection payloads. Full parameterized/`JSONEachRow` inserts come in Stage 3.
2. `services/ingestor/main.go:1041-1043`
3. **Risk:** Very low. Output only differs for strings containing a backslash, which were previously corrupted.
4. Validation: unit tests (`a\'b`, `\\`, newline, unicode); integration insert against ClickHouse in CI.
5. No product approval needed (pure bug fix). **Blocked only on toolchain.**

### S1-09 · Rollback revision selection (F-043) — 🟠 needs Go toolchain
1. Parse `deployment.kubernetes.io/revision` as an integer; choose the highest revision lower than current; skip unparsable values.
2. `services/ai-agent/main.go:515-533` (extract to a pure function `selectPreviousRevision`)
3. **Risk:** Very low. It only changes which ReplicaSet is chosen, and S1-03 makes it dry-run by default anyway.
4. Validation: table test with revisions `["9","10","11"]`, current `11` → `10` (currently returns `9`).
5. No product approval needed.

### S1-10 · Kubernetes least privilege and default network policy (F-002, F-018 partial) — 🟡 approval
1.
   - Create per-component ServiceAccounts (only `ai-agent` bound to the remediation ClusterRole; `oneagent` bound to its read-only role; all others get no RBAC).
   - Remove `pods/exec` and `nodes` update from the AI role. The node patch stays only if `cordon_node` stays enabled (default off).
   - Set `networkPolicy.enabled: true`.
   - Drop `SYS_ADMIN`/`hostIPC` from the default oneagent profile unless `oneagent.ebpf.enabled=true`.
2. `deployments/helm/observex/templates/{serviceaccount,rbac,deployments,daemonset,networkpolicy}.yaml`, `values.yaml`
3. **Risk:** Clusters without a NetworkPolicy-capable CNI ignore the policies (no harm). Clusters with one may block traffic not covered by the existing policy rules, so **staging validation is required**. The existing `networkpolicy.yaml` rules haven't been exercised.
4. Validation: `helm lint`, `helm template | kubeconform`, install on kind with Calico, run connectivity smoke tests, and `kubectl auth can-i` checks per SA.
5. **Approval required.**

### S1-11 · Postgres schema drift reconciliation (F-006, F-051) — 🟡 approval (schema; D-03)
1.
   - Add `internal/db/migrations/008_reconcile.sql` (idempotent, forward-only):
     - create `incidents`, `service_catalog`, `agent_knowledge`, `agent_playbooks` with `org_id`
     - `ALTER TABLE sessions ADD COLUMN IF NOT EXISTS org_id`
     - column set for `sso_configs` as chosen in D-03
   - Change store SQL to canonical names (`namespace_permissions`, `password_reset_tokens`).
   - Incidents handlers use `auth.OrgID` and filter by org.
   - Stop discarding errors in `AuthContextStore.Build` and the session and reset stores; log and return them.
   - Hash reset tokens (F-022).
2. `internal/db/migrations/008_reconcile.sql` (new), `internal/db/store/stores.go`, `services/api-gateway/main.go` (incident handlers), `advanced_features.go`, `ai_monitoring_agent.go` (only if names change)
3. **Risk:**
   - Schema change on existing databases. Mitigated because it's additive only (no drops or renames of existing tables). Take a backup before applying.
   - **Column definitions for the four new tables are inferred from the code that queries them**, so review is needed.
   - Surfacing previously swallowed errors may make hidden failures visible as 500s, which is intentional and should be tested in staging.
4. Validation:
   - New CI job: start Postgres, apply 001–008, and `PREPARE` every SQL statement extracted from the store and handlers (schema contract test).
   - Unit tests for incidents org scoping.
   - Run 008 twice (idempotence).
5. **Approval required** (schema + D-03).

### S1-12 · Replace seeded admin with one-time bootstrap (F-005, C1) — 🟡 approval
1.
   - Migration `009` disables the seeded `admin@observex.io` account (`is_active=false`) **only if its password hash still equals the seeded hash**, so installs that changed it are untouched.
   - The gateway, on startup with zero active admins, creates one from `BOOTSTRAP_ADMIN_EMAIL` plus `BOOTSTRAP_ADMIN_PASSWORD`, or generates a random password and prints it once to logs, with forced password change on first login.
   - README updated.
2. `internal/db/migrations/009_bootstrap_admin.sql` (new), `services/api-gateway/main.go` (startup), README, Compose/Helm env
3. **Risk:** Anyone who knows the seeded password (unknown to us, C1) loses access. That's intentional. Installs that already use that account without changing the password must bootstrap a new admin.
4. Validation: integration test on an empty DB → bootstrap admin created once; restart → not recreated; seeded hash untouched → disabled.
5. **Approval required.**

---

## Stage 2 early items (proposed, after Stage 1)

| ID | Item | Status | Notes |
|---|---|---|---|
| S2-01 | Rewrite CI for the actual layout (Go root module + `services/query-engine`, `services/db-monitor`, `services/observex-agent` modules; frontend; Python; RUM SDK; Helm lint) | 🟠 toolchain | No product approval needed |
| S2-02 | Restore canonical module paths in `go.mod`; Go 1.23+ toolchain in Dockerfiles and CI | 🟡 approval (build change) | Needs an environment with proxy access |
| S2-03 | **Deletions (D-22):** root binaries `activegate`, `ai-agent`, `api-gateway`, `ingestor`, `processor`, `trivy-scanner`; `services/query-engine/query-engine`; `services/db-monitor/db-monitor`; `services/oneagent/oneagent`; `services/observex-agent/oneagent/oneagent`; directory `services/observex-agent/oneagent/` (identical copy of `services/oneagent/`); `services/observex-agent/main.go.bak`; committed `frontend/dist/` | 🟡 approval | Archive branch first |
| S2-04 | `gofmt -w` mechanical commit | 🟠 toolchain | Formatting only |
| S2-05 | ESLint config at warn level | Ready (can do here) | Holding: adds devDependency config files; waiting for pack approval so it lands with CI |
| S2-06 | `db-monitor` Dockerfile `CGO_ENABLED=0`; non-root query-engine image | 🟠 needs Docker | |
| S2-07 | Dependency upgrades (starlette/fastapi, requests, axios, lodash, form-data, react-router; Go libs after govulncheck) | 🟡 approval | Some are major-version bumps and need regression testing |

---

## Execution protocol once approved
1. One PR per item (S1-01 … S1-12), each with tests, a changelog entry and a rollback note.
2. Order:
   - S1-05, S1-03, S1-01/02, S1-04, S1-06 (closes exploit paths)
   - S1-07, S1-08, S1-09 (bug fixes)
   - S1-11, S1-12 (schema)
   - S1-10 (infra, staging first)
3. After each PR: `go build ./... && go vet ./... && go test ./...` (per module), `pytest`, `node --test`, `tsc`, `npm run build`. Record results in a Stage 1 log alongside the pre-existing baseline failures, kept separate from new ones.
4. Stage 1 exit review with you before Stage 2 starts.
