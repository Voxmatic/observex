// AdaptiveTelemetryPage.tsx — Intelligent metric reduction: auto-detect unused/redundant signals
// Grafana's Adaptive Telemetry concept made actionable in ObserveX
import { useState } from 'react'
import { TrendingDown, AlertTriangle, CheckCircle, DollarSign, Database, Zap, Settings, ToggleRight, ToggleLeft } from 'lucide-react'
import toast from 'react-hot-toast'

const METRIC_RULES = [
  { id:'ar1', type:'drop', name:'Drop unused native host metrics', description:'42 native host metrics with 0 queries in 30 days. These are collected but never read.', savings:284, savingsPct:18, affected:42000, status:'active', risk:'LOW', category:'Infrastructure' },
  { id:'ar2', type:'aggregate', name:'Aggregate high-cardinality pod metrics', description:'Pod-level metrics have 8,400+ unique label combinations. Aggregate to namespace level for unused label dimensions.', savings:840, savingsPct:44, affected:8400, status:'recommended', risk:'MEDIUM', category:'Kubernetes' },
  { id:'ar3', type:'drop', name:'Drop staging environment from production retention', description:'Staging metrics retained same as production (90 days). Reduce to 7 days — zero production impact.', savings:1240, savingsPct:32, affected:280000, status:'recommended', risk:'LOW', category:'Cost' },
  { id:'ar4', type:'sample', name:'Reduce high-frequency health check metrics', description:'/health endpoint scraped every 5s — reduce to 60s. No change in alerting granularity.', savings:148, savingsPct:8, affected:12, status:'active', risk:'LOW', category:'APM' },
  { id:'ar5', type:'drop', name:'Remove duplicate collector streams', description:'14 services sending duplicate native and OTLP samples for the same metric names. Keep the higher-fidelity ObserveX source.', savings:420, savingsPct:22, affected:14, status:'recommended', risk:'MEDIUM', category:'Dedup' },
]

const SIGNAL_HEALTH = [
  { signal:'Metrics', total:284000, used:141000, unused:143000, usedPct:49.6, cost:'$420/mo', trend:'+12%' },
  { signal:'Logs', total:8400000, used:4200000, unused:4200000, usedPct:50, cost:'$1,240/mo', trend:'+28%' },
  { signal:'Traces', total:1200000, used:960000, unused:240000, usedPct:80, cost:'$840/mo', trend:'+5%' },
  { signal:'Profiles', total:48000, used:48000, unused:0, usedPct:100, cost:'$180/mo', trend:'stable' },
]

const typeC: Record<string,string> = { drop:'#ff4d6a', aggregate:'#f5a623', sample:'#6c72ff', deduplicate:'#a855f7' }
const riskC: Record<string,string> = { LOW:'#0fcf8a', MEDIUM:'#f5a623', HIGH:'#ff4d6a' }

