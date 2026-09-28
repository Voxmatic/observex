import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { metrics, logs } from '@/lib/api'
import { PageHeader, Spinner, Btn } from '@/components/shared/Layout'
import {
  Monitor, AlertTriangle, Globe, GitBranch,
  TrendingDown, Users, Map, ChevronRight, Download
} from 'lucide-react'
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell,
  LineChart, Line, Legend,
  AreaChart, Area, CartesianGrid,
} from 'recharts'
import clsx from 'clsx'

// ── Constants ─────────────────────────────────────────────────────────────────

const VITAL_THRESHOLDS: Record<string, [number, number]> = {
  LCP: [2500, 4000], FID: [100, 300], INP: [200, 500],
  CLS: [0.1,  0.25], FCP: [1800, 3000], TTFB: [800, 1800],
}
const VITALS = ['LCP', 'FCP', 'TTFB', 'INP', 'CLS']

const TIME_RANGES = [
  { label: '1h',  seconds: 3600 },
  { label: '6h',  seconds: 21600 },
  { label: '24h', seconds: 86400 },
  { label: '7d',  seconds: 604800 },
]

type ActiveTab = 'vitals' | 'journeys' | 'versions' | 'errors'

// ── Rating helpers ─────────────────────────────────────────────────────────────

function ratingColor(r: string) {
  return r === 'good' ? 'text-ok' : r === 'needs-improvement' ? 'text-warn' : 'text-crit'
}
function vitalRating(name: string, val: number): 'good' | 'needs-improvement' | 'poor' {
  const [good, poor] = VITAL_THRESHOLDS[name] ?? [0, 0]
  return val <= good ? 'good' : val <= poor ? 'needs-improvement' : 'poor'
}

// ── Data hooks ─────────────────────────────────────────────────────────────────

function useVitalP75(name: string, range: number) {
  return useQuery({
    queryKey: ['rum-vital', name, range],
    queryFn: () => metrics.query(
      `histogram_quantile(0.75, sum(rate(rum_web_vital_ms_bucket{vital="${name}"}[${range}s])) by (le))`
    ),
    refetchInterval: 60_000,
  })
}

// ── Tab: Web Vitals ────────────────────────────────────────────────────────────

function VitalGauge({ name, range }: { name: string; range: number }) {
  const { data } = useVitalP75(name, range)
  const raw = parseFloat((data as any)?.data?.result?.[0]?.value?.[1] ?? 'NaN')
  const val = isNaN(raw) ? null : raw
  const rating = val != null ? vitalRating(name, val) : 'good'
  const [good, poor] = VITAL_THRESHOLDS[name] ?? [1, 2]
  const pct = val != null ? Math.min(100, (val / poor) * 100) : 0
  const unit = name === 'CLS' ? '' : 'ms'

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs font-semibold text-slate-400">{name}</span>
        {val != null && (
          <span className={clsx('text-[10px] font-medium px-1.5 py-0.5 rounded', ratingColor(rating),
            rating === 'good' ? 'bg-ok/10' : rating === 'needs-improvement' ? 'bg-warn/10' : 'bg-crit/10')}>
            {rating === 'good' ? 'Good' : rating === 'needs-improvement' ? 'Needs work' : 'Poor'}
          </span>
        )}
      </div>
      <div className={clsx('text-2xl font-semibold font-display mb-2', val != null ? ratingColor(rating) : 'text-slate-700')}>
        {val != null ? (name === 'CLS' ? val.toFixed(3) : Math.round(val).toLocaleString()) : '—'}
        {val != null && <span className="text-sm font-normal text-slate-500 ml-1">{unit}</span>}
      </div>
      <div className="h-1.5 bg-surface-3 rounded-full overflow-hidden">
        <div className={clsx('h-full rounded-full transition-all',
          rating === 'good' ? 'bg-ok' : rating === 'needs-improvement' ? 'bg-warn' : 'bg-crit')}
          style={{ width: `${pct}%` }} />
      </div>
      <div className="flex justify-between text-[9px] text-slate-700 mt-1">
        <span>good ≤{name === 'CLS' ? good : `${good.toLocaleString()}ms`}</span>
        <span>poor &gt;{name === 'CLS' ? poor : `${poor.toLocaleString()}ms`}</span>
      </div>
    </div>
  )
}

