// DeploymentIntelligencePage.tsx — Detect deploys, before/after metric comparison
// Dynatrace Deployment Intelligence + Davis problem correlation with deploys

import { useState } from 'react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, LineChart, Line, ReferenceLine, ReferenceArea } from 'recharts'
import { GitBranch, AlertTriangle, CheckCircle, Clock, ArrowRight, TrendingUp, TrendingDown } from 'lucide-react'

const DEPLOYS = [
  { id: 'd1', svc: 'checkout-service', ver: 'v2.4.1', prev: 'v2.4.0', env: 'production', by: 'alice@corp.io', ts: '2024-01-15 19:25 UTC', duration: '4m 12s',
    status: 'healthy', change_rate: 1.4, p99_delta: +12, err_delta: -0.3, traffic_delta: 0,
    problems: 0, alerts: 0, slo_impact: false,
    commit: 'a8f4c2d', message: 'feat: optimize cart calculation algorithm', pr: '#481',
  },
  { id: 'd2', svc: 'user-service', ver: 'v1.8.3', prev: 'v1.8.2', env: 'production', by: 'bob@corp.io', ts: '2024-01-15 18:40 UTC', duration: '3m 47s',
    status: 'regression', change_rate: 8.4, p99_delta: +3800, err_delta: +7.9, traffic_delta: 0,
    problems: 1, alerts: 3, slo_impact: true,
    commit: 'f2b8a1e', message: 'fix: connection pool timeout handling', pr: '#478',
  },
  { id: 'd3', svc: 'api-gateway', ver: 'v3.1.2', prev: 'v3.1.1', env: 'production', by: 'carol@corp.io', ts: '2024-01-15 17:15 UTC', duration: '2m 58s',
    status: 'healthy', change_rate: 0.2, p99_delta: -8, err_delta: -0.1, traffic_delta: +5,
    problems: 0, alerts: 0, slo_impact: false,
    commit: '9c4d7f3', message: 'perf: connection pooling improvements', pr: '#476',
  },
  { id: 'd4', svc: 'payment-service', ver: 'v2.0.1', prev: 'v2.0.0', env: 'staging', by: 'dave@corp.io', ts: '2024-01-15 16:30 UTC', duration: '5m 21s',
    status: 'healthy', change_rate: 0.08, p99_delta: +2, err_delta: 0, traffic_delta: 0,
    problems: 0, alerts: 0, slo_impact: false,
    commit: 'b6e1a8c', message: 'chore: upgrade stripe SDK to 13.x', pr: '#474',
  },
]

const genMetricData = (baseVal: number, deployIdx: number, delta: number) =>
  Array.from({ length: 60 }, (_, i) => ({
    t: i,
    v: i < deployIdx
      ? baseVal + (Math.random() - 0.5) * baseVal * 0.05
      : baseVal + delta + (Math.random() - 0.5) * Math.abs(baseVal + delta) * 0.08,
  }))

