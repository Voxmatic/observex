import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { alertRules as rulesApi } from '@/lib/api'
import { PageHeader, SeverityBadge, Spinner, EmptyState, Btn } from '@/components/shared/Layout'
import { Bell, Plus, Trash2, Edit2, X, Check, VolumeX, Volume2 } from 'lucide-react'
import clsx from 'clsx'
import toast from 'react-hot-toast'
import type { AlertRule } from '@/types'

function RuleRow({ rule, onEdit, onDelete, onToggleSilence }: {
  rule: AlertRule
  onEdit: () => void
  onDelete: () => void
  onToggleSilence: () => void
}) {
  const operator = { gt: '>', lt: '<', gte: '≥', lte: '≤', eq: '=' }[rule.operator] ?? rule.operator
  return (
    <div className={clsx('flex items-center gap-3 px-4 py-3 border-b border-surface-3 hover:bg-surface-2/40 transition-colors', rule.silenced && 'opacity-50')}>
      <SeverityBadge severity={rule.severity} />
      <div className="flex-1 min-w-0">
        <div className="text-sm text-slate-200 truncate">{rule.name}</div>
        <div className="text-xs text-slate-500 font-mono mt-0.5 truncate">{rule.expr} {operator} {rule.threshold}</div>
      </div>
      <div className="hidden md:flex items-center gap-1.5 shrink-0">
        {rule.channels.map(c => (
          <span key={c} className="text-[10px] bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded capitalize">{c}</span>
        ))}
        {(rule as any).runbook_url && (
          <a href={(rule as any).runbook_url} target="_blank" rel="noopener noreferrer"
            className="text-[10px] text-brand hover:underline" title="Runbook"
            onClick={e => e.stopPropagation()}>runbook ↗</a>
        )}
      </div>
      <div className="text-xs text-slate-600 shrink-0 w-12 text-right">{rule.duration}</div>
      <div className="flex items-center gap-1 shrink-0">
        <button onClick={onToggleSilence} title={rule.silenced ? 'Unsilence' : 'Silence'}
          className="p-1.5 text-slate-600 hover:text-slate-300 transition-colors">
          {rule.silenced ? <Volume2 size={13} /> : <VolumeX size={13} />}
        </button>
        <button onClick={onEdit}   className="p-1.5 text-slate-600 hover:text-slate-300"><Edit2 size={12} /></button>
        <button onClick={onDelete} className="p-1.5 text-slate-600 hover:text-crit transition-colors"><Trash2 size={12} /></button>
      </div>
    </div>
  )
}

