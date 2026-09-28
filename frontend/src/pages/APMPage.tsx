// frontend/src/pages/APMPage.tsx
// Application Performance Monitoring — Apdex, RED metrics, transactions, error fingerprints
// Data from: /api/v1/apm/services (VictoriaMetrics-backed Apdex derivation)

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apm, metrics } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, StatusDot } from '@/components/shared/Layout'
import { Activity, AlertTriangle, BarChart2 } from 'lucide-react'
import {
  AreaChart, Area, LineChart, Line,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend
} from 'recharts'
import { format, subMinutes } from 'date-fns'
import clsx from 'clsx'

const TOOLTIP = {
  contentStyle: {
    background: 'hsl(220 11% 12%)',
    border: '1px solid hsl(220 9% 21%)',
    borderRadius: 8,
    fontSize: 11,
  }
}

// ── Apdex gauge SVG ──────────────────────────────────────────────────────────

function ApdexGauge({ score }: { score: number }) {
  const clamped = Math.max(0, Math.min(1, score))
  const color = clamped >= 0.95 ? '#22c55e' : clamped >= 0.75 ? '#f59e0b' : '#ef4444'
  const label = clamped >= 0.95 ? 'Excellent' : clamped >= 0.85 ? 'Good' : clamped >= 0.7 ? 'Fair' : 'Poor'

  // Arc from -210° to 30° (240° sweep)
  const toRad = (d: number) => (d * Math.PI) / 180
  const cx = 60, cy = 60, r = 44
  const startDeg = -210
  const endDeg = startDeg + clamped * 240
  const arcPath = (s: number, e: number) => {
    const sx = cx + r * Math.cos(toRad(s)), sy = cy + r * Math.sin(toRad(s))
    const ex = cx + r * Math.cos(toRad(e)), ey = cy + r * Math.sin(toRad(e))
    const large = Math.abs(e - s) > 180 ? 1 : 0
    return `M ${sx} ${sy} A ${r} ${r} 0 ${large} 1 ${ex} ${ey}`
  }

  return (
    <div className="flex flex-col items-center">
      <svg width="120" height="80" viewBox="0 0 120 80">
        <path d={arcPath(-210, 30)} fill="none" stroke="hsl(220 9% 18%)" strokeWidth="10" strokeLinecap="round" />
        <path d={arcPath(startDeg, endDeg)} fill="none" stroke={color} strokeWidth="10" strokeLinecap="round" />
        <text x="60" y="62" textAnchor="middle" fill={color} fontSize="20" fontWeight="800"
          fontFamily="JetBrains Mono,monospace">{clamped.toFixed(2)}</text>
      </svg>
      <div className={clsx('text-xs font-semibold',
        clamped >= 0.95 ? 'text-ok' : clamped >= 0.75 ? 'text-warn' : 'text-crit')}>
        {label}
      </div>
      <div className="text-[10px] text-slate-600 mt-0.5">T=500ms threshold</div>
    </div>
  )
}

// ── Service APM row ──────────────────────────────────────────────────────────

function ServiceRow({ svc, selected, onSelect }: {
  svc: any; selected: boolean; onSelect: () => void
}) {
  return (
    <div onClick={onSelect}
      className={clsx('grid items-center px-4 py-3 border-b border-surface-3/50 text-xs cursor-pointer transition-colors hover:bg-surface-2/30',
        selected && 'bg-brand/5 border-brand/20',
        svc.status === 'critical' && 'bg-crit/3')}>
      <div style={{ gridTemplateColumns: '180px 1fr 100px 90px 90px 80px', display: 'grid', gap: '8px', alignItems: 'center' }}>
        <div className="flex items-center gap-2">
          <StatusDot status={svc.status === 'good' ? 'GOOD' : svc.status === 'degraded' ? 'DEGRADED' : 'CRITICAL'} />
          <span className="font-semibold text-slate-200 font-mono truncate">{svc.name}</span>
        </div>
        <ApdexGauge score={svc.apdex ?? 0} />
        <div className="text-center">
          <div className="text-[10px] text-slate-500">RPM</div>
          <div className="text-sm font-semibold font-mono text-slate-200">{(svc.rpm ?? 0).toLocaleString()}</div>
        </div>
        <div className="text-center">
          <div className="text-[10px] text-slate-500">P99</div>
          <div className={clsx('text-sm font-semibold font-mono',
            svc.p99_ms > 500 ? 'text-crit' : svc.p99_ms > 200 ? 'text-warn' : 'text-ok')}>
            {(svc.p99_ms ?? 0).toFixed(0)}ms
          </div>
        </div>
        <div className="text-center">
          <div className="text-[10px] text-slate-500">Errors</div>
          <div className={clsx('text-sm font-semibold font-mono',
            svc.error_rate_pct > 5 ? 'text-crit' : svc.error_rate_pct > 1 ? 'text-warn' : 'text-ok')}>
            {(svc.error_rate_pct ?? 0).toFixed(2)}%
          </div>
        </div>
        <div className="text-[10px] text-slate-500">{svc.namespace}</div>
      </div>
    </div>
  )
}

