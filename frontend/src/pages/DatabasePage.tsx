// frontend/src/pages/DatabasePage.tsx
// Database Monitoring — slow queries, connection pools, table bloat, live activity
// Backed by db-monitor service scraping pg_stat_statements + pg_stat_activity

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { databases } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import { Database, Clock, Zap, Users, Table2, Activity } from 'lucide-react'
import clsx from 'clsx'

type Tab = 'overview' | 'queries' | 'connections' | 'tables' | 'activity'

function HealthDot({ status }: { status: string }) {
  const c = status === 'healthy' ? 'bg-ok' : status === 'warning' ? 'bg-warn' : 'bg-crit'
  return <span className={clsx('inline-block w-2 h-2 rounded-full', c)} />
}

function StatCell({ label, value, unit, color }: { label: string; value: string | number; unit?: string; color?: string }) {
  return (
    <div className="text-center">
      <div className="text-[10px] text-slate-500 mb-0.5">{label}</div>
      <div className={clsx('text-base font-semibold font-mono', color ?? 'text-slate-200')}>
        {value}<span className="text-[10px] text-slate-500 ml-0.5">{unit}</span>
      </div>
    </div>
  )
}

export default function DatabasePage() {
  const [tab, setTab]       = useState<Tab>('overview')
  const [selDB, setSelDB]   = useState('observex')
  const [minMs, setMinMs]   = useState(10)

  const { data: dbList, isLoading: dbLoading } = useQuery({
    queryKey: ['databases'],
    queryFn: () => databases.list(),
    refetchInterval: 30_000,
  })
  const { data: queryData, isLoading: qLoading } = useQuery({
    queryKey: ['db-queries', selDB, minMs],
    queryFn: () => databases.queries(selDB, { min_ms: minMs, limit: 25 }),
    refetchInterval: 60_000,
    enabled: tab === 'queries',
  })
  const { data: connData, isLoading: cLoading } = useQuery({
    queryKey: ['db-connections', selDB],
    queryFn: () => databases.connections(selDB),
    refetchInterval: 30_000,
    enabled: tab === 'connections',
  })
  const { data: tableData, isLoading: tLoading } = useQuery({
    queryKey: ['db-tables', selDB],
    queryFn: () => databases.tables(selDB),
    refetchInterval: 120_000,
    enabled: tab === 'tables',
  })
  const { data: actData, isLoading: aLoading } = useQuery({
    queryKey: ['db-activity'],
    queryFn: () => databases.activity(),
    refetchInterval: 10_000,
    enabled: tab === 'activity',
  })

  const dbs: any[] = dbList?.databases ?? []
  const queries: any[] = queryData?.queries ?? []
  const pools: any[] = connData?.pools ?? []
  const tables: any[] = tableData?.tables ?? []
  const activity: any[] = actData?.connections ?? []

  const TABS: { id: Tab; label: string; icon: React.ElementType }[] = [
    { id: 'overview',     label: 'Overview',    icon: Database  },
    { id: 'queries',      label: 'Slow queries', icon: Clock    },
    { id: 'connections',  label: 'Connections',  icon: Users    },
    { id: 'tables',       label: 'Table stats',  icon: Table2   },
    { id: 'activity',     label: 'Live activity',icon: Activity },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Database Monitoring"
        subtitle="pg_stat_statements · pg_stat_activity · connection pools · table bloat"
        actions={
          <div className="flex items-center gap-2">
            <select value={selDB} onChange={e => setSelDB(e.target.value)}
              className="px-2.5 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
              {dbs.map(d => <option key={d.database} value={d.database}>{d.database}</option>)}
            </select>
          </div>
        }
      />

      <div className="flex border-b border-surface-3 px-4">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('flex items-center gap-1.5 px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            <t.icon size={12} /> {t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-4">

        {/* OVERVIEW */}
        {tab === 'overview' && (
          <div className="space-y-3">
            {dbLoading ? <Spinner /> : dbs.map(db => (
              <div key={db.database}
                className={clsx('bg-surface-1 border rounded-xl p-4',
                  db.status === 'critical' ? 'border-crit/30' : db.status === 'warning' ? 'border-warn/20' : 'border-surface-3')}>
                <div className="flex items-center justify-between mb-4">
                  <div className="flex items-center gap-2">
                    <HealthDot status={db.status} />
                    <span className="text-sm font-semibold text-slate-200 font-mono">{db.database}</span>
                    <span className="text-xs text-slate-600">{db.version}</span>
                  </div>
                  <span className={clsx('text-xs font-semibold capitalize',
                    db.status === 'healthy' ? 'text-ok' : db.status === 'warning' ? 'text-warn' : 'text-crit')}>
                    {db.status}
                  </span>
                </div>
                <div className="grid grid-cols-6 gap-4">
                  <StatCell label="Active conns" value={db.conn_active} color={db.conn_active > 80 ? 'text-warn' : 'text-ok'} />
                  <StatCell label="Idle conns"   value={db.conn_idle} />
                  <StatCell label="Max use"      value={db.conn_max_use_pct.toFixed(0)} unit="%" color={db.conn_max_use_pct > 80 ? 'text-crit' : db.conn_max_use_pct > 60 ? 'text-warn' : 'text-ok'} />
                  <StatCell label="Cache hit"    value={db.cache_hit_pct.toFixed(1)} unit="%" color={db.cache_hit_pct < 90 ? 'text-warn' : 'text-ok'} />
                  <StatCell label="TPS"          value={db.tps.toFixed(0)} />
                  <StatCell label="Deadlocks"    value={db.deadlocks} color={db.deadlocks > 0 ? 'text-crit' : 'text-ok'} />
                </div>
                {db.replication_lag_s > 0 && (
                  <div className="mt-3 text-xs text-warn bg-warn/10 border border-warn/20 rounded-lg px-3 py-2">
                    Replication lag: {db.replication_lag_s.toFixed(1)}s
                  </div>
                )}
                {db.slow_queries_1m > 0 && (
                  <div className="mt-2 text-xs text-warn">⚠ {db.slow_queries_1m} slow queries in last minute</div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* SLOW QUERIES */}
        {tab === 'queries' && (
          <>
            <div className="flex items-center gap-3 mb-4">
              <span className="text-xs text-slate-500">Min avg time:</span>
              {[10, 50, 100, 500].map(ms => (
                <button key={ms} onClick={() => setMinMs(ms)}
                  className={clsx('px-2.5 py-1 text-xs rounded-lg transition-colors',
                    minMs === ms ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                  {ms}ms
                </button>
              ))}
              <span className="ml-auto text-xs text-slate-600">{queries.length} queries</span>
            </div>
            {qLoading ? <Spinner /> : (
              <div className="space-y-2">
                {queries.map((q, i) => (
                  <div key={q.query_id} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                    <div className="flex items-start justify-between mb-2">
                      <div className="flex-1 min-w-0">
                        <div className="text-xs font-mono text-slate-200 truncate pr-4">{q.normalized}</div>
                        <div className="text-[10px] text-slate-500 mt-1">database: {q.database}</div>
                      </div>
                      <div className="flex items-center gap-4 flex-shrink-0">
                        <div className="text-center">
                          <div className="text-[9px] text-slate-600">avg</div>
                          <div className={clsx('text-sm font-semibold font-mono', q.avg_time_ms > 500 ? 'text-crit' : q.avg_time_ms > 100 ? 'text-warn' : 'text-ok')}>
                            {q.avg_time_ms.toFixed(0)}ms
                          </div>
                        </div>
                        <div className="text-center">
                          <div className="text-[9px] text-slate-600">calls</div>
                          <div className="text-sm font-semibold font-mono text-slate-300">
                            {q.calls.toLocaleString()}
                          </div>
                        </div>
                        <div className="text-center">
                          <div className="text-[9px] text-slate-600">cache</div>
                          <div className={clsx('text-sm font-semibold font-mono', q.cache_hit_pct < 80 ? 'text-warn' : 'text-ok')}>
                            {q.cache_hit_pct?.toFixed(0) ?? '—'}%
                          </div>
                        </div>
                      </div>
                    </div>
                    <div className="flex gap-3 text-[10px] text-slate-600">
                      <span>min: {q.min_time_ms?.toFixed(0)}ms</span>
                      <span>max: {q.max_time_ms?.toFixed(0)}ms</span>
                      <span>rows: {q.rows?.toLocaleString()}</span>
                      <span>total: {(q.total_time_ms / 1000).toFixed(1)}s</span>
                    </div>
                  </div>
                ))}
                {!queries.length && <EmptyState icon={Clock} title="No slow queries" description="No queries above threshold. Great performance!" />}
              </div>
            )}
          </>
        )}

        {/* CONNECTIONS */}
        {tab === 'connections' && (
          <div className="space-y-3">
            {cLoading ? <Spinner /> : pools.map((p, i) => {
              const color = p.use_pct > 90 ? 'text-crit' : p.use_pct > 75 ? 'text-warn' : 'text-ok'
              const barColor = p.use_pct > 90 ? 'bg-crit' : p.use_pct > 75 ? 'bg-warn' : 'bg-ok'
              return (
                <div key={i} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-3">
                    <div>
                      <span className="font-semibold text-slate-200 font-mono">{p.database}</span>
                      <span className="ml-2 text-xs text-slate-500 capitalize">{p.state}</span>
                    </div>
                    <span className={clsx('text-sm font-bold font-mono', color)}>{p.use_pct?.toFixed(0)}%</span>
                  </div>
                  <div className="h-2 bg-surface-3 rounded-full overflow-hidden mb-2">
                    <div className={clsx('h-full rounded-full', barColor)} style={{ width: `${p.use_pct}%` }} />
                  </div>
                  <div className="flex gap-4 text-[10px] text-slate-600">
                    <span>{p.count} / {p.max_allowed} connections</span>
                    {p.avg_wait_ms > 0 && <span className="text-warn">avg wait: {p.avg_wait_ms}ms</span>}
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {/* TABLES */}
        {tab === 'tables' && (
          <>
            {tLoading ? <Spinner /> : (
              <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
                <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
                  style={{ gridTemplateColumns: '80px 1fr 80px 80px 70px 80px 80px 80px 60px' }}>
                  <span>Schema</span><span>Table</span><span>Rows</span><span>Dead tuples</span>
                  <span>Bloat</span><span>Last vacuum</span><span>Idx scans</span><span>Seq scans</span><span>Size</span>
                </div>
                {tables.map((t, i) => (
                  <div key={i} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
                    style={{ gridTemplateColumns: '80px 1fr 80px 80px 70px 80px 80px 80px 60px' }}>
                    <span className="text-slate-500 font-mono">{t.schema}</span>
                    <span className="font-semibold text-slate-200 font-mono">{t.table}</span>
                    <span className="text-slate-400 font-mono">{t.row_count?.toLocaleString()}</span>
                    <span className={clsx('font-mono', t.dead_tuples > 100000 ? 'text-warn' : 'text-slate-400')}>
                      {t.dead_tuples?.toLocaleString()}
                    </span>
                    <span className={clsx('font-mono font-semibold', t.bloat_pct > 20 ? 'text-crit' : t.bloat_pct > 10 ? 'text-warn' : 'text-ok')}>
                      {t.bloat_pct?.toFixed(1)}%
                    </span>
                    <span className="text-slate-500 text-[10px]">{t.last_vacuum}</span>
                    <span className="text-slate-400 font-mono">{t.index_scans?.toLocaleString()}</span>
                    <span className={clsx('font-mono', t.seq_scans > 100 ? 'text-warn' : 'text-slate-400')}>
                      {t.seq_scans?.toLocaleString()}
                    </span>
                    <span className="text-slate-400 font-mono">{t.size_mb?.toFixed(0)}MB</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}

        {/* LIVE ACTIVITY */}
        {tab === 'activity' && (
          <>
            <div className="flex items-center justify-between mb-3">
              <span className="text-xs text-slate-500">{activity.length} active queries</span>
              <span className="text-[10px] text-ok flex items-center gap-1.5">
                <span className="w-1.5 h-1.5 rounded-full bg-ok animate-pulse" /> Live · refreshing every 10s
              </span>
            </div>
            {aLoading ? <Spinner /> : (
              <div className="space-y-2">
                {activity.map((conn, i) => (
                  <div key={i} className={clsx('bg-surface-1 border rounded-xl p-3',
                    conn.duration_ms > 5000 ? 'border-crit/30 bg-crit/3' :
                    conn.duration_ms > 1000 ? 'border-warn/20' : 'border-surface-3')}>
                    <div className="flex items-center justify-between mb-2">
                      <div className="flex items-center gap-2">
                        <span className="text-[10px] font-mono bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded">PID {conn.pid}</span>
                        <span className={clsx('text-[10px] font-semibold capitalize',
                          conn.state === 'active' ? 'text-ok' :
                          conn.state === 'idle in transaction' ? 'text-crit' : 'text-slate-500')}>
                          {conn.state}
                        </span>
                        {conn.wait_event && <span className="text-[10px] text-warn bg-warn/10 px-1.5 rounded">wait: {conn.wait_event}</span>}
                      </div>
                      <span className={clsx('text-xs font-mono font-semibold',
                        conn.duration_ms > 5000 ? 'text-crit' : conn.duration_ms > 1000 ? 'text-warn' : 'text-ok')}>
                        {conn.duration_ms > 1000 ? `${(conn.duration_ms/1000).toFixed(1)}s` : `${conn.duration_ms?.toFixed(0)}ms`}
                      </span>
                    </div>
                    <div className="text-[11px] font-mono text-slate-300 truncate">{conn.query}</div>
                    <div className="flex gap-3 mt-1.5 text-[10px] text-slate-600">
                      <span>{conn.database}</span>
                      <span>{conn.user}</span>
                      <span>{conn.application}</span>
                    </div>
                  </div>
                ))}
                {!activity.length && (
                  <div className="text-center py-12 text-sm text-ok">✓ No slow or blocked queries</div>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
