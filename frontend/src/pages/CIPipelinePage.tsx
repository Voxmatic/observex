// CIPipelinePage.tsx — CI/CD Pipeline Intelligence
// Inspired by Datadog CI Visibility + Grafana's approach
// Smart ObserveX take: correlates pipeline failures with runtime APM data
import { useState } from 'react'
import { Play, CheckCircle, XCircle, Clock, AlertTriangle, GitBranch, GitCommit, Zap, TrendingUp, RefreshCw, ExternalLink } from 'lucide-react'

const PIPELINES = [
  { id:'p1', name:'observex-backend', branch:'main', trigger:'push', status:'failed', duration:'8m 24s', stages:6, failedStage:'unit-tests', author:'jin.park', commit:'a1b2c3d', commitMsg:'fix: resolve DB connection timeout', startTime:'19:47:00', coverage:84, passedJobs:14, totalJobs:16, flaky:2, trend:'stable' },
  { id:'p2', name:'checkout-frontend', branch:'main', trigger:'push', status:'success', duration:'3m 12s', stages:4, failedStage:null, author:'sara.chen', commit:'e5f6g7h', commitMsg:'feat: one-click checkout v2', startTime:'19:30:00', coverage:91, passedJobs:12, totalJobs:12, flaky:0, trend:'improving' },
  { id:'p3', name:'ml-inference-svc', branch:'perf/quantization', trigger:'pr', status:'success', duration:'12m 48s', stages:7, failedStage:null, author:'ai-team', commit:'i9j0k1l', commitMsg:'perf: INT8 quantization', startTime:'18:55:00', coverage:76, passedJobs:18, totalJobs:18, flaky:1, trend:'stable' },
  { id:'p4', name:'payment-service', branch:'patch/retry', trigger:'push', status:'running', duration:'2m 11s', stages:5, failedStage:null, author:'david.wu', commit:'m2n3o4p', commitMsg:'fix: improved retry logic', startTime:'19:58:00', coverage:88, passedJobs:6, totalJobs:14, flaky:0, trend:'stable' },
  { id:'p5', name:'api-gateway', branch:'fix/auth-middleware', trigger:'push', status:'failed', duration:'5m 33s', stages:5, failedStage:'integration-tests', author:'jin.park', commit:'q7r8s9t', commitMsg:'fix: auth middleware refactor', startTime:'18:42:00', coverage:72, passedJobs:9, totalJobs:12, flaky:3, trend:'degrading' },
  { id:'p6', name:'notification-svc', branch:'main', trigger:'schedule', status:'success', duration:'1m 48s', stages:3, failedStage:null, author:'ops-team', commit:'u0v1w2x', commitMsg:'chore: dependency update', startTime:'18:00:00', coverage:93, passedJobs:8, totalJobs:8, flaky:0, trend:'improving' },
]

const STAGES_DETAIL: Record<string, any[]> = {
  'p1': [
    { name:'checkout', duration:'12s', status:'success', jobs:2 },
    { name:'install', duration:'48s', status:'success', jobs:1 },
    { name:'lint', duration:'34s', status:'success', jobs:3 },
    { name:'unit-tests', duration:'4m 12s', status:'failed', jobs:8, error:'TestUserService.TestGetProfile: NullPointerException at line 142' },
    { name:'build', duration:null, status:'skipped', jobs:1 },
    { name:'deploy-staging', duration:null, status:'skipped', jobs:1 },
  ],
  'p2': [
    { name:'checkout', duration:'8s', status:'success', jobs:1 },
    { name:'install', duration:'32s', status:'success', jobs:1 },
    { name:'test', duration:'2m 18s', status:'success', jobs:8 },
    { name:'build-deploy', duration:'34s', status:'success', jobs:2 },
  ],
}

const FLAKY_TESTS = [
  { name:'TestCheckoutFlow.TestConcurrentOrders', file:'checkout/tests/flow_test.go', flakeRate:28, runs:54, lastFail:'2h ago', service:'checkout-service' },
  { name:'TestMLInference.TestBatchPrediction', file:'ml/tests/inference_test.py', flakeRate:12, runs:81, lastFail:'6h ago', service:'ml-inference' },
  { name:'TestAPIGateway.TestAuthRetry', file:'gateway/tests/auth_test.go', flakeRate:42, runs:38, lastFail:'18m ago', service:'api-gateway' },
  { name:'TestNotification.TestSlackWebhook', file:'notify/tests/slack_test.go', flakeRate:8, runs:124, lastFail:'1d ago', service:'notification-svc' },
]

