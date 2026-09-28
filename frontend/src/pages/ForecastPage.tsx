// frontend/src/pages/ForecastPage.tsx
// S21 — Advanced AI & Predictive Analytics
//
// Displays ML-driven forecasts for key operational metrics:
//   - Cluster-wide capacity forecast (CPU, memory, latency, error rate)
//   - Per-service metric trend extrapolation
//   - Self-healing recommendations when breach is predicted
//   - Traffic spike prediction using Holt-Winters smoothing
//
// Data source: GET /api/v1/forecasts (processor ForecastEngine)
// Method: Linear regression → Holt-Winters → Seasonal decomposition
//         depending on sample count (5 / 30 / 72 samples)

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { forecast as forecastApi, topology } from '@/lib/api'
import type { ServiceForecast, CapacityForecast } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, Btn } from '@/components/shared/Layout'
import {
  TrendingUp, TrendingDown, Minus, AlertTriangle,
  CheckCircle, Activity, ChevronDown, ChevronUp, Cpu
} from 'lucide-react'
import {
  AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer,
  CartesianGrid, ReferenceLine, Legend,
} from 'recharts'
import clsx from 'clsx'

// ── Types ──────────────────────────────────────────────────────────────────────

const METRIC_LABELS: Record<string, string> = {
  container_cpu_usage_percent:      'CPU usage',
  container_memory_working_set_pct: 'Memory usage',
  trace_latency_p99_ms:             'P99 latency',
  trace_error_rate:                 'Error rate',
  kafka_consumer_lag:               'Queue lag',
}

const METRIC_UNITS: Record<string, string> = {
  container_cpu_usage_percent:      '%',
  container_memory_working_set_pct: '%',
  trace_latency_p99_ms:             'ms',
  trace_error_rate:                 '',
  kafka_consumer_lag:               ' msgs',
}

const METRIC_WARN: Record<string, number> = {
  container_cpu_usage_percent:      75,
  container_memory_working_set_pct: 80,
  trace_latency_p99_ms:             1000,
  trace_error_rate:                 0.05,
}

const METRIC_CRIT: Record<string, number> = {
  container_cpu_usage_percent:      90,
  container_memory_working_set_pct: 90,
  trace_latency_p99_ms:             3000,
  trace_error_rate:                 0.15,
}

// ── Components ─────────────────────────────────────────────────────────────────

function TrendIcon({ trend, pct }: { trend: string; pct: number }) {
  if (trend === 'rising')  return <TrendingUp  size={13} className={pct > 20 ? 'text-crit' : pct > 5 ? 'text-warn' : 'text-slate-400'} />
  if (trend === 'falling') return <TrendingDown size={13} className="text-ok" />
  return <Minus size={13} className="text-slate-500" />
}

function AlertBadge({ alert }: { alert?: string }) {
  if (!alert) return null
  return (
    <span className={clsx('text-[10px] font-semibold px-1.5 py-0.5 rounded',
      alert === 'critical' ? 'bg-crit/20 text-crit' : 'bg-warn/20 text-warn')}>
      {alert === 'critical' ? '🔴 Critical' : '🟡 Warning'}
    </span>
  )
}

