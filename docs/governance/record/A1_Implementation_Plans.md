# A1 — Implementation Plans for Approved Items

**Decision:** A1, Approved with conditions (see `07_Approval_Log.md`)
**Items:** S1-01 · S1-04 · S1-06 · S1-08 · S1-09
**Status:** **PLANS ONLY. No code has been changed.** Each plan needs your confirmation before I write code.

> ⚠️ **Not production-ready.** Per condition C11, none of these fixes may be called production-ready until Go compilation, `go vet`, tests and security validation have passed, and staging validation (C10) is complete.

---

## 0. Plan-level questions that block or shape implementation

I found these while planning. I haven't assumed answers. Each has a recommendation, but **none is decided**.

| ID | Question | Why it matters | Options | Recommendation | Status |
|---|---|---|---|---|---|
| **P-0** | **How will Go code be compiled and tested?** (This is decision A9, but A1 can't meet C11 without it.) | Go module downloads are blocked here (`proxy.golang.org` → 403). Code written without compiling would be unvalidated. | (a) Allow `proxy.golang.org` and `sum.golang.org` in this environment's egress settings. (b) I write the code and tests; you run the validation script in your environment or CI and send me the output. (c) Both. | **(a)** is fastest, and I can iterate on failures. (b) works but adds a round trip per failure. | **Blocked — your answer needed** |
| **P-1** | Internal service-token naming (condition C9) | Today the code uses `INTERNAL_TOKEN` + `X-Internal-Token` (gateway middleware, processor). No Helm, Compose or docs set it. | (a) Keep `INTERNAL_TOKEN` / `X-Internal-Token`. (b) Namespaced: env `OBSERVEX_INTERNAL_TOKEN`, file variant `OBSERVEX_INTERNAL_TOKEN_FILE`, header `X-ObserveX-Internal-Token`, Helm secret key `internal-token`. | **(b).** It matches existing `OBSERVEX_INGEST_TOKEN*` naming, avoids collisions with generic env vars, and breaks nothing because no deployment sets the old name. | Pending |
| **P-2** | How clients present credentials on the WebSocket (`/ws`) | Browsers can't set an `Authorization` header on WebSocket connections. No frontend code uses `/ws` today (verified by grep). | (a) `Authorization: Bearer` header only (non-browser clients). (b) (a) + `Sec-WebSocket-Protocol: observex.bearer.<token>` for browsers. (c) `?token=` query parameter. | **(b).** (c) leaks tokens into logs and proxies. D-09 later moves browsers to cookies. | Pending |
| **P-3** | S1-06 scope: only the routes named in the audit, or **every** write route missing a role check? | Planning found **~31 non-GET routes with no role check**, not just the 9 in the audit. Examples: `POST /llm/regions/failover`, `POST /llm/models/:id/rollback`, `PUT /llm/models/:id/traffic`, notebooks create/update/execute, `POST /pipeline/rules/:id/test`, `POST /ai-agent/inhouse-scan`, `POST /dora/deployments`, `POST /errors/events`. Full list in §4.2. | (a) Only the audited routes. (b) All write routes default to Editor (Admin where listed), with a short documented allowlist of read-only POSTs that viewers keep, enforced by a router-enumeration test. | **(b)** is what C4 (fail-closed) implies. | Pending |
| **P-4** | S1-08 scope: escaping only, or also fix how ClickHouse writes are sent? | Planning found that the ingestor sends SQL **unencoded in the URL** (`clickhouseQueryErr`, `ingestor/main.go:959-975`). I reproduced this with Go's HTTP server: any query containing a space produces an invalid request line → **400**; `#` truncates the query. So ingestor writes to ClickHouse (metrics, services, edges, profiles) **likely fail today, silently** (logged at Debug). *Likely: Go server tested, not ClickHouse itself.* | (a) Escaping only (as approved). (b) Escaping + send SQL in the POST body, as a **separate follow-up item S1-08b** needing its own approval. | **(a) now, and decide (b) separately.** Fixing transport would make data start landing in ClickHouse for the first time. That changes load, storage and cost, which goes beyond what A1 approved. | Pending |
| **P-5** | S1-09: also strip the `pod-template-hash` label when copying the old template? | The rollback copies the old ReplicaSet's template, including the `pod-template-hash` label. `kubectl rollout undo` removes that label. *Likely correctness issue; not tested on a cluster.* | (a) Revision fix only. (b) Revision fix + strip the label, matching kubectl. | **(b)**, same function and same PR. It isn't a default change, and remediation stays governed by A3 later. | Pending |
| **P-6** | Protect the ingestor's `/internal/*` routes with the internal token in S1-06? | `/internal/services` and `/internal/agents` are unauthenticated today (`ingestor/main.go:226`). The gateway's `/api/v1/agents` is their only consumer. | (a) Leave for later. (b) Include in S1-06; the gateway forwards the token. | **(b).** It's the same fail-closed fix and has one consumer. | Pending |
| **P-7** | Staging environment (condition C10) | Validation must happen in staging before production. | Tell me: does a staging environment exist? Is it Compose or Kubernetes? Who deploys, and can I get deploy logs or test output from it? | If none exists: a kind-based ephemeral staging in CI covers these five items, but **doesn't replace** a production-like staging. | **Pending clarification** |

---

## 1. Shared specification: internal service token (introduced in S1-06, used by S1-04)

*Written with the P-1 recommendation (b). If you pick (a), only the names change.*

| Aspect | Specification |
|---|---|
| Env var | `OBSERVEX_INTERNAL_TOKEN` |
| File variant | `OBSERVEX_INTERNAL_TOKEN_FILE` (path to a mounted secret; takes precedence over the env var) |
| Header | `X-ObserveX-Internal-Token` |
| Minimum length | 32 bytes. Shorter values are treated as **not configured**. |
| Comparison | `crypto/subtle.ConstantTimeCompare` |
| Unset or invalid on a **receiving** service | **Fail-closed:** protected routes return `503 {"error":"internal authentication not configured"}`; one `ERROR` log at startup. `/health` stays open. The service still starts; refusing to start is S1-05 / A4 and isn't approved. |
| Missing or wrong header on a request | `401 {"error":"unauthorized"}`; the token value is never logged |
| Sender behavior | Gateway attaches the header **only** to requests whose host matches configured internal service URLs (`QUERY_ENGINE_URL`, `INGESTOR_URL`), via a host-scoped `http.RoundTripper` on `gw.client`. **It's never sent to external hosts** (Slack, PagerDuty, IdPs). A test covers this. |
| Receivers in A1 | Gateway `InternalOnly()` middleware; query engine (S1-04); ingestor `/internal/*` (if P-6 = b) |
| Senders in A1 | Gateway → query engine, gateway → ingestor `/internal/*`, processor → gateway (header rename only; see O-2) |
| Helm | Secret key `internal-token` in the chart secret, mounted as a file. If not supplied: generated once with `randAlphaNum 48` and preserved using `lookup`. If `lookup` isn't available (GitOps rendering), set `secrets.existingSecret`. Documented. |
| Compose | `OBSERVEX_INTERNAL_TOKEN: ${OBSERVEX_INTERNAL_TOKEN:?set OBSERVEX_INTERNAL_TOKEN in .env}`, so Compose refuses to start without it. `.env.example` shows how to generate one: `openssl rand -hex 32`. |
| Docs (C8) | New `docs/configuration/internal-service-token.md`: purpose, generation, rotation (dual-token window deferred to D-10; for now, rolling restart), troubleshooting (401 vs 503). Plus updates to `.env.example`, Helm `values.yaml` comments and the README configuration section. |

**Rotation limitation (documented, not solved):** one token at a time, so rotation needs a coordinated restart. Dual-token support belongs to D-10.

---

## 2. Test strategy and required regression tests (conditions C6, C7)

### 2.1 Test layers
| Layer | Tooling | Where it runs | Needs |
|---|---|---|---|
| Unit | Go `testing`, Fiber `app.Test()` | `go test ./...` per module | Go toolchain (P-0) |
| Integration | Go tests with build tag `integration`, real Postgres (CI service container or local Docker) and a stub ClickHouse (`httptest`) | `go test -tags=integration ./...` | Postgres 16 |
| Security (automated) | `gosec`, `govulncheck` on changed modules; exploit-reproduction tests in Go | CI | Toolchain |
| Security (staging) | `tests/security/a1/*.sh` curl scripts that reproduce each original exploit; expected **before: exploitable, after: blocked** | Staging (P-7) | Staging URL + test accounts |

### 2.2 Test seams (small, behavior-neutral refactors inside each PR)
- **Middleware (S1-06):** add an `AuthResolver` interface to `middleware.Config` (the default is the existing `AuthContextStore.Build`), so route tests can inject viewer, editor or admin contexts without a database.
- **SAML (S1-01):** extract a pure function `authorizeSAMLLogin(cfg samlOrgConfig, relayState string, user *User) error`. The handler behaves the same except for the approved new checks.
- **Rollback (S1-09):** extract a pure function `selectPreviousReplicaSet(current string, rsList []appsv1.ReplicaSet) (name string, err error)`.
- **Escaping (S1-08):** `esc()` becomes `chString()` with a test-vector table.

### 2.3 Mapping of your required regression tests (C7)
| Required test | Plan | Test names (proposed) | Layer |
|---|---|---|---|
| Cross-organization SAML login | S1-01 | `TestSAMLCallback_UserFromOtherOrg_Rejected` (integration), `TestAuthorizeSAMLLogin_OrgMismatch` (unit) | Integration + unit |
| Missing SAML certificate | S1-01 | `TestSAMLCallback_NoCertConfigured_Rejected`, `TestSAMLCallback_EmptyRelayState_Rejected`, `TestSAMLCallback_SSODisabledForOrg_Rejected` | Integration + unit |
| Missing internal token | S1-06, S1-04 | `TestInternalOnly_TokenUnset_503`, `TestInternalOnly_HeaderMissing_401`, `TestInternalOnly_WrongToken_401`, `TestQueryEngine_NoToken_401`, `TestGatewayClient_DoesNotSendTokenToExternalHosts` | Unit |
| Unauthorized viewer actions | S1-06 | `TestRouteAuthz_Matrix` (table: viewer/editor/admin × each changed route), `TestRouteAuthz_AllWriteRoutesRequireRole` (router enumeration, P-3) | Unit |
| WebSocket authentication | S1-06 | `TestWS_NoCredentials_401`, `TestWS_InvalidToken_401`, `TestWS_ValidToken_Upgrades_OrgScoped`, `TestWSHub_DoesNotDeliverOtherOrgEvents` | Unit/integration |
| SQL injection through metric labels | S1-08 | `TestCHString_Vectors` (quote, backslash, `\'`, NUL, newline, unicode), `TestMetricInsert_LabelInjection_NoBreakout` (builds statement, parses with a literal-boundary checker), `TestServiceInsert_FieldInjection` | Unit |
| Numeric rollback revision selection | S1-09 | `TestSelectPreviousReplicaSet_NumericOrder` (`9,10,11` with current `11` → `10`), `_SkipsNonNumeric`, `_NoPrevious_Error`, `_UnsortedInput` | Unit |

---

## 3. Plan S1-01 — SAML login containment (F-001)

**Branch / PR:** `a1/s1-01-saml-containment` · one commit, or a small series squashed on merge

### 3.1 Scope
**In scope:**
- Reject SAML callbacks when:
  - `RelayState` is empty
  - there's no SAML config row for that org
  - the config isn't enabled
  - `idp_cert` is empty
- Reject when the user matched by email belongs to a **different org** than the SAML config.
- Auto-provisioning stays governed by the existing `auto_provision` flag, but only inside the SAML org.
- Stop returning verification error details to the client; they're logged server-side only.
- Remove the `c.Query("org_id")` fallback. The org comes from `RelayState` only.

**Out of scope (C2):** XML-DSig digest verification, canonicalization, XSW, audience/time/replay checks, signed AuthnRequest, group-sync hardening. All documented in `D-06_SAML_Deferred_Weaknesses.md`. The OIDC flow and the SSO default are also out of scope (S1-02 / A2).

### 3.2 Files
| File | Change |
|---|---|
| `services/api-gateway/main.go` | `handleSAMLCallbackSecure` (≈4284-4366): new guard sequence; remove the "no cert → allow" branch (≈4326-4332); remove the `org_id` query fallback (≈4299-4301); generic 401 body. `ssoLoginOrProvision` (≈4000-4031): for `provider=="saml"`, require `user.OrgID == orgID` for existing users. |
| `services/api-gateway/saml_policy.go` (new) | `samlOrgConfig`, `authorizeSAMLLogin(...)` |
| `services/api-gateway/saml_policy_test.go` (new) | Unit tests |
| `services/api-gateway/saml_callback_integration_test.go` (new, `//go:build integration`) | Handler tests against Postgres with seeded orgs, users, sso_configs; generated RSA test cert; test-signed fixtures |
| `tests/security/a1/s1-01-saml-bypass.sh` (new) | Staging exploit reproduction |
| `CHANGELOG.md` | Entry |

### 3.3 Behavior after the change
| Request | Before | After |
|---|---|---|
| Unsigned SAMLResponse, empty RelayState | JWT issued for any existing email | **401** `{"error":"saml login rejected"}` |
| RelayState = org with no SAML config | JWT issued | **401** |
| RelayState = org with SAML config but `enabled=false` | JWT issued | **401** |
| RelayState = org with enabled config but empty `idp_cert` | JWT issued (warning logged) | **401** |
| Valid signature, user exists in a **different** org | JWT for the other org's account | **403** `{"error":"saml login rejected"}` |
| Valid signature, user in the same org | JWT | JWT (unchanged) |
| Valid signature, unknown user, `auto_provision=true` | User created (org updated after insert) | Unchanged (created in the SAML org) |
| Invalid signature | 401 with verification error detail | 401 with generic body; detail only in server logs |

The rejection body is deliberately the same for all failure reasons, so outsiders can't use it to probe which orgs have SSO configured. The audit and logs record the specific reason.

### 3.4 Tests
- **Unit:** every row in 3.3 via `authorizeSAMLLogin`.
- **Integration (Postgres):** every row in 3.3 through the real handler, plus a **positive control** (a correctly signed test response → JWT) so the lockdown provably doesn't break legitimate SAML.
- **Security:** exploit script against staging; `gosec` on the package.

### 3.5 Risks
| Risk | Likelihood | Mitigation |
|---|---|---|
| An org using SAML without a cert loses SAML login | Low (SSO config can't even be saved via API today, F-006) | Release note; configure the cert before upgrading |
| Existing SSO users stored under `org-default` but logging in via another org's SAML get 403 | Possible | Release note; admin moves users (a manual SQL snippet is documented, not automated) |
| Integration fixtures depend on tables affected by F-006 | Medium | Fixtures only use `orgs`, `users`, `sso_configs`, which exist in migrations |
| Residual XSW/replay risk remains | Certain | Documented (D-06); SSO default-off proposed in A2 |

### 3.6 Validation steps
1. `go build ./... && go vet ./...` (root module)
2. `go test ./services/api-gateway/... -run 'SAML'`
3. `go test -tags=integration ./services/api-gateway/... -run 'SAML'` with Postgres
4. `gosec ./services/api-gateway/...` — no new High findings
5. Staging: run `s1-01-saml-bypass.sh` → all cases blocked; perform one legitimate SAML login if a test IdP exists (otherwise **mark as not validated**)

### 3.7 Rollback
Revert the single PR commit. No schema or data changes. No config changes required to roll back.

---

## 4. Plan S1-06 — Authorization gaps, fail-closed internal auth, WebSocket auth (F-010, F-011, F-047)

**Branch / PR:** `a1/s1-06-authz-fail-closed` · **must merge before S1-04**

### 4.1 Scope
1. **Internal token** per §1: config loader, `InternalOnly()` fail-closed (503 when unset, 401 on mismatch), host-scoped sender transport on `gw.client`, processor header rename.
2. **Route role fixes**, with the final list depending on P-3. The table below assumes P-3 = (b).
3. **WebSocket:** authenticate before upgrade (P-2); set `orgID` from the auth context; subscriptions protected by a mutex; hub drops events whose org differs from the client's (including org-less events, unless the event type is explicitly global).
4. **`handleLLMIngest`:** ignore body `org_id`; always use `auth.OrgID`.
5. **`handleDeleteComment`:** keep author-only deletion (existing SQL); return **404** when no row was deleted, instead of the current silent 204.
6. **Ingestor `/internal/*`** requires the internal token (if P-6 = b).
7. **Docs** (C8): see §1.

**Out of scope:**
- Session revocation (A-later/D-09)
- API keys (F-050)
- The namespace-check bypass on empty namespace (F-010 part 2 depends on F-006 schema fix A7; documented)
- Cross-org user update/delete (O-1, new finding, reported separately)

### 4.2 Route changes (assuming P-3 = b)
| Route | Today | After |
|---|---|---|
| `POST/PUT/DELETE /postmortems[/:id]` | any authenticated user | Editor |
| `POST /integrations`, `DELETE /integrations/:id`, `POST /integrations/:id/test` | any authenticated user | **Admin** |
| `DELETE /problems/:id/comments/:cid` | any authenticated user | Editor + author-only (existing) + 404 if not deleted |
| `POST /llm/ingest` | any authenticated user; body org trusted | Editor; org from auth |
| `GET /remediations` | any authenticated user | Editor |
| `GET /agents` | any authenticated user | Editor |
| `POST /llm/models`, `PUT /llm/models/:id/traffic`, `POST /llm/models/:id/rollback` | any authenticated user | Editor |
| `POST /llm/regions/failover` | any authenticated user | **Admin** |
| `POST /llm/infer/event`, `/llm/gpu/report`, `/llm/safety/event`, `/llm/agents/trace` | any authenticated user | Editor |
| `POST /errors/events`, `POST /dora/deployments` | any authenticated user | Editor |
| `POST /notebooks`, `PUT /notebooks/:id`, `POST /notebooks/:id/cells`, `POST /notebooks/:id/execute` | any authenticated user | Editor |
| `POST /watchdog/anomalies/:id/dismiss`, `POST /pipeline/rules/:id/test`, `POST /logs/search/saved` | any authenticated user | Editor |
| `POST /ai-agent/inhouse-simulate`, `POST /ai-agent/inhouse-scan` | any authenticated user | Editor |
| `GET /ws` | no authentication | Valid token required (P-2) |
| **Viewer-allowed POST allowlist** (read-only semantics, documented in code): `POST /logs/search/nl`, `POST /anomaly/preview`, `POST /apikeys` + `DELETE /apikeys/:id` (own keys), `PUT /users/:id` (self, existing owner check) | unchanged | unchanged; listed explicitly in the enumeration test |

> I'll re-verify this list against the router at implementation time (`app.GetRoutes()`), and the enumeration test fails if a new unlisted write route appears.

### 4.3 Files
| File | Change |
|---|---|
| `internal/middleware/middleware.go` | `AuthResolver` seam; `InternalOnly()` fail-closed; token loader (`OBSERVEX_INTERNAL_TOKEN[_FILE]`, min length) |
| `internal/middleware/internal_token_test.go` (new) | Token tests |
| `internal/servicetoken/transport.go` (new) + test | Host-scoped `RoundTripper` |
| `services/api-gateway/main.go` | Route middleware changes (≈470-640, 4740-4810, 5809-5900 registrars); `gw.client` transport; WS upgrade middleware + `handleWebSocket` org/mutex; hub org filter (≈1895-1905); `handleLLMIngest`; `handleDeleteComment` |
| `internal/db/store/stores.go` | `IncidentCommentStore.Delete` returns an error when 0 rows affected |
| `services/api-gateway/inhouse_agent.go` | Route roles |
| `services/api-gateway/authz_routes_test.go` (new), `ws_auth_test.go` (new) | Matrix, enumeration and WebSocket tests |
| `services/processor/main.go:1871` | Header/env rename (sender only) |
| `services/ingestor/main.go` (P-6) | `/internal/*` token check |
| `deployments/helm/observex/templates/secrets.yaml`, `deployments.yaml`, `values.yaml` | Secret key + env/file mount for gateway, processor, ingestor, query engine |
| `deployments/docker/docker-compose.yml`, `docker-compose.yml`, `.env.example` | Required env var |
| `docs/configuration/internal-service-token.md` (new), `README.md` | Documentation |
| `tests/security/a1/s1-06-authz.sh` (new) | Staging script |

### 4.4 Risks
| Risk | Mitigation |
|---|---|
| Viewers lose actions they could do before; UI still shows buttons → 403 | Intended (C4). Release note lists every changed route. UI button hiding is follow-up work, not included. |
| Internal token not configured → metrics views (via query engine) and `/api/v1/agents` return errors | Fail-closed by design. Compose refuses to start without it; Helm auto-generates it; 503 message is explicit. |
| Token leaks to external hosts | Host-scoped transport + test |
| WebSocket clients (none in repo) break | Documented; no in-repo consumer |
| Processor → gateway `/internal/notify` still fails (route doesn't exist, O-2) | Not introduced by this PR; reported separately |

### 4.5 Validation steps
1. Build, vet and unit tests (root module); `go test -race` on middleware and WS packages
2. Router enumeration test green
3. `helm template` with and without a provided token; `kubeconform`
4. `docker compose config` fails without the var and succeeds with it
5. `gosec`, `govulncheck`
6. Staging:
   - Viewer, editor and admin test accounts run `s1-06-authz.sh` (expected status per route)
   - WS connect without/with token
   - Query-engine-backed page loads with the token set
   - Returns 503 when the token is removed from one service

### 4.6 Rollback
Revert the PR. The env var and Helm secret can stay (they're ignored by older code), so rollback needs no config change.

---

## 5. Plan S1-04 — Remove raw SQL from the query engine (F-003, F-017)

**Branch / PR:** `a1/s1-04-query-engine-sql` · **depends on S1-06** (token spec + gateway sender)

> **Amended 17 Sep 2026 (approval log: S1-04 decisions D1–D5):** `/query/events`, `EventQuery` and the built-in events query are **removed entirely** (C5 amended); routes move into `newApp()`; the stale `services/query-engine/query-engine` binary is deleted and git-ignored. **Not in S1-04:** internal-token authentication, CORS removal and rejected-request logging (follow-ups FU-S104-1/2/3), and `X-ObserveX-Org` escaping (FU-S104-4). The §5.1–5.5 text below is the original plan and is superseded where it conflicts.

### 5.1 Scope
**In scope:**
- **Delete** the `case "events", "sql"` branch in `handleUnifiedQuery` (`query-engine/main.go:524-526`). Unknown types → 400 (existing default).
- `handleEventsQuery` (`:458-470`): if the body contains a non-empty `sql` field → **400** `{"error":"custom SQL is not supported"}`. Otherwise run the **existing built-in bounded query** unchanged, with `limit` clamped to 1..1000 (default 100) as today (C5).
- Remove `SQL` from the `EventQuery` request struct, so it's no longer accepted as an input field.
- **Remove the CORS middleware entirely** (`:556`). The query engine is internal and has no browser consumers. No new env var.
- Require the internal token on every route except `/health` (§1).
- Log a structured warning (no payload) when a `sql`-bearing request is rejected, so any hidden external user is visible.

**Out of scope:**
- Read-only ClickHouse user (infra; proposed under D-04/D-08)
- Log/trace tenancy
- PromQL function support (F-066)

### 5.2 Files
| File | Change |
|---|---|
| `services/query-engine/main.go` | As above; token middleware (local implementation, since this is a separate Go module and can't import the root `internal/` package; identical semantics, shared test vectors copied) |
| `services/query-engine/main_test.go` (new) | Tests |
| `services/query-engine/go.mod` | No new dependencies expected |
| `tests/security/a1/s1-04-raw-sql.sh` (new) | Staging script (run from inside the cluster network) |
| Docs | Query engine section in `internal-service-token.md` |

### 5.3 Tests
| Test | Expectation |
|---|---|
| `POST /query {"type":"sql","query":"SELECT 1"}` | 400; stub ClickHouse receives **nothing** |
| `POST /query {"type":"events","query":"DROP TABLE metrics"}` | 400; nothing received |
| `POST /query/events {"sql":"SELECT * FROM system.users"}` | 400; nothing received |
| `POST /query/events {}` | 200; stub receives exactly the built-in query with `LIMIT 100` |
| `POST /query/events {"limit":5000}` / `{"limit":-1}` | Built-in query with `LIMIT 100` (existing clamp) |
| `POST /query/events {"limit":50}` | `LIMIT 50` |
| Any route without token / wrong token / service token unset | 401 / 401 / 503; `/health` 200 |
| `OPTIONS` preflight from a browser origin | No `Access-Control-Allow-Origin` header |
| Existing metrics routes with a valid token | Behavior unchanged (range/instant/labels/series against the stub) |

### 5.4 Risks
| Risk | Mitigation |
|---|---|
| Unknown external scripts use custom SQL | I verified that neither the frontend nor the gateway does. Rejections are logged. Release note included. |
| Gateway → query engine fails if the token is missing | Fail-closed by design (S1-06 config) |
| `/api/v1/*` compatibility routes also require the token | Intended; the gateway sends it |

### 5.5 Validation steps
1. `cd services/query-engine && go build ./... && go vet ./... && go test ./...`
2. `gosec`, `govulncheck` for the module
3. Staging:
   - `s1-04-raw-sql.sh` from a pod in the cluster (custom SQL rejected; events page still works through the gateway)
   - Metrics page works

### 5.6 Rollback
Revert the PR. **Warning:** reverting re-exposes the SQL endpoint. Only roll back together with a network-level block (NetworkPolicy/firewall) on the query engine.

---

## 6. Plan S1-08 — ClickHouse string escaping in the ingestor (F-012)

**Branch / PR:** `a1/s1-08-clickhouse-escaping` · independent of the others

### 6.1 Scope
**In scope:**
- Replace `esc()` (`ingestor/main.go:1041-1043`) with `chString(s string) string`, which returns a fully quoted ClickHouse string literal:
  - escapes `\` → `\\` **first**, then `'` → `\'`
  - escapes control characters (`\n`, `\r`, `\t`, `\0`)
  - validates UTF-8 by replacing invalid bytes
- Apply it to **all 9 call sites**: metrics (`:706-713`), services (`:924-927`), topology edges (`:936-937`), profiles (`:946-947`).
- Format floats safely: reject NaN and ±Inf. Those points are dropped with a **counter**, instead of producing invalid SQL.

**Out of scope (decide with P-4):**
- Changing the transport so SQL goes in the POST body (the likely write failure, S1-08b)
- Batching and parameterized inserts (Stage 3)

### 6.2 Files
| File | Change |
|---|---|
| `services/ingestor/main.go` | `chString`, call sites, float guard, drop counter |
| `services/ingestor/clickhouse_escape_test.go` (new) | Vector and injection tests |

### 6.3 Tests
- **Vector table:** `a'b` · `a\b` · `a\'b` · `'); DROP TABLE metrics; --` · `\'); DROP TABLE metrics; --` · `\\'` · newline, tab, NUL · emoji/unicode · invalid UTF-8 · empty string.
- **Breakout checker:** a small test-only lexer walks each generated INSERT and asserts that every value literal opens and closes exactly where expected and the statement has exactly the expected number of literals. Payloads in metric name, service ID and **label keys/values** (JSON-marshalled, the injection path you named) must stay inside their literal.
- **Service, edge and profile inserts:** the same checker with injected fields.
- **NaN/Inf:** points dropped, counter incremented.
- *(Optional, if P-0 allows Docker in CI)* integration against a real ClickHouse 24.3 container that inserts payloads through the corrected escaper using a POST-body client **inside the test only**, confirming ClickHouse reads back the exact original strings. This validates the escaping rules without changing production transport.

### 6.4 Risks
| Risk | Mitigation |
|---|---|
| Values containing backslashes are now stored differently | Previously corrupted or broken, so it's an improvement; documented |
| NaN/Inf points now dropped explicitly | Counter + release note; previously they produced invalid SQL |
| Writes may still fail because of the URL transport bug (P-4) | Reported, not hidden; escaping correctness is proven independently |

### 6.5 Validation steps
1. Build, vet and tests (root module)
2. `gosec` on ingestor
3. Optional ClickHouse container test
4. Staging: send an agent payload with injection strings in labels; confirm no ClickHouse errors attributable to SQL syntax. *If P-4(b) isn't approved, writes may still fail for the transport reason. The staging report will distinguish the two.*

### 6.6 Rollback
Revert the PR. No data migration.

---

## 7. Plan S1-09 — Numeric rollback revision selection (F-043)

**Branch / PR:** `a1/s1-09-rollback-revision` · independent

### 7.1 Scope
**In scope:**
- Extract `selectPreviousReplicaSet(currentRevision string, rsList []appsv1.ReplicaSet) (string, error)`:
  - parses revision annotations with `strconv.ParseInt`
  - ignores unparsable or missing annotations (logged)
  - ignores ReplicaSets whose owner isn't this Deployment (`metav1.IsControlledBy`), because a label selector alone can match foreign ReplicaSets
  - returns the **highest revision strictly lower than current**
  - returns a clear error when none qualifies or current is unparsable
- If P-5 = (b): remove the `pod-template-hash` label from the copied template before `Update`, matching `kubectl rollout undo`.

**Out of scope:**
- Approval enforcement and dry-run default (S1-03 / A3)
- Moving rollback to the Kubernetes rollback API or GitOps (D-11)

### 7.2 Files
| File | Change |
|---|---|
| `services/ai-agent/main.go` (≈507-550) | Use the new function; P-5 label strip |
| `services/ai-agent/rollback_test.go` (new) | Tests |

### 7.3 Tests
| Case | Expected |
|---|---|
| Revisions `9, 10, 11`, current `11` | `10` (today returns `9`, the regression test) |
| `1, 2, 10`, current `10` | `2` |
| Unsorted input `11, 9, 10`, current `11` | `10` |
| Current `3`, list contains `5` (newer than current, odd state) | `2` if present, never `5` |
| Non-numeric or missing annotation among valid | Ignored |
| ReplicaSet owned by another Deployment with a higher revision | Ignored |
| Only the current revision exists | Error "no previous revision" |
| Current annotation unparsable | Error |
| (P-5 b) Copied template has no `pod-template-hash` label; other labels preserved | ✓ |
| Integration-style test with `k8s.io/client-go/kubernetes/fake` | `Update` called with the template of the expected ReplicaSet |

### 7.4 Risks
Very low. Only the selection logic changes. Remediation safety defaults are still unchanged until A3, so this fix **doesn't make auto-remediation safe**. It only removes a wrong-version bug.

### 7.5 Validation steps
1. Build, vet and tests (root module)
2. Staging (only if the AI agent runs there with a test namespace): deploy a sample app through 3 revisions, trigger rollback via the agent in **dry-run disabled test namespace only with your explicit permission**, verify revision N-1 is restored. Otherwise **mark this as validated by the unit and fake-client tests only.**

### 7.6 Rollback
Revert the PR.

---

## 8. Execution order, per-PR gates and results reporting

### 8.1 Order
1. **S1-09** (independent, lowest risk; proves the toolchain path)
2. **S1-08** (independent)
3. **S1-06** (defines the internal token)
4. **S1-04** (depends on S1-06)
5. **S1-01** (independent; needs Postgres for integration tests)

### 8.2 Gate for every PR (all must pass before I report it as "validated")
- [ ] `go build ./...` and `go vet ./...` for every touched module
- [ ] `go test ./...` (+ `-race` where concurrency changed) for touched modules
- [ ] Integration tests where listed
- [ ] `gosec` (no new High) and `govulncheck` (no new reachable vulns) for touched modules
- [ ] Pre-existing baseline (frontend build, pytest, RUM tests) re-run to confirm nothing regressed
- [ ] Docs and changelog updated
- [ ] Staging validation per plan (C10), or explicitly marked **"not yet validated in staging"**

### 8.3 What you'll receive for the A1 results review (C13)
- Per PR: diff summary, test output (pass/fail counts), gosec and govulncheck output, open issues, staging status.
- A consolidated **A1 validation report** separating pre-existing failures from new ones.
- An explicit statement of what's **not** validated.
- A2 is presented only after you've reviewed that report.

---

## 9. New observations found during planning (not in A1 scope; nothing implemented)

| ID | Observation | Evidence | Severity (proposed) | Where it should go |
|---|---|---|---|---|
| **O-1** | **Cross-org admin IDOR on users:** `RequireAdmin` checks "admin of own org", but `handleUpdateUser`/`handleDeleteUser` and `UserStore.Update/Delete` have no org filter. An admin of org A can change the email of, or delete, users in org B. | `api-gateway/main.go:988-1010`; `stores.go:89-98` | **Critical** (multi-tenant) | New finding F-007x; recommend adding to Stage 1 as a new approval item |
| **O-2** | Processor sends alert notifications to gateway `/internal/notify`, **a route that doesn't exist**, so alert notifications from the processor likely never deliver | `processor/main.go:1861-1871`; no `/internal/notify` route in gateway | High | Register; relevant to A3/D-11 |
| **O-3** | Ingestor ClickHouse writes are sent unencoded in the URL, so they likely fail (see P-4) | `ingestor/main.go:959-975`; Go reproduction | High | S1-08b (needs approval) |
| **O-4** | Compose creates ClickHouse tables in the `default` database, but the query engine queries `database=observex`; Helm creates `observex` with a **different** table set (`events` etc.) | `deployments/docker/configs/clickhouse-init.sql`; `query-engine/main.go` `/?database=observex`; Helm `configmap.yaml:23-40` | High | D-04 |
| **O-5** | Gateway `writeNativeMetrics` posts to the ingestor without an agent token, so it's rejected when `REQUIRE_AGENT_AUTH` is on and LLM ingest metrics are silently dropped | `api-gateway/main.go:161-185`; `ingestor/main.go:223-235` | Medium | Register |
| **O-6** | Query-engine `quotedSQL` only doubles `'` and doesn't escape backslash. It's currently safe **only because** the selector regex forbids `\` and `"` in label values. | `query-engine/main.go:98-99, 143-145` | Low (latent) | Note for D-04 query layer |
| **O-7** | Correction to register F-066: the `/api/v1/query` path exists (compatibility handler); the actual failure is PromQL function expressions | `query-engine/main.go:534-547, 570` | — | Register v1.2 updated |

**Question for later (not now):** once A1 results are reviewed, do you want **O-1** added as a new Stage 1 approval item before A2? I haven't acted on it.
