// UptimePage.tsx — Global uptime monitoring with distributed checkpoints
import { useState } from 'react'
import { Plus, Globe, CheckCircle, XCircle, Clock, AlertTriangle, Wifi } from 'lucide-react'
import toast from 'react-hot-toast'

const CHECKS = [
  { id:'u1', name:'API Health Endpoint', url:'https://api.acme.io/health', interval:'60s', status:'up', uptime30:99.84, uptime90:99.91, lastCheck:'12s ago', latency:124, regions:['us-east','eu-west','ap-south'], incidents:1 },
  { id:'u2', name:'Checkout Flow', url:'https://app.acme.io/checkout', interval:'5m', status:'up', uptime30:99.12, uptime90:99.48, lastCheck:'2m ago', latency:842, regions:['us-east','eu-west'], incidents:3 },
  { id:'u3', name:'Payment Gateway', url:'https://api.acme.io/v3/payments/health', interval:'30s', status:'up', uptime30:99.98, uptime90:99.99, lastCheck:'8s ago', latency:89, regions:['us-east','eu-west','ap-south','us-west'], incidents:0 },
  { id:'u4', name:'Admin Dashboard', url:'https://admin.acme.io', interval:'5m', status:'down', uptime30:98.44, uptime90:99.12, lastCheck:'48s ago', latency:0, regions:['us-east'], incidents:5 },
  { id:'u5', name:'ML Inference API', url:'https://ml-api.acme.io/v1/health', interval:'2m', status:'degraded', uptime30:97.8, uptime90:98.4, lastCheck:'1m ago', latency:4200, regions:['us-east','eu-west'], incidents:4 },
  { id:'u6', name:'CDN Assets', url:'https://cdn.acme.io/index.js', interval:'10m', status:'up', uptime30:100, uptime90:100, lastCheck:'5m ago', latency:28, regions:['us-east','eu-west','ap-south','us-west','sa-east'], incidents:0 },
]

const REGIONS = [
  { id:'us-east',  label:'🇺🇸 US East',   lat:38, lng:-77 },
  { id:'eu-west',  label:'🇪🇺 EU West',   lat:53, lng:-6  },
  { id:'ap-south', label:'🇮🇳 AP South',  lat:19, lng:73  },
  { id:'us-west',  label:'🇺🇸 US West',   lat:37, lng:-122},
  { id:'sa-east',  label:'🇧🇷 SA East',   lat:-23,lng:-46 },
]

const stC: Record<string,string> = { up:'#0fcf8a', down:'#ff4d6a', degraded:'#f5a623' }
const stLabel: Record<string,string> = { up:'Up', down:'Down', degraded:'Degraded' }

// 90-day uptime history bar
function UptimeBar({ uptime }: { uptime: number }) {
  const bars = Array.from({length:90},(_,i)=>{
    const dayUptime = i>85?(Math.random()>0.3?1:0):i>80?(Math.random()>0.15?1:0):1
    return dayUptime
  })
  return (
    <div style={{display:'flex',gap:1}} title={`${uptime}% uptime (90d)`}>
      {bars.map((v,i)=><div key={i} style={{width:3,height:20,background:v?'#0fcf8a':'#ff4d6a',borderRadius:1,opacity:.7}}/>)}
    </div>
  )
}

