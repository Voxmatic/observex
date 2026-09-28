# 11 — Prioritized Remediation Plan (from `run-20260916-local` + audit)

**Status:** PLAN ONLY. No files modified, no fixes implemented. A1, A2 and S0 remain on hold. **Nothing here is production-ready.**

**Evidence sources:**
- **[V]** validation run `evidence/run-20260916-local` (check IDs such as FE-05, PG-01)
- **[A]** audit findings register `01` (F-xxx) and A1 plans `A1_Implementation_Plans.md` (O-x)

**Approval column:**

| Label | Meaning |
|---|---|
| A1, A2, S0, A7 | As defined in `06`/`07` |
| A3–A8, B1–B4 | Existing IDs in `06` |
| **NEW** | Needs a new approval item |

**Priority:**

| Priority | Meaning |
|---|---|
| P0 | Exploitable security issue, or blocks a fresh install |
| P1 | High risk |
| P2 | Quality |
| P3 | Hygiene |

---

## Recommended order

| Step | Items | Why first |
|---|---|---|
| 1 | SEC-1 … SEC-6 (approved A1 items + critical audit gaps) | Exploitable today |
| 2 | DB-1 … DB-4 | A fresh install can't create its database; every Postgres-backed test is blocked |
| 3 | SEC-7 … SEC-9 (dependency vulnerabilities) | Known advisories with fixes available |
| 4 | GO-1 … GO-3 | Needed before Go changes can be validated and reviewed |
| 5 | FE-1, PY-1, PY-2 | Quality gates |
| 6 | CI-1 … CI-6 | Make the gates run automatically |

**Global blockers (unchanged):**
- **RP-2:** Go module access, needed for any Go compile or test.
- **RP-3:** Docker CI, needed for ClickHouse, image and staging checks.
- **P-7:** staging environment.
- **RP-1:** repository baseline.

---

## 1. Critical security vulnerabilities

