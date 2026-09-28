// DeviationAnalysisPage.tsx — "What's different right now?" - BubbleUp concept from Honeycomb
// ObserveX smart take: correlates across all signals (metrics+logs+traces) automatically
import { useState } from 'react'
import { Zap, TrendingUp, TrendingDown, Eye, Brain, AlertTriangle, BarChart3 } from 'lucide-react'

const BASELINE = { window:'last 7 days', startTime:'72h ago', endTime:'now' }
const INCIDENT = { window:'last 30 minutes', p99:4200, errorRate:8.4, rps:1240 }

const DEVIATIONS = [
  { dimension:'user.region', value:'us-east-1', baseline:28, incident:71, delta:'+154%', impact:'HIGH', type:'metric', interpretation:'Traffic from us-east-1 region is 2.5x higher than normal — possibly a deployment or regional event' },
  { dimension:'db.query_type', value:'SELECT with JOIN', baseline:8, incident:34, delta:'+325%', impact:'CRITICAL', type:'metric', interpretation:'Complex JOIN queries spiking. Likely missing index or new query pattern introduced in recent deployment.' },
  { dimension:'http.route', value:'GET /api/v1/users/:id', baseline:18, incident:72, delta:'+300%', impact:'CRITICAL', type:'metric', interpretation:'This specific endpoint making up 72% of errors vs 18% baseline. Root cause likely in user profile fetch.' },
  { dimension:'error.type', value:'ConnectionTimeout', baseline:0.2, incident:6.4, delta:'+3100%', impact:'CRITICAL', type:'error', interpretation:'ConnectionTimeout errors exploded. Strongly correlated with DB pool exhaustion (P-1001).' },
  { dimension:'k8s.pod', value:'user-svc-7f9d4-xk2p1', baseline:6, incident:28, delta:'+367%', impact:'HIGH', type:'metric', interpretation:'One specific pod generating disproportionate errors — may be unhealthy and should be restarted.' },
  { dimension:'user.plan', value:'Enterprise', baseline:12, incident:18, delta:'+50%', impact:'MEDIUM', type:'business', interpretation:'Enterprise users slightly over-represented in affected traffic. May impact SLAs.' },
  { dimension:'sdk.version', value:'v2.3.x', baseline:22, incident:22, delta:'0%', impact:'NONE', type:'metric', interpretation:'SDK version distribution is normal. Not a factor.' },
  { dimension:'http.method', value:'POST', baseline:34, incident:36, delta:'+6%', impact:'LOW', type:'metric', interpretation:'Slightly elevated POST traffic but within normal variance.' },
]

const impactC: Record<string,string> = { CRITICAL:'#ff4d6a', HIGH:'#f5a623', MEDIUM:'#a855f7', LOW:'#3d9bff', NONE:'#3d5070' }
const impactBg: Record<string,string> = { CRITICAL:'rgba(255,77,106,.12)', HIGH:'rgba(245,166,35,.12)', MEDIUM:'rgba(168,85,247,.12)', LOW:'rgba(61,155,255,.12)', NONE:'rgba(61,80,112,.1)' }
const typeC: Record<string,string> = { metric:'#6c72ff', error:'#ff4d6a', business:'#0fcf8a', log:'#f5a623' }

function DeltaBar({ baseline, incident, maxVal }: { baseline:number, incident:number, maxVal:number }) {
  const bPct = (baseline/maxVal)*100
  const iPct = (incident/maxVal)*100
  return (
    <div style={{position:'relative',height:20,background:'#0b1628',borderRadius:4,overflow:'hidden',width:'100%'}}>
      <div style={{position:'absolute',height:'100%',width:`${bPct}%`,background:'rgba(139,144,255,.3)',borderRadius:4}}/>
      <div style={{position:'absolute',height:'100%',width:`${iPct}%`,background:'rgba(255,77,106,.5)',borderRadius:4}}/>
      <div style={{position:'absolute',inset:0,display:'flex',alignItems:'center',justifyContent:'space-between',padding:'0 6px',fontSize:9}}>
        <span style={{color:'#8b90ff'}}>baseline {baseline}%</span>
        <span style={{color:'#ff7a92'}}>now {incident}%</span>
      </div>
    </div>
  )
}

