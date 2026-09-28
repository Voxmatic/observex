// LoadTestingPage.tsx — Load test execution, results, APM correlation during test
// Inspired by Grafana k6, but linked live to ObserveX APM for instant visibility
import { useState } from 'react'
import { Play, Square, BarChart3, TrendingUp, AlertTriangle, Clock, Users, Zap } from 'lucide-react'
import toast from 'react-hot-toast'

const TEST_RUNS = [
  { id:'lt1', name:'Checkout Soak Test', script:'checkout-soak.js', status:'completed', startTime:'2h ago', duration:'30m', vus:500, peakVus:500, totalRequests:1842000, rps:1024, p50:142, p90:284, p95:412, p99:892, errors:0.8, thresholds:'PASS', triggered:'manual', env:'staging' },
  { id:'lt2', name:'API Spike Test', script:'api-spike.js', status:'completed', startTime:'4h ago', duration:'10m', vus:2000, peakVus:2000, totalRequests:480000, rps:800, p50:184, p90:482, p95:840, p99:2400, errors:3.2, thresholds:'FAIL', triggered:'schedule', env:'staging' },
  { id:'lt3', name:'User Login Stress', script:'login-stress.js', status:'running', startTime:'8m ago', duration:'20m (running)', vus:120, peakVus:120, totalRequests:48400, rps:100, p50:88, p90:184, p95:240, p99:480, errors:0.1, thresholds:'PASS (ongoing)', triggered:'manual', env:'staging' },
  { id:'lt4', name:'DB Load Baseline', script:'db-baseline.js', status:'scheduled', startTime:'In 2h', duration:'60m', vus:100, peakVus:100, totalRequests:0, rps:0, p50:0, p90:0, p95:0, p99:0, errors:0, thresholds:'—', triggered:'schedule', env:'production' },
]

const THRESHOLDS = [
  { metric:'http_req_duration{p95}', condition:'< 500ms', status:'PASS', actual:'412ms' },
  { metric:'http_req_duration{p99}', condition:'< 1000ms', status:'PASS', actual:'892ms' },
  { metric:'http_req_failed', condition:'< 1%', status:'PASS', actual:'0.8%' },
  { metric:'http_reqs', condition:'> 900/s', status:'FAIL', actual:'1024/s' },
  { metric:'vus', condition:'> 400', status:'PASS', actual:'500' },
]

const stC: Record<string,string> = { completed:'#8fa8cc', running:'#6c72ff', scheduled:'#546a88', failed:'#ff4d6a' }
const stBg: Record<string,string> = { completed:'rgba(143,168,204,.1)', running:'rgba(108,114,255,.12)', scheduled:'rgba(84,106,136,.1)', failed:'rgba(255,77,106,.1)' }

