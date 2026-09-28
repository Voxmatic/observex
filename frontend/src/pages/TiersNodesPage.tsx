// TiersNodesPage.tsx — Application → Tier → Node hierarchy, per-node CPU/heap/GC/threads
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { apm } from '@/lib/api'
import { ChevronDown, ChevronRight, Cpu, Server, Activity, AlertTriangle, MemoryStick } from 'lucide-react'

const DEMO_TIERS = [
  { id:'checkout', name:'checkout-service', app:'ecommerce', status:'warning', avgCpu:62, avgMem:74, rps:284, err:3.2, nodeCount:3,
    nodes:[
      { name:'checkout-7f9d-xk2p1', status:'warning', cpu:72, mem:78, heap:412, heapMax:512, gc:14, threads:124, rps:98, pid:1842 },
      { name:'checkout-7f9d-lm8q3', status:'healthy', cpu:58, mem:71, heap:380, heapMax:512, gc:8,  threads:118, rps:94, pid:1843 },
      { name:'checkout-7f9d-nr4k7', status:'healthy', cpu:56, mem:73, heap:391, heapMax:512, gc:9,  threads:121, rps:92, pid:1844 },
    ]},
  { id:'user', name:'user-service', app:'ecommerce', status:'critical', avgCpu:48, avgMem:89, rps:1240, err:8.4, nodeCount:4,
    nodes:[
      { name:'user-svc-6b8c-pk9f1', status:'critical', cpu:52, mem:94, heap:892, heapMax:1024, gc:48, threads:248, rps:312, pid:2100 },
      { name:'user-svc-6b8c-jh5m2', status:'critical', cpu:49, mem:91, heap:871, heapMax:1024, gc:44, threads:241, rps:308, pid:2101 },
      { name:'user-svc-6b8c-qw3n4', status:'warning', cpu:46, mem:87, heap:844, heapMax:1024, gc:39, threads:238, rps:321, pid:2102 },
      { name:'user-svc-6b8c-ty1o8', status:'healthy', cpu:45, mem:84, heap:812, heapMax:1024, gc:31, threads:228, rps:299, pid:2103 },
    ]},
  { id:'payment', name:'payment-service', app:'ecommerce', status:'healthy', avgCpu:18, avgMem:42, rps:78, err:0.08, nodeCount:2,
    nodes:[
      { name:'payment-9a2e-cd4r2', status:'healthy', cpu:19, mem:43, heap:184, heapMax:512, gc:3, threads:48, rps:39, pid:3000 },
      { name:'payment-9a2e-ef6s4', status:'healthy', cpu:17, mem:41, heap:176, heapMax:512, gc:2, threads:45, rps:39, pid:3001 },
    ]},
  { id:'ml', name:'ml-inference', app:'ecommerce', status:'warning', avgCpu:94, avgMem:68, rps:48, err:0.72, nodeCount:4,
    nodes:[
      { name:'ml-svc-1a2b-xy9z1', status:'critical', cpu:94, mem:72, heap:0, heapMax:0, gc:0, threads:24, rps:12, pid:4000 },
      { name:'ml-svc-1a2b-ab8c2', status:'critical', cpu:91, mem:70, heap:0, heapMax:0, gc:0, threads:24, rps:12, pid:4001 },
      { name:'ml-svc-1a2b-cd7d3', status:'warning', cpu:88, mem:68, heap:0, heapMax:0, gc:0, threads:24, rps:12, pid:4002 },
      { name:'ml-svc-1a2b-ef6e4', status:'warning', cpu:85, mem:64, heap:0, heapMax:0, gc:0, threads:24, rps:12, pid:4003 },
    ]},
]

const STATUS_C: Record<string,string> = { healthy:'#0fcf8a', warning:'#f5a623', critical:'#ff4d6a' }

