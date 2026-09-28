import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { Search, Plus, ExternalLink, AlertTriangle, CheckCircle, GitBranch, Users, Clock, Layers, Code, Shield } from 'lucide-react'
import clsx from 'clsx'

const TIER_CONFIG: Record<number, { label: string; color: string; border: string }> = {
  1: { label: 'Tier 1 — Critical', color: '#ef4444', border: 'border-t-[#ef4444]' },
  2: { label: 'Tier 2 — Important', color: '#f59e0b', border: 'border-t-[#f59e0b]' },
  3: { label: 'Tier 3 — Standard', color: '#10b981', border: 'border-t-[#10b981]' },
}
const LANG_COLORS: Record<string, string> = {
  Go: '#00acd7', 'Node.js': '#8cc84b', Python: '#3572a5',
  Java: '#b07219', TypeScript: '#2b7489', SQL: '#e38c00',
}

function HealthBar({ score }: { score: number }) {
  const color = score >= 95 ? '#10b981' : score >= 85 ? '#f59e0b' : '#ef4444'
  return (
    <div className="flex items-center gap-2">
      <div className="flex-1 h-1.5 bg-[#1e2433] rounded-full overflow-hidden">
        <div className="h-full rounded-full transition-all" style={{ width: `${score}%`, background: color }} />
      </div>
      <span className="text-[11px] font-bold w-12 text-right" style={{ color }}>{score.toFixed(1)}%</span>
    </div>
  )
}

function ServiceCard({ svc, onClick, selected }: { svc: any; onClick: () => void; selected: boolean }) {
  const tier = TIER_CONFIG[svc.tier] ?? TIER_CONFIG[3]
  const langColor = LANG_COLORS[svc.language] ?? '#9ca3af'

  return (
    <div onClick={onClick}
      className={clsx('bg-[#111827] border-t-2 border border-[#1e2433] rounded-xl p-4 cursor-pointer transition-all hover:border-[#2a3447]', tier.border, selected && '!border-[#6366f1] ring-1 ring-[#6366f1]/30')}>
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="text-[14px] font-bold text-slate-100">{svc.name}</div>
          <div className="text-[11px] text-slate-500 mt-0.5">
            <span>Owner: </span><span className="text-[#818cf8]">{svc.owner}</span>
          </div>
        </div>
        <span className="text-[9px] font-bold px-2 py-0.5 rounded-full" style={{ background: tier.color + '20', color: tier.color }}>
          T{svc.tier}
        </span>
      </div>

      {svc.description && <p className="text-[11px] text-slate-400 mb-3 line-clamp-2">{svc.description}</p>}

      {/* Tech stack */}
      <div className="flex items-center gap-2 mb-3 flex-wrap">
        {svc.language && (
          <span className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded font-medium" style={{ background: langColor + '20', color: langColor }}>
            <Code size={9} />{svc.language}
          </span>
        )}
        {svc.framework && (
          <span className="text-[10px] text-slate-500 bg-[#1e2433] px-1.5 py-0.5 rounded">{svc.framework}</span>
        )}
        {(svc.tags ?? []).slice(0, 2).map((t: string) => (
          <span key={t} className="text-[10px] text-slate-600 bg-[#0d1423] px-1.5 py-0.5 rounded">{t}</span>
        ))}
      </div>

      {/* Health */}
      <div className="mb-2">
        <div className="flex justify-between text-[10px] text-slate-500 mb-1">
          <span>Health score</span>
        </div>
        <HealthBar score={svc.health_score ?? 95} />
      </div>

      {/* Stats */}
      <div className="flex items-center gap-3 mt-3 pt-3 border-t border-[#0d1423]">
        <div className="flex items-center gap-1 text-[10px] text-slate-500">
          <Shield size={10} className="text-[#818cf8]" />
          <span>{svc.slo_count ?? 0} SLOs</span>
        </div>
        <div className="flex items-center gap-1 text-[10px] text-slate-500">
          <AlertTriangle size={10} className="text-[#f59e0b]" />
          <span>{svc.alert_count ?? 0} alerts</span>
        </div>
        {svc.deploy_freq != null && (
          <div className="flex items-center gap-1 text-[10px] text-slate-500 ml-auto">
            <GitBranch size={10} />
            <span>{svc.deploy_freq}/wk</span>
          </div>
        )}
      </div>
    </div>
  )
}

