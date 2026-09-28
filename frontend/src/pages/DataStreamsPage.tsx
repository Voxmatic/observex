// DataStreamsPage.tsx — Kafka/SQS/Kinesis pipeline monitoring
// ObserveX take: end-to-end data pipeline health, lag, producer-consumer correlation
import { useState } from 'react'
import { Activity, AlertTriangle, TrendingUp, Clock } from 'lucide-react'

const TOPICS = [
  { name:'user.events', type:'Kafka', partitions:24, producers:3, consumers:5, msgRate:8420, lagTotal:0, lagMax:0, size:'2.4 GB', retention:'7d', status:'healthy', p99:12 },
  { name:'checkout.orders', type:'Kafka', partitions:12, producers:2, consumers:4, msgRate:284, lagTotal:0, lagMax:0, size:'840 MB', retention:'7d', status:'healthy', p99:8 },
  { name:'ml.inference-requests', type:'Kafka', partitions:8, producers:4, consumers:2, msgRate:48, lagTotal:1240, lagMax:620, size:'120 MB', retention:'24h', status:'critical', p99:184 },
  { name:'notifications.email', type:'Kafka', partitions:4, producers:2, consumers:3, msgRate:840, lagTotal:0, lagMax:0, size:'280 MB', retention:'3d', status:'healthy', p99:6 },
  { name:'prod-payment-queue', type:'SQS', partitions:1, producers:1, consumers:4, msgRate:78, lagTotal:0, lagMax:0, size:'—', retention:'14d', status:'healthy', p99:4 },
  { name:'analytics.clickstream', type:'Kinesis', partitions:10, producers:8, consumers:3, msgRate:124000, lagTotal:48000, lagMax:8000, size:'48 GB', retention:'24h', status:'warning', p99:2 },
]

const CONSUMER_GROUPS = [
  { id:'cg1', name:'checkout-consumer-group', topic:'checkout.orders', lag:0, members:4, status:'healthy', offsetLag:0 },
  { id:'cg2', name:'ml-inference-consumer', topic:'ml.inference-requests', lag:1240, members:2, status:'critical', offsetLag:1240 },
  { id:'cg3', name:'analytics-processor', topic:'analytics.clickstream', lag:48000, members:3, status:'warning', offsetLag:48000 },
  { id:'cg4', name:'email-sender', topic:'notifications.email', lag:0, members:3, status:'healthy', offsetLag:0 },
  { id:'cg5', name:'event-archiver', topic:'user.events', lag:0, members:2, status:'healthy', offsetLag:0 },
]

const stC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }
const typeC: Record<string,string> = { Kafka:'#6c72ff', SQS:'#f5a623', Kinesis:'#a855f7', RabbitMQ:'#ff4d6a' }

