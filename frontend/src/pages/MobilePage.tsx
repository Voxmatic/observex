// MobilePage.tsx — Mobile app monitoring: iOS + Android, crashes, sessions, network
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { mobile } from '@/lib/api'
import { Smartphone, AlertTriangle, Activity, Wifi, Users, Clock, TrendingDown, TrendingUp, ChevronRight, BarChart3 } from 'lucide-react'

const DEMO_APPS = [
  { id: 'ios-shop', name: 'ShopApp iOS', platform: 'iOS', version: '4.2.1', users: 142800, crashes: 247, crashRate: 0.173, sessions: 312400, sessionDuration: 4.2, httpErrors: 1.84, status: 'warning', icon: '🍎', buildNum: '1842' },
  { id: 'android-shop', name: 'ShopApp Android', platform: 'Android', version: '4.2.0', users: 289400, crashes: 814, crashRate: 0.281, sessions: 628100, sessionDuration: 3.8, httpErrors: 2.14, status: 'warning', icon: '🤖', buildNum: '2041' },
  { id: 'ios-driver', name: 'Driver iOS', platform: 'iOS', version: '2.1.4', users: 18400, crashes: 12, crashRate: 0.065, sessions: 84200, sessionDuration: 22.4, httpErrors: 0.42, status: 'healthy', icon: '🍎', buildNum: '412' },
  { id: 'android-driver', name: 'Driver Android', platform: 'Android', version: '2.1.3', users: 32100, crashes: 28, crashRate: 0.087, sessions: 148600, sessionDuration: 19.8, httpErrors: 0.61, status: 'healthy', icon: '🤖', buildNum: '589' },
]

const DEMO_CRASHES = [
  { id: 'cr1', title: 'NullPointerException: UserProfileManager.fetchUser()', count: 142, users: 89, version: '4.2.1', os: 'iOS 17.4', firstSeen: '2h ago', lastSeen: '4m ago', status: 'unresolved' },
  { id: 'cr2', title: 'NetworkOnMainThreadException: HttpClient.execute()', count: 98, users: 76, version: '4.2.0', os: 'Android 14', firstSeen: '6h ago', lastSeen: '12m ago', status: 'unresolved' },
  { id: 'cr3', title: 'IndexOutOfBoundsException: CartAdapter.getItem()', count: 67, users: 54, version: '4.2.0', os: 'Android 13', firstSeen: '1d ago', lastSeen: '1h ago', status: 'unresolved' },
  { id: 'cr4', title: 'EXC_BAD_ACCESS KERN_INVALID_ADDRESS at ImageLoader.swift:284', count: 43, users: 38, version: '4.2.1', os: 'iOS 16.7', firstSeen: '3d ago', lastSeen: '8h ago', status: 'investigating' },
  { id: 'cr5', title: 'java.lang.OutOfMemoryError: Failed to allocate 4MB', count: 31, users: 28, version: '4.1.9', os: 'Android 12', firstSeen: '5d ago', lastSeen: '2d ago', status: 'resolved' },
]

const NETWORK_REQUESTS = [
  { url: '/api/v2/products', method: 'GET', avgTime: 284, p99: 892, errors: 3.2, calls: 12840 },
  { url: '/api/v2/cart/add', method: 'POST', avgTime: 142, p99: 640, errors: 1.1, calls: 4410 },
  { url: '/api/v1/user/profile', method: 'GET', avgTime: 4200, p99: 8400, errors: 8.4, calls: 55800 },
  { url: '/api/v3/payments/charge', method: 'POST', avgTime: 89, p99: 340, errors: 0.08, calls: 3510 },
  { url: '/api/v2/recommendations', method: 'GET', avgTime: 1240, p99: 3200, errors: 1.2, calls: 2160 },
]

const statC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }
const crashC: Record<string,string> = { unresolved:'#ff4d6a', investigating:'#f5a623', resolved:'#0fcf8a' }

