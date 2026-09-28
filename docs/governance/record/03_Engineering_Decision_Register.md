# 03 — ObserveX Engineering Decision Register

**Status:** All decisions are **PROPOSED — awaiting product-owner approval**. None are implemented.

**Status update (2026-09-22):** D-14 is amended by **D-14-E, approved 2026-09-22** — see the amendment in the D-14 section and `07_Approval_Log.md`. This update concerns D-14 only and does not change the status of any other decision.

**Rating key** (relative between the options within one decision, not absolute):

| Symbol | Meaning |
|---|---|
| ●●● | Strong |
| ●● | Adequate |
| ● | Weak |

Cost means infrastructure plus people cost. Ops means operational simplicity: ●●● is simplest. Lock-in: ●●● means least lock-in.

**Evidence rule:** "Impact on existing code" cites what exists today. Nothing here assumes performance numbers I haven't measured.

**Decision order** (dependencies):

```
D-01 → D-07 → (D-04, D-05, D-09, D-06) → D-02, D-10 → D-11, D-13, D-14 → D-16, D-18, D-19 → D-20, D-21
```

**Can be approved immediately, independent of the others:**

- D-03
- D-08
- D-12
- D-15
- D-17
- D-22

---

## D-01 · Deployment model
**Problem.** The README says "self-hosted, no data leaves your network". `docs/observex-agent-saas-playbook.md`, Railway/Vercel configs and plan/quota fields (`orgs.plan`, `max_users`) suggest SaaS. Tenancy (D-07), secrets, compliance, cost controls and support all depend on this.

| Option | Security | Reliability | Scalability | Cost | Ops | Maint. | Lock-in |
|---|---|---|---|---|---|---|---|
| A. Self-hosted only (Helm/Compose, single org per install) | ●●● | ●● (customer-operated) | ●● | ●●● for you | ●● | ●●● | ●●● |
| B. Multi-tenant SaaS only | ●● (shared blast radius) | ●●● (you operate it) | ●●● | ●● | ● | ●● | ●● |
| C. Both, from one codebase (SaaS multi-tenant + self-hosted with org=1) | ●● | ●● | ●●● | ● | ● | ● | ●●● |
| D. Self-hosted now, SaaS-ready architecture later | ●●● | ●● | ●● | ●●● | ●● | ●● | ●●● |

**Impact on existing code.** Every option requires fixing F-004. A and D can ship with a single tenant enforced; B and C need full isolation before any customer data is onboarded.

**Recommendation: D.** Build tenancy correctly (D-07 B) but launch self-hosted/single-org first. That avoids operating a shared SaaS while isolation is still unproven, and keeps the SaaS path open.

**❓ Approval:** Which model is the business target, and on what timeline?

---

## D-02 · Service architecture
**Problem.** There are 11 services plus 6 AI/remediation implementations (F-060). The gateway is a 6,823-line monolith (F-062). Services talk over synchronous HTTP (F-064), and there are API contract mismatches (F-066).

| Option | Security | Reliability | Scalability | Perf | Cost | Ops | Maint. | Lock-in |
|---|---|---|---|---|---|---|---|---|
| A. Keep all services; fix contracts | ●● | ● | ●● | ●● | ● | ● | ● | ●●● |
| B. **Consolidate to a modular core:** `gateway` (API+auth), `ingest`, `query`, `detect` (processor + AI advisory), `remediate`, `agent`; retire duplicates; domain packages inside | ●●● | ●●● | ●● | ●● | ●● | ●● | ●●● | ●●● |
| C. Modular monolith (one binary, separately scalable roles by flag) | ●● | ●● | ●● | ●●● | ●●● | ●●● | ●● | ●●● |
| D. Finer microservices (per feature) | ● | ● | ●●● | ● | ● | ● | ● | ●●● |

**Impact.** Option B retires `ai-agent-v2`, `ai-agent-python` (logic ported), `observex-agent`, and the gateway `inhouse_agent.go`/`ai_monitoring_agent.go` paths. It keeps `activegate`, `trivy-scanner` and `db-monitor` as optional add-ons. Public REST routes stay; handlers move into packages.

