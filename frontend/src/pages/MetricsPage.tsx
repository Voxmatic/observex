// MetricsPage.tsx — Metrics explorer: browse, query, chart any metric
import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { metrics } from '@/lib/api'
import { Search, Play, Plus, Star, Clock, BarChart3, TrendingUp, Activity } from 'lucide-react'

const METRIC_CATALOG = [
  { name:'http.response_time.p99',    desc:'HTTP P99 response time',     unit:'ms',   tags:['apm','latency'],     category:'APM' },
  { name:'http.error_rate',           desc:'HTTP error rate percentage',  unit:'%',    tags:['apm','errors'],      category:'APM' },
  { name:'http.requests_per_second',  desc:'Requests per second',        unit:'rps',  tags:['apm','throughput'],  category:'APM' },
  { name:'system.cpu.usage',          desc:'Host CPU utilization',        unit:'%',    tags:['host','cpu'],        category:'Infrastructure' },
  { name:'system.memory.usage',       desc:'Host memory utilization',     unit:'%',    tags:['host','memory'],     category:'Infrastructure' },
  { name:'process.memory.rss',        desc:'Process RSS memory',          unit:'MB',   tags:['process','memory'],  category:'Infrastructure' },
  { name:'db.connection_pool.used',   desc:'DB pool connections active',  unit:'count',tags:['database','pool'],   category:'Database' },
  { name:'db.query_time.p95',         desc:'DB query P95 latency',        unit:'ms',   tags:['database','latency'],category:'Database' },
  { name:'k8s.pod.cpu.usage',         desc:'Kubernetes pod CPU usage',    unit:'cores',tags:['k8s','cpu'],         category:'Kubernetes' },
  { name:'k8s.pod.memory.usage',      desc:'Kubernetes pod memory',       unit:'Mi',   tags:['k8s','memory'],      category:'Kubernetes' },
  { name:'llm.calls.total',           desc:'Total LLM API calls',         unit:'count',tags:['ai','llm'],          category:'AI' },
  { name:'llm.tokens.total',          desc:'Total tokens consumed',       unit:'tokens',tags:['ai','cost'],        category:'AI' },
  { name:'slo.compliance.rate',       desc:'SLO compliance percentage',   unit:'%',    tags:['slo','reliability'], category:'Reliability' },
  { name:'chaos.experiment.failures', desc:'Chaos experiment failures',   unit:'count',tags:['chaos','testing'],   category:'Testing' },
]

const SAVED_QUERIES = [
  { name:'API Error Rate by Service',    query:'rate(http_requests_total{status=~"5.."} [5m])', pinned:true },
  { name:'Top P99 Latency Services',     query:'topk(10, http_response_time_p99)',              pinned:true },
  { name:'K8s Pod Memory Pressure',      query:'container_memory_usage_bytes > 400Mi',          pinned:false },
  { name:'DB Connection Pool Usage',     query:'db_connection_pool_used / db_connection_pool_max', pinned:false },
]

// Generate realistic metric data
const genSeries = (base: number, variance: number, n = 60) =>
  Array.from({length:n},(_,i) => ({ t:i, v: Math.max(0, base + (Math.sin(i/8)*variance) + (Math.random()-0.5)*variance*0.5) }))

function LineChart({ data, color, h=80 }: { data:{t:number,v:number}[], color:string, h?:number }) {
  if (!data.length) return null
  const max = Math.max(...data.map(d=>d.v)), min = Math.min(...data.map(d=>d.v))
  const w = 480
  const pts = data.map((d,i) => `${(i/(data.length-1))*w},${h - ((d.v-min)/(max-min||1))*(h-8)-4}`).join(' ')
  const area = `0,${h} ${pts} ${w},${h}`
  return (
    <svg width={w} height={h} style={{overflow:'hidden'}}>
      <defs><linearGradient id="cg" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor={color} stopOpacity="0.25"/><stop offset="100%" stopColor={color} stopOpacity="0"/>
      </linearGradient></defs>
      <polygon points={area} fill="url(#cg)"/>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round"/>
      {/* Last value dot */}
      {data.length>0 && <circle cx={(data.length-1)/(data.length-1)*w} cy={h-((data[data.length-1].v-min)/(max-min||1))*(h-8)-4} r="3" fill={color}/>}
    </svg>
  )
}

