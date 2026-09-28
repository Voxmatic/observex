// FeatureFlagsPage.tsx — Feature flag management linked to APM impact analysis
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Flag, Plus, ToggleLeft, ToggleRight, Users, TrendingUp, AlertTriangle, Eye, Code } from 'lucide-react'
import toast from 'react-hot-toast'

const DEMO_FLAGS = [
  { id:'ff-001', key:'new-checkout-v2', name:'New Checkout V2 UI', status:'enabled', rollout:100, env:'production', service:'checkout-service', type:'release', created:'2026-04-10', impact:{ users:284000, conversionDelta:'+2.4%', errorDelta:'+3.2%', latencyDelta:'+180ms' }, tags:['checkout','ui','v2'] },
  { id:'ff-002', key:'ai-recommendations', name:'AI Product Recommendations', status:'partial', rollout:25, env:'production', service:'recommendation-svc', type:'experiment', created:'2026-04-14', impact:{ users:71000, conversionDelta:'+1.8%', errorDelta:'+0.1%', latencyDelta:'+840ms' }, tags:['ai','recommendations','experiment'] },
  { id:'ff-003', key:'new-payment-flow', name:'Streamlined Payment Flow', status:'disabled', rollout:0, env:'production', service:'payment-service', type:'release', created:'2026-04-15', impact:{ users:0, conversionDelta:'N/A', errorDelta:'N/A', latencyDelta:'N/A' }, tags:['payment','ux'] },
  { id:'ff-004', key:'dark-mode', name:'Dark Mode UI', status:'enabled', rollout:100, env:'production', service:'frontend', type:'release', created:'2026-03-20', impact:{ users:484000, conversionDelta:'+0.3%', errorDelta:'-0.1%', latencyDelta: '-5ms' }, tags:['ui','theme'] },
  { id:'ff-005', key:'beta-search', name:'Enhanced Search (Beta)', status:'partial', rollout:10, env:'production', service:'api-gateway', type:'experiment', created:'2026-04-16', impact:{ users:48400, conversionDelta:'+4.1%', errorDelta:'+0.8%', latencyDelta:'+320ms' }, tags:['search','beta'] },
  { id:'ff-006', key:'ml-pricing', name:'ML Dynamic Pricing', status:'disabled', rollout:0, env:'staging', service:'pricing-svc', type:'kill-switch', created:'2026-04-17', impact:{ users:0, conversionDelta:'N/A', errorDelta:'N/A', latencyDelta:'N/A' }, tags:['pricing','ml','kill-switch'] },
]

const typeC: Record<string,[string,string]> = {
  release:    ['rgba(15,207,138,.15)','#0fcf8a'],
  experiment: ['rgba(108,114,255,.15)','#8b90ff'],
  'kill-switch':['rgba(255,77,106,.15)','#ff4d6a'],
}

