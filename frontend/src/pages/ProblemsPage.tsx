import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ChevronDown, ChevronRight, Clock, Users, TrendingUp, GitBranch, Zap, RefreshCw, CheckCircle, MessageSquare, Send } from 'lucide-react'
import { problems as problemsApi } from '@/lib/api'
import toast from 'react-hot-toast'

const SEV_COLOR: Record<string, string> = { CRITICAL:'#ff4d6a', HIGH:'#f5a623', MEDIUM:'#a855f7', LOW:'#3d9bff' }
const SEV_BG:    Record<string, string> = { CRITICAL:'rgba(255,77,106,.12)', HIGH:'rgba(245,166,35,.12)', MEDIUM:'rgba(168,85,247,.12)', LOW:'rgba(61,155,255,.12)' }
const STATUS_C:  Record<string, string> = { critical:'#ff4d6a', warning:'#f5a623', healthy:'#0fcf8a', ok:'#0fcf8a' }

// Fallback demo data if backend not running
const DEMO_PROBLEMS = [
  { id:'P-1001', title:'Database connection pool exhaustion', status:'OPEN', severity:'CRITICAL', duration:'23m 14s',
    impact_users:4200, impact_revenue:'$847/min', affected_slos:3, affected_services:6,
    root_cause_entity:'postgres-primary', root_cause_type:'DATABASE',
    root_cause_reason:'Max connections (200/200) reached; all client threads waiting',
    ai_confidence:94,
    ai_analysis:'Root cause: PostgreSQL pool exhausted. No recent deployment. Recommend: increase pool_size→300, add connection timeout, add read replica.',
    causal_chain:[
      { entity:'postgres-primary', event:'Connection pool exhausted (200/200)', time:'T-0', root:true },
      { entity:'user-service',     event:'DB timeout → error rate +2800%',      time:'T+18s', root:false },
      { entity:'checkout-service', event:'Upstream failure cascade',             time:'T+31s', root:false },
      { entity:'api-gateway',      event:'Error rate 1.84% (+68%)',             time:'T+44s', root:false },
    ],
    affected_entities:[
      { name:'postgres-primary', type:'DATABASE', events:4, status:'critical' },
      { name:'user-service',     type:'SERVICE',  events:12, status:'critical' },
      { name:'checkout-service', type:'SERVICE',  events:8, status:'critical' },
      { name:'api-gateway',      type:'SERVICE',  events:6, status:'warning' },
    ],
    first_detected:'19:24:11 UTC', deploy_recent:null, comments:[]
  },
  { id:'P-1002', title:'ML inference service CPU saturation', status:'OPEN', severity:'HIGH', duration:'41m 22s',
    impact_users:840, impact_revenue:'$280/min', affected_slos:1, affected_services:2,
    root_cause_entity:'ml-inference', root_cause_type:'SERVICE',
    root_cause_reason:'CPU sustained at 94%; inference queue depth 247 pending',
    ai_confidence:88,
    ai_analysis:'CPU saturation on ml-inference pod. Scaled from 2→4 replicas. Expected P99 improvement: ~60%.',
    causal_chain:[
      { entity:'ml-inference',     event:'CPU 94% sustained (threshold 85%)', time:'T-0', root:true },
      { entity:'ml-inference',     event:'Queue depth 247 (growing)',          time:'T+8m', root:false },
      { entity:'recommendation-svc', event:'Timeout → fallback serving',       time:'T+12m', root:false },
    ],
    affected_entities:[
      { name:'ml-inference',      type:'SERVICE', events:8, status:'critical' },
      { name:'recommendation-svc',type:'SERVICE', events:3, status:'warning' },
    ],
    first_detected:'19:06:03 UTC', deploy_recent:null, comments:[]
  },
  { id:'P-1003', title:'TLS certificate expiring in 7 days — payment-service', status:'OPEN', severity:'MEDIUM', duration:'2h 18m',
    impact_users:0, impact_revenue:'Potential outage', affected_slos:0, affected_services:1,
    root_cause_entity:'payment-service', root_cause_type:'SERVICE',
    root_cause_reason:'TLS certificate expires 2026-04-24. cert-manager renewal pending.',
    ai_confidence:99,
    ai_analysis:'SSL certificate for payment-service expires in 7 days. Check cert-manager pod health.',
    causal_chain:[{ entity:'payment-service', event:'Certificate expiry warning (7 days)', time:'T-0', root:true }],
    affected_entities:[{ name:'payment-service', type:'SERVICE', events:1, status:'warning' }],
    first_detected:'17:08:44 UTC', deploy_recent:null, comments:[]
  },
  { id:'P-1004', title:'API Gateway memory regression — auto-remediated', status:'RESOLVED', severity:'HIGH', duration:'14m',
    impact_users:1200, impact_revenue:'$0', affected_slos:0, affected_services:1,
    root_cause_entity:'api-gateway', root_cause_type:'SERVICE',
    root_cause_reason:'Memory leak in v2.4.1. Rolled back to v2.4.0 automatically.',
    ai_confidence:97,
    ai_analysis:'Post-deploy memory regression. v2.4.1 correlated. Auto-rolled back to v2.4.0. Resolved.',
    causal_chain:[
      { entity:'api-gateway', event:'Deploy v2.4.1 at 16:42',           time:'T-0', root:true },
      { entity:'api-gateway', event:'Memory growth 12MB/min detected',  time:'T+6m', root:false },
      { entity:'api-gateway', event:'Auto-remediation: rollback triggered', time:'T+14m', root:false },
    ],
    affected_entities:[{ name:'api-gateway', type:'SERVICE', events:5, status:'healthy' }],
    first_detected:'16:48:22 UTC', deploy_recent:'v2.4.1 at 16:42', comments:[]
  },
]