function RuleForm({ initial, onSave, onCancel }: {
  initial?: Partial<AlertRule>
  onSave: (d: Partial<AlertRule>) => void
  onCancel: () => void
}) {
  const [d, setD] = useState<Partial<AlertRule>>({
    name: '', expr: '', threshold: 0, operator: 'gt',
    severity: 'HIGH', duration: '5m', message: '',
    channels: ['slack'], silenced: false, runbook_url: '',
    ...initial,
  })
  const set = (k: keyof AlertRule, v: any) => setD(p => ({ ...p, [k]: v }))

  const toggleChannel = (c: string) => {
    const ch = d.channels ?? []
    set('channels', ch.includes(c) ? ch.filter(x => x !== c) : [...ch, c])
  }

  return (
    <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
      <div className="w-full max-w-lg bg-surface-1 border border-surface-3 rounded-2xl p-5 animate-slide-up max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-slate-200">{initial?.id ? 'Edit alert rule' : 'New alert rule'}</h2>
          <button onClick={onCancel}><X size={15} className="text-slate-500" /></button>
        </div>

        <div className="space-y-3">
          <div>
            <label className="block text-xs text-slate-500 mb-1">Name</label>
            <input value={d.name ?? ''} onChange={e => set('name', e.target.value)}
              placeholder="High error rate"
              className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">PromQL expression</label>
            <textarea value={d.expr ?? ''} onChange={e => set('expr', e.target.value)} rows={2}
              placeholder="sum(rate(http_requests_total{status=~&quot;5..&quot;}[5m])) / sum(rate(http_requests_total[5m])) * 100"
              className="w-full px-3 py-2 text-sm font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60 resize-none" />
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-slate-500 mb-1">Operator</label>
              <select value={d.operator} onChange={e => set('operator', e.target.value)}
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                {[['gt', '>'], ['lt', '<'], ['gte', '≥'], ['lte', '≤'], ['eq', '=']].map(([v, l]) => (
                  <option key={v} value={v}>{l}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Threshold</label>
              <input type="number" value={d.threshold ?? 0} onChange={e => set('threshold', +e.target.value)}
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">For duration</label>
              <input value={d.duration ?? '5m'} onChange={e => set('duration', e.target.value)}
                placeholder="5m"
                className="w-full px-3 py-2 text-sm font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-500 mb-1">Severity</label>
              <select value={d.severity ?? 'HIGH'} onChange={e => set('severity', e.target.value)}
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                {['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'].map(s => <option key={s}>{s}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Namespace (optional)</label>
              <input value={d.namespace ?? ''} onChange={e => set('namespace', e.target.value)}
                placeholder="production"
                className="w-full px-3 py-2 text-sm font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
            </div>
          </div>

          <div>
            <label className="block text-xs text-slate-500 mb-1">Alert message</label>
            <input value={d.message ?? ''} onChange={e => set('message', e.target.value)}
              placeholder="Error rate exceeded threshold on {{ $labels.service }}"
              className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
          </div>

          <div>
            <div>
              <label className="block text-xs text-slate-500 mb-1">Runbook URL</label>
              <input value={(d as any).runbook_url ?? ''} onChange={e => set('runbook_url' as any, e.target.value)}
                placeholder="https://wiki.example.com/runbooks/high-error-rate"
                className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/60" />
            </div>
            <label className="block text-xs text-slate-500 mb-2">Notification channels</label>
            <div className="flex gap-2">
              {['slack', 'pagerduty', 'email'].map(c => (
                <button key={c} onClick={() => toggleChannel(c)}
                  className={clsx('px-3 py-1.5 text-xs rounded-lg border transition-colors capitalize', (d.channels ?? []).includes(c) ? 'bg-brand/20 border-brand/40 text-brand' : 'bg-surface-2 border-surface-3 text-slate-500 hover:text-slate-300')}>
                  {c}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-5">
          <Btn onClick={onCancel}>Cancel</Btn>
          <Btn variant="primary" onClick={() => onSave(d)}>
            <Check size={13} /> {initial?.id ? 'Save' : 'Create rule'}
          </Btn>
        </div>
      </div>
    </div>
  )
}

export default function AlertRulesPage() {
  const qc = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [editing, setEditing]   = useState<AlertRule | null>(null)
  const [filterSev, setFilterSev] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['alert-rules'],
    queryFn: () => rulesApi.list(),
    refetchInterval: 30_000,
  })

  const create  = useMutation({ mutationFn: (d: Partial<AlertRule>) => rulesApi.create(d), onSuccess: () => { toast.success('Rule created'); qc.invalidateQueries({ queryKey: ['alert-rules'] }); setShowForm(false) } })
  const update  = useMutation({ mutationFn: ({ id, d }: { id: string; d: Partial<AlertRule> }) => rulesApi.update(id, d), onSuccess: () => { toast.success('Rule updated'); qc.invalidateQueries({ queryKey: ['alert-rules'] }); setEditing(null) } })
  const del     = useMutation({ mutationFn: (id: string) => rulesApi.delete(id), onSuccess: () => { toast.success('Rule deleted'); qc.invalidateQueries({ queryKey: ['alert-rules'] }) } })
  const silence = useMutation({ mutationFn: ({ id, s }: { id: string; s: boolean }) => rulesApi.silence(id, s), onSuccess: () => qc.invalidateQueries({ queryKey: ['alert-rules'] }) })

  const rules = (data?.rules ?? []).filter(r => !filterSev || r.severity === filterSev)

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Alert rules"
        subtitle={`${data?.total ?? 0} rules · ${(data?.rules ?? []).filter(r => r.silenced).length} silenced`}
        actions={
          <div className="flex gap-2">
            <select value={filterSev} onChange={e => setFilterSev(e.target.value)}
              className="px-2.5 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-400 focus:outline-none">
              <option value="">All severities</option>
              {['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'].map(s => <option key={s}>{s}</option>)}
            </select>
            <Btn variant="primary" onClick={() => setShowForm(true)}><Plus size={13} /> New rule</Btn>
          </div>
        }
      />

      <div className="flex-1 overflow-y-auto">
        {/* Column headers */}
        <div className="flex items-center gap-3 px-4 py-2 bg-surface-2 border-b border-surface-3 text-[10px] font-semibold uppercase tracking-wider text-slate-600">
          <span className="w-16">Severity</span>
          <span className="flex-1">Name / Expression</span>
          <span className="hidden md:block w-32">Channels</span>
          <span className="w-12 text-right">For</span>
          <span className="w-20 text-right">Actions</span>
        </div>

        {isLoading && <div className="flex justify-center py-12"><Spinner size={20} /></div>}
        {!isLoading && rules.length === 0 && (
          <EmptyState icon={Bell} title="No alert rules" description="Create rules to get notified when metrics breach thresholds." />
        )}
        {rules.map(r => (
          <RuleRow key={r.id} rule={r}
            onEdit={() => setEditing(r)}
            onDelete={() => del.mutate(r.id)}
            onToggleSilence={() => silence.mutate({ id: r.id, s: !r.silenced })} />
        ))}
      </div>

      {(showForm || editing) && (
        <RuleForm
          initial={editing ?? undefined}
          onSave={d => editing ? update.mutate({ id: editing.id, d }) : create.mutate(d)}
          onCancel={() => { setShowForm(false); setEditing(null) }}
        />
      )}
    </div>
  )
}
