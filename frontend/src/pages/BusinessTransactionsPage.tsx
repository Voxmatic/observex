// BusinessTransactionsPage.tsx — AppDynamics-parity Business Transaction monitoring
// Named flows (Checkout, Login, Payment) with SLA, Flow Map, Transaction Snapshots

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, LineChart, Line, Cell } from 'recharts'
import { Activity, Clock, AlertTriangle, ChevronRight, Zap, Database, GitBranch, CheckCircle } from 'lucide-react'
import clsx from 'clsx'

// ── Sample Data ───────────────────────────────────────────────────────────────
const BTS = [
  { id: 'bt1', name: 'User Checkout', svc: 'checkout-service', method: 'POST', endpoint: '/api/v2/checkout',
    normal: 72, slow: 14, vslow: 8, stalled: 2, errors: 4,
    calls: 1240, art: 892, p99: 2100, baseline: 310, slo: 96.2, col: '#0fcf8a',
    tiers: ['api-gateway', 'checkout-service', 'payment-service', 'postgres-primary', 'redis-cache'],
    impact: '$840/min',
  },
  { id: 'bt2', name: 'User Login', svc: 'user-service', method: 'POST', endpoint: '/api/v1/auth/login',
    normal: 91, slow: 5, vslow: 2, stalled: 0, errors: 2,
    calls: 4820, art: 142, p99: 380, baseline: 120, slo: 99.2, col: '#3d9bff',
    tiers: ['api-gateway', 'user-service', 'postgres-primary', 'redis-cache'],
    impact: '$0/min',
  },
  { id: 'bt3', name: 'Payment Processing', svc: 'payment-service', method: 'POST', endpoint: '/api/v3/payment',
    normal: 97, slow: 1, vslow: 1, stalled: 0, errors: 1,
    calls: 620, art: 98, p99: 240, baseline: 110, slo: 99.9, col: '#0fcf8a',
    tiers: ['api-gateway', 'payment-service', 'postgres-primary', 'stripe-api'],
    impact: '$0/min',
  },
  { id: 'bt4', name: 'ML Inference', svc: 'ml-inference', method: 'POST', endpoint: '/api/v1/infer',
    normal: 58, slow: 24, vslow: 12, stalled: 4, errors: 2,
    calls: 840, art: 1240, p99: 4200, baseline: 440, slo: 91.1, col: '#ff4d6a',
    tiers: ['api-gateway', 'ml-inference', 'redis-cache', 'model-store'],
    impact: '$280/min',
  },
  { id: 'bt5', name: 'Order History', svc: 'checkout-service', method: 'GET', endpoint: '/api/v2/orders',
    normal: 88, slow: 7, vslow: 3, stalled: 1, errors: 1,
    calls: 2140, art: 180, p99: 440, baseline: 160, slo: 98.7, col: '#0fcf8a',
    tiers: ['api-gateway', 'checkout-service', 'postgres-primary'],
    impact: '$0/min',
  },
]

