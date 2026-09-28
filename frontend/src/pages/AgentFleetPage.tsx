// AgentFleetPage.tsx — Agent lifecycle management: update, restart, configure at scale
import { useState } from 'react'
import { Server, RefreshCw, Download, Settings, Trash2, CheckCircle, AlertTriangle, Clock, ChevronDown } from 'lucide-react'
import toast from 'react-hot-toast'

const AGENTS = [
  { id:'ag-001', host:'k8s-node-01.prod', ip:'10.0.1.10', version:'2.0.0', mode:'Full-Stack', status:'healthy', platform:'linux/amd64', uptime:'14d 6h', lastSeen:'2s ago', cpu:12, mem:148, services:24, namespace:'production', autoUpdate:true },
  { id:'ag-002', host:'k8s-node-02.prod', ip:'10.0.1.11', version:'2.0.0', mode:'Full-Stack', status:'healthy', platform:'linux/amd64', uptime:'14d 6h', lastSeen:'1s ago', cpu:18, mem:172, services:22, namespace:'production', autoUpdate:true },
  { id:'ag-003', host:'k8s-node-03.prod', ip:'10.0.1.12', version:'1.9.2', mode:'Full-Stack', status:'update-available', platform:'linux/amd64', uptime:'42d 2h', lastSeen:'3s ago', cpu:8, mem:124, services:18, namespace:'production', autoUpdate:false },
  { id:'ag-004', host:'db-host-01.prod',  ip:'10.0.2.10', version:'2.0.0', mode:'Infrastructure', status:'healthy', platform:'linux/amd64', uptime:'8d 14h', lastSeen:'5s ago', cpu:4, mem:96, services:3, namespace:'database', autoUpdate:true },
  { id:'ag-005', host:'payment-host-01',  ip:'10.0.3.10', version:'2.0.0', mode:'Full-Stack', status:'healthy', platform:'linux/amd64', uptime:'22d 8h', lastSeen:'2s ago', cpu:6, mem:112, services:8, namespace:'payment', autoUpdate:true },
  { id:'ag-006', host:'ml-host-01.prod',  ip:'10.0.4.10', version:'1.8.0', mode:'Full-Stack', status:'outdated', platform:'linux/arm64', uptime:'61d 3h', lastSeen:'8s ago', cpu:94, mem:3800, services:4, namespace:'ml', autoUpdate:false },
  { id:'ag-007', host:'monitoring-01',    ip:'10.0.5.10', version:'2.0.0', mode:'Discovery', status:'healthy', platform:'linux/amd64', uptime:'3d 18h', lastSeen:'4s ago', cpu:2, mem:64, services:0, namespace:'monitoring', autoUpdate:true },
  { id:'ag-008', host:'win-app-srv-01',   ip:'10.0.6.10', version:'2.0.0', mode:'Full-Stack', status:'healthy', platform:'windows/amd64', uptime:'7d 4h', lastSeen:'6s ago', cpu:28, mem:284, services:6, namespace:'legacy', autoUpdate:true },
]

const statusC: Record<string,string> = {
  healthy:'#0fcf8a', 'update-available':'#f5a623', outdated:'#ff4d6a', offline:'#546a88', warning:'#f5a623'
}
const statusLabel: Record<string,string> = {
  healthy:'Healthy', 'update-available':'Update Available', outdated:'Outdated', offline:'Offline'
}

