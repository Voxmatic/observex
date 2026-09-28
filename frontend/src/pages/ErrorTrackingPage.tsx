// ErrorTrackingPage.tsx — Error groups, stack traces, issue management
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { errorsInbox } from '@/lib/api'
import { Bug, Users, Clock, CheckCircle, EyeOff, Search, Filter, ExternalLink } from 'lucide-react'
import toast from 'react-hot-toast'

const DEMO_ERRORS = [
  { id:'err-001', title:'NullPointerException in UserService.getProfile()', count:1842, users:312, service:'user-service', version:'v2.4.0', firstSeen:'2h ago', lastSeen:'12s ago', status:'unresolved', severity:'CRITICAL', freq:'high',
    stack:`java.lang.NullPointerException\n  at UserService.getProfile(UserService.java:142)\n  at UserController.handleGet(UserController.java:67)\n  at HttpHandler.dispatch(HttpHandler.java:231)`,
    breadcrumbs:[{time:'19:47:11',type:'navigation',msg:'GET /api/v1/users/8420'},{time:'19:47:12',type:'db',msg:'SELECT * FROM users WHERE id=8420 → null'},{time:'19:47:12',type:'error',msg:'NPE: user is null'}],
    assignee:null },
  { id:'err-002', title:'TimeoutError: DB connection timeout after 30000ms', count:847, users:198, service:'checkout-service', version:'v2.5.0', firstSeen:'23m ago', lastSeen:'3s ago', status:'unresolved', severity:'HIGH', freq:'high',
    stack:`TimeoutError: DB connection timeout after 30000ms\n  at ConnectionPool.acquire(pool.js:84)\n  at CheckoutRepo.findOrder(checkout.js:156)`,
    breadcrumbs:[{time:'19:30:01',type:'network',msg:'POST /api/v2/checkout/initiate'},{time:'19:30:28',type:'db',msg:'Waiting for DB connection... (28s)'},{time:'19:30:31',type:'error',msg:'Timeout: pool exhausted'}],
    assignee:'sarah.chen' },
  { id:'err-003', title:'TypeError: Cannot read properties of undefined (reading map)', count:284, users:284, service:'api-gateway', version:'v2.4.0', firstSeen:'4h ago', lastSeen:'8m ago', status:'investigating', severity:'MEDIUM', freq:'medium',
    stack:`TypeError: Cannot read properties of undefined (reading 'map')\n  at ResponseTransformer.transform(transformer.js:44)\n  at Router.handle(router.js:112)`,
    breadcrumbs:[{time:'15:42:18',type:'navigation',msg:'GET /api/v1/products'},{time:'15:42:19',type:'error',msg:'products.data is undefined'}],
    assignee:'jin.park' },
  { id:'err-004', title:'PaymentGateway: DECLINED - insufficient_funds', count:142, users:142, service:'payment-service', version:'v3.1.2', firstSeen:'1h ago', lastSeen:'2m ago', status:'unresolved', severity:'MEDIUM', freq:'low',
    stack:`PaymentError: DECLINED - insufficient_funds\n  at PaymentGateway.charge(gateway.js:44)\n  at PaymentService.processPayment(payment.svc.js:112)`,
    breadcrumbs:[{time:'18:50:11',type:'network',msg:'POST /api/v3/payments/charge'},{time:'18:50:12',type:'error',msg:'Stripe: insufficient_funds'}],
    assignee:null },
  { id:'err-005', title:'ConnectionRefusedError: Redis ECONNREFUSED 127.0.0.1:6379', count:28, users:0, service:'ml-inference', version:'v1.8.3', firstSeen:'6h ago', lastSeen:'1h ago', status:'resolved', severity:'HIGH', freq:'low',
    stack:`Error: connect ECONNREFUSED 127.0.0.1:6379\n  at TCPConnectWrap.afterConnect\n  at RedisClient.connect(redis.js:211)`,
    breadcrumbs:[{time:'13:20:00',type:'network',msg:'Redis connection attempt'},{time:'13:20:01',type:'error',msg:'ECONNREFUSED'}],
    assignee:'platform-team' },
]

