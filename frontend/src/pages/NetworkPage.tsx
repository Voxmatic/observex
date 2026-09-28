// NetworkPage.tsx — Network Performance Monitoring (NPM): flows, topology, DNS, TCP, devices
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { network } from '@/lib/api'
import { Wifi, Server, Activity, AlertTriangle, TrendingUp, Globe, Shield } from 'lucide-react'

const FLOWS = [
  { src:'checkout-service', dst:'postgres-primary', proto:'TCP', port:5432, rps:284, latency:12, packet_loss:0.0, bytes_sec:2.4, status:'healthy' },
  { src:'user-service',     dst:'postgres-primary', proto:'TCP', port:5432, rps:1240, latency:4200, packet_loss:0.2, bytes_sec:8.1, status:'critical' },
  { src:'api-gateway',      dst:'checkout-service', proto:'HTTP', port:8080, rps:2140, latency:184, packet_loss:0.0, bytes_sec:18.4, status:'healthy' },
  { src:'api-gateway',      dst:'user-service',     proto:'HTTP', port:8081, rps:1240, latency:4200, packet_loss:0.1, bytes_sec:9.2, status:'critical' },
  { src:'checkout-service', dst:'redis-cache',      proto:'TCP',  port:6379, rps:4820, latency:1, packet_loss:0.0, bytes_sec:3.8, status:'healthy' },
  { src:'ml-inference',     dst:'redis-cache',      proto:'TCP',  port:6379, rps:840, latency:2, packet_loss:0.0, bytes_sec:1.2, status:'healthy' },
  { src:'api-gateway',      dst:'ml-inference',     proto:'HTTP', port:8082, rps:48, latency:1240, packet_loss:0.0, bytes_sec:0.8, status:'warning' },
  { src:'payment-service',  dst:'stripe-api',       proto:'HTTPS',port:443, rps:78, latency:89, packet_loss:0.0, bytes_sec:0.4, status:'healthy' },
  { src:'notification-svc', dst:'kafka-broker-1',   proto:'TCP',  port:9092, rps:312, latency:3, packet_loss:0.0, bytes_sec:2.1, status:'healthy' },
]

const DEVICES = [
  { name:'core-switch-01', type:'SWITCH', vendor:'Cisco Catalyst', ip:'10.0.0.1', status:'healthy', uptime:'142d', ports:48, utilization:34, packets_per_sec:84200 },
  { name:'core-switch-02', type:'SWITCH', vendor:'Cisco Catalyst', ip:'10.0.0.2', status:'healthy', uptime:'142d', ports:48, utilization:28, packets_per_sec:62800 },
  { name:'edge-router-01', type:'ROUTER', vendor:'Juniper MX', ip:'10.0.1.1', status:'warning', uptime:'72d', ports:24, utilization:81, packets_per_sec:284000 },
  { name:'firewall-01',    type:'FIREWALL', vendor:'Palo Alto', ip:'10.0.2.1', status:'healthy', uptime:'89d', ports:16, utilization:42, packets_per_sec:148000 },
  { name:'lb-01',          type:'LOAD_BALANCER', vendor:'F5', ip:'10.0.3.1', status:'healthy', uptime:'52d', ports:8, utilization:67, packets_per_sec:412000 },
]

const DNS_QUERIES = [
  { domain:'postgres-primary.production.svc', queries_per_min:4820, latency_ms:0.4, errors:0, status:'healthy' },
  { domain:'redis-cache.production.svc',      queries_per_min:8410, latency_ms:0.3, errors:0, status:'healthy' },
  { domain:'kafka-broker-1.production.svc',   queries_per_min:1240, latency_ms:0.8, errors:2, status:'warning' },
  { domain:'stripe.com',                       queries_per_min:78, latency_ms:12.4, errors:0, status:'healthy' },
  { domain:'api.openai.com',                   queries_per_min:48, latency_ms:18.2, errors:1, status:'warning' },
]

const statusC: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }
const protoC: Record<string,string>  = { TCP:'#6c72ff', HTTP:'#0fcf8a', HTTPS:'#a855f7', gRPC:'#f5a623' }
const deviceType: Record<string,string> = { SWITCH:'🔀', ROUTER:'🌐', FIREWALL:'🛡', LOAD_BALANCER:'⚖️' }

