// frontend/src/pages/DashboardsPage.tsx
// ═══════════════════════════════════════════════════════════════════════
//  ObserveX — In-house Dashboard Builder (built from scratch)
//  Grafana-equivalent: no Grafana required.
//
//  Features:
//  • 10 live widget types: timeseries · area · bar · stat · gauge ·
//    heatmap · pie · logs · slo · alertlist · text · table
//  • 12-column CSS grid layout with col-span per widget
//  • Real data from VictoriaMetrics PromQL + Loki LogQL + ObserveX APIs
//  • Dashboard create / clone / delete
//  • 5 built-in templates with real PromQL queries
//  • Per-widget live refresh (30s metrics, 30s logs)
//  • Widget editor: type + query + thresholds + options
//  • Time range selector per dashboard
//  • Fullscreen viewer mode
//  • Import / export dashboard JSON
// ═══════════════════════════════════════════════════════════════════════

import { useState, useMemo, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { dashboards as dashApi, metrics, logs, slos, problems } from '@/lib/api'
import { PageHeader, Spinner, EmptyState, Btn, SearchInput } from '@/components/shared/Layout'
import {
  LayoutDashboard, Plus, Copy, Trash2, Clock, X, Check,
  ChevronLeft, RefreshCw, Settings, Download, Upload,
  TrendingUp, BarChart2, Gauge, Activity, FileText,
  ShieldCheck, Bell, Type, Table2, PieChart, Layers,
  Maximize2, Minimize2, Edit3, Eye,
} from 'lucide-react'
import {
  LineChart, Line, AreaChart, Area, BarChart, Bar,
  PieChart as RechartsPie, Pie, Cell,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend,
} from 'recharts'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'
import toast from 'react-hot-toast'

// ── Constants ─────────────────────────────────────────────────────────────────

const PALETTE = [
  '#3b82f6','#22c55e','#f59e0b','#ef4444','#8b5cf6',
  '#ec4899','#14b8a6','#f97316','#06b6d4','#84cc16',
  '#a855f7','#10b981','#fb923c','#60a5fa',
]

const TOOLTIP_STYLE = {
  contentStyle: {
    background: 'hsl(220 11% 12%)',
    border: '1px solid hsl(220 9% 21%)',
    borderRadius: 8,
    fontSize: 11,
  },
}

const COL_SPAN: Record<number, string> = {
  2:'col-span-2', 3:'col-span-3', 4:'col-span-4',
  5:'col-span-5', 6:'col-span-6', 7:'col-span-7',
  8:'col-span-8', 9:'col-span-9', 10:'col-span-10',
  12:'col-span-12',
}

const WIDGET_HEIGHTS: Record<string, number> = {
  timeseries:200, area:200, bar:180, stat:110, gauge:140,
  heatmap:180, pie:180, logs:200, slo:180, alertlist:160,
  text:100, table:180,
}

// ── Widget type catalog ───────────────────────────────────────────────────────

const WIDGET_TYPES = [
  { id:'timeseries', label:'Time series',  icon:TrendingUp, desc:'Line chart over time · PromQL range query' },
  { id:'area',       label:'Area chart',   icon:Layers,     desc:'Filled area chart · PromQL range query' },
  { id:'bar',        label:'Bar chart',    icon:BarChart2,  desc:'Horizontal/vertical bars · instant PromQL' },
  { id:'stat',       label:'Stat',         icon:Activity,   desc:'Single big number with threshold color' },
  { id:'gauge',      label:'Gauge',        icon:Gauge,      desc:'Radial gauge with min/max · instant PromQL' },
  { id:'pie',        label:'Pie chart',    icon:PieChart,   desc:'Pie/donut · instant PromQL' },
  { id:'heatmap',    label:'Heatmap',      icon:LayoutDashboard, desc:'Value grid over time · PromQL range' },
  { id:'logs',       label:'Logs',         icon:FileText,   desc:'Live log stream · LogQL query via Loki' },
  { id:'slo',        label:'SLO status',   icon:ShieldCheck,desc:'Error budget + burn rates from ObserveX' },
  { id:'alertlist',  label:'Alert list',   icon:Bell,       desc:'Open incidents from ObserveX' },
  { id:'table',      label:'Table',        icon:Table2,     desc:'Instant PromQL results as table' },
  { id:'text',       label:'Text / MD',    icon:Type,       desc:'Markdown text panel — headings, links' },
]

// ── Built-in dashboard templates with real PromQL ──────────────────────────────

const TEMPLATES: Record<string, { description: string; tags: string[]; widgets: any[] }> = {
  'Platform overview': {
    description: 'Service health, request rate, errors, SLOs, incidents',
    tags: ['overview', 'slo', 'incidents'],
    widgets: [
      { id:'w1', type:'timeseries', title:'Request rate by service',   col:8, row:3, query:'sum(rate(http_requests_total[1m])) by (service_name)' },
      { id:'w2', type:'stat',       title:'Open incidents',            col:4, row:3, query:'sum(observex_problems_open_total)', thresholds:[{v:1,c:'orange'},{v:3,c:'red'}] },
      { id:'w3', type:'gauge',      title:'Cluster CPU %',             col:4, row:3, query:'avg((1 - avg(rate(node_cpu_idle[5m])) / avg(rate(node_cpu_total[5m]))) * 100)', options:{max:100,unit:'%'} },
      { id:'w4', type:'slo',        title:'SLO health',                col:4, row:3, query:'' },
      { id:'w5', type:'alertlist',  title:'Active incidents',          col:4, row:3, query:'' },
      { id:'w6', type:'area',       title:'Error rate by service (%)', col:6, row:3, query:'sum(rate(http_requests_total{status_code=~"5.."}[5m])) by (service_name) / sum(rate(http_requests_total[5m])) by (service_name) * 100' },
      { id:'w7', type:'logs',       title:'Recent error logs',         col:6, row:3, query:'{namespace!=""} | level = "error"' },
    ],
  },
  'Infrastructure': {
    description: 'Node CPU, memory, disk, network metrics',
    tags: ['infra', 'nodes', 'k8s'],
    widgets: [
      { id:'w1', type:'timeseries', title:'CPU % by node',         col:6, row:3, query:'(1 - avg(rate(node_cpu_idle[5m])) by (node) / avg(rate(node_cpu_total[5m])) by (node)) * 100' },
      { id:'w2', type:'timeseries', title:'Memory % by node',      col:6, row:3, query:'(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100' },
      { id:'w3', type:'timeseries', title:'Network Rx (Mbps)',      col:6, row:3, query:'rate(node_network_receive_bytes_total[5m]) * 8 / 1e6' },
      { id:'w4', type:'timeseries', title:'Network Tx (Mbps)',      col:6, row:3, query:'rate(node_network_transmit_bytes_total[5m]) * 8 / 1e6' },
      { id:'w5', type:'bar',        title:'Disk usage % by node',   col:6, row:2, query:'(1 - node_filesystem_avail_bytes / node_filesystem_size_bytes) * 100' },
      { id:'w6', type:'stat',       title:'Healthy nodes',          col:3, row:2, query:'count(up{job="node"}==1)', thresholds:[] },
      { id:'w7', type:'gauge',      title:'Avg memory %',           col:3, row:2, query:'avg((1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100)', options:{max:100,unit:'%'} },
    ],
  },
  'APM — RED metrics': {
    description: 'Rate, errors, duration per service (Apdex)',
    tags: ['apm', 'latency', 'errors'],
    widgets: [
      { id:'w1', type:'timeseries', title:'Request rate (req/s)',     col:8, row:3, query:'sum(rate(http_requests_total[1m])) by (service_name)' },
      { id:'w2', type:'stat',       title:'Avg P99 latency',          col:4, row:3, query:'avg(histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service_name))) * 1000', options:{unit:'ms'}, thresholds:[{v:200,c:'orange'},{v:1000,c:'red'}] },
      { id:'w3', type:'area',       title:'P99 latency by service',   col:6, row:3, query:'histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service_name)) * 1000' },
      { id:'w4', type:'area',       title:'Error rate % by service',  col:6, row:3, query:'sum(rate(http_requests_total{status_code=~"5.."}[5m])) by (service_name) / sum(rate(http_requests_total[5m])) by (service_name) * 100' },
      { id:'w5', type:'bar',        title:'Top services by RPS',      col:6, row:2, query:'topk(6, sum(rate(http_requests_total[5m])) by (service_name))' },
      { id:'w6', type:'logs',       title:'Error logs',               col:6, row:2, query:'{namespace!=""} | level = "error"' },
    ],
  },
  'SLO tracker': {
    description: 'Error budgets, burn rates, compliance across all services',
    tags: ['slo', 'reliability', 'error-budget'],
    widgets: [
      { id:'w1', type:'slo',        title:'All SLOs — error budget',  col:12, row:5, query:'' },
      { id:'w2', type:'timeseries', title:'SLI trend (error rate %)', col:8,  row:3, query:'sum(rate(http_requests_total{status_code=~"5.."}[5m])) by (service_name) / sum(rate(http_requests_total[5m])) by (service_name) * 100' },
      { id:'w3', type:'stat',       title:'Burn rate 1h',             col:4,  row:3, query:'observex_slo_burn_rate_1h', thresholds:[{v:6,c:'orange'},{v:14.4,c:'red'}] },
    ],
  },
  'Security posture': {
    description: 'CVE scanning, eBPF threats, auth failures, risk scores',
    tags: ['security', 'cve', 'compliance'],
    widgets: [
      { id:'w1', type:'stat',  title:'Critical CVEs',          col:3, row:2, query:'sum(image_scan_vulns_critical)', thresholds:[{v:1,c:'orange'},{v:5,c:'red'}] },
      { id:'w2', type:'stat',  title:'Runtime threats (24h)',  col:3, row:2, query:'sum(increase(security_events_total[24h]))', thresholds:[{v:1,c:'orange'}] },
      { id:'w3', type:'stat',  title:'Failed logins (24h)',    col:3, row:2, query:'sum(increase(auth_login_failed_total[24h]))', thresholds:[{v:5,c:'orange'},{v:20,c:'red'}] },
      { id:'w4', type:'gauge', title:'Auth failure rate %',    col:3, row:2, query:'sum(rate(auth_login_failed_total[1h])) / sum(rate(auth_login_total[1h])) * 100', options:{max:100,unit:'%'} },
      { id:'w5', type:'bar',   title:'CVEs by image',          col:6, row:3, query:'sum(image_scan_vulns_total) by (image)' },
      { id:'w6', type:'logs',  title:'Security events',        col:6, row:3, query:'{source="security"}' },
    ],
  },
}

// ─────────────────────────────────────────────────────────────────────────────
//  LIVE WIDGET COMPONENTS
// ─────────────────────────────────────────────────────────────────────────────

function fmtVal(v: number): string {
  if (v == null || isNaN(v)) return '—'
  if (Math.abs(v) >= 1e9)  return `${(v/1e9).toFixed(1)}G`
  if (Math.abs(v) >= 1e6)  return `${(v/1e6).toFixed(1)}M`
  if (Math.abs(v) >= 1000) return `${(v/1000).toFixed(1)}k`
  if (Math.abs(v) >= 1)    return v.toFixed(2)
  return v.toFixed(4)
}

function useRangeData(widget: any, enabled = true) {
  const now = Math.floor(Date.now() / 1000)
  return useQuery({
    queryKey: ['dw-range', widget.id, widget.query],
    queryFn:  () => metrics.queryRange(widget.query, now - 3600, now, '60'),
    refetchInterval: 30_000,
    enabled: enabled && !!widget.query,
  })
}

function useInstantData(widget: any, enabled = true) {
  return useQuery({
    queryKey: ['dw-instant', widget.id, widget.query],
    queryFn:  () => metrics.query(widget.query),
    refetchInterval: 30_000,
    enabled: enabled && !!widget.query,
  })
}

function buildTimeseriesData(data: any) {
  const result = data?.result ?? []
  if (!result.length) return { pts: [], keys: [] }
  const allTs = new Set<number>()
  result.forEach((s: any) => s.values?.forEach(([t]: any) => allTs.add(+t)))
  const keys: string[] = result.map((s: any, i: number) =>
    (Object.values(s.metric as any)[0] as string) || `s${i}`)
  const pts = [...allTs].sort().map(ts => {
    const row: any = { t: format(new Date(ts * 1000), 'HH:mm') }
    result.forEach((s: any, i: number) => {
      const pt = s.values?.find(([v]: any) => +v === ts)
      row[keys[i]] = pt ? (parseFloat(pt[1]) || 0) : null
    })
    return row
  })
  return { pts, keys }
}

// Time series widget
function TimeseriesWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useRangeData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const { pts, keys } = buildTimeseriesData(data)
  if (!pts.length) return <div className="text-xs text-slate-600 py-4 text-center">No data · waiting for metrics</div>
  return (
    <ResponsiveContainer width="100%" height="100%">
      <LineChart data={pts}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 17%)" vertical={false}/>
        <XAxis dataKey="t" tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false}/>
        <YAxis tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false} width={36} tickFormatter={fmtVal}/>
        <Tooltip {...TOOLTIP_STYLE}/>
        {keys.map((k, i) => <Line key={k} type="monotone" dataKey={k} stroke={PALETTE[i%PALETTE.length]} dot={false} strokeWidth={1.5} connectNulls isAnimationActive={false}/>)}
      </LineChart>
    </ResponsiveContainer>
  )
}