const SEV_C: Record<string,string> = { CRITICAL:'#ff4d6a', HIGH:'#f5a623', MEDIUM:'#a855f7', LOW:'#546a88' }
const STAT_C: Record<string,string> = { unresolved:'#ff4d6a', investigating:'#f5a623', resolved:'#0fcf8a', ignored:'#546a88' }

export default function ErrorTrackingPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState(DEMO_ERRORS[0])
  const [statusFilter, setStatusFilter] = useState<'ALL'|'unresolved'|'resolved'|'investigating'>('ALL')
  const [search, setSearch] = useState('')
  const [errors, setErrors] = useState(DEMO_ERRORS)

  const { data } = useQuery({
    queryKey: ['error-groups', statusFilter],
    queryFn: async () => { try { return await errorsInbox.list({ status: statusFilter === 'ALL' ? undefined : statusFilter }) } catch { return { groups: DEMO_ERRORS } } },
    staleTime: 15_000,
  })

  const allErrors: typeof DEMO_ERRORS = (data as any)?.groups ?? errors
  const shown = allErrors.filter(e =>
    (statusFilter === 'ALL' || e.status === statusFilter) &&
    (!search || e.title.toLowerCase().includes(search.toLowerCase()))
  )

  const setStatus = (id: string, status: string) => {
    setErrors(e => e.map(x => x.id === id ? { ...x, status } : x))
    toast.success(`Error ${status}`)
  }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'14px 20px 10px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Error Tracking</div>
          <div style={{fontSize:12,color:'#546a88',flex:1}}>Cross-service error groups · Stack traces · Breadcrumbs · Assignment · Resolution</div>
        </div>
        <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:8}}>
          {(['ALL','unresolved','investigating','resolved'] as const).map(f=>(
            <button key={f} onClick={()=>setStatusFilter(f)} style={{padding:'4px 12px',border:`1px solid ${statusFilter===f?(STAT_C[f]||'#6c72ff'):'#1a2d4a'}`,borderRadius:20,background:statusFilter===f?`${STAT_C[f]||'#6c72ff'}18`:'transparent',color:statusFilter===f?STAT_C[f]||'#8b90ff':'#546a88',fontSize:10,fontWeight:600,cursor:'pointer'}}>{f}</button>
          ))}
          <div style={{marginLeft:'auto',display:'flex',gap:6,alignItems:'center'}}>
            <Search size={11} style={{color:'#546a88'}}/>
            <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search errors..."
              style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:7,padding:'4px 10px',fontSize:11,color:'#c8d8ef',outline:'none',width:200}}/>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total Groups',v:allErrors.length,c:'#6c72ff'},{l:'Unresolved',v:allErrors.filter(e=>e.status==='unresolved').length,c:'#ff4d6a'},{l:'Total Occurrences',v:allErrors.reduce((a,e)=>a+e.count,0).toLocaleString(),c:'#f5a623'},{l:'Affected Users',v:allErrors.reduce((a,e)=>a+e.users,0).toLocaleString(),c:'#8b90ff'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 12px'}}>
              <div style={{fontSize:8,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Error list */}
        <div style={{width:340,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {shown.map(e=>(
            <div key={e.id} onClick={()=>setSelected(e)}
              style={{padding:'11px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===e.id?SEV_C[e.severity]:'transparent'}`,background:selected.id===e.id?'rgba(108,114,255,.04)':'transparent'}}>
              <div style={{display:'flex',gap:6,marginBottom:5}}>
                <Bug size={11} style={{color:SEV_C[e.severity],flexShrink:0,marginTop:1}}/>
                <div style={{flex:1,fontSize:11,fontWeight:700,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace',lineHeight:1.3,overflow:'hidden',textOverflow:'ellipsis',display:'-webkit-box',WebkitLineClamp:2,WebkitBoxOrient:'vertical'}}>{e.title}</div>
              </div>
              <div style={{display:'flex',gap:8,fontSize:10,color:'#546a88',marginBottom:3}}>
                <span style={{color:SEV_C[e.severity],fontWeight:700}}>{e.severity}</span>
                <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{e.service}</span>
                <span style={{color:STAT_C[e.status],fontWeight:700}}>{e.status}</span>
              </div>
              <div style={{display:'flex',gap:10,fontSize:9,color:'#3d5070'}}>
                <span>⚡ {e.count.toLocaleString()} occurrences</span>
                <span>👤 {e.users}</span>
                <span>last: {e.lastSeen}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Error detail */}
        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:`${SEV_C[selected.severity]}08`,border:`1px solid ${SEV_C[selected.severity]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:10,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:6,lineHeight:1.3}}>{selected.title}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                  <span style={{background:`${SEV_C[selected.severity]}22`,color:SEV_C[selected.severity],fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{selected.severity}</span>
                  <span style={{color:'#6c72ff',fontSize:11,fontFamily:'JetBrains Mono,monospace'}}>{selected.service} {selected.version}</span>
                  <span style={{background:`${STAT_C[selected.status]}22`,color:STAT_C[selected.status],fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:10}}>{selected.status}</span>
                  {selected.assignee&&<span style={{fontSize:10,color:'#546a88'}}>Assigned: <strong style={{color:'#8fa8cc'}}>{selected.assignee}</strong></span>}
                </div>
              </div>
              <div style={{display:'flex',gap:7,flexShrink:0}}>
                {selected.status!=='resolved'&&<button onClick={()=>setStatus(selected.id,'resolved')} style={{display:'flex',gap:4,alignItems:'center',background:'rgba(15,207,138,.1)',border:'1px solid rgba(15,207,138,.25)',color:'#0fcf8a',padding:'6px 12px',borderRadius:8,fontSize:11,cursor:'pointer'}}><CheckCircle size={11}/>Resolve</button>}
                {selected.status!=='ignored'&&<button onClick={()=>setStatus(selected.id,'ignored')} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#546a88',padding:'6px 10px',borderRadius:8,fontSize:11,cursor:'pointer'}}><EyeOff size={11}/></button>}
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:8}}>
              {[{l:'Occurrences',v:selected.count.toLocaleString(),c:'#ff4d6a'},{l:'Affected Users',v:selected.users,c:'#f5a623'},{l:'First Seen',v:selected.firstSeen,c:'#8fa8cc'},{l:'Last Seen',v:selected.lastSeen,c:'#0fcf8a'}].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'7px 10px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:14,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Stack trace */}
          <div style={{background:'#080d1b',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden',marginBottom:14}}>
            <div style={{padding:'9px 14px',borderBottom:'1px solid #1a2d4a',fontSize:11,fontWeight:700,color:'#c8d8ef',background:'#0b1628',display:'flex',gap:8,alignItems:'center'}}>
              <Bug size={12} style={{color:'#ff4d6a'}}/>Stack Trace
            </div>
            <pre style={{padding:14,margin:0,fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#ff7a92',lineHeight:1.7,overflowX:'auto',whiteSpace:'pre-wrap'}}>{selected.stack}</pre>
          </div>

          {/* Breadcrumbs */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
            <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Breadcrumbs</div>
            {selected.breadcrumbs.map((b,i)=>(
              <div key={i} style={{display:'flex',gap:10,padding:'6px 0',borderBottom:i<selected.breadcrumbs.length-1?'1px solid rgba(26,45,74,.3)':'none'}}>
                <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88',flexShrink:0}}>{b.time}</span>
                <span style={{background:b.type==='error'?'rgba(255,77,106,.15)':'rgba(108,114,255,.1)',color:b.type==='error'?'#ff4d6a':'#8b90ff',fontSize:9,padding:'1px 6px',borderRadius:8,fontWeight:700,flexShrink:0}}>{b.type}</span>
                <span style={{fontSize:11,color:'#8fa8cc',fontFamily:b.type==='error'?'JetBrains Mono,monospace':'inherit'}}>{b.msg}</span>
              </div>
            ))}
          </div>

          <div style={{display:'flex',gap:8}}>
            <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>🔗 Create Jira Ticket</button>
            <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'7px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View APM Traces</button>
          </div>
        </div>
      </div>
    </div>
  )
}
