// frontend/src/pages/CostPage.tsx
// FinOps — Cost Attribution Dashboard
// Estimates cloud infrastructure cost per service from live CPU + memory metrics.

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { costs } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, Btn } from '@/components/shared/Layout'
import { DollarSign, TrendingUp, Cpu, MemoryStick, Download } from 'lucide-react'
import {
  BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell,
  PieChart, Pie, Legend,
} from 'recharts'
import clsx from 'clsx'
import type { ServiceCost } from '@/lib/api'

const COLORS = ['#3b82f6','#6366f1','#8b5cf6','#a855f7','#ec4899','#f97316','#f59e0b','#10b981','#14b8a6','#22c55e']

function CostBar({ value, max, color }: { value: number; max: number; color: string }) {
  const pct = max > 0 ? (value / max) * 100 : 0
  return (
    <div className="h-1.5 bg-surface-3 rounded-full overflow-hidden">
      <div className="h-full rounded-full transition-all" style={{ width: `${pct}%`, background: color }} />
    </div>
  )
}

export default function CostPage() {
  const [view, setView] = useState<'service' | 'namespace'>('service')
  const [period, setPeriod] = useState<'hour' | 'day' | 'month'>('day')

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['costs'],
    queryFn: () => costs.report(),
    refetchInterval: 120_000,
  })

  const services = data?.services ?? []
  const totalPerHour = data?.total_per_hour ?? 0

  const periodKey = period === 'hour' ? 'cost_per_hour' : period === 'day' ? 'cost_per_day' : 'cost_per_month'
  const periodLabel = period === 'hour' ? '/hour' : period === 'day' ? '/day' : '/month'
  const totalForPeriod = period === 'hour' ? (data?.total_per_hour ?? 0) :
                         period === 'day'  ? (data?.total_per_day  ?? 0) :
                                             (data?.total_per_month ?? 0)

  // Group by namespace if namespace view
  const nsGroups = services.reduce((acc, s) => {
    const ns = s.namespace || 'default'
    if (!acc[ns]) acc[ns] = { namespace: ns, cost: 0, services: 0 }
    acc[ns].cost += s[periodKey]
    acc[ns].services++
    return acc
  }, {} as Record<string, { namespace: string; cost: number; services: number }>)

  const barData = view === 'service'
    ? services.slice(0, 10).map((s, i) => ({
        name: s.service_name.length > 14 ? s.service_name.slice(0, 13) + '…' : s.service_name,
        cost: s[periodKey],
        color: COLORS[i % COLORS.length],
      }))
    : Object.values(nsGroups).sort((a, b) => b.cost - a.cost).map((n, i) => ({
        name: n.namespace,
        cost: n.cost,
        color: COLORS[i % COLORS.length],
      }))

  const pieData = services.slice(0, 8).map((s, i) => ({
    name: s.service_name.length > 16 ? s.service_name.slice(0, 15) + '…' : s.service_name,
    value: s[periodKey],
    color: COLORS[i % COLORS.length],
  }))

  const handleExportCSV = () => {
    const rows = ['service,namespace,cpu_cores,mem_gb,cost_per_hour,cost_per_day,cost_per_month',
      ...services.map(s => `${s.service_name},${s.namespace},${s.cpu_cores},${s.mem_gb},${s.cost_per_hour},${s.cost_per_day},${s.cost_per_month}`)
    ].join('\n')
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([rows], { type: 'text/csv' }))
    a.download = `observex-costs-${new Date().toISOString().slice(0,10)}.csv`
    a.click()
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Cost Attribution"
        subtitle="Per-service cloud cost from live CPU + memory usage · updated every 2m"
        actions={
          <div className="flex items-center gap-2">
            <Btn size="xs" variant="ghost" onClick={handleExportCSV}>
              <Download size={12} /> CSV
            </Btn>
            <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
              {(['hour','day','month'] as const).map(p => (
                <button key={p} onClick={() => setPeriod(p)}
                  className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                    period === p ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                  /{p}
                </button>
              ))}
            </div>
          </div>
        }
      />

      {isLoading ? (
        <div className="flex justify-center py-20"><Spinner size={24} /></div>
      ) : services.length === 0 ? (
        <div className="flex-1 flex items-center justify-center">
          <EmptyState icon={DollarSign} title="No cost data yet"
            description="Cost attribution requires CPU + memory metrics. Ensure the OneAgent is running and services are being scraped." />
        </div>
      ) : (
        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          {/* Summary cards */}
          <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
            {[
              { label: 'Total/hour',  val: `$${totalPerHour.toFixed(4)}`,            icon: DollarSign,  color: 'text-brand' },
              { label: 'Total/day',   val: `$${(data?.total_per_day ?? 0).toFixed(2)}`, icon: TrendingUp,  color: 'text-slate-200' },
              { label: 'Total/month', val: `$${(data?.total_per_month ?? 0).toFixed(0)}`, icon: DollarSign, color: 'text-slate-200' },
              { label: 'Services',    val: services.length,                           icon: Cpu,         color: 'text-slate-200' },
            ].map(({ label, val, icon: Icon, color }) => (
              <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-slate-500">{label}</span>
                  <Icon size={13} className="text-slate-600" />
                </div>
                <div className={clsx('text-xl font-semibold font-display font-mono', color)}>{val}</div>
              </div>
            ))}
          </div>

          {/* Bar chart + Pie */}
          <div className="grid grid-cols-3 gap-4">
            <div className="col-span-2 bg-surface-1 border border-surface-3 rounded-xl p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-xs font-medium text-slate-400">Cost{periodLabel} by {view}</span>
                <div className="flex gap-1">
                  {(['service','namespace'] as const).map(v => (
                    <button key={v} onClick={() => setView(v)}
                      className={clsx('text-[10px] px-2 py-0.5 rounded transition-colors',
                        view === v ? 'bg-brand/20 text-brand' : 'text-slate-600 hover:text-slate-300')}>
                      {v}
                    </button>
                  ))}
                </div>
              </div>
              <ResponsiveContainer width="100%" height={200}>
                <BarChart data={barData} barSize={18}>
                  <XAxis dataKey="name" tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} />
                  <YAxis tickFormatter={v => `$${v.toFixed(3)}`} tick={{ fill: 'hsl(215 20% 45%)', fontSize: 10 }} tickLine={false} width={60} />
                  <Tooltip
                    contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
                    formatter={(v: number) => [`$${v.toFixed(4)}${periodLabel}`, 'Cost']} />
                  <Bar dataKey="cost" radius={[4,4,0,0]}>
                    {barData.map((d, i) => <Cell key={i} fill={d.color} />)}
                  </Bar>
                </BarChart>
              </ResponsiveContainer>
            </div>
            <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
              <div className="text-xs font-medium text-slate-400 mb-3">Cost share</div>
              <ResponsiveContainer width="100%" height={200}>
                <PieChart>
                  <Pie data={pieData} cx="50%" cy="50%" innerRadius={50} outerRadius={80} dataKey="value" paddingAngle={2}>
                    {pieData.map((d, i) => <Cell key={i} fill={d.color} />)}
                  </Pie>
                  <Tooltip
                    contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
                    formatter={(v: number) => [`$${v.toFixed(4)}`, '']} />
                </PieChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Service table */}
          <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
            <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                            bg-surface-2 border-b border-surface-3 px-4 py-2"
              style={{ gridTemplateColumns: '1fr 70px 70px 90px 90px 90px 80px' }}>
              <span>Service</span><span className="text-right">CPU cores</span>
              <span className="text-right">Mem GB</span>
              <span className="text-right">$/hour</span>
              <span className="text-right">$/day</span>
              <span className="text-right">$/month</span>
              <span>Share</span>
            </div>
            {services.map((s, i) => {
              const sharePct = totalForPeriod > 0 ? (s[periodKey] / totalForPeriod) * 100 : 0
              return (
                <div key={s.service_id}
                  className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
                  style={{ gridTemplateColumns: '1fr 70px 70px 90px 90px 90px 80px' }}>
                  <div>
                    <span className="font-medium text-slate-200 font-mono">{s.service_name}</span>
                    <span className="ml-2 text-slate-600 text-[10px]">{s.namespace}</span>
                  </div>
                  <span className="text-right font-mono text-slate-400">{s.cpu_cores.toFixed(3)}</span>
                  <span className="text-right font-mono text-slate-400">{s.mem_gb.toFixed(2)}</span>
                  <span className="text-right font-mono text-slate-300">${s.cost_per_hour.toFixed(4)}</span>
                  <span className="text-right font-mono text-slate-300">${s.cost_per_day.toFixed(2)}</span>
                  <span className="text-right font-mono text-slate-300">${s.cost_per_month.toFixed(0)}</span>
                  <div className="pl-2">
                    <CostBar value={sharePct} max={100} color={COLORS[i % COLORS.length]} />
                    <span className="text-[9px] text-slate-600">{sharePct.toFixed(1)}%</span>
                  </div>
                </div>
              )
            })}
          </div>

          {/* Pricing note */}
          <div className="text-[10px] text-slate-700 flex items-center gap-2">
            <span>Pricing: ${data?.price_per_core_hour ?? 0.048}/vCPU-hour · ${data?.price_per_gb_hour ?? 0.006}/GB-hour</span>
            <span>·</span>
            <span>Estimates only — configure actual cloud pricing in processor env vars</span>
          </div>
        </div>
      )}
    </div>
  )
}
