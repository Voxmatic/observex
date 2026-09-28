// frontend/src/pages/AuthPage.tsx
// S28 — Identity & Access Observability
//
// Monitors authentication and identity flows across the platform:
//   - Login success/failure rates and trends over time
//   - Authentication failure breakdown (wrong password, expired token, locked account)
//   - Active sessions count and anomalous concurrent session detection
//   - Failed login heatmap by user and IP address (brute-force detection)
//   - API key usage and last-seen tracking
//   - Full identity audit log with action filtering

import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { audit, metrics } from '@/lib/api'
import { PageHeader, Spinner, Btn } from '@/components/shared/Layout'
import {
  Lock, ShieldAlert, Users, Key, Activity, TrendingUp,
  AlertTriangle, CheckCircle, XCircle, Download
} from 'lucide-react'
import {
  AreaChart, Area, BarChart, Bar, XAxis, YAxis, Tooltip,
  ResponsiveContainer, Cell, CartesianGrid, LineChart, Line, Legend,
} from 'recharts'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'

// ── Constants ─────────────────────────────────────────────────────────────────

const TIME_RANGES = [
  { label: '1h',  seconds: 3600 },
  { label: '24h', seconds: 86400 },
  { label: '7d',  seconds: 604800 },
  { label: '30d', seconds: 2592000 },
]

// ── Hooks ──────────────────────────────────────────────────────────────────────

