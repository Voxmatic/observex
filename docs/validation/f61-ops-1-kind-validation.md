# F6.1 OPS-1 validation in a disposable kind cluster

**Status:** validation **blocked at the environment stage**. No kind cluster could be created and no image or Helm operation could run, because the sandbox's egress policy refuses every container registry and the Helm and Kubernetes download hosts. Acceptance tests A–E were therefore **not executed in Kubernetes**. Test F (migrations) and G (cleanup) were executed outside Kubernetes against disposable PostgreSQL 16 databases.
**Date:** executed 2026-09-26; egress re-checked 2026-09-27 13:34 UTC with the same result (registries, `get.helm.sh`, `dl.k8s.io`, chart repositories all still refused) · **Scope:** F6.1 Increment 3, OPS-1 (runtime and deployment) · **Governance:** every G-1 Increment-3 decision remains **PROPOSED and unapproved** (`ObserveX-F6.1-PROPOSED-Decision-Record-G1-Increment-3-v0.1.1.md`). This report approves nothing and changes no register or approval log.

Evidence tags used below: **[E]** executed and its result checked · **[S]** static inspection or reasoning, nothing executed against Kubernetes · **[A]** assumption or inference that could not be verified here.

---

## 0. Results at a glance

| Test | Result | Basis |
|---|---|---|
| Image builds (processor, api-gateway, synthetic-probe, db-migrate) | **BLOCKED** | [E] every base image pull refused: `registry-1.docker.io`, `gcr.io` → 403 |
| Helm dependency build / lint / template | **BLOCKED** | [E] no Helm binary; `get.helm.sh` 403; chart repositories 403 |
| kind cluster | **BLOCKED** | [E] `kind create cluster` fails pulling `kindest/node:v1.31.0` (403) |
| A. Work delivery over TLS | **BLOCKED** (not run in Kubernetes) | no cluster. A defect that would break it was found and reproduced with the real binaries (D1, §6) |
| B. TLS observation and reporting | **BLOCKED** (not run in Kubernetes) | no cluster |
| C. Revocation | **BLOCKED** (not run in Kubernetes) | no cluster; supplementary non-Kubernetes check passed (§3.C) |
| D. Credential rotation | **BLOCKED** (not run in Kubernetes) | no cluster; supplementary kubelet-style Secret-volume swap passed (§3.D) |
| E. NetworkPolicy installed and enforced | **BLOCKED** | no cluster; kind CNI enforcement **unknown**. Static evaluation found a policy defect (D2) |
| F. Migrations: fresh, E1, E2, ordering, data preservation | **PASS** (local PostgreSQL, supported script) · in-cluster Job path **BLOCKED** | [E] 25/25 checks. **Security finding** on repeated runs (F5, §6) |
| G. Cleanup | **PASS** | [E] nothing left running; no external or shared infrastructure touched |

**Verdict:** the results do **not** support moving to the next validation stage (staging). They support re-running *this* stage in an environment that has registry and Helm access, after the two Increment-3 chart defects in §6 are fixed and after a decision on the migration re-run behaviour (§7).

---

## 1. Environment and tool versions [E]

