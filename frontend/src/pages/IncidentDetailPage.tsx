// IncidentDetailPage.tsx — /incidents/:id — War room: timeline, comments, actions, postmortem
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { MetricCard } from '@/components/shared/MetricCard'
import { ArrowLeft, Clock, AlertTriangle, Users, MessageSquare, Zap, GitBranch, CheckCircle, Send } from 'lucide-react'

const INCIDENTS: Record<string,any> = {
  'INC-2847': {
    id:'INC-2847', title:'API Gateway elevated error rate — checkout impact', severity:'CRITICAL', status:'investigating', priority:'P1',
    commander:'sara.chen', responders:['jin.park','david.wu','platform-team'],
    startTime:'19:24 UTC', duration:'23 minutes', impactUsers:14200,
    impactSummary:'Checkout flow error rate 3.2%, user profile queries timing out, 14.2K users impacted',
    linkedProblem:'P-1001', linkedServices:['api-gateway','user-service','checkout-service','postgres-primary'],
    sloImpact:[{name:'API Availability',current:98.2,target:99.9,burning:true},{name:'Checkout Success',current:96.8,target:99.5,burning:true}],
    timeline:[
      { time:'19:24', actor:'AI Agent', type:'detection', msg:'Anomaly detected: postgres-primary connection pool at 96%' },
      { time:'19:25', actor:'PagerDuty', type:'page', msg:'P1 page sent to sara.chen (Incident Commander)' },
      { time:'19:27', actor:'sara.chen', type:'action', msg:'Acknowledged. Investigating DB connection pool.' },
      { time:'19:30', actor:'AI Agent', type:'analysis', msg:'Root cause identified: postgres-primary pool exhaustion. Blast radius: user-service, checkout-service, api-gateway.' },
      { time:'19:32', actor:'jin.park', type:'action', msg:'Checking pg_stat_activity — 18 idle-in-transaction connections from user-service' },
      { time:'19:35', actor:'sara.chen', type:'update', msg:'Updated status page: API — Partial Outage. Subscribers notified.' },
      { time:'19:38', actor:'AI Agent', type:'suggestion', msg:'Suggested: Increase pool max_connections from 20 to 50. Confidence: 92%. Auto-executable.' },
      { time:'19:40', actor:'david.wu', type:'action', msg:'Approved AI suggestion. Pool size increased to 50.' },
      { time:'19:42', actor:'AI Agent', type:'metric', msg:'Connection pool utilization dropping: 96% → 72% → 48%' },
      { time:'19:44', actor:'platform-team', type:'action', msg:'Added index on users.preferences JSONB column to prevent future long queries.' },
    ],
    comments:[
      { author:'sara.chen', time:'19:28', body:'This looks like the DB pool issue we had last month. Checking if the fix from P-892 regressed.' },
      { author:'jin.park', time:'19:33', body:'Confirmed: user-service v2.4.0 introduced a new preferences query that holds connections 4x longer than v2.3.x. Missing index on JSONB column.' },
      { author:'david.wu', time:'19:41', body:'Pool increased. Error rates are normalizing. User-service P99 dropping from 4200ms → 1800ms.' },
    ],
  },
}

const TYPE_ICONS: Record<string,{color:string,label:string}> = {
  detection:{color:'#ff4d6a',label:'Detection'}, page:{color:'#f5a623',label:'Page'},
  action:{color:'#0fcf8a',label:'Action'}, analysis:{color:'#8b90ff',label:'AI Analysis'},
  update:{color:'#6c72ff',label:'Update'}, suggestion:{color:'#a855f7',label:'AI Suggestion'},
  metric:{color:'#3d9bff',label:'Metric'}, resolved:{color:'#0fcf8a',label:'Resolved'},
}

