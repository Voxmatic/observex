// frontend/src/pages/MultiClusterPage.tsx
// Multi-cluster Kubernetes — register, monitor and compare multiple clusters
// Connects to /api/v1/clusters and shows per-cluster health from VictoriaMetrics

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { clusters, type K8sCluster } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import {
  Boxes, Plus, Trash2, CheckCircle, XCircle, AlertTriangle,
  Clock, Server, Activity, Copy, Terminal, ChevronDown, ChevronRight,
  Globe, RefreshCw
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import toast from 'react-hot-toast'
import clsx from 'clsx'

// ── Status helpers ────────────────────────────────────────────────────────────

const STATUS_CONFIG: Record<string, { icon: React.ReactNode; color: string; label: string }> = {
  active:      { icon: <CheckCircle size={12} />, color: 'text-ok',   label: 'Active'      },
  unreachable: { icon: <XCircle     size={12} />, color: 'text-crit', label: 'Unreachable' },
  unknown:     { icon: <Clock       size={12} />, color: 'text-slate-400', label: 'Pending agent' },
  no_agent:    { icon: <AlertTriangle size={12}/>, color: 'text-warn', label: 'No agent'    },
}

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.unknown
  return (
    <span className={clsx('flex items-center gap-1 text-[11px] font-medium', cfg.color)}>
      {cfg.icon} {cfg.label}
    </span>
  )
}

function ProviderBadge({ provider }: { provider: string }) {
  const colors: Record<string, string> = {
    eks:       'bg-orange-500/10 text-orange-400 border-orange-500/20',
    gke:       'bg-blue-500/10   text-blue-400   border-blue-500/20',
    aks:       'bg-cyan-500/10   text-cyan-400   border-cyan-500/20',
    k3s:       'bg-purple-500/10 text-purple-400 border-purple-500/20',
    openshift: 'bg-red-500/10    text-red-400    border-red-500/20',
    vanilla:   'bg-slate-500/10  text-slate-400  border-slate-500/20',
  }
  return (
    <span className={clsx('text-[10px] font-bold uppercase px-2 py-0.5 rounded border', colors[provider] ?? colors.vanilla)}>
      {provider}
    </span>
  )
}

// ── Metric tile ────────────────────────────────────────────────────────────────

function MetricTile({ label, value, icon: Icon, sub }: {
  label: string; value: string | number
  icon: React.ElementType
  sub?: string
}) {
  return (
    <div className="text-center">
      <div className="flex items-center justify-center gap-1 text-[10px] text-slate-500 mb-1">
        <Icon size={10} />{label}
      </div>
      <div className="text-lg font-bold text-slate-200">{value}</div>
      {sub && <div className="text-[10px] text-slate-500">{sub}</div>}
    </div>
  )
}

// ── Install instructions ───────────────────────────────────────────────────────

function InstallInstructions({ cluster }: { cluster: K8sCluster }) {
  const [copied, setCopied] = useState(false)
  const ingestorURL = window.location.origin
  const cmd = `kubectl create secret generic observex-agent-secret \\
  --from-literal=ingestor-url=${ingestorURL} \\
  --from-literal=cluster-name=${cluster.name} \\
  -n observex-agents --dry-run=client -o yaml | kubectl apply -f -

kubectl apply -f https://raw.githubusercontent.com/observex/platform/main/deployments/k8s/k8s-agent.yaml`

  const copy = () => {
    navigator.clipboard.writeText(cmd)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="mt-4 bg-surface-0 rounded-lg overflow-hidden border border-surface-3">
      <div className="flex items-center justify-between px-4 py-2 border-b border-surface-3">
        <div className="flex items-center gap-2 text-xs text-slate-400">
          <Terminal size={12} />
          Deploy the ObserveX agent to this cluster
        </div>
        <button onClick={copy}
          className="flex items-center gap-1 text-[11px] text-slate-400 hover:text-slate-200 transition-colors">
          <Copy size={11} />
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>
      <pre className="px-4 py-3 text-xs text-slate-300 font-mono whitespace-pre-wrap overflow-x-auto">
        {cmd}
      </pre>
    </div>
  )
}

// ── Cluster card ──────────────────────────────────────────────────────────────