const SNAPSHOTS = [
  { id: 's1', bt: 'bt1', ts: '19:47:23', duration: 2847, status: 'slow', tier: 'checkout-service',
    http_url: '/api/v2/checkout?user=u_8f4a', session: 'sess_9ab2', thread: 'http-nio-8080-exec-3',
    segments: [
      { tier: 'api-gateway', dur: 12, color: '#3d9bff' },
      { tier: 'checkout-service', dur: 1240, color: '#f5a623' },
      { tier: 'payment-service', dur: 84, color: '#0fcf8a' },
      { tier: 'postgres-primary', dur: 1380, color: '#ff4d6a' },
      { tier: 'redis-cache', dur: 8, color: '#a855f7' },
    ],
    slowest: [
      { call: 'SELECT * FROM orders WHERE user_id=$1 AND created_at>NOW()-30d', time: 1380, type: 'db' },
      { call: 'calculateShipping(cartItems, address)', time: 284, type: 'method' },
      { call: 'validatePromoCode(code, userId)', time: 142, type: 'method' },
    ],
    callgraph: [
      { method: 'CheckoutController.processCheckout', pct: 100, time: 2847, line: 142 },
      { method: '  CartService.validateCart', pct: 48, time: 1380, line: 89 },
      { method: '    JdbcTemplate.query', pct: 45, time: 1290, line: 201 },
      { method: '      PreparedStatement.executeQuery', pct: 44, time: 1260, line: null },
      { method: '  ShippingService.calculate', pct: 10, time: 284, line: 54 },
      { method: '  PromoService.validate', pct: 5, time: 142, line: 31 },
      { method: '  PaymentGateway.charge', pct: 3, time: 84, line: 78 },
    ],
  },
  { id: 's2', bt: 'bt4', ts: '19:45:11', duration: 4200, status: 'very_slow', tier: 'ml-inference',
    http_url: '/api/v1/infer?model=recommendation', session: 'sess_7cd1', thread: 'worker-pool-0',
    segments: [
      { tier: 'api-gateway', dur: 8, color: '#3d9bff' },
      { tier: 'ml-inference', dur: 4180, color: '#ff4d6a' },
      { tier: 'redis-cache', dur: 12, color: '#a855f7' },
    ],
    slowest: [
      { call: 'model.predict(features)', time: 3840, type: 'method' },
      { call: 'FeatureStore.fetch(userId)', time: 210, type: 'method' },
    ],
    callgraph: [
      { method: 'InferenceController.infer', pct: 100, time: 4200, line: 78 },
      { method: '  ModelRunner.predict', pct: 91, time: 3840, line: 45 },
      { method: '    TorchModel.forward', pct: 89, time: 3740, line: null },
      { method: '  FeatureStore.fetch', pct: 5, time: 210, line: 22 },
    ],
  },
]

// ── Helpers ───────────────────────────────────────────────────────────────────
function ScoreCard({ normal, slow, vslow, stalled, errors }: any) {
  const total = normal + slow + vslow + stalled + errors
  const bars = [
    { label: 'Normal', val: normal, col: '#0fcf8a' },
    { label: 'Slow', val: slow, col: '#f5a623' },
    { label: 'V.Slow', val: vslow, col: '#ff7a3d' },
    { label: 'Stalled', val: stalled, col: '#a855f7' },
    { label: 'Errors', val: errors, col: '#ff4d6a' },
  ]
  return (
    <div>
      <div style={{ display: 'flex', height: 8, borderRadius: 4, overflow: 'hidden', marginBottom: 6 }}>
        {bars.map(b => (
          <div key={b.label} style={{ width: `${b.val}%`, background: b.col }} />
        ))}
      </div>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
        {bars.map(b => (
          <span key={b.label} style={{ display: 'flex', alignItems: 'center', gap: 3, fontSize: 10, color: '#8fa8cc' }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: b.col, display: 'inline-block' }} />
            {b.label}: <strong style={{ color: b.col }}>{b.val}%</strong>
          </span>
        ))}
      </div>
    </div>
  )
}

function WaterfallBar({ segments, totalDur }: any) {
  let cumulative = 0
  return (
    <div style={{ position: 'relative', height: 32, background: '#0b1628', borderRadius: 6, overflow: 'hidden', marginBottom: 4 }}>
      {segments.map((s: any) => {
        const left = (cumulative / totalDur) * 100
        const width = (s.dur / totalDur) * 100
        cumulative += s.dur
        return (
          <div key={s.tier} style={{
            position: 'absolute', left: `${left}%`, width: `${width}%`, height: '100%',
            background: s.color, opacity: .8, display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 9, color: '#fff', fontWeight: 700, fontFamily: 'monospace', overflow: 'hidden',
            borderRight: '1px solid rgba(0,0,0,.3)',
          }}>
            {width > 8 ? s.tier : ''}
          </div>
        )
      })}
    </div>
  )
}

