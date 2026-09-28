// ProfilingPage.tsx — Continuous profiling: flame graph + call graph + timeline
// Inspired by Datadog Continuous Profiler but with ObserveX's unified signal correlation
import { useState } from 'react'
import { Flame, Clock, Cpu, MemoryStick, Activity, TrendingUp } from 'lucide-react'

const SERVICES = ['api-gateway','checkout-service','user-service','payment-service','ml-inference']
const PROFILE_TYPES = ['CPU','Memory','Allocations','Goroutines','Mutex']

const FLAME_DATA = [
  { name:'main.ServeHTTP', self:0, total:100, depth:0, color:'#6c72ff' },
  { name:'router.Handle', self:2, total:98, depth:1, color:'#6c72ff' },
  { name:'middleware.Auth', self:8, total:24, depth:2, color:'#f5a623' },
  { name:'jwt.Parse', self:16, total:16, depth:3, color:'#ff4d6a' },
  { name:'handler.GetUser', self:4, total:74, depth:2, color:'#6c72ff' },
  { name:'UserService.GetProfile', self:6, total:70, depth:3, color:'#6c72ff' },
  { name:'db.QueryRow', self:2, total:64, depth:4, color:'#a855f7' },
  { name:'pgx.Query', self:62, total:62, depth:5, color:'#ff4d6a' },
  { name:'cache.Get', self:18, total:18, depth:4, color:'#0fcf8a' },
  { name:'json.Marshal', self:12, total:12, depth:3, color:'#f5a623' },
]

// Build proper flame graph SVG
function FlameGraph({ data }: { data: typeof FLAME_DATA }) {
  const totalWidth = 700
  const rowH = 22
  const padding = 2
  return (
    <svg width="100%" height={FLAME_DATA.reduce((a,d)=>Math.max(a,d.depth),0)*rowH+rowH+20} viewBox={`0 0 ${totalWidth} ${FLAME_DATA.reduce((a,d)=>Math.max(a,d.depth),0)*rowH+rowH+20}`} style={{width:'100%',fontFamily:'JetBrains Mono,monospace',overflow:'visible'}}>
      {data.map((frame,i)=>{
        const w = (frame.total/100)*totalWidth - padding*2
        // Calculate x position based on preceding siblings at same depth
        const siblingsAtDepth = data.filter(f=>f.depth===frame.depth && data.indexOf(f)<i)
        const xOffset = siblingsAtDepth.reduce((a,f)=>(f.depth===frame.depth?a+(f.total/100)*totalWidth:a),0)
        const x = xOffset + padding
        const y = (FLAME_DATA.reduce((a,d)=>Math.max(a,d.depth),0) - frame.depth)*rowH + 10
        const color = frame.color
        return (
          <g key={i}>
            <rect x={x} y={y} width={Math.max(w,2)} height={rowH-padding} rx="3" fill={`${color}cc`} stroke={color} strokeWidth="0.5"/>
            {w>50&&<text x={x+4} y={y+rowH-7} fontSize="9" fill="#fff" style={{userSelect:'none'}}>{w>120?frame.name:frame.name.split('.').pop()}</text>}
            {w>80&&<text x={x+w-2} y={y+rowH-7} fontSize="8" fill="rgba(255,255,255,.6)" textAnchor="end" style={{userSelect:'none'}}>{frame.total}%</text>}
          </g>
        )
      })}
    </svg>
  )
}

function CallGraph() {
  const nodes = [
    {x:350,y:30,label:'ServeHTTP',pct:100,color:'#6c72ff'},
    {x:180,y:110,label:'Auth',pct:24,color:'#f5a623'},
    {x:500,y:110,label:'GetUser',pct:74,color:'#6c72ff'},
    {x:100,y:190,label:'jwt.Parse',pct:16,color:'#ff4d6a'},
    {x:350,y:190,label:'QueryRow',pct:64,color:'#a855f7'},
    {x:600,y:190,label:'json',pct:12,color:'#f5a623'},
    {x:250,y:270,label:'cache.Get',pct:18,color:'#0fcf8a'},
    {x:480,y:270,label:'pgx.Query',pct:62,color:'#ff4d6a'},
  ]
  const edges = [
    [0,1],[0,2],[1,3],[2,4],[2,5],[4,6],[4,7]
  ]
  return (
    <svg width="100%" height={320} viewBox="0 0 700 320" style={{width:'100%',fontFamily:'Syne,sans-serif'}}>
      {edges.map(([from,to],i)=>{
        const f=nodes[from],t=nodes[to]
        const thickness = Math.max(1,(t.pct/100)*6)
        return <line key={i} x1={f.x} y1={f.y+14} x2={t.x} y2={t.y-14} stroke="rgba(108,114,255,.4)" strokeWidth={thickness} strokeLinecap="round"/>
      })}
      {nodes.map((n,i)=>(
        <g key={i}>
          <circle cx={n.x} cy={n.y} r={14+n.pct/10} fill={`${n.color}33`} stroke={n.color} strokeWidth={1.5}/>
          <text x={n.x} y={n.y+1} textAnchor="middle" fill={n.color} fontSize="9" fontWeight="700" fontFamily="JetBrains Mono">{n.label.split('.').pop()}</text>
          <text x={n.x} y={n.y+12} textAnchor="middle" fill="rgba(255,255,255,.5)" fontSize="7">{n.pct}%</text>
        </g>
      ))}
    </svg>
  )
}

