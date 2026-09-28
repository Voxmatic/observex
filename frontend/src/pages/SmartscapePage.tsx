// SmartscapePage.tsx — 5-layer Dynatrace Smartscape equivalent topology
// Applications → Services → Processes → Hosts → Network/Datacenter
// Both vertical (cross-tier) and horizontal (same-tier) dependency views

import { useState, useRef, useEffect } from 'react'
import { Filter, RefreshCw, ZoomIn, ZoomOut } from 'lucide-react'
import clsx from 'clsx'

// ── Data model ────────────────────────────────────────────────────────────────
const TIERS = ['Applications', 'Services', 'Processes', 'Hosts', 'Network'] as const
type Tier = typeof TIERS[number]

const ENTITIES = {
  Applications: [
    { id: 'app-web', label: 'Web Frontend', icon: '🌐', status: 'healthy', users: '42.8K', slo: 99.2, calls: 4820 },
    { id: 'app-mobile', label: 'Mobile App', icon: '📱', status: 'healthy', users: '18.4K', slo: 99.7, calls: 2140 },
    { id: 'app-api', label: 'Partner API', icon: '🔌', status: 'warning', users: '0', slo: 97.1, calls: 840 },
  ],
  Services: [
    { id: 'svc-gw', label: 'API Gateway', icon: '⬡', status: 'warning', p99: 284, err: 1.84, calls: 4820 },
    { id: 'svc-checkout', label: 'Checkout', icon: '⬡', status: 'warning', p99: 892, err: 3.20, calls: 1240 },
    { id: 'svc-payment', label: 'Payment', icon: '⬡', status: 'healthy', p99: 98, err: 0.08, calls: 620 },
    { id: 'svc-user', label: 'User Service', icon: '⬡', status: 'critical', p99: 4200, err: 8.40, calls: 120 },
    { id: 'svc-ml', label: 'ML Inference', icon: '⬡', status: 'warning', p99: 1240, err: 0.72, calls: 840 },
    { id: 'svc-notify', label: 'Notification', icon: '⬡', status: 'healthy', p99: 45, err: 0.04, calls: 280 },
  ],
  Processes: [
    { id: 'proc-gw', label: 'api-gateway:8080', icon: '◉', status: 'warning', cpu: 28, mem: 512, pid: 12401 },
    { id: 'proc-checkout', label: 'checkout:8081', icon: '◉', status: 'warning', cpu: 62, mem: 768, pid: 12402 },
    { id: 'proc-payment', label: 'payment:8082', icon: '◉', status: 'healthy', cpu: 14, mem: 384, pid: 12403 },
    { id: 'proc-user', label: 'user-svc:8083', icon: '◉', status: 'critical', cpu: 94, mem: 980, pid: 12404 },
    { id: 'proc-ml', label: 'ml-worker:8090', icon: '◉', status: 'warning', cpu: 82, mem: 4096, pid: 12405 },
    { id: 'proc-pg', label: 'postgres:5432', icon: '◉', status: 'healthy', cpu: 18, mem: 2048, pid: 12406 },
    { id: 'proc-redis', label: 'redis:6379', icon: '◉', status: 'healthy', cpu: 4, mem: 256, pid: 12407 },
  ],
  Hosts: [
    { id: 'host-web-01', label: 'prod-web-01', icon: '🖥', status: 'healthy', cpu: 42, mem: 68, disk: 34, os: 'Ubuntu 22.04' },
    { id: 'host-web-02', label: 'prod-web-02', icon: '🖥', status: 'warning', cpu: 78, mem: 82, disk: 41, os: 'Ubuntu 22.04' },
    { id: 'host-db-01', label: 'prod-db-01', icon: '🖥', status: 'healthy', cpu: 22, mem: 56, disk: 74, os: 'Ubuntu 20.04' },
    { id: 'host-ml-01', label: 'prod-ml-01', icon: '🖥', status: 'critical', cpu: 96, mem: 88, disk: 52, os: 'Amazon Linux 2023' },
  ],
  Network: [
    { id: 'net-prod', label: 'prod-vpc-10.0.0.0/8', icon: '🌐', status: 'healthy', bw: '2.4 Gbps', lat: '0.8ms' },
    { id: 'net-k8s', label: 'k8s-cluster-eks', icon: '⎈', status: 'healthy', bw: '4.8 Gbps', lat: '0.4ms' },
    { id: 'net-db', label: 'db-subnet-10.0.4.0/24', icon: '🌐', status: 'healthy', bw: '1.2 Gbps', lat: '0.2ms' },
  ],
}

