// CronMonitorPage.tsx — Heartbeat monitoring for scheduled jobs
// Smart ObserveX take: links directly to logs, traces & APM when a cron fails
import { useState } from 'react'
import { Clock, CheckCircle, XCircle, AlertTriangle, Plus, RefreshCw, Zap } from 'lucide-react'

const CRONS = [
  { id:'c1', name:'daily-report-generator', schedule:'0 2 * * *', lastRun:'02:00:12', nextRun:'Tomorrow 02:00', duration:'4m 12s', status:'ok', streak:142, service:'reporting-svc', env:'production', missedRuns:0, avgDuration:'3m 58s', p95Duration:'6m 14s', timeout:'15m', tags:['finance','daily'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1] },
  { id:'c2', name:'user-session-cleanup', schedule:'*/15 * * * *', lastRun:'19:45:00', nextRun:'20:00:00', duration:'48s', status:'ok', streak:8421, service:'user-service', env:'production', missedRuns:0, avgDuration:'45s', p95Duration:'1m 12s', timeout:'5m', tags:['cleanup','frequent'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1] },
  { id:'c3', name:'ml-model-retraining', schedule:'0 3 * * 0', lastRun:'Sun 03:00', nextRun:'Sun 03:00', duration:null, status:'missed', streak:0, service:'ml-inference', env:'production', missedRuns:2, avgDuration:'2h 14m', p95Duration:'3h 8m', timeout:'4h', tags:['ml','weekly'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,0,1,1,1,0,1,1] },
  { id:'c4', name:'payment-reconciliation', schedule:'0 1 * * *', lastRun:'01:00:08', nextRun:'Tomorrow 01:00', duration:'12m 44s', status:'timeout', streak:0, service:'payment-service', env:'production', missedRuns:0, avgDuration:'8m 22s', p95Duration:'11m 48s', timeout:'10m', tags:['finance','critical'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,2,1,2,3] },
  { id:'c5', name:'cache-warmup', schedule:'*/5 * * * *', lastRun:'19:55:00', nextRun:'20:00:00', duration:'8s', status:'ok', streak:12480, service:'api-gateway', env:'production', missedRuns:0, avgDuration:'6s', p95Duration:'14s', timeout:'2m', tags:['performance'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1] },
  { id:'c6', name:'invoice-email-sender', schedule:'0 9 * * 1-5', lastRun:'09:00:14', nextRun:'Tomorrow 09:00', duration:'2m 38s', status:'ok', streak:248, service:'notification-svc', env:'production', missedRuns:0, avgDuration:'2m 12s', p95Duration:'4m 8s', timeout:'15m', tags:['email','billing'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1] },
  { id:'c7', name:'db-vacuum-analyze', schedule:'0 4 * * *', lastRun:'04:00:22', nextRun:'Tomorrow 04:00', duration:'18m 42s', status:'ok', streak:89, service:'postgres-primary', env:'production', missedRuns:0, avgDuration:'16m 48s', p95Duration:'24m 12s', timeout:'60m', tags:['database','maintenance'], history:[1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1] },
]

const stC: Record<string,string> = { ok:'#0fcf8a', missed:'#f5a623', timeout:'#ff4d6a', error:'#ff4d6a', running:'#6c72ff' }
const stBg: Record<string,string> = { ok:'rgba(15,207,138,.08)', missed:'rgba(245,166,35,.08)', timeout:'rgba(255,77,106,.08)', error:'rgba(255,77,106,.08)' }
const stIcon: Record<string,any> = { ok:CheckCircle, missed:AlertTriangle, timeout:XCircle, error:XCircle, running:Clock }

function HistoryBar({ history }: { history: number[] }) {
  const c = (v: number) => v===1?'#0fcf8a':v===0?'#f5a623':v>=2?'#ff4d6a':'#1a2d4a'
  return (
    <div style={{display:'flex',gap:1}}>
      {history.map((v,i)=><div key={i} style={{width:7,height:20,borderRadius:2,background:c(v)}} title={v===1?'ok':v===0?'missed':'timeout'}/>)}
    </div>
  )
}

