import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { traces } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import {
  ComposedChart, Bar, Line, XAxis, YAxis, Tooltip,
  ResponsiveContainer, Legend, CartesianGrid
} from 'recharts'
import {
  RefreshCw, Plus, ChevronDown, ChevronRight, Search,
  MoreHorizontal, Columns, SlidersHorizontal, BarChart2, Activity
} from 'lucide-react'
import { format } from 'date-fns'
import clsx from 'clsx'

// ── Facet definitions ─────────────────────────────────────────────────────────
const TRACE_FACET_GROUPS = [
  { id:'core', label:'Core', alwaysOpen:true, facets:[] }, // Duration slider handled separately
  { id:'request_status', label:'Request status', alwaysOpen:true, options:['Success','Failure'] },
  { id:'span_status', label:'Span status', options:['Ok','Error'] },
  { id:'span_kind', label:'Span kind', options:['client','server','consumer','producer','internal','link'] },
  { id:'http', label:'HTTP', options:[] },
  { id:'http_headers', label:'HTTP headers', options:[] },
  { id:'request_attrs', label:'Request attributes', options:[] },
  { id:'kubernetes', label:'Kubernetes', options:[] },
  { id:'k8s_ns_labels', label:'Kubernetes namespace labels', options:[] },
  { id:'process', label:'Process', options:[] },
  { id:'host', label:'Host', options:[] },
  { id:'deployment', label:'Deployment information', options:[] },
  { id:'databases', label:'Databases', options:[] },
  { id:'code_attrs', label:'Code attributes', options:[] },
  { id:'exceptions', label:'Exceptions', options:[] },
  { id:'messaging', label:'Messaging', options:[] },
  { id:'networking', label:'Networking', options:[] },
  { id:'rpc', label:'Remote procedure call', options:[] },
  { id:'metadata', label:'Metadata', options:[] },
]

const TIME_RANGES = [
  {label:'Last 15 minutes',ms:900_000},
  {label:'Last 30 minutes',ms:1_800_000},
  {label:'Last 1 hour',ms:3_600_000},
  {label:'Last 6 hours',ms:21_600_000},
  {label:'Last 24 hours',ms:86_400_000},
]

// ── Types ─────────────────────────────────────────────────────────────────────
interface TraceRow {
  traceID: string; rootSpan: string; rootService: string;
  durationMs: number; spanCount: number; errorCount: number;
  startTime: number; httpStatus?: number; k8sCluster?: string; k8sNamespace?: string
}

// ── Helpers ───────────────────────────────────────────────────────────────────
function parseTraces(raw: any[]): TraceRow[] {
  return (raw ?? []).map((t: any) => ({
    traceID:      t.traceID ?? t.trace_id ?? t.id ?? '',
    rootSpan:     t.rootSpanName ?? t.rootTraceName ?? t.root_operation ?? t.operationName ?? 'unknown',
    rootService:  t.rootServiceName ?? t.root_service ?? t.serviceName ?? 'unknown',
    durationMs:   Math.round((t.durationMs ?? (t.duration ?? 0) / 1_000_000) * 100) / 100,
    spanCount:    t.spanCount ?? t.span_count ?? 1,
    errorCount:   t.errorCount ?? t.error_count ?? 0,
    startTime:    t.startTimeUnixMs ?? (t.start_time ? parseInt(t.start_time) / 1_000_000 : Date.now()),
    httpStatus:   t.httpStatus ?? t.http_status ?? undefined,
    k8sCluster:   t.k8sCluster ?? t.k8s_cluster ?? undefined,
    k8sNamespace: t.k8sNamespace ?? t.k8s_namespace ?? undefined,
  }))
}

function buildChartData(rows: TraceRow[], timeMs: number): any[] {
  const buckets = 30, bucketMs = timeMs / buckets, now = Date.now()
  const data: any[] = Array.from({ length: buckets }, (_, i) => ({
    t: now - (buckets - i) * bucketMs,
    Requests: 0, Failures: 0, avgMs: 0, p50: 0, p90: 0, _durations: [] as number[]
  }))
  for (const row of rows) {
    const idx = Math.min(buckets - 1, Math.max(0, Math.floor((row.startTime - (now - timeMs)) / bucketMs)))
    data[idx].Requests++
    if (row.errorCount > 0) data[idx].Failures++
    data[idx]._durations.push(row.durationMs)
  }
  return data.map(d => {
    const durs = d._durations.sort((a: number, b: number) => a - b)
    const avg = durs.length ? durs.reduce((s: number, v: number) => s + v, 0) / durs.length : 0
    const p50 = durs[Math.floor(durs.length * 0.5)] ?? 0
    const p90 = durs[Math.floor(durs.length * 0.9)] ?? 0
    return { label: format(new Date(d.t), 'HH:mm'), Requests: d.Requests, Failures: d.Failures, avgMs: +avg.toFixed(1), p50: +p50.toFixed(1), p90: +p90.toFixed(1) }
  })
}

