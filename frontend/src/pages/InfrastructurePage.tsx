// frontend/src/pages/InfrastructurePage.tsx
//
// Infrastructure & Node Monitoring Dashboard
//
// Displays per-host resource utilization from node_* metrics collected
// by the OneAgent /proc scraper and pushed to VictoriaMetrics every 15s.
//
// Metrics shown per node:
//   CPU usage %          — (1 - idle/total) * 100
//   Memory used %        — (total - available) / total * 100
//   Disk used %          — (size - avail) / size * 100
//   Disk IOPS read/write — rate(node_disk_reads_completed_total[5m])
//   Network Rx/Tx Mbps   — rate(node_network_receive_bytes_total[5m]) * 8 / 1e6
//
// Click any node to see full time-series drilldown (1h/6h/24h).

import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { nodes as nodesApi, metrics } from '@/lib/api'
import type { NodeInfo } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, SearchInput } from '@/components/shared/Layout'
import {
  Cpu, MemoryStick, HardDrive, Network, Server,
  CheckCircle, AlertTriangle, XCircle, ChevronLeft,
  TrendingUp, TrendingDown
} from 'lucide-react'
import {
  AreaChart, Area, LineChart, Line, XAxis, YAxis, CartesianGrid,
  Tooltip, ResponsiveContainer, Legend
} from 'recharts'
import { format } from 'date-fns'
import clsx from 'clsx'

// ── Helpers ───────────────────────────────────────────────────────────────────

function statusColor(s: string) {
  return s === 'critical' ? 'text-crit' : s === 'warning' ? 'text-warn' : 'text-ok'
}
function statusBorder(s: string) {
  return s === 'critical' ? 'border-crit/30 bg-crit/5' : s === 'warning' ? 'border-warn/20 bg-warn/5' : 'border-surface-3'
}
function StatusIcon({ status }: { status: string }) {
  if (status === 'critical') return <XCircle size={13} className="text-crit" />
  if (status === 'warning')  return <AlertTriangle size={13} className="text-warn" />
  return <CheckCircle size={13} className="text-ok" />
}

function GaugeBar({ pct, status }: { pct: number; status?: string }) {
  const color = status === 'critical' || pct > 90 ? 'bg-crit'
    : status === 'warning' || pct > 75 ? 'bg-warn' : 'bg-ok'
  return (
    <div className="w-full h-1.5 bg-surface-3 rounded-full overflow-hidden mt-1">
      <div className={clsx('h-full rounded-full transition-all', color)}
        style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
    </div>
  )
}

function MetricCell({ label, value, unit, pct, icon: Icon }: {
  label: string; value: string | number; unit?: string; pct?: number; icon: React.ElementType
}) {
  return (
    <div>
      <div className="flex items-center gap-1 text-[10px] text-slate-500 mb-0.5">
        <Icon size={10} />
        {label}
      </div>
      <div className="text-sm font-semibold font-mono text-slate-200">
        {typeof value === 'number' ? value.toFixed(1) : value}
        {unit && <span className="text-xs text-slate-500 font-normal ml-0.5">{unit}</span>}
      </div>
      {pct != null && <GaugeBar pct={pct} />}
    </div>
  )
}

// ── Node card ─────────────────────────────────────────────────────────────────

function NodeCard({ node, onClick }: { node: NodeInfo; onClick: () => void }) {
  return (
    <div
      onClick={onClick}
      className={clsx(
        'bg-surface-1 border rounded-xl p-4 cursor-pointer transition-all hover:border-brand/40 hover:shadow-brand/5',
        statusBorder(node.status)
      )}
    >
      <div className="flex items-start justify-between mb-4">
        <div className="flex items-center gap-2">
          <Server size={14} className="text-slate-500" />
          <div>
            <div className="text-sm font-semibold text-slate-200 font-mono">{node.name}</div>
            <div className={clsx('text-xs mt-0.5 capitalize', statusColor(node.status))}>
              {node.status}
            </div>
          </div>
        </div>
        <StatusIcon status={node.status} />
      </div>

      <div className="grid grid-cols-2 gap-x-4 gap-y-3">
        <MetricCell label="CPU" value={node.cpu_usage_pct} unit="%" pct={node.cpu_usage_pct} icon={Cpu} />
        <MetricCell label="Memory" value={node.mem_used_pct} unit="%" pct={node.mem_used_pct} icon={MemoryStick} />
        <MetricCell label="Disk" value={node.disk_used_pct} unit="%" pct={node.disk_used_pct} icon={HardDrive} />
        <div>
          <div className="flex items-center gap-1 text-[10px] text-slate-500 mb-0.5">
            <Network size={10} />Network
          </div>
          <div className="text-xs font-mono text-slate-300">
            <span className="text-ok">↓{node.net_rx_mbps.toFixed(1)}</span>
            {' / '}
            <span className="text-brand">↑{node.net_tx_mbps.toFixed(1)}</span>
            <span className="text-slate-600 ml-0.5">Mbps</span>
          </div>
        </div>
      </div>

      <div className="mt-3 pt-3 border-t border-surface-3 grid grid-cols-2 gap-2 text-[10px] text-slate-600">
        <span>RAM {node.mem_used_gb.toFixed(1)}/{node.mem_total_gb.toFixed(1)} GB</span>
        <span>Disk {node.disk_total_gb.toFixed(0)} GB total</span>
      </div>
    </div>
  )
}