export default function MetricsPage() {
  const [search, setSearch] = useState('')
  const [selectedMetric, setSelectedMetric] = useState(METRIC_CATALOG[0])
  const [category, setCategory] = useState('All')
  const [oqlQuery, setOqlQuery] = useState('fetch metric: "http.response_time.p99", filter: service == "api-gateway", rollup: 1m, last: 1h')
  const [chartData, setChartData] = useState(genSeries(280, 120))
  const [lastValue, setLastValue] = useState(284)

  useEffect(() => {
    const t = setInterval(() => {
      setChartData(d => {
        const next = [...d.slice(1), { t: d[d.length-1].t+1, v: Math.max(0, d[d.length-1].v + (Math.random()-0.48)*30) }]
        setLastValue(Math.round(next[next.length-1].v))
        return next
      })
    }, 2000)
    return () => clearInterval(t)
  }, [])

  const categories = ['All', ...Array.from(new Set(METRIC_CATALOG.map(m=>m.category)))]
  const filtered = METRIC_CATALOG.filter(m =>
    (category === 'All' || m.category === category) &&
    (!search || m.name.includes(search) || m.desc.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div style={{display:'flex',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      {/* Left: metric catalog */}
      <div style={{width:280,borderRight:'1px solid #1a2d4a',display:'flex',flexDirection:'column',flexShrink:0}}>
        <div style={{padding:'12px 12px 8px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
          <div style={{fontSize:13,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:8}}>Metric Catalog</div>
          <div style={{display:'flex',alignItems:'center',gap:6,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:7,padding:'5px 9px',marginBottom:8}}>
            <Search size={11} style={{color:'#546a88'}}/>
            <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search metrics..."
              style={{background:'none',border:'none',outline:'none',fontSize:11,color:'#c8d8ef',flex:1}}/>
          </div>
          <div style={{display:'flex',gap:4,flexWrap:'wrap'}}>
            {categories.map(c=>(
              <button key={c} onClick={()=>setCategory(c)} style={{padding:'3px 8px',border:`1px solid ${category===c?'#6c72ff':'#1a2d4a'}`,borderRadius:8,background:category===c?'rgba(108,114,255,.15)':'transparent',color:category===c?'#8b90ff':'#3d5070',fontSize:9,fontWeight:600,cursor:'pointer'}}>{c}</button>
            ))}
          </div>
        </div>
        <div style={{flex:1,overflowY:'auto'}}>
          {filtered.map(m=>(
            <div key={m.name} onClick={()=>{setSelectedMetric(m);setChartData(genSeries(Math.random()*500+50, 80))}}
              style={{padding:'9px 12px',cursor:'pointer',borderBottom:'1px solid rgba(26,45,74,.3)',borderLeft:`3px solid ${selectedMetric.name===m.name?'#6c72ff':'transparent'}`,background:selectedMetric.name===m.name?'rgba(108,114,255,.06)':'transparent'}}>
              <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace',marginBottom:3}}>{m.name}</div>
              <div style={{fontSize:10,color:'#546a88',marginBottom:4}}>{m.desc}</div>
              <div style={{display:'flex',gap:4}}>
                {m.tags.map(t=><span key={t} style={{background:'#182844',color:'#546a88',fontSize:8,padding:'1px 5px',borderRadius:8}}>{t}</span>)}
                <span style={{marginLeft:'auto',background:'rgba(108,114,255,.15)',color:'#8b90ff',fontSize:8,padding:'1px 5px',borderRadius:8}}>{m.unit}</span>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Right: explorer + chart */}
      <div style={{flex:1,display:'flex',flexDirection:'column',overflow:'hidden'}}>
        {/* OQL query bar */}
        <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
          <div style={{fontSize:11,color:'#546a88',marginBottom:5}}>OQL Query <span style={{background:'rgba(108,114,255,.15)',color:'#8b90ff',fontSize:9,padding:'1px 6px',borderRadius:8,marginLeft:4}}>★ UNIQUE</span></div>
          <div style={{display:'flex',gap:8}}>
            <div style={{flex:1,background:'#080d1b',border:'1px solid #254060',borderRadius:8,padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8b90ff',cursor:'text'}}>
              {oqlQuery}
            </div>
            <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'8px 14px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer',display:'flex',gap:5,alignItems:'center'}}>
              <Play size={12}/>Run
            </button>
          </div>
        </div>

        {/* Saved queries */}
        <div style={{padding:'10px 16px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
          <div style={{fontSize:10,color:'#546a88',marginBottom:6}}>SAVED QUERIES</div>
          <div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
            {SAVED_QUERIES.map(q=>(
              <button key={q.name} onClick={()=>setOqlQuery(q.query)}
                style={{display:'flex',gap:4,alignItems:'center',padding:'4px 10px',background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,color:'#8fa8cc',fontSize:10,cursor:'pointer'}}>
                {q.pinned&&<Star size={9} style={{color:'#f5a623',fill:'#f5a623'}}/>}{q.name}
              </button>
            ))}
          </div>
        </div>

        {/* Chart */}
        <div style={{flex:1,overflowY:'auto',padding:16}}>
          {/* Selected metric info */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:16,marginBottom:14}}>
            <div style={{display:'flex',alignItems:'flex-start',gap:12,marginBottom:14}}>
              <div style={{flex:1}}>
                <div style={{fontSize:15,fontWeight:800,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace',marginBottom:3}}>{selectedMetric.name}</div>
                <div style={{fontSize:12,color:'#546a88'}}>{selectedMetric.desc} · unit: {selectedMetric.unit} · {selectedMetric.category}</div>
              </div>
              <div style={{textAlign:'right'}}>
                <div style={{fontSize:28,fontWeight:900,color:'#8b90ff',fontFamily:'Syne,sans-serif'}}>{lastValue}<span style={{fontSize:13,fontWeight:400}}> {selectedMetric.unit}</span></div>
                <div style={{fontSize:10,color:'#546a88'}}>Current value</div>
              </div>
            </div>
            <div style={{background:'#080d1b',borderRadius:10,padding:'12px 8px',overflow:'hidden'}}>
              <LineChart data={chartData} color="#6c72ff" h={100}/>
            </div>
            <div style={{display:'flex',justifyContent:'space-between',fontSize:9,color:'#3d5070',marginTop:4,padding:'0 8px'}}>
              <span>60min ago</span><span>45min</span><span>30min</span><span>15min</span><span>Now</span>
            </div>
          </div>

          {/* Stats */}
          <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10,marginBottom:14}}>
            {[{l:'Min',v:Math.round(Math.min(...chartData.map(d=>d.v))),c:'#0fcf8a'},{l:'Max',v:Math.round(Math.max(...chartData.map(d=>d.v))),c:'#ff4d6a'},{l:'Avg',v:Math.round(chartData.reduce((a,d)=>a+d.v,0)/chartData.length),c:'#8b90ff'},{l:'P95',v:Math.round([...chartData].sort((a,b)=>a.v-b.v)[Math.floor(chartData.length*0.95)]?.v||0),c:'#f5a623'}].map(k=>(
              <div key={k.l} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:'10px 14px'}}>
                <div style={{fontSize:9,color:'#546a88'}}>{k.l} ({selectedMetric.unit})</div>
                <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
              </div>
            ))}
          </div>

          {/* Related metrics */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Related Metrics</div>
            <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
              {METRIC_CATALOG.filter(m=>m.category===selectedMetric.category&&m.name!==selectedMetric.name).slice(0,5).map(m=>(
                <button key={m.name} onClick={()=>{setSelectedMetric(m);setChartData(genSeries(100+Math.random()*400,60))}}
                  style={{padding:'5px 12px',background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',borderRadius:8,fontSize:10,cursor:'pointer',fontFamily:'JetBrains Mono,monospace'}}>{m.name}</button>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