export default function FeatureFlagsPage() {
  const [flags, setFlags] = useState(DEMO_FLAGS)
  const [selected, setSelected] = useState(DEMO_FLAGS[0])
  const [search, setSearch] = useState('')
  const [envFilter, setEnvFilter] = useState('all')

  const toggle = (id: string) => {
    setFlags(f => f.map(x => x.id===id ? {...x, status: x.status==='enabled'?'disabled':'enabled', rollout: x.status==='enabled'?0:100} : x))
    toast.success('Flag updated')
  }

  const setRollout = (id: string, pct: number) => {
    setFlags(f => f.map(x => x.id===id ? {...x, rollout:pct, status: pct===0?'disabled':pct===100?'enabled':'partial'} : x))
  }

  const filtered = flags.filter(f =>
    (envFilter==='all' || f.env===envFilter) &&
    (!search || f.name.toLowerCase().includes(search.toLowerCase()) || f.key.includes(search.toLowerCase()))
  )

  const statusBg: Record<string,string> = { enabled:'rgba(15,207,138,.15)', partial:'rgba(245,166,35,.15)', disabled:'rgba(84,106,136,.1)' }
  const statusC: Record<string,string>  = { enabled:'#0fcf8a', partial:'#f5a623', disabled:'#546a88' }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🚩</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Feature Flags</div>
            <div style={{fontSize:12,color:'#546a88'}}>Gradual rollouts · A/B experiments · Kill switches · APM impact correlation</div>
          </div>
          <button style={{display:'flex',alignItems:'center',gap:6,background:'#6c72ff',color:'#fff',border:'none',padding:'8px 16px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}>
            <Plus size={13}/>New Flag
          </button>
        </div>
        <div style={{display:'flex',gap:10,alignItems:'center'}}>
          <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search flags..."
            style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 12px',fontSize:12,color:'#c8d8ef',outline:'none',width:250}}/>
          {['all','production','staging'].map(e=>(
            <button key={e} onClick={()=>setEnvFilter(e)} style={{padding:'5px 12px',border:`1px solid ${envFilter===e?'#6c72ff':'#1a2d4a'}`,borderRadius:20,background:envFilter===e?'rgba(108,114,255,.15)':'transparent',color:envFilter===e?'#8b90ff':'#546a88',fontSize:11,cursor:'pointer'}}>{e}</button>
          ))}
          <div style={{marginLeft:'auto',display:'flex',gap:10}}>
            {[{l:'Total',v:flags.length,c:'#6c72ff'},{l:'Enabled',v:flags.filter(f=>f.status==='enabled').length,c:'#0fcf8a'},{l:'Experiments',v:flags.filter(f=>f.type==='experiment').length,c:'#8b90ff'}].map(k=>(
              <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'6px 12px'}}>
                <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Flag list */}
        <div style={{width:320,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {filtered.map(f=>{
            const [tb,tc] = typeC[f.type] || typeC.release
            return (
              <div key={f.id} onClick={()=>setSelected(f)}
                style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===f.id?statusC[f.status]:'transparent'}`,background:selected.id===f.id?'rgba(108,114,255,.05)':'transparent'}}>
                <div style={{display:'flex',gap:6,marginBottom:6,alignItems:'center'}}>
                  <span style={{background:tb,color:tc,fontSize:9,fontWeight:800,padding:'2px 6px',borderRadius:8}}>{f.type.toUpperCase()}</span>
                  <span style={{background:statusBg[f.status],color:statusC[f.status],fontSize:9,fontWeight:800,padding:'2px 6px',borderRadius:8}}>{f.status.toUpperCase()}</span>
                  <span style={{marginLeft:'auto',fontSize:10,color:'#546a88'}}>{f.env}</span>
                </div>
                <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{f.name}</div>
                <div style={{fontSize:10,color:'#6c72ff',fontFamily:'JetBrains Mono,monospace',marginBottom:6}}>{f.key}</div>
                {/* Rollout bar */}
                <div style={{display:'flex',alignItems:'center',gap:8}}>
                  <div style={{flex:1,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                    <div style={{height:'100%',width:`${f.rollout}%`,background:statusC[f.status]}}/>
                  </div>
                  <span style={{fontSize:10,color:statusC[f.status],fontWeight:700,width:30,textAlign:'right'}}>{f.rollout}%</span>
                </div>
              </div>
            )
          })}
        </div>

        {/* Detail */}
        <div style={{flex:1,overflowY:'auto',padding:18,display:'flex',flexDirection:'column',gap:12}}>
          <div style={{background:`${statusBg[selected.status]}`,border:`1px solid ${statusC[selected.status]}33`,borderRadius:14,padding:16}}>
            <div style={{display:'flex',alignItems:'flex-start',gap:12,marginBottom:12}}>
              <div style={{flex:1}}>
                <div style={{fontSize:17,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:3}}>{selected.name}</div>
                <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:12,color:'#6c72ff',marginBottom:8}}>{selected.key}</div>
                <div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
                  {selected.tags.map(t=><span key={t} style={{background:'#182844',color:'#8fa8cc',fontSize:9,padding:'2px 8px',borderRadius:12}}>{t}</span>)}
                </div>
              </div>
              <button onClick={()=>toggle(selected.id)} style={{display:'flex',alignItems:'center',gap:6,padding:'8px 16px',border:`1px solid ${statusC[selected.status]}44`,borderRadius:8,background:`${statusC[selected.status]}15`,color:statusC[selected.status],fontSize:12,fontWeight:700,cursor:'pointer'}}>
                {selected.status==='enabled'?<><ToggleRight size={16}/>Enabled</>:<><ToggleLeft size={16}/>Disabled</>}
              </button>
            </div>
            {/* Rollout slider */}
            <div style={{marginBottom:8}}>
              <div style={{display:'flex',justifyContent:'space-between',fontSize:11,marginBottom:6}}>
                <span style={{color:'#546a88'}}>Rollout Percentage</span>
                <span style={{color:statusC[selected.status],fontWeight:800,fontSize:14}}>{selected.rollout}%</span>
              </div>
              <input type="range" min={0} max={100} value={selected.rollout}
                onChange={e=>{setRollout(selected.id,+e.target.value);setSelected(s=>({...s,rollout:+e.target.value}))}}
                style={{width:'100%',accentColor:'#6c72ff'}}/>
              <div style={{display:'flex',justifyContent:'space-between',fontSize:9,color:'#3d5070',marginTop:2}}>
                <span>0% (disabled)</span><span>25%</span><span>50%</span><span>75%</span><span>100% (all users)</span>
              </div>
            </div>
          </div>

          {/* APM Impact */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
            <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12,display:'flex',gap:8,alignItems:'center'}}>
              <TrendingUp size={13} style={{color:'#6c72ff'}}/>APM Impact Correlation
              <span style={{fontSize:10,color:'#546a88',marginLeft:'auto'}}>Since flag enabled · Service: {selected.service}</span>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
              {[
                {l:'Affected Users',   v:selected.impact.users.toLocaleString(),    c:'#6c72ff'},
                {l:'Conversion Δ',    v:selected.impact.conversionDelta,            c:selected.impact.conversionDelta.startsWith('+')?'#0fcf8a':'#f5a623'},
                {l:'Error Rate Δ',    v:selected.impact.errorDelta,                 c:selected.impact.errorDelta.startsWith('+')?'#ff4d6a':selected.impact.errorDelta.startsWith('-')?'#0fcf8a':'#546a88'},
                {l:'Latency Δ',       v:selected.impact.latencyDelta,               c:selected.impact.latencyDelta.startsWith('+')?'#f5a623':selected.impact.latencyDelta.startsWith('-')?'#0fcf8a':'#546a88'},
              ].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'10px 12px'}}>
                  <div style={{fontSize:9,color:'#546a88',marginBottom:3}}>{k.l}</div>
                  <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
            {selected.impact.errorDelta.startsWith('+') && parseFloat(selected.impact.errorDelta)>2 &&(
              <div style={{marginTop:10,padding:'8px 12px',background:'rgba(255,77,106,.07)',border:'1px solid rgba(255,77,106,.2)',borderRadius:8,display:'flex',gap:8,alignItems:'center'}}>
                <AlertTriangle size={13} style={{color:'#ff4d6a'}}/>
                <span style={{fontSize:11,color:'#ff4d6a',fontWeight:700}}>Warning: Error rate increased significantly. Consider pausing rollout or investigating.</span>
              </div>
            )}
          </div>

          {/* SDK snippet */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'10px 16px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef',display:'flex',gap:8,alignItems:'center'}}>
              <Code size={13} style={{color:'#6c72ff'}}/>SDK Integration
            </div>
            <pre style={{padding:14,margin:0,fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc',lineHeight:1.7,overflowX:'auto'}}>{`// Node.js / TypeScript
import { flags } from '@observex/sdk'

if (await flags.isEnabled('${selected.key}', { userId: user.id })) {
  // New feature code
} else {
  // Fallback code
}

// React hook
const { enabled, variant } = useFlag('${selected.key}')
`}</pre>
          </div>
        </div>
      </div>
    </div>
  )
}
