# ObserveX Agent SaaS Playbook

This playbook defines the production path for the ObserveX Agent, excluding the future AI monitoring agent.

## SaaS Model

ObserveX hosts the control plane, APIs, and ingest endpoints. Customers install a lightweight ObserveX Agent on hosts, VMs, containers, or Kubernetes nodes. The agent uses a tenant deployment token and a local config file to connect telemetry back to ObserveX directly or through an ActiveGate.

## Runtime Contract

The agent reads configuration from `--config`, `OBSERVEX_CONFIG`, or `/etc/observex/agent.yaml`, with environment variables overriding file values.

Required keys:

- `token`
- `org_id` or `environment_id`
- `tenant_url`
- `ingestor_url`

Fleet keys:

- `monitoring_mode`: `fullstack`, `infrastructure`, or `discovery-only`
- `collection_mode`: `native`
- `host_group`
- `network_zone`
- `environment`
- `auto_update`
- `update_channel`

Collectors:

- `process_discovery`
- `k8s_monitoring`
- `ebpf_enabled`
- `profiling_enabled`
- `log_monitoring`
- `native_metrics`

## Backend Flow

1. `POST /api/v1/agent/install-token` creates a scoped deployment token.
2. `GET /api/v1/agent/install/:platform` returns Linux, Windows, Kubernetes, Docker, or Helm install content.
3. `GET /api/v1/deployment/installer/agent/:os/:flavor/latest` returns a Dynatrace-style bootstrap installer artifact.
4. Agent registers at `/v1/agents/register`.
5. Agent heartbeats at `/v1/agents/:id/heartbeat`.
6. Ingestor upserts fleet metadata and emits `observex_agent_up`.

## ObserveX-Native Collection

The default install mode is `collection_mode: native`. The ObserveX Agent collects host, process, container, Kubernetes, log, topology, security, trace, and profile telemetry directly and sends ObserveX JSON batches to the ingestor. Metrics are enriched with tenant and fleet labels:

- `org`
- `agent_id`
- `node`
- `cluster`
- `environment`
- `host_group`
- `network_zone`
- `monitoring_mode`
- `collection_mode`

## ActiveGate

ActiveGate is the private-network bridge. It forwards agent identity headers and can enforce a default `OBSERVEX_NETWORK_ZONE` for routed telemetry. Agents can point `ingestor_url` at ActiveGate when direct SaaS egress is not allowed.

## Launch Checklist

- Configure `AGENT_TOKEN_SECRET` consistently on API gateway and ingestor.
- Set `OBSERVEX_TENANT_URL`, `OBSERVEX_INGEST_URL`, and optional `OBSERVEX_ACTIVEGATE_URL`.
- Publish signed Linux and Windows agent binaries for the release channel.
- Package Kubernetes manifests and Helm chart values from the same config contract.
- Enable TLS, token rotation, and short enough deployment-token TTL for the target customer environment.
- Validate fleet list, heartbeat freshness, and `observex_agent_up` labels in the native metrics table.