function MiniSparkBar({ val, max, color }: { val: number, max: number, color: string }) {
  return <div style={{width:60,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}><div style={{height:'100%',width:`${Math.min((val/max)*100,100)}%`,background:color}}/></div>
}

export default function MobilePage() {
  const [selectedApp, setSelectedApp] = useState(DEMO_APPS[0])
  const [tab, setTab] = useState<'overview'|'crashes'|'network'|'sessions'>('overview')

  const { data: appsData } = useQuery({
    queryKey: ['mobile-apps'],
    queryFn: async () => { try { return await mobile.apps() } catch { return { apps: DEMO_APPS } } },
    staleTime: 30_000,
  })

  const apps: typeof DEMO_APPS = (appsData as any)?.apps ?? DEMO_APPS
  const selectedAppId = selectedApp?.id || apps[0]?.id

  useEffect(() => {
    const latest = apps.find(app => app.id === selectedApp.id) || apps[0]
    if (latest && latest !== selectedApp) setSelectedApp(latest)
  }, [appsData])

  const { data: crashesData } = useQuery({
    queryKey: ['mobile-crashes', selectedAppId],
    enabled: !!selectedAppId,
    queryFn: async () => { try { return await mobile.crashes(selectedAppId as string) } catch { return { crashes: DEMO_CRASHES } } },
    staleTime: 30_000,
  })

  const { data: networkData } = useQuery({
    queryKey: ['mobile-network', selectedAppId],
    enabled: !!selectedAppId,
    queryFn: async () => { try { return await mobile.network(selectedAppId as string) } catch { return { requests: NETWORK_REQUESTS } } },
    staleTime: 30_000,
  })

  const crashes: typeof DEMO_CRASHES = (crashesData as any)?.crashes ?? DEMO_CRASHES
  const networkRequests: typeof NETWORK_REQUESTS = (networkData as any)?.requests ?? NETWORK_REQUESTS

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      {/* Header */}
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>📱</div>
          <div>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Mobile Monitoring</div>
            <div style={{fontSize:12,color:'#546a88'}}>iOS + Android · Crash analytics · Session tracking · HTTP performance · Core metrics</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[
            {l:'Total Apps',      v:apps.length,                                           c:'#6c72ff'},
            {l:'Active Users',    v:(apps.reduce((a,x)=>a+x.users,0)/1000).toFixed(0)+'K', c:'#8b90ff'},
            {l:'Total Crashes',   v:apps.reduce((a,x)=>a+x.crashes,0).toLocaleString(),    c:'#ff4d6a'},
            {l:'Avg Crash Rate',  v:(apps.reduce((a,x)=>a+x.crashRate,0)/apps.length).toFixed(3)+'%', c:'#f5a623'},
            {l:'Avg Session Dur', v:(apps.reduce((a,x)=>a+x.sessionDuration,0)/apps.length).toFixed(1)+'m', c:'#0fcf8a'},
          ].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* App list */}
        <div style={{width:240,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {apps.map(app=>(
            <div key={app.id} onClick={()=>setSelectedApp(app)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selectedApp.id===app.id?statC[app.status]:'transparent'}`,background:selectedApp.id===app.id?'rgba(108,114,255,.05)':'transparent'}}>
              <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:6}}>
                <span style={{fontSize:20}}>{app.icon}</span>
                <div style={{flex:1}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{app.name}</div>
                  <div style={{fontSize:10,color:'#546a88'}}>{app.platform} · v{app.version}</div>
                </div>
                <div style={{width:7,height:7,borderRadius:'50%',background:statC[app.status]}}/>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:5}}>
                {[{l:'Users',v:(app.users/1000).toFixed(0)+'K',c:'#8b90ff'},{l:'Crashes',v:app.crashes.toLocaleString(),c:'#ff4d6a'}].map(k=>(
                  <div key={k.l} style={{background:'#0b1628',borderRadius:6,padding:'4px 6px'}}>
                    <div style={{fontSize:8,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:12,fontWeight:700,color:k.c}}>{k.v}</div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>

        {/* Detail */}
        <div style={{flex:1,display:'flex',flexDirection:'column',overflow:'hidden'}}>
          {/* App header + tabs */}
          <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
            <div style={{display:'flex',alignItems:'center',gap:12,marginBottom:10}}>
              <span style={{fontSize:28}}>{selectedApp.icon}</span>
              <div style={{flex:1}}>
                <div style={{fontSize:15,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{selectedApp.name}</div>
                <div style={{fontSize:11,color:'#546a88'}}>v{selectedApp.version} · Build {selectedApp.buildNum} · {selectedApp.platform}</div>
              </div>
              <span style={{background:`${statC[selectedApp.status]}22`,color:statC[selectedApp.status],fontSize:11,fontWeight:700,padding:'4px 12px',borderRadius:20}}>{selectedApp.status.toUpperCase()}</span>
            </div>
            <div style={{display:'flex',gap:0,borderBottom:'1px solid #1a2d4a'}}>
              {(['overview','crashes','network','sessions'] as const).map(t=>(
                <button key={t} onClick={()=>setTab(t)} style={{padding:'7px 16px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
              ))}
            </div>
          </div>

          <div style={{flex:1,overflowY:'auto',padding:16}}>
            {tab==='overview'&&(
              <div style={{display:'flex',flexDirection:'column',gap:12}}>
                {/* KPIs */}
                <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
                  {[
                    {l:'Active Users',    v:selectedApp.users.toLocaleString(),              i:Users,    c:'#6c72ff'},
                    {l:'Crash-Free Rate', v:`${(100-selectedApp.crashRate).toFixed(2)}%`,    i:AlertTriangle, c:selectedApp.crashRate<0.2?'#0fcf8a':'#f5a623'},
                    {l:'Avg Session',     v:`${selectedApp.sessionDuration}m`,               i:Clock,    c:'#8b90ff'},
                    {l:'HTTP Error Rate', v:`${selectedApp.httpErrors}%`,                    i:Wifi,     c:selectedApp.httpErrors>2?'#ff4d6a':'#f5a623'},
                  ].map(k=>{
                    const Icon=k.i
                    return (
                      <div key={k.l} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:'14px 16px'}}>
                        <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:6}}>
                          <Icon size={13} style={{color:k.c}}/>
                          <span style={{fontSize:10,color:'#546a88'}}>{k.l}</span>
                        </div>
                        <div style={{fontSize:22,fontWeight:900,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                      </div>
                    )
                  })}
                </div>

                {/* Platform breakdown */}
                <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>OS Version Distribution</div>
                  <div style={{display:'flex',flexDirection:'column',gap:7}}>
                    {(selectedApp.platform==='iOS'
                      ? [['iOS 17.4',38],['iOS 17.2',24],['iOS 16.7',18],['iOS 16.6',12],['Older',8]]
                      : [['Android 14',31],['Android 13',29],['Android 12',22],['Android 11',11],['Older',7]]
                    ).map(([os,pct])=>(
                      <div key={os as string} style={{display:'flex',alignItems:'center',gap:10}}>
                        <span style={{fontSize:11,width:100,color:'#8fa8cc'}}>{os}</span>
                        <div style={{flex:1,height:16,background:'#0b1628',borderRadius:4,overflow:'hidden',position:'relative'}}>
                          <div style={{height:'100%',width:`${pct}%`,background:'rgba(108,114,255,.5)'}}/>
                          <span style={{position:'absolute',left:6,top:'50%',transform:'translateY(-50%)',fontSize:9,color:'#c8d8ef',fontWeight:700}}>{pct}%</span>
                        </div>
                        <span style={{fontSize:10,color:'#546a88',width:30,textAlign:'right'}}>{Math.round((selectedApp.users*(pct as number)/100)/1000)}K</span>
                      </div>
                    ))}
                  </div>
                </div>

                {/* Apdex + Response time */}
                <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12}}>
                  <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                    <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Apdex Score (User Satisfaction)</div>
                    <div style={{textAlign:'center',marginBottom:10}}>
                      <div style={{fontSize:42,fontWeight:900,color:'#f5a623',fontFamily:'Syne,sans-serif'}}>0.74</div>
                      <div style={{fontSize:12,color:'#546a88'}}>Fair — target: &gt;0.85</div>
                    </div>
                    {[['Satisfied (&lt;2s)',62,'#0fcf8a'],['Tolerating (2–8s)',24,'#f5a623'],['Frustrated (&gt;8s)',14,'#ff4d6a']].map(([l,v,c])=>(
                      <div key={l as string} style={{display:'flex',gap:8,alignItems:'center',marginBottom:5}}>
                        <div style={{width:8,height:8,borderRadius:'50%',background:c as string}}/>
                        <span style={{fontSize:10,flex:1,color:'#8fa8cc'}}>{l}</span>
                        <div style={{width:60,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                          <div style={{height:'100%',width:`${v}%`,background:c as string}}/>
                        </div>
                        <span style={{fontSize:10,color:'#546a88',width:28,textAlign:'right'}}>{v}%</span>
                      </div>
                    ))}
                  </div>
                  <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                    <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Top Slowest Screens</div>
                    {[['Checkout Screen','4,200ms','#ff4d6a'],['Product Detail','1,840ms','#f5a623'],['Search Results','1,240ms','#f5a623'],['Cart View','840ms','#8fa8cc'],['Home Feed','420ms','#0fcf8a']].map(([s,t,c])=>(
                      <div key={s as string} style={{display:'flex',gap:8,alignItems:'center',marginBottom:7}}>
                        <span style={{fontSize:11,flex:1,color:'#8fa8cc'}}>{s}</span>
                        <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:c as string,fontWeight:700}}>{t}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {tab==='crashes'&&(
              <div>
                <div style={{display:'flex',gap:10,marginBottom:14}}>
                  {[{l:'Total Crashes',v:selectedApp.crashes,c:'#ff4d6a'},{l:'Affected Users',v:Math.round(selectedApp.users*selectedApp.crashRate/100),c:'#f5a623'},{l:'Crash Rate',v:`${selectedApp.crashRate}%`,c:'#f5a623'}].map(k=>(
                    <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
                      <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
                      <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                    </div>
                  ))}
                </div>
                <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
                  <table style={{width:'100%',borderCollapse:'collapse'}}>
                    <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                      {['Crash Title','Count','Users','OS','Version','Last Seen','Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
                    </tr></thead>
                    <tbody>
                      {crashes.map(c=>(
                        <tr key={c.id} style={{borderBottom:'1px solid rgba(26,45,74,.4)',cursor:'pointer'}}>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#f0f6ff',maxWidth:280}}>
                            <div style={{overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{c.title}</div>
                          </td>
                          <td style={{padding:'9px 12px',fontSize:12,color:'#ff4d6a',fontWeight:700}}>{c.count}</td>
                          <td style={{padding:'9px 12px',fontSize:12,color:'#f5a623'}}>{c.users}</td>
                          <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{c.os}</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{c.version}</td>
                          <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{c.lastSeen}</td>
                          <td style={{padding:'9px 12px'}}><span style={{background:`${crashC[c.status]}22`,color:crashC[c.status],fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:700}}>{c.status}</span></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {tab==='network'&&(
              <div>
                <div style={{marginBottom:14,fontSize:12,color:'#546a88'}}>HTTP requests made from the mobile app · Error rates, latency percentiles</div>
                <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
                  <table style={{width:'100%',borderCollapse:'collapse'}}>
                    <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                      {['Endpoint','Method','Avg Time','P99','Error %','Calls/h'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
                    </tr></thead>
                    <tbody>
                      {networkRequests.map((r,i)=>(
                        <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#c8d8ef'}}>{r.url}</td>
                          <td style={{padding:'9px 12px'}}><span style={{background:r.method==='GET'?'rgba(15,207,138,.15)':'rgba(108,114,255,.15)',color:r.method==='GET'?'#0fcf8a':'#8b90ff',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:6}}>{r.method}</span></td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:r.avgTime>1000?'#f5a623':'#8fa8cc'}}>{r.avgTime}ms</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:r.p99>2000?'#ff4d6a':'#8fa8cc'}}>{r.p99}ms</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:r.errors>2?'#ff4d6a':r.errors>0.5?'#f5a623':'#0fcf8a',fontWeight:700}}>{r.errors}%</td>
                          <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{r.calls.toLocaleString()}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}

            {tab==='sessions'&&(
              <div style={{display:'flex',flexDirection:'column',gap:12}}>
                <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:10}}>
                  {[{l:'Total Sessions',v:(selectedApp.sessions/1000).toFixed(0)+'K',c:'#6c72ff'},{l:'Avg Duration',v:`${selectedApp.sessionDuration}m`,c:'#8b90ff'},{l:'Sessions/User',v:(selectedApp.sessions/selectedApp.users).toFixed(1),c:'#0fcf8a'}].map(k=>(
                    <div key={k.l} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                      <div style={{fontSize:10,color:'#546a88',marginBottom:4}}>{k.l}</div>
                      <div style={{fontSize:24,fontWeight:900,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                    </div>
                  ))}
                </div>
                <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Session Duration Distribution</div>
                  {[['&lt;1m',18,'Short sessions'],['1–3m',24,'Quick browsing'],['3–10m',38,'Core user flow'],['10–30m',14,'Deep engagement'],['&gt;30m',6,'Power users']].map(([range,pct,desc])=>(
                    <div key={range as string} style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
                      <span style={{fontSize:10,width:60,color:'#8fa8cc',fontFamily:'JetBrains Mono,monospace'}}>{range}</span>
                      <div style={{flex:1,height:20,background:'#0b1628',borderRadius:4,overflow:'hidden',position:'relative'}}>
                        <div style={{height:'100%',width:`${pct}%`,background:'rgba(108,114,255,.4)'}}/>
                        <span style={{position:'absolute',left:8,top:'50%',transform:'translateY(-50%)',fontSize:10,color:'#c8d8ef',fontWeight:700}}>{pct}% — {desc}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
