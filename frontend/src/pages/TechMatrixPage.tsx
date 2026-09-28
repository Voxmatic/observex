// TechMatrixPage.tsx — Technology Coverage Matrix (Dynatrace Hub equivalent)
// Shows every language, framework, DB, cloud, AI/ML platform supported

import { useState } from 'react'
import { CheckCircle, AlertCircle, Circle, Search } from 'lucide-react'

// Coverage levels
const L = { FULL: 'full', PARTIAL: 'partial', OTLP: 'otlp', NO: 'no' } as const
type Level = typeof L[keyof typeof L]

const GROUPS = [
  {
    id: 'languages', label: 'Languages & Runtimes', icon: '⌨️',
    items: [
      { name: 'Java', versions: '8–23', modes: ['full-stack', 'app-only', 'lambda'], lvl: L.FULL, notes: 'JVM metrics, heap, GC, thread pools' },
      { name: '.NET / C#', versions: '.NET 4.x – 8.0', modes: ['full-stack', 'app-only'], lvl: L.FULL, notes: 'Framework + Core, CLR metrics' },
      { name: 'Node.js', versions: '14, 16, 18, 20, 22', modes: ['full-stack', 'lambda'], lvl: L.FULL, notes: 'Event loop lag, V8 heap, libuv' },
      { name: 'Python', versions: '2.7, 3.6–3.12', modes: ['full-stack', 'lambda'], lvl: L.FULL, notes: 'GIL metrics, thread profiling' },
      { name: 'Go', versions: '1.18+ (all stable)', modes: ['full-stack', 'lambda'], lvl: L.FULL, notes: 'Goroutine profiling, GC pause' },
      { name: 'PHP', versions: '7.4–8.3', modes: ['full-stack'], lvl: L.FULL, notes: 'FPM process metrics, OPcache' },
      { name: 'Ruby', versions: '2.7–3.3', modes: ['full-stack'], lvl: L.FULL, notes: 'GC stats, object allocation' },
      { name: 'C / C++', versions: 'Any', modes: ['custom'], lvl: L.PARTIAL, notes: 'Via SDK or OpenTelemetry' },
      { name: 'Rust', versions: 'Stable', modes: ['otlp'], lvl: L.OTLP, notes: 'Via OpenTelemetry auto-instrumentation' },
      { name: 'Kotlin', versions: 'JVM only', modes: ['full-stack'], lvl: L.FULL, notes: 'Same as Java on JVM' },
      { name: 'Scala', versions: 'JVM only', modes: ['full-stack'], lvl: L.FULL, notes: 'Same as Java on JVM' },
      { name: 'Erlang / Elixir', versions: 'OTP 24+', modes: ['otlp'], lvl: L.OTLP, notes: 'Via OpenTelemetry' },
    ],
  },
  {
    id: 'frameworks', label: 'Web Frameworks', icon: '🌐',
    items: [
      { name: 'Spring / Spring Boot', versions: '5.x, 6.x', modes: ['full-stack'], lvl: L.FULL, notes: 'Auto-discovers all REST endpoints' },
      { name: 'Quarkus', versions: '2.x, 3.x', modes: ['full-stack'], lvl: L.FULL, notes: 'Native image supported' },
      { name: 'Micronaut', versions: '3.x, 4.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Django', versions: '3.2–5.x', modes: ['full-stack'], lvl: L.FULL, notes: 'ORM queries auto-traced' },
      { name: 'Flask', versions: '2.x, 3.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'FastAPI', versions: '0.95+', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Express.js', versions: '4.x, 5.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'NestJS', versions: '9.x, 10.x', modes: ['full-stack'], lvl: L.FULL, notes: 'Via Express or Fastify' },
      { name: 'ASP.NET Core', versions: '6.0–8.0', modes: ['full-stack'], lvl: L.FULL, notes: 'Kestrel + IIS' },
      { name: 'Rails', versions: '6.x, 7.x', modes: ['full-stack'], lvl: L.FULL, notes: 'ActiveRecord traced' },
      { name: 'Laravel', versions: '9.x–11.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Gin / Echo (Go)', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'gRPC', versions: 'All languages', modes: ['full-stack'], lvl: L.FULL, notes: 'Request/response auto-traced' },
      { name: 'GraphQL', versions: 'All', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'Query-level tracing via SDK' },
    ],
  },
  {
    id: 'databases', label: 'Databases & Storage', icon: '🗄',
    items: [
      { name: 'PostgreSQL', versions: '12–16', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Slow query, lock waits, vacuum' },
      { name: 'MySQL / MariaDB', versions: '5.7–8.3', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Query fingerprinting, replication lag' },
      { name: 'Oracle DB', versions: '19c, 21c', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Wait events, ASH metrics' },
      { name: 'Microsoft SQL Server', versions: '2017–2022', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: '' },
      { name: 'MongoDB', versions: '4.4–7.x', modes: ['full-stack'], lvl: L.FULL, notes: 'Query plan analysis' },
      { name: 'Redis', versions: '6.x, 7.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Command latency, keyspace' },
      { name: 'Cassandra', versions: '3.x, 4.x', modes: ['full-stack'], lvl: L.FULL, notes: 'CQL query tracing' },
      { name: 'Elasticsearch', versions: '7.x, 8.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Query, indexing metrics' },
      { name: 'ClickHouse', versions: '23.x, 24.x', modes: ['infra'], lvl: L.PARTIAL, notes: 'Native system table metrics' },
      { name: 'DynamoDB', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: 'Via AWS SDK auto-instrumentation' },
      { name: 'Cosmos DB', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: 'Via Azure SDK' },
      { name: 'SAP HANA', versions: '2.x', modes: ['infra'], lvl: L.PARTIAL, notes: 'Via extension' },
      { name: 'IBM Db2', versions: '11.5+', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'JDBC tracing' },
      { name: 'CockroachDB', versions: '23.x, 24.x', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'PostgreSQL-compatible wire protocol' },
    ],
  },
  {
    id: 'queues', label: 'Messaging & Queues', icon: '📨',
    items: [
      { name: 'Apache Kafka', versions: '2.8–3.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Consumer lag, broker metrics' },
      { name: 'RabbitMQ', versions: '3.11–3.13', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Queue depth, message rates' },
      { name: 'Apache ActiveMQ', versions: '5.x, Artemis', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'IBM MQ', versions: '9.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'AWS SQS / SNS', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: 'Via SDK auto-instrumentation' },
      { name: 'Azure Service Bus', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Google Pub/Sub', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'NATS', versions: '2.x', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'Via OTel' },
    ],
  },
  {
    id: 'webservers', label: 'Web & App Servers', icon: '🖥',
    items: [
      { name: 'Apache HTTP Server', versions: '2.4.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Request stats, worker pool' },
      { name: 'Nginx', versions: '1.20+', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Connection stats, upstream health' },
      { name: 'IIS', versions: '8.5–10.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'App pool metrics, request queue' },
      { name: 'Apache Tomcat', versions: '9.x, 10.x', modes: ['full-stack'], lvl: L.FULL, notes: 'Thread pool, session stats' },
      { name: 'WildFly / JBoss', versions: '26+, 27+', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'IBM WebSphere', versions: 'Traditional + Liberty', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Oracle WebLogic', versions: '14c', modes: ['full-stack'], lvl: L.PARTIAL, notes: '' },
      { name: 'Jetty', versions: '11.x', modes: ['full-stack'], lvl: L.FULL, notes: '' },
      { name: 'Envoy', versions: '1.28+', modes: ['infra'], lvl: L.FULL, notes: 'xDS metrics, circuit breakers' },
    ],
  },
  {
    id: 'os', label: 'Operating Systems & Platforms', icon: '💻',
    items: [
      { name: 'Linux (x86-64)', versions: 'All major distros', modes: ['full-stack', 'infra', 'discovery'], lvl: L.FULL, notes: 'Ubuntu, RHEL, Debian, Amazon Linux, Alpine (containers)' },
      { name: 'Linux (ARM64)', versions: 'AWS Graviton, RPi', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Including AWS Graviton 2/3' },
      { name: 'Windows Server', versions: '2012 R2 – 2022', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'WMI, Event Log, Perf Counters' },
      { name: 'Windows 10 / 11', versions: '10+ (non-IoT)', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: '' },
      { name: 'AIX', versions: '7.2, 7.3 (POWER8/9/10)', modes: ['infra', 'app-only'], lvl: L.FULL, notes: 'POWER architecture, WPARs not supported' },
      { name: 'Solaris', versions: '11.4 (SPARC + x86-64)', modes: ['infra', 'app-only'], lvl: L.FULL, notes: '' },
      { name: 'IBM z/OS', versions: '2.4+', modes: ['app-only'], lvl: L.PARTIAL, notes: 'CICS, IMS, IBM MQ, Db2 on z/OS' },
      { name: 'macOS', versions: '12+ (dev only)', modes: ['infra'], lvl: L.PARTIAL, notes: 'Development monitoring only' },
    ],
  },
  {
    id: 'containers', label: 'Containers & Orchestration', icon: '📦',
    items: [
      { name: 'Kubernetes', versions: '1.25–1.30', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'EKS, GKE, AKS, OpenShift, Rancher, k3s, kind' },
      { name: 'OpenShift', versions: '4.x', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'oc CLI + Operator deployment' },
      { name: 'Docker', versions: '20.10+', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'Container CPU/mem per process' },
      { name: 'Docker Compose', versions: 'v2', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: '' },
      { name: 'containerd', versions: '1.6+', modes: ['infra'], lvl: L.FULL, notes: 'CRI-level metrics' },
      { name: 'CRI-O', versions: '1.26+', modes: ['infra'], lvl: L.FULL, notes: '' },
      { name: 'Podman', versions: '4.x', modes: ['infra'], lvl: L.PARTIAL, notes: 'crun runtime only' },
      { name: 'Helm', versions: '3.x', modes: ['deploy'], lvl: L.FULL, notes: 'Official chart via observex/agent' },
      { name: 'Istio / Envoy', versions: '1.17+', modes: ['infra'], lvl: L.FULL, notes: 'mTLS, circuit breaker metrics' },
    ],
  },
  {
    id: 'serverless', label: 'Serverless & PaaS', icon: '⚡',
    items: [
      { name: 'AWS Lambda', versions: 'All runtimes', modes: ['app-only'], lvl: L.FULL, notes: 'Cold start, memory, duration, errors' },
      { name: 'Azure Functions', versions: 'v4 (.NET, Node, Python, Java)', modes: ['app-only'], lvl: L.FULL, notes: 'Durable functions supported' },
      { name: 'Google Cloud Functions', versions: '2nd gen', modes: ['app-only'], lvl: L.FULL, notes: 'Node.js, Python, Go, Java' },
      { name: 'AWS ECS / Fargate', versions: 'Latest', modes: ['infra', 'app-only'], lvl: L.FULL, notes: 'Task-level metrics, sidecar agent' },
      { name: 'Azure App Service', versions: 'Windows + Linux', modes: ['app-only'], lvl: L.FULL, notes: 'Site extension or container' },
      { name: 'Google App Engine', versions: 'Standard + Flex', modes: ['app-only'], lvl: L.PARTIAL, notes: 'Flex only (container-based)' },
      { name: 'Cloud Foundry', versions: 'CF4K8s + TAS', modes: ['app-only'], lvl: L.FULL, notes: 'Buildpack auto-injection' },
      { name: 'Heroku', versions: 'Any dyno type', modes: ['app-only'], lvl: L.FULL, notes: 'Buildpack injection' },
      { name: 'Vercel / Netlify', versions: 'Edge functions', modes: ['otlp'], lvl: L.OTLP, notes: 'Via OpenTelemetry HTTP exporter' },
    ],
  },
  {
    id: 'cloud', label: 'Cloud Platforms', icon: '☁️',
    items: [
      { name: 'AWS (full suite)', versions: 'All regions', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'EC2, RDS, S3, ELB, CloudFront, SQS, Lambda, EKS, ECS, DynamoDB...' },
      { name: 'Microsoft Azure', versions: 'All regions', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'AKS, App Service, Functions, Cosmos, SQL, Service Bus...' },
      { name: 'Google Cloud Platform', versions: 'All regions', modes: ['full-stack', 'infra'], lvl: L.FULL, notes: 'GKE, Cloud Run, BigQuery, Pub/Sub, Spanner...' },
      { name: 'IBM Cloud', versions: 'Latest', modes: ['infra'], lvl: L.PARTIAL, notes: 'IKS, IBM Cloud Object Storage' },
      { name: 'Oracle Cloud', versions: 'Latest', modes: ['infra'], lvl: L.PARTIAL, notes: 'OKE, Autonomous Database' },
      { name: 'Cloudflare Workers', versions: 'Latest', modes: ['otlp'], lvl: L.OTLP, notes: 'Via OTLP HTTP exporter' },
    ],
  },
  {
    id: 'aiml', label: 'AI / ML Platforms', icon: '🤖',
    items: [
      { name: 'OpenAI API', versions: 'All models', modes: ['full-stack'], lvl: L.FULL, notes: 'GPT-4o, o1, o3 — tokens, cost, latency, errors' },
      { name: 'Anthropic (Claude)', versions: 'claude-3/4 family', modes: ['full-stack'], lvl: L.FULL, notes: 'claude-sonnet-4-6, haiku-4-5 — live health check' },
      { name: 'Google Gemini', versions: '1.5, 2.0', modes: ['full-stack'], lvl: L.FULL, notes: 'Pro, Flash — token tracking' },
      { name: 'Mistral AI', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: 'Mistral-large, codestral' },
      { name: 'AWS Bedrock', versions: 'All models', modes: ['full-stack'], lvl: L.FULL, notes: 'Claude, Titan, Llama via Bedrock API' },
      { name: 'Azure OpenAI', versions: 'GPT-4 family', modes: ['full-stack'], lvl: L.FULL, notes: 'Private endpoint monitoring' },
      { name: 'Google Vertex AI', versions: 'Latest', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'Token usage, predictions' },
      { name: 'Ollama (local LLM)', versions: 'Latest', modes: ['full-stack'], lvl: L.FULL, notes: '★ UNIQUE — in-house agent integration, zero egress' },
      { name: 'LangChain', versions: '0.2+', modes: ['full-stack'], lvl: L.FULL, notes: 'Chain steps, retrieval, tool calls' },
      { name: 'LlamaIndex', versions: '0.10+', modes: ['full-stack'], lvl: L.PARTIAL, notes: 'Query engine tracing' },
      { name: 'NVIDIA GPU', versions: 'CUDA 11+', modes: ['infra'], lvl: L.FULL, notes: 'GPU utilization, memory, temperature, NVML' },
      { name: 'Hugging Face', versions: 'Inference API', modes: ['otlp'], lvl: L.OTLP, notes: 'Via OTLP exporter from transformers' },
    ],
  },
]

const LVL_CONFIG = {
  full: { label: 'Full Support', icon: CheckCircle, color: '#0fcf8a' },
  partial: { label: 'Partial', icon: AlertCircle, color: '#f5a623' },
  otlp: { label: 'Via OTel', icon: Circle, color: '#3d9bff' },
  no: { label: 'Not supported', icon: Circle, color: '#546a88' },
}

export default function TechMatrixPage() {
  const [activeGroup, setActiveGroup] = useState('languages')
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<Level | 'all'>('all')

  const group = GROUPS.find(g => g.id === activeGroup)!
  const items = group.items.filter(item => {
    const matchSearch = !search || item.name.toLowerCase().includes(search.toLowerCase()) || item.notes.toLowerCase().includes(search.toLowerCase())
    const matchFilter = filter === 'all' || item.lvl === filter
    return matchSearch && matchFilter
  })

  const totalFull = group.items.filter(i => i.lvl === 'full').length
  const totalItems = group.items.length

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 0, background: '#070f1e', minHeight: '100%', color: '#c8d8ef' }}>
      {/* Header */}
      <div style={{ padding: '18px 20px 12px', borderBottom: '1px solid #1a2d4a' }}>
        <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 4 }}>Technology Coverage Matrix</div>
        <div style={{ fontSize: 12, color: '#546a88', marginBottom: 14 }}>
          Every language, framework, database, and platform ObserveX Agent supports — 800+ technologies
        </div>
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 8, padding: '6px 12px', flex: 1, maxWidth: 280 }}>
            <Search size={13} color="#546a88" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search technology..."
              style={{ background: 'none', border: 'none', outline: 'none', fontSize: 12, color: '#c8d8ef', flex: 1 }} />
          </div>
          {(['all', 'full', 'partial', 'otlp'] as const).map(f => (
            <button key={f} onClick={() => setFilter(f)}
              style={{ padding: '6px 14px', border: `1px solid ${filter === f ? '#6c72ff' : '#1a2d4a'}`, borderRadius: 20, background: filter === f ? 'rgba(108,114,255,.15)' : 'transparent', color: filter === f ? '#8b90ff' : '#546a88', fontSize: 11, fontWeight: 600, cursor: 'pointer' }}>
              {f === 'all' ? 'All' : LVL_CONFIG[f].label}
            </button>
          ))}
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Category sidebar */}
        <div style={{ width: 200, borderRight: '1px solid #1a2d4a', overflowY: 'auto', flexShrink: 0 }}>
          {GROUPS.map(g => {
            const fullCount = g.items.filter(i => i.lvl === 'full').length
            return (
              <div key={g.id} onClick={() => setActiveGroup(g.id)}
                style={{ padding: '12px 14px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e', borderLeft: `3px solid ${activeGroup === g.id ? '#6c72ff' : 'transparent'}`, background: activeGroup === g.id ? 'rgba(108,114,255,.08)' : 'transparent' }}>
                <div style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 3 }}>
                  <span>{g.icon}</span>
                  <span style={{ fontSize: 11, fontWeight: 600, color: activeGroup === g.id ? '#8b90ff' : '#c8d8ef' }}>{g.label}</span>
                </div>
                <div style={{ fontSize: 10, color: '#546a88' }}>{fullCount}/{g.items.length} fully supported</div>
              </div>
            )
          })}
        </div>

        {/* Matrix table */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
            <span style={{ fontSize: 20 }}>{group.icon}</span>
            <div>
              <div style={{ fontSize: 16, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>{group.label}</div>
              <div style={{ fontSize: 11, color: '#546a88' }}>{totalFull}/{totalItems} fully supported · {totalItems - totalFull} partial or via OpenTelemetry</div>
            </div>
          </div>

          <div style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 14, overflow: 'hidden' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #1a2d4a' }}>
                  <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>Technology</th>
                  <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>Versions</th>
                  <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>Support Level</th>
                  <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>Monitoring Modes</th>
                  <th style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>Notes</th>
                </tr>
              </thead>
              <tbody>
                {items.map((item, i) => {
                  const cfg = LVL_CONFIG[item.lvl]
                  const Icon = cfg.icon
                  return (
                    <tr key={item.name} style={{ borderBottom: '1px solid rgba(26,45,74,.5)' }}>
                      <td style={{ padding: '10px 16px', fontWeight: 600, color: '#f0f6ff' }}>
                        {item.name}
                        {item.notes.includes('★ UNIQUE') && (
                          <span style={{ marginLeft: 6, fontSize: 9, background: 'rgba(108,114,255,.2)', color: '#8b90ff', padding: '1px 6px', borderRadius: 8 }}>★ UNIQUE</span>
                        )}
                      </td>
                      <td style={{ padding: '10px 16px', color: '#8fa8cc', fontFamily: 'monospace', fontSize: 11 }}>{item.versions}</td>
                      <td style={{ padding: '10px 16px' }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                          <Icon size={13} style={{ color: cfg.color }} />
                          <span style={{ color: cfg.color, fontWeight: 600, fontSize: 11 }}>{cfg.label}</span>
                        </div>
                      </td>
                      <td style={{ padding: '10px 16px' }}>
                        <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                          {item.modes.map(m => (
                            <span key={m} style={{ background: '#182844', color: '#8fa8cc', fontSize: 9, padding: '2px 7px', borderRadius: 10, fontWeight: 600 }}>{m}</span>
                          ))}
                        </div>
                      </td>
                      <td style={{ padding: '10px 16px', color: '#546a88', fontSize: 11 }}>{item.notes.replace('★ UNIQUE — ', '')}</td>
                    </tr>
                  )
                })}
                {items.length === 0 && (
                  <tr><td colSpan={5} style={{ padding: 32, textAlign: 'center', color: '#546a88' }}>No results matching "{search}"</td></tr>
                )}
              </tbody>
            </table>
          </div>

          {/* Legend */}
          <div style={{ display: 'flex', gap: 20, marginTop: 14, fontSize: 11, color: '#546a88' }}>
            {Object.entries(LVL_CONFIG).filter(([k]) => k !== 'no').map(([k, v]) => {
              const Icon = v.icon
              return (
                <span key={k} style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                  <Icon size={12} style={{ color: v.color }} /> {v.label}
                </span>
              )
            })}
            <span style={{ marginLeft: 'auto' }}>
              <a href="https://docs.observex.io/tech-matrix" style={{ color: '#6c72ff', textDecoration: 'none', fontSize: 11 }}>Full documentation ↗</a>
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
