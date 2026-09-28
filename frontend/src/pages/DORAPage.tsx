// DORAPage.tsx — DORA metrics: Deployment Frequency, Lead Time, MTTR, Change Failure Rate
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apm } from '@/lib/api'
import { Rocket, Clock, TrendingUp, AlertTriangle, CheckCircle, ArrowUp, ArrowDown } from 'lucide-react'

const DORA_DATA = {
  deploymentFrequency: { value: 4.2, unit: '/day', grade: 'Elite', trend: '+18%', up: true, target: '> 1/day', color: '#0fcf8a' },
  leadTime:           { value: 2.4, unit: 'hours', grade: 'Elite', trend: '-22%', up: false, target: '< 1 day', color: '#0fcf8a' },
  mttr:               { value: 14,  unit: 'minutes', grade: 'Elite', trend: '-8%', up: false, target: '< 1 hour', color: '#0fcf8a' },
  changeFailureRate:  { value: 8.2, unit: '%',    grade: 'High', trend: '+2.1%', up: true, target: '< 15%', color: '#f5a623' },
}

const RECENT_DEPLOYS = [
  { service:'checkout-service', version:'v2.5.0', env:'prod', time:'19:30', author:'sara.chen',   result:'success', rollback:false, leadTime:'1h 42m' },
  { service:'api-gateway',      version:'v2.4.1', env:'prod', time:'18:42', author:'jin.park',    result:'failed',  rollback:true,  leadTime:'3h 18m' },
  { service:'api-gateway',      version:'v2.4.0', env:'prod', time:'18:56', author:'system',      result:'success', rollback:false, leadTime:'0m (rollback)' },
  { service:'payment-service',  version:'v3.1.2', env:'prod', time:'16:30', author:'david.wu',    result:'success', rollback:false, leadTime:'2h 10m' },
  { service:'ml-inference',     version:'v1.8.3', env:'prod', time:'14:15', author:'ai-team',     result:'success', rollback:false, leadTime:'4h 32m' },
  { service:'user-service',     version:'v2.3.9', env:'prod', time:'11:00', author:'eng-team',    result:'success', rollback:false, leadTime:'1h 58m' },
  { service:'notification-svc', version:'v1.4.2', env:'prod', time:'09:20', author:'ops-team',    result:'success', rollback:false, leadTime:'52m' },
]

// Sparkline data for 30 days
const DEPLOY_TREND = [2,3,4,3,5,4,6,4,5,3,4,6,5,4,4,5,3,6,7,5,4,5,4,6,5,4,5,4,4,4]

function Sparkline({ data, color }: { data: number[], color: string }) {
  const max = Math.max(...data)
  const w = 200, h = 48
  const pts = data.map((v,i) => `${(i/(data.length-1))*w},${h - (v/max)*(h-8)-4}`).join(' ')
  const area = `0,${h} ${pts} ${w},${h}`
  return (
    <svg width={w} height={h} style={{overflow:'hidden'}}>
      <defs><linearGradient id={`sg${color}`} x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor={color} stopOpacity="0.3"/><stop offset="100%" stopColor={color} stopOpacity="0"/>
      </linearGradient></defs>
      <polygon points={area} fill={`url(#sg${color})`}/>
      <polyline points={pts} fill="none" stroke={color} strokeWidth="2" strokeLinejoin="round"/>
    </svg>
  )
}

const gradeColor = { Elite:'#0fcf8a', High:'#6c72ff', Medium:'#f5a623', Low:'#ff4d6a' } as Record<string,string>

