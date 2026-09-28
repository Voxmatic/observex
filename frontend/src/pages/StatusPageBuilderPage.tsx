// StatusPageBuilderPage.tsx — Public status page management with auto-detection from problems
import { useState } from 'react'
import { Globe, Plus, CheckCircle, Clock, AlertTriangle, XCircle, Eye, Edit3, Bell, ExternalLink } from 'lucide-react'

const STATUS_PAGE = {
  name: 'ObserveX Platform Status',
  url: 'status.acme.io',
  logo: '🏢',
  subscribers: 14284,
  lastUpdated: '19:47:00 UTC',
}

const COMPONENTS = [
  { id:'api', name:'API — REST & GraphQL', status:'partial_outage', category:'Core API', description:'REST API, GraphQL, WebSocket endpoints', uptime30:'99.12', uptime90:'99.84', autoLinked:true, linkedService:'api-gateway' },
  { id:'web', name:'Web Dashboard', status:'operational', category:'Frontend', description:'Main web interface and admin console', uptime30:'99.98', uptime90:'99.99', autoLinked:false, linkedService:null },
  { id:'agents', name:'Monitoring Agents', status:'operational', category:'Data Collection', description:'ObserveX agents, eBPF instrumentation', uptime30:'100.00', uptime90:'99.99', autoLinked:true, linkedService:'observex-agent' },
  { id:'ingest', name:'Metrics Ingestion', status:'operational', category:'Data Collection', description:'Metrics, logs, traces ingest pipeline', uptime30:'99.99', uptime90:'99.99', autoLinked:true, linkedService:'ingestor' },
  { id:'alerts', name:'Alerting & Notifications', status:'operational', category:'Core Features', description:'Alert evaluation, PagerDuty, Slack, email', uptime30:'99.97', uptime90:'99.98', autoLinked:false, linkedService:null },
  { id:'ai', name:'AI Agent', status:'operational', category:'AI & Intelligence', description:'Causal AI, anomaly detection, LLM integration', uptime30:'99.94', uptime90:'99.95', autoLinked:true, linkedService:'ai-agent' },
  { id:'dash', name:'Dashboards', status:'operational', category:'Core Features', description:'Dashboard rendering, chart queries', uptime30:'100.00', uptime90:'99.99', autoLinked:false, linkedService:null },
  { id:'db', name:'Database (writes)', status:'major_outage', category:'Infrastructure', description:'PostgreSQL write path, connection pooling', uptime30:'98.44', uptime90:'99.61', autoLinked:true, linkedService:'postgres-primary' },
]

const INCIDENTS_HISTORY = [
  { id:'i1', title:'Database connection pool exhausted — checkout impact', status:'ongoing', severity:'critical', startTime:'19:24 UTC', updatedTime:'19:47 UTC', components:['api','db'],
    updates:[
      { time:'19:47 UTC', body:'We are working to increase the PostgreSQL connection pool size. Engineering team is actively investigating.' },
      { time:'19:35 UTC', body:'The issue has been identified as connection pool exhaustion in postgres-primary. Checkout and user profile APIs are degraded.' },
      { time:'19:28 UTC', body:'We are investigating reports of API errors affecting checkout flows.' },
    ]},
  { id:'i2', title:'API Gateway elevated error rate — resolved', status:'resolved', severity:'high', startTime:'18:42 UTC', updatedTime:'18:56 UTC', components:['api'],
    updates:[{ time:'18:56 UTC', body:'Auto-rollback of api-gateway v2.4.1 to v2.4.0 completed. All metrics returning to normal.' }]},
]

const stC: Record<string,string> = { operational:'#0fcf8a', partial_outage:'#f5a623', major_outage:'#ff4d6a', degraded_performance:'#f5a623', under_maintenance:'#546a88' }
const stLabel: Record<string,string> = { operational:'Operational', partial_outage:'Partial Outage', major_outage:'Major Outage', degraded_performance:'Degraded', under_maintenance:'Maintenance' }
const stIcon: Record<string,any> = { operational:CheckCircle, partial_outage:AlertTriangle, major_outage:XCircle, degraded_performance:AlertTriangle }

const overall = COMPONENTS.some(c=>c.status==='major_outage')?'major_outage':COMPONENTS.some(c=>c.status==='partial_outage')?'partial_outage':'operational'

