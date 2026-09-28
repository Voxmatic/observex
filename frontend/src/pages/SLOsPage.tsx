// SLOsPage.tsx — SLO tracking with burn rate, error budget, multi-window
import { useState } from 'react'
import { Shield, TrendingDown, TrendingUp, AlertTriangle, CheckCircle, Clock, Plus, Flame } from 'lucide-react'

const SLOS = [
  { id: 'slo-001', name: 'API Availability', service: 'api-gateway', target: 99.9, current: 91.2,
    budget: { total: 43.8, remaining: -76.4, pct: -174 }, burnRate: 18.2, window: '30d',
    sli: 'success_rate', status: 'burning', breaches: 2,
    history: [99.95, 99.93, 99.91, 99.88, 99.82, 99.61, 97.4, 95.2, 92.1, 91.2] },
  { id: 'slo-002', name: 'Checkout Success Rate', service: 'checkout-service', target: 99.9, current: 96.2,
    budget: { total: 43.8, remaining: -120.6, pct: -275 }, burnRate: 6.8, window: '30d',
    sli: 'error_rate', status: 'burning', breaches: 1,
    history: [99.9, 99.88, 99.85, 99.6, 99.1, 98.4, 97.8, 96.8, 96.4, 96.2] },
  { id: 'slo-003', name: 'User Service P99 < 500ms', service: 'user-service', target: 99.5, current: 87.4,
    budget: { total: 131.4, remaining: -359, pct: -273 }, burnRate: 12.1, window: '30d',
    sli: 'latency_p99', status: 'burning', breaches: 3,
    history: [99.5, 99.3, 99.1, 98.4, 97.2, 95.8, 93.4, 90.8, 88.9, 87.4] },
  { id: 'slo-004', name: 'Payment Success Rate', service: 'payment-service', target: 99.99, current: 99.91,
    budget: { total: 4.38, remaining: 1.3, pct: 29.7 }, burnRate: 0.8, window: '30d',
    sli: 'success_rate', status: 'at_risk', breaches: 0,
    history: [99.99, 99.99, 99.98, 99.97, 99.96, 99.95, 99.94, 99.93, 99.92, 99.91] },
  { id: 'slo-005', name: 'Synthetic Monitor Uptime', service: 'monitoring', target: 99.5, current: 99.7,
    budget: { total: 131.4, remaining: 92.4, pct: 70.3 }, burnRate: 0.0, window: '30d',
    sli: 'availability', status: 'healthy', breaches: 0,
    history: [99.5, 99.6, 99.65, 99.7, 99.72, 99.74, 99.7, 99.71, 99.7, 99.7] },
  { id: 'slo-006', name: 'ML Inference P99 < 2s', service: 'ml-inference', target: 99.0, current: 91.1,
    budget: { total: 262.8, remaining: -177, pct: -67 }, burnRate: 7.4, window: '30d',
    sli: 'latency_p99', status: 'burning', breaches: 1,
    history: [99.0, 98.8, 98.2, 97.4, 96.1, 94.8, 93.2, 92.0, 91.4, 91.1] },
]

const statusStyles: Record<string, {bg: string, color: string, label: string}> = {
  healthy: { bg: 'rgba(15,207,138,.12)', color: '#0fcf8a', label: '✓ Healthy' },
  at_risk: { bg: 'rgba(245,166,35,.12)', color: '#f5a623', label: '⚠ At Risk' },
  burning: { bg: 'rgba(255,77,106,.12)', color: '#ff4d6a', label: '🔥 Burning' },
}

function MiniTrend({ history, target }: { history: number[], target: number }) {
  const w = 120, h = 36
  const min = Math.min(...history, target - 2)
  const max = Math.max(...history, target) + 0.5
  const scale = (v: number) => h - ((v - min) / (max - min)) * (h - 4) - 2
  const pts = history.map((v, i) => `${(i / (history.length - 1)) * w},${scale(v)}`).join(' ')
  const targetY = scale(target)
  const lastColor = history[history.length - 1] >= target ? '#0fcf8a' : '#ff4d6a'
  return (
    <svg width={w} height={h} style={{ overflow: 'visible' }}>
      <line x1="0" y1={targetY} x2={w} y2={targetY} stroke="#f5a623" strokeWidth="1" strokeDasharray="3,2" opacity="0.6" />
      <polyline points={pts} fill="none" stroke={lastColor} strokeWidth="1.5" strokeLinejoin="round" />
      <circle cx={(history.length - 1) / (history.length - 1) * w} cy={scale(history[history.length - 1])} r="3" fill={lastColor} />
    </svg>
  )
}