const stC: Record<string,string> = { success:'#0fcf8a', failed:'#ff4d6a', running:'#6c72ff', skipped:'#3d5070', cancelled:'#546a88' }
const stBg: Record<string,string> = { success:'rgba(15,207,138,.12)', failed:'rgba(255,77,106,.12)', running:'rgba(108,114,255,.12)', skipped:'rgba(61,80,112,.12)' }
const trendC: Record<string,string> = { improving:'#0fcf8a', stable:'#8fa8cc', degrading:'#ff4d6a' }

export default function CIPipelinePage() {
  const [selected, setSelected] = useState(PIPELINES[0])
  const [tab, setTab] = useState<'pipelines'|'flaky'|'coverage'>('pipelines')

  const failed = PIPELINES.filter(p=>p.status==='failed').length
  const running = PIPELINES.filter(p=>p.status==='running').length
  const avgDuration = '5m 40s'
  const successRate = Math.round((PIPELINES.filter(p=>p.status==='success').length/PIPELINES.filter(p=>p.status!=='running').length)*100)

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>⚙️</div>
          <div style={{flex:1}}>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>CI/CD Pipeline Intelligence</div>
            <div style={{fontSize:12,color:'#546a88'}}>Pipeline health · Stage-level traces · Flaky test detection · APM correlation · Test coverage trends</div>
          </div>
          <button style={{display:'flex',gap:5,alignItems:'center',background:'#0b1628',border:'1px solid #1a2d4a',color:'#546a88',padding:'6px 12px',borderRadius:8,fontSize:11,cursor:'pointer'}}>
            <RefreshCw size={11}/>Sync Now
          </button>
        </div>
        <div style={{display:'flex',gap:10,marginBottom:10}}>
          {[{l:'Total Pipelines',v:PIPELINES.length,c:'#6c72ff'},{l:'Running',v:running,c:'#6c72ff'},{l:'Failed',v:failed,c:failed>0?'#ff4d6a':'#0fcf8a'},{l:'Success Rate',v:`${successRate}%`,c:successRate>85?'#0fcf8a':'#f5a623'},{l:'Avg Duration',v:avgDuration,c:'#8fa8cc'},{l:'Flaky Tests',v:FLAKY_TESTS.length,c:'#f5a623'}].map(k=>(
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase'}}>{k.l}</div>
              <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
        <div style={{display:'flex',gap:0}}>
          {(['pipelines','flaky','coverage'] as const).map(t=>(
            <button key={t} onClick={()=>setTab(t)} style={{padding:'7px 18px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>{t==='flaky'?'Flaky Tests':t==='coverage'?'Test Coverage':t}</button>
          ))}
        </div>
      </div>

      {tab==='pipelines'&&(
        <div style={{display:'flex',flex:1,overflow:'hidden'}}>
          {/* List */}
          <div style={{width:320,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
            {PIPELINES.map(p=>(
              <div key={p.id} onClick={()=>setSelected(p)}
                style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===p.id?stC[p.status]:'transparent'}`,background:selected.id===p.id?stBg[p.status]:'transparent'}}>
                <div style={{display:'flex',gap:8,marginBottom:6,alignItems:'center'}}>
                  {p.status==='success'?<CheckCircle size={13} style={{color:'#0fcf8a'}}/>:p.status==='failed'?<XCircle size={13} style={{color:'#ff4d6a'}}/>:<Play size={13} style={{color:'#6c72ff',animation:p.status==='running'?'pulse 1s infinite':''}}/>}
                  <span style={{fontSize:12,fontWeight:700,color:'#f0f6ff',flex:1}}>{p.name}</span>
                  <span style={{background:stBg[p.status],color:stC[p.status],fontSize:9,fontWeight:800,padding:'2px 7px',borderRadius:10}}>{p.status.toUpperCase()}</span>
                </div>
                <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginBottom:5}}>
                  <span style={{display:'flex',gap:3,alignItems:'center'}}><GitBranch size={9}/>{p.branch}</span>
                  <span style={{display:'flex',gap:3,alignItems:'center'}}><Clock size={9}/>{p.duration}</span>
                  <span style={{color:trendC[p.trend],fontWeight:600}}>{p.trend==='improving'?'↑':p.trend==='degrading'?'↓':'→'} {p.trend}</span>
                </div>
                <div style={{fontSize:10,color:'#546a88',fontFamily:'JetBrains Mono,monospace',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{p.commitMsg}</div>
                {p.flaky>0&&<div style={{marginTop:4,fontSize:9,color:'#f5a623',background:'rgba(245,166,35,.1)',padding:'2px 7px',borderRadius:8,display:'inline-block',fontWeight:700}}>⚠ {p.flaky} flaky test{p.flaky>1?'s':''}</div>}
              </div>
            ))}
          </div>

          {/* Detail */}
          <div style={{flex:1,overflowY:'auto',padding:16}}>
            <div style={{background:stBg[selected.status],border:`1px solid ${stC[selected.status]}33`,borderRadius:14,padding:16,marginBottom:14}}>
              <div style={{display:'flex',alignItems:'flex-start',gap:12,marginBottom:12}}>
                <div style={{flex:1}}>
                  <div style={{fontSize:17,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif',marginBottom:4}}>{selected.name}</div>
                  <div style={{display:'flex',gap:8,flexWrap:'wrap',fontSize:11}}>
                    <span style={{color:'#6c72ff',fontFamily:'JetBrains Mono,monospace'}}>{selected.branch}</span>
                    <span style={{color:'#546a88'}}>triggered by {selected.trigger}</span>
                    <span style={{color:'#546a88'}}>by {selected.author}</span>
                    <span style={{color:'#546a88',fontFamily:'JetBrains Mono,monospace'}}>{selected.commit}</span>
                  </div>
                </div>
                <div style={{textAlign:'right'}}>
                  <div style={{fontSize:20,fontWeight:900,color:stC[selected.status],fontFamily:'Syne,sans-serif'}}>{selected.status.toUpperCase()}</div>
                  <div style={{fontSize:11,color:'#546a88'}}>{selected.duration}</div>
                </div>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'repeat(4,1fr)',gap:10}}>
                {[{l:'Passed Jobs',v:`${selected.passedJobs}/${selected.totalJobs}`,c:'#0fcf8a'},{l:'Coverage',v:`${selected.coverage}%`,c:selected.coverage>80?'#0fcf8a':'#f5a623'},{l:'Stages',v:selected.stages,c:'#8b90ff'},{l:'Flaky Tests',v:selected.flaky,c:selected.flaky>0?'#f5a623':'#0fcf8a'}].map(k=>(
                  <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 12px'}}>
                    <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:18,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                  </div>
                ))}
              </div>
            </div>

            {/* Commit info */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:12,marginBottom:14,display:'flex',gap:10,alignItems:'center'}}>
              <GitCommit size={14} style={{color:'#6c72ff'}}/>
              <div style={{flex:1}}>
                <div style={{fontSize:12,fontWeight:600,color:'#f0f6ff'}}>{selected.commitMsg}</div>
                <div style={{fontSize:10,color:'#546a88'}}>{selected.author} · {selected.startTime} · <span style={{fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{selected.commit}</span></div>
              </div>
              <button style={{display:'flex',gap:5,alignItems:'center',background:'transparent',border:'1px solid #1a2d4a',color:'#546a88',padding:'5px 10px',borderRadius:7,fontSize:10,cursor:'pointer'}}>
                <ExternalLink size={10}/>GitHub
              </button>
            </div>

            {/* Stage pipeline visual */}
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14,marginBottom:14}}>
              <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Pipeline Stages</div>
              <div style={{display:'flex',gap:4,overflowX:'auto',paddingBottom:8}}>
                {(STAGES_DETAIL[selected.id] || Array(selected.stages).fill(null).map((_,i)=>({name:`stage-${i+1}`,status:'success',duration:'1m',jobs:2,error:null}))).map((s:any,i:number,arr:any[])=>(
                  <div key={i} style={{display:'flex',alignItems:'center',gap:4}}>
                    <div style={{background:stBg[s.status]||'rgba(17,31,53,.7)',border:`1px solid ${stC[s.status]||'#1a2d4a'}`,borderRadius:10,padding:'10px 14px',minWidth:110,textAlign:'center'}}>
                      <div style={{fontSize:14,marginBottom:5}}>{s.status==='success'?'✓':s.status==='failed'?'✗':s.status==='skipped'?'⊘':'⟳'}</div>
                      <div style={{fontSize:10,fontWeight:700,color:stC[s.status]||'#546a88',marginBottom:3}}>{s.name}</div>
                      {s.duration&&<div style={{fontSize:9,color:'#546a88'}}>{s.duration}</div>}
                      {s.status==='skipped'&&<div style={{fontSize:9,color:'#3d5070'}}>skipped</div>}
                      {s.jobs&&<div style={{fontSize:9,color:'#546a88'}}>{s.jobs} jobs</div>}
                    </div>
                    {i<arr.length-1&&<div style={{color:'#3d5070',fontSize:18}}>→</div>}
                  </div>
                ))}
              </div>

              {selected.failedStage&&(
                <div style={{marginTop:12,padding:'10px 14px',background:'rgba(255,77,106,.06)',border:'1px solid rgba(255,77,106,.25)',borderRadius:10}}>
                  <div style={{fontSize:11,fontWeight:700,color:'#ff4d6a',marginBottom:4}}>❌ Failed in stage: {selected.failedStage}</div>
                  {(STAGES_DETAIL[selected.id]||[]).find((s:any)=>s.name===selected.failedStage)?.error&&(
                    <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#ff7a92',background:'rgba(0,0,0,.3)',padding:'8px 10px',borderRadius:8}}>
                      {(STAGES_DETAIL[selected.id]||[]).find((s:any)=>s.name===selected.failedStage)?.error}
                    </div>
                  )}
                  <div style={{marginTop:8,fontSize:11,color:'#546a88'}}>💡 <strong style={{color:'#f5a623'}}>AI suggestion:</strong> This error also appears in APM error tracking. <a href="/errors-inbox" style={{color:'#6c72ff',textDecoration:'none'}}>View in Errors Inbox →</a></div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {tab==='flaky'&&(
        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{marginBottom:12,padding:'10px 14px',background:'rgba(245,166,35,.06)',border:'1px solid rgba(245,166,35,.2)',borderRadius:10,fontSize:11,color:'#f5a623'}}>
            ⚠ {FLAKY_TESTS.length} flaky tests detected across your pipelines. Flaky tests waste CI time and erode confidence in your test suite.
          </div>
          <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
            <table style={{width:'100%',borderCollapse:'collapse'}}>
              <thead><tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                {['Test Name','File','Service','Flake Rate','Total Runs','Last Failed'].map(h=><th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>)}
              </tr></thead>
              <tbody>
                {FLAKY_TESTS.map((t,i)=>(
                  <tr key={i} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                    <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#f0f6ff',maxWidth:280}}>
                      <div style={{overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{t.name}</div>
                    </td>
                    <td style={{padding:'10px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#6c72ff'}}>{t.file}</td>
                    <td style={{padding:'10px 12px'}}><span style={{background:'rgba(108,114,255,.15)',color:'#8b90ff',fontSize:9,padding:'2px 7px',borderRadius:8}}>{t.service}</span></td>
                    <td style={{padding:'10px 12px'}}>
                      <div style={{display:'flex',gap:6,alignItems:'center'}}>
                        <div style={{width:50,height:5,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                          <div style={{height:'100%',width:`${t.flakeRate}%`,background:t.flakeRate>30?'#ff4d6a':t.flakeRate>15?'#f5a623':'#6c72ff'}}/>
                        </div>
                        <span style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:t.flakeRate>30?'#ff4d6a':t.flakeRate>15?'#f5a623':'#f0f6ff',fontWeight:700}}>{t.flakeRate}%</span>
                      </div>
                    </td>
                    <td style={{padding:'10px 12px',fontSize:11,color:'#8fa8cc'}}>{t.runs}</td>
                    <td style={{padding:'10px 12px',fontSize:11,color:'#546a88'}}>{t.lastFail}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {tab==='coverage'&&(
        <div style={{flex:1,overflowY:'auto',padding:16}}>
          <div style={{display:'grid',gridTemplateColumns:'repeat(3,1fr)',gap:12}}>
            {PIPELINES.map(p=>(
              <div key={p.id} style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                <div style={{fontSize:13,fontWeight:700,color:'#f0f6ff',marginBottom:3}}>{p.name}</div>
                <div style={{fontSize:10,color:'#546a88',marginBottom:10}}>{p.branch}</div>
                <div style={{marginBottom:6}}>
                  <div style={{display:'flex',justifyContent:'space-between',fontSize:10,color:'#546a88',marginBottom:4}}>
                    <span>Test Coverage</span>
                    <span style={{color:p.coverage>80?'#0fcf8a':p.coverage>60?'#f5a623':'#ff4d6a',fontWeight:700}}>{p.coverage}%</span>
                  </div>
                  <div style={{height:8,background:'#0b1628',borderRadius:4,overflow:'hidden'}}>
                    <div style={{height:'100%',width:`${p.coverage}%`,background:p.coverage>80?'#0fcf8a':p.coverage>60?'#f5a623':'#ff4d6a',borderRadius:4}}/>
                  </div>
                </div>
                <div style={{display:'flex',gap:8,fontSize:10}}>
                  <span style={{color:'#0fcf8a'}}>✓ {p.passedJobs} passed</span>
                  {p.totalJobs-p.passedJobs>0&&<span style={{color:'#ff4d6a'}}>✗ {p.totalJobs-p.passedJobs} failed</span>}
                  {p.flaky>0&&<span style={{color:'#f5a623'}}>⚠ {p.flaky} flaky</span>}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
