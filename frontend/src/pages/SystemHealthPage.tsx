// frontend/src/pages/SystemHealthPage.tsx
// Global system health — platform health score, dependency tree, MTTR/MTTA, SLO aggregate

import { useQuery } from '@tanstack/react-query'
import { topology, problems, slos, synthetic, nodes as nodesApi } from '@/lib/api'
import { PageHeader, Spinner, StatusDot, SeverityBadge } from '@/components/shared/Layout'
import { ShieldCheck, TrendingUp, TrendingDown, Minus, Activity, Radio, Server } from 'lucide-react'
import { AreaChart, Area, ResponsiveContainer, Tooltip, RadialBarChart, RadialBar } from 'recharts'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

// ── Health score gauge ────────────────────────────────────────────────────────

function HealthGauge({ score }: { score: number }) {
  const color = score >= 95 ? '#22c55e' : score >= 80 ? '#f59e0b' : '#ef4444'
  const r = 70, cx = 90, cy = 90
  const toRad = (d: number) => (d * Math.PI) / 180
  const startAngle = -210
  const pct = score / 100
  const endAngle = startAngle + pct * 240
  const arcPath = (s: number, e: number) => {
    const sx = cx + r * Math.cos(toRad(s)), sy = cy + r * Math.sin(toRad(s))
    const ex = cx + r * Math.cos(toRad(e)), ey = cy + r * Math.sin(toRad(e))
    const large = Math.abs(e - s) > 180 ? 1 : 0
    return `M ${sx} ${sy} A ${r} ${r} 0 ${large} 1 ${ex} ${ey}`
  }
  const label = score >= 95 ? 'Excellent' : score >= 85 ? 'Good' : score >= 70 ? 'Degraded' : 'Critical'
  return (
    <svg width="180" height="110" viewBox="0 0 180 110">
      <path d={arcPath(-210, 30)} fill="none" stroke="hsl(220 9% 18%)" strokeWidth="12" strokeLinecap="round" />
      <path d={arcPath(startAngle, endAngle)} fill="none" stroke={color} strokeWidth="12" strokeLinecap="round" />
      <text x="90" y="85" textAnchor="middle" fill={color} fontSize="28" fontWeight="800" fontFamily="JetBrains Mono,monospace">{score}</text>
      <text x="90" y="100" textAnchor="middle" fill="hsl(215 20% 45%)" fontSize="11" fontFamily="Plus Jakarta Sans,sans-serif">{label}</text>
    </svg>
  )
}

// ── Status category card ──────────────────────────────────────────────────────

function CategoryCard({ label, ok, warn, crit, icon: Icon }: {
  label: string; ok: number; warn: number; crit: number
  icon: React.ElementType
}) {
  const total = ok + warn + crit
  const pctOk = total > 0 ? (ok / total) * 100 : 100
  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <Icon size={13} className="text-slate-500" />
          <span className="text-xs font-medium text-slate-400">{label}</span>
        </div>
        <span className={clsx('text-xs font-semibold', crit > 0 ? 'text-crit' : warn > 0 ? 'text-warn' : 'text-ok')}>
          {crit > 0 ? `${crit} critical` : warn > 0 ? `${warn} warning` : 'All healthy'}
        </span>
      </div>
      <div className="flex items-center gap-2 mb-2">
        <span className="text-2xl font-bold font-mono text-slate-200">{total}</span>
        <span className="text-xs text-slate-600">total</span>
      </div>
      <div className="h-2 bg-surface-3 rounded-full overflow-hidden flex">
        <div className="bg-ok h-full" style={{ width: `${(ok/total)*100}%` }} />
        <div className="bg-warn h-full" style={{ width: `${(warn/total)*100}%` }} />
        <div className="bg-crit h-full" style={{ width: `${(crit/total)*100}%` }} />
      </div>
      <div className="flex gap-3 mt-1.5 text-[10px] text-slate-600">
        <span className="text-ok">{ok} healthy</span>
        {warn > 0 && <span className="text-warn">{warn} warn</span>}
        {crit > 0 && <span className="text-crit">{crit} crit</span>}
      </div>
    </div>
  )
}

// ── MTTR/MTTA trend card ──────────────────────────────────────────────────────

