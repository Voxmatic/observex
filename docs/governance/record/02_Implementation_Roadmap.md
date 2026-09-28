# 02 — ObserveX Prioritized Implementation Roadmap

**Status:** Proposed. Nothing beyond patch S0 has been implemented.
**Rule:** Stages run **one at a time**. Only preparation work (spikes, design docs) for the next stage may overlap. Each stage has an entry gate and an exit gate. If an exit gate isn't met, the next stage doesn't start.

Durations assume **2 experienced engineers (backend/infra) plus part-time product owner review**. ⚠️ *Please confirm team size. Every estimate below scales with it.* Estimates are ranges because the Go services haven't been compiled yet (F-082/F-101), which is the largest unknown.

---

## Stage dependency graph

```
Stage 1  Critical security & reliability containment
   │  (closes exploitable paths; no architecture change)
   ▼
Stage 2  Build, test & deployment stabilization
   │  (reproducible builds + real tests = the ability to change safely)
   ▼
Stage 3  Core architecture   ◄── needs decisions D-02…D-10, D-19
   │  (tenancy, identity/sessions, storage & queue, migrations, service auth)
   ▼
Stage 4  Observability foundation  ◄── needs D-04, D-15, D-16, D-18
   │  (real logs/traces/events, durable processor, self-monitoring)
   ▼
Stage 5  AI capabilities  ◄── needs D-14
   │  (advisory only: detection, correlation, RCA explanations)
   ▼
Stage 6  Controlled remediation  ◄── needs D-11, D-13
   │  (policy → approval → execute → verify → audit)
   ▼
Stage 7  Enterprise & SaaS readiness  ◄── needs D-01, D-17, D-20, D-21
```

### Why this order
| Dependency | Reason |
|---|---|
| **1 → 2** | Stage 1 closes the exploitable paths first. An exposed SAML bypass or remote Kubernetes control can't wait for CI to be rebuilt. The containment changes are deliberately small so they're safe with minimal testing, and Stage 2 then locks them in with regression tests. |
| **2 → 3** | Stage 3 changes schema, auth and data flow. Without reproducible builds, real package tests and a migration runner, that work can't be done safely or rolled back. |
| **3 → 4** | Real logs/traces/events and durable processor state need the tenancy model (org in every query), the storage decision and the queue. Otherwise they'd be built twice. |
| **4 → 5** | AI detection and RCA are only as good as the telemetry underneath. Today much of it is mocked (F-007/F-066). Building AI on fake data produces fake intelligence. |
| **5 → 6** | Remediation consumes AI/detection output. It needs trustworthy signals, verification metrics (Stage 4) and identity-bound approvals (Stage 3). |
| **3,4,6 → 7** | Enterprise features (SSO/SCIM, quotas, compliance evidence, DR, SLAs) depend on tenancy, retention, audit and a safe remediation story. |

---

