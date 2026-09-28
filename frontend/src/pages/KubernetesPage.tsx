// frontend/src/pages/KubernetesPage.tsx
// Kubernetes cluster monitoring — pods, namespaces, deployments, events
// Data from OneAgent K8s watch (pods/nodes/deployments via client-go)
// + VictoriaMetrics for CPU/memory per pod (cgroup v2 metrics)

import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { kubernetes } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, SearchInput, SeverityBadge } from '@/components/shared/Layout'
import { Server, AlertTriangle, CheckCircle, XCircle, Clock, Box, Layers, Filter } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

// ── Helpers ───────────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<string, { color: string; icon: React.ReactNode; border: string }> = {
  Running: { color: 'text-ok',   icon: <CheckCircle  size={12} className="text-ok"  />, border: 'border-ok/20'   },
  Pending: { color: 'text-warn', icon: <Clock        size={12} className="text-warn" />, border: 'border-warn/20' },
  Failed:  { color: 'text-crit', icon: <XCircle      size={12} className="text-crit" />, border: 'border-crit/30' },
  Unknown: { color: 'text-slate-500', icon: <AlertTriangle size={12} className="text-slate-500" />, border: 'border-surface-3' },
}

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.Unknown
  return (
    <span className={clsx('inline-flex items-center gap-1 text-[10px] font-bold', cfg.color)}>
      {cfg.icon} {status}
    </span>
  )
}

function ResourceBar({ pct, label }: { pct: number; label: string }) {
  const color = pct > 90 ? 'bg-crit' : pct > 75 ? 'bg-warn' : 'bg-ok'
  return (
    <div className="flex items-center gap-2 text-[10px]">
      <span className="text-slate-600 w-8">{label}</span>
      <div className="flex-1 h-1.5 bg-surface-3 rounded-full overflow-hidden">
        <div className={clsx('h-full rounded-full', color)} style={{ width: `${Math.min(100, pct)}%` }} />
      </div>
      <span className="text-slate-400 w-10 text-right">{pct.toFixed(0)}%</span>
    </div>
  )
}

// ── Namespace summary card ────────────────────────────────────────────────────

function NSCard({ ns, podCount, onClick, active }: {
  ns: string; podCount: number; onClick: () => void; active: boolean
}) {
  return (
    <button onClick={onClick}
      className={clsx('text-left bg-surface-1 border rounded-xl p-3 transition-all hover:border-brand/40',
        active ? 'border-brand/50 bg-brand/5' : 'border-surface-3')}>
      <div className="flex items-center justify-between mb-1">
        <div className="flex items-center gap-1.5">
          <Layers size={12} className="text-slate-500" />
          <span className="text-xs font-semibold text-slate-200 font-mono">{ns}</span>
        </div>
        <span className="text-[10px] bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded font-mono">{podCount}</span>
      </div>
      <div className="text-[10px] text-slate-600">{podCount} pods</div>
    </button>
  )
}

// ── Pod row ───────────────────────────────────────────────────────────────────