| Item | Value |
|---|---|
| Host | Ubuntu 24.04.4 LTS, kernel 6.18.44, x86_64, 2 vCPU, 7 GiB RAM, cgroup v1 |
| Docker | client 29.4.3 (055a478), daemon 29.4.3 (56be731), storage `overlayfs` (containerd snapshotter); daemon started for this run and stopped afterwards |
| containerd / runc | containerd.io v2.2.3 / runc 1.3.5 |
| kind | **v0.24.0** (go1.22.6), official GitHub release, SHA-256 `b89aada5…b600d` verified against the release's `.sha256sum` |
| kubectl | **not available** (not installed; `dl.k8s.io`, `cdn.dl.k8s.io`, `pkgs.k8s.io` → 403; no Ubuntu package) |
| Helm | **not available** (not installed; `get.helm.sh` → 403; no Ubuntu package; Helm's GitHub releases carry no binaries) |
| Go | 1.24.7 (module language version `go 1.22.0`) |
| PostgreSQL | 16.13 server (disposable data directory, 127.0.0.1:55432) and `psql` 16.13 |
| OpenSSL | 3.0.13 |
| kubeconform | v0.6.7, official GitHub release, checksum verified; Kubernetes 1.31.0 schemas from `raw.githubusercontent.com` (static validation only) |

**Egress policy (proxy-reported CONNECT refusals, 403):** `registry-1.docker.io`, `auth.docker.io`, `index.docker.io`, `mirror.gcr.io`, `gcr.io`, `ghcr.io`, `quay.io`, `registry.k8s.io`, `public.ecr.aws`, `us-docker.pkg.dev`, `get.helm.sh`, `dl.k8s.io`, `cdn.dl.k8s.io`, `pkgs.k8s.io`, `kind.sigs.k8s.io`, `download.docker.com`, `grafana.github.io`, `charts.bitnami.com`, `helm.neo4j.com`, `sum.golang.org`. Reachable: `github.com` release assets, `raw.githubusercontent.com`, `archive.ubuntu.com`, `pypi.org`, `registry.npmjs.org`. As the proxy documentation instructs, the refusals were **reported, not routed around**: no unofficial mirrors, re-packaged binaries or source rebuilds of blocked artifacts were used.

**Repository state [E]:** the working copy matched your `observex` folder file-for-file for every F6.1 path (listing compared by size and modification time). Your `Makefile`, `.github/workflows/ci.yml` and `cd.yml` are the **original** versions; the Increment-3 patch for them (`Claude outputs/f61-g1-inc3/PROTECTED-FILES.patch`) is **not applied** and was not used. This run invoked `docker build` directly rather than `make docker-build`.

---

## 2. Commands executed [E]

Sandbox paths are shown as they ran. `$REPO` is the repository copy.

```bash
# Environment
uname -a; cat /etc/os-release; nproc; free -g
docker --version; dockerd --version; containerd --version; runc --version; go version; psql --version; openssl version
command -v kind kubectl helm crictl nerdctl podman k3s minikube          # none found
curl -sS -o /dev/null -w '%{http_code}' -I --max-time 20 <URL>          # 30 URLs, §1
curl -sS "$HTTPS_PROXY/__agentproxy/status"                              # refusal reasons
apt-get update; apt-cache policy kubectl kubernetes-client helm kind     # no candidates

# kind (official release) and the cluster attempt
curl -sSL -o kind https://github.com/kubernetes-sigs/kind/releases/download/v0.24.0/kind-linux-amd64
curl -sSL -o kind.sha256sum https://github.com/kubernetes-sigs/kind/releases/download/v0.24.0/kind-linux-amd64.sha256sum
sha256sum -c; ./kind version
nohup dockerd --host=unix:///var/run/docker.sock &
./kind create cluster --name f61-validation --wait 120s

# Real Dockerfiles, repository root as context
for s in processor api-gateway synthetic-probe db-migrate; do
  docker build --progress=plain -f services/$s/Dockerfile -t observex/$s:f61-validation .
done

# Static checks (no Helm available — see §3.0)
<render harness> deployments/helm/observex image.tag=ci trivyScanner.enabled=false \
  f61.enabled=true f61.intake.tlsSecret=observex-f61-intake-tls \
  f61.probe.processorCASecret.name=observex-f61-intake-ca \
  'f61.vantages[0].name=kind-a' 'f61.vantages[0].credentialSecret=observex-probe-kind-a' \
  networkPolicy.enabled=true f61.migrations.job.enabled=true > render-f61.yaml
kubeconform -strict -summary -kubernetes-version 1.31.0 render-default.yaml render-f61.yaml
go vet -stdversion ./internal/{evaluate,result,intake,probe}/f61/ ./internal/db/dbtest/ ./internal/probetoken/ \
  ./internal/observe/tlscert/ ./services/{processor,api-gateway,synthetic-probe}/

# F — migrations (harness in Appendix A) and the repository's migration tests
./mig-validate.sh
OBSERVEX_TEST_POSTGRES_DSN=postgres://postgres@127.0.0.1:55432/postgres?sslmode=disable \
  go test -count=1 -v ./internal/db/dbtest/

# D1 reproduction and the supplementary C/D checks (real binaries, §3.A, §3.C, §3.D)
CGO_ENABLED=0 go build -trimpath -o bin/processor ./services/processor/
CGO_ENABLED=0 go build -trimpath -o bin/synthetic-probe ./services/synthetic-probe/
# plus openssl CA/leaf generation, an /etc/hosts entry (removed afterwards), processor and probe runs, curl --noproxy checks

# G — cleanup
./kind delete cluster --name f61-validation; ./kind get clusters
docker ps -a; docker images -q; docker builder prune -af; pkill -x dockerd
cp hosts.orig /etc/hosts; DROP DATABASE … (every f61val_* database)
```

---

## 3. Results per test

### 3.0 Prerequisites

**Image builds — BLOCKED [E].** All four Increment-3 deployment images fail at the first `FROM`:

```
processor / synthetic-probe:  failed to resolve source metadata for docker.io/library/golang:1.22-alpine:
  Head "https://registry-1.docker.io/v2/library/golang/manifests/1.22-alpine": Forbidden
api-gateway:                  … gcr.io/distroless/static-debian12:nonroot: Head "https://gcr.io/v2/…/manifests/nonroot": Forbidden
db-migrate:                   … docker.io/library/postgres:16-alpine: Head "https://registry-1.docker.io/…/16-alpine": Forbidden
```

Nothing after the base-image step ran. That includes the `COPY`s, the Go builds inside the images, the non-root users and the entry points. As a partial substitute [E], `go vet -stdversion` passed for every F6.1 package and service (exit 0). It reports standard-library use newer than the module's Go 1.22, which is the version the Dockerfiles build with. This does **not** show that the images build.

**Helm — BLOCKED [E].** No Helm binary could be obtained from an allowed official source, and the chart dependencies (Loki, Tempo, Neo4j, and Bitnami PostgreSQL/Redis/ClickHouse) are all on refused hosts. `helm dependency build`, `helm lint` and `helm template` were **not run**.
*Static substitute [S], not Helm:* the chart was rendered with the local text/template harness from Increment 3, which approximates Helm's functions and **does not render sub-charts**. `kubeconform -strict` against Kubernetes 1.31.0 schemas reported **26/26** resources valid (defaults) and **39/39** (F6.1 enabled, one vantage, NetworkPolicies, migration Job). The 24 design checks on that render all passed; the list is in Appendix C. A render by this harness is not evidence that `helm template` succeeds.

**kind cluster — BLOCKED [E].**
```
ERROR: failed to create cluster: failed to pull image "kindest/node:v1.31.0@sha256:53df588e…": …
Head "https://registry-1.docker.io/v2/kindest/node/manifests/sha256:53df588e…": Forbidden
```
No node container was created and no kubeconfig was written.

### 3.A Work delivery — BLOCKED (not run in Kubernetes)

**Defect found [E], relevant to A (D1):** with the chart's defaults the probe **cannot** reach the processor over TLS. The chart points probes at `https://<release>-processor-probe-intake:8443`, a short Service name. `values.yaml` tells operators to issue the listener certificate for `<release>-processor-probe-intake.<namespace>.svc`. Go's TLS client checks the name it dialled, so the certificate does not match. I reproduced this with the real `processor` and `synthetic-probe` binaries, a private CA, and a hosts entry standing in for cluster DNS:

| Listener certificate SANs | Probe URL | Result |
|---|---|---|
| `…-probe-intake.observex.svc` (as `values.yaml` instructs) | short name (as the chart renders) | **fails**: `tls: failed to verify certificate: x509: certificate is valid for observex-processor-probe-intake.observex.svc …` |
| `…-probe-intake.observex.svc` | `.svc` name | work refreshed (TLS verified, credential accepted) |
| short name + `.svc` | short name | work refreshed |

This is an Increment-3 regression. A proposed fix is in Appendix B; it is **not applied** (§5).

### 3.B TLS observation and reporting — BLOCKED (not run in Kubernetes)

Nothing was run in Kubernetes. The process-level runs from Increment 3 and the supplementary run below are **not** Kubernetes evidence.

### 3.C Revocation — BLOCKED (not run in Kubernetes)

*Supplementary, outside Kubernetes [E]:* real processor and probe binaries, TLS listener, disposable database, one SSL check assigned to zone `kind-a`, and a TLS target on loopback. The probe had `OBSERVEX_PROBE_ALLOW_PRIVATE_TARGETS=true`, the explicit internal-vantage setting a loopback or in-cluster target needs. The vantage was revoked with the same SQL statement `resultf61.Store.Revoke` executes; the gateway endpoint itself was not used in this run. Outcome:

```
revocation bound 2026-09-26 11:53:51+00
11:53:59 probe report failed status=401 → "dropping all assignments" (credential rejected)
11:54:00 work refresh failed status=401 ; /readyz 503
old credential: GET /v1/synthetic/probe/work → 401 ; POST …/tls-observations → 401 ; no credential → 401
```

### 3.D Credential rotation — BLOCKED (not run in Kubernetes)

*Supplementary, outside Kubernetes [E]:* the credential file was laid out the way kubelet lays out a Secret volume: `credential → ..data/credential`, with `..data → ..<timestamp>`. After the revocation bound had passed, a replacement credential was issued for the **same vantage** (`rotated true`) using the code the gateway calls. The `..data` link was then swapped atomically, as kubelet does:

```
11:54:12 replacement issued ; ..data → ..2026_09_26_v2 (old directory removed)
11:54:19, :29, :39, :49 probe reported   (resumed without restart) ; /readyz 200
old credential → 401 on /work ; replacement → 200 ; credential text found in logs: 0 files
```

This does not show kubelet's actual Secret propagation delay or behaviour.

### 3.E NetworkPolicy — BLOCKED

Nothing was installed, so enforcement could not be tested. **Whether kind's default CNI (kindnet in v0.24.0) enforces NetworkPolicy is unknown here [A]**. The release notes could not be read (the GitHub API for that repository is not enabled in this session). The next run must prove enforcement with a deny test before any E result counts. If kindnet does not enforce, a policy-enforcing CNI such as Calico is needed.

Static evaluation of the rendered policies [S]:
- The probe pod is selected by `default-deny` (egress DNS only) and `f61-probe`: no ingress; egress to the processor on 8443 and to public addresses on 443, excluding 10/8, 172.16/12, 192.168/16, 169.254/16, 100.64/10, 127/8 and 0/8. PostgreSQL is not reachable from probes.
- **Defect D2 (Increment-3 regression):** the migration Job pod (`component: db-migrate`) is selected only by `default-deny`. Its union of allowed egress is **port 53 only**. With `networkPolicy.enabled=true` and the Job enabled, the Job **cannot reach PostgreSQL**; it will time out and fail. Evaluation output: `union of allowed egress ports for the Job pod: [53] -> PostgreSQL 5432 allowed: False`. A proposed fix is in Appendix B; it is **not applied**.

### 3.F Database migrations — PASS (local PostgreSQL 16) · in-cluster Job BLOCKED

Harness: Appendix A. Mechanism: `scripts/db-migrate.sh`, run directly, because the `db-migrate` image could not be built. Every database was disposable (`f61val_*`) and dropped afterwards. Result **25 passed, 0 failed** [E]:

| Case | Checks (all PASS) |
|---|---|
| F1 fresh | exit 0; `schema_migrations` in order `001…007, 009`; **no 008 recorded**; four F6.1 tables; schema fingerprint `15bf9240…` |
| F2 E1 (docker init stopped at 001:217 `syntax error at or near "window"`; 6 tables) + data in 5 tables | upgrade exit 0; order as fresh; **schema identical to fresh**; **0 rows lost or modified** (14 rows, original columns except `updated_at`); re-run changes nothing |
| F3 E2 (old `psql -f … \|\| true` loop; 17 ERROR lines; 27 tables; `slos` and `network_flows` missing) + data in 9 tables | upgrade exit 0; order; schema identical to fresh; **0 rows lost or modified** (20 rows); F6.1 result row accepted for a check that existed **before** the upgrade; mismatched-org row refused by the composite FK; re-run changes nothing |
| F4 ordering and fail-fast | a broken `010` stops the run (exit 3); the failing file is rolled back whole; `011` not applied; earlier files recorded in order |

The repository's own migration tests also pass [E]: `TestFreshInitialisationSucceeds`, `TestOriginalStatementsCannotSucceed`, `TestUpgradeFromStoppedInitialisation`, `TestUpgradeFromTolerantApplication`, `TestMigrationsAreIdempotent`.

Expected data effects of an upgrade [E], from the existing migration 002 and not from Increment 3:
- `updated_at` is bumped on every pre-existing `teams`, `users` and `dashboards` row. 002's backfill of `org_id` fires the `set_updated_at` triggers.
- Users are backfilled into `org-default`.
- `team_members` rows are derived from `users.team_id`; in E2 this added 2 rows to an existing, empty table.

**F5 — security finding [E]: re-running the migration set is not side-effect free.** MIG-1's interim script re-applies every file on every run, by design, so partial E1/E2 states can be completed. On a live database, a second run (for example the Helm Job on every `helm upgrade`, or `make db-migrate`) did the following:

| Before the re-run | After the re-run |
|---|---|
| default admin `admin@observex.io` deleted | **re-created with role `admin`, org `org-default`, and 001's fixed bcrypt hash** |
| a user with no organization (the app's `UserStore.Create` inserts users without `org_id`) | **assigned to `org-default`** |
| a user's team membership removed (`team_members`), `users.team_id` still set | **membership re-created with role `admin`** (copied from the user's org role) |
| template dashboards deleted | 6 re-created |

The statements responsible are pre-existing (001's admin and dashboard seeds; 002's backfills). Re-running them routinely comes from the PROPOSED MIG-1 re-apply-all mechanism. The old `make db-migrate` loop had the same property, but it was not wired to deployments. The seeded hash did not match 11 common passwords; no further cracking was attempted. The consequence is a known-hash administrator reappearing after an operator removed it, plus tenant and privilege changes made without an administrator acting. This needs a decision (§7).

### 3.G Cleanup — PASS [E]

`kind delete cluster` (nothing existed), `kind get clusters` → none. Docker had 0 containers and 0 images; build cache pruned; `dockerd` stopped. The hosts entry was removed (original restored). Remaining `f61val_*`/`f61test_*` databases: 0. No test processes remain. No kubeconfig was written; an empty `~/.kube` directory created by `kind delete` was removed. No cloud credentials, remote clusters or shared services were used.

---

## 4. Evidence files

Raw evidence was kept in the sandbox that ran the validation (`/tmp/claude-0/val/`) and is **not** in the repository: `env-probe.txt`, `net-probe.txt`, `proxy-denials.txt`, `apt-probe.txt`, `kind-create.txt`, `build-{processor,api-gateway,synthetic-probe,db-migrate}.txt`, `render-default.yaml`, `render-f61.yaml`, `static-checks.txt`, `d2-static.txt`, `vet-stdversion.txt`, `mig-validate.out` and `mig-*.log`, `go-dbtest.txt`, `d1/` (processor and probe logs; certificates are throwaway), `cleanup.txt`. The relevant lines are quoted above. The only secrets involved were generated test keys and credentials; none appear in this report or in any log (checked by search).

---

## 5. Changes made

**No repository code, chart or configuration was changed.** The fixes for D1 and D2 are ready (Appendix B) but not applied. This task allowed only changes required to *run* the tests, and no test could run in Kubernetes, so applying them could not have been verified here. The only repository addition is this report.

Tools used outside the repository, all removed or left only in the sandbox:
- the local render harness (Increment 3);
- a scratch credential issuer calling `probetoken.Key.Issue`, the code the gateway's issuance route calls;
- a scratch bcrypt check;
- the migration harness (Appendix A).

`go.mod` and `go.sum` were verified unchanged after each scratch build.

---

## 6. Defects and risks

**Increment-3 regressions (fix before the next run):**

| ID | Defect | Evidence | Impact |
|---|---|---|---|
| D1 | Probe URL uses the short Service name; the documented certificate SAN is the `.svc` name | [E] reproduced with real binaries | Test A fails on a correctly documented install: no work, no reports |
| D2 | Migration Job has no egress to PostgreSQL under `networkPolicy.enabled` | [S] policy evaluation | Job fails whenever policies are on and enforced |
| D3 | The `…report could not be delivered` transport error text is also used for work requests | [E] probe log | Misleading log only |

**Pre-existing defects (not Increment 3):**

| ID | Defect | Evidence |
|---|---|---|
| P1 | `trivy-scanner.yaml` calls `observex.image` without `.Values`, so the chart fails while `trivyScanner.enabled` (default) | [S] harness render error |
| P2 | db-monitor reads `.Values.image.tag`, which exists only when set (`--set image.tag=…`) | [S] |
| P3 | The chart Secret's `postgres-password` is not wired to the Bitnami sub-chart (`postgresql.auth.password: ""`, no `existingSecret`). The sub-chart then generates its own password, and the gateway (and F6.1's composed DSN) would fail to authenticate | [A] needs `helm dependency build` to confirm |
| P4 | The chart applies no migrations (`initdb.scriptsConfigMap: ""`); a fresh Helm install has an empty schema unless someone runs them | [S] |
| P5 | 001 seeds an admin with a fixed password hash; the documented password `observex` does not match it (found during the Increment-3 demo) | [E] |
| P6 | CI uses `working-directory: observex`, which does not match this repository's layout | [S] |

