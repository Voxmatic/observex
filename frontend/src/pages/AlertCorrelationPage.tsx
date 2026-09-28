// AlertCorrelationPage.tsx — AI-powered alert correlation & deduplication
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { alertGroups } from '@/lib/api'
import { Link2, Brain, Zap, Bell, ChevronDown, ChevronRight, Shield, Eye } from 'lucide-react'

const DEMO_GROUPS = [
  { id:'ag1', name:'DB Connection Cascade', alerts:['user-service P99 > 4000ms','checkout-service error rate > 3%','api-gateway error rate > 1.8%','payment-service timeout alert'], rootCause:'postgres-primary connection pool exhausted', confidence:94, suppressed:3, notified:1, severity:'CRITICAL', duration:'23m', services:['user-service','checkout-service','api-gateway'], topology:'cascade' },
  { id:'ag2', name:'ML Inference Overload', alerts:['ml-inference CPU > 85%','recommendation-svc timeout','api-gateway slow response'], rootCause:'ml-inference CPU saturation triggering timeouts', confidence:88, suppressed:2, notified:1, severity:'HIGH', duration:'41m', services:['ml-inference','recommendation-svc','api-gateway'], topology:'dependency' },
  { id:'ag3', name:'Deploy-Triggered Error Spike', alerts:['api-gateway error rate spike','api-gateway P99 regression','api-gateway memory growth'], rootCause:'Deployment v2.4.1 regression (auto-rolled back)', confidence:97, suppressed:2, notified:1, severity:'HIGH', duration:'14m (resolved)', services:['api-gateway'], topology:'same-service' },
  { id:'ag4', name:'Kafka Consumer Lag', alerts:['ml-inference-consumer lag > 10K','search-index-update lag > 1K'], rootCause:'Consumer groups falling behind — scale consumers', confidence:85, suppressed:1, notified:1, severity:'HIGH', duration:'1h', services:['ml-inference','search-svc'], topology:'kafka' },
]

const SEV_C: Record<string,string> = { CRITICAL:'#ff4d6a', HIGH:'#f5a623', MEDIUM:'#a855f7', LOW:'#546a88' }
const TOPO_C: Record<string,string> = { cascade:'#ff4d6a', dependency:'#f5a623', 'same-service':'#6c72ff', kafka:'#a855f7' }
const TOPO_ICON: Record<string,string> = { cascade:'🌊', dependency:'🔗', 'same-service':'📦', kafka:'📨' }