// ── Main ──────────────────────────────────────────────────────────────────────
export default function BusinessTransactionsPage() {
  const [activeBT, setActiveBT] = useState(BTS[0])
  const [activeSnap, setActiveSnap] = useState<any>(null)
  const [tab, setTab] = useState<'overview' | 'snapshots' | 'flowmap'>('overview')

  const snaps = SNAPSHOTS.filter(s => s.bt === activeBT.id)
  const perfColor = (art: number, baseline: number) =>
    art > baseline * 2 ? '#ff4d6a' : art > baseline * 1.3 ? '#f5a623' : '#0fcf8a'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 0, background: '#070f1e', minHeight: '100%', color: '#c8d8ef' }}>
      {/* Header */}
      <div style={{ padding: '18px 20px 12px', borderBottom: '1px solid #1a2d4a' }}>
        <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 4 }}>
          Business Transactions
        </div>
        <div style={{ fontSize: 12, color: '#546a88' }}>
          Named flows tracked end-to-end across all tiers · AppDynamics-parity SLA monitoring · Transaction snapshots with call graphs
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* BT List */}
        <div style={{ width: 280, borderRight: '1px solid #1a2d4a', overflowY: 'auto', flexShrink: 0 }}>
          <div style={{ padding: '10px 14px 4px', fontSize: 9, color: '#546a88', fontWeight: 700, textTransform: 'uppercase', letterSpacing: '.1em' }}>
            {BTS.length} Business Transactions
          </div>
          {BTS.map(bt => (
            <div key={bt.id} onClick={() => setActiveBT(bt)}
              style={{
                padding: '12px 14px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e',
                borderLeft: `3px solid ${activeBT.id === bt.id ? bt.col : 'transparent'}`,
                background: activeBT.id === bt.id ? 'rgba(108,114,255,.08)' : 'transparent',
              }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
                <span style={{ fontSize: 12, fontWeight: 700, color: '#f0f6ff' }}>{bt.name}</span>
                <span style={{ marginLeft: 'auto', fontSize: 10, color: perfColor(bt.art, bt.baseline), fontFamily: 'monospace', fontWeight: 700 }}>{bt.art}ms</span>
              </div>
              <div style={{ fontSize: 10, color: '#546a88', marginBottom: 8, fontFamily: 'monospace' }}>
                {bt.method} {bt.endpoint}
              </div>
              <ScoreCard normal={bt.normal} slow={bt.slow} vslow={bt.vslow} stalled={bt.stalled} errors={bt.errors} />
              {bt.impact !== '$0/min' && (
                <div style={{ marginTop: 6, fontSize: 10, color: '#ff4d6a', fontWeight: 700 }}>⚠ Impact: {bt.impact}</div>
              )}
            </div>
          ))}
        </div>

        {/* Detail Panel */}
        <div style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column' }}>
          {/* BT Header */}
          <div style={{ padding: '16px 20px', borderBottom: '1px solid #1a2d4a', background: 'rgba(17,31,53,.5)' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 10 }}>
              <div style={{ fontSize: 18, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>{activeBT.name}</div>
              <span style={{ background: perfColor(activeBT.art, activeBT.baseline) + '22', color: perfColor(activeBT.art, activeBT.baseline), fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 12 }}>
                {activeBT.art > activeBT.baseline * 2 ? 'VERY SLOW' : activeBT.art > activeBT.baseline * 1.3 ? 'SLOW' : 'NORMAL'}
              </span>
              <span style={{ fontSize: 11, color: '#546a88', fontFamily: 'monospace' }}>{activeBT.method} {activeBT.endpoint}</span>
              {activeBT.impact !== '$0/min' && (
                <span style={{ marginLeft: 'auto', color: '#ff4d6a', fontWeight: 700, fontSize: 13 }}>Revenue Impact: {activeBT.impact}</span>
              )}
            </div>
            {/* KPI row */}
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 10 }}>
              {[
                { l: 'Calls/min', v: activeBT.calls, c: '#8fa8cc' },
                { l: 'Avg Response', v: activeBT.art + 'ms', c: perfColor(activeBT.art, activeBT.baseline) },
                { l: 'P99 Latency', v: activeBT.p99 + 'ms', c: activeBT.p99 > 2000 ? '#ff4d6a' : '#8fa8cc' },
                { l: 'Baseline', v: activeBT.baseline + 'ms', c: '#546a88' },
                { l: 'SLO', v: activeBT.slo + '%', c: activeBT.slo < 99 ? '#f5a623' : '#0fcf8a' },
                { l: 'Error Rate', v: activeBT.errors + '%', c: activeBT.errors > 2 ? '#ff4d6a' : '#0fcf8a' },
              ].map(k => (
                <div key={k.l} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '10px 12px' }}>
                  <div style={{ fontSize: 10, color: '#546a88', marginBottom: 4 }}>{k.l}</div>
                  <div style={{ fontSize: 18, fontWeight: 800, color: k.c, fontFamily: 'Syne, sans-serif' }}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Tabs */}
          <div style={{ display: 'flex', borderBottom: '1px solid #1a2d4a', padding: '0 20px' }}>
            {(['overview', 'snapshots', 'flowmap'] as const).map(t => (
              <button key={t} onClick={() => setTab(t)}
                style={{ padding: '10px 16px', background: 'none', border: 'none', borderBottom: tab === t ? '2px solid #6c72ff' : '2px solid transparent', color: tab === t ? '#8b90ff' : '#546a88', cursor: 'pointer', fontWeight: 600, fontSize: 12, textTransform: 'capitalize' }}>
                {t === 'flowmap' ? 'Flow Map' : t}
              </button>
            ))}
          </div>

          <div style={{ padding: 20, flex: 1 }}>
            {tab === 'overview' && (
              <div>
                <ScoreCard normal={activeBT.normal} slow={activeBT.slow} vslow={activeBT.vslow} stalled={activeBT.stalled} errors={activeBT.errors} />
                <div style={{ marginTop: 16 }}>
                  <div style={{ fontSize: 11, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', marginBottom: 10 }}>Response Time vs Baseline</div>
                  <div style={{ height: 120, background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '10px 12px' }}>
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={[
                        { t: '-30m', v: activeBT.baseline + Math.random() * 80 },
                        { t: '-25m', v: activeBT.baseline + Math.random() * 60 },
                        { t: '-20m', v: activeBT.baseline * 1.1 },
                        { t: '-15m', v: activeBT.baseline * 1.4 },
                        { t: '-10m', v: activeBT.art * 0.7 },
                        { t: '-5m', v: activeBT.art * 0.9 },
                        { t: 'Now', v: activeBT.art },
                      ]}>
                        <XAxis dataKey="t" tick={{ fontSize: 9, fill: '#546a88' }} />
                        <YAxis tick={{ fontSize: 9, fill: '#546a88' }} />
                        <Tooltip contentStyle={{ background: '#111f35', border: '1px solid #1a2d4a', fontSize: 11 }} formatter={(v: any) => [Math.round(v) + 'ms']} />
                        <Bar dataKey="v" radius={[2, 2, 0, 0]}>
                          {[0,1,2,3,4,5,6].map(i => (
                            <Cell key={i} fill={i >= 4 ? '#ff4d6a' : i >= 3 ? '#f5a623' : '#0fcf8a'} />
                          ))}
                        </Bar>
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </div>
              </div>
            )}

            {tab === 'snapshots' && (
              <div>
                <div style={{ fontSize: 11, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', marginBottom: 12 }}>
                  Transaction Snapshots — Slow & Error Executions
                </div>
                {snaps.length === 0 ? (
                  <div style={{ color: '#546a88', fontSize: 12, padding: 20, textAlign: 'center' }}>No snapshots for this transaction in the selected timeframe</div>
                ) : (
                  snaps.map(snap => (
                    <div key={snap.id} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 12, padding: 16, marginBottom: 12, cursor: 'pointer' }}
                      onClick={() => setActiveSnap(activeSnap?.id === snap.id ? null : snap)}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
                        <span style={{ background: snap.status === 'slow' ? '#f5a62322' : '#ff4d6a22', color: snap.status === 'slow' ? '#f5a623' : '#ff4d6a', fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 12 }}>
                          {snap.status.toUpperCase()}
                        </span>
                        <span style={{ fontSize: 12, color: '#f0f6ff', fontFamily: 'monospace' }}>{snap.http_url}</span>
                        <span style={{ marginLeft: 'auto', fontSize: 14, fontWeight: 800, color: snap.status === 'slow' ? '#f5a623' : '#ff4d6a', fontFamily: 'Syne, sans-serif' }}>{snap.duration}ms</span>
                        <span style={{ fontSize: 11, color: '#546a88' }}>{snap.ts}</span>
                        <ChevronRight size={14} style={{ color: '#546a88', transform: activeSnap?.id === snap.id ? 'rotate(90deg)' : 'none', transition: '.2s' }} />
                      </div>

                      {/* Waterfall */}
                      <div style={{ marginBottom: 8 }}>
                        <div style={{ fontSize: 10, color: '#546a88', marginBottom: 6 }}>Execution waterfall</div>
                        <WaterfallBar segments={snap.segments} totalDur={snap.duration} />
                        <div style={{ display: 'flex', gap: 12, marginTop: 4 }}>
                          {snap.segments.map((s: any) => (
                            <span key={s.tier} style={{ display: 'flex', alignItems: 'center', gap: 3, fontSize: 9, color: '#546a88' }}>
                              <span style={{ width: 6, height: 6, background: s.color, borderRadius: 2, display: 'inline-block' }} />
                              {s.tier} ({s.dur}ms)
                            </span>
                          ))}
                        </div>
                      </div>

                      {/* Expanded: call graph */}
                      {activeSnap?.id === snap.id && (
                        <div style={{ borderTop: '1px solid #1a2d4a', paddingTop: 16, marginTop: 8 }}>
                          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
                            <div>
                              <div style={{ fontSize: 11, fontWeight: 700, color: '#546a88', marginBottom: 8 }}>Slowest Calls</div>
                              {snap.slowest.map((c: any, i: number) => (
                                <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 8px', background: '#111f35', borderRadius: 8, marginBottom: 4 }}>
                                  <span style={{ fontSize: 9, background: c.type === 'db' ? '#3d9bff22' : '#a855f722', color: c.type === 'db' ? '#3d9bff' : '#a855f7', padding: '1px 6px', borderRadius: 8 }}>{c.type}</span>
                                  <span style={{ flex: 1, fontSize: 11, fontFamily: 'monospace', color: '#c8d8ef', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.call}</span>
                                  <span style={{ color: '#ff4d6a', fontWeight: 700, fontSize: 11, fontFamily: 'monospace' }}>{c.time}ms</span>
                                </div>
                              ))}
                            </div>
                            <div>
                              <div style={{ fontSize: 11, fontWeight: 700, color: '#546a88', marginBottom: 8 }}>Call Graph</div>
                              {snap.callgraph.map((c: any, i: number) => (
                                <div key={i} style={{ fontSize: 10, fontFamily: 'monospace', color: '#8fa8cc', padding: '2px 0', display: 'flex', gap: 8 }}>
                                  <span style={{ color: '#ff4d6a', minWidth: 36 }}>{c.pct}%</span>
                                  <span style={{ color: '#546a88', minWidth: 52 }}>{c.time}ms</span>
                                  <span style={{ color: c.method.startsWith(' ') ? '#8fa8cc' : '#f0f6ff' }}>{c.method}</span>
                                  {c.line && <span style={{ color: '#546a88', marginLeft: 'auto' }}>L{c.line}</span>}
                                </div>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  ))
                )}
                {snaps.length === 0 && (
                  <div style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 12, padding: 40, textAlign: 'center', color: '#546a88' }}>
                    No slow or error transactions captured for this flow in the last hour
                  </div>
                )}
              </div>
            )}

            {tab === 'flowmap' && (
              <div>
                <div style={{ fontSize: 11, fontWeight: 700, color: '#546a88', textTransform: 'uppercase', marginBottom: 12 }}>
                  Flow Map — {activeBT.name}
                </div>
                <div style={{ position: 'relative', height: 260, background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 14, overflow: 'hidden' }}>
                  {/* SVG flow lines */}
                  <svg style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', pointerEvents: 'none' }}>
                    {activeBT.tiers.slice(0, -1).map((t, i) => {
                      const x1 = 60 + i * (660 / Math.max(activeBT.tiers.length - 1, 1))
                      const x2 = 60 + (i + 1) * (660 / Math.max(activeBT.tiers.length - 1, 1))
                      const color = i === 0 && activeBT.art > activeBT.baseline * 1.3 ? '#f5a623' : '#0fcf8a'
                      return (
                        <g key={t}>
                          <line x1={x1 + 46} y1={130} x2={x2 - 46} y2={130} stroke={color} strokeWidth={2} strokeOpacity={.6} />
                          <text x={(x1 + x2) / 2} y={118} textAnchor="middle" fill={color} fontSize={9}>
                            {activeBT.calls}/min
                          </text>
                        </g>
                      )
                    })}
                  </svg>
                  {/* Tier nodes */}
                  {activeBT.tiers.map((tier, i) => {
                    const x = 14 + i * (660 / Math.max(activeBT.tiers.length - 1, 1))
                    const isDb = tier.includes('postgres') || tier.includes('redis') || tier.includes('store')
                    const isExt = tier.includes('stripe') || tier.includes('api') && i > 0
                    const isSlow = activeBT.art > activeBT.baseline * 1.3 && i === 1
                    return (
                      <div key={tier} style={{
                        position: 'absolute', left: x, top: '50%', transform: 'translateY(-50%)',
                        width: 92, background: isSlow ? 'rgba(245,166,35,.12)' : 'rgba(17,31,53,.9)',
                        border: `1px solid ${isSlow ? '#f5a623' : '#254060'}`,
                        borderRadius: 10, padding: '10px 8px', textAlign: 'center',
                      }}>
                        <div style={{ fontSize: 14, marginBottom: 4 }}>{isDb ? '🗄' : isExt ? '🌐' : '⬡'}</div>
                        <div style={{ fontSize: 10, fontWeight: 700, color: isSlow ? '#f5a623' : '#c8d8ef', wordBreak: 'break-all' }}>{tier}</div>
                        <div style={{ fontSize: 9, color: '#546a88', marginTop: 4, fontFamily: 'monospace' }}>
                          {isSlow ? activeBT.art + 'ms' : Math.round(activeBT.art * 0.1 + Math.random() * 20) + 'ms'}
                        </div>
                      </div>
                    )
                  })}
                </div>
                <div style={{ display: 'flex', gap: 16, marginTop: 10, fontSize: 10, color: '#546a88' }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 10, height: 2, background: '#0fcf8a', display: 'inline-block' }} />Normal</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 10, height: 2, background: '#f5a623', display: 'inline-block' }} />Slower than baseline</span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}><span style={{ width: 10, height: 2, background: '#ff4d6a', display: 'inline-block' }} />Critically slow</span>
                  <span style={{ marginLeft: 'auto' }}>Solid line = sync · Dashed = async</span>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