function PodRow({ pod }: { pod: any }) {
  const [expanded, setExpanded] = useState(false)
  const cfg = STATUS_CONFIG[pod.status] ?? STATUS_CONFIG.Unknown

  return (
    <>
      <div className={clsx('grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30 cursor-pointer transition-colors',
        pod.status === 'Failed' ? 'bg-crit/3' : '')}
        onClick={() => setExpanded(e => !e)}
        style={{ gridTemplateColumns: '160px 90px 80px 100px 80px 80px 1fr' }}>
        <span className="font-medium text-slate-200 font-mono truncate">{pod.name}</span>
        <StatusBadge status={pod.status} />
        <span className="text-slate-500 truncate">{pod.namespace}</span>
        <span className="text-slate-500 font-mono text-[10px] truncate">{pod.node?.split('-').slice(-2).join('-') || '—'}</span>
        <span className={clsx('font-mono', pod.cpu_pct > 90 ? 'text-crit' : pod.cpu_pct > 75 ? 'text-warn' : 'text-ok')}>
          {pod.cpu_pct?.toFixed(1) ?? '—'}%
        </span>
        <span className={clsx('font-mono', pod.mem_mb > 400 ? 'text-warn' : 'text-ok')}>
          {pod.mem_mb ? `${pod.mem_mb.toFixed(0)}MB` : '—'}
        </span>
        <div className="flex gap-1 flex-wrap">
          {(pod.tech_stack ?? []).slice(0, 3).map((t: string) => (
            <span key={t} className="text-[9px] bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded">{t}</span>
          ))}
          {pod.restarts > 0 && (
            <span className="text-[9px] bg-crit/15 text-crit px-1.5 py-0.5 rounded">{pod.restarts} restarts</span>
          )}
        </div>
      </div>
      {expanded && (
        <div className="bg-surface-2/50 px-4 py-3 border-b border-surface-3/50">
          <div className="grid grid-cols-2 gap-4 text-[11px]">
            <div>
              <div className="text-slate-500 mb-1.5">Resource usage</div>
              <ResourceBar pct={pod.cpu_pct ?? 0} label="CPU" />
              <div className="mt-1">
                <ResourceBar pct={pod.mem_mb ? Math.min(100, pod.mem_mb / 5.12) : 0} label="Mem" />
              </div>
            </div>
            <div>
              <div className="text-slate-500 mb-1.5">Labels</div>
              <div className="flex flex-wrap gap-1">
                {Object.entries(pod.labels ?? {}).slice(0, 6).map(([k, v]) => (
                  <span key={k} className="text-[9px] bg-surface-3 px-1.5 py-0.5 rounded font-mono text-slate-400">
                    {k}=<span className="text-slate-300">{String(v)}</span>
                  </span>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// ── Events panel ──────────────────────────────────────────────────────────────

function EventsPanel() {
  const { data, isLoading } = useQuery({
    queryKey: ['k8s-events'],
    queryFn: () => kubernetes.events(),
    refetchInterval: 30_000,
  })

  const events: any[] = data?.events ?? []

  if (isLoading) return <div className="flex justify-center py-6"><Spinner /></div>

  return (
    <div className="space-y-1">
      {events.map((evt, i) => (
        <div key={i} className={clsx('flex items-start gap-3 px-4 py-2.5 text-xs border-b border-surface-3/40',
          evt.type === 'Warning' ? 'bg-warn/3' : '')}>
          <span className={clsx('shrink-0 font-bold text-[10px] w-14',
            evt.type === 'Warning' ? 'text-warn' : 'text-ok')}>{evt.type}</span>
          <span className="shrink-0 w-32 font-mono text-slate-400 text-[10px] bg-surface-3 px-1.5 py-0.5 rounded">{evt.reason}</span>
          <span className="flex-1 text-slate-300">{evt.message}</span>
          <span className="shrink-0 text-slate-600 font-mono text-[10px]">{evt.age}</span>
          {evt.count > 1 && <span className="shrink-0 text-crit text-[10px] bg-crit/10 px-1.5 rounded">×{evt.count}</span>}
        </div>
      ))}
      {!events.length && <div className="text-center py-8 text-sm text-slate-600">No events</div>}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

type ActiveTab = 'pods' | 'events' | 'namespaces'

export default function KubernetesPage() {
  const [tab, setTab]         = useState<ActiveTab>('pods')
  const [nsFilter, setNsFilter] = useState('')
  const [stateFilter, setStateFilter] = useState('')
  const [search, setSearch]   = useState('')

  const { data: overview, isLoading: ovLoading } = useQuery({
    queryKey: ['k8s-overview'],
    queryFn: () => kubernetes.overview(),
    refetchInterval: 30_000,
  })
  const { data: podData, isLoading: podLoading } = useQuery({
    queryKey: ['k8s-pods', nsFilter, stateFilter],
    queryFn: () => kubernetes.pods({ namespace: nsFilter || undefined, state: stateFilter || undefined, limit: 200 }),
    refetchInterval: 15_000,
  })

  const pods: any[] = podData?.pods ?? []
  const namespaces: any[] = overview?.namespaces ?? []
  const podSummary = overview?.pods ?? { total: 0, running: 0, pending: 0, failed: 0 }

  const filteredPods = useMemo(() =>
    pods.filter(p => !search || p.name.includes(search) || p.namespace.includes(search)),
    [pods, search])

  const TABS: { id: ActiveTab; label: string }[] = [
    { id: 'pods',       label: `Pods (${pods.length})` },
    { id: 'events',     label: 'Events' },
    { id: 'namespaces', label: `Namespaces (${namespaces.length})` },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Kubernetes"
        subtitle={`Cluster: ${overview?.cluster_name ?? 'production'} · pod watch via OneAgent client-go`}
      />

      {/* Summary row */}
      <div className="grid grid-cols-5 gap-3 p-4 pb-0">
        {[
          { label: 'Nodes',   value: overview?.node_count ?? 0, color: 'text-slate-200' },
          { label: 'Pods',    value: podSummary.total,          color: 'text-slate-200' },
          { label: 'Running', value: podSummary.running,        color: 'text-ok'   },
          { label: 'Pending', value: podSummary.pending,        color: 'text-warn' },
          { label: 'Failed',  value: podSummary.failed,         color: 'text-crit' },
        ].map(({ label, value, color }) => (
          <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-3">
            <div className="text-[10px] text-slate-500 mb-1">{label}</div>
            <div className={clsx('text-2xl font-semibold font-mono', color)}>{ovLoading ? '—' : value}</div>
          </div>
        ))}
      </div>

      {/* Tabs */}
      <div className="flex border-b border-surface-3 px-4 mt-3">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            {t.label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {tab === 'pods' && (
          <>
            {/* Filters */}
            <div className="flex items-center gap-3 px-4 py-2.5 border-b border-surface-3 flex-shrink-0">
              <SearchInput value={search} onChange={setSearch} placeholder="Search pods…" />
              <select value={nsFilter} onChange={e => setNsFilter(e.target.value)}
                className="px-2.5 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
                <option value="">All namespaces</option>
                {namespaces.map((n: any) => (
                  <option key={n.name} value={n.name}>{n.name}</option>
                ))}
              </select>
              <select value={stateFilter} onChange={e => setStateFilter(e.target.value)}
                className="px-2.5 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
                <option value="">All states</option>
                <option value="Running">Running</option>
                <option value="Pending">Pending</option>
                <option value="Failed">Failed</option>
              </select>
              <span className="text-xs text-slate-600">{filteredPods.length} pods</span>
            </div>
            {/* Table */}
            <div className="flex-1 overflow-auto">
              <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
                style={{ gridTemplateColumns: '160px 90px 80px 100px 80px 80px 1fr' }}>
                <span>Name</span><span>Status</span><span>Namespace</span>
                <span>Node</span><span>CPU</span><span>Memory</span><span>Tags / Restarts</span>
              </div>
              {podLoading ? (
                <div className="flex justify-center py-12"><Spinner size={20} /></div>
              ) : filteredPods.length === 0 ? (
                <EmptyState icon={Box} title="No pods found"
                  description="No pods match the current filter. The OneAgent must be running in the cluster." />
              ) : (
                filteredPods.map(pod => <PodRow key={pod.id} pod={pod} />)
              )}
            </div>
          </>
        )}

        {tab === 'events' && (
          <div className="flex-1 overflow-auto">
            <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
              style={{ gridTemplateColumns: '56px 120px 1fr 50px 40px' }}>
              <span>Type</span><span>Reason</span><span>Message</span><span>Age</span><span>Count</span>
            </div>
            <EventsPanel />
          </div>
        )}

        {tab === 'namespaces' && (
          <div className="flex-1 overflow-auto p-4">
            <div className="grid grid-cols-3 gap-3">
              {namespaces.map((ns: any) => (
                <div key={ns.name} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <Layers size={13} className="text-slate-500" />
                      <span className="font-semibold text-slate-200 font-mono">{ns.name}</span>
                    </div>
                    <span className="text-xs text-slate-500">{ns.pod_count} pods</span>
                  </div>
                  <ResourceBar pct={ns.cpu_pct ?? 0} label="CPU" />
                  <div className="mt-1.5">
                    <ResourceBar pct={ns.mem_pct ?? 0} label="Mem" />
                  </div>
                </div>
              ))}
              {!namespaces.length && <div className="col-span-3 text-center text-sm text-slate-600 py-12">No namespaces found</div>}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
