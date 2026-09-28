// ChangeTrackingPage.tsx — New Relic Change Tracking parity
// Deployment markers overlaid on every metric chart, APM correlation
import { useState } from 'react'
import { Rocket, GitCommit, User, Clock, TrendingUp, TrendingDown, AlertTriangle, CheckCircle } from 'lucide-react'

const DEPLOYMENTS = [
  { id: 'd1', service: 'api-gateway', version: 'v2.4.1', prev: 'v2.4.0', ts: '18:42:00', author: 'jin.park', commit: 'a1b2c3d', branch: 'fix/auth-middleware', env: 'production', status: 'rolled-back', duration: '14m',
    impact: { errorRateDelta: +420, p99Delta: +18, errorRateBefore: 0.4, errorRateAfter: 2.1, p99Before: 180, p99After: 213 },
    changes: ['Added auth middleware to all routes', 'Upgraded JWT lib to v9.2', 'Added rate limiting per user'] },
  { id: 'd2', service: 'checkout-service', version: 'v2.5.0', prev: 'v2.4.9', ts: '19:30:00', author: 'sara.chen', commit: 'e5f6g7h', branch: 'feat/one-click-checkout', env: 'production', status: 'healthy', duration: '2m 14s',
    impact: { errorRateDelta: +2, p99Delta: -8, errorRateBefore: 1.2, errorRateAfter: 1.23, p99Before: 320, p99After: 295 },
    changes: ['One-click checkout flow', 'Removed legacy payment step', 'Optimized DB queries', 'New promo code validation'] },
  { id: 'd3', service: 'payment-service', version: 'v3.1.2', prev: 'v3.1.1', ts: '16:30:00', author: 'david.wu', commit: 'i9j0k1l', branch: 'patch/retry-logic', env: 'production', status: 'healthy', duration: '1m 48s',
    impact: { errorRateDelta: -12, p99Delta: -5, errorRateBefore: 0.09, errorRateAfter: 0.08, p99Before: 103, p99After: 98 },
    changes: ['Improved retry logic for gateway timeouts', 'Added circuit breaker', 'Enhanced error logging'] },
  { id: 'd4', service: 'ml-inference', version: 'v1.8.3', prev: 'v1.8.2', ts: '14:15:00', author: 'ai-team', commit: 'm2n3o4p', branch: 'perf/model-quantization', env: 'production', status: 'healthy', duration: '3m 02s',
    impact: { errorRateDelta: 0, p99Delta: -28, errorRateBefore: 0.72, errorRateAfter: 0.72, p99Before: 1720, p99After: 1240 },
    changes: ['INT8 model quantization', 'Batch size optimization 8→16', 'GPU memory improvements'] },
]

const statusColors: Record<string, [string, string]> = {
  'healthy': ['rgba(15,207,138,.15)', '#0fcf8a'],
  'rolled-back': ['rgba(255,77,106,.15)', '#ff4d6a'],
  'deploying': ['rgba(108,114,255,.15)', '#8b90ff'],
}

// Mini sparkline chart with deployment marker
function MiniChart({ data, markerAt, color }: { data: number[], markerAt: number, color: string }) {
  const max = Math.max(...data), min = Math.min(...data)
  const norm = (v: number) => 100 - ((v - min) / (max - min || 1)) * 80 - 10
  const pts = data.map((v, i) => `${(i / (data.length - 1)) * 200},${norm(v)}`).join(' ')
  const mx = (markerAt / (data.length - 1)) * 200
  return (
    <svg width="200" height="60" style={{ overflow: 'visible' }}>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" />
      <line x1={mx} y1="5" x2={mx} y2="55" stroke="#6c72ff" strokeWidth="1.5" strokeDasharray="3,2" />
      <polygon points={`${mx},3 ${mx-4},11 ${mx+4},11`} fill="#6c72ff" />
    </svg>
  )
}

