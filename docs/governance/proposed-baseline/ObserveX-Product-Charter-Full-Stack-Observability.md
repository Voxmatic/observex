# ObserveX — Product Charter: Full-Stack Observability Platform

> **PROPOSED — NOT APPROVED.** This is a planning charter.
> - Every architectural choice here is a **proposal**, unless it is marked as existing in code or as an approved decision.
> - It promises no dates and claims no capability that has not been validated.
> - It does not claim that ObserveX is the first, only or best of anything.
> - Adopting it as a planning direction is owner decision CH-1. Each architecture choice still needs its own register decision.

| Field | Value |
|---|---|
| Document | `docs/governance/proposed-baseline/ObserveX-Product-Charter-Full-Stack-Observability.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |

**Sources inspected.**
- Project folder:
  - `README.md`;
  - the service directories under `services/` and the `internal/` packages;
  - `sdk/rum`, `sdk/rn`;
  - `frontend/src/pages` (82 page files);
  - the Helm `Chart.yaml` dependencies and `values.yaml`;
  - `services/ingestor/main.go` routes;
  - `services/query-engine/main.go` routes;
  - the `services/processor` synthetic check types;
  - `03_Engineering_Decision_Register.md`;
  - `07_Approval_Log.md`;
  - the F6.1 design and decision documents in `Claude outputs/`;
  - VR.
- Session store (authority unknown; see the reconciliation record):
  - `01_Audit_Findings_Register.md`;
  - `02_Implementation_Roadmap.md`;
  - `04_Production_Readiness_Baseline.md`.

**Evidence labels:**

| Label | Meaning |
|---|---|
| **[CODE]** | Present in the repository. Behaviour **not validated** unless stated |
| **[VALIDATED]** | Executed evidence exists, and its scope is stated |
| **[AUDIT]** | Finding in `01_Audit_Findings_Register.md` (session store, 2026-09-16) |
| **[CLAIM]** | Stated in `README.md`; **not verified** |
| **[DECIDED]** | An approved decision in `07` |
| **[PROPOSED]** | A proposal in this charter or in a PROPOSED register entry |
| **[RESEARCH]** | Requires research or validation before any commitment |

---

## 1. Mission and product principles

**Mission** [PROPOSED]. ObserveX aims to be a coherent, production-grade, full-stack observability platform. It is meant to be one system in which infrastructure, Kubernetes, applications, data systems, user experience, synthetic checks and network signals share one identity model. It is meant to support an investigation workflow that moves across them without switching tools. It is not a synthetic-monitoring point product, not only a dashboard, and not a wrapper around third-party tools.

**Principles** [PROPOSED]:
1. **One platform, one identity.** Every signal carries the same resource identity (tenant, environment, service, workload, host, cluster), so correlation is structural rather than heuristic.
2. **Evidence before intelligence.** Detection, correlation and root-cause assistance must be explainable and traceable to observed data.
   - **[DECIDED]** D-14-E (2026-09-22): "ObserveX will not use an external LLM runtime". Intelligence uses deterministic methods, statistical models and narrowly scoped in-house models.
   - **[DECIDED]** D-INT-2 and D-INT-3: the SRE knowledge catalog is embedded read-only with `go:embed`.
3. **Trustworthy automation.** Automation is advisory by default. Any action path goes through policy, approval and audit. (The remediation model is D-11, PROPOSED.)
4. **Open ingestion.** Vendor-neutral, OpenTelemetry-compatible ingestion wherever it fits, so customers are not locked in. (The agent architecture is D-13, PROPOSED; recommendation there: OTel Collector base.)
5. **Security and tenancy by construction.** Tenant identity comes only from authenticated context, never from client-supplied headers or labels. (**[AUDIT]** F-004: telemetry has no tenant isolation today. The tenancy model is D-07, PROPOSED.)
6. **Deployment flexibility.** The architecture should not block either self-hosted or SaaS delivery. The deployment model itself is D-01 (PROPOSED; its recommendation D is "self-hosted now, SaaS-ready architecture later"). This charter does not choose it.
7. **Honest claims.** Public claims match validated evidence. (D-17, PROPOSED; **[AUDIT]** F-032.)
8. **Cost-aware by design.** Retention, cardinality and query cost are controlled per tenant. (D-16, D-18, PROPOSED.)

---

## 2. What exists today

This is an honest inventory, taken from code presence and audit findings.

| Area | What the repository contains | Evidence and caveats |
|---|---|---|
| Ingestion | `services/ingestor`: native batch endpoints for metrics, logs, traces and profiles; RUM (`/v1/rum`); security events and scans; service and topology upserts; agent registration. OTLP/HTTP routes `/v1/metrics`, `/v1/logs`, `/v1/traces` | [CODE]. OTLP **protobuf** is forwarded to an external collector (`OTEL_COLLECTOR_URL`). OTLP **JSON** is routed to the native batch handlers; whether they accept the OTLP JSON schema is **unverified**. [AUDIT] F-040: per-point inserts and silent loss. F-041: unbounded buffers and cardinality. F-004: no org isolation |
| Query | `services/query-engine`: unified `/query`; metrics range, instant, labels and series; logs query and labels; trace search and by-ID | [CODE]. [AUDIT] F-066: gateway PromQL expressions rejected by the selector parser. S1-04 removed raw SQL (implemented, not validated) |
| Storage | Helm sub-charts: Loki, Tempo, Neo4j, PostgreSQL, Redis, ClickHouse | [CODE]. `03` D-04 notes VictoriaMetrics and Neo4j are "configured but unused". Choice of telemetry storage is D-04 (PROPOSED) |
| Processing | `services/processor`: anomaly detection, SLO logic, forecasting (`forecast.go`, `regression.go`, `ml_cost_forecast.go`), alerts; legacy synthetic scheduler (HTTP, TCP, DNS, SSL, ping in code); F6.1 probe intake | [CODE]. [AUDIT] F-042: state in memory, single replica. **[CLAIM]** README lists 8 synthetic check types, including gRPC and WebSocket; the processor code switch shows 5 |
| API and UI | `services/api-gateway` (~349 route registrations across its files); React frontend with 82 page files | [CODE]. [AUDIT] F-062: 6,823-line gateway monolith. F-007: logs/traces/events endpoints return fabricated data; "27 of 80 UI pages make no API calls" |
| Agents | `services/oneagent` (DaemonSet: K8s watch, `/proc`, eBPF loaders), `services/activegate` (agent proxy), `services/observex-agent` | [CODE]. [AUDIT] F-093: eBPF probes are stubs. F-032: trademarked names |
| AI and remediation | `services/ai-agent`, `ai-agent-v2`, `observex-agent`, gateway in-house agents | [CODE]. [AUDIT] F-060: six overlapping implementations. D-11 PROPOSED. **[DECIDED]** D-14-E: no external LLM runtime |
| Knowledge and detection | `internal/knowledge` (embedded SRE failure catalog); `internal/detect/certexpiry` | **[VALIDATED]** focused local tests only: `internal/knowledge` "focused local validation PASSED 2026-09-22" (`07`, both copies); the certexpiry record exists only in the session-store copy of `07` (G4; see reconciliation C-1). **[DECIDED]** D-INT-2, D-INT-3, KB-FMT-1, KB-SEED-2 |
| Synthetic (F6.1) | `services/synthetic-probe` plus `internal/{observe,adapt,correlate,compose,evaluate,wire,probe,intake,result}` and migration `009`: multi-vantage TLS certificate expiry, with results, events, revocation and Helm wiring | **[VALIDATED]** outside Kubernetes (VR: 277 tests; revocation and rotation runs). **All decisions PROPOSED.** OPS-1 **INCOMPLETE** |
| RUM | `sdk/rum` (browser JS), `sdk/rn` (React Native) | [CODE]. `05` S0-01 (session store) reports 28/28 tests after a test-harness fix; that fix is **not present** in the project's `sdk/rum/observex-rum.test.js` (no `after()` hook found), so no current test result is claimed. Privacy controls not assessed |
| Databases | `services/db-monitor`: PostgreSQL stats; optional MySQL and Mongo | [CODE]. Chart Secret contract missing (Packet Addendum ADD-F5) |
| Security scanning | `services/trivy-scanner` | [CODE] |
| Tenancy and identity | PostgreSQL `org_id` (migration `002`); internal service token (S1-06); dedicated probe credentials (F6.1) | [CODE]. [AUDIT] F-004: telemetry not isolated. D-07, D-09, D-10 PROPOSED |
| Self-observability | Custom JSON `/metrics` | [AUDIT] F-096 |
| Deployment | Helm chart, Compose, a raw K8s manifest, Railway config | [AUDIT] F-106: never linted or built. OPS-1 Helm/kind validation BLOCKED |

**Implication** [PROPOSED]. The product surface described in `README.md` is broader than what is validated. The charter treats the README feature list as **aspiration, not inventory**. Phase 0 and Phase 1 exist to close that gap before new breadth is added.

---

## 3. Capability areas

Each area lists:
- **Scope:** what it covers.
- **Today:** what exists.
- **Direction [PROPOSED]:** where it should go.
- **Depends on:** the register decisions it needs.
- **Validate:** what must be proven.

**3.1 Infrastructure and cloud resources.**
- **Scope:** hosts, VMs, containers, cloud services, capacity and utilization.
- **Today:** the `oneagent` `/proc` collection; `CostPage`/`FinOpsPage` UI [CODE]; cost claims [CLAIM].
- **Direction:** host and container metrics through OTel-compatible collectors, resource-attributed (host, cloud account, region); capacity and forecasting reuse the processor forecasting code after validation.
- **Depends on:** D-13, D-04, D-16.
- **Validate:** collector overhead; metric completeness against a reference host.

**3.2 Kubernetes and orchestration.**
- **Scope:** clusters, nodes, workloads, deployments, events, resource health and topology.
- **Today:** the `oneagent` K8s watch; `KubernetesPage`, `WorkloadsPage` and `MultiClusterPage` UI; `k8s_clusters` table (migration `006`) [CODE].
- **Direction:** Kubernetes object inventory and events as first-class entities joined to workload telemetry by resource identity. The multi-cluster registry is persisted per tenant.
- **Depends on:** D-07, D-13, D-20.
- **Validate:** the kind-based validation path (OPS-1 runbook pattern) extended to a support matrix.

**3.3 Application performance monitoring.**
- **Scope:** runtimes, request paths, errors, latency, dependencies and service health.
- **Today:** `APMPage`, `ErrorTrackingPage` and `ServiceDetailPage` UI; RED/Apdex [CLAIM]; trace ingestion endpoints [CODE].
- **Direction:** RED metrics derived from traces and OTel semantic conventions; error grouping; service health computed from SLIs.
- **Depends on:** D-04, D-13.
- **Validate:** derived-metric accuracy against known workloads.

**3.4 Distributed tracing and service maps.**
- **Scope:** trace context, cross-service correlation, dependency graphs and bottlenecks.
- **Today:** Tempo sub-chart; trace query endpoints; topology edge ingestion; `TopologyPage`/`SmartscapePage` UI [CODE]; [AUDIT] F-007 (fabricated traces in the gateway path).
- **Direction:** W3C trace-context propagation end to end; a service map derived from spans plus declared topology, persisted in one entity store.
- **Depends on:** D-04; the Neo4j question in D-04.
- **Validate:** map correctness on a reference multi-service app.

**3.5 Metrics and time-series analytics.**
- **Scope:** ingestion, dimensional query, aggregation, retention and history.
- **Today:** ClickHouse native metrics; query-engine range and instant endpoints [CODE]; [AUDIT] F-040, F-041, F-066.
- **Direction:**
  - batched, durable ingestion;
  - cardinality limits per tenant;
  - a query language decision (PromQL compatibility or not, D-04);
  - downsampling for long-term retention (D-16).
- **Validate:** ingest and query SLOs set by the owner (candidates in `04` are unconfirmed assumptions).

**3.6 Centralized logs.**
- **Scope:** structured ingestion, indexing, search, correlation, retention and access control.
- **Today:** Loki sub-chart; logs batch ingestion; logs query endpoint [CODE]; [AUDIT] F-007.
- **Direction:**
  - structured logs with `trace_id`/`span_id` and resource attributes;
  - tenant-scoped query;
  - secret and PII scrubbing at ingest (AQ-8 policy unapproved).
- **Depends on:** D-04, D-16, AQ-8.
- **Validate:** log-to-trace pivot correctness; scrubbing tests.

**3.7 Databases and data systems.**
- **Scope:** availability, query performance, connections, replication and storage.
- **Today:** `db-monitor` (PostgreSQL; optional MySQL and Mongo); `DatabasePage` UI [CODE]; credential contract missing (ADD-F5).
- **Direction:** database telemetry as entities linked to the services that call them (via span `db.*` attributes); least-privilege monitoring roles.
- **Depends on:** the db-monitor contract (register §4), D-08.
- **Validate:** privilege scope; overhead on the monitored database.

**3.8 Real user monitoring.**
- **Scope:** browser experience, page performance, client errors and privacy safeguards.
- **Today:** `sdk/rum`, `sdk/rn`, `/v1/rum` ingestion, `RUMPage`, `SessionReplayPage` [CODE].
- **Direction:**
  - RUM spans joined to backend traces through trace context;
  - privacy by default: no PII capture without explicit configuration;
  - consent hooks;
  - tenant-scoped retention.
- **Depends on:** D-21, D-16.
- **Validate:** payload inspection for PII; overhead on page load.

**3.9 Synthetic monitoring.**
- **Scope:** API checks, browser journeys, uptime, geographic probes and business transactions.
- **Today:**
  - legacy in-processor checks (5 types in code);
  - F6.1 dedicated probe service with vantages, credentials, revocation and results [VALIDATED outside Kubernetes; decisions PROPOSED].
- **Direction:** generalize the F6.1 probe architecture (vantage identity, assignment, signed results, per-vantage evidence) to further check types. Browser journeys need research.
- **Depends on:** the F6.1 decisions; OPS-1 closure.
- **Validate:** OPS-1 A–E in Kubernetes.

**3.10 Network and service connectivity.**
- **Scope:** DNS, TLS, endpoints, connectivity and dependency availability.
- **Today:** TLS certificate expiry detection (F6.1) [VALIDATED outside Kubernetes]; network flow tables (migration `005`); `NetworkPage` [CODE]; eBPF network probes are stubs [AUDIT F-093].
- **Direction:** DNS/TLS/endpoint evidence from multiple vantages; flow data only after an eBPF or collector decision.
- **Depends on:** D-13.
- **Validate:** multi-vantage disagreement detection (the F6.1 vantage-correlation pattern in `internal/correlate`; its decisions, including F6.1-SPAN-1, are PROPOSED).

**3.11 Alerting and incident management.**
- **Scope:** actionable alerts, routing, deduplication, escalation, timelines and ownership.
- **Today:** processor alerting; `alert_rules`; on-call and postmortem UI and tables [CODE]; [AUDIT] F-007 (ack/resolve no-ops); F6.1 result outbox [CODE].
- **Direction:**
  - one event-outbox pattern for every alert source, reusing the F6.1 result outbox (`internal/result/f61`, migration `009`) [CODE; F6.1-DEST-1 PROPOSED];
  - deduplication and grouping keyed on entity identity;
  - routing and escalation persisted per tenant.
- **Depends on:** D-05, D-07.
- **Validate:** alert-to-incident latency; duplicate rate on replayed incidents.

**3.12 Root-cause investigation.**
- **Scope:** correlate logs, metrics, traces, deployments, configuration, topology and alerts.
- **Today:** knowledge catalog and certificate-expiry detector [DECIDED inputs; VALIDATED focused tests]; multiple AI agents [AUDIT F-060].
- **Direction:**
  - evidence-graph investigation: from an alert's entity, traverse topology, recent changes and correlated signals within a time window;
  - deterministic detectors bound to catalog patterns;
  - **no external LLM** (D-14-E).
- **Validate:** time-to-diagnosis experiments (§5).

**3.13 SLOs and reliability management.**
- **Scope:** SLIs, objectives, error budgets and reporting.
- **Today:** `slos` table; processor SLO logic; `SLOsPage` [CODE]; F-042 in-memory windows [AUDIT].
- **Direction:** SLIs defined over the shared metric model; burn-rate alerts; durable SLO state.
- **Depends on:** D-04, D-19.
- **Validate:** burn-rate correctness on synthetic series.

**3.14 Deployment and change intelligence.**
- **Scope:** correlating releases and configuration changes with telemetry and incidents.
- **Today:** deployment markers (migration `004`); `ChangeTrackingPage`, `DeploymentIntelligencePage`, `ReleaseHealthPage`; `processor/regression.go` [CODE].
- **Direction:** change events as entities with version attributes; automatic before/after comparison around each change.
- **Validate:** change-correlation precision on seeded regressions.

**3.15 Security and governance.**
- **Scope:** tenant isolation, RBAC, audit, secrets, encryption, retention and privacy.
- **Today:** S1-06 internal token [CODE; partially validated per `07`]; probe credentials with revocation [VALIDATED outside Kubernetes]; `audit_log` [CODE]; [AUDIT] F-004, F-005, F-008.
- **Direction:** see §4.4.
- **Depends on:** D-07, D-08, D-09, D-10, D-21, AQ-8.
- **Validate:** cross-tenant test suite; secret-in-log tests.

**3.16 Platform operations.**
- **Scope:** onboarding, agents and collectors, integrations, APIs, multi-tenancy, scalability, self-monitoring and DR.
- **Today:** Helm, Compose, agent install pages [CODE]; [AUDIT] F-095 (no backup or DR), F-096, F-106.
- **Direction:**
  - a supported deployment matrix (D-20);
  - self-observability through a path independent of ObserveX (D-15);
  - backup and restore (D-19);
  - schema migration governance (D-03, MIG-1a).
- **Validate:** install, upgrade and restore drills in disposable environments.

---

## 4. How the capabilities work together: proposed shared telemetry architecture

All of §4 is **[PROPOSED]**, except where it cites existing code.

### 4.1 Layers

```
 Instrumentation      OTel SDKs · ObserveX RUM SDK · host/K8s collectors · synthetic probes (F6.1 pattern)
        │             (resource attributes: tenant*, environment, service, version, host, cluster, workload)
        ▼
 Collection/gateway   OTel-compatible collector or gateway tier (D-13) · existing ActiveGate proxy (D-02, PROPOSED; its option B keeps it as an optional add-on)
        ▼
 Ingestion API        OTLP/HTTP (+ gRPC: research) and native endpoints (existing ingestor) · authN → tenant stamping
        ▼
 Normalization        semantic-convention mapping · resource-identity resolution · PII/secret scrubbing (AQ-8)
        ▼
 Durable transport    queue/stream (D-05) — replaces synchronous fan-out (F-064)
        ▼
 Storage per signal   metrics · logs · traces · events · profiles (D-04: ClickHouse-centric vs LGTM vs hybrid)
        │             tenant key in every table/sort key (D-07)
        ▼
 Processing           stream evaluation: alerts, SLO burn, detectors (catalog-bound), durable state (F-042)
        ▼
 Entity & correlation  entity store (services, workloads, hosts, checks, changes) + relationship graph
        ▼
 Query & API          unified query API (existing query-engine) · public REST/gRPC · integrations
        ▼
 Experience           unified UI: entity-centric pages; investigation workspace; dashboards
