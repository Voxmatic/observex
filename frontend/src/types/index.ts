// src/types/index.ts — shared types matching backend Go models exactly

export type Role = 'admin' | 'editor' | 'viewer'
export type OrgPlan = 'free' | 'pro' | 'enterprise'
export type AccessLevel = 'read' | 'write' | 'admin'

export interface User {
  id: string
  email: string
  name: string
  role: Role
  team_id?: string
  avatar_url?: string
  is_active: boolean
  last_login_at?: string
  created_at: string
}

export interface Org {
  id: string
  name: string
  display_name: string
  plan: OrgPlan
  max_users: number
  max_teams: number
  max_dashboards: number
  owner_id: string
  settings: Record<string, unknown>
  is_active: boolean
  user_count?: number
  team_count?: number
}

export interface AuthContext {
  user: User
  org: Org
  teams: string[]
  namespace_access: Record<string, AccessLevel>
  effective_role: Role
}

// ── Services / Topology ──────────────────────────────────────────────────────

export type HealthState = 'GOOD' | 'DEGRADED' | 'CRITICAL' | 'UNKNOWN'
export type ServiceKind = 'Deployment' | 'StatefulSet' | 'DaemonSet' | 'Service' | 'Database' | 'Cache' | 'Queue' | 'External'

export interface Service {
  id: string
  name: string
  display_name: string
  kind: ServiceKind
  namespace: string
  cluster_name: string
  node_name?: string
  pod_name?: string
  deployment?: string
  labels?: Record<string, string>
  tech_stack?: string[]
  health: { state: HealthState; score: number; message?: string }
  last_seen_at: string
  discovered_at: string
  version?: string
  endpoints?: Array<{ address: string; port: number; protocol: string }>
}

export interface TopoEdge {
  id: string
  source_id: string
  target_id: string
  protocol: string
  calls_per_min: number
  avg_latency_ms: number
  error_rate: number
  bytes_per_sec: number
  updated_at: string
}

// ── Problems / Incidents ─────────────────────────────────────────────────────

export type Severity = 'CRITICAL' | 'HIGH' | 'MEDIUM' | 'LOW'
export type ProblemStatus = 'OPEN' | 'ACKNOWLEDGED' | 'RESOLVED'

export interface Problem {
  id: string
  title: string
  description: string
  severity: Severity
  status: ProblemStatus
  service_name: string
  service_id: string
  namespace: string
  cluster_name: string
  detected_at: string
  acknowledged_at?: string
  resolved_at?: string
  root_cause?: string
  affected_services: string[]
  metric_value?: number
  threshold?: number
  labels?: Record<string, string>
}

export interface Remediation {
  id: string
  problem_id: string
  action_type: string
  status: 'pending' | 'executing' | 'completed' | 'failed' | 'rejected'
  description: string
  dry_run: boolean
  executed_at?: string
  result?: string
  error?: string
}

// ── Metrics ──────────────────────────────────────────────────────────────────

export type MetricPoint = [number, string]  // [unix_timestamp, value_string] — VictoriaMetrics format
export interface MetricPointObj { t: number; v: number }
export interface MetricSeries { metric: Record<string, string>; values: MetricPoint[] }
export interface QueryRangeResult { resultType: string; result: MetricSeries[] }

// ── Logs ─────────────────────────────────────────────────────────────────────

export interface LogLine { ts: string; labels: Record<string, string>; line: string }
export interface LogStream { stream: Record<string, string>; values: [string, string][] }

// ── Traces ───────────────────────────────────────────────────────────────────

export interface TraceResult {
  traceID: string
  rootServiceName: string
  rootTraceName: string
  startTimeUnixNano: string
  durationMs: number
  spanCount: number
}

export interface Span {
  traceID: string
  spanID: string
  parentSpanID?: string
  operationName: string
  serviceName: string
  startTimeUnixNano: string
  durationNanos: number
  statusCode: number
  attributes: Record<string, unknown>
}

// ── Dashboards ───────────────────────────────────────────────────────────────

export type WidgetType = 'timeseries' | 'stat' | 'table' | 'heatmap' | 'topology' | 'alertlist' | 'slo' | 'log' | 'text'
export type DataSource = 'metrics' | 'logs' | 'traces' | 'topology' | 'problems' | 'slos' | 'synthetic'

export interface Widget {
  id: string
  type: WidgetType
  title: string
  data_source: DataSource
  query?: string
  time_range?: string
  x: number; y: number; w: number; h: number
  options?: Record<string, unknown>
}

export interface Dashboard {
  id: string
  name: string
  description: string
  owner_id: string
  team_id?: string
  tags: string[]
  widgets: Widget[]
  variables: DashboardVariable[]
  time_range: string
  auto_refresh_sec: number
  is_template: boolean
  is_public: boolean
  created_at: string
  updated_at: string
}

export interface DashboardVariable {
  name: string
  label: string
  type: 'query' | 'constant' | 'interval' | 'custom'
  query?: string
  options?: string[]
  current?: string
}

// ── SLOs ─────────────────────────────────────────────────────────────────────

export type SLOKind = 'availability' | 'latency' | 'error_rate' | 'throughput'

export interface SLO {
  id: string
  name: string
  description: string
  service_name: string
  namespace: string
  kind: SLOKind
  target: number
  window: '7d' | '30d' | '90d'
  sli_query: string
  sli?: number
  budget_left?: number
  status?: 'OK' | 'WARN' | 'BREACHED'
  burn_rate_1h?: number
  burn_rate_6h?: number
  burn_rate_24h?: number
  created_at: string
}