// ── Sub-components ────────────────────────────────────────────────────────────
function DurationSlider({ min, max, onMin, onMax }: { min: string; max: string; onMin: (v: string) => void; onMax: (v: string) => void }) {
  return (
    <div className="px-3 py-2 border-b border-[#1e2433]">
      <div className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider mb-2">Duration</div>
      <div className="flex items-center gap-2">
        <div className="flex-1">
          <label className="text-[10px] text-slate-500 block mb-1">Min</label>
          <input value={min} onChange={e => onMin(e.target.value)} className="w-full bg-[#111827] border border-[#1e2433] rounded px-2 py-1 text-[11px] text-slate-300 outline-none" placeholder="0ns" />
        </div>
        <div className="flex-1">
          <label className="text-[10px] text-slate-500 block mb-1">Max</label>
          <input value={max} onChange={e => onMax(e.target.value)} className="w-full bg-[#111827] border border-[#1e2433] rounded px-2 py-1 text-[11px] text-slate-300 outline-none" placeholder="1000s" />
        </div>
      </div>
    </div>
  )
}

function CheckboxFacet({ label, options, active, onToggle }: {
  label: string; options: string[]; active: Set<string>; onToggle: (v: string) => void
}) {
  const [open, setOpen] = useState(label === 'Request status' || label === 'Span status' || label === 'Span kind')
  if (options.length === 0) {
    return (
      <div className="border-b border-[#1e2433]">
        <button onClick={() => setOpen(o => !o)} className="w-full flex items-center justify-between px-3 py-2 hover:bg-[#141929]">
          <span className="text-[11px] font-medium text-slate-400">{label}</span>
          {open ? <ChevronDown size={12} className="text-slate-600" /> : <ChevronRight size={12} className="text-slate-600" />}
        </button>
      </div>
    )
  }
  return (
    <div className="border-b border-[#1e2433]">
      <button onClick={() => setOpen(o => !o)} className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-[#141929]">
        <span className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider">{label}</span>
        <div className="flex items-center gap-1">
          {active.size > 0 && <span className="w-1.5 h-1.5 rounded-full bg-indigo-400" />}
          {open ? <ChevronDown size={13} className="text-slate-500" /> : <ChevronRight size={13} className="text-slate-500" />}
        </div>
      </button>
      {open && (
        <div className="px-3 pb-2 space-y-1">
          {options.map(opt => {
            const isActive = active.has(opt)
            const isSuccess = opt === 'Success' || opt === 'Ok'
            const isError = opt === 'Failure' || opt === 'Error'
            return (
              <label key={opt} className="flex items-center gap-2 cursor-pointer hover:text-slate-200">
                <input type="checkbox" checked={isActive} onChange={() => onToggle(opt)}
                  className="w-3.5 h-3.5 rounded border-slate-600 bg-[#111827] checked:bg-indigo-600 outline-none cursor-pointer" />
                <div className="flex items-center gap-1.5">
                  {isSuccess && <span className="text-green-400">✓</span>}
                  {isError && <span className="text-red-400">✗</span>}
                  <span className={clsx('text-[12px]', isActive ? 'text-slate-200' : 'text-slate-400')}>{opt}</span>
                </div>
              </label>
            )
          })}
        </div>
      )}
    </div>
  )
}

function ServiceFacet({ services, suggestions, activeService, onSelect }: {
  services: string[]; suggestions: string[]; activeService: string; onSelect: (s: string) => void
}) {
  const [open, setOpen] = useState(true)
  const [search, setSearch] = useState('')
  const filtered = (services.length ? services : suggestions).filter(s => !search || s.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="border-b border-[#1e2433]">
      <button onClick={() => setOpen(o => !o)} className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-[#141929]">
        <span className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider">Service</span>
        <div className="flex items-center gap-1">
          {activeService && <span className="w-1.5 h-1.5 rounded-full bg-indigo-400" />}
          {open ? <ChevronDown size={13} className="text-slate-500" /> : <ChevronRight size={13} className="text-slate-500" />}
        </div>
      </button>
      {open && (
        <div className="px-3 pb-2">
          <div className="flex items-center gap-1.5 bg-[#111827] border border-[#1e2433] rounded px-2 py-1 mb-2">
            <Search size={11} className="text-slate-500" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search suggestions" className="bg-transparent text-[11px] text-slate-300 outline-none flex-1 placeholder-slate-600" />
          </div>
          <div className="space-y-0.5 max-h-36 overflow-y-auto">
            {filtered.slice(0, 10).map(svc => (
              <label key={svc} className="flex items-center gap-2 cursor-pointer hover:text-slate-200">
                <input type="checkbox" checked={activeService === svc} onChange={() => onSelect(activeService === svc ? '' : svc)}
                  className="w-3.5 h-3.5 rounded border-slate-600 bg-[#111827] cursor-pointer" />
                <span className={clsx('text-[11px] truncate', activeService === svc ? 'text-indigo-300' : 'text-slate-400')}>{svc}</span>
              </label>
            ))}
            {filtered.length > 10 && <button className="text-[10px] text-indigo-400 hover:text-indigo-300">More ({filtered.length - 10})</button>}
          </div>
        </div>
      )}
    </div>
  )
}

function EndpointFacet({ endpoints, activeEndpoint, onSelect }: { endpoints: string[]; activeEndpoint: string; onSelect: (e: string) => void }) {
  const [open, setOpen] = useState(true)
  const [search, setSearch] = useState('')
  const suggestions = endpoints.length ? endpoints : ['Static resources', 'ingress', 'GET', 'GET /*', 'imageprovider']
  const filtered = suggestions.filter(e => !search || e.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="border-b border-[#1e2433]">
      <button onClick={() => setOpen(o => !o)} className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-[#141929]">
        <span className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider">Endpoint</span>
        <div className="flex items-center gap-1">
          {activeEndpoint && <span className="w-1.5 h-1.5 rounded-full bg-indigo-400" />}
          {open ? <ChevronDown size={13} className="text-slate-500" /> : <ChevronRight size={13} className="text-slate-500" />}
        </div>
      </button>
      {open && (
        <div className="px-3 pb-2">
          <div className="flex items-center gap-1.5 bg-[#111827] border border-[#1e2433] rounded px-2 py-1 mb-2">
            <Search size={11} className="text-slate-500" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search suggestions" className="bg-transparent text-[11px] text-slate-300 outline-none flex-1 placeholder-slate-600" />
          </div>
          <div className="space-y-0.5 max-h-36 overflow-y-auto">
            {filtered.slice(0, 8).map(ep => (
              <label key={ep} className="flex items-center gap-2 cursor-pointer hover:text-slate-200">
                <input type="checkbox" checked={activeEndpoint === ep} onChange={() => onSelect(activeEndpoint === ep ? '' : ep)}
                  className="w-3.5 h-3.5 rounded border-slate-600 bg-[#111827] cursor-pointer" />
                <span className={clsx('text-[11px] truncate', activeEndpoint === ep ? 'text-indigo-300' : 'text-slate-400')}>{ep}</span>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

function RequestRow({ row }: { row: TraceRow }) {
  const isSuccess = row.errorCount === 0
  const ts = format(new Date(row.startTime), 'MMM d, HH:mm:ss')
  const ms = row.durationMs.toFixed(2)
  return (
    <tr className="border-b border-[#111827] hover:bg-[#0d1423]/60 cursor-pointer">
      <td className="px-3 py-2 text-[11px] text-slate-500 whitespace-nowrap font-mono">{ts}<span className="text-slate-700">.{String(row.startTime % 1000).padStart(3, '0')}</span></td>
      <td className="px-3 py-2 text-[12px] text-slate-300 max-w-[180px] truncate">{row.rootSpan}</td>
      <td className="px-3 py-2 text-[12px] text-indigo-400 whitespace-nowrap">{row.rootService}</td>
      <td className="px-3 py-2 text-[12px] text-slate-300 whitespace-nowrap text-right">{ms} ms</td>
      <td className="px-3 py-2">
        <span className={clsx('inline-flex items-center gap-1 text-[11px] font-semibold px-2 py-0.5 rounded-full',
          isSuccess ? 'bg-green-500/15 text-green-400' : 'bg-red-500/15 text-red-400')}>
          {isSuccess ? '✓' : '✗'} {isSuccess ? 'Success' : 'Failure'}
        </span>
      </td>
      <td className="px-3 py-2">
        {row.httpStatus && (
          <span className={clsx('inline-block text-[11px] font-bold px-1.5 py-0.5 rounded',
            row.httpStatus < 400 ? 'bg-green-500/20 text-green-400' : 'bg-red-500/20 text-red-400')}>
            {row.httpStatus}
          </span>
        )}
      </td>
      <td className="px-3 py-2 text-[11px] text-slate-500">{row.k8sCluster ?? '—'}</td>
      <td className="px-3 py-2 text-[11px] text-slate-500">{row.k8sNamespace ?? '—'}</td>
      <td className="px-3 py-2 text-[11px] text-slate-600"><MoreHorizontal size={13} /></td>
    </tr>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────
export default function TracesPage() {
  const [activeTab, setActiveTab] = useState<'requests' | 'spans'>('requests')
  const [timeMs, setTimeMs] = useState(1_800_000)
  const [chartView, setChartView] = useState<'timeseries' | 'histogram'>('timeseries')
  const [filterText, setFilterText] = useState('')
  const [minDuration, setMinDuration] = useState('0ns')
  const [maxDuration, setMaxDuration] = useState('1000s')
  const [activeService, setActiveService] = useState('')
  const [activeEndpoint, setActiveEndpoint] = useState('')
  const [reqStatus, setReqStatus] = useState<Set<string>>(new Set())
  const [spanStatus, setSpanStatus] = useState<Set<string>>(new Set())
  const [spanKind, setSpanKind] = useState<Set<string>>(new Set())
  const [tableSearch, setTableSearch] = useState('')

  const now = Math.floor(Date.now() / 1000)
  const start = now - Math.floor(timeMs / 1000)

  const { data: traceData, isFetching, refetch } = useQuery({
    queryKey: ['traces-search', timeMs, activeService],
    queryFn: () => traces.search({ limit: 500, start: `${start}000000000`, end: `${now}000000000`, service: activeService || undefined }),
    refetchInterval: 60_000,
  })

  const { data: serviceData } = useQuery({
    queryKey: ['trace-services'],
    queryFn: () => traces.services(),
  })

  const allRows = useMemo(() => parseTraces((traceData as any)?.traces ?? []), [traceData])
  const serviceList: string[] = (serviceData as any)?.services ?? []

  // Extract unique endpoints from traces
  const endpointList = useMemo(() => {
    const eps = new Set<string>()
    allRows.forEach(r => eps.add(r.rootSpan))
    return [...eps].slice(0, 20)
  }, [allRows])

  // Apply all filters
  const filteredRows = useMemo(() => {
    let rows = allRows
    if (activeService) rows = rows.filter(r => r.rootService === activeService)
    if (activeEndpoint) rows = rows.filter(r => r.rootSpan.includes(activeEndpoint))
    if (reqStatus.size > 0) rows = rows.filter(r => {
      const isSuccess = r.errorCount === 0
      return (reqStatus.has('Success') && isSuccess) || (reqStatus.has('Failure') && !isSuccess)
    })
    if (tableSearch) {
      const q = tableSearch.toLowerCase()
      rows = rows.filter(r => r.rootSpan.toLowerCase().includes(q) || r.rootService.toLowerCase().includes(q))
    }
    return rows
  }, [allRows, activeService, activeEndpoint, reqStatus, tableSearch])

  const chartData = useMemo(() => buildChartData(filteredRows, timeMs), [filteredRows, timeMs])

  const toggleSet = (set: Set<string>, val: string) => {
    const next = new Set(set); next.has(val) ? next.delete(val) : next.add(val); return next
  }

  return (
    <div className="flex h-full bg-[#070c18] text-slate-200">
      {/* ── Facet sidebar ─────────────────────────────────────────────── */}
      <aside className="w-64 shrink-0 border-r border-[#1e2433] flex flex-col overflow-hidden">
        {/* Requests / Spans tabs */}
        <div className="flex border-b border-[#1e2433]">
          <button
            onClick={() => setActiveTab('requests')}
            className={clsx('flex-1 py-3 text-[13px] font-semibold border-r border-[#1e2433] transition-colors',
              activeTab === 'requests' ? 'text-slate-100 border-b-2 border-b-indigo-500 bg-indigo-500/10' : 'text-slate-400 hover:text-slate-300')}
          >Requests</button>
          <button
            onClick={() => setActiveTab('spans')}
            className={clsx('flex-1 py-3 text-[13px] font-semibold transition-colors',
              activeTab === 'spans' ? 'text-slate-100 border-b-2 border-b-indigo-500 bg-indigo-500/10' : 'text-slate-400 hover:text-slate-300')}
          >Spans</button>
        </div>

        <div className="flex-1 overflow-y-auto">
          {/* Duration slider */}
          <DurationSlider min={minDuration} max={maxDuration} onMin={setMinDuration} onMax={setMaxDuration} />

          {/* Service facet */}
          <ServiceFacet services={serviceList} suggestions={['api-gateway', 'checkout-service', 'payment-service', 'ingress', 'frontend']} activeService={activeService} onSelect={setActiveService} />

          {/* Endpoint facet */}
          <EndpointFacet endpoints={endpointList} activeEndpoint={activeEndpoint} onSelect={setActiveEndpoint} />

          {/* Request status */}
          <CheckboxFacet label="Request status" options={['Success', 'Failure']} active={reqStatus} onToggle={v => setReqStatus(s => toggleSet(s, v))} />

          {/* Span status */}
          <CheckboxFacet label="Span status" options={['Ok', 'Error']} active={spanStatus} onToggle={v => setSpanStatus(s => toggleSet(s, v))} />

          {/* Span kind */}
          <CheckboxFacet label="Span kind" options={['client', 'server', 'consumer', 'producer', 'internal', 'link']} active={spanKind} onToggle={v => setSpanKind(s => toggleSet(s, v))} />

          {/* Collapsed groups */}
          {['HTTP', 'HTTP headers', 'Request attributes', 'Kubernetes', 'Kubernetes namespace labels',
            'Process', 'Host', 'Deployment information', 'Databases', 'Code attributes',
            'Exceptions', 'Messaging', 'Networking', 'Remote procedure call', 'Metadata'].map(label => (
            <CheckboxFacet key={label} label={label} options={[]} active={new Set()} onToggle={() => {}} />
          ))}
        </div>
      </aside>

      {/* ── Main content ──────────────────────────────────────────────── */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Toolbar */}
        <div className="flex items-center gap-2 px-3 py-2 border-b border-[#1e2433] bg-[#080d1b] shrink-0">
          {/* Filter bar */}
          <div className="flex-1 flex items-center gap-2 bg-[#111827] border border-[#1e2433] rounded-lg px-3 py-1.5">
            <SlidersHorizontal size={13} className="text-slate-500" />
            <input value={filterText} onChange={e => setFilterText(e.target.value)} placeholder="Type to filter" className="bg-transparent text-[12px] text-slate-300 placeholder-slate-600 outline-none flex-1" />
          </div>

          {/* Time range */}
          <select value={timeMs} onChange={e => setTimeMs(+e.target.value)} className="bg-[#111827] border border-[#1e2433] text-slate-300 text-[12px] rounded-lg px-2.5 py-1.5 outline-none">
            {TIME_RANGES.map(t => <option key={t.ms} value={t.ms}>{t.label}</option>)}
          </select>

          {/* Refresh */}
          <button onClick={() => refetch()} className="flex items-center gap-1.5 bg-[#111827] border border-[#1e2433] text-slate-300 text-[12px] rounded-lg px-3 py-1.5 hover:border-indigo-500/50 hover:text-slate-200">
            <RefreshCw size={12} className={isFetching ? 'animate-spin' : ''} /> Refresh
          </button>

          {/* Add traces */}
          <button className="flex items-center gap-1.5 bg-indigo-600 hover:bg-indigo-500 text-white text-[12px] font-semibold rounded-lg px-3 py-1.5 transition-colors">
            <Plus size={13} /> Add traces
          </button>
        </div>

        {/* Chart section */}
        <div className="shrink-0 border-b border-[#1e2433] bg-[#080d1b] px-3 pt-3 pb-2">
          <div className="flex items-center justify-between mb-2">
            <div className="flex items-center gap-1">
              <span className="text-[15px] font-bold text-slate-200 mr-3">Requests</span>
              <button onClick={() => setChartView('timeseries')} className={clsx('flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[12px] font-medium transition-colors', chartView === 'timeseries' ? 'bg-indigo-500/20 text-indigo-300 border border-indigo-500/40' : 'text-slate-400 hover:text-slate-200 border border-transparent')}>
                <Activity size={13} /> Timeseries
              </button>
              <button onClick={() => setChartView('histogram')} className={clsx('flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[12px] font-medium transition-colors', chartView === 'histogram' ? 'bg-indigo-500/20 text-indigo-300 border border-indigo-500/40' : 'text-slate-400 hover:text-slate-200 border border-transparent')}>
                <BarChart2 size={13} /> Histogram
              </button>
            </div>
            <ChevronDown size={14} className="text-slate-500" />
          </div>

          <ResponsiveContainer width="100%" height={130}>
            <ComposedChart data={chartData} margin={{ top: 0, right: 40, left: -10, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#1e2433" />
              <XAxis dataKey="label" tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false} interval={5} />
              <YAxis yAxisId="left" tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false} />
              <YAxis yAxisId="right" orientation="right" tick={{ fontSize: 10, fill: '#6b7280' }} tickLine={false} axisLine={false} unit="ms" />
              <Tooltip contentStyle={{ background: '#111827', border: '1px solid #1e2433', fontSize: 11, borderRadius: 8 }} />
              <Legend iconSize={8} wrapperStyle={{ fontSize: 11, paddingTop: 4 }} />
              <Bar yAxisId="left" dataKey="Requests" fill="#1e3a5f" opacity={0.8} />
              <Bar yAxisId="left" dataKey="Failures" fill="#7f1d1d" opacity={0.8} />
              <Line yAxisId="right" type="monotone" dataKey="avgMs" stroke="#3b82f6" dot={false} strokeWidth={2} name="Average" />
              <Line yAxisId="right" type="monotone" dataKey="p50" stroke="#8b5cf6" dot={false} strokeWidth={1.5} name="50th ...tile" strokeDasharray="4 2" />
              <Line yAxisId="right" type="monotone" dataKey="p90" stroke="#ec4899" dot={false} strokeWidth={1.5} name="90th ...tile" strokeDasharray="4 2" />
            </ComposedChart>
          </ResponsiveContainer>
        </div>

        {/* Table toolbar */}
        <div className="flex items-center gap-2 px-3 py-2 border-b border-[#1e2433] bg-[#080d1b] shrink-0">
          <div className="flex items-center gap-2 bg-[#111827] border border-[#1e2433] rounded-lg px-2.5 py-1.5 w-48">
            <Search size={12} className="text-slate-500" />
            <input value={tableSearch} onChange={e => setTableSearch(e.target.value)} placeholder="Search requests" className="bg-transparent text-[11px] text-slate-300 outline-none flex-1 placeholder-slate-600" />
          </div>
          <span className="text-[12px] text-slate-400 bg-[#111827] border border-[#1e2433] rounded-lg px-3 py-1.5">
            {filteredRows.length.toLocaleString()} req…
          </span>
          <div className="flex-1" />
          <button className="flex items-center gap-1.5 text-[12px] text-slate-400 border border-[#1e2433] rounded-lg px-2.5 py-1.5 hover:border-indigo-500/40 hover:text-slate-300">
            Group by <ChevronDown size={11} />
          </button>
          <button className="flex items-center gap-1.5 text-[12px] text-slate-400 border border-[#1e2433] rounded-lg px-2.5 py-1.5 hover:border-indigo-500/40 hover:text-slate-300">
            <Columns size={12} /> 559 columns hidden
          </button>
        </div>

        {/* Request table */}
        <div className="flex-1 overflow-auto">
          {isFetching ? (
            <div className="flex items-center justify-center h-32"><Spinner /></div>
          ) : (
            <table className="w-full text-left">
              <thead className="sticky top-0 bg-[#080d1b] z-10">
                <tr className="border-b border-[#1e2433]">
                  {['Start time ↓', 'Endpoint', 'Service', 'Duration', 'Request status', 'HTTP...', 'Process...', 'Kube...', 'Kube...'].map(col => (
                    <th key={col} className="px-3 py-2 text-[11px] font-semibold text-slate-400 whitespace-nowrap">{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filteredRows.length === 0 ? (
                  <tr><td colSpan={9} className="text-center py-12 text-slate-500">No traces found</td></tr>
                ) : (
                  filteredRows.slice(0, 200).map(row => <RequestRow key={row.traceID} row={row} />)
                )}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  )
}
