# ObserveX — AI-Powered Full-Stack Observability Platform

> Self-hosted · Production-grade · Zero vendor lock-in
> Built from scratch to match Dynatrace / New Relic / Datadog feature parity

[![CI](https://github.com/your-org/observex/actions/workflows/ci.yml/badge.svg)](https://github.com/your-org/observex/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

## What is ObserveX?

ObserveX is a production-grade, AI-powered observability platform that you deploy on your own infrastructure. It collects, stores, and analyzes metrics, logs, traces, and events from your entire stack — then uses AI to detect anomalies, forecast capacity issues, and automatically remediate problems.

**No data leaves your network. No per-seat pricing. No vendor lock-in.**

## Feature coverage

| Category | Features |
|----------|----------|
| **APM** | Apdex score, RED metrics, transaction tracing, error fingerprinting |
| **Infrastructure** | Per-node CPU/mem/disk/network, K8s pod monitoring, node drilldown |
| **Kubernetes** | Pod list + status, namespace view, K8s events, resource usage |
| **Databases** | PostgreSQL slow queries, connection pools, table bloat, live activity |
| **Logs** | LogQL explorer, live tail, trace→log correlation via trace_id Loki label |
| **Traces** | Waterfall view, Tempo backend, span-level log links |
| **Metrics** | Native ObserveX metric store, multi-series charts, bounded metric APIs |
| **SLOs** | Error budget, multi-window burn rate (1h/6h/24h), Google SRE thresholds |
| **Alerts** | Threshold + anomaly rules, inhibition rules, alert grouping |
| **AI Remediation** | Auto-detect → plan → execute → verify loop, 9 K8s actions |
| **Forecasting** | Linear regression → Holt-Winters → seasonal decomposition |
| **Security** | Trivy CVE scanning, eBPF syscall probes, MITRE ATT&CK tagging |
| **RUM** | Core Web Vitals, user journeys, app versions, crash reporting |
| **Synthetic** | HTTP, DNS, TCP, SSL, ping, gRPC, WebSocket — 8 check types |
| **Profiling** | CPU/heap/goroutine/mutex flame graphs |
| **Cost (FinOps)** | Per-service $/hour from CPU+memory metrics |
| **Integrations** | Slack, PagerDuty, OpsGenie, Teams, OTLP, AWS, GCP |
| **On-call** | Schedules, escalation policies, alert routing tree |
| **Postmortems** | Structured 5-whys, timeline, action items |
| **Dashboards** | Grafana-like builder, 8 live widget types, 4 templates |
| **Serverless** | Lambda/Cloud Functions monitoring via AWS integration |
| **Network** | Inter-service flow monitoring, bandwidth, latency |

## Quick start (Docker Compose — 5 minutes)

```bash
# 1. Clone
git clone https://github.com/your-org/observex.git
cd observex

# 2. Generate go.sum (needs internet, once only)
go mod tidy

# 3. Create the internal service token file (git-ignored, never commit it)
cd deployments/docker
mkdir -p secrets && openssl rand -hex 32 > secrets/internal-token

# 4. Start everything
docker compose up -d

# 5. Open dashboard
open http://localhost:3001
# Login: admin@observex.io / admin123
```

## Architecture

```
Agents (OneAgent DaemonSet)
  ↓ HTTPS :9999
ActiveGate Proxy Tier  ←→  nginx load balancer
  ↓ gzip + buffer
Ingestor cluster (:4318)
  ↓ parallel fanout
  ├── ClickHouse native metrics
  ├── Loki (logs + trace_id labels)
  ├── Tempo (traces)
  ├── ClickHouse (events + topology history)
  └── PostgreSQL (config, SLOs, dashboards)
  ↓
Processor (anomaly detection + SLO + alerts)
  ↓ problems
AI Agent (plan → approve/reject → execute → verify)
  ↓ REST API
API Gateway (:3001) → React Frontend
```

## Services

| Service | Port | Purpose |
|---------|------|---------|
| `api-gateway` | 3001 | REST API, auth (JWT+RBAC), routing |
| `ingestor` | 4318 | Native agent, OTLP, logs, traces, topology, and profile receiver |
| `processor` | 8080 | Anomaly detection, SLOs, alerting |
| `ai-agent` | 8081 | Auto-remediation orchestrator |
| `oneagent` | — | DaemonSet: K8s watch, /proc, eBPF |
| `activegate` | 9999 | Agent proxy + HA load balancer |
| `trivy-scanner` | 8085 | CVE scanning scheduler |
| `db-monitor` | 8087 | PostgreSQL/ClickHouse metrics |
| `query-engine` | 8086 | Multi-source federated queries |

## Deployment options

### Docker Compose (development / small teams)
```bash
make dev
```

### Kubernetes with Helm (production)
```bash
make helm-install REGISTRY=your-registry VERSION=v1.0.0
```

### Manual build
```bash
make build          # Go binaries → bin/
make build-frontend # React → frontend/dist/
make docker-build   # Docker images
```

## Configuration

All services are configured via environment variables. Copy `.env.example` to `.env`:

```bash
cp .env.example .env
# Edit .env with your values
docker compose --env-file .env up -d
```

Key variables:
```bash
JWT_SECRET=<random-32-char-string>
POSTGRES_PASSWORD=<strong-password>
CLUSTER_NAME=production
CLICKHOUSE_URL=http://clickhouse:8123
LOKI_URL=http://loki:3100
TEMPO_URL=http://tempo:3200
OBSERVEX_INGEST_TOKEN=<shared ingest token for ObserveX Agents>
OBSERVEX_COLLECTION_MODE=native
OBSERVEX_INTERNAL_TOKEN_FILE=<path to internal service token file, min 32 bytes>
```

Internal service authentication (token file precedence, fail-closed behaviour,
401 vs 503, rotation) and the WebSocket authentication limits are described in
[docs/configuration/internal-service-token.md](docs/configuration/internal-service-token.md).

## Metrics Collection

ObserveX Agent is the production metrics collector. It reads host and process data from the operating system, Kubernetes/container data from native runtime interfaces, and application/runtime data from agent modules. It sends authenticated batches to:

```text
https://<observex-ingestor>:4318/v1/metrics/batch
```

The ingestor validates the ObserveX Agent token, writes native metric records to ClickHouse, and fans logs, traces, topology, security events, and profiles to their dedicated stores. Native `/proc` and container collectors are enabled by default; no external scraping tier is required.

## Instrument your app

**Browser (Web):**
```html
<script src="https://your-observex.com/observex-rum.min.js"></script>
<script>
  ObserveX.init({ appId: 'my-app', appVersion: '1.0.0', ingestor: 'https://your-observex.com:4318' });
</script>
```

**React Native (Mobile):**
```js
import ObserveX from './sdk/rn/observex-rn';
ObserveX.init({ appId: 'my-app', appVersion: '1.0.0', platform: 'ios', ingestor: 'https://...' });
ObserveX.attachAppState(require('react-native').AppState);
```

**OTLP (any language):**
```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://your-observex.com:4318 \
OTEL_SERVICE_NAME=my-service \
./your-app
```

## License

MIT License — see [LICENSE](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and contribution guidelines.