// Area widget
function AreaWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useRangeData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const { pts, keys } = buildTimeseriesData(data)
  if (!pts.length) return <div className="text-xs text-slate-600 py-4 text-center">No data</div>
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={pts}>
        <defs>
          {keys.map((k, i) => (
            <linearGradient key={k} id={`ag-${i}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%"  stopColor={PALETTE[i%PALETTE.length]} stopOpacity={0.25}/>
              <stop offset="95%" stopColor={PALETTE[i%PALETTE.length]} stopOpacity={0}/>
            </linearGradient>
          ))}
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 17%)" vertical={false}/>
        <XAxis dataKey="t" tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false}/>
        <YAxis tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false} width={36} tickFormatter={fmtVal}/>
        <Tooltip {...TOOLTIP_STYLE}/>
        {keys.map((k, i) => (
          <Area key={k} type="monotone" dataKey={k}
            stroke={PALETTE[i%PALETTE.length]} fill={`url(#ag-${i})`}
            strokeWidth={1.5} dot={false} connectNulls isAnimationActive={false}/>
        ))}
      </AreaChart>
    </ResponsiveContainer>
  )
}

// Bar widget (instant query)
function BarWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useInstantData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const bd = ((data as any)?.data?.result ?? []).map((r: any, i: number) => ({
    name: String(Object.values(r.metric as any)[0] ?? `s${i}`).slice(0, 14),
    value: parseFloat(r.value?.[1] ?? 0),
    color: PALETTE[i % PALETTE.length],
  }))
  if (!bd.length) return <div className="text-xs text-slate-600 py-4 text-center">No data</div>
  return (
    <ResponsiveContainer width="100%" height="100%">
      <BarChart data={bd} barSize={18}>
        <CartesianGrid strokeDasharray="3 3" stroke="hsl(220 9% 17%)" vertical={false}/>
        <XAxis dataKey="name" tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false}/>
        <YAxis tick={{fill:'hsl(215 20% 40%)',fontSize:9}} tickLine={false} axisLine={false} width={36} tickFormatter={fmtVal}/>
        <Tooltip {...TOOLTIP_STYLE}/>
        <Bar dataKey="value" radius={[3,3,0,0]}>
          {bd.map((d: any, i: number) => <Cell key={i} fill={d.color}/>)}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

