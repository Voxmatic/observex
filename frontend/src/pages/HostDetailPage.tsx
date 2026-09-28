// HostDetailPage.tsx — /infrastructure/:hostId — Host deep dive with processes
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { TimeRangePicker } from '@/components/shared/TimeRangePicker'
import { MetricCard } from '@/components/shared/MetricCard'
import { SparkLine } from '@/components/shared/SparkLine'
import { ArrowLeft, Cpu, MemoryStick, HardDrive, Wifi, Activity } from 'lucide-react'

const HOSTS: Record<string,any> = {
  'k8s-node-01': {
    id:'k8s-node-01', hostname:'k8s-node-01.prod.acme.io', os:'Ubuntu 24.04 LTS', kernel:'6.5.0-44-generic',
    arch:'x86_64', cpuModel:'AMD EPYC 7763 64-Core', cpuCores:16, memTotal:'64 GB', diskTotal:'500 GB',
    status:'healthy', cloud:'AWS', region:'us-east-1', instanceType:'m6a.4xlarge', ip:'10.0.42.18', publicIp:'54.23.184.92',
    uptime:'42d 8h', agent:'observex-agent v1.2.0',
    cpu:42, mem:68, diskUsed:52, networkIn:'284 MB/s', networkOut:'142 MB/s', loadAvg:'3.24 2.18 1.84', iops:4200,
    cpuTrend:[38,42,44,48,42,40,38,42,46,42,40,38,42,44,42,40],
    memTrend:[64,65,66,68,68,67,66,68,69,68,67,66,68,68,67,68],
    processes:[
      { pid:1842, name:'api-gateway', user:'observex', cpu:12.4, mem:2.1, memMB:1344, threads:124, status:'running', type:'service' },
      { pid:2100, name:'user-service', user:'observex', cpu:8.2, mem:4.8, memMB:3072, threads:248, status:'running', type:'service' },
      { pid:2200, name:'checkout-service', user:'observex', cpu:6.8, mem:1.6, memMB:1024, threads:84, status:'running', type:'service' },
      { pid:3000, name:'payment-service', user:'observex', cpu:2.1, mem:0.8, memMB:512, threads:48, status:'running', type:'service' },
      { pid:8420, name:'postgres', user:'postgres', cpu:18.4, mem:24.2, memMB:15488, threads:42, status:'running', type:'database' },
      { pid:8500, name:'redis-server', user:'redis', cpu:1.2, mem:0.4, memMB:256, threads:4, status:'running', type:'cache' },
      { pid:9100, name:'observex-native-collector', user:'root', cpu:0.1, mem:0.1, memMB:64, threads:8, status:'running', type:'agent' },
      { pid:9200, name:'observex-agent', user:'root', cpu:0.8, mem:0.2, memMB:128, threads:12, status:'running', type:'agent' },
    ],
    containers:[
      { name:'api-gateway-container', image:'observex/api-gateway:v2.0.0', cpu:12.4, mem:1344, status:'running', restarts:0 },
      { name:'user-svc-container', image:'observex/user-service:v2.4.0', cpu:8.2, mem:3072, status:'running', restarts:3 },
      { name:'checkout-svc-container', image:'observex/checkout:v2.5.0', cpu:6.8, mem:1024, status:'running', restarts:0 },
    ],
    disks:[
      { mount:'/', device:'/dev/nvme0n1p1', size:'500 GB', used:'260 GB', usedPct:52, type:'nvme' },
      { mount:'/data/postgres', device:'/dev/nvme1n1', size:'1 TB', used:'420 GB', usedPct:42, type:'nvme' },
    ],
    networkInterfaces:[
      { name:'eth0', ip:'10.0.42.18', rx:'284 MB/s', tx:'142 MB/s', errors:0 },
      { name:'docker0', ip:'172.17.0.1', rx:'12 MB/s', tx:'8 MB/s', errors:0 },
    ],
  },
}

const PROC_TYPE_C: Record<string,string> = { service:'#6c72ff', database:'#a855f7', cache:'#0fcf8a', agent:'#546a88' }