## Stage 1 — Critical security & reliability fixes
**Goal:** Remove the confirmed exploit paths and silent-failure hazards without architectural change.
**Entry gate:** You approve the Stage 1 pack (`05`, items S1-01…S1-12). The Go toolchain can reach module proxies in your CI or dev environment (I can't compile Go here).
**Duration:** ~1–2 weeks

| # | Work item | Findings | PO approval |
|---|---|---|---|
| S1-01 | SAML: reject callbacks without SSO enabled and a cert; require non-empty RelayState that maps to an enabled config | F-001 | Pack approval |
| S1-02 | OIDC: disable routes behind a feature flag (default off) until Stage 3 | F-014 | **Yes** (turns off a feature that doesn't work today) |
| S1-03 | AI agents: `DRY_RUN` default true; Python `MODE` default `suggest`; fail-closed internal token on `/v1/problems`, `/mode`, approvals | F-002, F-011 | **Yes** (changes defaults) |
| S1-04 | Query engine: remove raw SQL passthrough; explicit CORS | F-003, F-017 | Pack approval |
| S1-05 | Fail-fast on missing, default or short `JWT_SECRET`, `AGENT_TOKEN_SECRET`, `INTEGRATION_ENC_KEY`; remove Helm secret defaults | F-005 | **Yes** (dev setups must set secrets) |
| S1-06 | `InternalOnly` fail-closed; role checks on the write routes in F-010; auth on `/ws` | F-010, F-011, F-047 | Pack approval |
| S1-07 | Ingestor: decompressed body cap; ignore org header for static tokens | F-015, F-004 (partial) | Limit value: **Yes** |
| S1-08 | ClickHouse ingest: escape backslash + quote now; parameterized inserts in Stage 3 | F-012 | Pack approval |
| S1-09 | Rollback revision integer comparison + test | F-043 | No |
| S1-10 | Helm: per-component ServiceAccounts, remove `pods/exec`, NetworkPolicy on by default | F-002, INF | **Yes** (deployment behavior) |
| S1-11 | Postgres schema drift: reconciliation migration `008` + stop swallowing store errors | F-006, F-051 | **Yes** (schema; needs D-03 at least as "use plain SQL files for now") |
| S1-12 | Replace seeded admin with a one-time bootstrap (env-provided password or generated and printed once) | F-005/C1 | **Yes** |

**Exit gate (measurable):**
- Every S1 item has an automated regression test or scripted HTTP proof that failed before the fix and passes after it.
- `go build ./...` and `go vet ./...` succeed for all changed modules, run in your environment or in CI.
- Manual verification checklist in `05` signed off by you.

---

## Stage 2 — Build, test & deployment stabilization
**Goal:** Every change from here on is buildable, testable, scannable and deployable in a repeatable way.
**Entry gate:** Stage 1 exit.
**Duration:** ~2–3 weeks

| Work item | Findings |
|---|---|
| Restore canonical `go.mod` paths; Go ≥ 1.23 in Dockerfiles and CI; `-mod=readonly` | F-082 |
| Rewrite CI for the real layout: lint (golangci, eslint, ruff), vet, unit tests, coverage, govulncheck, pip-audit, npm audit, helm lint, kubeconform, image builds | F-101, F-106, F-084, F-020, F-021 |
| Real Go unit tests importing product packages (middleware, store, ingestor parsing, query builder, remediation planner) | F-100 |
| Integration tests with testcontainers (Postgres, ClickHouse, Redis) including a **schema contract test** (every SQL statement prepared against a migrated DB) | F-006, F-102 |
| Remove committed binaries and duplicates (after D-22 approval) | F-080, F-081, F-085 |
| gofmt commit; ESLint config (warn level first) | F-083, F-084 |
| Fix `db-monitor` CGO build; non-root images; image SBOM + signing | F-089, F-029 |
| Dependency upgrades (Python, npm, Go libs) | F-020 |
| Single compose file for dev + overlays; working quick start | F-087, F-104 |
| Staging environment with automated deploy and rollback (Helm `--atomic`) | Baseline §Deployment |

**Exit gate:**
- CI is green on a clean checkout for 10 consecutive merges.
- Coverage is measured from product code (the threshold is set in `04`).
- An image for every service builds in CI.
- Staging deploy and rollback have been demonstrated.

---

## Stage 3 — Core architecture improvements
**Goal:** Implement the approved foundational decisions.
**Entry gate:** Stage 2 exit **and** approval of D-02, D-03, D-04, D-05, D-06, D-07, D-08, D-09, D-10.
**Duration:** ~6–10 weeks (largest stage)

| Workstream | Content | Decisions |
|---|---|---|
| 3A Tenancy | Org derived only from identity; `org_id` in every Postgres table and ClickHouse ORDER BY key; one query layer that enforces it; membership model | D-07 |
| 3B Identity & sessions | Session model; API keys with scopes; route authorization matrix (default deny); SAML/OIDC rebuilt on vetted libraries; rate limiting | D-06, D-09 |
| 3C Data & pipeline | Batched ingest; durable queue between ingestor → processor → detection; migration tool; single owner for SLOs and alert rules | D-03, D-04, D-05 |
| 3D Service boundaries | Split gateway into domain packages (no behavior change); fix gateway ↔ query-engine contract (F-066); workload identity between services | D-02, D-10 |
| 3E Secrets | Secret manager integration; key rotation | D-08 |

**Exit gate:**
- An automated cross-tenant test suite (read, write, query, stream, AI decisions) passes.
- Authorization matrix tests cover 100% of routes.
- The ingest load test meets the agreed baseline with zero silent drops.
- Upgrade from the previous release has been migrated and rolled back in staging.

---

## Stage 4 — Observability foundation
**Goal:** Real, trustworthy telemetry end to end, and the platform observes itself.
**Entry gate:** Stage 3 exit; D-04, D-15, D-16, D-18 approved.
**Duration:** ~4–6 weeks

- Replace mocked logs/traces/events with real tenant-scoped queries. Unfinished features stay behind the preview flag (D-12).
- Durable processor state; horizontal scaling (partitioning or leader election).
- Retention per data type and tenant; cardinality limits; ingest quotas.
- Platform self-monitoring: Prometheus metrics, OTel traces, structured logs with request IDs, platform SLOs and alerts, dashboards, runbooks.
- Backup/restore automation for Postgres and ClickHouse, plus a restore drill.

**Exit gate:**
- No mocked responses in a production build.
- The platform SLO dashboard is live.
- A restore drill has met the RPO/RTO from D-19.

---

## Stage 5 — AI capabilities (advisory only)
**Goal:** Useful, explainable detection and RCA that never acts on its own.
**Entry gate:** Stage 4 exit; D-14 approved.
**Duration:** ~4–6 weeks

- Consolidate onto one AI service (D-11/D-14). Retire the others (F-060).
- Anomaly detection evaluated on labeled or replayed incidents, with precision/recall reported rather than assumed.
- Event correlation and problem grouping; LLM-generated summaries and RCA hypotheses with citations to the underlying telemetry.
- Prompt-injection defenses, PII redaction, per-tenant data boundaries, token/cost budgets.
- Offline evaluation harness and golden datasets.

**Exit gate:**
- An evaluation report exists with measured detection quality on agreed datasets.
- Red-team tests show no data crosses tenant boundaries and LLM output can't trigger actions.

---

## Stage 6 — Controlled remediation
**Goal:** Remediation that's policy-bound, approved, reversible and fully audited.
**Entry gate:** Stage 5 exit; D-11 and D-13 approved.
**Duration:** ~6–8 weeks

- Persistent decision state machine: proposed → policy-checked → approved → executing → verifying → done/rolled-back.
- Policy engine: action allowlist, namespace/label scope, blast-radius limits, change windows, concurrency and rate limits, kill switch.
- Approvals bound to user identity and role, with expiry. Two-person rule for high-risk actions (configurable).
- Least-privilege execution identity per action class; no cluster-wide writes by default.
- Automatic verification against SLO signals and automatic undo of the remediation when possible.
- Tiers: suggest → approve → auto (per action and per namespace, opt-in).
- Immutable, exportable audit trail.
- Game days: forged events, prompt injection, flapping signals, partial failures.

**Exit gate:**
- The game-day suite passes.
- No action can execute without a matching policy.
- 100% of actions are attributable and appear in the audit log.

---

## Stage 7 — Enterprise & SaaS readiness
**Goal:** Commercially operable at the chosen deployment model.
**Entry gate:** Stage 6 exit; D-01, D-17, D-20, D-21 approved.
**Duration:** depends on the deployment model and compliance scope.

- SCIM and user lifecycle, multi-org membership, custom roles.
- Tenant provisioning, plans, quotas, metering and billing hooks (SaaS).
- Compliance evidence program (for example SOC 2 readiness *if chosen*), data residency, DPA-supporting controls.
- HA across zones; DR per D-19; published SLAs (only after measured SLO history).
- External penetration test; security whitepaper; naming and trademark changes (D-17).
- Upgrade and support policy; air-gapped install option (if self-hosted is in scope).

**Exit gate:** criteria in `04` §Enterprise, plus a pen-test report with Critical/High findings closed.

---

## What is explicitly *not* happening yet
- No new product features and no expansion of autonomous remediation.
- No deletions (binaries, duplicates, legacy agents) until D-22 is approved.
- No schema changes until S1-11 / D-03 are approved.