export default function CronMonitorPage() {
  const [selected, setSelected] = useState(CRONS[0])
  const [search, setSearch] = useState('')
  const ok = CRONS.filter(c=>c.status==='ok').length
  const alerting = CRONS.filter(c=>c.status!=='ok').length

  const filtered = CRONS.filter(c=>!search||c.name.includes(search)||c.service.includes(search))

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>⏱</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Cron & Job Monitor</div>
            <div style={{fontSize:12,color:'#546a88'}}>Heartbeat monitoring for scheduled jobs · Miss detection · Timeout alerting · Direct APM drill-down</div>
          </div>
          <button style={{display:'flex',gap:5,alignItems:'center',background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}><Plus size={12}/>Add Monitor</button>
        </div>
        <div style={{display:'flex',gap:10,marginBottom:8}}>
          {[{l:'Total Monitors',v:CRONS.length,c:'#6c72ff'},{l:'Healthy',v:ok,c:'#0fcf8a'},{l:'Alerting',v:alerting,c:alerting>0?'#ff4d6a':'#0fcf8a'},{l:'Missed Runs',v:CRONS.reduce((a,c)=>a+c.missedRuns,0),c:'#f5a623'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
          <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search jobs..."
            style={{marginLeft:'auto',background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 12px',fontSize:12,color:'#c8d8ef',outline:'none',width:200}}/>
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:340,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {filtered.map(c=>{
            const Icon = stIcon[c.status]||Clock
            return (
              <div key={c.id} onClick={()=>setSelected(c)}
                style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===c.id?stC[c.status]:'transparent'}`,background:selected.id===c.id?stBg[c.status]:'transparent'}}>
                <div style={{display:'flex',gap:8,marginBottom:6}}>
                  <Icon size={13} style={{color:stC[c.status],flexShrink:0,marginTop:1}}/>
                  <span style={{flex:1,fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{c.name}</span>
                  <span style={{background:stBg[c.status],color:stC[c.status],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{c.status.toUpperCase()}</span>
                </div>
                <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginBottom:5}}>
                  <span style={{fontFamily:'JetBrains Mono,monospace'}}>{c.schedule}</span>
                  <span>next: {c.nextRun}</span>
                  {c.streak>0&&<span style={{color:'#0fcf8a'}}>🔥 {c.streak.toLocaleString()}</span>}
                </div>
                <HistoryBar history={c.history}/>
              </div>
            )
          })}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:stBg[selected.status],border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:12,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap',fontSize:11}}>
                  <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{selected.service}</span>
                  <span style={{color:'#546a88'}}>schedule: <span style={{fontFamily:'JetBrains Mono,monospace',color:'#8b90ff'}}>{selected.schedule}</span></span>
                  <span style={{color:'#546a88'}}>timeout: {selected.timeout}</span>
                  {selected.tags.map(t=><span key={t} style={{background:'#182844',color:'#546a88',fontSize:9,padding:'1px 6px',borderRadius:8}}>{t}</span>)}
                </div>
              </div>
              <div style={{textAlign:'right'}}>
                <div style={{fontSize:22,fontWeight:900,color:stC[selected.status],fontFamily:'Syne,sans-serif'}}>{selected.status.toUpperCase()}</div>
                {selected.streak>0&&<div style={{fontSize:11,color:'#0fcf8a'}}>🔥 {selected.streak.toLocaleString()} successful runs</div>}
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(5,1fr)',gap:10}}>
              {[{l:'Last Run',v:selected.lastRun||'Never',c:'#8fa8cc'},{l:'Last Duration',v:selected.duration||'—',c:'#8fa8cc'},{l:'Avg Duration',v:selected.avgDuration,c:'#8fa8cc'},{l:'P95 Duration',v:selected.p95Duration,c:selected.status==='timeout'?'#ff4d6a':'#f5a623'},{l:'Missed Runs',v:selected.missedRuns,c:selected.missedRuns>0?'#ff4d6a':'#0fcf8a'}].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:15,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {selected.status==='timeout'&&(
            <div style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.25)',borderRadius:12,padding:14,marginBottom:14}}>
              <div style={{fontSize:13,fontWeight:700,color:'#ff4d6a',marginBottom:8}}>⏱ Timeout Detected — exceeded {selected.timeout} limit</div>
              <div style={{fontSize:11,color:'#8fa8cc',marginBottom:10}}>The job started but did not complete within the configured timeout. Traces have been captured.</div>
              <div style={{display:'flex',gap:8}}>
                <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View APM Traces</button>
                <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Logs</button>
                <button style={{background:'rgba(15,207,138,.1)',border:'1px solid rgba(15,207,138,.25)',color:'#0fcf8a',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Create Incident</button>
              </div>
            </div>
          )}
          {selected.status==='missed'&&(
            <div style={{background:'rgba(245,166,35,.06)',border:'1px solid rgba(245,166,35,.25)',borderRadius:12,padding:14,marginBottom:14}}>
              <div style={{fontSize:13,fontWeight:700,color:'#f5a623',marginBottom:8}}>⚠ Missed Runs Detected ({selected.missedRuns} total)</div>
              <div style={{fontSize:11,color:'#8fa8cc',marginBottom:10}}>The job did not check in at the expected time. This could indicate the scheduler is down or the job itself crashed silently.</div>
              <div style={{display:'flex',gap:8}}>
                <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Check Kubernetes Cron Jobs</button>
                <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Scheduler Logs</button>
              </div>
            </div>
          )}

          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Run History (last 24 runs)</div>
            <div style={{display:'flex',gap:2,marginBottom:8}}>
              {selected.history.map((v,i)=>{
                const c = v===1?'#0fcf8a':v===0?'#f5a623':'#ff4d6a'
                const label = v===1?'✓ OK':v===0?'⚠ Missed':'✗ Timeout'
                return <div key={i} style={{flex:1,height:32,background:`${c}22`,border:`1px solid ${c}44`,borderRadius:4,display:'flex',alignItems:'center',justifyContent:'center',fontSize:8,color:c}} title={label}>{v===1?'✓':v===0?'!':'✗'}</div>
              })}
            </div>
            <div style={{display:'flex',gap:12,fontSize:10,color:'#546a88'}}>
              <span style={{color:'#0fcf8a'}}>✓ OK</span><span style={{color:'#f5a623'}}>! Missed</span><span style={{color:'#ff4d6a'}}>✗ Timeout/Error</span>
            </div>
          </div>

          <div style={{background:'rgba(17,31,53,.5)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>SDK Integration — Check-In Pattern</div>
            <pre style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8b90ff',background:'#080d1b',padding:12,borderRadius:8,margin:0,overflowX:'auto'}}>{`// Node.js / Python / Go SDK
import { crons } from '@observex/sdk'

// Wrap your job function
const monitor = crons.monitor('${selected.name}')
await monitor.checkin('in_progress')

try {
  await yourJobFunction()
  await monitor.checkin('ok', { duration: Date.now() - start })
} catch (err) {
  await monitor.checkin('error', { error: err.message })
}

// Or use the wrapper helper
await crons.withMonitor('${selected.name}', yourJobFunction)`}</pre>
          </div>
        </div>
      </div>
    </div>
  )
}
