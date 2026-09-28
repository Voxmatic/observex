# 09 — Staging Inventory Form (P-7)

**How to use:** Fill in each **Value** and **Owner**, run the **Verification command** on the runner, and paste the evidence. Only an item with evidence can move from **Pending** to **Verified**. Every field starts as **Pending**, and nothing here is assumed.

**Context (provisional):** SD-1 disposable kind cluster per run · SD-2 synthetic data only · SD-3 owner runs and returns evidence · SD-4 isolated test IdP required · SD-5 real rollback test with per-run permission.

**Evidence format (every row):** `command` · `environment/host` · `UTC timestamp` · `exit code` · `output summary`

---

## 1. Inventory

| # | Area | Field | Value | Owner | Verification command (suggested) | Evidence | Status |
|---|---|---|---|---|---|---|---|
| I-1 | Runner & Go module access | Runner type | [local machine / GitHub Actions / other CI: ______] | | — | | Pending |
| | | OS / arch | | | `uname -sm` | | Pending |
| | | Go version (≥ 1.23) | | | `go version` | | Pending |
| | | Go module download | [available / blocked] | | `go mod download github.com/gofiber/fiber/v2@v2.52.4` | | Pending |
| | | Checksum DB reachable | [yes / no] | | `curl -sS -o /dev/null -w '%{http_code}' https://sum.golang.org/lookup/github.com/gofiber/fiber/v2@v2.52.4` | | Pending |
| | | Security tools | gosec [ver] / govulncheck [ver] | | `gosec -version`; `govulncheck -version` | | Pending |
| | | Claude sandbox module access | Blocked (403 proxy.golang.org, 2026-09-16T15:36Z) | Owner (egress settings) | as above | Recorded in `07` | **Blocked** |
| I-2 | Docker & kind | Docker engine | [version] | | `docker version --format '{{.Server.Version}}'` | | Pending |
| | | kind | [version] | | `kind version` | | Pending |
| | | kubectl / Helm | [versions] | | `kubectl version --client`; `helm version --short` | | Pending |
| | | Resources for a run (CPU/RAM/disk) | [e.g. __ vCPU / __ GiB / __ GiB] | | `nproc; free -g; df -h` | | Pending |
| | | Image source for A1 builds | [local build / registry: ______] | | `docker build …` or registry pull | | Pending |
| I-3 | Cluster/host owner | Accountable owner for the disposable cluster runs | [name / team] | | — | | Pending |
| | | Teardown guaranteed after each run | [yes / no; method] | | `kind get clusters` after the run → empty | | Pending |
| I-4 | Postgres | Source | [container in kind via Helm / sidecar / other] | | — | | Pending |
| | | Version (16.x) | | | `psql -c 'select version()'` | | Pending |
| | | Migrations 001–007 applied | [yes / no] | | `psql -c '\dt'` (expect orgs, users, sso_configs…) | | Pending |
| | | ClickHouse source & version (24.3) | | | `curl -s 'http://<ch>:8123/?query=SELECT%20version()'` | | Pending |
| | | ClickHouse schema loaded (which: Compose `clickhouse-init.sql` or Helm `configmap.yaml`; see O-4) | [compose / helm] | | `SHOW TABLES FROM default` / `FROM observex` | | Pending |
| I-5 | Ingress / proxy | Ingress controller | [ingress-nginx / Traefik / other] | | `kubectl get ingressclass` | | Pending |
| | | `/api/auth/sso/*` block rule in place | [yes / no; rule ref] | | `curl -s -o /dev/null -w '%{http_code}' http://<host>/api/auth/sso/providers` → blocked status | | Pending |
| | | Confirmation request never reached gateway | [yes / no] | | gateway logs show no `/api/auth/sso` entries | | Pending |
| I-6 | Secret provisioning | Method | [Helm-generated / pre-created Secret / Compose .env] | | — | | Pending |
| | | Helm key `internal-token` present | [yes / no] | | `kubectl get secret <name> -o jsonpath='{.data.internal-token}' \| wc -c` (length only, **never print value**) | | Pending |
| | | Mounted as `OBSERVEX_INTERNAL_TOKEN_FILE` | [yes / no] | | `kubectl exec … -- sh -c 'test -s "$OBSERVEX_INTERNAL_TOKEN_FILE" && echo present'` | | Pending |
| | | Other secrets (JWT, agent token, integration key) are run-unique test values | [yes / no] | | — | | Pending |
| I-7 | Network isolation | No route to production DBs, APIs, IdP tenants | [yes / no] | | from a pod: `nc -zv <prod-host> <port>` → fails | | Pending |
| | | No production kubeconfig/credentials on runner during the run | [yes / no] | | `kubectl config get-contexts` shows only `kind-*` | | Pending |
| | | Egress policy for the kind cluster | [open / restricted: ______] | | — | | Pending |
| I-8 | Log access | Gateway, query engine, ingestor, processor, AI agent logs retrievable | [yes / no] | | `kubectl logs deploy/<svc> --since=1h` | | Pending |
| | | Log retention for evidence | [attached to CI run / archived at ______] | | — | | Pending |
| | | Token-leak check on collected logs | [yes / no] | | `grep -c "<token>" logs/*` → 0 (run locally; don't paste token) | | Pending |
| I-9 | Test IdP (SD-4) | Owner | [name / team] | | — | | Pending |
| | | IdP product and deployment | [e.g. Keycloak container in kind / other: ______] | | `kubectl get pods -n <idp-ns>` | | Pending |
| | | Isolated (not a production IdP tenant) | [yes / no] | | — | | Pending |
| | | Realm/app configured with SP entity ID and ACS `…/api/auth/sso/saml/callback`; signing cert exported | [yes / no] | | IdP metadata fetch | | Pending |
| | | Test users for Org A (and a user existing only in Org B) | [yes / no] | | — | | Pending |
| | | Note: the SSO ingress block must be **lifted only inside the disposable cluster** for the SAML test | [acknowledged / no] | | — | | Pending |
| I-10 | Rollback namespace (SD-5) | Owner | [name / team] | | — | | Pending |
| | | Namespace name (disposable) | [e.g. `a1-rollback-test`] | | `kubectl get ns <name>` | | Pending |
| | | Sample app with ≥ 3 revisions | [yes / no] | | `kubectl rollout history deploy/<app> -n <ns>` | | Pending |
| | | AI agent RBAC limited to this namespace for the run | [yes / no] | | `kubectl auth can-i update deployments -n default --as system:serviceaccount:<ns>:<sa>` → no | | Pending |
| | | Per-run permission recorded (§3) | [yes / no] | | — | | Pending |

---

## 2. Fixture creation status

| # | Fixture | Owner | Status |
|---|---|---|---|
| SF-1 | Org A / Org B with viewer, editor, admin users (synthetic) | | Pending |
| SF-2 | SAML config rows: Org A enabled + cert; Org B none; Org C enabled, no cert | | Pending |
| SF-3 | Isolated test IdP (SD-4) | | Pending |
| SF-4 | Synthetic telemetry with injection strings | Provided with S1-08 PR | Pending (PR not written; A1 on hold) |
| SF-5 | Sample app ≥ 3 revisions in disposable namespace (SD-5) | | Pending |
| SF-6 | `tests/security/a1/*.sh` exploit scripts | Provided with PRs | Pending (A1 on hold) |

---

## 3. Per-run rollback permission record (SD-5)

A real rollback run may start **only** after a new row is added here by the owner.

| Run ID | Date/time (UTC) | Cluster (`kind-…`) | Namespace | Deployment | Approved by | Scope limits | Result evidence |
|---|---|---|---|---|---|---|---|
| | | | | | | | |

---

## 4. Holds (unchanged)
A1 implementation **on hold** · A2 **on hold** · S0 patch **on hold** · P-7 **Pending clarification**.
