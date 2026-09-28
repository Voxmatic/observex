import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { metrics, logs } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, SeverityBadge, Btn } from '@/components/shared/Layout'
import {
  Shield, AlertTriangle, Activity, Package, ChevronDown,
  ChevronUp, ExternalLink, Search, Filter
} from 'lucide-react'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell, PieChart, Pie, Legend } from 'recharts'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'

// ── Types ─────────────────────────────────────────────────────────────────

interface ImageRisk {
  image: string
  critical: number
  high: number
  medium: number
  low: number
  riskScore: number
  scannedAgo: string
}

interface ThreatEvent {
  ts: string
  severity: string
  eventType: string
  process: string
  pod: string
  namespace: string
  node: string
  description: string
  mitreTactic: string
  mitreTechID: string
}

interface CVEEntry {
  vulnID: string
  pkg: string
  installed: string
  fixed: string
  severity: string
  cvss: number
  title: string
  image: string
}

// ── Colour helpers ─────────────────────────────────────────────────────────

const SEV_COLORS: Record<string, string> = {
  CRITICAL: '#ef4444',
  HIGH:     '#f97316',
  MEDIUM:   '#f59e0b',
  LOW:      '#6b7280',
  INFO:     '#3b82f6',
}

const MITRE_COLORS = [
  '#3b82f6', '#8b5cf6', '#ec4899', '#f97316',
  '#10b981', '#f59e0b', '#ef4444', '#14b8a6',
]

// ── Data hooks ─────────────────────────────────────────────────────────────

function useImageRisks() {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - 86400 * 7        // 7-day window

  const { data: riskData } = useQuery({
    queryKey: ['sec-image-risk'],
    queryFn: () => metrics.queryRange(
      'max by (image) (image_scan_risk_score)',
      start, now, '3600'
    ),
    refetchInterval: 300_000,
  })

  const { data: critData } = useQuery({
    queryKey: ['sec-image-crit'],
    queryFn: () => metrics.queryRange(
      'max by (image) (image_scan_vulns_critical)',
      start, now, '3600'
    ),
    refetchInterval: 300_000,
  })

  const { data: highData } = useQuery({
    queryKey: ['sec-image-high'],
    queryFn: () => metrics.queryRange(
      'max by (image) (image_scan_vulns_high)',
      start, now, '3600'
    ),
    refetchInterval: 300_000,
  })

  return useMemo<ImageRisk[]>(() => {
    const risks: Record<string, ImageRisk> = {}

    const extract = (series: any[], field: keyof ImageRisk) => {
      for (const s of (series ?? [])) {
        const img = s.metric?.image
        if (!img) continue
        if (!risks[img]) risks[img] = { image: img, critical: 0, high: 0, medium: 0, low: 0, riskScore: 0, scannedAgo: '' }
        const last = s.values?.[s.values.length - 1]
        if (last) (risks[img] as any)[field] = Math.round(parseFloat(last.v))
      }
    }

    extract(riskData?.result ?? [], 'riskScore')
    extract(critData?.result ?? [], 'critical')
    extract(highData?.result ?? [], 'high')

    return Object.values(risks).sort((a, b) => b.riskScore - a.riskScore)
  }, [riskData, critData, highData])
}

function useSecurityEvents(range: number) {
  const now   = Math.floor(Date.now() * 1000)  // microseconds for Loki
  const start = now - range * 1000 * 1000

  return useQuery({
    queryKey: ['sec-events', range],
    queryFn: () => logs.queryRange(
      '{source="ebpf-security"} | level =~ "error|warn"',
      start, now, 100
    ),
    refetchInterval: 15_000,
  })
}

function useCVEList() {
  const now   = Math.floor(Date.now() * 1000)
  const start = now - 86400 * 7 * 1000 * 1000

  return useQuery({
    queryKey: ['sec-cves'],
    queryFn: () => logs.queryRange(
      '{source="trivy"} | severity =~ "CRITICAL|HIGH"',
      start, now, 200
    ),
    refetchInterval: 300_000,
  })
}

function useMitreSummary(range: number) {
  const now   = Math.floor(Date.now() / 1000)
  const start = now - range

  return useQuery({
    queryKey: ['sec-mitre', range],
    queryFn: () => metrics.queryRange(
      `sum by (event_type) (increase(security_events_total[${range}s]))`,
      start, now, String(range)
    ),
    refetchInterval: 60_000,
  })
}