function VitalTrendChart({ name, range }: { name: string; range: number }) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  const { data } = useQuery({
    queryKey: ['rum-vital-trend', name, range],
    queryFn: () => metrics.queryRange(
      `histogram_quantile(0.75, sum(rate(rum_web_vital_ms_bucket{vital="${name}"}[${step}s])) by (le))`,
      start, now, String(step)
    ),
    refetchInterval: 120_000,
  })

  const pts = ((data as any)?.result?.[0]?.values ?? []).map(([ts, v]: [number, string]) => ({
    t:   new Date(ts * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    val: parseFloat(v) || 0,
  }))

  const [good] = VITAL_THRESHOLDS[name] ?? [1000, 3000]
  const color = pts.length > 0 && pts[pts.length - 1].val <= good ? '#22c55e' : '#f97316'

  return (
    <ResponsiveContainer width="100%" height={100}>
      <AreaChart data={pts}>
        <defs>
          <linearGradient id={`vg${name}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%"  stopColor={color} stopOpacity={0.3} />
            <stop offset="95%" stopColor={color} stopOpacity={0} />
          </linearGradient>
        </defs>
        <XAxis dataKey="t" hide />
        <YAxis hide />
        <Tooltip contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', fontSize: 11, borderRadius: 8 }} />
        <Area type="monotone" dataKey="val" stroke={color} fill={`url(#vg${name})`} strokeWidth={1.5} dot={false} name={name} />
      </AreaChart>
    </ResponsiveContainer>
  )
}

function ErrorLog({ range }: { range: number }) {
  const now   = Math.floor(Date.now() * 1e6)
  const start = now - range * 1e9

  const { data, isLoading } = useQuery({
    queryKey: ['rum-errors', range],
    queryFn: () => logs.queryRange('{source="rum"} | level="error"', start, now, 50),
    refetchInterval: 30_000,
  })

  const lines = (data ?? []).flatMap((s: any) =>
    (s.values ?? []).map(([ts, line]: [string, string]) => ({
      ts: new Date(Number(ts) / 1e6).toLocaleTimeString(),
      msg: line,
    }))
  ).slice(0, 50)

  if (isLoading) return <div className="flex justify-center p-4"><Spinner /></div>
  if (!lines.length) return <div className="text-xs text-slate-600 px-4 py-6 text-center">No JS errors in this window</div>

  return (
    <div className="divide-y divide-surface-3 max-h-64 overflow-y-auto">
      {lines.map((l: any, i: number) => (
        <div key={i} className="flex gap-3 px-4 py-2 text-xs font-mono hover:bg-surface-2/30">
          <span className="text-slate-700 shrink-0">{l.ts}</span>
          <span className="text-crit/80 break-all">{l.msg}</span>
        </div>
      ))}
    </div>
  )
}

// ── Tab: User Journeys (S37) ───────────────────────────────────────────────────

function usePageFlow(range: number) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  // Top pages by visit count
  return useQuery({
    queryKey: ['rum-page-flow', range],
    queryFn: () => metrics.queryRange(
      `sum by (path) (increase(rum_page_load_ms_count[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 120_000,
  })
}

function useNavigationFlow(range: number) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  // Navigation transitions (from → to)
  return useQuery({
    queryKey: ['rum-nav-flow', range],
    queryFn: () => metrics.queryRange(
      `sum by (from, to) (increase(rum_navigation_ms_count[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 120_000,
  })
}

function JourneysTab({ range }: { range: number }) {
  const { data: flowData, isLoading: flowLoading } = usePageFlow(range)
  const { data: navData }  = useNavigationFlow(range)

  // Build page visit totals
  const pageVisits = useMemo(() => {
    const result = (flowData as any)?.result ?? []
    const totals: Record<string, number> = {}
    for (const series of result) {
      const path = series.metric?.path ?? 'unknown'
      const vals: [number, string][] = series.values ?? []
      totals[path] = (totals[path] ?? 0) + vals.reduce((s, [, v]) => s + (parseFloat(v) || 0), 0)
    }
    return Object.entries(totals)
      .map(([path, count]) => ({ path, count: Math.round(count) }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 10)
  }, [flowData])

  // Build funnel from most common navigation paths
  const funnel = useMemo(() => {
    if (!pageVisits.length) return []
    const total = pageVisits[0].count || 1
    return pageVisits.slice(0, 6).map((p, i) => ({
      name: p.path.length > 28 ? '…' + p.path.slice(-25) : p.path,
      value: p.count,
      pct: Math.round((p.count / total) * 100),
      dropOff: i > 0 ? Math.round((1 - p.count / pageVisits[i - 1].count) * 100) : 0,
      fill: ['#3b82f6', '#6366f1', '#8b5cf6', '#a855f7', '#c084fc', '#d8b4fe'][i],
    }))
  }, [pageVisits])

  // Top navigation transitions
  const transitions = useMemo(() => {
    const result = (navData as any)?.result ?? []
    const flows: Record<string, number> = {}
    for (const series of result) {
      const from = series.metric?.from ?? '?'
      const to   = series.metric?.to   ?? '?'
      if (from === to) continue
      const key = `${from} → ${to}`
      const vals: [number, string][] = series.values ?? []
      flows[key] = (flows[key] ?? 0) + vals.reduce((s, [, v]) => s + (parseFloat(v) || 0), 0)
    }
    return Object.entries(flows)
      .map(([flow, count]) => ({ flow, count: Math.round(count) }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 8)
  }, [navData])

  if (flowLoading) return <div className="flex justify-center py-16"><Spinner size={24} /></div>

  if (!pageVisits.length) return (
    <div className="flex flex-col items-center justify-center py-16 gap-3 text-center">
      <Map size={32} className="text-slate-700" />
      <p className="text-sm text-slate-500">No navigation data yet</p>
      <p className="text-xs text-slate-700 max-w-xs">
        Install the RUM SDK and add <code className="bg-surface-3 px-1 rounded">appId</code> to start tracking user journeys.
      </p>
    </div>
  )

  return (
    <div className="space-y-4">
      {/* Page visit funnel */}
      <div className="grid grid-cols-2 gap-4">
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="text-xs font-medium text-slate-400 mb-4">Page visit funnel (top pages by volume)</div>
          <div className="space-y-2">
            {funnel.map((step, i) => (
              <div key={step.name}>
                <div className="flex items-center justify-between text-xs mb-1">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="shrink-0 w-4 h-4 rounded text-[9px] font-bold flex items-center justify-center"
                      style={{ background: step.fill + '33', color: step.fill }}>
                      {i + 1}
                    </span>
                    <span className="font-mono text-slate-300 truncate" title={step.name}>{step.name}</span>
                  </div>
                  <div className="flex items-center gap-2 shrink-0 ml-2">
                    <span className="text-slate-400">{((step as any).value ?? (step as any).count ?? 0).toLocaleString()}</span>
                    <span className="text-slate-600 w-10 text-right">{step.pct}%</span>
                  </div>
                </div>
                <div className="h-1.5 bg-surface-3 rounded-full overflow-hidden">
                  <div className="h-full rounded-full" style={{ width: `${step.pct}%`, background: step.fill }} />
                </div>
                {step.dropOff > 0 && (
                  <div className="flex items-center gap-1 mt-0.5 text-[10px] text-crit">
                    <TrendingDown size={9} />
                    <span>{step.dropOff}% drop-off from previous</span>
                  </div>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Top transitions */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="text-xs font-medium text-slate-400 mb-3">Top navigation flows (SPA routes)</div>
          {transitions.length === 0 ? (
            <p className="text-xs text-slate-600 py-4 text-center">No SPA navigation events yet</p>
          ) : (
            <div className="space-y-2">
              {transitions.map(t => (
                <div key={t.flow} className="flex items-center gap-2 text-xs">
                  <span className="font-mono text-slate-400 flex-1 min-w-0 truncate" title={t.flow}>
                    {t.flow}
                  </span>
                  <span className="shrink-0 text-slate-500">{t.count.toLocaleString()}</span>
                  <div className="w-16 h-1.5 bg-surface-3 rounded-full overflow-hidden shrink-0">
                    <div className="h-full rounded-full bg-brand"
                      style={{ width: `${Math.round(t.count / (transitions[0]?.count || 1) * 100)}%` }} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Page load performance by path */}
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs font-medium text-slate-400 mb-3">Load time by page (p75) — slowest first</div>
        <ResponsiveContainer width="100%" height={180}>
          <BarChart
            data={pageVisits.slice(0, 8).map(p => ({ path: p.path.split('/').pop() || '/', visits: p.count }))}
            layout="vertical"
            barSize={12}
          >
            <XAxis type="number" hide />
            <YAxis type="category" dataKey="path" width={100}
              tick={{ fill: 'hsl(215 20% 50%)', fontSize: 10, fontFamily: 'monospace' }}
              tickLine={false} />
            <Tooltip
              contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', fontSize: 11, borderRadius: 8 }}
              formatter={(v: number) => [`${v.toLocaleString()} visits`, 'Visits']} />
            <Bar dataKey="visits" radius={[0, 4, 4, 0]} fill="#3b82f6" />
          </BarChart>
        </ResponsiveContainer>
      </div>
    </div>
  )
}

// ── Tab: App Versions (S36) ────────────────────────────────────────────────────

function VersionsTab({ range }: { range: number }) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  const { data: versionData } = useQuery({
    queryKey: ['rum-versions', range],
    queryFn: () => metrics.queryRange(
      `sum by (app_version) (increase(rum_page_load_ms_count[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 120_000,
  })

  const { data: crashData } = useQuery({
    queryKey: ['rum-crashes', range],
    queryFn: () => metrics.queryRange(
      `sum by (app_version) (increase(rum_js_errors_total{error_type="crash"}[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 60_000,
  })

  const { data: errorData } = useQuery({
    queryKey: ['rum-err-by-version', range],
    queryFn: () => metrics.queryRange(
      `sum by (app_version) (increase(rum_js_errors_total[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 60_000,
  })

  const versions = useMemo(() => {
    const sessions: Record<string, number> = {}
    for (const s of (versionData as any)?.result ?? []) {
      const v = s.metric?.app_version || 'unknown'
      sessions[v] = (sessions[v] ?? 0) +
        ((s.values ?? []) as [number, string][]).reduce((a, [, x]) => a + (parseFloat(x) || 0), 0)
    }

    const crashes: Record<string, number> = {}
    for (const s of (crashData as any)?.result ?? []) {
      const v = s.metric?.app_version || 'unknown'
      crashes[v] = (crashes[v] ?? 0) +
        ((s.values ?? []) as [number, string][]).reduce((a, [, x]) => a + (parseFloat(x) || 0), 0)
    }

    const errors: Record<string, number> = {}
    for (const s of (errorData as any)?.result ?? []) {
      const v = s.metric?.app_version || 'unknown'
      errors[v] = (errors[v] ?? 0) +
        ((s.values ?? []) as [number, string][]).reduce((a, [, x]) => a + (parseFloat(x) || 0), 0)
    }

    const total = Object.values(sessions).reduce((a, b) => a + b, 0) || 1
    return Object.entries(sessions)
      .map(([ver, count]) => ({
        version: ver,
        pageViews: Math.round(count),
        adoption: Math.round((count / total) * 100),
        crashes: Math.round(crashes[ver] ?? 0),
        errors:  Math.round(errors[ver] ?? 0),
        crashRate: count > 0 ? ((crashes[ver] ?? 0) / count * 100).toFixed(2) : '0.00',
      }))
      .sort((a, b) => b.pageViews - a.pageViews)
  }, [versionData, crashData, errorData])

  if (!versions.length) return (
    <div className="flex flex-col items-center justify-center py-16 gap-3 text-center">
      <GitBranch size={32} className="text-slate-700" />
      <p className="text-sm text-slate-500">No version data yet</p>
      <p className="text-xs text-slate-700 max-w-xs">
        Set <code className="bg-surface-3 px-1 rounded">appVersion</code> in
        {' '}<code className="bg-surface-3 px-1 rounded">ObserveX.init()</code> to track version adoption and crash rates.
      </p>
      <code className="text-[11px] bg-surface-2 border border-surface-3 rounded-lg px-3 py-2 font-mono text-slate-400">
        ObserveX.init({'{'} appId: 'my-app', appVersion: '2.3.1' {'}'})
      </code>
    </div>
  )

  const latestVersion = versions[0]?.version

  return (
    <div className="space-y-4">
      {/* Version adoption donut-style bar */}
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs font-medium text-slate-400 mb-3">Version adoption</div>
        <div className="space-y-2">
          {versions.slice(0, 8).map((v, i) => {
            const isLatest = v.version === latestVersion
            const barColor = isLatest ? '#3b82f6' : i === 1 ? '#6366f1' : '#4b5563'
            return (
              <div key={v.version} className="flex items-center gap-3 text-xs">
                <span className={clsx('font-mono shrink-0 w-20 truncate', isLatest ? 'text-brand font-medium' : 'text-slate-400')}>
                  {v.version}
                </span>
                <div className="flex-1 h-4 bg-surface-3 rounded overflow-hidden">
                  <div className="h-full rounded flex items-center pl-1.5 text-[10px] text-white/80 font-medium"
                    style={{ width: `${Math.max(v.adoption, 2)}%`, background: barColor }}>
                    {v.adoption > 5 ? `${v.adoption}%` : ''}
                  </div>
                </div>
                <span className="text-slate-500 shrink-0 w-16 text-right">{v.pageViews.toLocaleString()} views</span>
                {v.crashRate !== '0.00' && (
                  <span className="text-crit shrink-0 text-[10px]">💥 {v.crashRate}% crash</span>
                )}
              </div>
            )
          })}
        </div>
      </div>

      {/* Crash & error table by version */}
      <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
        <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                        bg-surface-2 border-b border-surface-3 px-4 py-2"
          style={{ gridTemplateColumns: '1fr 80px 80px 80px 80px 90px' }}>
          <span>Version</span>
          <span className="text-right">Page views</span>
          <span className="text-right">Adoption</span>
          <span className="text-right">Errors</span>
          <span className="text-right">Crashes</span>
          <span className="text-right">Crash rate</span>
        </div>
        {versions.map(v => (
          <div key={v.version}
            className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
            style={{ gridTemplateColumns: '1fr 80px 80px 80px 80px 90px' }}>
            <div className="flex items-center gap-2">
              <span className={clsx('font-mono', v.version === latestVersion ? 'text-brand font-medium' : 'text-slate-300')}>
                {v.version}
              </span>
              {v.version === latestVersion && (
                <span className="text-[9px] bg-brand/15 text-brand px-1.5 py-0.5 rounded-full">latest</span>
              )}
            </div>
            <span className="text-right text-slate-400">{v.pageViews.toLocaleString()}</span>
            <span className="text-right text-slate-400">{v.adoption}%</span>
            <span className={clsx('text-right', v.errors > 0 ? 'text-warn' : 'text-slate-600')}>{v.errors}</span>
            <span className={clsx('text-right', v.crashes > 0 ? 'text-crit font-medium' : 'text-slate-600')}>{v.crashes}</span>
            <span className={clsx('text-right font-mono', parseFloat(v.crashRate) > 1 ? 'text-crit' : 'text-slate-500')}>
              {v.crashRate}%
            </span>
          </div>
        ))}
      </div>

      {/* Setup snippet */}
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs font-medium text-slate-400 mb-2">Crash reporting setup</div>
        <pre className="text-[10px] font-mono text-slate-400 leading-relaxed overflow-x-auto">{`// In your app entry point:
ObserveX.init({ appId: 'my-app', appVersion: '${latestVersion}',
  ingestor: 'https://ingestor.example.com' })

// In React error boundary:
componentDidCatch(err, info) {
  ObserveX.crash(err, { component: info.componentStack })
}

// After a service worker update:
ObserveX.setAppVersion(newVersion)`}</pre>
      </div>
    </div>
  )
}

// ── Tab: Web Vitals ────────────────────────────────────────────────────────────

function VitalsTab({ range }: { range: number }) {
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-5 gap-3">
        {VITALS.map(v => <VitalGauge key={v} name={v} range={range} />)}
      </div>
      <div className="grid grid-cols-2 gap-3">
        {['LCP', 'FCP'].map(v => (
          <div key={v} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
            <div className="text-xs font-medium text-slate-400 mb-2">{v} over time (p75)</div>
            <VitalTrendChart name={v} range={range} />
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Tab: Errors ────────────────────────────────────────────────────────────────

function ErrorsTab({ range }: { range: number }) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range
  const step  = Math.max(60, Math.floor(range / 60))

  const { data: errTypes } = useQuery({
    queryKey: ['rum-err-types', range],
    queryFn: () => metrics.queryRange(
      `sum by (error_type) (increase(rum_js_errors_total[${step}s]))`,
      start, now, String(step)
    ),
    refetchInterval: 60_000,
  })

  const typeData = useMemo(() => {
    const result = (errTypes as any)?.result ?? []
    return result.map((s: any) => ({
      type:  s.metric?.error_type || 'unknown',
      count: Math.round((s.values ?? []).reduce((a: number, [, v]: [number, string]) => a + (parseFloat(v) || 0), 0)),
    })).filter((d: any) => d.count > 0).sort((a: any, b: any) => b.count - a.count)
  }, [errTypes])

  const COLORS = ['#ef4444', '#f97316', '#f59e0b', '#6366f1', '#3b82f6', '#10b981']

  return (
    <div className="space-y-4">
      {typeData.length > 0 && (
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="text-xs font-medium text-slate-400 mb-3">Errors by type</div>
          <ResponsiveContainer width="100%" height={160}>
            <BarChart data={typeData} barSize={20}>
              <XAxis dataKey="type" tick={{ fill: 'hsl(215 20% 50%)', fontSize: 10 }} tickLine={false} />
              <YAxis hide />
              <Tooltip contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', fontSize: 11, borderRadius: 8 }} />
              <Bar dataKey="count" radius={[4, 4, 0, 0]}>
                {typeData.map((_: any, i: number) => (
                  <Cell key={i} fill={COLORS[i % COLORS.length]} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}
      <div className="bg-surface-1 border border-surface-3 rounded-xl">
        <div className="flex items-center justify-between px-4 py-3 border-b border-surface-3">
          <span className="text-xs font-medium text-slate-400">Recent JS errors</span>
        </div>
        <ErrorLog range={range} />
      </div>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

export default function RUMPage() {
  const [range, setRange] = useState(3600)
  const [tab, setTab]     = useState<ActiveTab>('vitals')

  const now   = Math.floor(Date.now() / 1000)
  const start = now - range

  const { data: pvData } = useQuery({
    queryKey: ['rum-pv', range],
    queryFn: () => metrics.query(`sum(increase(rum_page_load_ms_count[${range}s]))`),
    refetchInterval: 60_000,
  })
  const { data: errData } = useQuery({
    queryKey: ['rum-err', range],
    queryFn: () => metrics.query(`sum(increase(rum_js_errors_total[${range}s]))`),
    refetchInterval: 30_000,
  })
  const { data: sessionData } = useQuery({
    queryKey: ['rum-sessions', range],
    queryFn: () => metrics.query(`count(sum by (session_id) (increase(rum_page_load_ms_count[${range}s])))`),
    refetchInterval: 60_000,
  })
  const { data: crashData } = useQuery({
    queryKey: ['rum-crashes-total', range],
    queryFn: () => metrics.query(`sum(increase(rum_js_errors_total{error_type="crash"}[${range}s]))`),
    refetchInterval: 60_000,
  })

  const pv      = Math.round(parseFloat((pvData as any)?.data?.result?.[0]?.value?.[1] ?? '0'))
  const errs    = Math.round(parseFloat((errData as any)?.data?.result?.[0]?.value?.[1] ?? '0'))
  const sessions = Math.round(parseFloat((sessionData as any)?.data?.result?.[0]?.value?.[1] ?? '0'))
  const crashes  = Math.round(parseFloat((crashData as any)?.data?.result?.[0]?.value?.[1] ?? '0'))

  const TABS: { id: ActiveTab; label: string }[] = [
    { id: 'vitals',   label: 'Web Vitals' },
    { id: 'journeys', label: 'User Journeys' },
    { id: 'versions', label: 'App Versions' },
    { id: 'errors',   label: `Errors${errs > 0 ? ` (${errs})` : ''}` },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Real User Monitoring"
        subtitle="Web Vitals · User journeys · App versions · Crash analytics"
        actions={
          <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
            {TIME_RANGES.map(r => (
              <button key={r.label} onClick={() => setRange(r.seconds)}
                className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                  range === r.seconds ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                {r.label}
              </button>
            ))}
          </div>
        }
      />

      {/* Summary stats */}
      <div className="grid grid-cols-4 gap-3 px-4 pt-4">
        {[
          { label: 'Page views',  value: pv.toLocaleString(),      color: 'text-brand',   icon: Globe },
          { label: 'Sessions',    value: sessions.toLocaleString(), color: 'text-slate-200', icon: Users },
          { label: 'JS errors',   value: errs.toLocaleString(),    color: errs > 0 ? 'text-warn' : 'text-ok', icon: AlertTriangle },
          { label: 'Crashes',     value: crashes.toLocaleString(), color: crashes > 0 ? 'text-crit' : 'text-ok', icon: AlertTriangle },
        ].map(({ label, value, color, icon: Icon }) => (
          <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-3">
            <div className="flex items-center justify-between mb-1">
              <span className="text-xs text-slate-500">{label}</span>
              <Icon size={12} className="text-slate-600" />
            </div>
            <div className={clsx('text-xl font-semibold font-display', color)}>{value || '—'}</div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <div className="flex border-b border-surface-3 px-4 mt-4 flex-shrink-0">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            {t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {tab === 'vitals'   && <VitalsTab range={range} />}
        {tab === 'journeys' && <JourneysTab range={range} />}
        {tab === 'versions' && <VersionsTab range={range} />}
        {tab === 'errors'   && <ErrorsTab range={range} />}
      </div>
    </div>
  )
}