export default function HostDetailPage() {
  const { hostId } = useParams<{hostId:string}>()
  const [timeRange, setTimeRange] = useState('1h')
  const [tab, setTab] = useState<'overview'|'processes'|'containers'|'disks'|'network'>('overview')
  const host = HOSTS[hostId||''] || HOSTS['k8s-node-01']

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
          <Link to="/infrastructure" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <div style={{width:8,height:8,borderRadius:'50%',background:'#0fcf8a'}}/>
          <div style={{flex:1}}>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{host.hostname}</div>
            <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginTop:2}}>
              <span>{host.os}</span><span>{host.cloud} {host.region}</span><span>{host.instanceType}</span>
              <span>{host.cpuCores} cores</span><span>{host.memTotal}</span><span>Up: {host.uptime}</span>
            </div>
          </div>
          <TimeRangePicker value={timeRange} onChange={setTimeRange}/>
        </div>
        <div style={{display:'flex',gap:6}}>
          <MetricCard label="CPU" value={`${host.cpu}%`} color={host.cpu>70?'#f5a623':'#0fcf8a'} trend={host.cpuTrend} small/>
          <MetricCard label="Memory" value={`${host.mem}%`} color={host.mem>85?'#ff4d6a':'#8fa8cc'} trend={host.memTrend} small/>
          <MetricCard label="Disk" value={`${host.diskUsed}%`} color={host.diskUsed>80?'#ff4d6a':'#8fa8cc'} small/>
          <MetricCard label="Net In" value={host.networkIn} color="#6c72ff" small/>
          <MetricCard label="Net Out" value={host.networkOut} color="#8b90ff" small/>
          <MetricCard label="Load" value={host.loadAvg.split(' ')[0]} color="#8fa8cc" small/>
          <MetricCard label="IOPS" value={host.iops.toLocaleString()} color="#8fa8cc" small/>
          <MetricCard label="Processes" value={host.processes.length} color="#6c72ff" small/>
        </div>
        <div style={{display:'flex',gap:0,marginTop:8}}>
          {(['overview','processes','containers','disks','network'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'6px 14px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:11,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='overview'&&(
          <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:14}}>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Host info</div>
              {[{l:'Hostname',v:host.hostname},{l:'IP (private)',v:host.ip},{l:'IP (public)',v:host.publicIp},{l:'OS',v:host.os},{l:'Kernel',v:host.kernel},{l:'CPU',v:`${host.cpuModel} (${host.cpuCores} cores)`},{l:'RAM',v:host.memTotal},{l:'Instance',v:host.instanceType},{l:'Agent',v:host.agent}].map(k=>(
                <div key={k.l} style={{display:'flex',justifyContent:'space-between',fontSize:11,paddingBottom:5,marginBottom:5,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                  <span style={{color:'#546a88'}}>{k.l}</span>
                  <span style={{color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace',textAlign:'right',maxWidth:280,overflow:'hidden',textOverflow:'ellipsis'}}>{k.v}</span>
                </div>
              ))}
            </div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:10}}>Top processes by CPU</div>
              {[...host.processes].sort((a:any,b:any)=>b.cpu-a.cpu).slice(0,6).map((p:any)=>(
                <div key={p.pid} style={{display:'flex',gap:8,alignItems:'center',marginBottom:8}}>
                  <div style={{width:6,height:6,borderRadius:'50%',background:PROC_TYPE_C[p.type]||'#546a88'}}/>
                  <span style={{fontSize:11,color:'#f0f6ff',flex:1,fontFamily:'JetBrains Mono,monospace'}}>{p.name}</span>
                  <span style={{fontSize:10,color:p.cpu>15?'#f5a623':'#8fa8cc'}}>{p.cpu}%</span>
                  <span style={{fontSize:10,color:'#546a88'}}>{p.memMB}MB</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {tab==='processes'&&(
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead><tr style={{background:'#080d1b'}}>
                {['PID','Name','User','Type','CPU %','Mem %','Mem MB','Threads','Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
              </tr></thead>
              <tbody>
                {host.processes.map((p:any)=>(
                  <tr key={p.pid} style={{borderTop:'1px solid rgba(26,45,74,.3)'}}>
                    <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88'}}>{p.pid}</td>
                    <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#f0f6ff',fontWeight:600}}>{p.name}</td>
                    <td style={{padding:'8px 12px',fontSize:10,color:'#546a88'}}>{p.user}</td>
                    <td style={{padding:'8px 12px'}}><span style={{fontSize:9,background:`${PROC_TYPE_C[p.type]}22`,color:PROC_TYPE_C[p.type],padding:'1px 6px',borderRadius:6,fontWeight:700}}>{p.type}</span></td>
                    <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:p.cpu>15?'#f5a623':'#8fa8cc',fontWeight:700}}>{p.cpu}%</td>
                    <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:p.mem>10?'#f5a623':'#8fa8cc'}}>{p.mem}%</td>
                    <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88'}}>{p.memMB}</td>
                    <td style={{padding:'8px 12px',fontSize:10,color:'#546a88'}}>{p.threads}</td>
                    <td style={{padding:'8px 12px'}}><div style={{width:6,height:6,borderRadius:'50%',background:'#0fcf8a',display:'inline-block'}}/></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {tab==='containers'&&(
          <div style={{display:'flex',flexDirection:'column',gap:10}}>
            {host.containers.map((c:any)=>(
              <div key={c.name} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14,display:'flex',gap:14,alignItems:'center'}}>
                <div style={{width:8,height:8,borderRadius:'50%',background:'#0fcf8a'}}/>
                <div style={{flex:1}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{c.name}</div>
                  <div style={{fontSize:10,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{c.image}</div>
                </div>
                <span style={{fontSize:11,color:'#8fa8cc'}}>CPU: {c.cpu}%</span>
                <span style={{fontSize:11,color:'#8fa8cc'}}>Mem: {c.mem}MB</span>
                <span style={{fontSize:11,color:c.restarts>0?'#f5a623':'#0fcf8a'}}>Restarts: {c.restarts}</span>
              </div>
            ))}
          </div>
        )}

        {tab==='disks'&&(
          <div style={{display:'flex',flexDirection:'column',gap:10}}>
            {host.disks.map((d:any)=>(
              <div key={d.mount} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14}}>
                <div style={{display:'flex',justifyContent:'space-between',marginBottom:8}}>
                  <span style={{fontSize:12,fontWeight:700,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace'}}>{d.mount}</span>
                  <span style={{fontSize:10,color:'#546a88'}}>{d.device} ({d.type})</span>
                </div>
                <div style={{height:8,background:'#0b1628',borderRadius:4,overflow:'hidden',marginBottom:6}}>
                  <div style={{height:'100%',width:`${d.usedPct}%`,background:d.usedPct>80?'#ff4d6a':d.usedPct>60?'#f5a623':'#0fcf8a'}}/>
                </div>
                <div style={{display:'flex',justifyContent:'space-between',fontSize:10,color:'#546a88'}}>
                  <span>{d.used} used</span><span>{d.size} total</span><span style={{fontWeight:700,color:d.usedPct>80?'#ff4d6a':'#8fa8cc'}}>{d.usedPct}%</span>
                </div>
              </div>
            ))}
          </div>
        )}

        {tab==='network'&&(
          <div style={{display:'flex',flexDirection:'column',gap:10}}>
            {host.networkInterfaces.map((n:any)=>(
              <div key={n.name} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:10,padding:14,display:'flex',gap:14,alignItems:'center'}}>
                <Wifi size={14} style={{color:'#6c72ff'}}/>
                <div style={{flex:1}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',fontFamily:'JetBrains Mono,monospace'}}>{n.name}</div>
                  <div style={{fontSize:10,color:'#546a88'}}>{n.ip}</div>
                </div>
                <span style={{fontSize:11,color:'#0fcf8a'}}>↓ {n.rx}</span>
                <span style={{fontSize:11,color:'#6c72ff'}}>↑ {n.tx}</span>
                <span style={{fontSize:11,color:n.errors>0?'#ff4d6a':'#0fcf8a'}}>Errors: {n.errors}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