function ForecastChart({ f }: { f: ServiceForecast }) {
  const warn = METRIC_WARN[f.metric]
  const crit = METRIC_CRIT[f.metric]
  const unit = METRIC_UNITS[f.metric] ?? ''

  const chartData = f.points.map(p => ({
    t:     `+${p.offset_min}m`,
    value: p.value,
    lower: p.lower_95,
    upper: p.upper_95,
  }))

  const maxVal = Math.max(...f.points.map(p => p.upper_95), crit ?? 0) * 1.1
  const alertColor = f.alert === 'critical' ? '#ef4444' : f.alert === 'warning' ? '#f97316' : '#3b82f6'

  return (
    <ResponsiveContainer width="100%" height={140}>
      <AreaChart data={chartData} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={`fg-${f.service_id}-${f.metric}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%"  stopColor={alertColor} stopOpacity={0.3} />
            <stop offset="95%" stopColor={alertColor} stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 15%)" vertical={false} />
        <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 40%)', fontSize: 9 }} tickLine={false} />
        <YAxis domain={[0, maxVal]} tick={{ fill: 'hsl(215 20% 40%)', fontSize: 9 }} tickLine={false} width={32}
          tickFormatter={v => `${v.toFixed(0)}${unit}`} />
        <Tooltip
          contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', fontSize: 10, borderRadius: 8 }}
          formatter={(v: number) => [`${v.toFixed(1)}${unit}`, '']} />
        {/* Confidence interval */}
        <Area type="monotone" dataKey="upper" stroke="none" fill={alertColor} fillOpacity={0.1} stackId="ci" />
        <Area type="monotone" dataKey="lower" stroke="none" fill="hsl(220 11% 12%)" fillOpacity={1} stackId="ci" />
        {/* Forecast line */}
        <Area type="monotone" dataKey="value" stroke={alertColor}
          fill={`url(#fg-${f.service_id}-${f.metric})`} strokeWidth={2} dot={false} />
        {warn != null && <ReferenceLine y={warn} stroke="#f97316" strokeDasharray="4 2" strokeWidth={1} label={{ value: 'warn', position: 'right', fontSize: 9, fill: '#f97316' }} />}
        {crit != null && <ReferenceLine y={crit} stroke="#ef4444" strokeDasharray="4 2" strokeWidth={1} label={{ value: 'crit', position: 'right', fontSize: 9, fill: '#ef4444' }} />}
      </AreaChart>
    </ResponsiveContainer>
  )
}

function ForecastCard({ f }: { f: ServiceForecast }) {
  const [expanded, setExpanded] = useState(false)
  const lastPt = f.points[f.points.length - 1]
  const unit   = METRIC_UNITS[f.metric] ?? ''
  const label  = METRIC_LABELS[f.metric] ?? f.metric

  return (
    <div className={clsx(
      'bg-surface-1 border rounded-xl overflow-hidden transition-colors',
      f.alert === 'critical' ? 'border-crit/40' : f.alert === 'warning' ? 'border-warn/30' : 'border-surface-3'
    )}>
      <div className="p-4">
        <div className="flex items-start justify-between gap-2 mb-3">
          <div className="min-w-0">
            <div className="text-sm font-medium text-slate-200 truncate">{f.service_name}</div>
            <div className="text-xs text-slate-500 mt-0.5">{label}</div>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <AlertBadge alert={f.alert} />
            <TrendIcon trend={f.trend} pct={f.trend_pct} />
          </div>
        </div>

        <div className="flex items-baseline gap-2 mb-3">
          {lastPt && (
            <span className={clsx('text-xl font-semibold font-mono',
              f.alert === 'critical' ? 'text-crit' : f.alert === 'warning' ? 'text-warn' : 'text-slate-200')}>
              {lastPt.value.toFixed(1)}{unit}
            </span>
          )}
          <span className={clsx('text-xs', f.trend_pct > 0 ? 'text-warn' : 'text-ok')}>
            {f.trend_pct > 0 ? '+' : ''}{f.trend_pct.toFixed(1)}% in {f.horizon}
          </span>
        </div>

        <ForecastChart f={f} />

        <div className="flex items-center justify-between mt-2">
          <span className="text-[10px] text-slate-700 capitalize">method: {f.method.replace('_', '-')} · {f.sample_count} samples</span>
          {f.suggestions && f.suggestions.length > 0 && (
            <button onClick={() => setExpanded(!expanded)}
              className="text-[10px] text-brand flex items-center gap-0.5 hover:text-brand-light">
              {f.suggestions.length} suggestion{f.suggestions.length !== 1 ? 's' : ''}
              {expanded ? <ChevronUp size={10} /> : <ChevronDown size={10} />}
            </button>
          )}
        </div>
      </div>

      {expanded && f.suggestions && (
        <div className="border-t border-surface-3 bg-surface-2/30 p-3 space-y-1.5">
          {f.suggestions.map((s, i) => (
            <div key={i} className="flex gap-2 text-xs text-slate-400">
              <span className="text-brand shrink-0 mt-0.5">→</span>
              <span>{s}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Capacity overview ─────────────────────────────────────────────────────────

function CapacityOverview({ data }: { data: CapacityForecast }) {
  const total = data.healthy_count + data.warning_count + data.critical_count
  return (
    <div className="grid grid-cols-4 gap-3">
      {[
        { label: 'Services monitored', value: total,               icon: Activity,   color: 'text-slate-200' },
        { label: 'Healthy',            value: data.healthy_count,  icon: CheckCircle, color: 'text-ok' },
        { label: 'Warning',            value: data.warning_count,  icon: AlertTriangle, color: 'text-warn' },
        { label: 'Critical forecast',  value: data.critical_count, icon: AlertTriangle, color: 'text-crit', alert: data.critical_count > 0 },
      ].map(({ label, value, icon: Icon, color, alert }) => (
        <div key={label} className={clsx(
          'bg-surface-1 border rounded-xl p-4',
          alert ? 'border-crit/40 bg-crit/5' : 'border-surface-3'
        )}>
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs text-slate-500">{label}</span>
            <Icon size={13} className={color} />
          </div>
          <div className={clsx('text-2xl font-semibold font-display', color)}>{value}</div>
        </div>
      ))}
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────────

const HORIZON_OPTIONS = ['15m', '30m', '1h', '4h']

export default function ForecastPage() {
  const [horizon,     setHorizon]     = useState('30m')
  const [filterAlert, setFilterAlert] = useState<'all' | 'warning' | 'critical'>('all')
  const [selectedSvc, setSelectedSvc] = useState<string>('')

  // Cluster-wide capacity forecast
  const { data: capacity, isLoading, error } = useQuery({
    queryKey: ['capacity-forecast', horizon],
    queryFn: () => forecastApi.capacity(horizon),
    refetchInterval: 120_000,
    staleTime: 90_000,
  })

  // Service list for per-service selector
  const { data: svcData } = useQuery({
    queryKey: ['services'],
    queryFn: () => topology.services(),
    staleTime: 300_000,
  })
  const services = (svcData as any)?.services ?? []

  // Per-service custom forecast
  const { data: svcForecast } = useQuery({
    queryKey: ['svc-forecast', selectedSvc, horizon],
    queryFn: () => forecastApi.metric(selectedSvc, 'container_cpu_usage_percent', horizon),
    enabled: !!selectedSvc,
    refetchInterval: 120_000,
  })

  const forecasts: ServiceForecast[] = (capacity?.forecasts ?? []).filter(f => {
    if (filterAlert === 'all') return true
    return f.alert === filterAlert
  })

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Predictive Analytics"
        subtitle="Capacity forecasting · Traffic spike prediction · Self-healing recommendations"
        actions={
          <div className="flex items-center gap-2">
            <span className="text-xs text-slate-500">Horizon</span>
            <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
              {HORIZON_OPTIONS.map(h => (
                <button key={h} onClick={() => setHorizon(h)}
                  className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                    horizon === h ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                  {h}
                </button>
              ))}
            </div>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {isLoading && (
          <div className="flex flex-col items-center gap-3 py-20">
            <Spinner size={24} />
            <p className="text-sm text-slate-500">Computing forecasts…</p>
            <p className="text-xs text-slate-700">Waiting for metric windows to accumulate (need ≥5 samples)</p>
          </div>
        )}

        {error && (
          <div className="flex flex-col items-center gap-3 py-16 text-center">
            <EmptyState icon={Cpu} title="No forecast data yet"
              description="The forecasting engine needs at least 5 metric samples per service. Make sure the OneAgent is running and services are being scraped." />
          </div>
        )}

        {capacity && !isLoading && (
          <>
            <CapacityOverview data={capacity} />

            {capacity.at_risk_services.length > 0 && (
              <div className="bg-crit/10 border border-crit/30 rounded-xl px-4 py-3 flex items-start gap-3">
                <AlertTriangle size={14} className="text-crit shrink-0 mt-0.5" />
                <div>
                  <div className="text-sm font-medium text-crit">Capacity risk detected</div>
                  <div className="text-xs text-slate-400 mt-0.5">
                    The following services are forecast to breach thresholds within {horizon}:{' '}
                    <span className="text-slate-300">{capacity.at_risk_services.join(', ')}</span>
                  </div>
                </div>
              </div>
            )}

            {/* Filters */}
            <div className="flex items-center gap-2">
              {(['all', 'warning', 'critical'] as const).map(f => (
                <Btn key={f} size="xs"
                  variant={filterAlert === f ? 'primary' : 'ghost'}
                  onClick={() => setFilterAlert(f)}>
                  {f === 'all' ? `All (${capacity.forecasts.length})` : f === 'critical' ? `🔴 Critical (${capacity.critical_count})` : `🟡 Warning (${capacity.warning_count})`}
                </Btn>
              ))}
              {services.length > 0 && (
                <select value={selectedSvc} onChange={e => setSelectedSvc(e.target.value)}
                  className="ml-auto px-2.5 py-1.5 text-xs bg-surface-1 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
                  <option value="">Custom service…</option>
                  {services.slice(0, 50).map((s: any) => (
                    <option key={s.id} value={s.id}>{s.display_name ?? s.name}</option>
                  ))}
                </select>
              )}
            </div>

            {/* Custom service forecast */}
            {svcForecast && (
              <div>
                <div className="text-xs font-medium text-slate-500 mb-2">Custom forecast</div>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                  <ForecastCard f={svcForecast} />
                </div>
              </div>
            )}

            {/* Forecast grid */}
            {forecasts.length === 0 ? (
              <div className="text-center py-12 text-sm text-slate-600">
                No forecasts match the current filter
              </div>
            ) : (
              <>
                <div className="text-xs font-medium text-slate-500">
                  Showing {forecasts.length} forecast{forecasts.length !== 1 ? 's' : ''} · {horizon} horizon · method: {forecasts[0]?.method ?? '—'}
                </div>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
                  {forecasts.map((f, i) => (
                    <ForecastCard key={`${f.service_id}-${f.metric}-${i}`} f={f} />
                  ))}
                </div>
              </>
            )}

            {/* Explanation */}
            <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
              <div className="text-xs font-medium text-slate-400 mb-3">How forecasting works</div>
              <div className="grid grid-cols-3 gap-4">
                {[
                  { title: 'Linear regression', badge: '≥5 samples', desc: 'Weighted least-squares fit. Recent values weighted 3× more than older ones. Used when data is sparse.' },
                  { title: 'Holt-Winters', badge: '≥30 samples', desc: 'Double exponential smoothing with trend component. Handles rising/falling trends more stably than regression.' },
                  { title: 'Seasonal decomp.', badge: '≥72 samples', desc: 'Extracts a 12-point (≈3 min) seasonal cycle, fits trend separately, then recombines. Best for cron/traffic patterns.' },
                ].map(({ title, badge, desc }) => (
                  <div key={title}>
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-xs font-medium text-slate-300">{title}</span>
                      <span className="text-[10px] bg-surface-3 text-slate-500 px-1.5 py-0.5 rounded">{badge}</span>
                    </div>
                    <p className="text-[11px] text-slate-600 leading-relaxed">{desc}</p>
                  </div>
                ))}
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