export default function SLOsPage() {
  const [selected, setSelected] = useState(SLOS[0])

  const burning = SLOS.filter(s => s.status === 'burning').length
  const healthy = SLOS.filter(s => s.status === 'healthy').length

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
          <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne,sans-serif' }}>SLOs</div>
          <div style={{ fontSize: 12, color: '#546a88', flex: 1 }}>Error budget tracking · Multi-window burn rates · SLI/SLO configuration</div>
          <button style={{ display: 'flex', alignItems: 'center', gap: 6, background: '#6c72ff', color: '#fff', border: 'none', padding: '7px 14px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer' }}>
            <Plus size={13} /> New SLO
          </button>
        </div>
        <div style={{ display: 'flex', gap: 10 }}>
          {[
            { l: 'Total SLOs', v: SLOS.length, c: '#6c72ff' },
            { l: '🔥 Burning', v: burning, c: '#ff4d6a' },
            { l: '✓ Healthy', v: healthy, c: '#0fcf8a' },
            { l: 'Avg Compliance', v: `${(SLOS.reduce((a,s)=>a+s.current,0)/SLOS.length).toFixed(1)}%`, c: '#f5a623' },
          ].map(k => (
            <div key={k.l} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '7px 14px' }}>
              <div style={{ fontSize: 9, color: '#546a88', textTransform: 'uppercase' }}>{k.l}</div>
              <div style={{ fontSize: 18, fontWeight: 800, color: k.c, fontFamily: 'Syne,sans-serif' }}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* SLO list */}
        <div style={{ width: 320, borderRight: '1px solid #1a2d4a', overflowY: 'auto', flexShrink: 0 }}>
          {SLOS.map(s => {
            const st = statusStyles[s.status]
            return (
              <div key={s.id} onClick={() => setSelected(s)}
                style={{ padding: '12px 14px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e', borderLeft: `3px solid ${selected.id === s.id ? st.color : 'transparent'}`, background: selected.id === s.id ? 'rgba(108,114,255,.05)' : 'transparent' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 7 }}>
                  <span style={{ background: st.bg, color: st.color, fontSize: 9, fontWeight: 800, padding: '2px 7px', borderRadius: 10 }}>{st.label}</span>
                  {s.burnRate > 5 && <span style={{ fontSize: 9, background: 'rgba(255,77,106,.2)', color: '#ff4d6a', padding: '1px 6px', borderRadius: 8, fontWeight: 800 }}>{s.burnRate}x burn</span>}
                </div>
                <div style={{ fontSize: 13, fontWeight: 700, color: '#f0f6ff', marginBottom: 4 }}>{s.name}</div>
                <div style={{ marginBottom: 6 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 3, fontSize: 10 }}>
                    <span style={{ color: '#546a88' }}>Current: <span style={{ color: s.current >= s.target ? '#0fcf8a' : '#ff4d6a', fontWeight: 700 }}>{s.current}%</span></span>
                    <span style={{ color: '#546a88' }}>Target: <span style={{ color: '#8fa8cc', fontWeight: 700 }}>{s.target}%</span></span>
                  </div>
                  <div style={{ height: 5, background: '#0b1628', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${Math.min(s.current, 100)}%`, background: s.current >= s.target ? '#0fcf8a' : s.current >= s.target - 2 ? '#f5a623' : '#ff4d6a' }} />
                  </div>
                </div>
                <div style={{ fontSize: 10, color: '#546a88' }}>
                  <span style={{ fontFamily: 'JetBrains Mono,monospace', color: '#6c72ff' }}>{s.service}</span>
                  <span style={{ marginLeft: 8 }}>Error budget: <span style={{ color: s.budget.pct > 0 ? '#0fcf8a' : '#ff4d6a', fontWeight: 700 }}>{s.budget.remaining.toFixed(0)}min remaining</span></span>
                </div>
              </div>
            )
          })}
        </div>

        {/* SLO Detail */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 18, display: 'flex', flexDirection: 'column', gap: 12 }}>
          {/* Header */}
          <div style={{ background: `${statusStyles[selected.status].bg}`, border: `1px solid ${statusStyles[selected.status].color}33`, borderRadius: 14, padding: 16 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 18, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne,sans-serif', marginBottom: 4 }}>{selected.name}</div>
                <div style={{ display: 'flex', gap: 8 }}>
                  <span style={{ background: statusStyles[selected.status].bg, color: statusStyles[selected.status].color, fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 10 }}>{statusStyles[selected.status].label}</span>
                  <span style={{ fontSize: 11, color: '#6c72ff', fontFamily: 'JetBrains Mono,monospace' }}>{selected.service}</span>
                  <span style={{ fontSize: 11, color: '#546a88' }}>SLI: {selected.sli}</span>
                  <span style={{ fontSize: 11, color: '#546a88' }}>Window: {selected.window}</span>
                </div>
              </div>
              {selected.burnRate > 0 && (
                <div style={{ textAlign: 'center', background: 'rgba(255,77,106,.1)', border: '1px solid rgba(255,77,106,.25)', borderRadius: 12, padding: '10px 16px' }}>
                  <Flame size={16} style={{ color: '#ff4d6a', margin: '0 auto 4px' }} />
                  <div style={{ fontSize: 22, fontWeight: 900, color: '#ff4d6a', fontFamily: 'Syne,sans-serif' }}>{selected.burnRate}x</div>
                  <div style={{ fontSize: 10, color: '#546a88' }}>burn rate</div>
                </div>
              )}
            </div>
            {/* Stats */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 10 }}>
              {[
                { l: 'Current', v: `${selected.current}%`, c: selected.current >= selected.target ? '#0fcf8a' : '#ff4d6a' },
                { l: 'Target', v: `${selected.target}%`, c: '#8fa8cc' },
                { l: 'Budget Total', v: `${selected.budget.total.toFixed(0)}m`, c: '#8fa8cc' },
                { l: 'Budget Left', v: `${selected.budget.remaining.toFixed(0)}m`, c: selected.budget.remaining > 0 ? '#0fcf8a' : '#ff4d6a' },
              ].map(k => (
                <div key={k.l} style={{ background: 'rgba(7,15,30,.5)', border: '1px solid #1a2d4a', borderRadius: 10, padding: '8px 12px' }}>
                  <div style={{ fontSize: 10, color: '#546a88' }}>{k.l}</div>
                  <div style={{ fontSize: 16, fontWeight: 800, color: k.c, fontFamily: 'Syne,sans-serif' }}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Trend chart */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
              <span style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef' }}>30-Day Compliance Trend</span>
              <div style={{ display: 'flex', gap: 10, marginLeft: 'auto', fontSize: 10 }}>
                <span style={{ color: '#f5a623' }}>— Target {selected.target}%</span>
                <span style={{ color: selected.current >= selected.target ? '#0fcf8a' : '#ff4d6a' }}>— Actual</span>
              </div>
            </div>
            <MiniTrend history={selected.history} target={selected.target} />
            {/* X labels */}
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 9, color: '#3d5070', marginTop: 4 }}>
              <span>30d ago</span><span>20d ago</span><span>10d ago</span><span>Now</span>
            </div>
          </div>

          {/* Multi-window burn rate */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 14 }}>
            <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 12 }}>Multi-Window Burn Rate</div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4,1fr)', gap: 10 }}>
              {[
                { w: '1h', rate: selected.burnRate * 2.4, threshold: 14.4 },
                { w: '6h', rate: selected.burnRate * 1.8, threshold: 6 },
                { w: '24h', rate: selected.burnRate, threshold: 3 },
                { w: '72h', rate: selected.burnRate * 0.7, threshold: 1 },
              ].map(b => {
                const isFiring = b.rate > b.threshold
                return (
                  <div key={b.w} style={{ background: isFiring ? 'rgba(255,77,106,.07)' : 'rgba(7,15,30,.5)', border: `1px solid ${isFiring ? 'rgba(255,77,106,.25)' : '#1a2d4a'}`, borderRadius: 10, padding: '10px 12px', textAlign: 'center' }}>
                    <div style={{ fontSize: 10, color: '#546a88', marginBottom: 4 }}>Window: {b.w}</div>
                    <div style={{ fontSize: 20, fontWeight: 900, color: isFiring ? '#ff4d6a' : '#0fcf8a', fontFamily: 'Syne,sans-serif' }}>{b.rate.toFixed(1)}x</div>
                    <div style={{ fontSize: 9, color: '#546a88', marginTop: 3 }}>threshold: {b.threshold}x</div>
                    {isFiring && <div style={{ fontSize: 9, color: '#ff4d6a', fontWeight: 700, marginTop: 4 }}>🔥 FIRING</div>}
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