export default function IncidentDetailPage() {
  const { id } = useParams<{id:string}>()
  const [tab, setTab] = useState<'timeline'|'impact'|'comments'>('timeline')
  const [newComment, setNewComment] = useState('')
  const inc = INCIDENTS[id||''] || INCIDENTS['INC-2847']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:6}}>
          <Link to="/incidents" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <div style={{flex:1}}>
            <div style={{display:'flex',gap:6,marginBottom:3}}>
              <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#6c72ff'}}>{inc.id}</span>
              <StatusBadge status={inc.severity}/><StatusBadge status={inc.status}/>
              <span style={{fontSize:10,background:'rgba(255,77,106,.15)',color:'#ff4d6a',padding:'1px 6px',borderRadius:6,fontWeight:800}}>{inc.priority}</span>
            </div>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{inc.title}</div>
          </div>
          <div style={{display:'flex',gap:8}}>
            <button style={{background:'rgba(15,207,138,.1)',border:'1px solid rgba(15,207,138,.25)',color:'#0fcf8a',padding:'6px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>Resolve</button>
            <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Create postmortem</button>
          </div>
        </div>
        <div style={{display:'flex',gap:8,marginBottom:8}}>
          <MetricCard label="Duration" value={inc.duration} color="#ff4d6a" small/>
          <MetricCard label="Commander" value={inc.commander} color="#6c72ff" small/>
          <MetricCard label="Responders" value={inc.responders.length} color="#8fa8cc" small/>
          <MetricCard label="Users affected" value={inc.impactUsers.toLocaleString()} color="#f5a623" small/>
          <MetricCard label="Services" value={inc.linkedServices.length} color="#6c72ff" small/>
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['timeline','impact','comments'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'6px 16px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>
              {t}{t==='comments'?` (${inc.comments.length})`:''}
            </button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='timeline'&&(
          <div style={{position:'relative',paddingLeft:24}}>
            <div style={{position:'absolute',left:7,top:0,bottom:0,width:2,background:'#1a2d4a'}}/>
            {inc.timeline.map((ev:any,i:number)=>{
              const cfg = TYPE_ICONS[ev.type]||TYPE_ICONS.action
              return (
                <div key={i} style={{position:'relative',marginBottom:12,paddingLeft:24}}>
                  <div style={{position:'absolute',left:-21,top:6,width:12,height:12,borderRadius:'50%',background:cfg.color,zIndex:1,opacity:.8}}/>
                  <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:8,padding:'8px 12px'}}>
                    <div style={{display:'flex',gap:8,marginBottom:3}}>
                      <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:9,color:'#3d5070'}}>{ev.time}</span>
                      <span style={{fontSize:9,background:`${cfg.color}22`,color:cfg.color,padding:'1px 5px',borderRadius:6,fontWeight:700}}>{cfg.label}</span>
                      <span style={{fontSize:10,fontWeight:700,color:'#8fa8cc'}}>{ev.actor}</span>
                    </div>
                    <div style={{fontSize:11,color:'#c8d8ef'}}>{ev.msg}</div>
                  </div>
                </div>
              )
            })}
          </div>
        )}

        {tab==='impact'&&(
          <div>
            <div style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.2)',borderRadius:12,padding:16,marginBottom:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#ff4d6a',marginBottom:6}}>Impact summary</div>
              <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.7}}>{inc.impactSummary}</div>
            </div>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>SLO impact</div>
            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:10,marginBottom:14}}>
              {inc.sloImpact.map((slo:any)=>(
                <div key={slo.name} style={{background:slo.burning?'rgba(255,77,106,.06)':'rgba(17,31,53,.7)',border:`1px solid ${slo.burning?'rgba(255,77,106,.3)':'#1a2d4a'}`,borderRadius:10,padding:12}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:6}}>{slo.name}</div>
                  <div style={{display:'flex',justifyContent:'space-between',fontSize:11}}>
                    <span style={{color:'#546a88'}}>Current: <strong style={{color:slo.current<slo.target?'#ff4d6a':'#0fcf8a'}}>{slo.current}%</strong></span>
                    <span style={{color:'#546a88'}}>Target: <strong style={{color:'#8fa8cc'}}>{slo.target}%</strong></span>
                  </div>
                  {slo.burning&&<div style={{marginTop:6,fontSize:10,color:'#ff4d6a',fontWeight:700}}>⚠ Error budget burning</div>}
                </div>
              ))}
            </div>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Linked services</div>
            <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
              {inc.linkedServices.map((s:string)=>(
                <Link key={s} to={`/services/${s}`} style={{textDecoration:'none',background:'rgba(108,114,255,.1)',border:'1px solid rgba(108,114,255,.2)',borderRadius:8,padding:'6px 12px',fontSize:11,color:'#8b90ff',fontWeight:600}}>{s}</Link>
              ))}
            </div>
            {inc.linkedProblem&&(
              <div style={{marginTop:14}}>
                <Link to={`/problems/${inc.linkedProblem}`} style={{display:'inline-flex',gap:6,alignItems:'center',padding:'8px 14px',background:'rgba(168,85,247,.1)',border:'1px solid rgba(168,85,247,.25)',borderRadius:8,textDecoration:'none',fontSize:11,color:'#c084fc',fontWeight:600}}>
                  <AlertTriangle size={12}/>Linked problem: {inc.linkedProblem}
                </Link>
              </div>
            )}
          </div>
        )}

        {tab==='comments'&&(
          <div>
            {inc.comments.map((c:any,i:number)=>(
              <div key={i} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14,marginBottom:10}}>
                <div style={{display:'flex',gap:8,marginBottom:6}}>
                  <div style={{width:24,height:24,borderRadius:'50%',background:'#1a2d4a',display:'flex',alignItems:'center',justifyContent:'center',fontSize:10,fontWeight:800,color:'#8fa8cc'}}>{c.author.charAt(0).toUpperCase()}</div>
                  <div><div style={{fontSize:11,fontWeight:700,color:'#f0f6ff'}}>{c.author}</div><div style={{fontSize:9,color:'#3d5070'}}>{c.time}</div></div>
                </div>
                <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.6,paddingLeft:32}}>{c.body}</div>
              </div>
            ))}
            <div style={{display:'flex',gap:8,marginTop:10}}>
              <input value={newComment} onChange={e=>setNewComment(e.target.value)} placeholder="Add a comment..."
                style={{flex:1,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'8px 12px',fontSize:12,color:'#c8d8ef',outline:'none'}}/>
              <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'8px 14px',borderRadius:8,fontSize:11,cursor:'pointer',display:'flex',gap:4,alignItems:'center'}}><Send size={12}/>Send</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
