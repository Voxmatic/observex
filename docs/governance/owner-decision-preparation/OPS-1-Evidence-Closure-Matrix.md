# ObserveX — OPS-1 Evidence Closure Matrix

> **Proposed — not approved.** This is a plan for collecting evidence. **OPS-1 remains INCOMPLETE.**
> - Kubernetes tests A–E have **not run**.
> - No test here is reported as passed unless it was executed. The executed results are quoted from the validation report with their category.
> - The migration Job stays **disabled** in persistent environments (`f61.migrations.job.enabled: false`).
> - `PROTECTED-FILES.patch` is **not applied** and must not be applied.

| Field | Value |
|---|---|
| Document | `docs/governance/owner-decision-preparation/OPS-1-Evidence-Closure-Matrix.md` |
| Version | v0.1.0 (proposed) |
| Date generated | 2026-09-28 |
| Latest validation report | `docs/validation/f61-ops-1-kind-validation.md` ("VR"): executed 2026-09-26; addendum §8 dated 2026-09-27; SHA-256 `80a35c9f…`, unchanged |
| External runbook | `docs/validation/f61-ops-1-external-runbook.md` ("RB"): **NOT EXECUTED**; SHA-256 `855b461a…`, unchanged |

**Evidence categories.** These are kept separate throughout:

| Tag | Meaning |
|---|---|
| **[STATIC]** | Static checks: source inspection, the stand-in renderer, the policy evaluator, `kubeconform` on a non-Helm render. **Nothing is executed against Kubernetes.** |
| **[PROGRAM]** | Program-level tests **outside Kubernetes**: Go tests, real binaries, disposable PostgreSQL 16 |
| **[K8S]** | Actual Kubernetes tests in a cluster. **None has been executed.** |
| **[EXT]** | External-environment prerequisites: an authorized machine, egress, tooling, charts |

`$EV/NN-…` names are the evidence files RB tells the operator to produce.

---

## 1. External-environment prerequisites

### 1.1 Status and evidence

| ID | Item | Category | Current status | Evidence available | Evidence missing | Environment and prerequisites | Owner authorization |
|---|---|---|---|---|---|---|---|
| X1 | Authorized disposable machine | [EXT] | **Not available** | None | A named machine that meets RB §0 | A fresh VM or workstation; no production credentials; a dedicated kubeconfig; every command pinned to context `kind-f61-ops1` | **Yes.** An organisational decision to provide it (Decision-Dependency-Matrix M13) |
| X2 | Egress or pre-seeded tooling | [EXT] | **BLOCKED** in Claude's sandbox | VR §1 lists 20 hosts refused with 403 (registries, `get.helm.sh`, `dl.k8s.io`, chart repositories, `sum.golang.org`) | Reachable hosts per RB §1, or a pre-seeded local registry and chart cache | RB §1 host list. **No bypass of egress controls** | **Yes.** An egress or tooling change only you or your organisation can make |
| X3 | Bitnami `postgresql` 15.5.4 chart and images available | [EXT] | **Unknown** (RB §1 "Check first") | None | A successful fetch, or the exact error | X2 | Choosing a replacement chart, if needed, is **your decision** |
| X4 | Repository checks on the authorized machine (RB §2) | [PROGRAM] | **NOT RUN there** | In Claude's sandbox (VR §8.4): `go test -race`, **277 passed, 0 failed, 0 skipped**; frozen packages PASS; `go vet` and `gofmt` clean | The same suites on the authorized machine; a disposable PostgreSQL started by RB §2 | X1, X2 (Go modules via `proxy.golang.org`, `sum.golang.org`) | Covered by X1 |
| X5 | Image builds (RB §3) | [EXT] then [PROGRAM] | **BLOCKED** (VR §3.0: every base-image pull refused) | Partial substitute only: `go vet -stdversion` exit 0. **This is not evidence that the images build** | Four real builds, base-image digests, image users and entrypoints | X1, X2 | Covered by X1 |
| X6 | Helm dependency build, lint and template (RB §4) | [STATIC] on real Helm output | **BLOCKED** (no Helm; chart repositories 403) | Stand-in render only: `kubeconform -strict` **26/26** (defaults) and **39/39** (F6.1 enabled); 24 design checks; chartcheck D1/D2/credential-source tests pass on the stand-in. **"A render by this harness is not evidence that `helm template` succeeds"** (VR §3.0) | `helm dependency build`, `lint` and `template` outputs; chartcheck re-run against real output | X1, X2, X3 | Covered by X1 |
| X7 | kind cluster and **CNI enforcement proof** (RB §5) | [K8S] | **BLOCKED** (node image pull refused) | None. Whether kindnet enforces NetworkPolicy is **unknown** | Cluster creation; the deny-all baseline result | X1, X2; kind v0.24.0 with the digest-pinned node image; Calico if kindnet does not enforce | Covered by X1 |
| X8 | Install (RB §6–§7) | [K8S] | **NOT RUN** | None | Install logs, resources, policies, the migration Job log | X4–X7; test-only secrets; the synthetic TLS target | Covered by X1. **The migration Job runs only against this throwaway database** |