export default function ChangeTrackingPage() {
  const [selected, setSelected] = useState(DEPLOYMENTS[0])
  const [envFilter, setEnvFilter] = useState('ALL')

  const errorBefore = [0.4, 0.38, 0.42, 0.4, 0.39, 0.41, 0.40]
  const errorAfter = [0.4, 0.38, 0.42, 2.1, 1.9, 2.3, 2.1, 2.0, 0.4, 0.38]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 3 }}>Change Tracking</div>
        <div style={{ fontSize: 12, color: '#546a88', marginBottom: 12 }}>Deployment markers on all metric charts · Correlation with error rate, P99 latency · Regression detection</div>
        <div style={{ display: 'flex', gap: 10 }}>
          {[{ l: 'Deployments Today', v: 4, c: '#6c72ff' }, { l: 'Healthy', v: 3, c: '#0fcf8a' }, { l: 'Rolled Back', v: 1, c: '#ff4d6a' }, { l: 'Avg Deploy Time', v: '1m 54s', c: '#8fa8cc' }].map(k => (
            <div key={k.l} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '7px 14px' }}>
              <div style={{ fontSize: 10, color: '#546a88' }}>{k.l}</div>
              <div style={{ fontSize: 18, fontWeight: 800, color: k.c, fontFamily: 'Syne, sans-serif' }}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Deployment list */}
        <div style={{ width: 340, borderRight: '1px solid #1a2d4a', overflowY: 'auto' }}>
          {DEPLOYMENTS.map(d => {
            const [bg, color] = statusColors[d.status] || statusColors.healthy
            return (
              <div key={d.id} onClick={() => setSelected(d)} style={{ padding: '14px 16px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e', borderLeft: `3px solid ${selected.id===d.id ? color : 'transparent'}`, background: selected.id===d.id ? 'rgba(108,114,255,.05)' : 'transparent' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  <Rocket size={13} style={{ color, flexShrink: 0 }} />
                  <span style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, fontWeight: 700, color: '#f0f6ff' }}>{d.service}</span>
                  <span style={{ background: bg, color, fontSize: 9, fontWeight: 800, padding: '2px 7px', borderRadius: 10, marginLeft: 'auto' }}>{d.status}</span>
                </div>
                <div style={{ display: 'flex', gap: 8, marginBottom: 6, fontSize: 11 }}>
                  <span style={{ color: '#0fcf8a', fontFamily: 'JetBrains Mono, monospace' }}>{d.version}</span>
                  <span style={{ color: '#546a88' }}>← {d.prev}</span>
                </div>
                <div style={{ display: 'flex', gap: 10, fontSize: 10, color: '#546a88' }}>
                  <span style={{ display: 'flex', gap: 3 }}><User size={9}/> {d.author}</span>
                  <span style={{ display: 'flex', gap: 3 }}><Clock size={9}/> {d.ts}</span>
                  <span>{d.duration}</span>
                  {d.impact.errorRateDelta > 5 && <span style={{ color: '#ff4d6a' }}>⚠ Err +{d.impact.errorRateDelta}%</span>}
                  {d.impact.errorRateDelta < -5 && <span style={{ color: '#0fcf8a' }}>▼ Err {d.impact.errorRateDelta}%</span>}
                </div>
              </div>
            )
          })}
        </div>

        {/* Detail */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          {/* Header */}
          <div style={{ background: `rgba(${selected.status==='healthy'?'15,207,138':'255,77,106'},.06)`, border: `1px solid ${statusColors[selected.status]?.[1] || '#1a2d4a'}44`, borderRadius: 14, padding: 16, marginBottom: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 10 }}>
              <Rocket size={20} style={{ color: statusColors[selected.status]?.[1] || '#6c72ff' }} />
              <div>
                <div style={{ fontSize: 16, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>{selected.service} {selected.version}</div>
                <div style={{ fontSize: 12, color: '#546a88' }}>from {selected.prev} · {selected.ts} · {selected.env} · {selected.duration}</div>
              </div>
              <span style={{ marginLeft: 'auto', background: statusColors[selected.status]?.[0], color: statusColors[selected.status]?.[1], fontSize: 11, fontWeight: 800, padding: '4px 12px', borderRadius: 20 }}>{selected.status.toUpperCase()}</span>
            </div>
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap', fontSize: 11 }}>
              {[{ l: 'Author', v: selected.author, c: '#8fa8cc' }, { l: 'Commit', v: selected.commit, c: '#6c72ff' }, { l: 'Branch', v: selected.branch, c: '#8b90ff' }].map(i => (
                <div key={i.l} style={{ background: 'rgba(7,15,30,.5)', border: '1px solid #1a2d4a', borderRadius: 8, padding: '6px 12px' }}>
                  <span style={{ color: '#546a88' }}>{i.l}: </span>
                  <span style={{ color: i.c, fontFamily: 'JetBrains Mono, monospace' }}>{i.v}</span>
                </div>
              ))}
            </div>
          </div>

          {/* Metric impact */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14, marginBottom: 14 }}>
            {[
              { label: 'Error Rate', before: selected.impact.errorRateBefore, after: selected.impact.errorRateAfter, delta: selected.impact.errorRateDelta, unit: '%', color: selected.impact.errorRateDelta > 5 ? '#ff4d6a' : '#0fcf8a' },
              { label: 'P99 Latency', before: selected.impact.p99Before, after: selected.impact.p99After, delta: selected.impact.p99Delta, unit: 'ms', color: selected.impact.p99Delta > 10 ? '#f5a623' : '#0fcf8a' },
            ].map(m => (
              <div key={m.label} style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 14 }}>
                <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 10 }}>{m.label}</div>
                <div style={{ display: 'flex', gap: 16, alignItems: 'center', marginBottom: 10 }}>
                  <div>
                    <div style={{ fontSize: 10, color: '#546a88' }}>Before</div>
                    <div style={{ fontSize: 18, fontWeight: 800, color: '#8fa8cc', fontFamily: 'Syne, sans-serif' }}>{m.before}{m.unit}</div>
                  </div>
                  <div style={{ color: m.color, fontSize: 16, fontWeight: 800 }}>→</div>
                  <div>
                    <div style={{ fontSize: 10, color: '#546a88' }}>After</div>
                    <div style={{ fontSize: 18, fontWeight: 800, color: m.color, fontFamily: 'Syne, sans-serif' }}>{m.after}{m.unit}</div>
                  </div>
                  <div style={{ marginLeft: 'auto', textAlign: 'right' }}>
                    <div style={{ fontSize: 10, color: '#546a88' }}>Change</div>
                    <div style={{ fontSize: 15, fontWeight: 800, color: m.color }}>{m.delta > 0 ? '+' : ''}{m.delta}%</div>
                  </div>
                </div>
                <MiniChart data={m.label === 'Error Rate' ? [...errorBefore, ...errorAfter] : [320,315,318,313,310,213,220,218,215,211]} markerAt={6} color={m.color} />
              </div>
            ))}
          </div>

          {/* Changes */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 14 }}>
            <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 10 }}>Changes in this deployment</div>
            {selected.changes.map((c, i) => (
              <div key={i} style={{ display: 'flex', gap: 8, alignItems: 'flex-start', padding: '6px 0', borderBottom: i < selected.changes.length-1 ? '1px solid #0d1a2e' : 'none' }}>
                <GitCommit size={11} style={{ color: '#6c72ff', flexShrink: 0, marginTop: 2 }} />
                <span style={{ fontSize: 12, color: '#8fa8cc' }}>{c}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