**Remaining security and operational risks:**
- **F5** (re-run side effects, §3.F): the most serious finding in this report.
- kind CNI NetworkPolicy enforcement is unknown. Probe pods have `ingress: []`; whether kubelet health checks reach them depends on the network plugin.
- The F6.1 NetworkPolicies allow PostgreSQL only as in-cluster pods (`app.kubernetes.io/name: postgresql`). An external database needs an `ipBlock` rule; this is the same for the existing gateway policy.
- **Install ordering:**
  - A probe Deployment needs its credential Secret before it starts, and the credential can only be issued by the running gateway. The first install therefore has to be two-phase: no vantages, then issue, then add the vantage.
  - With `--wait` and the migration Job enabled, the Job is a post-install hook and runs only after readiness. A probe added to a fresh database stays unready until the schema exists.
- The Dockerfiles, image users and read-only root filesystems are untested.

---

## 7. Can we proceed to the next stage?

**No.** None of the Kubernetes behaviour this stage exists to prove has been observed:
- images;
- Helm output;
- work over TLS in a cluster;
- results;
- revocation;
- rotation through a real Secret;
- NetworkPolicy enforcement;
- the in-cluster migration Job.

The migration mechanism itself worked. One confirmed defect (D1) would make test A fail as documented, and a second (D2) would make the migration Job fail under NetworkPolicy.

Required before re-running this stage:
1. **Environment:** a disposable machine where `registry-1.docker.io` (the `kindest/node`, `golang`, `postgres` images), `gcr.io` (distroless), `get.helm.sh` and the chart repositories are reachable, or a pre-seeded local registry and chart cache. This is an egress or tooling change only you or your organisation can make.
2. **Fix D1 and D2** (Appendix B), with a regression check each.
3. **Decision (governance), before the migration Job is enabled anywhere persistent:** should the interim migration mechanism keep re-applying every file on every run (MIG-1 as proposed), or apply only versions not yet recorded in `schema_migrations`, after the one-time E1/E2 completion? This is an amendment to the PROPOSED MIG-1 and touches the open D-03. It is not decided here. Until it is decided, the re-run effects in F5 apply to every run.

