# 07 — ObserveX Engineering Approval Log

Status values: **Approved** · **Rejected** · **Approved with conditions** · **Pending clarification** · **Blocked**

Silence is never counted as approval. Each entry records the answer as given.

---

## A1 — Stage 1 security fixes without product-default changes

| Field | Record |
|---|---|
| **Decision ID** | A1 |
| **Answer (as given)** | Approve all five items: S1-01, S1-04, S1-06, S1-08, S1-09 |
| **Status** | **Approved with conditions** |
| **Date recorded** | 16 Sep 2026 |
| **Implementation status** | Plans prepared (`A1_Implementation_Plans.md`). **No code changes made.** Waiting for plan confirmation. |

### Conditions recorded (verbatim intent)
| # | Condition | How it will be honored |
|---|---|---|
| C1 | One separate commit/PR per fix | 5 branches/PRs: `a1/s1-01-saml-containment`, `a1/s1-04-query-engine-sql`, `a1/s1-06-authz-fail-closed`, `a1/s1-08-clickhouse-escaping`, `a1/s1-09-rollback-revision` |
| C2 | No deeper SAML signature-wrapping changes; document for D-06 | Documented in `D-06_SAML_Deferred_Weaknesses.md`; S1-01 plan excludes XML-DSig changes |
| C3 | Remove raw SQL endpoint entirely; not admin-only | S1-04 removes the code path; no flag, no role exception |
| C4 | Authorization changes fail-closed | Deny when the token is unset or the role is unknown; no compatibility bypass |
| C5 | Preserve the fixed built-in events query | S1-04 keeps the bounded default query; the `sql` field is rejected |
| C6 | Comprehensive unit, integration and security tests | Per-plan test matrix |
| C7 | Specific regression tests (7 listed) | Mapped in the plans' "Required regression tests" table |
| C8 | Document env vars and service-token configuration | `docs/configuration/internal-service-token.md` + `.env.example` + Helm values docs (part of S1-06 PR) |
| C9 | One consistent internal-token name everywhere | Proposed name in plans §0; **needs your confirmation (P-1)** |
| C10 | Staging validation before production | Staging checklist per plan; **staging environment details needed (P-7)** |
| C11 | No production-ready claim until Go build, vet, tests and security validation pass | Results reported per PR; status stays "Not production-ready" until all gates pass |
| C12 | S0 patch stays separate; not merged under A1 | S0 remains decision A10 |
| C13 | Don't proceed to A2 until A1 implementation and validation results are reviewed | A2 not presented until the A1 results review |

### Affected stabilization items
S1-01, S1-04, S1-06, S1-08, S1-09. Not affected: S1-02, S1-03, S1-05, S1-07, S1-10, S1-11, S1-12, S0.

### Dependencies identified
| Dependency | Effect | Status |
|---|---|---|
| **Go toolchain for compile, vet and test (A9)** | Condition C11 can't be satisfied without it. Implementation can be written but not validated. | **Blocked — needs your choice (P-0)** |
| S1-04 depends on S1-06 | The query engine requires the internal token that S1-06 defines, and the gateway must send it | Order S1-06 → S1-04 |
| S1-01 tests need Postgres | Integration tests run against a real Postgres (CI service or local Docker) | Environment needed |
| F-006 schema drift | Some integration fixtures must create tables that the code expects but migrations lack; S1-11 (A7) isn't approved, so fixtures stay test-local | Noted; no schema change |
| Staging environment (C10) | Required before production | **Pending clarification (P-7)** |

---

## Remaining decisions
| ID | Status |
|---|---|
| A2–A10 | Not yet presented (blocked by C13 until A1 results are reviewed) |
| B1–B4, C1–C10, D1–D10 | Not yet presented |

---

## A1 prerequisites (Prompt 4)

Rule: A1 implementation doesn't start until **all three** prerequisites below are resolved. A2 isn't presented, and the S0 patch isn't applied.

| ID | Prerequisite | Status | Answer | Conditions | Affects |
|---|---|---|---|---|---|
| P-0 | Go toolchain & validation environment | **Approved with conditions** — but **blocked** on module access (see below) | Option 3 — Both | See P-0 record | All five A1 items (C11 gate) |
| P-1 | Internal service-token naming | **Approved with conditions** | Option 2 — ObserveX-prefixed names | See P-1 record | S1-06, S1-04 (C9) |
| P-7 | Staging environment details | **Pending clarification** (provisional SD-1…SD-5 recorded) | Disposable kind per run; infra unverified (see P-7 record) | No production data; no invented infra | All five A1 items (C10 gate) |

Other plan questions (P-2 WebSocket credentials, P-3 S1-06 route scope, P-4 S1-08 transport, P-5 rollback label, P-6 ingestor `/internal/*`) remain **Pending** and will be asked after the three prerequisites, before the affected PR is written.

### Standing constraint recorded: "Keep SSO disabled until D-06 is completed"
| Field | Record |
|---|---|
| Source | Prompt 4, rule 8 |
| **Factual state of the code** | **SSO is not disabled in code today.** SAML/OIDC routes are registered unconditionally (`services/api-gateway/main.go` ≈376-382), and SAML/OIDC become active for any org with an `enabled=true` row in `sso_configs`. The code switch that turns SSO off by default is S1-02, which belongs to A2, and A2 is on hold. |
| Implication | Until S1-02 is approved and deployed, SSO can only be kept disabled **operationally**. |
| Operational measures available now (no code change) | (1) Keep no `sso_configs` rows with `enabled=true`, and check each environment with `SELECT org_id,type,enabled FROM sso_configs WHERE enabled;`. (2) Block `/api/auth/sso/*` at the ingress or reverse proxy. (3) Measure (1) alone does **not** stop the F-001 bypass, because the bypass works precisely when an org has *no* SAML config. Measure (2) does block it (the callback is `/api/auth/sso/saml/callback`), so (2) is the effective interim control until S1-01 is deployed. |
| Status | **Recorded; needs your confirmation of which operational measures apply to each environment** (to be asked after the prerequisites) |

---

## P-0 record — Go toolchain & validation environment

| Field | Record |
|---|---|
| Decision | **Option 3 — Both** |
| Status | **Approved with conditions**. Execution is **Blocked** until Go module egress is available here (local track) and runner details are known (independent track). |
| Recorded | 16 Sep 2026 |

### Conditions (as given)
1. Claude may run local validation (Go compilation, `go vet`, unit tests, `gosec`, `govulncheck`) **once module access is available**.
2. **Independent validation must run in the owner's environment before staging**, including the Postgres and ClickHouse integration tests.
3. **No claim that validation has passed until actual command results are recorded** (command, environment, timestamp, exit code, output summary).
4. Keep SSO disabled via ingress/reverse-proxy blocking of `/api/auth/sso/*`.
5. Don't begin A2; don't apply S0.

