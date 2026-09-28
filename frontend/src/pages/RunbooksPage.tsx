// RunbooksPage.tsx — Create, edit, execute runbooks. Linked from Problems + AI Agent.
import { useEffect, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { runbooks } from '@/lib/api'
import { Plus, Play, Edit3, Trash2, CheckCircle, Clock, AlertTriangle, ChevronDown, ChevronRight, Save } from 'lucide-react'
import toast from 'react-hot-toast'

const DEMO_RUNBOOKS = [
  { id:'rb-001', name:'DB Connection Pool Recovery', category:'database', severity:'CRITICAL', execTime:'3-5m', lastRun:'19:45:00', runs:47, successRate:94, autoTrigger:true, description:'Restores PostgreSQL connection pool when exhausted.',
    steps:[
      { id:'s1', title:'Identify blocked queries', type:'check', command:'SELECT pid, query, state FROM pg_stat_activity WHERE wait_event_type IS NOT NULL;', expected:'List of blocking PIDs', done:false },
      { id:'s2', title:'Kill long-running queries', type:'action', command:'SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state != \'active\' AND query_start < NOW() - INTERVAL \'5 min\';', expected:'Queries terminated', done:false },
      { id:'s3', title:'Verify pool recovery', type:'check', command:'SELECT count(*) FROM pg_stat_activity;', expected:'Count < 150', done:false },
      { id:'s4', title:'Notify team', type:'notify', command:'slack #oncall-db "DB pool recovered. Monitoring for recurrence."', expected:'Notification sent', done:false },
    ]
  },
  { id:'rb-002', name:'K8s Pod CrashLoop Recovery', category:'kubernetes', severity:'HIGH', execTime:'2-4m', lastRun:'16:12:00', runs:23, successRate:87, autoTrigger:true, description:'Diagnoses and recovers pods in CrashLoopBackOff state.',
    steps:[
      { id:'s1', title:'Get crash logs', type:'check', command:'kubectl logs <pod-name> --previous -n production', expected:'Error stack trace', done:false },
      { id:'s2', title:'Check resource limits', type:'check', command:'kubectl describe pod <pod-name> -n production | grep -A5 Limits', expected:'Resource constraints', done:false },
      { id:'s3', title:'Restart pod', type:'action', command:'kubectl rollout restart deployment/<name> -n production', expected:'Rollout triggered', done:false },
      { id:'s4', title:'Monitor rollout', type:'check', command:'kubectl rollout status deployment/<name> -n production', expected:'Successfully rolled out', done:false },
    ]
  },
  { id:'rb-003', name:'High Error Rate Response', category:'apm', severity:'HIGH', execTime:'5-10m', lastRun:'18:42:00', runs:31, successRate:90, autoTrigger:false, description:'Investigates and responds to service error rate spikes.',
    steps:[
      { id:'s1', title:'Check error logs', type:'check', command:'kubectl logs -l app=<service> --tail=100 -n production | grep ERROR', expected:'Error pattern identified', done:false },
      { id:'s2', title:'Check recent deployments', type:'check', command:'kubectl rollout history deployment/<service> -n production', expected:'Deployment timeline', done:false },
      { id:'s3', title:'Assess rollback viability', type:'decision', command:'If error rate > 5% and deployment within 30min: rollback', expected:'Decision made', done:false },
      { id:'s4', title:'Execute rollback if needed', type:'action', command:'kubectl rollout undo deployment/<service> -n production', expected:'Previous version restored', done:false },
      { id:'s5', title:'Verify error rate normalized', type:'check', command:'Check ObserveX: error rate < 1%', expected:'Error rate < 1%', done:false },
    ]
  },
  { id:'rb-004', name:'SSL Certificate Emergency Renewal', category:'security', severity:'CRITICAL', execTime:'10-15m', lastRun:'Never', runs:2, successRate:100, autoTrigger:false, description:'Emergency TLS certificate renewal when auto-renewal fails.',
    steps:[
      { id:'s1', title:'Check cert-manager status', type:'check', command:'kubectl get certificaterequests -n production', expected:'Pending or Failed certificate', done:false },
      { id:'s2', title:'Force cert renewal', type:'action', command:'kubectl delete secret <tls-secret> -n production', expected:'Secret deleted (cert-manager will recreate)', done:false },
      { id:'s3', title:'Monitor cert-manager', type:'check', command:'kubectl logs -l app=cert-manager -n cert-manager --tail=50', expected:'Certificate issued', done:false },
      { id:'s4', title:'Verify certificate', type:'check', command:'openssl s_client -connect <hostname>:443 < /dev/null 2>&1 | grep "Verify return code"', expected:'Verify return code: 0 (ok)', done:false },
    ]
  },
]

const catC: Record<string,[string,string]> = {
  database:   ['rgba(168,85,247,.15)','#c084fc'],
  kubernetes: ['rgba(61,155,255,.15)','#3d9bff'],
  apm:        ['rgba(108,114,255,.15)','#8b90ff'],
  security:   ['rgba(255,77,106,.15)','#ff4d6a'],
}
const stepType: Record<string,string> = { check:'🔍', action:'⚡', notify:'📢', decision:'🤔' }

export default function RunbooksPage() {
  const [selected, setSelected] = useState(DEMO_RUNBOOKS[0])
  const [executing, setExecuting] = useState(false)
  const [runSteps, setRunSteps] = useState<string[]>([])
  const [editing, setEditing] = useState(false)
  const [search, setSearch] = useState('')
  const [runbooks_, setRunbooks] = useState(DEMO_RUNBOOKS)

  const { data } = useQuery({
    queryKey: ['runbooks'],
    queryFn: async () => { try { return await runbooks.list() } catch { return { runbooks: DEMO_RUNBOOKS } } },
    staleTime: 30_000,
  })

  useEffect(() => {
    const next = (data as any)?.runbooks
    if (Array.isArray(next) && next.length > 0) {
      setRunbooks(next)
      setSelected(current => next.find((r: any) => r.id === current.id) ?? next[0])
    }
  }, [data])

  const filtered = runbooks_.filter(r =>
    !search || r.name.toLowerCase().includes(search.toLowerCase()) || r.category.includes(search.toLowerCase())
  )

  const executeRunbook = async () => {
    setExecuting(true)
    setRunSteps([])
    for (let i = 0; i < selected.steps.length; i++) {
      await new Promise(r => setTimeout(r, 800 + Math.random() * 400))
      setRunSteps(s => [...s, selected.steps[i].id])
    }
    setExecuting(false)
    toast.success(`Runbook "${selected.name}" completed successfully!`)
    setRunbooks(r => r.map(x => x.id===selected.id ? {...x, runs:x.runs+1, lastRun: new Date().toLocaleTimeString()} : x))
  }

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>📋</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>Runbooks</div>
            <div style={{fontSize:12,color:'#546a88'}}>Step-by-step incident response · AI-triggered execution · Linked from Problems &amp; AI Agent</div>
          </div>
          <button style={{display:'flex',alignItems:'center',gap:6,background:'#6c72ff',color:'#fff',border:'none',padding:'8px 16px',borderRadius:8,fontSize:12,fontWeight:700,cursor:'pointer'}}>
            <Plus size={13}/>New Runbook
          </button>
        </div>
        <div style={{display:'flex',gap:10,alignItems:'center'}}>
          <input value={search} onChange={e=>setSearch(e.target.value)} placeholder="Search runbooks..."
            style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,padding:'6px 12px',fontSize:12,color:'#c8d8ef',outline:'none',width:280}}/>
          <div style={{display:'flex',gap:10,marginLeft:'auto'}}>
            {[{l:'Total',v:runbooks_.length,c:'#6c72ff'},{l:'Auto-trigger',v:runbooks_.filter(r=>r.autoTrigger).length,c:'#0fcf8a'},{l:'Avg Success',v:Math.round(runbooks_.reduce((a,r)=>a+r.successRate,0)/runbooks_.length)+'%',c:'#f5a623'}].map(k=>(
              <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'6px 12px'}}>
                <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
                <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* List */}
        <div style={{width:280,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {filtered.map(r=>{
            const [cbg,cc] = catC[r.category] || ['rgba(108,114,255,.15)','#8b90ff']
            return (
              <div key={r.id} onClick={()=>{setSelected(r);setRunSteps([]);setEditing(false)}}
                style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===r.id?cc:'transparent'}`,background:selected.id===r.id?'rgba(108,114,255,.05)':'transparent'}}>
                <div style={{display:'flex',gap:6,marginBottom:6,alignItems:'center'}}>
                  <span style={{background:cbg,color:cc,fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:8}}>{r.category.toUpperCase()}</span>
                  {r.autoTrigger&&<span style={{fontSize:9,background:'rgba(15,207,138,.15)',color:'#0fcf8a',padding:'1px 6px',borderRadius:8,fontWeight:700}}>AUTO</span>}
                </div>
                <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff',marginBottom:5,lineHeight:1.3}}>{r.name}</div>
                <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88'}}>
                  <span>⌚ {r.execTime}</span>
                  <span>✓ {r.successRate}%</span>
                  <span>{r.runs} runs</span>
                </div>
              </div>
            )
          })}
        </div>

        {/* Detail */}
        <div style={{flex:1,overflowY:'auto',padding:18,display:'flex',flexDirection:'column',gap:12}}>
          {/* Header */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:14,padding:16}}>
            <div style={{display:'flex',alignItems:'flex-start',gap:12,marginBottom:10}}>
              <div style={{flex:1}}>
                <div style={{fontSize:17,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                <div style={{fontSize:12,color:'#8fa8cc',marginBottom:8}}>{selected.description}</div>
                <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
                  {[{l:'Category',v:selected.category},{l:'Est. Time',v:selected.execTime},{l:'Total Runs',v:selected.runs},{l:'Success Rate',v:`${selected.successRate}%`},{l:'Last Run',v:selected.lastRun}].map(k=>(
                    <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:8,padding:'5px 10px'}}>
                      <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                      <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef'}}>{k.v}</div>
                    </div>
                  ))}
                </div>
              </div>
              <div style={{display:'flex',gap:8}}>
                <button onClick={executeRunbook} disabled={executing}
                  style={{display:'flex',gap:6,alignItems:'center',background:executing?'rgba(108,114,255,.3)':'#6c72ff',color:'#fff',border:'none',padding:'8px 16px',borderRadius:8,fontSize:12,fontWeight:700,cursor:executing?'wait':'pointer'}}>
                  <Play size={13}/>{executing?'Running...':'Execute'}
                </button>
              </div>
            </div>
          </div>

          {/* Steps */}
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <div style={{padding:'12px 16px',borderBottom:'1px solid #1a2d4a',fontSize:12,fontWeight:700,color:'#c8d8ef'}}>Steps ({selected.steps.length})</div>
            <div style={{padding:14,display:'flex',flexDirection:'column',gap:8}}>
              {selected.steps.map((step,i)=>{
                const done = runSteps.includes(step.id)
                const running = executing && runSteps.length===i
                return (
                  <div key={step.id} style={{display:'flex',gap:12,alignItems:'flex-start'}}>
                    {/* Step number */}
                    <div style={{display:'flex',flexDirection:'column',alignItems:'center',flexShrink:0}}>
                      <div style={{width:28,height:28,borderRadius:'50%',background:done?'rgba(15,207,138,.2)':running?'rgba(108,114,255,.2)':'#0b1628',border:`2px solid ${done?'#0fcf8a':running?'#6c72ff':'#1a2d4a'}`,display:'flex',alignItems:'center',justifyContent:'center',fontSize:12,fontWeight:800,color:done?'#0fcf8a':running?'#8b90ff':'#546a88'}}>
                        {done?'✓':running?'⟳':(i+1)}
                      </div>
                      {i<selected.steps.length-1&&<div style={{width:2,height:24,background:done?'#0fcf8a':'#1a2d4a',margin:'3px 0'}}/>}
                    </div>
                    {/* Content */}
                    <div style={{flex:1,padding:'8px 12px',background:done?'rgba(15,207,138,.05)':running?'rgba(108,114,255,.06)':'rgba(7,15,30,.5)',border:`1px solid ${done?'rgba(15,207,138,.25)':running?'rgba(108,114,255,.25)':'#1a2d4a'}`,borderRadius:10}}>
                      <div style={{display:'flex',gap:6,alignItems:'center',marginBottom:5}}>
                        <span style={{fontSize:13}}>{stepType[step.type]||'•'}</span>
                        <span style={{fontSize:12,fontWeight:700,color:done?'#0fcf8a':running?'#8b90ff':'#f0f6ff'}}>{step.title}</span>
                        <span style={{marginLeft:'auto',fontSize:9,background:`${done?'rgba(15,207,138,.15)':running?'rgba(108,114,255,.15)':'rgba(84,106,136,.1)'}`,color:done?'#0fcf8a':running?'#8b90ff':'#546a88',padding:'1px 6px',borderRadius:8,fontWeight:700,textTransform:'uppercase'}}>{step.type}</span>
                      </div>
                      <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#6c72ff',background:'rgba(7,15,30,.7)',padding:'5px 8px',borderRadius:6,marginBottom:4}}>{step.command}</div>
                      <div style={{fontSize:10,color:'#546a88'}}>Expected: {step.expected}</div>
                    </div>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