function MiniBar({v,max,color}:{v:number,max:number,color:string}) {
  return <div style={{width:50,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}><div style={{height:'100%',width:`${Math.min((v/max)*100,100)}%`,background:color}}/></div>
}

export default function NetworkPage() {
  const [tab, setTab] = useState<'flows'|'devices'|'dns'|'topology'>('flows')

  const { data } = useQuery({
    queryKey: ['network-flows'],
    queryFn: async () => { try { return await network.flows() } catch { return { flows: FLOWS } } },
    refetchInterval: 15_000,
  })

  const flows = (data as any)?.flows ?? FLOWS
  const criticalFlows = flows.filter((f:any) => f.status === 'critical').length
  const totalRps = flows.reduce((a:number,f:any) => a + f.rps, 0)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🌐</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Network Performance</div>
            <div style={{fontSize:12,color:'#546a88'}}>Inter-service flows · Device health · DNS latency · TCP connections · Packet loss</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10,marginBottom:10}}>
          {[
            {l:'Service Flows',  v:flows.length,         c:'#6c72ff'},
            {l:'Critical Flows', v:criticalFlows,        c:criticalFlows>0?'#ff4d6a':'#0fcf8a'},
            {l:'Total RPS',      v:totalRps.toLocaleString(), c:'#8b90ff'},
            {l:'Network Devices',v:DEVICES.length,       c:'#8fa8cc'},
            {l:'Cluster Rx',     v:'18.4 MB/s',          c:'#0fcf8a'},
            {l:'Cluster Tx',     v:'12.8 MB/s',          c:'#0fcf8a'},
          ].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['flows','devices','dns','topology'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'7px 18px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t==='dns'?'DNS':t}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto',padding:16}}>
        {tab==='flows'&&(
          <div>
            <div style={{marginBottom:12,fontSize:12,color:'#546a88'}}>Real-time inter-service network flows with latency, throughput, and packet loss</div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <table style={{width:'100%',borderCollapse:'collapse'}}>
                <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                  {['Source','→ Destination','Protocol','RPS','Latency','Packet Loss','Throughput','Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase',whiteSpace:'nowrap'}}>{h}</th>)}
                </tr></thead>
                <tbody>
                  {flows.map((f:any,i:number)=>(
                    <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                      <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8b90ff',fontWeight:600}}>{f.src}</td>
                      <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{f.dst}</td>
                      <td style={{padding:'9px 12px'}}><span style={{background:`${protoC[f.proto]||'#546a88'}22`,color:protoC[f.proto]||'#546a88',fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8}}>{f.proto}:{f.port}</span></td>
                      <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{f.rps.toLocaleString()}/s</td>
                      <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:f.latency>500?'#ff4d6a':f.latency>100?'#f5a623':'#0fcf8a',fontWeight:700}}>{f.latency}ms</td>
                      <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:f.packet_loss>0.1?'#f5a623':'#0fcf8a'}}>{f.packet_loss}%</td>
                      <td style={{padding:'9px 12px',fontSize:11,color:'#546a88'}}>{f.bytes_sec} MB/s</td>
                      <td style={{padding:'9px 12px'}}><div style={{display:'flex',alignItems:'center',gap:5}}><div style={{width:6,height:6,borderRadius:'50%',background:statusC[f.status]}}/><span style={{color:statusC[f.status],fontSize:11,fontWeight:600}}>{f.status}</span></div></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {tab==='devices'&&(
          <div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:12,marginBottom:14}}>
              {DEVICES.map(d=>(
                <div key={d.name} style={{background:'rgba(17,31,53,.7)',border:`1px solid ${statusC[d.status]}33`,borderRadius:12,padding:14}}>
                  <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
                    <span style={{fontSize:24}}>{deviceType[d.type]||'📡'}</span>
                    <div style={{flex:1}}>
                      <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff'}}>{d.name}</div>
                      <div style={{fontSize:10,color:'#546a88'}}>{d.vendor} · {d.ip}</div>
                    </div>
                    <div style={{width:8,height:8,borderRadius:'50%',background:statusC[d.status]}}/>
                  </div>
                  <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:6}}>
                    {[{l:'Uptime',v:d.uptime,c:'#0fcf8a'},{l:'Ports',v:d.ports,c:'#8b90ff'},{l:'PPS',v:d.packets_per_sec.toLocaleString(),c:'#8fa8cc'},{l:'Utilization',v:`${d.utilization}%`,c:d.utilization>80?'#f5a623':'#8fa8cc'}].map(k=>(
                      <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 8px'}}>
                        <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                        <div style={{fontSize:13,fontWeight:700,color:k.c}}>{k.v}</div>
                      </div>
                    ))}
                  </div>
                  <div style={{marginTop:8}}>
                    <div style={{display:'flex',justifyContent:'space-between',fontSize:9,color:'#546a88',marginBottom:3}}>
                      <span>Port utilization</span><span style={{color:d.utilization>80?'#f5a623':'#8fa8cc'}}>{d.utilization}%</span>
                    </div>
                    <div style={{height:5,background:'#0b1628',borderRadius:3,overflow:'hidden'}}><div style={{height:'100%',width:`${d.utilization}%`,background:d.utilization>80?'#f5a623':'#6c72ff'}}/></div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {tab==='dns'&&(
          <div>
            <div style={{marginBottom:12,fontSize:12,color:'#546a88'}}>DNS query monitoring — latency, error rates, top domains</div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <table style={{width:'100%',borderCollapse:'collapse'}}>
                <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                  {['Domain','Queries/min','DNS Latency','Errors','Status'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
                </tr></thead>
                <tbody>
                  {DNS_QUERIES.map((d,i)=>(
                    <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                      <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#c8d8ef',fontWeight:600}}>{d.domain}</td>
                      <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{d.queries_per_min.toLocaleString()}</td>
                      <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:d.latency_ms>10?'#f5a623':'#0fcf8a',fontWeight:700}}>{d.latency_ms}ms</td>
                      <td style={{padding:'10px 12px',fontSize:11,color:d.errors>0?'#ff4d6a':'#0fcf8a',fontWeight:d.errors>0?700:400}}>{d.errors}</td>
                      <td style={{padding:'10px 12px'}}><div style={{display:'flex',alignItems:'center',gap:5}}><div style={{width:6,height:6,borderRadius:'50%',background:statusC[d.status]}}/><span style={{color:statusC[d.status],fontSize:11}}>{d.status}</span></div></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        {tab==='topology'&&(
          <div>
            <div style={{marginBottom:12,fontSize:12,color:'#546a88'}}>Network topology visualization — service-to-service call graph with flow metrics</div>
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:20,minHeight:400,position:'relative',overflow:'hidden'}}>
              {/* SVG network diagram */}
              <svg width="100%" height="380" viewBox="0 0 800 380" style={{overflow:'visible'}}>
                <defs>
                  <marker id="arrow" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                    <polygon points="0 0, 10 3.5, 0 7" fill="#6c72ff" opacity="0.6"/>
                  </marker>
                  <marker id="arrow-red" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                    <polygon points="0 0, 10 3.5, 0 7" fill="#ff4d6a" opacity="0.8"/>
                  </marker>
                </defs>
                {/* Nodes */}
                {[
                  {x:400,y:50,  label:'api-gateway',       sub:'2,140 rps',color:'#8b90ff',status:'warning'},
                  {x:200,y:180, label:'checkout-service',  sub:'284 rps',  color:'#f5a623',status:'warning'},
                  {x:600,y:180, label:'user-service',      sub:'1,240 rps',color:'#ff4d6a',status:'critical'},
                  {x:100,y:300, label:'postgres-primary',  sub:'DB',       color:'#ff4d6a',status:'critical'},
                  {x:300,y:300, label:'redis-cache',       sub:'Cache',    color:'#0fcf8a',status:'healthy'},
                  {x:500,y:300, label:'payment-service',   sub:'78 rps',   color:'#0fcf8a',status:'healthy'},
                  {x:700,y:300, label:'ml-inference',      sub:'48 rps',   color:'#f5a623',status:'warning'},
                ].map((n,i)=>(
                  <g key={i}>
                    <circle cx={n.x} cy={n.y} r="40" fill={`${n.color}15`} stroke={n.color} strokeWidth="1.5"/>
                    <text x={n.x} y={n.y-5} textAnchor="middle" fill={n.color} fontSize="9" fontWeight="800" fontFamily="JetBrains Mono,monospace">{n.label.split('-')[0]}</text>
                    <text x={n.x} y={n.y+8} textAnchor="middle" fill={n.color} fontSize="7" fontFamily="JetBrains Mono,monospace">{n.label.includes('-')&&n.label.split('-').slice(1).join('-')}</text>
                    <text x={n.x} y={n.y+20} textAnchor="middle" fill="#546a88" fontSize="8">{n.sub}</text>
                    <circle cx={n.x+35} cy={n.y-35} r="5" fill={n.status==='critical'?'#ff4d6a':n.status==='warning'?'#f5a623':'#0fcf8a'}/>
                  </g>
                ))}
                {/* Edges */}
                {[
                  {x1:400,y1:90, x2:220,y2:145,color:'#6c72ff',label:'2140/s'},
                  {x1:400,y1:90, x2:580,y2:145,color:'#ff4d6a',label:'1240/s'},
                  {x1:200,y1:220,x2:120,y2:260,color:'#ff4d6a',label:'latency!'},
                  {x1:200,y1:220,x2:290,y2:260,color:'#0fcf8a',label:'ok'},
                  {x1:200,y1:220,x2:490,y2:265,color:'#0fcf8a',label:'78/s'},
                  {x1:400,y1:90, x2:695,y2:145,color:'#6c72ff',label:'48/s'},
                ].map((e,i)=>(
                  <g key={i}>
                    <line x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} stroke={e.color} strokeWidth="1.5" opacity="0.6" markerEnd={`url(#arrow${e.color==='#ff4d6a'?'-red':''})`}/>
                    <text x={(e.x1+e.x2)/2} y={(e.y1+e.y2)/2-5} textAnchor="middle" fill={e.color} fontSize="8" opacity="0.8">{e.label}</text>
                  </g>
                ))}
              </svg>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
