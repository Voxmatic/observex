# 10 — Evaluation: Claude-Hosted Development + CI/Remote Validation

**Question:** Can development and initial validation happen in Claude's environment, with CI or remote staging handling Docker-based integration and staging tests, so nothing is installed on your Windows machine?

**Short answer:** **Yes, with three conditions:**
1. Allow Go module and vulnerability-database hosts in this environment's egress settings.
2. Provide a Docker-capable CI runner that you own.
3. Agree how code moves from here into your repository.

Until condition 1 is met, I can't compile or test any Go code. That's all of A1.

**Evidence basis:** every capability claim below comes from commands run in this sandbox on **2026-09-16 (15:36–15:47 UTC)**. Nothing was changed in the codebase, S0 wasn't applied, and A1, A2 and S0 remain on hold.

---

## 1. Checks I can execute myself

### 1a. Available now (verified)
| Check | Tool / version found | Evidence |
|---|---|---|
| Frontend install, type-check, production build | Node 22.22.2, npm | Previously run: `npm ci`, `tsc --noEmit`, `npm run build` all exit 0 |
| Frontend dependency audit | `npm audit` (registry.npmjs.org reachable, HTTP 200) | Previously run: 7 vulnerabilities (3 high) |
| Python agent tests, lint, dependency audit | Python 3, pytest, ruff 0.15.11, pip-audit (pypi.org reachable, HTTP 200) | Previously run: 20 passed; pip-audit advisories |
| RUM SDK tests | `node --test` | Previously run: 28/28 pass (with S0 test fix, not applied to your repo) |
| Go source parse and format check | Go 1.24.7 stdlib (`gofmt -e`, `gofmt -l`) | No parse errors; 46 of 57 files unformatted |
| Go tests that use only the standard library (`tests/` module) | `go test` | Previously run: pass (these don't test product code) |
| **Real PostgreSQL 16 server (no Docker)** | `/usr/lib/postgresql/16/bin/postgres` 16.13 | `initdb` + `pg_ctl start` + `select version()` succeeded, 15:4x UTC |
| **Schema checks: applying migrations to a real Postgres** | psql 16.13 | Run at 15:47 UTC; **found a new critical defect (§6)** |
| Static review, test design, exploit-script authoring, docs, Helm/Compose YAML review | — | — |
| Patch generation (`git diff`/`format-patch`) and `git apply --check` | git | Used for S0 |

### 1b. Available **after** Go egress is allowed (not verified yet)
| Check | Needs |
|---|---|
| `go build ./...`, `go vet ./...` for all Go modules | `proxy.golang.org`, `sum.golang.org` |
| Go unit tests, `-race`, coverage | same |
| `golangci-lint` (binary v2.5.0 already present; needs modules) | same |
| `gosec` (install via `go install`) | same |
| `govulncheck` | same **+ `vuln.go.dev`** (currently blocked, 403) |
| **Go integration tests against Postgres 16** (S1-01 SAML, S1-06 route/authorization, incident scoping) | same + local Postgres (available) |
| Query-engine and ingestor tests with a stubbed ClickHouse (`httptest`) (S1-04, S1-08 escaping) | same |
| Rollback selection tests with the Kubernetes fake client (S1-09) | same |

---

## 2. Checks blocked here

| Blocked check | Cause (verified) | Can it be unblocked here? |
|---|---|---|
| Any Go compile, vet, test, lint or security scan | `proxy.golang.org` → 403; `sum.golang.org` → 403 | **Yes**, via your egress allowlist |
| `govulncheck` vulnerability data | `vuln.go.dev` → 403 | **Yes**, via allowlist |
| Container image builds | Docker CLI present, **no Docker daemon** (`/var/run/docker.sock` missing); Docker Hub and ghcr.io → 403 | No (no daemon) → CI |
| kind cluster, Helm install, NetworkPolicy, ingress tests | No daemon; `kind`, `kubectl`, `helm` not installed; `get.helm.sh`, `dl.k8s.io` → 403 | No → CI |
| Static Helm checks (`helm lint`, `helm template`, `kubeconform`) | Binaries not installed; download hosts blocked | Only if `get.helm.sh` (and a kubeconform source) are allowlisted; otherwise CI |
| Real ClickHouse 24.3 (S1-08 round-trip, S1-04 events query against a real server) | No Docker; `builds.clickhouse.com`, `packages.clickhouse.com`, GitHub release downloads → 403 | No → CI |
| Test identity provider (Keycloak) for the legitimate SAML login (SD-4) | No Docker | No → CI |
| Real rollback in a disposable namespace (SD-5) | No cluster | No → CI |
| Compose stack start-up | No Docker | No → CI |
| Machine resources for heavy runs | 2 vCPU, 7 GiB RAM here | Adequate for unit and Postgres tests only |

**Not used without your explicit approval:** `github.com` is reachable, so `GOPROXY=direct` could fetch some modules straight from GitHub. That route bypasses the blocked module proxy and checksum database, so I **won't use it** unless you approve it separately. It isn't recommended: checksum verification is part of supply-chain safety.

**Hosts to allowlist (minimum for Claude-side Go validation):**

| Host | Needed for |
|---|---|
| `proxy.golang.org` | Go module downloads |
| `sum.golang.org` | Checksum verification |
| `vuln.go.dev` | govulncheck vulnerability data |
| *Optional:* `get.helm.sh` | Static Helm linting here instead of CI |

---

## 3. Minimum CI / remote infrastructure

Your Windows machine needs **only a browser**, to approve runs and download logs.

| # | Component | Minimum | Purpose | Status |
|---|---|---|---|---|
| R-1 | Git repository hosting the ObserveX source | Existing or new private repo | Branch per A1 fix (condition C1); source of truth for CI | **Pending: unknown where code is hosted** |
| R-2 | CI service with Linux runners **that have Docker** | e.g. GitHub Actions `ubuntu-latest`, GitLab CI Docker runner, or self-hosted Linux VM | Independent validation (P-0 condition 2) and disposable kind staging (SD-1) | Pending |
| R-3 | Runner size | **Estimate, to be measured on the first run:** ≥ 4 vCPU, ≥ 8–16 GiB RAM, ≥ 30 GiB disk for kind + Postgres + ClickHouse + Keycloak + ObserveX services | Disposable staging fits in one job | Pending |
| R-4 | Runner egress | Go proxy/sumdb/vuln DB, container registries (Docker Hub; ghcr/quay as used), Helm/kind downloads | Build and pull images | Pending |
| R-5 | Job-level approval gate | Protected environment with required reviewer (e.g. GitHub "Environments") | **Per-run permission for the real rollback test (SD-5)**, recorded in `09` §3 | Pending |
| R-6 | Artifact storage | CI artifacts, retention ≥ 90 days *(assumption to confirm)* | Evidence retention | Pending |
| R-7 | Secrets policy | **No production secrets in CI.** All tokens generated per run inside the job; synthetic data only (SD-2) | Isolation (SI-7) | Pending |
| R-8 | Isolation | Ephemeral runners, or self-hosted runners with no network route or credentials to production | SI-7 | Pending |

### Proposed CI jobs (to be written as part of A1, after approvals)
| Job | Trigger | Contents | Docker? |
|---|---|---|---|
| `go-validate` | Every PR | `go build`, `go vet`, `go test -race`, `golangci-lint`, `gosec`, `govulncheck` for every Go module | No |
| `integration` | Every PR | Postgres 16 + ClickHouse 24.3 service containers; `go test -tags=integration` | Yes |
| `baseline` | Every PR | Frontend build + audit, pytest, RUM tests (keeps pre-existing failures separate from new ones) | No |
| `kind-staging-a1` | Manual or on release candidate | kind cluster → build images from branch → Helm install with generated secrets → ingress with `/api/auth/sso/*` blocked → Keycloak (isolated) → fixtures (SF-1/2/3/4) → `tests/security/a1/*.sh` → log collection + token-leak grep → teardown | Yes |
| `kind-rollback-a1` | **Manual, protected environment approval required per run** | Sample app with ≥ 3 revisions in a disposable namespace; AI agent limited to that namespace; real rollback; verify revision N-1 | Yes |

---

## 4. Evidence collection and sharing

### 4.1 Evidence record (same format in both environments; matches P-0 and SD-3)
Every check produces one record:
```
check_id:        A1/S1-06/go-test
environment:     claude-sandbox | ci:<provider>/<runner> | kind-staging
commit:          <git sha of the branch under test>
command:         go test -race ./internal/middleware/...
tool_versions:   go1.24.7; gosec x.y; …
started_utc:     2026-..-..T..:..:..Z
exit_code:       0
summary:         42 passed, 0 failed, 0 skipped
artifacts:       evidence/<run-id>/<check_id>.log (full output)
```

### 4.2 Flow
| Step | Claude sandbox | Your CI / kind staging |
|---|---|---|
| 1. Produce | Wrapper script runs each command and writes the record plus full log under `evidence/<run-id>/` | CI step writes the same record format and uploads `evidence/` as an artifact (JUnit XML where available) |
| 2. Share | Evidence bundle sent to you as files in this chat | You attach the artifact zip or paste the records into chat. **Optional, separate approval:** a read-only CI token so I can fetch logs via API (`api.github.com` is reachable from here). |
| 3. Record | I transcribe results into `07_Approval_Log.md` per check, linking the artifact, and keep **pre-existing** failures separate from **new** ones | — |
| 4. Claim rule | A check is "passed" **only** when its record shows exit code 0 and matching summary counts. Missing record = **not validated**. | Same |
| 5. Secrets | Logs never contain token values; CI masks secrets; the token-leak grep result is itself recorded as evidence | Same |

### 4.3 Code handoff from Claude's environment to your repository
| Option | How it works | Trade-off |
|---|---|---|
| (a) Patch files | One `git format-patch` series per fix (C1), delivered in chat; your team applies and pushes the branch | No repo credentials shared; manual step per PR |
| (b) Push with a scoped token | You provide a token limited to pushing `a1/*` branches in one repo | Fewer manual steps; needs a credentials decision and egress to your git host |

**Prerequisite either way:** confirm that `version1.zip` matches the current repository head. If the repo has moved on, patches must be rebased and the audit baseline re-checked.

---

## 5. Checks that must be performed in your own environments

"Your own environment" includes CI runners **you** own and control. Please confirm that CI counts as your own environment for P-0 condition 2.

| Check | Why it can't be done in Claude's sandbox | Where |
|---|---|---|
| Postgres + ClickHouse integration tests (**independent run**, P-0 condition 2) | Condition requires independence; ClickHouse is unavailable here | Your CI |
| Container image builds and image scanning | No Docker daemon | Your CI |
| Disposable kind staging for all A1 items (SD-1) | No Docker or cluster | Your CI |
| Legitimate SAML login via isolated test IdP (SD-4) | Needs a Keycloak container and a cluster | Your CI (kind) |
| Real rollback test (SD-5), with per-run approval | Mutates workloads; needs a cluster and your permission | Your CI (kind), protected environment |
| **Verification of the SSO ingress block on your real environments** (dev, staging, production) | I can't reach your environments, and the ingress is unknown | Your environments |
| Network isolation from production for runners (SI-7) | Only you can see your networks and credentials | Your CI/network |
| Secret backend provisioning under the approved names | Real secret stores are yours | Your environments |
| **Production-like staging** before any production deployment (`08` §4): real ingress, cloud platform, NetworkPolicy/CNI, secret backend, backup/restore and rollback rehearsal | kind can't reproduce your production specifics | Your long-lived staging (to be created) |
| Final sign-off on the A1 validation report | Owner decision | You |

---

## 6. New finding from the checks run today (not fixed; no code changed)

**Migrations don't apply on PostgreSQL 16.**

**Evidence:** applied migrations 001–007 in order with `psql -v ON_ERROR_STOP=1` to a fresh local PostgreSQL 16.13, 2026-09-16T15:47Z.

| Migration | Result |
|---|---|
| `001_initial.sql` | **FAIL** at line 217: `syntax error at or near "window"`. The `slos` table uses `window` as a column name, and `WINDOW` is a reserved word in PostgreSQL. |
| `002_multitenancy.sql` | FAIL: `relation "slos" does not exist` (cascade) |
| `003_comments_runbooks.sql` | FAIL: `relation "alert_rules" does not exist` (cascade) |
| `004_deployments.sql` | FAIL: `function set_updated_at() does not exist` (defined later in 001, never reached) |
| `005_integrations_postmortems.sql` | FAIL at line 91: `unique constraint on partitioned table must include all partitioning columns` (independent defect) |
| `006_sso_clusters.sql` | OK |
| `007_feature_state.sql` | FAIL: `function set_updated_at() does not exist` |

**Resulting schema:** only 15 tables exist. `slos`, `alert_rules`, `team_members`, `namespace_permissions`, `password_reset_tokens`, `incidents` and others are missing. `users` has **no `org_id` column**, because 002 failed.

**Impact:**
- **Likely:** fresh Compose installs fail database initialization. Both Compose files mount these migrations into `/docker-entrypoint-initdb.d`, and the official Postgres image runs init scripts with `ON_ERROR_STOP=1`. *(This inference is from the image's documented behavior; I didn't run it in Docker.)*
- **Confirmed by the run above:** the database can't be created from the repository's migrations.
- Every Postgres-backed integration test for A1 (S1-01, S1-06) needs a working schema.

**Relationship to existing items:** this extends F-006 (schema drift) and F-044 (migration tooling). The fix is schema work under **S1-11 / A7, which isn't approved.**

**Consequence for A1 (decision needed later, not now):** the A1 integration tests need a schema. Options will be presented when A1 resumes:

| Option | What it means |
|---|---|
| (i) Test-only fixture schema | Containing just the tables A1 touches |
| (ii) Bring the schema fix (A7) forward | Before A1 integration tests |
| (iii) Run A1 with unit tests only | Integration marked **not validated** until A7 |

---

## 7. Summary of what's needed to proceed

| # | Item | Owner | Status |
|---|---|---|---|
| 1 | Allowlist `proxy.golang.org`, `sum.golang.org`, `vuln.go.dev` for Claude's environment | You | Pending |
| 2 | Where the source is hosted, and whether `version1.zip` matches its head | You | Pending |
| 3 | CI platform with Docker-capable Linux runners, size ≥ estimate in R-3 | You | Pending |
| 4 | Confirm CI runners you own count as "your own environment" (P-0 condition 2) | You | Pending |
| 5 | Protected-environment approval gate for rollback runs (SD-5) | You | Pending |
| 6 | Code handoff method: patch files or scoped push token | You | Pending |
| 7 | Evidence sharing: artifact upload into chat, or a read-only CI token (separate approval) | You | Pending |
| 8 | Schema approach for A1 integration tests (§6) | You, when A1 resumes | Pending |

**Holds unchanged:** A1 implementation on hold · A2 on hold · S0 on hold · P-7 Pending clarification · SSO kept disabled only via ingress block (unverified).