### 1.2 Artifacts and acceptance criteria

| ID | Expected artifacts | Pass | Fail or BLOCKED |
|---|---|---|---|
| X1 | A written authorization naming the machine; `$EV` directory created | Machine meets RB §0; no production credentials present | No authorized machine: record **BLOCKED** |
| X2 | `00-tool-versions.txt` | kind, kubectl and Helm **checksums verified** against official releases; versions recorded | Any required host unreachable: record **BLOCKED** with the error; **do not** use unofficial mirrors |
| X3 | `20-helm-dep.txt`, `21-Chart.lock` | Dependency fetched; lock file recorded | Fetch fails: record **BLOCKED** with the exact error |
| X4 | `01-go-mod.sha256`, `02-vet.txt`, `03-go-test-unit.txt`, `04-go-test-db.txt` | vet clean; all tests pass; `grep -c -- '--- SKIP'` on `04` prints **0**; `go.mod`/`go.sum` unchanged | Any failure or skip |
| X5 | `10-build-<svc>.txt`, `11-base-image-digests.txt`, `12-built-images.txt` | All four builds succeed; `synthetic-probe` runs as `nonroot` with entrypoint `/service` and cmd `serve`; `db-migrate` runs as `70:70` (RB §3) | Any build fails, or the user or entrypoint differs |
| X6 | `22-helm-lint.txt`, `23-rendered-f61.yaml`, `23-render.err`, `24-rendered-kind.yaml`, `25-chartcheck-real-helm.txt` | Lint and template succeed; `TestD1ProbeDialsTheCertificateName`, `TestD2MigrationJobReachesPostgreSQLOnly` and `TestCharacterizationDatabaseCredentialSources` pass **against real Helm output** | Any error; any of the three tests fails |
| X7 | `30-kind-create.txt`, `31-nodes.txt`, `32-kube-system.txt`, `33-cni-baseline.txt` | Before the deny-all policy: `200`. After it: timeout or non-zero exit. The CNI and its version are recorded | The request still returns `200`: the CNI does **not** enforce. Recreate with an enforcing CNI (RB §5); **test E does not count** until the baseline passes |
| X8 | `40-helm-install.txt`, `41-resources.txt`, `42-networkpolicies.yaml`, `43-migrate-job.log`, `44-port-forward.log` | `helm exit=0`; processor and gateway rollouts complete; the probe waits for its credential Secret (two-phase enrolment) | Helm failure; hook Job failure |

**Known workarounds in the disposable cluster only** (RB §7). Each must be recorded, not treated as a fix:
- `postgresql.auth.password` is set to the chart password, working around P3.
- The seeded admin's password is set by SQL, working around P5.
- `trivyScanner` is disabled (P1).
- db-monitor may stay in `ImagePullBackOff` (P2).

---

## 2. Kubernetes acceptance tests A–G (original definitions, RB §8)

### 2.1 Status and evidence

