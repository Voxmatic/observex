import { useState, useCallback, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { logs } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Legend } from 'recharts'
import { Play, ChevronRight, ChevronDown, Search, EyeOff, Eye, Filter, MoreHorizontal } from 'lucide-react'
import { format } from 'date-fns'
import clsx from 'clsx'

const FACET_GROUPS = [
  { id:'core', label:'Core', facets:[{key:'service',label:'Service'},{key:'app',label:'Application'},{key:'env',label:'Environment'}] },
  { id:'status', label:'Status', facets:[{key:'level',label:'Log level'},{key:'severity',label:'Severity'}] },
  { id:'log_source', label:'Log source', facets:[{key:'source',label:'Source'},{key:'filename',label:'File name'}] },
  { id:'k8s', label:'K8s', facets:[
    {key:'k8s.cluster.name',label:'k8s.cluster.name'},
    {key:'k8s.node.name',label:'k8s.node.name'},
    {key:'k8s.namespace.name',label:'k8s.namespace.name'},
    {key:'k8s.workload.kind',label:'k8s.workload.kind'},
    {key:'k8s.workload.name',label:'k8s.workload.name'},
    {key:'k8s.pod.name',label:'k8s.pod.name'},
    {key:'k8s.container.name',label:'k8s.container.name'},
  ]},
  { id:'process', label:'Process', facets:[{key:'process.name',label:'process.name'},{key:'process.pid',label:'process.pid'}] },
  { id:'host', label:'Host', facets:[{key:'host.name',label:'host.name'},{key:'host.ip',label:'host.ip'}] },
  { id:'cloud', label:'Cloud', facets:[{key:'cloud.provider',label:'cloud.provider'},{key:'cloud.region',label:'cloud.region'}] },
  { id:'aws', label:'AWS', facets:[{key:'aws.region',label:'aws.region'},{key:'aws.account.id',label:'aws.account.id'},{key:'aws.availability_zone',label:'aws.availability_zone'}] },
  { id:'azure', label:'Azure', facets:[{key:'azure.region',label:'azure.region'},{key:'azure.resource.group',label:'azure.resource.group'}] },
  { id:'gcp', label:'GCP', facets:[
    {key:'gcp.instance.name',label:'gcp.instance.name'},
    {key:'gcp.project.id',label:'gcp.project.id'},
    {key:'gcp.region',label:'gcp.region'},
    {key:'gcp.resource.type',label:'gcp.resource.type'},
  ]},
]

const TIME_RANGES = [
  {label:'Last 15 minutes',ms:900_000},
  {label:'Last 30 minutes',ms:1_800_000},
  {label:'Last 1 hour',ms:3_600_000},
  {label:'Last 6 hours',ms:21_600_000},
  {label:'Last 24 hours',ms:86_400_000},
]

const LEVEL_COLORS:Record<string,string> = {info:'#3b82f6',warn:'#f59e0b',warning:'#f59e0b',error:'#ef4444',debug:'#6b7280',none:'#374151'}
const CHART_COLORS:Record<string,string> = {INFO:'#3b82f6',WARN:'#f59e0b',ERROR:'#ef4444',NONE:'#374151'}

interface LogEntry {ts:string;level:string;line:string;labels:Record<string,string>}

function parseStreams(streams:any[]):LogEntry[]{
  const entries:LogEntry[]=[]
  for(const s of streams){
    const labels=s.stream??{}
    for(const [tsNs,line] of (s.values??[])){
      const lvl=labels.level??(line.match(/"level"\s*:\s*"(\w+)"/)?.[1]??'none')
      entries.push({ts:tsNs,level:lvl.toLowerCase(),line,labels})
    }
  }
  return entries.sort((a,b)=>b.ts.localeCompare(a.ts))
}

function buildTimeseries(entries:LogEntry[],timeMs:number):any[]{
  const buckets=30,bucketMs=timeMs/buckets,now=Date.now()
  const data:any[]=Array.from({length:buckets},(_,i)=>({t:now-(buckets-i)*bucketMs,INFO:0,WARN:0,ERROR:0,NONE:0}))
  for(const e of entries){
    const tsMs=parseInt(e.ts)/1_000_000
    const idx=Math.min(buckets-1,Math.max(0,Math.floor((tsMs-(now-timeMs))/bucketMs)))
    const lvl=e.level==='warning'?'WARN':e.level==='error'?'ERROR':e.level==='info'?'INFO':'NONE'
    data[idx][lvl]++
  }
  return data.map(d=>({...d,label:format(new Date(d.t),'HH:mm')}))
}

function buildFacetValues(entries:LogEntry[],facetKey:string):Map<string,number>{
  const counts=new Map<string,number>()
  const simpleKey=facetKey.includes('.')?facetKey.split('.').pop()!:facetKey
  for(const e of entries){
    const val=e.labels[facetKey]??e.labels[simpleKey]??''
    if(val)counts.set(val,(counts.get(val)??0)+1)
  }
  return new Map([...counts.entries()].sort((a,b)=>b[1]-a[1]))
}