export default function AdaptiveTelemetryPage() {
  const [rules, setRules] = useState(METRIC_RULES)
  const totalSavings = rules.filter(r=>r.status==='active').reduce((a,r)=>a+r.savings,0)
  const potentialSavings = rules.filter(r=>r.status==='recommended').reduce((a,r)=>a+r.savings,0)

  const applyRule = (id: string) => {
    setRules(r=>r.map(x=>x.id===id?{...x,status:'active'}:x))
    toast.success('Adaptive rule applied — savings starting in ~5min')
  }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#0fcf8a,#6c72ff)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>♻️</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Adaptive Telemetry <span style={{fontSize:11,background:'rgba(108,114,255,.2)',color:'#8b90ff',padding:'2px 8px',borderRadius:10,fontWeight:700,verticalAlign:'middle'}}>AI-Powered</span></div>
            <div style={{fontSize:12,color:'#546a88'}}>Auto-detect unused signals · Smart aggregation · Deduplication · Cut telemetry costs by up to 60%</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Active Savings',v:`$${totalSavings}/mo`,c:'#0fcf8a'},{l:'Potential Additional',v:`$${potentialSavings}/mo`,c:'#f5a623'},{l:'Total Potential',v:`$${totalSavings+potentialSavings}/mo`,c:'#6c72ff'},{l:'Active Rules',v:rules.filter(r=>r.status==='active').length,c:'#8fa8cc'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {/* Signal health overview */}
        <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10,marginBottom:14}}>
          {SIGNAL_HEALTH.map(s=>(
            <div key={s.signal} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:8}}>{s.signal}</div>
              <div style={{marginBottom:8}}>
                <div style={{display:'flex',justifyContent:'space-between',fontSize:10,color:'#546a88',marginBottom:4}}>
                  <span>Used</span><span style={{color:'#0fcf8a',fontWeight:700}}>{s.usedPct}%</span>
                </div>
                <div style={{height:6,background:'#0b1628',borderRadius:3,overflow:'hidden',marginBottom:4}}>
                  <div style={{height:'100%',width:`${s.usedPct}%`,background:'#0fcf8a'}}/>
                </div>
                <div style={{height:6,background:'#0b1628',borderRadius:3,overflow:'hidden'}}>
                  <div style={{height:'100%',width:`${100-s.usedPct}%`,background:'#ff4d6a',opacity:.5}}/>
                </div>
              </div>
              <div style={{display:'flex',justifyContent:'space-between',fontSize:10}}>
                <span style={{color:'#546a88'}}>Cost:</span>
                <span style={{color:'#f5a623',fontWeight:700}}>{s.cost}</span>
              </div>
              <div style={{display:'flex',justifyContent:'space-between',fontSize:10,marginTop:2}}>
                <span style={{color:'#546a88'}}>Growth:</span>
                <span style={{color:s.trend.startsWith('+')?'#ff4d6a':'#0fcf8a'}}>{s.trend}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Adaptive rules */}
        <div style={{marginBottom:10,display:'flex',gap:8,alignItems:'center'}}>
          <div style={{fontSize:13,fontWeight:700,color:'#c8d8ef'}}>Adaptive Rules</div>
          <div style={{fontSize:11,color:'#546a88'}}>AI-generated recommendations based on 30-day query analysis</div>
        </div>
        <div style={{display:'flex',flexDirection:'column',gap:10}}>
          {rules.map(r=>(
            <div key={r.id} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${r.status==='active'?'rgba(15,207,138,.2)':'#1a2d4a'}`,borderRadius:12,padding:16}}>
              <div style={{display:'flex',gap:12,alignItems:'flex-start'}}>
                <div style={{flex:1}}>
                  <div style={{display:'flex',gap:8,marginBottom:6}}>
                    <span style={{background:`${typeC[r.type]}22`,color:typeC[r.type],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8,textTransform:'uppercase'}}>{r.type}</span>
                    <span style={{background:`${riskC[r.risk]}22`,color:riskC[r.risk],fontSize:9,fontWeight:700,padding:'2px 7px',borderRadius:8}}>Risk: {r.risk}</span>
                    <span style={{background:'rgba(84,106,136,.15)',color:'#546a88',fontSize:9,padding:'2px 7px',borderRadius:8}}>{r.category}</span>
                    {r.status==='active'&&<span style={{background:'rgba(15,207,138,.15)',color:'#0fcf8a',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8}}>✓ ACTIVE</span>}
                  </div>
                  <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:4}}>{r.name}</div>
                  <div style={{fontSize:11,color:'#8fa8cc',lineHeight:1.5}}>{r.description}</div>
                </div>
                <div style={{textAlign:'right',flexShrink:0}}>
                  <div style={{fontSize:20,fontWeight:900,color:'#0fcf8a',fontFamily:'Syne,sans-serif'}}>${r.savings}/mo</div>
                  <div style={{fontSize:10,color:'#546a88'}}>saves {r.savingsPct}%</div>
                  {r.status==='recommended'&&(
                    <button onClick={()=>applyRule(r.id)} style={{marginTop:8,background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>Apply Rule</button>
                  )}
                  {r.status==='active'&&(
                    <button onClick={()=>{setRules(x=>x.map(rx=>rx.id===r.id?{...rx,status:'recommended'}:rx));toast('Rule disabled')}} style={{marginTop:8,background:'transparent',color:'#546a88',border:'1px solid #1a2d4a',padding:'5px 12px',borderRadius:8,fontSize:10,cursor:'pointer'}}>Disable</button>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
