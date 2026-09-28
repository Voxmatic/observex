// BackgroundJobsPage.tsx — Queue depth, worker health, job duration, Kafka/SQS/Celery/Sidekiq
import { useState } from 'react'
import { Activity, Clock, AlertTriangle, TrendingUp, BarChart3, Layers } from 'lucide-react'

const QUEUES = [
  { id:'q1', name:'email-notifications', type:'Kafka', topic:'notifications.email', workers:8, activeJobs:142, pendingJobs:284, failedJobs:3, avgDuration:'1.2s', p99Duration:'4.8s', throughput:840, errorRate:0.4, lag:284, status:'healthy', dlq:3 },
  { id:'q2', name:'payment-processing', type:'SQS', queue:'prod-payment-queue.fifo', workers:4, activeJobs:12, pendingJobs:0, failedJobs:0, avgDuration:'0.8s', p99Duration:'2.4s', throughput:78, errorRate:0, lag:0, status:'healthy', dlq:0 },
  { id:'q3', name:'ml-inference-batch', type:'Celery', queue:'ml_tasks', workers:2, activeJobs:4, pendingJobs:847, failedJobs:12, avgDuration:'8.4s', p99Duration:'32s', throughput:48, errorRate:2.8, lag:847, status:'critical', dlq:12 },
  { id:'q4', name:'user-avatar-resize', type:'Sidekiq', queue:'image_processing', workers:6, activeJobs:24, pendingJobs:18, failedJobs:0, avgDuration:'0.4s', p99Duration:'1.2s', throughput:312, errorRate:0, lag:18, status:'healthy', dlq:0 },
  { id:'q5', name:'report-generation', type:'Celery', queue:'reports', workers:3, activeJobs:1, pendingJobs:42, failedJobs:2, avgDuration:'124s', p99Duration:'310s', throughput:12, errorRate:1.2, lag:42, status:'warning', dlq:2 },
  { id:'q6', name:'search-index-update', type:'Kafka', topic:'search.indexing', workers:12, activeJobs:284, pendingJobs:1240, failedJobs:0, avgDuration:'0.2s', p99Duration:'0.8s', throughput:4820, errorRate:0, lag:1240, status:'warning', dlq:0 },
]

const typeC: Record<string,string> = { Kafka:'#6c72ff', SQS:'#f5a623', Celery:'#0fcf8a', Sidekiq:'#a855f7', RabbitMQ:'#ff4d6a', BullMQ:'#3d9bff' }
const stC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }

