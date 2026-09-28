import { useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { useAuth } from '@/store/auth'
import {
  LayoutDashboard, Activity, Layers, FileText, GitBranch, Zap, Flame,
  ShieldCheck, Bell, Settings, LogOut, ChevronRight, ChevronDown,
  Radio, Search, Monitor, Boxes, Database, BarChart3, Wifi,
  Server, Lock, BookOpen, Plug, Shield, ShieldAlert,
  DollarSign, Clock, TrendingUp, Heart, Package, Globe,
  Brain, AlertTriangle, BookMarked, Cpu, BarChart2, Grid,
  Shuffle, Bug, Target, FlaskConical, Rocket, Link2, Flag,
  Download, AlertCircle, RefreshCw, Network, Layers2, GitMerge,
  Eye, Microscope, Users, Gauge, Play, Waves, Star, 
  CheckSquare, Smartphone, DatabaseZap, Repeat, Terminal
} from 'lucide-react'

const NAV = [
  {
    id:'observe', label:'Observe & Explore',
    items:[
      { to:'/',                  icon:LayoutDashboard, label:'Overview',          badge:null },
      { to:'/problems',          icon:AlertCircle,     label:'Problems',          badge:'2', bc:'#ff4d6a' },
      { to:'/events',            icon:Zap,             label:'Events',            badge:null },
      { to:'/smartscape',        icon:Wifi,            label:'Smartscape',        badge:null },
      { to:'/topology',          icon:GitBranch,       label:'Service Map',       badge:null },
      { to:'/logs',              icon:FileText,        label:'Logs',              badge:null },
      { to:'/metrics',           icon:Activity,        label:'Metrics',           badge:null },
      { to:'/traces',            icon:Layers,          label:'Traces',            badge:null },
      { to:'/notebooks',         icon:BookMarked,      label:'Notebooks',         badge:null },
      { to:'/dashboards',        icon:Grid,            label:'Dashboards',        badge:null },
    ],
  },
  {
    id:'application', label:'Application',
    items:[
      { to:'/apm',                   icon:BarChart3,  label:'Services / APM',      badge:null },
      { to:'/business-transactions', icon:GitMerge,   label:'Business Transactions',badge:null },
      { to:'/service-endpoints',     icon:Link2,      label:'Service Endpoints',   badge:null },
      { to:'/tiers-nodes',           icon:Layers2,    label:'Tiers & Nodes',       badge:null },
      { to:'/errors',                icon:Bug,        label:'Error Tracking',      badge:null },
      { to:'/errors-inbox',          icon:Target,     label:'Errors Inbox',        badge:'18', bc:'#f5a623' },
      { to:'/profiling',             icon:Flame,      label:'Code Profiling',      badge:null },
      { to:'/release-health',        icon:Heart,      label:'Release Health',      badge:null },
    ],
  },
  {
    id:'experience', label:'Digital Experience',
    items:[
      { to:'/rum',            icon:Monitor,     label:'Real User Monitoring', badge:null },
      { to:'/mobile',         icon:Smartphone,  label:'Mobile',              badge:null },
      { to:'/session-replay', icon:Eye,         label:'Session Replay',      badge:null },
      { to:'/synthetic',      icon:Radio,       label:'Synthetic Monitoring', badge:null },
      { to:'/uptime',         icon:Globe,       label:'Uptime Monitor',      badge:null },
    ],
  },
  {
    id:'infrastructure', label:'Infrastructure',
    items:[
      { to:'/infrastructure', icon:Server,    label:'Hosts',              badge:null },
      { to:'/kubernetes',     icon:Boxes,     label:'Kubernetes',         badge:null },
      { to:'/database',       icon:Database,  label:'Databases',          badge:null },
      { to:'/network',        icon:Network,   label:'Network',            badge:null },
      { to:'/serverless',     icon:Globe,     label:'Serverless',         badge:null },
    ],
  },
  {
    id:'data', label:'Data & Pipelines',
    items:[
      { to:'/data-streams',    icon:Waves,       label:'Data Streams',        badge:null },
      { to:'/background-jobs', icon:Cpu,         label:'Background Jobs',     badge:null },
      { to:'/cron-monitor',    icon:Clock,       label:'Cron Monitor',        badge:null },
      { to:'/pipeline',        icon:Shuffle,     label:'OTel Pipeline',       badge:null },
    ],
  },
  {
    id:'ai', label:'AI & Intelligence',
    items:[
      { to:'/ai-agent',         icon:Brain,       label:'AI Agent',            badge:'●', bc:'#0fcf8a' },
      { to:'/deviation-analysis',icon:Eye,        label:'Deviation Analysis',  badge:null },
      { to:'/llm-monitor',      icon:Microscope,  label:'LLM Monitor',         badge:null },
      { to:'/watchdog',         icon:ShieldCheck, label:'Watchdog AI',         badge:null },
      { to:'/anomaly',          icon:AlertTriangle,label:'Anomaly Detection',  badge:null },
      { to:'/forecast',         icon:TrendingUp,  label:'ML Forecast',         badge:null },
      { to:'/adaptive-telemetry',icon:RefreshCw,  label:'Adaptive Telemetry',  badge:null },
    ],
  },
  {
    id:'alerts', label:'Alerts & Incidents',
    items:[
      { to:'/alerts',          icon:Bell,        label:'Alerts',              badge:null },
      { to:'/incidents',       icon:Zap,         label:'Incidents',           badge:'1', bc:'#ff4d6a' },
      { to:'/slos',            icon:ShieldCheck, label:'SLOs',                badge:null },
      { to:'/change-tracking', icon:GitMerge,    label:'Change Tracking',     badge:null },
      { to:'/workloads',       icon:Users,       label:'Workloads',           badge:null },
      { to:'/oncall',          icon:Clock,       label:'On-Call',             badge:null },
      { to:'/runbooks',        icon:BookOpen,    label:'Runbooks',            badge:null },
      { to:'/status-page',     icon:Globe,       label:'Status Page',         badge:null },
      { to:'/postmortems',     icon:FileText,    label:'Postmortems',         badge:null },
    ],
  },
  {
    id:'security', label:'Security',
    items:[
      { to:'/security', icon:ShieldAlert, label:'Runtime Security',     badge:null },
      { to:'/auth',     icon:Lock,        label:'Vulnerability Mgmt',   badge:null },
      { to:'/compliance',icon:Shield,     label:'Compliance / Audit',   badge:null },
    ],
  },
  {
    id:'devops', label:'DevOps & Delivery',
    items:[
      { to:'/dora',                   icon:Gauge,       label:'DORA Metrics',         badge:null },
      { to:'/ci-pipeline',            icon:Play,        label:'CI/CD Pipelines',      badge:null },
      { to:'/load-testing',           icon:Zap,         label:'Load Testing',         badge:null },
      { to:'/chaos',                  icon:FlaskConical,label:'Chaos Engineering',    badge:null },
      { to:'/feature-flags',          icon:Flag,        label:'Feature Flags',        badge:null },
      { to:'/releases',               icon:Package,     label:'Releases',             badge:null },
      { to:'/deployment-intelligence',icon:Rocket,      label:'Deploy Intelligence',  badge:null },
      { to:'/kpis',                   icon:Target,      label:'Business KPIs',        badge:null },
    ],
  },
  {
    id:'enterprise', label:'Enterprise',
    items:[
      { to:'/catalog',     icon:BookOpen, label:'Service Catalog',      badge:null },
      { to:'/tech-matrix', icon:Grid,     label:'Tech Matrix',          badge:null },
      { to:'/api-catalog', icon:Globe,    label:'API Catalog',          badge:null },
      { to:'/sso',         icon:Shield,   label:'SSO / SAML',           badge:null },
      { to:'/settings',    icon:Settings, label:'Settings',             badge:null },
    ],
  },
  {
    id:'deploy', label:'Deploy & Connect',
    items:[
      { to:'/hub',          icon:Package, label:'Hub / Marketplace',    badge:null },
      { to:'/agent-install',icon:Download,label:'Install Agent',        badge:null },
      { to:'/agent-fleet',  icon:Server,  label:'Agent Fleet',          badge:null },
      { to:'/integrations', icon:Plug,    label:'Integrations',         badge:null },
    ],
  },
]

export function Layout() {
  const { user, org, logout } = useAuth()
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState<Record<string,boolean>>({})
  const toggle = (id:string) => setCollapsed(s=>({...s,[id]:!s[id]}))

  return (
    <div style={{display:'flex',height:'100vh',background:'#070f1e',overflow:'hidden'}}>
      <aside style={{width:218,flexShrink:0,display:'flex',flexDirection:'column',background:'#080d1b',borderRight:'1px solid #1a2d4a',overflow:'hidden'}}>
        {/* Logo */}
        <div style={{height:52,display:'flex',alignItems:'center',padding:'0 14px',borderBottom:'1px solid #1a2d4a',flexShrink:0,gap:8}}>
          <div style={{width:26,height:26,borderRadius:7,background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:13,flexShrink:0}}>✦</div>
          <span style={{fontFamily:'Syne,sans-serif',fontWeight:900,fontSize:16,background:'linear-gradient(90deg,#6c72ff,#a855f7)',WebkitBackgroundClip:'text',WebkitTextFillColor:'transparent'}}>ObserveX</span>
          {org&&<span style={{fontSize:9,color:'#546a88',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{org.display_name||org.name}</span>}
        </div>
        {/* Nav */}
        <nav style={{flex:1,overflowY:'auto',padding:'6px 0',scrollbarWidth:'none'}}>
          {NAV.map(group=>{
            const isCol = collapsed[group.id]
            return (
              <div key={group.id} style={{marginBottom:1}}>
                <button onClick={()=>toggle(group.id)} style={{width:'100%',display:'flex',alignItems:'center',padding:'5px 12px 3px',background:'none',border:'none',cursor:'pointer',gap:4}}>
                  <span style={{flex:1,fontSize:9,fontWeight:800,color:'#2d4060',textTransform:'uppercase',letterSpacing:'.08em',textAlign:'left'}}>{group.label}</span>
                  <ChevronDown size={9} style={{color:'#2d4060',transform:isCol?'rotate(-90deg)':'none',transition:'.15s'}}/>
                </button>
                {!isCol&&group.items.map(({to,icon:Icon,label,badge,bc})=>(
                  <NavLink key={to} to={to} end={to==='/'}
                    style={({isActive})=>({
                      display:'flex',alignItems:'center',gap:7,padding:'4px 10px 4px 12px',margin:'0 5px',borderRadius:7,
                      fontSize:11.5,fontWeight:isActive?700:400,
                      color:isActive?'#9da5ff':'#566a82',
                      background:isActive?'rgba(108,114,255,.1)':'transparent',
                      textDecoration:'none',transition:'.1s',
                    })}>
                    <Icon size={12} style={{flexShrink:0}}/>
                    <span style={{flex:1,lineHeight:1.2}}>{label}</span>
                    {badge&&<span style={{fontSize:badge==='●'?11:8,fontWeight:800,color:bc||'#546a88',background:badge==='●'?'none':`${bc||'#546a88'}22`,padding:badge==='●'?'0':'1px 4px',borderRadius:8,flexShrink:0}}>{badge}</span>}
                  </NavLink>
                ))}
              </div>
            )
          })}
        </nav>
        {/* User */}
        <div style={{padding:'8px',borderTop:'1px solid #1a2d4a',flexShrink:0}}>
          <div style={{display:'flex',alignItems:'center',gap:7,padding:'5px 8px',borderRadius:8,cursor:'pointer'}}
            onClick={async()=>{await logout();navigate('/login')}}
            onMouseEnter={e=>(e.currentTarget as HTMLElement).style.background='rgba(255,255,255,.04)'}
            onMouseLeave={e=>(e.currentTarget as HTMLElement).style.background='none'}>
            <div style={{width:24,height:24,borderRadius:'50%',background:'linear-gradient(135deg,#6c72ff,#a855f7)',display:'flex',alignItems:'center',justifyContent:'center',fontSize:10,fontWeight:800,color:'#fff',flexShrink:0}}>
              {user?.name?.charAt(0).toUpperCase()||'U'}
            </div>
            <div style={{flex:1,minWidth:0}}>
              <div style={{fontSize:11,fontWeight:600,color:'#8fa8cc',overflow:'hidden',textOverflow:'ellipsis',whiteSpace:'nowrap'}}>{user?.name}</div>
              <div style={{fontSize:9,color:'#3d5070',textTransform:'capitalize'}}>{user?.role}</div>
            </div>
            <LogOut size={11} style={{color:'#3d5070',flexShrink:0}}/>
          </div>
        </div>
      </aside>
      <main style={{flex:1,overflow:'hidden',display:'flex',flexDirection:'column',minWidth:0}}><Outlet/></main>
    </div>
  )
}

// ─── Shared UI Components ────────────────────────────────────────────────────
export function PageHeader({ title, subtitle, actions, breadcrumb, icon, children, className }: { title:string; subtitle?:string; actions?:React.ReactNode; breadcrumb?:string[]; icon?:React.ReactNode; children?:React.ReactNode; className?:string }) {
  return (
    <div style={{flexShrink:0,display:'flex',alignItems:'center',justifyContent:'space-between',padding:'14px 20px',borderBottom:'1px solid #1a2d4a'}}>
      <div style={{display:'flex',alignItems:'center',gap:10}}>
        {icon&&<div>{icon}</div>}
        <div>
          {breadcrumb&&<div style={{display:'flex',alignItems:'center',gap:4,fontSize:10,color:'#546a88',marginBottom:2}}>
            {breadcrumb.map((b,i)=><span key={i} style={{display:'flex',alignItems:'center',gap:4}}>{i>0&&<ChevronRight size={9}/>}{b}</span>)}
          </div>}
          <h1 style={{fontSize:16,fontWeight:700,color:'#f0f6ff',fontFamily:'Syne,sans-serif'}}>{title}</h1>
          {subtitle&&<p style={{fontSize:12,color:'#546a88',marginTop:2}}>{subtitle}</p>}
        </div>
      </div>
      <div style={{display:'flex',alignItems:'center',gap:8}}>
        {actions&&<>{actions}</>}
        {children&&<>{children}</>}
      </div>
    </div>
  )
}
export function StatusDot({ status }: { status:string }) {
  const c: Record<string,string> = { UP:'#0fcf8a',GOOD:'#0fcf8a',OK:'#0fcf8a',DOWN:'#ff4d6a',CRITICAL:'#ff4d6a',BREACHED:'#ff4d6a',DEGRADED:'#f5a623',HIGH:'#f5a623',WARNING:'#f5a623' }
  return <span style={{display:'inline-block',width:7,height:7,borderRadius:'50%',background:c[status.toUpperCase()]||'#546a88',flexShrink:0}}/>
}
export function SeverityBadge({ severity }: { severity:string }) {
  const c: Record<string,[string,string]> = { CRITICAL:['rgba(255,77,106,.15)','#ff7a92'],HIGH:['rgba(245,166,35,.15)','#f5a623'],MEDIUM:['rgba(168,85,247,.15)','#c084fc'],LOW:['rgba(84,106,136,.15)','#546a88'] }
  const [bg,color] = c[severity]||c.LOW
  return <span style={{fontSize:10,fontWeight:700,padding:'2px 7px',borderRadius:10,background:bg,color}}>{severity}</span>
}
export function Spinner({ size=16 }: { size?:number }) {
  return <svg width={size} height={size} viewBox="0 0 24 24" style={{animation:'spin 1s linear infinite',color:'#6c72ff'}}><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" strokeWidth="3" strokeDasharray="31.4" strokeDashoffset="10"/></svg>
}
export function EmptyState({ icon:Icon, title, description, children }: { icon:React.ElementType; title:string; description?:string; children?:React.ReactNode }) {
  return (
    <div style={{display:'flex',flexDirection:'column',alignItems:'center',justifyContent:'center',padding:'48px 24px',textAlign:'center'}}>
      <Icon size={36} style={{color:'#254060',marginBottom:12}}/>
      <h3 style={{fontSize:13,fontWeight:600,color:'#546a88',marginBottom:4}}>{title}</h3>
      {description&&<p style={{fontSize:12,color:'#3d5070',maxWidth:280}}>{description}</p>}
      {children&&<div style={{marginTop:14}}>{children}</div>}
    </div>
  )
}
export function SearchInput({ value, onChange, placeholder }: { value:string; onChange:(v:string)=>void; placeholder?:string }) {
  return (
    <div style={{position:'relative'}}>
      <Search size={12} style={{position:'absolute',left:9,top:'50%',transform:'translateY(-50%)',color:'#546a88'}}/>
      <input value={value} onChange={e=>onChange(e.target.value)} placeholder={placeholder||'Search…'}
        style={{paddingLeft:28,paddingRight:10,paddingTop:6,paddingBottom:6,fontSize:12,background:'#0b1628',border:'1px solid #1a2d4a',borderRadius:8,color:'#c8d8ef',outline:'none',width:200}}/>
    </div>
  )
}
export function Btn({ children, onClick, variant='default', size='sm', disabled, className }: {
  children:React.ReactNode; onClick?:()=>void; variant?:'default'|'primary'|'danger'|'ghost'; size?:'sm'|'xs'; disabled?:boolean; className?:string
}) {
  const s: Record<string,React.CSSProperties> = {
    default:{background:'#0b1628',border:'1px solid #1a2d4a',color:'#8fa8cc'},
    primary:{background:'#6c72ff',border:'none',color:'#fff'},
    danger: {background:'rgba(255,77,106,.1)',border:'1px solid rgba(255,77,106,.3)',color:'#ff4d6a'},
    ghost:  {background:'transparent',border:'none',color:'#546a88'},
  }
  const ps = size==='sm'?{padding:'6px 14px',fontSize:12}:{padding:'4px 10px',fontSize:11}
  return <button onClick={onClick} disabled={disabled} style={{...s[variant],...ps,borderRadius:8,fontWeight:600,cursor:disabled?'not-allowed':'pointer',opacity:disabled?.5:1,display:'inline-flex',alignItems:'center',gap:6}}>{children}</button>
}