---

## Appendix A — migration harness (as executed)

Verbatim. Sandbox paths are specific to this run; it creates only `f61val_*` databases and drops them at the end.

```bash
#!/bin/bash
# F6.1 OPS-1 validation, test F: migrations with the supported mechanism
# (scripts/db-migrate.sh) against disposable PostgreSQL databases.
# Reproduces the two existing-installation states, seeds representative data,
# upgrades, and checks ordering, schema equality with a fresh install, data
# preservation, F6.1 usability over pre-existing rows, idempotency and
# fail-fast. Every database it creates is named f61val_* and dropped at the end.
set -u
REPO=/home/claude/work/observex
MIG=$REPO/internal/db/migrations
ORIG=$REPO/internal/db/dbtest/testdata/pre-mig1
ADMIN="postgres://postgres@127.0.0.1:55432/postgres?sslmode=disable"
export PGOPTIONS="-c client_min_messages=warning"
dsn() { echo "postgres://postgres@127.0.0.1:55432/$1?sslmode=disable"; }
q() { psql "$(dsn "$1")" -qtAX -c "$2"; }
newdb() { psql "$ADMIN" -qc "DROP DATABASE IF EXISTS $1 WITH (FORCE)" -c "CREATE DATABASE $1" >/dev/null; }
migrate() { POSTGRES_DSN="$(dsn "$1")" DB_MIGRATE_WAIT_SECONDS=5 sh "$REPO/scripts/db-migrate.sh" "${2:-$MIG}"; }
PASS=0; FAIL=0
ok() { if [ "$1" = 0 ]; then PASS=$((PASS+1)); echo "  PASS $2"; else FAIL=$((FAIL+1)); echo "  FAIL $2"; fi; }

FP_SQL="SELECT md5(string_agg(x, E'\n' ORDER BY x)) FROM (
  SELECT 'col ' || table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || coalesce(column_default, '') AS x
    FROM information_schema.columns WHERE table_schema = 'public'
  UNION ALL SELECT 'con ' || c.relname || ' ' || k.conname || ' ' || pg_get_constraintdef(k.oid)
    FROM pg_constraint k JOIN pg_class c ON c.oid = k.conrelid JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public'
  UNION ALL SELECT 'idx ' || indexdef FROM pg_indexes WHERE schemaname = 'public'
  UNION ALL SELECT 'typ ' || t.typname || ' ' || coalesce((SELECT string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder) FROM pg_enum e WHERE e.enumtypid = t.oid), '')
    FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e'
  UNION ALL SELECT 'fn ' || p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public'
  UNION ALL SELECT 'trg ' || tgname FROM pg_trigger WHERE NOT tgisinternal) s"

# Snapshot of every public table's rows over the columns it has NOW, so the
# comparison after an upgrade ignores columns a later migration adds.
snapshot() { # db outfile
  : > "$2"; : > "$2.cols"
  for t in $(q "$1" "SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename <> 'schema_migrations' ORDER BY 1"); do
    cols=$(q "$1" "SELECT string_agg(quote_ident(column_name), ',' ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND table_name='$t' AND column_name <> 'updated_at'")
    echo "$t|$cols" >> "$2.cols"
    q "$1" "SELECT md5(r::text) FROM (SELECT $cols FROM $t) r ORDER BY 1" > "$2.rows.$t"
    echo "$t $(q "$1" "SELECT count(*) || ' ' || coalesce(md5(string_agg(r::text, '|' ORDER BY r::text)), '-') FROM (SELECT $cols FROM $t) r")" >> "$2"
  done
}
resnapshot() { # db colsfile outfile
  : > "$3"
  while IFS='|' read -r t cols; do
    echo "$t $(q "$1" "SELECT count(*) || ' ' || coalesce(md5(string_agg(r::text, '|' ORDER BY r::text)), '-') FROM (SELECT $cols FROM $t) r" 2>&1)" >> "$3"
    q "$1" "SELECT md5(r::text) FROM (SELECT $cols FROM $t) r ORDER BY 1" > "$3.rows.$t"
  done < "$2"
}
order_of() { q "$1" "SELECT string_agg(version, ',' ORDER BY applied_at, version) FROM schema_migrations"; }
EXPECTED="001_initial,002_multitenancy,003_comments_runbooks,004_deployments,005_integrations_postmortems,006_sso_clusters,007_feature_state,009_f61_tls_certificates"

seed_e1() { # tables that exist after 001 stopped at slos
  q "$1" "INSERT INTO teams (id, name, namespaces) VALUES ('t-pay','payments','{payments}'), ('t-ops','ops','{}');
          INSERT INTO users (id, email, name, password_hash, role, team_id) VALUES
            ('u-1','alice@example.test','Alice','x','editor','t-pay'), ('u-2','bob@example.test','Bob','x','viewer','t-ops');
          INSERT INTO sessions (id, user_id, token, expires_at) VALUES ('s-1','u-1','tok-1', now() + interval '1 day');
          INSERT INTO api_keys (id, user_id, name, key_hash, key_prefix) VALUES ('k-1','u-2','ci','hash-1','oxk_abcd');
          INSERT INTO dashboards (id, name, owner_id, team_id, widgets_json) VALUES ('d-1','Latency','u-1','t-pay','[{\"type\":\"graph\"}]');" >/dev/null
}
seed_e2() { # E1 data plus tables created by 002-007 in a tolerant run
  seed_e1 "$1"
  # In E2, 002's DO block that adds org_id to users/teams/... failed as a whole
  # (it also names slos), so those tables have no org_id: seed what exists.
  q "$1" "INSERT INTO orgs (id, name, owner_id) VALUES ('org-a','acme','u-1')" >/dev/null
  q "$1" "INSERT INTO synthetic_checks (id, org_id, name, type, target, namespace, locations) VALUES
            ('chk-ssl','org-a','api tls','ssl','api.example.test:443','payments','{edge-eu}'),
            ('chk-http','org-a','home','http','https://www.example.test','default','{local}')" >/dev/null
  q "$1" "INSERT INTO feature_state (org_id, kind, id, payload) VALUES ('org-a','flag','beta','{\"on\":true}')" >/dev/null
  q "$1" "INSERT INTO deployments (id, service_name, version) VALUES ('dep-1','checkout','v1.2.3')" >/dev/null
  echo "  E2 seed: users.org_id present=$(q "$1" "SELECT count(*)>0 FROM information_schema.columns WHERE table_name='users' AND column_name='org_id'"), orgs=$(q "$1" 'SELECT count(*) FROM orgs'), synthetic_checks=$(q "$1" 'SELECT count(*) FROM synthetic_checks')"
}

echo "== F1 fresh database"
newdb f61val_fresh
migrate f61val_fresh > /tmp/claude-0/val/mig-F1.log 2>&1; ok $? "db-migrate.sh exits 0 on an empty database"
ok $([ "$(order_of f61val_fresh)" = "$EXPECTED" ]; echo $?) "schema_migrations order: $(order_of f61val_fresh)"
ok $([ "$(q f61val_fresh "SELECT count(*) FROM schema_migrations WHERE version LIKE '008%'")" = 0 ]; echo $?) "no 008 recorded (reserved for A7/F-006)"
ok $([ "$(q f61val_fresh "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename LIKE 'f61_%'")" = 4 ]; echo $?) "four F6.1 tables present"
FRESH_FP=$(q f61val_fresh "$FP_SQL"); echo "  fresh schema fingerprint $FRESH_FP"

for state in E1 E2; do
  db=f61val_$(echo $state | tr A-Z a-z)
  echo "== F2/F3 upgrade from $state with representative data"
  newdb $db; : > /tmp/claude-0/val/mig-$state-legacy.log
  if [ $state = E1 ]; then
    # docker-entrypoint-initdb.d semantics: psql -v ON_ERROR_STOP=1 per file; the first file stops.
    psql "$(dsn $db)" -qX -v ON_ERROR_STOP=1 -f $ORIG/001_initial.sql > /tmp/claude-0/val/mig-$state-legacy.log 2>&1
    ok $([ $? -ne 0 ]; echo $?) "original 001 stops with an error (as docker init did): $(grep -m1 ERROR /tmp/claude-0/val/mig-$state-legacy.log | sed 's|.*/||' | cut -c1-110)"
    seed_e1 $db
  else
    # old 'make db-migrate': psql -f FILE || true for every file.
    for f in $ORIG/001_initial.sql $MIG/002_multitenancy.sql $MIG/003_comments_runbooks.sql $MIG/004_deployments.sql $ORIG/005_integrations_postmortems.sql $MIG/006_sso_clusters.sql $MIG/007_feature_state.sql; do
      psql "$(dsn $db)" -qX -f $f >> /tmp/claude-0/val/mig-$state-legacy.log 2>&1 || true
    done
    ok $([ "$(grep -c ERROR /tmp/claude-0/val/mig-$state-legacy.log)" -gt 0 ]; echo $?) "tolerant legacy run left errors ($(grep -c ERROR /tmp/claude-0/val/mig-$state-legacy.log) ERROR lines, e.g. $(grep -m1 ERROR /tmp/claude-0/val/mig-$state-legacy.log | sed 's|.*/||' | cut -c1-110))"
    seed_e2 $db
  fi
  echo "  legacy state: $(q $db "SELECT count(*) FROM pg_tables WHERE schemaname='public'") tables, slos present=$(q $db "SELECT to_regclass('public.slos') IS NOT NULL"), network_flows present=$(q $db "SELECT to_regclass('public.network_flows') IS NOT NULL"), schema_migrations rows=$(q $db 'SELECT count(*) FROM schema_migrations')"
  snapshot $db /tmp/claude-0/val/snap-$state-before
  migrate $db > /tmp/claude-0/val/mig-$state.log 2>&1; ok $? "db-migrate.sh upgrades $state (exit 0)"
  ok $([ "$(order_of $db)" = "$EXPECTED" ]; echo $?) "schema_migrations order after upgrade: $(order_of $db)"
  ok $([ "$(q $db "$FP_SQL")" = "$FRESH_FP" ]; echo $?) "schema identical to a fresh install"
  resnapshot $db /tmp/claude-0/val/snap-$state-before.cols /tmp/claude-0/val/snap-$state-after
  echo "  rows whose updated_at changed during the upgrade (002 backfill fires set_updated_at): $(for t in teams users dashboards; do printf '%s=%s ' $t "$(q $db "SELECT count(*) FROM $t WHERE updated_at > now() - interval '10 minutes' AND created_at < now() - interval '0 seconds'")"; done)"
  echo "  rows added by 002 backfills: team_members=$(q $db 'SELECT count(*) FROM team_members'), users now in org-default=$(q $db "SELECT count(*) FROM users WHERE org_id='org-default'")"
  lost=0; added=""
  while IFS='|' read -r t cols; do
    b=/tmp/claude-0/val/snap-$state-before.rows.$t; a=/tmp/claude-0/val/snap-$state-after.rows.$t
    n=$(comm -23 "$b" "$a" | wc -l); lost=$((lost+n))
    m=$(comm -13 "$b" "$a" | wc -l); [ "$m" -gt 0 ] && added="$added $t:+$m"
  done < /tmp/claude-0/val/snap-$state-before.cols
  ok $([ $lost -eq 0 ]; echo $?) "every pre-existing row still present and unchanged ($(wc -l < /tmp/claude-0/val/snap-$state-before) tables over their original columns except updated_at; $(awk '{s+=$2} END {print s}' /tmp/claude-0/val/snap-$state-before) rows; lost or modified: $lost; rows added by migrations to pre-existing tables:${added:- none})"
  if [ $state = E2 ]; then
    # 009's composite key must work over a check row that existed before the upgrade.
    q $db "INSERT INTO f61_tls_results (check_id, org_id, namespace, endpoint, status, horizon_seconds, span_seconds, interval_sec, vantages, status_changed_at, evaluated_at, last_observed_at)
           VALUES ('chk-ssl','org-a','payments','api.example.test:443','ok',2592000,180,60,'[]',now(),now(),now())" >/dev/null 2>&1
    ok $? "F6.1 result row accepted for a check that existed before the upgrade"
    q $db "INSERT INTO f61_tls_results (check_id, org_id, namespace, endpoint, status, horizon_seconds, span_seconds, interval_sec, vantages, status_changed_at, evaluated_at, last_observed_at)
           VALUES ('chk-http','org-other','default','x','ok',1,1,60,'[]',now(),now(),now())" >/dev/null 2>&1
    ok $([ $? -ne 0 ]; echo $?) "F6.1 result row with a mismatched org is refused by the composite FK"
  fi
  before=$(q $db "SELECT count(*) FROM schema_migrations"); snap2=/tmp/claude-0/val/snap-$state-rerun
  migrate $db > /tmp/claude-0/val/mig-$state-rerun.log 2>&1; ok $? "re-running the full set on the upgraded $state database exits 0"
  resnapshot $db /tmp/claude-0/val/snap-$state-before.cols $snap2
  ok $([ "$(q $db "$FP_SQL")" = "$FRESH_FP" ] && [ "$(q $db 'SELECT count(*) FROM schema_migrations')" = "$before" ] && diff -q /tmp/claude-0/val/snap-$state-after $snap2 >/dev/null; echo $?) "re-run changes nothing (schema, bookkeeping, data)"
done

echo "== F5 side effects of re-applying every file on a later run (live database)"
newdb f61val_rerun
migrate f61val_rerun > /tmp/claude-0/val/mig-F5-1.log 2>&1; ok $? "initial migration"
q f61val_rerun "INSERT INTO orgs (id, name, owner_id) VALUES ('org-b','tenant-b','u-b1');
  INSERT INTO teams (id, name, org_id) VALUES ('t-b','team-b','org-b');
  INSERT INTO users (id, email, name, password_hash, role, team_id, org_id) VALUES ('u-b1','b1@example.test','B1','x','admin','t-b','org-b');
  INSERT INTO users (id, email, name, password_hash, role) VALUES ('u-noorg','pending@example.test','Pending','x','viewer');" >/dev/null
q f61val_rerun "DELETE FROM team_members WHERE user_id='u-b1'" >/dev/null
q f61val_rerun "DELETE FROM dashboards WHERE is_template" >/dev/null
q f61val_rerun "UPDATE orgs SET owner_id='u-b1' WHERE owner_id='usr-admin-default'" >/dev/null
q f61val_rerun "DELETE FROM users WHERE id='usr-admin-default'" >/dev/null
echo "  before re-run: default admin=$(q f61val_rerun "SELECT count(*) FROM users WHERE email='admin@observex.io'") u-noorg.org_id=$(q f61val_rerun "SELECT coalesce(org_id,'NULL') FROM users WHERE id='u-noorg'") u-b1 memberships=$(q f61val_rerun "SELECT count(*) FROM team_members WHERE user_id='u-b1'") template dashboards=$(q f61val_rerun 'SELECT count(*) FROM dashboards WHERE is_template')"
migrate f61val_rerun > /tmp/claude-0/val/mig-F5-2.log 2>&1; ok $? "re-run exits 0"
echo "  after re-run:  default admin=$(q f61val_rerun "SELECT count(*) || ' (role ' || coalesce(max(role),'-') || ', org ' || coalesce(max(org_id),'NULL') || ')' FROM users WHERE email='admin@observex.io'") u-noorg.org_id=$(q f61val_rerun "SELECT coalesce(org_id,'NULL') FROM users WHERE id='u-noorg'") u-b1 memberships=$(q f61val_rerun "SELECT count(*) || ' (role ' || coalesce(max(role),'-') || ')' FROM team_members WHERE user_id='u-b1'") template dashboards=$(q f61val_rerun 'SELECT count(*) FROM dashboards WHERE is_template')"
psql "$ADMIN" -qc "DROP DATABASE IF EXISTS f61val_rerun WITH (FORCE)"

echo "== F4 ordering and fail-fast"
tmp=$(mktemp -d); cp $MIG/*.sql $tmp/
printf 'CREATE TABLE f61val_marker (id int);\nSELECT this_is_not_sql;\n' > $tmp/010_broken.sql
printf 'CREATE TABLE f61val_after (id int);\n' > $tmp/011_after.sql
newdb f61val_failfast
migrate f61val_failfast $tmp > /tmp/claude-0/val/mig-F4.log 2>&1; rc=$?
ok $([ $rc -ne 0 ]; echo $?) "a failing migration stops the run (exit $rc)"
ok $([ "$(q f61val_failfast "SELECT to_regclass('public.f61val_marker') IS NULL AND to_regclass('public.f61val_after') IS NULL")" = t ]; echo $?) "the failing file is rolled back as a whole and later files are not applied"
ok $([ "$(order_of f61val_failfast)" = "$EXPECTED" ]; echo $?) "earlier migrations recorded in lexical order; the failing one is not recorded"
rm -rf $tmp

echo "== cleanup"
for db in f61val_fresh f61val_e1 f61val_e2 f61val_failfast; do psql "$ADMIN" -qc "DROP DATABASE IF EXISTS $db WITH (FORCE)"; done
echo "  remaining f61val databases: $(psql "$ADMIN" -qtAXc "SELECT count(*) FROM pg_database WHERE datname LIKE 'f61val%'")"
echo "RESULT: $PASS passed, $FAIL failed"
[ $FAIL -eq 0 ]
```

