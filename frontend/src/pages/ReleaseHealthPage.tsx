// ReleaseHealthPage.tsx — Crash-free rate per release, version adoption, release health score
import { useState } from 'react'
import { Package, TrendingUp, TrendingDown, Users, AlertTriangle, CheckCircle, BarChart3, GitBranch } from 'lucide-react'

const RELEASES = [
  { version:'v2.5.0', service:'checkout-service', env:'production', deployedAt:'19:30', adopters:71200, totalUsers:284000, adoption:25, crashFreeUsers:98.4, crashFreeSessions:98.8, newIssues:3, regressions:1, score:74, trend:'degrading', status:'degraded',
    sessions:142400, crashedSessions:1712, p99:892, errorRate:3.2, commits:8 },
  { version:'v2.4.0', service:'checkout-service', env:'production', deployedAt:'2d ago', adopters:212800, totalUsers:284000, adoption:75, crashFreeUsers:99.6, crashFreeSessions:99.7, newIssues:1, regressions:0, score:96, trend:'stable', status:'healthy',
    sessions:425600, crashedSessions:1277, p99:640, errorRate:1.1, commits:12 },
  { version:'v2.0.0', service:'api-gateway', env:'production', deployedAt:'14d ago', adopters:284000, totalUsers:284000, adoption:100, crashFreeUsers:99.2, crashFreeSessions:99.3, newIssues:0, regressions:0, score:99, trend:'stable', status:'healthy',
    sessions:2840000, crashedSessions:19880, p99:284, errorRate:1.84, commits:24 },
  { version:'v1.8.3', service:'ml-inference', env:'production', deployedAt:'6h ago', adopters:48000, totalUsers:48000, adoption:100, crashFreeUsers:97.8, crashFreeSessions:98.1, newIssues:2, regressions:0, score:82, trend:'improving', status:'warning',
    sessions:96000, crashedSessions:1824, p99:1240, errorRate:0.72, commits:5 },
  { version:'v3.1.2', service:'payment-service', env:'production', deployedAt:'8h ago', adopters:78000, totalUsers:78000, adoption:100, crashFreeUsers:99.95, crashFreeSessions:99.96, newIssues:0, regressions:0, score:100, trend:'stable', status:'healthy',
    sessions:156000, crashedSessions:62, p99:98, errorRate:0.08, commits:3 },
  { version:'v2.3.9', service:'user-service', env:'production', deployedAt:'12h ago', adopters:289400, totalUsers:289400, adoption:100, crashFreeUsers:91.2, crashFreeSessions:91.8, newIssues:8, regressions:3, score:42, trend:'degrading', status:'critical',
    sessions:578800, crashedSessions:47461, p99:4200, errorRate:8.4, commits:6 },
]

const scoreC = (s:number) => s>90?'#0fcf8a':s>70?'#f5a623':s>50?'#ff7a92':'#ff4d6a'
const stC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', degraded:'#f5a623', critical:'#ff4d6a' }

function ScoreRing({ score, size=52 }: { score:number, size?:number }) {
  const r=size/2-5, circ=2*Math.PI*r, dash=circ*(score/100)
  const color = scoreC(score)
  return (
    <svg width={size} height={size}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="#1a2d4a" strokeWidth="5"/>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={color} strokeWidth="5"
        strokeDasharray={`${dash} ${circ}`} strokeLinecap="round" transform={`rotate(-90 ${size/2} ${size/2})`}/>
      <text x={size/2} y={size/2+4} textAnchor="middle" fill={color} fontSize="13" fontWeight="900" fontFamily="Syne,sans-serif">{score}</text>
    </svg>
  )
}

function AdoptionBar({ pct, color }: { pct:number, color:string }) {
  return (
    <div style={{display:'flex',gap:6,alignItems:'center'}}>
      <div style={{flex:1,height:6,background:'#0b1628',borderRadius:3,overflow:'hidden'}}>
        <div style={{height:'100%',width:`${pct}%`,background:color,borderRadius:3}}/>
      </div>
      <span style={{fontSize:10,fontWeight:700,color,width:32,textAlign:'right'}}>{pct}%</span>
    </div>
  )
}

