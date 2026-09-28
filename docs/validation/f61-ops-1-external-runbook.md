# F6.1 OPS-1 — runbook for the disposable kind run

**Status:** **NOT EXECUTED.** This is a handoff for running OPS-1 tests A–G on an authorised, disposable machine. The Claude sandbox cannot reach the registries and download hosts these steps need (see `f61-ops-1-kind-validation.md` §1). Every command below was derived from the repository's files at the time of writing; none has been run end to end.
**Governance:** all G-1 Increment-3 decisions remain **PROPOSED**. The migration-runner policy (MIG-1 / MIG-1a, D-03) is **UNRESOLVED**: the migration Job may run **only** against this throwaway cluster's database. Nothing here is for a persistent environment.

The acceptance tests are the original OPS-1 A–G definitions, unchanged. Record every result as PASS, FAIL, BLOCKED or NOT RUN in the validation report, with the evidence listed per step.

---

## 0. Ground rules

- **The machine:**
  - Use a fresh VM or workstation with no production credentials.
  - Unset cloud and cluster credentials: `unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY GOOGLE_APPLICATION_CREDENTIALS AZURE_CLIENT_SECRET`.
  - Use a dedicated kubeconfig: `export KUBECONFIG=$HOME/f61-ops1.kubeconfig`.
- **Every** `kubectl`/`helm` command carries `--context kind-f61-ops1` (Helm: `--kube-context kind-f61-ops1`). If `kubectl config current-context` is anything else, stop.
- Secrets are generated here, are test-only, and are written to files with mode `600`. Never `echo` or `cat` them, and never paste them into logs or reports.
- Evidence directory: `export EV=$HOME/f61-ops1-evidence/$(date -u +%Y%m%dT%H%M%SZ); mkdir -p "$EV"`.
- Work from the repository root (`observex/`). `$REPO` below is that directory.

## 1. Prerequisites and required access

| Need | Version (from the repository) | Why |
|---|---|---|
| Docker Engine | any current; record `docker version` | image builds, kind nodes |
| Go | ≥ 1.22 (`go.mod`: `go 1.22.0`) | repository tests |
| kind | **v0.24.0** (sandbox attempt), node image `kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865` (the digest kind v0.24.0 pins) | cluster |
| kubectl | v1.31.x (matches the node image) | cluster access |
| Helm | v3 (pin one, e.g. v3.15.4, and record `helm version`) | dependency build, lint, template, install |
| PostgreSQL client `psql` | 16 | repository migration tests (they call `psql`) |
| openssl, curl | any | test certificates, downloads |

