// AnomalyDetectionPage.tsx — Configure baseline detection, static thresholds, adaptive learning
import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { anomalyDetection } from '@/lib/api'
import { Plus, Brain, Zap, Shield, Activity, Edit, Trash2, ToggleLeft, ToggleRight, Info } from 'lucide-react'
import toast from 'react-hot-toast'

const DEMO_RULES = [
  { id:'ad-001', name:'Response Time Spike', type:'BASELINE', metric:'http.response_time.p99', services:['api-gateway','checkout-service'], sensitivity:2, enabled:true, triggered:12, lastFired:'19:24:11', algorithm:'AUTO_ADAPTIVE', window:'5m' },
  { id:'ad-002', name:'Error Rate Anomaly', type:'BASELINE', metric:'http.error_rate', services:['all'], sensitivity:1, enabled:true, triggered:8, lastFired:'18:44:00', algorithm:'AUTO_ADAPTIVE', window:'3m' },
  { id:'ad-003', name:'CPU Saturation', type:'STATIC', metric:'system.cpu.usage', services:['all'], sensitivity:0, enabled:true, triggered:4, lastFired:'19:45:11', algorithm:'THRESHOLD', window:'2m', threshold:85 },
  { id:'ad-004', name:'Memory Leak Detector', type:'TREND', metric:'process.memory.rss', services:['all'], sensitivity:2, enabled:true, triggered:2, lastFired:'14:20:00', algorithm:'LINEAR_REGRESSION', window:'30m' },
  { id:'ad-005', name:'DB Connection Pool', type:'STATIC', metric:'db.connection_pool.used_pct', services:['postgres-primary'], sensitivity:0, enabled:true, triggered:3, lastFired:'19:24:11', algorithm:'THRESHOLD', window:'1m', threshold:90 },
  { id:'ad-006', name:'Throughput Drop', type:'BASELINE', metric:'http.requests_per_second', services:['all'], sensitivity:3, enabled:false, triggered:0, lastFired:'Never', algorithm:'AUTO_ADAPTIVE', window:'10m' },
]

const ALGORITHMS = [
  { id:'AUTO_ADAPTIVE', label:'Auto Adaptive', icon:'🧠', desc:'Learns normal patterns automatically using ML. Adjusts thresholds daily.' },
  { id:'THRESHOLD', label:'Static Threshold', icon:'📏', desc:'Fixed threshold. Fires immediately when value crosses the line.' },
  { id:'LINEAR_REGRESSION', label:'Trend Detection', icon:'📈', desc:'Detects abnormal growth rates using linear regression over a time window.' },
  { id:'PERCENTILE', label:'Percentile Baseline', icon:'📊', desc:'Alert when metric exceeds Nth percentile of historical baseline.' },
]

const SENSITIVITY_LABELS = ['Low (fewer alerts)', 'Medium', 'High (more alerts)']

