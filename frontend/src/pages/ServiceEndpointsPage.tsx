// ServiceEndpointsPage.tsx — Full endpoint-level metrics with request sampling
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apm } from '@/lib/api'
import { Activity, Clock, AlertTriangle, Filter, ChevronDown } from 'lucide-react'

const DEMO_ENDPOINTS = [
  { id:'ep1', service:'checkout-service', method:'POST', path:'/api/v2/checkout/initiate', rps:284, p50:142, p90:412, p99:892, err:3.2, status:'warning', calls:12840, uptime:99.12, avgPayload:'4.2KB' },
  { id:'ep2', service:'checkout-service', method:'GET',  path:'/api/v2/cart/:id', rps:481, p50:48, p90:120, p99:184, err:0.3, status:'healthy', calls:21645, uptime:99.97, avgPayload:'1.8KB' },
  { id:'ep3', service:'user-service',     method:'GET',  path:'/api/v1/users/:id', rps:1240, p50:2800, p90:3800, p99:4200, err:8.4, status:'critical', calls:55800, uptime:91.2, avgPayload:'2.1KB' },
  { id:'ep4', service:'user-service',     method:'POST', path:'/api/v1/users/login', rps:342, p50:112, p90:280, p99:480, err:0.8, status:'healthy', calls:15390, uptime:99.91, avgPayload:'0.8KB' },
  { id:'ep5', service:'user-service',     method:'PUT',  path:'/api/v1/users/:id/profile', rps:84, p50:180, p90:340, p99:680, err:1.2, status:'healthy', calls:3780, uptime:99.8, avgPayload:'3.4KB' },
  { id:'ep6', service:'payment-service',  method:'POST', path:'/api/v3/payments/charge', rps:78, p50:89, p90:210, p99:340, err:0.08, status:'healthy', calls:3510, uptime:99.98, avgPayload:'2.8KB' },
  { id:'ep7', service:'payment-service',  method:'GET',  path:'/api/v3/payments/:id/status', rps:156, p50:24, p90:68, p99:98, err:0.02, status:'healthy', calls:7020, uptime:100, avgPayload:'0.4KB' },
  { id:'ep8', service:'api-gateway',      method:'POST', path:'/api/*', rps:2140, p50:184, p90:620, p99:1284, err:1.84, status:'warning', calls:96300, uptime:99.2, avgPayload:'3.1KB' },
  { id:'ep9', service:'api-gateway',      method:'GET',  path:'/health', rps:120, p50:2, p90:6, p99:12, err:0, status:'healthy', calls:5400, uptime:100, avgPayload:'0.1KB' },
  { id:'ep10', service:'ml-inference',    method:'POST', path:'/v1/recommend', rps:48, p50:840, p90:2100, p99:3200, err:1.2, status:'warning', calls:2160, uptime:97.8, avgPayload:'8.4KB' },
  { id:'ep11', service:'notification-svc',method:'POST', path:'/api/v1/notify/email', rps:84, p50:48, p90:120, p99:280, err:0.4, status:'healthy', calls:3780, uptime:99.9, avgPayload:'12KB' },
  { id:'ep12', service:'notification-svc',method:'POST', path:'/api/v1/notify/slack', rps:24, p50:180, p90:480, p99:840, err:2.1, status:'warning', calls:1080, uptime:99.1, avgPayload:'4KB' },
]

const SERVICES = ['All Services',...new Set(DEMO_ENDPOINTS.map(e=>e.service))]
const METHODS = ['All Methods','GET','POST','PUT','DELETE']
const METHOD_C: Record<string,string> = { GET:'#0fcf8a', POST:'#6c72ff', PUT:'#f5a623', DELETE:'#ff4d6a', PATCH:'#a855f7' }
const STATUS_C: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }

function LatencyBar({ p50, p90, p99, max }: { p50:number; p90:number; p99:number; max:number }) {
  const pct = (v:number) => `${Math.min((v/max)*100,100)}%`
  return (
    <div style={{position:'relative',height:8,background:'#0b1628',borderRadius:4,overflow:'hidden',width:120}}>
      <div style={{position:'absolute',height:'100%',left:0,width:pct(p99),background:'rgba(255,77,106,.4)'}}/>
      <div style={{position:'absolute',height:'100%',left:0,width:pct(p90),background:'rgba(245,166,35,.5)'}}/>
      <div style={{position:'absolute',height:'100%',left:0,width:pct(p50),background:'#6c72ff'}}/>
    </div>
  )
}

