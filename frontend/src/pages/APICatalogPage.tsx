import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { Globe, AlertTriangle, CheckCircle, TrendingUp, Users, Clock, Plus, ExternalLink } from 'lucide-react'
import clsx from 'clsx'

const TYPE_COLORS: Record<string, string> = { rest: '#6366f1', grpc: '#10b981', graphql: '#f59e0b', webhook: '#8b5cf6' }
const STATUS_STYLES: Record<string, string> = {
  active: 'bg-[#10b981]/20 text-[#10b981]',
  deprecated: 'bg-[#f59e0b]/20 text-[#f59e0b]',
  sunset: 'bg-[#ef4444]/20 text-[#ef4444]',
}

export default function APICatalogPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<any>(null)
  const [statusFilter, setStatusFilter] = useState('all')

  const { data, isLoading } = useQuery({
    queryKey: ['api-catalog'],
    queryFn: () => http.get('/api/v1/api-catalog').then(r => r.data),
    refetchInterval: 60_000,
  })

  const { data: consumers } = useQuery({
    queryKey: ['api-consumers', selected?.id],
    queryFn: () => http.get(`/api/v1/api-catalog/${selected.id}/consumers`).then(r => r.data),
    enabled: !!selected,
  })

  const apis: any[] = (data?.apis ?? []).filter((a: any) => statusFilter === 'all' || a.status === statusFilter)

  const totals = {
    totalRPS: apis.reduce((s: number, a: any) => s + (a.requests_per_sec ?? 0), 0),
    avgP99: apis.length ? apis.reduce((s: number, a: any) => s + (a.p99_latency_ms ?? 0), 0) / apis.length : 0,
    sloBreaches: apis.filter((a: any) => !a.slo_compliant).length,
    deprecated: apis.filter((a: any) => a.status === 'deprecated').length,
  }

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { label: 'Total APIs',    value: data?.total ?? 0,             color: '#6366f1', icon: Globe },
          { label: 'Total RPS',     value: totals.totalRPS.toFixed(0),   color: '#10b981', icon: TrendingUp },
          { label: 'SLO Breaches',  value: totals.sloBreaches,           color: '#ef4444', icon: AlertTriangle },
          { label: 'Deprecated',    value: totals.deprecated,            color: '#f59e0b', icon: Clock },
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

      {/* Filters */}
      <div className="flex gap-2">
        {['all', 'active', 'deprecated', 'sunset'].map(f => (
          <button key={f} onClick={() => setStatusFilter(f)}
            className={clsx('px-3 py-1.5 text-[11px] font-medium rounded-lg capitalize border transition-colors',
              statusFilter === f ? 'bg-[#6366f1]/20 text-[#818cf8] border-[#6366f1]/30' : 'bg-[#111827] text-slate-400 border-[#1e2433] hover:text-slate-200')}>
            {f}
          </button>
        ))}
      </div>

      <div className="grid grid-cols-5 gap-5">
        {/* API list */}
        <div className="col-span-3">
          {isLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
            <div className="bg-[#111827] border border-[#1e2433] rounded-xl overflow-hidden">
              <table className="w-full">
                <thead>
                  <tr className="border-b border-[#1e2433]">
                    {['API', 'Type', 'Status', 'RPS', 'P99', 'Errors', 'SLO', 'Consumers'].map(h => (
                      <th key={h} className="px-4 py-2.5 text-left text-[10px] font-semibold text-slate-500 uppercase tracking-wide">{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {apis.map((api: any) => (
                    <tr key={api.id} onClick={() => setSelected(selected?.id === api.id ? null : api)}
                      className={clsx('border-b border-[#0d1423] cursor-pointer transition-colors', selected?.id === api.id ? 'bg-[#6366f1]/10' : 'hover:bg-[#0d1423]')}>
                      <td className="px-4 py-3">
                        <div className="text-[12px] font-semibold text-slate-200">{api.name}</div>
                        <div className="text-[10px] text-slate-500 font-mono">{api.url}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-[10px] font-bold px-2 py-0.5 rounded uppercase" style={{ background: (TYPE_COLORS[api.type] ?? '#6b7280') + '20', color: TYPE_COLORS[api.type] ?? '#9ca3af' }}>
                          {api.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={clsx('text-[10px] font-bold px-2 py-0.5 rounded', STATUS_STYLES[api.status] ?? STATUS_STYLES.active)}>
                          {api.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-[12px] font-mono text-slate-300">{api.requests_per_sec?.toFixed(1)}</td>
                      <td className="px-4 py-3 text-[12px] font-mono" style={{ color: api.p99_latency_ms > 500 ? '#ef4444' : api.p99_latency_ms > 200 ? '#f59e0b' : '#e5e7eb' }}>
                        {api.p99_latency_ms?.toFixed(0)}ms
                      </td>
                      <td className="px-4 py-3 text-[12px] font-mono" style={{ color: api.error_rate_pct > 1 ? '#ef4444' : '#e5e7eb' }}>
                        {api.error_rate_pct?.toFixed(2)}%
                      </td>
                      <td className="px-4 py-3">
                        {api.slo_compliant
                          ? <CheckCircle size={14} className="text-[#10b981]" />
                          : <AlertTriangle size={14} className="text-[#ef4444]" />}
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1">
                          <Users size={11} className="text-slate-500" />
                          <span className="text-[12px] text-slate-400">{api.consumers}</span>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          }
          <button className="mt-3 flex items-center gap-2 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[12px] font-semibold px-4 py-2 rounded-lg">
            <Plus size={13} /> Register API
          </button>
        </div>

        {/* Detail */}
        <div className="col-span-2 space-y-4">
          {selected ? (
            <>
              <div className="bg-[#111827] border border-[#6366f1]/30 rounded-xl p-4">
                <div className="flex items-start justify-between mb-3">
                  <div>
                    <div className="text-[14px] font-bold text-slate-100">{selected.name}</div>
                    <div className="text-[11px] text-slate-500">v{selected.version} · {selected.owner}</div>
                  </div>
                  {selected.spec_url && (
                    <a href={selected.spec_url} target="_blank" rel="noreferrer"
                      className="flex items-center gap-1 text-[11px] text-[#818cf8] hover:text-[#6366f1]">
                      <ExternalLink size={11} /> OpenAPI
                    </a>
                  )}
                </div>
                <div className="grid grid-cols-2 gap-2 text-[11px]">
                  {[
                    ['Endpoint', selected.url],
                    ['Type', selected.type?.toUpperCase()],
                    ['Consumers', selected.consumers],
                    ['SLO', selected.slo_compliant ? '✓ Compliant' : '✗ Breach'],
                  ].map(([k, v]) => (
                    <div key={k} className="bg-[#0d1423] rounded-lg p-2.5">
                      <div className="text-slate-500 mb-0.5">{k}</div>
                      <div className="text-slate-200 font-medium truncate">{v}</div>
                    </div>
                  ))}
                </div>
              </div>

              {consumers && (
                <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
                  <h4 className="text-[12px] font-semibold text-slate-300 mb-3">Top Consumers</h4>
                  {(consumers.consumers ?? []).map((c: any) => (
                    <div key={c.service} className="py-2 border-b border-[#0d1423] last:border-0">
                      <div className="flex justify-between items-center mb-1">
                        <span className="text-[12px] font-medium text-[#818cf8]">{c.service}</span>
                        <span className="text-[11px] font-mono text-slate-400">{c.requests_per_sec?.toFixed(1)} rps</span>
                      </div>
                      <div className="text-[10px] text-slate-500">Error rate: {c.error_rate_pct?.toFixed(2)}%</div>
                    </div>
                  ))}
                </div>
              )}
            </>
          ) : (
            <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-8 flex flex-col items-center text-slate-500">
              <Globe size={32} className="mb-3 opacity-30" />
              <p className="text-sm">Select an API to view consumers and metrics</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