export default function ReleaseHealthPage() {
  const [selected, setSelected] = useState(RELEASES[RELEASES.findIndex(r=>r.service==='user-service')])
  const [svcFilter, setSvcFilter] = useState('All')
  const services = ['All',...new Set(RELEASES.map(r=>r.service))]
  const shown = svcFilter==='All'?RELEASES:RELEASES.filter(r=>r.service===svcFilter)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🏥</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Release Health</div>
            <div style={{fontSize:12,color:'#546a88'}}>Crash-free rate · Version adoption · Health score · New issues per release · APM correlation</div>
          </div>
        </div>
        <div style={{display:'flex',gap:6,flexWrap:'wrap',marginBottom:8}}>
          {services.map(s=>(
            <button key={s} onClick={()=>setSvcFilter(s)} style={{padding:'4px 12px',border:`1px solid ${svcFilter===s?'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:svcFilter===s?'rgba(108,114,255,.15)':'transparent',color:svcFilter===s?'#8b90ff':'#546a88',fontSize:11,cursor:'pointer'}}>{s}</button>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:340,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {shown.map(r=>(
            <div key={`${r.service}-${r.version}`} onClick={()=>setSelected(r)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.version===r.version&&selected.service===r.service?stC[r.status]:'transparent'}`,background:selected.version===r.version&&selected.service===r.service?'rgba(108,114,255,.04)':'transparent'}}>
              <div style={{display:'flex',gap:10,alignItems:'flex-start',marginBottom:8}}>
                <ScoreRing score={r.score} size={44}/>
                <div style={{flex:1}}>
                  <div style={{fontSize:13,fontWeight:800,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace'}}>{r.version}</div>
                  <div style={{fontSize:10,color:'#6c72ff',marginBottom:3}}>{r.service}</div>
                  <div style={{fontSize:10,color:'#546a88'}}>{r.deployedAt}</div>
                </div>
                <div style={{width:7,height:7,borderRadius:'50%',background:stC[r.status],marginTop:4}}/>
              </div>
              <div style={{display:'flex',gap:10,fontSize:10,marginBottom:6}}>
                <span style={{color:r.crashFreeUsers>99?'#0fcf8a':r.crashFreeUsers>95?'#f5a623':'#ff4d6a',fontWeight:700}}>👤 {r.crashFreeUsers}% crash-free</span>
                {r.regressions>0&&<span style={{color:'#ff4d6a',fontWeight:700}}>↓ {r.regressions} regression{r.regressions>1?'s':''}</span>}
              </div>
              <AdoptionBar pct={r.adoption} color={stC[r.status]}/>
              <div style={{display:'flex',gap:6,marginTop:6}}>
                {r.newIssues>0&&<span style={{fontSize:9,background:'rgba(255,77,106,.15)',color:'#ff4d6a',padding:'1px 6px',borderRadius:8,fontWeight:700}}>{r.newIssues} new issues</span>}
                {r.regressions>0&&<span style={{fontSize:9,background:'rgba(245,166,35,.15)',color:'#f5a623',padding:'1px 6px',borderRadius:8,fontWeight:700}}>{r.regressions} regressions</span>}
              </div>
            </div>
          ))}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:`${stC[selected.status]}08`,border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:16,alignItems:'flex-start',marginBottom:14}}>
              <ScoreRing score={selected.score} size={72}/>
              <div style={{flex:1}}>
                <div style={{fontSize:18,fontWeight:900,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:3}}>{selected.service} <span style={{fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{selected.version}</span></div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap',fontSize:11,marginBottom:6}}>
                  <span style={{color:'#546a88'}}>deployed {selected.deployedAt}</span>
                  <span style={{color:'#546a88'}}>{selected.commits} commits</span>
                  <span style={{background:`${stC[selected.status]}22`,color:stC[selected.status],padding:'2px 8px',borderRadius:8,fontWeight:700}}>{selected.status.toUpperCase()}</span>
                </div>
                {selected.status==='critical'&&<div style={{padding:'6px 10px',background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.3)',borderRadius:8,fontSize:11,color:'#ff4d6a'}}>⚠ This release is causing significant crashes. Consider rollback to previous stable version.</div>}
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
              {[
                {l:'Crash-Free Users',  v:`${selected.crashFreeUsers}%`,   c:selected.crashFreeUsers>99?'#0fcf8a':selected.crashFreeUsers>95?'#f5a623':'#ff4d6a'},
                {l:'Crash-Free Sessions',v:`${selected.crashFreeSessions}%`,c:selected.crashFreeSessions>99?'#0fcf8a':'#f5a623'},
                {l:'Adopters',          v:selected.adopters.toLocaleString(),c:'#6c72ff'},
                {l:'Adoption Rate',     v:`${selected.adoption}%`,          c:'#8b90ff'},
                {l:'Total Sessions',    v:(selected.sessions/1000).toFixed(0)+'K', c:'#8fa8cc'},
                {l:'Crashed Sessions',  v:selected.crashedSessions.toLocaleString(), c:selected.crashedSessions>10000?'#ff4d6a':'#f5a623'},
                {l:'P99 Latency',       v:`${selected.p99}ms`,              c:selected.p99>1000?'#ff4d6a':selected.p99>400?'#f5a623':'#0fcf8a'},
                {l:'Error Rate',        v:`${selected.errorRate}%`,          c:selected.errorRate>2?'#ff4d6a':selected.errorRate>0.5?'#f5a623':'#0fcf8a'},
              ].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12,marginBottom:14}}>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>New Issues in this Release</div>
              {selected.newIssues===0?(
                <div style={{fontSize:12,color:'#0fcf8a',textAlign:'center',padding:'20px 0'}}>✓ No new issues introduced</div>
              ):(
                Array(Math.min(selected.newIssues,4)).fill(0).map((_,i)=>(
                  <div key={i} style={{padding:'6px 0',borderBottom:'1px solid rgba(26,45,74,.4)',fontSize:11,color:'#8fa8cc'}}>
                    <div style={{color:'#ff4d6a',fontFamily:'JetBrains Mono,monospace',fontSize:10}}>#{1001+i} NullPointerException in {selected.service}</div>
                    <div style={{fontSize:9,color:'#546a88'}}>{Math.floor(Math.random()*200)+20} occurrences · first seen in this release</div>
                  </div>
                ))
              )}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Crash-Free Rate Trend</div>
              <div style={{display:'flex',flexDirection:'column',gap:4}}>
                {[100,99.8,99.6,selected.crashFreeUsers+0.4,selected.crashFreeUsers+0.2,selected.crashFreeUsers].map((v,i)=>(
                  <div key={i} style={{display:'flex',gap:8,alignItems:'center',fontSize:10}}>
                    <span style={{color:'#546a88',width:32}}>{i===0?'-5h':i===5?'Now':`-${5-i}h`}</span>
                    <div style={{flex:1,height:14,background:'#0b1628',borderRadius:3,overflow:'hidden',position:'relative'}}>
                      <div style={{height:'100%',width:`${v-90}%`,background:v>99?'#0fcf8a':v>95?'#f5a623':'#ff4d6a',opacity:.8}}/>
                      <span style={{position:'absolute',right:4,top:'50%',transform:'translateY(-50%)',fontSize:9,color:'#c8d8ef',fontWeight:700}}>{v.toFixed(1)}%</span>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div style={{display:'flex',gap:8}}>
            <button style={{flex:1,background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.3)',color:'#ff4d6a',padding:'8px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>🔄 Rollback to Previous</button>
            <button style={{flex:1,background:'rgba(108,114,255,.15)',border:'1px solid rgba(108,114,255,.3)',color:'#8b90ff',padding:'8px',borderRadius:8,fontSize:11,cursor:'pointer'}}>📊 View APM Impact</button>
            <button style={{flex:1,background:'rgba(15,207,138,.1)',border:'1px solid rgba(15,207,138,.25)',color:'#0fcf8a',padding:'8px',borderRadius:8,fontSize:11,cursor:'pointer'}}>📋 Create Postmortem</button>
          </div>
        </div>
      </div>
    </div>
  )
}