export default function DORAPage() {
  const [period, setPeriod] = useState('30d')

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflowY:'auto'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>📏</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>DORA Metrics</div>
            <div style={{fontSize:12,color:'#546a88'}}>DevOps Research and Assessment · Elite performance benchmark · 30-day rolling window</div>
          </div>
          <div style={{display:'flex',gap:6}}>
            {['7d','30d','90d'].map(p=>(
              <button key={p} onClick={()=>setPeriod(p)} style={{padding:'5px 12px',border:`1px solid ${period===p?'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:period===p?'rgba(108,114,255,.15)':'transparent',color:period===p?'#8b90ff':'#546a88',fontSize:11,cursor:'pointer'}}>{p}</button>
            ))}
          </div>
        </div>
      </div>

      <div style={{padding:16,display:'flex',flexDirection:'column',gap:14}}>
        {/* DORA four key metrics */}
        <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:12}}>
          {[
            { key:'deploymentFrequency', label:'Deployment Frequency', icon:Rocket,     desc:'How often we deploy to production' },
            { key:'leadTime',            label:'Lead Time for Changes', icon:Clock,      desc:'Commit to production time' },
            { key:'mttr',                label:'Mean Time to Restore',  icon:AlertTriangle,desc:'Time to recover from failure' },
            { key:'changeFailureRate',   label:'Change Failure Rate',   icon:TrendingUp, desc:'% deploys causing incidents' },
          ].map(k => {
            const d = DORA_DATA[k.key as keyof typeof DORA_DATA]
            const Icon = k.icon
            return (
              <div key={k.key} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${d.color}33`,borderRadius:14,padding:16}}>
                <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:10}}>
                  <Icon size={14} style={{color:d.color}}/>
                  <span style={{fontSize:11,color:'#546a88',flex:1}}>{k.label}</span>
                  <span style={{background:`${gradeColor[d.grade]}22`,color:gradeColor[d.grade],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{d.grade}</span>
                </div>
                <div style={{fontSize:28,fontWeight:900,color:d.color,fontFamily:'Syne,sans-serif',marginBottom:3}}>{d.value}<span style={{fontSize:14,fontWeight:500}}>{d.unit}</span></div>
                <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:6}}>
                  {d.up ? <ArrowUp size={11} style={{color:k.key==='changeFailureRate'?'#ff4d6a':'#0fcf8a'}}/> : <ArrowDown size={11} style={{color:k.key==='deploymentFrequency'?'#ff4d6a':'#0fcf8a'}}/>}
                  <span style={{fontSize:11,color:d.up&&k.key!=='deploymentFrequency'?'#f5a623':'#0fcf8a',fontWeight:700}}>{d.trend}</span>
                  <span style={{fontSize:10,color:'#546a88'}}>vs last period</span>
                </div>
                <div style={{fontSize:10,color:'#546a88'}}>Target: {d.target}</div>
                <div style={{marginTop:8}}>
                  <Sparkline data={DEPLOY_TREND.map(v=>v*Math.random()*0.5+v*0.5)} color={d.color}/>
                </div>
              </div>
            )
          })}
        </div>

        {/* Grade breakdown */}
        <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
          <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Performance Classification (Google DORA Research)</div>
          <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
            {[
              {grade:'Elite',  deploy:'Multiple/day', lead:'< 1 hour',  mttr:'< 1 hour',  cfr:'< 5%',  color:'#0fcf8a'},
              {grade:'High',   deploy:'1/day–1/week', lead:'1d–1week',  mttr:'< 1 day',   cfr:'< 15%', color:'#6c72ff'},
              {grade:'Medium', deploy:'1/week–1/month',lead:'1w–6months',mttr:'1d–1week', cfr:'< 30%', color:'#f5a623'},
              {grade:'Low',    deploy:'< 1/month',   lead:'> 6 months', mttr:'> 1 week',  cfr:'> 30%', color:'#ff4d6a'},
            ].map(g=>(
              <div key={g.grade} style={{background:'rgba(7,15,30,.5)',border:`1px solid ${g.color}33`,borderRadius:10,padding:12}}>
                <div style={{fontSize:13,fontWeight:800,color:g.color,marginBottom:8}}>{g.grade}</div>
                {[{l:'Deploy Freq',v:g.deploy},{l:'Lead Time',v:g.lead},{l:'MTTR',v:g.mttr},{l:'CFR',v:g.cfr}].map(r=>(
                  <div key={r.l} style={{display:'flex',justifyContent:'space-between',fontSize:10,marginBottom:4}}>
                    <span style={{color:'#546a88'}}>{r.l}</span>
                    <span style={{color:'#8fa8cc'}}>{r.v}</span>
                  </div>
                ))}
              </div>
            ))}
          </div>
        </div>

        {/* Recent deployments */}
        <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
          <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Recent Deployments ({period})</div>
          <table style={{width:'100%',borderCollapse:'collapse'}}>
            <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
              {['Service','Version','Time','Author','Lead Time','Result','Rollback'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
            </tr></thead>
            <tbody>
              {RECENT_DEPLOYS.map((d,i)=>(
                <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8b90ff',fontWeight:600}}>{d.service}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#6c72ff'}}>{d.version}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88'}}>{d.time}</td>
                  <td style={{padding:'9px 12px',fontSize:11,color:'#8fa8cc'}}>{d.author}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{d.leadTime}</td>
                  <td style={{padding:'9px 12px'}}>
                    <div style={{display:'flex',alignItems:'center',gap:5}}>
                      {d.result==='success'?<CheckCircle size={12} style={{color:'#0fcf8a'}}/>:<AlertTriangle size={12} style={{color:'#ff4d6a'}}/>}
                      <span style={{color:d.result==='success'?'#0fcf8a':'#ff4d6a',fontSize:11,fontWeight:600}}>{d.result}</span>
                    </div>
                  </td>
                  <td style={{padding:'9px 12px'}}>{d.rollback?<span style={{background:'rgba(255,77,106,.15)',color:'#ff4d6a',fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:700}}>ROLLED BACK</span>:'—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