export default function ServiceEndpointsPage() {
  const [svc, setSvc]     = useState('All Services')
  const [method, setMethod] = useState('All Methods')
  const [sort, setSort]   = useState<'err'|'p99'|'rps'|'calls'>('err')
  const [selected, setSelected] = useState<typeof DEMO_ENDPOINTS[0]|null>(null)

  const { data } = useQuery({
    queryKey: ['endpoints', svc],
    queryFn: async () => { return { endpoints: DEMO_ENDPOINTS } },
    staleTime: 15_000,
  })

  const endpoints: typeof DEMO_ENDPOINTS = (data as any)?.endpoints ?? DEMO_ENDPOINTS
  const filtered = endpoints
    .filter(e => (svc==='All Services'||e.service===svc) && (method==='All Methods'||e.method===method))
    .sort((a,b) => b[sort] - a[sort])

  const maxP99 = Math.max(...filtered.map(e=>e.p99), 1)
  const critCount = filtered.filter(e=>e.status==='critical').length
  const warnCount = filtered.filter(e=>e.status==='warning').length

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'14px 20px 10px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Service Endpoints</div>
          <div style={{fontSize:12,color:'#546a88',flex:1}}>Entry-point metrics per route · Latency distribution · Error isolation · Request sampling</div>
        </div>
        <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:8}}>
          <select value={svc} onChange={e=>setSvc(e.target.value)} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',borderRadius:7,padding:'5px 10px',fontSize:11}}>
            {SERVICES.map(s=><option key={s}>{s}</option>)}
          </select>
          <select value={method} onChange={e=>setMethod(e.target.value)} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',borderRadius:7,padding:'5px 10px',fontSize:11}}>
            {METHODS.map(m=><option key={m}>{m}</option>)}
          </select>
          <div style={{marginLeft:'auto',display:'flex',gap:5}}>
            {(['err','p99','rps','calls'] as const).map(s=>(
              <button key={s} onClick={()=>setSort(s)} style={{padding:'4px 10px',border:`1px solid ${sort===s?'#6c72ff':'#1a2d4a'}`,borderRadius:7,background:sort===s?'rgba(108,114,255,.15)':'#0b1628',color:sort===s?'#8b90ff':'#546a88',fontSize:10,cursor:'pointer',fontWeight:600}}>
                {s==='err'?'Error %':s==='p99'?'P99':s.toUpperCase()}
              </button>
            ))}
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Endpoints',v:filtered.length,c:'#6c72ff'},{l:'Critical',v:critCount,c:critCount>0?'#ff4d6a':'#0fcf8a'},{l:'Warning',v:warnCount,c:warnCount>0?'#f5a623':'#0fcf8a'},{l:'Total RPS',v:filtered.reduce((a,e)=>a+e.rps,0).toLocaleString(),c:'#8fa8cc'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 12px'}}>
              <div style={{fontSize:8,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{flex:1,overflowY:'auto'}}>
          <table style={{width:'100%',borderCollapse:'collapse',fontSize:12}}>
            <thead style={{position:'sticky',top:0,background:'#080d1b',zIndex:1}}>
              <tr style={{borderBottom:'2px solid #1a2d4a'}}>
                {['Method','Endpoint','Service','RPS','Latency (P50/P90/P99)','Error %','Calls/h','Uptime','Status'].map(h=>(
                  <th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase',whiteSpace:'nowrap'}}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.map(ep=>(
                <tr key={ep.id} onClick={()=>setSelected(ep===selected?null:ep)}
                  style={{borderBottom:'1px solid rgba(26,45,74,.35)',cursor:'pointer',background:selected?.id===ep.id?'rgba(108,114,255,.05)':'transparent'}}>
                  <td style={{padding:'9px 12px'}}>
                    <span style={{background:`${METHOD_C[ep.method]||'#546a88'}22`,color:METHOD_C[ep.method]||'#546a88',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:5,fontFamily:'JetBrains Mono,monospace'}}>{ep.method}</span>
                  </td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#c8d8ef'}}>{ep.path}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#6c72ff'}}>{ep.service}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{ep.rps}/s</td>
                  <td style={{padding:'9px 12px'}}>
                    <div style={{display:'flex',gap:8,alignItems:'center'}}>
                      <LatencyBar p50={ep.p50} p90={ep.p90} p99={ep.p99} max={maxP99}/>
                      <span style={{fontSize:9,color:'#546a88',whiteSpace:'nowrap'}}>{ep.p50}/{ep.p90}/<span style={{color:ep.p99>1000?'#ff4d6a':ep.p99>500?'#f5a623':'#8fa8cc',fontWeight:700}}>{ep.p99}</span>ms</span>
                    </div>
                  </td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',color:ep.err>5?'#ff4d6a':ep.err>1?'#f5a623':'#0fcf8a',fontWeight:700}}>{ep.err}%</td>
                  <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{ep.calls.toLocaleString()}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:ep.uptime<99?'#f5a623':'#0fcf8a'}}>{ep.uptime}%</td>
                  <td style={{padding:'9px 12px'}}>
                    <div style={{display:'flex',gap:5,alignItems:'center'}}>
                      <div style={{width:6,height:6,borderRadius:'50%',background:STATUS_C[ep.status]}}/>
                      <span style={{color:STATUS_C[ep.status],fontSize:10,fontWeight:600}}>{ep.status}</span>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {/* Detail panel */}
          {selected && (
            <div style={{margin:14,background:'rgba(17,31,53,.7)',border:`1px solid ${STATUS_C[selected.status]}44`,borderRadius:12,padding:16}}>
              <div style={{display:'flex',gap:10,marginBottom:12,alignItems:'center'}}>
                <span style={{background:`${METHOD_C[selected.method]}22`,color:METHOD_C[selected.method],fontSize:11,fontWeight:800,padding:'3px 10px',borderRadius:7,fontFamily:'JetBrains Mono,monospace'}}>{selected.method}</span>
                <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:13,color:'#f0f6ff',fontWeight:700}}>{selected.path}</span>
                <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace',fontSize:11}}>{selected.service}</span>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(6,1fr)',gap:8}}>
                {[{l:'P50',v:`${selected.p50}ms`,c:'#0fcf8a'},{l:'P90',v:`${selected.p90}ms`,c:'#f5a623'},{l:'P99',v:`${selected.p99}ms`,c:selected.p99>1000?'#ff4d6a':'#f5a623'},{l:'Error Rate',v:`${selected.err}%`,c:selected.err>5?'#ff4d6a':'#0fcf8a'},{l:'Calls/h',v:selected.calls.toLocaleString(),c:'#8fa8cc'},{l:'Avg Payload',v:selected.avgPayload,c:'#8fa8cc'}].map(k=>(
                  <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'8px 10px'}}>
                    <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:14,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
