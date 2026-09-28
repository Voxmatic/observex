// PodDetailPage.tsx — /kubernetes/pods/:podName — Pod metrics, logs, events, containers
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { TimeRangePicker } from '@/components/shared/TimeRangePicker'
import { MetricCard } from '@/components/shared/MetricCard'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ArrowLeft, Boxes, Activity, FileText } from 'lucide-react'

const PODS: Record<string,any> = {
  'user-svc-6b8c-pk9f1': {
    name:'user-svc-6b8c-pk9f1', namespace:'production', node:'k8s-node-01', status:'Running', ready:'1/1',
    app:'user-service', version:'v2.4.0', image:'observex/user-service:v2.4.0',
    restarts:3, age:'2d 14h', ip:'10.244.1.42', serviceAccount:'user-service',
    cpu:52, cpuRequest:'250m', cpuLimit:'1000m', mem:91, memUsed:'936Mi', memRequest:'512Mi', memLimit:'1Gi',
    containers:[
      { name:'user-service', image:'observex/user-service:v2.4.0', status:'Running', cpu:48, mem:884, restarts:3, ports:'8080,8081', ready:true },
      { name:'observex-sidecar', image:'observex/sidecar:v1.2.0', status:'Running', cpu:4, mem:52, restarts:0, ports:'9090', ready:true },
    ],
    events:[
      { time:'19:47', type:'Warning', reason:'HighMemory', msg:'Container user-service memory usage at 91% of limit' },
      { time:'19:30', type:'Normal', reason:'Pulled', msg:'Successfully pulled image observex/user-service:v2.4.0' },
      { time:'19:30', type:'Normal', reason:'Started', msg:'Started container user-service' },
      { time:'18:15', type:'Warning', reason:'OOMKilled', msg:'Container user-service was OOM killed (exit code 137)' },
      { time:'18:15', type:'Normal', reason:'Restarting', msg:'Restarting container user-service (restart count: 3)' },
    ],
    logs:[
      { time:'19:47:23', level:'ERROR', msg:'NullPointerException: user.preferences is null for userId=8420' },
      { time:'19:47:22', level:'WARN',  msg:'DB connection pool: 18/20 connections active, 2 waiting' },
      { time:'19:47:20', level:'INFO',  msg:'GET /api/v1/users/8420 - 4202ms - 500' },
      { time:'19:47:18', level:'INFO',  msg:'GET /api/v1/users/1234 - 142ms - 200' },
      { time:'19:47:15', level:'ERROR', msg:'ConnectionTimeout: Failed to acquire connection within 30000ms' },
      { time:'19:47:12', level:'INFO',  msg:'POST /api/v1/users/login - 112ms - 200' },
      { time:'19:47:10', level:'WARN',  msg:'GC pause: 48ms (threshold: 20ms)' },
      { time:'19:47:08', level:'INFO',  msg:'Health check: OK (heap: 892/1024 MB, threads: 248)' },
    ],
    labels:{'app':'user-service','version':'v2.4.0','team':'user-platform','environment':'production'},
  },
}

const LOG_C: Record<string,string> = { ERROR:'#ff4d6a', WARN:'#f5a623', INFO:'#546a88', DEBUG:'#3d5070' }
const EV_C: Record<string,string> = { Warning:'#f5a623', Normal:'#0fcf8a' }