**Egress required** (every host the sandbox was refused, plus Go modules for the Docker builds):
- **Images:** `registry-1.docker.io`, `auth.docker.io`, `production.cloudflare.docker.com`. These serve `golang:1.22-alpine`, `postgres:16-alpine`, `kindest/node`, the Bitnami images, `nginx`, `curlimages/curl` and, if needed, the Calico images.
- **Distroless:** `gcr.io`.
- **Go modules (Dockerfile `go mod download`):** `proxy.golang.org`, `sum.golang.org`.
- **Downloads:** `get.helm.sh`, `dl.k8s.io`, `github.com` release assets (kind, and Calico if needed).
- **Chart repositories:** `grafana.github.io`, `charts.bitnami.com` (the chart's `common` dependency is served from `oci://registry-1.docker.io/bitnamicharts`), `helm.neo4j.com`.

> **Check first [assumption]:** Bitnami changed how its public catalogue is distributed in 2025. If `helm dependency build` cannot fetch `postgresql 15.5.4` (or its images), record the chart dependency step as **BLOCKED** with the exact error. Choosing a replacement is a decision for the project owner, not part of this run.

### Verify every binary

```bash
cd "$(mktemp -d)"
# kind
curl -fsSLo kind https://github.com/kubernetes-sigs/kind/releases/download/v0.24.0/kind-linux-amd64
curl -fsSLo kind.sha256 https://github.com/kubernetes-sigs/kind/releases/download/v0.24.0/kind-linux-amd64.sha256sum
echo "$(cut -d' ' -f1 kind.sha256)  kind" | sha256sum -c -      # expected b89aada5…b600d
# kubectl
curl -fsSLO https://dl.k8s.io/release/v1.31.0/bin/linux/amd64/kubectl
curl -fsSLO https://dl.k8s.io/release/v1.31.0/bin/linux/amd64/kubectl.sha256
echo "$(cat kubectl.sha256)  kubectl" | sha256sum -c -
# helm
curl -fsSLO https://get.helm.sh/helm-v3.15.4-linux-amd64.tar.gz
curl -fsSLO https://get.helm.sh/helm-v3.15.4-linux-amd64.tar.gz.sha256sum
sha256sum -c helm-v3.15.4-linux-amd64.tar.gz.sha256sum && tar xzf helm-v3.15.4-linux-amd64.tar.gz linux-amd64/helm
install -m 0755 kind kubectl linux-amd64/helm /usr/local/bin/
{ docker version; kind version; kubectl version --client; helm version; go version; psql --version; } > "$EV/00-tool-versions.txt" 2>&1
```

## 2. Repository checks before anything is deployed

```bash
cd "$REPO"
sha256sum go.mod go.sum > "$EV/01-go-mod.sha256"
# Unit and static suites (no database needed)
go vet ./internal/... ./services/processor/ ./services/api-gateway/ ./services/synthetic-probe/ ./deployments/helm/chartcheck/ \
  > "$EV/02-vet.txt" 2>&1
go test -race -count=1 ./internal/observe/tlscert/ ./internal/detect/certexpiry/ ./internal/adapt/tlscertexpiry/ \
  ./internal/wire/f61/ ./internal/correlate/f61/ ./internal/compose/f61/ ./internal/evaluate/f61/ ./internal/probetoken/ \
  ./internal/intake/f61/ ./internal/probe/f61/ ./services/processor/ ./services/api-gateway/ ./services/synthetic-probe/ \
  ./deployments/helm/chartcheck/ > "$EV/03-go-test-unit.txt" 2>&1
# Database suites against a DISPOSABLE local PostgreSQL (never a shared one)
PGPW=$(openssl rand -hex 16)
docker run -d --name f61-ops1-pg -e POSTGRES_PASSWORD="$PGPW" -p 127.0.0.1:55433:5432 postgres:16-alpine
until docker exec f61-ops1-pg pg_isready -U postgres >/dev/null 2>&1; do sleep 1; done
OBSERVEX_TEST_POSTGRES_DSN="postgres://postgres:$PGPW@127.0.0.1:55433/postgres?sslmode=disable" \
  go test -race -count=1 -v ./internal/db/dbtest/ ./internal/result/f61/ ./services/processor/ ./services/api-gateway/ \
  > "$EV/04-go-test-db.txt" 2>&1
grep -c -- '--- SKIP' "$EV/04-go-test-db.txt"   # must be 0: database tests must not skip
docker rm -f f61-ops1-pg
```

`internal/db/dbtest` includes the **characterization** tests in `rerun_characterization_test.go`. They pass while the runner still re-applies every file (current behaviour). They are documentation, not acceptance of that behaviour.

## 3. Build the images (real Dockerfiles, repository root as context)

The protected `Makefile` (original version) does not list `synthetic-probe` or `db-migrate`, so build explicitly:

```bash
cd "$REPO"
for s in processor api-gateway synthetic-probe db-migrate; do
  docker build --pull -f services/$s/Dockerfile -t observex/$s:f61-validation . > "$EV/10-build-$s.txt" 2>&1 || echo "BUILD FAILED: $s"
done
for img in golang:1.22-alpine gcr.io/distroless/static-debian12:nonroot postgres:16-alpine; do
  docker image inspect --format '{{.RepoTags}} {{index .RepoDigests 0}}' "$img"
done > "$EV/11-base-image-digests.txt"
for s in processor api-gateway synthetic-probe db-migrate; do
  docker image inspect --format '{{.RepoTags}} {{.Id}} user={{.Config.User}} entrypoint={{.Config.Entrypoint}} cmd={{.Config.Cmd}}' observex/$s:f61-validation
done > "$EV/12-built-images.txt"
```

Pass criteria:
- all four builds succeed;
- `synthetic-probe` runs as `nonroot` with entrypoint `/service` and cmd `serve`;
- `db-migrate` runs as `70:70`.

Optional provenance check: verify the distroless base signature with `cosign`, following the distroless project's instructions, and record the output.

## 4. Helm: dependencies, lint, template

```bash
cd "$REPO"
helm dependency build deployments/helm/observex > "$EV/20-helm-dep.txt" 2>&1
cp deployments/helm/observex/Chart.lock "$EV/21-Chart.lock"                       # dependency versions and digest
V1=deployments/helm/chartcheck/testdata/f61-kind-values.yaml
V2=deployments/helm/chartcheck/testdata/f61-kind-runtime-values.yaml
helm lint deployments/helm/observex -f "$V1" > "$EV/22-helm-lint.txt" 2>&1
helm template observex deployments/helm/observex -n observex -f "$V1" > "$EV/23-rendered-f61.yaml" 2> "$EV/23-render.err"
helm template observex deployments/helm/observex -n observex -f "$V1" -f "$V2" > "$EV/24-rendered-kind.yaml"
# The repository's static checks, now against REAL Helm output:
OBSERVEX_HELM_TEMPLATE_OUTPUT="$EV/23-rendered-f61.yaml" go test -count=1 -v ./deployments/helm/chartcheck/ \
  -run 'TestD1ProbeDialsTheCertificateName|TestD2MigrationJobReachesPostgreSQLOnly|TestCharacterizationDatabaseCredentialSources' \
  > "$EV/25-chartcheck-real-helm.txt" 2>&1
```

Known pre-existing chart defects, not F6.1:
- `trivy-scanner.yaml` cannot render while enabled; `f61-kind-values.yaml` disables it.
- The db-monitor Deployment needs `image.tag` and has no enable flag, so it stays in ImagePullBackOff unless you also build `services/db-monitor`. Record this; it does not affect the F6.1 tests.

## 5. Cluster and CNI

```bash
kind create cluster --name f61-ops1 --image kindest/node:v1.31.0@sha256:53df588e04085fd41ae12de0c3fe4c72f7013bba32a20e7325357a1ac94ba865 \
  --kubeconfig "$KUBECONFIG" > "$EV/30-kind-create.txt" 2>&1
K="kubectl --context kind-f61-ops1"
$K get nodes -o wide > "$EV/31-nodes.txt"; $K -n kube-system get pods -o wide > "$EV/32-kube-system.txt"
```

**Prove NetworkPolicy enforcement before test E counts.** An installed policy object is not proof.

```bash
$K create ns np-baseline
$K -n np-baseline run srv --image=nginx:1.27-alpine --labels=app=srv --port=80
$K -n np-baseline expose pod srv --port=80
$K -n np-baseline wait --for=condition=Ready pod/srv --timeout=120s
$K -n np-baseline run cli --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 3600
$K -n np-baseline wait --for=condition=Ready pod/cli --timeout=120s
$K -n np-baseline exec cli -- curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://srv   # before: 200
cat <<'EOF' | $K -n np-baseline apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata: {name: deny-all-ingress}
spec: {podSelector: {}, policyTypes: [Ingress]}
EOF
sleep 10
$K -n np-baseline exec cli -- curl -s -m 5 -o /dev/null -w '%{http_code}\n' http://srv; echo "exit=$?"   # enforced: timeout, non-zero exit
```

If the second request still returns 200, the default CNI does **not** enforce policies:
1. Record that.
2. Delete the cluster.
3. Recreate it with `networking: {disableDefaultCNI: true, podSubnet: 192.168.0.0/16}` in a kind config file.
4. Install a policy-enforcing CNI. For example, Calico: pin a release, download its `calico.yaml` from the release assets, record its SHA-256, then `kubectl apply` it.
5. Repeat the baseline until the second request is blocked.

Save the outputs as `33-cni-baseline.txt`. Then `$K delete ns np-baseline`.

## 6. Test-only secrets and the synthetic TLS target

```bash
S=$EV/secrets; mkdir -m 700 "$S"; cd "$S"
# Probe credential signing key (KEY-1) and chart secrets — test values only, URL-safe
openssl rand -base64 48 | tr -d '\n' > probe-key
for f in jwt internal pg neo4j admin; do openssl rand -hex 24 | tr -d '\n' > $f; done   # --set-file keeps a trailing newline, so strip it
chmod 600 *
# Intake listener certificate: SAN = the name probes verify (D1). Read it from the render, don't retype it:
TLSNAME=$(grep -m1 'observex.io/tls-server-name' "$EV/24-rendered-kind.yaml" | sed -E 's/.*: "?([^"]*)"?/\1/')
echo "$TLSNAME"   # observex-processor-probe-intake.observex.svc
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout ca.key -out ca.crt -days 3 -subj /CN=f61-ops1-intake-ca
openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout tls.key -out tls.csr -subj /CN=f61-intake
printf 'subjectAltName=DNS:%s\nextendedKeyUsage=serverAuth\n' "$TLSNAME" > tls.ext
openssl x509 -req -in tls.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out tls.crt -days 3 -extfile tls.ext
$K create ns observex
$K -n observex create secret generic observex-synthetic-probe-key --from-file=key=probe-key
$K -n observex create secret tls observex-f61-intake-tls --cert=tls.crt --key=tls.key
$K -n observex create secret generic observex-f61-intake-ca --from-file=ca.crt=ca.crt
# Synthetic TLS target (never a real endpoint): own namespace, own CA, certificate expiring in 20 days
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout tca.key -out tca.crt -days 400 -subj /CN=f61-ops1-target-ca
mktarget() { # $1 = days of validity, $2 = file prefix
  openssl req -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -keyout $2.key -out $2.csr -subj /CN=f61-target
  printf 'subjectAltName=DNS:f61-target.f61-target.svc\nextendedKeyUsage=serverAuth\n' > $2.ext
  openssl x509 -req -in $2.csr -CA tca.crt -CAkey tca.key -CAcreateserial -out $2.crt -days $1 -extfile $2.ext; }
mktarget 20 t20; mktarget 200 t200
$K create ns f61-target
$K -n f61-target create secret tls target-tls --cert=t20.crt --key=t20.key
cat <<'EOF' | $K -n f61-target apply -f -
apiVersion: v1
kind: ConfigMap
metadata: {name: nginx-tls}
data:
  default.conf: |
    server { listen 443 ssl; ssl_certificate /tls/tls.crt; ssl_certificate_key /tls/tls.key; location / { return 200 "ok\n"; } }
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: f61-target}
spec:
  replicas: 1
  selector: {matchLabels: {app: f61-target}}
  template:
    metadata: {labels: {app: f61-target}}
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports: [{containerPort: 443}]
          volumeMounts:
            - {name: tls, mountPath: /tls, readOnly: true}
            - {name: conf, mountPath: /etc/nginx/conf.d, readOnly: true}
      volumes:
        - {name: tls, secret: {secretName: target-tls}}
        - {name: conf, configMap: {name: nginx-tls}}
---
apiVersion: v1
kind: Service
metadata: {name: f61-target}
spec: {selector: {app: f61-target}, ports: [{port: 443, targetPort: 443}]}
EOF
```

## 7. Install

Do not use a global `--wait`: the db-monitor Deployment (pre-existing) cannot become ready. Load the images first.

`postgresql.auth.password` is set to the same test value as `secrets.postgresPassword`. This works around the confirmed credential mismatch (validation report, Workstream D) using the sub-chart's documented `auth.password` value. To observe the mismatch itself, see step W-D.

The migration Job is a Helm hook with `hook-delete-policy: before-hook-creation,hook-succeeded`, so a **successful** Job is deleted as soon as it finishes and `kubectl logs job/…` finds nothing afterwards. Capture its log while it runs (the Job controller labels its pods `job-name=observex-db-migrate`):

```bash
for s in processor api-gateway synthetic-probe db-migrate; do kind load docker-image observex/$s:f61-validation --name f61-ops1; done
( until $K -n observex logs -f -l job-name=observex-db-migrate --tail=-1 > "$EV/43-migrate-job.log" 2>/dev/null \
        && [ -s "$EV/43-migrate-job.log" ]; do sleep 1; done ) &
MIGLOG=$!
helm --kube-context kind-f61-ops1 upgrade --install observex "$REPO/deployments/helm/observex" -n observex \
  -f "$REPO/deployments/helm/chartcheck/testdata/f61-kind-values.yaml" \
  -f "$REPO/deployments/helm/chartcheck/testdata/f61-kind-runtime-values.yaml" \
  --set-file secrets.jwtSecret="$S/jwt" --set-file secrets.internalToken="$S/internal" \
  --set-file secrets.postgresPassword="$S/pg" --set-file postgresql.auth.password="$S/pg" \
  --set-file secrets.neo4jPassword="$S/neo4j" --set-file secrets.adminPassword="$S/admin" \
  --timeout 15m > "$EV/40-helm-install.txt" 2>&1
echo "helm exit=$?" >> "$EV/40-helm-install.txt"   # Helm fails the release if the post-install hook Job fails
$K -n observex get all,networkpolicy,job -o wide > "$EV/41-resources.txt"
$K -n observex get networkpolicy -o yaml > "$EV/42-networkpolicies.yaml"
wait "$MIGLOG" 2>/dev/null; $K -n observex get job observex-db-migrate -o wide >> "$EV/43-migrate-job.log" 2>&1 || true
$K -n observex rollout status deploy/observex-processor --timeout=5m
$K -n observex rollout status deploy/observex-api-gateway --timeout=5m
```

The probe pod waits in `ContainerCreating` until its credential Secret exists (test A). That is the two-phase enrolment.

**Disposable admin login.** The seeded admin's hash does not match the documented password (known defect P5). Set a known one in **this cluster only**:

```bash
$K -n observex exec observex-postgresql-0 -- env PGPASSWORD="$(cat $S/pg)" psql -U observex -d observex -v ON_ERROR_STOP=1 \
  -c "UPDATE users SET password_hash = crypt('$(cat $S/admin)', gen_salt('bf', 12)) WHERE email = 'admin@observex.io'"
$K -n observex port-forward svc/observex-api-gateway 3001:3001 > "$EV/44-port-forward.log" 2>&1 &
TOKEN=$(curl -s http://127.0.0.1:3001/api/auth/login -H 'Content-Type: application/json' \
  -d "{\"email\":\"admin@observex.io\",\"password\":\"$(cat $S/admin)\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
printf '%s' "$TOKEN" > "$S/session"; unset TOKEN
```

(`crypt(…, gen_salt('bf', 12))` from `pgcrypto`, which migration 001 enables, produces `$2a$` bcrypt hashes. In the sandbox the gateway's bcrypt check accepted such a hash.)

## 8. Acceptance tests (original definitions)

`API()` is a shorthand: `API() { curl -s -H "Authorization: Bearer $(cat $S/session)" -H 'Content-Type: application/json' "$@"; }`

### A. Work delivery over TLS
```bash
API -X POST http://127.0.0.1:3001/api/v1/synthetic/checks -d '{"name":"f61 kind target","type":"ssl",
  "target":"f61-target.f61-target.svc:443","locations":["kind-a"],"interval_sec":30,"timeout_sec":5,"namespace":"payments"}' \
  | tee "$EV/50-check.json" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])' > "$S/check-id"
API -X POST http://127.0.0.1:3001/api/v1/synthetic/probe-credentials -d '{"network_zone":"kind-a","ttl_hours":24}' > "$S/issue.json"
python3 -c 'import json;d=json.load(open("'"$S"'/issue.json"));open("'"$S"'/cred","w").write(d["token"]);print(d["vantage_id"])' > "$S/vantage-id"
chmod 600 "$S/cred" "$S/issue.json"
$K -n observex create secret generic observex-probe-kind-a --from-file=credential="$S/cred"
$K -n observex rollout status deploy/observex-probe-kind-a --timeout=5m
sleep 90; $K -n observex logs deploy/observex-probe-kind-a > "$EV/51-probe-A.log"
$K -n observex logs deploy/observex-processor | grep -E 'f61 probe (listener|endpoints)' > "$EV/52-processor-listener.log"
```

**PASS** requires all of the following:
- the probe log shows `"work refreshed"` with `"assignments":1`, and no TLS error;
- the processor log shows `f61 probe listener started (TLS)`;
- the probe's `OBSERVEX_PROCESSOR_URL` (from `41-resources.txt`, or `kubectl get deploy -o yaml`) is `https://$TLSNAME:8443`;
- the listener certificate carries exactly that SAN: `openssl x509 -in $S/tls.crt -noout -ext subjectAltName`.

### B. TLS observation and reporting
```bash
sleep 70   # at least two intervals
API http://127.0.0.1:3001/api/v1/synthetic/checks/$(cat $S/check-id)/tls-certificate > "$EV/53-result-expiring.json"
API http://127.0.0.1:3001/api/v1/synthetic/tls-certificates/events > "$EV/54-events-1.json"
$K -n observex exec observex-postgresql-0 -- env PGPASSWORD="$(cat $S/pg)" psql -U observex -d observex -At \
  -c "SELECT check_id, vantage_id, outcome, leaf_not_after FROM f61_tls_observations" > "$EV/55-db-observations.txt"
# Renew the controlled certificate (200 days) and restart the target
$K -n f61-target create secret tls target-tls --cert="$S/t200.crt" --key="$S/t200.key" --dry-run=client -o yaml | $K -n f61-target apply -f -
$K -n f61-target rollout restart deploy/f61-target; sleep 90
API http://127.0.0.1:3001/api/v1/synthetic/checks/$(cat $S/check-id)/tls-certificate > "$EV/56-result-renewed.json"
API http://127.0.0.1:3001/api/v1/synthetic/tls-certificates/events > "$EV/57-events-2.json"
```

**PASS** requires all of the following:
- first result `status` = `expiring`, `alert_state` = `open`;
- an `opened` event;
- a row in `f61_tls_observations` for the check;
- after renewal, `status` = `ok`, `alert_state` = `none`, and a `resolved` event with the same `episode_id`.

The target's CA is not trusted by the probe (the chart provides no target CA), so `trusted` is false. Expiry is still evaluated. Record this.

### C. Revocation
```bash
API -X POST http://127.0.0.1:3001/api/v1/synthetic/probe-credentials/revoke -d "{\"vantage_id\":\"$(cat $S/vantage-id)\",\"reason\":\"ops1 test C\"}" \
  > "$EV/60-revoke.json"
sleep 60; $K -n observex logs deploy/observex-probe-kind-a --since=2m > "$EV/61-probe-C.log"
$K -n observex get pod -l observex.io/vantage=kind-a -o wide > "$EV/62-probe-readiness.txt"
# Direct check with the revoked credential from a test client that has the probe's network labels (see E)
$K -n observex run f61-client --image=curlimages/curl:8.10.1 --restart=Never \
  --labels=app.kubernetes.io/name=observex,app.kubernetes.io/instance=observex,app.kubernetes.io/component=synthetic-probe \
  --command -- sleep 3600
$K -n observex wait --for=condition=Ready pod/f61-client --timeout=120s
$K -n observex cp "$S/ca.crt" f61-client:/tmp/ca.crt
$K -n observex exec -i f61-client -- sh -c 'curl -s -o /dev/null -w "%{http_code}\n" --cacert /tmp/ca.crt \
  -H "Authorization: Bearer $(cat)" https://'"$TLSNAME"':8443/v1/synthetic/probe/work' < "$S/cred" > "$EV/63-revoked-cred-work.txt"
```

**PASS** requires all of the following:
- the probe log shows a `401` then `"dropping all assignments"`;
- the probe pod is `0/1` Ready;
- the direct request with the revoked credential returns `401`.

### D. Credential rotation
```bash
sleep 2   # the revocation bound (next whole second) must have passed
API -X POST http://127.0.0.1:3001/api/v1/synthetic/probe-credentials -d "{\"vantage_id\":\"$(cat $S/vantage-id)\",\"network_zone\":\"kind-a\",\"ttl_hours\":24}" \
  > "$S/rotate.json"
python3 -c 'import json;d=json.load(open("'"$S"'/rotate.json"));open("'"$S"'/cred2","w").write(d["token"]);print(d["rotated"],d["vantage_id"])' > "$EV/70-rotated.txt"
chmod 600 "$S/cred2" "$S/rotate.json"
$K -n observex create secret generic observex-probe-kind-a --from-file=credential="$S/cred2" --dry-run=client -o yaml | $K -n observex apply -f -
# kubelet propagates the Secret; the probe re-reads it on its next attempt (backoff up to 5 minutes)
for i in $(seq 1 60); do $K -n observex get pod -l observex.io/vantage=kind-a -o jsonpath='{.items[0].status.containerStatuses[0].ready}' | grep -q true && break; sleep 10; done
$K -n observex logs deploy/observex-probe-kind-a --since=10m > "$EV/71-probe-D.log"
$K -n observex exec -i f61-client -- sh -c 'curl -s -o /dev/null -w "%{http_code}\n" --cacert /tmp/ca.crt \
  -H "Authorization: Bearer $(cat)" https://'"$TLSNAME"':8443/v1/synthetic/probe/work' < "$S/cred" > "$EV/72-old-cred.txt"
$K -n observex exec -i f61-client -- sh -c 'curl -s -o /dev/null -w "%{http_code}\n" --cacert /tmp/ca.crt \
  -H "Authorization: Bearer $(cat)" https://'"$TLSNAME"':8443/v1/synthetic/probe/work' < "$S/cred2" > "$EV/73-new-cred.txt"
$K -n observex get pod -l observex.io/vantage=kind-a -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}' > "$EV/74-restarts.txt"
```

**PASS** requires all of the following:
- `rotated` is `True` with the same vantage ID;
- the probe becomes Ready again **without a restart** (restart count unchanged) and logs `"work refreshed"` then `"probe reported"`;
- the old credential gets `401` and the new one `200`.

### E. NetworkPolicy
Run only after §5 proved enforcement. The test clients carry the named labels only to take on that workload's **network** identity; they hold no credentials except where stated.

```bash
$K -n observex get networkpolicy > "$EV/80-policies.txt"   # expect default-deny, f61-probe, f61-probe-private-targets, f61-processor, f61-db-migrate, storage-access, …
PGIP=$($K -n observex get svc observex-postgresql -o jsonpath='{.spec.clusterIP}')
TGTPOD=$($K -n f61-target get pod -l app=f61-target -o jsonpath='{.items[0].status.podIP}')
# tcp POD HOST PORT LABEL: did a TCP connection open? curl speaks HTTP to any port; exit 7 (could not
# connect) or 28 (timed out) means no connection, any other exit means one was made. kubectl exec returns curl's exit code.
tcp() { $K -n observex exec "$1" -- curl -s -m 5 -o /dev/null "http://$2:$3/" >/dev/null 2>&1; rc=$?
        case $rc in 7|28) echo "BLOCKED    $4 (curl exit $rc)";; *) echo "CONNECTED  $4 (curl exit $rc)";; esac; }
{
 # f61-client has probe labels but NOT observex.io/private-targets
 tcp f61-client "$TLSNAME" 8443          "probe-labelled -> intake 8443 (expect CONNECTED)"
 tcp f61-client "$PGIP" 5432             "probe-labelled -> PostgreSQL 5432 (expect BLOCKED)"
 tcp f61-client observex-processor 8080  "probe-labelled -> processor 8080 (expect BLOCKED)"
 tcp f61-client "$TGTPOD" 443            "probe-labelled, no private-targets label -> private target pod 443 (expect BLOCKED)"
 tcp f61-client 169.254.169.254 80       "probe-labelled -> metadata address (expect BLOCKED)"
} > "$EV/81-probe-network.txt"
$K -n observex run f61-unlabelled --image=curlimages/curl:8.10.1 --restart=Never --command -- sleep 3600
$K -n observex run f61-migrate-like --image=curlimages/curl:8.10.1 --restart=Never \
  --labels=app.kubernetes.io/name=observex,app.kubernetes.io/instance=observex,app.kubernetes.io/component=db-migrate --command -- sleep 3600
$K -n observex wait --for=condition=Ready pod/f61-unlabelled pod/f61-migrate-like --timeout=120s
{
 tcp f61-unlabelled   observex-processor-probe-intake 8443 "unlabelled pod -> intake 8443 (expect BLOCKED: ingress)"
 tcp f61-migrate-like "$PGIP" 5432                          "db-migrate-labelled -> PostgreSQL 5432 (expect CONNECTED)"
 tcp f61-migrate-like observex-processor-probe-intake 8443 "db-migrate-labelled -> intake 8443 (expect BLOCKED)"
 tcp f61-migrate-like 93.184.216.34 443                    "db-migrate-labelled -> public address 443 (expect BLOCKED)"
} > "$EV/82-other-network.txt"
```

**PASS** requires that every line matches its expectation, that tests A–D ran with the policies enforced (the probe could reach the processor and the private target), and that the migration Job completed under those policies (`43-migrate-job.log` contains `db-migrate: done`, and `40-helm-install.txt` ends `helm exit=0`).

How to read a BLOCKED line:
- A policy drop usually shows as exit 28 (timeout). Exit 7 (refused) proves nothing on its own, because a destination with no listener also refuses. It counts only when the same destination is shown listening: PostgreSQL and the intake by the CONNECTED lines above, the target pod by test B.
- kind runs no metadata service, so the `169.254.169.254` line is not meaningful there. Record it as such.
- If a policy uses `ipBlock` for pod IPs, the result depends on the CNI. Record which CNI and version decided each line.

### F. Database migrations
- **In cluster (fresh database):** the Job log `43-migrate-job.log` lists `db-migrate: applying 001_initial` through `db-migrate: applying 007_feature_state`, then `db-migrate: applying 009_f61_tls_certificates` (there is no 008 file in `internal/db/migrations/`), then `db-migrate: done`. Also check `SELECT string_agg(version, ',' ORDER BY applied_at, version) FROM schema_migrations` via `psql` in `observex-postgresql-0`.
- **E1/E2 upgrades, ordering, data preservation:** the repository suites in §2 (`04-go-test-db.txt`). For the full harness, including row-level preservation, see Appendix A of the validation report; it ran in the sandbox on 2026-09-26 and may be re-run here against the disposable PostgreSQL.
- Do **not** re-run the Job against any database you intend to keep. The re-run effects are recorded in MIG-1a and unresolved.

### G. Cleanup
```bash
kill %1 2>/dev/null   # port-forward
kind delete cluster --name f61-ops1
kind get clusters > "$EV/90-clusters-after.txt"                 # must not list f61-ops1
docker ps -a --filter name=f61-ops1 --format '{{.Names}}' > "$EV/91-containers-after.txt"   # must be empty
docker image rm observex/{processor,api-gateway,synthetic-probe,db-migrate}:f61-validation
rm -f "$KUBECONFIG"; shred -u "$S"/* 2>/dev/null || rm -f "$S"/*; rmdir "$S"
```

**PASS:**
- no kind cluster, container or kubeconfig remains;
- no command ever used a context other than `kind-f61-ops1`;
- no secret appears in `$EV` (check with `grep -rl -E 'oxpt_[A-Za-z0-9]{8}' "$EV"`, which must print nothing).

## 9. Step W-D (optional): observe the database credential mismatch

Install as in §7 but **without** `--set-file postgresql.auth.password=…`. Then:
- `kubectl logs deploy/observex-api-gateway` should show a PostgreSQL authentication failure;
- `kubectl get secret observex-postgresql -o jsonpath='{.data}'` should show a `password` key the chart generated.

This is the runtime confirmation that the validation could not perform. Record it; the fix needs a decision (see the report).
