import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell, Treemap } from 'recharts'
import { DollarSign, TrendingDown, Cpu, HardDrive, Wifi, AlertTriangle, CheckCircle, ArrowRight } from 'lucide-react'
import clsx from 'clsx'

const COST_COLORS = ['#6366f1', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4']

export default function FinOpsPage() {
  const [activeTab, setActiveTab] = useState<'overview' | 'namespaces' | 'workloads' | 'nodes' | 'rightsizing'>('overview')

  const { data: summary } = useQuery({
    queryKey: ['finops-summary'],
    queryFn: () => http.get('/api/v1/finops/summary').then(r => r.data),
    refetchInterval: 300_000,
  })

  const { data: nsData, isLoading: nsLoading } = useQuery({
    queryKey: ['finops-namespaces'],
    queryFn: () => http.get('/api/v1/finops/namespaces').then(r => r.data),
    enabled: activeTab === 'namespaces' || activeTab === 'overview',
    refetchInterval: 300_000,
  })

  const { data: workloadsData, isLoading: wkLoading } = useQuery({
    queryKey: ['finops-workloads'],
    queryFn: () => http.get('/api/v1/finops/workloads').then(r => r.data),
    enabled: activeTab === 'workloads',
    refetchInterval: 300_000,
  })

  const { data: nodesData, isLoading: nodesLoading } = useQuery({
    queryKey: ['finops-nodes'],
    queryFn: () => http.get('/api/v1/finops/nodes').then(r => r.data),
    enabled: activeTab === 'nodes',
    refetchInterval: 300_000,
  })

  const { data: rightsizingData, isLoading: rightLoading } = useQuery({
    queryKey: ['finops-rightsizing'],
    queryFn: () => http.get('/api/v1/finops/rightsizing').then(r => r.data),
    enabled: activeTab === 'rightsizing',
    refetchInterval: 300_000,
  })

  const nsItems: any[] = nsData?.namespaces ?? []
  const wkItems: any[] = workloadsData?.workloads ?? []
  const nodeItems: any[] = nodesData?.nodes ?? []
  const rsItems: any[] = rightsizingData?.recommendations ?? []

  const pieData = (summary?.breakdown ? [
    { name: 'Compute', value: summary.breakdown.compute_pct },
    { name: 'Memory', value: summary.breakdown.memory_pct },
    { name: 'Storage', value: summary.breakdown.storage_pct },
    { name: 'Network', value: summary.breakdown.network_pct },
  ] : [
    { name: 'Compute', value: 62 },
    { name: 'Memory', value: 21 },
    { name: 'Storage', value: 9 },
    { name: 'Network', value: 8 },
  ])

  const budgetPct = summary ? (summary.monthly_cost_usd / summary.monthly_budget_usd * 100) : 85.6
  const budgetColor = budgetPct >= 95 ? '#ef4444' : budgetPct >= 80 ? '#f59e0b' : '#10b981'

  const TABS = ['overview', 'namespaces', 'workloads', 'nodes', 'rightsizing'] as const

  return (
    <div className="flex flex-col gap-5 p-5 bg-[#070c18] min-h-full text-slate-200">
      {/* Summary cards */}
      <div className="grid grid-cols-4 gap-4">
        {[
          { icon: DollarSign, label: 'Monthly Cost', value: `$${(+(summary?.monthly_cost_usd ?? 12840)).toLocaleString()}`, sub: `of $${(+(summary?.monthly_budget_usd ?? 15000)).toLocaleString()} budget`, color: '#6366f1' },
          { icon: AlertTriangle, label: 'Detected Waste', value: `$${(+(summary?.waste_usd ?? 3004)).toLocaleString()}`, sub: `${summary?.waste_pct ?? 23.4}% of spend`, color: '#ef4444' },
          { icon: TrendingDown, label: 'Savings Available', value: `$${(+(summary?.savings_opportunity_usd ?? 2100)).toLocaleString()}/mo`, sub: 'via rightsizing', color: '#10b981' },
          { icon: Cpu, label: 'Overall Efficiency', value: `${summary?.efficiency_pct ?? 68}%`, sub: 'CPU + Memory combined', color: '#f59e0b' },
        ].map(s => (
          <div key={s.label} className="bg-[#111827] rounded-xl border border-[#1e2433] p-4">
            <div className="flex items-center gap-2 mb-2"><s.icon size={14} style={{ color: s.color }} /><span className="text-[11px] text-slate-500">{s.label}</span></div>
            <div className="text-[22px] font-bold" style={{ color: s.color }}>{s.value}</div>
            <div className="text-[10px] text-slate-600 mt-1">{s.sub}</div>
          </div>
        ))}
      </div>

      {/* Budget gauge */}
      <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
        <div className="flex justify-between items-center mb-2">
          <span className="text-[12px] font-semibold text-slate-300">Monthly Budget Utilisation</span>
          <span className="text-[13px] font-bold" style={{ color: budgetColor }}>{budgetPct.toFixed(1)}%</span>
        </div>
        <div className="h-3 bg-[#0d1423] rounded-full overflow-hidden">
          <div className="h-full rounded-full transition-all" style={{ width: `${Math.min(budgetPct, 100)}%`, background: budgetColor }} />
        </div>
        <div className="flex justify-between text-[10px] text-slate-600 mt-1.5">
          <span>$0</span>
          <span>${(+(summary?.monthly_budget_usd ?? 15000)).toLocaleString()}</span>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex border-b border-[#1e2433]">
        {TABS.map(t => (
          <button key={t} onClick={() => setActiveTab(t)}
            className={clsx('py-2.5 px-5 text-[12px] font-medium capitalize border-b-2 transition-colors',
              activeTab === t ? 'border-[#6366f1] text-[#818cf8]' : 'border-transparent text-slate-400 hover:text-slate-200')}>
            {t}
          </button>
        ))}
      </div>

      {/* Overview */}
      {activeTab === 'overview' && (
        <div className="grid grid-cols-2 gap-5">
          {/* Cost by namespace */}
          <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
            <h3 className="text-[12px] font-semibold text-slate-300 mb-4">Cost by Namespace</h3>
            <ResponsiveContainer width="100%" height={180}>
              <BarChart data={nsItems} layout="vertical" margin={{ left: 20, right: 20 }}>
                <XAxis type="number" tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false} tickFormatter={v => `$${v}`} />
                <YAxis type="category" dataKey="name" tick={{ fontSize: 11, fill: '#9ca3af' }} tickLine={false} axisLine={false} width={80} />
                <Tooltip contentStyle={{ background: '#111827', border: '1px solid #1e2433', fontSize: 11, borderRadius: 8 }} formatter={(v: any) => [`$${v.toLocaleString()}`, 'Cost']} />
                <Bar dataKey="monthly_cost" fill="#6366f1" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>

          {/* Cost breakdown pie */}
          <div className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
            <h3 className="text-[12px] font-semibold text-slate-300 mb-4">Cost Breakdown</h3>
            <div className="flex items-center gap-4">
              <ResponsiveContainer width={160} height={160}>
                <PieChart>
                  <Pie data={pieData} cx="50%" cy="50%" innerRadius={40} outerRadius={70} dataKey="value" strokeWidth={0}>
                    {pieData.map((_: any, i: number) => <Cell key={i} fill={COST_COLORS[i % COST_COLORS.length]} />)}
                  </Pie>
                </PieChart>
              </ResponsiveContainer>
              <div className="space-y-2">
                {pieData.map((d: any, i: number) => (
                  <div key={d.name} className="flex items-center gap-2">
                    <div className="w-2.5 h-2.5 rounded-sm shrink-0" style={{ background: COST_COLORS[i % COST_COLORS.length] }} />
                    <span className="text-[11px] text-slate-400">{d.name}</span>
                    <span className="text-[11px] font-bold text-slate-300 ml-auto">{d.value}%</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Namespaces */}
      {activeTab === 'namespaces' && (
        nsLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
        <div className="bg-[#111827] border border-[#1e2433] rounded-xl overflow-hidden">
          <table className="w-full">
            <thead><tr className="border-b border-[#1e2433]">
              {['Namespace', 'Monthly Cost', 'CPU', 'Memory', 'Efficiency'].map(h => (
                <th key={h} className="px-4 py-3 text-left text-[10px] font-semibold text-slate-500 uppercase tracking-wide">{h}</th>
              ))}
            </tr></thead>
            <tbody>
              {nsItems.map((ns: any) => (
                <tr key={ns.name} className="border-b border-[#0d1423] hover:bg-[#0d1423]">
                  <td className="px-4 py-3 text-[13px] font-semibold text-slate-200">{ns.name}</td>
                  <td className="px-4 py-3 text-[13px] font-bold text-[#6366f1]">${(+(ns.monthly_cost ?? 0)).toLocaleString()}/mo</td>
                  <td className="px-4 py-3 text-[12px] text-slate-400">{ns.cpu_req}</td>
                  <td className="px-4 py-3 text-[12px] text-slate-400">{ns.mem_req}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2">
                      <div className="w-20 h-1.5 bg-[#1e2433] rounded-full overflow-hidden">
                        <div className="h-full rounded-full" style={{ width: `${ns.efficiency_pct ?? 0}%`, background: (ns.efficiency_pct ?? 0) > 70 ? '#10b981' : (ns.efficiency_pct ?? 0) > 50 ? '#f59e0b' : '#ef4444' }} />
                      </div>
                      <span className="text-[11px] font-bold" style={{ color: (ns.efficiency_pct ?? 0) > 70 ? '#10b981' : (ns.efficiency_pct ?? 0) > 50 ? '#f59e0b' : '#ef4444' }}>{ns.efficiency_pct}%</span>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Workloads */}
      {activeTab === 'workloads' && (
        wkLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
        <div className="bg-[#111827] border border-[#1e2433] rounded-xl overflow-hidden">
          <table className="w-full">
            <thead><tr className="border-b border-[#1e2433]">
              {['Workload', 'Type', 'Replicas', 'Monthly Cost', 'CPU Eff.', 'Mem Eff.'].map(h => (
                <th key={h} className="px-4 py-3 text-left text-[10px] font-semibold text-slate-500 uppercase tracking-wide">{h}</th>
              ))}
            </tr></thead>
            <tbody>
              {wkItems.map((w: any) => (
                <tr key={w.name} className="border-b border-[#0d1423] hover:bg-[#0d1423]">
                  <td className="px-4 py-3 text-[12px] font-semibold text-[#818cf8]">{w.name}</td>
                  <td className="px-4 py-3"><span className="text-[10px] bg-[#1e2433] text-slate-400 px-2 py-0.5 rounded">{w.type}</span></td>
                  <td className="px-4 py-3 text-[12px] text-slate-300">{w.replicas}</td>
                  <td className="px-4 py-3 text-[12px] font-bold text-[#6366f1]">${(+(w.monthly_cost ?? 0)).toLocaleString()}/mo</td>
                  <td className="px-4 py-3">
                    <span className="text-[11px] font-bold" style={{ color: (w.cpu_efficiency_pct ?? 0) > 70 ? '#10b981' : (w.cpu_efficiency_pct ?? 0) > 50 ? '#f59e0b' : '#ef4444' }}>{w.cpu_efficiency_pct}%</span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="text-[11px] font-bold" style={{ color: (w.mem_efficiency_pct ?? 0) > 70 ? '#10b981' : (w.mem_efficiency_pct ?? 0) > 50 ? '#f59e0b' : '#ef4444' }}>{w.mem_efficiency_pct}%</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Nodes */}
      {activeTab === 'nodes' && (
        nodesLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
        <div className="space-y-3">
          {nodeItems.map((node: any) => (
            <div key={node.name} className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <div className="text-[13px] font-semibold text-slate-200">{node.name}</div>
                  <div className="text-[11px] text-slate-500">{node.instance_type} · ${(+(node.monthly_cost ?? 0)).toLocaleString()}/mo</div>
                </div>
                <span className="text-[11px] font-bold px-2 py-0.5 rounded-full" style={{ background: (node.waste_pct ?? 0) > 40 ? '#ef444420' : '#10b98120', color: (node.waste_pct ?? 0) > 40 ? '#ef4444' : '#10b981' }}>
                  {node.waste_pct}% waste
                </span>
              </div>
              <div className="grid grid-cols-2 gap-3">
                {[{ label: 'CPU', v: node.cpu_util_pct }, { label: 'Memory', v: node.mem_util_pct }].map(m => (
                  <div key={m.label}>
                    <div className="flex justify-between text-[10px] text-slate-500 mb-1"><span>{m.label}</span><span>{m.v}%</span></div>
                    <div className="h-1.5 bg-[#1e2433] rounded-full"><div className="h-full rounded-full" style={{ width: `${m.v ?? 0}%`, background: (m.v ?? 0) > 85 ? '#ef4444' : (m.v ?? 0) > 70 ? '#f59e0b' : '#10b981' }} /></div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Rightsizing */}
      {activeTab === 'rightsizing' && (
        rightLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
        <div className="space-y-3">
          <div className="bg-[#10b981]/10 border border-[#10b981]/30 rounded-xl p-4 flex items-center gap-4">
            <CheckCircle size={20} className="text-[#10b981]" />
            <div>
              <div className="text-[13px] font-semibold text-[#10b981]">Total Savings Opportunity</div>
              <div className="text-[12px] text-slate-400">${(+(rightsizingData?.annual_savings ?? 7208)).toLocaleString()} / year · Apply {rsItems.length} recommendations</div>
            </div>
          </div>
          {rsItems.map((r: any) => (
            <div key={r.workload} className="bg-[#111827] border border-[#1e2433] rounded-xl p-4">
              <div className="flex items-center justify-between mb-2">
                <div className="text-[13px] font-semibold text-slate-200">{r.workload}</div>
                <span className="text-[13px] font-bold text-[#10b981]">{r.monthly_savings}/mo</span>
              </div>
              <div className="flex items-center gap-3 text-[12px] text-slate-400">
                <span className="bg-[#ef4444]/10 text-[#ef4444] px-2 py-0.5 rounded">{r.current}</span>
                <ArrowRight size={12} className="text-slate-600" />
                <span className="bg-[#10b981]/10 text-[#10b981] px-2 py-0.5 rounded">{r.recommended}</span>
                <span className="ml-auto text-[11px] text-slate-500">Confidence: {r.confidence}</span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