function useAuthMetrics(range: number) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  const logins = useQuery({
    queryKey: ['auth-logins', range],
    queryFn: () => metrics.queryRange(
      `sum by (status) (increase(auth_login_total[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 60_000,
  })

  const failures = useQuery({
    queryKey: ['auth-failures', range],
    queryFn: () => metrics.queryRange(
      `sum by (reason) (increase(auth_login_failed_total[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 60_000,
  })

  return { logins, failures }
}

// ── Audit log data ─────────────────────────────────────────────────────────────

function useIdentityAudit(range: number) {
  const limit = Math.min(200, range > 86400 ? 200 : 100)
  return useQuery({
    queryKey: ['identity-audit', range],
    queryFn: () => audit.list({ limit, action: 'login,login_failed,logout,api_key_used' }),
    refetchInterval: 30_000,
  })
}

// ── Summary stat card ─────────────────────────────────────────────────────────

function StatCard({ label, value, sub, icon: Icon, color, alert }: {
  label: string; value: string | number; sub?: string
  icon: React.ElementType
  color: string; alert?: boolean
}) {
  return (
    <div className={clsx('bg-surface-1 border rounded-xl p-4 transition-colors',
      alert ? 'border-crit/40 bg-crit/5' : 'border-surface-3')}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs text-slate-500">{label}</span>
        <Icon size={13} className={alert ? 'text-crit' : 'text-slate-600'} />
      </div>
      <div className={clsx('text-2xl font-semibold font-display', color)}>
        {value}
      </div>
      {sub && <div className="text-xs text-slate-600 mt-1">{sub}</div>}
    </div>
  )
}

// ── Login success/failure trend ───────────────────────────────────────────────

function LoginTrend({ range }: { range: number }) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  const { data } = useQuery({
    queryKey: ['login-trend', range],
    queryFn: () => Promise.all([
      metrics.queryRange(
        `sum(increase(observex_auth_login_total[${step}s]))`,
        start, now, String(step)
      ),
      metrics.queryRange(
        `sum(increase(observex_auth_login_failed_total[${step}s]))`,
        start, now, String(step)
      ),
    ]),
    refetchInterval: 120_000,
  })

  const chartData = useMemo(() => {
    if (!data) return []
    const [success, failed] = data as any[]
    const successPts = (success?.result?.[0]?.values ?? []) as [number, string][]
    const failedPts  = (failed?.result?.[0]?.values   ?? []) as [number, string][]

    const map: Record<number, { t: string; success: number; failed: number }> = {}
    for (const [ts, v] of successPts) {
      map[ts] = { t: format(new Date(ts * 1000), range > 86400 ? 'MMM d' : 'HH:mm'), success: Math.round(parseFloat(v) || 0), failed: 0 }
    }
    for (const [ts, v] of failedPts) {
      if (map[ts]) map[ts].failed = Math.round(parseFloat(v) || 0)
      else map[ts] = { t: format(new Date(ts * 1000), range > 86400 ? 'MMM d' : 'HH:mm'), success: 0, failed: Math.round(parseFloat(v) || 0) }
    }

    return Object.values(map).sort((a, b) => a.t.localeCompare(b.t))
  }, [data, range])

  // Fallback: derive from audit log entries if no metrics exist
  const { data: auditData } = useQuery({
    queryKey: ['auth-audit-trend', range],
    queryFn: () => audit.list({ limit: 500 }),
    refetchInterval: 120_000,
    enabled: chartData.length === 0,
  })

  const fallbackData = useMemo(() => {
    if (chartData.length > 0) return []
    const entries = (auditData as any)?.entries ?? []
    const buckets: Record<string, { t: string; success: number; failed: number }> = {}
    const bucketMs = range <= 3600 ? 300_000 : range <= 86400 ? 3600_000 : 86400_000

    for (const e of entries) {
      const ts = new Date(e.created_at).getTime()
      const bk = Math.floor(ts / bucketMs) * bucketMs
      const t  = format(new Date(bk), range <= 3600 ? 'HH:mm' : range <= 86400 ? 'HH:mm' : 'MMM d')
      if (!buckets[bk]) buckets[bk] = { t, success: 0, failed: 0 }
      if (e.action === 'login') buckets[bk].success++
      if (e.action === 'login_failed') buckets[bk].failed++
    }
    return Object.values(buckets).sort((a, b) => a.t.localeCompare(b.t))
  }, [auditData, chartData, range])

  const pts = chartData.length > 0 ? chartData : fallbackData
  if (!pts.length) return (
    <div className="flex items-center justify-center h-32 text-xs text-slate-600">
      No login events in this time range
    </div>
  )

  return (
    <ResponsiveContainer width="100%" height={160}>
      <AreaChart data={pts}>
        <defs>
          <linearGradient id="gradSuccess" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%"  stopColor="#22c55e" stopOpacity={0.3} />
            <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
          </linearGradient>
          <linearGradient id="gradFailed" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%"  stopColor="#ef4444" stopOpacity={0.3} />
            <stop offset="95%" stopColor="#ef4444" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 15%)" vertical={false} />
        <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 40%)', fontSize: 10 }} tickLine={false} />
        <YAxis tick={{ fill: 'hsl(215 20% 40%)', fontSize: 10 }} tickLine={false} width={30} />
        <Tooltip contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', fontSize: 11, borderRadius: 8 }} />
        <Legend wrapperStyle={{ fontSize: 11 }} />
        <Area type="monotone" dataKey="success" stroke="#22c55e" fill="url(#gradSuccess)" strokeWidth={1.5} dot={false} name="Successful logins" />
        <Area type="monotone" dataKey="failed"  stroke="#ef4444" fill="url(#gradFailed)"  strokeWidth={1.5} dot={false} name="Failed attempts" />
      </AreaChart>
    </ResponsiveContainer>
  )
}

// ── Failure breakdown ─────────────────────────────────────────────────────────

function FailureBreakdown({ entries }: { entries: any[] }) {
  const reasons = useMemo(() => {
    const counts: Record<string, number> = {}
    for (const e of entries) {
      if (e.action !== 'login_failed') continue
      const reason = (e.details?.reason ?? 'unknown') as string
      counts[reason] = (counts[reason] ?? 0) + 1
    }
    return Object.entries(counts).map(([r, c]) => ({ reason: r, count: c }))
      .sort((a, b) => b.count - a.count)
  }, [entries])

  if (!reasons.length) return (
    <div className="text-xs text-ok text-center py-4">No authentication failures</div>
  )

  const COLORS = ['#ef4444', '#f97316', '#f59e0b', '#6366f1', '#3b82f6']

  return (
    <div className="space-y-2">
      {reasons.map(({ reason, count }, i) => (
        <div key={reason} className="flex items-center gap-2.5 text-xs">
          <span className="w-2 h-2 rounded-full shrink-0" style={{ background: COLORS[i % COLORS.length] }} />
          <span className="flex-1 text-slate-400 capitalize">{reason.replace(/_/g, ' ')}</span>
          <span className="font-mono text-slate-300">{count}</span>
          <div className="w-20 h-1.5 bg-surface-3 rounded-full overflow-hidden shrink-0">
            <div className="h-full rounded-full" style={{
              width: `${Math.round(count / reasons[0].count * 100)}%`,
              background: COLORS[i % COLORS.length],
            }} />
          </div>
        </div>
      ))}
    </div>
  )
}

