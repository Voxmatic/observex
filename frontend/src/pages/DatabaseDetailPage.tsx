// DatabaseDetailPage.tsx — /database/:id — Query analysis, slow log, connections, locks
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { TimeRangePicker } from '@/components/shared/TimeRangePicker'
import { MetricCard } from '@/components/shared/MetricCard'
import { ArrowLeft, Database, Clock, AlertTriangle, Lock, Activity } from 'lucide-react'

const DBS: Record<string,any> = {
  'postgres-primary': {
    id:'postgres-primary', name:'PostgreSQL Primary', type:'PostgreSQL 16', host:'10.0.42.18:5432',
    status:'warning', version:'16.2', dbSize:'84 GB', tables:142, indexes:284,
    connections:{active:18,idle:2,max:20,used_pct:96,waiting:4},
    cpu:18.4, mem:24.2, diskUsed:'420 GB', diskTotal:'1 TB', iops:4200, cacheHitRatio:98.4,
    replicationLag:'0ms', walSize:'2.4 GB',
    slowQueries:[
      { query:'SELECT * FROM users u JOIN preferences p ON u.id=p.user_id WHERE u.id=$1', avgTime:4200, calls:1842, rows:1, source:'user-service' },
      { query:'SELECT * FROM orders WHERE status=$1 AND created_at > $2 ORDER BY created_at DESC LIMIT 100', avgTime:840, calls:284, rows:84, source:'checkout-service' },
      { query:'UPDATE sessions SET last_active=NOW() WHERE token=$1', avgTime:24, calls:12400, rows:1, source:'api-gateway' },
      { query:'SELECT count(*) FROM audit_log WHERE org_id=$1 AND created_at > $2', avgTime:1200, calls:48, rows:1, source:'api-gateway' },
    ],
    locks:[
      { mode:'RowExclusiveLock', relation:'users', pid:2100, duration:'2.4s', waiting:false },
      { mode:'AccessShareLock', relation:'preferences', pid:2101, duration:'4.8s', waiting:true },
    ],
    waitEvents:[
      { event:'ClientRead', count:42, pct:35 },
      { event:'DataFileRead', count:18, pct:15 },
      { event:'WALWrite', count:12, pct:10 },
      { event:'LWLockNamed', count:8, pct:7 },
    ],
    clients:['user-service','checkout-service','api-gateway','payment-service'],
  },
}

