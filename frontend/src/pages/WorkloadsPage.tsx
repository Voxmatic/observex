// WorkloadsPage.tsx — New Relic Workloads parity: team-based entity grouping
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { workloads as workloadsApi } from '@/lib/api'
import { Plus, Users, CheckCircle, AlertTriangle, XCircle } from 'lucide-react'

const WORKLOADS = [
  {
    id: 'wl-checkout', name: 'Checkout Team', owner: 'commerce@corp.io', env: 'production',
    health: 'degraded', healthPct: 72,
    entities: [
      { name: 'checkout-service', type: 'SERVICE', status: 'warning', p99: '892ms', err: '3.2%' },
      { name: 'payment-service', type: 'SERVICE', status: 'healthy', p99: '98ms', err: '0.08%' },
      { name: 'postgres-primary', type: 'DATABASE', status: 'critical', p99: null, err: null },
      { name: 'redis-cache', type: 'DATABASE', status: 'healthy', p99: null, err: null },
      { name: 'checkout-ui', type: 'BROWSER', status: 'warning', p99: null, err: null },
    ],
    slos: [
      { name: 'Checkout success rate', target: 99.9, current: 96.2, ok: false },
      { name: 'Payment P99 < 500ms', target: 99.5, current: 99.9, ok: true },
    ],
    errorCount: 247,
    openProblems: 2,
  },
  {
    id: 'wl-platform', name: 'Platform Team', owner: 'platform@corp.io', env: 'production',
    health: 'critical', healthPct: 45,
    entities: [
      { name: 'api-gateway', type: 'SERVICE', status: 'warning', p99: '284ms', err: '1.84%' },
      { name: 'user-service', type: 'SERVICE', status: 'critical', p99: '4200ms', err: '8.4%' },
      { name: 'k8s-node-01', type: 'HOST', status: 'healthy', p99: null, err: null },
      { name: 'k8s-node-02', type: 'HOST', status: 'warning', p99: null, err: null },
    ],
    slos: [
      { name: 'API availability', target: 99.9, current: 91.2, ok: false },
      { name: 'User service P99', target: 99.5, current: 87.4, ok: false },
    ],
    errorCount: 1842,
    openProblems: 1,
  },
  {
    id: 'wl-ml', name: 'ML Platform', owner: 'ml@corp.io', env: 'production',
    health: 'warning', healthPct: 81,
    entities: [
      { name: 'ml-inference', type: 'SERVICE', status: 'warning', p99: '1240ms', err: '0.72%' },
      { name: 'model-store', type: 'DATABASE', status: 'healthy', p99: null, err: null },
      { name: 'recommendation-svc', type: 'SERVICE', status: 'warning', p99: '2100ms', err: '1.2%' },
    ],
    slos: [
      { name: 'Inference P99 < 2s', target: 99.0, current: 91.1, ok: false },
    ],
    errorCount: 84,
    openProblems: 1,
  },
  {
    id: 'wl-notification', name: 'Notification Service', owner: 'platform@corp.io', env: 'production',
    health: 'healthy', healthPct: 99,
    entities: [
      { name: 'notification-svc', type: 'SERVICE', status: 'healthy', p99: '45ms', err: '0.04%' },
    ],
    slos: [
      { name: 'Delivery success', target: 99.5, current: 99.8, ok: true },
    ],
    errorCount: 3,
    openProblems: 0,
  },
]

const healthColor = { healthy: '#0fcf8a', warning: '#f5a623', degraded: '#f5a623', critical: '#ff4d6a' } as any
const statusColor = { healthy: '#0fcf8a', warning: '#f5a623', critical: '#ff4d6a' } as any

