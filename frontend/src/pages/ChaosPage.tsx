import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { Zap, Play, Square, AlertTriangle, CheckCircle, Clock, Target, Activity, Plus } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

const TYPE_STYLES: Record<string, { color: string; icon: string }> = {
  latency: { color: '#f59e0b', icon: '🐢' },
  kill:    { color: '#ef4444', icon: '💀' },
  cpu:     { color: '#f97316', icon: '🔥' },
  memory:  { color: '#8b5cf6', icon: '💾' },
  network: { color: '#06b6d4', icon: '🌐' },
  disk:    { color: '#6b7280', icon: '💿' },
}

const STATUS_CONFIG: Record<string, { color: string; bg: string; label: string }> = {
  ready:     { color: '#10b981', bg: '#10b98120', label: 'Ready' },
  running:   { color: '#f59e0b', bg: '#f59e0b20', label: 'Running' },
  completed: { color: '#6366f1', bg: '#6366f120', label: 'Completed' },
  failed:    { color: '#ef4444', bg: '#ef444420', label: 'Failed' },
  stopped:   { color: '#6b7280', bg: '#6b728020', label: 'Stopped' },
}

export default function ChaosPage() {
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [selected, setSelected] = useState<any>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['chaos-experiments'],
    queryFn: () => http.get('/api/v1/chaos/experiments').then(r => r.data),
    refetchInterval: 15_000,
  })

  const { data: blastData } = useQuery({
    queryKey: ['chaos-blast', selected?.id],
    queryFn: () => http.get('/api/v1/chaos/blast-radius', { params: { service: selected?.target?.selectors?.service } }).then(r => r.data),
    enabled: !!selected,
  })

  const run = useMutation({
    mutationFn: (id: string) => http.post(`/api/v1/chaos/experiments/${id}/run`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chaos-experiments'] }),
  })
  const stop = useMutation({
    mutationFn: (id: string) => http.post(`/api/v1/chaos/experiments/${id}/stop`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['chaos-experiments'] }),
  })

  const experiments: any[] = data?.experiments ?? []

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Warning banner */}
      <div className="flex items-start gap-3 bg-[#ef4444]/10 border border-[#ef4444]/30 rounded-xl p-4">
        <AlertTriangle size={18} className="text-[#ef4444] shrink-0 mt-0.5" />
        <div>
          <div className="text-[13px] font-semibold text-[#ef4444] mb-1">Chaos Engineering — Production Risk</div>
          <div className="text-[12px] text-slate-400">All experiments are logged and audited. Blast radius is calculated before execution. Use dry-run mode first. Never affect more than 25% of pods.</div>
        </div>
      </div>

      {/* Stats row */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { label: 'Experiments', value: experiments.length, color: '#6366f1' },
          { label: 'Running',     value: experiments.filter(e => e.status === 'running').length, color: '#f59e0b' },
          { label: 'Completed',   value: experiments.filter(e => e.status === 'completed').length, color: '#10b981' },
          { label: 'Ready',       value: experiments.filter(e => e.status === 'ready').length, color: '#9ca3af' },
        ].map(s => (
          <div key={s.label} className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <div className="text-[11px] text-slate-500 mb-1">{s.label}</div>
            <div className="text-2xl font-bold" style={{ color: s.color }}>{s.value}</div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-5 gap-5">
        {/* Experiments list */}
        <div className="col-span-3 space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-[13px] font-semibold text-slate-300">Experiments</h2>
            <button onClick={() => setShowCreate(true)}
              className="flex items-center gap-1.5 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[11px] font-semibold px-3 py-1.5 rounded-lg transition-colors">
              <Plus size={12} /> New Experiment
            </button>
          </div>

          {isLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
            experiments.map((exp: any) => {
              const type = TYPE_STYLES[exp.type] ?? { color: '#9ca3af', icon: '⚡' }
              const status = STATUS_CONFIG[exp.status] ?? STATUS_CONFIG.ready
              return (
                <div key={exp.id}
                  onClick={() => setSelected(exp)}
                  className={clsx('bg-[#111827] border rounded-xl p-4 cursor-pointer transition-all', selected?.id === exp.id ? 'border-[#6366f1]' : 'border-[#1e2433] hover:border-[#2a3447]')}>
                  <div className="flex items-start justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className="text-lg">{type.icon}</span>
                      <div>
                        <div className="text-[13px] font-semibold text-slate-100">{exp.name}</div>
                        <div className="text-[10px] text-slate-500 mt-0.5">Type: {exp.type} · {exp.target?.kind}</div>
                      </div>
                    </div>
                    <span className="text-[10px] font-bold px-2 py-0.5 rounded-full" style={{ background: status.bg, color: status.color }}>
                      {status.label}
                    </span>
                  </div>

                  {exp.description && <p className="text-[12px] text-slate-400 mb-3">{exp.description}</p>}

                  <div className="flex items-center gap-2">
                    <div className="flex items-center gap-1 text-[11px] text-slate-500">
                      <Target size={11} />
                      <span>{exp.target?.percentage ?? 100}% blast radius</span>
                    </div>
                    {exp.last_run_at && (
                      <span className="text-[11px] text-slate-500 ml-auto">
                        Last: {formatDistanceToNow(new Date(exp.last_run_at), { addSuffix: true })}
                      </span>
                    )}
                  </div>

                  <div className="flex gap-2 mt-3">
                    {exp.status === 'ready' && (
                      <button onClick={e => { e.stopPropagation(); run.mutate(exp.id) }}
                        className="flex items-center gap-1.5 bg-[#ef4444] hover:bg-[#dc2626] text-white text-[11px] font-bold px-3 py-1.5 rounded-lg transition-colors">
                        <Play size={11} /> Run Experiment
                      </button>
                    )}
                    {exp.status === 'running' && (
                      <button onClick={e => { e.stopPropagation(); stop.mutate(exp.id) }}
                        className="flex items-center gap-1.5 bg-[#f59e0b] hover:bg-[#d97706] text-white text-[11px] font-bold px-3 py-1.5 rounded-lg transition-colors">
                        <Square size={11} /> Stop
                      </button>
                    )}
                    <button onClick={e => e.stopPropagation()} className="text-[11px] text-slate-400 border border-[#1e2433] px-3 py-1.5 rounded-lg hover:border-[#2a3447]">
                      Blast Radius
                    </button>
                  </div>
                </div>
              )
            })
          }
        </div>

        {/* Detail panel */}
        <div className="col-span-2 space-y-4">
          {selected ? (
            <>
              <div className="bg-[#111827] border border-[#6366f1]/40 rounded-xl p-4">
                <h3 className="text-[12px] font-semibold text-slate-300 mb-3">Experiment Details</h3>
                <div className="space-y-2 text-[11px]">
                  {[
                    ['Name', selected.name],
                    ['Type', selected.type],
                    ['Target', `${selected.target?.kind} (${selected.target?.percentage}%)`],
                    ['Status', selected.status],
                    ['Created by', selected.created_by ?? 'system'],
                  ].map(([k, v]) => (
                    <div key={k} className="flex justify-between py-1 border-b border-[#1e2433]">
                      <span className="text-slate-500">{k}</span>
                      <span className="text-slate-200 font-medium">{v}</span>
                    </div>
                  ))}
                </div>
                {selected.parameters && (
                  <div className="mt-3">
                    <div className="text-[10px] text-slate-500 mb-1">Parameters</div>
                    <pre className="text-[10px] text-[#818cf8] bg-[#0d1423] rounded-lg p-2.5 overflow-x-auto">{JSON.stringify(selected.parameters, null, 2)}</pre>
                  </div>
                )}
              </div>

              {blastData && (
                <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
                  <h3 className="text-[12px] font-semibold text-slate-300 mb-3 flex items-center gap-2">
                    <Target size={13} className="text-[#ef4444]" /> Blast Radius Analysis
                  </h3>
                  <div className="space-y-3 text-[11px]">
                    <div>
                      <span className="text-slate-500">Direct dependents</span>
                      <div className="flex gap-1 mt-1 flex-wrap">
                        {(blastData.direct_dependents ?? []).map((d: string) => (
                          <span key={d} className="bg-[#ef4444]/20 text-[#ef4444] px-2 py-0.5 rounded text-[10px]">{d}</span>
                        ))}
                      </div>
                    </div>
                    <div>
                      <span className="text-slate-500">Estimated user impact</span>
                      <div className="text-[13px] font-bold text-[#f59e0b] mt-1">{blastData.estimated_user_impact_pct}%</div>
                    </div>
                    <div>
                      <span className="text-slate-500">Revenue impact</span>
                      <div className="text-[13px] font-bold text-[#ef4444] mt-1">{blastData.estimated_revenue_impact}</div>
                    </div>
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-8 flex flex-col items-center text-center text-slate-500">
              <Zap size={32} className="mb-3 opacity-40" />
              <p className="text-sm">Select an experiment to view details and blast radius</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