function FacetGroup({group,entries,activeFilters,onFilter,facetSearch}:{
  group:typeof FACET_GROUPS[0];entries:LogEntry[];activeFilters:Map<string,string>;onFilter:(k:string,v:string)=>void;facetSearch:string
}){
  const [open,setOpen]=useState(['k8s','core','status'].includes(group.id))
  const [expandedFacets,setExpandedFacets]=useState<Set<string>>(new Set())
  const matching=group.facets.filter(f=>!facetSearch||f.label.toLowerCase().includes(facetSearch.toLowerCase()))
  if(matching.length===0)return null
  return(
    <div className="border-b border-[#1e2433]">
      <button onClick={()=>setOpen(o=>!o)} className="w-full flex items-center justify-between px-3 py-2.5 hover:bg-[#141929] transition-colors">
        <span className="text-[11px] font-semibold text-slate-300 uppercase tracking-wider">{group.label}</span>
        <div className="flex items-center gap-1">
          {group.facets.some(f=>activeFilters.has(f.key))&&<span className="w-1.5 h-1.5 rounded-full bg-indigo-400"/>}
          {open?<ChevronDown size={13} className="text-slate-500"/>:<ChevronRight size={13} className="text-slate-500"/>}
        </div>
      </button>
      {open&&<div className="pb-1">
        {matching.map(facet=>{
          const values=buildFacetValues(entries,facet.key)
          const isExpanded=expandedFacets.has(facet.key)
          const activeFacet=activeFilters.get(facet.key)
          const visible=isExpanded?[...values.entries()]:[...values.entries()].slice(0,5)
          const maxCount=Math.max(...[...values.values(),1])
          return(
            <div key={facet.key} className="px-3 py-1">
              <div className="flex items-center justify-between">
                <span className="text-[11px] text-slate-400 font-medium">{facet.label}</span>
                <div className="flex items-center gap-1">
                  {activeFacet&&<span className="text-[9px] text-indigo-400 font-bold">ACTIVE</span>}
                  <MoreHorizontal size={11} className="text-slate-600"/>
                </div>
              </div>
              {values.size>0&&<div className="mt-1 space-y-0.5">
                {visible.map(([val,count])=>{
                  const isActive=activeFacet===val
                  const pct=Math.min(100,Math.round(count/maxCount*100))
                  return(
                    <div key={val} onClick={()=>onFilter(facet.key,val)} className={clsx('flex items-center gap-2 px-1.5 py-0.5 rounded cursor-pointer group',isActive?'bg-indigo-500/20':'hover:bg-[#141929]')}>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                          <span className={clsx('text-[11px] truncate',isActive?'text-indigo-300 font-medium':'text-slate-400 group-hover:text-slate-300')}>{val}</span>
                          <span className="text-[10px] text-slate-600 ml-1">{count}</span>
                        </div>
                        <div className="mt-0.5 h-0.5 bg-[#1e2433] rounded">
                          <div className={clsx('h-full rounded',isActive?'bg-indigo-500':'bg-slate-600')} style={{width:`${pct}%`}}/>
                        </div>
                      </div>
                    </div>
                  )
                })}
                {values.size>5&&<button onClick={()=>setExpandedFacets(s=>{const n=new Set(s);n.has(facet.key)?n.delete(facet.key):n.add(facet.key);return n})} className="text-[10px] text-indigo-400 hover:text-indigo-300 px-1.5 py-0.5">{isExpanded?'Show less':`+${values.size-5} more`}</button>}
              </div>}
            </div>
          )
        })}
      </div>}
    </div>
  )
}

function LogRow({entry}:{entry:LogEntry}){
  const [open,setOpen]=useState(false)
  const color=LEVEL_COLORS[entry.level]||'#6b7280'
  const ts=parseInt(entry.ts)/1_000_000
  let parsed:any=null;try{parsed=JSON.parse(entry.line)}catch{}
  return(
    <div className="border-b border-[#111827] cursor-pointer hover:bg-[#0d1423]/60" onClick={()=>setOpen(o=>!o)}>
      <div className="flex items-start gap-3 px-3 py-1.5 font-mono text-[12px]">
        <span className="text-slate-600 shrink-0 w-20 text-right">{format(new Date(ts),'HH:mm:ss.SSS')}</span>
        <span className="shrink-0 w-10 font-semibold uppercase text-[10px]" style={{color}}>{entry.level}</span>
        <span className="text-slate-300 truncate flex-1 leading-5">{parsed?.message??parsed?.msg??entry.line}</span>
        {entry.labels.service&&<span className="text-indigo-400 text-[10px] shrink-0">{entry.labels.service}</span>}
      </div>
      {open&&<div className="px-4 pb-3 bg-[#080e1a]">
        <div className="grid grid-cols-2 gap-x-8 gap-y-1 text-[11px] mb-2">
          {Object.entries(entry.labels).map(([k,v])=>(<div key={k} className="flex gap-2"><span className="text-slate-500 font-medium shrink-0">{k}</span><span className="text-slate-300 truncate">{v}</span></div>))}
        </div>
        <pre className="text-[11px] text-slate-400 whitespace-pre-wrap break-all bg-[#0d1423] rounded p-2 mt-2">{parsed?JSON.stringify(parsed,null,2):entry.line}</pre>
      </div>}
    </div>
  )
}