Output excerpt:

```
== F1 fresh database
  PASS db-migrate.sh exits 0 on an empty database
  PASS schema_migrations order: 001_initial,002_multitenancy,003_comments_runbooks,004_deployments,005_integrations_postmortems,006_sso_clusters,007_feature_state,009_f61_tls_certificates
  PASS no 008 recorded (reserved for A7/F-006)
  PASS four F6.1 tables present
== F2/F3 upgrade from E1 with representative data
  PASS original 001 stops with an error (as docker init did): 001_initial.sql:217: ERROR:  syntax error at or near "window"
  legacy state: 6 tables, slos present=f, network_flows present=f, schema_migrations rows=0
  PASS db-migrate.sh upgrades E1 (exit 0)
  PASS schema identical to a fresh install
  PASS every pre-existing row still present and unchanged (5 tables …; 14 rows; lost or modified: 0; rows added …: none)
  PASS re-run changes nothing (schema, bookkeeping, data)
== F2/F3 upgrade from E2 with representative data
  legacy state: 27 tables, slos present=f, network_flows present=f, schema_migrations rows=0
  PASS every pre-existing row still present and unchanged (26 tables …; 20 rows; lost or modified: 0; rows added …: team_members:+2)
  PASS F6.1 result row accepted for a check that existed before the upgrade
  PASS F6.1 result row with a mismatched org is refused by the composite FK
== F5 side effects of re-applying every file on a later run (live database)
  before re-run: default admin=0 u-noorg.org_id=NULL u-b1 memberships=0 template dashboards=0
  after re-run:  default admin=1 (role admin, org org-default) u-noorg.org_id=org-default u-b1 memberships=1 (role admin) template dashboards=6
== F4 ordering and fail-fast
  PASS a failing migration stops the run (exit 3)
  PASS the failing file is rolled back as a whole and later files are not applied
RESULT: 25 passed, 0 failed
```