export default function LoadTestingPage() {
  const [selected, setSelected] = useState(TEST_RUNS[0])
  const [running, setRunning] = useState(false)

  const runTest = () => {
    setRunning(true)
    toast.success('Load test started on staging cluster')
    setTimeout(()=>setRunning(false), 3000)
  }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>⚡</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Load Testing</div>
            <div style={{fontSize:12,color:'#546a88'}}>k6 / Artillery test execution · Live APM correlation · Threshold validation · Performance regression detection</div>
          </div>
          <div style={{display:'flex',gap:8}}>
            <button onClick={runTest} disabled={running} style={{display:'flex',gap:5,alignItems:'center',background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:12,fontWeight:700,cursor:running?'wait':'pointer'}}>
              <Play size={12}/>{running?'Starting...':'Run Test'}
            </button>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total Tests',v:TEST_RUNS.length,c:'#6c72ff'},{l:'Running',v:TEST_RUNS.filter(t=>t.status==='running').length,c:'#6c72ff'},{l:'Pass Rate',v:'75%',c:'#f5a623'},{l:'Total Requests',v:'2.37M',c:'#8fa8cc'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
          <div style={{marginLeft:'auto',padding:'6px 12px',background:'rgba(108,114,255,.08)',border:'1px solid rgba(108,114,255,.2)',borderRadius:8,fontSize:11,color:'#8b90ff',display:'flex',alignItems:'center',gap:4}}>
            💡 APM metrics auto-overlay during active tests
          </div>
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:300,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {TEST_RUNS.map(t=>(
            <div key={t.id} onClick={()=>setSelected(t)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===t.id?stC[t.status]:'transparent'}`,background:selected.id===t.id?stBg[t.status]:'transparent'}}>
              <div style={{display:'flex',gap:8,marginBottom:5,alignItems:'center'}}>
                <span style={{background:stBg[t.status],color:stC[t.status],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{t.status.toUpperCase()}</span>
                {t.status==='running'&&<div style={{width:7,height:7,borderRadius:'50%',background:'#6c72ff',animation:'pulse 1s infinite'}}/>}
              </div>
              <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{t.name}</div>
              <div style={{fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace',marginBottom:4}}>{t.script}</div>
              <div style={{display:'flex',gap:8,fontSize:10,color:'#546a88'}}>
                <span><Users size={9} style={{display:'inline',verticalAlign:'middle'}}/> {t.vus} VUs</span>
                <span><Clock size={9} style={{display:'inline',verticalAlign:'middle'}}/> {t.duration}</span>
                <span style={{color:t.thresholds==='PASS'?'#0fcf8a':t.thresholds.startsWith('FAIL')?'#ff4d6a':'#546a88',fontWeight:700}}>{t.thresholds}</span>
              </div>
            </div>
          ))}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:stBg[selected.status],border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:12,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap',fontSize:11}}>
                  <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{selected.script}</span>
                  <span style={{color:'#546a88'}}>{selected.env}</span>
                  <span style={{color:'#546a88'}}>triggered: {selected.triggered}</span>
                  <span style={{color:'#546a88'}}>{selected.startTime}</span>
                  <span style={{background:stBg[selected.status],color:stC[selected.status],padding:'2px 8px',borderRadius:8,fontWeight:700}}>{selected.status.toUpperCase()}</span>
                </div>
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(5,1fr)',gap:8}}>
              {[
                {l:'Peak VUs',         v:selected.peakVus.toLocaleString(),            c:'#6c72ff'},
                {l:'Total Requests',   v:(selected.totalRequests/1000).toFixed(0)+'K', c:'#8b90ff'},
                {l:'RPS',              v:`${selected.rps}/s`,                          c:'#8fa8cc'},
                {l:'P99 Latency',      v:selected.p99>0?`${selected.p99}ms`:'—',      c:selected.p99>1000?'#ff4d6a':selected.p99>500?'#f5a623':'#0fcf8a'},
                {l:'Error Rate',       v:selected.errors>0?`${selected.errors}%`:'0%', c:selected.errors>2?'#ff4d6a':'#0fcf8a'},
              ].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {selected.totalRequests>0&&(
            <>
              {/* Latency breakdown */}
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Latency Percentiles</div>
                <div style={{display:'flex',gap:4,alignItems:'flex-end',height:80}}>
                  {[{l:'P50',v:selected.p50},{l:'P90',v:selected.p90},{l:'P95',v:selected.p95},{l:'P99',v:selected.p99}].map(p=>{
                    const maxP=selected.p99||1
                    const pct=(p.v/maxP)*100
                    const color=p.l==='P99'?(p.v>1000?'#ff4d6a':'#f5a623'):'#6c72ff'
                    return (
                      <div key={p.l} style={{flex:1,display:'flex',flexDirection:'column',alignItems:'center',gap:4}}>
                        <div style={{fontSize:11,fontWeight:700,color}}>{p.v}ms</div>
                        <div style={{width:'100%',border:`1px solid ${color}44`,borderRadius:'4px 4px 0 0',height:`${pct}%`,minHeight:4,background:color,opacity:.7}}/>
                        <div style={{fontSize:9,color:'#546a88'}}>{p.l}</div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* Thresholds */}
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Threshold Validation</div>
                {THRESHOLDS.map((t,i)=>(
                  <div key={i} style={{display:'flex',gap:10,alignItems:'center',padding:'7px 0',borderBottom:i<THRESHOLDS.length-1?'1px solid rgba(26,45,74,.3)':'none'}}>
                    <span style={{fontSize:13}}>{t.status==='PASS'?'✓':'✗'}</span>
                    <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,flex:1,color:'#c8d8ef'}}>{t.metric}</span>
                    <span style={{fontSize:10,color:'#546a88'}}>{t.condition}</span>
                    <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:t.status==='PASS'?'#0fcf8a':'#ff4d6a',fontWeight:700}}>{t.actual}</span>
                    <span style={{background:t.status==='PASS'?'rgba(15,207,138,.15)':'rgba(255,77,106,.15)',color:t.status==='PASS'?'#0fcf8a':'#ff4d6a',fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:800}}>{t.status}</span>
                  </div>
                ))}
              </div>

              <div style={{background:'rgba(108,114,255,.06)',border:'1px solid rgba(108,114,255,.2)',borderRadius:12,padding:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#8b90ff',marginBottom:6}}>🔗 APM Correlation</div>
                <div style={{fontSize:11,color:'#8fa8cc',marginBottom:10}}>During this load test, the following services showed elevated metrics. Click to view the correlated APM traces.</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                  {['api-gateway (P99: +184ms)','checkout-service (Err: +2.1%)','postgres-primary (Conn pool: 94%)'].map(s=>(
                    <button key={s} style={{background:'rgba(108,114,255,.15)',border:'1px solid rgba(108,114,255,.3)',color:'#8b90ff',padding:'5px 12px',borderRadius:8,fontSize:10,cursor:'pointer'}}>{s}</button>
                  ))}
                </div>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
