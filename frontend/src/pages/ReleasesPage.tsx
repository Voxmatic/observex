import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { deployments as deploymentsApi, type DeploymentMarker } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, Btn } from '@/components/shared/Layout'
import {
  GitBranch, Plus, X, Check, Clock, TrendingUp, TrendingDown,
  Minus, ChevronDown, ChevronUp, AlertTriangle, Terminal, Copy
} from 'lucide-react'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'
import toast from 'react-hot-toast'

// ── Status helpers ─────────────────────────────────────────────────────────

type DeployStatus = DeploymentMarker['status']

const STATUS_CONFIG: Record<DeployStatus, { label: string; color: string; bg: string; Icon: React.ElementType }> = {
  pending:    { label: 'Pending',    color: 'text-slate-400',  bg: 'bg-slate-400/10 border-slate-400/20',  Icon: Clock },
  ok:         { label: 'Healthy',    color: 'text-ok',         bg: 'bg-ok/10 border-ok/20',               Icon: Check },
  improved:   { label: 'Improved',   color: 'text-brand',      bg: 'bg-brand/10 border-brand/20',         Icon: TrendingUp },
  regression: { label: 'Regression', color: 'text-crit',       bg: 'bg-crit/10 border-crit/30',           Icon: AlertTriangle },
}

function StatusBadge({ status }: { status: DeployStatus }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.pending
  const { Icon } = cfg
  return (
    <span className={clsx('inline-flex items-center gap-1.5 text-xs font-medium px-2 py-0.5 rounded-full border', cfg.color, cfg.bg)}>
      <Icon size={11} />
      {cfg.label}
    </span>
  )
}

// ── Delta display ──────────────────────────────────────────────────────────

function Delta({ value, unit = '%', invert = false }: { value?: number; unit?: string; invert?: boolean }) {
  if (value == null || value === 0) return <span className="text-slate-600 text-xs">—</span>
  const isGood = invert ? value > 0 : value < 0
  const isBad  = invert ? value < 0 : value > 0
  const color  = isGood ? 'text-ok' : isBad ? 'text-crit' : 'text-slate-400'
  const Icon   = isGood ? TrendingDown : isBad ? TrendingUp : Minus
  return (
    <span className={clsx('inline-flex items-center gap-0.5 text-xs font-mono font-medium', color)}>
      <Icon size={11} />
      {value > 0 ? '+' : ''}{value.toFixed(1)}{unit}
    </span>
  )
}

// ── Metric comparison card ─────────────────────────────────────────────────

function MetricCompare({ label, before, after, delta, unit = 'ms', lowerIsBetter = true }: {
  label: string; before?: number; after?: number; delta?: number
  unit?: string; lowerIsBetter?: boolean
}) {
  const noData = !before && !after
  return (
    <div className="bg-surface-2 rounded-lg p-3 min-w-0">
      <div className="text-[10px] text-slate-600 mb-2 font-medium uppercase tracking-wider">{label}</div>
      {noData ? (
        <div className="text-xs text-slate-700">No data</div>
      ) : (
        <>
          <div className="flex items-baseline gap-2 mb-1.5">
            <span className="text-xs text-slate-500">{before?.toFixed(1) ?? '—'}{unit}</span>
            <span className="text-slate-700">→</span>
            <span className={clsx('text-sm font-semibold font-mono',
              delta == null ? 'text-slate-300'
                : (lowerIsBetter ? delta > 0 : delta < 0) ? 'text-crit'
                : (lowerIsBetter ? delta < 0 : delta > 0) ? 'text-ok'
                : 'text-slate-300'
            )}>
              {after?.toFixed(1) ?? '—'}{unit}
            </span>
          </div>
          {delta != null && (
            <Delta value={delta} unit="%" invert={!lowerIsBetter} />
          )}
        </>
      )}
    </div>
  )
}

// ── Deploy row ─────────────────────────────────────────────────────────────

