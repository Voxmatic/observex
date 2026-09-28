import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { GitBranch, Filter, Shuffle, BarChart2, Plus, ArrowRight, Zap, DollarSign, ChevronDown } from 'lucide-react'
import clsx from 'clsx'

const TYPE_META: Record<string, { color: string; icon: React.ElementType; label: string }> = {
  filter:    { color: '#ef4444', icon: Filter,   label: 'Filter' },
  transform: { color: '#6366f1', icon: Shuffle,  label: 'Transform' },
  route:     { color: '#10b981', icon: GitBranch,label: 'Route' },
  sample:    { color: '#f59e0b', icon: BarChart2,label: 'Sample' },
  aggregate: { color: '#8b5cf6', icon: Zap,      label: 'Aggregate' },
}

const SIGNAL_COLORS: Record<string, string> = { metrics:'#6366f1', logs:'#10b981', traces:'#f59e0b' }

function RuleCard({ rule, onToggle }: { rule: any; onToggle: () => void }) {
  const [expanded, setExpanded] = useState(false)
  const meta = TYPE_META[rule.type] ?? TYPE_META.filter
  const Icon = meta.icon
  const inNum = rule.stats_in ?? 0
  const outNum = rule.stats_out ?? 0
  const dropPct = inNum > 0 ? ((inNum - outNum) / inNum * 100) : (parseFloat(rule.drop_pct) || 0)

  return (
    <div className={clsx('bg-[#111827] border rounded-xl overflow-hidden transition-all', rule.enabled ? 'border-[#1e2433]' : 'border-[#1e2433] opacity-60')}>
      <div className="flex items-center gap-3 p-4 cursor-pointer" onClick={() => setExpanded(e => !e)}>
        {/* Priority badge */}
        <div className="w-7 h-7 rounded-lg flex items-center justify-center text-[11px] font-bold bg-[#0d1423] text-slate-400 shrink-0">
          {rule.priority ?? 0}
        </div>

        {/* Icon + type */}
        <div className="w-8 h-8 rounded-lg flex items-center justify-center shrink-0" style={{ background: meta.color + '20' }}>
          <Icon size={14} style={{ color: meta.color }} />
        </div>

        {/* Name + signal */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-[13px] font-semibold text-slate-200 truncate">{rule.name}</span>
            <span className="text-[9px] font-bold px-1.5 py-0.5 rounded shrink-0"
              style={{ background: SIGNAL_COLORS[rule.signal] + '20', color: SIGNAL_COLORS[rule.signal] ?? '#9ca3af' }}>
              {rule.signal}
            </span>
          </div>
          <div className="flex items-center gap-3 mt-0.5">
            <span className="text-[10px] font-bold px-1.5 py-0.5 rounded" style={{ background: meta.color + '20', color: meta.color }}>
              {meta.label}
            </span>
          </div>
        </div>

        {/* Stats */}
        <div className="flex items-center gap-5 text-[11px] shrink-0">
          <div className="text-right">
            <div className="text-slate-400">{typeof inNum === 'number' ? inNum.toLocaleString() : inNum}/s in</div>
            <div className="text-slate-500">{typeof outNum === 'number' ? outNum.toLocaleString() : outNum}/s out</div>
          </div>
          <div className="text-right min-w-[50px]">
            <div className="font-bold" style={{ color: dropPct > 0 ? '#10b981' : '#6b7280' }}>
              {typeof dropPct === 'number' ? dropPct.toFixed(1) : dropPct}% drop
            </div>
          </div>
        </div>

        {/* Toggle */}
        <button onClick={e => { e.stopPropagation(); onToggle() }}
          className={clsx('w-9 h-5 rounded-full flex items-center transition-colors shrink-0', rule.enabled ? 'bg-[#6366f1] justify-end' : 'bg-[#2a3447] justify-start')}
          style={{ padding: '2px' }}>
          <div className="w-4 h-4 rounded-full bg-white shadow-sm" />
        </button>

        <ChevronDown size={14} className={clsx('text-slate-500 transition-transform shrink-0', expanded && 'rotate-180')} />
      </div>

      {expanded && (
        <div className="px-4 pb-4 border-t border-[#0d1423] pt-3">
          <div className="grid grid-cols-2 gap-4">
            {rule.conditions?.length > 0 && (
              <div>
                <div className="text-[10px] text-slate-500 uppercase tracking-wider mb-2">Conditions</div>
                {rule.conditions.map((c: any, i: number) => (
                  <div key={i} className="text-[11px] font-mono bg-[#0d1423] rounded px-2.5 py-1.5 mb-1 text-slate-300">
                    <span className="text-[#818cf8]">{c.field}</span>
                    <span className="text-slate-500"> {c.operator} </span>
                    <span className="text-[#10b981]">"{c.value}"</span>
                  </div>
                ))}
              </div>
            )}
            {rule.actions?.length > 0 && (
              <div>
                <div className="text-[10px] text-slate-500 uppercase tracking-wider mb-2">Actions</div>
                {rule.actions.map((a: any, i: number) => (
                  <div key={i} className="flex items-center gap-2 text-[11px] bg-[#0d1423] rounded px-2.5 py-1.5 mb-1">
                    <span className="font-bold" style={{ color: meta.color }}>{a.type.toUpperCase()}</span>
                    {a.params && <span className="text-slate-500 text-[10px] font-mono">{JSON.stringify(a.params)}</span>}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

export default function PipelinePage() {
  const qc = useQueryClient()

  const { data: rulesData, isLoading } = useQuery({
    queryKey: ['pipeline-rules'],
    queryFn: () => http.get('/api/v1/pipeline/rules').then(r => r.data),
    refetchInterval: 30_000,
  })

  const { data: stats } = useQuery({
    queryKey: ['pipeline-stats'],
    queryFn: () => http.get('/api/v1/pipeline/stats').then(r => r.data),
    refetchInterval: 10_000,
  })

  const toggle = useMutation({
    mutationFn: (rule: any) => http.put(`/api/v1/pipeline/rules/${rule.id}`, { ...rule, enabled: !rule.enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['pipeline-rules'] }),
  })

  const rules: any[] = (rulesData?.rules ?? []).sort((a: any, b: any) => (a.priority ?? 99) - (b.priority ?? 99))

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { label: 'Metrics In/s',  value: (stats?.metrics_in_per_sec ?? 284000).toLocaleString(), unit: 'events/s', color: '#6366f1' },
          { label: 'Logs In/s',     value: (stats?.logs_in_per_sec ?? 12400).toLocaleString(),    unit: 'lines/s',  color: '#10b981' },
          { label: 'Total Dropped', value: `${stats?.total_drop_pct ?? 14.2}%`,                  unit: 'of volume', color: '#f59e0b' },
          { label: 'Cost Saved/mo', value: `$${(stats?.cost_savings_usd_month ?? 840).toLocaleString()}`, unit: 'USD',  color: '#10b981' },
        ].map(s => (
          <div key={s.label} className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <div className="text-[11px] text-slate-500 mb-1">{s.label}</div>
            <div className="text-2xl font-bold" style={{ color: s.color }}>{s.value}</div>
            <div className="text-[10px] text-slate-600 mt-0.5">{s.unit}</div>
          </div>
        ))}
      </div>

      {/* Visual flow */}
      <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
        <div className="text-[11px] text-slate-500 uppercase tracking-wider mb-3">Pipeline Flow</div>
        <div className="flex items-center gap-3 overflow-x-auto pb-2">
          {['Ingest', 'Filter', 'Transform', 'Route', 'Sample', 'Storage'].map((step, i, arr) => (
            <div key={step} className="flex items-center gap-3 shrink-0">
              <div className="bg-[#0d1423] border border-[#2a3447] rounded-lg px-4 py-2 text-[12px] font-medium text-slate-300">{step}</div>
              {i < arr.length - 1 && <ArrowRight size={14} className="text-slate-600" />}
            </div>
          ))}
        </div>
      </div>

      {/* Rules */}
      <div>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-[13px] font-semibold text-slate-300">Pipeline Rules ({rules.length})</h2>
          <button className="flex items-center gap-2 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[11px] font-semibold px-3 py-1.5 rounded-lg">
            <Plus size={12} /> Add Rule
          </button>
        </div>
        {isLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
          <div className="space-y-2">
            {rules.map((rule: any) => (
              <RuleCard key={rule.id} rule={rule} onToggle={() => toggle.mutate(rule)} />
            ))}
          </div>
        }
      </div>
    </div>
  )
}