### Validation track 1 — Local (Claude's sandbox)
| Item | Value | Evidence |
|---|---|---|
| Go toolchain installed | go1.24.7 linux/amd64 (meets 1.23+) | `go version`, 2026-09-16T15:36Z |
| Network egress for Go modules | **Pending: not available** | `go mod download github.com/gofiber/fiber/v2@v2.52.4` → `403 Forbidden: Host not in allowlist: proxy.golang.org`; `sum.golang.org` unreachable (HTTP 000), 2026-09-16T15:36Z |
| Docker | Not available in this sandbox (integration tests can't run here) | Environment observation from the audit |
| Unblock action | Owner adds `proxy.golang.org` and `sum.golang.org` to this environment's network egress settings | — |

### Validation track 2 — Independent (owner's environment)
| Item | Value |
|---|---|
| Runner | **Pending** [local machine / GitHub Actions / other CI] |
| Go version | **Pending** (requirement: 1.23+) |
| Docker availability | **Pending** [yes / no] (required for the Postgres/ClickHouse integration tests) |
| Network egress for Go module downloads | **Pending** [available / pending] |
| Who runs it and returns results | **Pending** |

### Evidence format required before any "passed" claim
`command` · `module/path` · `environment (local sandbox / owner runner)` · `Go version` · `UTC timestamp` · `exit code` · `pass/fail/skip counts` · `gosec/govulncheck findings` · `link or attached log`

### SSO interim control — updated
| Field | Record |
|---|---|
| Chosen control | **Ingress/reverse-proxy block of `/api/auth/sso/*`** (owner instruction) |
| Implementation status | **Unverified.** No evidence yet that the block is deployed in any environment. |
| Verification needed per environment | `curl -sS -o /dev/null -w '%{http_code}' https://<host>/api/auth/sso/providers` and `-X POST .../api/auth/sso/saml/callback` should return the proxy's block status (e.g. 403/404) and never reach the gateway (confirm via gateway access logs) |
| Status | **Pending confirmation of deployment per environment** |

---

## P-1 record — Internal service-token naming

| Field | Record |
|---|---|
| Decision | **Option 2 — ObserveX-prefixed names** |
| Status | **Approved with conditions** |
| Recorded | 16 Sep 2026 |

### Canonical names (approved)
| Element | Name |
|---|---|
| Environment variable | `OBSERVEX_INTERNAL_TOKEN` |
| File variant | `OBSERVEX_INTERNAL_TOKEN_FILE` |
| Precedence | **File variant takes priority over the environment variable** |
| HTTP header | `X-ObserveX-Internal-Token` |
| Helm secret key | `internal-token` |

### Conditions (as given)
1. Use these names consistently across the gateway, processor, Helm, Compose, `.env.example`, documentation and tests.
2. **No legacy-name support** (`INTERNAL_TOKEN`, `X-Internal-Token`) unless separately approved. The existing uses in `services/api-gateway/main.go:290`, `internal/middleware/middleware.go:34,261-265` and `services/processor/main.go:1871` will be replaced, not aliased.
3. **Never log token values.** This applies to startup logs, error messages, request logs, test output and debug dumps.
4. Document secret handling and precedence clearly.
5. A2 and S0 stay on hold.

### Affected items and dependencies
| Item | Effect |
|---|---|
| S1-06 | Defines the loader, receiver middleware, host-scoped sender, processor rename, Helm/Compose/.env/docs |
| S1-04 | Query engine receives the same header and variables (separate Go module, same names and semantics) |
| P-6 (pending) | If approved, the ingestor's `/internal/*` uses the same names |
| Dependency | None new; still gated by P-0 (blocked on module egress) and P-7 |

### Implementation details still to confirm in the S1-06 plan review (not assumed as approved)
| # | Detail | Proposed behavior |
|---|---|---|
| N-1 | `OBSERVEX_INTERNAL_TOKEN_FILE` is set but the file is missing, unreadable or empty | Treat as **not configured** (fail-closed: 503 on protected routes); **don't** fall back to the env var; log path and error class only, never contents |
| N-2 | Both are set | The file wins; log one INFO line saying "internal token source: file" (no value) |
| N-3 | Trailing newline in the secret file | Trim surrounding whitespace |
| N-4 | Minimum length | 32 bytes after trimming; shorter → not configured |
| N-5 | Services outside the listed scope that also use the token | Query engine (S1-04, already approved) and ingestor (only if P-6 is approved) |
| N-6 | Tests that prove no token logging | Capture the logger in tests and assert the token string never appears in any log line or error body |

---

## P-7 record — Staging environment

| Field | Record |
|---|---|
| Status | **Pending clarification** |
| Recorded | 16 Sep 2026 |

| # | Item | Recorded value |
|---|---|---|
| 1 | Staging environment | **Unknown** |
| 2 | Platform | **Unknown** |
| 3 | Ingress / reverse proxy | **Unknown** |
| 4 | Data classification | **Unknown.** Production data **must not** be used for testing until explicitly approved. |
| 5 | Deployment owner & result-sharing method | **Pending** |
| 6 | Test accounts across two organizations | **Pending** |
| 7 | SAML test identity provider | **Pending** |
| 8 | AI agent + disposable rollback namespace | **Pending** |

### Conditions (as given)
- Don't invent infrastructure details or assume staging exists.
- **A1 implementation, A2 and the S0 patch stay on hold** until the staging approach is confirmed.
- **No production-readiness claim** without staging validation.

### Consequences recorded
- The SSO interim control (ingress block of `/api/auth/sso/*`) **can't be verified** until item 3 is known. It stays Unverified.
- The staging-readiness checklist is in `08_Staging_Readiness_Checklist.md`.

---

## Current gate summary
| Gate | Status | Blocking |
|---|---|---|
| A1 | Approved with conditions | Implementation **on hold** |
| P-0 | Approved with conditions | Local track blocked (module egress); independent runner details pending |
| P-1 | Approved with conditions | — |
| P-7 | **Pending clarification** | Approach chosen provisionally (SD-1…SD-5); infrastructure, owners and fixtures unverified (`09`) |
| P-2…P-6 | Pending (not yet asked) | Asked after P-7, before the affected PRs |
| SSO interim control | Unverified | Ingress unknown |
| A2 | On hold | C13 + owner instruction |
| S0 patch | On hold | Owner instruction |

---

## P-7 provisional staging decisions (SD-1 … SD-5)

| Field | Record |
|---|---|
| Status | **Provisional.** P-7 remains **Pending clarification** (owner instruction: don't mark Approved yet) |
| Recorded | 16 Sep 2026 |

| ID | Decision (as given) | Status | Consequences recorded |
|---|---|---|---|
| SD-1 | **(b) Disposable kind cluster per validation run** | Provisional | Requires Docker + kind on the owner's runner. Per §4 of `08`, this does **not** replace production-like staging before any production deployment. |
| SD-2 | **Synthetic data only** | Provisional | Fixtures generated per run; no production or sanitized copies |
| SD-3 | **Owner's team or CI runner deploys and executes validation.** Results returned as command output or CI logs containing **command, environment, timestamp, exit code, output summary**. | Provisional | Matches the P-0 evidence format; no "passed" claim without it |
| SD-4 | **Yes.** A legitimate SAML login must be proven using an **isolated test IdP** | Provisional | Adds fixture SF-3 (test IdP with signing cert) to S1-01 staging; the S1-01 positive control is required, not optional |
| SD-5 | **Yes.** A real rollback test in a **disposable namespace**, subject to **explicit permission for each run** | Provisional | Adds fixture SF-5; S1-09 staging run must not start without a recorded per-run permission entry (template in `09` §3) |

### Still Pending (unchanged, per owner instruction)
Infrastructure availability, owners, runner details, fixture creation status. Inventory form: `09_Staging_Inventory_Form.md`.

### Holds
A1 implementation: **on hold** · A2: **on hold** · S0 patch: **on hold**

---

## Environment evaluation — Claude-hosted development + CI/remote validation (no decision taken)

| Field | Record |
|---|---|
| Request | Owner prefers not to install the ObserveX dev environment on a local Windows machine |
| Output | `10_Remote_Validation_Evaluation.md` |
| Status | **Evaluation only.** No decision recorded; no code changed; S0 not applied; A1, A2 and S0 on hold |
| Verified sandbox facts (2026-09-16 15:36–15:47 UTC) | Go 1.24.7 present; `proxy.golang.org`, `sum.golang.org`, `vuln.go.dev`, Docker Hub, ghcr.io, `get.helm.sh`, `dl.k8s.io`, ClickHouse download hosts → **403**; npm registry and PyPI → 200; `api.github.com` → 200; Docker CLI present but **no daemon**; kind/kubectl/helm absent; **PostgreSQL 16.13 server runs locally without Docker**; 2 vCPU / 7 GiB RAM |
| New finding | **Migrations 001–007 fail on PostgreSQL 16** (001 line 217 reserved word `window`, cascading failures; 005 line 91 partitioned unique constraint). See `10` §6. Extends F-006/F-044; fix belongs to S1-11/A7 (not approved). |
| Open items raised | Egress allowlist; repo hosting + zip-vs-head check; CI platform with Docker; whether owner-controlled CI counts as "own environment"; rollback approval gate; code handoff method; evidence sharing method; schema approach for A1 integration tests |

---

## P-0 clarification — owner-controlled CI as "own environment"

| Field | Record |
|---|---|
| Recorded | 16 Sep 2026 |
| Answer (as given) | **Yes.** CI runners the owner's team owns and controls count as the owner's own environment for P-0 condition 2, **provided that all of these hold** |
| Status | **Approved with conditions** (clarification to P-0) |

### Conditions (all mandatory; each must be evidenced in `09` before CI results count as independent validation)
| # | Condition | Evidence required | Status |
|---|---|---|---|
| CI-1 | Runners on **isolated Linux infrastructure** | Runner OS/arch + hosting description (`uname -sm`, runner labels) | Pending |
| CI-2 | **Docker support** on runners | `docker version` output from a runner job | Pending |
| CI-3 | **No production credentials or network routes** | Credential inventory for the CI org/project (no prod secrets); failed connection test from a runner to production endpoints | Pending |
| CI-4 | Integration and staging results **independently recorded** | Evidence records (format in `10` §4.1) produced by the CI job itself and stored as CI artifacts, separate from Claude's sandbox evidence | Pending |
| CI-5 | Rollback runs require the **approved per-run permission** | Protected-environment approval gate configured + a row in `09` §3 before each run | Pending |

### Standing restriction recorded
**Don't use `GOPROXY=direct`, `GONOSUMDB`, `GONOSUMCHECK`, `GOFLAGS=-insecure`, `GOSUMDB=off`, or any other way to bypass the module proxy or checksum verification**, in Claude's sandbox or in CI. Go validation in Claude's sandbox waits for the allowlist of `proxy.golang.org`, `sum.golang.org` and `vuln.go.dev`.

### Holds (unchanged)
A1 implementation **on hold** · A2 **on hold** · S0 **on hold**, until the remaining prerequisites **and the schema decision** (`10` §6) are reviewed.

### Remaining prerequisites (to be asked one at a time)
| # | Item | Status |
|---|---|---|
| RP-1 | Source repository hosting + whether `version1.zip` matches the current head | **Pending clarification** (see RP-1 record) |
| RP-2 | Egress allowlist for Claude's sandbox (`proxy.golang.org`, `sum.golang.org`, `vuln.go.dev`) | **Pending clarification** (see RP-2 record) |
| RP-3 | CI platform and runner details (satisfying CI-1…CI-5) | **Pending — presented** |
| RP-4 | Code handoff method (patch files / scoped push token) | Pending |
| RP-5 | Evidence sharing method (artifacts attached to chat / read-only CI token) | Pending |
| RP-6 | Schema approach for A1 integration tests (fixture schema / bring A7 forward / unit-only) | Pending |
| RP-7 | P-7 inventory evidence (`09`) | Pending |

---

## RP-1 record — Repository baseline

| Field | Record |
|---|---|
| Answer (as given) | **Option 4: Pending clarification.** Owner will confirm hosting platform, repository URL/name, default branch, and the commit SHA matching `version1.zip`. |
| Status | **Pending clarification** |
| Constraint | **No A1 work resumes until the baseline is confirmed** |
| Recorded | 16 Sep 2026 |

| Item | Value |
|---|---|
| Hosting platform | Pending |
| Repository URL / name | Pending |
| Default branch | Pending |
| Commit SHA matching `version1.zip` | Pending |

### Verification aids provided (read-only; no code changed)
| Artifact | Purpose |
|---|---|
| `baseline/version1_zip_git_blob_manifest.txt` | Git blob SHA-1 for each of the 287 files in the zip (zip sha256 `87351b555c6cf9741dec8544e69569e0d728f9e01c3e9c31d8603609ada0b4d6`), each tagged `tracked` or `gitignored` |
| `baseline/compare_baseline.sh` | Compares any commit with the manifest using `git ls-tree` (no checkout changes). Exit 0 = tracked files identical. Works at the repo root or in an `observex/` subdirectory. Self-tested on a synthetic repo: identical → exit 0; one modified file → exit 1 naming the file. Can run in CI (RP-3), so nothing needs installing on Windows. |

### Facts about the zip relevant to the baseline (verified)
- **Correction:** my earlier message said the newest file was `docker-compose.yml` (30 Aug 2026 15:54). The newest file is actually **`services/api-gateway/main.go`, modified 2026-08-30 16:19** (zip timestamps; timezone not recorded in the zip).
- The zip contains **23 files that its own `.gitignore` excludes**: 6 root binaries plus 17 `frontend/dist` files. The zip is therefore **most likely a working-directory copy, not a `git archive` export**. Local changes that were never committed may be included, so an exact commit match may not exist.
- The zip has no `.git` directory, so no commit SHA can be derived from it.

### If no exact match exists
To be decided when RP-1 is answered (not assumed): (a) commit the zip contents as the baseline on a new branch, or (b) re-baseline the audit on the repo head (re-run baseline checks; update line numbers and findings).

---

## RP-2 record — Egress allowlist for Claude's environment

| Field | Record |
|---|---|
| Answer (as given) | **Option 4: Pending clarification.** Owner is confirming whether the organization permits allowlisting `proxy.golang.org`, `sum.golang.org`, `vuln.go.dev` for this Claude environment. |
| Status | **Pending clarification** |
| Recorded | 16 Sep 2026 |
| Constraints | A1 stays paused. **No `GOPROXY=direct`, checksum bypass or other workaround.** |
| Contingency (owner's) | If allowlisting isn't possible, the owner decides whether to **revise P-0 so all Go validation runs in CI**. Not decided; P-0 stays option 3 until the owner says otherwise. |
| Current evidence | All three hosts return 403 from this environment (2026-09-16T15:36Z) |

---

## Local folder inspection — `C:\Users\91832\Documents\observex` (read-only)

| Field | Record |
|---|---|
| Access | Granted for this folder only, 2026-09-16 |
| Command | `device_list_dir C:\Users\91832\Documents\observex (recursive)` |
| Result | **Folder is empty** (0 files, 0 subfolders) |
| Comparison with `version1.zip` manifest | Not meaningful: 0 matching, 0 different, 0 added; all 287 zip files absent locally |
| Does the zip match the local project? | **No.** The local folder contains no project. |
| RP-1 | Still **Pending clarification**: this folder isn't the hosted repository and holds no baseline |
| Changes made | None |
| Next | Owner decides whether to populate the folder from `version1.zip` (option a: 264 tracked source files, 23 gitignored build outputs excluded) |

---

## Local working copy created from `version1.zip` (owner-approved copy)

| Field | Record |
|---|---|
| Approval | Owner: "Yes. Copy the 264 source files from version1.zip into Documents\observex" (conditions: preserve structure, exclude 23 ignored build outputs, verify each file against manifest, no source modification, stop after verification) |
| Target | `C:\Users\91832\Documents\observex` |
| Source | Pristine re-extraction of `version1.zip` (sha256 `87351b55…b4d6`); the audit working copy with S0 edits was **not** used |
| Excluded by design | 23 gitignored build outputs (6 root binaries, 17 `frontend/dist` files) |
| Written | **259 of 264** tracked files |
| Not written (5) | **Protected by the device bridge, can't be written remotely:** `.github/workflows/cd.yml`, `.github/workflows/ci.yml`, `Makefile`. **Over the 20 MB per-file transfer limit:** `services/oneagent/oneagent` (50,226,675 bytes) and `services/observex-agent/oneagent/oneagent` (50,226,675 bytes), both compiled binaries. |
| Verification method | Re-staged all 259 written files from the device and computed `git hash-object` for each; compared with the manifest blob SHA-1 |
| Verification result | **259 match · 0 mismatch · 0 missing** (2026-09-16T16:18:27Z). Byte-identical, no line-ending conversion. |
| Extra files in folder | None (recursive listing shows only the copied files) |
| Source modified | No |
| Holds | A1, A2, S0 on hold. RP-1 still pending: this copy comes from the zip, not the hosted repository. |

### Owner-placed protected files — verification (read-only)
| File | Bytes | Manifest blob | Result |
|---|---|---|---|
| `.github/workflows/cd.yml` | 9173 | `20dad823…086a` | MATCH |
| `.github/workflows/ci.yml` | 6764 | `08f2f22b…7916` | MATCH |
| `Makefile` | 8744 | `6aa4462e…3467` | MATCH |

Verified 2026-09-16T16:47:20Z. The local copy now holds **262 of 264** tracked files, all byte-identical to the manifest. The only files absent are the two 50.2 MB `oneagent` binaries (owner decision pending). A1, A2 and S0 remain on hold.

---

## Read-only project inspection of the local baseline (2026-09-16T16:48Z)
- **Owner decision recorded:** the two `oneagent` binaries stay excluded. Baseline = 262 verified files.
- **Method:** inspected verified copies of the 262 local files in the sandbox. No changes made.
- **Limitation:** there's no command-running tool on the Windows machine, so the Windows toolchain (Go/Node/Python/Git/Docker) **couldn't be checked**. Only the sandbox toolchain is reported.
- **Sandbox:** Go 1.24.7, Node 22.22.2/npm 10.9.7, Python 3.11.15, Git 2.43.0, GNU Make 4.3, psql/Postgres 16.13, golangci-lint 2.5.0, ruff 0.15.11. Docker CLI 29.4.3 present but **no daemon**. kind/kubectl/helm/gosec/govulncheck absent. `proxy.golang.org` still 403.
- **Makefile observations (static, not executed):** `test-frontend` calls `npm test`, but no `test` script exists. `db-migrate` appends `|| true`, which hides migration failures (see migrations failing on PG16). The `gosec` install path is `github.com/securecgo/...`, which looks like a typo for `securego`. `test-integration` uses `-tags=integration`, but no Go file has that tag. `make test` runs only the root module, which has no test files (the tests live in the separate `tests/` module). `SERVICES` includes `db-monitor`, a separate Go module, so `go build ./services/db-monitor/` from the root is likely to fail. `SERVICES` also omits `query-engine`. `lint`/`gosec` install `@latest` (unpinned).
- **Holds:** A1, A2 and S0 remain on hold.

---

## Validation evidence run `run-20260916-local` (owner-approved read-only checks)
- **Evidence:** `evidence/run-20260916-local/` (report, index, full logs, SHA256SUMS) and `evidence/run-20260916-local.zip`.
- **Results:** 18 checks · 9 PASS · 8 FAIL (all pre-existing) · 1 BLOCKED (Go modules).
- **Failures:** ESLint config missing; npm audit (7 prod / 17 total); pip-audit (15 unique advisories); ruff 24 errors; RUM test process hang (timeout 124); 46/58 Go files unformatted; PG16 migrations 6/7 files fail.
- **Passes:** npm ci, tsc, frontend build, venv install, pytest 20/20, pip check, gofmt parse 58/58, tests-module vet, tests-module 69/69 (non-product tests).
- **Integrity:** 262 source files re-verified unchanged after the run; nothing written to the user's folder.
- **Holds:** A1, A2 and S0 on hold. No production-readiness claim.

---

## S1-09 implementation (A1, approved) — written 2026-09-17, NOT validated
- **Files changed in `C:\Users\91832\Documents\observex`:** `services/ai-agent/main.go` (only the selection block in `rollbackDeployment`, not reformatted; mtime guard used), new `services/ai-agent/rollback.go`, new `services/ai-agent/rollback_test.go`. Read-back confirmed identical to the sandbox versions. Patch: `s1-09/S1-09.patch` (`git apply --check` OK against baseline `main.go`).
- **Scope:** numeric selection only; `pod-template-hash` untouched; no dependency or other file changes.
- **Sandbox-only stub check (NOT validation):** the new file and tests compiled against minimal hand-written stand-ins for `appsv1.ReplicaSet`/`metav1.ObjectMeta` with `GOPROXY=off`. `go vet` OK; 13/13 subtests pass. The same tests against the old string algorithm (scratch copy only) fail D1, D2 and D3 cases.
- **Not run (Go modules blocked):** real `go build`/`go vet`/`go test` of `services/ai-agent` (including the `main.go` edit), `-race`, gosec, govulncheck, owner-runner validation, staging rollback (SD-5).
- **Status:** implemented, **not validated, not production-ready**. A2 and S0 on hold; other A1 items not started.
- **S1-09 cleanup (owner request, 2026-09-17):** removed the added "selected previous revision" logger call from `services/ai-agent/main.go`; the unused revision return value is now discarded with `_`. `rollback.go` and `rollback_test.go` are unchanged. The first write attempt landed stale content (30,843 bytes); a second guarded write was confirmed by read-back (30,615 bytes, identical). `S1-09.patch` regenerated. Still **not validated**.

---

## S1-06 implementation plan v2 — decisions D1–D7 (approved 2026-09-17)

| Field | Record |
|---|---|
| Decision | **Approve S1-06 implementation plan v2 and decisions D1–D7 as recommended** |
| Status | **Approved with conditions** |
| Recorded | 17 Sep 2026 |

### Decisions (as approved)
| # | Decision | Approved outcome |
|---|---|---|
| D1 | Receiver without a configured internal token | Protected routes return **503** `{"error":"internal authentication not configured"}`; missing or wrong header returns **401** `{"error":"unauthorized"}` |
| D2 | Minimum token length | **32 bytes** after `strings.TrimSpace` (Go `len`, UTF-8 bytes, not characters); shorter = not configured |
| D3 | `GET /api/v1/remediations` | Editor role, then **403** `{"error":"remediation data is not organization-scoped"}` until the AI agent records org IDs |
| D4 | `GET /api/v1/agents` | Editor role; gateway filters to `org_id == auth.OrgID`; agents without an org are hidden |
| D5 | WebSocket events without an org | No longer delivered |
| D6 | Gateway sender transport | In S1-06; header added only for the exact `QUERY_ENGINE_URL` scheme, host and port |
| D7 | Compose token wiring | Compose `secrets:` file + `OBSERVEX_INTERNAL_TOKEN_FILE` |

### Resolved plan questions
| Question | Outcome |
|---|---|
| P-2 WebSocket credentials | **Header-only `Authorization: Bearer`** (same as REST). No query-string, subprotocol or cookie tokens. Browser WebSocket clients unsupported until a separate method is approved. No in-repo `/ws` client exists; external clients: **unknown**. |
| P-3 route scope | **Audited routes only** (R1–R11). The other ~22 unprotected write routes are separate gaps needing approval. |
| P-6 ingestor `/internal/*` | **Out of scope** (unchanged) |
| N-1 file set but unusable | Not configured; **no fallback** to the env var |

### Conditions (as given)
1. Follow plan v2 steps 0–17; S1-06 scope only.
2. No changes to the ~22 additional routes, ingestor `/internal/*`, S1-04 (except the approved query-engine token transport), S1-08 or S1-09 files.
3. Preserve behaviour outside scope; do not reformat unrelated code.
4. Never expose token values in logs, errors or tests.
5. Verify baseline checksums before editing; stop on unexpected changes.
6. Document browser WebSocket limitation and unknown external-client status.
7. No production-readiness claim.

---

## S1-06 implementation — written 2026-09-17, NOT validated
- **Step 0:** baseline checksums verified for all 12 existing target files (git blob match to `version1.zip` manifest); S1-08, S1-09, `go.mod`, `go.sum`, `Makefile` checksums recorded unchanged. No new target path existed.
- **Delivered to `C:\Users\91832\Documents\observex`:** 22 files (12 modified, 10 new); mtime guards used; read-back byte-identical. Diff `s1-06/S1-06.diff`; checksums `s1-06/SHA256SUMS-S1-06.txt`; validation script `s1-06/S1-06-local-validation.ps1`.
- **Sandbox checks (NOT validation):** `internal/servicetoken` compiled, vetted and tested with `-race` (stdlib only); redaction test proven to catch a plain-string leak. gofmt clean on new Go files; parse OK on modified Go files; 16 added gateway lines keep the surrounding pre-existing non-gofmt style (no unrelated reformatting). `docker compose config` renders both Compose files with only the intended additions. Helm templates parse with Go text/template once a **pre-existing** unbalanced parenthesis in `deployments.yaml` (queryEngine `CLICKHOUSE_URL`, original line 441) is patched in a scratch copy only; staging script smoke-tested against a stub server (43 checks, no token in output).
- **Not run:** gateway/middleware/store/processor compilation, `go vet`, tests (Go modules blocked in sandbox; to run locally), `-race` for gateway (CGO), `helm template` (helm unavailable; chart has the pre-existing parse error), gosec/govulncheck, P-0 CI, staging.
- **Deviations from plan (smaller change):** comment-delete test seam uses a pure error-mapping helper (`commentDeleteError`) instead of changing `gw.comments` to an interface; processor loads the token once via `sync.OnceValue` and logs nothing.
- **New pre-existing finding (not fixed, out of scope):** Helm chart `templates/deployments.yaml` has an unbalanced parenthesis in the queryEngine env list, so the chart likely cannot render at all.
- **Status:** implemented, **not validated, not production-ready**. A2 and S0 on hold; S1-01 and S1-04 not started.

---

## S1-06 local validation run 1 — stopped at step 3 (build), 2026-09-17
- **Result:** integrity steps passed up to the build; `go build ./...` failed in `services/api-gateway` only: `row.Namespace undefined (type nativeMetricRow has no field or method Namespace)` in `native_metrics.go`. **Pre-existing** (present in the `version1.zip` baseline; not touched by S1-06). servicetoken, middleware, store and processor reported no build errors.
- **Unexpected change found:** `go.mod` and `go.sum` on the device had changed at 2026-09-17 09:55:18Z (not by Claude). The contents were consistent with `go mod tidy` (fasthttp/websocket moved to direct requires; four go-yaml lines removed from go.sum). Pasted diff text in PowerShell produced only parse errors; no other file changes found.
- **S1-06 status:** not validated.

## Restore go.mod / go.sum to GO-1a / GO-1a.1 — approved and applied 2026-09-17
| Field | Value |
|---|---|
| Approval | User selected "Restore approved (Recommended)" |
| Scope | `go.mod` and `go.sum` only, back to the approved GO-1a / GO-1a.1 bytes |
| Method | `device_commit_files` with mtime guards (go.mod 1789638918364, go.sum 1789638918363); none rejected |
| Read-back (10:05:59Z) | Byte-identical; `go.mod` 60986B8E48523D35D5D2EF0B03119FFAE742006860AEDBC2B8386AA32E303A28 (5834 bytes); `go.sum` 748605EA9E60F735536B156BB1EF846A7C1265EAED84D250666DF6DDC14386F5 (17232 bytes) |
| Condition | Do not run `go mod tidy`; check editor/IDE auto-tidy settings (e.g. gopls) before re-running |

## GO-7 — add `Namespace` to `nativeMetricRow` (pre-existing build error) — approved and applied 2026-09-17
| Field | Value |
|---|---|
| Approval | User selected "Approve as GO-7 (Recommended)"; separate from S1-06 |
| File | `services/api-gateway/native_metrics.go` (baseline verified against the git blob manifest before editing) |
| Change | One added struct field: `Namespace string \`json:"namespace"\`` (the APM services query already selects `... AS namespace`; rows are JSON-decoded into `[]nativeMetricRow`) |
| Diff | `go-7/GO-7.diff` (F174952BF326128C5A4F680AB4207B24E140C9288EA72127693524651FE9D38A) |
| Method | `device_commit_files` with mtime guard 1789575189763; not rejected |
| Read-back (10:05:59Z) | Byte-identical; 463C9764648D87A554A4E7EA6EDF1ABF9E553EEC98DF4EFB853AE0B5386BB45E (7911 bytes) |
| Not done | Not compiled or tested in the sandbox (modules blocked, RP-2). Line kept in the file's existing non-gofmt alignment (no unrelated reformatting). |
| Status | Applied, **not validated**; validated only by the S1-06 re-run build/vet/test steps |

- **Validation script updated:** `s1-06/S1-06-local-validation.ps1` now also checks `services\api-gateway\native_metrics.go` (GO-7 set); expected changed files 30 (22 S1-06 + 1 GO-7 + 7 earlier-approved files from GO-1a/GO-1a.1, S1-08 and S1-09). New SHA-256 51CC4E93413D7FFBEE6D464A57154320257AD0C0D7E559C7D5EF00FE0FABA9F7. No other script change.

---

## S1-06 local validation run 2 — stopped at step 4 (go vet), 2026-09-17T10:12:41Z
- Steps 1 (integrity, 30/30), 2 (gofmt new files) and 3 (build, including GO-7) **PASS**. Step 4 `go vet` **FAIL** with two findings, both in `services/api-gateway` and both **pre-existing** (byte-identical to the `version1.zip` baseline; not introduced by S1-06):
  - `ai_monitoring_agent.go:580:64` — `fmt.Sprintf format %29+ has unknown verb +` (URL-encoded PromQL used as a format string; builds a garbage URL).
  - `main.go:6154:2` — `unreachable code` (legacy body after an always-returning block in `handleAgentInstallScript`; baseline line 5991).
- No vet findings listed for servicetoken, middleware, store or processor. S1-06 status: **not validated**.

## GO-8 — fix malformed agent error-rate query (approved 2026-09-17)
| Field | Value |
|---|---|
| Approval | User approved GO-8 as a separate fix (not part of S1-06) |
| File | `services/api-gateway/ai_monitoring_agent.go` (+ new focused test file) |
| Scope | Build the error-rate query with `url.QueryEscape`; escape `orgID` properly (quotes and special characters); preserve intended PromQL semantics for normal org IDs; focused tests (normal org ID, org ID with quotes/special characters, expected query structure); only necessary import, implementation and test changes |
| Constraints | No go.mod/go.sum change; no `go mod tidy`; no S1-08/S1-09 changes; no additional changes without approval |
| Pre-edit check | Device file SHA-256 EA1276390B542AF6BE2E39C1397FDA93E9C935D9650D0B52CB2728B0BE4AA40F, git blob bd68ffe7… = baseline manifest; mtime 1789575187402 (unchanged since baseline extraction) |

## GO-9 — remove unreachable legacy code in `handleAgentInstallScript` (approved 2026-09-17)
| Field | Value |
|---|---|
| Approval | User approved GO-9 as a separate fix (not part of S1-06) |
| File | `services/api-gateway/main.go` (an S1-06-modified file; its expected checksum changes to S1-06 + GO-9) |
| Scope | Delete only the confirmed unreachable legacy block at the end of `handleAgentInstallScript`; keep the active implementation, braces and behaviour; no refactoring or reformatting |
| Pre-edit check | Device file SHA-256 66C5910E93C96868D6C13D980361D2AE316BB800973DC0279CFCD64DA5D570F1 = approved S1-06 version; mtime 1789638446954; function body byte-identical to baseline |
| Script update approved | Add `ai_monitoring_agent.go` checksum, update `main.go` checksum, raise expected changed files from 30 to 31, keep all other checksums and checks |

## GO-8 implementation — written 2026-09-17, NOT validated
- **Change (`ai_monitoring_agent.go`, +12/−1):** `"net/url"` import; `collectSignals` now calls new helper `agentErrorRateQueryURL(metricURL, orgID)`, which builds `sum by (service) (rate(http_requests_total{org=%q,status=~"5.."}[5m]))` (orgID quoted with Go `%q`, compatible with PromQL double-quoted string escapes) and URL-encodes the whole expression with `url.QueryEscape`. For `org-a` the URL is byte-identical to what the original encoded template intended.
- **New test file `ai_monitoring_agent_query_test.go` (117 lines, stdlib only):** `TestAgentErrorRateQueryURL_NormalOrgID` (exact URL and decoded query), `..._SpecialCharacterOrgIDs` (8 cases: quote matcher injection, backslash+quote, URL delimiters `& # ?`, `% + space`, PromQL syntax, control characters, Unicode, empty; org literal must equal `strconv.Quote(orgID)` and unquote back to orgID), `..._QueryStructure` (single fully encoded `query` parameter, fixed target/path, exact template).
- **Checksums:** `ai_monitoring_agent.go` 10B3BC6B8D12D975B7AEC731648083B08CC81DBEB1F97F2E35B2D8910498B9C8 (39732 bytes); test file D72EEE97B859C982FC1EF0AB6EA4C4A5B65828630A6112A80F9FD98AA23AEDDC (4406 bytes); diff `go-8-9/GO-8.diff` 8D216FDD7684B73E990865DAB3C96FCE50413E484F3CBFA9DA5FCF870E9785C4.
- **Sandbox checks (NOT validation; api-gateway package cannot compile here, RP-2):** gofmt parse OK; test file gofmt-clean; the GO-8 change set is identical before/after gofmt (file keeps its pre-existing non-gofmt lines). Helper copied verbatim + test file in a stdlib-only scratch module: `go vet` exit 0; `go test -v` 3 tests / 8 subtests PASS; `-race` PASS. Mutation checks: tests FAIL against the original format string, the `%%`-only fix (no orgID escaping), QueryEscape with raw quotes, and %q without URL encoding.
- **Not run:** `go build`/`go vet`/`go test` of `./services/api-gateway/` (local script).
- **Scope notes:** one new file beyond the approved "31 files" estimate (tests required a new `_test.go`; adding them to an S1-06 test file would have changed S1-06 checksums), so the script expects **32** changed files. Observed, not changed: `collectSignals`' ALERTS query (line 571) still inserts `%q` without URL encoding; the in-repo query engine accepts only native selectors, so `sum by (...) (rate(...))` is still rejected there (existing finding **F-066**). GO-8 fixes URL construction and escaping, not query-engine support.
- **Status:** applied, **not validated**.

## GO-9 implementation — written 2026-09-17, NOT validated
- **Change (`main.go`, 1 hunk, −260/+0):** deleted lines 6154–6413 (unreachable legacy body of `handleAgentInstallScript`; byte-identical to baseline lines 5991–6250). Active block, its braces and the function's closing brace unchanged; everything outside the removed range byte-identical.
- **Checksum:** `main.go` 4D77D7D13360D991A375C6AF034A6FA1019D6D165EDC121FE42EDE6BA63D88F2 (296451 bytes; S1-06 + GO-9); diff `go-8-9/GO-9.diff` 34A7DE1B0ED2A3696A8A93623C8EF1464208580652B88DD106BD68A33CB19EE5.
- **Sandbox checks (NOT validation):** gofmt parse OK; change set identical before/after gofmt; all packages used by the removed lines are still used elsewhere in `main.go`. Handler extracted verbatim with stub fiber/middleware types: original → `go vet` "unreachable code" (exit 1); GO-9 version → type-checks and `go vet` exit 0 (no missing-return error).
- **Status:** applied, **not validated**.

## Delivery and script update (GO-8/GO-9) — 2026-09-17T10:23:54Z
- Committed with mtime guards (ai_monitoring_agent.go 1789575187402, main.go 1789638446954, script 1789639622320); new test file path confirmed absent immediately before; none rejected. Read-back byte-identical for all six delivered files; `go.mod`/`go.sum` re-checked unchanged (60986B8E…, 748605EA…).
- `S1-06-local-validation.ps1` → BBD1B050C6E756FE6A7C70DCD7C9BCD1E8BC3971B26148841FBC99DEACEF612B: `main.go` expected hash → 4D77D7D1… (S1-06 + GO-9); new `$go8` set (ai_monitoring_agent.go, test file) included in hash checks and the expected-file list; expected changed files **32**. All other checksums and steps unchanged. GO-8 tests run in step 6 (package tests for `./services/api-gateway/`).

---

## S1-06 local validation run 3 (with GO-7, GO-8, GO-9) — 2026-09-17T10:26:41Z → 10:27:28Z
- **Environment (user's Windows PC):** go1.27.1 windows/amd64; GOPROXY=https://proxy.golang.org,direct; GOSUMDB=sum.golang.org; GOFLAGS empty; CGO_ENABLED=0. Script `S1-06-local-validation.ps1` SHA-256 BBD1B050C6E756FE6A7C70DCD7C9BCD1E8BC3971B26148841FBC99DEACEF612B.
- **Results (as reported by the user):**

| Step | Result |
|---|---|
| 1 Integrity before (32/32) | PASS |
| 2 gofmt new S1-06 Go files | PASS |
| 3 go build (servicetoken, middleware, store, api-gateway, processor) | PASS |
| 4 go vet (same packages) | PASS |
| 5a servicetoken tests (-v) | PASS |
| 5b middleware `TestInternalOnly` (-v) | PASS (9 subtests) |
| 5c gateway S1-06 tests (-v) | PASS |
| 6 package tests (-cover) | PASS — servicetoken 96.7%, middleware 7.0%, api-gateway 36.4%; store and processor have no test files |
| 7 S1-08 + S1-09 regression (ingestor, ai-agent) | PASS |
| 8 token values in test output | PASS (0 matches) |
| 9 race detector | NOT RUN (CGO_ENABLED=0) |
| 10 docker compose config | NOT RUN (Docker CLI not installed) |
| 11 Integrity after (32/32) | PASS |

- **GO-7 / GO-9:** build and vet pass locally (their acceptance checks). **GO-8:** its tests are in `./services/api-gateway/` and ran in step 6 (package `ok` with `-count=1`), but not individually listed (no `-v`); a focused `-v` run would give per-test evidence.
- **Cosmetic:** Fiber startup banners in WebSocket tests render as mojibake (UTF-8 box characters in a non-UTF-8 console). No tokens in them (step 8).
- **S1-06 remains NOT validated / NOT production-ready.** Open: race detector (cgo/CI), gosec + govulncheck on S1-06/GO-7/8/9 changes, `helm template` (helm not available; pre-existing chart parse error), compose config with Docker, Postgres comment-delete test (RP-6), P-0 CI, independent validation, staging script run (P-7). A2 and S0 on hold; S1-01 and S1-04 not started.

---

## S1-04 — remove raw SQL execution from the query engine: decisions (approved 2026-09-17)
| Field | Record |
|---|---|
| Approval | User approved D1, D3, D4, D5 and gave D2 instructions (below) |
| Plan | Read-only S1-04 plan of 2026-09-17 (raw SQL paths E1 `POST /query/events`, E2 `POST /query` type `events`/`sql`, E3 tracked stale binary) |

| # | Decision | Approved outcome |
|---|---|---|
| D1 | `/query/events` | **Remove completely**: route, `handleEventsQuery`, `EventQuery`, and every caller-controlled SQL branch (`case "events", "sql"` in `handleUnifiedQuery`). **Amends A1 condition C5** ("preserve the fixed built-in events query"): the built-in events query is removed with the route. |
| D2 | Other items from A1 plan §5.1 | **Not in S1-04**: no internal-token authentication on the query engine, no CORS change, no rejected-request logging. Recorded as separate follow-up security work, each requiring explicit approval. Planning docs updated only where needed to reflect the separation. |
| D3 | Testability | Move route registration into `newApp()`; test the real route inventory and 404s |
| D4 | `X-ObserveX-Org` escaping in query-engine `quotedSQL` (backslash not escaped) | **Separate security fix; not implemented in S1-04** |
| D5 | Stale binary `services/query-engine/query-engine` | Delete it and add it to `.gitignore` |

### Conditions (as given)
1. Verify all approved checksums before editing; stop on unexpected changes.
2. Preserve S1-06, GO-7, GO-8, GO-9, S1-08 and S1-09 changes.
3. Do not modify go.mod or go.sum.
4. Implement the planned tests and staging script.
5. No validation or production-readiness claim.
6. Report all changed files, checksums and deviations.
7. Stop if any repository caller or additional dependency is discovered.

### Follow-up security work created by D2/D4 (NOT approved; each needs explicit approval)
| ID | Item | Source |
|---|---|---|
| FU-S104-1 | Internal-token authentication on query-engine routes (receiver side of P-1; gateway sender already exists from S1-06 D6) | A1 plan §5.1, C9 |
| FU-S104-2 | Query-engine CORS (`AllowOrigins: "*"`) removal / allowlist | F-017, A1 plan §5.1 |
| FU-S104-3 | Structured logging of rejected SQL-bearing requests (now moot for removed routes; revisit with FU-S104-1) | A1 plan §5.1 |
| FU-S104-4 | `X-ObserveX-Org` / `quotedSQL` ClickHouse string escaping in the query engine (D4) | S1-04 plan finding |
| (existing) | Direct ClickHouse exposure (no password, published 8123/9000; Grafana ClickHouse datasource) | F-003 remainder, D-04/D-08 |

## S1-04 implementation — written 2026-09-17, NOT validated
- **Pre-edit verification (10:45:06Z):** 33/33 approved checksums match (S1-06, GO-7, GO-8, GO-9, S1-08, S1-09, GO-1a/1a.1, Makefile); query-engine `main.go`, `go.mod`, `go.sum`, `Dockerfile`, binary match the `version1.zip` baseline; full recursive listing shows only the 32 expected changed files; `main_test.go` and `s1-04-raw-sql.sh` did not exist. Caller search (plan) found no repository caller; no new dependency introduced.
- **Changes delivered (read-back byte-identical, 10:52:06Z):**

| File | Change | SHA-256 |
|---|---|---|
| `services/query-engine/main.go` | Removed Events section, `EventQuery`, `handleEventsQuery`, `case "events", "sql"`, route `POST /query/events`; route registration moved unchanged into `newApp()` (+12/−32) | 1CA27505FA39908CC43CFE925809CA459778513453C17706AD480AE4F13F8A3A |
| `services/query-engine/main_test.go` (new) | `TestS104_RouteInventory`, `TestS104_QueryEventsRemoved`, `TestS104_UnifiedQuery_SQLTypesRejected`, `TestS104_UnifiedQuery_OtherTypesUnchanged`, `TestS104_NativeMetricRoutesUnchanged` (httptest backend stubs; stdlib + existing zap/fiber only) | 3709A2E7194D40598E6A5068B44AC9457EE927822A000BFABED58C39683F2E6B |
| `tests/security/a1/s1-04-raw-sql.sh` (new) | Staging check from inside the network; harmless read-only SQL probes only | 1B83F135F238D0901305589798622B3117C111F4E9CEC904C4E3C5D8AE7B7DE7 |
| `.gitignore` | +`/services/query-engine/query-engine` (S1-06 + S1-04 D5) | 82717698D0F9712A733909C2226DB1BAE0737EF4A42641230E0C61237FB3FD0F |
| `services/query-engine/query-engine` | **To be deleted by the user** (no delete capability in this session); validation script fails until absent | was 19B0BA546C255A69FFD0B6509127CB1AEBD7D40867D214228CD95497581ABCA3 |

- **Unchanged (verified):** root `go.mod` 60986B8E…, `go.sum` 748605EA…; query-engine `go.mod` 8BD3A3DB…, `go.sum` CED6F005…; all S1-06/GO-7/GO-8/GO-9/S1-08/S1-09 files except `.gitignore` (D5).
- **Outputs:** `s1-04/S1-04.diff` (8F6D6C50…), `s1-04/SHA256SUMS-S1-04.txt`, `s1-04/S1-04-local-validation.ps1` (F82A3283…; 35 expected changed files, binary must be absent, static no-SQL-path and repo-caller scans, query-engine build `-o NUL`/vet/tests, go.mod/go.sum unchanged, root regression). `s1-06/S1-06-local-validation.ps1` updated to FCA70CC6… (new `.gitignore` hash, `$s104` set, 35 expected files; nothing else changed).
- **Sandbox checks (NOT validation; Fiber/zap modules cannot be downloaded here, RP-2):** gofmt parse OK; `main_test.go` gofmt-clean; `main.go` change set identical before/after gofmt (file keeps its pre-existing non-gofmt lines); no `/query/events`, `EventQuery`, `handleEventsQuery`, `"sql"` or `observex.logs` left in `main.go`; `main.go` + `main_test.go` type-check and `go vet` pass against **stub** fiber/zap packages (API-shape check only; tests not executed); staging script: `bash -n` OK, PASS against a patched stub, FAIL (exit 1) against an unpatched stub, exit 7 on connection error.
- **Deviations / notes:** (1) `docs/configuration/internal-service-token.md` line "Query engine – Receiver – Planned in S1-04" is now inaccurate (receiver moved to FU-S104-1); not changed because it is an S1-06 file and no doc change was approved. (2) Planning docs amended: `A1_Implementation_Plans.md` §5 amendment note; `11_Remediation_Plan.md` SEC-2 row. (3) The route inventory test uses `app.Stack()` (confirmed present in Fiber v2.52.4 via the gateway binary's symbols) rather than `GetRoutes()`.
- **Status:** implemented, **not validated, not production-ready**. Open: binary deletion, local S1-04 script run, gosec/govulncheck on the query-engine module, CI (P-0; ci.yml query-engine job path is wrong), staging (P-7, SI-4), FU-S104-1…4.

## D-14-E, D-INT-2, D-INT-3, KB-FMT-1 — native SRE intelligence and knowledge catalog: decisions (approved 2026-09-22)
| Field | Record |
|---|---|
| **Decision ID** | D-14-E (amends register D-14), D-INT-2, D-INT-3, KB-FMT-1 |
| **Status** | **Approved** |
| **Date recorded** | 22 Sep 2026 |
| **Implementation status** | Knowledge catalog generated and validated as a data artifact, outside the repository (`kb-catalog-v0.1.1/`). `internal/knowledge` not created; `go:embed` not implemented; no runtime change. **Not production-ready.** |

- **Correction (recorded 2026-09-22 with KB-SEED-2; the wording above is kept unchanged as the historical record):** "validated" above is inaccurate. The checks run at generation confirmed only that the catalog faithfully converted the seed and carried the correct provenance. They did not check either artifact against the schema. Both artifacts were later found to contain **42 schema violations in 29 of the 63 patterns**. These came from YAML representation defects in the v0.1.1 seed and were carried faithfully into the catalog. The artifacts were **not schema-valid**. They are superseded by seed v0.1.2 and its catalog (KB-SEED-2, recorded below).

| # | Decision | Approved outcome (as given) |
|---|---|---|
| D-14-E | AI provider & model strategy — amends register D-14 | ObserveX will not use an external LLM runtime. Future intelligence may use deterministic methods, statistical models, and narrowly scoped in-house models. No hosted LLM API, Ollama runtime, Anthropic API, or equivalent is permitted in the new intelligence architecture. |
| D-INT-2 | Knowledge catalog storage | The SRE knowledge catalog will be embedded into the Go binary using go:embed. The embedded catalog is read-only at runtime. |
| D-INT-3 | First implementation approach | go:embed is the first implementation approach. No database-backed catalog or runtime network retrieval will be implemented. |
| KB-FMT-1 | Catalog format | The catalog format is generated deterministic JSON. Production parsing will use the Go standard library. The production knowledge package will not add a YAML dependency. |

### Authoritative artifacts (as given)
| Artifact | SHA-256 |
|---|---|
| `observex-sre-kb-seed-v0.1.1.yaml` | e0aaa08d340a07a388add8122260b9f22cb591377b7c370800dfaf0b16a61890 |
| `catalog.json` | ecfd1a2e6e015f46d2cc96a4a0d93c11ee4c2a88c9100ba464f95510065bed56 |

- **Superseded (recorded 2026-09-22, KB-SEED-2):** both artifacts above are superseded by seed v0.1.2 (`e0cc03d3…`) and catalog `95aa0bd8…`. They are no longer authoritative. Both remain preserved byte-for-byte unchanged as historical provenance: the seed is still 432,271 bytes and the catalog still 457,723 bytes, with the SHA-256 values above.

### Register amendment
- `03_Engineering_Decision_Register.md` D-14: D-14-E recorded as an amendment and marked current. The option C recommendation and the original approval question are **retained unchanged** as the historical record; option C was not approved.
- The register's header status line is retained; a dated status-update line was added beneath it.

## KB-SEED-2 — corrected knowledge seed v0.1.2 and catalog (approved 2026-09-22)
| Field | Record |
|---|---|
| **Decision ID** | KB-SEED-2 |
| **Status** | **Approved** |
| **Date recorded** | 22 Sep 2026 |
| **Supersedes** | Seed v0.1.1 `e0aaa08d…` and catalog `ecfd1a2e…`, recorded as authoritative in the D-14-E, D-INT-2, D-INT-3, KB-FMT-1 entry above. That entry and its approvals are retained. Its "validated" wording has been corrected in place: a correction note was added and the original text was kept. v0.1.1 remains preserved unchanged as historical provenance. |
| **Implementation status** | The approved artifacts exist outside the repository in `kb-seed-2-proposal/`. They have not yet been moved to authoritative locations; moving them would not change their bytes or hashes. `internal/knowledge` has not been created, `go:embed` is not implemented, and nothing in the repository or runtime has changed. **Not production-ready.** |

### Approved artifacts (as given)
| Artifact | Bytes | SHA-256 |
|---|---|---|
| `observex-sre-kb-seed-v0.1.2.yaml` | 432,375 | e0cc03d3b4a00cfeef8eb34c33c48926c854a0f9d8e884f5b7e5c5312076c8fb |
| `catalog.json` (generated from seed v0.1.2) | 457,121 | 95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767 |

Both hashes were re-verified against the files on disk when this entry was recorded.

| # | Decision | Approved outcome (as given) |
|---|---|---|
| 1 | Seed v0.1.2 | Approved. It contains 51 YAML representation repairs across 32 patterns, 1 explicit structural decision (F11.3) and no substantive knowledge changes. |
| 2 | F11.3 structural decision | F11.3 `affected_systems` stays exactly **one** list item: "any predict-and-act loop — failure prediction, capacity prediction, anomaly-driven remediation". It is not split, rewritten, paraphrased or otherwise altered. This is a structural decision, separate from the YAML repairs. |
| 3 | Catalog | Approved: SHA-256 95aa0bd8…, 457,121 bytes. |
| 4 | Generator | The generator v1.1.0 schema-validation gate is approved as part of the catalog-generation process. The generator must reject schema-invalid input before writing any catalog output. The generator file is `generate_catalog.py`, SHA-256 2ade0cf29ed7755f5012720de3df48b4f35f520bff03a4230a62e234d38b9aad. |
| 5 | Per-pattern `version` fields | **Not bumped.** The 32 affected patterns keep their existing values. This is deliberately left as a separate future decision. |

### What the correction contains
- **51 YAML representation repairs:**
  - 22 telemetry signals that had been cut off at a comma;
  - 11 list items that had been read as objects;
  - 9 bare-year incident dates;
  - 9 list items that had been split at a comma.

  The first 42 of these remove all 42 schema violations, leaving **0** in the seed and catalog.
- **1 structural decision:** F11.3, as in decision 2. It is not a YAML repair.
- **Mechanism:**
  - 104 quote characters were inserted, 52 pairs in all: 51 for the repairs and 1 for the F11.3 decision.
  - 3 metadata edits: the header comment, `metadata.version: 0.1.2`, and `metadata.supersedes` now set to `{0.1.1, e0aaa08d…}`.
  - No wording was added, removed or paraphrased. Pattern IDs and order are unchanged.

### Evidence
- **Proposal:** `kb-seed-2-proposal/PROPOSAL.md`, revised; SHA-256 74d547866ddac89a…. **Seed diff:** `seed-v0.1.1-to-v0.1.2.diff`; SHA-256 1783d1b7….
- **Independent audit:** `kb-seed-2-audit/AUDIT-REPORT.md`; SHA-256 a2ab50be….
  - Its findings:
    - no YAML defects beyond those corrected;
    - 0 schema violations;
    - the generator rejects invalid input without writing output;
    - two regenerations were byte-identical.
  - It classified the proposal NEEDS REVISION for two reasons only: F11.3's structure, and the repair count.
  - Both were resolved before approval: the product owner made the F11.3 decision, and the proposal documentation was corrected. The artifact bytes did not change.
  - The revised proposal was not re-audited.

### Not changed
- **`03_Engineering_Decision_Register.md`:** no existing decision or amendment section covers the knowledge seed or catalog artifacts. The D-14-E amendment concerns the AI-provider decision.
- **Other files:** the v0.1.1 seed and catalog, the v0.1.2 seed, `catalog.json`, `catalog.sha256`, and all repository and application files.

### Open (each needs separate instruction)
- Move the approved files to their authoritative locations.
- Restate the internal/knowledge slice's pinned values for v0.1.2. The slice is not started.
- Issue an erratum for `ObserveX-SRE-Failure-Knowledge-v0.1.0.md`. Its claim of "63 records, 0 violations" is false for v0.1.0 and v0.1.1.
- Update two out-of-date documents:
  - `kb-catalog-v0.1.1/GENERATION.md`;
  - the "NOT APPROVED" status line in `PROPOSAL.md`.
- Log KB-SEED-1.
- Decide whether to bump the per-pattern versions.

## `internal/knowledge` implementation (D-INT-2, D-INT-3, KB-FMT-1, KB-SEED-2) — written 2026-09-22, recording approved; focused local validation PASSED 2026-09-22
| Field | Record |
|---|---|
| **Governing decisions** | Approved earlier; nothing is re-decided here. **D-INT-2:** the catalog is embedded with go:embed and is read-only at runtime. **D-INT-3:** go:embed is the first implementation, with no database-backed catalog and no runtime network retrieval. **KB-FMT-1:** generated deterministic JSON, parsed with the Go standard library, with no YAML dependency. **KB-SEED-2:** seed v0.1.2 `e0cc03d3…` and catalog `95aa0bd8…`. The package is consistent with D-14-E (no external LLM runtime). |
| **Recording** | The product owner approved recording this implementation on 22 Sep 2026. |
| **Status** | Implemented and delivered. **Focused local validation passed** on the user's Windows machine: `go test` and `go vet` for this package only (see below). **Not production-ready.** No callers. *(This row and the heading were updated 2026-09-22 when the local validation was recorded; both previously read "NOT validated locally".)* |

### Delivered to `C:\Users\91832\Documents\observex`
All files are new. They were written 2026-09-22T17:05:22Z–17:05:26Z and read back byte-identical. When this entry was recorded, their sizes and modification times were unchanged and the directory held no other files.

| File | Purpose | Bytes | SHA-256 |
|---|---|---|---|
| `internal/knowledge/catalog/catalog.json` | Byte-for-byte copy of the approved catalog. Not regenerated or re-serialized. | 457,121 | 95aa0bd8aa24d000ee09c4b97cac925f2ba470cad4fead39896d5ffb6ba95767 |
| `internal/knowledge/catalog/.gitattributes` | `catalog.json -text`, so git line-ending conversion cannot alter the embedded bytes | 306 | f3fc22253e690d7f893449d6cf49ba61d9d26c29d49b25f51cc0041ef49e754b |
| `internal/knowledge/doc.go` | Package documentation: scope, provenance, validation, and the stated differences from the schema | 2,907 | bd76be16394a4a013d22e20e97662687f398439e22d344ed43b02aca893534e6 |
| `internal/knowledge/knowledge.go` | go:embed, provenance constants, `Load`, read-only `Catalog` accessors | 4,690 | 7763d2556f3ecf50d30cfc97cea7fa48264b046809eee2285c37be59252eae8f |
| `internal/knowledge/types.go` | Typed model, enum constants, deep copy | 10,673 | 631f69b4ffe759b5bcea4fa4be2034b67b5134060363c6454ce58a464c02e4ef |
| `internal/knowledge/validate.go` | Validation performed before `Load` succeeds | 17,048 | b21e8618472a1e7172079d572aa5bbdc9b96a6138dc2e09ebc0f87ce86c3f6e3 |
| `internal/knowledge/knowledge_test.go` | Tests: load, count, IDs, lookup, filters, copies, provenance | 10,018 | 2490c369dce0e6650494ad9a6ee3b00f7197772437e109795cdde3ca2d1d5a68 |
| `internal/knowledge/validate_test.go` | Tests: rejection of invalid catalogs | 11,198 | 1f2440f07da3ac5e9d4f2cb2d9630660ecdeb4afba08867509df11904eac7ea5 |
| `internal/knowledge/boundary_test.go` | Tests: import boundaries and the embed directive | 7,433 | ca7108eb5e63d7292cc51d16d69769cb97941c20ec841836dfe961a5b478cb27 |

### Implemented package behaviour
- **Embedding.** go:embed of `catalog/catalog.json`. This is the package's only embed directive.
- **`Load()`:**
  - Returns the catalog only if the embedded bytes hash to `95aa0bd8…` and total 457,121 bytes, and every check below passes.
  - Validation runs once per process and the result is cached.
  - On failure, it returns an empty `Catalog` and an error that wraps `ErrInvalidCatalog`.
- **Validation before `Load()` succeeds:**
  - **Syntax:** valid UTF-8 and valid JSON, with no duplicate object keys and no trailing data.
  - **Required keys:** every key the schema requires is present and non-null. `human_authority_required` is also required.
  - **Strict typed decoding:** unknown fields and wrong JSON types are rejected.
  - **Metadata:**
    - `version` must be `0.1.2` and `record_count` must be 63.
    - Source artifact `observex-sre-kb-seed-v0.1.2.yaml`, source version `0.1.2`, and source SHA-256 **exactly** `e0cc03d3b4a00cfeef8eb34c33c48926c854a0f9d8e884f5b7e5c5312076c8fb` (case-sensitive).
    - Schema name `observex-sre-kb-schema-v0.1.0.json`.
    - Supersedes version `0.1.1`, with a distinct, lowercase supersedes hash.
  - **Patterns:**
    - Exactly 63, with unique IDs in the schema's ID format.
    - Each family matches its ID.
    - Schema enums, minimum lengths and minimum item counts hold.
    - Required strings are non-empty.
- **Public API:**
  - `Load`.
  - `Catalog` methods: `Metadata`, `Len`, `PatternIDs` (catalog order), `Pattern(id)`, `ByFamily`, `ByMethodFit`, `ByAutomationClass`.
  - Typed `Family`, `MethodFit` and `AutomationClass`, with constants and `Families()`, `MethodFits()`, `AutomationClasses()`.
  - Provenance constants.
  - Every accessor returns a new slice or a deep copy, so no caller can reach internal slices or maps. `ByAutomationClass` is a knowledge lookup and authorizes nothing.
- **Dependencies.** Standard library only: `bytes`, `crypto/sha256`, `embed`, `encoding/hex`, `encoding/json`, `errors`, `fmt`, `io`, `strconv`, `strings`, `sync`, `unicode/utf8`.
- **Not present:**
  - network, database, os/exec, LLM, Kubernetes, cloud or YAML dependencies;
  - runtime configuration;
  - detection, remediation or policy logic;
  - callers.

### Validation performed in Claude's environment — NOT local validation
**Environment:** the sandbox copy of the repository, on Linux with Go 1.24.7. The module's language version is go1.22, and no toolchain or module download was needed. These results do not validate the user's local copy.

- `go test ./internal/knowledge/`: **25 top-level tests with 57 subtests, 82 results in total, 0 failures.**
- `go vet ./internal/knowledge/`: clean for linux, and clean with `GOOS=windows`. `gofmt`: clean.
- **Import boundaries:**
  - **Allowlist:** production imports are exactly the 12 packages listed above.
  - **Forbidden categories:** no file imports first-party code (services, gateway, remediation/execution, `internal/db`), networking, database drivers, LLM clients, Kubernetes, cloud SDKs or YAML libraries.
  - **Transitive closure:** 91 standard-library packages, none forbidden. `go list -deps` for linux, windows and darwin also found none.
  - **Negative probes:** a temporary `net/http` import, and separately a first-party import, each made all three boundary tests fail. The probe file was removed.
- **Catalog:** the repository copy is byte-identical to the approved artifact (`95aa0bd8…`, 457,121 bytes). The provenance test confirms:
  - source SHA-256 `e0cc03d3…`;
  - the catalog does not contain its own hash;
  - the superseded pre-correction hash is absent.

### Local validation on the user's machine — passed 2026-09-22
Run by the product owner in Windows PowerShell with the local toolchain, and reported here; not observed in this session.

- `go test ./internal/knowledge/` → **PASS**: `ok github.com/observex/platform/internal/knowledge 11.120s`.
- `go vet ./internal/knowledge/` → **clean**, no output. Run twice; clean both times.

**Scope:** these two focused commands for this package only. No full-repository build or test run, and no staging, Docker, race, security-scanning or integration testing. The package remains **not production-ready**.

### Unchanged (verified)
- **Module files:** `go.mod` 60986b8e48523d35… and `go.sum` 748605ea9e60f735…, in both the local folder and the sandbox.
- **Existing files:** no existing repository file was modified.
- **Catalog artifacts:** the v0.1.2 seed and catalog, `catalog.sha256`, and the v0.1.1 seed and catalog.

### Limitations and notes
1. **Not production-ready.** It is not integrated into detectors, processors, agents, remediation or the gateway.
2. **Deliberately not duplicated in the Go loader:** the schema's conditional (allOf) rules and its date and URI format checks. The generator v1.1.0 schema gate enforces these.
3. **Stricter than the schema:**
   - `human_authority_required` is required, so its absence can never read as "false";
   - required strings must be non-empty;
   - `Load` checks the embedded bytes' hash and size.
4. **Superseded hashes in tests.** Test files contain v0.1.0, v0.1.1 and pre-correction hashes only as inputs that must be rejected.
5. **Not addressed:** the existing gateway OLLAMA/Anthropic code paths are unaffected and remain a separate decision. The D-14 guardrails are also not addressed.
6. **Sandbox and local folder differ.** The sandbox copy of the repository differs from the local folder in unrelated files: `docker-compose.yml` is a different size, and `ai-agent-python/` exists only in the sandbox. The package is self-contained, standard-library only and in a new directory, so the difference does not affect it, but local validation is still required.
7. **Where the artifacts are.** The approved seed and catalog files are still in `kb-seed-2-proposal/`. `catalog.sha256` was not copied into the repository; the hash is pinned in code and tests instead.

**Open:** any caller integration needs separate approval.

## F6.1 certificate-expiry detector `internal/detect/certexpiry` — written 2026-09-23; focused local validation PASSED 2026-09-23
| Field | Record |
|---|---|
| **Governing decisions and inputs** | Approved earlier; nothing is re-decided here. **D-INT-2** and **D-INT-3** (catalog embedded with go:embed, read-only at runtime, first implementation), **KB-FMT-1** (deterministic JSON parsed with the Go standard library, no YAML dependency), **KB-SEED-2** (seed v0.1.2 `e0cc03d3…`, catalog `95aa0bd8…`). The design it implements is `ObserveX-F6.1-Detector-Contract-v0.1.0.md`, which in turn follows the read-only architecture note `ObserveX-First-Intelligence-Layer-Design-v0.1.0.md`. Consistent with **D-14-E**: no LLM of any kind is involved. |
| **What was built** | A pure deterministic detector for catalog record **F6.1, "TLS certificate expiry"**, implementing its `leading_indicators[0]` — "notAfter minus now, per certificate, measured on the live production endpoint" (`method_fit: deterministic-rule`, `prediction_class: P1-deterministic`). |
| **Status** | Implemented and delivered. **Not integrated into the processor, the gateway or any other runtime path. No caller exists. Not production-ready.** |

### Delivered to `C:\Users\91832\Documents\observex`
All files are new; no existing file was modified. Written 2026-09-23, read back byte-identical, no line-ending conversion.

| File | Purpose | Bytes | SHA-256 |
|---|---|---|---|
| `internal/detect/certexpiry/certexpiry.go` | `Observation`, `Params`, `Finding`, sentinel errors, `Evaluate`, and the package doc recording the safety boundary | 8,413 | 8a4d58d6b0937736f613ffb897f31e0b95ed789a27ab58e1cc86eaae84b37e0f |
| `internal/detect/certexpiry/certexpiry_test.go` | Decision, boundary, unusable-input, representation and determinism tests | 10,683 | 09d198b52fe6c08aa6ec18500d2f68752d569e969f11dba80a3de306dd2da348 |
| `internal/detect/certexpiry/knowledge_test.go` | Catalog-binding tests and the import-boundary tests | 9,771 | af0ced9b58f9ad5b33a4a6d3250465e55cc4af9c6defbf6f00e9e6e7616cde77 |

### Implemented behaviour
- **Signature:** `Evaluate(now time.Time, obs Observation, p Params) (Finding, bool, error)` — three outcomes: no finding; finding with evidence; unusable input.
- **Rule:** a finding is produced when `NotAfter.Sub(now) <= Horizon`. The horizon boundary is **inclusive**, and an already-expired certificate (negative remaining time) is inside every horizon.
- **No default horizon.** `Params.Horizon` and `Params.MaxObservationAge` are caller-supplied and must be greater than zero; the zero `Params` cannot evaluate. The processor's existing `daysLeft < 7` rule is **not** inherited, and a test asserts that a certificate six days out does not fire without a horizon.
- **Unusable input** returns one of eight sentinel errors and never a finding: zero `NotAfter`; zero `ObservedAt`; observation newer than the evaluation time; observation older than `MaxObservationAge` (exactly at the bound is accepted); empty endpoint; non-positive horizon; non-positive maximum age; and zero evaluation time.
- **Evidence carried:** observed `NotAfter`, `ObservedAt`, `EvaluatedAt`, derived `Remaining` and `ObservationAge`, `Endpoint`, `CheckID`, and four provenance fields (`PatternID`, `PatternVersion`, `CatalogSHA256`, `PredictionClass`) copied from the approved catalog. Timestamps are normalised to UTC with monotonic readings stripped.
- **Not carried:** no severity, no priority, no confidence score, no action, no remediation and no automation class. A test asserts the exact field list, so adding such a field fails the build; another test asserts that F6.1's documented automation class never appears in a finding.
- **Constraints held:** no I/O, no network, no TLS dialling, no `time.Now`, no configuration or environment reads, no persistence, no global mutable state, no goroutines, no logging. Imports are `errors`, `time` and `internal/knowledge` only.

### Deviation from the contract, recorded
The contract did not specify behaviour for a zero evaluation time. The smallest safe choice was taken — refuse with `ErrNoEvaluationTime` — because a zero `now` would otherwise place every certificate far beyond any horizon and silently suppress findings. It is documented in the package source and covered by a test.

### Validation in Claude's environment — NOT local validation
Sandbox copy of the repository, Linux, Go 1.24.7, module language go1.22. No dependency was downloaded.

- `go test ./internal/detect/certexpiry/`: **17 tests with 18 subtests, 35 results, 0 failures.** All 25 cases of the contract's test matrix are covered, including the inclusive horizon boundary (case 3) and the inclusive freshness bound (case 13).
- `go vet` and `gofmt`: clean.
- **Import boundary exercised:** a temporary `crypto/tls` import, and separately a first-party import, each made all three boundary tests fail; the probe file was removed. The dependency closure is 92 packages with no forbidden package in it.

### Local validation on the user's machine — passed 2026-09-23
Run by the product owner in Windows PowerShell with the local toolchain and reported here; not observed in this session.

- `go test ./internal/detect/certexpiry/` → **PASS**: `ok github.com/observex/platform/internal/detect/certexpiry 9.745s`.
- `go vet ./internal/detect/certexpiry/` → **clean**, no output.
- `gofmt -l (Get-ChildItem .\internal\detect\certexpiry\*.go)` → **no output**: all files formatted.

**Scope of this validation:** these three focused commands for this package only. No full-repository build or test run, and no staging, Docker, race, security-scanning or integration testing.

### Unchanged (verified)
- `go.mod` 60986b8e48523d35… and `go.sum` 748605ea9e60f735… — no dependency added.
- `internal/knowledge` (all nine files, catalog `95aa0bd8…`), the v0.1.2 seed, `catalog.sha256`, and the v0.1.1 artifacts.
- The processor, the gateway, remediation and execution code, the confidence logic, incident persistence, tenancy and alerting. The existing SSL synthetic check behaves exactly as before.

### Open — observation-layer questions, none resolved by this slice
1. **Horizon ownership.** F6.1 defines no numeric threshold; its escalation trigger says "within **the defined** expiry horizon". Who supplies the value, and whether it is global, per check or per org, is undecided. Until it is, the detector has no caller.
2. **Live certificate observation.** Nothing currently emits `notAfter`. `runSSLCheck` computes days-remaining and discards it. Producing the observation is a separate slice that touches an existing file.
3. **Already-expired certificates cannot be observed.** The existing probe verifies the chain, so an expired certificate fails the handshake and `notAfter` is never read — exactly the case the pattern describes. Addressing it is a security-relevant design decision.
4. **Leaf versus chain.** The probe reads `PeerCertificates[0]`; the catalog's `verification` asks for the whole chain.
5. **Single vantage point.** The catalog asks for multiple external vantage points and per-region checks; the probe runs once from the processor, and the `locations` column of `synthetic_checks` is not selected.
6. **Tenancy.** The synthetic-check query selects neither `org_id` nor `namespace`, so a finding cannot yet be attributed to a tenant.

**Open, other:** integration into any runtime path needs separate approval; the processor's `daysLeft < 7` rule is untouched and unreconciled.

---

## OD-01 — Approval log of record and its location (GOV-1a, GOV-2) — approved 2026-09-28

| Field | Record |
|---|---|
| **Decision IDs** | GOV-1a, GOV-2 (labels from `docs/governance/owner-decision-preparation/Owner-Decision-Sheet.md` §1 and §5) |
| **Answer (as given)** | "GOV-1a: Option A. G4 (session store observex-plan/07_Approval_Log.md, 75,923 bytes, fb2beb0c485120cb) is the approval log of record." "GOV-2: Option C now, D later. The log of record lives at docs/governance/record/07_Approval_Log.md. Seed it as a byte-exact copy of G4, verified by SHA-256. When the repository exists (RP-1), this folder is committed and versioned there. All other copies are 'not of record'. Do not edit or delete them. List them with their hashes in docs/governance/record/NOT-OF-RECORD.md." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Approver** | akash (owner and sole approver) |
| **Instruction to append** | The owner's message of 2026-09-28 is the explicit instruction to create this log of record and append to it. It is the separate instruction that the standing rule "Do not modify 07_Approval_Log.md unless explicitly instructed separately" requires. |
| **Seed** | This file's first 75,923 bytes are the unchanged G4 (`observex-plan/07_Approval_Log.md`, SHA-256 `fb2beb0c485120cb9ab9eae7061c5f51758cc3e668763b949e8ecf703497b2c8`). Everything after byte 75,923 was appended from 2026-09-28. The seed is verified in `MANIFEST.md`. |
| **Rule from now on** | Append-only. No earlier entry is edited or removed. Corrections are dated notes that keep the original text. |
| **Not of record** | Every other copy of `07` (including G3 in the project's `Claude outputs\` and G4 itself) is "not of record" and unchanged: `NOT-OF-RECORD.md`. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-02 — Remediation plan of record (GOV-1b) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "GOV-1b: Option A. G6 (11_Remediation_Plan.md, 9f961d90a01f265c, with the amended SEC-2 row) is the plan of record. Copy it byte-exact to docs/governance/record/." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Record file** | `docs/governance/record/11_Remediation_Plan.md`, SHA-256 `9f961d90a01f265c1fe275c6db70740f604cdd8241a2d1a379c1bda4b3c3252e`, 19,395 bytes |
| **Not of record** | G5 (`Claude outputs\11_Remediation_Plan.md`, `812c26b2…`), unchanged |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-03 — Planning records imported as unversioned originals (GOV-1c) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "GOV-1c: Option A. Import these byte-exact into docs/governance/record/ as unversioned originals, listed with their hashes in MANIFEST.md: G7 (05) and G8 (06); A1_Implementation_Plans.md, 02_Implementation_Roadmap.md, 08, 09 and 10 from the session store. A7 and A8 keep their current definitions. The 009 naming collision is noted, with no number chosen." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Files** | `05_Stabilization_Plan.md`, `06_Decisions_Requiring_Approval.md`, `A1_Implementation_Plans.md`, `02_Implementation_Roadmap.md`, `08_Staging_Readiness_Checklist.md`, `09_Staging_Inventory_Form.md`, `10_Remote_Validation_Evaluation.md`. Hashes in `MANIFEST.md`. |
| **Effect on `06`** | Importing `06` does not tick any of its answer boxes. Decisions are recorded only in this log. |
| **`009` collision** | `05` S1-12 plans `009_bootstrap_admin.sql`; `009_f61_tls_certificates.sql` exists. Noted; **no number chosen**. Deferred with A8 (OD-18). |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-04 — Decision register of record (GOV-1d) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "GOV-1d: Confirmed. 03 (81441f356e5ba4f9) is the register of record. Copy it byte-exact to docs/governance/record/." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Record file** | `docs/governance/record/03_Engineering_Decision_Register.md`, SHA-256 `81441f356e5ba4f9866cf372cd544680d9d1fc45cfd60e238b3e35d291f39100`, 27,137 bytes. The register still states "All decisions are **PROPOSED**" except those recorded as approved in this log. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-05 — Owner facts F-1 … F-7 — confirmed 2026-09-28

| Fact | Answer (as given) | Kind |
|---|---|---|
| F-1 | "There are NO persistent installations. ObserveX has only run on throwaway local environments. No database holds data that must be kept." | Fact confirmed by the owner |
| F-2 | "No existing database was built from a hand-edited 001 or 005." | Fact confirmed by the owner |
| F-3 | "Supported PostgreSQL = 16 only, for new installs, until D-20 says otherwise." | Policy |
| F-4 | "The seeded admin (admin@observex.io) is not used or relied on by any existing database, person or integration." | Fact confirmed by the owner |
| F-5 | "There are no persistent Helm releases, so no existing install has a password configuration to migrate." | Fact confirmed by the owner |
| F-6 | "The superuser password is separate from the application user's. ObserveX workloads never connect as superuser. db-monitor, if ever supported, uses a least-privilege monitoring role (pg_monitor), never superuser." | Policy |
| F-7 | "Only akash may authorize any migration, adoption run or migration Job against a persistent database. Each run needs: a verified backup and a tested restore; a dry-run count; its own approval-log entry. The migration Job stays disabled everywhere." | Policy |

| Field | Record |
|---|---|
| **Status** | **Confirmed / Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Evidence** | The owner's statement of 2026-09-28. No separate evidence artifact was supplied. |
| **Where else recorded** | `docs/governance/owner-decision-preparation/Owner-Facts-Questionnaire.md` |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-06 — A1 condition C13: scope and review condition (GOV-3) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "GOV-3 (C13): Scope (b): C13 blocks presenting A2–A10 for approval. Keep it (i). Preparation documents labelled 'Proposed' are allowed. Approval requests are not. The review is satisfied only by the evidence in Owner-Decision-Sheet §6.5 items 1–5, in the P-0 evidence format, plus my dated review entry. Waiver: C10 (staging) is waived for the C13 review only. C10 and C11 still apply before any production or production-ready claim." |
| **Status** | **Approved.** **C13 remains in force and is NOT satisfied.** |
| **Date recorded** | 28 Sep 2026 |
| **Required for satisfaction** | Decision sheet §6.5 items 1–5: (1) a dated owner review entry in this log; (2) per A1 item, an implementation record (files, SHA-256) and results against C6, C7 and C11 in the P-0 evidence format; (3) the owner's statement on the A1 holds (OD-07); (4) resolution of GOV-4 for S1-01 and S1-08 (OD-08, OD-09); (5) the scope statement (this entry). |
| **Waiver** | C10 waived **for the C13 review only**. C10 and C11 still apply before any production or production-ready claim. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-07 — Ratification of A1 implementation authorization (S1-04, S1-06, S1-09) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "I ratify the A1 implementation work recorded on 17 Sep (S1-04, S1-06, S1-09) as authorized by my approvals of the S1-04 decisions and the S1-06 plan v2. From 17 Sep, the A1 holds are treated as lifted for those three items only. This ratifies authorization, not validation: they remain 'not validated'." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Holds affected** | The 16 Sep holds: the A1 prerequisites rule, the RP-1 constraint "No A1 work resumes until the baseline is confirmed", and the P-0 clarification "Holds". Treated as lifted from 17 Sep **for S1-04, S1-06 and S1-09 only**. |
| **Not covered** | S1-01 (authorized separately, OD-08) and S1-08 (implementation recorded in OD-09; this entry does not ratify its authorization). |
| **Validation status** | Unchanged: S1-04, S1-06 and S1-09 remain **not validated, not production-ready**. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-08 — Disposition of unrecorded work (GOV-4) — approved 2026-09-28

| Item | Answer (as given) | Recorded effect |
|---|---|---|
| S1-01 | "not implemented; the Claude outputs files are unapplied drafts. Implementation is authorized under A1 (task 4 below)." | Drafts listed in `UNAPPLIED-DRAFTS.md` §3. Implementation authorized under A1, with conditions C6, C7 and C11 |
| S1-03 | "NOT APPROVED (A3) and NOT APPLIED. Quarantine it: record every file with its hash in docs/governance/record/UNAPPLIED-DRAFTS.md. Do not apply, move or delete anything." | Quarantined: `UNAPPLIED-DRAFTS.md` §1 |
| SEC-6b | "not approved, not applied. Quarantine it the same way." | Quarantined: `UNAPPLIED-DRAFTS.md` §2 |
| S1-08 | "in the repository. Record an implementation entry with file hashes, status 'implemented, not validated'." | OD-09 |
| QE-DEP | "tooling scripts only; archived. No product change." | Listed in `UNAPPLIED-DRAFTS.md` §4 as archived tooling |

| Field | Record |
|---|---|
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-09 — S1-08 implementation record (A1) — recorded 2026-09-28; implemented, NOT validated

| Field | Record |
|---|---|
| **Basis** | OD-08: "Record an implementation entry with file hashes, status 'implemented, not validated'." |
| **Scope** | ClickHouse string escaping in the ingestor (F-012): `esc` now escapes backslashes as well as single quotes, in one pass |
| **Status** | **Implemented, NOT validated, not production-ready** |
| **Date recorded** | 28 Sep 2026. The files carry device modification times of 2026-09-17 (≈06:26 UTC). |

| File | Change | Before (version1.zip) SHA-256 | Now SHA-256 | Bytes now |
|---|---|---|---|---|
| `services/ingestor/main.go` | `esc` now uses a single-pass `strings.NewReplacer` that escapes backslash and single quote, plus a comment. Verified by diff against `version1.zip`: only these lines differ | `61131b637a514185eddfb9e3ca7c5490ccd3fcd828154a48e2df1991b1711a4d` | `c75d6c309da29a8ad22fe542763c903a7e39bb44f46aec26180beca22973a5b0` | 46,996 |
| `services/ingestor/clickhouse_escape_test.go` | New test file | — | `81e0d40db3f3218a6ec911e475c65eed3e1300824e44864e5e527e447176fa6e` | 9,309 |

| Field | Record |
|---|---|
| **Diff artifact** | `Claude outputs\S1-08.diff`, SHA-256 `5535f24a59bca782954f7f73c32342574bf2e11e735b36a1f806bb0f9cbf70aa` |
| **Evidence so far** | S1-06 local validation run 3, step 7 "S1-08 + S1-09 regression (ingestor, ai-agent) — PASS" (as reported by the user, 2026-09-17). No other validation recorded. |
| **Open** | Race detector, gosec/govulncheck, C6/C7 results in the P-0 evidence format |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-10 — Hosted repository baseline (RP-1) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "RP-1: No hosted repository exists today. Decision: create a private GitHub repository (task 2), using RP-1 option (a): Commit 1 = the exact contents of version1.zip (SHA-256 87351b55…b4d6), verified with version1_zip_git_blob_manifest.txt and compare_baseline.sh. Tag it baseline-version1. Commit 2 = the current project folder, tagged baseline-2026-09-28. It follows .gitignore and excludes: 'Claude outputs\'; .env and any file with secret values; build outputs." |
| **Status** | **Approved.** RP-1 is no longer "Pending clarification". |
| **Date recorded** | 28 Sep 2026 |
| **Hosted repository today** | None exists (owner statement) |
| **Push** | Not authorized in this step. Push steps are handed to the owner; GitHub Actions must be disabled before the first push (the CI decision RP-3 is open). |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-11 — Canonical baseline acceptance (GOV-5) — approved with conditions 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "GOV-5: Accept with conditions. The canonical baseline is tag baseline-2026-09-28, with baseline-version1 as its audit ancestor. Acceptance approves no unapproved change within it. F6.1 Increments 1–3, MIG-1 and D-03 remain PROPOSED. The quarantined drafts are not part of it." |
| **Status** | **Approved with conditions** |
| **Date recorded** | 28 Sep 2026 |
| **Conditions** | (1) No unapproved change within the baseline is approved by this acceptance. (2) F6.1 Increments 1–3, MIG-1 and D-03 remain PROPOSED. (3) The quarantined drafts (`UNAPPLIED-DRAFTS.md`) are not part of the baseline. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-12 — Product charter direction (CH-1, CH-2) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "CH-1: Adopt the product charter as the planning direction. This approves no architecture choice. D-14-E (no external LLM runtime) is binding on all AI work." "CH-2: Charter Phases 0–5 are the successor planning structure. 02 Stages 1–7 are kept as a reference, with a mapping table." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Charter** | `docs/governance/proposed-baseline/ObserveX-Product-Charter-Full-Stack-Observability.md` (SHA-256 `7a398e62335ca5f1…`), unchanged. Its §6 maps Phases 0–5 to `02` Stages 1–7. |
| **Not approved** | Any architecture choice. D-01 … D-22 remain PROPOSED except D-14-E. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-13 — No secrets in logs or output (AQ-8) — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "AQ-8: Adopt a broad rule: never log, print or send any secret value or fragment. That covers passwords (including generated ones), DSNs containing credentials, tokens, keys and password hashes. S1-12 as written must be redesigned before A8 is presented." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Relation to earlier rules** | Broadens P-1 condition 3 ("Never log token values", internal-token scope), which remains in force. |
| **Consequence** | S1-12 as written (a generated password printed once to logs) conflicts with this rule and must be redesigned before A8 is presented. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-14 — D4 connection-string exposure: processor and gateway scope — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "D4, processor (M9a): in scope. The fix is authorized (task 5)." "D4, gateway (M9b): I authorize a security review and reproduction in a disposable, network-isolated environment. The fix scope will be decided on that evidence." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Processor fix conditions (as given in task 5)** | Build the processor's database connection without string-composing unescaped passwords; never log the DSN or any fragment of it; tests with passwords containing `@ : / ? # % &` and no external DNS; touch the processor only; record SHA-256 before and after. |
| **Gateway** | Review and reproduction only, in a disposable, network-isolated environment. **No gateway fix is authorized.** |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-15 — P3 PostgreSQL credential ownership — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "P3: Option 1. The PostgreSQL sub-chart reads the ObserveX Secret (auth.existingSecret plus secretKeys), with separate admin and user keys per F-6. Implement it only after the OPS-1 current-code run confirms the mismatch (runbook §9 W-D)." |
| **Status** | **Approved with condition** |
| **Date recorded** | 28 Sep 2026 |
| **Condition** | Implementation only after the OPS-1 current-code run confirms the mismatch (runbook §9, step W-D). **Not implemented today.** |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-16 — db-monitor support status — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "db-monitor: NOT part of the supported install. Gating its Deployment behind an enabled flag (default false) is authorized, together with the P3 change, after the first OPS-1 run." |
| **Status** | **Approved with condition** |
| **Date recorded** | 28 Sep 2026 |
| **Condition** | The gating change is made together with the P3 change, after the first OPS-1 run. **Not implemented today.** The db-monitor credential contract stays deferred (OD-18). |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-17 — OPS-1 external disposable environment — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "OPS-1 environment: I authorize one external disposable environment. It is a throwaway VM, or kind on Docker Desktop on my PC, with egress to container registries, Helm repositories and the Go proxy. Throwaway PostgreSQL only. Randomly generated test secrets, never logged. Destroy it afterwards. Run the CURRENT code first (A–E, CNI enforcement proof, W-D). Re-run after approved changes. Store the evidence in docs/governance/evidence/ops-1/<UTC date>/." |
| **Status** | **Approved** |
| **Date recorded** | 28 Sep 2026 |
| **Limits** | One environment; throwaway PostgreSQL only; test secrets never logged; destroyed afterwards. No persistent or production database. The migration Job runs only against the throwaway database in that environment. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-18 — Items deferred until the C13 review is recorded — approved 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "DEFERRED until the C13 review is recorded: A7, AQ-2, the 008/009 numbering, D-03, MIG-1, MIG-1a Q1–Q4, A8 and AQ-4 to AQ-7/AQ-11, PROTECTED-FILES.patch, and the db-monitor credential contract. F-1 is now answered (fresh installs only), which will simplify AQ-2 when it comes up." |
| **Status** | **Approved (deferral)** |
| **Date recorded** | 28 Sep 2026 |
| **Effect** | None of these items is approved or presented. MIG-1 and MIG-1a remain PROPOSED — UNRESOLVED. `PROTECTED-FILES.patch` remains unapplied. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-19 — Standing restrictions (still not authorized) — recorded 2026-09-28

| Field | Record |
|---|---|
| **Answer (as given)** | "STILL NOT AUTHORIZED: Anything against a persistent or production database. Enabling the migration Job. Applying PROTECTED-FILES.patch. Editing the Makefile, ci.yml or cd.yml. Changing migrations 001–009. Presenting A2–A10 for approval. Deleting or moving any existing file. Printing any secret value." |
| **Status** | **Recorded restriction** |
| **Date recorded** | 28 Sep 2026 |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.

---

## OD-24 — Build-only compile check authorized — approved 2026-09-29

| Field | Record |
|---|---|
| **Answer (as given)** | "I want to build the code now, without running tests. This is a compile check for development and demos only. It is NOT validation evidence: it does not count toward the A1 review (C13); it does not count toward OPS-1; it does not count toward any 'production-ready' claim (C11). C13 stays in force and not satisfied. Tasks 3–6 from my earlier instruction (A1 evidence, S1-01, D4 fix, OPS-1) are PAUSED until I say to resume. Do not change any code." |
| **Status** | **Approved** |
| **Date recorded** | 29 Sep 2026 |
| **Scope** | (1) Build in Claude's workspace from the project folder as it is (baseline tag `baseline-2026-09-28`): `go build ./...`, then one binary per main package under `services/` into a folder outside the project; the web app (`npm install`, `npm run build`) in a copy of `frontend/`, never in the project. (2) A PowerShell script for the owner's PC, `Claude outputs\build-tools\build-only.ps1`, that builds the same way and stops if the project folder changes. (3) Build failures are reported, not fixed. |
| **Not validation evidence** | Does not count toward the A1 review (C13), OPS-1, or any production-ready claim (C11). **C13 remains in force and is not satisfied.** |
| **Paused** | Tasks 3–6 of the owner's instruction of 2026-09-28: the A1 evidence pack, S1-01, the D4 processor fix, OPS-1. Paused until the owner says to resume. **No code change.** |
| **Still not authorized** | Any code change; tests of any kind (`go test`, `go vet`, `-race`, gosec, govulncheck); anything against a persistent database; Docker, Helm or cluster work; changes to the Makefile, `ci.yml`, `cd.yml` or migrations; pushing; printing any secret value. |
| **Numbering note** | This log contains no entries OD-20 … OD-23. The ID OD-24 was assigned by the owner. |

Drafted by Claude (Cowork) as decision advisor; approved by owner akash.