export default function DatabaseDetailPage() {
  const { id } = useParams<{id:string}>()
  const [timeRange, setTimeRange] = useState('1h')
  const [tab, setTab] = useState<'overview'|'queries'|'connections'|'locks'>('overview')
  const db = DBS[id||''] || DBS['postgres-primary']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
          <Link to="/database" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <Database size={18} style={{color:'#a855f7'}}/>
          <div style={{flex:1}}>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{db.name}</div>
            <div style={{fontSize:10,color:'#546a88'}}>{db.type} · {db.host} · {db.dbSize} · {db.tables} tables · {db.indexes} indexes</div>
          </div>
          <TimeRangePicker value={timeRange} onChange={setTimeRange}/>
        </div>
        <div style={{display:'flex',gap:6}}>
          <MetricCard label="Pool Usage" value={`${db.connections.used_pct}%`} color={db.connections.used_pct>85?'#ff4d6a':'#0fcf8a'} small/>
          <MetricCard label="Active Conns" value={`${db.connections.active}/${db.connections.max}`} color={db.connections.active>15?'#ff4d6a':'#8fa8cc'} small/>
          <MetricCard label="Waiting" value={db.connections.waiting} color={db.connections.waiting>0?'#f5a623':'#0fcf8a'} small/>
          <MetricCard label="Cache Hit" value={`${db.cacheHitRatio}%`} color={db.cacheHitRatio>95?'#0fcf8a':'#f5a623'} small/>
          <MetricCard label="CPU" value={`${db.cpu}%`} color="#8fa8cc" small/>
          <MetricCard label="IOPS" value={db.iops.toLocaleString()} color="#8fa8cc" small/>
          <MetricCard label="Repl Lag" value={db.replicationLag} color="#0fcf8a" small/>
          <MetricCard label="WAL" value={db.walSize} color="#8fa8cc" small/>
        </div>
        <div style={{display:'flex',gap:0,marginTop:8}}>
          {(['overview','queries','connections','locks'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'6px 14px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='overview'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Connection pool</div>
              <div style={{height:12,background:'#0b1628',borderRadius:6,overflow:'hidden',marginBottom:8}}>
                <div style={{height:'100%',width:`${db.connections.used_pct}%`,background:db.connections.used_pct>85?'#ff4d6a':'#0fcf8a'}}/>
              </div>
              <div style={{display:'flex',justifyContent:'space-between',fontSize:10,color:'#546a88'}}>
                <span>Active: {db.connections.active}</span><span>Idle: {db.connections.idle}</span><span>Max: {db.connections.max}</span><span>Waiting: {db.connections.waiting}</span>
              </div>
              {db.connections.used_pct>85&&(
                <div style={{marginTop:10,padding:'8px 10px',background:'rgba(255,77,106,.08)',border:'1px solid rgba(255,77,106,.2)',borderRadius:8,fontSize:10,color:'#ff4d6a'}}>⚠ Connection pool near exhaustion — consider increasing max_connections</div>
              )}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Wait events</div>
              {db.waitEvents.map((w:any,i:number)=>(
                <div key={i} style={{display:'flex',gap:8,alignItems:'center',marginBottom:6}}>
                  <span style={{fontSize:10,color:'#f0f6ff',flex:1,fontFamily:'JetBrains Mono,monospace'}}>{w.event}</span>
                  <div style={{width:80,height:5,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                    <div style={{height:'100%',width:`${w.pct}%`,background:'#a855f7'}}/>
                  </div>
                  <span style={{fontSize:10,color:'#a855f7',fontWeight:700,width:28,textAlign:'right'}}>{w.pct}%</span>
                </div>
              ))}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,gridColumn:'1/-1'}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Client services</div>
              <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                {db.clients.map((c:string)=>(
                  <Link key={c} to={`/services/${c}`} style={{textDecoration:'none',background:'rgba(108,114,255,.1)',border:'1px solid rgba(108,114,255,.2)',borderRadius:8,padding:'6px 12px',fontSize:11,color:'#8b90ff'}}>{c}</Link>
                ))}
              </div>
            </div>
          </div>
        )}

        {tab==='queries'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'10px 14px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Slow queries (P99 ≥ 100ms)</div>
            {db.slowQueries.map((q:any,i:number)=>(
              <div key={i} style={{padding:14,borderBottom:i<db.slowQueries.length-1?'1px solid rgba(26,45,74,.3)':'none'}}>
                <pre style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#c8d8ef',margin:0,whiteSpace:'pre-wrap',lineHeight:1.5,marginBottom:6}}>{q.query}</pre>
                <div style={{display:'flex',gap:12,fontSize:10}}>
                  <span style={{color:q.avgTime>1000?'#ff4d6a':q.avgTime>500?'#f5a623':'#8fa8cc'}}>Avg: <strong>{q.avgTime}ms</strong></span>
                  <span style={{color:'#546a88'}}>Calls: {q.calls.toLocaleString()}</span>
                  <span style={{color:'#546a88'}}>Rows: {q.rows}</span>
                  <span style={{color:'#6c72ff'}}>Source: {q.source}</span>
                  {q.avgTime>2000&&<span style={{color:'#ff4d6a',fontWeight:700}}>⚠ Needs optimization</span>}
                </div>
              </div>
            ))}
          </div>
        )}

        {tab==='connections'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Active connections (pg_stat_activity)</div>
            {Array.from({length:db.connections.active},(_,i)=>(
              <div key={i} style={{display:'flex',gap:10,alignItems:'center',padding:'6px 0',borderBottom:'1px solid rgba(26,45,74,.3)',fontSize:10}}>
                <span style={{color:'#546a88',width:30}}>{2100+i}</span>
                <span style={{color:'#6c72ff',flex:1,fontFamily:'JetBrains Mono,monospace'}}>{db.clients[i%db.clients.length]}</span>
                <span style={{color:i<4?'#0fcf8a':'#f5a623'}}>{i<4?'active':'idle in transaction'}</span>
                <span style={{color:'#546a88'}}>{i<4?`${Math.round(Math.random()*100)}ms`:`${(4+Math.random()*8).toFixed(1)}s`}</span>
              </div>
            ))}
          </div>
        )}

        {tab==='locks'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead><tr style={{background:'#080d1b'}}>
                {['Lock Mode','Relation','PID','Duration','Waiting'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
              </tr></thead>
              <tbody>
                {db.locks.map((l:any,i:number)=>(
                  <tr key={i} style={{borderTop:'1px solid rgba(26,45,74,.3)'}}>
                    <td style={{padding:'8px 12px',fontSize:11,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace'}}>{l.mode}</td>
                    <td style={{padding:'8px 12px',fontSize:11,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{l.relation}</td>
                    <td style={{padding:'8px 12px',fontSize:11,color:'#546a88'}}>{l.pid}</td>
                    <td style={{padding:'8px 12px',fontSize:11,color:parseFloat(l.duration)>2?'#f5a623':'#8fa8cc'}}>{l.duration}</td>
                    <td style={{padding:'8px 12px'}}>{l.waiting?<span style={{color:'#ff4d6a',fontWeight:700}}>⚠ WAITING</span>:<span style={{color:'#0fcf8a'}}>No</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
