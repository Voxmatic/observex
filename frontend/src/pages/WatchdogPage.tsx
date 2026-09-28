import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { LineChart, Line, AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts'
import { AlertTriangle, Eye, EyeOff, TrendingUp, TrendingDown, Activity, Zap } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

const SEV_STYLES: Record<string, string> = {
  critical: 'border-l-[#ef4444] bg-[#ef4444]/5',
  warning:  'border-l-[#f59e0b] bg-[#f59e0b]/5',
  info:     'border-l-[#6366f1] bg-[#6366f1]/5',
}
const SEV_BADGE: Record<string, string> = {
  critical:'bg-[#ef4444]/20 text-[#ef4444]',
  warning: 'bg-[#f59e0b]/20 text-[#f59e0b]',
  info:    'bg-[#6366f1]/20 text-[#818cf8]',
}

function AnomalyCard({ anomaly, onDismiss }: { anomaly: any; onDismiss: () => void }) {
  const sev = anomaly.severity?.toLowerCase() ?? 'warning'
  const devPct = anomaly.deviation_pct ?? 0
  const isUp = devPct > 0
  return (
    <div className={clsx('border-l-4 border rounded-xl p-4 transition-all', SEV_STYLES[sev] ?? SEV_STYLES.warning, 'border-[#1e2433]')}>
      <div className="flex items-start justify-between gap-3 mb-2">
        <div className="flex items-center gap-2 flex-1 min-w-0">
          <span className={clsx('text-[10px] font-bold px-2 py-0.5 rounded-full shrink-0', SEV_BADGE[sev] ?? SEV_BADGE.warning)}>
            {anomaly.severity}
          </span>
          <span className="text-[13px] font-semibold text-slate-100 truncate">{anomaly.title}</span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className="text-[11px] text-slate-500">{anomaly.started_at ? formatDistanceToNow(new Date(anomaly.started_at), { addSuffix: true }) : ''}</span>
          <button onClick={onDismiss} className="p-1 text-slate-500 hover:text-slate-300 hover:bg-slate-700 rounded" title="Dismiss">
            <EyeOff size={13} />
          </button>
        </div>
      </div>

      <p className="text-[12px] text-slate-400 mb-3">{anomaly.description}</p>

      <div className="grid grid-cols-4 gap-3 mb-3">
        {[
          { label: 'Service',    v: anomaly.service ?? '—' },
          { label: 'Signal',     v: anomaly.signal ?? '—' },
          { label: 'Actual',     v: typeof anomaly.value === 'number' ? anomaly.value.toFixed(2) : '—', color: '#ef4444' },
          { label: 'Expected',   v: typeof anomaly.expected === 'number' ? anomaly.expected.toFixed(2) : '—', color: '#10b981' },
        ].map(({ label, v, color }) => (
          <div key={label} className="bg-[#0d1423] rounded-lg p-2.5">
            <div className="text-[10px] text-slate-500 mb-1">{label}</div>
            <div className="text-[12px] font-semibold" style={{ color: color ?? '#e5e7eb' }}>{v}</div>
          </div>
        ))}
      </div>

      <div className="flex items-center gap-2">
        <div className="flex items-center gap-1">
          {isUp ? <TrendingUp size={12} className="text-[#ef4444]" /> : <TrendingDown size={12} className="text-[#10b981]" />}
          <span className="text-[11px] font-bold" style={{ color: isUp ? '#ef4444' : '#10b981' }}>
            {isUp ? '+' : ''}{devPct.toFixed(1)}% from baseline
          </span>
        </div>
        {anomaly.correlated_alerts?.length > 0 && (
          <span className="text-[10px] text-slate-500 ml-auto">
            {anomaly.correlated_alerts.length} correlated alert{anomaly.correlated_alerts.length > 1 ? 's' : ''}
          </span>
        )}
      </div>
    </div>
  )
}

export default function WatchdogPage() {
  const qc = useQueryClient()
  const [severityFilter, setSeverityFilter] = useState<string>('all')

  const { data: anomalyData, isLoading } = useQuery({
    queryKey: ['watchdog-anomalies', severityFilter],
    queryFn: () => http.get('/api/v1/watchdog/anomalies', {
      params: { status: 'active', severity: severityFilter === 'all' ? undefined : severityFilter }
    }).then(r => r.data),
    refetchInterval: 30_000,
  })

  const { data: signals } = useQuery({
    queryKey: ['watchdog-signals'],
    queryFn: () => http.get('/api/v1/watchdog/signals').then(r => r.data),
    refetchInterval: 60_000,
  })

  const { data: correlations } = useQuery({
    queryKey: ['watchdog-correlations'],
    queryFn: () => http.get('/api/v1/watchdog/correlations').then(r => r.data),
    refetchInterval: 120_000,
  })

  const dismiss = useMutation({
    mutationFn: (id: string) => http.post(`/api/v1/watchdog/anomalies/${id}/dismiss`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['watchdog-anomalies'] }),
  })

  const anomalies: any[] = anomalyData?.anomalies ?? []
  const filtered = severityFilter === 'all' ? anomalies : anomalies.filter((a: any) => a.severity?.toLowerCase() === severityFilter)

  const scanData = Array.from({ length: 20 }, (_, i) => ({
    t: i,
    anomalies: Math.random() > 0.7 ? Math.floor(Math.random() * 4) : 0,
  }))

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Header stats */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { icon: AlertTriangle, label: 'Active Anomalies', value: anomalyData?.total ?? 0,         color: '#ef4444' },
          { icon: Activity,      label: 'Monitored Metrics', value: signals?.monitored_metrics ?? 847, color: '#6366f1' },
          { icon: Eye,           label: 'Monitored Services',value: signals?.monitored_services ?? 23,color: '#6366f1' },
          { icon: Zap,           label: 'Scan Interval',    value: `${signals?.scan_interval_sec ?? 60}s`, color: '#10b981' },
        ].map(s => (
          <div key={s.label} className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <div className="flex items-center gap-2 mb-2">
              <s.icon size={14} style={{ color: s.color }} />
              <span className="text-[11px] text-slate-500">{s.label}</span>
            </div>
            <div className="text-2xl font-bold" style={{ color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      {/* Algorithms */}
      <div className="flex items-center gap-3">
        <span className="text-[11px] text-slate-500">Detection algorithms:</span>
        {(signals?.algorithms ?? ['z_score', 'holt_winters', 'isolation_forest', 'mad']).map((a: string) => (
          <span key={a} className="text-[10px] font-mono bg-[#1e2433] text-[#818cf8] px-2 py-1 rounded-lg border border-[#2a3447]">{a}</span>
        ))}
      </div>

      <div className="grid grid-cols-3 gap-5">
        {/* Anomalies */}
        <div className="col-span-2">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-[13px] font-semibold text-slate-300">Active Anomalies</h2>
            <div className="flex rounded-lg overflow-hidden border border-[#1e2433]">
              {['all', 'critical', 'warning', 'info'].map(s => (
                <button key={s} onClick={() => setSeverityFilter(s)}
                  className={clsx('px-3 py-1.5 text-[11px] font-medium capitalize transition-colors',
                    severityFilter === s ? 'bg-[#6366f1] text-white' : 'bg-[#111827] text-slate-400 hover:text-slate-200')}>
                  {s}
                </button>
              ))}
            </div>
          </div>

          {isLoading ? (
            <div className="flex justify-center py-8"><Spinner /></div>
          ) : filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-slate-500">
              <Eye size={32} className="mb-3 text-green-500 opacity-60" />
              <p className="text-sm">All clear — no active anomalies detected</p>
            </div>
          ) : (
            <div className="space-y-3">
              {filtered.map((a: any) => (
                <AnomalyCard key={a.id} anomaly={a} onDismiss={() => dismiss.mutate(a.id)} />
              ))}
            </div>
          )}
        </div>

        {/* Right panel */}
        <div className="space-y-4">
          {/* Scan activity */}
          <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <h3 className="text-[12px] font-semibold text-slate-400 mb-3">Anomaly detection activity</h3>
            <ResponsiveContainer width="100%" height={80}>
              <AreaChart data={scanData}>
                <defs>
                  <linearGradient id="anom" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#ef4444" stopOpacity={0.4}/>
                    <stop offset="95%" stopColor="#ef4444" stopOpacity={0}/>
                  </linearGradient>
                </defs>
                <XAxis hide /><YAxis hide />
                <Area type="monotone" dataKey="anomalies" stroke="#ef4444" fill="url(#anom)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          {/* Signal correlations */}
          <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <h3 className="text-[12px] font-semibold text-slate-400 mb-3">Signal correlations</h3>
            {(correlations?.correlations ?? []).map((c: any, i: number) => (
              <div key={i} className="py-2 border-b border-[#1e2433] last:border-0">
                <div className="flex justify-between items-center mb-1">
                  <span className="text-[11px] text-slate-300">{c.signal_a} ↔ {c.signal_b}</span>
                  <span className="text-[11px] font-bold text-[#6366f1]">{(c.correlation * 100).toFixed(0)}%</span>
                </div>
                <div className="text-[10px] text-slate-500">Lag: {c.lag_seconds}s</div>
                <div className="mt-1 h-1 bg-[#1e2433] rounded">
                  <div className="h-full bg-[#6366f1] rounded" style={{ width: `${c.correlation * 100}%` }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
