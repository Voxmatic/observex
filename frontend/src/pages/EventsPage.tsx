// EventsPage.tsx — Real-time event stream: deployments, anomalies, config changes, alerts
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { events as eventsApi } from '@/lib/api'
import { Zap, Filter, RefreshCw, GitBranch, AlertTriangle, Server, Shield, Brain, Clock, Search } from 'lucide-react'

const DEMO_EVENTS = [
  { id:'e1', type:'ANOMALY', severity:'CRITICAL', source:'user-service', title:'Error rate spike detected (+2800%)', detail:'Davis AI: connection timeout cascade from postgres-primary', timestamp:'19:47:23', tags:['ai-detected','database'] },
  { id:'e2', type:'DEPLOYMENT', severity:'INFO', source:'checkout-service', title:'Deployment v2.5.0 completed', detail:'Rollout successful — 3 replicas running, all health checks passed', timestamp:'19:30:08', tags:['deployment','production'] },
  { id:'e3', type:'ALERT', severity:'HIGH', source:'ai-agent', title:'AI Agent: Scale-up executed on ml-inference', detail:'PILOT mode: Scaled 2→4 replicas (CPU was 94%). Confidence: 92%', timestamp:'19:45:11', tags:['ai-action','k8s','auto-remediated'] },
  { id:'e4', type:'CONFIG', severity:'INFO', source:'api-gateway', title:'Alert rule updated: p99-latency-threshold', detail:'jin.park changed threshold from 800ms → 1000ms', timestamp:'18:42:00', tags:['config-change','alerts'] },
  { id:'e5', type:'ANOMALY', severity:'HIGH', source:'ml-inference', title:'CPU saturation pattern detected', detail:'Rule R001: CPU 94% sustained >5m. Scale-up recommended.', timestamp:'19:41:00', tags:['cpu','auto-detected'] },
  { id:'e6', type:'SECURITY', severity:'MEDIUM', source:'auth-service', title:'Unusual login pattern: 48 failed attempts', detail:'Source IP 185.x.x.x — multiple accounts targeted. Blocked by WAF.', timestamp:'19:15:44', tags:['security','brute-force'] },
  { id:'e7', type:'DEPLOYMENT', severity:'WARN', source:'api-gateway', title:'Deploy v2.4.1 REGRESSION detected — auto-rollback', detail:'Error rate +420% within 14min. System rolled back to v2.4.0 automatically.', timestamp:'18:56:00', tags:['deployment','rollback','regression'] },
  { id:'e8', type:'SLO', severity:'HIGH', source:'slo-engine', title:'SLO burning: API Availability at 18.2x rate', detail:'Error budget will exhaust in ~40 minutes at current burn rate.', timestamp:'19:40:00', tags:['slo','burn-rate'] },
  { id:'e9', type:'INFRA', severity:'INFO', source:'k8s-controller', title:'Pod scheduling: user-svc-7f9d-xk2p1 rescheduled', detail:'Evicted from k8s-node-03 (memory pressure), rescheduled on k8s-node-01', timestamp:'19:12:00', tags:['kubernetes','pod'] },
  { id:'e10', type:'CERT', severity:'MEDIUM', source:'cert-monitor', title:'TLS certificate expiring: payment-service (7 days)', detail:'cert-manager renewal in progress. Auto-renewal expected within 24h.', timestamp:'17:08:44', tags:['security','tls','certificate'] },
  { id:'e11', type:'DEPLOYMENT', severity:'INFO', source:'payment-service', title:'Deployment v3.1.2 — healthy', detail:'Patch: improved retry logic. No regressions detected. P99 improved -8%.', timestamp:'16:30:00', tags:['deployment','production'] },
  { id:'e12', type:'ANOMALY', severity:'INFO', source:'watchdog', title:'Anomaly resolved: API latency normalised', detail:'P99 returned to 284ms baseline after DB pool recovery.', timestamp:'17:30:00', tags:['resolved','latency'] },
]

const TYPE_CONFIG: Record<string, {icon:any, color:string, bg:string}> = {
  ANOMALY:    { icon:Brain,       color:'#ff4d6a', bg:'rgba(255,77,106,.1)' },
  DEPLOYMENT: { icon:GitBranch,   color:'#6c72ff', bg:'rgba(108,114,255,.1)' },
  ALERT:      { icon:AlertTriangle,color:'#f5a623',bg:'rgba(245,166,35,.1)' },
  CONFIG:     { icon:Server,      color:'#8fa8cc', bg:'rgba(143,168,204,.1)' },
  SECURITY:   { icon:Shield,      color:'#a855f7', bg:'rgba(168,85,247,.1)' },
  SLO:        { icon:Zap,         color:'#ff4d6a', bg:'rgba(255,77,106,.1)' },
  INFRA:      { icon:Server,      color:'#8fa8cc', bg:'rgba(143,168,204,.1)' },
  CERT:       { icon:Shield,      color:'#f5a623', bg:'rgba(245,166,35,.1)' },
}