## Appendix B — proposed fixes (NOT applied)

**D1** — `deployments/helm/observex/templates/_f61_helpers.tpl`: make the probe URL match the documented SAN.
```diff
 {{- else if .Values.f61.intake.insecurePlaintext -}}
-http://{{ include "observex.f61.intakeServiceName" . }}:{{ .Values.f61.intake.port }}
+http://{{ include "observex.f61.intakeServiceName" . }}.{{ .Release.Namespace }}.svc:{{ .Values.f61.intake.port }}
 {{- else -}}
-https://{{ include "observex.f61.intakeServiceName" . }}:{{ .Values.f61.intake.port }}
+https://{{ include "observex.f61.intakeServiceName" . }}.{{ .Release.Namespace }}.svc:{{ .Values.f61.intake.port }}
 {{- end -}}
```
Regression check: render and assert that the URL host equals the documented SAN, then repeat the §3.A matrix (row 2 must pass).

**D2** — `deployments/helm/observex/templates/f61.yaml`, inside the `networkPolicy.enabled` block:
```yaml
{{- if $f.migrations.job.enabled }}
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ $fullname }}-f61-db-migrate
  namespace: {{ .Release.Namespace | quote }}
  labels:
    {{- include "observex.labels" . | nindent 4 }}
    app.kubernetes.io/component: db-migrate
spec:
  podSelector:
    matchLabels:
      {{- include "observex.selectorLabels" . | nindent 6 }}
      app.kubernetes.io/component: db-migrate
  policyTypes: [Egress]
  egress:
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: postgresql
      ports:
        - port: 5432
          protocol: TCP
{{- end }}
```
Regression check: the §3.E policy evaluation must report 5432 allowed for the Job pod. In the cluster, the Job must complete with policies enforced.

## Appendix C — static design checks on the F6.1 render [S]

All 24 passed on the harness render, not a Helm render:
- one probe Deployment per vantage; `replicas: 1`; `Recreate`; `serve`;
- no service-account token; non-root; `RuntimeDefault` seccomp; read-only root; no privilege escalation; all capabilities dropped;
- HTTPS processor URL with a CA file; private targets and plaintext off;
- credential read from its own projected Secret file, never an env value;
- health on `/healthz` and `/readyz`;
- probe key Secret mounted only into api-gateway and processor;
- processor uses `OBSERVEX_F61_POSTGRES_DSN` and not `POSTGRES_DSN`, so the legacy scheduler stays off; the password variable is defined before the DSN that expands it; TLS files set; plaintext off;
- intake Service on 8443;
- probe NetworkPolicy: no ingress, public-only egress on 443, processor on 8443;
- processor F6.1 policy: 8443 in, 5432 out;
- no private-target policy selecting pods;
- migration Job present only when enabled, as a post-install/post-upgrade hook;
- no credential or key value rendered.


---

## 8. Addendum — remediation and handoff (2026-09-27)

> **Added after the original run.** Sections 0–7 and Appendices A–C above are the original results. They are **unchanged**, with one exception: the findings they call "not applied" (D1, D2) have since been fixed in the repository, as stated in §8.2. **Nothing in this addendum was executed in Kubernetes.** OPS-1 is **not complete**. Tests A–E remain **BLOCKED**.

Evidence tags as above: **[E]** executed and checked · **[S]** static only · **[A]** assumption.

### 8.1 Governance status [E: files inspected]

- **D-03 is PROPOSED** in `Claude outputs/03_Engineering_Decision_Register.md`. `Claude outputs/07_Approval_Log.md` records no approval of D-03, or of A7/S1-11. **MIG-1 is PROPOSED** in the Increment-3 decision record v0.1.1. The repository contains no evidence of an authorised decision on migration-runner semantics.
  - The runner (`scripts/db-migrate.sh`) was therefore **not changed**.
  - The re-run behaviour is written up as a decision proposal, **MIG-1a (PROPOSED, UNRESOLVED)**: `Claude outputs/ObserveX-F6.1-MIG-1a-Migration-Ledger-Decision-Proposal-v0.1.0.md`.
