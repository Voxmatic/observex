// OverviewPage.tsx — Platform command center
// Real-time health across all 52 pages of the platform
import { useState, useEffect } from 'react'
import { AlertTriangle, Activity, CheckCircle, Zap, Brain, Server, Database, Globe, TrendingUp, TrendingDown, Shield, Clock, Users, GitBranch, BarChart3, Cpu, MemoryStick, HardDrive } from 'lucide-react'

// Generate sparkline data
const spark = (base: number, variance: number, n = 24) =>
  Array.from({length: n}, (_, i) => ({
    t: i,
    v: Math.max(0, base + (Math.random() - 0.5) * variance * 2)
  }))

// Mini spark SVG
function Spark({ data, color, h = 36 }: { data: {v:number}[], color: string, h?: number }) {
  const max = Math.max(...data.map(d=>d.v)) || 1
  const min = Math.min(...data.map(d=>d.v))
  const w = 120
  const pts = data.map((d,i) => {
    const x = (i/(data.length-1))*w
    const y = h - ((d.v - min)/(max - min || 1)) * (h - 6) - 3
    return `${x},${y}`
  }).join(' ')
  const area = `${0},${h} ${pts} ${w},${h}`
  return (
    <svg width={w} height={h} style={{overflow:'visible'}}>
      <defs>
        <linearGradient id={`g${color.replace('#','')}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.25"/>
          <stop offset="100%" stopColor={color} stopOpacity="0"/>
        </linearGradient>
      </defs>
      <polygon points={area} fill={`url(#g${color.replace('#','')})`}/>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round"/>
    </svg>
  )
}

// Gauge arc
function Gauge({ val, max=100, color, size=64 }: {val:number,max?:number,color:string,size?:number}) {
  const r = (size-10)/2, cx=size/2, cy=size/2
  const pct = val/max
  const circ = Math.PI * r
  return (
    <svg width={size} height={size/2+8} style={{overflow:'visible'}}>
      <path d={`M ${cx-r} ${cy} A ${r} ${r} 0 0 1 ${cx+r} ${cy}`} fill="none" stroke="#1a2d4a" strokeWidth="8" strokeLinecap="round"/>
      <path d={`M ${cx-r} ${cy} A ${r} ${r} 0 0 1 ${cx+r} ${cy}`} fill="none" stroke={color} strokeWidth="8" strokeLinecap="round"
        strokeDasharray={`${circ*pct} ${circ}`}/>
      <text x={cx} y={cy+2} textAnchor="middle" fill={color} fontSize="13" fontWeight="800" fontFamily="Syne,sans-serif">{val}%</text>
    </svg>
  )
}

const SERVICES = [
  { name:'api-gateway',     rps:2140, p99:284,  err:1.84, cpu:34, status:'warning',  trend:'+12%' },
  { name:'checkout-service',rps:284,  p99:892,  err:3.20, cpu:62, status:'warning',  trend:'+8%'  },
  { name:'user-service',    rps:1240, p99:4200, err:8.40, cpu:48, status:'critical', trend:'+420%'},
  { name:'payment-service', rps:78,   p99:98,   err:0.08, cpu:18, status:'healthy',  trend:'-12%' },
  { name:'ml-inference',    rps:48,   p99:1240, err:0.72, cpu:94, status:'warning',  trend:'+280%'},
  { name:'notification-svc',rps:312,  p99:45,   err:0.04, cpu:12, status:'healthy',  trend: '-4%' },
]

const statusC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a', degraded:'#f5a623' }

export default function OverviewPage() {
  const [tick, setTick] = useState(0)
  const [sparkData] = useState({
    requests: spark(2800, 400),
    errors:   spark(2.1, 0.8),
    latency:  spark(284, 80),
    cpu:      spark(48, 15),
  })

  useEffect(() => {
    const t = setInterval(() => setTick(x => x+1), 3000)
    return () => clearInterval(t)
  }, [])

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflowY:'auto'}}>
      {/* Header */}
      <div style={{padding:'16px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:12,marginBottom:12}}>
          <div>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Platform Overview</div>
            <div style={{fontSize:12,color:'#546a88'}}>Real-time health across all services · Last updated: just now</div>
          </div>
          <div style={{marginLeft:'auto',display:'flex',gap:8,alignItems:'center'}}>
            <div style={{display:'flex',alignItems:'center',gap:6,padding:'5px 12px',background:'rgba(255,77,106,.08)',border:'1px solid rgba(255,77,106,.25)',borderRadius:8}}>
              <div style={{width:7,height:7,borderRadius:'50%',background:'#ff4d6a',animation:'pulse 2s infinite'}}/>
              <span style={{fontSize:11,color:'#ff4d6a',fontWeight:700}}>2 Active Problems</span>
            </div>
            <div style={{display:'flex',alignItems:'center',gap:6,padding:'5px 12px',background:'rgba(15,207,138,.08)',border:'1px solid rgba(15,207,138,.25)',borderRadius:8}}>
              <div style={{width:7,height:7,borderRadius:'50%',background:'#0fcf8a'}}/>
              <span style={{fontSize:11,color:'#0fcf8a',fontWeight:700}}>AI Agent: ACTIVE</span>
            </div>
          </div>
        </div>

        {/* Top KPIs */}
        <div style={{display:'grid',gridTemplateColumns:'repeat(6,1fr)',gap:10}}>
          {[
            {l:'Total RPS', v:'4,102', delta:'+8.4%', up:true, c:'#6c72ff', data:sparkData.requests, icon:Activity},
            {l:'Error Rate', v:'2.84%', delta:'+68%', up:false, c:'#ff4d6a', data:sparkData.errors, icon:AlertTriangle},
            {l:'P99 Latency', v:'892ms', delta:'+180ms', up:false, c:'#f5a623', data:sparkData.latency, icon:Clock},
            {l:'CPU Cluster', v:'48%', delta:'+12%', up:false, c:'#a855f7', data:sparkData.cpu, icon:Cpu},
            {l:'Active SLOs', v:'8/12 ✓', delta:'4 burning', up:false, c:'#ff4d6a', data:sparkData.errors, icon:Shield},
            {l:'AI Decisions', v:'14', delta:'9 auto-fixed', up:true, c:'#0fcf8a', data:sparkData.requests, icon:Brain},
          ].map(k => {
            const Icon = k.icon
            return (
              <div key={k.l} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${k.up||k.l==='AI Decisions'?'rgba(15,207,138,.2)':'rgba(255,77,106,.15)'}`,borderRadius:12,padding:'12px 14px',position:'relative',overflow:'hidden'}}>
                <div style={{display:'flex',alignItems:'center',gap:6,marginBottom:5}}>
                  <Icon size={12} style={{color:k.c}}/>
                  <span style={{fontSize:10,color:'#546a88'}}>{k.l}</span>
                </div>
                <div style={{fontSize:19,fontWeight:900,color:k.c,fontFamily:'Syne,sans-serif',marginBottom:4}}>{k.v}</div>
                <div style={{fontSize:10,color:k.up?'#0fcf8a':'#f5a623',fontWeight:600}}>{k.delta}</div>
                <div style={{position:'absolute',right:0,bottom:0,opacity:.5}}>
                  <Spark data={k.data} color={k.c} h={32}/>
                </div>
              </div>
            )
          })}
        </div>
      </div>

      <div style={{flex:1,padding:'16px 20px',display:'flex',flexDirection:'column',gap:14}}>
        {/* Problem alert banner */}
        <div style={{background:'rgba(255,77,106,.05)',border:'1px solid rgba(255,77,106,.3)',borderRadius:12,padding:'12px 16px',display:'flex',alignItems:'center',gap:12}}>
          <div style={{width:36,height:36,borderRadius:10,background:'rgba(255,77,106,.15)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:16,flexShrink:0}}>🔴</div>
          <div style={{flex:1}}>
            <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:2}}>🔴 CRITICAL: Database connection pool exhaustion — postgres-primary</div>
            <div style={{fontSize:11,color:'#546a88'}}>Affecting 4,200 users · $847/min revenue impact · Root cause confirmed (94%) · Causal chain: postgres-primary → user-service → checkout-service → api-gateway</div>
          </div>
          <div style={{display:'flex',gap:8}}>
            <a href="/problems" style={{display:'flex',alignItems:'center',gap:5,background:'#ff4d6a',color:'#fff',padding:'6px 14px',borderRadius:8,fontSize:11,fontWeight:700,textDecoration:'none'}}>View Problem</a>
            <button style={{background:'rgba(108,114,255,.15)',border:'1px solid rgba(108,114,255,.3)',color:'#8b90ff',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>🧠 AI Suggest Fix</button>
          </div>
        </div>

        <div style={{display:'grid',gridTemplateColumns:'1fr 1fr 1fr',gap:14}}>
          {/* Services health */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden',gridColumn:'span 2'}}>
            <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',display:'flex',alignItems:'center',gap:10}}>
              <BarChart3 size={14} style={{color:'#6c72ff'}}/>
              <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Services Health</span>
              <span style={{marginLeft:'auto',fontSize:10,color:'#546a88'}}>{SERVICES.filter(s=>s.status==='healthy').length}/{SERVICES.length} healthy</span>
            </div>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead>
                <tr style={{borderBottom:'1px solid #1a2d4a'}}>
                  {['Service','Status','RPS','P99','Error Rate','CPU'].map(h => (
                    <th key={h} style={{padding:'7px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {SERVICES.map(s => (
                  <tr key={s.name} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontWeight:700,color:'#f0f6ff',fontSize:11}}>{s.name}</td>
                    <td style={{padding:'9px 12px'}}>
                      <div style={{display:'flex',alignItems:'center',gap:5}}>
                        <div style={{width:6,height:6,borderRadius:'50%',background:statusC[s.status]}}/>
                        <span style={{color:statusC[s.status],fontSize:11,fontWeight:600}}>{s.status}</span>
                      </div>
                    </td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{s.rps}/s</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:s.p99>500?'#f5a623':'#8fa8cc',fontWeight:s.p99>500?700:400}}>{s.p99}ms</td>
                    <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:s.err>2?'#ff4d6a':s.err>0.5?'#f5a623':'#0fcf8a',fontWeight:700}}>{s.err}%</td>
                    <td style={{padding:'9px 12px'}}>
                      <div style={{display:'flex',gap:6,alignItems:'center'}}>
                        <div style={{width:48,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                          <div style={{height:'100%',width:`${s.cpu}%`,background:s.cpu>80?'#ff4d6a':s.cpu>60?'#f5a623':'#0fcf8a'}}/>
                        </div>
                        <span style={{fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{s.cpu}%</span>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Right column: Infrastructure + AI */}
          <div style={{display:'flex',flexDirection:'column',gap:14}}>
            {/* Cluster gauges */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:'12px 14px'}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>🖥 Cluster Health</div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:8,textAlign:'center'}}>
                {[{l:'CPU',v:48,c:'#a855f7'},{l:'Memory',v:74,c:'#f5a623'},{l:'Disk',v:34,c:'#0fcf8a'}].map(g => (
                  <div key={g.l}>
                    <Gauge val={g.v} color={g.c} size={60}/>
                    <div style={{fontSize:10,color:'#546a88',marginTop:3}}>{g.l}</div>
                  </div>
                ))}
              </div>
              <div style={{marginTop:10,display:'flex',gap:8}}>
                {[{l:'Nodes',v:'12/12',c:'#0fcf8a'},{l:'Pods',v:'148/152',c:'#f5a623'},{l:'PVCs',v:'24/24',c:'#0fcf8a'}].map(k => (
                  <div key={k.l} style={{flex:1,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 8px'}}>
                    <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:13,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                  </div>
                ))}
              </div>
            </div>

            {/* AI Agent status */}
            <div style={{background:'linear-gradient(135deg,rgba(108,114,255,.1),rgba(168,85,247,.05))',border:'1px solid rgba(108,114,255,.25)',borderRadius:12,padding:'12px 14px',flex:1}}>
              <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:10}}>
                <span style={{fontSize:16}}>🧠</span>
                <span style={{fontSize:12,fontWeight:700,color:'#8b90ff'}}>AI Agent — Live</span>
                <div style={{width:7,height:7,borderRadius:'50%',background:'#0fcf8a',animation:'pulse 2s infinite',marginLeft:'auto'}}/>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:6,marginBottom:10}}>
                {[{l:'Mode',v:'SUGGEST',c:'#6c72ff'},{l:'Decisions',v:'14',c:'#8b90ff'},{l:'Auto-fixed',v:'9',c:'#0fcf8a'},{l:'Confidence',v:'87%',c:'#0fcf8a'}].map(k => (
                  <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 8px'}}>
                    <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:14,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                  </div>
                ))}
              </div>
              <div style={{background:'rgba(7,15,30,.5)',border:'1px solid rgba(245,166,35,.25)',borderRadius:8,padding:'8px 10px'}}>
                <div style={{fontSize:10,color:'#f5a623',fontWeight:700,marginBottom:3}}>⏳ Pending decision</div>
                <div style={{fontSize:11,color:'#8fa8cc'}}>notify_oncall + open_incident for postgres-primary pool exhaustion</div>
              </div>
            </div>
          </div>
        </div>

        {/* SLOs + Recent Events row */}
        <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
          {/* SLOs */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',display:'flex',alignItems:'center',gap:10}}>
              <Shield size={14} style={{color:'#6c72ff'}}/>
              <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>SLOs — Burn Rate</span>
              <span style={{marginLeft:'auto',fontSize:10,color:'#ff4d6a',fontWeight:700}}>4 burning</span>
            </div>
            <div style={{padding:'10px 14px'}}>
              {[
                {n:'API Availability',t:99.9,c:91.2,burn:18.2},
                {n:'Checkout Success Rate',t:99.9,c:96.2,burn:6.8},
                {n:'User Service P99',t:99.5,c:87.4,burn:12.1},
                {n:'Payment Success',t:99.99,c:99.91,burn:0.8},
                {n:'Synthetic Uptime',t:99.5,c:99.7,burn:0},
              ].map(s => (
                <div key={s.n} style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
                  <div style={{width:7,height:7,borderRadius:'50%',background:s.c>=s.t?'#0fcf8a':s.burn>10?'#ff4d6a':'#f5a623',flexShrink:0}}/>
                  <span style={{fontSize:11,flex:1,color:'#8fa8cc'}}>{s.n}</span>
                  <div style={{width:80,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                    <div style={{height:'100%',width:`${s.c}%`,background:s.c>=s.t?'#0fcf8a':s.burn>10?'#ff4d6a':'#f5a623'}}/>
                  </div>
                  <span style={{fontSize:11,fontWeight:800,color:s.c>=s.t?'#0fcf8a':s.burn>10?'#ff4d6a':'#f5a623',width:44,textAlign:'right',fontFamily:'JetBrains Mono,monospace'}}>{s.c}%</span>
                  {s.burn > 0 && <span style={{fontSize:9,background:'rgba(255,77,106,.15)',color:'#ff4d6a',padding:'1px 6px',borderRadius:8,fontWeight:700,flexShrink:0}}>{s.burn}x burn</span>}
                </div>
              ))}
            </div>
          </div>

          {/* Recent Deployments */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',display:'flex',alignItems:'center',gap:10}}>
              <GitBranch size={14} style={{color:'#6c72ff'}}/>
              <span style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Recent Activity</span>
            </div>
            <div style={{padding:'10px 14px'}}>
              {[
                {t:'19:47',type:'🔴',msg:'CRITICAL: DB connection pool exhausted (200/200)',c:'#ff4d6a'},
                {t:'19:45',type:'⚡',msg:'AI Agent: Scaled ml-inference 2→4 replicas (CPU 94%)',c:'#0fcf8a'},
                {t:'19:30',type:'🚀',msg:'checkout-service v2.5.0 deployed → healthy',c:'#6c72ff'},
                {t:'19:25',type:'🔔',msg:'Alert: api-gateway P99 > 1000ms',c:'#f5a623'},
                {t:'18:56',type:'↩️',msg:'Auto-rollback: api-gateway v2.4.1 → v2.4.0 (regression)',c:'#f5a623'},
                {t:'18:42',type:'🚀',msg:'api-gateway v2.4.1 deployed → REGRESSION DETECTED',c:'#ff4d6a'},
                {t:'17:08',type:'🔐',msg:'CERT WARN: payment-service TLS expiring in 7 days',c:'#a855f7'},
              ].map((e,i) => (
                <div key={i} style={{display:'flex',gap:10,alignItems:'flex-start',padding:'5px 0',borderBottom:i<6?'1px solid rgba(26,45,74,.3)':'none'}}>
                  <span style={{fontSize:11,fontFamily:'JetBrains Mono,monospace',color:'#3d5070',flexShrink:0,marginTop:1}}>{e.t}</span>
                  <span style={{fontSize:12,flexShrink:0}}>{e.type}</span>
                  <span style={{fontSize:11,color:e.c,flex:1,lineHeight:1.4}}>{e.msg}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* Platform capabilities strip */}
        <div style={{background:'rgba(17,31,53,.5)',border:'1px solid #1a2d4a',borderRadius:12,padding:'14px 16px'}}>
          <div style={{fontSize:11,fontWeight:700,color:'#546a88',textTransform:'uppercase',letterSpacing:'.06em',marginBottom:12}}>Platform Capabilities — ObserveX Unique Features vs Dynatrace / Datadog / New Relic</div>
          <div style={{display:'grid',gridTemplateColumns:'repeat(5,1fr)',gap:10}}>
            {[
              {f:'OQL Query Language',d:'Unified metrics+logs+traces+events in one syntax',u:true},
              {f:'Built-in LLM Monitor',d:'Claude, GPT-4o, Gemini, Ollama in one platform',u:true},
              {f:'AI Agent Zero Egress',d:'Ollama local LLM — no data leaves your network',u:true},
              {f:'Chaos Engineering',d:'Built-in free (competitors charge extra)',u:true},
              {f:'Compliance Reports',d:'SOC2/HIPAA/GDPR with SHA-256 attestation',u:true},
            ].map(f => (
              <div key={f.f} style={{background:'rgba(108,114,255,.06)',border:'1px solid rgba(108,114,255,.2)',borderRadius:10,padding:'10px 12px'}}>
                <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:5}}>
                  <span style={{fontSize:10,fontWeight:800,color:'#6c72ff',background:'rgba(108,114,255,.2)',padding:'1px 6px',borderRadius:8}}>★ UNIQUE</span>
                </div>
                <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{f.f}</div>
                <div style={{fontSize:10,color:'#546a88',lineHeight:1.4}}>{f.d}</div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
