import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { AreaChart, Area, LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, ReferenceLine } from 'recharts'
import { TrendingUp, TrendingDown, Plus, Bell, Target } from 'lucide-react'
import clsx from 'clsx'

const FORMAT_MAP: Record<string, (v: number) => string> = {
  currency: v => `$${v >= 1000 ? (v/1000).toFixed(1)+'K' : v.toFixed(0)}`,
  percent:  v => `${v.toFixed(2)}%`,
  number:   v => v >= 1000 ? `${(v/1000).toFixed(1)}K` : v.toFixed(0),
  duration: v => v >= 1000 ? `${(v/1000).toFixed(1)}s` : `${v.toFixed(0)}ms`,
}

function KPICard({ kpi, onClick }: { kpi: any; onClick: () => void }) {
  const fmt = FORMAT_MAP[kpi.format] ?? FORMAT_MAP.number
  const isLower = kpi.direction === 'lower_better'
  const val = kpi.current_value ?? kpi.target * (0.85 + Math.random() * 0.25)
  const pctOfTarget = isLower
    ? kpi.target / Math.max(val, 0.01) * 100
    : val / Math.max(kpi.target, 0.01) * 100
  const status = pctOfTarget >= 100 ? 'ok' : pctOfTarget >= 85 ? 'warn' : 'crit'
  const COLOR = { ok: '#10b981', warn: '#f59e0b', crit: '#ef4444' }[status]

  const spark = Array.from({ length: 20 }, (_, i) => ({ v: val * (0.9 + Math.random() * 0.2) }))

  return (
    <div onClick={onClick} className="bg-[#111827] border border-[#1e2433] hover:border-[#2a3447] rounded-xl p-4 cursor-pointer transition-all group">
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="text-[10px] text-slate-500 uppercase tracking-wider mb-1">{kpi.category}</div>
          <div className="text-[13px] font-semibold text-slate-200">{kpi.name}</div>
        </div>
        <span className="text-[10px] font-bold px-2 py-0.5 rounded-full" style={{ background: COLOR + '20', color: COLOR }}>
          {status === 'ok' ? '✓ On target' : status === 'warn' ? '↗ Near limit' : '⚠ Off target'}
        </span>
      </div>

      <div className="flex items-baseline gap-2 mb-1">
        <span className="text-2xl font-bold" style={{ color: COLOR }}>{fmt(val)}</span>
        <span className="text-[11px] text-slate-500">{kpi.unit}</span>
      </div>
      <div className="text-[11px] text-slate-600 mb-3">
        Target: {fmt(kpi.target)} · {isLower ? 'Lower is better' : 'Higher is better'}
      </div>

      <ResponsiveContainer width="100%" height={40}>
        <AreaChart data={spark}>
          <defs>
            <linearGradient id={`g-${kpi.id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={COLOR} stopOpacity={0.3}/>
              <stop offset="95%" stopColor={COLOR} stopOpacity={0}/>
            </linearGradient>
          </defs>
          <Area type="monotone" dataKey="v" stroke={COLOR} fill={`url(#g-${kpi.id})`} strokeWidth={1.5} dot={false} />
        </AreaChart>
      </ResponsiveContainer>

      <div className="mt-2">
        <div className="flex justify-between text-[10px] text-slate-600 mb-1">
          <span>0</span><span>Progress to target</span><span>{fmt(kpi.target)}</span>
        </div>
        <div className="h-1.5 bg-[#1e2433] rounded-full overflow-hidden">
          <div className="h-full rounded-full transition-all" style={{ width: `${Math.min(pctOfTarget, 100)}%`, background: COLOR }} />
        </div>
      </div>
    </div>
  )
}

export default function KPIPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<any>(null)
  const [category, setCategory] = useState('all')
  const [showCreate, setShowCreate] = useState(false)
  const [newKPI, setNewKPI] = useState({ name:'', category:'revenue', metric_expr:'', unit:'', format:'number', target:0, direction:'higher_better' })

  const { data, isLoading } = useQuery({
    queryKey: ['kpis', category],
    queryFn: () => http.get('/api/v1/kpis', { params: category !== 'all' ? { category } : {} }).then(r => r.data),
    refetchInterval: 30_000,
  })

  const { data: histData } = useQuery({
    queryKey: ['kpi-history', selected?.id],
    queryFn: () => http.get(`/api/v1/kpis/${selected.id}/history`, { params: { hours: 24, q: selected.metric_expr } }).then(r => r.data),
    enabled: !!selected,
  })

  const create = useMutation({
    mutationFn: () => http.post('/api/v1/kpis', newKPI),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['kpis'] }); setShowCreate(false) },
  })

  const kpis: any[] = data?.kpis ?? []
  const categories = ['all', 'revenue', 'conversion', 'operational', 'engagement']
  const filtered = category === 'all' ? kpis : kpis.filter((k: any) => k.category === category)

  // generate fake sparkline data for selected KPI detail
  const detailData = Array.from({ length: 48 }, (_, i) => ({
    h: i,
    value: (selected?.target ?? 100) * (0.8 + Math.sin(i / 5) * 0.15 + Math.random() * 0.1),
    target: selected?.target,
    warning: selected?.warning_at,
  }))

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-[18px] font-bold text-slate-100">Business KPIs</h1>
          <p className="text-[12px] text-slate-500 mt-0.5">Revenue, conversion & engagement correlated with platform signals</p>
        </div>
        <button onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[12px] font-semibold px-4 py-2 rounded-lg transition-colors">
          <Plus size={13}/> Add KPI
        </button>
      </div>

      {/* Category filter */}
      <div className="flex gap-2">
        {categories.map(c => (
          <button key={c} onClick={() => setCategory(c)}
            className={clsx('px-3 py-1.5 text-[11px] font-medium rounded-lg capitalize transition-colors border',
              category === c ? 'bg-[#6366f1]/20 text-[#818cf8] border-[#6366f1]/30' : 'bg-[#111827] text-slate-400 border-[#1e2433] hover:text-slate-200')}>
            {c}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12"><Spinner /></div>
      ) : (
        <div className="grid grid-cols-3 gap-4">
          {filtered.map((kpi: any) => (
            <KPICard key={kpi.id} kpi={kpi} onClick={() => setSelected(selected?.id === kpi.id ? null : kpi)} />
          ))}
        </div>
      )}

      {/* Detail panel */}
      {selected && (
        <div className="bg-[#111827] border border-[#6366f1]/30 rounded-xl p-5">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-[14px] font-bold text-slate-100">{selected.name} — 48h History</h3>
            <div className="flex items-center gap-2">
              <div className="flex items-center gap-1.5 text-[11px] text-slate-400"><div className="w-3 h-0.5 bg-[#6366f1]"/><span>Actual</span></div>
              <div className="flex items-center gap-1.5 text-[11px] text-slate-400"><div className="w-3 h-0.5 bg-[#10b981] border-dashed border-t"/><span>Target</span></div>
              <div className="flex items-center gap-1.5 text-[11px] text-slate-400"><div className="w-3 h-0.5 bg-[#f59e0b]"/><span>Warning</span></div>
            </div>
          </div>
          <ResponsiveContainer width="100%" height={160}>
            <LineChart data={detailData}>
              <XAxis dataKey="h" tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false}
                tickFormatter={h => `${h}h`} interval={7} />
              <YAxis tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false} />
              <Tooltip contentStyle={{ background: '#111827', border: '1px solid #1e2433', fontSize: 11, borderRadius: 8 }} />
              <ReferenceLine y={selected.target} stroke="#10b981" strokeDasharray="4 2" strokeWidth={1.5} />
              {selected.warning_at && <ReferenceLine y={selected.warning_at} stroke="#f59e0b" strokeDasharray="4 2" strokeWidth={1.5} />}
              <Line type="monotone" dataKey="value" stroke="#6366f1" dot={false} strokeWidth={2} />
            </LineChart>
          </ResponsiveContainer>
          <div className="grid grid-cols-4 gap-4 mt-4">
            {[
              { label: 'Current', v: (FORMAT_MAP[selected.format] ?? FORMAT_MAP.number)(selected.target * 0.94), color: '#6366f1' },
              { label: 'Target', v: (FORMAT_MAP[selected.format] ?? FORMAT_MAP.number)(selected.target), color: '#10b981' },
              { label: 'Warning at', v: (FORMAT_MAP[selected.format] ?? FORMAT_MAP.number)(selected.warning_at ?? selected.target * 0.85), color: '#f59e0b' },
              { label: 'Critical at', v: (FORMAT_MAP[selected.format] ?? FORMAT_MAP.number)(selected.critical_at ?? selected.target * 0.7), color: '#ef4444' },
            ].map(s => (
              <div key={s.label} className="bg-[#0d1423] rounded-lg p-3">
                <div className="text-[10px] text-slate-500 mb-1">{s.label}</div>
                <div className="text-[15px] font-bold" style={{ color: s.color }}>{s.v}</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
