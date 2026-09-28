// LLMMonitoringPage.tsx — ★ UNIQUE feature: monitor Claude, GPT-4, Gemini, Ollama in one platform
import { useState, useEffect } from 'react'
import { Brain, Zap, DollarSign, Clock, AlertTriangle, TrendingUp, CheckCircle, Activity, Target } from 'lucide-react'

const PROVIDERS = [
  { id: 'claude', name: 'Claude Haiku 4.5', provider: 'Anthropic', icon: '🟣', calls: 48420, tokens: 18_200_000, cost: 4.55, avgLatency: 380, errorRate: 0.02, p99: 1240, status: 'healthy', trend: '+12%', model: 'claude-haiku-4-5' },
  { id: 'claude-s', name: 'Claude Sonnet 4.6', provider: 'Anthropic', icon: '🟣', calls: 1820, tokens: 3_640_000, cost: 10.92, avgLatency: 1240, errorRate: 0.01, p99: 3800, status: 'healthy', trend: '+5%', model: 'claude-sonnet-4-6' },
  { id: 'gpt4', name: 'GPT-4o', provider: 'OpenAI', icon: '🟢', calls: 12440, tokens: 6_220_000, cost: 18.66, avgLatency: 890, errorRate: 0.14, p99: 2800, status: 'warning', trend: '+3%', model: 'gpt-4o' },
  { id: 'gpt4m', name: 'GPT-4o Mini', provider: 'OpenAI', icon: '🟢', calls: 89000, tokens: 22_250_000, cost: 3.34, avgLatency: 420, errorRate: 0.08, p99: 1100, status: 'healthy', trend: '+28%', model: 'gpt-4o-mini' },
  { id: 'gemini', name: 'Gemini 1.5 Flash', provider: 'Google', icon: '🔵', calls: 22100, tokens: 11_050_000, cost: 2.21, avgLatency: 610, errorRate: 0.06, p99: 1900, status: 'healthy', trend: '+7%', model: 'gemini-1.5-flash' },
  { id: 'ollama', name: 'Llama3 (Local)', provider: 'Ollama', icon: '🦙', calls: 7840, tokens: 4_700_000, cost: 0.00, avgLatency: 890, errorRate: 0.0, p99: 2400, status: 'healthy', trend: '+180%', model: 'llama3' },
]

const RECENT_CALLS = [
  { id: 'c1', ts: '19:47:12', model: 'Claude Haiku 4.5', prompt: 'Analyze DB connection pool exhaustion and recommend actions', tokens: 1840, cost: '$0.00046', latency: 380, status: 'success', cached: false },
  { id: 'c2', ts: '19:46:58', model: 'Llama3 (Local)', prompt: 'Root cause analysis: user-service error rate spike', tokens: 2200, cost: '$0.00', latency: 820, status: 'success', cached: false },
  { id: 'c3', ts: '19:46:44', model: 'GPT-4o Mini', prompt: 'Generate incident summary for P-1001', tokens: 680, cost: '$0.0001', latency: 412, status: 'success', cached: true },
  { id: 'c4', ts: '19:46:30', model: 'Claude Haiku 4.5', prompt: 'Should I rollback checkout-service based on error trends?', tokens: 1120, cost: '$0.00028', latency: 356, status: 'success', cached: false },
  { id: 'c5', ts: '19:45:18', model: 'GPT-4o', prompt: 'Predict memory growth trajectory for ml-inference pod', tokens: 3400, cost: '$0.0102', latency: 2240, status: 'timeout', cached: false },
  { id: 'c6', ts: '19:44:50', model: 'Gemini 1.5 Flash', prompt: 'Classify this log pattern as anomaly or normal', tokens: 920, cost: '$0.0001', latency: 580, status: 'success', cached: false },
]

const statC: Record<string,string> = { healthy: '#0fcf8a', warning: '#f5a623', critical: '#ff4d6a' }
const callC: Record<string,string> = { success: '#0fcf8a', timeout: '#ff4d6a', error: '#ff4d6a' }