| Test | Category | Current status | Evidence available (not Kubernetes evidence) | Evidence missing | Environment and prerequisites | Owner authorization |
|---|---|---|---|---|---|---|
| **A.** Work delivery over TLS | [K8S] | **NOT RUN in Kubernetes** (VR §8.7: BLOCKED) | [PROGRAM] D1 regression: `TestF61ProbeVerifiesIntakeServiceName` (5 subtests) and `TestF61IntakeNameVerifiesAgainstSAN` PASS; real processor and probe binaries logged `work refreshed` with the `.svc` name (VR §8.2). [STATIC] `TestD1…` PASS on the stand-in | A probe in a cluster receiving its assignment over TLS | X1–X8; one SSL check in zone `kind-a`; an issued probe credential | X1 |
| **B.** TLS observation and reporting | [K8S] | **NOT RUN in Kubernetes** | None that counts. VR §3.B: process-level runs "are **not** Kubernetes evidence" | Result `expiring`/`open` → `opened` event → renewal → `ok`/`none` and a `resolved` event | A passed; the synthetic target (20-day certificate, then 200-day) | X1 |
| **C.** Revocation | [K8S] | **NOT RUN in Kubernetes** | [PROGRAM] VR §3.C: revocation outside Kubernetes. Probe `401` → "dropping all assignments"; `/readyz 503`; old credential `401` on work and report | The same in a cluster, with pod readiness and a direct client check | A passed; a test client carrying the probe's network labels | X1 |
| **D.** Credential rotation | [K8S] | **NOT RUN in Kubernetes** | [PROGRAM] VR §3.D: kubelet-style Secret-volume swap outside Kubernetes. Resumed without restart; old `401`, new `200`. **"This does not show kubelet's actual Secret propagation"** | Real Secret propagation; restart count unchanged | C passed (the revocation bound has passed) | X1 |
| **E.** NetworkPolicy installed and enforced | [K8S] | **NOT RUN** | [STATIC] policy evaluation of the stand-in render; `TestD2MigrationJobReachesPostgreSQLOnly`, `TestD2PolicyOnlyWhenJobAndPoliciesEnabled` PASS. **A model, not a CNI** | The allow/deny matrix under a **proven-enforcing** CNI | **X7 baseline passed**; A–D ran with the policies enforced | X1 |
| **F.** Migrations, in-cluster Job | [K8S] (Job) and [PROGRAM] (local) | [PROGRAM] **PASS: 25/25** on local PostgreSQL 16, with the supported script (VR §3.F). Repository migration tests PASS. [K8S] Job **NOT RUN**. Re-run semantics **UNRESOLVED** (MIG-1a) | Fresh, E1 and E2 upgrades; ordering; data preservation; fail-fast; F5 characterization | The Job log in a cluster on a **throwaway** database | X8. **The Job is enabled only in `f61-kind-values.yaml`** for the disposable cluster | X1. **No persistent database.** Re-running the Job against any database you keep is prohibited (RB §8 F) |
| **G.** Cleanup | [EXT]/[K8S] | [PROGRAM] **PASS** in Claude's sandbox (VR §3.G, §8.7). **NOT RUN** on the external machine | Sandbox cleanup record | External cleanup evidence | After A–F | X1 |

### 2.2 Artifacts and acceptance criteria (RB §8, unchanged)

| Test | Expected artifacts | PASS requires all of | FAIL |
|---|---|---|---|
| A | `50-check.json`, `51-probe-A.log`, `52-processor-listener.log` | Probe log `"work refreshed"` with `"assignments":1` and no TLS error. Processor log `f61 probe listener started (TLS)`. Probe `OBSERVEX_PROCESSOR_URL` = `https://$TLSNAME:8443`. Listener certificate SAN exactly `$TLSNAME` | Any criterion not met |
| B | `53-result-expiring.json`, `54-events-1.json`, `55-db-observations.txt`, `56-result-renewed.json`, `57-events-2.json` | First `status`=`expiring`, `alert_state`=`open`; an `opened` event; a row in `f61_tls_observations`; after renewal `status`=`ok`, `alert_state`=`none`, and a `resolved` event with the same `episode_id`. Record that `trusted` is false (no target CA) | Any criterion not met |
| C | `60-revoke.json`, `61-probe-C.log`, `62-probe-readiness.txt`, `63-revoked-cred-work.txt` | Probe log `401` then `"dropping all assignments"`; probe pod `0/1` Ready; direct request with the revoked credential returns `401` | Any criterion not met |
| D | `70-rotated.txt`, `71-probe-D.log`, `72-old-cred.txt`, `73-new-cred.txt`, `74-restarts.txt` | `rotated` = `True` with the same vantage ID; probe Ready again **without restart**; logs `"work refreshed"` then `"probe reported"`; old credential `401`, new `200` | Any criterion not met |
| E | `33-cni-baseline.txt`, `80-policies.txt`, `81-probe-network.txt`, `82-other-network.txt` | Every line matches its expectation; A–D ran with the policies enforced; the Job completed under the policies (`43-migrate-job.log` contains `db-migrate: done`; `40-helm-install.txt` ends `helm exit=0`). Exit 7 counts only where the destination is shown listening; the `169.254.169.254` line is recorded as not meaningful on kind | Any line differs from its expectation |
| F | `43-migrate-job.log`; `schema_migrations` query output | Job log lists `001_initial` … `007_feature_state`, then `009_f61_tls_certificates` (**there is no `008` file**), then `db-migrate: done`; the ledger order matches | Job fails, or the order or set differs |
| G | `90-clusters-after.txt`, `91-containers-after.txt` | No kind cluster, container or kubeconfig remains; no other context used; `grep -rl -E 'oxpt_[A-Za-z0-9]{8}' "$EV"` prints nothing | Any residue, or a secret found in `$EV` |