export default function DeviationAnalysisPage() {
  const [selected, setSelected] = useState(DEVIATIONS[0])
  const [filter, setFilter] = useState('ALL')
  const maxVal = 100

  const shown = filter==='ALL' ? DEVIATIONS : DEVIATIONS.filter(d=>d.impact===filter||d.type===filter)
  const criticalCount = DEVIATIONS.filter(d=>d.impact==='CRITICAL').length

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#ff4d6a,#f5a623)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🔍</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Deviation Analysis <span style={{fontSize:11,background:'rgba(108,114,255,.2)',color:'#8b90ff',padding:'2px 8px',borderRadius:10,fontWeight:700,verticalAlign:'middle'}}>AI-Powered</span></div>
            <div style={{fontSize:12,color:'#546a88'}}>What's different <strong style={{color:'#ff4d6a'}}>right now</strong> vs baseline? · Cross-signal correlation · Automatic dimension scanning</div>
          </div>
        </div>

        <div style={{display:'flex',gap:12,marginBottom:10,padding:'10px 14px',background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.2)',borderRadius:10}}>
          <div style={{flex:1}}>
            <div style={{fontSize:11,fontWeight:700,color:'#ff4d6a',marginBottom:4}}>📍 Analysis Window</div>
            <div style={{display:'flex',gap:16,fontSize:11}}>
              <span style={{color:'#546a88'}}>Baseline: <strong style={{color:'#8fa8cc'}}>{BASELINE.window}</strong></span>
              <span style={{color:'#546a88'}}>Incident window: <strong style={{color:'#ff4d6a'}}>last 30 minutes</strong></span>
              <span style={{color:'#546a88'}}>P99: <strong style={{color:'#ff4d6a'}}>{INCIDENT.p99}ms</strong></span>
              <span style={{color:'#546a88'}}>Error rate: <strong style={{color:'#ff4d6a'}}>{INCIDENT.errorRate}%</strong></span>
            </div>
          </div>
          <div style={{display:'flex',alignItems:'center',gap:6,padding:'5px 12px',background:'linear-gradient(135deg,rgba(108,114,255,.15),rgba(168,85,247,.1))',border:'1px solid rgba(108,114,255,.25)',borderRadius:8}}>
            <Brain size={13} style={{color:'#8b90ff'}}/>
            <span style={{fontSize:11,color:'#8b90ff',fontWeight:700}}>AI found {criticalCount} critical deviations</span>
          </div>
        </div>

        <div style={{display:'flex',gap:6}}>
          {['ALL','CRITICAL','HIGH','metric','error','business'].map(f=>(
            <button key={f} onClick={()=>setFilter(f)} style={{padding:'4px 10px',border:`1px solid ${filter===f?impactC[f]||'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:filter===f?`${impactC[f]||'#6c72ff'}18`:'transparent',color:filter===f?impactC[f]||'#8b90ff':'#546a88',fontSize:10,fontWeight:600,cursor:'pointer'}}>{f}</button>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:360,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {shown.map((d,i)=>(
            <div key={i} onClick={()=>setSelected(d)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected===d?impactC[d.impact]:'transparent'}`,background:selected===d?impactBg[d.impact]:'transparent'}}>
              <div style={{display:'flex',gap:6,marginBottom:6,alignItems:'center'}}>
                <span style={{background:impactBg[d.impact],color:impactC[d.impact],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{d.impact}</span>
                <span style={{background:`${typeC[d.type]}22`,color:typeC[d.type],fontSize:9,padding:'1px 5px',borderRadius:8}}>{d.type}</span>
                <span style={{marginLeft:'auto',fontSize:12,fontWeight:900,color:d.delta.startsWith('+')?'#ff4d6a':'#0fcf8a'}}>{d.delta}</span>
              </div>
              <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:2}}>
                <span style={{color:'#546a88'}}>{d.dimension} = </span>
                <span style={{color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace'}}>{d.value}</span>
              </div>
              <DeltaBar baseline={d.baseline} incident={d.incident} maxVal={maxVal}/>
            </div>
          ))}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:impactBg[selected.impact],border:`1px solid ${impactC[selected.impact]}44`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:10,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:15,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>
                  {selected.dimension} = <span style={{fontFamily:'JetBrains Mono,monospace',color:'#c8d8ef'}}>{selected.value}</span>
                </div>
                <div style={{display:'flex',gap:8}}>
                  <span style={{background:impactBg[selected.impact],color:impactC[selected.impact],fontSize:10,fontWeight:800,padding:'2px 8px',borderRadius:10}}>{selected.impact} IMPACT</span>
                  <span style={{fontSize:14,fontWeight:900,color:selected.delta.startsWith('+')?'#ff4d6a':'#0fcf8a'}}>{selected.delta}</span>
                  <span style={{fontSize:11,color:'#546a88',display:'flex',alignItems:'center'}}>{selected.type} dimension</span>
                </div>
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:10,marginBottom:12}}>
              {[{l:'Baseline (7d avg)',v:`${selected.baseline}%`,c:'#8b90ff'},{l:'During Incident',v:`${selected.incident}%`,c:impactC[selected.impact]}].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'10px 14px',textAlign:'center'}}>
                  <div style={{fontSize:10,color:'#546a88',marginBottom:3}}>{k.l}</div>
                  <div style={{fontSize:24,fontWeight:900,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
            <DeltaBar baseline={selected.baseline} incident={selected.incident} maxVal={maxVal}/>
          </div>

          <div style={{background:'linear-gradient(135deg,rgba(108,114,255,.08),rgba(168,85,247,.04))',border:'1px solid rgba(108,114,255,.2)',borderRadius:12,padding:14,marginBottom:14}}>
            <div style={{display:'flex',gap:8,marginBottom:8}}><Brain size={14} style={{color:'#8b90ff'}}/><span style={{fontSize:12,fontWeight:700,color:'#8b90ff'}}>AI Interpretation</span></div>
            <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.7}}>{selected.interpretation}</div>
          </div>

          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Investigate Further</div>
            <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
              <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>🔍 Filter Traces by {selected.dimension}={selected.value}</button>
              <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>📋 Filter Logs</button>
              <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>📊 Open in Metrics Explorer</button>
              {selected.impact==='CRITICAL'&&<button style={{background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.3)',color:'#ff4d6a',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>🚨 Create Problem</button>}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
