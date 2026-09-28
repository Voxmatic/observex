// TraceDetailPage.tsx — /traces/:traceId — Distributed tracing waterfall
// Equivalent to: Datadog Trace Flamegraph, New Relic Trace Waterfall, Jaeger
import { useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { StatusBadge } from '@/components/shared/StatusBadge'
import { ArrowLeft, Clock, AlertTriangle, Layers, ChevronDown, ChevronRight } from 'lucide-react'

const TRACES: Record<string,any> = {
  'abc-123-def': {
    traceId:'abc-123-def', rootService:'api-gateway', rootOperation:'POST /api/v2/checkout/initiate',
    duration:842, startTime:'2026-04-18T19:47:23.142Z', status:'error', spanCount:14, serviceCount:4, errorCount:2,
    spans:[
      { id:'s1', parent:null, service:'api-gateway', operation:'POST /api/v2/checkout/initiate', duration:842, start:0, status:'error', tags:{http_method:'POST',http_status:500}, depth:0 },
      { id:'s2', parent:'s1', service:'api-gateway', operation:'middleware.auth', duration:12, start:2, status:'ok', tags:{}, depth:1 },
      { id:'s3', parent:'s1', service:'api-gateway', operation:'middleware.rateLimit', duration:1, start:14, status:'ok', tags:{}, depth:1 },
      { id:'s4', parent:'s1', service:'checkout-service', operation:'handler.initiateCheckout', duration:820, start:16, status:'error', tags:{http_status:500}, depth:1 },
      { id:'s5', parent:'s4', service:'checkout-service', operation:'cartService.getCart', duration:48, start:18, status:'ok', tags:{cart_items:3}, depth:2 },
      { id:'s6', parent:'s5', service:'redis-cache', operation:'GET cart:8420', duration:4, start:20, status:'ok', tags:{cache:'hit'}, depth:3 },
      { id:'s7', parent:'s4', service:'checkout-service', operation:'userService.getProfile', duration:760, start:66, status:'error', tags:{}, depth:2 },
      { id:'s8', parent:'s7', service:'user-service', operation:'GET /api/v1/users/8420', duration:748, start:70, status:'error', tags:{http_status:500}, depth:3 },
      { id:'s9', parent:'s8', service:'user-service', operation:'UserService.getProfile', duration:740, start:72, status:'error', tags:{error:'NullPointerException'}, depth:4 },
      { id:'s10', parent:'s9', service:'postgres-primary', operation:'SELECT * FROM users WHERE id=$1', duration:2, start:74, status:'ok', tags:{db:'observex',rows:1}, depth:5 },
      { id:'s11', parent:'s9', service:'postgres-primary', operation:'SELECT * FROM preferences WHERE user_id=$1', duration:730, start:76, status:'error', tags:{db:'observex',error:'connection timeout after 30000ms'}, depth:5 },
      { id:'s12', parent:'s4', service:'checkout-service', operation:'paymentService.validate', duration:2, start:828, status:'ok', tags:{}, depth:2 },
      { id:'s13', parent:'s4', service:'checkout-service', operation:'kafka.produce checkout.events', duration:8, start:832, status:'ok', tags:{topic:'checkout.events'}, depth:2 },
      { id:'s14', parent:'s1', service:'api-gateway', operation:'response.serialize', duration:2, start:838, status:'ok', tags:{}, depth:1 },
    ],
  },
}

const SVC_COLORS: Record<string,string> = {'api-gateway':'#6c72ff','checkout-service':'#0fcf8a','user-service':'#f5a623','redis-cache':'#a855f7','postgres-primary':'#ff4d6a','payment-service':'#3d9bff','kafka':'#8fa8cc'}
const STATUS_C: Record<string,string> = {ok:'#0fcf8a',error:'#ff4d6a',slow:'#f5a623'}

export default function TraceDetailPage() {
  const { traceId } = useParams<{traceId:string}>()
  const [selected, setSelected] = useState<any>(null)
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  const trace = TRACES[traceId||''] || TRACES['abc-123-def']
  const totalDuration = trace.duration

  const toggleCollapse = (id:string) => {
    setCollapsed(prev => { const n = new Set(prev); n.has(id)?n.delete(id):n.add(id); return n })
  }

  // Filter visible spans (respect collapsed parents)
  const visibleSpans = trace.spans.filter((span:any) => {
    let parent = span.parent
    while(parent) {
      if(collapsed.has(parent)) return false
      parent = trace.spans.find((s:any)=>s.id===parent)?.parent
    }
    return true
  })

  return (
    <div style={{display:'flex',flexDirection:'column',height:'100%',background:'#070f1e',color:'#c8d8ef',overflow:'hidden'}}>
      <div style={{padding:'12px 20px',borderBottom:'1px solid #1a2d4a',flexShrink:0}}>
        <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:6}}>
          <Link to="/traces" style={{color:'#546a88',display:'flex'}}><ArrowLeft size={16}/></Link>
          <div style={{flex:1}}>
            <div style={{fontSize:16,fontWeight:800,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{trace.rootOperation}</div>
            <div style={{display:'flex',gap:10,fontSize:10,color:'#546a88',marginTop:3}}>
              <span>Trace: <span style={{fontFamily:'JetBrains Mono,monospace',color:'#6c72ff'}}>{trace.traceId}</span></span>
              <span>{trace.spanCount} spans</span>
              <span>{trace.serviceCount} services</span>
              <span style={{color:trace.errorCount>0?'#ff4d6a':'#0fcf8a'}}>{trace.errorCount} errors</span>
            </div>
          </div>
          <div style={{textAlign:'right'}}>
            <StatusBadge status={trace.status}/>
            <div style={{fontSize:18,fontWeight:900,color:STATUS_C[trace.status],fontFamily:'Syne,sans-serif',marginTop:3}}>{trace.duration}ms</div>
          </div>
        </div>
        {/* Service legend */}
        <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
          {[...new Set(trace.spans.map((s:any)=>s.service))].map((svc:string)=>(
            <div key={svc} style={{display:'flex',gap:4,alignItems:'center'}}>
              <div style={{width:10,height:10,borderRadius:3,background:SVC_COLORS[svc]||'#546a88'}}/>
              <span style={{fontSize:10,color:'#c8d8ef'}}>{svc}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Waterfall */}
      <div style={{display:'flex',flex:1,overflow:'hidden'}}>
        <div style={{flex:1,overflowY:'auto',overflowX:'hidden'}}>
          {/* Time ruler */}
          <div style={{display:'flex',height:20,borderBottom:'1px solid #1a2d4a',position:'sticky',top:0,background:'#080d1b',zIndex:2}}>
            <div style={{width:280,flexShrink:0,borderRight:'1px solid #1a2d4a'}}/>
            <div style={{flex:1,position:'relative'}}>
              {[0,25,50,75,100].map(pct=>(
                <span key={pct} style={{position:'absolute',left:`${pct}%`,fontSize:9,color:'#3d5070',transform:'translateX(-50%)',top:3}}>
                  {Math.round(totalDuration*pct/100)}ms
                </span>
              ))}
            </div>
          </div>

          {/* Spans */}
          {visibleSpans.map((span:any,i:number) => {
            const leftPct = (span.start/totalDuration)*100
            const widthPct = Math.max((span.duration/totalDuration)*100, 0.5)
            const color = SVC_COLORS[span.service]||'#546a88'
            const hasChildren = trace.spans.some((s:any)=>s.parent===span.id)
            const isCollapsed = collapsed.has(span.id)
            const isSelected = selected?.id===span.id
            const indent = span.depth * 16

            return (
              <div key={span.id} onClick={()=>setSelected(span)}
                style={{display:'flex',height:28,borderBottom:'1px solid rgba(26,45,74,.2)',cursor:'pointer',background:isSelected?'rgba(108,114,255,.06)':'transparent'}}>
                {/* Label */}
                <div style={{width:280,flexShrink:0,borderRight:'1px solid #1a2d4a',display:'flex',alignItems:'center',paddingLeft:8+indent,gap:4,overflow:'hidden'}}>
                  {hasChildren&&(
                    <div onClick={e=>{e.stopPropagation();toggleCollapse(span.id)}} style={{cursor:'pointer',color:'#546a88',flexShrink:0}}>
                      {isCollapsed?<ChevronRight size={10}/>:<ChevronDown size={10}/>}
                    </div>
                  )}
                  {!hasChildren&&<div style={{width:10}}/>}
                  <div style={{width:8,height:8,borderRadius:2,background:color,flexShrink:0}}/>
                  <span style={{fontSize:10,color:span.status==='error'?'#ff7a92':'#c8d8ef',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap',fontFamily:'JetBrains Mono,monospace'}}>
                    {span.operation}
                  </span>
                </div>
                {/* Waterfall bar */}
                <div style={{flex:1,position:'relative',display:'flex',alignItems:'center'}}>
                  <div style={{position:'absolute',left:`${leftPct}%`,width:`${widthPct}%`,height:16,background:span.status==='error'?`${color}88`:color,borderRadius:3,minWidth:2,border:span.status==='error'?`1px solid ${color}`:'none',opacity:span.status==='error'?0.8:0.7}}/>
                  <span style={{position:'absolute',left:`${leftPct+widthPct+0.5}%`,fontSize:9,color:'#546a88',whiteSpace:'nowrap'}}>
                    {span.duration}ms
                    {span.status==='error'&&<span style={{color:'#ff4d6a',marginLeft:4}}>✗</span>}
                  </span>
                </div>
              </div>
            )
          })}
        </div>

        {/* Span detail sidebar */}
        {selected&&(
          <div style={{width:300,borderLeft:'1px solid #1a2d4a',overflowY:'auto',padding:14,flexShrink:0,background:'#080d1b'}}>
            <div style={{fontSize:12,fontWeight:800,color:'#f0f6ff',marginBottom:8,fontFamily:'Syne,sans-serif'}}>Span detail</div>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:11,color:SVC_COLORS[selected.service],marginBottom:4}}>{selected.service}</div>
            <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#c8d8ef',marginBottom:12,wordBreak:'break-all'}}>{selected.operation}</div>
            {[
              {l:'Duration', v:`${selected.duration}ms`, c:selected.duration>500?'#ff4d6a':'#8fa8cc'},
              {l:'Start',    v:`+${selected.start}ms`,   c:'#546a88'},
              {l:'Status',   v:selected.status,          c:STATUS_C[selected.status]},
              {l:'Span ID',  v:selected.id,              c:'#546a88'},
              {l:'Parent',   v:selected.parent||'root',  c:'#546a88'},
            ].map(k=>(
              <div key={k.l} style={{display:'flex',justifyContent:'space-between',fontSize:11,marginBottom:6,paddingBottom:6,borderBottom:'1px solid rgba(26,45,74,.3)'}}>
                <span style={{color:'#546a88'}}>{k.l}</span>
                <span style={{color:k.c,fontFamily:'JetBrains Mono,monospace',fontWeight:600}}>{k.v}</span>
              </div>
            ))}
            {Object.keys(selected.tags).length>0&&(
              <>
                <div style={{fontSize:11,fontWeight:700,color:'#c8d8ef',marginTop:10,marginBottom:6}}>Tags</div>
                {Object.entries(selected.tags).map(([k,v])=>(
                  <div key={k} style={{display:'flex',justifyContent:'space-between',fontSize:10,marginBottom:4}}>
                    <span style={{color:'#546a88'}}>{k}</span>
                    <span style={{color:k==='error'?'#ff4d6a':'#8fa8cc',fontFamily:'JetBrains Mono,monospace',maxWidth:160,overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap',textAlign:'right'}}>{String(v)}</span>
                  </div>
                ))}
              </>
            )}
            {selected.status==='error'&&selected.tags.error&&(
              <div style={{marginTop:12,padding:'8px 10px',background:'rgba(255,77,106,.08)',border:'1px solid rgba(255,77,106,.2)',borderRadius:8}}>
                <div style={{fontSize:10,fontWeight:700,color:'#ff4d6a',marginBottom:4}}>Error</div>
                <div style={{fontFamily:'JetBrains Mono,monospace',fontSize:10,color:'#ff7a92',lineHeight:1.5}}>{selected.tags.error}</div>
              </div>
            )}
            <div style={{marginTop:12,display:'flex',gap:6}}>
              <Link to={`/services/${selected.service}`} style={{background:'#6c72ff',color:'#fff',border:'none',padding:'6px 10px',borderRadius:6,fontSize:10,textDecoration:'none',fontWeight:600}}>View service</Link>
              <button style={{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc',padding:'6px 10px',borderRadius:6,fontSize:10,cursor:'pointer'}}>View logs</button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