const CONNECTIONS = [
  // app → service
  { from: 'app-web', to: 'svc-gw', rps: 4820, status: 'warning' },
  { from: 'app-mobile', to: 'svc-gw', rps: 2140, status: 'healthy' },
  { from: 'app-api', to: 'svc-gw', rps: 840, status: 'warning' },
  // service → service
  { from: 'svc-gw', to: 'svc-checkout', rps: 1240, status: 'warning' },
  { from: 'svc-gw', to: 'svc-payment', rps: 620, status: 'healthy' },
  { from: 'svc-gw', to: 'svc-user', rps: 120, status: 'critical' },
  { from: 'svc-gw', to: 'svc-ml', rps: 840, status: 'warning' },
  { from: 'svc-checkout', to: 'svc-payment', rps: 620, status: 'healthy' },
  { from: 'svc-ml', to: 'svc-notify', rps: 280, status: 'healthy' },
  // service → process
  { from: 'svc-gw', to: 'proc-gw', rps: null, status: 'warning' },
  { from: 'svc-checkout', to: 'proc-checkout', rps: null, status: 'warning' },
  { from: 'svc-user', to: 'proc-user', rps: null, status: 'critical' },
  { from: 'svc-ml', to: 'proc-ml', rps: null, status: 'warning' },
  // process → process
  { from: 'proc-checkout', to: 'proc-pg', rps: null, status: 'healthy' },
  { from: 'proc-user', to: 'proc-pg', rps: null, status: 'healthy' },
  { from: 'proc-gw', to: 'proc-redis', rps: null, status: 'healthy' },
  // process → host
  { from: 'proc-gw', to: 'host-web-01', rps: null, status: 'healthy' },
  { from: 'proc-checkout', to: 'host-web-02', rps: null, status: 'warning' },
  { from: 'proc-user', to: 'host-ml-01', rps: null, status: 'critical' },
  { from: 'proc-pg', to: 'host-db-01', rps: null, status: 'healthy' },
  // host → network
  { from: 'host-web-01', to: 'net-prod', rps: null, status: 'healthy' },
  { from: 'host-web-02', to: 'net-prod', rps: null, status: 'healthy' },
  { from: 'host-db-01', to: 'net-db', rps: null, status: 'healthy' },
  { from: 'host-ml-01', to: 'net-k8s', rps: null, status: 'critical' },
]

const STATUS_COL: Record<string, string> = {
  healthy: '#0fcf8a', warning: '#f5a623', critical: '#ff4d6a',
}

const DOMAIN_FILTERS = [
  { id: 'all', label: 'All' },
  { id: 'cloud', label: 'Cloud (AWS)' },
  { id: 'k8s', label: 'Kubernetes' },
  { id: 'services', label: 'Services' },
  { id: 'infra', label: 'Infrastructure' },
]

