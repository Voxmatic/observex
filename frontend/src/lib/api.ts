/// <reference types="vite/client" />
// src/lib/api.ts — typed API client wired to the ObserveX backend
import axios, { AxiosInstance, AxiosError } from 'axios'
import type {
  User, AuthContext, Org, Service, TopoEdge, Problem, Remediation,
  Dashboard, SLO, AlertRule, CheckState,
  QueryRangeResult, LogStream, TraceResult, PagedResponse,
  Role, AccessLevel,
} from '@/types'

// ── Extra types not in the main types file ────────────────────────────────────

export interface NamespacePermission {
  id: string; org_id: string; team_id: string; team_name?: string
  namespace: string; access_level: AccessLevel; granted_by: string; granted_at: string
}
export interface Invitation {
  id: string; org_id: string; org_name?: string; team_id?: string; team_name?: string
  email: string; role: Role; invited_by: string; inviter_name?: string
  accepted_at?: string; expires_at: string; created_at: string
  is_expired: boolean; is_pending: boolean
}
export interface Team {
  id: string; name: string; description: string; namespaces: string[]
  org_id?: string; created_at: string; updated_at: string
}
export interface TeamMember {
  team_id: string; user_id: string; user_name?: string; email?: string
  role: Role; joined_at: string
}
export interface APIKey {
  id: string; user_id: string; name: string; key_prefix: string
  role: Role; expires_at?: string; last_used_at?: string
  created_at: string; revoked: boolean
}
export interface AuditEntry {
  id: string; org_id: string; user_id: string; user_email: string
  action: string; resource: string; resource_id: string
  ip_address: string; user_agent: string
  details?: Record<string, unknown>; created_at: string
}
export interface ObserveXAgent {
  id: string; org_id?: string; node_name: string; cluster_name: string
  environment?: string; host_group?: string; network_zone?: string
  monitoring_mode?: string; collection_mode?: string; log_monitoring?: boolean
  auto_update?: boolean; update_channel?: string; update_state?: string; target_version?: string; version: string
  ip_address?: string; os?: string; arch?: string; kernel_version?: string
  ebpf_enabled?: boolean; capabilities?: Record<string, boolean>; module_status?: Record<string, string>
  status: string; last_seen?: string; registered_at?: string
}
export interface AgentInstallTokenResponse {
  token: string; tenant_url: string; ingestor_url: string
  environment_id: string; org_id: string; expires_at: string
  expires_in_sec: number; download_base_url: string; activegate_url?: string
  scopes: string[]; installer_endpoints: Record<string, string>
}

const BASE = import.meta.env.VITE_API_URL || ''

const http: AxiosInstance = axios.create({
  baseURL: BASE,
  timeout: 30_000,
  withCredentials: true,
})

// Inject JWT from localStorage on every request
http.interceptors.request.use(cfg => {
  const token = localStorage.getItem('observex_token')
  if (token) cfg.headers.Authorization = `Bearer ${token}`
  return cfg
})