```

`*` Tenant is **never** taken from telemetry payloads or headers. It is derived from the authenticated sender.

### 4.2 Resource identity and correlation

- **Identity keys** [PROPOSED]:
  - `tenant` (from auth);
  - `environment`;
  - `service.name` plus `service.version`;
  - `k8s.cluster`, `k8s.namespace`, `k8s.workload`;
  - `host.id`;
  - `cloud.account` and `cloud.region`;
  - for synthetic evidence, `check_id` plus `vantage_id`. The F6.1 pattern already binds vantage identity server-side [CODE].
- **Correlation joins:**
  - traces ↔ logs: `trace_id`/`span_id`;
  - traces ↔ metrics: exemplars, plus shared resource attributes (**[RESEARCH]** exemplar support depends on D-04);
  - metrics and logs ↔ changes: entity plus a time window;
  - alerts ↔ entities: entity key;
  - RUM ↔ backend: trace context propagation;
  - profiles ↔ traces: **[RESEARCH]**.
- **One entity store.** Every page, alert and investigation resolves to entities. The existing `services` and topology-edge ingestion [CODE] is the starting point.

### 4.3 Storage, query, retention and cost

- **[PROPOSED]** Choose the storage topology once (D-04), with explicit per-signal retention (D-16), downsampling, tenant quotas and cardinality limits (D-18, F-041). The query cost guardrails in `04` are unconfirmed placeholders.
- **[RESEARCH]** Measure ingest cost per GB and per active series on the chosen stack before any pricing or scale claim.

### 4.4 Security and multi-tenancy

**[PROPOSED]**, building on the F6.1 patterns.
- **Isolation.** Tenant isolation in one data-access layer, plus database-level defence (D-07 recommendation B: mandatory `org_id` plus PostgreSQL RLS). A cross-tenant test suite gates every release.
- **Service identity.** Per-service identity (D-10), and dedicated, scoped credentials per external sender (the F6.1 probe-key model).
- **Secrets.** Secrets management (D-08). No secrets in logs (**AQ-8, not approved**; today only the internal-token rule P-1 condition 3 is approved).
- **Audit and encryption.** An immutable audit trail. Encryption in transit (TLS with hostname verification, as in D1) and at rest (**[RESEARCH]** per storage choice).
- **Retention and privacy.** Controls per D-16 and D-21.

### 4.5 Real-time processing and alert evaluation

**[PROPOSED]**
- Processor state becomes durable and horizontally partitionable (F-042, D-19).
- Evaluation is idempotent per event (the F6.1 monotonic per-vantage state is a reusable pattern).
- Results flow through an outbox to notification routing.

### 4.6 Unified experience

**[PROPOSED]**
- Entity-centric navigation.
- An investigation workspace that pins evidence (signals, changes, topology) to an incident timeline.
- Mocked or preview features are flagged per D-12 until they are backed by real data.

---

## 5. Differentiation: hypotheses and research backlog

**[RESEARCH]** Nothing in this section is a verified fact about ObserveX, competitors or customers. **No competitor research, customer interviews or benchmarks were performed for this charter.**

### 5.1 Hypotheses (to test, not claims)

| ID | Hypothesis | Measurable acceptance criterion for a future experiment |
|---|---|---|
| H1 | One enforced resource-identity model across all signals shortens investigation time | Controlled exercise: N seeded incidents, M engineers. Median time-to-correct-root-cause with ObserveX investigation vs a baseline workflow; success = a reduction the owner sets in advance |
| H2 | Evidence-first, catalog-bound deterministic detectors produce explainable findings with acceptable precision (consistent with D-14-E) | On a labelled incident corpus: precision and recall per detector, against thresholds the owner sets in advance; 100% of findings carry evidence fields |
| H3 | Multi-vantage synthetic evidence (the F6.1 pattern) catches partial failures that single-vantage checks miss | Seeded partial-renewal scenarios: detection rate for multi-vantage vs single-vantage |
| H4 | A self-hosted deployment with a small component footprint lowers operational complexity for target customers | Install-to-first-signal time; component count; upgrade success rate across the D-20 matrix |
| H5 | Suggest-by-default automation with approvals is acceptable to operators | Structured usability study; share of suggested actions accepted; zero unapproved actions executed |
| H6 | Telemetry cost per tenant can be kept predictable with quotas and downsampling | Cost per GB and per series on reference workloads; variance within an owner-set bound |

### 5.2 Research backlog

| # | Topic | Questions | Method | Output | Done when |
|---|---|---|---|---|---|
| R1 | Existing platforms' documented capabilities | Which full-stack capabilities are standard, and which documented gaps exist? | Review public documentation of commercial and open-source platforms, with citations | Capability matrix with sources | Every cell cites a public source and date |
| R2 | Open-source alternatives and interoperability | What can ObserveX interoperate with or build on (OTel, storage engines)? | Documentation review plus prototypes | Interop matrix, licence notes (D-04 AGPL question) | Licence review complete (legal) |
| R3 | Full-stack integration gaps | Where do users switch tools during an investigation? | Interviews (to be conducted; none done) | Journey maps | ≥ owner-set number of interviews recorded |
| R4 | Deployment models | Self-hosted vs SaaS demand and constraints (D-01) | Interviews plus requirements | Deployment requirements | Owner decision on D-01 |
| R5 | Cost, scale and operational complexity | Ingest and storage cost; operator effort | Load tests on disposable infrastructure | Cost model | Measured on reference workloads |
| R6 | Investigation workflows and time-to-diagnosis | Which workflow steps dominate? | Observation studies; H1 experiment | Baseline metrics | H1 run completed |
| R7 | Security, privacy, enterprise | SSO, RBAC, audit, residency, compliance (D-21) | Requirements gathering | Requirements catalogue | Owner-approved scope |
| R8 | Candidate distinctive capabilities | Which of H1–H6 survive feasibility and customer validation? | Prototype plus experiment per hypothesis | Go/no-go per hypothesis | Criteria in §5.1 met or rejected |

---

## 6. Phased roadmap and implementation gates

**[PROPOSED]** The roadmap builds on the existing code. It maps onto the proposed Stages 1–7 of `02_Implementation_Roadmap.md` (session store; authority unknown), without replacing them silently; mapping is CH-2. **No dates.**

### Phase 0: Governance, baseline reconciliation, owner decisions, OPS-1 evidence closure

- **Scope:** GOV-1 … GOV-5; C13; the seven facts; migration, credential and security decisions (register §3–§4); OPS-1 external validation.
- **Reuse:** this package; the six OPS-1 owner documents; the external runbook; VR.
- **New:** nothing in code.
- **Gates:** decisions recorded in the approval log of record; C13 resolved.
- **Tests:** the external runbook A–G on an authorized, disposable machine; CNI enforcement proof.
- **Acceptance:**
  - every register item has a recorded owner status;
  - OPS-1 A–E recorded as PASS in Kubernetes, **or** documented as FAIL/BLOCKED with evidence;
  - no approval field filled without a log entry.
- **Out of scope:** new features.
- **Maps to:** the `02` Stage 1 preconditions.

### Phase 1: Secure, reliable platform foundation

- **Scope:**
  - identity and sessions (D-09), service identity (D-10), tenancy (D-07), secrets (D-08);
  - configuration fail-fast (S1-05/A4, not approved);
  - migrations (D-03, MIG-1a outcome);
  - ingestion durability (F-040, D-05);
  - deployment safety (CI, Helm lint/template, image builds);
  - applying or replacing `PROTECTED-FILES.patch`, only if authorized.
- **Reuse:** S1-06 service token; F6.1 credential and revocation patterns; `internal/db/dbtest` fixtures; chartcheck static tests.
- **New:** tenancy data-access layer; migration tool per D-03; queue per D-05.
- **Gates:** approvals of A2–A10 as the owner directs after C13; security review for identity and secrets.
- **Tests:** cross-tenant suite; migration fresh and upgrade suites per supported history (AQ-2); secret-in-log tests; `go test -race` and CI on owner-controlled runners (P-0 CI-1 … CI-5).
- **Acceptance:**
  - zero cross-tenant reads in the suite;
  - every telemetry table carries the tenant key;
  - migrations pass on every supported PostgreSQL version (F-3);
  - CI green on a clean checkout.
- **Out of scope:** new signal types.
- **Maps to:** `02` Stages 1–3.

### Phase 2: Unified telemetry foundation

- **Scope:** instrumentation and collection (D-13); OTLP ingestion conformance; metrics, logs and traces storage (D-04); resource identity; correlation keys; retention and cost controls (D-16, D-18); self-observability (D-15).
- **Reuse:** ingestor endpoints, query-engine, Loki/Tempo/ClickHouse charts (subject to D-04).
- **New:** normalization and identity service; entity store.
- **Gates:** D-04, D-13, D-15, D-16 and D-18 recorded (`02` Stage 4 lists D-04, D-15, D-16, D-18; D-13 is added here because collection depends on it).
- **Tests:** OTLP spec test vectors; end-to-end trace → log → metric pivots on a reference app; load tests on disposable infrastructure.
- **Acceptance:**
  - OTLP/HTTP accepted for metrics, logs and traces per the spec test vectors;
  - 100% of stored records carry the identity keys;
  - removal of the fabricated-data paths behind F-007, or flagging per D-12.
- **Out of scope:** AI-assisted RCA.
- **Maps to:** `02` Stage 4.

### Phase 3: Full-stack visibility

- **Scope:** infrastructure, Kubernetes, APM, databases, network, RUM and synthetic, all on the Phase 2 foundation. Generalize the F6.1 probe architecture.
- **Reuse:** oneagent (after D-13), db-monitor (after its contract), `sdk/rum`, the F6.1 probe service.
- **New:** per-domain collectors and entity types.
- **Gates:** privacy review for RUM (D-21); support matrix (D-20).
- **Tests:** per-domain correctness against reference environments; overhead budgets.
- **Acceptance:** each domain's entities are visible and correlated, with documented overhead within owner-set budgets.
- **Out of scope:** automated remediation.
- **Maps to:** no single `02` stage. It extends `02` Stage 4 scope and depends on D-13, which `02` places at the Stage 6 entry gate; reconciling this is part of CH-2.

### Phase 4: Incident intelligence

- **Scope:** service maps, SLOs and burn rates, change correlation, investigation workspace, incident management, deterministic detectors bound to the knowledge catalog (D-14-E; D-INT-2/3).
- **Reuse:** `internal/knowledge`, `internal/detect/certexpiry`, processor forecasting (after validation), the F6.1 outbox.
- **New:** evidence-graph investigation; incident timeline.
- **Gates:** D-11 for any action path (suggest-only by default); D-14-E compliance review.
- **Tests:** H1–H3 experiments.
- **Acceptance:** criteria in §5.1 met as the owner sets them.
- **Out of scope:** external LLM use (prohibited by D-14-E).
- **Maps to:** `02` Stages 5–6.

### Phase 5: Scale, enterprise readiness, integrations, validated differentiation

- **Scope:** HA and DR (D-19), quotas (D-18), compliance (D-21), integrations, advanced analytics, capabilities validated by R8.
- **Reuse:** all prior phases.
- **Gates:** security and compliance reviews; D-17 claims review.
- **Tests:** scale tests; DR drills; H4–H6.
- **Acceptance:** owner-set SLOs met on reference scale; DR restore drill succeeds; every public claim traceable to evidence.
- **Out of scope:** capabilities that R8 did not validate.
- **Maps to:** `02` Stage 7.

---

## 7. Decisions reserved for the owner

- CH-1 (adopt as direction) and CH-2 (roadmap mapping).
- D-01 … D-22 (except the approved D-14-E).
- Every acceptance threshold marked "owner-set".
- Research priorities in §5.2.

## 8. Known conflicts and unresolved evidence

- README feature claims vs audit findings F-007, F-032 and others.
- Synthetic check types: 8 claimed in the README vs 5 in the processor code.
- OTLP JSON compatibility unverified.
- Governance conflicts C-1 … C-8 (reconciliation record).
- **OPS-1 INCOMPLETE:** A–E not run in Kubernetes; D2 is static only; gateway D4 unconfirmed; P3 not runtime verified.

## 9. Exact next action required

**[OWNER]** Complete Phase 0: GOV-1 and GOV-2 first (see `ObserveX-Owner-Decisions-Register.md`). Then CH-1, CH-2.

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