export default function AlertCorrelationPage() {
  const [selected, setSelected] = useState(DEMO_GROUPS[0])
  const [expanded, setExpanded] = useState<string[]>(['ag1'])
  const toggle = (id:string) => setExpanded(e=>e.includes(id)?e.filter(x=>x!==id):[...e,id])

  const { data } = useQuery({
    queryKey: ['alert-groups'],
    queryFn: async () => { try { return await alertGroups.list() } catch { return { groups: DEMO_GROUPS } } },
    refetchInterval: 30_000,
  })

  const groups: typeof DEMO_GROUPS = (data as any)?.groups ?? DEMO_GROUPS
  const totalSuppressed = groups.reduce((a,g)=>a+g.suppressed,0)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'14px 20px 10px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🔗</div>
          <div style={{flex:1}}>
            <div style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Alert Correlation AI</div>
            <div style={{fontSize:12,color:'#546a88'}}>Groups related alerts · Suppresses noise · Finds root cause · Reduces alert fatigue by 80%+</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Alert Groups',v:groups.length,c:'#6c72ff'},{l:'Alerts Suppressed',v:totalSuppressed,c:'#0fcf8a'},{l:'Noise Reduction',v:`${Math.round(totalSuppressed/(totalSuppressed+groups.length)*100)}%`,c:'#0fcf8a'},{l:'Avg Confidence',v:`${Math.round(groups.reduce((a,g)=>a+g.confidence,0)/groups.length)}%`,c:'#8b90ff'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 12px'}}>
              <div style={{fontSize:8,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
          <div style={{marginLeft:'auto',padding:'6px 12px',background:'rgba(15,207,138,.06)',border:'1px solid rgba(15,207,138,.2)',borderRadius:8,fontSize:11,color:'#0fcf8a',display:'flex',alignItems:'center',gap:6}}>
            <Brain size={12}/> AI correlation active
          </div>
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Groups list */}
        <div style={{flex:1,overflowY:'auto',padding:14}}>
          {groups.map(g=>{
            const isExp = expanded.includes(g.id)
            return (
              <div key={g.id} style={{marginBottom:10,background:`${SEV_C[g.severity]}06`,border:`1px solid ${SEV_C[g.severity]}33`,borderRadius:12,overflow:'hidden'}}>
                <div onClick={()=>{toggle(g.id);setSelected(g)}} style={{padding:'12px 16px',display:'flex',alignItems:'center',gap:10,cursor:'pointer'}}>
                  {isExp?<ChevronDown size={13} style={{color:'#546a88'}}/>:<ChevronRight size={13} style={{color:'#546a88'}}/>}
                  <span style={{fontSize:18}}>{TOPO_ICON[g.topology]||'🔔'}</span>
                  <div style={{flex:1}}>
                    <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{g.name}</div>
                    <div style={{display:'flex',gap:10,fontSize:10}}>
                      <span style={{background:`${SEV_C[g.severity]}22`,color:SEV_C[g.severity],padding:'1px 6px',borderRadius:8,fontWeight:700}}>{g.severity}</span>
                      <span style={{color:'#546a88'}}>{g.duration}</span>
                      <span style={{color:'#0fcf8a'}}>✓ {g.suppressed} alerts suppressed</span>
                    </div>
                  </div>
                  <div style={{textAlign:'right'}}>
                    <div style={{fontSize:20,fontWeight:900,color:'#0fcf8a',fontFamily:'Syne,sans-serif'}}>{g.confidence}%</div>
                    <div style={{fontSize:9,color:'#546a88'}}>AI confidence</div>
                  </div>
                </div>

                {isExp&&(
                  <div style={{padding:'10px 16px 14px',borderTop:'1px solid rgba(26,45,74,.4)'}}>
                    <div style={{background:'linear-gradient(135deg,rgba(108,114,255,.08),rgba(168,85,247,.04))',border:'1px solid rgba(108,114,255,.2)',borderRadius:10,padding:12,marginBottom:12}}>
                      <div style={{display:'flex',gap:6,marginBottom:5}}><Brain size={13} style={{color:'#8b90ff'}}/><span style={{fontSize:11,fontWeight:700,color:'#8b90ff'}}>Root Cause (AI)</span></div>
                      <div style={{fontSize:12,color:'#c8d8ef'}}>{g.rootCause}</div>
                    </div>
                    <div style={{marginBottom:10}}>
                      <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:7}}>Grouped Alerts ({g.alerts.length})</div>
                      {g.alerts.map((a,i)=>(
                        <div key={i} style={{display:'flex',gap:8,alignItems:'center',padding:'5px 0',borderBottom:i<g.alerts.length-1?'1px solid rgba(26,45,74,.3)':'none'}}>
                          <Bell size={11} style={{color:'#f5a623',flexShrink:0}}/>
                          <span style={{fontSize:11,color:'#8fa8cc'}}>{a}</span>
                          {i>0&&<span style={{marginLeft:'auto',fontSize:9,background:'rgba(15,207,138,.1)',color:'#0fcf8a',padding:'1px 7px',borderRadius:8,fontWeight:700}}>suppressed</span>}
                          {i===0&&<span style={{marginLeft:'auto',fontSize:9,background:'rgba(108,114,255,.15)',color:'#8b90ff',padding:'1px 7px',borderRadius:8,fontWeight:700}}>notified</span>}
                        </div>
                      ))}
                    </div>
                    <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                      <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View in Problems</button>
                      <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Topology</button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>

        {/* Correlation rules sidebar */}
        <div style={{width:240,borderLeft:'1px solid #1a2d4a',overflowY:'auto',padding:14,flexShrink:0}}>
          <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Correlation Strategies</div>
          {[
            { label:'Topology correlation', desc:'Groups alerts from services with upstream/downstream dependencies', icon:'🔗', active:true },
            { label:'Time-window grouping', desc:'Alerts within 5 min window clustered together', icon:'⏱', active:true },
            { label:'Deploy correlation', desc:'Links errors to recent deployments automatically', icon:'🚀', active:true },
            { label:'Log-to-alert match', desc:'Correlates log error spikes with metric alerts', icon:'📋', active:true },
            { label:'Machine learning', desc:'Pattern-based clustering from historical data', icon:'🧠', active:true },
          ].map(r=>(
            <div key={r.label} style={{background:'rgba(17,31,53,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'9px 10px',marginBottom:7}}>
              <div style={{display:'flex',gap:6,marginBottom:3}}>
                <span style={{fontSize:14}}>{r.icon}</span>
                <span style={{fontSize:11,fontWeight:700,color:'#f0f6ff'}}>{r.label}</span>
                {r.active&&<span style={{marginLeft:'auto',fontSize:8,background:'rgba(15,207,138,.15)',color:'#0fcf8a',padding:'1px 5px',borderRadius:8,fontWeight:800}}>ON</span>}
              </div>
              <div style={{fontSize:10,color:'#546a88',lineHeight:1.4}}>{r.desc}</div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