// ── Components ─────────────────────────────────────────────────────────────

function RiskBadge({ score }: { score: number }) {
  const color = score >= 50 ? '#ef4444' : score >= 25 ? '#f97316' : score >= 10 ? '#f59e0b' : '#10b981'
  const label = score >= 50 ? 'Critical' : score >= 25 ? 'High' : score >= 10 ? 'Medium' : 'Low'
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium px-2 py-0.5 rounded"
      style={{ background: `${color}22`, color }}>
      {label} {score.toFixed(0)}
    </span>
  )
}

function ImageRiskTable({ images }: { images: ImageRisk[] }) {
  const [expanded, setExpanded] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const filtered = images.filter(i =>
    !search || i.image.toLowerCase().includes(search.toLowerCase())
  )

  if (!filtered.length) return (
    <EmptyState icon={Package}
      title="No scan results yet"
      description="The Trivy scanner is discovering images. Results appear here after the first scan completes (usually within 5 minutes of deployment)." />
  )

  return (
    <div>
      <div className="px-4 py-2 border-b border-surface-3">
        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input value={search} onChange={e => setSearch(e.target.value)}
            placeholder="Filter images…"
            className="pl-8 pr-3 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50 w-56" />
        </div>
      </div>

      {/* Header */}
      <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                      bg-surface-2 border-b border-surface-3 px-4 py-2"
        style={{ gridTemplateColumns: '1fr 70px 70px 70px 70px 100px' }}>
        <span>Image</span>
        <span className="text-center text-crit">Critical</span>
        <span className="text-center text-warn">High</span>
        <span className="text-center text-yellow-500">Medium</span>
        <span className="text-center text-slate-500">Low</span>
        <span className="text-right">Risk score</span>
      </div>

      <div className="divide-y divide-surface-3">
        {filtered.map(img => (
          <div key={img.image}>
            <div
              className="grid items-center px-4 py-2.5 hover:bg-surface-2/40 cursor-pointer transition-colors text-sm"
              style={{ gridTemplateColumns: '1fr 70px 70px 70px 70px 100px' }}
              onClick={() => setExpanded(expanded === img.image ? null : img.image)}
            >
              <div className="flex items-center gap-2 min-w-0">
                {expanded === img.image
                  ? <ChevronUp size={12} className="text-slate-500 shrink-0" />
                  : <ChevronDown size={12} className="text-slate-500 shrink-0" />}
                <span className="font-mono text-xs text-slate-300 truncate">{img.image}</span>
              </div>
              <span className={clsx('text-center text-sm font-semibold', img.critical > 0 ? 'text-crit' : 'text-slate-600')}>{img.critical}</span>
              <span className={clsx('text-center text-sm font-semibold', img.high > 0 ? 'text-warn' : 'text-slate-600')}>{img.high}</span>
              <span className="text-center text-sm text-yellow-500">{img.medium}</span>
              <span className="text-center text-sm text-slate-500">{img.low}</span>
              <div className="text-right"><RiskBadge score={img.riskScore} /></div>
            </div>

            {expanded === img.image && (
              <div className="px-4 py-3 bg-surface-2/30 border-t border-surface-3 animate-fade-in">
                <p className="text-xs text-slate-500 mb-2">
                  Top CVEs for this image are visible in the CVE Feed below (filtered by image name).
                </p>
                <div className="flex items-center gap-3 text-xs">
                  <span className="text-slate-600">Query in Loki:</span>
                  <code className="text-brand font-mono bg-surface-3 px-2 py-0.5 rounded text-[10px]">
                    {`{source="trivy", image="${img.image}"}`}
                  </code>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function parseThreatEvents(streams: any[]): ThreatEvent[] {
  const events: ThreatEvent[] = []
  for (const s of (streams ?? [])) {
    const labels = s.stream ?? {}
    for (const [tsNs, line] of (s.values ?? [])) {
      events.push({
        ts:          new Date(parseInt(tsNs) / 1_000_000).toISOString(),
        severity:    labels.severity ?? 'LOW',
        eventType:   labels.event_type ?? 'unknown',
        process:     '',
        pod:         labels.pod ?? '',
        namespace:   labels.namespace ?? '',
        node:        labels.node ?? '',
        description: String(line),
        mitreTactic: '',
        mitreTechID: '',
      })
    }
  }
  return events.sort((a, b) => b.ts.localeCompare(a.ts)).slice(0, 50)
}

function ThreatFeed({ streams }: { streams: any[] }) {
  const events = parseThreatEvents(streams)

  if (!events.length) return (
    <div className="py-8 text-center text-xs text-slate-600">
      No runtime threats detected in this period.
    </div>
  )

  return (
    <div className="divide-y divide-surface-3 text-xs">
      {events.map((e, i) => (
        <div key={i} className="flex items-start gap-3 px-4 py-2.5 hover:bg-surface-2/30">
          <span className="shrink-0 w-1.5 h-1.5 rounded-full mt-1.5"
            style={{ background: SEV_COLORS[e.severity] ?? '#888' }} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-0.5">
              <span className="font-medium" style={{ color: SEV_COLORS[e.severity] ?? '#888' }}>
                {e.severity}
              </span>
              <span className="text-slate-600 capitalize">{e.eventType}</span>
              {e.namespace && <span className="text-slate-700 font-mono">{e.namespace}/{e.pod}</span>}
            </div>
            <p className="text-slate-400 break-all leading-relaxed">{e.description}</p>
          </div>
          <span className="text-slate-700 shrink-0 text-[10px]">
            {formatDistanceToNow(new Date(e.ts), { addSuffix: true })}
          </span>
        </div>
      ))}
    </div>
  )
}

function parseCVEs(streams: any[]): CVEEntry[] {
  const cves: CVEEntry[] = []
  for (const s of (streams ?? [])) {
    const labels = s.stream ?? {}
    for (const [, line] of (s.values ?? [])) {
      const parts = String(line).split(' ')
      cves.push({
        vulnID:    labels.cve ?? parts[1] ?? '',
        pkg:       labels.pkg ?? parts[3] ?? '',
        installed: '',
        fixed:     '',
        severity:  labels.severity ?? 'UNKNOWN',
        cvss:      0,
        title:     String(line),
        image:     labels.image ?? '',
      })
    }
  }
  // Deduplicate by CVE ID + image
  const seen = new Set<string>()
  return cves.filter(c => {
    const key = `${c.vulnID}::${c.image}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).slice(0, 100)
}

function CVEFeed({ streams, filterSev }: { streams: any[], filterSev: string }) {
  const all  = parseCVEs(streams)
  const cves = filterSev ? all.filter(c => c.severity === filterSev) : all

  if (!cves.length) return (
    <div className="py-8 text-center text-xs text-slate-600">
      No CVEs found. Images are clean or scans haven't run yet.
    </div>
  )

  return (
    <div className="divide-y divide-surface-3 text-xs max-h-96 overflow-y-auto">
      {cves.map((c, i) => (
        <div key={i} className="flex items-start gap-3 px-4 py-2.5 hover:bg-surface-2/30">
          <span className="font-mono font-medium shrink-0 w-32 truncate"
            style={{ color: SEV_COLORS[c.severity] ?? '#888' }}>
            {c.vulnID}
          </span>
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-2 mb-0.5">
              <span className="text-slate-300 truncate max-w-[160px] font-mono">{c.image}</span>
              <span className="text-slate-600">{c.pkg}</span>
            </div>
            <p className="text-slate-500 truncate">{c.title}</p>
          </div>
          <span className="text-[10px] px-1.5 py-0.5 rounded shrink-0"
            style={{ background: `${SEV_COLORS[c.severity] ?? '#888'}22`, color: SEV_COLORS[c.severity] ?? '#888' }}>
            {c.severity}
          </span>
        </div>
      ))}
    </div>
  )
}

function MitreChart({ data }: { data: any }) {
  const series = data?.result ?? []
  const chartData = series.map((s: any, i: number) => ({
    name: (s.metric?.event_type ?? `type-${i}`).replace('_', ' '),
    value: Math.round(parseFloat(s.values?.[s.values.length - 1]?.v ?? '0')),
    color: MITRE_COLORS[i % MITRE_COLORS.length],
  })).filter((d: any) => d.value > 0)

  if (!chartData.length) return (
    <div className="h-40 flex items-center justify-center text-xs text-slate-600">
      No security events recorded
    </div>
  )

  return (
    <ResponsiveContainer width="100%" height={180}>
      <PieChart>
        <Pie data={chartData} dataKey="value" nameKey="name" cx="50%" cy="50%"
          outerRadius={70} innerRadius={40}
          label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
          labelLine={false}
          style={{ fontSize: 10 }}>
          {chartData.map((entry: any, i: number) => (
            <Cell key={i} fill={entry.color} />
          ))}
        </Pie>
        <Tooltip contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
          formatter={(v: number, name: string) => [v, name]} />
      </PieChart>
    </ResponsiveContainer>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────

const TIME_RANGES = [
  { label: '1h',  s: 3600 },
  { label: '24h', s: 86400 },
  { label: '7d',  s: 604800 },
]

type ActiveTab = 'overview' | 'images' | 'runtime' | 'cves'

export default function SecurityPage() {
  const [range, setRange] = useState(86400)
  const [tab, setTab]     = useState<ActiveTab>('overview')
  const [cveSev, setCveSev] = useState('')

  const images       = useImageRisks()
  const { data: threatData } = useSecurityEvents(range)
  const { data: cveData }    = useCVEList()
  const { data: mitreData }  = useMitreSummary(range)

  const threatStreams = threatData ?? []
  const cveStreams    = cveData    ?? []

  // Summary counts
  const now   = Math.floor(Date.now() / 1000)
  const { data: threatCount } = useQuery({
    queryKey: ['sec-count', range],
    queryFn: () => metrics.query(
      `sum(increase(security_events_total{severity=~"CRITICAL|HIGH"}[${range}s]))`
    ),
    refetchInterval: 30_000,
  })
  const tc = Math.round(parseFloat((threatCount as any)?.data?.result?.[0]?.value?.[1] ?? '0'))

  const critImages = images.filter(i => i.critical > 0).length
  const totalCVEs  = parseCVEs(cveStreams).length

  const TABS: { id: ActiveTab; label: string }[] = [
    { id: 'overview', label: 'Overview' },
    { id: 'images',   label: `Image risks (${images.length})` },
    { id: 'runtime',  label: `Runtime threats` },
    { id: 'cves',     label: `CVE feed` },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Security monitoring"
        subtitle="Container vulnerability scanning · Runtime threat detection via eBPF"
        actions={
          <div className="flex bg-surface-2 border border-surface-3 rounded-lg overflow-hidden">
            {TIME_RANGES.map(r => (
              <button key={r.label} onClick={() => setRange(r.s)}
                className={clsx('px-2.5 py-1.5 text-xs transition-colors',
                  range === r.s ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
                {r.label}
              </button>
            ))}
          </div>
        }
      />

      {/* Tabs */}
      <div className="flex border-b border-surface-3 px-4 flex-shrink-0">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            {t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {/* ── OVERVIEW ── */}
        {tab === 'overview' && (
          <div className="space-y-4">
            {/* Stat cards */}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
              {[
                { label: 'Images scanned',       value: images.length,   icon: Package,   color: 'text-brand' },
                { label: 'Images with criticals', value: critImages,      icon: AlertTriangle, color: critImages > 0 ? 'text-crit' : 'text-ok' },
                { label: 'Critical/High CVEs',    value: totalCVEs,       icon: Shield,    color: totalCVEs > 0 ? 'text-warn' : 'text-ok' },
                { label: 'Runtime threats',       value: tc,              icon: Activity,  color: tc > 0 ? 'text-crit' : 'text-ok' },
              ].map(({ label, value, icon: Icon, color }) => (
                <div key={label} className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-xs text-slate-500">{label}</span>
                    <Icon size={13} className="text-slate-600" />
                  </div>
                  <div className={clsx('text-2xl font-semibold font-display', color)}>{value}</div>
                </div>
              ))}
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
              {/* Image risk bar chart */}
              <div className="lg:col-span-2 bg-surface-1 border border-surface-3 rounded-xl p-4">
                <div className="text-xs font-medium text-slate-400 mb-3">Image risk scores</div>
                {images.length > 0 ? (
                  <ResponsiveContainer width="100%" height={200}>
                    <BarChart data={images.slice(0, 12)} layout="vertical" barSize={12}>
                      <XAxis type="number" hide domain={[0, 100]} />
                      <YAxis type="category" dataKey="image" width={140}
                        tick={{ fill: 'hsl(215 20% 50%)', fontSize: 10, fontFamily: 'JetBrains Mono' }}
                        tickFormatter={v => v.split('/').pop()?.slice(0, 22) ?? v}
                        tickLine={false} />
                      <Tooltip
                        contentStyle={{ background: 'hsl(220 11% 12%)', border: '1px solid hsl(220 9% 21%)', borderRadius: 8, fontSize: 11 }}
                        formatter={(v: number, _, props) => [
                          `${v.toFixed(0)}/100 — C:${props.payload.critical} H:${props.payload.high}`,
                          'Risk score'
                        ]} />
                      <Bar dataKey="riskScore" radius={[0, 4, 4, 0]}>
                        {images.slice(0, 12).map((img, i) => (
                          <Cell key={i}
                            fill={img.riskScore >= 50 ? '#ef4444' : img.riskScore >= 25 ? '#f97316' : img.riskScore >= 10 ? '#f59e0b' : '#10b981'} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                ) : (
                  <div className="h-48 flex items-center justify-center text-xs text-slate-600">
                    Waiting for Trivy scan results…
                  </div>
                )}
              </div>

              {/* MITRE tactic breakdown */}
              <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
                <div className="text-xs font-medium text-slate-400 mb-1">Runtime events by type</div>
                <MitreChart data={mitreData} />
              </div>
            </div>

            {/* Recent critical threats */}
            <div className="bg-surface-1 border border-surface-3 rounded-xl">
              <div className="px-4 py-3 border-b border-surface-3 text-xs font-medium text-slate-400">
                Recent critical/high threats
              </div>
              <ThreatFeed streams={threatStreams} />
            </div>
          </div>
        )}

        {/* ── IMAGE RISKS ── */}
        {tab === 'images' && (
          <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
            <ImageRiskTable images={images} />
          </div>
        )}

        {/* ── RUNTIME THREATS ── */}
        {tab === 'runtime' && (
          <div className="space-y-4">
            <div className="grid grid-cols-4 gap-3">
              {['CRITICAL', 'HIGH', 'MEDIUM', 'LOW'].map(sev => {
                const count = parseThreatEvents(threatStreams).filter(e => e.severity === sev).length
                return (
                  <div key={sev} className="bg-surface-1 border border-surface-3 rounded-xl p-3 text-center">
                    <div className="text-xl font-semibold font-display" style={{ color: SEV_COLORS[sev] }}>{count}</div>
                    <div className="text-xs text-slate-500 mt-0.5">{sev}</div>
                  </div>
                )
              })}
            </div>

            <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
              <div className="px-4 py-3 border-b border-surface-3 flex items-center justify-between">
                <span className="text-xs font-medium text-slate-400">eBPF threat events</span>
                <span className="text-xs text-slate-600">
                  Probes: execve · connect · openat · ptrace
                </span>
              </div>
              <ThreatFeed streams={threatStreams} />
            </div>
          </div>
        )}

        {/* ── CVE FEED ── */}
        {tab === 'cves' && (
          <div className="space-y-3">
            {/* Severity filter */}
            <div className="flex items-center gap-2">
              <Filter size={12} className="text-slate-500" />
              <div className="flex gap-1">
                {['', 'CRITICAL', 'HIGH', 'MEDIUM'].map(sev => (
                  <button key={sev} onClick={() => setCveSev(sev)}
                    className={clsx('px-2.5 py-1 text-xs rounded-lg border transition-colors',
                      cveSev === sev
                        ? 'border-brand/40 bg-brand/15 text-brand'
                        : 'border-surface-3 bg-surface-2 text-slate-500 hover:text-slate-300')}>
                    {sev || 'All'}
                  </button>
                ))}
              </div>
            </div>

            <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
              <div className="px-4 py-3 border-b border-surface-3 flex items-center justify-between">
                <span className="text-xs font-medium text-slate-400">CVE findings from Trivy</span>
                <span className="text-xs text-slate-600">{totalCVEs} unique CVEs</span>
              </div>
              <CVEFeed streams={cveStreams} filterSev={cveSev} />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