function MTTRCard() {
  // Demo MTTR/MTTA data
  const data = [
    { day: 'Mon', mttr: 22, mtta: 5 },
    { day: 'Tue', mttr: 18, mtta: 4 },
    { day: 'Wed', mttr: 31, mtta: 7 },
    { day: 'Thu', mttr: 14, mtta: 3 },
    { day: 'Fri', mttr: 19, mtta: 5 },
    { day: 'Sat', mttr: 8,  mtta: 2 },
    { day: 'Sun', mttr: 12, mtta: 4 },
  ]
  const avgMTTR = Math.round(data.reduce((s, d) => s + d.mttr, 0) / data.length)
  const avgMTTA = Math.round(data.reduce((s, d) => s + d.mtta, 0) / data.length)

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs font-medium text-slate-400">MTTR / MTTA — 7 days</span>
        <div className="flex gap-3 text-[10px]">
          <span className="text-brand">avg MTTR: {avgMTTR}m</span>
          <span className="text-ok">avg MTTA: {avgMTTA}m</span>
        </div>
      </div>
      <ResponsiveContainer width="100%" height={80}>
        <AreaChart data={data}>
          <defs>
            <linearGradient id="mttrGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="hsl(217 91% 60%)" stopOpacity={0.3} />
              <stop offset="95%" stopColor="hsl(217 91% 60%)" stopOpacity={0} />
            </linearGradient>
            <linearGradient id="mttaGrad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="#22c55e" stopOpacity={0.3} />
              <stop offset="95%" stopColor="#22c55e" stopOpacity={0} />
            </linearGradient>
          </defs>
          <Tooltip
            contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
            formatter={(v: number, name: string) => [`${v}m`, name.toUpperCase()]} />
          <Area type="monotone" dataKey="mttr" stroke="hsl(217 91% 60%)" fill="url(#mttrGrad)" strokeWidth={1.5} dot={false} />
          <Area type="monotone" dataKey="mtta" stroke="#22c55e" fill="url(#mttaGrad)" strokeWidth={1.5} dot={false} />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  )
}

// ── SLO aggregate ─────────────────────────────────────────────────────────────

function SLOAggregate({ sloList }: { sloList: any[] }) {
  const total    = sloList.length
  const ok       = sloList.filter(s => s.status === 'OK').length
  const warn     = sloList.filter(s => s.status === 'WARN').length
  const breached = sloList.filter(s => s.status === 'BREACHED').length
  const avgBudget = total > 0
    ? sloList.reduce((s, sl) => s + (sl.budget_left ?? 100), 0) / total
    : 100

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="flex items-center justify-between mb-3">
        <span className="text-xs font-medium text-slate-400">SLO aggregate</span>
        <span className={clsx('text-xs font-semibold', breached > 0 ? 'text-crit' : warn > 0 ? 'text-warn' : 'text-ok')}>
          {total - breached}/{total} within target
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2 mb-3">
        <div className="text-center bg-surface-2 rounded-lg p-2">
          <div className="text-[10px] text-slate-600">Healthy</div>
          <div className="text-xl font-semibold text-ok">{ok}</div>
        </div>
        <div className="text-center bg-surface-2 rounded-lg p-2">
          <div className="text-[10px] text-slate-600">Warning</div>
          <div className="text-xl font-semibold text-warn">{warn}</div>
        </div>
        <div className="text-center bg-surface-2 rounded-lg p-2">
          <div className="text-[10px] text-slate-600">Breached</div>
          <div className="text-xl font-semibold text-crit">{breached}</div>
        </div>
      </div>
      <div>
        <div className="flex justify-between text-[10px] text-slate-600 mb-1">
          <span>Avg error budget remaining</span>
          <span className={clsx(avgBudget < 20 ? 'text-crit' : avgBudget < 50 ? 'text-warn' : 'text-ok')}>
            {avgBudget.toFixed(1)}%
          </span>
        </div>
        <div className="h-2 bg-surface-3 rounded-full overflow-hidden">
          <div className={clsx('h-full rounded-full', avgBudget < 20 ? 'bg-crit' : avgBudget < 50 ? 'bg-warn' : 'bg-ok')}
            style={{ width: `${avgBudget}%` }} />
        </div>
      </div>
    </div>
  )
}

// ── Main ──────────────────────────────────────────────────────────────────────