---

## 3. Outstanding findings and deployment items

### 3.1 Status and evidence

| ID | Finding | Category of current evidence | Current status | Evidence missing | Environment and prerequisites | Owner authorization |
|---|---|---|---|---|---|---|
| **D1** | Probe dialled the short Service name; the certificate SAN is the `.svc` name | [PROGRAM] + [STATIC] | **Fixed in chart** (a correction to the PROPOSED Increment-3 chart); **Kubernetes validation pending** | Test A in a cluster | As test A | X1. The chart change itself stays PROPOSED under G-1 |
| **D2** | Migration Job had no egress to PostgreSQL under NetworkPolicy | [STATIC] only | **Fixed statically; CNI enforcement untested** | Test E and the Job completing under enforced policies | X7 baseline, then E | X1 |
| **D3** | Misleading "report could not be delivered" text on work requests | [PROGRAM] (probe log) | Open, unchanged; not an A–G criterion | — | — | Not required for OPS-1 |
| **D4, processor** | An unescaped `OBSERVEX_F61_POSTGRES_DSN` leaks password fragments to logs and DNS | [PROGRAM]: reproduced locally (VR §8.6) | **Not fixed**; proposal only | If a fix is authorized: a regression test with a URL-reserved-character password | Disposable, **network-isolated** (no external DNS resolution) | **Yes:** a D4 processor scope decision (Decision-Dependency-Matrix M9a) before any fix or test is written |
| **D4, gateway** | `store.DefaultConfig` composes the URL without escaping; pgx `ConnectError` echoes the parsed host, user and database (ADD-F6). `maskDSN` logs the first 20 DSN characters (ADD-F7) | [STATIC] source inspection only | **Unconfirmed; not reproduced** | Reproduction evidence; log capture | As the processor path; no external DNS | **Yes:** a separate gateway scope and a **security review** (AQ-10, M9b). A processor decision does not cover it |
| **P3** | Chart Secret password not wired to the PostgreSQL sub-chart | [STATIC] source inspection of Bitnami 15.5.4; `TestCharacterizationDatabaseCredentialSources` PASS (static) | **Confirmed at source level; runtime NOT verified; not fixed** | Runtime observation (RB §9, step W-D); after a decision, a fresh install without the workaround | Disposable cluster (X1–X8), installed **without** `postgresql.auth.password` | X1 for observation. **P3 decision (M8)** for any fix |
| **F5** | Every run of the interim runner re-executes seeds and backfills | [PROGRAM]: 5 repository tests, plus the concurrency characterization | **Characterized; not fixed**; MIG-1a unresolved | MIG-1a tests 3 … 12, after Q1 is decided and implemented | Disposable databases only | **Yes:** MIG-1a Q1 (M6) |
| **Migration Job, persistent** | Enabling the Job outside a throwaway cluster | — | **Disabled; must stay disabled** | — | — | M14: Q1 (and Q2), F-7 authority, backup, D2 enforcement on that CNI |
| **P1** | `trivy-scanner.yaml` cannot render while enabled | [STATIC] | Pre-existing, unchanged; disabled in the kind values | Real Helm render with it enabled | X6 | Not required for OPS-1; any fix needs its own authorization |
| **P2** | db-monitor image depends on `image.tag` | [STATIC] | Pre-existing, unchanged | Record its pod state in X8 | X8 | As P1 |
| **ADD-F5** | db-monitor reads `postgres-dsn` from `<fullname>-secrets`, which the chart never creates; its Deployment is always rendered | [STATIC] | Unresolved; no credential contract (AQ-9) | Its pod state in X8 (expected not to start; **unverified**) | X8 | **Yes:** db-monitor inclusion and contract (M11) |
| **P4** | The chart applies no migrations unless the Job is enabled | [STATIC] | Pre-existing | — | — | Relates to M14 |
| **P5** | Seeded admin hash does not match the documented password | [PROGRAM] | Pre-existing; disposable-cluster workaround in RB §7 | — | — | Relates to M10 (A8) |
| **P6** | CI uses `working-directory: observex`, which does not match the layout | [STATIC] | Pre-existing; `ci.yml` is protected and unchanged | — | — | Relates to M7 and RP-1 |
| **Install ordering** | Two-phase enrolment: the probe needs a credential Secret that only the running gateway can issue. Post-install hook ordering with `--wait` | [STATIC] | Documented (VR §6; RB §7) | Observation in X8 and test A | X8 | X1 |
| **Image hardening** | Dockerfiles, image users, read-only root filesystems | — | **Untested** (VR §6) | X5 image inspection; pod security context observed in X8 | X5, X8 | X1 |

