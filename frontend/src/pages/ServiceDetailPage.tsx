// ServiceDetailPage.tsx — /services/:id — Full service deep dive
// Equivalent to: Dynatrace Service Flow, New Relic APM Summary, Datadog Service Page
// Golden signals · Dependency map · Top endpoints · Recent traces · Error groups · Deployments
import { useState, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { apm } from '@/lib/api'
import { TimeRangePicker } from '@/components/shared/TimeRangePicker'
import { SparkLine } from '@/components/shared/SparkLine'
import { MetricCard } from '@/components/shared/MetricCard'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ArrowLeft, Activity, Clock, AlertTriangle, GitBranch, TrendingUp, Layers, Bug, Zap, Server, ExternalLink, ChevronRight } from 'lucide-react'

// Demo data per service
const SERVICES: Record<string,any> = {
  'checkout-service': { name:'checkout-service', status:'warning', type:'HTTP', language:'Go', runtime:'go1.22', version:'v2.5.0', team:'Checkout Squad', owner:'sara.chen', namespace:'production', cluster:'prod-us-east', replicas:3, cpu:62, mem:74, rps:284, p50:142, p90:412, p99:892, errRate:3.2, apdex:0.84, uptime:99.12,
    throughputTrend:[240,260,280,310,284,290,260,280,300,284,270,290,310,284,275,290],
    latencyTrend:[120,135,150,180,142,155,130,148,170,142,138,155,175,142,140,150],
    errorTrend:[1.2,1.8,2.4,3.8,3.2,2.8,2.0,3.0,3.5,3.2,2.6,2.9,3.4,3.2,2.5,3.0],
    dependencies:[{name:'payment-service',type:'downstream',rps:78,p99:340,errRate:0.08},{name:'user-service',type:'downstream',rps:180,p99:280,errRate:8.4},{name:'postgres-primary',type:'database',rps:420,p99:24,errRate:0},{name:'redis-cache',type:'cache',rps:840,p99:4,errRate:0},{name:'kafka',type:'queue',rps:284,p99:12,errRate:0}],
    callers:[{name:'api-gateway',rps:284,p99:892}],
    endpoints:[{method:'POST',path:'/api/v2/checkout/initiate',rps:98,p50:180,p99:840,err:4.2,calls:4410},{method:'GET',path:'/api/v2/cart/:id',rps:120,p50:48,p99:184,err:0.3,calls:5400},{method:'POST',path:'/api/v2/checkout/confirm',rps:42,p50:220,p99:1200,err:2.1,calls:1890},{method:'DELETE',path:'/api/v2/cart/:id/items/:itemId',rps:24,p50:35,p99:120,err:0.1,calls:1080}],
    recentErrors:[{title:'TimeoutError: DB connection timeout after 30000ms',count:847,firstSeen:'23m ago'},{title:'PaymentGatewayError: upstream timeout',count:124,firstSeen:'1h ago'}],
    recentTraces:[{traceId:'abc-123-def',operation:'POST /checkout/initiate',duration:'842ms',status:'error',spans:14,timestamp:'19:47:23'},{traceId:'ghi-456-jkl',operation:'GET /cart/8420',duration:'48ms',status:'ok',spans:6,timestamp:'19:47:18'},{traceId:'mno-789-pqr',operation:'POST /checkout/confirm',duration:'1.2s',status:'slow',spans:18,timestamp:'19:46:55'}],
    deployments:[{version:'v2.5.0',time:'19:30',status:'warning',errDelta:'+2800%'},{version:'v2.4.9',time:'2d ago',status:'healthy',errDelta:'+0%'}],
  },
  'user-service': { name:'user-service', status:'critical', type:'HTTP', language:'Java', runtime:'JDK 21', version:'v2.4.0', team:'User Platform', owner:'jin.park', namespace:'production', cluster:'prod-us-east', replicas:4, cpu:48, mem:89, rps:1240, p50:2800, p90:3800, p99:4200, errRate:8.4, apdex:0.42, uptime:91.2,
    throughputTrend:[1100,1200,1150,1240,1180,1240,1200,1180,1240,1220,1180,1240,1200,1240,1220,1240],
    latencyTrend:[800,1200,1800,2800,3200,2800,2400,2800,3400,2800,2600,2800,3200,2800,2400,2800],
    errorTrend:[0.8,1.2,2.4,8.4,7.2,8.4,6.8,8.4,9.2,8.4,7.6,8.4,8.8,8.4,7.2,8.4],
    dependencies:[{name:'postgres-primary',type:'database',rps:1860,p99:84,errRate:0.4},{name:'redis-cache',type:'cache',rps:2480,p99:3,errRate:0}],
    callers:[{name:'api-gateway',rps:1240,p99:4200},{name:'checkout-service',rps:180,p99:280}],
    endpoints:[{method:'GET',path:'/api/v1/users/:id',rps:620,p50:2800,p99:4200,err:12.4,calls:27900},{method:'POST',path:'/api/v1/users/login',rps:342,p50:112,p99:480,err:0.8,calls:15390},{method:'PUT',path:'/api/v1/users/:id/profile',rps:84,p50:180,p99:680,err:1.2,calls:3780},{method:'GET',path:'/api/v1/users/:id/preferences',rps:194,p50:3200,p99:4800,err:14.2,calls:8730}],
    recentErrors:[{title:'NullPointerException in UserService.getProfile()',count:1842,firstSeen:'2h ago'},{title:'ConnectionPool exhausted: max pool size 20',count:484,firstSeen:'45m ago'}],
    recentTraces:[{traceId:'usr-001',operation:'GET /users/8420',duration:'4.2s',status:'error',spans:8,timestamp:'19:47:12'},{traceId:'usr-002',operation:'POST /users/login',duration:'112ms',status:'ok',spans:4,timestamp:'19:47:10'}],
    deployments:[{version:'v2.4.0',time:'3d ago',status:'healthy',errDelta:'+0%'}],
  },
}