export default function DeploymentIntelligencePage() {
  const [selected, setSelected] = useState(DEPLOYS[1]) // default to regression

  const p99Before = 120; const p99After = p99Before + selected.p99_delta
  const errBefore = 0.5; const errAfter = errBefore + selected.err_delta

  const p99Data = genMetricData(p99Before, 30, selected.p99_delta)
  const errData = genMetricData(errBefore, 30, selected.err_delta)

  const statusCol = { healthy: '#0fcf8a', regression: '#ff4d6a', deploying: '#f5a623' }[selected.status] ?? '#546a88'

  return (
    <div className="flex flex-col bg-[#070f1e] min-h-full text-slate-200">
      <div className="px-5 py-4 border-b border-[#1a2d4a]">
        <h1 className="text-[20px] font-bold text-slate-100" style={{ fontFamily: 'Syne, sans-serif' }}>Deployment Intelligence</h1>
        <p className="text-[11px] text-slate-500 mt-0.5">Auto-detect deploys · Before/after metric comparison · AI regression analysis</p>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Deploy list */}
        <div className="w-72 border-r border-[#1a2d4a] overflow-y-auto flex-shrink-0">
          <div className="p-3 text-[9px] font-bold text-slate-600 uppercase tracking-wider">Recent Deployments</div>
          {DEPLOYS.map(d => (
            <div key={d.id} onClick={() => setSelected(d)}
              className="cursor-pointer border-b border-[#0d1a2e] p-3"
              style={{ borderLeft: `3px solid ${d.id === selected.id ? statusCol : 'transparent'}`, background: d.id === selected.id ? 'rgba(108,114,255,.06)' : 'transparent' }}>
              <div className="flex items-center gap-2 mb-1">
                <div className="w-2 h-2 rounded-full" style={{ background: { healthy: '#0fcf8a', regression: '#ff4d6a', deploying: '#f5a623' }[d.status] ?? '#546a88' }} />
                <span className="text-[12px] font-bold text-slate-200">{d.svc}</span>
                <span className="ml-auto text-[10px] text-slate-500 font-mono">{d.ver}</span>
              </div>
              <div className="text-[10px] text-slate-500 mb-1.5">{d.message}</div>
              <div className="flex items-center gap-3 text-[10px]">
                <span className="text-slate-500">{d.ts.split(' ').slice(-2).join(' ')}</span>
                {d.problems > 0 && <span className="text-[#ff4d6a] font-semibold">⚠ {d.problems} problem</span>}
                {d.status === 'healthy' && <span className="text-[#0fcf8a]">✓ Clean</span>}
              </div>
            </div>
          ))}
        </div>

        {/* Detail */}
        <div className="flex-1 overflow-y-auto p-5">
          {/* Deploy header */}
          <div className="flex items-start gap-4 mb-5 p-4 rounded-xl border"
            style={{ background: `${statusCol}08`, borderColor: `${statusCol}40` }}>
            <div className="w-10 h-10 rounded-xl flex items-center justify-center text-xl"
              style={{ background: `${statusCol}20` }}>
              {selected.status === 'regression' ? '⚠️' : '✅'}
            </div>
            <div className="flex-1">
              <div className="flex items-center gap-3 mb-1">
                <span className="text-[16px] font-bold text-slate-100">{selected.svc}</span>
                <span className="text-[11px] font-mono text-slate-400">{selected.prev} → <strong style={{ color: statusCol }}>{selected.ver}</strong></span>
                <span className="px-2 py-0.5 rounded text-[10px] font-bold" style={{ background: `${statusCol}25`, color: statusCol }}>
                  {selected.status.toUpperCase()}
                </span>
              </div>
              <div className="text-[12px] text-slate-300 mb-2">{selected.message}</div>
              <div className="flex gap-6 text-[11px] text-slate-500">
                <span><GitBranch size={11} className="inline mr-1" />{selected.commit}</span>
                <span>PR {selected.pr}</span>
                <span><Clock size={11} className="inline mr-1" />{selected.ts}</span>
                <span>by {selected.by}</span>
                <span>Duration: {selected.duration}</span>
              </div>
            </div>
          </div>

          {/* Before/after KPIs */}
          <div className="grid grid-cols-4 gap-3 mb-5">
            {[
              { l: 'Error Rate (before)', v: `${errBefore.toFixed(1)}%`, c: '#8fa8cc' },
              { l: 'Error Rate (after)', v: `${errAfter.toFixed(1)}%`, c: errAfter > errBefore ? '#ff4d6a' : '#0fcf8a', delta: selected.err_delta > 0 ? `+${selected.err_delta}%` : `${selected.err_delta}%` },
              { l: 'P99 Latency (before)', v: `${p99Before}ms`, c: '#8fa8cc' },
              { l: 'P99 Latency (after)', v: `${p99After}ms`, c: p99After > p99Before * 1.2 ? '#ff4d6a' : p99After > p99Before ? '#f5a623' : '#0fcf8a', delta: selected.p99_delta > 0 ? `+${selected.p99_delta}ms` : `${selected.p99_delta}ms` },
            ].map(k => (
              <div key={k.l} className="bg-[#0b1628] border border-[#1a2d4a] rounded-xl p-3">
                <div className="text-[10px] text-slate-500 mb-1">{k.l}</div>
                <div className="text-[20px] font-bold" style={{ color: k.c, fontFamily: 'Syne, sans-serif' }}>{k.v}</div>
                {(k as any).delta && <div className="text-[11px] font-semibold mt-1" style={{ color: k.c }}>{(k as any).delta}</div>}
              </div>
            ))}
          </div>

          {/* Charts */}
          <div className="grid grid-cols-2 gap-4 mb-5">
            {[
              { label: 'Error Rate', data: errData, unit: '%', col: errAfter > errBefore ? '#ff4d6a' : '#0fcf8a', base: errBefore },
              { label: 'P99 Latency', data: p99Data, unit: 'ms', col: p99After > p99Before * 1.2 ? '#ff4d6a' : '#f5a623', base: p99Before },
            ].map(chart => (
              <div key={chart.label} className="bg-[#0b1628] border border-[#1a2d4a] rounded-xl p-4">
                <div className="flex items-center gap-2 mb-3 text-[12px] font-semibold text-slate-300">
                  {chart.label}
                  <span className="text-[10px] text-slate-500">— Deploy at T+30m</span>
                </div>
                <ResponsiveContainer width="100%" height={100}>
                  <LineChart data={chart.data}>
                    <XAxis hide />
                    <YAxis tick={{ fontSize: 9, fill: '#546a88' }} tickLine={false} axisLine={false} unit={chart.unit} />
                    <Tooltip contentStyle={{ background: '#111f35', border: '1px solid #1a2d4a', fontSize: 11 }} formatter={(v: any) => [v.toFixed(2) + chart.unit]} />
                    <ReferenceLine x={30} stroke="#6c72ff" strokeWidth={1.5} strokeDasharray="3 2" label={{ value: 'Deploy', position: 'top', fill: '#8b90ff', fontSize: 9 }} />
                    <ReferenceArea x1={0} x2={30} fill="#ffffff" fillOpacity={0.01} />
                    <ReferenceArea x1={30} x2={59} fill={chart.col} fillOpacity={0.04} />
                    <Line type="monotone" dataKey="v" stroke={chart.col} dot={false} strokeWidth={2} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ))}
          </div>

          {/* AI analysis */}
          {selected.status === 'regression' && (
            <div className="bg-gradient-to-r from-[#ff4d6a]/10 to-transparent border border-[#ff4d6a]/30 rounded-xl p-4">
              <div className="flex items-center gap-3 mb-3">
                <span className="text-lg">🧠</span>
                <span className="text-[13px] font-bold text-slate-100">AI Regression Analysis</span>
                <span className="text-[10px] bg-[#ff4d6a]/20 text-[#ff4d6a] px-2 py-0.5 rounded font-bold">ROLLBACK RECOMMENDED</span>
              </div>
              <div className="text-[12px] text-slate-300 mb-3">
                Deploy <strong>{selected.ver}</strong> caused a <strong className="text-[#ff4d6a]">+{selected.p99_delta}ms</strong> P99 latency spike and error rate increase to <strong className="text-[#ff4d6a]">{errAfter.toFixed(1)}%</strong> within 8 minutes of deployment. Pattern matches connection pool exhaustion — the commit attempts to increase timeout thresholds, which caused connections to queue rather than fail fast.
              </div>
              <div className="flex gap-3">
                <button className="bg-[#ff4d6a] text-white px-5 py-2 rounded-lg text-[12px] font-bold hover:bg-[#dc2626]">
                  ↩ Rollback to {selected.prev}
                </button>
                <button className="bg-[#0b1628] text-slate-400 border border-[#1a2d4a] px-5 py-2 rounded-lg text-[12px] hover:text-slate-200">
                  📋 View diff
                </button>
                <button className="bg-[#0b1628] text-slate-400 border border-[#1a2d4a] px-5 py-2 rounded-lg text-[12px] hover:text-slate-200">
                  🔕 Mark as expected
                </button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