export default function LogsPage(){
  const [query,setQuery]=useState('')
  const [filterText,setFilterText]=useState('')
  const [submitted,setSubmitted]=useState('{level!=""}')
  const [timeMs,setTimeMs]=useState(1_800_000)
  const [showChart,setShowChart]=useState(true)
  const [facetSearch,setFacetSearch]=useState('')
  const [activeFilters,setActiveFilters]=useState<Map<string,string>>(new Map())
  const [hasRun,setHasRun]=useState(false)
  const now=Math.floor(Date.now()/1000)
  const start=now-Math.floor(timeMs/1000)
  const {data:streamData,isFetching,refetch}=useQuery({
    queryKey:['logs-range',submitted,timeMs],
    queryFn:()=>logs.queryRange(submitted,start,now,2000),
    enabled:hasRun,
  })
  const allEntries=useMemo(()=>{
    if(!streamData)return[]
    return parseStreams(Array.isArray(streamData)?streamData:(streamData as any).result??[])
  },[streamData])
  const entries=useMemo(()=>{
    if(activeFilters.size===0)return allEntries
    return allEntries.filter(e=>{
      for(const [key,val] of activeFilters){
        const sk=key.includes('.')?key.split('.').pop()!:key
        const ev=e.labels[key]??e.labels[sk]??''
        if(ev!==val)return false
      }
      return true
    })
  },[allEntries,activeFilters])
  const chartData=useMemo(()=>buildTimeseries(entries,timeMs),[entries,timeMs])
  const visibleEntries=useMemo(()=>{
    if(!filterText)return entries
    const q=filterText.toLowerCase()
    return entries.filter(e=>e.line.toLowerCase().includes(q)||Object.values(e.labels).some(v=>v.toLowerCase().includes(q)))
  },[entries,filterText])
  const handleFilter=useCallback((key:string,val:string)=>{
    setActiveFilters(prev=>{const next=new Map(prev);next.get(key)===val?next.delete(key):next.set(key,val);return next})
  },[])
  const handleRun=()=>{
    let q=query||'{level!=""}'
    if(activeFilters.size>0){
      const sel=[...activeFilters.entries()].map(([k,v])=>{const sk=k.includes('.')?k.split('.').pop()!:k;return`${sk}="${v}"`}).join(',')
      q=`{${sel}}`
    }
    setSubmitted(q);setHasRun(true)
  }
  return(
    <div className="flex h-full bg-[#070c18] text-slate-200">
      {/* Facet sidebar */}
      <aside className="w-64 shrink-0 border-r border-[#1e2433] flex flex-col overflow-hidden">
        <div className="p-2 border-b border-[#1e2433]">
          <div className="flex items-center gap-2 bg-[#111827] border border-[#1e2433] rounded-lg px-2.5 py-1.5">
            <Search size={13} className="text-slate-500"/>
            <input value={facetSearch} onChange={e=>setFacetSearch(e.target.value)} placeholder="Search facets" className="bg-transparent text-[12px] text-slate-300 placeholder-slate-600 outline-none flex-1"/>
          </div>
        </div>
        <div className="flex-1 overflow-y-auto">
          {FACET_GROUPS.map(group=><FacetGroup key={group.id} group={group} entries={allEntries} activeFilters={activeFilters} onFilter={handleFilter} facetSearch={facetSearch}/>)}
        </div>
      </aside>
      {/* Main */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Toolbar */}
        <div className="flex items-center gap-2 px-3 py-2 border-b border-[#1e2433] bg-[#080d1b] shrink-0">
          <select value={timeMs} onChange={e=>setTimeMs(+e.target.value)} className="bg-[#111827] border border-[#1e2433] text-slate-300 text-[12px] rounded-lg px-2.5 py-1.5 outline-none">
            {TIME_RANGES.map(t=><option key={t.ms} value={t.ms}>{t.label}</option>)}
          </select>
          <div className="flex-1"/>
          <button onClick={()=>setShowChart(s=>!s)} className="flex items-center gap-1.5 text-[12px] text-slate-400 hover:text-slate-200 px-2 py-1.5 rounded-lg hover:bg-[#111827]">
            {showChart?<EyeOff size={13}/>:<Eye size={13}/>} {showChart?'Hide chart':'Show chart'}
          </button>
          <button onClick={handleRun} disabled={isFetching} className="flex items-center gap-2 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-60 text-white text-[12px] font-semibold px-4 py-1.5 rounded-lg transition-colors">
            {isFetching?<Spinner size={12}/>:<Play size={13}/>} Run query
          </button>
        </div>
        {/* Filter bar */}
        <div className="flex items-center gap-2 px-3 py-2 border-b border-[#1e2433] bg-[#080d1b] shrink-0 flex-wrap">
          <Filter size={13} className="text-slate-500 shrink-0"/>
          <input value={filterText} onChange={e=>setFilterText(e.target.value)} placeholder="Type to filter" className="flex-1 min-w-[120px] bg-transparent text-[13px] text-slate-300 placeholder-slate-600 outline-none"/>
          {[...activeFilters.entries()].map(([k,v])=>(
            <button key={k} onClick={()=>handleFilter(k,v)} className="flex items-center gap-1 bg-indigo-500/20 border border-indigo-500/40 text-indigo-300 text-[11px] px-2 py-0.5 rounded-full hover:bg-indigo-500/30">
              <span>{k.split('.').pop()}={v}</span><span className="text-indigo-400 font-bold">×</span>
            </button>
          ))}
        </div>
        {/* Timeseries chart */}
        {showChart&&<div className="shrink-0 border-b border-[#1e2433] bg-[#080d1b] px-3 pt-3 pb-1">
          <div className="flex items-center justify-between mb-1">
            <span className="text-[12px] font-semibold text-slate-300">Timeseries</span>
          </div>
          <ResponsiveContainer width="100%" height={110}>
            <BarChart data={chartData} margin={{top:0,right:8,left:-20,bottom:0}} barCategoryGap="2%">
              <XAxis dataKey="label" tick={{fontSize:10,fill:'#6b7280'}} tickLine={false} axisLine={false} interval={5}/>
              <YAxis tick={{fontSize:10,fill:'#6b7280'}} tickLine={false} axisLine={false}/>
              <Tooltip contentStyle={{background:'#111827',border:'1px solid #1e2433',fontSize:11,borderRadius:8}} labelStyle={{color:'#9ca3af'}}/>
              <Legend iconSize={8} wrapperStyle={{fontSize:11,paddingTop:4}} iconType="circle"/>
              {(['INFO','WARN','ERROR','NONE'] as const).map(lvl=><Bar key={lvl} dataKey={lvl} stackId="a" fill={CHART_COLORS[lvl]}/>)}
            </BarChart>
          </ResponsiveContainer>
        </div>}
        {/* Query input */}
        <div className="shrink-0 px-3 py-2 border-b border-[#1e2433] bg-[#080d1b]">
          <input value={query} onChange={e=>setQuery(e.target.value)} onKeyDown={e=>e.key==='Enter'&&handleRun()} placeholder='{service="api-gateway"} | level = "error"' className="w-full bg-[#111827] border border-[#1e2433] rounded-lg px-3 py-1.5 text-[12px] font-mono text-slate-300 placeholder-slate-600 outline-none focus:border-indigo-500/50"/>
        </div>
        {/* Results */}
        <div className="flex-1 overflow-y-auto">
          {!hasRun?(
            <div className="flex flex-col items-center justify-center h-full text-center p-8">
              <div className="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center mb-4"><Play size={24} className="text-indigo-400"/></div>
              <p className="text-slate-300 font-medium text-sm mb-1">Run query to fetch logs</p>
              <p className="text-slate-500 text-xs mb-5">Set your preferred timeframe and filters, and run the query.</p>
              <button onClick={handleRun} className="flex items-center gap-2 bg-indigo-600 hover:bg-indigo-500 text-white text-[12px] font-semibold px-5 py-2 rounded-lg"><Play size={13}/> Run query</button>
            </div>
          ):isFetching?(
            <div className="flex items-center justify-center h-32"><Spinner/></div>
          ):(
            <>
              <div className="flex items-center justify-between px-3 py-1.5 bg-[#080d1b] border-b border-[#111827] sticky top-0 z-10">
                <span className="text-[11px] text-slate-500">{visibleEntries.length.toLocaleString()} results{activeFilters.size>0?` (filtered from ${allEntries.length.toLocaleString()})`:''}</span>
                <span className="text-[11px] text-slate-500">Open with</span>
              </div>
              {visibleEntries.length===0?<div className="text-center py-12 text-slate-500 text-sm">No log lines matched</div>:visibleEntries.slice(0,1000).map((e,i)=><LogRow key={`${e.ts}-${i}`} entry={e}/>)}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