**Recommendation: B.** It reduces the attack surface and gives each safety-critical component (ingest, remediation) its own deployment, security identity and scaling. Remediation especially needs its own least-privilege service account. C would put Kubernetes write rights in the same process as the internet-facing API.

**❓ Approval:** Approve B, including retiring the listed implementations after their useful logic is ported (deletion is covered by D-22)?

---

## D-03 · Operational database & migrations
**Problem.** Postgres holds config and identity. Migrations only run on first container init (F-044), and code and schema have drifted (F-006).

| Option | Security | Reliability | Ops | Maint. | Lock-in | Notes |
|---|---|---|---|---|---|---|
| A. **Postgres + goose** (SQL files, embedded in binary, version table) | ●●● | ●●● | ●●● | ●●● | ●●● | Fits the existing `.sql` files |
| B. Postgres + golang-migrate | ●●● | ●●● | ●● | ●● | ●●● | Separate up/down files; widely used |
| C. Postgres + Atlas (declarative diff) | ●●● | ●●● | ●● | ●●● | ●● | Great drift detection; extra tool/licensing tiers |
| D. Switch DB (e.g. CockroachDB) | ●● | ●●● | ● | ● | ●● | Not justified by any current evidence |

**Schema canonical direction (part of this decision).** For each F-006 mismatch, choose which side wins. Recommended defaults:

- Keep migration names `namespace_permissions` and `password_reset_tokens`, and fix the code.
- Add `sessions.org_id`.
- Rename the store fields to the migration's `issuer/client_id/client_secret` columns.
- Create the missing `incidents`, `service_catalog`, `agent_knowledge` and `agent_playbooks` tables.

**Impact.** Adds migration `008_reconcile.sql` and a startup or `migrate` subcommand. `docker-entrypoint-initdb.d` stays for dev only.

**Recommendation: A**, plus the canonical-direction defaults above, plus a CI schema contract test.

**❓ Approval:** Approve goose and the default reconciliation directions?

---

## D-04 · Telemetry storage & query language
**Problem.** Metrics go to ClickHouse with no org column (F-004) and per-point inserts (F-040). Logs and traces are meant for Loki/Tempo but the gateway returns mocks (F-007). The gateway sends PromQL to an endpoint that doesn't exist (F-066). VictoriaMetrics and Neo4j are configured but unused.

| Option | Security (tenancy) | Reliability | Scalability | Perf | Cost | Ops | Maint. | Lock-in |
|---|---|---|---|---|---|---|---|---|
| A. **ClickHouse for metrics/logs/traces/events** (OTel schema, org in sort key) + own query API with PromQL-subset translation | ●● (you enforce it) | ●● | ●●● | ●●● | ●●● | ●● | ●● | ●●● |
| B. **Grafana LGTM** (Mimir, Loki, Tempo) — native `X-Scope-OrgID` tenancy, PromQL/LogQL/TraceQL | ●●● | ●●● | ●●● | ●● | ●● | ● (3 systems) | ●●● | ●● (AGPL for some components — *legal review needed*) |
| C. Hybrid: Prometheus-compatible TSDB (VictoriaMetrics/Mimir) for metrics + ClickHouse for logs/traces/events | ●● | ●● | ●●● | ●●● | ●● | ● | ● | ●● |
| D. Keep the current mix and patch it | ● | ● | ● | ● | ●● | ● | ● | ●●● |

**Impact.**
- **A:** keeps the existing ClickHouse writer and query engine but reworks their schema, and requires implementing log/trace queries.
- **B:** replaces the ClickHouse metrics path and the query-engine metrics code, and makes the gateway's PromQL calls correct as written.
- **C:** has the largest integration surface.

**Recommendation.**
- **If PromQL compatibility is a product requirement:** choose **B**. It also gives tenancy built into the storage layer.
- **Otherwise:** choose **A**, since it's closer to the current code and lower cost.