export default function BackgroundJobsPage() {
  const [selected, setSelected] = useState(QUEUES[2])
  const totalPending = QUEUES.reduce((a,q)=>a+q.pendingJobs,0)
  const totalFailed  = QUEUES.reduce((a,q)=>a+q.failedJobs,0)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>📤</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Background Jobs</div>
            <div style={{fontSize:12,color:'#546a88'}}>Queue depth · Worker health · Job duration · Kafka/SQS/Celery/Sidekiq · DLQ monitoring · Trace correlation</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total Queues',v:QUEUES.length,c:'#6c72ff'},{l:'Total Workers',v:QUEUES.reduce((a,q)=>a+q.workers,0),c:'#8b90ff'},{l:'Active Jobs',v:QUEUES.reduce((a,q)=>a+q.activeJobs,0).toLocaleString(),c:'#8fa8cc'},{l:'Queue Depth',v:totalPending.toLocaleString(),c:totalPending>1000?'#f5a623':'#8fa8cc'},{l:'Failed Jobs',v:totalFailed,c:totalFailed>0?'#ff4d6a':'#0fcf8a'},{l:'Throughput',v:QUEUES.reduce((a,q)=>a+q.throughput,0).toLocaleString()+'/min',c:'#0fcf8a'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{width:320,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {QUEUES.map(q=>(
            <div key={q.id} onClick={()=>setSelected(q)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===q.id?stC[q.status]:'transparent'}`,background:selected.id===q.id?'rgba(108,114,255,.04)':'transparent'}}>
              <div style={{display:'flex',gap:8,marginBottom:6,alignItems:'center'}}>
                <span style={{background:`${typeC[q.type]||'#546a88'}22`,color:typeC[q.type]||'#546a88',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8}}>{q.type}</span>
                <span style={{flex:1,fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{q.name}</span>
                <div style={{width:7,height:7,borderRadius:'50%',background:stC[q.status]}}/>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:6}}>
                {[{l:'Workers',v:q.workers},{l:'Pending',v:q.pendingJobs.toLocaleString(),alert:q.pendingJobs>500},{l:'Failed',v:q.failedJobs,alert:q.failedJobs>0}].map(k=>(
                  <div key={k.l} style={{background:'#0b1628',borderRadius:6,padding:'4px 6px'}}>
                    <div style={{fontSize:8,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:12,fontWeight:700,color:(k as any).alert?'#ff4d6a':'#8fa8cc'}}>{k.v}</div>
                  </div>
                ))}
              </div>
              {q.lag>500&&<div style={{marginTop:5,fontSize:9,color:'#ff4d6a',fontWeight:700}}>⚠ High lag: {q.lag.toLocaleString()} messages</div>}
            </div>
          ))}
        </div>

        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{background:'rgba(17,31,53,.7)',border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
            <div style={{display:'flex',gap:10,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap',fontSize:11}}>
                  <span style={{background:`${typeC[selected.type]}22`,color:typeC[selected.type],fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:8}}>{selected.type}</span>
                  <span style={{color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{selected.topic||selected.queue}</span>
                  <span style={{background:`${stC[selected.status]}22`,color:stC[selected.status],fontSize:10,fontWeight:700,padding:'2px 8px',borderRadius:20}}>{selected.status.toUpperCase()}</span>
                </div>
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10,marginBottom:14}}>
              {[
                {l:'Workers',        v:selected.workers,                 c:'#6c72ff'},
                {l:'Active Jobs',    v:selected.activeJobs,              c:'#8b90ff'},
                {l:'Pending (lag)',  v:selected.pendingJobs.toLocaleString(), c:selected.pendingJobs>500?'#ff4d6a':'#8fa8cc'},
                {l:'Failed Jobs',   v:selected.failedJobs,              c:selected.failedJobs>0?'#ff4d6a':'#0fcf8a'},
                {l:'Avg Duration',  v:selected.avgDuration,             c:'#8fa8cc'},
                {l:'P99 Duration',  v:selected.p99Duration,             c:'#f5a623'},
                {l:'Throughput',    v:`${selected.throughput}/min`,      c:'#0fcf8a'},
                {l:'Error Rate',    v:`${selected.errorRate}%`,          c:selected.errorRate>1?'#ff4d6a':'#0fcf8a'},
              ].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>

            {/* Queue depth visualization */}
            <div style={{background:'#080d1b',borderRadius:10,padding:12}}>
              <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:8}}>Queue Depth — last hour</div>
              <svg width="100%" height="60" viewBox="0 0 400 60" preserveAspectRatio="none">
                <defs><linearGradient id="qg" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={stC[selected.status]} stopOpacity="0.3"/>
                  <stop offset="100%" stopColor={stC[selected.status]} stopOpacity="0"/>
                </linearGradient></defs>
                {(() => {
                  const data = Array.from({length:60},(_,i)=>Math.max(0,selected.pendingJobs*(0.3+Math.sin(i/8)*0.4+Math.random()*0.3)))
                  const max = Math.max(...data)||1
                  const pts = data.map((v,i)=>`${(i/59)*400},${60-((v/max)*52)-4}`).join(' ')
                  const area = `0,60 ${pts} 400,60`
                  return <>
                    <polygon points={area} fill="url(#qg)"/>
                    <polyline points={pts} fill="none" stroke={stC[selected.status]} strokeWidth="2" strokeLinejoin="round"/>
                  </>
                })()}
              </svg>
            </div>
          </div>

          {selected.dlq>0&&(
            <div style={{background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.25)',borderRadius:12,padding:14,marginBottom:14}}>
              <div style={{fontSize:13,fontWeight:700,color:'#ff4d6a',marginBottom:6}}>💀 Dead Letter Queue — {selected.dlq} messages</div>
              <div style={{fontSize:11,color:'#8fa8cc',marginBottom:10}}>These jobs failed all retry attempts and require manual intervention. Inspect traces to understand root cause.</div>
              <div style={{display:'flex',gap:8}}>
                <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View DLQ Messages</button>
                <button style={{background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.25)',color:'#ff4d6a',padding:'6px 14px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Replay All</button>
              </div>
            </div>
          )}

          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Worker Instances</div>
            {Array.from({length:Math.min(selected.workers,5)},(_,i)=>(
              <div key={i} style={{display:'flex',gap:12,alignItems:'center',padding:'8px 0',borderBottom:i<Math.min(selected.workers,5)-1?'1px solid rgba(26,45,74,.4)':'none'}}>
                <div style={{width:8,height:8,borderRadius:'50%',background:'#0fcf8a'}}/>
                <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc',flex:1}}>{selected.name}-worker-{i+1}</span>
                <span style={{fontSize:10,color:'#546a88'}}>CPU: {Math.round(10+Math.random()*40)}%</span>
                <span style={{fontSize:10,color:'#546a88'}}>{Math.floor(Math.random()*20)+2} jobs/min</span>
                <span style={{fontSize:9,background:'rgba(15,207,138,.15)',color:'#0fcf8a',padding:'1px 7px',borderRadius:8}}>active</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
