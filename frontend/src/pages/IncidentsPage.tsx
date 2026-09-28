import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { problems as problemsApi, incidentComments as commentsApi } from '@/lib/api'
import type { IncidentComment } from '@/lib/api'
import { PageHeader, StatusDot, SeverityBadge, Spinner, EmptyState, Btn } from '@/components/shared/Layout'
import { Zap, Bot, CheckCircle, XCircle, Clock, ChevronDown, ChevronUp, MessageSquare, Send, Pencil, Trash2 } from 'lucide-react'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'
import toast from 'react-hot-toast'
import type { Problem, Remediation } from '@/types'
import { useAuth } from '@/store/auth'

const STATUS_FILTERS = ['ALL', 'OPEN', 'ACKNOWLEDGED', 'RESOLVED'] as const
type StatusFilter = typeof STATUS_FILTERS[number]

// ── Comment thread ─────────────────────────────────────────────────────────────

function CommentThread({ problemId }: { problemId: string }) {
  const { user } = useAuth()
  const qc = useQueryClient()
  const [draft, setDraft] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editDraft, setEditDraft] = useState('')

  const { data } = useQuery({
    queryKey: ['comments', problemId],
    queryFn: () => commentsApi.list(problemId),
    refetchInterval: 30_000,
  })
  const comments: IncidentComment[] = data?.comments ?? []

  const createMut = useMutation({
    mutationFn: (content: string) => commentsApi.create(problemId, content),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['comments', problemId] }); setDraft('') },
    onError: () => toast.error('Failed to post comment'),
  })
  const updateMut = useMutation({
    mutationFn: ({ id, content }: { id: string; content: string }) => commentsApi.update(problemId, id, content),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['comments', problemId] }); setEditingId(null) },
  })
  const deleteMut = useMutation({
    mutationFn: (id: string) => commentsApi.delete(problemId, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['comments', problemId] }),
  })

  return (
    <div className="border-t border-surface-3 pt-3 mt-1">
      <div className="flex items-center gap-1.5 text-xs font-medium text-slate-500 mb-2">
        <MessageSquare size={12} />
        Comments {comments.length > 0 && <span className="text-slate-600">({comments.length})</span>}
      </div>

      {/* Thread */}
      {comments.map(cm => (
        <div key={cm.id} className="flex gap-2.5 mb-3 group">
          <div className="w-6 h-6 rounded-full bg-brand/20 text-brand text-[10px] font-semibold flex items-center justify-center shrink-0">
            {(cm.user_name ?? cm.user_email ?? 'U').charAt(0).toUpperCase()}
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-baseline gap-2 mb-0.5">
              <span className="text-xs font-medium text-slate-300">{cm.user_name ?? cm.user_email}</span>
              <span className="text-[10px] text-slate-600">
                {formatDistanceToNow(new Date(cm.created_at), { addSuffix: true })}
                {cm.edited && <span className="ml-1 italic">(edited)</span>}
              </span>
            </div>
            {editingId === cm.id ? (
              <div className="flex gap-1.5">
                <input
                  value={editDraft}
                  onChange={e => setEditDraft(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); updateMut.mutate({ id: cm.id, content: editDraft }) } if (e.key === 'Escape') setEditingId(null) }}
                  className="flex-1 text-xs bg-surface-2 border border-surface-3 rounded px-2 py-1 text-slate-300 focus:outline-none focus:border-brand/50"
                  autoFocus
                />
                <Btn size="xs" variant="primary" disabled={!editDraft.trim()} onClick={() => updateMut.mutate({ id: cm.id, content: editDraft })}>Save</Btn>
                <Btn size="xs" onClick={() => setEditingId(null)}>Cancel</Btn>
              </div>
            ) : (
              <p className="text-xs text-slate-400 leading-relaxed break-words">{cm.content}</p>
            )}
          </div>
          {cm.user_id === user?.id && editingId !== cm.id && (
            <div className="flex gap-0.5 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
              <button onClick={() => { setEditingId(cm.id); setEditDraft(cm.content) }}
                className="p-1 text-slate-600 hover:text-slate-300"><Pencil size={11} /></button>
              <button onClick={() => deleteMut.mutate(cm.id)}
                className="p-1 text-slate-600 hover:text-crit"><Trash2 size={11} /></button>
            </div>
          )}
        </div>
      ))}

      {/* New comment input */}
      <div className="flex gap-2">
        <input
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && draft.trim()) { e.preventDefault(); createMut.mutate(draft) } }}
          placeholder="Add a comment… (Enter to submit)"
          className="flex-1 text-xs bg-surface-2 border border-surface-3 rounded-lg px-3 py-1.5 text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50"
        />
        <button
          disabled={!draft.trim() || createMut.isPending}
          onClick={() => draft.trim() && createMut.mutate(draft)}
          className="p-1.5 text-brand hover:text-brand-glow disabled:opacity-40 disabled:cursor-not-allowed transition-colors">
          <Send size={14} />
        </button>
      </div>
    </div>
  )
}


