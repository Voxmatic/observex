# 08 — Staging Readiness Checklist (minimum to unblock A1)

**Status:** Nothing below is verified. Every item is **Open** until you record evidence.
**Scope:** the minimum needed to validate the five A1 fixes (S1-01, S1-04, S1-06, S1-08, S1-09). A production-like staging environment for later stages is listed separately in §4.

## 1. Decisions needed (owner)

| # | Decision | Options | Why it's needed | Status |
|---|---|---|---|---|
| SD-1 | Staging approach for A1 | (a) Existing staging (b) Disposable kind cluster per run (c) Build long-lived staging first | Nothing else can be planned without it | Provisional: (b) disposable kind per run |
| SD-2 | Data policy | Synthetic only (recommended) / sanitized copy / production copy (**needs explicit approval + backup + restore plan**) | Recorded constraint: no production data without approval | Provisional: synthetic only |
| SD-3 | Deployment owner and how results return | Named person or CI job; pasted output or CI log links | C11: no "passed" claim without recorded command results | Provisional: owner team/CI; evidence format per `07` |
| SD-4 | Whether a legitimate SAML login must be proven in staging for A1 | Yes (needs test IdP) / No (mark as "not validated in staging") | Positive-control test for S1-01 | Provisional: Yes (isolated test IdP) |
| SD-5 | Whether a real rollback test is required for S1-09 | Yes (needs AI agent + disposable namespace + your per-run permission) / No (fake-client tests only) | Rollback mutates workloads | Provisional: Yes (per-run permission) |

## 2. Minimum infrastructure (either existing or disposable)

| # | Requirement | Needed for | Minimum acceptable | Status |
|---|---|---|---|---|
| SI-1 | Runner with Go 1.23+ and Docker | P-0 independent track; integration tests | Local machine or CI runner | Pending |
| SI-2 | Isolated cluster or host running ObserveX from the A1 branches | All items | kind/k3s with Helm, or Compose on a VM | Unknown |
| SI-3 | Postgres 16 with migrations 001–007 applied | S1-01, S1-06 | Container in the staging stack | Unknown |
| SI-4 | ClickHouse 24.3 with the init schema | S1-04, S1-08 | Container in the staging stack | Unknown |
| SI-5 | Ingress/reverse proxy that can block `/api/auth/sso/*` | SSO interim control | nginx/Traefik/cloud LB rule | Unknown |
| SI-6 | Internal token provisioned under the approved names (`OBSERVEX_INTERNAL_TOKEN[_FILE]`, key `internal-token`) | S1-04, S1-06 | Generated secret, not reused elsewhere | Not started |
| SI-7 | Network isolation from production (no shared DBs, secrets, IdP tenants or kubeconfigs) | All | Separate namespace/cluster and credentials | Unknown |
| SI-8 | Log access to gateway, query engine, ingestor, AI agent | Verifying blocks, 401/403/503, no token logging | `kubectl logs` / `docker logs` | Unknown |

## 3. Test fixtures

| # | Fixture | Needed for | Status |
|---|---|---|---|
| SF-1 | Two orgs (Org A, Org B) with viewer, editor and admin users in each; synthetic credentials | S1-01 cross-org, S1-06 route matrix | Pending |
| SF-2 | SAML config rows: Org A enabled with test cert; Org B none; Org C enabled without cert | S1-01 | Pending |
| SF-3 | Test IdP (e.g. Keycloak container) with a signing cert, **only if SD-4 = Yes** | S1-01 positive control | Pending |
| SF-4 | Synthetic telemetry payloads containing injection strings in metric names, service IDs and label keys/values | S1-08 | Can be provided with the PR |
| SF-5 | Sample app with ≥ 3 deployment revisions in a disposable namespace, **only if SD-5 = Yes** | S1-09 | Pending |
| SF-6 | Exploit reproduction scripts `tests/security/a1/*.sh` | All | Written with the PRs |

## 4. Before any production deployment (beyond A1)

Not required to *validate* A1 in a disposable environment, but required before production rollout:

- [ ] Long-lived staging that matches the production platform, ingress, network policy and secret backend (ties to D-20, D-08)
- [ ] Backup and restore of staging Postgres/ClickHouse demonstrated
- [ ] Rollback procedure for each A1 PR rehearsed in staging
- [ ] Production ingress block of `/api/auth/sso/*` verified with the curl check in `07`
- [ ] Owner sign-off on the A1 validation report

## 5. Exit criteria for "staging approach confirmed"

P-7 moves from **Pending clarification** to **Approved** when all of these are recorded:
1. SD-1 through SD-5 decided.
2. SI-1, SI-2, SI-3, SI-4, SI-7 and SI-8 confirmed available, with who owns each.
3. SF-1 and SF-2 confirmed creatable, plus SF-3/SF-5 if SD-4/SD-5 = Yes.
4. Result-sharing method (SD-3) agreed.

Until then: **A1 implementation, A2 and S0 remain on hold, and nothing is described as validated or production-ready.**