| ID | Pri | Affected file(s) | Problem | Risk / impact | Proposed fix | Approval | Validation test |
|---|---|---|---|---|---|---|---|
| SEC-1 | P0 | `services/api-gateway/main.go` (`handleSAMLCallbackSecure` ≈4284-4366, `ssoLoginOrProvision` ≈4000-4066) | [A F-001] SAML callback skips signature verification when the org has no cert; org comes from `RelayState`; user looked up by global email | Account takeover including admin | S1-01 containment (reject without enabled config + cert; org match). Deeper XML-DSig rework deferred to D-06. Interim: ingress block of `/api/auth/sso/*` (unverified). | **A1** (approved, on hold); SSO default-off = **A2** | Unit + Postgres integration: no cert → 401, empty RelayState → 401, cross-org user → 403, signed test fixture → JWT; staging exploit script |
| SEC-2 | P0 | `services/query-engine/main.go` (458-470, 524-526, 556) | [A F-003] Raw SQL passthrough to ClickHouse; no auth; `CORS *` | Read, alter or drop all telemetry | S1-04 (amended 17 Sep 2026): remove `/query/events` and the `sql`/`events` passthrough entirely; internal token and CORS moved to follow-ups FU-S104-1/2 (separate approval) | **A1** | `type:sql` → 400 with nothing sent to stub ClickHouse; `/query/events` → 404 for every method |
| SEC-3 | P0 | `internal/middleware/middleware.go` (169, 185, 265); `services/api-gateway/main.go` (484, 535-537, 567-569, 616, 640; registrars ≈4742-4808, 5809-5900); `inhouse_agent.go:801,804` | [A F-010/F-011] Viewer write access on ~31 routes; `InternalOnly` fails open; unauthenticated WebSocket | Privilege escalation, SSRF via integration tests, cross-tenant streaming | S1-06 per plan (scope P-3 pending) | **A1** (+ P-2, P-3, P-6 pending) | Route matrix (viewer/editor/admin × route); router-enumeration test; WS no-token → 401 |
| SEC-4 | P0 | `services/ingestor/main.go` (`esc` 1041-1043; call sites 706-713, 924-947) | [A F-012] ClickHouse string escaping ignores backslash | SQL injection via metric names/labels | S1-08 `chString()` escaper; transport fix (O-3) separate | **A1**; transport = **NEW (S1-08b)** | Injection vector table + literal-boundary checker; optional real ClickHouse round-trip in CI |
| SEC-5 | P0 | `services/ai-agent/main.go` (72, 77, 128, 252-360, 899-906, 926-929); `ai-agent-python/src/server.py` (67-112); `deployments/helm/observex/templates/rbac.yaml` (19-40), `deployments.yaml` (SA refs) | [A F-002] Unauthenticated remediation; approvals are no-ops; `DRY_RUN` off by default; cluster-wide RBAC incl. `pods/exec`; shared ServiceAccount | Any network peer can trigger rollbacks, restarts, cordons | S1-03 safe defaults + internal token; S1-10 per-component SA and least privilege | **A3** (S1-03), **A6** (S1-10) | Default config → dry-run; no token → 403; `kubectl auth can-i` per SA in kind |
| SEC-6 | P0 | `services/api-gateway/main.go` (92, 103, 244-248); `deployments/helm/observex/values.yaml:48`; `internal/db/migrations/001_initial.sql:57-65` | [A F-005] Default JWT/agent secrets accepted; all-zero AES key; seeded admin with unknown password | Forgeable admin tokens; decryptable integration secrets | S1-05 fail-fast on weak secrets; S1-12 bootstrap admin | **A4** (S1-05), **A8** (S1-12) | Table-driven config tests; `helm template` fails without secrets; empty-DB bootstrap test |
| SEC-6b | P0 | `services/api-gateway/main.go` (`handleUpdateUser`/`handleDeleteUser` ≈988-1010); `internal/db/store/stores.go` (89-98) | [A O-1] Admin of org A can update/delete users in org B (no org filter) | Cross-tenant account takeover | Scope user update/delete by `auth.OrgID`; return 404 otherwise | **NEW** | Integration test: org-A admin → org-B user update/delete = 404; same-org = 200 |
| SEC-7 | P1 | `frontend/package.json` (`axios ^1.7.2`, `react-router-dom ^6.23.1`); `frontend/package-lock.json` (resolved `axios@1.14.0`, transitive `form-data@4.0.5`, `follow-redirects@1.15.11`, `lodash@4.17.23` via recharts, `react-router`, `@remix-run/router`) | [V FE-05] `npm audit --omit=dev`: **7 vulnerabilities (3 high, 4 moderate)**. Highs: axios (GHSA-3p68-rc4w-qgx5, GHSA-w9j2-pvgh-6h63, GHSA-pmwg-cvhr-8vh7, GHSA-3w6x-2g7m-8v23), form-data (GHSA-hmw2-7cc7-3qxx), lodash (GHSA-r5fr-rjxr-66jc, GHSA-f23m-r3pf-42rh). All report fixes available. | Prototype pollution / header leakage / CRLF injection in the browser client's HTTP stack | Lockfile-only upgrade within declared ranges first (`npm audit fix`, no `--force`); review any major bumps separately | **B3** (dependency upgrades) | FE-05 re-run → 0 high; FE-02 `tsc` and FE-03 build pass; smoke-load key pages |
| SEC-8 | P2 | `frontend/package-lock.json` (dev tree) | [V FE-06] Full audit: 17 (9 high, 6 moderate, 2 low), including dev tooling | Build-time supply-chain exposure (not shipped to browser) | Upgrade dev dependencies alongside FE-1 (ESLint 8.57.1 is deprecated) | **B3** | FE-06 re-run; build and lint still pass |
| SEC-9 | P1 | `ai-agent-python/requirements.txt` | [V PY-04] **15 unique advisories:** starlette 0.37.2 ×7 (fixes up to 1.3.1), requests 2.31.0 ×3 (≥2.33.0), kafka-python 2.0.2 ×2 (2.3.2), python-dotenv 1.0.1 (1.2.2), scikit-learn 1.4.2 (1.5.0), pytest 8.2.0 (9.0.3) | HTTP-layer vulnerabilities in the agent API | Raise pins (starlette arrives via fastapi, so bump fastapi together). Remove deps the code doesn't import (sklearn, kafka-python only lazily imported in `collector.py:214`). | **B3** | PY-01 install, PY-02 pytest 20/20, PY-04 pip-audit → 0; `pip check` |

---

## 2. Database migration failures (install blockers)

All migration changes require **A7** (S1-11 + D-03), which isn't approved. I haven't changed any schema.

