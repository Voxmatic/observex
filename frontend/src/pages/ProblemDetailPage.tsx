// ProblemDetailPage.tsx — /problems/:id — Root cause timeline, causal chain, blast radius
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { TimeRangePicker } from '@/components/shared/TimeRangePicker'
import { MetricCard } from '@/components/shared/MetricCard'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ArrowLeft, Brain, Clock, AlertTriangle, Zap, GitBranch, Users, ChevronRight, CheckCircle } from 'lucide-react'

const PROBLEMS: Record<string,any> = {
  'P-1001': {
    id:'P-1001', title:'Connection pool exhaustion cascading from postgres-primary', status:'open', severity:'CRITICAL',
    rootCause:'postgres-primary', rootCauseType:'DATABASE', aiConfidence:0.94,
    aiAnalysis:'The PostgreSQL primary database connection pool reached 96% capacity at 19:24 UTC. This caused connection timeouts in user-service (dependent on postgres-primary for profile queries), which cascaded to checkout-service (calls user-service for cart validation), and finally manifested as elevated error rates on api-gateway. The root cause is likely the combination of a recent traffic spike (+40% from us-east-1) and a new query pattern introduced in user-service v2.4.0 that holds connections longer due to missing index on users.preferences JSONB column.',
    impactUsers:14200, impactRevenue:'$42K/hr est.',
    firstSeen:'19:24:00 UTC', lastSeen:'now', duration:'23 minutes',
    affectedServices:['user-service','checkout-service','api-gateway','payment-service'],
    blastRadius:['14,200 users','$42K/hr revenue','3 SLOs breaching','checkout flow 100% degraded'],
    causalChain:[
      { time:'T-23m', entity:'postgres-primary', event:'Connection pool reaches 85%', type:'DATABASE', root:false },
      { time:'T-18m', entity:'postgres-primary', event:'Pool exhaustion: 96% used, queries queueing', type:'DATABASE', root:true },
      { time:'T-16m', entity:'user-service',     event:'Connection timeout errors spike to 8.4%', type:'SERVICE', root:false },
      { time:'T-14m', entity:'user-service',     event:'P99 latency jumps 800ms → 4200ms', type:'SERVICE', root:false },
      { time:'T-12m', entity:'checkout-service', event:'Upstream timeout from user-service: 3.2% error rate', type:'SERVICE', root:false },
      { time:'T-10m', entity:'api-gateway',      event:'Error rate 1.84%, P99 1284ms', type:'SERVICE', root:false },
      { time:'T-5m',  entity:'payment-service',  event:'Timeout on user validation calls', type:'SERVICE', root:false },
      { time:'T-0',   entity:'SLO Engine',       event:'API Availability SLO burning at 18.2x rate', type:'SLO', root:false },
    ],
    relatedTraces:['abc-123-def','ghi-456-jkl','mno-789-pqr'],
    relatedAlerts:['High Error Rate: user-service','P99 Latency: api-gateway','SLO Burn Rate: API Availability'],
    suggestedActions:[
      { action:'Increase DB pool size', risk:'LOW', confidence:0.92, automated:true },
      { action:'Add index on users.preferences', risk:'LOW', confidence:0.88, automated:false },
      { action:'Scale user-service replicas 4→6', risk:'LOW', confidence:0.85, automated:true },
      { action:'Enable connection pool monitoring alert at 70%', risk:'LOW', confidence:0.99, automated:false },
    ],
  },
}

const TC: Record<string,string> = { DATABASE:'#a855f7', SERVICE:'#6c72ff', SLO:'#ff4d6a', HOST:'#f5a623', NETWORK:'#0fcf8a' }