function MiniBar({val, max, color}: {val:number,max:number,color:string}) {
  return <div style={{width:60,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}><div style={{height:'100%',width:`${(val/max)*100}%`,background:color}}/></div>
}

export default function LLMMonitoringPage() {
  const [selected, setSelected] = useState(PROVIDERS[0])
  const [tab, setTab] = useState<'overview'|'calls'|'cost'>('overview')
  const [totalCost] = useState(PROVIDERS.reduce((a,p)=>a+p.cost,0).toFixed(2))
  const [totalCalls] = useState(PROVIDERS.reduce((a,p)=>a+p.calls,0).toLocaleString())
  const [totalTokens] = useState((PROVIDERS.reduce((a,p)=>a+p.tokens,0)/1_000_000).toFixed(1))

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'16px 20px 12px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:4}}>
          <div style={{width:36,height:36,borderRadius:10,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:18}}>🔬</div>
          <div>
            <div style={{fontSize:20,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>LLM Monitor <span style={{fontSize:12,background:'rgba(108,114,255,.2)',color:'#8b90ff',padding:'2px 8px',borderRadius:10,fontWeight:700,verticalAlign:'middle'}}>★ UNIQUE</span></div>
            <div style={{fontSize:12,color:'#546a88'}}>Unified observability for all AI models — Claude, GPT-4, Gemini, Llama3 (Ollama) in one platform</div>
          </div>
        </div>
        <div style={{display:'flex',gap:10,marginTop:10}}>
          {[
            {l:'Total Calls (24h)', v: totalCalls, c:'#6c72ff'},
            {l:'Total Tokens', v:`${totalTokens}M`, c:'#a855f7'},
            {l:'Total Cost (24h)', v:`$${totalCost}`, c:'#f5a623'},
            {l:'Avg Latency', v:'521ms', c:'#0fcf8a'},
            {l:'Ollama Savings', v:'100% free', c:'#0fcf8a'},
            {l:'Cache Hit Rate', v:'18.4%', c:'#8b90ff'},
          ].map(k => (
            <div key={k.l} style={{background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:10,padding:'7px 14px'}}>
              <div style={{fontSize:9,color:'#546a88',textTransform:'uppercase',letterSpacing:'.05em'}}>{k.l}</div>
              <div style={{fontSize:17,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        {/* Provider list */}
        <div style={{width:260,borderRight:'1px solid #1a2d4a',overflowY:'auto',flexShrink:0}}>
          {PROVIDERS.map(p => (
            <div key={p.id} onClick={()=>setSelected(p)}
              style={{padding:'12px 14px',cursor:'pointer',borderBottom:'1px solid #0d1a2e',borderLeft:`3px solid ${selected.id===p.id?statC[p.status]:'transparent'}`,background:selected.id===p.id?'rgba(108,114,255,.05)':'transparent'}}>
              <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:7}}>
                <span style={{fontSize:18}}>{p.icon}</span>
                <div style={{flex:1}}>
                  <div style={{fontSize:12,fontWeight:700,color:'#f0f6ff'}}>{p.name}</div>
                  <div style={{fontSize:10,color:'#546a88'}}>{p.provider}</div>
                </div>
                <div style={{width:7,height:7,borderRadius:'50%',background:statC[p.status]}}/>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr 1fr',gap:4}}>
                {[{l:'Calls',v:p.calls.toLocaleString(),c:'#8b90ff'},{l:'Cost',v:`$${p.cost.toFixed(2)}`,c:p.cost===0?'#0fcf8a':'#f5a623'},{l:'P99',v:`${p.p99}ms`,c:p.p99>2000?'#f5a623':'#8fa8cc'}].map(k=>(
                  <div key={k.l} style={{background:'#0b1628',borderRadius:6,padding:'4px 6px'}}>
                    <div style={{fontSize:8,color:'#546a88'}}>{k.l}</div>
                    <div style={{fontSize:11,fontWeight:700,color:k.c,fontFamily:'JetBrains Mono,monospace'}}>{k.v}</div>
                  </div>
                ))}
              </div>
              {p.id==='ollama' && (
                <div style={{marginTop:6,padding:'3px 8px',background:'rgba(15,207,138,.08)',border:'1px solid rgba(15,207,138,.2)',borderRadius:8,fontSize:9,color:'#0fcf8a',fontWeight:700}}>★ Zero data egress · On-premise · FREE</div>
              )}
            </div>
          ))}
        </div>

        {/* Detail panel */}
        <div style={{flex:1,overflowY:'auto',padding:18,display:'flex',flexDirection:'column',gap:12}}>
          {/* Selected model header */}
          <div style={{background:'rgba(17,31,53,.7)',border:`1px solid ${statC[selected.status]}33`,borderRadius:14,padding:16}}>
            <div style={{display:'flex',alignItems:'center',gap:12,marginBottom:12}}>
              <span style={{fontSize:32}}>{selected.icon}</span>
              <div style={{flex:1}}>
                <div style={{fontSize:18,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{selected.name}</div>
                <div style={{fontSize:12,color:'#546a88'}}>{selected.provider} · <span style={{fontFamily:'JetBrains Mono,monospace'}}>{selected.model}</span></div>
              </div>
              <div style={{textAlign:'right'}}>
                <div style={{fontSize:12,color:'#546a88'}}>Trend</div>
                <div style={{fontSize:16,fontWeight:800,color:selected.trend.startsWith('+')?'#0fcf8a':'#f5a623',fontFamily:'Syne,sans-serif'}}>{selected.trend}</div>
              </div>
            </div>
            <div style={{display:'grid',gridTemplateColumns:'repeat(6,1fr)',gap:8}}>
              {[
                {l:'24h Calls',v:selected.calls.toLocaleString(),c:'#6c72ff'},
                {l:'Tokens Used',v:`${(selected.tokens/1_000_000).toFixed(1)}M`,c:'#a855f7'},
                {l:'Cost (24h)',v:`$${selected.cost.toFixed(2)}`,c:selected.cost===0?'#0fcf8a':'#f5a623'},
                {l:'Avg Latency',v:`${selected.avgLatency}ms`,c:'#8fa8cc'},
                {l:'P99 Latency',v:`${selected.p99}ms`,c:selected.p99>2000?'#f5a623':'#8fa8cc'},
                {l:'Error Rate',v:`${selected.errorRate}%`,c:selected.errorRate>0.1?'#f5a623':'#0fcf8a'},
              ].map(k=>(
                <div key={k.l} style={{background:'rgba(7,15,30,.5)',border:'1px solid #1a2d4a',borderRadius:10,padding:'8px 10px'}}>
                  <div style={{fontSize:9,color:'#546a88'}}>{k.l}</div>
                  <div style={{fontSize:16,fontWeight:800,color:k.c,fontFamily:'Syne,sans-serif'}}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Tab strip */}
          <div style={{display:'flex',gap:0,borderBottom:'1px solid #1a2d4a'}}>
            {(['overview','calls','cost'] as const).map(t=>(
              <button key={t} onClick={()=>setTab(t)}
                style={{padding:'8px 18px',background:'none',border:'none',borderBottom:`2px solid ${tab===t?'#6c72ff':'transparent'}`,color:tab===t?'#8b90ff':'#546a88',fontSize:12,fontWeight:600,cursor:'pointer',textTransform:'capitalize'}}>
                {t==='overview'?'Latency Distribution':t==='calls'?'Recent Calls':'Cost Breakdown'}
              </button>
            ))}
          </div>

          {tab==='overview' && (
            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12}}>
              {/* Latency histogram */}
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Latency Distribution (ms)</div>
                {[
                  {range:'0–200ms',count:28,pct:28},
                  {range:'200–500ms',count:44,pct:44},
                  {range:'500ms–1s',count:18,pct:18},
                  {range:'1s–2s',count:7,pct:7},
                  {range:'2s+',count:3,pct:3},
                ].map(b=>(
                  <div key={b.range} style={{display:'flex',alignItems:'center',gap:8,marginBottom:7}}>
                    <span style={{fontSize:10,color:'#546a88',width:80,flexShrink:0,fontFamily:'JetBrains Mono,monospace'}}>{b.range}</span>
                    <div style={{flex:1,height:16,background:'#0b1628',borderRadius:4,overflow:'hidden',position:'relative'}}>
                      <div style={{height:'100%',width:`${b.pct}%`,background:b.pct>30?'#6c72ff':b.pct>15?'#a855f7':'#f5a623',opacity:.8}}/>
                      <span style={{position:'absolute',left:6,top:'50%',transform:'translateY(-50%)',fontSize:9,color:'#c8d8ef',fontWeight:700}}>{b.pct}%</span>
                    </div>
                    <span style={{fontSize:10,color:'#546a88',width:28,textAlign:'right'}}>{b.count}%</span>
                  </div>
                ))}
              </div>
              {/* Model comparison */}
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>All Models — Cost vs Latency</div>
                {PROVIDERS.map(p=>(
                  <div key={p.id} style={{display:'flex',alignItems:'center',gap:8,marginBottom:7}}>
                    <span style={{fontSize:14}}>{p.icon}</span>
                    <span style={{fontSize:11,flex:1,color:'#8fa8cc'}}>{p.name.split(' ').slice(0,-1).join(' ')}</span>
                    <span style={{fontSize:10,fontFamily:'JetBrains Mono,monospace',color:p.cost===0?'#0fcf8a':'#f5a623',width:40,textAlign:'right'}}>${p.cost.toFixed(2)}</span>
                    <MiniBar val={p.avgLatency} max={1300} color={p.avgLatency>700?'#f5a623':'#6c72ff'}/>
                    <span style={{fontSize:10,fontFamily:'JetBrains Mono,monospace',color:'#546a88',width:40,textAlign:'right'}}>{p.avgLatency}ms</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {tab==='calls' && (
            <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,overflow:'hidden'}}>
              <table style={{width:'100%',borderCollapse:'collapse'}}>
                <thead>
                  <tr style={{background:'#080d1b',borderBottom:'1px solid #1a2d4a'}}>
                    {['Time','Model','Prompt (truncated)','Tokens','Cost','Latency','Status'].map(h=>(
                      <th key={h} style={{padding:'8px 12px',textAlign:'left',fontSize:9,fontWeight:700,color:'#546a88',textTransform:'uppercase'}}>{h}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {RECENT_CALLS.map(c=>(
                    <tr key={c.id} style={{borderBottom:'1px solid rgba(26,45,74,.4)'}}>
                      <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#546a88',whiteSpace:'nowrap'}}>{c.ts}</td>
                      <td style={{padding:'8px 12px',fontSize:11,color:'#8b90ff',whiteSpace:'nowrap'}}>{c.model}</td>
                      <td style={{padding:'8px 12px',fontSize:11,color:'#8fa8cc',maxWidth:280}}>
                        <div style={{overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{c.prompt}</div>
                      </td>
                      <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:'#8fa8cc'}}>{c.tokens.toLocaleString()}</td>
                      <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:c.cost==='$0.00'?'#0fcf8a':'#f5a623'}}>{c.cost}</td>
                      <td style={{padding:'8px 12px',fontFamily:'JetBrains Mono,monospace',fontSize:11,color:c.latency>1000?'#f5a623':'#8fa8cc'}}>{c.latency}ms</td>
                      <td style={{padding:'8px 12px'}}>
                        <div style={{display:'flex',alignItems:'center',gap:5}}>
                          <div style={{width:6,height:6,borderRadius:'50%',background:callC[c.status]}}/>
                          <span style={{color:callC[c.status],fontSize:11}}>{c.status}</span>
                          {c.cached && <span style={{fontSize:9,background:'rgba(108,114,255,.2)',color:'#8b90ff',padding:'1px 5px',borderRadius:8}}>cached</span>}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {tab==='cost' && (
            <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:12}}>
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Cost Breakdown by Provider (24h)</div>
                {PROVIDERS.sort((a,b)=>b.cost-a.cost).map(p=>(
                  <div key={p.id} style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
                    <span style={{fontSize:16}}>{p.icon}</span>
                    <span style={{fontSize:11,flex:1,color:'#8fa8cc'}}>{p.name}</span>
                    <MiniBar val={p.cost} max={20} color={p.cost===0?'#0fcf8a':'#6c72ff'}/>
                    <span style={{fontSize:13,fontWeight:800,color:p.cost===0?'#0fcf8a':'#f5a623',fontFamily:'Syne,sans-serif',width:48,textAlign:'right'}}>${p.cost.toFixed(2)}</span>
                  </div>
                ))}
                <div style={{marginTop:12,padding:'8px 12px',background:'rgba(15,207,138,.07)',border:'1px solid rgba(15,207,138,.2)',borderRadius:8}}>
                  <div style={{fontSize:11,color:'#0fcf8a',fontWeight:700}}>💡 Ollama savings: ~$14.10/day</div>
                  <div style={{fontSize:10,color:'#546a88'}}>7,840 calls routed to local Llama3 at zero cost</div>
                </div>
              </div>
              <div style={{background:'rgba(17,31,53,.7)',border:'1px solid #1a2d4a',borderRadius:12,padding:14}}>
                <div style={{fontSize:12,fontWeight:700,color:'#c8d8ef',marginBottom:12}}>Token Usage by Model (24h)</div>
                {PROVIDERS.sort((a,b)=>b.tokens-a.tokens).map(p=>(
                  <div key={p.id} style={{display:'flex',alignItems:'center',gap:10,marginBottom:8}}>
                    <span style={{fontSize:16}}>{p.icon}</span>
                    <span style={{fontSize:11,flex:1,color:'#8fa8cc'}}>{p.name}</span>
                    <div style={{width:80,height:4,background:'#0b1628',borderRadius:2,overflow:'hidden'}}>
                      <div style={{height:'100%',width:`${(p.tokens/22_250_000)*100}%`,background:'#6c72ff'}}/>
                    </div>
                    <span style={{fontSize:11,fontFamily:'JetBrains Mono,monospace',color:'#8fa8cc',width:52,textAlign:'right'}}>{(p.tokens/1_000_000).toFixed(1)}M</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