| ID | Pri | Affected file(s) | Problem | Risk / impact | Proposed fix | Approval | Validation test |
|---|---|---|---|---|---|---|---|
| DB-1 | P0 | `internal/db/migrations/001_initial.sql:217` (`slos` table, column `window`) | [V PG-01] `syntax error at or near "window"`. `WINDOW` is reserved in PostgreSQL, so 001 aborts before `slos`, `alert_rules`, `audit_log`, `set_updated_at()` (defined at 278) and the triggers | **Fresh install can't initialize the DB.** Compose mounts migrations into `/docker-entrypoint-initdb.d`, which stops on first error (Likely). Cascades into DB-2. | Quote or rename the column (e.g. `window_days`, aligned with DB-3). Decide whether to edit 001 (it can't apply unmodified on any PostgreSQL ≥ 8.4) or add a corrective migration. **The choice of approach is itself part of A7.** | **A7** | PG-01 Mode A: all files OK in order; Mode B: 0 failures |
| DB-2 | P0 | `002_multitenancy.sql:90`, `003_comments_runbooks.sql:24`, `004_deployments.sql:65`, `007_feature_state.sql:26` | [V PG-01] Cascade failures: `slos` missing, `alert_rules` missing, `set_updated_at()` missing. Result: `users.org_id` never added, `team_members`/`namespace_permissions`/`password_reset_tokens` absent | Multitenancy columns and auth tables never created | Resolved by DB-1 ordering. Verify each file after DB-1; move `set_updated_at()` ahead of first use or into its own early migration. | **A7** | PG-01 Mode B per-file OK; schema assertions for listed tables and `users.org_id` |
| DB-3 | P0 | `005_integrations_postmortems.sql:79-91` (`network_flows … PARTITION BY RANGE (captured_at)` with `id BIGSERIAL PRIMARY KEY`) | [V PG-01] `unique constraint on partitioned table must include all partitioning columns` (independent of DB-1) | `network_flows` never created; network flow store (`stores.go:1174-1183`) fails | Primary key `(id, captured_at)` or drop partitioning | **A7** | PG-01 005 OK; insert/select round-trip into a partition |
| DB-4 | P0 | `internal/db/store/stores.go` (344-357 slos; 435-455; 513-527; 125; 1046-1059); `advanced_features.go:467-546`; `ai_monitoring_agent.go:356-415`; `main.go:1561-1596` | [A F-006, confirmed further here] Code/schema drift: slos code uses `service_id, metric_name, target_pct, window_days` vs migration `service_name, kind, target, window…`; `namespace_perms` vs `namespace_permissions`; `password_resets` vs `password_reset_tokens`; `sessions.org_id` absent; SSO column names differ; `incidents`, `service_catalog`, `agent_knowledge`, `agent_playbooks` have no migration | Even after DB-1…3 install, SLOs, namespace permissions, password reset, sessions, SSO config, incidents and catalog fail silently (errors ignored) | S1-11 reconciliation migration + code alignment + stop ignoring store errors, per D-03 canonical directions | **A7** | **Schema contract test:** `PREPARE` every SQL string from store/handlers against the fully migrated DB → 0 failures (needs Go access) |
| DB-5 | P1 | `Makefile` (`db-migrate`: `psql … -f $$f \|\| true`) | [V Makefile inspection] Errors suppressed, so a broken migration reports success | Hides DB-1…3 during manual setup | Remove `\|\| true`, add `ON_ERROR_STOP=1` | **NEW** (Makefile change) | Run `make db-migrate` against empty PG16: must fail loudly before DB-1 fix, pass after |
| DB-6 | P1 | `deployments/docker/configs/clickhouse-init.sql`; `deployments/helm/observex/templates/configmap.yaml:23-40`; `services/query-engine/main.go` (`database=observex`) | [A O-4] Compose creates tables in `default`; query engine reads `observex`; Helm defines a different table set | Metrics queries fail in Compose; inconsistent schemas | Decide one ClickHouse schema and DB name | **NEW**, tied to D-04 | ClickHouse container: apply init, run query-engine instant/range queries (needs Docker CI) |

---

## 3. Application and Go code issues