export default function ProblemDetailPage() {
  const { id } = useParams<{id:string}>()
  const [tab, setTab] = useState<'timeline'|'impact'|'actions'|'related'>('timeline')
  const problem = PROBLEMS[id||''] || PROBLEMS['P-1001']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
          <Link to="/problems" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <div style={{flex:1}}>
            <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:4}}>
              <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#6c72ff'}}>{problem.id}</span>
              <StatusBadge status={problem.severity}/>
              <StatusBadge status={problem.status}/>
            </div>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{problem.title}</div>
          </div>
          <div style={{textAlign:'right'}}>
            <div style={{fontSize:12,color:'#546a88'}}>Duration: <strong style={{color:'#ff4d6a'}}>{problem.duration}</strong></div>
            <div style={{fontSize:10,color:'#546a88'}}>Since {problem.firstSeen}</div>
          </div>
        </div>
        <div style={{display:'flex',gap:8,marginBottom:8}}>
          <MetricCard label="Affected users" value={problem.impactUsers.toLocaleString()} color="#ff4d6a" small/>
          <MetricCard label="Revenue impact" value={problem.impactRevenue} color="#f5a623" small/>
          <MetricCard label="Services affected" value={problem.affectedServices.length} color="#6c72ff" small/>
          <MetricCard label="AI confidence" value={`${Math.round(problem.aiConfidence*100)}%`} color="#8b90ff" small/>
          <MetricCard label="Root cause" value={problem.rootCause} color="#a855f7" small/>
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['timeline','impact','actions','related'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'6px 16px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='timeline'&&(
          <div>
            {/* AI Analysis */}
            <div style={{background:'linear-gradient(135deg,rgba(108,114,255,.08),rgba(168,85,247,.04))',border:'1px solid rgba(108,114,255,.25)',borderRadius:12,padding:16,marginBottom:16}}>
              <div style={{display:'flex',gap:8,marginBottom:8}}><Brain size={14} style={{color:'#8b90ff'}}/><span style={{fontSize:12,fontWeight:700,color:'#8b90ff'}}>AI root cause analysis</span><span style={{fontSize:10,color:'#546a88'}}>confidence: {Math.round(problem.aiConfidence*100)}%</span></div>
              <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.7}}>{problem.aiAnalysis}</div>
            </div>

            {/* Causal chain timeline */}
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Causal chain timeline</div>
            <div style={{position:'relative',paddingLeft:24}}>
              <div style={{position:'absolute',left:7,top:0,bottom:0,width:2,background:'#1a2d4a'}}/>
              {problem.causalChain.map((step:any,i:number)=>(
                <div key={i} style={{position:'relative',marginBottom:16,paddingLeft:24}}>
                  <div style={{position:'absolute',left:-21,top:4,width:14,height:14,borderRadius:'50%',background:step.root?'#ff4d6a':TC[step.type]||'#546a88',border:`2px solid ${step.root?'#ff4d6a':'#1a2d4a'}`,zIndex:1}}/>
                  <div style={{background:step.root?'rgba(255,77,106,.08)':'rgba(17,31,53,.7)',border:`1px solid ${step.root?'rgba(255,77,106,.3)':'#1a2d4a'}`,borderRadius:10,padding:'10px 14px'}}>
                    <div style={{display:'flex',gap:8,marginBottom:4}}>
                      <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88'}}>{step.time}</span>
                      <Link to={`/services/${step.entity}`} style={{fontSize:11,fontWeight:700,color:TC[step.type],textDecoration:'none'}}>{step.entity}</Link>
                      <span style={{fontSize:9,background:`${TC[step.type]}22`,color:TC[step.type],padding:'1px 6px',borderRadius:6}}>{step.type}</span>
                      {step.root&&<span style={{fontSize:9,background:'rgba(255,77,106,.2)',color:'#ff4d6a',padding:'1px 6px',borderRadius:6,fontWeight:800}}>ROOT CAUSE</span>}
                    </div>
                    <div style={{fontSize:11,color:'#c8d8ef'}}>{step.event}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {tab==='impact'&&(
          <div>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Blast radius</div>
            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12,marginBottom:16}}>
              {problem.blastRadius.map((b:string,i:number)=>(
                <div key={i} style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.2)',borderRadius:10,padding:'12px 16px',display:'flex',gap:8,alignItems:'center'}}>
                  <AlertTriangle size={14} style={{color:'#ff4d6a',flexShrink:0}}/>
                  <span style={{fontSize:12,color:'#f0f6ff',fontWeight:600}}>{b}</span>
                </div>
              ))}
            </div>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Affected services</div>
            <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
              {problem.affectedServices.map((s:string)=>(
                <Link key={s} to={`/services/${s}`} style={{textDecoration:'none',background:'rgba(108,114,255,.1)',border:'1px solid rgba(108,114,255,.25)',borderRadius:8,padding:'8px 14px',fontSize:12,color:'#8b90ff',fontWeight:600}}>{s}</Link>
              ))}
            </div>
          </div>
        )}

        {tab==='actions'&&(
          <div>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>AI-suggested remediation actions</div>
            {problem.suggestedActions.map((a:any,i:number)=>(
              <div key={i} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14,marginBottom:10,display:'flex',gap:12,alignItems:'center'}}>
                <div style={{flex:1}}>
                  <div style={{fontSize:13,fontWeight:600,color:'#f0f6ff',marginBottom:4}}>{a.action}</div>
                  <div style={{display:'flex',gap:8,fontSize:10}}>
                    <span style={{color:a.risk==='LOW'?'#0fcf8a':'#f5a623'}}>Risk: {a.risk}</span>
                    <span style={{color:'#8b90ff'}}>Confidence: {Math.round(a.confidence*100)}%</span>
                    {a.automated&&<span style={{color:'#0fcf8a',background:'rgba(15,207,138,.1)',padding:'1px 6px',borderRadius:6}}>Auto-executable</span>}
                  </div>
                </div>
                {a.automated?
                  <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>Execute</button>:
                  <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Create ticket</button>
                }
              </div>
            ))}
          </div>
        )}

        {tab==='related'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Related traces</div>
              {problem.relatedTraces.map((t:string)=>(
                <Link key={t} to={`/traces/${t}`} style={{display:'block',textDecoration:'none',padding:'6px 0',borderBottom:'1px solid rgba(26,45,74,.3)',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#6c72ff'}}>{t}</Link>
              ))}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Related alerts</div>
              {problem.relatedAlerts.map((a:string,i:number)=>(
                <div key={i} style={{padding:'6px 0',borderBottom:'1px solid rgba(26,45,74,.3)',fontSize:11,color:'#f5a623'}}>{a}</div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