export default function SystemHealthPage() {
  const { data: topoData }  = useQuery({ queryKey: ['topo'],     queryFn: () => topology.graph(),     refetchInterval: 30_000 })
  const { data: probData }  = useQuery({ queryKey: ['probs'],    queryFn: () => problems.list({ limit: 10 }), refetchInterval: 15_000 })
  const { data: sloData }   = useQuery({ queryKey: ['slos-sys'], queryFn: () => slos.list({ limit: 50 }), refetchInterval: 60_000 })
  const { data: synData }   = useQuery({ queryKey: ['syn-sys'],  queryFn: () => synthetic.states(),   refetchInterval: 30_000 })
  const { data: nodeData }  = useQuery({ queryKey: ['nodes-sys'],queryFn: () => nodesApi.list(),      refetchInterval: 30_000 })

  const services  = topoData?.nodes ?? []
  const openProbs = probData?.problems?.filter((p: any) => p.status === 'OPEN') ?? []
  const sloList   = sloData?.slos ?? []
  const synStates = synData?.states ?? []
  const nodeList  = nodeData?.nodes ?? []

  const svcOk   = services.filter((s: any) => s.health?.state === 'GOOD').length
  const svcWarn = services.filter((s: any) => s.health?.state === 'DEGRADED').length
  const svcCrit = services.filter((s: any) => s.health?.state === 'CRITICAL').length

  const synOk   = synStates.filter((s: any) => s.status === 'UP').length
  const synWarn = synStates.filter((s: any) => s.status === 'DEGRADED').length
  const synCrit = synStates.filter((s: any) => s.status === 'DOWN').length

  const nodeOk   = nodeList.filter((n: any) => n.status === 'healthy').length
  const nodeWarn = nodeList.filter((n: any) => n.status === 'warning').length
  const nodeCrit = nodeList.filter((n: any) => n.status === 'critical').length

  const sloOk      = sloList.filter((s: any) => s.status === 'OK').length
  const sloBreach  = sloList.filter((s: any) => s.status === 'BREACHED').length
  const critProbs  = openProbs.filter((p: any) => p.severity === 'CRITICAL').length

  // Compute global health score
  const score = Math.max(0, Math.min(100, Math.round(
    100
    - critProbs * 15
    - (openProbs.length - critProbs) * 5
    - svcCrit * 8
    - svcWarn * 2
    - synCrit * 6
    - sloBreach * 10
    - nodeCrit * 5
    - nodeWarn * 1
  )))

  return (
    <div className="flex-1 overflow-y-auto p-4">
      <PageHeader title="System Health" subtitle="Global platform health score · dependency status · MTTR/MTTA · SLO aggregate" />

      <div className="grid grid-cols-3 gap-4 mb-4">
        {/* Health gauge */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4 flex flex-col items-center justify-center">
          <div className="text-xs font-medium text-slate-400 mb-3">Platform health score</div>
          <HealthGauge score={score} />
          <div className="mt-2 text-[10px] text-slate-600 text-center">
            Composite score from services, SLOs, incidents, nodes, and synthetic checks
          </div>
        </div>

        {/* Open incidents */}
        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="text-xs font-medium text-slate-400 mb-3">Active incidents</div>
          <div className="grid grid-cols-2 gap-2 mb-3">
            <div className="bg-surface-2 rounded-lg p-3 text-center">
              <div className="text-[10px] text-slate-600">Open</div>
              <div className={clsx('text-2xl font-semibold', openProbs.length > 0 ? 'text-crit' : 'text-ok')}>
                {openProbs.length}
              </div>
            </div>
            <div className="bg-surface-2 rounded-lg p-3 text-center">
              <div className="text-[10px] text-slate-600">Critical</div>
              <div className={clsx('text-2xl font-semibold', critProbs > 0 ? 'text-crit' : 'text-ok')}>
                {critProbs}
              </div>
            </div>
          </div>
          <div className="space-y-1.5">
            {openProbs.slice(0, 3).map((p: any) => (
              <div key={p.id} className="flex items-center gap-2 text-xs">
                <SeverityBadge severity={p.severity} />
                <span className="flex-1 text-slate-300 truncate">{p.title}</span>
              </div>
            ))}
            {!openProbs.length && (
              <div className="text-xs text-ok flex items-center gap-1.5 py-2">
                <span className="w-2 h-2 rounded-full bg-ok" /> All systems operational
              </div>
            )}
          </div>
        </div>

        {/* MTTR */}
        <MTTRCard />
      </div>

      {/* Category cards */}
      <div className="grid grid-cols-4 gap-3 mb-4">
        <CategoryCard label="Services"   ok={svcOk}  warn={svcWarn}  crit={svcCrit}  icon={Activity} />
        <CategoryCard label="Nodes"      ok={nodeOk} warn={nodeWarn} crit={nodeCrit} icon={Server} />
        <CategoryCard label="Synthetic"  ok={synOk}  warn={synWarn}  crit={synCrit}  icon={Radio} />
        <CategoryCard label="SLOs"       ok={sloOk}  warn={sloList.filter((s:any)=>s.status==='WARN').length} crit={sloBreach} icon={ShieldCheck} />
      </div>

      {/* SLO aggregate + service health */}
      <div className="grid grid-cols-2 gap-4">
        <SLOAggregate sloList={sloList} />

        <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
          <div className="flex items-center justify-between mb-3">
            <span className="text-xs font-medium text-slate-400">Service health heatmap</span>
            <span className="text-[10px] text-slate-600">{services.length} services</span>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {services.slice(0, 40).map((s: any) => {
              const c = s.health?.state === 'GOOD' ? 'bg-ok' : s.health?.state === 'DEGRADED' ? 'bg-warn' : 'bg-crit'
              return (
                <div key={s.id} title={`${s.name}: ${s.health?.state}`}
                  className={clsx('w-5 h-5 rounded-sm opacity-80 cursor-pointer hover:opacity-100', c)} />
              )
            })}
            {services.length === 0 && (
              <div className="text-xs text-slate-600">No services yet</div>
            )}
          </div>
          <div className="flex gap-3 mt-3 text-[10px]">
            <span className="text-ok">■ Healthy ({svcOk})</span>
            <span className="text-warn">■ Warning ({svcWarn})</span>
            <span className="text-crit">■ Critical ({svcCrit})</span>
          </div>
        </div>
      </div>
    </div>
  )
}