// Stat widget
function StatWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useInstantData(widget)
  if (isLoading) return <div className="flex justify-center pt-2"><Spinner size={14}/></div>
  const val = parseFloat((data as any)?.data?.result?.[0]?.value?.[1] ?? 'NaN')
  const thresholds: any[] = widget.thresholds ?? []
  let color = 'text-slate-200'
  for (const t of [...thresholds].sort((a, b) => a.v - b.v)) {
    if (!isNaN(val) && val >= t.v)
      color = t.c === 'red' ? 'text-crit' : t.c === 'orange' ? 'text-warn' : 'text-ok'
  }
  return (
    <div className="flex flex-col items-center justify-center h-full gap-0.5">
      <div className={clsx('text-4xl font-bold font-mono leading-none', isNaN(val) ? 'text-slate-600' : color)}>
        {isNaN(val) ? '—' : fmtVal(val)}
      </div>
      {widget.options?.unit && <div className="text-xs text-slate-500">{widget.options.unit}</div>}
    </div>
  )
}

// Gauge widget (SVG arc)
function GaugeWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useInstantData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const val  = parseFloat((data as any)?.data?.result?.[0]?.value?.[1] ?? '0') || 0
  const max  = widget.options?.max ?? 100
  const pct  = Math.min(100, (val / max) * 100)
  const color = pct > 90 ? '#ef4444' : pct > 75 ? '#f59e0b' : '#22c55e'
  const r = 44, cx = 60, cy = 58
  const toRad = (d: number) => d * Math.PI / 180
  const arc = (s: number, e: number) => {
    const sx = cx + r*Math.cos(toRad(s)), sy = cy + r*Math.sin(toRad(s))
    const ex = cx + r*Math.cos(toRad(e)), ey = cy + r*Math.sin(toRad(e))
    return `M ${sx} ${sy} A ${r} ${r} 0 ${Math.abs(e-s)>180?1:0} 1 ${ex} ${ey}`
  }
  return (
    <div className="flex flex-col items-center justify-center h-full">
      <svg width="120" height="80" viewBox="0 0 120 80">
        <path d={arc(-210,30)} fill="none" stroke="hsl(220 9% 21%)" strokeWidth="10" strokeLinecap="round"/>
        <path d={arc(-210,-210+(pct/100)*240)} fill="none" stroke={color} strokeWidth="10" strokeLinecap="round"/>
        <text x="60" y="66" textAnchor="middle" fill={color} fontSize="17" fontWeight="700" fontFamily="JetBrains Mono,monospace">{val.toFixed(1)}</text>
      </svg>
      <div className="text-xs text-slate-500 -mt-1">{widget.options?.unit ?? ''}</div>
    </div>
  )
}