// Redirect to login on 401
http.interceptors.response.use(
  r => r,
  (err: AxiosError) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('observex_token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

// ── Auth ─────────────────────────────────────────────────────────────────────

export const auth = {
  login: (email: string, password: string) =>
    http.post<{ token: string; expires_at: string; user: User }>('/api/auth/login', { email, password }).then(r => r.data),

  logout: () => http.post('/api/auth/logout'),

  me: () => http.get<AuthContext>('/api/auth/me').then(r => r.data),

  changePassword: (current: string, next: string) =>
    http.put('/api/auth/password', { current_password: current, new_password: next }),

  forgotPassword: (email: string) =>
    http.post('/api/auth/forgot-password', { email }),

  resetPassword: (token: string, new_password: string) =>
    http.post('/api/auth/reset-password', { token, new_password }),

  getInvitation: (token: string) =>
    http.get<Invitation>(`/api/auth/invite/${token}`).then(r => r.data),

  acceptInvitation: (token: string, payload: object) =>
    http.post(`/api/auth/invite/${token}/accept`, payload),
}

// ── Org ──────────────────────────────────────────────────────────────────────

export const agents = {
  list: () =>
    http.get<{ agents: ObserveXAgent[] }>('/api/v1/agents').then(r => r.data),

  installToken: () =>
    http.post<AgentInstallTokenResponse>('/api/v1/agent/install-token').then(r => r.data),

  installScript: (platform: 'linux' | 'windows' | 'kubernetes' | 'docker' | 'helm') =>
    http.get<string>(`/api/v1/agent/install/${platform}`, { responseType: 'text' }).then(r => r.data),

  installerChecksum: (os: 'unix' | 'windows', flavor = 'default') =>
    http.get<string>(`/api/v1/deployment/installer/agent/${os}/${flavor}/latest/checksum`, { responseType: 'text' }).then(r => r.data),

  status: () =>
    http.get<{ active_agents: number; org_id: string; checked_at: string }>('/api/v1/agent/status').then(r => r.data),

  versions: () =>
    http.get<{ latest: string; versions: Array<{ version: string; released: string; notes: string }>; platforms: string[] }>('/api/v1/agent/versions').then(r => r.data),
}

export const org = {
  get:            ()                         => http.get<Org>('/api/v1/org').then(r => r.data),
  update:         (body: Partial<Org>)       => http.put<Org>('/api/v1/org', body).then(r => r.data),
  listNsPerms:    ()                         => http.get<{ permissions: NamespacePermission[] }>('/api/v1/org/namespaces').then(r => r.data),
  grantNs:        (body: object)             => http.post('/api/v1/org/namespaces', body),
  revokeNs:       (team: string, ns: string) => http.delete(`/api/v1/org/namespaces/${team}/${ns}`),
  listInvitations:()                         => http.get<{ invitations: Invitation[] }>('/api/v1/invitations').then(r => r.data),
  sendInvitation: (body: object)             => http.post('/api/v1/invitations', body),
  cancelInvitation:(id: string)              => http.delete(`/api/v1/invitations/${id}`),
}

// ── Users ────────────────────────────────────────────────────────────────────

export const users = {
  list:   (params?: object) => http.get<{ users: User[]; total: number }>('/api/v1/users', { params }).then(r => r.data),
  get:    (id: string)      => http.get<User>(`/api/v1/users/${id}`).then(r => r.data),
  create: (body: object)    => http.post<User>('/api/v1/users', body).then(r => r.data),
  update: (id: string, body: object) => http.put<User>(`/api/v1/users/${id}`, body).then(r => r.data),
  delete: (id: string)      => http.delete(`/api/v1/users/${id}`),
}

// ── Teams ────────────────────────────────────────────────────────────────────

export const teams = {
  list:         ()                         => http.get<{ teams: Team[] }>('/api/v1/teams').then(r => r.data),
  get:          (id: string)               => http.get<{ team: Team; members: TeamMember[] }>(`/api/v1/teams/${id}`).then(r => r.data),
  create:       (body: object)             => http.post<Team>('/api/v1/teams', body).then(r => r.data),
  update:       (id: string, body: object) => http.put<Team>(`/api/v1/teams/${id}`, body).then(r => r.data),
  delete:       (id: string)               => http.delete(`/api/v1/teams/${id}`),
  listMembers:  (id: string)               => http.get<{ members: TeamMember[] }>(`/api/v1/teams/${id}/members`).then(r => r.data),
  addMember:    (id: string, body: object) => http.post(`/api/v1/teams/${id}/members`, body),
  updateRole:   (teamId: string, userId: string, role: string) => http.put(`/api/v1/teams/${teamId}/members/${userId}/role`, { role }),
  removeMember: (teamId: string, userId: string) => http.delete(`/api/v1/teams/${teamId}/members/${userId}`),
}

// ── API Keys ─────────────────────────────────────────────────────────────────

export const apiKeys = {
  list:   ()              => http.get<{ api_keys: APIKey[] }>('/api/v1/apikeys').then(r => r.data),
  create: (body: object)  => http.post<{ api_key: APIKey; key: string }>('/api/v1/apikeys', body).then(r => r.data),
  revoke: (id: string)    => http.delete(`/api/v1/apikeys/${id}`),
}

// ── Services / Topology ──────────────────────────────────────────────────────

export const topology = {
  services:    (params?: object) => http.get<{ services: Service[] }>('/api/v1/services', { params }).then(r => r.data),
  service:     (id: string)      => http.get<Service>(`/api/v1/services/${id}`).then(r => r.data),
  graph:       (params?: object) => http.get<{ nodes: Service[]; edges: TopoEdge[] }>('/api/v1/topology', { params }).then(r => r.data),
  subgraph:    (id: string)      => http.get<{ nodes: Service[]; edges: TopoEdge[] }>(`/api/v1/topology/service/${id}/subgraph`).then(r => r.data),
}

// ── Problems ──────────────────────────────────────────────────────────────────

export const problems = {
  list:        (params?: object) => http.get<{ problems: Problem[] }>('/api/v1/problems', { params }).then(r => r.data),
  get:         (id: string)      => http.get<Problem>(`/api/v1/problems/${id}`).then(r => r.data),
  acknowledge: (id: string)      => http.post(`/api/v1/problems/${id}/acknowledge`),
  resolve:     (id: string)      => http.post(`/api/v1/problems/${id}/resolve`),
  remediations:()                => http.get<{ remediations: Remediation[] }>('/api/v1/remediations').then(r => r.data),
  approve:     (id: string)      => http.post(`/api/v1/remediations/${id}/approve`),
  reject:      (id: string)      => http.post(`/api/v1/remediations/${id}/reject`),
}

// ── Metrics ───────────────────────────────────────────────────────────────────

export const metrics = {
  query:      (query: string, time?: number) =>
    http.get('/api/v1/metrics/query', { params: { query, time } }).then(r => r.data),
  queryRange: (query: string, start: number, end: number, step = '15s') =>
    http.get<{ data: QueryRangeResult }>('/api/v1/metrics/query_range', { params: { query, start, end, step } }).then(r => r.data.data),
  labels:     ()                   => http.get('/api/v1/metrics/labels').then(r => r.data),
  series:     (match: string)      => http.get('/api/v1/metrics/series', { params: { 'match[]': match } }).then(r => r.data),
}

// ── Logs ──────────────────────────────────────────────────────────────────────

export const logs = {
  queryRange: (query: string, start: number, end: number, limit = 1000) =>
    http.get<{ data: { result: LogStream[] } }>('/api/v1/logs/query_range', { params: { query, start, end, limit } }).then(r => r.data.data.result),
  labels:     () => http.get('/api/v1/logs/labels').then(r => r.data),
}

// ── Traces ────────────────────────────────────────────────────────────────────

export const traces = {
  search:   (params: object)  => http.get<{ traces: TraceResult[] }>('/api/v1/traces/search', { params }).then(r => r.data),
  get:      (id: string)      => http.get<{ batches: unknown[] }>(`/api/v1/traces/${id}`).then(r => r.data),
  services: ()                => http.get<{ services: string[] }>('/api/v1/traces/services').then(r => r.data),
}

// ── Dashboards ────────────────────────────────────────────────────────────────

export const dashboards = {
  list:      (params?: object)             => http.get<{ dashboards: Dashboard[]; total: number }>('/api/v1/dashboards', { params }).then(r => r.data),
  templates: ()                            => http.get<{ templates: Dashboard[] }>('/api/v1/dashboards/templates').then(r => r.data),
  get:       (id: string)                  => http.get<Dashboard>(`/api/v1/dashboards/${id}`).then(r => r.data),
  create:    (body: Partial<Dashboard>)    => http.post<Dashboard>('/api/v1/dashboards', body).then(r => r.data),
  update:    (id: string, body: Partial<Dashboard>) => http.put<Dashboard>(`/api/v1/dashboards/${id}`, body).then(r => r.data),
  delete:    (id: string)                  => http.delete(`/api/v1/dashboards/${id}`),
  clone:     (id: string, name?: string)   => http.post<Dashboard>(`/api/v1/dashboards/templates/${id}/clone`, { name }).then(r => r.data),
}

// ── SLOs ──────────────────────────────────────────────────────────────────────

export const slos = {
  list:   (params?: object)        => http.get<{ slos: SLO[]; total: number }>('/api/v1/slos', { params }).then(r => r.data),
  get:    (id: string)             => http.get<SLO>(`/api/v1/slos/${id}`).then(r => r.data),
  create: (body: Partial<SLO>)     => http.post<SLO>('/api/v1/slos', body).then(r => r.data),
  update: (id: string, body: Partial<SLO>) => http.put<SLO>(`/api/v1/slos/${id}`, body).then(r => r.data),
  delete: (id: string)             => http.delete(`/api/v1/slos/${id}`),
}

// ── Alert Rules ───────────────────────────────────────────────────────────────

export const alertRules = {
  list:    (params?: object)              => http.get<{ rules: AlertRule[]; total: number }>('/api/v1/alerts/rules', { params }).then(r => r.data),
  get:     (id: string)                   => http.get<AlertRule>(`/api/v1/alerts/rules/${id}`).then(r => r.data),
  create:  (body: Partial<AlertRule>)     => http.post<AlertRule>('/api/v1/alerts/rules', body).then(r => r.data),
  update:  (id: string, body: Partial<AlertRule>) => http.put<AlertRule>(`/api/v1/alerts/rules/${id}`, body).then(r => r.data),
  delete:  (id: string)                   => http.delete(`/api/v1/alerts/rules/${id}`),
  silence: (id: string, silenced: boolean) => http.post(`/api/v1/alerts/rules/${id}/silence`, { silenced }),
}

// ── Audit ─────────────────────────────────────────────────────────────────────

export const audit = {
  list: (params?: object) => http.get<{ entries: AuditEntry[]; total: number }>('/api/v1/audit', { params }).then(r => r.data),
}

// ── Synthetic ─────────────────────────────────────────────────────────────────

export interface SyntheticCheck {
  id: string; org_id: string; name: string; type: string
  target: string; interval_sec: number; timeout_sec: number
  locations: string[]; enabled: boolean
  expect_status?: number; expect_body_contains?: string
  namespace: string; created_at: string
}

export const synthetic = {
  checks:      () => http.get<{ checks: SyntheticCheck[]; total: number }>('/api/v1/synthetic/checks').then(r => r.data),
  createCheck: (d: Partial<SyntheticCheck>) => http.post('/api/v1/synthetic/checks', d).then(r => r.data),
  updateCheck: (id: string, d: Partial<SyntheticCheck>) => http.put(`/api/v1/synthetic/checks/${id}`, d).then(r => r.data),
  deleteCheck: (id: string) => http.delete(`/api/v1/synthetic/checks/${id}`),
  states:      () => http.get<{ states: CheckState[]; total: number }>('/api/v1/synthetic/states').then(r => r.data),
  results:     (checkId: string, limit = 90) => http.get(`/api/v1/synthetic/results`, { params: { check_id: checkId, limit } }).then(r => r.data),
  runNow:      (id: string) => http.post(`/api/v1/synthetic/checks/${id}/run`).then(r => r.data),
}

// ── Deployments ───────────────────────────────────────────────────────────────

export interface DeploymentMarker {
  id: string
  service_name: string
  service_id?: string
  namespace?: string
  cluster_name?: string
  version: string
  prev_version?: string
  deployed_at: string
  deployed_by?: string
  environment: string
  notes?: string
  status: 'pending' | 'ok' | 'regression' | 'improved'
  analysed_at?: string
  p99_latency_before_ms?: number
  p99_latency_after_ms?: number
  p99_latency_delta_pct?: number
  error_rate_before_pct?: number
  error_rate_after_pct?: number
  error_rate_delta_pct?: number
  problem_id?: string
  created_at: string
  updated_at: string
}

export const deployments = {
  list:   (params?: object)                    => http.get<{ deployments: DeploymentMarker[]; total: number }>('/api/v1/deployments', { params }).then(r => r.data),
  get:    (id: string)                         => http.get<DeploymentMarker>(`/api/v1/deployments/${id}`).then(r => r.data),
  create: (body: Partial<DeploymentMarker>)    => http.post<DeploymentMarker>('/api/v1/deployments', body).then(r => r.data),
}

// ── Profiling ─────────────────────────────────────────────────────────────────

export interface FlameNode {
  name: string
  full_name: string
  value: number
  self_value?: number
  children?: FlameNode[]
  diff_value?: number
}

export interface FlameGraph {
  service_name: string
  profile_type: string
  total_value: number
  total_samples?: number
  unit?: string
  root: FlameNode
}

export interface ProfileMeta {
  id: string
  service_name: string
  profile_type: string
  started_at: string
  duration_ns: number
  labels: Record<string, string>
}

export const profiling = {
  services: () =>
    http.get<{ services: string[] }>('/api/v1/profiling/services').then(r => r.data),
  list: (params?: object) =>
    http.get<{ profiles: ProfileMeta[]; total: number }>('/api/v1/profiling/profiles', { params }).then(r => r.data),
  flamegraph: (params: { service_name?: string; profile_type?: string; window?: string }) =>
    http.get<FlameGraph>('/api/v1/profiling/flamegraph', { params }).then(r => r.data),
  flamegraphDiff: (params: { base_id: string; cmp_id: string }) =>
    http.get<FlameGraph>('/api/v1/profiling/flamegraph/diff', { params }).then(r => r.data),
  topFunctions: (params?: object) =>
    http.get<{ functions: any[] }>('/api/v1/profiling/topfunctions', { params }).then(r => r.data),
}


// ── Forecasting (S21) ──────────────────────────────────────────────────────────

export interface ForecastPoint {
  offset_min: number
  value: number
  lower_95: number
  upper_95: number
}

export interface ServiceForecast {
  service_id: string
  service_name: string
  metric: string
  unit: string
  method: 'linear' | 'holt_winters' | 'seasonal'
  horizon: string
  points: ForecastPoint[]
  trend: 'rising' | 'falling' | 'stable'
  trend_pct: number
  alert?: 'warning' | 'critical'
  suggestions?: string[]
  sample_count: number
  generated_at: string
}

export interface CapacityForecast {
  cluster_name: string
  generated_at: string
  horizon_min: number
  forecasts: ServiceForecast[]
  at_risk_services: string[]
  healthy_count: number
  warning_count: number
  critical_count: number
}

export const forecast = {
  metric:   (service: string, metric: string, horizon = '30m') =>
    http.get<ServiceForecast>('/api/v1/forecasts', { params: { service, metric, horizon } }).then(r => r.data),
  capacity: (horizon = '30m') =>
    http.get<CapacityForecast>('/api/v1/forecasts/capacity', { params: { horizon } }).then(r => r.data),
}

// ── Compliance & export (S24, S34) ────────────────────────────────────────────

export const compliance = {
  report: (days = 90, format: 'json' | 'csv' = 'json') =>
    http.get('/api/v1/compliance/report', { params: { days, format } }).then(r => r.data),
  exportMetrics: (service: string, metric: string, start: number, end: number) =>
    http.get('/api/v1/export/metrics', { params: { service, metric, start, end } }).then(r => r.data),
  exportLogs: (service: string, start: number, end: number, limit = 5000) =>
    http.get('/api/v1/export/logs', { params: { service, start, end, limit } }).then(r => r.data),
}




// ── Kubernetes monitoring ──────────────────────────────────────────────────────

export interface K8sPod {
  id: string; name: string; namespace: string; node: string
  deployment: string; status: string; restarts: number
  cpu_pct: number; mem_mb: number; labels: Record<string,string>
  tech_stack: string[]
}
export interface K8sEvent {
  type: string; reason: string; message: string
  object: string; namespace: string; count: number; age: string
}
export const kubernetes = {
  overview: () => http.get('/api/v1/kubernetes/overview').then(r => r.data),
  pods:     (params?: {namespace?:string;state?:string;limit?:number}) =>
    http.get('/api/v1/kubernetes/pods', { params }).then(r => r.data),
  events:   () => http.get('/api/v1/kubernetes/events').then(r => r.data),
}

// ── Database monitoring ────────────────────────────────────────────────────────

export interface DBHealth {
  database: string; status: string; conn_total: number; conn_active: number
  conn_idle: number; conn_max_use_pct: number; tps: number; cache_hit_pct: number
  slow_queries_1m: number; deadlocks: number; replication_lag_s: number
  uptime_hours: number; version: string
}
export interface SlowQuery {
  query_id: string; query: string; calls: number; total_time_ms: number
  avg_time_ms: number; min_time_ms: number; max_time_ms: number
  rows: number; cache_hit_pct: number; database: string; normalized: string
}
export interface TableStat {
  schema: string; table: string; row_count: number; dead_tuples: number
  bloat_pct: number; last_vacuum: string; last_analyze: string
  index_scans: number; seq_scans: number; size_mb: number
}
export const databases = {
  list:        () => http.get('/api/v1/databases').then(r => r.data),
  queries:     (db: string, params?: {min_ms?:number;limit?:number}) =>
    http.get(`/api/v1/databases/${db}/queries`, { params }).then(r => r.data),
  connections: (db: string) => http.get(`/api/v1/databases/${db}/connections`).then(r => r.data),
  tables:      (db: string) => http.get(`/api/v1/databases/${db}/tables`).then(r => r.data),
  activity:    () => http.get('/api/v1/databases/activity').then(r => r.data),
}

// ── Network monitoring ────────────────────────────────────────────────────────
export const network = {
  flows:    (params?: {namespace?:string}) =>
    http.get('/api/v1/network/flows', { params }).then(r => r.data),
  topology: () => http.get('/api/v1/network/topology').then(r => r.data),
}

// ── Integrations ──────────────────────────────────────────────────────────────
export const integrations = {
  list:   () => http.get('/api/v1/integrations').then(r => r.data),
  create: (d: any) => http.post('/api/v1/integrations', d).then(r => r.data),
  delete: (id: string) => http.delete(`/api/v1/integrations/${id}`).then(r => r.data),
  test:   (id: string) => http.post(`/api/v1/integrations/${id}/test`).then(r => r.data),
}

// ── APM ───────────────────────────────────────────────────────────────────────
export const apm = {
  services: (params?: {sort?:string;limit?:number}) =>
    http.get('/api/v1/apm/services', { params }).then(r => r.data),
  transactions: (serviceId: string, params?: {limit?:number}) =>
    http.get(`/api/v1/apm/services/${serviceId}/transactions`, { params }).then(r => r.data),
  errors: (serviceId: string) =>
    http.get(`/api/v1/apm/services/${serviceId}/errors`).then(r => r.data),
}

// ── Infrastructure / Node monitoring ──────────────────────────────────────────

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

export const nodes = {
  list: () => http.get<{ nodes: NodeInfo[]; total: number }>('/api/v1/nodes').then(r => r.data),
  metrics: (node: string, hours = 1) =>
    http.get(`/api/v1/nodes/${encodeURIComponent(node)}/metrics`, { params: { hours } }).then(r => r.data),
}

// ── Cost Attribution (FinOps) ─────────────────────────────────────────────────

export interface ServiceCost {
  service_id: string; service_name: string; namespace: string
  cpu_cores: number; mem_gb: number
  cost_per_hour: number; cost_per_day: number; cost_per_month: number
}
export interface CostReport {
  services: ServiceCost[]
  total_per_hour: number; total_per_day: number; total_per_month: number
  price_per_core_hour?: number; price_per_gb_hour?: number
  currency: string
}
export const costs = {
  report: () => http.get<CostReport>('/api/v1/costs').then(r => r.data),
}

// ── On-call & escalation ──────────────────────────────────────────────────────

export interface OncallSchedule {
  id: string; name: string; description: string; timezone: string; created_at: string
}
export interface EscalationPolicy {
  id: string; name: string; description: string
  steps: Array<{ delay_min: number; targets: string[] }>
}
export const oncall = {
  schedules:       () => http.get<{ schedules: OncallSchedule[]; total: number }>('/api/v1/oncall/schedules').then(r => r.data),
  createSchedule:  (d: Partial<OncallSchedule>) => http.post('/api/v1/oncall/schedules', d).then(r => r.data),
  updateSchedule:  (id: string, d: Partial<OncallSchedule>) => http.put(`/api/v1/oncall/schedules/${id}`, d).then(r => r.data),
  deleteSchedule:  (id: string) => http.delete(`/api/v1/oncall/schedules/${id}`),
  rotations:       (scheduleId: string) => http.get(`/api/v1/oncall/schedules/${scheduleId}/rotations`).then(r => r.data),
  addRotation:     (scheduleId: string, d: any) => http.post(`/api/v1/oncall/schedules/${scheduleId}/rotations`, d).then(r => r.data),
  deleteRotation:  (rotationId: string) => http.delete(`/api/v1/oncall/rotations/${rotationId}`),
  policies:        () => http.get<{ policies: EscalationPolicy[]; total: number }>('/api/v1/oncall/policies').then(r => r.data),
  createPolicy:    (d: Partial<EscalationPolicy>) => http.post('/api/v1/oncall/policies', d).then(r => r.data),
  updatePolicy:    (id: string, d: Partial<EscalationPolicy>) => http.put(`/api/v1/oncall/policies/${id}`, d).then(r => r.data),
  deletePolicy:    (id: string) => http.delete(`/api/v1/oncall/policies/${id}`),
  whoIsOnCall:     (scheduleId?: string) => http.get('/api/v1/oncall/who', { params: { schedule_id: scheduleId } }).then(r => r.data),
}

// ── Incident comments ─────────────────────────────────────────────────────────

export interface IncidentComment {
  id: string
  org_id: string
  problem_id: string
  user_id: string
  user_name?: string
  user_email?: string
  content: string
  edited: boolean
  edited_at?: string
  deleted: boolean
  created_at: string
  updated_at: string
}

export const incidentComments = {
  list:   (problemId: string) =>
    http.get<{ comments: IncidentComment[]; total: number }>(`/api/v1/problems/${problemId}/comments`).then(r => r.data),
  create: (problemId: string, content: string) =>
    http.post<IncidentComment>(`/api/v1/problems/${problemId}/comments`, { content }).then(r => r.data),
  update: (problemId: string, commentId: string, content: string) =>
    http.put<IncidentComment>(`/api/v1/problems/${problemId}/comments/${commentId}`, { content }).then(r => r.data),
  delete: (problemId: string, commentId: string) =>
    http.delete(`/api/v1/problems/${problemId}/comments/${commentId}`),
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// ── Event Explorer (ClickHouse) ───────────────────────────────────────────────
export const events = {
  query: (params?: { q?: string; hours?: number; service_id?: string; limit?: number }) =>
    http.get('/api/v1/events', { params }).then(r => r.data),
  stats: () => http.get('/api/v1/events/stats').then(r => r.data),
}

// ── Postmortems ───────────────────────────────────────────────────────────────
export const postmortems = {
  list:   (params?: { status?: string }) => http.get('/api/v1/postmortems', { params }).then(r => r.data),
  get:    (id: string) => http.get(`/api/v1/postmortems/${id}`).then(r => r.data),
  create: (d: any) => http.post('/api/v1/postmortems', d).then(r => r.data),
  update: (id: string, d: any) => http.put(`/api/v1/postmortems/${id}`, d).then(r => r.data),
  delete: (id: string) => http.delete(`/api/v1/postmortems/${id}`).then(r => r.data),
}

// ── Alert groups (M7) ─────────────────────────────────────────────────────────
export const alertGroups = {
  list: () => http.get('/api/v1/alerts/groups').then(r => r.data),
}

// ── Serverless ────────────────────────────────────────────────────────────────
export const serverless = {
  functions: (params?: { region?: string; runtime?: string }) =>
    http.get('/api/v1/serverless/functions', { params }).then(r => r.data),
}


// ── SSO / SAML / OIDC ─────────────────────────────────────────────────────────
export interface SSOConfig {
  id: string
  org_id: string
  type: 'saml' | 'oidc' | 'github' | 'google' | 'azure'
  enabled: boolean
  idp_entity_id?: string
  idp_sso_url?: string
  sp_entity_id?: string
  issuer?: string
  client_id?: string
  email_attr?: string
  name_attr?: string
  role_attr?: string
  auto_provision: boolean
  default_role: string
  created_at: string
  updated_at: string
}

export const sso = {
  providers:    () => http.get<{ providers: SSOConfig[]; total: number }>('/api/auth/sso/providers').then(r => r.data),
  getConfig:    () => http.get<{ configs: SSOConfig[]; total: number }>('/api/v1/sso/providers').then(r => r.data),
  upsert:       (d: Partial<SSOConfig>) => http.put('/api/v1/sso/providers', d).then(r => r.data),
  delete:       (id: string) => http.delete(`/api/v1/sso/providers/${id}`).then(r => r.data),
  test:         (type: string) => http.post('/api/v1/sso/providers/test', { type }).then(r => r.data),
  samlMetadata: () => http.get('/api/auth/sso/saml/metadata').then(r => r.data),
}

// ── Multi-cluster Kubernetes ───────────────────────────────────────────────────
export interface K8sCluster {
  id: string
  org_id: string
  name: string
  display_name: string
  region: string
  provider: string
  api_server_url: string
  agent_version: string
  node_count: number
  pod_count: number
  status: 'active' | 'unreachable' | 'unknown' | 'no_agent'
  last_seen_at?: string
  created_at: string
}

export const clusters = {
  list:       () => http.get<{ clusters: K8sCluster[]; total: number }>('/api/v1/clusters').then(r => r.data),
  register:   (d: Partial<K8sCluster>) => http.post('/api/v1/clusters', d).then(r => r.data),
  deregister: (id: string) => http.delete(`/api/v1/clusters/${id}`).then(r => r.data),
  health:     (id: string) => http.get(`/api/v1/clusters/${id}/health`).then(r => r.data),
  metrics:    (id: string, query: string, start: string, end: string, step = '60') =>
    http.get(`/api/v1/clusters/${id}/metrics`, { params: { query, start, end, step } }).then(r => r.data),
}

// ── Session Replay ────────────────────────────────────────────────────────────
export interface SessionReplay {
  id: string
  user_id?: string
  session_id: string
  url: string
  duration_ms: number
  event_count: number
  error_count: number
  rage_click_count: number
  country?: string
  browser?: string
  os?: string
  device?: string
  screen?: string
  started_at: string
  ended_at?: string
  tags?: string[]
}

export const sessionReplay = {
  list: (params?: { limit?: number; offset?: number; url?: string; has_errors?: boolean }) =>
    http.get<{ sessions: SessionReplay[]; total: number }>('/api/v1/rum/sessions', { params }).then(r => r.data),
  get:     (id: string) => http.get<SessionReplay>(`/api/v1/rum/sessions/${id}`).then(r => r.data),
  events:  (id: string) => http.get<{ events: any[] }>(`/api/v1/rum/sessions/${id}/events`).then(r => r.data),
  heatmap: (url: string) => http.get<{ clicks: any[] }>('/api/v1/rum/heatmap', { params: { url } }).then(r => r.data),
}

// ── Workloads ────────────────────────────────────────────────────────────────
export const workloads = {
  list:   () => http.get<any>('/api/v1/workloads').then(r => r.data),
  get:    (id: string) => http.get<any>(`/api/v1/workloads/${id}`).then(r => r.data),
  create: (body: any) => http.post('/api/v1/workloads', body).then(r => r.data),
  update: (id: string, body: any) => http.put(`/api/v1/workloads/${id}`, body).then(r => r.data),
  delete: (id: string) => http.delete(`/api/v1/workloads/${id}`),
}

// ── Change Tracking ──────────────────────────────────────────────────────────
export const changeTracking = {
  list: (params?: any) => http.get<any>('/api/v1/deployments', { params }).then(r => r.data),
  get:  (id: string) => http.get<any>(`/api/v1/deployments/${id}`).then(r => r.data),
  record: (body: any) => http.post('/api/v1/deployments', body).then(r => r.data),
  impact: (id: string) => http.get<any>(`/api/v1/deployments/${id}/impact`).then(r => r.data),
}

// ── Errors Inbox ─────────────────────────────────────────────────────────────
export const errorsInbox = {
  list:    (params?: any) => http.get<any>('/api/v1/errors/groups', { params }).then(r => r.data),
  get:     (id: string) => http.get<any>(`/api/v1/errors/groups/${id}`).then(r => r.data),
  resolve: (id: string) => http.post(`/api/v1/errors/groups/${id}/resolve`),
  ignore:  (id: string) => http.post(`/api/v1/errors/groups/${id}/ignore`),
  assign:  (id: string, user: string) => http.post(`/api/v1/errors/groups/${id}/assign`, { user }),
}

// ── LLM Monitor ──────────────────────────────────────────────────────────────
export const llmMonitor = {
  stats:      () => http.get<any>('/api/v1/llm/stats').then(r => r.data),
  models:     () => http.get<any>('/api/v1/llm/models').then(r => r.data),
  timeseries: (params?: any) => http.get<any>('/api/v1/llm/timeseries', { params }).then(r => r.data),
  ingest:     (body: any) => http.post('/api/v1/llm/ingest', body),
  calls:      (params?: any) => http.get<any>('/api/v1/llm/calls', { params }).then(r => r.data),
}

// ── Mobile Monitoring ────────────────────────────────────────────────────────
export const mobile = {
  apps:       () => http.get<any>('/api/v1/mobile/apps').then(r => r.data),
  crashes:    (appId: string, params?: any) => http.get<any>(`/api/v1/mobile/apps/${appId}/crashes`, { params }).then(r => r.data),
  sessions:   (appId: string, params?: any) => http.get<any>(`/api/v1/mobile/apps/${appId}/sessions`, { params }).then(r => r.data),
  metrics:    (appId: string, params?: any) => http.get<any>(`/api/v1/mobile/apps/${appId}/metrics`, { params }).then(r => r.data),
  network:    (appId: string) => http.get<any>(`/api/v1/mobile/apps/${appId}/network`).then(r => r.data),
}

// ── Hub / Integrations Marketplace ───────────────────────────────────────────
export const hub = {
  extensions: (params?: any) => http.get<any>('/api/v1/hub/extensions', { params }).then(r => r.data),
  install:    (id: string) => http.post(`/api/v1/hub/extensions/${id}/install`),
  uninstall:  (id: string) => http.delete(`/api/v1/hub/extensions/${id}`),
  installed:  () => http.get<any>('/api/v1/hub/installed').then(r => r.data),
}

// ── Anomaly Detection ─────────────────────────────────────────────────────────
export const anomalyDetection = {
  rules:   () => http.get<any>('/api/v1/anomaly/rules').then(r => r.data),
  create:  (body: any) => http.post('/api/v1/anomaly/rules', body).then(r => r.data),
  update:  (id: string, body: any) => http.put(`/api/v1/anomaly/rules/${id}`, body),
  delete:  (id: string) => http.delete(`/api/v1/anomaly/rules/${id}`),
  preview: (body: any) => http.post('/api/v1/anomaly/preview', body).then(r => r.data),
}

// ── Runbooks ─────────────────────────────────────────────────────────────────
export const runbooks = {
  list:    (params?: any) => http.get<any>('/api/v1/runbooks', { params }).then(r => r.data),
  get:     (id: string) => http.get<any>(`/api/v1/runbooks/${id}`).then(r => r.data),
  create:  (body: any) => http.post('/api/v1/runbooks', body).then(r => r.data),
  update:  (id: string, body: any) => http.put(`/api/v1/runbooks/${id}`, body),
  delete:  (id: string) => http.delete(`/api/v1/runbooks/${id}`),
  execute: (id: string, params: any) => http.post(`/api/v1/runbooks/${id}/execute`, params).then(r => r.data),
}

// Re-export http for custom calls
export { http }