export default function ProfilingPage() {
  const [service, setService] = useState(SERVICES[2])
  const [profileType, setProfileType] = useState('CPU')
  const [view, setView] = useState<'flame'|'callgraph'|'timeline'>('flame')
  const [timeRange, setTimeRange] = useState('1h')

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#ff4d6a,#f5a623)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🔥</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Code Profiling</div>
            <div style={{fontSize:12,color:'#546a88'}}>Continuous profiling · Flame graph · Call graph · Thread timeline · CPU / Memory / Allocs / Goroutines</div>
          </div>
        </div>
        <div style={{display:'flex',gap:8,flexWrap:'wrap',marginBottom:8}}>
          <select value={service} onChange={e=>setService(e.target.value)} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#c8d8ef',borderRadius:8,padding:'6px 12px',fontSize:12}}>
            {SERVICES.map(s=><option key={s}>{s}</option>)}
          </select>
          {PROFILE_TYPES.map(t=>(
            <button key={t} onClick={()=>setProfileType(t)} style={{padding:'5px 12px',border:`1px solid ${profileType===t?'#6c72ff':'#1a2d4a'}`,borderRadius:8,background:profileType===t?'rgba(108,114,255,.15)':'transparent',color:profileType===t?'#8b90ff':'#546a88',fontSize:11,cursor:'pointer'}}>{t}</button>
          ))}
          <div style={{marginLeft:'auto',display:'flex',gap:4}}>
            {['15m','1h','6h','24h'].map(t=>(
              <button key={t} onClick={()=>setTimeRange(t)} style={{padding:'5px 10px',border:`1px solid ${timeRange===t?'#6c72ff':'#1a2d4a'}`,borderRadius:6,background:timeRange===t?'rgba(108,114,255,.15)':'transparent',color:timeRange===t?'#8b90ff':'#546a88',fontSize:10,cursor:'pointer'}}>{t}</button>
            ))}
          </div>
        </div>
        <div style={{display:'flex',gap:10,marginBottom:8}}>
          {[{l:'CPU P95',v:'12.4%',c:'#6c72ff'},{l:'Heap Used',v:'847 MB',c:'#a855f7'},{l:'Goroutines',v:'248',c:'#8fa8cc'},{l:'Alloc Rate',v:'42 MB/s',c:'#f5a623'},{l:'GC Pause P99',v:'3.2ms',c:'#0fcf8a'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['flame','callgraph','timeline'] as const).map(v=>(
            <button key={v} onClick={()=>setView(v)} style={{padding:'7px 18px',background:'none',border:'none',borderBottom:`2px solid ${view===v?'#6c72ff':'transparent'}`,color:view===v?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer'}}>{v==='flame'?'🔥 Flame Graph':v==='callgraph'?'🕸 Call Graph':'⏱ Timeline'}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {view==='flame'&&(
          <div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:12}}>
              <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>
                {profileType} Flame Graph — {service} — {timeRange}
                <span style={{marginLeft:12,fontSize:10,color:'#546a88',fontWeight:400}}>Click frame to drill down · Width = time spent · Top = call stack</span>
              </div>
              <div style={{background:'#080d1b',borderRadius:10,padding:'12px 8px',overflowX:'auto'}}>
                <FlameGraph data={FLAME_DATA}/>
              </div>
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Top Functions by Self Time</div>
              {[...FLAME_DATA].filter(f=>f.self>0).sort((a,b)=>b.self-a.self).slice(0,8).map((f,i)=>(
                <div key={i} style={{display:'flex',alignItems:'center',gap:10,marginBottom:7}}>
                  <span style={{fontSize:10,color:'#546a88',width:16,textAlign:'right'}}>{i+1}</span>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,flex:1,color:'#c8d8ef'}}>{f.name}</span>
                  <div style={{width:100,height:8,background:'#0b1628',borderRadius:4,overflow:'hidden'}}>
                    <div style={{height:'100%',width:`${f.self}%`,background:f.color}}/>
                  </div>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:f.self>30?'#ff4d6a':f.self>15?'#f5a623':'#8fa8cc',fontWeight:700,width:40,textAlign:'right'}}>{f.self}%</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {view==='callgraph'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Call Graph — node size = self time · edge thickness = time flow</div>
            <div style={{background:'#080d1b',borderRadius:10,padding:12}}>
              <CallGraph/>
            </div>
          </div>
        )}

        {view==='timeline'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Thread Timeline — {service} ({timeRange})</div>
            {['goroutine-pool-1','goroutine-pool-2','http-worker-3','http-worker-4','db-conn-pool'].map((thread,i)=>(
              <div key={i} style={{display:'flex',gap:8,alignItems:'center',marginBottom:8}}>
                <span style={{fontSize:9,color:'#546a88',width:120,flexShrink:0,fontFamily:'JetBrains Mono,monospace'}}>{thread}</span>
                <div style={{flex:1,height:18,background:'#080d1b',borderRadius:4,overflow:'hidden',display:'flex'}}>
                  {Array.from({length:60},(_,j)=>{
                    const state = j>40&&i===2?'blocking':j>50&&i===3?'blocking':Math.random()>0.15?'running':'idle'
                    return <div key={j} style={{flex:1,background:state==='running'?'#6c72ff':state==='blocking'?'#ff4d6a':'transparent',opacity:.7}}/>
                  })}
                </div>
              </div>
            ))}
            <div style={{display:'flex',gap:12,marginTop:8,fontSize:10}}>
              <span style={{display:'flex',gap:5,alignItems:'center'}}><div style={{width:12,height:8,background:'#6c72ff',borderRadius:2,opacity:.7}}/> Running</span>
              <span style={{display:'flex',gap:5,alignItems:'center'}}><div style={{width:12,height:8,background:'#ff4d6a',borderRadius:2,opacity:.7}}/> Blocking/Wait</span>
              <span style={{display:'flex',gap:5,alignItems:'center'}}><div style={{width:12,height:8,background:'#1a2d4a',borderRadius:2}}/> Idle</span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