const SEV_C: Record<string,string> = { CRITICAL:'#ff4d6a', HIGH:'#f5a623', MEDIUM:'#a855f7', WARN:'#f5a623', INFO:'#546a88' }

const EVENT_TYPES = ['ALL', 'ANOMALY', 'DEPLOYMENT', 'ALERT', 'CONFIG', 'SECURITY', 'SLO', 'INFRA']

export default function EventsPage() {
  const [typeFilter, setTypeFilter] = useState('ALL')
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState<any>(DEMO_EVENTS[0])
  const [live, setLive] = useState(true)

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['events', typeFilter],
    queryFn: async () => {
      try {
        return await eventsApi.query({ q: typeFilter !== 'ALL' ? typeFilter : undefined, hours: 24, limit: 100 })
      } catch { return { events: DEMO_EVENTS } }
    },
    refetchInterval: live ? 10_000 : false,
  })

  const rawEvents: any[] = (data as any)?.events ?? DEMO_EVENTS
  const shown = rawEvents.filter(e =>
    (typeFilter === 'ALL' || e.type === typeFilter) &&
    (!search || e.title.toLowerCase().includes(search.toLowerCase()) || e.source.includes(search.toLowerCase()))
  )

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'14px 20px 10px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Events</div>
          <div style={{fontSize:12,color:'#546a88',flex:1}}>Real-time stream · Deployments · Anomalies · Config changes · Security · SLO · AI actions</div>
          <div style={{display:'flex',gap:8}}>
            <div style={{display:'flex',alignItems:'center',gap:5,cursor:'pointer',padding:'5px 10px',background:live?'rgba(15,207,138,.1)':'#0b1628',border:`1px solid ${live?'rgba(15,207,138,.3)':'#1a2d4a'}`,borderRadius:8}}
              onClick={()=>setLive(!live)}>
              <div style={{width:7,height:7,borderRadius:'50%',background:live?'#0fcf8a':'#546a88',animation:live?'pulse 2s infinite':''}}/>
              <span style={{fontSize:11,color:live?'#0fcf8a':'#546a88',fontWeight:600}}>Live</span>
            </div>
            <button onClick={()=>refetch()} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#546a88',padding:'5px 10px',borderRadius:8,cursor:'pointer',display:'flex',gap:4,alignItems:'center',fontSize:11}}>
              <RefreshCw size={11}/>Refresh
            </button>
          </div>
        </div>
        {/* Type filters */}
        <div style={{display:'flex',gap:5,marginBottom:8,flexWrap:'wrap'}}>
          {EVENT_TYPES.map(t=>{
            const cfg = TYPE_CONFIG[t]
            return (
              <button key={t} onClick={()=>setTypeFilter(t)}
                style={{padding:'4px 10px',border:`1px solid ${typeFilter===t?(cfg?.color||'#6c72ff'):'#1a2d4a'}`,borderRadius:20,background:typeFilter===t?`${cfg?.bg||'rgba(108,114,255,.1)'}`:' transparent',color:typeFilter===t?(cfg?.color||'#8b90ff'):'#546a88',fontSize:10,fontWeight:600,cursor:'pointer',display:'flex',gap:4,alignItems:'center'}}>
                {t}
              </button>
            )
          })}
          <div style={{marginLeft:'auto',display:'flex',gap:6,alignItems:'center'}}>
            <Search size={12} style={{color:'#546a88'}}/>
            <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search events..."
              style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',borderRadius:7,padding:'4px 10px',fontSize:11,outline:'none',width:180}}/>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total',v:rawEvents.length},{l:'Anomalies',v:rawEvents.filter(e=>e.type==='ANOMALY').length,c:'#ff4d6a'},{l:'Deployments',v:rawEvents.filter(e=>e.type==='DEPLOYMENT').length,c:'#6c72ff'},{l:'AI Actions',v:rawEvents.filter(e=>e.tags?.includes?.('ai-action')).length,c:'#0fcf8a'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 12px'}}>
              <div style={{fontSize:8,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:15,fontWeight:800,color:(k as any).c||'#8fa8cc',fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Event list */}
        <div style={{width:380,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {shown.map((ev:any)=>{
            const cfg = TYPE_CONFIG[ev.type] || TYPE_CONFIG.ALERT
            const Icon = cfg.icon
            return (
              <div key={ev.id} onClick={()=>setSelected(ev)}
                style={{padding:'10px 14px',cursor:'pointer',borderBottom:'1px solid rgba(26,45,74,.3)',borderLeft:`3px solid ${selected?.id===ev.id?cfg.color:'transparent'}`,background:selected?.id===ev.id?cfg.bg:'transparent',transition:'.1s'}}>
                <div style={{display:'flex',gap:8,marginBottom:4}}>
                  <Icon size={12} style={{color:cfg.color,flexShrink:0,marginTop:1}}/>
                  <span style={{flex:1,fontSize:12,fontWeight:600,color:'#f0f6ff',lineHeight:1.3}}>{ev.title}</span>
                  <span style={{fontSize:9,color:SEV_C[ev.severity]||'#546a88',fontWeight:800,flexShrink:0,background:`${SEV_C[ev.severity]||'#546a88'}18`,padding:'1px 6px',borderRadius:8}}>{ev.severity}</span>
                </div>
                <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginBottom:4,paddingLeft:20}}>
                  <span style={{fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{ev.source}</span>
                  <span>{ev.timestamp}</span>
                  <span style={{background:`${cfg.bg}`,color:cfg.color,padding:'0px 5px',borderRadius:6,fontWeight:700}}>{ev.type}</span>
                </div>
                {ev.tags&&ev.tags.length>0&&(
                  <div style={{display:'flex',gap:4,flexWrap:'wrap',paddingLeft:20}}>
                    {ev.tags.slice(0,3).map((t:string)=><span key={t} style={{fontSize:8,background:'#182844',color:'#546a88',padding:'1px 5px',borderRadius:8}}>{t}</span>)}
                  </div>
                )}
              </div>
            )
          })}
          {shown.length === 0 && (
            <div style={{padding:40,textAlign:'center',color:'#3d5070'}}>No events match your filter</div>
          )}
        </div>

        {/* Event detail */}
        {selected && (
          <div style={{flex:1,padding:18,overflowY:'auto'}}>
            {(() => {
              const cfg = TYPE_CONFIG[selected.type] || TYPE_CONFIG.ALERT
              const Icon = cfg.icon
              return (
                <>
                  <div style={{background:cfg.bg,border:`1px solid ${cfg.color}44`,borderRadius:14,padding:16,marginBottom:14}}>
                    <div style={{display:'flex',gap:12,alignItems:'flex-start',marginBottom:12}}>
                      <div style={{width:40,height:40,borderRadius:12,background:`${cfg.color}22`,display:'flex',alignItems:'center',justifyContent:'center',flexShrink:0}}>
                        <Icon size={20} style={{color:cfg.color}}/>
                      </div>
                      <div style={{flex:1}}>
                        <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:5}}>{selected.title}</div>
                        <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                          <span style={{background:`${cfg.color}22`,color:cfg.color,fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{selected.type}</span>
                          <span style={{background:`${SEV_C[selected.severity]||'#546a88'}22`,color:SEV_C[selected.severity]||'#546a88',fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{selected.severity}</span>
                          <span style={{fontSize:11,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{selected.source}</span>
                          <span style={{fontSize:11,color:'#546a88'}}>⌚ {selected.timestamp}</span>
                        </div>
                      </div>
                    </div>
                    <div style={{background:'rgba(7,15,30,.5)',borderRadius:10,padding:'10px 14px'}}>
                      <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.6}}>{selected.detail}</div>
                    </div>
                  </div>

                  {selected.tags?.length>0&&(
                    <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
                      <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:8}}>Tags</div>
                      <div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
                        {selected.tags.map((t:string)=><span key={t} style={{background:'rgba(108,114,255,.15)',color:'#8b90ff',fontSize:11,padding:'4px 10px',borderRadius:8,fontWeight:600}}>{t}</span>)}
                      </div>
                    </div>
                  )}

                  <div style={{display:'flex',gap:8}}>
                    {selected.type==='ANOMALY'&&<button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View in Problems</button>}
                    {selected.type==='DEPLOYMENT'&&<button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View in Change Tracking</button>}
                    {selected.type==='ALERT'&&<button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Alert Rule</button>}
                    <button style={{background:'rgba(108,114,255,.15)',border:'1px solid rgba(108,114,255,.3)',color:'#8b90ff',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Create Notebook Entry</button>
                  </div>
                </>
              )
            })()}
          </div>
        )}
      </div>
    </div>
  )
}