function ProblemRow({ p, onSelect, selected }: { p: Problem; onSelect: () => void; selected: boolean }) {
  const qc = useQueryClient()

  const ack = useMutation({
    mutationFn: () => problemsApi.acknowledge(p.id),
    onSuccess: () => { toast.success('Acknowledged'); qc.invalidateQueries({ queryKey: ['problems'] }) },
  })
  const resolve = useMutation({
    mutationFn: () => problemsApi.resolve(p.id),
    onSuccess: () => { toast.success('Resolved'); qc.invalidateQueries({ queryKey: ['problems'] }) },
  })

  return (
    <div className={clsx('border-b border-surface-3 transition-colors', selected ? 'bg-surface-2' : 'hover:bg-surface-2/50')}>
      <div className="flex items-start gap-3 px-4 py-3 cursor-pointer" onClick={onSelect}>
        <div className="mt-0.5"><SeverityBadge severity={p.severity} /></div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <span className="text-sm font-medium text-slate-200 truncate">{p.title}</span>
            {selected ? <ChevronUp size={13} className="text-slate-500 shrink-0" /> : <ChevronDown size={13} className="text-slate-500 shrink-0" />}
          </div>
          <div className="flex items-center gap-3 text-xs text-slate-500">
            <span className="font-mono">{p.service_name}</span>
            <span>{p.namespace}</span>
            <span>{formatDistanceToNow(new Date(p.detected_at), { addSuffix: true })}</span>
          </div>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <span className={clsx('text-xs px-2 py-0.5 rounded-full', {
            'bg-crit/15 text-crit': p.status === 'OPEN',
            'bg-warn/15 text-warn': p.status === 'ACKNOWLEDGED',
            'bg-ok/15 text-ok':     p.status === 'RESOLVED',
          })}>{p.status}</span>
          {p.status === 'OPEN' && (
            <Btn size="xs" onClick={() => ack.mutate()} disabled={ack.isPending}>
              Ack
            </Btn>
          )}
          {p.status !== 'RESOLVED' && (
            <Btn size="xs" variant="primary" onClick={() => resolve.mutate()} disabled={resolve.isPending}>
              Resolve
            </Btn>
          )}
        </div>
      </div>

      {selected && (
        <div className="px-4 pb-3 space-y-3 animate-fade-in">
          <p className="text-sm text-slate-400">{p.description}</p>
          <div className="grid grid-cols-2 gap-4 text-xs">
            <div className="space-y-1.5">
              {[
                ['Detected', format(new Date(p.detected_at), 'MMM dd, HH:mm:ss')],
                ['Cluster', p.cluster_name],
                ['Namespace', p.namespace],
              ].map(([k, v]) => (
                <div key={k} className="flex gap-2">
                  <span className="text-slate-600 w-20 shrink-0">{k}</span>
                  <span className="text-slate-300 font-mono">{v}</span>
                </div>
              ))}
            </div>
            <div className="space-y-1.5">
              {p.metric_value != null && [
                ['Value', String(p.metric_value?.toFixed(3))],
                ['Threshold', String(p.threshold)],
              ].map(([k, v]) => (
                <div key={k} className="flex gap-2">
                  <span className="text-slate-600 w-20 shrink-0">{k}</span>
                  <span className="text-slate-300 font-mono">{v}</span>
                </div>
              ))}
              {p.affected_services?.length > 0 && (
                <div className="flex gap-2">
                  <span className="text-slate-600 w-20 shrink-0">Affected</span>
                  <span className="text-slate-300">{p.affected_services.join(', ')}</span>
                </div>
              )}
            </div>
          </div>
          {p.root_cause && (
            <div className="bg-surface-3/50 rounded-lg px-3 py-2 text-xs text-slate-400">
              <span className="text-slate-500 font-medium">Root cause: </span>{p.root_cause}
            </div>
          )}
          <CommentThread problemId={p.id} />
        </div>
      )}
    </div>
  )
}