// ── Component ────────────────────────────────────────────────────────────────
export default function SmartscapePage() {
  const [activeTier, setActiveTier] = useState<Tier | 'all'>('all')
  const [domain, setDomain] = useState('all')
  const [selected, setSelected] = useState<string | null>(null)
  const [view, setView] = useState<'vertical' | 'horizontal'>('vertical')
  const canvasRef = useRef<HTMLDivElement>(null)

  const selectedEntity = Object.values(ENTITIES).flat().find(e => e.id === selected)
  const selectedConns = CONNECTIONS.filter(c => c.from === selected || c.to === selected)

  const TIER_Y: Record<Tier, number> = {
    Applications: 40, Services: 130, Processes: 220, Hosts: 310, Network: 400,
  }

  // Layout entities per tier
  const entityPositions: Record<string, { x: number; y: number }> = {}
  TIERS.forEach(tier => {
    const items = ENTITIES[tier]
    items.forEach((e, i) => {
      entityPositions[e.id] = {
        x: 80 + i * (680 / Math.max(items.length, 1)),
        y: TIER_Y[tier],
      }
    })
  })

  // Compute visible connections based on activeTier
  const visibleConns = CONNECTIONS.filter(c => {
    if (activeTier === 'all') return true
    const fromTier = TIERS.find(t => ENTITIES[t].some(e => e.id === c.from))
    const toTier = TIERS.find(t => ENTITIES[t].some(e => e.id === c.to))
    return fromTier === activeTier || toTier === activeTier
  })

  return (
    <div className="flex flex-col bg-[#070f1e] min-h-full text-slate-200">
      {/* Header */}
      <div className="px-5 py-4 border-b border-[#1a2d4a]">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-[20px] font-bold text-slate-100" style={{ fontFamily: 'Syne, sans-serif' }}>
              Smartscape Topology
            </h1>
            <p className="text-[11px] text-slate-500 mt-0.5">
              5-layer real-time dependency graph · Auto-discovered · Zero configuration
            </p>
          </div>
          <div className="flex items-center gap-2">
            {/* Domain filter */}
            <div className="flex gap-1 bg-[#0b1628] border border-[#1a2d4a] rounded-xl p-1">
              {DOMAIN_FILTERS.map(d => (
                <button key={d.id} onClick={() => setDomain(d.id)}
                  className={clsx('text-[10px] px-3 py-1.5 rounded-lg font-semibold transition-all', domain === d.id ? 'bg-[#6c72ff] text-white' : 'text-slate-500 hover:text-slate-300')}>
                  {d.label}
                </button>
              ))}
            </div>
            {/* View toggle */}
            <div className="flex gap-1 bg-[#0b1628] border border-[#1a2d4a] rounded-xl p-1">
              {(['vertical', 'horizontal'] as const).map(v => (
                <button key={v} onClick={() => setView(v)}
                  className={clsx('text-[10px] px-3 py-1.5 rounded-lg font-semibold capitalize transition-all', view === v ? 'bg-[#6c72ff] text-white' : 'text-slate-500 hover:text-slate-300')}>
                  {v}
                </button>
              ))}
            </div>
            <button className="p-2 text-slate-500 hover:text-slate-300 border border-[#1a2d4a] rounded-lg bg-[#0b1628]">
              <RefreshCw size={13} />
            </button>
          </div>
        </div>

        {/* Tier tabs */}
        <div className="flex items-center gap-1 mt-3">
          <button onClick={() => setActiveTier('all')}
            className={clsx('text-[11px] px-3 py-1.5 rounded-lg font-semibold transition-all border', activeTier === 'all' ? 'bg-[#6c72ff]/20 border-[#6c72ff]/50 text-[#8b90ff]' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            All Tiers
          </button>
          {TIERS.map(t => {
            const items = ENTITIES[t]
            const critical = items.filter(e => e.status === 'critical').length
            const warning = items.filter(e => e.status === 'warning').length
            return (
              <button key={t} onClick={() => setActiveTier(t)}
                className={clsx('text-[11px] px-3 py-1.5 rounded-lg font-semibold transition-all border flex items-center gap-2', activeTier === t ? 'bg-[#6c72ff]/20 border-[#6c72ff]/50 text-[#8b90ff]' : 'border-transparent text-slate-500 hover:text-slate-300')}>
                {t}
                <span className="text-[9px] text-slate-500">{items.length}</span>
                {critical > 0 && <span className="w-1.5 h-1.5 rounded-full bg-[#ff4d6a]" />}
                {warning > 0 && !critical && <span className="w-1.5 h-1.5 rounded-full bg-[#f5a623]" />}
              </button>
            )
          })}
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Main canvas */}
        <div className="flex-1 relative overflow-auto p-4">
          {/* Legend */}
          <div className="flex items-center gap-6 mb-4 text-[10px] text-slate-500">
            {Object.entries(STATUS_COL).map(([s, c]) => (
              <span key={s} className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full" style={{ background: c }} />
                {s.charAt(0).toUpperCase() + s.slice(1)}
              </span>
            ))}
            <span className="ml-auto">Solid line = sync · Dashed = async/DB</span>
            <span className="text-slate-600">Auto-refreshes every 60s</span>
          </div>

          {/* Canvas */}
          <div ref={canvasRef} style={{ position: 'relative', width: 800, height: 480, background: 'radial-gradient(ellipse at 50% 50%, rgba(108,114,255,.04) 0%, transparent 70%)' }}>
            {/* Tier labels on left */}
            {TIERS.map(tier => (
              <div key={tier} style={{ position: 'absolute', left: 0, top: TIER_Y[tier] + 10, width: 70, fontSize: 9, fontWeight: 700, color: '#546a88', textAlign: 'right', paddingRight: 8, textTransform: 'uppercase', letterSpacing: '.06em' }}>
                {tier}
              </div>
            ))}

            {/* Tier dividers */}
            {TIERS.map(tier => (
              <div key={tier} style={{ position: 'absolute', left: 75, right: 0, top: TIER_Y[tier] - 2, height: 1, background: 'rgba(26,45,74,.4)', borderTop: '1px dashed rgba(26,45,74,.6)' }} />
            ))}

            {/* SVG for connections */}
            <svg style={{ position: 'absolute', top: 0, left: 0, width: '100%', height: '100%', pointerEvents: 'none' }}>
              {visibleConns.map((c, i) => {
                const fp = entityPositions[c.from]
                const tp = entityPositions[c.to]
                if (!fp || !tp) return null
                const col = STATUS_COL[c.status] ?? '#546a88'
                const isHighlighted = selected && (c.from === selected || c.to === selected)
                const isDB = c.from?.includes('proc') && c.to?.includes('proc')
                return (
                  <g key={i}>
                    <line
                      x1={fp.x + 44} y1={fp.y + 28}
                      x2={tp.x + 44} y2={tp.y + 8}
                      stroke={col}
                      strokeWidth={isHighlighted ? 2.5 : 1}
                      strokeOpacity={selected && !isHighlighted ? 0.15 : 0.5}
                      strokeDasharray={isDB ? '4 2' : undefined}
                    />
                    {c.rps && (
                      <text
                        x={(fp.x + tp.x) / 2 + 44}
                        y={(fp.y + tp.y) / 2 + 18}
                        fill={col} fontSize={8} textAnchor="middle" opacity={0.7}>
                        {c.rps}/s
                      </text>
                    )}
                  </g>
                )
              })}
            </svg>

            {/* Entity nodes */}
            {TIERS.map(tier =>
              ENTITIES[tier].map(entity => {
                const pos = entityPositions[entity.id]
                const col = STATUS_COL[entity.status] ?? '#546a88'
                const isSelected = selected === entity.id
                const isConnected = selected && CONNECTIONS.some(c => (c.from === selected && c.to === entity.id) || (c.to === selected && c.from === entity.id))
                const isDimmed = selected && !isSelected && !isConnected
                return (
                  <div key={entity.id}
                    onClick={() => setSelected(isSelected ? null : entity.id)}
                    style={{
                      position: 'absolute', left: pos.x, top: pos.y,
                      width: 88, cursor: 'pointer',
                      opacity: isDimmed ? 0.25 : 1,
                      transition: 'opacity .2s, transform .2s',
                      transform: isSelected ? 'scale(1.08)' : 'scale(1)',
                      zIndex: isSelected ? 10 : 1,
                    }}>
                    <div style={{
                      background: isSelected ? `${col}20` : 'rgba(11,22,40,.9)',
                      border: `1px solid ${isSelected ? col : isConnected ? col + '60' : '#1a2d4a'}`,
                      borderRadius: 10, padding: '8px 6px', textAlign: 'center',
                      boxShadow: isSelected ? `0 0 12px ${col}40` : undefined,
                    }}>
                      <div style={{ fontSize: 13, marginBottom: 3 }}>{entity.icon}</div>
                      <div style={{ fontSize: 10, fontWeight: 700, color: isSelected ? col : '#c8d8ef', wordBreak: 'break-all', lineHeight: 1.2 }}>{entity.label}</div>
                      <div style={{ fontSize: 9, color: col, marginTop: 3, fontWeight: 600 }}>{entity.status}</div>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </div>

        {/* Right panel: entity detail */}
        <div className="w-72 border-l border-[#1a2d4a] overflow-y-auto flex-shrink-0 p-4">
          {selectedEntity ? (
            <div>
              <div className="flex items-center gap-3 mb-4">
                <span className="text-2xl">{selectedEntity.icon}</span>
                <div>
                  <div className="text-[14px] font-bold text-slate-100">{selectedEntity.label}</div>
                  <div className="text-[10px] font-semibold" style={{ color: STATUS_COL[selectedEntity.status] }}>● {selectedEntity.status}</div>
                </div>
              </div>

              {/* Properties */}
              <div className="space-y-2 mb-4">
                {Object.entries(selectedEntity).filter(([k]) => !['id', 'label', 'icon', 'status'].includes(k)).map(([k, v]) => (
                  <div key={k} className="flex items-center justify-between text-[11px]">
                    <span className="text-slate-500 capitalize">{k.replace(/_/g, ' ')}</span>
                    <span className="text-slate-200 font-mono">{String(v)}{k === 'cpu' || k === 'mem' || k === 'disk' ? '%' : ''}</span>
                  </div>
                ))}
              </div>

              {/* Connections */}
              <div className="border-t border-[#1a2d4a] pt-3 mt-3">
                <div className="text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-2">Dependencies ({selectedConns.length})</div>
                {selectedConns.map((c, i) => {
                  const isOut = c.from === selected
                  const otherId = isOut ? c.to : c.from
                  const other = Object.values(ENTITIES).flat().find(e => e.id === otherId)
                  return (
                    <div key={i} className="flex items-center gap-2 py-1.5 text-[11px]">
                      <span className="text-slate-500">{isOut ? '→' : '←'}</span>
                      <span className="text-slate-300 flex-1 truncate">{other?.label ?? otherId}</span>
                      <span className="text-[9px] font-semibold" style={{ color: STATUS_COL[c.status] }}>●</span>
                      {c.rps && <span className="text-slate-500 font-mono text-[10px]">{c.rps}/s</span>}
                    </div>
                  )
                })}
              </div>

              <button onClick={() => setSelected(null)} className="mt-4 w-full text-[11px] text-slate-500 hover:text-slate-300 border border-[#1a2d4a] rounded-lg py-2">
                Clear selection
              </button>
            </div>
          ) : (
            <div>
              <div className="text-[11px] font-bold text-slate-500 uppercase tracking-wider mb-4">Environment Summary</div>
              {TIERS.map(tier => {
                const items = ENTITIES[tier]
                const healthy = items.filter(e => e.status === 'healthy').length
                const warn = items.filter(e => e.status === 'warning').length
                const crit = items.filter(e => e.status === 'critical').length
                return (
                  <div key={tier} className="mb-4">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-[11px] font-semibold text-slate-300">{tier}</span>
                      <span className="text-[10px] text-slate-500">{items.length} entities</span>
                    </div>
                    <div className="flex gap-2 text-[10px]">
                      {healthy > 0 && <span className="text-[#0fcf8a]">{healthy} healthy</span>}
                      {warn > 0 && <span className="text-[#f5a623]">{warn} warning</span>}
                      {crit > 0 && <span className="text-[#ff4d6a]">{crit} critical</span>}
                    </div>
                  </div>
                )
              })}
              <div className="border-t border-[#1a2d4a] pt-3 mt-3 text-[11px] text-slate-500">
                <p>Click any entity to explore its dependencies across all 5 tiers.</p>
                <p className="mt-2">Highlighted connections show upstream ← and downstream → dependencies.</p>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