export default function DataStreamsPage() {
  const [selected, setSelected] = useState(TOPICS[2])

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🌊</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Data Streams</div>
            <div style={{fontSize:12,color:'#546a88'}}>Kafka · SQS · Kinesis · Consumer lag · Producer throughput · End-to-end pipeline latency</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Topics/Queues',v:TOPICS.length,c:'#6c72ff'},{l:'Total Msg/s',v:(TOPICS.reduce((a,t)=>a+t.msgRate,0)/1000).toFixed(0)+'K',c:'#8b90ff'},{l:'Lag Issues',v:TOPICS.filter(t=>t.lagTotal>0).length,c:'#f5a623'},{l:'Consumer Groups',v:CONSUMER_GROUPS.length,c:'#8fa8cc'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {/* Topic overview */}
        <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden',marginBottom:14}}>
          <div style={{padding:'10px 14px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Topics & Queues</div>
          <table style={{width:'100%',borderCollapse:'collapse'}}>
            <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
              {['Topic / Queue','Type','Msg Rate','Consumer Lag','Partitions','P99 Latency','Size','Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase',whiteSpace:'nowrap'}}>{h}</th>)}
            </tr></thead>
            <tbody>
              {TOPICS.map((t,i)=>(
                <tr key={i} onClick={()=>setSelected(t)} style={{borderBottom:'1px solid rgba(26,45,74,.4)',cursor:'pointer',background:selected.name===t.name?'rgba(108,114,255,.04)':'transparent'}}>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#f0f6ff',fontWeight:700}}>{t.name}</td>
                  <td style={{padding:'9px 12px'}}><span style={{background:`${typeC[t.type]}22`,color:typeC[t.type],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8}}>{t.type}</span></td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{t.msgRate.toLocaleString()}/s</td>
                  <td style={{padding:'9px 12px'}}>
                    {t.lagTotal>0
                      ? <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:t.lagTotal>10000?'#ff4d6a':'#f5a623',fontWeight:700}}>⚠ {t.lagTotal.toLocaleString()}</span>
                      : <span style={{color:'#0fcf8a',fontSize:11}}>✓ 0</span>}
                  </td>
                  <td style={{padding:'9px 12px',fontSize:11,color:'#8fa8cc'}}>{t.partitions}</td>
                  <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{t.p99}ms</td>
                  <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{t.size}</td>
                  <td style={{padding:'9px 12px'}}><div style={{display:'flex',gap:5,alignItems:'center'}}><div style={{width:6,height:6,borderRadius:'50%',background:stC[t.status]}}/><span style={{color:stC[t.status],fontSize:11}}>{t.status}</span></div></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Consumer groups */}
        <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'10px 14px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Consumer Groups</div>
            {CONSUMER_GROUPS.map((g,i)=>(
              <div key={g.id} style={{padding:'10px 14px',borderBottom:i<CONSUMER_GROUPS.length-1?'1px solid rgba(26,45,74,.4)':'none'}}>
                <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:4}}>
                  <div style={{width:7,height:7,borderRadius:'50%',background:stC[g.status]}}/>
                  <span style={{fontSize:11,fontWeight:700,color:'#f0f6ff',flex:1}}>{g.name}</span>
                  <span style={{fontSize:10,color:'#546a88'}}>{g.members} members</span>
                </div>
                <div style={{display:'flex',gap:8,fontSize:10,color:'#546a88'}}>
                  <span style={{fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{g.topic}</span>
                  <span style={{color:g.lag>0?'#f5a623':'#0fcf8a',fontWeight:700}}>lag: {g.lag.toLocaleString()}</span>
                </div>
              </div>
            ))}
          </div>

          {selected&&(
            <div style={{background:selected.lagTotal>0?'rgba(255,77,106,.06)':'rgba(17,31,53,.7)',border:`1px solid ${selected.lagTotal>0?'rgba(255,77,106,.3)':'#1a2d4a'}`,borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>📊 {selected.name}</div>
              {selected.lagTotal>0&&(
                <div style={{background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.25)',borderRadius:8,padding:'10px 12px',marginBottom:12}}>
                  <div style={{fontSize:11,fontWeight:700,color:'#ff4d6a',marginBottom:4}}>⚠ Consumer Lag Detected</div>
                  <div style={{fontSize:11,color:'#c8d8ef'}}>Total lag: <strong style={{color:'#ff4d6a'}}>{selected.lagTotal.toLocaleString()} messages</strong></div>
                  <div style={{fontSize:11,color:'#c8d8ef'}}>Max partition lag: <strong style={{color:'#f5a623'}}>{selected.lagMax.toLocaleString()}</strong></div>
                  <div style={{fontSize:10,color:'#546a88',marginTop:6}}>At current processing rate, ETA to clear: ~{Math.ceil(selected.lagTotal/selected.msgRate)}s</div>
                </div>
              )}
              <div style={{display:'flex',flexDirection:'column',gap:6}}>
                {[{l:'Message Rate',v:`${selected.msgRate.toLocaleString()}/s`},{l:'Partitions',v:selected.partitions},{l:'Producers',v:selected.producers},{l:'Consumers',v:selected.consumers},{l:'P99 Latency',v:`${selected.p99}ms`},{l:'Retention',v:selected.retention}].map(k=>(
                  <div key={k.l} style={{display:'flex',justifyContent:'space-between',fontSize:11,paddingBottom:4,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                    <span style={{color:'#546a88'}}>{k.l}</span>
                    <span style={{color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace',fontWeight:600}}>{k.v}</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