const MC: Record<string,string> = { GET:'#0fcf8a', POST:'#6c72ff', PUT:'#f5a623', DELETE:'#ff4d6a', PATCH:'#a855f7' }
const SC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a', ok:'#0fcf8a', error:'#ff4d6a', slow:'#f5a623' }
const DC: Record<string,string> = { downstream:'#6c72ff', database:'#a855f7', cache:'#0fcf8a', queue:'#f5a623', upstream:'#8fa8cc' }

export default function ServiceDetailPage() {
  const { id } = useParams<{id:string}>()
  const [timeRange, setTimeRange] = useState('1h')
  const [tab, setTab] = useState<'overview'|'endpoints'|'traces'|'errors'|'dependencies'|'deployments'>('overview')

  const svc = SERVICES[id||''] || SERVICES['checkout-service']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      {/* Header */}
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
          <Link to="/apm" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <div style={{width:8,height:8,borderRadius:'50%',background:SC[svc.status]}}/>
          <div style={{flex:1}}>
            <div style={{display:'flex',alignItems:'baseline',gap:8}}>
              <span style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{svc.name}</span>
              <span style={{fontSize:10,fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{svc.version}</span>
              <StatusBadge status={svc.status}/>
            </div>
            <div style={{display:'flex',gap:12,fontSize:10,color:'#546a88',marginTop:2}}>
              <span>{svc.language} · {svc.runtime}</span>
              <span>Team: {svc.team}</span>
              <span>Owner: {svc.owner}</span>
              <span>{svc.namespace}/{svc.cluster}</span>
              <span>{svc.replicas} replicas</span>
            </div>
          </div>
          <TimeRangePicker value={timeRange} onChange={setTimeRange}/>
        </div>

        {/* Golden signals bar */}
        <div style={{display:'flex',gap:8}}>
          <MetricCard label="Throughput" value={svc.rps} unit="req/s" color="#6c72ff" trend={svc.throughputTrend} small/>
          <MetricCard label="P50 Latency" value={svc.p50} unit="ms" color={svc.p50>500?'#f5a623':'#0fcf8a'} small/>
          <MetricCard label="P99 Latency" value={svc.p99} unit="ms" color={svc.p99>1000?'#ff4d6a':'#f5a623'} trend={svc.latencyTrend} small/>
          <MetricCard label="Error Rate" value={`${svc.errRate}%`} color={svc.errRate>5?'#ff4d6a':svc.errRate>1?'#f5a623':'#0fcf8a'} trend={svc.errorTrend} small change={svc.errRate>2?'↑':'→'} changeColor={svc.errRate>2?'#ff4d6a':'#0fcf8a'}/>
          <MetricCard label="Apdex" value={svc.apdex} color={svc.apdex>0.9?'#0fcf8a':svc.apdex>0.7?'#f5a623':'#ff4d6a'} small/>
          <MetricCard label="Uptime" value={`${svc.uptime}%`} color={svc.uptime>99?'#0fcf8a':'#f5a623'} small/>
          <MetricCard label="CPU" value={`${svc.cpu}%`} color={svc.cpu>70?'#f5a623':'#8fa8cc'} small/>
          <MetricCard label="Memory" value={`${svc.mem}%`} color={svc.mem>85?'#ff4d6a':'#8fa8cc'} small/>
        </div>

        {/* Tabs */}
        <div style={{display:'flex',gap:0,marginTop:8}}>
          {(['overview','endpoints','traces','errors','dependencies','deployments'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)}
              style={{padding:'6px 16px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>
              {t}{t==='errors'?` (${svc.recentErrors.length})`:t==='endpoints'?` (${svc.endpoints.length})`:''}
            </button>
          ))}
        </div>
      </div>

      {/* Content */}
      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='overview'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
            {/* Dependency map */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Service dependency map</div>
              {/* Callers → This Service → Dependencies */}
              <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:16}}>
                {/* Callers */}
                <div style={{display:'flex',flexDirection:'column',gap:6,flex:1}}>
                  <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase',marginBottom:4}}>Callers (upstream)</div>
                  {svc.callers.map((c:any)=>(
                    <Link key={c.name} to={`/services/${c.name}`} style={{textDecoration:'none',background:'rgba(143,168,204,.08)',border:'1px solid rgba(143,168,204,.15)',borderRadius:8,padding:'6px 10px'}}>
                      <div style={{fontSize:11,fontWeight:600,color:'#f0f6ff'}}>{c.name}</div>
                      <div style={{fontSize:9,color:'#546a88'}}>{c.rps}/s · P99: {c.p99}ms</div>
                    </Link>
                  ))}
                </div>
                <div style={{color:'#3d5070',fontSize:18}}>→</div>
                {/* This service */}
                <div style={{background:`${SC[svc.status]}12`,border:`1px solid ${SC[svc.status]}44`,borderRadius:10,padding:'12px 16px',minWidth:120,textAlign:'center'}}>
                  <div style={{width:10,height:10,borderRadius:'50%',background:SC[svc.status],margin:'0 auto 6px'}}/>
                  <div style={{fontSize:12,fontWeight:800,color:'#f0f6ff'}}>{svc.name}</div>
                  <div style={{fontSize:10,color:SC[svc.status]}}>{svc.rps}/s</div>
                </div>
                <div style={{color:'#3d5070',fontSize:18}}>→</div>
                {/* Dependencies */}
                <div style={{display:'flex',flexDirection:'column',gap:6,flex:1}}>
                  <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase',marginBottom:4}}>Dependencies</div>
                  {svc.dependencies.slice(0,4).map((d:any)=>(
                    <div key={d.name} style={{background:`${DC[d.type]}08`,border:`1px solid ${DC[d.type]}22`,borderRadius:8,padding:'6px 10px'}}>
                      <div style={{display:'flex',gap:6,alignItems:'center'}}>
                        <span style={{fontSize:11,fontWeight:600,color:'#f0f6ff',flex:1}}>{d.name}</span>
                        <span style={{fontSize:8,color:DC[d.type],fontWeight:700,background:`${DC[d.type]}22`,padding:'1px 5px',borderRadius:6}}>{d.type}</span>
                      </div>
                      <div style={{fontSize:9,color:'#546a88'}}>{d.rps}/s · P99: {d.p99}ms{d.errRate>0?` · err: ${d.errRate}%`:''}</div>
                    </div>
                  ))}
                </div>
              </div>
            </div>

            {/* Recent errors */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{display:'flex',justifyContent:'space-between',marginBottom:12}}>
                <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Recent errors</span>
                <button onClick={()=>setTab('errors')} style={{fontSize:10,color:'#6c72ff',background:'none',border:'none',cursor:'pointer'}}>View all →</button>
              </div>
              {svc.recentErrors.map((e:any,i:number)=>(
                <div key={i} style={{padding:'8px 0',borderBottom:i<svc.recentErrors.length-1?'1px solid rgba(26,45,74,.4)':'none'}}>
                  <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#ff7a92',marginBottom:3,lineHeight:1.4}}>{e.title}</div>
                  <div style={{fontSize:9,color:'#546a88'}}>{e.count.toLocaleString()} occurrences · first seen {e.firstSeen}</div>
                </div>
              ))}
            </div>

            {/* Top endpoints */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{display:'flex',justifyContent:'space-between',marginBottom:12}}>
                <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Top endpoints by error rate</span>
                <button onClick={()=>setTab('endpoints')} style={{fontSize:10,color:'#6c72ff',background:'none',border:'none',cursor:'pointer'}}>View all →</button>
              </div>
              {[...svc.endpoints].sort((a:any,b:any)=>b.err-a.err).slice(0,4).map((ep:any,i:number)=>(
                <div key={i} style={{display:'flex',gap:8,alignItems:'center',padding:'6px 0',borderBottom:i<3?'1px solid rgba(26,45,74,.3)':'none'}}>
                  <span style={{fontSize:9,fontWeight:800,color:MC[ep.method],fontFamily:'JetBrains Mono,monospace',width:32}}>{ep.method}</span>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#c8d8ef',flex:1}}>{ep.path}</span>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:ep.err>5?'#ff4d6a':ep.err>1?'#f5a623':'#0fcf8a',fontWeight:700}}>{ep.err}%</span>
                  <span style={{fontSize:9,color:'#546a88'}}>{ep.rps}/s</span>
                </div>
              ))}
            </div>

            {/* Recent traces */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{display:'flex',justifyContent:'space-between',marginBottom:12}}>
                <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Recent traces</span>
                <button onClick={()=>setTab('traces')} style={{fontSize:10,color:'#6c72ff',background:'none',border:'none',cursor:'pointer'}}>View all →</button>
              </div>
              {svc.recentTraces.map((t:any,i:number)=>(
                <Link key={i} to={`/traces/${t.traceId}`} style={{display:'flex',gap:8,alignItems:'center',padding:'6px 0',borderBottom:i<svc.recentTraces.length-1?'1px solid rgba(26,45,74,.3)':'none',textDecoration:'none'}}>
                  <div style={{width:6,height:6,borderRadius:'50%',background:SC[t.status]||'#546a88'}}/>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc',flex:1}}>{t.operation}</span>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:SC[t.status],fontWeight:700}}>{t.duration}</span>
                  <span style={{fontSize:9,color:'#546a88'}}>{t.spans} spans</span>
                  <span style={{fontSize:9,color:'#3d5070'}}>{t.timestamp}</span>
                </Link>
              ))}
            </div>
          </div>
        )}

        {tab==='endpoints'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead><tr style={{background:'#080d1b'}}>
                {['Method','Endpoint','RPS','P50','P99','Error %','Calls/h'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
              </tr></thead>
              <tbody>
                {svc.endpoints.map((ep:any,i:number)=>(
                  <tr key={i} style={{borderTop:'1px solid rgba(26,45,74,.3)'}}>
                    <td style={{padding:'9px 12px'}}><span style={{color:MC[ep.method],fontFamily:'JetBrains Mono,monospace',fontSize:10,fontWeight:800}}>{ep.method}</span></td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#c8d8ef'}}>{ep.path}</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{ep.rps}/s</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{ep.p50}ms</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:ep.p99>1000?'#ff4d6a':'#f5a623',fontWeight:700}}>{ep.p99}ms</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:ep.err>5?'#ff4d6a':ep.err>1?'#f5a623':'#0fcf8a',fontWeight:700}}>{ep.err}%</td>
                    <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{ep.calls.toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {tab==='traces'&&(
          <div style={{display:'flex',flexDirection:'column',gap:6}}>
            {svc.recentTraces.map((t:any,i:number)=>(
              <Link key={i} to={`/traces/${t.traceId}`} style={{display:'flex',gap:12,alignItems:'center',background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:'10px 14px',textDecoration:'none'}}>
                <div style={{width:8,height:8,borderRadius:'50%',background:SC[t.status]||'#546a88'}}/>
                <div style={{flex:1}}>
                  <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#f0f6ff',marginBottom:2}}>{t.operation}</div>
                  <div style={{fontSize:9,color:'#546a88'}}>Trace: {t.traceId} · {t.spans} spans · {t.timestamp}</div>
                </div>
                <StatusBadge status={t.status} size="xs"/>
                <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:13,color:SC[t.status],fontWeight:700}}>{t.duration}</span>
                <ChevronRight size={14} style={{color:'#3d5070'}}/>
              </Link>
            ))}
          </div>
        )}

        {tab==='errors'&&(
          <div style={{display:'flex',flexDirection:'column',gap:8}}>
            {svc.recentErrors.map((e:any,i:number)=>(
              <div key={i} style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.2)',borderRadius:10,padding:14}}>
                <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'#ff7a92',marginBottom:6,lineHeight:1.4}}>{e.title}</div>
                <div style={{display:'flex',gap:12,fontSize:10,color:'#546a88'}}>
                  <span>{e.count.toLocaleString()} occurrences</span>
                  <span>First seen: {e.firstSeen}</span>
                </div>
              </div>
            ))}
          </div>
        )}

        {tab==='dependencies'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12}}>
            {svc.dependencies.map((d:any,i:number)=>(
              <div key={i} style={{background:`${DC[d.type]}06`,border:`1px solid ${DC[d.type]}22`,borderRadius:10,padding:14}}>
                <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:8}}>
                  <span style={{fontSize:14,fontWeight:700,color:'#f0f6ff'}}>{d.name}</span>
                  <span style={{fontSize:9,background:`${DC[d.type]}22`,color:DC[d.type],padding:'2px 7px',borderRadius:8,fontWeight:700}}>{d.type}</span>
                </div>
                <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:8}}>
                  <MetricCard label="RPS" value={d.rps} unit="/s" small/>
                  <MetricCard label="P99" value={d.p99} unit="ms" color={d.p99>500?'#ff4d6a':'#8fa8cc'} small/>
                  <MetricCard label="Error" value={`${d.errRate}%`} color={d.errRate>0?'#ff4d6a':'#0fcf8a'} small/>
                </div>
              </div>
            ))}
          </div>
        )}

        {tab==='deployments'&&(
          <div style={{display:'flex',flexDirection:'column',gap:8}}>
            {svc.deployments.map((d:any,i:number)=>(
              <div key={i} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14,display:'flex',gap:14,alignItems:'center'}}>
                <div style={{width:8,height:8,borderRadius:'50%',background:SC[d.status]}}/>
                <div style={{flex:1}}>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:13,fontWeight:700,color:'#f0f6ff'}}>{d.version}</span>
                  <span style={{fontSize:11,color:'#546a88',marginLeft:10}}>{d.time}</span>
                </div>
                <StatusBadge status={d.status} size="xs"/>
                <span style={{fontSize:11,color:d.errDelta.startsWith('+')?'#ff4d6a':'#0fcf8a',fontWeight:700}}>{d.errDelta}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