- **Unresolved tension, reported only:** Increment 3 edited migrations 001 and 005 in place under the PROPOSED MIG-1. `07` and `11_Remediation_Plan.md` place migration fixes (DB-1, DB-3) under A7, which is not approved.
- **D1 and D2** are corrections to the Increment-3 chart, which was itself built under proposed G-1 decisions (EGRESS-1 and OPS-1 remain PROPOSED).
  - The fixes make the chart do what its own documentation already required: the certificate SAN documented in `values.yaml`, and the migration Job's documented need to reach PostgreSQL. They approve nothing.
  - The migration Job stays **disabled by default** (`f61.migrations.job.enabled: false`). It is enabled only in `deployments/helm/chartcheck/testdata/f61-kind-values.yaml`, for a disposable test cluster.
- **Unchanged:**
  - Registers `03`, `06`, `07` and all approval logs: not written by this work.
  - `Makefile`, `.github/workflows/ci.yml` and `cd.yml` in the repository folder are the **original, unpatched** versions. They were read back and are byte-identical to the pre-image of `PROTECTED-FILES.patch` (SHA-256 `feb38acf…`, `a2e1f095…`, `c13f2509…`). The patch is **not applied**.
  - `go.mod` (`60986b8e…`) and `go.sum` (`748605ea…`).
- **Correction to earlier hand-offs:** the original report (§1) and the Increment-3 hand-off said that `ObserveX-F6.1-PROPOSED-Decision-Record-G1-Increment-3-v0.1.1.md` and `f61-g1-inc3/PROTECTED-FILES.patch` were in `Claude outputs/`. They were not on disk. They are now delivered there with their content unchanged. The patch remains **unapplied**.

### 8.2 Findings: status now

| ID | Finding | Status | Basis |
|---|---|---|---|
| D1 | Probe dialled the short Service name; the certificate SAN is the `.svc` name | **FIXED in chart**. Kubernetes run **NOT RUN** | [E] Go TLS tests with the real listener and real probe client; [E] real processor and probe binaries (below); [S] render checks |
| D2 | Migration Job had no egress to PostgreSQL under `networkPolicy.enabled` | **FIXED in chart**. Enforcement **NOT RUN** | [S] policy evaluation of the render only |
| D3 | Misleading `report could not be delivered` text on work requests | open, unchanged | outside this remediation |
| F5 | Every run of the runner re-executes seeds and backfills | **Characterised, NOT FIXED.** Decision MIG-1a required | [E] 5 repository tests against disposable PostgreSQL 16 (§8.4) |
| P3 | Chart Secret password is not wired to the PostgreSQL sub-chart | **Confirmed at source level, NOT FIXED.** Decision required. Runtime **NOT verified** | [S] Bitnami `postgresql` 15.5.4 sources (§8.5) |
| D4 (new) | A URL-unsafe database password leaks into processor logs and DNS queries | **NOT FIXED.** Proposal in §8.6 | [E] local processor run |
| P1, P2, P4, P5, P6 | Pre-existing chart and CI defects (§6) | unchanged | — |

**D1 with real binaries [E]:** the processor ran with the TLS listener on `127.0.0.1:18443`, a local PostgreSQL database and a throwaway CA, and a temporary `/etc/hosts` entry mapped both names to `127.0.0.1` (restored afterwards). The certificate SAN was `observex-processor-probe-intake.observex.svc`.
- **Old URL:** the probe logged `work refresh failed … x509: certificate is valid for observex-processor-probe-intake.observex.svc, not observex-processor-probe-intake`.
- **New URL:** the probe logged `work refreshed` (0 assignments: no checks were configured).

### 8.3 Changes

| File | Change | Why |
|---|---|---|
| `deployments/helm/observex/templates/_f61_helpers.tpl` | New helper `observex.f61.intakeTLSName` = `<intake Service>.<release namespace>.svc`. The default `processorUrl` (https, and the development-only http mode) now uses it. The Service name is truncated to 63 characters as a DNS label. | **D1.** No cluster domain is named or guessed: pods resolve `<svc>.<ns>.svc` through the search path Kubernetes configures. An explicit `f61.probe.processorUrl` is still used as given. |
| `deployments/helm/observex/templates/f61.yaml` | The intake Service always carries the annotation `observex.io/tls-server-name: <that name>`; user annotations follow it. New NetworkPolicy `<fullname>-f61-db-migrate`, rendered only when both `networkPolicy.enabled` and `f61.migrations.job.enabled` are true: selects only the Job pods; egress to pods labelled `app.kubernetes.io/name: postgresql` on TCP 5432, nothing else. | **D1**: the verified name is visible to whoever issues the certificate. **D2**: DNS was already allowed by the default-deny policy, and PostgreSQL ingress by the storage-access policy. |
| `deployments/helm/observex/values.yaml` | Comments only. The SAN must be exactly `<fullname>-processor-probe-intake.<namespace>.svc`, as shown in the annotation. External probes add their `processorUrl` host. | D1 documentation |
| `deployments/helm/chartcheck/render_test.go` (new) | Stand-in renderer for static checks: Go `text/template` with the subset of Helm functions this chart uses, no sub-charts. When `OBSERVEX_HELM_TEMPLATE_OUTPUT` names a real `helm template` output, the checks read that instead. | Static evidence where no Helm binary exists |
| `deployments/helm/chartcheck/chartcheck_test.go` (new) | D1, D2 and database-credential-source checks, plus a simplified NetworkPolicy evaluator (pod selection, union of rules, `podSelector`/`ipBlock` peers, ports) | Regression tests |
| `deployments/helm/chartcheck/testdata/f61-kind-values.yaml`, `f61-kind-runtime-values.yaml` (new) | Values for the disposable kind run only | Shared by the tests and the runbook |
| `services/processor/f61_tls_identity_test.go` (new) | Real TLS listener (`listenF61`) and real probe client (`probef61.Client.Work`); only name resolution is redirected | D1 regression |
| `internal/db/dbtest/rerun_characterization_test.go` (new) | Characterisation of the **current** re-run behaviour | Evidence for MIG-1a. These tests describe current behaviour; passing them does not accept it. |
| `internal/db/dbtest/migrations_test.go` | Comment only: a re-run leaves the **schema** unchanged but is not a data no-op | Accuracy |
| `docs/validation/f61-ops-1-external-runbook.md` (new) | Handoff runbook for the authorised disposable machine, **not executed** | Workstream E |
| this section | Addendum | — |

**Not changed:** the runner script, all migrations, the application code, the Makefile, the CI/CD workflows, go.mod/go.sum and every governance record. No TLS verification, authentication, revocation, tenant-isolation or NetworkPolicy restriction was relaxed.

### 8.4 Tests

The environment for every row is the Claude sandbox: Go 1.24.7 with `GOPROXY=off`, and a disposable PostgreSQL 16.13 at `127.0.0.1:55432`, reached through `OBSERVEX_TEST_POSTGRES_DSN`. There was no Kubernetes, Helm or image registry.