export default function UptimePage() {
  const [selected, setSelected] = useState(CHECKS[0])
  const up = CHECKS.filter(c=>c.status==='up').length
  const down = CHECKS.filter(c=>c.status==='down').length

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#0fcf8a,#6c72ff)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>📡</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Uptime Monitor</div>
            <div style={{fontSize:12,color:'#546a88'}}>Global endpoint monitoring · {REGIONS.length} regions · SSL tracking · Incident auto-creation · History</div>
          </div>
          <button onClick={()=>toast.success('Uptime check created')} style={{display:'flex',gap:5,alignItems:'center',background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}><Plus size={12}/>Add Check</button>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total Checks',v:CHECKS.length,c:'#6c72ff'},{l:'Up',v:up,c:'#0fcf8a'},{l:'Down',v:down,c:down>0?'#ff4d6a':'#0fcf8a'},{l:'Avg Uptime (30d)',v:`${(CHECKS.reduce((a,c)=>a+c.uptime30,0)/CHECKS.length).toFixed(2)}%`,c:'#8fa8cc'},{l:'Monitoring Regions',v:REGIONS.length,c:'#8b90ff'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:320,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {CHECKS.map(c=>{
            const Icon = c.status==='up'?CheckCircle:c.status==='down'?XCircle:AlertTriangle
            return (
              <div key={c.id} onClick={()=>setSelected(c)}
                style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===c.id?stC[c.status]:'transparent'}`,background:selected.id===c.id?`${stC[c.status]}08`:'transparent'}}>
                <div style={{display:'flex',gap:8,marginBottom:5,alignItems:'center'}}>
                  <Icon size={13} style={{color:stC[c.status],flexShrink:0}}/>
                  <span style={{flex:1,fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{c.name}</span>
                  <span style={{background:`${stC[c.status]}22`,color:stC[c.status],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{stLabel[c.status]}</span>
                </div>
                <div style={{fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace',marginBottom:5,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{c.url}</div>
                <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginBottom:5}}>
                  <span>⏱ {c.interval}</span>
                  <span style={{color:c.latency>1000?'#f5a623':c.latency===0?'#ff4d6a':'#0fcf8a'}}>{c.latency>0?`${c.latency}ms`:'Unreachable'}</span>
                  <span>{c.uptime30}% (30d)</span>
                </div>
                <UptimeBar uptime={c.uptime30}/>
              </div>
            )
          })}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:`${stC[selected.status]}08`,border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:12,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                <div style={{fontSize:11,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace',marginBottom:6}}>{selected.url}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                  <span style={{background:`${stC[selected.status]}22`,color:stC[selected.status],fontSize:11,fontWeight:700,padding:'3px 10px',borderRadius:20}}>{stLabel[selected.status].toUpperCase()}</span>
                  <span style={{fontSize:11,color:'#546a88'}}>interval: {selected.interval}</span>
                  <span style={{fontSize:11,color:'#546a88'}}>last check: {selected.lastCheck}</span>
                </div>
              </div>
              <div style={{textAlign:'right'}}>
                <div style={{fontSize:24,fontWeight:900,color:selected.latency>1000?'#f5a623':selected.latency===0?'#ff4d6a':'#0fcf8a',fontFamily:'Syne,sans-serif'}}>{selected.latency>0?`${selected.latency}ms`:'Down'}</div>
                <div style={{fontSize:10,color:'#546a88'}}>Response time</div>
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
              {[{l:'30d Uptime',v:`${selected.uptime30}%`,c:selected.uptime30>99?'#0fcf8a':'#f5a623'},{l:'90d Uptime',v:`${selected.uptime90}%`,c:'#8fa8cc'},{l:'Incidents',v:selected.incidents,c:selected.incidents>0?'#ff4d6a':'#0fcf8a'},{l:'Regions',v:selected.regions.length,c:'#6c72ff'}].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Regions status */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Monitoring Regions — Live Status</div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:8}}>
              {REGIONS.map(region=>{
                const isMonitored = selected.regions.includes(region.id)
                const isDown = selected.status==='down'
                const latency = isMonitored?(selected.latency + Math.round(Math.random()*50-25)):null
                return (
                  <div key={region.id} style={{background:isMonitored?`${isDown?'rgba(255,77,106,.08)':'rgba(15,207,138,.06)'}`:' rgba(7,15,30,.5)',border:`1px solid ${isMonitored?(isDown?'rgba(255,77,106,.3)':'rgba(15,207,138,.2)'):'#1a2d4a'}`,borderRadius:10,padding:'10px 12px'}}>
                    <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:4}}>
                      <div style={{width:6,height:6,borderRadius:'50%',background:isMonitored?(isDown?'#ff4d6a':'#0fcf8a'):'#3d5070'}}/>
                      <span style={{fontSize:12}}>{region.label}</span>
                    </div>
                    {isMonitored
                      ? <div style={{fontSize:11,color:isDown?'#ff4d6a':latency&&latency>1000?'#f5a623':'#0fcf8a',fontWeight:700}}>{isDown?'Unreachable':`${latency}ms`}</div>
                      : <div style={{fontSize:10,color:'#3d5070'}}>Not monitored</div>}
                  </div>
                )
              })}
            </div>
          </div>

          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>90-Day Uptime History</div>
            <UptimeBar uptime={selected.uptime90}/>
            <div style={{display:'flex',justifyContent:'space-between',fontSize:9,color:'#3d5070',marginTop:4}}>
              <span>90 days ago</span><span>60 days ago</span><span>30 days ago</span><span>Today</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