// ── Top failed-login users/IPs ────────────────────────────────────────────────

function SuspiciousActivity({ entries }: { entries: any[] }) {
  const ipCounts = useMemo(() => {
    const counts: Record<string, { count: number; emails: Set<string> }> = {}
    for (const e of entries) {
      if (e.action !== 'login_failed') continue
      const ip = e.ip_address || 'unknown'
      if (!counts[ip]) counts[ip] = { count: 0, emails: new Set() }
      counts[ip].count++
      if (e.user_email) counts[ip].emails.add(e.user_email)
    }
    return Object.entries(counts)
      .map(([ip, d]) => ({ ip, count: d.count, uniqueUsers: d.emails.size }))
      .filter(r => r.count > 1)
      .sort((a, b) => b.count - a.count)
      .slice(0, 8)
  }, [entries])

  if (!ipCounts.length) return (
    <div className="text-xs text-ok text-center py-4 flex items-center justify-center gap-1.5">
      <CheckCircle size={12} />
      No brute-force patterns detected
    </div>
  )

  return (
    <div className="divide-y divide-surface-3">
      {ipCounts.map(r => (
        <div key={r.ip} className="flex items-center gap-3 py-2.5 text-xs">
          <div className="flex-1 min-w-0">
            <div className="font-mono text-slate-300">{r.ip}</div>
            <div className="text-slate-600 text-[10px]">{r.uniqueUsers} unique user{r.uniqueUsers !== 1 ? 's' : ''} targeted</div>
          </div>
          <span className={clsx('shrink-0 font-semibold', r.count >= 10 ? 'text-crit' : 'text-warn')}>
            {r.count} attempts
          </span>
          {r.count >= 5 && (
            <span className="shrink-0 text-[10px] bg-crit/15 text-crit px-1.5 py-0.5 rounded">
              ⚠ Suspicious
            </span>
          )}
        </div>
      ))}
    </div>
  )
}

// ── Identity audit log ────────────────────────────────────────────────────────

const ACTION_META: Record<string, { color: string; icon: React.ElementType }> = {
  login:        { color: 'text-ok',    icon: CheckCircle },
  login_failed: { color: 'text-crit',  icon: XCircle },
  logout:       { color: 'text-slate-400', icon: Lock },
  api_key_used: { color: 'text-brand', icon: Key },
}