| ID | Pri | Affected file(s) | Problem | Risk / impact | Proposed fix | Approval | Validation test |
|---|---|---|---|---|---|---|---|
| GO-1 | P1 | `go.mod` (replace block); all Go modules | [V GO-05] Root module can't resolve without module access; `go.mod` replaces canonical paths with GitHub mirrors (F-082) | No compile, vet, test or security scan of any service is possible | Unblock RP-2 (allowlist). Then restore canonical module paths and Go ≥1.23 (**no `GOPROXY=direct`**). | RP-2 (owner) + **B2** | `go mod download && go mod verify`; `go build ./...`; `go vet ./...` per module |
| GO-2 | P2 | 46 files: `internal/db/models/models.go`, `internal/db/store/{store,stores}.go`, `internal/middleware/middleware.go`, `pkg/models/models.go`, `services/activegate/main.go`, `services/ai-agent{,-v2}/main.go`, all 11 `services/api-gateway/*.go`, `services/db-monitor/main.go`, `services/ingestor/main.go`, `services/observex-agent/**` (8), `services/oneagent/**` (7), `services/processor/{forecast,main,ml_cost_forecast,regression}.go`, `services/query-engine/main.go`, `services/trivy-scanner/main.go`, `tests/*.go` (4); full list in `logs/GO-02-gofmt-list.log` | [V GO-02] Not gofmt-formatted (58 files parse cleanly, GO-01) | Noisy diffs make security fixes hard to review; lint gate can't pass | One mechanical `gofmt -w` commit, **separate** from any functional change and **before** A1 PRs to keep A1 diffs reviewable | **NEW** | GO-02 → 0 files; GO-01 still 0 parse errors; `go build`/`go test` unchanged (needs GO-1) |
| GO-3 | P1 | `tests/unit_test.go`, `tests/ingestor_test.go`, `tests/processor_test.go`, `tests/e2e_test.go` | [V GO-04 + A F-100] 69 tests pass but re-implement logic locally (e.g. `extractSignedInfoBytes`, `zScore`); no product package imported | False confidence; 0% product coverage | Keep as-is for now. Add real package tests with each A1 PR (per A1 plan §2) and migrate valuable cases later. | Covered by **A1** conditions C6/C7 for new tests; retiring old tests = **NEW** | Coverage report generated from product packages (needs GO-1) |
| GO-4 | P1 | `services/ai-agent/main.go:529` | [A F-043] Rollback revision compared as strings | Rolls back to the wrong version | S1-09 numeric selection | **A1** | `selectPreviousReplicaSet` table test (9,10,11 → 10) |
| GO-5 | P1 | `services/ingestor/main.go:959-975` (`clickhouseQueryErr`) | [A O-3, reproduced with Go HTTP server] SQL sent unencoded in URL, so writes likely rejected (400); errors logged at Debug | Metrics/services/edges/profiles likely never stored | Send query in POST body; log failures at warn with counters | **NEW (S1-08b)** | ClickHouse container insert + read-back (needs Docker CI) |
| GO-6 | P2 | `services/processor/main.go:1861-1871` | [A O-2] Processor posts to gateway `/internal/notify`, which doesn't exist | Alert notifications never delivered | Add route (behind internal auth) or change target | **NEW** (depends on S1-06 token) | Integration: processor alert → gateway notify → stub webhook receives |

---

## 4. Frontend and Python quality issues

| ID | Pri | Affected file(s) | Problem | Risk / impact | Proposed fix | Approval | Validation test |
|---|---|---|---|---|---|---|---|
| FE-1 | P2 | `frontend/` (no `.eslintrc*`/`eslint.config.*`); `frontend/package.json` (`lint` script, eslint 8.57.1 deprecated) | [V FE-04] `ESLint couldn't find a configuration file` (exit 2) | No lint gate; `--max-warnings 0` will likely fail on first run | Add a config with rules at *warn* initially; record baseline warning count; tighten later | **NEW** | FE-04 exits 0 with recorded warning count; FE-02/FE-03 still pass |
| FE-2 | P3 | `frontend/src/lib/api.ts:96` | [A C3] `changePassword` sends `current_password`; backend expects `old_password` (no UI caller today) | Latent broken API client | Included in S0 patch | **S0** (on hold) | `tsc`, build; contract test when API tests exist |
| FE-3 | P3 | `frontend/package.json` (no `test` script) | [V inspection] No frontend unit tests | Regressions undetected | Add a test runner in a later stage | **NEW** | Runner executes ≥1 smoke test in CI |
| PY-1 | P2 | `ai-agent-python/src/agent.py` (17, 23 ×3), `collector.py:9`, `executor.py` (7, 10, 12, 172), `llm.py` (7, 12), `main.py` (17, 70, 75, 184, 212, 219, 223), `rules.py` (9, 13, 14, 673), `server.py:115`, `tests/test_rules.py:310` | [V PY-05] ruff: 24 errors (17 × F401 unused import, 5 × F841 unused variable, 2 × F541 f-string without placeholders); 19 auto-fixable | Dead code hides real issues; blocks a lint gate | Review each (not blind `--fix`); remove unused imports/variables | **NEW** | PY-05 → 0 errors; PY-02 pytest 20/20 |
| PY-2 | P2 | `ai-agent-python/Dockerfile` (`python:3.12-slim`) vs validated interpreter | [V PY-01] Tests ran on Python 3.11.15, not 3.12 | Version-specific failures undetected | Run PY-01…05 on 3.12 in CI | **NEW** (CI) | Same checks on 3.12 all green |
| PY-3 | P3 | `ai-agent-python/src/__pycache__/*.pyc` (8 files) | [baseline] Compiled bytecode tracked in the source tree | Stale artifacts; repository noise | Remove and add to `.gitignore` | **B1** (deletions) | Files absent; tests still pass |