export default function AgentFleetPage() {
  const [agents, setAgents] = useState(AGENTS)
  const [selected, setSelected] = useState<string[]>([])
  const [filter, setFilter] = useState('all')

  const needsUpdate = agents.filter(a=>a.status!=='healthy').length
  const toggleSelect = (id:string) => setSelected(s=>s.includes(id)?s.filter(x=>x!==id):[...s,id])
  const selectAll = () => setSelected(selected.length===filtered.length?[]:filtered.map(a=>a.id))

  const bulkUpdate = () => {
    const ids = selected.length ? selected : agents.filter(a=>a.status!=='healthy').map(a=>a.id)
    setAgents(a=>a.map(x=>ids.includes(x.id)?{...x,version:'2.0.0',status:'healthy'}:x))
    toast.success(`Updated ${ids.length} agents to v2.0.0`)
    setSelected([])
  }

  const filtered = filter==='all' ? agents : agents.filter(a=>
    filter==='needs-update' ? a.status!=='healthy' : a.namespace===filter
  )

  const namespaces = [...new Set(agents.map(a=>a.namespace))]

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🚢</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Agent Fleet</div>
            <div style={{fontSize:12,color:'#546a88'}}>Centralized agent lifecycle management · Bulk update · Remote restart · Coverage view</div>
          </div>
          <div style={{display:'flex',gap:8}}>
            {needsUpdate>0&&<button onClick={bulkUpdate} style={{display:'flex',gap:5,alignItems:'center',background:'#f5a623',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>
              <Download size={12}/>Update {needsUpdate} agents
            </button>}
            {selected.length>0&&<button onClick={()=>{toast.success(`Restarted ${selected.length} agents`);setSelected([])}} style={{display:'flex',gap:5,alignItems:'center',background:'#6c72ff',color:'#fff',border:'none',padding:'7px 14px',borderRadius:8,fontSize:11,fontWeight:700,cursor:'pointer'}}>
              <RefreshCw size={12}/>Restart ({selected.length})
            </button>}
          </div>
        </div>
        <div style={{display:'flex',gap:10,marginBottom:10}}>
          {[{l:'Total Agents',v:agents.length,c:'#6c72ff'},{l:'Healthy',v:agents.filter(a=>a.status==='healthy').length,c:'#0fcf8a'},{l:'Needs Update',v:needsUpdate,c:needsUpdate>0?'#f5a623':'#0fcf8a'},{l:'v2.0.0',v:agents.filter(a=>a.version==='2.0.0').length,c:'#8b90ff'},{l:'Services Covered',v:agents.reduce((a,x)=>a+x.services,0),c:'#8fa8cc'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
        <div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
          {[{v:'all',l:'All'},{v:'needs-update',l:'Needs Update'},...namespaces.map(n=>({v:n,l:n}))].map(f=>(
            <button key={f.v} onClick={()=>setFilter(f.v)} style={{padding:'4px 10px',border:`1px solid ${filter===f.v?'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:filter===f.v?'rgba(108,114,255,.15)':'transparent',color:filter===f.v?'#8b90ff':'#546a88',fontSize:10,cursor:'pointer'}}>{f.l}</button>
          ))}
        </div>
      </div>

      <div style={{flex:1,overflowY:'auto'}}>
        <table style={{width:'100%',borderCollapse:'collapse'}}>
          <thead style={{position:'sticky',top:0,background:'#080d1b',zIndex:1}}>
            <tr style={{borderBottom:'1px solid #1a2d4a'}}>
              <th style={{padding:'8px 12px',width:36}}>
                <input type="checkbox" checked={selected.length===filtered.length&&filtered.length>0} onChange={selectAll} style={{accentColor:'#6c72ff'}}/>
              </th>
              {['Host','IP','Version','Mode','Platform','Status','Uptime','CPU','Mem','Services','Actions'].map(h=>(
                <th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase',whiteSpace:'nowrap'}}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {filtered.map(a=>(
              <tr key={a.id} style={{borderBottom:'1px solid rgba(26,45,74,.4)',background:selected.includes(a.id)?'rgba(108,114,255,.05)':'transparent'}}>
                <td style={{padding:'10px 12px'}}>
                  <input type="checkbox" checked={selected.includes(a.id)} onChange={()=>toggleSelect(a.id)} style={{accentColor:'#6c72ff'}}/>
                </td>
                <td style={{padding:'10px 12px'}}>
                  <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,fontWeight:700,color:'#f0f6ff'}}>{a.host}</div>
                  <div style={{fontSize:9,color:'#546a88'}}>{a.namespace}</div>
                </td>
                <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88'}}>{a.ip}</td>
                <td style={{padding:'10px 12px'}}>
                  <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,fontWeight:700,color:a.version==='2.0.0'?'#0fcf8a':a.version==='1.9.2'?'#f5a623':'#ff4d6a'}}>{a.version}</span>
                </td>
                <td style={{padding:'10px 12px'}}><span style={{background:'rgba(108,114,255,.15)',color:'#8b90ff',fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:600}}>{a.mode}</span></td>
                <td style={{padding:'10px 12px',fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{a.platform}</td>
                <td style={{padding:'10px 12px'}}>
                  <div style={{display:'flex',alignItems:'center',gap:5}}>
                    <div style={{width:6,height:6,borderRadius:'50%',background:statusC[a.status]||'#546a88'}}/>
                    <span style={{color:statusC[a.status],fontSize:10,fontWeight:600}}>{statusLabel[a.status]||a.status}</span>
                  </div>
                </td>
                <td style={{padding:'10px 12px',fontSize:10,color:'#546a88'}}>{a.uptime}</td>
                <td style={{padding:'10px 12px'}}>
                  <div style={{display:'flex',gap:5,alignItems:'center'}}>
                    <div style={{width:36,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}><div style={{height:'100%',width:`${a.cpu}%`,background:a.cpu>80?'#ff4d6a':a.cpu>60?'#f5a623':'#6c72ff'}}/></div>
                    <span style={{fontSize:9,color:a.cpu>80?'#ff4d6a':'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{a.cpu}%</span>
                  </div>
                </td>
                <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc'}}>{a.mem}Mi</td>
                <td style={{padding:'10px 12px',fontSize:11,color:'#8fa8cc'}}>{a.services}</td>
                <td style={{padding:'10px 12px'}}>
                  <div style={{display:'flex',gap:5}}>
                    {a.status!=='healthy'&&<button onClick={()=>{setAgents(ag=>ag.map(x=>x.id===a.id?{...x,version:'2.0.0',status:'healthy'}:x));toast.success('Updated to v2.0.0')}} style={{background:'rgba(15,207,138,.1)',border:'1px solid rgba(15,207,138,.2)',color:'#0fcf8a',padding:'3px 8px',borderRadius:6,fontSize:9,cursor:'pointer'}}>Update</button>}
                    <button onClick={()=>toast.success(`Restarted ${a.host}`)} style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#546a88',padding:'3px 8px',borderRadius:6,fontSize:9,cursor:'pointer'}}>
                      <RefreshCw size={9}/>
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
