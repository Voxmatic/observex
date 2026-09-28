import { useQuery } from '@tanstack/react-query'
import { synthetic as synApi, http } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import { Radio, CheckCircle, XCircle, AlertTriangle, Clock } from 'lucide-react'
import clsx from 'clsx'
import { formatDistanceToNow } from 'date-fns'
import { BarChart, Bar, XAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import type { CheckState } from '@/types'

function UptimePips({ checkId }: { checkId: string }) {
  const { data } = useQuery({
    queryKey: ['synthetic-history', checkId],
    queryFn: () => http.get(`/api/v1/synthetic/results?check_id=${checkId}&limit=90`).then(r => r.data),
    staleTime: 60_000,
  })
  const results: any[] = (data as any)?.results ?? []
  const pips = results.slice(-90)

  if (pips.length === 0) {
    return <div className="flex gap-0.5">{Array.from({ length: 30 }, (_, i) => <div key={i} className="w-1.5 h-4 rounded-sm bg-surface-3" />)}</div>
  }

  return (
    <div className="flex gap-0.5">
      {pips.map((r, i) => (
        <div key={i} title={`${r.status} · ${r.duration_ms?.toFixed(0)}ms`}
          className={clsx('w-1.5 h-4 rounded-sm', {
            'bg-ok':      r.status === 'UP',
            'bg-crit':    r.status === 'DOWN',
            'bg-warn':    r.status === 'DEGRADED',
            'bg-surface-3': r.status === 'UNKNOWN',
          })} />
      ))}
    </div>
  )
}

function CheckCard({ state }: { state: CheckState }) {
  const icon = {
    UP:      <CheckCircle size={14} className="text-ok" />,
    DOWN:    <XCircle size={14} className="text-crit" />,
    DEGRADED:<AlertTriangle size={14} className="text-warn" />,
    UNKNOWN: <Clock size={14} className="text-slate-500" />,
  }[state.status]

  const statusColor = {
    UP: 'border-ok/20', DOWN: 'border-crit/30', DEGRADED: 'border-warn/20', UNKNOWN: 'border-surface-3'
  }[state.status]

  const uptime = state.uptime_30d ?? 100
  const uptimeColor = uptime >= 99.9 ? 'text-ok' : uptime >= 99 ? 'text-warn' : 'text-crit'

  return (
    <div className={clsx('bg-surface-1 border rounded-xl p-4', statusColor)}>
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          {icon}
          <span className="text-sm font-medium text-slate-200 truncate max-w-[160px]">{state.check_name}</span>
        </div>
        <span className={clsx('text-sm font-semibold font-display', uptimeColor)}>
          {uptime.toFixed(2)}%
        </span>
      </div>

      <div className="mb-3">
        <UptimePips checkId={state.check_id} />
        <div className="flex items-center justify-between mt-1 text-[10px] text-slate-600">
          <span>90 days</span>
          <span>{state.consecutive_fails > 0 ? `${state.consecutive_fails} fail${state.consecutive_fails > 1 ? 's' : ''}` : 'passing'}</span>
        </div>
      </div>

      <div className="flex items-center justify-between text-xs text-slate-500">
        <span>{state.last_duration_ms.toFixed(0)}ms</span>
        <span>{formatDistanceToNow(new Date(state.last_checked), { addSuffix: true })}</span>
      </div>

      <div className="mt-2 grid grid-cols-3 gap-1 text-[10px] text-center">
        {[['24h', state.uptime_24h], ['7d', state.uptime_7d], ['30d', state.uptime_30d]].map(([period, val]) => (
          <div key={period} className="bg-surface-2 rounded p-1">
            <div className="text-slate-600">{period}</div>
            <div className={clsx('font-medium', (val as number) >= 99.9 ? 'text-ok' : (val as number) >= 99 ? 'text-warn' : 'text-crit')}>
              {(val as number)?.toFixed(1) ?? '—'}%
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function ResponseTimeChart({ states }: { states: CheckState[] }) {
  const data = states.map(s => ({
    name: s.check_name.length > 14 ? s.check_name.slice(0, 13) + '…' : s.check_name,
    ms: s.last_duration_ms,
    ok: s.status === 'UP',
  }))

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="text-xs font-medium text-slate-400 mb-3">Response times (last check)</div>
      <ResponsiveContainer width="100%" height={120}>
        <BarChart data={data} barSize={14}>
          <XAxis dataKey="name" tick={{ fill: 'hsl(215 20% 50%)', fontSize: 10 }} tickLine={false} axisLine={false} />
          <Tooltip contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
            formatter={(v: number) => [`${v.toFixed(0)}ms`, 'Duration']} />
          <Bar dataKey="ms" radius={[3, 3, 0, 0]}>
            {data.map((d, i) => <Cell key={i} fill={d.ok ? 'hsl(142 71% 45%)' : 'hsl(0 84% 60%)'} />)}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </div>
  )
}

export default function SyntheticPage() {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['synthetic-states'],
    queryFn: () => synApi.states(),
    refetchInterval: 30_000,
  })

  const states = data?.states ?? []
  const down    = states.filter(s => s.status === 'DOWN').length
  const degraded= states.filter(s => s.status === 'DEGRADED').length
  const up      = states.filter(s => s.status === 'UP').length

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Synthetic monitoring"
        subtitle={`${states.length} checks · ${up} passing · ${down + degraded} failing`}
      />

      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {/* Summary bar */}
        {states.length > 0 && (
          <div className="flex gap-4 text-sm">
            {[
              { label: 'Up',      count: up,       color: 'text-ok' },
              { label: 'Down',    count: down,     color: 'text-crit' },
              { label: 'Degraded',count: degraded, color: 'text-warn' },
            ].map(({ label, count, color }) => (
              <div key={label} className="flex items-center gap-1.5">
                <span className={clsx('text-xl font-semibold font-display', color)}>{count}</span>
                <span className="text-slate-500">{label}</span>
              </div>
            ))}
          </div>
        )}

        {isLoading && <div className="flex justify-center py-12"><Spinner size={24} /></div>}

        {!isLoading && states.length === 0 && (
          <EmptyState icon={Radio} title="No synthetic checks"
            description="Create checks via the API or configure them in the synthetic service." />
        )}

        {states.length > 0 && (
          <>
            <ResponseTimeChart states={states} />
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
              {states.map(s => <CheckCard key={s.check_id} state={s} />)}
            </div>
          </>
        )}
      </div>
    </div>
  )
}