// ── Alert Rules ───────────────────────────────────────────────────────────────

export interface AlertRule {
  id: string
  name: string
  description: string
  service_name?: string
  namespace?: string
  expr: string
  threshold: number
  operator: 'gt' | 'lt' | 'gte' | 'lte' | 'eq'
  duration: string
  severity: Severity
  message: string
  labels?: Record<string, string>
  channels: string[]
  silenced: boolean
  runbook_url?: string
  created_at: string
}

// ── Synthetic ────────────────────────────────────────────────────────────────

export type CheckStatus = 'UP' | 'DOWN' | 'DEGRADED' | 'UNKNOWN'
export type CheckType = 'http' | 'tcp' | 'dns' | 'ssl' | 'ping' | 'grpc' | 'websocket' | 'multi_step'

export interface CheckState {
  check_id: string
  check_name: string
  status: CheckStatus
  last_checked: string
  last_duration_ms: number
  consecutive_fails: number
  consecutive_oks: number
  uptime_24h?: number
  uptime_7d?: number
  uptime_30d?: number
}

// ── Profiling ────────────────────────────────────────────────────────────────

export interface ProfileMeta {
  id: string
  service_name: string
  profile_type: 'cpu' | 'heap' | 'goroutine' | 'block' | 'mutex'
  started_at: string
  duration_ns: number
  labels: Record<string, string>
}

// ── API responses ────────────────────────────────────────────────────────────

export interface PagedResponse<T> { data: T[]; total: number; limit: number; offset: number }
export interface APIError { error: string; code?: string }

// ── Database monitoring types ─────────────────────────────────────────────────

export interface DBHealth {
  database: string
  status: 'healthy' | 'warning' | 'critical'
  conn_total: number
  conn_active: number
  conn_idle: number
  conn_max_use_pct: number
  tps: number
  cache_hit_pct: number
  slow_queries_1m: number
  deadlocks: number
  replication_lag_s: number
  uptime_hours: number
  version: string
}

export interface SlowQuery {
  query_id: string
  query: string
  normalized: string
  calls: number
  total_time_ms: number
  avg_time_ms: number
  min_time_ms: number
  max_time_ms: number
  rows: number
  cache_hit_pct: number
  database: string
  user: string
}

export interface TableStat {
  schema: string
  table: string
  row_count: number
  dead_tuples: number
  live_tuples: number
  bloat_pct: number
  last_vacuum: string
  last_analyze: string
  index_scans: number
  seq_scans: number
  size_mb: number
}

// ── Kubernetes types ──────────────────────────────────────────────────────────

export interface K8sPod {
  id: string
  name: string
  namespace: string
  node: string
  deployment: string
  status: 'Running' | 'Pending' | 'Failed' | 'Unknown'
  restarts: number
  cpu_pct: number
  mem_mb: number
  labels: Record<string, string>
  tech_stack: string[]
}

export interface K8sEvent {
  type: 'Normal' | 'Warning'
  reason: string
  message: string
  object: string
  namespace: string
  count: number
  age: string
}

// ── Node / Infrastructure types ───────────────────────────────────────────────

export interface NodeInfo {
  name: string
  cpu_usage_pct: number
  mem_total_gb: number
  mem_used_gb: number
  mem_used_pct: number
  disk_read_iops: number
  disk_write_iops: number
  net_rx_mbps: number
  net_tx_mbps: number
  disk_total_gb: number
  disk_used_pct: number
  status: 'healthy' | 'warning' | 'critical'
}

// ── APM types ─────────────────────────────────────────────────────────────────

export interface APMService {
  id: string
  name: string
  namespace: string
  apdex: number
  rpm: number
  p50_ms: number
  p99_ms: number
  error_rate_pct: number
  throughput_rps: number
  status: 'good' | 'degraded' | 'critical'
}

export interface APMTransaction {
  endpoint: string
  service_name: string
  rpm: number
  p50_ms: number
  p99_ms: number
  error_pct: number
  throughput_rps: number
}

// ── Network flow types ────────────────────────────────────────────────────────

export interface NetworkFlow {
  src_service: string
  dst_service: string
  protocol: string
  bytes_per_sec: number
  packets_per_sec: number
  latency_ms: number
  error_rate: number
  established_conns: number
}

// ── Integration types ─────────────────────────────────────────────────────────

export interface Integration {
  id: string
  type: string
  config: Record<string, string>
  created_at: string
  updated_at: string
}

// ── Cost types ────────────────────────────────────────────────────────────────

export interface ServiceCost {
  service_id: string
  service_name: string
  namespace: string
  cpu_cores: number
  mem_gb: number
  cost_per_hour: number
  cost_per_day: number
  cost_per_month: number
}

export interface CostReport {
  services: ServiceCost[]
  total_per_hour: number
  total_per_day: number
  total_per_month: number
  price_per_core_hour: number
  price_per_gb_hour: number
  currency: string
}

// ── Postmortem types ──────────────────────────────────────────────────────────

export interface Postmortem {
  id: string
  incident_id: string
  title: string
  severity: string
  status: 'draft' | 'review' | 'published'
  detected_at: string
  resolved_at: string
  duration: string
  impact: string
  summary: string
  timeline: { time: string; event: string; who: string }[]
  root_cause: string
  whys: { level: number; why: string }[]
  action_items: { id: string; what: string; who: string; by: string; done: boolean }[]
  learnings: string
}