export default function ServiceCatalogPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<any>(null)
  const [search, setSearch] = useState('')
  const [tier, setTier] = useState<string>('all')
  const [owner, setOwner] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['catalog-services', tier, owner],
    queryFn: () => http.get('/api/v1/catalog/services', { params: { tier: tier !== 'all' ? tier : undefined, owner: owner || undefined } }).then(r => r.data),
    refetchInterval: 60_000,
  })

  const { data: healthData } = useQuery({
    queryKey: ['catalog-health', selected?.id],
    queryFn: () => http.get(`/api/v1/catalog/services/${selected.id}/health`).then(r => r.data),
    enabled: !!selected?.id,
    refetchInterval: 30_000,
  })

  const { data: slosData } = useQuery({
    queryKey: ['catalog-slos', selected?.id],
    queryFn: () => http.get(`/api/v1/catalog/services/${selected.id}/slos`).then(r => r.data),
    enabled: !!selected?.id,
  })

  const { data: techData } = useQuery({
    queryKey: ['catalog-technologies'],
    queryFn: () => http.get('/api/v1/catalog/technologies').then(r => r.data),
  })

  const services: any[] = (data?.services ?? []).filter((s: any) =>
    !search || s.name?.toLowerCase().includes(search.toLowerCase()) ||
    s.owner?.toLowerCase().includes(search.toLowerCase())
  )

  const tiers = [
    { v: 'all', label: 'All' },
    { v: '1', label: 'Tier 1 — Critical' },
    { v: '2', label: 'Tier 2 — Important' },
    { v: '3', label: 'Tier 3 — Standard' },
  ]

  return (
    <div className="flex flex-col h-full bg-[#070c18] text-slate-200">
      {/* Top bar */}
      <div className="flex items-center gap-3 px-5 py-3 border-b border-[#1e2433] bg-[#080d1b] shrink-0">
        <div className="flex items-center gap-2 bg-[#111827] border border-[#1e2433] rounded-lg px-3 py-1.5 flex-1 max-w-xs">
          <Search size={13} className="text-slate-500" />
          <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search services, owners..." className="bg-transparent text-[12px] text-slate-300 outline-none flex-1 placeholder-slate-600" />
        </div>
        <div className="flex rounded-lg border border-[#1e2433] overflow-hidden">
          {tiers.map(t => (
            <button key={t.v} onClick={() => setTier(t.v)}
              className={clsx('px-3 py-1.5 text-[11px] font-medium transition-colors', tier === t.v ? 'bg-[#6366f1] text-white' : 'bg-[#111827] text-slate-400 hover:text-slate-200')}>
              {t.label}
            </button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-2">
          <span className="text-[11px] text-slate-500">{services.length} services</span>
          <button className="flex items-center gap-1.5 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[11px] font-semibold px-3 py-1.5 rounded-lg transition-colors">
            <Plus size={12} /> Register Service
          </button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        {/* Grid */}
        <div className="flex-1 overflow-y-auto p-5">
          {isLoading ? (
            <div className="flex justify-center py-16"><Spinner /></div>
          ) : (
            <>
              {/* Tech landscape */}
              {techData && (
                <div className="mb-5 bg-[#111827] border border-[#1e2433] rounded-xl p-4">
                  <div className="text-[11px] font-semibold text-slate-400 uppercase tracking-wider mb-3">Technology Landscape</div>
                  <div className="grid grid-cols-4 gap-3">
                    {[
                      { label: 'Languages', items: techData.languages },
                      { label: 'Frameworks', items: techData.frameworks },
                      { label: 'Databases', items: techData.databases },
                      { label: 'Infrastructure', items: techData.infra },
                    ].map(g => (
                      <div key={g.label}>
                        <div className="text-[10px] text-slate-600 mb-2">{g.label}</div>
                        <div className="flex flex-wrap gap-1">
                          {(g.items ?? []).map((item: string) => (
                            <span key={item} className="text-[10px] bg-[#0d1423] text-slate-400 px-1.5 py-0.5 rounded border border-[#1e2433]">{item}</span>
                          ))}
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              <div className="grid grid-cols-3 gap-4">
                {services.map((svc: any) => (
                  <ServiceCard key={svc.id} svc={svc}
                    onClick={() => setSelected(selected?.id === svc.id ? null : svc)}
                    selected={selected?.id === svc.id} />
                ))}
              </div>
            </>
          )}
        </div>

        {/* Detail panel */}
        {selected && (
          <div className="w-80 border-l border-[#1e2433] bg-[#080d1b] flex flex-col overflow-hidden shrink-0">
            <div className="p-4 border-b border-[#1e2433]">
              <div className="flex items-start justify-between">
                <div>
                  <div className="text-[14px] font-bold text-slate-100">{selected.name}</div>
                  <div className="text-[11px] text-slate-500">{selected.language} · {selected.framework}</div>
                </div>
                <button onClick={() => setSelected(null)} className="text-slate-500 hover:text-slate-300 text-lg leading-none">×</button>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {/* Live metrics */}
              {healthData && (
                <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-3">
                  <div className="text-[11px] font-semibold text-slate-400 mb-2">Live Health</div>
                  <div className="grid grid-cols-2 gap-2">
                    {[
                      { label: 'Error Rate', v: healthData.error_rate != null ? `${(+healthData.error_rate).toFixed(2)}%` : '—', bad: (healthData.error_rate ?? 0) > 1 },
                      { label: 'P99 Latency', v: healthData.p99_ms != null ? `${(+healthData.p99_ms).toFixed(0)}ms` : '—', bad: (healthData.p99_ms ?? 0) > 500 },
                    ].map(m => (
                      <div key={m.label} className="bg-[#0d1423] rounded-lg p-2">
                        <div className="text-[10px] text-slate-500">{m.label}</div>
                        <div className="text-[13px] font-bold mt-0.5" style={{ color: m.bad ? '#ef4444' : '#10b981' }}>{m.v}</div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Links */}
              <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-3">
                <div className="text-[11px] font-semibold text-slate-400 mb-2">Links</div>
                {[
                  { label: 'Repository', url: selected.repo },
                  { label: 'Documentation', url: selected.doc_url },
                  { label: 'Runbook', url: selected.runbook_url },
                ].filter(l => l.url).map(link => (
                  <a key={link.label} href={link.url} target="_blank" rel="noreferrer"
                    className="flex items-center gap-2 py-1.5 text-[11px] text-[#818cf8] hover:text-[#6366f1]">
                    <ExternalLink size={11} />{link.label}
                  </a>
                ))}
              </div>

              {/* Dependencies */}
              {selected.depends_on?.length > 0 && (
                <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-3">
                  <div className="text-[11px] font-semibold text-slate-400 mb-2">Depends On</div>
                  {selected.depends_on.map((dep: string) => (
                    <div key={dep} className="flex items-center gap-2 py-1 text-[11px] text-slate-300">
                      <GitBranch size={10} className="text-slate-600" />{dep}
                    </div>
                  ))}
                </div>
              )}

              {/* SLOs */}
              {slosData?.slos?.length > 0 && (
                <div className="bg-[#111827] rounded-xl border border-[#1e2433] p-3">
                  <div className="text-[11px] font-semibold text-slate-400 mb-2">SLOs ({slosData.total})</div>
                  {slosData.slos.slice(0, 4).map((slo: any) => (
                    <div key={slo.id} className="py-1.5 border-b border-[#0d1423] last:border-0">
                      <div className="flex justify-between items-center">
                        <span className="text-[11px] text-slate-300">{slo.name}</span>
                        <span className="text-[11px] font-bold" style={{ color: (slo.sli ?? 99.9) >= slo.target_pct ? '#10b981' : '#ef4444' }}>
                          {(slo.sli ?? slo.target_pct)?.toFixed(2)}%
                        </span>
                      </div>
                      <div className="mt-0.5 h-1 bg-[#1e2433] rounded">
                        <div className="h-full rounded" style={{ width: `${Math.min((slo.sli ?? slo.target_pct), 100)}%`, background: (slo.sli ?? 99) >= slo.target_pct ? '#10b981' : '#ef4444' }} />
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
