// frontend/src/pages/EventExplorerPage.tsx  (M10)
// ClickHouse Event Explorer — high-cardinality event search and analytics
import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import { Search, Database, Filter } from 'lucide-react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell } from 'recharts'
import clsx from 'clsx'

const TOOLTIP = { contentStyle: { background:'hsl(220 11% 12%)', border:'1px solid hsl(220 9% 21%)', borderRadius:8, fontSize:11 } }
const COLORS = ['#3b82f6','#22c55e','#8b5cf6','#f59e0b','#ef4444','#ec4899','#14b8a6']

export default function EventExplorerPage() {
  const [search, setSearch] = useState('')
  const [submitted, setSubmitted] = useState('')
  const [hours, setHours] = useState(1)
  const [serviceFilter, setServiceFilter] = useState('')

  const { data: statsData } = useQuery({
    queryKey: ['ch-stats'],
    queryFn: () => http.get('/api/v1/events/stats').then(r => r.data),
    refetchInterval: 60_000,
  })

  const { data: eventsData, isLoading } = useQuery({
    queryKey: ['ch-events', submitted, hours, serviceFilter],
    queryFn: () => http.get('/api/v1/events', {
      params: { q: submitted, hours, service_id: serviceFilter, limit: 100 }
    }).then(r => r.data),
    enabled: true,
    refetchInterval: 30_000,
  })

  const run = useCallback(() => setSubmitted(search), [search])

  const rows: any[] = eventsData?.data ?? []
  const tables: any[] = statsData?.data ?? []

  // Build events-per-kind chart from results
  const kindCounts = rows.reduce((acc: Record<string,number>, row: any) => {
    const k = row.kind || 'unknown'
    acc[k] = (acc[k] || 0) + 1
    return acc
  }, {})
  const chartData = Object.entries(kindCounts)
    .map(([kind, count]) => ({ kind, count }))
    .sort((a,b) => (b.count as number) - (a.count as number))
    .slice(0, 10)

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Event Explorer"
        subtitle="Query raw events from ClickHouse — high-cardinality search · topology history · profile data"
      />

      {/* ClickHouse table sizes */}
      {tables.length > 0 && (
        <div className="flex gap-3 px-4 py-2 border-b border-surface-3 flex-shrink-0 overflow-x-auto">
          {tables.slice(0,6).map((t: any) => (
            <div key={t.table} className="flex items-center gap-2 text-[10px] bg-surface-2 border border-surface-3 rounded-lg px-2.5 py-1.5 flex-shrink-0">
              <Database size={10} className="text-slate-500"/>
              <span className="font-mono text-slate-300">{t.table}</span>
              <span className="text-slate-600">{t.rows?.toLocaleString()} rows</span>
              <span className="text-slate-700">{t.bytes >= 1e9 ? `${(t.bytes/1e9).toFixed(1)}GB` : t.bytes >= 1e6 ? `${(t.bytes/1e6).toFixed(0)}MB` : `${(t.bytes/1e3).toFixed(0)}KB`}</span>
            </div>
          ))}
        </div>
      )}

      {/* Query bar */}
      <div className="px-4 py-3 border-b border-surface-3 flex-shrink-0">
        <div className="flex gap-2">
          <div className="flex-1 relative">
            <Search size={12} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-600"/>
            <input value={search} onChange={e => setSearch(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && run()}
              placeholder="Search events by name, namespace, or label…"
              className="w-full pl-8 pr-3 py-2 text-xs font-mono bg-surface-1 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60"/>
          </div>
          <div className="flex bg-surface-1 border border-surface-3 rounded-lg overflow-hidden">
            {[1,6,24,72].map(h => (
              <button key={h} onClick={() => setHours(h)}
                className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                  hours === h ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                {h}h
              </button>
            ))}
          </div>
          <button onClick={run} className="px-3 py-1.5 text-xs bg-brand text-white rounded-lg font-semibold hover:bg-brand2 transition-colors">
            ▶ Query
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-hidden flex flex-col p-4 gap-3">
        {/* Kind distribution chart */}
        {chartData.length > 0 && (
          <div className="bg-surface-1 border border-surface-3 rounded-xl p-4 flex-shrink-0">
            <div className="text-xs font-medium text-slate-400 mb-3">Event distribution by kind</div>
            <ResponsiveContainer width="100%" height={80}>
              <BarChart data={chartData} barSize={20}>
                <XAxis dataKey="kind" tick={{ fill:'hsl(215 20% 45%)',fontSize:10 }} tickLine={false}/>
                <YAxis tick={{ fill:'hsl(215 20% 45%)',fontSize:10 }} tickLine={false} width={35}/>
                <Tooltip {...TOOLTIP}/>
                <Bar dataKey="count" radius={[3,3,0,0]}>
                  {chartData.map((_,i) => <Cell key={i} fill={COLORS[i%COLORS.length]}/>)}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          </div>
        )}

        {/* Results table */}
        <div className="flex-1 overflow-auto bg-surface-1 border border-surface-3 rounded-xl">
          <div className="sticky top-0 grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
            style={{ gridTemplateColumns: '1fr 80px 80px 90px 80px 100px' }}>
            <span>Name / ID</span><span>Kind</span><span>Namespace</span>
            <span>Cluster</span><span>Health</span><span>Last seen</span>
          </div>
          {isLoading ? (
            <div className="flex justify-center py-12"><Spinner size={20}/></div>
          ) : rows.length === 0 ? (
            <EmptyState icon={Database} title="No events found"
              description="Events are written to ClickHouse by the ingestor. Ensure services are being discovered by the OneAgent." />
          ) : (
            rows.map((row: any, i: number) => (
              <div key={i} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
                style={{ gridTemplateColumns: '1fr 80px 80px 90px 80px 100px' }}>
                <div>
                  <div className="font-semibold text-slate-200 font-mono truncate">{row.name || row.id}</div>
                  <div className="text-[10px] text-slate-600 font-mono truncate">{row.id}</div>
                </div>
                <span className="text-slate-400 font-mono text-[10px] bg-surface-3 px-1.5 py-0.5 rounded">{row.kind}</span>
                <span className="text-slate-500">{row.namespace}</span>
                <span className="text-slate-500 font-mono text-[10px]">{row.cluster}</span>
                <span className={clsx('font-semibold text-[10px]',
                  row.health_state==='GOOD'?'text-ok':row.health_state==='DEGRADED'?'text-warn':'text-crit')}>
                  {row.health_state}
                </span>
                <span className="text-slate-600 text-[10px]">{row.last_seen_at}</span>
              </div>
            ))
          )}
        </div>
        <div className="text-[10px] text-slate-700">
          {rows.length} results · ClickHouse query · {hours}h window · Tables: services, topology_edges, profiles
        </div>
      </div>
    </div>
  )
}