### 3.2 Artifacts and acceptance criteria

| ID | Expected artifact | Pass | Fail |
|---|---|---|---|
| D1 | Test A artifacts | Test A PASS | Any TLS name error in `51-probe-A.log` |
| D2 | Test E artifacts; `43-migrate-job.log` | `f61-migrate-like` → PostgreSQL 5432 CONNECTED; → intake 8443 and public 443 BLOCKED; Job `db-migrate: done` under enforced policies | Any mismatch, or Job timeout |
| D4, processor | If authorized: test output with a throwaway URL-reserved-character password; log capture; resolver query capture | No password fragment in any log line or DNS query | Any fragment appears |
| D4, gateway | Confirmation: log capture and resolver capture under the same conditions. If a fix is authorized: the same test after the fix | Confirmation evidence is recorded either way (observed or not observed), with its inputs. After a fix: no fragment | After a fix: any fragment appears |
| P3 | `kubectl logs deploy/observex-api-gateway` excerpt; the Secret **key names** of `observex-postgresql`, with values never recorded (RB §9 prints data, so record names only) | Confirmation: the gateway shows a PostgreSQL authentication failure, and a chart-generated `password` key exists. After an approved fix: a fresh install authenticates without the workaround | After a fix: authentication still fails |
| F5 | MIG-1a §5 test outputs (after Q1) | Tests 3 … 12 pass as specified in MIG-1a | Any failure |
| ADD-F5, P1, P2 | X8 resource listing | Recorded as observed | — (recorded, not graded) |
| Install ordering, image hardening | `12-built-images.txt`, `41-resources.txt` | Matches RB §3 and the chart's security context | Any difference is recorded |

---

## 4. When OPS-1 can be called complete

All of the following, recorded as a **new dated section** of VR:
1. X1–X8 met, with their artifacts.
2. **A, B, C, D and E PASS in Kubernetes**, with E under a proven-enforcing CNI, and the chart checks re-run against real `helm template` output.
3. F: the in-cluster Job PASS on a **throwaway** database.
4. G: PASS on the external machine.
5. Every FAIL or BLOCKED result recorded with its evidence. None is omitted or re-labelled.

**What completion would not do:**
- It would **not** approve any G-1 or Increment-3 decision (all PROPOSED).
- It would **not** resolve MIG-1a, P3 or D4.
- It would **not** authorize enabling the migration Job in any persistent environment (M14).

**Status today: OPS-1 INCOMPLETE.**

---

## 5. Decisions reserved for the owner

- X1 and X2: the environment and its egress.
- X3: a chart replacement, if one is needed.
- M8 (P3), M9a and M9b (D4), M6 (MIG-1a), M11 (db-monitor), M14 (persistent Job).
- The sequencing choice in `Decision-Dependency-Matrix.md` §3.

**Next action.** **[OWNER]** Decide whether to authorize an external disposable machine with the RB §1 egress (X1, X2).

*Proposed — not approved. Generated 2026-09-28. OPS-1 remains INCOMPLETE.*