| Test | Level | Command | Result |
|---|---|---|---|
| Full regression of the F6.1 and affected packages (`internal/db/dbtest`, `internal/result/f61`, `services/processor`, `services/api-gateway`, `services/synthetic-probe`, `internal/probe/f61`, `internal/intake/f61`, `deployments/helm/chartcheck`) | unit + integration (real PostgreSQL) | `go test -race -count=1 -v <packages>` | **PASS**: 277 test and subtest results passed, 0 failed, **0 skipped** |
| Frozen packages (`compose`, `correlate`, `evaluate`, `wire` f61, `probetoken`, `servicetoken`, `adapt`, `detect`, `observe`, `knowledge`, `middleware`) | unit | `go test -race -count=1 …` | **PASS** |
| `go vet` on the changed packages; `gofmt -l` on the new files | static | — | **PASS** (clean) |
| `TestF61ProbeVerifiesIntakeServiceName` (5 subtests): correct SAN succeeds; short-name-only SAN fails (`certificate is valid for observex-processor-probe-intake, not …`); other host fails; untrusted CA fails (`unknown authority`); expired certificate fails (`expired`) | integration (real TLS, real client) | `go test -race ./services/processor/ -run TestF61` | **PASS** |
| `TestF61IntakeNameVerifiesAgainstSAN` | unit | same | **PASS** |
| `TestD1ProbeDialsTheCertificateName`, `TestD1OtherModes` (plaintext mode, `processorUrl` override, 50-character `fullnameOverride`) | **STATIC ONLY** (stand-in renderer) | `go test ./deployments/helm/chartcheck/` | **PASS**. **FAIL** against the pre-fix chart, as intended |
| `TestD2MigrationJobReachesPostgreSQLOnly`: allowed to PostgreSQL 5432 and DNS; denied to processor 5432/8443, PostgreSQL 5433, `93.184.216.34:443`, `169.254.169.254:80` and `10.0.0.5:5432`; PostgreSQL ingress admits the Job; the probe still cannot reach PostgreSQL | **STATIC ONLY** | same | **PASS**. **FAIL** against the pre-fix chart |
| `TestD2PolicyOnlyWhenJobAndPoliciesEnabled` | **STATIC ONLY** | same | **PASS** |
| `TestCharacterizationDatabaseCredentialSources` | **STATIC ONLY** | same | **PASS**: the gateway, processor and Job read `observex-secrets`/`postgres-password`; `postgresql.auth.password` and `existingSecret` are empty |
| `TestKindRuntimeValuesRender` | **STATIC ONLY** | same | **PASS** |
| `TestCharacterizationRerunIgnoresLedger`, `…RecreatesDeletedDefaultAdmin`, `…AssignsOrglessUserToDefaultOrg`, `…RestoresRemovedTeamMembership`, `TestFailedMigrationIsNotRecorded` | integration (real `scripts/db-migrate.sh`, real PostgreSQL) | `go test -race ./internal/db/dbtest/` | **PASS**: current behaviour reproduced (§8.2 F5) |
| D1 with the real processor and probe binaries | supplementary, non-Kubernetes | see §8.2 | **PASS** |
| Concurrency of the current runner: 3 simultaneous runs on a fresh database, 5 trials | supplementary (sandbox script, not a repository test) | `scripts/db-migrate.sh` ×3 in parallel, then one sequential run | **Characterised.** In every trial two of the three runs failed (exit 3, duplicate-key errors on `pg_type`/`pg_extension` while creating types or extensions) and one succeeded. The sequential re-run completed with 8 ledger rows and 1 default admin. There is no lock (MIG-1a §3) |
| Helm dependency build, lint and template; image builds; kind cluster | — | — | **BLOCKED** (egress, §1; not retried) |
| OPS-1 A–E in Kubernetes; NetworkPolicy enforcement on a CNI | Kubernetes | runbook §8 | **NOT RUN / BLOCKED** |

The stand-in renderer is **not Helm**. It renders no sub-charts, performs no schema or lint validation, and approximates some functions. The policy evaluator is a model of the NetworkPolicy API, **not a CNI**. None of these results shows that Kubernetes accepts or enforces anything.

### 8.5 Workstream D: PostgreSQL credential mismatch (P3) [S]

**Sources inspected:** Bitnami `postgresql` chart **15.5.4** (appVersion 16.3.0; `common` 2.x): `values.yaml`, `templates/secrets.yaml`, `templates/_helpers.tpl`, and `common`'s `_secrets.tpl`, fetched from the chart's source repository at the release tag.

- The sub-chart takes its credentials only from its own values: `auth.username`, `auth.password`, `auth.database`, and `auth.existingSecret` with `auth.secretKeys.{adminPasswordKey,userPasswordKey}`, or the `global.postgresql.auth.*` equivalents. It cannot read the parent chart's `secrets.postgresPassword`.
- **Where the password comes from.** `common.secrets.passwords.manage` resolves it in this order:
  1. an existing `<release>-postgresql` Secret, looked up at install time;
  2. the provided `auth.password`;
  3. otherwise, on install, a random 10-character value.
- **What ObserveX sets and reads.** ObserveX sets `postgresql.auth.password: ""` and `existingSecret: ""`, while its comment claims the password comes from `secrets.postgresPassword`. The gateway (`POSTGRES_PASSWORD`), the F6.1 processor and the migration Job (`F61_POSTGRES_PASSWORD`) all read `observex-secrets`/`postgres-password` (static test above).
- **Conclusion [S]:** on a fresh install with the bundled PostgreSQL, the database user's password is a random value that no ObserveX workload receives, so authentication fails. **Runtime confirmation is NOT RUN**: it needs `helm dependency build` and a cluster (runbook §9, step W-D).

**Not fixed here.** The defect predates F6.1, it affects the gateway, and the fix changes how an existing release's database password is sourced on upgrade. Options for the owner:
1. Set `postgresql.auth.existingSecret` to the chart Secret, with `secretKeys.userPasswordKey: postgres-password` and a matching `adminPasswordKey`. These are documented sub-chart values. The effect on existing releases whose data directory was initialised with a generated password needs a stated migration step.
2. Point the ObserveX consumers at the sub-chart's Secret (`<release>-postgresql`, key `password`).

The runbook works around it only in the disposable cluster, using the documented `postgresql.auth.password` value.

### 8.6 New finding D4: password fragments in logs and DNS [E]

The chart composes `OBSERVEX_F61_POSTGRES_DSN` as a URL by substituting `$(F61_POSTGRES_PASSWORD)` into it without escaping. A local processor run used a throwaway password containing URL-reserved characters. The URL parser then read parts of the password as host and database, and the processor logged them:

```
"msg":"f61 probe request refused" … "error":"… failed to connect to `host=ss user=f61rem_user database=w0rd`: hostname resolving error (lookup ss on 8.8.8.8:53: no such host)"
```

A fragment of the password was written to the log and sent to the DNS resolver as a host name.
- **Scope:** random values from `openssl rand -hex` or the sub-chart's alphanumeric generator are URL-safe. Operator-chosen passwords may not be.
- **Proposed fix (not applied; needs scoping):** either pass the DSN components as separate variables and build the URL with escaping, or stop including raw connection error text in the refusal log.

### 8.7 OPS-1 disposition (unchanged by this addendum)

| Item | Result |
|---|---|
| A. Work delivery over TLS | **BLOCKED** (not run in Kubernetes). D1 is fixed and unit-, integration- and statically tested |
| B. TLS observation and reporting | **BLOCKED** |
| C. Revocation | **BLOCKED** (supplementary non-Kubernetes check passed, §3.C) |
| D. Credential rotation | **BLOCKED** (supplementary check passed, §3.D) |
| E. NetworkPolicy installed and enforced | **BLOCKED**. D2 is fixed **statically**; enforcement is unknown |
| F. Migrations | **PASS** on local PostgreSQL with the supported script; in-cluster Job **BLOCKED**; re-run semantics **UNRESOLVED** (MIG-1a) |
| G. Cleanup | **PASS** (original run). In this remediation the `/etc/hosts` entry was restored, and every database it created was dropped: the per-test databases and `f61conc_*`. The disposable PostgreSQL instance exists only in the sandbox |

**OPS-1 is incomplete.** Kubernetes validation has not been performed.

### 8.8 What has to happen next, in dependency order

1. **Owner decisions:**
   - MIG-1a Q1–Q4: runner semantics, legacy adoption, `008` ordering, the seeded admin.
   - P3 option 1 or 2.
   - Whether D4 is fixed within F6.1.
   - The 001/005 in-place edits versus A7.
   Until MIG-1a is decided, the migration Job stays disabled everywhere persistent.
2. **Engineering, after the decisions:** implement the chosen P3 option and MIG-1a (if approved), each with its tests. Fix D4 if in scope.
3. **External environment:** an authorised disposable machine with the egress listed in the runbook §1. This is an organisational change; this sandbox's restrictions were not bypassed.
4. **Kubernetes and CNI validation:** execute `docs/validation/f61-ops-1-external-runbook.md`. Prove CNI enforcement first (§5), then run A–G with their original definitions, and re-run the chart checks against real `helm template` output. Record the results here as a new dated section.