function MiniBar({ val, max=100, color }: { val:number; max?:number; color:string }) {
  return (
    <div style={{width:56,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
      <div style={{height:'100%',width:`${Math.min((val/max)*100,100)}%`,background:color,borderRadius:2}}/>
    </div>
  )
}

function HeapRing({ used, max, size=36 }: { used:number; max:number; size?:number }) {
  if (!max) return <span style={{fontSize:9,color:'#3d5070'}}>N/A</span>
  const pct = Math.min(used/max,1)
  const r = size/2-3, circ = Math.PI*r
  const color = pct>0.85?'#ff4d6a':pct>0.7?'#f5a623':'#6c72ff'
  return (
    <svg width={size} height={size} style={{flexShrink:0}}>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke="#1a2d4a" strokeWidth="3"/>
      <circle cx={size/2} cy={size/2} r={r} fill="none" stroke={color} strokeWidth="3"
        strokeDasharray={`${circ*pct} ${circ}`} strokeLinecap="round"
        transform={`rotate(-90 ${size/2} ${size/2})`}/>
      <text x={size/2} y={size/2+3} textAnchor="middle" fill={color} fontSize="8" fontWeight="700">{Math.round(pct*100)}%</text>
    </svg>
  )
}

export default function TiersNodesPage() {
  const [expanded, setExpanded] = useState<string[]>(['user'])
  const [selectedNode, setSelectedNode] = useState<any>(DEMO_TIERS[1].nodes[0])
  const toggle = (id:string) => setExpanded(e=>e.includes(id)?e.filter(x=>x!==id):[...e,id])

  const { data } = useQuery({
    queryKey: ['tiers-nodes'],
    queryFn: async () => { return { tiers: DEMO_TIERS } },
    refetchInterval: 30_000,
  })

  const tiers: typeof DEMO_TIERS = (data as any)?.tiers ?? DEMO_TIERS
  const critical = tiers.reduce((a,t)=>a+t.nodes.filter(n=>n.status==='critical').length,0)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'14px 20px 10px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',gap:10,alignItems:'center',marginBottom:10}}>
          <div style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Tiers & Nodes</div>
          <div style={{fontSize:12,color:'#546a88',flex:1}}>Application → Tier → Node · Per-instance CPU/memory/heap/GC · Thread analysis</div>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Services',v:tiers.length},{l:'Total Nodes',v:tiers.reduce((a,t)=>a+t.nodeCount,0)},{l:'Critical Nodes',v:critical,c:critical>0?'#ff4d6a':'#0fcf8a'},{l:'Avg CPU',v:`${Math.round(tiers.reduce((a,t)=>a+t.avgCpu,0)/tiers.length)}%`}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 12px'}}>
              <div style={{fontSize:8,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:14,fontWeight:800,color:(k as any).c||'#8fa8cc',fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Tiers accordion */}
        <div style={{flex:1,overflowY:'auto',padding:14}}>
          {tiers.map(tier=>{
            const isExp = expanded.includes(tier.id)
            return (
              <div key={tier.id} style={{marginBottom:10,background:'rgba(17,31,53,.7)',border:`1px solid ${STATUS_C[tier.status]}33`,borderRadius:12,overflow:'hidden'}}>
                {/* Tier header */}
                <div onClick={()=>toggle(tier.id)} style={{padding:'12px 16px',display:'flex',alignItems:'center',gap:12,cursor:'pointer',background:isExp?'rgba(108,114,255,.04)':'transparent'}}>
                  {isExp?<ChevronDown size={14} style={{color:'#546a88'}}/>:<ChevronRight size={14} style={{color:'#546a88'}}/>}
                  <div style={{width:8,height:8,borderRadius:'50%',background:STATUS_C[tier.status]}}/>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:14,fontWeight:700,color:'#f0f6ff',flex:1}}>{tier.name}</span>
                  <div style={{display:'flex',gap:14,fontSize:11,color:'#546a88'}}>
                    <span>{tier.nodeCount} nodes</span>
                    <span>CPU: <strong style={{color:tier.avgCpu>70?'#f5a623':'#8fa8cc'}}>{tier.avgCpu}%</strong></span>
                    <span>Mem: <strong style={{color:tier.avgMem>85?'#ff4d6a':'#8fa8cc'}}>{tier.avgMem}%</strong></span>
                    <span>Err: <strong style={{color:tier.err>2?'#ff4d6a':'#0fcf8a'}}>{tier.err}%</strong></span>
                    <span>RPS: <strong style={{color:'#8fa8cc'}}>{tier.rps}/s</strong></span>
                  </div>
                </div>

                {/* Node table */}
                {isExp&&(
                  <table style={{width:'100%',borderCollapse:'collapse',borderTop:'1px solid #1a2d4a'}}>
                    <thead>
                      <tr style={{background:'#080d1b'}}>
                        {['Node Instance','PID','Status','CPU','Mem %','Heap','GC/min','Threads','RPS'].map(h=>(
                          <th key={h} style={{padding:'7px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#3d5070',textTransform:'uppercase'}}>{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {tier.nodes.map((node,i)=>(
                        <tr key={i} onClick={()=>setSelectedNode(node)}
                          style={{borderTop:'1px solid rgba(26,45,74,.3)',cursor:'pointer',background:selectedNode?.name===node.name?'rgba(108,114,255,.06)':'transparent'}}>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc'}}>{node.name}</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:9,color:'#3d5070'}}>{node.pid}</td>
                          <td style={{padding:'9px 12px'}}>
                            <div style={{display:'flex',gap:4,alignItems:'center'}}>
                              <div style={{width:6,height:6,borderRadius:'50%',background:STATUS_C[node.status]}}/>
                              <span style={{color:STATUS_C[node.status],fontSize:10}}>{node.status}</span>
                            </div>
                          </td>
                          <td style={{padding:'9px 12px'}}>
                            <div style={{display:'flex',gap:6,alignItems:'center'}}>
                              <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:node.cpu>80?'#ff4d6a':node.cpu>60?'#f5a623':'#8fa8cc',fontWeight:700,width:28}}>{node.cpu}%</span>
                              <MiniBar val={node.cpu} color={node.cpu>80?'#ff4d6a':node.cpu>60?'#f5a623':'#0fcf8a'}/>
                            </div>
                          </td>
                          <td style={{padding:'9px 12px'}}>
                            <div style={{display:'flex',gap:6,alignItems:'center'}}>
                              <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:node.mem>88?'#ff4d6a':node.mem>75?'#f5a623':'#8fa8cc',fontWeight:700,width:28}}>{node.mem}%</span>
                              <MiniBar val={node.mem} color={node.mem>88?'#ff4d6a':node.mem>75?'#f5a623':'#0fcf8a'}/>
                            </div>
                          </td>
                          <td style={{padding:'9px 12px'}}>
                            {node.heapMax>0
                              ? <div style={{display:'flex',gap:6,alignItems:'center'}}>
                                  <HeapRing used={node.heap} max={node.heapMax} size={32}/>
                                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:9,color:'#546a88'}}>{node.heap}/{node.heapMax}MB</span>
                                </div>
                              : <span style={{fontSize:9,color:'#3d5070'}}>N/A</span>}
                          </td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:node.gc>30?'#f5a623':'#8fa8cc'}}>{node.gc}</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc'}}>{node.threads}</td>
                          <td style={{padding:'9px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc'}}>{node.rps}/s</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            )
          })}
        </div>

        {/* Node detail sidebar */}
        {selectedNode&&(
          <div style={{width:240,borderLeft:'1px solid #1a2d4a',overflowY:'auto',padding:14,flexShrink:0}}>
            <div style={{fontSize:12,fontWeight:800,color:'#f0f6ff',marginBottom:12,fontFamily:'Syne,sans-serif'}}>Node Detail</div>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#6c72ff',marginBottom:12,wordBreak:'break-all'}}>{selectedNode.name}</div>
            {[
              {l:'Status', v:selectedNode.status, c:STATUS_C[selectedNode.status]},
              {l:'PID', v:selectedNode.pid, c:'#546a88'},
              {l:'CPU Usage', v:`${selectedNode.cpu}%`, c:selectedNode.cpu>80?'#ff4d6a':'#8fa8cc'},
              {l:'Memory', v:`${selectedNode.mem}%`, c:selectedNode.mem>85?'#ff4d6a':'#8fa8cc'},
              {l:'Heap Used', v:selectedNode.heapMax?`${selectedNode.heap}MB`:'N/A', c:'#8fa8cc'},
              {l:'Heap Max', v:selectedNode.heapMax?`${selectedNode.heapMax}MB`:'N/A', c:'#546a88'},
              {l:'GC/min', v:selectedNode.gc, c:selectedNode.gc>30?'#f5a623':'#8fa8cc'},
              {l:'Threads', v:selectedNode.threads, c:'#8fa8cc'},
              {l:'Requests/s', v:`${selectedNode.rps}/s`, c:'#8fa8cc'},
            ].map(k=>(
              <div key={k.l} style={{display:'flex',justifyContent:'space-between',fontSize:11,paddingBottom:7,marginBottom:7,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                <span style={{color:'#546a88'}}>{k.l}</span>
                <span style={{color:k.c,fontWeight:600,fontFamily:'JetBrains Mono,monospace'}}>{k.v}</span>
              </div>
            ))}
            <div style={{display:'flex',flexDirection:'column',gap:6,marginTop:12}}>
              <button style={{background:'#6c72ff',color:'#fff',border:'none',padding:'7px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Profiling</button>
              <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'7px',borderRadius:8,fontSize:11,cursor:'pointer'}}>View Logs</button>
              <button style={{background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.25)',color:'#ff4d6a',padding:'7px',borderRadius:8,fontSize:11,cursor:'pointer'}}>Restart Node</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