function ClusterCard({ cluster, onDelete }: { cluster: K8sCluster; onDelete: () => void }) {
  const [expanded, setExpanded] = useState(false)

  const { data: healthData, isLoading: healthLoading, refetch } = useQuery({
    queryKey: ['cluster-health', cluster.id],
    queryFn: () => clusters.health(cluster.id),
    refetchInterval: 60_000,
    enabled: cluster.status === 'active' || cluster.status === 'unknown',
  })

  const health = healthData

  return (
    <div className={clsx(
      'bg-surface-1 border rounded-xl overflow-hidden transition-colors',
      cluster.status === 'active'      ? 'border-surface-2'       : '',
      cluster.status === 'unreachable' ? 'border-crit/20'         : '',
      cluster.status === 'no_agent'    ? 'border-warn/20'         : '',
      cluster.status === 'unknown'     ? 'border-surface-2'       : '',
    )}>
      {/* Header */}
      <div className="flex items-start gap-4 p-5">
        <div className="w-10 h-10 rounded-lg bg-brand/10 border border-brand/20 flex items-center justify-center flex-shrink-0">
          <Boxes size={18} className="text-brand" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 flex-wrap">
            <h3 className="text-sm font-semibold text-slate-200">{cluster.display_name || cluster.name}</h3>
            <ProviderBadge provider={cluster.provider} />
            <StatusBadge status={health?.health ?? cluster.status} />
          </div>
          <div className="flex items-center gap-3 mt-1 text-xs text-slate-500 flex-wrap">
            <span className="font-mono">{cluster.name}</span>
            {cluster.region && <span className="flex items-center gap-1"><Globe size={10} />{cluster.region}</span>}
            {cluster.last_seen_at && (
              <span className="flex items-center gap-1">
                <Clock size={10} />
                {formatDistanceToNow(new Date(cluster.last_seen_at), { addSuffix: true })}
              </span>
            )}
            {cluster.agent_version && (
              <span className="text-slate-600">v{cluster.agent_version}</span>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <button onClick={() => refetch()}
            className="p-1.5 text-slate-500 hover:text-slate-200 hover:bg-surface-2 rounded transition-colors">
            <RefreshCw size={13} />
          </button>
          <button
            onClick={() => onDelete()}
            className="p-1.5 text-slate-500 hover:text-crit hover:bg-crit/10 rounded transition-colors">
            <Trash2 size={13} />
          </button>
          <button
            onClick={() => setExpanded(e => !e)}
            className="p-1.5 text-slate-500 hover:text-slate-200 hover:bg-surface-2 rounded transition-colors">
            {expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          </button>
        </div>
      </div>

      {/* Quick stats */}
      <div className="grid grid-cols-4 gap-0 border-t border-surface-2 divide-x divide-surface-2">
        <div className="px-4 py-3">
          <MetricTile
            label="Nodes"
            value={health?.node_count ?? cluster.node_count}
            icon={Server}
          />
        </div>
        <div className="px-4 py-3">
          <MetricTile
            label="Pods"
            value={health?.pod_count ?? cluster.pod_count}
            icon={Boxes}
          />
        </div>
        <div className="px-4 py-3">
          <MetricTile
            label="Health"
            value={health?.health === 'healthy' ? '✓' : health?.health ?? '…'}
            icon={Activity}
          />
        </div>
        <div className="px-4 py-3">
          <MetricTile
            label="Last seen"
            value={cluster.last_seen_at
              ? formatDistanceToNow(new Date(cluster.last_seen_at)).replace(' ago', '')
              : '—'}
            icon={Clock}
            sub={cluster.last_seen_at ? 'ago' : undefined}
          />
        </div>
      </div>

      {/* Expanded: install instructions */}
      {expanded && (
        <div className="px-5 pb-5 border-t border-surface-2 pt-4">
          {cluster.status === 'active' ? (
            <div className="space-y-3">
              <div className="text-xs text-ok flex items-center gap-1.5">
                <CheckCircle size={12} /> Agent is active and reporting metrics
              </div>
              <div className="text-xs text-slate-500">
                All metrics from this cluster are tagged with{' '}
                <code className="text-slate-300 bg-surface-2 px-1 py-0.5 rounded">cluster="{cluster.name}"</code>{' '}
                in VictoriaMetrics. Use the Kubernetes page cluster selector to filter.
              </div>
            </div>
          ) : (
            <div>
              <div className="text-xs text-warn flex items-center gap-1.5 mb-3">
                <AlertTriangle size={12} />
                {cluster.status === 'no_agent' || cluster.status === 'unknown'
                  ? 'No agent detected. Run the install command below in your cluster.'
                  : 'Cluster unreachable. Check that the agent can reach this ObserveX instance.'}
              </div>
              <InstallInstructions cluster={cluster} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ── Register cluster form ──────────────────────────────────────────────────────

function RegisterForm({ onSave, onCancel }: {
  onSave: (d: Partial<K8sCluster>) => void
  onCancel: () => void
}) {
  const [form, setForm] = useState({
    name: '', display_name: '', region: '', provider: 'vanilla', api_server_url: '',
  })
  const set = (k: string, v: string) => setForm(f => ({ ...f, [k]: v }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!form.name.trim()) { toast.error('Cluster name required'); return }
    onSave({ ...form, name: form.name.trim().toLowerCase().replace(/\s+/g, '-') })
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="grid grid-cols-2 gap-4">
        {[
          { key: 'name',         label: 'Cluster name *',   placeholder: 'production-eu-west-1', help: 'Used as metric label — lowercase, no spaces' },
          { key: 'display_name', label: 'Display name',     placeholder: 'Production (EU)' },
          { key: 'region',       label: 'Region',           placeholder: 'eu-west-1 / europe-west1 / westeurope' },
          { key: 'api_server_url', label: 'API server URL (optional)', placeholder: 'https://xxx.eks.amazonaws.com' },
        ].map(f => (
          <div key={f.key}>
            <label className="block text-xs font-medium text-slate-400 mb-1">{f.label}</label>
            <input
              value={(form as any)[f.key]}
              onChange={e => set(f.key, e.target.value)}
              placeholder={f.placeholder}
              className="w-full px-3 py-2 text-sm bg-surface-0 border border-surface-3 rounded-lg
                         text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50"
            />
            {f.help && <p className="text-[10px] text-slate-600 mt-1">{f.help}</p>}
          </div>
        ))}
      </div>

      <div>
        <label className="block text-xs font-medium text-slate-400 mb-1">Provider</label>
        <div className="flex flex-wrap gap-2">
          {['vanilla', 'eks', 'gke', 'aks', 'k3s', 'openshift', 'rke2'].map(p => (
            <button
              type="button"
              key={p}
              onClick={() => set('provider', p)}
              className={clsx(
                'px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors uppercase',
                form.provider === p
                  ? 'bg-brand/20 border-brand/40 text-brand'
                  : 'bg-surface-0 border-surface-3 text-slate-400 hover:text-slate-200'
              )}
            >
              {p}
            </button>
          ))}
        </div>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <button type="submit"
          className="flex-1 py-2 bg-brand/20 hover:bg-brand/30 text-brand text-sm font-medium rounded-lg border border-brand/30 transition-colors">
          Register cluster
        </button>
        <button type="button" onClick={onCancel}
          className="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">
          Cancel
        </button>
      </div>
    </form>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function MultiClusterPage() {
  const [showForm, setShowForm] = useState(false)
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['clusters'],
    queryFn: () => clusters.list(),
    refetchInterval: 30_000,
  })

  const clusterList: K8sCluster[] = data?.clusters ?? []

  const registerMutation = useMutation({
    mutationFn: (d: Partial<K8sCluster>) => clusters.register(d),
    onSuccess: (res: any) => {
      qc.invalidateQueries({ queryKey: ['clusters'] })
      setShowForm(false)
      toast.success(`Cluster "${res.name}" registered`)
    },
    onError: (e: any) => toast.error(e?.response?.data?.error ?? 'Registration failed'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => clusters.deregister(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['clusters'] })
      toast.success('Cluster removed')
    },
  })

  // Summary stats
  const active      = clusterList.filter(c => c.status === 'active').length
  const totalNodes  = clusterList.reduce((s, c) => s + c.node_count, 0)
  const totalPods   = clusterList.reduce((s, c) => s + c.pod_count, 0)

  return (
    <div className="space-y-5">
      <PageHeader
        title="Multi-Cluster"
        subtitle="Register and monitor multiple Kubernetes clusters from a single pane of glass"
        actions={<button onClick={() => setShowForm(f => !f)} style={{display:'flex',alignItems:'center',gap:6,background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}><Plus size={13}/> Add Cluster</button>}
      />

      {/* Summary stats */}
      {clusterList.length > 0 && (
        <div className="grid grid-cols-4 gap-4">
          {[
            { label: 'Total clusters', value: clusterList.length, icon: Boxes   },
            { label: 'Active',         value: active,              icon: CheckCircle },
            { label: 'Total nodes',    value: totalNodes,          icon: Server  },
            { label: 'Total pods',     value: totalPods,           icon: Activity},
          ].map(s => (
            <div key={s.label} className="bg-surface-1 border border-surface-2 rounded-lg p-4">
              <div className="flex items-center justify-between mb-1">
                <span className="text-xs text-slate-500">{s.label}</span>
                <s.icon size={13} className="text-slate-500" />
              </div>
              <div className="text-2xl font-bold text-slate-200">{s.value}</div>
            </div>
          ))}
        </div>
      )}

      {/* Add cluster form */}
      {showForm && (
        <div className="bg-surface-1 border border-brand/20 rounded-xl p-5">
          <h3 className="text-sm font-medium text-slate-200 mb-4 flex items-center gap-2">
            <Plus size={14} className="text-brand" />
            Register new cluster
          </h3>
          <RegisterForm
            onSave={d => registerMutation.mutate(d)}
            onCancel={() => setShowForm(false)}
          />
        </div>
      )}

      {/* Cluster list */}
      {isLoading && <div className="text-center py-12"><Spinner /></div>}

      {!isLoading && clusterList.length === 0 && !showForm && (
        <EmptyState
          icon={Boxes}
          title="No clusters registered"
          description="Add your first Kubernetes cluster to start monitoring it alongside your other infrastructure."
        >
          <button
            onClick={() => setShowForm(true)}
            className="flex items-center gap-2 px-4 py-2 bg-brand/20 hover:bg-brand/30 text-brand text-sm font-medium rounded-lg border border-brand/30 transition-colors"
          >
            <Plus size={14} />
            Register first cluster
          </button>
        </EmptyState>
      )}

      {!isLoading && clusterList.length > 0 && (
        <div className="space-y-4">
          {clusterList.map(cluster => (
            <ClusterCard
              key={cluster.id}
              cluster={cluster}
              onDelete={() => deleteMutation.mutate(cluster.id)}
            />
          ))}
        </div>
      )}

      {/* How it works */}
      {!isLoading && clusterList.length === 0 && !showForm && (
        <div className="bg-surface-1 border border-surface-2 rounded-xl p-6">
          <h3 className="text-sm font-medium text-slate-200 mb-4 flex items-center gap-2">
            <Activity size={14} className="text-brand" />
            How multi-cluster monitoring works
          </h3>
          <div className="grid grid-cols-3 gap-4 text-sm text-slate-400">
            {[
              {
                step: '1',
                title: 'Register cluster',
                desc: 'Give your cluster a name and provider type. ObserveX generates the install command.',
              },
              {
                step: '2',
                title: 'Deploy agent',
                desc: 'Run the one-liner kubectl command to deploy the ObserveX DaemonSet. It connects back automatically.',
              },
              {
                step: '3',
                title: 'Monitor all clusters',
                desc: 'All metrics are tagged with cluster=name. The Kubernetes page shows a cluster selector at the top.',
              },
            ].map(s => (
              <div key={s.step} className="flex gap-3">
                <div className="w-6 h-6 rounded-full bg-brand/20 border border-brand/30 text-brand text-xs font-bold flex items-center justify-center flex-shrink-0">
                  {s.step}
                </div>
                <div>
                  <div className="text-slate-300 font-medium mb-1">{s.title}</div>
                  <div className="text-xs leading-relaxed">{s.desc}</div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