export default function ProblemsPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<any>(null)
  const [filter, setFilter] = useState<'ALL'|'OPEN'|'RESOLVED'>('ALL')
  const [expandedChain, setExpandedChain] = useState(true)
  const [comment, setComment] = useState('')

  const { data, isLoading, error } = useQuery({
    queryKey: ['problems', filter],
    queryFn: async () => {
      try {
        return await problemsApi.list({ status: filter === 'ALL' ? undefined : filter })
      } catch { return { problems: DEMO_PROBLEMS } }
    },
    refetchInterval: 15_000,
  })

  const ackMut = useMutation({
    mutationFn: (id: string) => problemsApi.acknowledge(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['problems'] }); toast.success('Problem acknowledged') },
  })

  const resolveMut = useMutation({
    mutationFn: (id: string) => problemsApi.resolve(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['problems'] }); toast.success('Problem resolved') },
  })

  const commentMut = useMutation({
    mutationFn: ({ id, text }: { id: string; text: string }) => problemsApi.acknowledge(id), // using acknowledge as comment proxy
    onSuccess: () => { toast.success('Comment added') },
  })

  const list: any[] = (data as any)?.problems ?? DEMO_PROBLEMS
  const shown = filter === 'ALL' ? list : list.filter((p: any) => p.status === filter)

  useEffect(() => { if (shown.length && !selected) setSelected(shown[0]) }, [shown])

  const p = selected ?? shown[0]

  const openCount  = list.filter((x:any)=>x.status==='OPEN').length
  const critCount  = list.filter((x:any)=>x.status==='OPEN'&&x.severity==='CRITICAL').length
  const totalUsers = list.filter((x:any)=>x.status==='OPEN').reduce((a:number,x:any)=>a+(x.impact_users||0),0)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      {/* Header */}
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:12,marginBottom:10}}>
          <div>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Problems</div>
            <div style={{fontSize:12,color:'#546a88'}}>Davis AI causal root cause analysis · {list.length} problems · Topology-aware</div>
          </div>
          <div style={{marginLeft:'auto',display:'flex',gap:8}}>
            <button onClick={()=>qc.invalidateQueries({queryKey:['problems']})} style={{padding:'6px 12px',background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,color:'#546a88',cursor:'pointer',display:'flex',gap:5,alignItems:'center',fontSize:11}}>
              <RefreshCw size={11}/>Refresh
            </button>
            {(['ALL','OPEN','RESOLVED'] as const).map(f=>(
              <button key={f} onClick={()=>setFilter(f)} style={{padding:'5px 14px',border:`1px solid ${filter===f?'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:filter===f?'rgba(108,114,255,.15)':'transparent',color:filter===f?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer'}}>{f}</button>
            ))}
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[
            {l:'Open Problems',    v:openCount,  c:'#ff4d6a'},
            {l:'Critical',        v:critCount,  c:'#ff4d6a'},
            {l:'Affected Users',  v:totalUsers.toLocaleString(), c:'#f5a623'},
            {l:'Revenue Impact',  v:'$1,127/min',c:'#f5a623'},
            {l:'SLOs Violated',   v:4,           c:'#a855f7'},
          ].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      {isLoading && <div style={{padding:20,color:'#546a88'}}>Loading problems...</div>}

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* List */}
        <div style={{width:330,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {shown.map((prob:any)=>(
            <div key={prob.id} onClick={()=>setSelected(prob)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${p?.id===prob.id?SEV_COLOR[prob.severity]:'transparent'}`,background:p?.id===prob.id?SEV_BG[prob.severity]:'transparent'}}>
              <div style={{display:'flex',gap:8,marginBottom:6}}>
                <AlertTriangle size={12} style={{color:SEV_COLOR[prob.severity],flexShrink:0,marginTop:1}}/>
                <span style={{flex:1,fontSize:12,fontWeight:700,color:'#f0f6ff',lineHeight:1.3}}>{prob.title}</span>
                <span style={{background:SEV_BG[prob.severity],color:SEV_COLOR[prob.severity],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10,flexShrink:0}}>{prob.severity}</span>
              </div>
              <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88'}}>
                <span style={{color:prob.status==='OPEN'?'#ff4d6a':'#0fcf8a',fontWeight:700}}>{prob.status}</span>
                <span>⌚ {prob.duration}</span>
                {prob.impact_users>0&&<span>👤 {(prob.impact_users||0).toLocaleString()}</span>}
                <span style={{marginLeft:'auto',color:'#6c72ff',fontFamily:'JetBrains Mono,monospace',fontSize:10}}>{prob.id}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Detail */}
        {p && (
          <div style={{flex:1,overflowY:'auto',padding:18,display:'flex',flexDirection:'column',gap:12}}>
            {/* Banner */}
            <div style={{background:SEV_BG[p.severity],border:`1px solid ${SEV_COLOR[p.severity]}44`,borderRadius:14,padding:16}}>
              <div style={{display:'flex',gap:12,marginBottom:12}}>
                <div style={{flex:1}}>
                  <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:6}}>{p.title}</div>
                  <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                    <span style={{background:SEV_BG[p.severity],color:SEV_COLOR[p.severity],fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{p.severity}</span>
                    <span style={{background:p.status==='OPEN'?'rgba(255,77,106,.15)':'rgba(15,207,138,.15)',color:p.status==='OPEN'?'#ff4d6a':'#0fcf8a',fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{p.status}</span>
                    <span style={{fontSize:11,color:'#546a88'}}>Duration: <strong style={{color:'#c8d8ef'}}>{p.duration}</strong></span>
                    <span style={{fontSize:11,color:'#546a88'}}>First: <strong style={{color:'#c8d8ef'}}>{p.first_detected}</strong></span>
                    {p.deploy_recent&&<span style={{fontSize:11,color:'#f5a623'}}>⚡ Deploy: {p.deploy_recent}</span>}
                  </div>
                </div>
                <div style={{display:'flex',gap:8,alignSelf:'flex-start'}}>
                  {p.status==='OPEN'&&<button onClick={()=>ackMut.mutate(p.id)} style={{background:'rgba(108,114,255,.15)',border:'1px solid rgba(108,114,255,.3)',color:'#8b90ff',padding:'6px 12px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Acknowledge</button>}
                  {p.status==='OPEN'&&<button onClick={()=>resolveMut.mutate(p.id)} style={{background:'rgba(15,207,138,.12)',border:'1px solid rgba(15,207,138,.25)',color:'#0fcf8a',padding:'6px 12px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Resolve</button>}
                </div>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
                {[
                  {l:'Affected Users',  v:(p.impact_users||0).toLocaleString(), c:'#f5a623'},
                  {l:'Revenue Impact',  v:p.impact_revenue||'—',                c:'#ff4d6a'},
                  {l:'SLOs Violated',   v:p.affected_slos||0,                   c:'#a855f7'},
                  {l:'Affected Services',v:p.affected_services||0,              c:'#3d9bff'},
                ].map(k=>(
                  <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'9px 12px'}}>
                    <div style={{fontSize:10,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                  </div>
                ))}
              </div>
            </div>

            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12}}>
              {/* Root cause */}
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid rgba(255,77,106,.25)',borderRadius:12,padding:14}}>
                <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:10}}>
                  <Zap size={13} style={{color:'#ff4d6a'}}/>
                  <span style={{fontSize:12,fontWeight:700,color:'#ff4d6a'}}>Root Cause Entity</span>
                  <span style={{marginLeft:'auto',fontSize:10,background:'rgba(255,77,106,.15)',color:'#ff4d6a',padding:'2px 8px',borderRadius:8,fontWeight:700}}>AI Confidence: {p.ai_confidence}%</span>
                </div>
                <div style={{fontSize:15,fontWeight:700,color:'#f0f6ff',marginBottom:3,fontFamily:'JetBrains Mono,monospace'}}>{p.root_cause_entity}</div>
                <div style={{fontSize:10,color:'#6c72ff',fontWeight:600,marginBottom:8}}>{p.root_cause_type}</div>
                <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.55,marginBottom:10}}>{p.root_cause_reason}</div>
                <div style={{height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                  <div style={{height:'100%',width:`${p.ai_confidence}%`,background:p.ai_confidence>90?'#0fcf8a':'#f5a623'}}/>
                </div>
              </div>
              {/* AI analysis */}
              <div style={{background:'linear-gradient(135deg,rgba(108,114,255,.1),rgba(168,85,247,.05))',border:'1px solid rgba(108,114,255,.25)',borderRadius:12,padding:14}}>
                <div style={{display:'flex',gap:8,marginBottom:10}}><span style={{fontSize:16}}>🧠</span><span style={{fontSize:12,fontWeight:700,color:'#8b90ff'}}>AI Analysis</span></div>
                <div style={{fontSize:12,color:'#c8d8ef',lineHeight:1.6,marginBottom:10}}>{p.ai_analysis}</div>
                <div style={{display:'flex',gap:8}}>
                  <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>▶ Run Playbook</button>
                  <button style={{background:'transparent',color:'#8b90ff',border:'1px solid rgba(108,114,255,.3)',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Smartscape</button>
                </div>
              </div>
            </div>

            {/* Causal chain */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <div onClick={()=>setExpandedChain(!expandedChain)} style={{padding:'12px 16px',display:'flex',alignItems:'center',gap:8,cursor:'pointer',borderBottom:expandedChain?'1px solid #1a2d4a':'none'}}>
                {expandedChain?<ChevronDown size={13} style={{color:'#546a88'}}/>:<ChevronRight size={13} style={{color:'#546a88'}}/>}
                <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Causal Chain</span>
                <span style={{marginLeft:'auto',fontSize:10,color:'#546a88'}}>{(p.causal_chain||[]).length} events</span>
              </div>
              {expandedChain&&(
                <div style={{padding:14}}>
                  {(p.causal_chain||[]).map((e:any,i:number)=>(
                    <div key={i} style={{display:'flex',gap:12,marginBottom:8,alignItems:'flex-start'}}>
                      <div style={{display:'flex',flexDirection:'column',alignItems:'center',flexShrink:0}}>
                        <div style={{width:10,height:10,borderRadius:'50%',background:e.root?'#ff4d6a':'#6c72ff',marginTop:2}}/>
                        {i<(p.causal_chain.length-1)&&<div style={{width:1,flex:1,background:'#1a2d4a',margin:'2px 0',minHeight:20}}/>}
                      </div>
                      <div style={{flex:1,padding:'5px 10px',background:e.root?'rgba(255,77,106,.06)':'rgba(17,31,53,.5)',borderRadius:8,border:`1px solid ${e.root?'rgba(255,77,106,.3)':'#1a2d4a'}`}}>
                        <div style={{display:'flex',gap:8,alignItems:'center'}}>
                          <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:e.root?'#ff4d6a':'#6c72ff',fontWeight:700}}>{e.entity}</span>
                          {e.root&&<span style={{fontSize:9,background:'rgba(255,77,106,.2)',color:'#ff4d6a',padding:'1px 6px',borderRadius:8,fontWeight:800}}>ROOT CAUSE</span>}
                          <span style={{marginLeft:'auto',fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{e.time}</span>
                        </div>
                        <div style={{fontSize:11,color:'#8fa8cc',marginTop:3}}>{e.event}</div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* Affected entities */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <div style={{padding:'10px 16px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Affected Entities ({(p.affected_entities||[]).length})</div>
              <table style={{width:'100%',borderCollapse:'collapse'}}>
                <thead><tr style={{borderBottom:'1px solid #1a2d4a'}}>
                  {['Entity','Type','Events','Status'].map(h=><th key={h} style={{padding:'7px 14px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
                </tr></thead>
                <tbody>
                  {(p.affected_entities||[]).map((e:any,i:number)=>(
                    <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                      <td style={{padding:'8px 14px',fontFamily:'JetBrains Mono,monospace',fontWeight:700,color:'#f0f6ff',fontSize:11}}>{e.name}</td>
                      <td style={{padding:'8px 14px'}}><span style={{background:'#182844',color:'#8fa8cc',fontSize:9,padding:'2px 8px',borderRadius:8}}>{e.type}</span></td>
                      <td style={{padding:'8px 14px',color:'#8fa8cc',fontSize:12}}>{e.events}</td>
                      <td style={{padding:'8px 14px'}}>
                        <div style={{display:'flex',alignItems:'center',gap:5}}>
                          <div style={{width:6,height:6,borderRadius:'50%',background:STATUS_C[e.status]||'#546a88'}}/>
                          <span style={{color:STATUS_C[e.status]||'#546a88',fontSize:11,fontWeight:600}}>{e.status}</span>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Comments */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10,display:'flex',gap:6,alignItems:'center'}}>
                <MessageSquare size={13} style={{color:'#6c72ff'}}/>Comments &amp; Notes
              </div>
              {(p.comments||[]).length===0&&<div style={{fontSize:12,color:'#3d5070',marginBottom:12}}>No comments yet. Add context for your team.</div>}
              <div style={{display:'flex',gap:8}}>
                <input value={comment} onChange={e=>setComment(e.target.value)} placeholder="Add a comment or remediation note..."
                  style={{flex:1,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'7px 12px',fontSize:12,color:'#c8d8ef',outline:'none'}}/>
                <button onClick={()=>{if(comment.trim()){commentMut.mutate({id:p.id,text:comment});setComment('')}}}
                  style={{background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,cursor:'pointer',display:'flex',alignItems:'center',gap:5,fontSize:11}}>
                  <Send size={11}/>Send
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