export default function StatusPageBuilderPage() {
  const [tab, setTab] = useState<'preview'|'manage'|'incidents'>('preview')
  const [components, setComponents] = useState(COMPONENTS)
  const [incidents] = useState(INCIDENTS_HISTORY)

  const setStatus = (id:string, status:string) => setComponents(c=>c.map(x=>x.id===id?{...x,status}:x))

  const categories = [...new Set(COMPONENTS.map(c=>c.category))]
  const grouped = categories.map(cat=>({ cat, items:components.filter(c=>c.category===cat) }))

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🌐</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Status Page</div>
            <div style={{fontSize:12,color:'#546a88'}}>Public status page · Auto-synced from Problems · Component management · Subscriber notifications</div>
          </div>
          <div style={{display:'flex',gap:8}}>
            <div style={{background:'rgba(15,207,138,.08)',border:'1px solid rgba(15,207,138,.2)',borderRadius:8,padding:'6px 12px',fontSize:11,color:'#0fcf8a'}}>{STATUS_PAGE.subscribers.toLocaleString()} subscribers</div>
            <button style={{display:'flex',gap:5,alignItems:'center',background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',padding:'6px 12px',borderRadius:8,fontSize:11,cursor:'pointer'}}>
              <ExternalLink size={11}/>{STATUS_PAGE.url}
            </button>
          </div>
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['preview','manage','incidents'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'7px 18px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='preview'&&(
          <div style={{maxWidth:720,margin:'0 auto'}}>
            {/* Status header */}
            <div style={{background:overall==='operational'?'rgba(15,207,138,.08)':'rgba(255,77,106,.08)',border:`1px solid ${stC[overall]}33`,borderRadius:16,padding:24,marginBottom:20,textAlign:'center'}}>
              <div style={{fontSize:32,marginBottom:8}}>{STATUS_PAGE.logo}</div>
              <div style={{fontSize:22,fontWeight:900,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:6}}>{STATUS_PAGE.name}</div>
              <div style={{fontSize:18,fontWeight:700,color:stC[overall],marginBottom:4}}>{stLabel[overall]}</div>
              <div style={{fontSize:12,color:'#546a88'}}>Last updated {STATUS_PAGE.lastUpdated}</div>
              {overall!=='operational'&&(
                <div style={{marginTop:10,fontSize:11,color:'#ff4d6a',background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.2)',padding:'6px 14px',borderRadius:20,display:'inline-block'}}>Active Incident — See details below</div>
              )}
            </div>

            {/* Components */}
            {grouped.map(g=>(
              <div key={g.cat} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden',marginBottom:12}}>
                <div style={{padding:'10px 16px',background:'#080d1b',borderBottom:'1px solid #1a2d4a',fontSize:11,fontWeight:800,color:'#546a88',textTransform:'uppercase',letterSpacing:'.06em'}}>{g.cat}</div>
                {g.items.map((comp,i)=>{
                  const Icon = stIcon[comp.status]||CheckCircle
                  return (
                    <div key={comp.id} style={{display:'flex',alignItems:'center',padding:'12px 16px',borderBottom:i<g.items.length-1?'1px solid rgba(26,45,74,.4)':'none',gap:12}}>
                      <Icon size={15} style={{color:stC[comp.status],flexShrink:0}}/>
                      <div style={{flex:1}}>
                        <div style={{fontSize:13,fontWeight:600,color:'#f0f6ff'}}>{comp.name}</div>
                        <div style={{fontSize:10,color:'#546a88'}}>{comp.description}</div>
                      </div>
                      {comp.autoLinked&&<span style={{fontSize:9,background:'rgba(108,114,255,.15)',color:'#8b90ff',padding:'1px 6px',borderRadius:8,fontWeight:700}}>AUTO-SYNCED</span>}
                      <div style={{textAlign:'right'}}>
                        <div style={{fontSize:12,fontWeight:700,color:stC[comp.status]}}>{stLabel[comp.status]}</div>
                        <div style={{fontSize:9,color:'#546a88'}}>30d: {comp.uptime30}%</div>
                      </div>
                    </div>
                  )
                })}
              </div>
            ))}

            {/* Active incident */}
            {incidents.filter(i=>i.status==='ongoing').map(inc=>(
              <div key={inc.id} style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.3)',borderRadius:12,padding:16,marginBottom:12}}>
                <div style={{display:'flex',gap:8,marginBottom:10}}>
                  <XCircle size={16} style={{color:'#ff4d6a',flexShrink:0}}/>
                  <div style={{flex:1}}>
                    <div style={{fontSize:14,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{inc.title}</div>
                    <div style={{fontSize:11,color:'#546a88'}}>Started {inc.startTime} · Last update {inc.updatedTime}</div>
                  </div>
                </div>
                {inc.updates.map((u,i)=>(
                  <div key={i} style={{display:'flex',gap:10,marginBottom:8,paddingLeft:24}}>
                    <div style={{fontSize:10,color:'#546a88',width:80,flexShrink:0}}>{u.time}</div>
                    <div style={{fontSize:11,color:'#c8d8ef'}}>{u.body}</div>
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}

        {tab==='manage'&&(
          <div>
            <div style={{marginBottom:12,display:'flex',gap:8,alignItems:'center'}}>
              <div style={{flex:1,fontSize:12,color:'#546a88'}}>Auto-linked components sync their status from ObserveX Problems in real-time. Manual overrides are possible.</div>
              <button style={{display:'flex',gap:5,alignItems:'center',background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}><Plus size={11}/>Add Component</button>
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <table style={{width:'100%',borderCollapse:'collapse'}}>
                <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                  {['Component','Category','Current Status','30d Uptime','Auto-Sync','Override Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
                </tr></thead>
                <tbody>
                  {components.map(c=>{
                    const Icon=stIcon[c.status]||CheckCircle
                    return (
                      <tr key={c.id} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                        <td style={{padding:'10px 12px'}}>
                          <div style={{fontSize:12,fontWeight:600,color:'#f0f6ff'}}>{c.name}</div>
                          {c.autoLinked&&<div style={{fontSize:9,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{c.linkedService}</div>}
                        </td>
                        <td style={{padding:'10px 12px',fontSize:11,color:'#546a88'}}>{c.category}</td>
                        <td style={{padding:'10px 12px'}}>
                          <div style={{display:'flex',gap:5,alignItems:'center'}}>
                            <Icon size={12} style={{color:stC[c.status]}}/>
                            <span style={{color:stC[c.status],fontSize:11,fontWeight:600}}>{stLabel[c.status]}</span>
                          </div>
                        </td>
                        <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:parseFloat(c.uptime30)>99?'#0fcf8a':'#f5a623'}}>{c.uptime30}%</td>
                        <td style={{padding:'10px 12px'}}>
                          {c.autoLinked
                            ? <span style={{fontSize:9,background:'rgba(15,207,138,.15)',color:'#0fcf8a',padding:'2px 7px',borderRadius:8,fontWeight:700}}>ENABLED</span>
                            : <span style={{fontSize:9,background:'rgba(84,106,136,.1)',color:'#546a88',padding:'2px 7px',borderRadius:8}}>MANUAL</span>}
                        </td>
                        <td style={{padding:'10px 12px'}}>
                          <select value={c.status} onChange={e=>setStatus(c.id,e.target.value)}
                            style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',borderRadius:6,padding:'4px 8px',fontSize:11,cursor:'pointer'}}>
                            {Object.entries(stLabel).map(([v,l])=><option key={v} value={v}>{l}</option>)}
                          </select>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {tab==='incidents'&&(
          <div>
            <div style={{display:'flex',gap:8,marginBottom:14}}>
              <button style={{display:'flex',gap:5,alignItems:'center',background:'#ff4d6a',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}><Plus size={11}/>Create Incident</button>
              <div style={{fontSize:11,color:'#546a88',display:'flex',alignItems:'center',gap:4}}>💡 Incidents are auto-created from Problems when you enable the integration</div>
            </div>
            {incidents.map(inc=>(
              <div key={inc.id} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${inc.status==='ongoing'?'rgba(255,77,106,.3)':'#1a2d4a'}`,borderRadius:12,padding:16,marginBottom:12}}>
                <div style={{display:'flex',gap:10,marginBottom:10}}>
                  <div style={{flex:1}}>
                    <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:4}}>
                      <span style={{background:inc.status==='ongoing'?'rgba(255,77,106,.15)':'rgba(15,207,138,.15)',color:inc.status==='ongoing'?'#ff4d6a':'#0fcf8a',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{inc.status.toUpperCase()}</span>
                      <span style={{background:'rgba(255,77,106,.1)',color:'#ff7a92',fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:700}}>{inc.severity.toUpperCase()}</span>
                    </div>
                    <div style={{fontSize:14,fontWeight:700,color:'#f0f6ff',marginBottom:4}}>{inc.title}</div>
                    <div style={{fontSize:11,color:'#546a88'}}>Started {inc.startTime} · Last update {inc.updatedTime}</div>
                  </div>
                  {inc.status==='ongoing'&&<button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Post Update</button>}
                </div>
                <div style={{borderLeft:'2px solid #1a2d4a',paddingLeft:12}}>
                  {inc.updates.map((u,i)=>(
                    <div key={i} style={{marginBottom:8,paddingBottom:8,borderBottom:i<inc.updates.length-1?'1px solid rgba(26,45,74,.3)':'none'}}>
                      <div style={{fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace',marginBottom:3}}>{u.time}</div>
                      <div style={{fontSize:12,color:'#c8d8ef'}}>{u.body}</div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
