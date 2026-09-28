import { createReadStream, existsSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = fileURLToPath(new URL('.', import.meta.url))
const root = normalize(join(here, '..', 'frontend', 'dist'))
const port = Number(process.env.PORT || 4180)

const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
}

const agents = [
  {
    id: 'agent-pay-01',
    org_id: 'demo-org',
    node_name: 'ip-10-12-1-44',
    cluster_name: 'payments-prod',
    environment: 'production',
    host_group: 'payments-prod',
    network_zone: 'private-activegate-mumbai',
    monitoring_mode: 'fullstack',
    collection_mode: 'native',
    log_monitoring: true,
    auto_update: true,
    update_channel: 'stable',
    version: '2.0.0',
    os: 'linux',
    arch: 'amd64',
    ebpf_enabled: true,
    status: 'active',
    last_seen: new Date().toISOString(),
    registered_at: new Date(Date.now() - 3600_000).toISOString(),
  },
  {
    id: 'agent-core-03',
    org_id: 'demo-org',
    node_name: 'aks-nodepool-3',
    cluster_name: 'core-prod',
    environment: 'production',
    host_group: 'core-prod',
    network_zone: 'public-saas',
    monitoring_mode: 'fullstack',
    collection_mode: 'native',
    log_monitoring: true,
    auto_update: true,
    update_channel: 'stable',
    version: '2.0.0',
    os: 'linux',
    arch: 'amd64',
    ebpf_enabled: true,
    status: 'active',
    last_seen: new Date().toISOString(),
    registered_at: new Date(Date.now() - 7200_000).toISOString(),
  },
]

function json(res, body, status = 200) {
  res.writeHead(status, { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' })
  res.end(JSON.stringify(body))
}

function text(res, body, contentType = 'text/plain; charset=utf-8') {
  res.writeHead(200, { 'content-type': contentType, 'cache-control': 'no-store' })
  res.end(body)
}

function installer(platform) {
  const token = 'oxat_demo_' + Math.random().toString(16).slice(2)
  const config = `token: "${token}"
org_id: "demo-org"
tenant_url: "http://127.0.0.1:${port}"
ingestor_url: "http://127.0.0.1:${port}"
agent_version: "2.0.0"
monitoring_mode: "fullstack"
collection_mode: "native"
host_group: "payments-prod"
network_zone: "private-activegate-mumbai"
log_monitoring: true
process_discovery: true
k8s_monitoring: true
ebpf_enabled: true
auto_update: true
update_channel: "stable"`
  if (platform === 'windows') {
    return `$ErrorActionPreference = "Stop"\nNew-Item -ItemType Directory -Force -Path "C:\\ProgramData\\ObserveX" | Out-Null\n@'\n${config}\n'@ | Set-Content "C:\\ProgramData\\ObserveX\\agent.yaml"\nNew-Service -Name ObserveXAgent -BinaryPathName "observex-agent.exe --config C:\\ProgramData\\ObserveX\\agent.yaml"\n`
  }
  if (platform === 'kubernetes') {
    return `apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: observex-agent-config\n  namespace: observex\ndata:\n  agent.yaml: |\n${config.split('\n').map(line => '    ' + line).join('\n')}\n---\napiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: observex-agent\n  namespace: observex\nspec:\n  template:\n    spec:\n      containers:\n        - name: observex-agent\n          image: ghcr.io/observex/agent:2.0.0\n          args: ["--config", "/etc/observex/agent.yaml"]\n`
  }
  return `#!/usr/bin/env bash\nset -euo pipefail\nsudo install -d -m 0755 /opt/observex/agent /etc/observex\ncat <<'OBSERVEX_CONFIG' | sudo tee /etc/observex/agent.yaml >/dev/null\n${config}\nOBSERVEX_CONFIG\nsudo systemctl enable --now observex-agent\n`
}

function api(req, res, url) {
  if (url.pathname === '/api/auth/me') {
    return json(res, {
      user: { id: 'demo-user', email: 'admin@observex.io', name: 'ObserveX Demo Admin', role: 'admin', is_active: true },
      org: { id: 'demo-org', name: 'observex-demo', display_name: 'ObserveX Demo', plan: 'enterprise' },
      effective_role: 'admin',
      namespace_access: { default: 'admin', production: 'admin', staging: 'admin' },
    })
  }
  if (url.pathname === '/api/v1/agents') return json(res, { agents })
  if (url.pathname === '/api/v1/agent/status') return json(res, { active_agents: agents.length, org_id: 'demo-org', checked_at: new Date().toISOString() })
  if (url.pathname === '/api/v1/agent/versions') {
    return json(res, { latest: '2.0.0', platforms: ['linux', 'windows', 'kubernetes', 'docker', 'helm'], versions: [{ version: '2.0.0', released: '2026-06-28', notes: 'SaaS install contract, native collection, ActiveGate metadata' }] })
  }
  if (url.pathname === '/api/v1/agent/install-token') {
    return json(res, {
      token: 'oxat_demo_' + Math.random().toString(16).slice(2),
      tenant_url: `http://127.0.0.1:${port}`,
      ingestor_url: `http://127.0.0.1:${port}`,
      environment_id: 'demo-org',
      org_id: 'demo-org',
      expires_at: new Date(Date.now() + 7 * 86400_000).toISOString(),
      expires_in_sec: 604800,
      download_base_url: `/api/v1/deployment/installer/agent`,
      scopes: ['agent:write', 'metrics:write', 'logs:write', 'traces:write', 'topology:write', 'security:write'],
      installer_endpoints: {
        linux: '/api/v1/agent/install/linux',
        windows: '/api/v1/agent/install/windows',
        kubernetes: '/api/v1/agent/install/kubernetes',
      },
    })
  }
  const installMatch = url.pathname.match(/^\/api\/v1\/agent\/install\/([^/]+)$/)
  if (installMatch) return text(res, installer(installMatch[1]))
  if (url.pathname.includes('/checksum')) return text(res, '9b4072d0f1 demo-observex-agent-installer\n')
  if (url.pathname.startsWith('/api/')) return json(res, { data: [], source: 'demo-mock', ok: true })
  return false
}

function staticFile(req, res, url) {
  let rel = decodeURIComponent(url.pathname).replace(/^\/+/, '')
  if (!rel || rel === '/') rel = 'index.html'
  let filePath = normalize(join(root, rel))
  if (!filePath.startsWith(root) || !existsSync(filePath) || !statSync(filePath).isFile()) {
    filePath = join(root, 'index.html')
  }
  res.writeHead(200, {
    'content-type': types[extname(filePath)] || 'application/octet-stream',
    'cache-control': 'no-store',
  })
  createReadStream(filePath).pipe(res)
}

createServer((req, res) => {
  const url = new URL(req.url || '/', `http://127.0.0.1:${port}`)
  if (api(req, res, url) !== false) return
  staticFile(req, res, url)
}).listen(port, '127.0.0.1', () => {
  console.log(`ObserveX full application demo: http://127.0.0.1:${port}`)
})
