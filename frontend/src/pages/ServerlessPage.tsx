// frontend/src/pages/ServerlessPage.tsx  (M2)
// Serverless monitoring — Lambda/Cloud Functions: invocations, cold starts, duration, cost
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { serverless, integrations } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import { Zap, Clock, DollarSign, AlertTriangle, TrendingUp } from 'lucide-react'
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer, BarChart, Bar, Cell } from 'recharts'
import { format, subHours } from 'date-fns'
import clsx from 'clsx'

const TOOLTIP = { contentStyle:{ background:'hsl(220 11% 12%)', border:'1px solid hsl(220 9% 21%)', borderRadius:8, fontSize:11 } }


function MetricCell({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <div className="text-center">
      <div className="text-[9px] text-slate-500 mb-0.5">{label}</div>
      <div className={clsx('text-sm font-semibold font-mono', color ?? 'text-slate-200')}>{value}</div>
    </div>
  )
}

function FunctionRow({ fn, expanded, onToggle }: { fn: any; expanded: boolean; onToggle: () => void }) {
  const errorRate = fn.invocations > 0 ? (fn.errors / fn.invocations * 100) : 0
  const coldStartRate = fn.invocations > 0 ? (fn.coldStarts / fn.invocations * 100) : 0

  // Build sparkline data
  const spark = Array.from({ length: 12 }, (_, i) => ({
    h: i, inv: fn.invocations / 12 * (0.7 + Math.sin(i * 0.5) * 0.3 + Math.sin(i * 2.3 + fn.invocations) * 0.1)
  }))

  return (
    <>
      <div className={clsx('grid items-center px-4 py-3 border-b border-surface-3/50 text-xs cursor-pointer hover:bg-surface-2/30 transition-colors',
        fn.errors > 0 ? 'bg-crit/2' : '')}
        onClick={onToggle}
        style={{ gridTemplateColumns: '200px 90px 60px 100px 90px 90px 80px 80px 1fr' }}>
        <div>
          <div className="font-semibold text-slate-200 font-mono truncate">{fn.name}</div>
          <div className="text-[10px] text-slate-600">{fn.runtime} · {fn.region}</div>
        </div>
        <span className="text-slate-400 font-mono">{(fn.invocations/1000).toFixed(1)}k</span>
        <span className={clsx('font-mono font-semibold', errorRate > 2 ? 'text-crit' : errorRate > 0.5 ? 'text-warn' : 'text-ok')}>
          {errorRate.toFixed(2)}%
        </span>
        <div>
          <span className={clsx('font-mono', fn.avgDurationMs > 1000 ? 'text-warn' : 'text-slate-300')}>
            {fn.avgDurationMs}ms avg
          </span>
          <div className="text-[10px] text-slate-600">p99: {fn.p99DurationMs}ms</div>
        </div>
        <span className={clsx('font-mono', coldStartRate > 5 ? 'text-warn' : 'text-slate-400')}>
          {coldStartRate.toFixed(1)}%
        </span>
        <span className="font-mono text-slate-300">{fn.memory}MB</span>
        <span className={clsx('font-mono font-semibold', fn.costUsd > 0.5 ? 'text-warn' : 'text-ok')}>
          ${fn.costUsd.toFixed(4)}
        </span>
        <span className="font-mono text-slate-500">{fn.timeout}s</span>
        {/* Mini sparkline */}
        <div style={{ display:'flex', alignItems:'flex-end', gap:1, height:24 }}>
          {spark.map((s,i) => (
            <div key={i} style={{ flex:1, height:`${Math.max(10,Math.round(s.inv/fn.invocations*12*100))}%`, background:'#3b82f6', borderRadius:1, opacity:0.6+i/12*0.4 }}/>
          ))}
        </div>
      </div>
      {expanded && (
        <div className="bg-surface-2/50 px-4 py-4 border-b border-surface-3/50 grid grid-cols-4 gap-4 text-xs">
          <div>
            <div className="text-[10px] text-slate-500 mb-2">Invocations (12h)</div>
            <ResponsiveContainer width="100%" height={80}>
              <AreaChart data={spark}>
                <defs><linearGradient id="invGrad" x1="0" y1="0" x2="0" y2="1"><stop offset="5%" stopColor="#3b82f6" stopOpacity={0.3}/><stop offset="95%" stopColor="#3b82f6" stopOpacity={0}/></linearGradient></defs>
                <Area type="monotone" dataKey="inv" stroke="#3b82f6" fill="url(#invGrad)" strokeWidth={1.5} dot={false}/>
                <Tooltip {...TOOLTIP} formatter={(v:number)=>[v.toFixed(0),'invocations']}/>
              </AreaChart>
            </ResponsiveContainer>
          </div>
          <div>
            <div className="text-[10px] text-slate-500 mb-2">Key metrics</div>
            <div className="space-y-1.5">
              {[
                ['Total invocations', fn.invocations.toLocaleString()],
                ['Total errors', fn.errors.toString()],
                ['Cold starts', fn.coldStarts.toString()],
                ['Avg duration', `${fn.avgDurationMs}ms`],
                ['P99 duration', `${fn.p99DurationMs}ms`],
                ['Memory', `${fn.memory}MB`],
                ['Timeout', `${fn.timeout}s`],
                ['Cost/month', `$${(fn.costUsd * 30).toFixed(2)}`],
              ].map(([k, v]) => (
                <div key={k} className="flex justify-between">
                  <span className="text-slate-600">{k}</span>
                  <span className="font-mono text-slate-300">{v}</span>
                </div>
              ))}
            </div>
          </div>
          <div className="col-span-2">
            <div className="text-[10px] text-slate-500 mb-2">Cost optimization</div>
            <div className="space-y-2 text-[10px] text-slate-400">
              {fn.memory > 512 && fn.avgDurationMs < 200 && (
                <div className="bg-ok/10 border border-ok/20 rounded p-2 text-ok">
                  💡 Reduce memory from {fn.memory}MB to 256MB — function is fast and memory-efficient
                </div>
              )}
              {coldStartRate > 5 && (
                <div className="bg-warn/10 border border-warn/20 rounded p-2 text-warn">
                  ⚠ High cold start rate ({coldStartRate.toFixed(1)}%). Consider provisioned concurrency.
                </div>
              )}
              {fn.p99DurationMs > fn.timeout * 800 && (
                <div className="bg-crit/10 border border-crit/20 rounded p-2 text-crit">
                  🔴 P99 duration {fn.p99DurationMs}ms is close to {fn.timeout}s timeout — risk of timeouts
                </div>
              )}
              {fn.errors === 0 && coldStartRate < 2 && (
                <div className="bg-surface-2 border border-surface-3 rounded p-2">
                  ✅ Function is healthy — no optimization needed
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

export default function ServerlessPage() {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [region, setRegion] = useState('')

  const { data: integrationData } = useQuery({
    queryKey: ['integrations'],
    queryFn: () => integrations.list(),
    staleTime: 60_000,
  })
  const { data: fnData, isLoading: fnLoading } = useQuery({
    queryKey: ['serverless-functions', region],
    queryFn: () => serverless.functions({ region: region || undefined }),
    refetchInterval: 60_000,
  })
  const hasAWS = (integrationData?.integrations ?? []).some((i:any) => i.type === 'aws')

  // Use API data if available, fall back to demo data embedded in the backend
  const allFunctions: any[] = fnData?.functions ?? []
  const filtered = allFunctions.filter((f: any) => !region || f.region === region)
  const totalCost = filtered.reduce((s,f) => s+f.costUsd, 0)
  const totalInvocations = filtered.reduce((s,f) => s+f.invocations, 0)
  const totalErrors = filtered.reduce((s,f) => s+f.errors, 0)
  const regions = [...new Set(allFunctions.map((f:any)=>f.region as string))]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader title="Serverless" subtitle="Lambda / Cloud Functions — invocations · cold starts · duration · cost per function"
        actions={
          !hasAWS ? (
            <div className="text-xs text-warn bg-warn/10 border border-warn/20 rounded-lg px-3 py-1.5">
              ⚠ Connect AWS in Integrations to see live data
            </div>
          ) : undefined
        }
      />

      {/* Summary */}
      <div className="grid grid-cols-4 gap-3 p-4 pb-0 flex-shrink-0">
        {[
          { label:'Functions',    value: filtered.length,                    icon:Zap,          color:'text-slate-200' },
          { label:'Invocations/h',value: `${(totalInvocations/24).toFixed(0)}`, icon:TrendingUp,  color:'text-slate-200' },
          { label:'Error rate',   value: `${(totalErrors/totalInvocations*100).toFixed(2)}%`, icon:AlertTriangle, color: totalErrors>0?'text-crit':'text-ok' },
          { label:'Hourly cost',  value: `$${totalCost.toFixed(4)}`,         icon:DollarSign,   color:'text-ok' },
        ].map(({ label, value, icon: Icon, color }) => (
          <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-3">
            <div className="flex items-center justify-between mb-2">
              <span className="text-[10px] text-slate-500">{label}</span>
              <Icon size={12} className="text-slate-600"/>
            </div>
            <div className={clsx('text-xl font-semibold font-mono', color)}>{value}</div>
          </div>
        ))}
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-surface-3 flex-shrink-0">
        <span className="text-xs text-slate-500">Region:</span>
        <div className="flex gap-1">
          <button onClick={()=>setRegion('')} className={clsx('px-2.5 py-1 text-xs rounded transition-colors', !region?'bg-brand/20 text-brand':'text-slate-500 hover:text-slate-300')}>All</button>
          {regions.map(r => (
            <button key={r} onClick={()=>setRegion(r)} className={clsx('px-2.5 py-1 text-xs rounded transition-colors font-mono', region===r?'bg-brand/20 text-brand':'text-slate-500 hover:text-slate-300')}>{r}</button>
          ))}
        </div>
        <span className="ml-auto text-[10px] text-slate-600">{hasAWS ? 'Live from CloudWatch' : 'Demo data — connect AWS integration for live data'}</span>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <div className="grid text-[9px] font-bold text-slate-600 uppercase tracking-wider bg-surface-2 border-b border-surface-3 px-4 py-2"
          style={{ gridTemplateColumns:'200px 90px 60px 100px 90px 90px 80px 80px 1fr' }}>
          <span>Function</span><span>Invocations</span><span>Errors</span>
          <span>Duration</span><span>Cold starts</span><span>Memory</span>
          <span>Cost/h</span><span>Timeout</span><span>Trend</span>
        </div>
        {filtered.map(fn => (
          <FunctionRow key={fn.name} fn={fn}
            expanded={expanded === fn.name}
            onToggle={() => setExpanded(expanded === fn.name ? null : fn.name)}/>
        ))}
      </div>
    </div>
  )
}