function DeployRow({ d, onSelect, isSelected }: {
  d: DeploymentMarker; onSelect: () => void; isSelected: boolean
}) {
  const analysisEta = d.status === 'pending'
    ? new Date(new Date(d.deployed_at).getTime() + 30 * 60 * 1000)
    : null

  return (
    <>
      {/* Main row */}
      <div
        onClick={onSelect}
        className={clsx(
          'flex items-center gap-3 px-4 py-3 border-b border-surface-3 cursor-pointer transition-colors',
          isSelected ? 'bg-surface-2' : 'hover:bg-surface-2/40'
        )}
      >
        {/* Version + service */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <span className="text-sm font-mono font-medium text-slate-200 truncate">{d.service_name}</span>
            <span className="text-xs font-mono bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded shrink-0">
              {d.version.length > 16 ? d.version.slice(0, 7) + '…' : d.version}
            </span>
          </div>
          <div className="flex items-center gap-2 text-xs text-slate-600">
            <span className="capitalize">{d.environment}</span>
            {d.namespace && <span>·</span>}
            {d.namespace && <span className="font-mono">{d.namespace}</span>}
            {d.deployed_by && <span>· by {d.deployed_by}</span>}
          </div>
        </div>

        {/* Status */}
        <StatusBadge status={d.status} />

        {/* P99 delta shorthand */}
        {d.p99_latency_delta_pct != null && d.status !== 'pending' && (
          <Delta value={d.p99_latency_delta_pct} unit="%" />
        )}
        {d.status === 'pending' && analysisEta && (
          <span className="text-xs text-slate-600 shrink-0">
            analysis {formatDistanceToNow(analysisEta, { addSuffix: true })}
          </span>
        )}

        {/* Time */}
        <span className="text-xs text-slate-600 shrink-0">
          {formatDistanceToNow(new Date(d.deployed_at), { addSuffix: true })}
        </span>

        {/* Expand chevron */}
        {isSelected
          ? <ChevronUp size={13} className="text-slate-500 shrink-0" />
          : <ChevronDown size={13} className="text-slate-500 shrink-0" />}
      </div>

      {/* Expanded detail */}
      {isSelected && (
        <div className="px-4 py-4 bg-surface-2/30 border-b border-surface-3 animate-fade-in">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
            {/* Metadata */}
            <div className="space-y-2 text-xs">
              {[
                ['Deployed at',    format(new Date(d.deployed_at), 'MMM dd yyyy, HH:mm:ss zzz')],
                ['Environment',    d.environment],
                ['Cluster',        d.cluster_name ?? '—'],
                ['Namespace',      d.namespace ?? '—'],
                ['Deployed by',    d.deployed_by ?? '—'],
                ['Previous ver.',  d.prev_version ?? '—'],
              ].map(([k, v]) => (
                <div key={k} className="flex items-baseline gap-2">
                  <span className="text-slate-600 w-28 shrink-0">{k}</span>
                  <span className="text-slate-300 font-mono truncate">{v}</span>
                </div>
              ))}
              {d.analysed_at && (
                <div className="flex items-baseline gap-2">
                  <span className="text-slate-600 w-28 shrink-0">Analysed at</span>
                  <span className="text-slate-300 font-mono">
                    {format(new Date(d.analysed_at), 'HH:mm:ss')}
                  </span>
                </div>
              )}
            </div>

            {/* Before/after metrics */}
            {d.status !== 'pending' && (
              <div className="grid grid-cols-2 gap-2">
                <MetricCompare
                  label="P99 latency"
                  before={d.p99_latency_before_ms}
                  after={d.p99_latency_after_ms}
                  delta={d.p99_latency_delta_pct}
                  unit="ms"
                  lowerIsBetter
                />
                <MetricCompare
                  label="Error rate"
                  before={d.error_rate_before_pct}
                  after={d.error_rate_after_pct}
                  delta={d.error_rate_delta_pct}
                  unit="%"
                  lowerIsBetter
                />
              </div>
            )}
            {d.status === 'pending' && (
              <div className="flex items-center justify-center bg-surface-2 rounded-xl p-4 text-center">
                <div>
                  <Clock size={20} className="text-slate-600 mx-auto mb-2" />
                  <p className="text-xs text-slate-500">Waiting 30 minutes post-deploy</p>
                  <p className="text-xs text-slate-700 mt-1">
                    Analysis runs {analysisEta ? formatDistanceToNow(analysisEta, { addSuffix: true }) : 'soon'}
                  </p>
                </div>
              </div>
            )}
          </div>

          {/* Notes or problem link */}
          {d.notes && (
            <p className="text-xs text-slate-500 bg-surface-3/50 rounded px-3 py-2 mt-2">{d.notes}</p>
          )}
          {d.problem_id && (
            <div className="flex items-center gap-2 mt-2 text-xs text-crit">
              <AlertTriangle size={12} />
              <span>Regression problem created: <span className="font-mono">{d.problem_id}</span></span>
            </div>
          )}
        </div>
      )}
    </>
  )
}

// ── New deployment form ────────────────────────────────────────────────────

function NewDeployForm({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState({
    service_name: '',
    version: '',
    prev_version: '',
    environment: 'production',
    namespace: '',
    deployed_by: '',
    notes: '',
  })

  const create = useMutation({
    mutationFn: () => deploymentsApi.create({
      ...form,
      deployed_at: new Date().toISOString(),
    }),
    onSuccess: (d) => {
      toast.success(`Deploy recorded — regression analysis in ~30 min`)
      qc.invalidateQueries({ queryKey: ['deployments'] })
      onClose()
    },
    onError: () => toast.error('Failed to record deployment'),
  })

  const set = (k: string, v: string) => setForm(p => ({ ...p, [k]: v }))
  const valid = form.service_name.trim() && form.version.trim()

  return (
    <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
      <div className="w-full max-w-lg bg-surface-1 border border-surface-3 rounded-2xl animate-slide-up">
        <div className="flex items-center justify-between px-5 py-4 border-b border-surface-3">
          <h2 className="text-sm font-semibold text-slate-200">Record deployment</h2>
          <button onClick={onClose}><X size={15} className="text-slate-500" /></button>
        </div>
        <div className="p-5 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-500 mb-1">Service name <span className="text-crit">*</span></label>
              <input value={form.service_name} onChange={e => set('service_name', e.target.value)}
                placeholder="api-gateway" autoFocus
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60 font-mono" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Version / tag <span className="text-crit">*</span></label>
              <input value={form.version} onChange={e => set('version', e.target.value)}
                placeholder="v2.3.1 or sha-abc1234"
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60 font-mono" />
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-500 mb-1">Previous version</label>
              <input value={form.prev_version} onChange={e => set('prev_version', e.target.value)}
                placeholder="v2.3.0"
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60 font-mono" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Environment</label>
              <select value={form.environment} onChange={e => set('environment', e.target.value)}
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                {['production', 'staging', 'development', 'canary'].map(e => (
                  <option key={e}>{e}</option>
                ))}
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-500 mb-1">Namespace</label>
              <input value={form.namespace} onChange={e => set('namespace', e.target.value)}
                placeholder="production"
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60 font-mono" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Deployed by</label>
              <input value={form.deployed_by} onChange={e => set('deployed_by', e.target.value)}
                placeholder="github-actions / alice"
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-500 mb-1">Notes</label>
            <input value={form.notes} onChange={e => set('notes', e.target.value)}
              placeholder="Optional — JIRA ticket, PR link, feature description"
              className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
          </div>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-surface-3">
          <Btn onClick={onClose}>Cancel</Btn>
          <Btn variant="primary" disabled={!valid || create.isPending} onClick={() => create.mutate()}>
            <Check size={13} /> {create.isPending ? 'Recording…' : 'Record deployment'}
          </Btn>
        </div>
      </div>
    </div>
  )
}

// ── CI integration snippet ─────────────────────────────────────────────────

function CISnippet({ ingestorURL }: { ingestorURL: string }) {
  const [copied, setCopied] = useState(false)
  const snippet = `# Add to your CI/CD pipeline after successful deploy
- name: Record deployment in ObserveX
  run: |
    curl -s -X POST ${ingestorURL}/api/v1/deployments \\
      -H "Content-Type: application/json" \\
      -H "Authorization: Bearer \${{ secrets.OBSERVEX_API_KEY }}" \\
      -d '{
        "service_name": "\${{ env.SERVICE_NAME }}",
        "version":      "\${{ github.sha }}",
        "prev_version": "\${{ env.PREV_SHA }}",
        "environment":  "production",
        "deployed_by":  "github-actions",
        "notes":        "PR #\${{ github.event.pull_request.number }}"
      }'`

  const copy = () => {
    navigator.clipboard.writeText(snippet)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
      <div className="flex items-center justify-between px-4 py-3 border-b border-surface-3">
        <div className="flex items-center gap-2">
          <Terminal size={13} className="text-slate-500" />
          <span className="text-xs font-medium text-slate-400">CI/CD integration (GitHub Actions)</span>
        </div>
        <Btn size="xs" onClick={copy}>
          {copied ? <><Check size={11} className="text-ok" /> Copied</> : <><Copy size={11} /> Copy</>}
        </Btn>
      </div>
      <pre className="text-[10px] font-mono text-slate-400 p-4 overflow-x-auto leading-relaxed bg-surface-0/40">
        {snippet}
      </pre>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────

export default function ReleasesPage() {
  const [showNew, setShowNew]   = useState(false)
  const [selected, setSelected] = useState<string | null>(null)
  const [serviceFilter, setServiceFilter] = useState('')
  const [statusFilter, setStatusFilter]   = useState('')
  const [showSnippet, setShowSnippet]     = useState(false)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['deployments', serviceFilter, statusFilter],
    queryFn: () => deploymentsApi.list({
      ...(serviceFilter && { service: serviceFilter }),
      ...(statusFilter  && { status: statusFilter }),
      limit: 100,
    }),
    refetchInterval: 30_000,
  })

  const deploys = data?.deployments ?? []

  // Summary counts
  const counts = deploys.reduce((acc, d) => {
    acc[d.status] = (acc[d.status] ?? 0) + 1
    return acc
  }, {} as Record<DeployStatus, number>)

  const recentRegression = deploys.find(d => d.status === 'regression')

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Releases"
        subtitle="Deployment history with automatic regression analysis"
        actions={
          <div className="flex items-center gap-2">
            <Btn size="xs" variant="ghost" onClick={() => setShowSnippet(!showSnippet)}>
              <Terminal size={12} /> CI setup
            </Btn>
            <Btn variant="primary" size="sm" onClick={() => setShowNew(true)}>
              <Plus size={13} /> Record deploy
            </Btn>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto">
        <div className="p-4 space-y-4">
          {/* Summary stats */}
          <div className="grid grid-cols-4 gap-3">
            {(['ok', 'improved', 'regression', 'pending'] as DeployStatus[]).map(status => {
              const cfg = STATUS_CONFIG[status]
              const { Icon } = cfg
              return (
                <button key={status}
                  onClick={() => setStatusFilter(statusFilter === status ? '' : status)}
                  className={clsx(
                    'bg-surface-1 border rounded-xl p-3 text-center transition-all',
                    statusFilter === status ? 'border-brand/50 bg-brand/5' : 'border-surface-3 hover:border-surface-4'
                  )}>
                  <div className={clsx('text-2xl font-semibold font-display', cfg.color)}>
                    {counts[status] ?? 0}
                  </div>
                  <div className="flex items-center justify-center gap-1 mt-1 text-xs text-slate-500">
                    <Icon size={11} />
                    {cfg.label}
                  </div>
                </button>
              )
            })}
          </div>

          {/* Regression banner */}
          {recentRegression && (
            <div className="flex items-center gap-3 bg-crit/10 border border-crit/30 rounded-xl px-4 py-3">
              <AlertTriangle size={15} className="text-crit shrink-0" />
              <div className="flex-1 text-sm">
                <span className="font-medium text-crit">Regression detected</span>
                <span className="text-slate-400 ml-2">
                  {recentRegression.service_name} {recentRegression.version} deployed{' '}
                  {formatDistanceToNow(new Date(recentRegression.deployed_at), { addSuffix: true })} —
                  P99 {recentRegression.p99_latency_delta_pct != null
                    ? ` ${recentRegression.p99_latency_delta_pct > 0 ? '+' : ''}${recentRegression.p99_latency_delta_pct.toFixed(1)}%`
                    : ''}
                </span>
              </div>
              <Btn size="xs" variant="danger" onClick={() => {
                setStatusFilter('regression')
                setSelected(recentRegression.id)
              }}>
                View
              </Btn>
            </div>
          )}

          {/* CI snippet */}
          {showSnippet && (
            <CISnippet ingestorURL={typeof window !== 'undefined' ? window.location.origin.replace(':5173', ':3001') : 'https://your-observex.example.com'} />
          )}

          {/* Filter bar */}
          <div className="flex items-center gap-2">
            <input
              value={serviceFilter}
              onChange={e => setServiceFilter(e.target.value)}
              placeholder="Filter by service…"
              className="px-3 py-1.5 text-xs bg-surface-1 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50 w-48 font-mono" />
            {statusFilter && (
              <Btn size="xs" variant="ghost" onClick={() => setStatusFilter('')}>
                <X size={11} /> Clear filter
              </Btn>
            )}
            <span className="text-xs text-slate-600 ml-auto">
              {deploys.length} deployments
            </span>
          </div>

          {/* Deployment list */}
          {isLoading && (
            <div className="flex justify-center py-12"><Spinner size={22} /></div>
          )}

          {!isLoading && deploys.length === 0 && (
            <EmptyState
              icon={GitBranch}
              title="No deployments recorded"
              description="Record your first deployment manually or integrate with your CI/CD pipeline to track releases automatically."
            />
          )}

          {deploys.length > 0 && (
            <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
              {/* Column headers */}
              <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                              bg-surface-2 border-b border-surface-3 px-4 py-2"
                style={{ gridTemplateColumns: '1fr 100px 80px 100px 100px 20px' }}>
                <span>Service / version</span>
                <span>Status</span>
                <span>Δ P99</span>
                <span></span>
                <span>Deployed</span>
                <span></span>
              </div>
              {deploys.map(d => (
                <DeployRow
                  key={d.id}
                  d={d}
                  isSelected={selected === d.id}
                  onSelect={() => setSelected(selected === d.id ? null : d.id)}
                />
              ))}
            </div>
          )}

          {/* How it works */}
          <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
            <div className="text-xs font-medium text-slate-400 mb-3">How regression detection works</div>
            <div className="grid grid-cols-1 sm:grid-cols-4 gap-3">
              {[
                { step: '1', title: 'Deploy recorded', desc: 'POST /api/v1/deployments from CI or manually. Processor snapshots current P99 and error rate as the "before" baseline.' },
                { step: '2', title: '30 min window', desc: 'The engine waits 30 minutes for traffic to stabilise after the deployment completes.' },
                { step: '3', title: 'Metrics compared', desc: 'P99 latency and error rate are compared between the 30-minute windows before and after the deploy.' },
                { step: '4', title: 'Alert if ≥20% worse', desc: 'If either metric worsens by ≥20%, a regression problem is created and the AI agent notifies your channels.' },
              ].map(({ step, title, desc }) => (
                <div key={step} className="flex gap-3">
                  <span className="w-5 h-5 rounded-full bg-brand/20 text-brand text-xs font-semibold flex items-center justify-center shrink-0 mt-0.5">{step}</span>
                  <div>
                    <div className="text-xs font-medium text-slate-300 mb-0.5">{title}</div>
                    <p className="text-[11px] text-slate-600 leading-relaxed">{desc}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {showNew && <NewDeployForm onClose={() => setShowNew(false)} />}
    </div>
  )
}