**❓ Approval / input needed:**
- Is PromQL/Grafana compatibility a must-have for customers?
- Is AGPL-licensed software acceptable in your distribution model? (This one needs legal confirmation.)

---

## D-05 · Event streaming / message queue
**Problem.** Ingest → processor → AI → remediation is synchronous fire-and-forget HTTP (F-064, F-040). There's no durability, replay or backpressure.

| Option | Reliability | Scalability | Cost | Ops | Lock-in | Notes |
|---|---|---|---|---|---|---|
| A. **NATS JetStream** | ●●● | ●●● | ●●● | ●●● | ●●● | Single small binary, K8s-friendly, good for self-hosted |
| B. Apache Kafka / Redpanda | ●●● | ●●● | ●● | ● (Kafka) / ●● (Redpanda) | ●●● / ●● (Redpanda BSL) | Highest-throughput option. Heavy to operate for self-hosted customers. |
| C. Redis Streams | ●● | ●● | ●●● | ●●● | ●●● | Redis already in the stack. Weaker durability and retention. |
| D. Postgres outbox + polling | ●● | ● | ●●● | ●●● | ●●● | Fine for remediation commands; not for telemetry volume |

**Impact.** Ingestor publishes batches; processor/detect consume; remediation consumes decisions from a separate stream. Redis stays for rate limits and session cache (D-09).

**Recommendation: A for telemetry and events, plus D (outbox) for remediation commands**, so decisions and approvals are transactionally consistent with the audit log. *Assumption to confirm:* expected ingest volume. If it's very high (many millions of points per second), revisit B.

**❓ Approval:** Approve NATS JetStream + Postgres outbox?

---

## D-06 · Federated authentication (SSO) implementation
**Problem.** The SAML and OIDC implementations are hand-rolled, bypassable and broken (F-001, F-014).

| Option | Security | Reliability | Cost | Ops | Maint. | Lock-in |
|---|---|---|---|---|---|---|
| A. **Vetted Go libraries** (`crewjam/saml` + `russellhaering/goxmldsig`; `coreos/go-oidc` + `x/oauth2`) | ●●● | ●●● | ●●● | ●●● | ●● | ●●● |
| B. Bundled identity broker (Keycloak / Dex / Zitadel); ObserveX speaks OIDC only | ●●● | ●● | ●● | ● | ●●● | ●● |
| C. Hosted IdP-as-a-service (Auth0, WorkOS, etc.) | ●●● | ●●● | ● (recurring) | ●●● | ●●● | ● |
| D. No SSO until Stage 7 | ●●● | ●●● | ●●● | ●●● | ●●● | ●●● |

**Impact.** Replaces SSO code in `api-gateway/main.go` (roughly lines 3250–4700: SSO types, SAML/OIDC init and callbacks, XML helpers, ID-token parsing) and the SSO store fields. S1-01/S1-02 contain the risk in the meantime.