export default function PodDetailPage() {
  const { podName } = useParams<{podName:string}>()
  const [timeRange, setTimeRange] = useState('1h')
  const [tab, setTab] = useState<'overview'|'logs'|'events'|'yaml'>('overview')
  const pod = PODS[podName||''] || PODS['user-svc-6b8c-pk9f1']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
          <Link to="/kubernetes" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <Boxes size={16} style={{color:'#6c72ff'}}/>
          <div style={{flex:1}}>
            <div style={{display:'flex',gap:6,alignItems:'center'}}>
              <span style={{fontSize:14,fontWeight:800,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace'}}>{pod.name}</span>
              <StatusBadge status={pod.status.toLowerCase()} size="xs"/>
              <span style={{fontSize:10,color:'#0fcf8a'}}>{pod.ready}</span>
            </div>
            <div style={{fontSize:10,color:'#546a88',marginTop:2}}>ns: {pod.namespace} · node: <Link to={`/infrastructure/${pod.node}`} style={{color:'#6c72ff',textDecoration:'none'}}>{pod.node}</Link> · {pod.age} · restarts: <span style={{color:pod.restarts>0?'#f5a623':'#0fcf8a'}}>{pod.restarts}</span></div>
          </div>
          <TimeRangePicker value={timeRange} onChange={setTimeRange}/>
        </div>
        <div style={{display:'flex',gap:6}}>
          <MetricCard label="CPU" value={`${pod.cpu}%`} color={pod.cpu>80?'#ff4d6a':'#f5a623'} small/>
          <MetricCard label="CPU Req/Lim" value={`${pod.cpuRequest}/${pod.cpuLimit}`} color="#546a88" small/>
          <MetricCard label="Memory" value={`${pod.mem}%`} color={pod.mem>85?'#ff4d6a':'#f5a623'} small/>
          <MetricCard label="Mem Used" value={pod.memUsed} color="#8fa8cc" small/>
          <MetricCard label="Mem Lim" value={pod.memLimit} color="#546a88" small/>
          <MetricCard label="Restarts" value={pod.restarts} color={pod.restarts>0?'#f5a623':'#0fcf8a'} small/>
          <MetricCard label="Containers" value={pod.containers.length} color="#6c72ff" small/>
        </div>
        <div style={{display:'flex',gap:0,marginTop:8}}>
          {(['overview','logs','events','yaml'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'6px 14px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='overview'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Containers</div>
              {pod.containers.map((c:any)=>(
                <div key={c.name} style={{marginBottom:10,padding:'8px 10px',background:'#080d1b',borderRadius:8}}>
                  <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:4}}>
                    <div style={{width:6,height:6,borderRadius:'50%',background:c.ready?'#0fcf8a':'#ff4d6a'}}/>
                    <span style={{fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{c.name}</span>
                    <span style={{fontSize:9,color:'#0fcf8a'}}>{c.status}</span>
                  </div>
                  <div style={{fontSize:10,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace',marginBottom:4}}>{c.image}</div>
                  <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88'}}>
                    <span>CPU: {c.cpu}%</span><span>Mem: {c.mem}Mi</span><span>Ports: {c.ports}</span><span style={{color:c.restarts>0?'#f5a623':'#0fcf8a'}}>Restarts: {c.restarts}</span>
                  </div>
                </div>
              ))}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Labels</div>
              {Object.entries(pod.labels).map(([k,v])=>(
                <div key={k} style={{display:'flex',justifyContent:'space-between',fontSize:10,paddingBottom:5,marginBottom:5,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                  <span style={{color:'#546a88'}}>{k}</span>
                  <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{String(v)}</span>
                </div>
              ))}
              <div style={{marginTop:10}}>
                <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:6}}>Pod info</div>
                {[{l:'IP',v:pod.ip},{l:'Node',v:pod.node},{l:'Service account',v:pod.serviceAccount},{l:'App',v:pod.app}].map(k=>(
                  <div key={k.l} style={{display:'flex',justifyContent:'space-between',fontSize:10,paddingBottom:4,marginBottom:4,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                    <span style={{color:'#546a88'}}>{k.l}</span>
                    <span style={{color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace'}}>{k.v}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {tab==='logs'&&(
          <div style={{background:'#080d1b',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'8px 14px',borderBottom:'1px solid #1a2d4a',display:'flex',gap:8,alignItems:'center'}}>
              <FileText size={12} style={{color:'#6c72ff'}}/>
              <span style={{fontSize:11,fontWeight:700,color:'#c8d8ef'}}>Container logs — {pod.containers[0].name}</span>
              <span style={{marginLeft:'auto',fontSize:9,color:'#0fcf8a'}}>Live tailing</span>
            </div>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,lineHeight:1.8,padding:12}}>
              {pod.logs.map((l:any,i:number)=>(
                <div key={i}>
                  <span style={{color:'#3d5070'}}>{l.time}</span>{' '}
                  <span style={{color:LOG_C[l.level],fontWeight:700}}>[{l.level}]</span>{' '}
                  <span style={{color:l.level==='ERROR'?'#ff7a92':'#8fa8cc'}}>{l.msg}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {tab==='events'&&(
          <div style={{display:'flex',flexDirection:'column',gap:6}}>
            {pod.events.map((e:any,i:number)=>(
              <div key={i} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${e.type==='Warning'?'rgba(245,166,35,.2)':'#1a2d4a'}`,borderRadius:8,padding:'8px 12px',display:'flex',gap:10,alignItems:'flex-start'}}>
                <div style={{width:6,height:6,borderRadius:'50%',background:EV_C[e.type],marginTop:5,flexShrink:0}}/>
                <div style={{flex:1}}>
                  <div style={{display:'flex',gap:6,marginBottom:3}}>
                    <span style={{fontSize:9,color:'#3d5070',fontFamily:'JetBrains Mono,monospace'}}>{e.time}</span>
                    <span style={{fontSize:9,color:EV_C[e.type],fontWeight:700}}>{e.type}</span>
                    <span style={{fontSize:9,color:'#8fa8cc',fontWeight:600}}>{e.reason}</span>
                  </div>
                  <div style={{fontSize:11,color:'#c8d8ef'}}>{e.msg}</div>
                </div>
              </div>
            ))}
          </div>
        )}

        {tab==='yaml'&&(
          <div style={{background:'#080d1b',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <pre style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc',margin:0,lineHeight:1.6}}>{`apiVersion: v1
kind: Pod
metadata:
  name: ${pod.name}
  namespace: ${pod.namespace}
  labels:
${Object.entries(pod.labels).map(([k,v])=>`    ${k}: ${v}`).join('\n')}
spec:
  nodeName: ${pod.node}
  serviceAccountName: ${pod.serviceAccount}
  containers:
${pod.containers.map((c:any)=>`  - name: ${c.name}
    image: ${c.image}
    ports: [${c.ports}]
    resources:
      requests: { cpu: ${pod.cpuRequest}, memory: ${pod.memRequest} }
      limits: { cpu: ${pod.cpuLimit}, memory: ${pod.memLimit} }`).join('\n')}
status:
  phase: ${pod.status}
  podIP: ${pod.ip}
  hostIP: 10.0.42.18`}</pre>
          </div>
        )}
      </div>
    </div>
  )
}