// Pie / donut widget
function PieWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useInstantData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const pd = ((data as any)?.data?.result ?? []).map((r: any, i: number) => ({
    name: String(Object.values(r.metric as any)[0] ?? `s${i}`).slice(0, 16),
    value: parseFloat(r.value?.[1] ?? 0),
    color: PALETTE[i % PALETTE.length],
  }))
  if (!pd.length) return <div className="text-xs text-slate-600 py-4 text-center">No data</div>
  return (
    <ResponsiveContainer width="100%" height="100%">
      <RechartsPie>
        <Tooltip {...TOOLTIP_STYLE}/>
        <Pie data={pd} cx="50%" cy="50%" innerRadius="40%" outerRadius="70%"
          dataKey="value" nameKey="name" paddingAngle={2} isAnimationActive={false}>
          {pd.map((d: any, i: number) => <Cell key={i} fill={d.color}/>)}
        </Pie>
        <Legend iconType="circle" iconSize={8} wrapperStyle={{fontSize:10,color:'hsl(215 20% 55%)'}}/>
      </RechartsPie>
    </ResponsiveContainer>
  )
}

// Heatmap widget (value grid across time, color-coded)
function HeatmapWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useRangeData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const result = data?.result ?? []
  if (!result.length) return <div className="text-xs text-slate-600 py-4 text-center">No data</div>

  // Build grid: rows=series, cols=timestamps (last 24 points)
  const series = result.slice(0, 6)
  const allTs = new Set<number>()
  series.forEach((s: any) => s.values?.slice(-24).forEach(([t]: any) => allTs.add(+t)))
  const tsList = [...allTs].sort().slice(-24)
  const allVals = series.flatMap((s: any) =>
    tsList.map(ts => { const pt = s.values?.find(([t]: any) => +t === ts); return pt ? parseFloat(pt[1]) : 0 })
  ).filter(v => !isNaN(v))
  const maxV = Math.max(...allVals, 1)

  const cellColor = (v: number) => {
    const p = v / maxV
    if (p > 0.9) return '#ef4444'
    if (p > 0.7) return '#f97316'
    if (p > 0.5) return '#f59e0b'
    if (p > 0.3) return '#22c55e'
    if (p > 0.1) return '#16a34a'
    return 'hsl(220 9% 21%)'
  }

  return (
    <div className="h-full flex flex-col gap-0.5 overflow-hidden pt-1">
      {series.map((s: any, si: number) => {
        const name = String(Object.values(s.metric as any)[0] ?? `s${si}`).slice(0,12)
        return (
          <div key={si} className="flex items-center gap-1 flex-1">
            <div className="text-[9px] text-slate-600 w-20 text-right shrink-0 font-mono truncate">{name}</div>
            <div className="flex-1 flex gap-px">
              {tsList.map(ts => {
                const pt = s.values?.find(([t]: any) => +t === ts)
                const v  = pt ? parseFloat(pt[1]) : 0
                return <div key={ts} title={`${name}: ${fmtVal(v)}`} className="flex-1 rounded-sm" style={{background:cellColor(v)}}/>
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

// Logs widget — Loki live stream
function LogsWidget({ widget }: { widget: any }) {
  const now   = Math.floor(Date.now() * 1e6)
  const start = now - 3600 * 1e9
  const { data, isLoading } = useQuery({
    queryKey: ['dw-logs', widget.id, widget.query],
    queryFn:  () => logs.queryRange(widget.query || '{namespace!=""}', start, now, 15),
    refetchInterval: 30_000,
  })
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const lines = (data ?? [])
    .flatMap((s: any) => (s.values ?? []).map(([ts, line]: [string,string]) => ({
      ts: format(new Date(Number(ts)/1e6), 'HH:mm:ss'),
      level: (line.match(/"level":"(\w+)"/) ?? [])[1] ?? 'info',
      line,
    })))
    .slice(-14)
  const lc: Record<string,string> = { error:'text-crit', warn:'text-warn', info:'text-info', debug:'text-slate-600' }
  return (
    <div className="overflow-auto h-full font-mono text-[10px] leading-relaxed">
      {lines.map((l: any, i: number) => (
        <div key={i} className="flex gap-2 py-0.5 border-b border-surface-3/25">
          <span className="text-slate-600 shrink-0 w-16">{l.ts}</span>
          <span className={clsx('font-semibold w-8 shrink-0 uppercase', lc[l.level] ?? 'text-slate-400')}>{l.level.slice(0,4)}</span>
          <span className="text-slate-300 truncate">{l.line.slice(0, 120)}</span>
        </div>
      ))}
      {!lines.length && <div className="text-slate-600 py-4 text-center">No logs · check LogQL query</div>}
    </div>
  )
}

// SLO status widget
function SLOWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useQuery({
    queryKey: ['dw-slos'],
    queryFn:  () => slos.list({ limit: 6 }),
    refetchInterval: 60_000,
  })
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const list = (data?.slos ?? []).slice(0, 5)
  return (
    <div className="space-y-2 overflow-auto h-full">
      {list.map((s: any) => {
        const sc = ({'OK': 'text-ok', 'WARN': 'text-warn', 'BREACHED': 'text-crit'} as Record<string,string>)[String(s.status || 'OK')] ?? 'text-ok'
        const bg = ({'OK': 'bg-ok', 'WARN': 'bg-warn', 'BREACHED': 'bg-crit'} as Record<string,string>)[String(s.status || 'OK')] ?? 'bg-ok'
        return (
          <div key={s.id} className="bg-surface-2 rounded-lg px-3 py-2">
            <div className="flex justify-between text-xs mb-1.5">
              <span className="text-slate-300 truncate">{s.name}</span>
              <span className={clsx('font-bold text-[11px] font-mono', sc)}>{(s.sli ?? 0).toFixed(2)}%</span>
            </div>
            <div className="h-1.5 bg-surface-3 rounded-full overflow-hidden">
              <div className={clsx('h-full rounded-full', bg)} style={{width:`${s.budget_left ?? 100}%`}}/>
            </div>
            <div className="flex justify-between text-[9px] text-slate-600 mt-0.5">
              <span>Budget: {(s.budget_left ?? 100).toFixed(1)}%</span>
              <span>1h burn: {(s.burn_rate_1h ?? 0).toFixed(1)}×</span>
            </div>
          </div>
        )
      })}
      {!list.length && <div className="text-xs text-slate-600 py-4 text-center">No SLOs configured</div>}
    </div>
  )
}

// Alert list widget
function AlertListWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useQuery({
    queryKey: ['dw-problems'],
    queryFn:  () => problems.list({ limit: 6, status: 'OPEN' }),
    refetchInterval: 30_000,
  })
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const list = data?.problems ?? []
  return (
    <div className="space-y-1.5 overflow-auto h-full">
      {list.slice(0,6).map((p: any) => {
        const sc = ({'CRITICAL': 'text-crit', 'HIGH': 'text-warn', 'MEDIUM': 'text-info'} as Record<string,string>)[p.severity] ?? 'text-slate-400'
        return (
          <div key={p.id} className="flex items-start gap-2 text-xs">
            <span className={clsx('font-bold shrink-0 text-[10px] w-14', sc)}>{p.severity}</span>
            <span className="text-slate-300 truncate leading-relaxed">{p.title}</span>
          </div>
        )
      })}
      {!list.length && (
        <div className="flex items-center gap-2 py-4 text-xs text-ok">
          <span className="w-1.5 h-1.5 rounded-full bg-ok"/>All systems operational
        </div>
      )}
    </div>
  )
}

// Table widget — instant PromQL as rows
function TableWidget({ widget }: { widget: any }) {
  const { data, isLoading } = useInstantData(widget)
  if (isLoading) return <div className="flex justify-center pt-4"><Spinner size={14}/></div>
  const results: any[] = (data as any)?.data?.result ?? []
  return (
    <div className="overflow-auto h-full">
      {results.map((r: any, i: number) => {
        const labels = Object.entries(r.metric ?? {}).map(([k,v]) => `${k}="${v}"`).join(', ')
        const val    = parseFloat(r.value?.[1] ?? '0')
        return (
          <div key={i} className="flex justify-between gap-3 py-1.5 border-b border-surface-3/30 text-[10px] font-mono">
            <span className="text-slate-400 truncate flex-1">{labels || '(no labels)'}</span>
            <span className="text-slate-200 font-semibold shrink-0">{fmtVal(val)}</span>
          </div>
        )
      })}
      {!results.length && <div className="text-slate-600 py-4 text-center text-xs">No data</div>}
    </div>
  )
}

// Text / Markdown widget
function TextWidget({ widget }: { widget: any }) {
  const content = widget.query || widget.options?.content || '# Text panel\nAdd markdown content in the widget editor.'
  return (
    <div className="text-xs text-slate-300 leading-relaxed overflow-auto h-full space-y-1.5">
      {content.split('\n').map((line: string, i: number) => {
        if (line.startsWith('# '))  return <h2 key={i} className="text-sm font-bold text-slate-200">{line.slice(2)}</h2>
        if (line.startsWith('## ')) return <h3 key={i} className="text-xs font-semibold text-slate-300">{line.slice(3)}</h3>
        if (line.startsWith('- '))  return <li key={i} className="ml-3 text-slate-400">{line.slice(2)}</li>
        return <p key={i} className="text-slate-400">{line}</p>
      })}
    </div>
  )
}

// ── Widget dispatcher ─────────────────────────────────────────────────────────

const WIDGET_MAP: Record<string, React.FC<{widget:any}>> = {
  timeseries: TimeseriesWidget,
  area:       AreaWidget,
  bar:        BarWidget,
  stat:       StatWidget,
  gauge:      GaugeWidget,
  pie:        PieWidget,
  heatmap:    HeatmapWidget,
  logs:       LogsWidget,
  slo:        SLOWidget,
  alertlist:  AlertListWidget,
  table:      TableWidget,
  text:       TextWidget,
}

function LiveWidget({ widget, onEdit, editable }: { widget: any; onEdit?: () => void; editable?: boolean }) {
  const Component = WIDGET_MAP[widget.type] ?? TextWidget
  const h = WIDGET_HEIGHTS[widget.type] ?? 160
  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-3 flex flex-col overflow-hidden group hover:border-brand/30 transition-colors"
      style={{ minHeight: h }}>
      <div className="text-[11px] font-medium text-slate-400 mb-2 flex-shrink-0 flex items-center justify-between gap-2">
        <span className="truncate">{widget.title || widget.type}</span>
        <div className="flex items-center gap-1 shrink-0">
          <span className="text-[9px] text-slate-700 font-mono bg-surface-2 px-1.5 py-0.5 rounded">{widget.type}</span>
          {editable && onEdit && (
            <button onClick={onEdit}
              className="opacity-0 group-hover:opacity-100 p-0.5 text-slate-600 hover:text-slate-300 transition-all">
              <Edit3 size={11}/>
            </button>
          )}
        </div>
      </div>
      <div className="flex-1 min-h-0">
        <Component widget={widget}/>
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
//  WIDGET EDITOR MODAL
// ─────────────────────────────────────────────────────────────────────────────

function WidgetEditor({ widget, onSave, onClose }: {
  widget: Partial<any>; onSave: (w: any) => void; onClose: () => void
}) {
  const [w, setW] = useState<any>({ type:'timeseries', title:'', query:'', col:6, ...widget })
  const upd = (k: string, v: any) => setW((p: any) => ({ ...p, [k]: v }))

  return (
    <div className="fixed inset-0 bg-black/70 z-50 flex items-center justify-center p-4">
      <div className="w-full max-w-lg bg-surface-1 border border-surface-3 rounded-2xl overflow-hidden shadow-2xl animate-slide-up">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-surface-3">
          <h2 className="text-sm font-semibold text-slate-200">
            {widget.id ? 'Edit widget' : 'Add widget'}
          </h2>
          <button onClick={onClose}><X size={15} className="text-slate-500 hover:text-slate-300"/></button>
        </div>

        <div className="p-5 space-y-4 max-h-[75vh] overflow-y-auto">
          {/* Widget type selector */}
          <div>
            <label className="block text-xs text-slate-500 mb-2">Widget type</label>
            <div className="grid grid-cols-4 gap-1.5">
              {WIDGET_TYPES.map(wt => {
                const Icon = wt.icon
                return (
                  <button key={wt.id} onClick={() => upd('type', wt.id)}
                    title={wt.desc}
                    className={clsx('flex flex-col items-center gap-1 p-2 rounded-lg border text-[10px] transition-all',
                      w.type === wt.id
                        ? 'border-brand/50 bg-brand/10 text-brand'
                        : 'border-surface-3 text-slate-500 hover:text-slate-300 hover:border-surface-4')}>
                    <Icon size={14}/>
                    <span>{wt.label}</span>
                  </button>
                )
              })}
            </div>
          </div>

          {/* Title */}
          <div>
            <label className="block text-xs text-slate-500 mb-1">Title</label>
            <input value={w.title} onChange={e => upd('title', e.target.value)}
              placeholder="My widget"
              className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60"/>
          </div>

          {/* Query */}
          {!['slo','alertlist','text'].includes(w.type) && (
            <div>
              <label className="block text-xs text-slate-500 mb-1">
                {w.type === 'logs' ? 'LogQL query' : 'PromQL query'}
              </label>
              <textarea value={w.query} onChange={e => upd('query', e.target.value)} rows={3}
                placeholder={w.type === 'logs'
                  ? '{namespace!=""} | level = "error"'
                  : 'sum(rate(http_requests_total[1m])) by (service_name)'}
                className="w-full px-3 py-2 text-xs font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60 resize-none"/>
            </div>
          )}

          {/* Width */}
          <div>
            <label className="block text-xs text-slate-500 mb-1">Width (columns of 12)</label>
            <div className="flex gap-1.5">
              {[3,4,6,8,12].map(n => (
                <button key={n} onClick={() => upd('col', n)}
                  className={clsx('flex-1 py-1.5 text-xs rounded-lg border transition-colors',
                    w.col === n ? 'bg-brand/20 border-brand/40 text-brand' : 'border-surface-3 text-slate-500 hover:text-slate-300')}>
                  {n === 12 ? 'Full' : n === 6 ? 'Half' : `${n}/12`}
                </button>
              ))}
            </div>
          </div>

          {/* Stat/gauge options */}
          {['stat','gauge'].includes(w.type) && (
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Unit label</label>
                <input value={w.options?.unit ?? ''} onChange={e => upd('options', {...(w.options??{}), unit: e.target.value})}
                  placeholder="ms · % · req/s"
                  className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none"/>
              </div>
              {w.type === 'gauge' && (
                <div>
                  <label className="block text-xs text-slate-500 mb-1">Max value</label>
                  <input type="number" value={w.options?.max ?? 100}
                    onChange={e => upd('options', {...(w.options??{}), max: Number(e.target.value)})}
                    className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none"/>
                </div>
              )}
            </div>
          )}

          {/* Text content */}
          {w.type === 'text' && (
            <div>
              <label className="block text-xs text-slate-500 mb-1">Markdown content</label>
              <textarea value={w.query} onChange={e => upd('query', e.target.value)} rows={5}
                placeholder={'# Title\n\nDescription text\n\n- bullet 1\n- bullet 2'}
                className="w-full px-3 py-2 text-xs font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none resize-none"/>
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 px-5 py-3.5 border-t border-surface-3">
          <Btn onClick={onClose}>Cancel</Btn>
          <Btn variant="primary" onClick={() => onSave({ ...w, id: w.id ?? `w${Date.now()}` })}>
            <Check size={13}/> {widget.id ? 'Update' : 'Add widget'}
          </Btn>
        </div>
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
//  DASHBOARD VIEWER / EDITOR
// ─────────────────────────────────────────────────────────────────────────────

function DashboardViewer({ dash, onClose, onUpdate }: {
  dash: any; onClose: () => void; onUpdate: (d: any) => void
}) {
  const [editing,    setEditing]    = useState(false)
  const [addWidget,  setAddWidget]  = useState(false)
  const [editWidget, setEditWidget] = useState<any | null>(null)
  const [key,        setKey]        = useState(0)
  const [fullscreen, setFullscreen] = useState(false)

  const widgets: any[] = dash.widgets?.length
    ? dash.widgets
    : TEMPLATES[dash.name]?.widgets ?? []

  const saveWidget = (w: any) => {
    const existing = widgets.findIndex((x: any) => x.id === w.id)
    const next = existing >= 0
      ? widgets.map((x: any) => x.id === w.id ? w : x)
      : [...widgets, w]
    onUpdate({ ...dash, widgets: next })
    setAddWidget(false)
    setEditWidget(null)
  }

  const removeWidget = (id: string) => {
    onUpdate({ ...dash, widgets: widgets.filter((w: any) => w.id !== id) })
  }

  // Export dashboard as JSON
  const exportDash = () => {
    const blob = new Blob([JSON.stringify({ ...dash, widgets }, null, 2)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href     = URL.createObjectURL(blob)
    a.download = `${dash.name.replace(/\s+/g, '-').toLowerCase()}.json`
    a.click()
  }

  return (
    <div className={clsx('flex flex-col overflow-hidden', fullscreen ? 'fixed inset-0 z-40 bg-surface-0' : 'flex-1')}>
      {/* Toolbar */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-surface-3 flex-shrink-0">
        <button onClick={onClose} className="flex items-center gap-1.5 text-xs text-slate-500 hover:text-slate-300 transition-colors">
          <ChevronLeft size={14}/> Dashboards
        </button>
        <span className="text-slate-700">/</span>
        <span className="text-sm font-semibold text-slate-200">{dash.name}</span>
        {dash.description && <span className="text-xs text-slate-600 hidden md:inline">{dash.description}</span>}

        <div className="flex items-center gap-1.5 ml-auto">
          <button onClick={() => setKey(k => k+1)}
            className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 px-2 py-1.5 rounded-lg hover:bg-surface-2 transition-colors">
            <RefreshCw size={11}/> Refresh
          </button>
          <button onClick={exportDash}
            className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 px-2 py-1.5 rounded-lg hover:bg-surface-2 transition-colors">
            <Download size={11}/> Export
          </button>
          <button onClick={() => setEditing(e => !e)}
            className={clsx('flex items-center gap-1 text-xs px-2 py-1.5 rounded-lg transition-colors',
              editing ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300 hover:bg-surface-2')}>
            {editing ? <><Eye size={11}/>View</> : <><Edit3 size={11}/>Edit</>}
          </button>
          {editing && (
            <button onClick={() => setAddWidget(true)}
              className="flex items-center gap-1 text-xs bg-brand text-white px-2.5 py-1.5 rounded-lg hover:bg-brand-glow transition-colors">
              <Plus size={11}/> Add widget
            </button>
          )}
          <button onClick={() => setFullscreen(f => !f)}
            className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300 px-2 py-1.5 rounded-lg hover:bg-surface-2 transition-colors">
            {fullscreen ? <Minimize2 size={11}/> : <Maximize2 size={11}/>}
          </button>
        </div>
      </div>

      {/* Grid */}
      <div className="flex-1 overflow-y-auto p-4">
        {widgets.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 gap-3 text-center">
            <LayoutDashboard size={48} className="text-slate-700"/>
            <p className="text-sm text-slate-500">Empty dashboard</p>
            <p className="text-xs text-slate-700">Click <strong className="text-slate-500">Edit → Add widget</strong> to get started</p>
            <button onClick={() => setAddWidget(true)}
              className="mt-2 flex items-center gap-1.5 text-xs bg-brand text-white px-3 py-2 rounded-lg">
              <Plus size={12}/> Add first widget
            </button>
          </div>
        ) : (
          <div key={key} className="grid grid-cols-12 gap-3">
            {widgets.map((w: any) => (
              <div key={w.id} className={COL_SPAN[w.col ?? 6] ?? 'col-span-6'}>
                {editing ? (
                  <div className="relative group">
                    <LiveWidget widget={w} editable onEdit={() => setEditWidget(w)}/>
                    <button onClick={() => removeWidget(w.id)}
                      className="absolute top-2 right-2 opacity-0 group-hover:opacity-100 p-1 bg-crit/80 text-white rounded-md transition-all z-10">
                      <Trash2 size={11}/>
                    </button>
                  </div>
                ) : (
                  <LiveWidget widget={w}/>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Modals */}
      {(addWidget || editWidget) && (
        <WidgetEditor
          widget={editWidget ?? {}}
          onSave={saveWidget}
          onClose={() => { setAddWidget(false); setEditWidget(null) }}
        />
      )}
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
//  DASHBOARD CARD
// ─────────────────────────────────────────────────────────────────────────────

function DashCard({ dash, onOpen, onClone, onDelete, onExport }: {
  dash: any; onOpen:()=>void; onClone:()=>void; onDelete:()=>void; onExport:()=>void
}) {
  const wc = dash.widgets?.length ?? TEMPLATES[dash.name]?.widgets?.length ?? 0
  return (
    <div onClick={onOpen}
      className="bg-surface-1 border border-surface-3 rounded-xl p-4 hover:border-brand/40 cursor-pointer transition-all group">
      <div className="flex items-start justify-between mb-2">
        <div className="flex-1 min-w-0">
          <h3 className="text-sm font-medium text-slate-200 truncate">{dash.name}</h3>
          {dash.description && <p className="text-xs text-slate-500 mt-0.5 truncate">{dash.description}</p>}
        </div>
        {dash.is_template && (
          <span className="ml-2 text-[10px] bg-brand/15 text-brand px-1.5 py-0.5 rounded shrink-0">template</span>
        )}
      </div>

      <div className="flex items-center gap-3 text-xs text-slate-600 mb-3">
        <span className="flex items-center gap-1"><LayoutDashboard size={11}/>{wc} panels</span>
        {dash.updated_at && (
          <span className="flex items-center gap-1">
            <Clock size={11}/>{formatDistanceToNow(new Date(dash.updated_at), {addSuffix:true})}
          </span>
        )}
      </div>

      {dash.tags?.length > 0 && (
        <div className="flex gap-1 flex-wrap mb-3">
          {dash.tags.slice(0, 4).map((t: string) => (
            <span key={t} className="text-[10px] bg-surface-3 text-slate-500 px-1.5 py-0.5 rounded">{t}</span>
          ))}
        </div>
      )}

      <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity" onClick={e => e.stopPropagation()}>
        <Btn size="xs" onClick={onClone}><Copy size={11}/> Clone</Btn>
        <Btn size="xs" onClick={onExport}><Download size={11}/> Export</Btn>
        <Btn size="xs" variant="danger" onClick={onDelete}><Trash2 size={11}/></Btn>
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
//  NEW DASHBOARD MODAL
// ─────────────────────────────────────────────────────────────────────────────

function NewDashModal({ onSave, onClose, onImport }: {
  onSave: (d: any) => void; onClose: () => void; onImport: (d: any) => void
}) {
  const [name, setName]   = useState('')
  const [desc, setDesc]   = useState('')
  const [tags, setTags]   = useState('')
  const [tmpl, setTmpl]   = useState('')
  const [mode, setMode]   = useState<'new'|'import'>('new')
  const [json, setJson]   = useState('')
  const [jsonErr, setJsonErr] = useState('')

  const tryImport = () => {
    try {
      const d = JSON.parse(json)
      if (!d.name) throw new Error('Missing "name" field')
      onImport(d)
    } catch (e: any) {
      setJsonErr(e.message)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 z-50 flex items-center justify-center p-4">
      <div className="w-full max-w-md bg-surface-1 border border-surface-3 rounded-2xl overflow-hidden shadow-2xl animate-slide-up">
        <div className="flex items-center justify-between px-5 py-3.5 border-b border-surface-3">
          <div className="flex gap-4">
            <button onClick={() => setMode('new')}
              className={clsx('text-sm font-medium pb-0.5 border-b-2 transition-colors',
                mode === 'new' ? 'text-brand border-brand' : 'text-slate-500 border-transparent')}>
              New dashboard
            </button>
            <button onClick={() => setMode('import')}
              className={clsx('text-sm font-medium pb-0.5 border-b-2 transition-colors',
                mode === 'import' ? 'text-brand border-brand' : 'text-slate-500 border-transparent')}>
              Import JSON
            </button>
          </div>
          <button onClick={onClose}><X size={15} className="text-slate-500"/></button>
        </div>

        <div className="p-5">
          {mode === 'new' ? (
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Name *</label>
                <input value={name} onChange={e => setName(e.target.value)} autoFocus
                  placeholder="My dashboard"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60"/>
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Description</label>
                <input value={desc} onChange={e => setDesc(e.target.value)}
                  placeholder="Optional description"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60"/>
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Tags</label>
                <input value={tags} onChange={e => setTags(e.target.value)}
                  placeholder="slo, k8s, production (comma-separated)"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60"/>
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Start from template</label>
                <select value={tmpl} onChange={e => setTmpl(e.target.value)}
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                  <option value="">Blank dashboard</option>
                  {Object.entries(TEMPLATES).map(([k, v]) => (
                    <option key={k} value={k}>{k} — {v.description}</option>
                  ))}
                </select>
              </div>
            </div>
          ) : (
            <div>
              <label className="block text-xs text-slate-500 mb-1">Paste dashboard JSON</label>
              <textarea value={json} onChange={e => { setJson(e.target.value); setJsonErr('') }} rows={10}
                placeholder={'{\n  "name": "My dashboard",\n  "widgets": [...]\n}'}
                className="w-full px-3 py-2 text-xs font-mono bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none resize-none"/>
              {jsonErr && <div className="mt-1 text-xs text-crit">{jsonErr}</div>}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 px-5 py-3.5 border-t border-surface-3">
          <Btn onClick={onClose}>Cancel</Btn>
          {mode === 'new' ? (
            <Btn variant="primary" disabled={!name}
              onClick={() => onSave({
                name, description: desc,
                tags: tags ? tags.split(',').map(t => t.trim()).filter(Boolean) : [],
                widgets: tmpl ? (TEMPLATES[tmpl]?.widgets ?? []) : [],
                is_template: false,
              })}>
              <Check size={13}/> Create
            </Btn>
          ) : (
            <Btn variant="primary" disabled={!json} onClick={tryImport}>
              <Upload size={13}/> Import
            </Btn>
          )}
        </div>
      </div>
    </div>
  )
}

// ─────────────────────────────────────────────────────────────────────────────
//  MAIN PAGE
// ─────────────────────────────────────────────────────────────────────────────

const DEFAULT_DASHBOARDS = Object.entries(TEMPLATES).map(([name, t], i) => ({
  id: `default-${i}`,
  name,
  description: t.description,
  tags: t.tags,
  is_template: true,
  updated_at: new Date(Date.now() - (i+1) * 3600_000).toISOString(),
}))

export default function DashboardsPage() {
  const qc      = useQueryClient()
  const [showNew,   setShowNew]   = useState(false)
  const [viewing,   setViewing]   = useState<any | null>(null)
  const [tab,       setTab]       = useState<'mine'|'templates'>('mine')
  const [search,    setSearch]    = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['dashboards'],
    queryFn:  () => dashApi.list(),
    staleTime: 60_000,
  })

  const create = useMutation({
    mutationFn: (d: any) => dashApi.create(d),
    onSuccess: res => {
      toast.success('Dashboard created')
      qc.invalidateQueries({ queryKey: ['dashboards'] })
      setShowNew(false)
      setViewing(res)
    },
    onError: () => toast.error('Failed to create dashboard'),
  })

  const update = useMutation({
    mutationFn: (d: any) => dashApi.update ? dashApi.update(d.id, d) : Promise.resolve(d),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['dashboards'] }),
  })

  const clone = useMutation({
    mutationFn: (id: string) => dashApi.clone(id),
    onSuccess: () => { toast.success('Cloned'); qc.invalidateQueries({ queryKey: ['dashboards'] }) },
  })

  const del = useMutation({
    mutationFn: (id: string) => dashApi.delete(id),
    onSuccess: () => { toast.success('Deleted'); qc.invalidateQueries({ queryKey: ['dashboards'] }) },
  })

  const handleUpdate = useCallback((d: any) => {
    setViewing(d)
    update.mutate(d)
  }, [update])

  const exportDash = (d: any) => {
    const blob = new Blob([JSON.stringify(d, null, 2)], { type: 'application/json' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = `${d.name.replace(/\s+/g,'-').toLowerCase()}.json`
    a.click()
  }

  const apiList = data?.dashboards ?? []
  const allList = apiList.length > 0 ? apiList : DEFAULT_DASHBOARDS
  const list = allList
    .filter((d: any) => tab === 'templates' ? d.is_template : !d.is_template)
    .filter((d: any) => !search ||
      d.name.toLowerCase().includes(search.toLowerCase()) ||
      d.description?.toLowerCase().includes(search.toLowerCase()) ||
      d.tags?.some((t: string) => t.includes(search.toLowerCase())))

  if (viewing) {
    return (
      <div className="flex-1 flex flex-col overflow-hidden">
        <DashboardViewer
          dash={viewing}
          onClose={() => setViewing(null)}
          onUpdate={handleUpdate}
        />
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Dashboards"
        subtitle="Built-in · 12 live widget types · templates · import/export JSON · no Grafana required"
        actions={
          <div className="flex gap-2">
            <Btn onClick={() => setShowNew(true)}>
              <Upload size={13}/> Import
            </Btn>
            <Btn variant="primary" onClick={() => setShowNew(true)}>
              <Plus size={13}/> New dashboard
            </Btn>
          </div>
        }
      />

      {/* Tabs + search */}
      <div className="flex items-center gap-3 px-4 pt-3 border-b border-surface-3">
        <div className="flex">
          {([['mine','My dashboards'],['templates','Templates']] as const).map(([id,label]) => (
            <button key={id} onClick={() => setTab(id)}
              className={clsx('px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
                tab === id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
              {label} {id === 'templates' && <span className="ml-1 text-[10px] text-slate-700">({Object.keys(TEMPLATES).length})</span>}
            </button>
          ))}
        </div>
        <div className="ml-auto mb-2">
          <SearchInput value={search} onChange={setSearch} placeholder="Search dashboards…"/>
        </div>
      </div>

      {/* Dashboard grid */}
      <div className="flex-1 overflow-y-auto p-4">
        {isLoading && <div className="flex justify-center py-12"><Spinner size={20}/></div>}

        {!isLoading && !list.length && (
          <EmptyState
            icon={LayoutDashboard}
            title={tab === 'mine' ? 'No custom dashboards yet' : 'No templates match'}
            description={tab === 'mine'
              ? 'Create a dashboard from scratch or start from a template.'
              : 'Try a different search term.'}
          />
        )}

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {list.map((d: any) => (
            <DashCard
              key={d.id}
              dash={d}
              onOpen={() => setViewing(d)}
              onClone={() => clone.mutate(d.id)}
              onDelete={() => del.mutate(d.id)}
              onExport={() => exportDash(d)}
            />
          ))}
        </div>

        {/* Widget type reference */}
        <div className="mt-8 pt-6 border-t border-surface-3">
          <div className="text-xs font-medium text-slate-500 mb-3">Available widget types</div>
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2">
            {WIDGET_TYPES.map(wt => {
              const Icon = wt.icon
              return (
                <div key={wt.id} className="flex items-center gap-2 text-[11px] text-slate-500 bg-surface-1 border border-surface-3 rounded-lg px-2.5 py-2">
                  <Icon size={12} className="text-slate-600 shrink-0"/>
                  <span>{wt.label}</span>
                </div>
              )
            })}
          </div>
        </div>
      </div>

      {showNew && (
        <NewDashModal
          onSave={d => create.mutate(d)}
          onImport={d => create.mutate(d)}
          onClose={() => setShowNew(false)}
        />
      )}
    </div>
  )
}