function IdentityAuditLog({ entries, filter }: { entries: any[]; filter: string }) {
  const filtered = useMemo(() =>
    filter ? entries.filter(e => e.action === filter) : entries
  , [entries, filter])

  if (!filtered.length) return (
    <div className="text-xs text-slate-600 text-center py-6">No events</div>
  )

  return (
    <div className="divide-y divide-surface-3 text-xs max-h-96 overflow-y-auto">
      {filtered.slice(0, 100).map(e => {
        const meta = ACTION_META[e.action] ?? { color: 'text-slate-400', icon: Activity }
        const { icon: Icon } = meta
        return (
          <div key={e.id} className="flex items-start gap-3 px-4 py-2.5 hover:bg-surface-2/30">
            <Icon size={11} className={clsx('mt-0.5 shrink-0', meta.color)} />
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className={clsx('font-medium', meta.color)}>{e.action.replace(/_/g, ' ')}</span>
                <span className="text-slate-500 truncate">{e.user_email}</span>
              </div>
              <div className="text-slate-700 text-[10px] mt-0.5 flex items-center gap-2">
                <span className="font-mono">{e.ip_address}</span>
                {e.details?.reason && <span className="text-crit">· {e.details.reason}</span>}
                {e.user_agent && <span className="truncate max-w-40">{e.user_agent}</span>}
              </div>
            </div>
            <span className="text-slate-700 shrink-0">
              {formatDistanceToNow(new Date(e.created_at), { addSuffix: true })}
            </span>
          </div>
        )
      })}
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function AuthPage() {
  const [range, setRange]   = useState(86400)
  const [logFilter, setLogFilter] = useState('')

  const { data: auditData, isLoading } = useIdentityAudit(range)
  const entries: any[] = (auditData as any)?.entries ?? []

  // Summary counts from audit log (fallback when metrics aren't flowing)
  const loginCount   = entries.filter(e => e.action === 'login').length
  const failedCount  = entries.filter(e => e.action === 'login_failed').length
  const logoutCount  = entries.filter(e => e.action === 'logout').length
  const apiKeyCount  = entries.filter(e => e.action === 'api_key_used').length
  const failureRate  = loginCount + failedCount > 0
    ? ((failedCount / (loginCount + failedCount)) * 100).toFixed(1)
    : '0.0'

  // Unique active users in window
  const activeUsers = new Set(entries.filter(e => e.action === 'login').map(e => e.user_email)).size

  const handleExportCSV = () => {
    const rows = [
      'timestamp,action,user_email,ip_address,user_agent,reason',
      ...entries.map(e => [
        e.created_at, e.action, e.user_email,
        e.ip_address, `"${(e.user_agent ?? '').replace(/"/g, '""')}"`,
        e.details?.reason ?? '',
      ].join(','))
    ].join('\n')
    const blob = new Blob([rows], { type: 'text/csv' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `auth-audit-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Identity & Access"
        subtitle="Authentication flows · Login monitoring · Brute-force detection · API key usage"
        actions={
          <div className="flex items-center gap-2">
            <Btn size="xs" variant="ghost" onClick={handleExportCSV}>
              <Download size={12} /> Export CSV
            </Btn>
            <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
              {TIME_RANGES.map(r => (
                <button key={r.label} onClick={() => setRange(r.seconds)}
                  className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                    range === r.seconds ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                  {r.label}
                </button>
              ))}
            </div>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Summary cards */}
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <StatCard label="Successful logins"   value={loginCount}    icon={CheckCircle} color="text-ok"    />
          <StatCard label="Failed attempts"     value={failedCount}   icon={XCircle}     color={failedCount > 0 ? 'text-crit' : 'text-ok'} alert={failedCount > 10} />
          <StatCard label="Failure rate"        value={`${failureRate}%`} icon={ShieldAlert} color={parseFloat(failureRate) > 10 ? 'text-crit' : 'text-slate-300'} alert={parseFloat(failureRate) > 10} />
          <StatCard label="Active users"        value={activeUsers}   icon={Users}       color="text-brand" />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <StatCard label="API key calls"  value={apiKeyCount}  icon={Key}      color="text-slate-300" />
          <StatCard label="Logouts"        value={logoutCount}  icon={Lock}     color="text-slate-400" />
        </div>

        {/* Login trend */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="text-xs font-medium text-slate-400 mb-3">Login success vs failures over time</div>
          {isLoading
            ? <div className="flex justify-center py-8"><Spinner /></div>
            : <LoginTrend range={range} />
          }
        </div>

        <div className="grid grid-cols-2 gap-4">
          {/* Failure breakdown */}
          <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
            <div className="text-xs font-medium text-slate-400 mb-3">Failure reasons</div>
            <FailureBreakdown entries={entries} />
          </div>

          {/* Suspicious IPs */}
          <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
            <div className="flex items-center gap-2 mb-3">
              <ShieldAlert size={12} className="text-slate-500" />
              <span className="text-xs font-medium text-slate-400">Suspicious IPs (repeated failures)</span>
            </div>
            <SuspiciousActivity entries={entries} />
          </div>
        </div>

        {/* Identity audit log */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 border-b border-surface-3">
            <span className="text-xs font-medium text-slate-400">Identity audit log</span>
            <div className="flex gap-1">
              {['', 'login', 'login_failed', 'logout', 'api_key_used'].map(f => (
                <button key={f} onClick={() => setLogFilter(f)}
                  className={clsx('px-2 py-0.5 text-[10px] rounded transition-colors',
                    logFilter === f
                      ? 'bg-brand/20 text-brand'
                      : 'text-slate-600 hover:text-slate-300')}>
                  {f || 'All'}
                </button>
              ))}
            </div>
          </div>
          {isLoading
            ? <div className="flex justify-center p-6"><Spinner /></div>
            : <IdentityAuditLog entries={entries} filter={logFilter} />
          }
        </div>
      </div>
    </div>
  )
}