function RemediationCard({ r }: { r: Remediation }) {
  const qc = useQueryClient()
  const approve = useMutation({
    mutationFn: () => problemsApi.approve(r.id),
    onSuccess: () => { toast.success('Approved'); qc.invalidateQueries({ queryKey: ['remediations'] }) },
  })
  const reject = useMutation({
    mutationFn: () => problemsApi.reject(r.id),
    onSuccess: () => { toast.success('Rejected'); qc.invalidateQueries({ queryKey: ['remediations'] }) },
  })

  const statusIcon = {
    pending:   <Clock size={13} className="text-warn" />,
    executing: <Spinner size={13} />,
    completed: <CheckCircle size={13} className="text-ok" />,
    failed:    <XCircle size={13} className="text-crit" />,
    rejected:  <XCircle size={13} className="text-slate-500" />,
  }[r.status]

  return (
    <div className="bg-surface-2 border border-surface-3 rounded-xl p-3">
      <div className="flex items-start gap-2 mb-2">
        {statusIcon}
        <div className="flex-1 min-w-0">
          <div className="text-xs font-medium text-slate-300 truncate">{r.action_type}</div>
          <div className="text-xs text-slate-500 mt-0.5">{r.description}</div>
        </div>
        {r.dry_run && (
          <span className="text-[10px] bg-brand/15 text-brand px-1.5 py-0.5 rounded shrink-0">DRY RUN</span>
        )}
      </div>
      {r.executed_at && (
        <div className="text-[10px] text-slate-600 mb-2">
          {formatDistanceToNow(new Date(r.executed_at), { addSuffix: true })}
        </div>
      )}
      {r.result && (
        <div className="text-[10px] text-ok bg-ok/10 rounded px-2 py-1 mb-2 font-mono">{r.result}</div>
      )}
      {r.error && (
        <div className="text-[10px] text-crit bg-crit/10 rounded px-2 py-1 mb-2 font-mono">{r.error}</div>
      )}
      {r.status === 'pending' && (
        <div className="flex gap-1.5 mt-2">
          <Btn size="xs" variant="primary" onClick={() => approve.mutate()} disabled={approve.isPending}>
            <CheckCircle size={11} /> Approve
          </Btn>
          <Btn size="xs" variant="danger" onClick={() => reject.mutate()} disabled={reject.isPending}>
            <XCircle size={11} /> Reject
          </Btn>
        </div>
      )}
    </div>
  )
}

export default function IncidentsPage() {
  const [filter, setFilter] = useState<StatusFilter>('ALL')
  const [selected, setSelected] = useState<string | null>(null)

  const { data: probData, isLoading } = useQuery({
    queryKey: ['problems'],
    queryFn: () => problemsApi.list({ limit: 100 }),
    refetchInterval: 15_000,
  })
  const { data: remData } = useQuery({
    queryKey: ['remediations'],
    queryFn: () => problemsApi.remediations(),
    refetchInterval: 10_000,
  })

  const all = probData?.problems ?? []
  const filtered = filter === 'ALL' ? all : all.filter(p => p.status === filter)
  const remediations = remData?.remediations ?? []
  const pending = remediations.filter(r => r.status === 'pending')

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Incidents"
        subtitle={`${all.filter(p => p.status === 'OPEN').length} open · ${all.filter(p => p.status === 'ACKNOWLEDGED').length} acknowledged`}
        actions={
          pending.length > 0 ? (
            <span className="flex items-center gap-1.5 text-xs bg-warn/15 text-warn border border-warn/30 px-2.5 py-1 rounded-lg">
              <Bot size={12} /> {pending.length} awaiting approval
            </span>
          ) : undefined
        }
      />

      <div className="flex-1 overflow-hidden flex gap-0">
        {/* Problem list */}
        <div className="flex-1 flex flex-col overflow-hidden border-r border-surface-3">
          {/* Status filter tabs */}
          <div className="flex border-b border-surface-3 px-4">
            {STATUS_FILTERS.map(f => {
              const count = f === 'ALL' ? all.length : all.filter(p => p.status === f).length
              return (
                <button key={f}
                  onClick={() => setFilter(f)}
                  className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 transition-colors -mb-px', filter === f ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
                  {f} {count > 0 && <span className="ml-1 text-[10px]">({count})</span>}
                </button>
              )
            })}
          </div>

          <div className="flex-1 overflow-y-auto">
            {isLoading && <div className="flex justify-center py-12"><Spinner size={20} /></div>}
            {!isLoading && filtered.length === 0 && (
              <EmptyState icon={Zap} title="No incidents" description="All clear — no problems match this filter." />
            )}
            {filtered.map(p => (
              <ProblemRow
                key={p.id} p={p}
                selected={selected === p.id}
                onSelect={() => setSelected(selected === p.id ? null : p.id)}
              />
            ))}
          </div>
        </div>

        {/* AI Agent panel */}
        <div className="w-72 flex-shrink-0 flex flex-col bg-surface-1">
          <div className="flex items-center gap-2 px-4 py-3 border-b border-surface-3">
            <Bot size={14} className="text-brand" />
            <span className="text-xs font-medium text-slate-300">AI Remediations</span>
          </div>
          <div className="flex-1 overflow-y-auto p-3 space-y-2">
            {remediations.length === 0 && (
              <div className="text-center py-8">
                <Bot size={28} className="text-slate-700 mx-auto mb-2" />
                <p className="text-xs text-slate-600">No remediations yet. The AI agent will propose actions when it detects problems.</p>
              </div>
            )}
            {remediations.map(r => <RemediationCard key={r.id} r={r} />)}
          </div>
        </div>
      </div>
    </div>
  )
}