export default function WorkloadsPage() {
  const [selectedId, setSelectedId] = useState(WORKLOADS[0].id)
  const { data } = useQuery({
    queryKey: ['workloads'],
    queryFn: async () => { try { return await workloadsApi.list() } catch { return { workloads: WORKLOADS } } },
    staleTime: 30_000,
  })
  const workloads = ((data as any)?.workloads ?? WORKLOADS) as typeof WORKLOADS
  const selected = workloads.find(w => w.id === selectedId) ?? workloads[0] ?? WORKLOADS[0]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 4 }}>
          <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>Workloads</div>
          <button style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 6, background: '#6c72ff', color: '#fff', border: 'none', padding: '7px 14px', borderRadius: 8, fontSize: 12, fontWeight: 700, cursor: 'pointer' }}>
            <Plus size={13} /> New Workload
          </button>
        </div>
        <div style={{ fontSize: 12, color: '#546a88' }}>Team-based entity grouping · Health rollup · Scoped SLOs · Unified error view</div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Workload list */}
        <div style={{ width: 280, borderRight: '1px solid #1a2d4a', overflowY: 'auto', flexShrink: 0 }}>
          {workloads.map(w => (
            <div key={w.id} onClick={() => setSelectedId(w.id)}
              style={{ padding: '14px 16px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e', borderLeft: `3px solid ${selected.id===w.id ? healthColor[w.health] : 'transparent'}`, background: selected.id===w.id ? 'rgba(108,114,255,.06)' : 'transparent' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 6 }}>
                <Users size={13} style={{ color: '#6c72ff' }} />
                <span style={{ fontSize: 13, fontWeight: 700, color: '#f0f6ff' }}>{w.name}</span>
                <div style={{ width: 8, height: 8, borderRadius: '50%', background: healthColor[w.health], marginLeft: 'auto' }} />
              </div>
              <div style={{ fontSize: 10, color: '#546a88', marginBottom: 6 }}>{w.owner} · {w.env}</div>
              {/* Mini health bar */}
              <div style={{ height: 4, background: '#0b1628', borderRadius: 2, overflow: 'hidden', marginBottom: 5 }}>
                <div style={{ height: '100%', width: `${w.healthPct}%`, background: healthColor[w.health] }} />
              </div>
              <div style={{ display: 'flex', gap: 10, fontSize: 10, color: '#546a88' }}>
                <span>{w.entities.length} entities</span>
                {w.openProblems > 0 && <span style={{ color: '#ff4d6a', fontWeight: 700 }}>⚠ {w.openProblems} problems</span>}
                <span style={{ marginLeft: 'auto' }}>{w.healthPct}%</span>
              </div>
            </div>
          ))}
        </div>

        {/* Workload detail */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
            <div>
              <div style={{ fontSize: 18, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>{selected.name}</div>
              <div style={{ fontSize: 12, color: '#546a88' }}>{selected.owner}</div>
            </div>
            <span style={{ background: `${healthColor[selected.health]}22`, color: healthColor[selected.health], fontSize: 11, fontWeight: 700, padding: '4px 12px', borderRadius: 20 }}>{selected.health.toUpperCase()}</span>
            {selected.openProblems > 0 && (
              <span style={{ background: 'rgba(255,77,106,.15)', color: '#ff4d6a', fontSize: 11, fontWeight: 700, padding: '4px 12px', borderRadius: 20 }}>
                {selected.openProblems} open {selected.openProblems === 1 ? 'problem' : 'problems'}
              </span>
            )}
            <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
              <button style={{ background: '#0b1628', border: '1px solid #1a2d4a', color: '#8fa8cc', padding: '6px 14px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}>View Errors Inbox</button>
            </div>
          </div>

          {/* SLOs */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 16, marginBottom: 14 }}>
            <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 12 }}>SLO Status</div>
            {selected.slos.map((s, i) => (
              <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 8 }}>
                {s.ok ? <CheckCircle size={14} style={{ color: '#0fcf8a', flexShrink: 0 }} /> : <XCircle size={14} style={{ color: '#ff4d6a', flexShrink: 0 }} />}
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, color: '#c8d8ef', marginBottom: 3 }}>{s.name}</div>
                  <div style={{ height: 5, background: '#0b1628', borderRadius: 3, overflow: 'hidden' }}>
                    <div style={{ height: '100%', width: `${s.current}%`, background: s.ok ? '#0fcf8a' : '#ff4d6a' }} />
                  </div>
                </div>
                <div style={{ textAlign: 'right', flexShrink: 0 }}>
                  <div style={{ fontSize: 14, fontWeight: 800, color: s.ok ? '#0fcf8a' : '#ff4d6a', fontFamily: 'Syne, sans-serif' }}>{s.current}%</div>
                  <div style={{ fontSize: 10, color: '#546a88' }}>target {s.target}%</div>
                </div>
              </div>
            ))}
          </div>

          {/* Entities */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, overflow: 'hidden' }}>
            <div style={{ padding: '12px 16px', borderBottom: '1px solid #1a2d4a', fontSize: 12, fontWeight: 700, color: '#c8d8ef' }}>
              Entities ({selected.entities.length})
            </div>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
              <thead>
                <tr style={{ borderBottom: '1px solid #1a2d4a' }}>
                  <th style={{ padding: '8px 14px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase' }}>Entity</th>
                  <th style={{ padding: '8px 14px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase' }}>Type</th>
                  <th style={{ padding: '8px 14px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase' }}>Status</th>
                  <th style={{ padding: '8px 14px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase' }}>P99</th>
                  <th style={{ padding: '8px 14px', textAlign: 'left', fontSize: 10, fontWeight: 600, color: '#546a88', textTransform: 'uppercase' }}>Error Rate</th>
                </tr>
              </thead>
              <tbody>
                {selected.entities.map((e, i) => (
                  <tr key={i} style={{ borderBottom: '1px solid rgba(26,45,74,.4)' }}>
                    <td style={{ padding: '10px 14px', fontWeight: 700, color: '#f0f6ff', fontFamily: 'JetBrains Mono, monospace', fontSize: 11 }}>{e.name}</td>
                    <td style={{ padding: '10px 14px' }}><span style={{ background: '#182844', color: '#8fa8cc', fontSize: 9, padding: '2px 8px', borderRadius: 8 }}>{e.type}</span></td>
                    <td style={{ padding: '10px 14px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
                        <div style={{ width: 6, height: 6, borderRadius: '50%', background: statusColor[e.status] || '#546a88' }} />
                        <span style={{ color: statusColor[e.status] || '#546a88', fontSize: 11, fontWeight: 600 }}>{e.status}</span>
                      </div>
                    </td>
                    <td style={{ padding: '10px 14px', fontFamily: 'JetBrains Mono, monospace', color: e.p99 ? (parseInt(e.p99) > 500 ? '#f5a623' : '#0fcf8a') : '#546a88' }}>{e.p99 || '—'}</td>
                    <td style={{ padding: '10px 14px', fontFamily: 'JetBrains Mono, monospace', color: e.err ? (parseFloat(e.err) > 1 ? '#ff4d6a' : '#0fcf8a') : '#546a88' }}>{e.err || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  )
}