---

## 5. Test and CI configuration problems

| ID | Pri | Affected file(s) | Problem | Risk / impact | Proposed fix | Approval | Validation test |
|---|---|---|---|---|---|---|---|
| CI-1 | P1 | `sdk/rum/observex-rum.test.js` | [V RUM-01] Process never exits (flush `setInterval` per test); killed at 90 s (exit 124). All 37 TAP lines `ok`, 0 `not ok`. | Hangs any CI job | S0 test-only timer cleanup (SDK unchanged) | **S0** | RUM-01: exit 0 in seconds, 28/28 pass |
| CI-2 | P1 | `.github/workflows/ci.yml` | [A F-101] Jobs reference nonexistent paths (`observex`, `observex-profiling`, `observex-synthetic`, `observex-neo4j`); Go 1.22 | No working quality gate | Rewrite for the real layout (5 Go modules, frontend, Python, RUM, migrations) once RP-3 platform is known | **NEW** (S2-01; depends on RP-3) | Workflow run green on a clean checkout |
| CI-3 | P1 | `.github/workflows/cd.yml` | [A F-021] Trivy `exit-code: 0` never blocks | Critical CVEs ship | Fail on CRITICAL with exceptions file | **B4** | CD run fails on a seeded vulnerable test image |
| CI-4 | P1 | `Makefile` (`test`, `test-integration`, `test-frontend`, `gosec`, `lint`, `build`/`SERVICES`) | [V inspection] `test` covers only root module (no tests); `test-integration` filters tag `integration` (no files have it); `test-frontend` calls missing `npm test`; `gosec` path `github.com/securecgo/…` (likely typo); `lint`/`gosec` install `@latest`; `SERVICES` omits `query-engine` and includes separate module `db-monitor` | Commands report success or fail misleadingly | Fix targets to iterate modules, pin tool versions, correct gosec path, align service list | **NEW** | Each target run once: expected pass/fail recorded; `make gosec` installs pinned version (needs GO-1) |
| CI-5 | P2 | Migration validation (no job exists) | [V PG-01] Migration breakage wasn't caught by any automation | Install regressions recur | Add PG16 migration job (Mode A + Mode B from `pg_validate.sh`) + schema contract test (DB-4) | **NEW** (with CI-2) | Job fails on current migrations, passes after A7 fixes |
| CI-6 | P2 | `services/db-monitor/Dockerfile:10` (`CGO_ENABLED=1` on alpine without gcc) | [A F-089] Image build likely fails (Likely; needs Docker) | db-monitor can't be built | `CGO_ENABLED=0` | **NEW** | `docker build` in CI succeeds |

---

## Decisions needed from you (not assumed)
1. **NEW approvals introduced by this plan:** SEC-6b, S1-08b (GO-5), GO-2 (gofmt), GO-6, DB-5, DB-6, FE-1, FE-3, PY-1, PY-2, CI-2, CI-4, CI-5, CI-6, plus retiring legacy tests (GO-3).
2. **A7 approach for DB-1:** edit `001_initial.sql` in place, or add corrective migrations. `WINDOW` has been a reserved word since PostgreSQL 8.4, so no supported PostgreSQL version could have applied this file unmodified. The open question is whether **any environment has a database created from a manually edited copy**, which would make in-place edits unsafe. That's unknown, so please confirm.
3. **Order of GO-2 (gofmt) relative to A1:** my recommendation is before A1, so A1 diffs stay small.

**Holds unchanged:** A1, A2 and S0 on hold. No changes made.