// ── Node drilldown ────────────────────────────────────────────────────────────

const HOURS_OPTIONS = [
  { label: '1h',  value: 1 },
  { label: '6h',  value: 6 },
  { label: '24h', value: 24 },
]

function NodeDrilldown({ node, onBack }: { node: NodeInfo; onBack: () => void }) {
  const [hours, setHours] = useState(1)

  const { data: metricData, isLoading } = useQuery({
    queryKey: ['node-metrics', node.name, hours],
    queryFn: () => nodesApi.metrics(node.name, hours),
    refetchInterval: 60_000,
  })

  const parseVMSeries = (raw: any, labelKey?: string): Array<{ t: string; v: number; label?: string }> => {
    if (!raw?.data?.result) return []
    const results = raw.data.result
    // Merge all series into one timeline
    const pts: { t: string; v: number }[] = []
    for (const series of results) {
      for (const [ts, val] of (series.values ?? [])) {
        pts.push({
          t: format(new Date(Number(ts) * 1000), hours <= 1 ? 'HH:mm' : 'MMM d HH:mm'),
          v: Math.round(parseFloat(val as string) * 100) / 100 || 0,
        })
      }
    }
    return pts.sort((a, b) => a.t.localeCompare(b.t))
  }

  const cpuPts   = parseVMSeries(metricData?.cpu)
  const memPts   = parseVMSeries(metricData?.memory)
  const diskPts  = parseVMSeries(metricData?.disk)
  const netRxPts = parseVMSeries(metricData?.net_rx)
  const netTxPts = parseVMSeries(metricData?.net_tx)

  // Merge net rx/tx into combined series
  const netPts = netRxPts.map((pt, i) => ({
    t: pt.t,
    rx: pt.v,
    tx: netTxPts[i]?.v ?? 0,
  }))

  const chartProps = {
    contentStyle: {
      background: 'hsl(220 11% 12%)',
      border: '1px solid hsl(220 9% 21%)',
      borderRadius: 8,
      fontSize: 11,
    },
  }

  function ChartCard({ title, children }: { title: string; children: React.ReactNode }) {
    return (
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs font-medium text-slate-400 mb-3">{title}</div>
        {children}
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-3 border-b border-surface-3 flex-shrink-0">
        <button onClick={onBack}
          className="flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-300 transition-colors">
          <ChevronLeft size={14} /> All nodes
        </button>
        <span className="text-slate-700">/</span>
        <Server size={13} className="text-slate-500" />
        <span className="text-sm font-semibold font-mono text-slate-200">{node.name}</span>
        <span className={clsx('text-xs capitalize ml-1', statusColor(node.status))}>{node.status}</span>
        <div className="ml-auto flex gap-1">
          {HOURS_OPTIONS.map(o => (
            <button key={o.value} onClick={() => setHours(o.value)}
              className={clsx('px-2.5 py-1 text-xs rounded-lg transition-colors',
                hours === o.value ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
              {o.label}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {/* Current stats */}
        <div className="grid grid-cols-4 gap-3 mb-4">
          {[
            { label: 'CPU',    value: `${node.cpu_usage_pct.toFixed(1)}%`,   pct: node.cpu_usage_pct,    icon: Cpu,        color: node.cpu_usage_pct > 90 ? 'text-crit' : node.cpu_usage_pct > 75 ? 'text-warn' : 'text-ok' },
            { label: 'Memory', value: `${node.mem_used_pct.toFixed(1)}%`,    pct: node.mem_used_pct,     icon: MemoryStick,color: node.mem_used_pct > 90 ? 'text-crit' : node.mem_used_pct > 80 ? 'text-warn' : 'text-ok' },
            { label: 'Disk',   value: `${node.disk_used_pct.toFixed(1)}%`,   pct: node.disk_used_pct,    icon: HardDrive,  color: node.disk_used_pct > 90 ? 'text-crit' : node.disk_used_pct > 80 ? 'text-warn' : 'text-ok' },
            { label: 'Net Rx', value: `${node.net_rx_mbps.toFixed(1)} Mbps`, pct: null,                  icon: Network,    color: 'text-slate-200' },
          ].map(({ label, value, pct, icon: Icon, color }) => (
            <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs text-slate-500">{label}</span>
                <Icon size={13} className="text-slate-600" />
              </div>
              <div className={clsx('text-2xl font-semibold font-mono', color)}>{value}</div>
              {pct != null && <GaugeBar pct={pct} />}
            </div>
          ))}
        </div>

        {isLoading ? (
          <div className="flex justify-center py-16"><Spinner size={24} /></div>
        ) : (
          <div className="grid grid-cols-2 gap-4">
            <ChartCard title={`CPU usage (${hours}h)`}>
              <ResponsiveContainer width="100%" height={160}>
                <AreaChart data={cpuPts}>
                  <defs>
                    <linearGradient id="cpuGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%"  stopColor="#3b82f6" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 18%)" />
                  <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
                  <YAxis domain={[0, 100]} tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} unit="%" width={35} />
                  <Tooltip {...chartProps} formatter={(v: number) => [`${v.toFixed(1)}%`, 'CPU']} />
                  <Area type="monotone" dataKey="v" stroke="#3b82f6" fill="url(#cpuGrad)" strokeWidth={1.5} dot={false} name="CPU %" />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>

            <ChartCard title={`Memory usage (${hours}h)`}>
              <ResponsiveContainer width="100%" height={160}>
                <AreaChart data={memPts}>
                  <defs>
                    <linearGradient id="memGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%"  stopColor="#8b5cf6" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#8b5cf6" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 18%)" />
                  <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
                  <YAxis domain={[0, 100]} tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} unit="%" width={35} />
                  <Tooltip {...chartProps} formatter={(v: number) => [`${v.toFixed(1)}%`, 'Memory']} />
                  <Area type="monotone" dataKey="v" stroke="#8b5cf6" fill="url(#memGrad)" strokeWidth={1.5} dot={false} name="Memory %" />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>

            <ChartCard title={`Disk usage (${hours}h)`}>
              <ResponsiveContainer width="100%" height={160}>
                <AreaChart data={diskPts}>
                  <defs>
                    <linearGradient id="diskGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%"  stopColor="#f59e0b" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 18%)" />
                  <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
                  <YAxis domain={[0, 100]} tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} unit="%" width={35} />
                  <Tooltip {...chartProps} formatter={(v: number) => [`${v.toFixed(1)}%`, 'Disk']} />
                  <Area type="monotone" dataKey="v" stroke="#f59e0b" fill="url(#diskGrad)" strokeWidth={1.5} dot={false} name="Disk %" />
                </AreaChart>
              </ResponsiveContainer>
            </ChartCard>

            <ChartCard title={`Network throughput (${hours}h)`}>
              <ResponsiveContainer width="100%" height={160}>
                <LineChart data={netPts}>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 18%)" />
                  <XAxis dataKey="t" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
                  <YAxis tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} unit=" Mb" width={42} />
                  <Tooltip {...chartProps} formatter={(v: number) => [`${v.toFixed(2)} Mbps`, '']} />
                  <Legend wrapperStyle={{ fontSize: 11 }} />
                  <Line type="monotone" dataKey="rx" stroke="#22c55e" strokeWidth={1.5} dot={false} name="Rx" />
                  <Line type="monotone" dataKey="tx" stroke="#3b82f6" strokeWidth={1.5} dot={false} name="Tx" />
                </LineChart>
              </ResponsiveContainer>
            </ChartCard>
          </div>
        )}

        {/* Node details */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4 mt-4">
          <div className="text-xs font-medium text-slate-400 mb-3">Node details</div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-2">
            {[
              ['Hostname',        node.name],
              ['Status',          node.status],
              ['RAM total',       `${node.mem_total_gb.toFixed(1)} GB`],
              ['RAM used',        `${node.mem_used_gb.toFixed(1)} GB (${node.mem_used_pct.toFixed(1)}%)`],
              ['Disk total',      `${node.disk_total_gb.toFixed(0)} GB`],
              ['Disk used',       `${node.disk_used_pct.toFixed(1)}%`],
              ['Disk IOPS read',  `${node.disk_read_iops.toFixed(1)}/s`],
              ['Disk IOPS write', `${node.disk_write_iops.toFixed(1)}/s`],
            ].map(([k, v]) => (
              <div key={k} className="flex justify-between py-1.5 border-b border-surface-3/50 text-xs">
                <span className="text-slate-500">{k}</span>
                <span className="text-slate-300 font-mono">{v}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Summary row ───────────────────────────────────────────────────────────────

function ClusterSummary({ nodeList }: { nodeList: NodeInfo[] }) {
  const healthy  = nodeList.filter(n => n.status === 'healthy').length
  const warning  = nodeList.filter(n => n.status === 'warning').length
  const critical = nodeList.filter(n => n.status === 'critical').length
  const avgCPU   = nodeList.reduce((s, n) => s + n.cpu_usage_pct, 0) / (nodeList.length || 1)
  const avgMem   = nodeList.reduce((s, n) => s + n.mem_used_pct, 0) / (nodeList.length || 1)

  return (
    <div className="grid grid-cols-5 gap-3 mb-4">
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs text-slate-500 mb-2">Total nodes</div>
        <div className="text-2xl font-semibold font-display">{nodeList.length}</div>
      </div>
      <div className="bg-surface-1 border border-ok/20 rounded-xl p-4">
        <div className="text-xs text-slate-500 mb-2">Healthy</div>
        <div className="text-2xl font-semibold font-display text-ok">{healthy}</div>
      </div>
      <div className="bg-surface-1 border border-warn/20 rounded-xl p-4">
        <div className="text-xs text-slate-500 mb-2">Warning</div>
        <div className="text-2xl font-semibold font-display text-warn">{warning}</div>
      </div>
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs text-slate-500 mb-2">Avg CPU</div>
        <div className={clsx('text-2xl font-semibold font-display font-mono',
          avgCPU > 80 ? 'text-crit' : avgCPU > 60 ? 'text-warn' : 'text-ok')}>
          {avgCPU.toFixed(1)}%
        </div>
      </div>
      <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
        <div className="text-xs text-slate-500 mb-2">Avg Memory</div>
        <div className={clsx('text-2xl font-semibold font-display font-mono',
          avgMem > 85 ? 'text-crit' : avgMem > 70 ? 'text-warn' : 'text-ok')}>
          {avgMem.toFixed(1)}%
        </div>
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function InfrastructurePage() {
  const [selectedNode, setSelectedNode] = useState<NodeInfo | null>(null)
  const [search, setSearch]             = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'healthy' | 'warning' | 'critical'>('all')
  const [sortBy, setSortBy]             = useState<'name' | 'cpu' | 'memory' | 'disk'>('cpu')

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['nodes'],
    queryFn: () => nodesApi.list(),
    refetchInterval: 30_000,
  })

  const nodeList = useMemo(() => {
    let list = data?.nodes ?? []
    if (search) list = list.filter(n => n.name.toLowerCase().includes(search.toLowerCase()))
    if (statusFilter !== 'all') list = list.filter(n => n.status === statusFilter)
    return [...list].sort((a, b) => {
      if (sortBy === 'name')   return a.name.localeCompare(b.name)
      if (sortBy === 'cpu')    return b.cpu_usage_pct - a.cpu_usage_pct
      if (sortBy === 'memory') return b.mem_used_pct - a.mem_used_pct
      if (sortBy === 'disk')   return b.disk_used_pct - a.disk_used_pct
      return 0
    })
  }, [data, search, statusFilter, sortBy])

  if (selectedNode) {
    return <NodeDrilldown node={selectedNode} onBack={() => setSelectedNode(null)} />
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Infrastructure"
        subtitle="Host monitoring — CPU · memory · disk · network · per-node drilldown"
        actions={
          <div className="flex items-center gap-2">
            <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
              {(['all', 'healthy', 'warning', 'critical'] as const).map(f => (
                <button key={f} onClick={() => setStatusFilter(f)}
                  className={clsx('px-2.5 py-1.5 text-xs transition-colors capitalize',
                    statusFilter === f ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                  {f}
                </button>
              ))}
            </div>
            <select value={sortBy} onChange={e => setSortBy(e.target.value as any)}
              className="px-2.5 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
              <option value="cpu">Sort: CPU</option>
              <option value="memory">Sort: Memory</option>
              <option value="disk">Sort: Disk</option>
              <option value="name">Sort: Name</option>
            </select>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto p-4">
        {isLoading ? (
          <div className="flex flex-col items-center gap-3 py-20">
            <Spinner size={24} />
            <p className="text-sm text-slate-500">Querying VictoriaMetrics for node metrics…</p>
          </div>
        ) : nodeList.length === 0 && !search ? (
          <EmptyState icon={Server} title="No node metrics yet"
            description="The OneAgent DaemonSet collects CPU, memory, disk, and network metrics from /proc every 15s and pushes them to VictoriaMetrics. Make sure the OneAgent is running." />
        ) : (
          <>
            <ClusterSummary nodeList={data?.nodes ?? []} />

            <div className="flex items-center gap-3 mb-4">
              <SearchInput value={search} onChange={setSearch} placeholder="Filter nodes…" />
              <span className="text-xs text-slate-600 flex-shrink-0">{nodeList.length} node{nodeList.length !== 1 ? 's' : ''}</span>
            </div>

            {nodeList.length === 0 ? (
              <div className="text-center py-12 text-sm text-slate-600">No nodes match the current filter</div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
                {nodeList.map(node => (
                  <NodeCard key={node.name} node={node} onClick={() => setSelectedNode(node)} />
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