// ── Throughput chart using real VictoriaMetrics data ─────────────────────────

function ThroughputChart() {
  const now   = Math.floor(Date.now() / 1000)
  const range = 3600

  const { data: rpmData } = useQuery({
    queryKey: ['apm-rpm'],
    queryFn: () => metrics.queryRange(
      'sum(rate(http_requests_total[1m])) * 60',
      now - range, now, '60'
    ),
    refetchInterval: 30_000,
  })
  const { data: errData } = useQuery({
    queryKey: ['apm-errors'],
    queryFn: () => metrics.queryRange(
      'sum(rate(http_requests_total{status_code=~"5.."}[1m])) * 60',
      now - range, now, '60'
    ),
    refetchInterval: 30_000,
  })

  const rpmPts = (rpmData?.result?.[0]?.values ?? []).map((pt: any) => ({
    t: format(new Date(Number(pt.t) * 1000), 'HH:mm'),
    rpm: Math.round(parseFloat(pt.v as string) || 0),
  }))
  const errPts = (errData?.result?.[0]?.values ?? []).map((pt: any) => ({
    t: format(new Date(Number(pt.t) * 1000), 'HH:mm'),
    errors: Math.round(parseFloat(pt.v as string) || 0),
  }))

  // Merge by timestamp
  const merged = rpmPts.map((pt, i) => ({
    t: pt.t,
    rpm: (pt as any).rpm ?? 0,
    errors: errPts[i]?.errors ?? 0,
  }))

  if (!merged.length) return null

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4 mb-3 flex-shrink-0">
      <div className="text-xs font-medium text-slate-400 mb-3">Request rate &amp; errors (1h)</div>
      <ResponsiveContainer width="100%" height={80}>
        <AreaChart data={merged}>
          <defs>
            <linearGradient id="rpmGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.25} />
              <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
            </linearGradient>
          </defs>
          <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
          <YAxis tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} width={45} />
          <Tooltip {...TOOLTIP} />
          <Area type="monotone" dataKey="rpm" stroke="#3b82f6" fill="url(#rpmGrad)" strokeWidth={1.5} dot={false} name="RPM" />
          <Line type="monotone" dataKey="errors" stroke="#ef4444" strokeWidth={1.5} dot={false} name="Errors/min" />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

// ── Main ─────────────────────────────────────────────────────────────────────

type Tab = 'services' | 'transactions' | 'errors'