export default function AnomalyDetectionPage() {
  const qc = useQueryClient()
  const [rules, setRules] = useState(DEMO_RULES)
  const [selected, setSelected] = useState<typeof DEMO_RULES[0] | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [form, setForm] = useState({ name:'', metric:'', algorithm:'AUTO_ADAPTIVE', sensitivity:1, window:'5m', threshold:80 })

  const { data } = useQuery({
    queryKey: ['anomaly-rules'],
    queryFn: async () => { try { return await anomalyDetection.rules() } catch { return { rules: DEMO_RULES } } },
    staleTime: 30_000,
  })

  useEffect(() => {
    const next = (data as any)?.rules
    if (Array.isArray(next) && next.length > 0) setRules(next)
  }, [data])

  const toggleRule = (id: string) => {
    setRules(r => r.map(x => x.id===id ? {...x,enabled:!x.enabled} : x))
    toast.success('Rule updated')
  }

  const deleteRule = (id: string) => {
    setRules(r => r.filter(x => x.id!==id))
    toast('Rule deleted')
  }

  const createRule = () => {
    const newRule = { ...form, id:`ad-${Date.now()}`, type:'BASELINE', services:['all'], triggered:0, lastFired:'Never' }
    setRules(r=>[...r, newRule as any])
    setShowCreate(false)
    toast.success('Anomaly detection rule created')
  }

  const typeBg: Record<string,[string,string]> = {
    BASELINE:  ['rgba(108,114,255,.15)','#8b90ff'],
    STATIC:    ['rgba(245,166,35,.15)','#f5a623'],
    TREND:     ['rgba(15,207,138,.15)','#0fcf8a'],
    PERCENTILE:['rgba(168,85,247,.15)','#c084fc'],
  }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🔮</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Anomaly Detection</div>
            <div style={{fontSize:12,color:'#546a88'}}>Configure baseline detection · Static thresholds · Trend analysis · Adaptive ML learning</div>
          </div>
          <button onClick={()=>setShowCreate(true)} style={{display:'flex',alignItems:'center',gap:6,background:'#6c72ff',color:'#fff',border:'none',padding:'8px 16px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}>
            <Plus size={13}/>New Rule
          </button>
        </div>
        <div style={{display:'flex',gap:10}}>
          {[{l:'Total Rules',v:rules.length,c:'#6c72ff'},{l:'Active',v:rules.filter(r=>r.enabled).length,c:'#0fcf8a'},{l:'Triggered Today',v:rules.reduce((a,r)=>a+r.triggered,0),c:'#f5a623'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Rules list */}
        <div style={{flex:1,overflowY:'auto',padding:16}}>
          {/* Algorithm guide */}
          <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:8,marginBottom:16}}>
            {ALGORITHMS.map(a=>(
              <div key={a.id} style={{background:'rgba(17,31,53,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'10px 12px'}}>
                <div style={{fontSize:16,marginBottom:5}}>{a.icon}</div>
                <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginBottom:4}}>{a.label}</div>
                <div style={{fontSize:10,color:'#546a88',lineHeight:1.4}}>{a.desc}</div>
              </div>
            ))}
          </div>

          {/* Rules table */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                {['Rule Name','Type','Metric','Algorithm','Window','Triggered','Last Fired','Enabled',''].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
              </tr></thead>
              <tbody>
                {rules.map(r=>{
                  const [tbg,tc] = typeBg[r.type as string] || ['rgba(108,114,255,.15)','#8b90ff']
                  return (
                    <tr key={r.id} onClick={()=>setSelected(r)} style={{borderBottom:'1px solid rgba(26,45,74,.4)',cursor:'pointer',background:selected?.id===r.id?'rgba(108,114,255,.05)':'transparent'}}>
                      <td style={{padding:'10px 12px',fontWeight:700,color:'#f0f6ff',fontSize:12}}>{r.name}</td>
                      <td style={{padding:'10px 12px'}}><span style={{background:tbg,color:tc,fontSize:9,padding:'2px 7px',borderRadius:8,fontWeight:800}}>{r.type}</span></td>
                      <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#8fa8cc'}}>{r.metric}</td>
                      <td style={{padding:'10px 12px',fontSize:11,color:'#546a88'}}>{(r as any).algorithm?.replace(/_/g,' ')}</td>
                      <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{r.window}</td>
                      <td style={{padding:'10px 12px',color:r.triggered>5?'#f5a623':'#8fa8cc',fontWeight:r.triggered>5?700:400,fontSize:12}}>{r.triggered}</td>
                      <td style={{padding:'10px 12px',fontSize:11,color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{r.lastFired}</td>
                      <td style={{padding:'10px 12px'}}>
                        <button onClick={e=>{e.stopPropagation();toggleRule(r.id)}} style={{background:'none',border:'none',cursor:'pointer',display:'flex',alignItems:'center',gap:5}}>
                          {r.enabled
                            ? <><ToggleRight size={20} style={{color:'#0fcf8a'}}/><span style={{fontSize:10,color:'#0fcf8a'}}>ON</span></>
                            : <><ToggleLeft size={20} style={{color:'#546a88'}}/><span style={{fontSize:10,color:'#546a88'}}>OFF</span></>}
                        </button>
                      </td>
                      <td style={{padding:'10px 12px'}}>
                        <button onClick={e=>{e.stopPropagation();deleteRule(r.id)}} style={{background:'none',border:'none',cursor:'pointer',padding:'3px'}}>
                          <Trash2 size={13} style={{color:'#3d5070'}}/>
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>

        {/* Detail panel */}
        {selected&&(
          <div style={{width:300,borderLeft:'1px solid #1a2d4a',overflowY:'auto',padding:16,flexShrink:0}}>
            <div style={{fontSize:13,fontWeight:800,color:'#f0f6ff',marginBottom:12,fontFamily:'Syne,sans-serif'}}>{selected.name}</div>
            {[
              {l:'Type', v:selected.type},
              {l:'Metric', v:selected.metric},
              {l:'Algorithm', v:(selected as any).algorithm?.replace(/_/g,' ')},
              {l:'Window', v:selected.window},
              {l:'Services', v:(selected.services||[]).join(', ')},
              {l:'Sensitivity', v:SENSITIVITY_LABELS[selected.sensitivity] || 'Medium'},
              {l:'Times Triggered', v:selected.triggered.toString()},
              {l:'Last Fired', v:selected.lastFired},
            ].map(k=>(
              <div key={k.l} style={{display:'flex',flexDirection:'column',gap:2,marginBottom:10,paddingBottom:10,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase',letterSpacing:'.06em'}}>{k.l}</div>
                <div style={{fontSize:12,color:'#c8d8ef',fontFamily:'JetBrains Mono,monospace',wordBreak:'break-all'}}>{k.v}</div>
              </div>
            ))}
            {(selected as any).threshold&&(
              <div style={{background:'rgba(245,166,35,.08)',border:'1px solid rgba(245,166,35,.2)',borderRadius:8,padding:'8px 12px',marginTop:6}}>
                <div style={{fontSize:10,color:'#f5a623',fontWeight:700}}>Static Threshold: {(selected as any).threshold}%</div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Create modal */}
      {showCreate&&(
        <div style={{position:'fixed',inset:0,background:'rgba(0,0,0,.7)',display:'flex',alignItems:'center',justifyContent:'center',zIndex:100}} onClick={()=>setShowCreate(false)}>
          <div style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:16,padding:24,width:480}} onClick={e=>e.stopPropagation()}>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:16}}>New Anomaly Detection Rule</div>
            {[{l:'Rule Name',k:'name',ph:'e.g., API Latency Spike'},{l:'Metric',k:'metric',ph:'e.g., http.response_time.p99'}].map(f=>(
              <div key={f.k} style={{marginBottom:12}}>
                <div style={{fontSize:11,color:'#546a88',marginBottom:5}}>{f.l}</div>
                <input value={(form as any)[f.k]} onChange={e=>setForm(p=>({...p,[f.k]:e.target.value}))} placeholder={f.ph}
                  style={{width:'100%',background:'#080d1b',border:'1px solid #1a2d4a',borderRadius:8,padding:'8px 12px',fontSize:12,color:'#c8d8ef',outline:'none',boxSizing:'border-box'}}/>
              </div>
            ))}
            <div style={{marginBottom:12}}>
              <div style={{fontSize:11,color:'#546a88',marginBottom:5}}>Detection Algorithm</div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:6}}>
                {ALGORITHMS.map(a=>(
                  <button key={a.id} onClick={()=>setForm(p=>({...p,algorithm:a.id}))}
                    style={{padding:'8px 10px',border:`1px solid ${form.algorithm===a.id?'#6c72ff':'#1a2d4a'}`,borderRadius:8,background:form.algorithm===a.id?'rgba(108,114,255,.15)':'#080d1b',color:form.algorithm===a.id?'#8b90ff':'#546a88',fontSize:11,cursor:'pointer',textAlign:'left'}}>
                    {a.icon} {a.label}
                  </button>
                ))}
              </div>
            </div>
            <div style={{marginBottom:16}}>
              <div style={{fontSize:11,color:'#546a88',marginBottom:5}}>Sensitivity: <span style={{color:'#c8d8ef'}}>{SENSITIVITY_LABELS[form.sensitivity]}</span></div>
              <input type="range" min={0} max={2} value={form.sensitivity} onChange={e=>setForm(p=>({...p,sensitivity:+e.target.value}))}
                style={{width:'100%',accentColor:'#6c72ff'}}/>
            </div>
            <div style={{display:'flex',gap:8}}>
              <button onClick={createRule} style={{flex:1,background:'#6c72ff',color:'#fff',border:'none',padding:'10px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}>Create Rule</button>
              <button onClick={()=>setShowCreate(false)} style={{padding:'10px 16px',background:'#0b1628',border:'1px solid #1a2d4a',color:'#546a88',borderRadius:8,fontSize:12,cursor:'pointer'}}>Cancel</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
