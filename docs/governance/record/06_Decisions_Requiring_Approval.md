# 06 — Decisions Requiring Your Approval

Reply with the ID and your choice (e.g. `D-07: B, yes multi-org`). Anything left unanswered stays blocked. Full options and trade-offs are in `03`; item details are in `05`.

## A. Needed now — to start Stage 1 (containment)

| # | Decision | Recommendation | Your answer |
|---|---|---|---|
| A1 | Approve the **Stage 1 pack without behavior-default changes**: S1-01 (SAML containment), S1-04 (remove raw SQL / CORS), S1-06 (authorization gaps), S1-08 (ingest escaping), S1-09 (rollback revision fix) | Approve | ☐ |
| A2 | **S1-02** Disable SSO by default behind `SSO_ENABLED`. *Also confirm: is any environment using SSO today?* | Approve | ☐ |
| A3 | **S1-03** Remediation defaults to dry-run/suggest; approvals enforced; empty namespace allowlist = deny; internal token required | Approve | ☐ |
| A4 | **S1-05** Refuse to start with default or weak secrets (dev escape hatch `OBSERVEX_DEV_INSECURE`), including zero-key re-encryption path | Approve | ☐ |
| A5 | **S1-07** Ingest decompressed body limit. *Confirm value (proposed 16 MiB) and the agents' max batch size.* | Approve with 16 MiB | ☐ |
| A6 | **S1-10** Per-component ServiceAccounts, remove `pods/exec`, NetworkPolicy on by default, minimal agent privileges (staging first) | Approve | ☐ |
| A7 | **S1-11 + D-03** goose migrations + additive reconciliation migration `008` with canonical names `namespace_permissions`, `password_reset_tokens`, add `sessions.org_id`, create missing tables | Approve | ☐ |
| A8 | **S1-12** Disable the seeded admin (if still using the seeded hash) and add a one-time bootstrap admin | Approve | ☐ |
| A9 | **Toolchain access:** allow `proxy.golang.org` and `sum.golang.org` in this environment's egress settings, *or* agree that I prepare Go patches and you run build/tests locally | Allow egress | ☐ |
| A10 | Confirm the **S0 patch** (RUM test hang fix + password-change client field) can be merged | Merge | ☐ |

## B. Needed before Stage 2

| # | Decision | Recommendation | Your answer |
|---|---|---|---|
| B1 | **D-22** Repository deletions (list in `05` S2-03), archive branch first | Approve | ☐ |
| B2 | **S2-02** Restore canonical Go module paths; Go ≥ 1.23 | Approve | ☐ |
| B3 | **S2-07** Dependency upgrades including major versions | Approve | ☐ |
| B4 | **F-021** Release policy: CD blocks on Critical image vulnerabilities (with an exceptions file) | Approve | ☐ |

## C. Needed before Stage 3 (architecture)

| # | Decision | Recommendation | Question for you | Your answer |
|---|---|---|---|---|
| C1 | **D-01** Deployment model | D: self-hosted first, SaaS-ready | Business target and timeline? | ☐ |
| C2 | **D-07** Multi-tenancy | B: shared stores + mandatory `org_id` + RLS | Should users belong to multiple orgs? | ☐ |
| C3 | **D-02** Service architecture | B: consolidated modular core; retire duplicate agents | — | ☐ |
| C4 | **D-04** Telemetry storage & query language | A (ClickHouse-everything) unless PromQL is required → B (LGTM) | Is PromQL/Grafana compatibility a must? Is AGPL acceptable? | ☐ |
| C5 | **D-05** Streaming/queue | NATS JetStream + Postgres outbox for remediation | Expected ingest volume? | ☐ |
| C6 | **D-06** SSO implementation | Disabled now → vetted Go libraries in Stage 3 | Build vs buy preference? | ☐ |
| C7 | **D-08** Secrets management | External Secrets Operator; SOPS as documented minimum | Which secret backends do customers use? | ☐ |
| C8 | **D-09** Sessions/API auth/authorization | HttpOnly session cookies + scoped API keys + route RBAC matrix | Custom roles needed early? | ☐ |
| C9 | **D-10** Service-to-service identity | Shared token now → workload tokens + NetworkPolicy | — | ☐ |
| C10 | **D-19** Availability & recovery | Tier B (multi-replica, HA Postgres, RPO ≤ 15 min, RTO ≤ 1 h) | Ingest vs UI availability targets? | ☐ |

## D. Needed before Stages 4–7

| # | Decision | Recommendation | Question for you | Your answer |
|---|---|---|---|---|
| D1 | **D-12** Mock/preview feature policy | Preview flag + badge; off in production | Separate sales-demo build? | ☐ |
| D2 | **D-15** Self-observability | Prometheus + OTel + external monitoring path | — | ☐ |
| D3 | **D-16** Data retention | Per-type defaults (15 d raw metrics, 13 mo downsampled, 30 d logs, 7 d traces, ≥ 1 y audit) + tenant overrides | Required retention values? | ☐ |
| D4 | **D-18** Cost controls | Request/query caps now; per-tenant quotas later | Confirm initial limits | ☐ |
| D5 | **D-14** AI provider & model strategy | Provider-abstracted; local default; hosted opt-in with redaction; LLM advisory only | Any restrictions on sending telemetry to third-party AI? | ☐ |
| D6 | **D-13** Agent architecture | OpenTelemetry Collector base + opt-in eBPF | Any customer commitments to the current agent protocol? | ☐ |
| D7 | **D-11** Remediation security model | Tiered suggest → approve → policy-scoped auto; GitOps executor option | Which actions may ever be auto-eligible? | ☐ |
| D8 | **D-20** Cloud/K8s support matrix | Helm on upstream K8s N-2 + EKS/GKE/AKS; Compose dev only | Any OpenShift or air-gapped commitments? | ☐ |
| D9 | **D-21** Compliance | SOC 2 path if SaaS; otherwise best practices; relabel "compliance PDF" | Required frameworks and deadlines? | ☐ |
| D10 | **D-17** Naming & claims | Rename trademarked components; align README claims | Preferred names? Legal review available? | ☐ |

## E. Assumptions to confirm (from `04`)
Team size and skills · 80% coverage on critical packages · 1 MiB API / 16 MiB ingest limits · 30 s / 10k-series query guardrails · 60 s session revocation · platform SLOs (ingest 99.9%, query p95 < 2 s, UI 99.5%) · RPO/RTO · 15 min rollback · 30 s kill switch · performance placeholders P1–P4 · 30 min agent buffering · 20 min CI.