**Recommendation: D now (Stage 1), A in Stage 3.** Choose C instead if the business prefers to buy enterprise SSO rather than build and maintain it (it's a SaaS dependency, so it conflicts with pure self-hosted D-01).

**❓ Approval:** Approve "disable now, rebuild with libraries in Stage 3"?

---

## D-07 · Multi-tenancy model
**Problem.** Tenancy is partial in Postgres and absent in telemetry, processor and AI state (F-004, F-051, F-065).

| Option | Security | Reliability | Scalability | Cost | Ops | Maint. | Lock-in |
|---|---|---|---|---|---|---|---|
| A. Single-tenant per deployment | ●●● | ●●● | ●● | ● (per-customer infra) | ●● | ●●● | ●●● |
| B. **Shared stores; `org_id` mandatory in every table/sort key; enforced in one data-access layer + Postgres RLS as defense-in-depth** | ●● → ●●● with RLS + tests | ●●● | ●●● | ●●● | ●● | ●● | ●●● |
| C. Per-tenant database/schema | ●●● | ●● | ●● | ●● | ● | ● | ●●● |
| D. Hybrid (B default; dedicated cells for enterprise) | ●●● | ●●● | ●●● | ●● | ● | ● | ●●● |

**Impact.**
- **Every** store query and ClickHouse table changes, and identity carries org.
- Users move to a membership table (F-065).
- Processor and remediation state become keyed by org.

**Recommendation: B, designed so D is possible later.** Enforcing in one layer plus Postgres RLS gives two independent barriers, and a cross-tenant test suite gates releases.

**❓ Approval:** Approve B? Should one user be able to belong to multiple orgs? (Recommended: yes.)

---

## D-08 · Secrets management
**Problem.** Default secrets are accepted at runtime (F-005), there's an all-zero encryption key, and secrets live in values files and Compose.

| Option | Security | Ops | Cost | Lock-in |
|---|---|---|---|---|
| A. **Kubernetes Secrets via External Secrets Operator** (backed by Vault/AWS/GCP/Azure), env-file for Compose; fail-fast validation | ●●● | ●● | ●●● | ●●● |
| B. HashiCorp Vault direct integration (dynamic DB creds) | ●●● | ● | ●● | ●● |
| C. Cloud KMS/Secret Manager SDKs directly | ●●● | ●● | ●● | ● |
| D. Plain K8s Secrets + SOPS in git | ●● | ●●● | ●●● | ●●● |

**Recommendation: A**, plus envelope encryption of integration secrets with a rotatable key ID. For self-hosted customers, D is an accepted minimum.

**❓ Approval:** Approve A (with D as the documented minimum)?

---

## D-09 · Sessions, API authentication & authorization model
**Problem.** JWTs sit in localStorage, sessions are never checked, API keys don't work, and RBAC has gaps (F-008, F-050, F-010).

| Option | Security | UX | Scalability | Ops |
|---|---|---|---|---|
| A. **Opaque session ID in HttpOnly/Secure/SameSite cookie (Redis-cached, Postgres-backed) for UI + scoped, hashed API keys for programs + CSRF token** | ●●● | ●●● | ●●● | ●● |
| B. Short-lived JWT (≤15 min) + rotating refresh token + `jti` denylist | ●● | ●● | ●●● | ●● |
| C. Keep 24h JWT, add `jti` check | ● | ●●● | ●●● | ●●● |

**Authorization sub-decision.**

| Option | Notes |
|---|---|
| (i) **Explicit per-route RBAC matrix** in code | Simple, testable |
| (ii) Policy engine (OPA/Cedar) | More flexible, more to operate |

**Impact.** Changes to middleware, login/logout, frontend auth store (no localStorage), and CORS with credentials.

**Recommendation: A + (i)**, with custom roles and a policy engine added in Stage 7 only if customers require them.

**❓ Approval:** Approve A + (i)?

---

## D-10 · Service-to-service identity
**Problem.** Internal endpoints are unauthenticated (F-011).

| Option | Security | Ops | Works in Compose? | Lock-in |
|---|---|---|---|---|
| A. Service mesh mTLS (Linkerd/Istio) | ●●● | ● | No | ●● |
| B. **Signed short-lived workload tokens** (K8s projected SA tokens verified via TokenReview/JWKS; static shared keys in Compose) + NetworkPolicy | ●●● | ●● | Yes | ●●● |
| C. NetworkPolicy + shared static token only | ●● | ●●● | Yes | ●●● |

**Recommendation: C in Stage 1 (containment), B in Stage 3.** A mesh stays optional for customers who already run one.

**❓ Approval:** Approve this staged path?

---

## D-11 · Remediation security model
**Problem.** Remediation actions run without authentication or approval, under a cluster-wide identity (F-002).

| Option | Safety | Speed of MTTR | Ops | Notes |
|---|---|---|---|---|
| A. Suggest-only forever | ●●● | ● | ●●● | Lowest risk, least product value |
| B. **Tiered: suggest → human approve → policy-scoped auto**; per action and per namespace opt-in; two-person rule for high-risk actions; kill switch; dedicated least-privilege executor identity per namespace | ●●● | ●● → ●●● | ●● | Industry-standard progressive trust |
| C. Autonomous by confidence threshold | ● | ●●● | ●● | Confidence is currently caller-supplied or LLM-influenced, so this isn't defensible |
| D. Delegate execution to GitOps (open a PR/Argo rollback) rather than mutating the cluster directly | ●●● | ●● | ●● | Strong audit, and reversible through git. Needs GitOps adoption. |

**Impact.** Replaces `ai-agent` execution flow and approval handlers, RBAC templates, and the Python agent modes. Adds a persistent decision table and audit.

**Recommendation: B, with D as an executor plug-in** for customers using GitOps. The default for every new install is **suggest**. Auto tier needs explicit per-namespace policy.

**❓ Approval:**
- Do you approve B?
- Which actions may ever be eligible for the auto tier? The proposed list is restart pod, scale out within HPA bounds, and rollback to the immediately previous revision.

---

## D-12 · Mocked / preview feature policy
**Problem.** Many features return fabricated data (F-007) and 27 UI pages are static.

| Option | Honesty | Demo value | Effort |
|---|---|---|---|
| A. Remove from UI/API | ●●● | ● | M |
| B. **Preview flag, off by default in production builds; visible "Preview / demo data" badge; APIs return `501` or `{"demo":true}` when off** | ●●● | ●●● | M |
| C. Leave as-is | ● | ●●● | — |

**Recommendation: B.** It's reversible and keeps sales demos working without misleading operators.

**❓ Approval:** Approve B? Do you want a separate "demo mode" build for sales?

---

## D-13 · Agent architecture (node/host agent)
**Problem.** OneAgent is privileged beyond what it ships (F-018). eBPF is stubbed (F-093). The project has its own agent protocol plus OTLP. Names are Dynatrace trademarks (F-032).

| Option | Security | Coverage | Maint. | Lock-in |
|---|---|---|---|---|
| A. Keep custom agent; add real eBPF (CO-RE) as an opt-in privileged module | ●● | ●●● | ● | ●●● |
| B. **OpenTelemetry Collector (contrib) as the base agent + ObserveX extensions/processors; eBPF via existing OSS (e.g. OTel eBPF / Beyla-class instrumentation) as opt-in** | ●●● | ●●● | ●●● | ●●● |
| C. Agentless only (OTLP from apps, K8s API, cloud APIs) | ●●● | ● | ●●● | ●●● |

**Impact.** B keeps the ingest OTLP endpoints and treats the native agent protocol as legacy. The `oneagent` discovery logic can become a collector receiver.

**Recommendation: B.** It's a standards-based agent that customers already trust, needs far less custom privileged code, and avoids the trademark issue. *Needs confirmation:* are there customer commitments to the current agent protocol?

**❓ Approval:** Approve B?

---

## D-14 · AI provider & model strategy
**Problem.** The Python agent calls Anthropic, OpenAI or Ollama with raw telemetry. The Go agents are rule-based. LLM output influences action gating (F-019).

| Option | Privacy | Quality | Cost | Ops | Lock-in |
|---|---|---|---|---|---|
| A. Hosted frontier LLM APIs only | ● (egress) | ●●● | ●● (usage-based) | ●●● | ● |
| B. Local/self-hosted open models only (Ollama/vLLM) | ●●● | ●● | ●● (GPU) | ● | ●●● |
| C. **Provider-abstracted: local by default for self-hosted; hosted provider opt-in per tenant with redaction, budgets and zero-retention settings where the provider offers them; LLM strictly advisory** | ●●● | ●●● | ●● | ●● | ●●● |
| D. No LLM; statistical/rule-based only | ●●● | ● (explanations) | ●●● | ●●● | ●●● |

**Guardrails for every option:**
- LLM output never selects or authorizes actions.
- Prompts get tenant-scoped data only.
- PII/secret redaction.
- Prompt-injection filtering on log content.
- An evaluation harness.

**Recommendation: C.** *Needs your input:* are there customer or legal constraints on sending telemetry to third-party AI providers?

**❓ Approval:** Approve C and the guardrails?

**Amendment — 2026-09-22 · D-14-E approved; this is the current decision.** The option C recommendation and the approval question above are retained unchanged as the historical record. Option C was not approved.

**E. No external LLM runtime** *(not among options A–D above)*. ObserveX will not use an external LLM runtime. Future intelligence may use deterministic methods, statistical models, and narrowly scoped in-house models. No hosted LLM API, Ollama runtime, Anthropic API, or equivalent is permitted in the new intelligence architecture.

- **Status:** Approved, 2026-09-22 — recorded in `07_Approval_Log.md`.
- **Supersedes:** "Recommendation: C" and "Approve C and the guardrails?" above.
- **Not addressed by this approval:** the guardrails listed above. Their status under E is unresolved.
- **Related decisions**, approved the same day and recorded in `07_Approval_Log.md`: D-INT-2, D-INT-3, KB-FMT-1.

---

## D-15 · Platform self-observability stack
**Problem.** Metrics are custom JSON; there's no tracing, no platform SLOs, and health checks are shallow (F-024, F-046, F-096).

| Option | Notes |
|---|---|
| A. **Prometheus `/metrics` + OTel SDK traces + zap JSON logs with request/trace IDs; self-ingest into ObserveX plus an optional external Prometheus** | Standard, and dogfoods the product |
| B. Self-ingest only | Circular dependency: if ObserveX is down you can't see why |
| C. External vendor | Lock-in and cost |

**Recommendation: A**, always with a path that doesn't depend on ObserveX itself.

**❓ Approval:** Approve A?

---

## D-16 · Data retention
**Problem.** ClickHouse TTL is hard-coded at 90 days, while `.env.example` advertises 12 months of metrics, 30 days of logs and 7 days of traces (F-094). Audit retention is undefined.

| Option | Cost | Compliance fit | Complexity |
|---|---|---|---|
| A. Global fixed TTLs | ●●● | ● | ●●● |
| B. **Per data type defaults + per-tenant overrides within plan limits; downsampled long-term metrics; audit log retained separately (longer, immutable)** | ●● | ●●● | ●● |
| C. Tiered storage (hot SSD → object storage) on top of B | ●●● at scale | ●●● | ● |

**Proposed defaults.** These are *assumptions to confirm*; the rationale is common industry defaults and cost:

| Data type | Retention |
|---|---|
| Raw metrics | 15 days |
| Downsampled metrics | 13 months (allows year-over-year comparison) |
| Logs | 30 days |
| Traces | 7 days |
| Security/audit events | 1 year minimum (common audit expectation; *confirm against your compliance scope D-21*) |

**Recommendation: B now, C in Stage 7.**

**❓ Approval:** Approve B and the defaults (or give your required values)?

---

## D-17 · Naming & public claims
**Problem.** Dynatrace trademarks (OneAgent, ActiveGate, Smartscape), plus "feature parity" and "production-grade" claims (F-032).

| Option |
|---|
| A. Rename components; revise README claims to match verified capabilities |
| B. Keep names; add disclaimers |
| C. Defer to legal |

**Recommendation: A**, with a legal check. Renaming is cheap now and expensive after customers adopt the product.

**❓ Approval:** Approve renaming? Do you have preferred names?

---

## D-18 · Cost controls
**Problem.** No quotas, cardinality limits or LLM budgets (F-041, F-094). Costs grow with customer behavior.

| Option | Protection | UX | Effort |
|---|---|---|---|
| A. Hard global limits | ●● | ● | S |
| B. **Per-tenant quotas (ingest rate, active series, log bytes/day, query cost/timeout, LLM tokens/day) with soft-warn → hard-limit, and usage metering** | ●●● | ●●● | L |
| C. Bill-only (no limits) | ● | ●●● | M |

**Proposed initial limits.** All are *placeholders to confirm*; they're chosen to protect a single small cluster, not to meet a business target:

| Limit | Value |
|---|---|
| Decompressed request body | 16 MiB |
| Query timeout | 30 s |
| Max series per query | 10k |
| Active series per tenant | 1M |
| LLM tokens per tenant per day | configurable; default off for hosted providers |

**Recommendation: A in Stage 1 (request/body/query caps only), B in Stage 4/7.**

**❓ Approval:** Approve the approach and give or confirm limit values?

---

## D-19 · Availability & recovery objectives
**Problem.** No SLOs, HA or backups are defined (F-042, F-095).

| Option | Availability target | RPO / RTO | Cost | Notes |
|---|---|---|---|---|
| A. Single-zone, backups only | ~99.5% | 24h / 8h | ●●● | Adequate for internal/self-hosted small installs |
| B. **Multi-replica stateless services, HA Postgres (primary + replica), replicated ClickHouse, durable queue; single region multi-AZ** | ~99.9% | ≤15 min / ≤1h | ●● | Typical baseline for a monitoring product customers depend on |
| C. Multi-region active/passive | ~99.95%+ | ≤5 min / ≤30 min | ● | Only for SaaS with contractual SLAs |

**Rationale.** An observability platform must stay up while customers' systems are failing. Ingest availability matters more than UI availability, so ingest should buffer at the agent (disk queue) to survive short outages.

**Recommendation: B as the design target**; A acceptable for early self-hosted releases. All numbers are *assumptions until you confirm business requirements*. No SLA should be published before 90 days of measured SLO data.

**❓ Approval:** Which tier, and what are the ingest vs UI availability targets?

---

## D-20 · Cloud & Kubernetes support matrix
**Problem.** Helm, raw manifests, Compose, Railway and Vercel all exist, and none are validated (F-106).

| Option | Scope | Effort |
|---|---|---|
| A. **Helm on upstream K8s (N-2 minor versions) + EKS/GKE/AKS conformance; Compose for dev/eval only** | Focused | M |
| B. Also OpenShift, k3s, air-gapped | Broad | L |
| C. Cloud-specific managed offerings (marketplaces) | Commercial | XL |

**Impact.** Remove or deprecate `deployments/k8s/observex-full.yaml`, `railway.toml` and `vercel.json` unless they're needed.

**Recommendation: A**, adding B targets only when customers ask for them.

**❓ Approval:** Approve A? Are any customer environments (OpenShift, air-gapped) already committed?

---

## D-21 · Compliance requirements
**Problem.** A "compliance PDF" exists, but there are no controls or evidence behind it. I won't claim any certification.

| Option | Effort | Notes |
|---|---|---|
| A. None formally; security best practices only | S | For early self-hosted |
| B. **SOC 2 Type I readiness → Type II** (controls: access, change management, logging, backup, vendor) | L–XL | Most common B2B SaaS request |
| C. ISO 27001 | XL | Common in EU/enterprise |
| D. Sector-specific (HIPAA, PCI, FedRAMP) | XL | Only with specific customer demand |

**Impact.** Drives audit retention (D-16), access reviews, change management in CI (Stage 2) and the vendor review of LLM providers (D-14).

**Recommendation.**
- **SaaS in scope:** B, starting with engineering controls in Stages 2–4.
- **Otherwise:** A.

The compliance PDF feature should be relabeled as a "configuration report", not a compliance attestation.

**❓ Approval:** Which frameworks, if any, are required, and by when?

---

## D-22 · Repository cleanup (deletions)
**Problem.** ~275 MB of binaries, a committed `frontend/dist`, duplicate agent trees, `.bak` files and dead handlers (F-080, F-081, F-085).

| Option |
|---|
| A. **Delete in one reviewed commit** (git history keeps them) |
| B. Move to an `archive/` branch first, then delete |
| C. Keep |

**Recommendation: B, then A.** It's safe and reversible, and keeps history easy to find.

**❓ Approval:** Approve deletion of the listed paths? (Exact list in `05`, S2-03.)