export default function APMPage() {
  const [tab, setTab]         = useState<Tab>('services')
  const [selected, setSelected] = useState<string | null>(null)

  const { data: svcData, isLoading: svcLoading } = useQuery({
    queryKey: ['apm-services'],
    queryFn: () => apm.services({ limit: 50 }),
    refetchInterval: 30_000,
  })
  const { data: txData, isLoading: txLoading } = useQuery({
    queryKey: ['apm-transactions', selected],
    queryFn: () => apm.transactions(selected ?? '', { limit: 20 }),
    refetchInterval: 30_000,
    enabled: tab === 'transactions',
  })
  const { data: errData, isLoading: errLoading } = useQuery({
    queryKey: ['apm-errors', selected],
    queryFn: () => apm.errors(selected ?? ''),
    refetchInterval: 30_000,
    enabled: tab === 'errors',
  })

  const services: any[]     = svcData?.services ?? []
  const transactions: any[] = txData?.transactions ?? []
  const errorProblems: any[] = errData?.problems ?? []

  const totalRPM   = services.reduce((s, sv) => s + (sv.rpm ?? 0), 0)
  const avgApdex   = services.length ? services.reduce((s, sv) => s + (sv.apdex ?? 1), 0) / services.length : 1
  const avgP99     = services.length ? services.reduce((s, sv) => s + (sv.p99_ms ?? 0), 0) / services.length : 0
  const avgErrRate = services.length ? services.reduce((s, sv) => s + (sv.error_rate_pct ?? 0), 0) / services.length : 0

  const TABS: { id: Tab; label: string }[] = [
    { id: 'services',     label: `Services (${services.length})` },
    { id: 'transactions', label: 'Top transactions' },
    { id: 'errors',       label: 'Error fingerprints' },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="APM"
        subtitle="Apdex · RED metrics · top transactions · error fingerprints · VictoriaMetrics-backed"
      />

      {/* Summary row */}
      <div className="grid grid-cols-5 gap-3 p-4 pb-0 flex-shrink-0">
        {[
          { label: 'Total RPM',  value: totalRPM.toLocaleString(),   color: 'text-slate-200' },
          { label: 'Avg Apdex',  value: avgApdex.toFixed(2),          color: avgApdex < 0.7 ? 'text-crit' : avgApdex < 0.85 ? 'text-warn' : 'text-ok' },
          { label: 'Avg P99',    value: `${avgP99.toFixed(0)}ms`,      color: avgP99 > 500 ? 'text-crit' : avgP99 > 200 ? 'text-warn' : 'text-ok' },
          { label: 'Error rate', value: `${avgErrRate.toFixed(2)}%`,   color: avgErrRate > 5 ? 'text-crit' : avgErrRate > 1 ? 'text-warn' : 'text-ok' },
          { label: 'Services',   value: services.length,               color: 'text-slate-200' },
        ].map(({ label, value, color }) => (
          <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-3">
            <div className="text-[10px] text-slate-500 mb-1">{label}</div>
            <div className={clsx('text-xl font-semibold font-mono', color)}>{value}</div>
          </div>
        ))}
      </div>

      {/* Throughput chart */}
      <div className="px-4 pt-3 flex-shrink-0">
        <ThroughputChart />
      </div>

      {/* Tabs */}
      <div className="flex border-b border-surface-3 px-4 flex-shrink-0">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            {t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* Services */}
        {tab === 'services' && (
          <>
            <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
              style={{ gridTemplateColumns: '180px 1fr 100px 90px 90px 80px', gap: '8px' }}>
              <span>Service</span><span>Apdex</span><span>RPM</span><span>P99</span><span>Error %</span><span>NS</span>
            </div>
            {svcLoading ? (
              <div className="flex justify-center py-12"><Spinner size={20} /></div>
            ) : services.length === 0 ? (
              <EmptyState icon={BarChart2} title="No APM data yet"
                description="APM data is derived from http_request_duration_seconds histogram metrics. Instrument your services with OTLP." />
            ) : (
              services.map(svc => (
                <ServiceRow key={svc.id} svc={svc}
                  selected={selected === svc.id}
                  onSelect={() => setSelected(selected === svc.id ? null : svc.id)} />
              ))
            )}
          </>
        )}

        {/* Transactions */}
        {tab === 'transactions' && (
          <div className="p-4">
            {!selected && (
              <div className="text-xs text-slate-500 mb-4 bg-surface-2 border border-surface-3 rounded-lg px-3 py-2">
                Select a service from the Services tab to see its top transactions
              </div>
            )}
            {txLoading ? <Spinner /> : (
              <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
                <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
                  style={{ gridTemplateColumns: '1fr 100px 70px 80px 70px' }}>
                  <span>Endpoint</span><span>Service</span><span>RPM</span><span>P99</span><span>Error %</span>
                </div>
                {transactions.map((t: any, i: number) => (
                  <div key={i} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
                    style={{ gridTemplateColumns: '1fr 100px 70px 80px 70px' }}>
                    <span className="font-mono text-slate-200 truncate">{t.endpoint || '—'}</span>
                    <span className="text-slate-500 font-mono text-[10px] truncate">{t.service_name}</span>
                    <span className="text-slate-400 font-mono">{(t.rpm ?? 0).toLocaleString()}</span>
                    <span className={clsx('font-mono font-semibold',
                      t.p99_ms > 500 ? 'text-crit' : t.p99_ms > 200 ? 'text-warn' : 'text-ok')}>
                      {(t.p99_ms ?? 0).toFixed(0)}ms
                    </span>
                    <span className={clsx('font-mono',
                      t.error_pct > 5 ? 'text-crit' : t.error_pct > 1 ? 'text-warn' : 'text-ok')}>
                      {(t.error_pct ?? 0).toFixed(2)}%
                    </span>
                  </div>
                ))}
                {!transactions.length && (
                  <div className="text-center py-8 text-sm text-slate-600">
                    No transaction data — instrument endpoints with http_request_duration_seconds histogram
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {/* Error fingerprints */}
        {tab === 'errors' && (
          <div className="p-4">
            {errLoading ? <Spinner /> : (
              <div className="space-y-2">
                {errorProblems.map((p: any) => (
                  <div key={p.id} className="bg-surface-1 border border-crit/20 rounded-xl p-4">
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex-1">
                        <div className="flex items-center gap-2 mb-1">
                          <span className="text-[10px] font-mono bg-crit/15 text-crit px-1.5 py-0.5 rounded">
                            {String(p.class).replace(/_/g, ' ')}
                          </span>
                          <span className="text-[10px] font-mono text-slate-500">{p.service_id}</span>
                        </div>
                        <div className="text-xs font-mono text-slate-200">{p.title}</div>
                      </div>
                      <span className={clsx('text-xs font-bold ml-4 flex-shrink-0',
                        p.severity === 'CRITICAL' ? 'text-crit' : 'text-warn')}>
                        {p.severity}
                      </span>
                    </div>
                    {p.detail && <div className="text-[11px] text-slate-400 mb-2">{p.detail}</div>}
                    {p.evidence?.length > 0 && (
                      <div className="bg-surface-2 border border-surface-3 rounded px-3 py-2 text-[10px] font-mono text-slate-400 space-y-0.5">
                        {p.evidence.map((e: string, i: number) => <div key={i}>{e}</div>)}
                      </div>
                    )}
                  </div>
                ))}
                {!errorProblems.length && (
                  <EmptyState icon={AlertTriangle} title="No error fingerprints"
                    description="Error fingerprints appear when problems are detected for the selected service." />
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
