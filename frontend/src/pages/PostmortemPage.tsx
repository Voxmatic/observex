// frontend/src/pages/PostmortemPage.tsx  (M9)
// Structured postmortem builder — 5-whys, timeline, action items, RCA, learnings
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { problems } from '@/lib/api'
import { PageHeader, Btn, EmptyState } from '@/components/shared/Layout'
import { FileText, Plus, Clock, CheckCircle, AlertTriangle, Users, Target } from 'lucide-react'
import { format } from 'date-fns'
import clsx from 'clsx'

interface ActionItem { id: string; what: string; who: string; by: string; done: boolean }
interface TimelineEntry { time: string; event: string; who: string }
interface WhyEntry { level: number; why: string }

interface Postmortem {
  id: string
  incidentId: string
  title: string
  severity: string
  detectedAt: string
  resolvedAt: string
  duration: string
  impact: string
  summary: string
  timeline: TimelineEntry[]
  rootCause: string
  whys: WhyEntry[]
  actionItems: ActionItem[]
  learnings: string
  status: 'draft' | 'review' | 'published'
}

const EMPTY_PM: Postmortem = {
  id: '', incidentId: '', title: '', severity: 'HIGH',
  detectedAt: '', resolvedAt: '', duration: '', impact: '',
  summary: '', timeline: [], rootCause: '', whys: [],
  actionItems: [], learnings: '', status: 'draft',
}

function Section({ title, icon: Icon, children }: { title: string; icon: React.ElementType; children: React.ReactNode }) {
  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden mb-4">
      <div className="flex items-center gap-2 px-4 py-3 border-b border-surface-3 bg-surface-2">
        <Icon size={13} className="text-slate-500"/>
        <span className="text-xs font-semibold text-slate-300">{title}</span>
      </div>
      <div className="p-4">{children}</div>
    </div>
  )
}

function PostmortemEditor({ pm, onChange, onSave }: {
  pm: Postmortem; onChange: (p: Postmortem) => void; onSave: () => void
}) {
  const upd = (k: keyof Postmortem, v: any) => onChange({ ...pm, [k]: v })

  const addWhy = () => upd('whys', [...pm.whys, { level: pm.whys.length + 1, why: '' }])
  const addTimeline = () => upd('timeline', [...pm.timeline, { time: format(new Date(), "HH:mm"), event: '', who: '' }])
  const addAction = () => upd('actionItems', [...pm.actionItems, { id: Date.now().toString(), what: '', who: '', by: '', done: false }])

  const input = (val: string, cb: (v:string)=>void, placeholder='', multi=false) =>
    multi
      ? <textarea value={val} onChange={e=>cb(e.target.value)} placeholder={placeholder} rows={3}
          className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 font-mono focus:outline-none focus:border-brand/60 resize-none"/>
      : <input value={val} onChange={e=>cb(e.target.value)} placeholder={placeholder}
          className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60"/>

  return (
    <div>
      {/* Header */}
      <Section title="Incident overview" icon={AlertTriangle}>
        <div className="grid grid-cols-2 gap-3 mb-3">
          <div><div className="text-[10px] text-slate-500 mb-1">Title</div>{input(pm.title, v=>upd('title',v), 'Descriptive incident title')}</div>
          <div><div className="text-[10px] text-slate-500 mb-1">Severity</div>
            <select value={pm.severity} onChange={e=>upd('severity',e.target.value)}
              className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
              <option>CRITICAL</option><option>HIGH</option><option>MEDIUM</option><option>LOW</option>
            </select>
          </div>
        </div>
        <div className="grid grid-cols-3 gap-3 mb-3">
          <div><div className="text-[10px] text-slate-500 mb-1">Detected at</div>{input(pm.detectedAt, v=>upd('detectedAt',v), '2024-03-23 14:10 UTC')}</div>
          <div><div className="text-[10px] text-slate-500 mb-1">Resolved at</div>{input(pm.resolvedAt, v=>upd('resolvedAt',v), '2024-03-23 14:28 UTC')}</div>
          <div><div className="text-[10px] text-slate-500 mb-1">Duration</div>{input(pm.duration, v=>upd('duration',v), '18 minutes')}</div>
        </div>
        <div><div className="text-[10px] text-slate-500 mb-1">Customer impact</div>{input(pm.impact, v=>upd('impact',v), 'e.g. Payment checkout unavailable for ~15% of users', true)}</div>
        <div className="mt-3"><div className="text-[10px] text-slate-500 mb-1">Executive summary</div>{input(pm.summary, v=>upd('summary',v), 'Brief description of what happened, why, and what we did', true)}</div>
      </Section>

      {/* Timeline */}
      <Section title="Incident timeline" icon={Clock}>
        {pm.timeline.map((t, i) => (
          <div key={i} className="flex gap-2 mb-2 items-start">
            <input value={t.time} onChange={e=>{const tl=[...pm.timeline];tl[i]={...tl[i],time:e.target.value};upd('timeline',tl)}}
              className="w-24 flex-shrink-0 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded font-mono text-slate-300 focus:outline-none"/>
            <input value={t.event} placeholder="What happened?" onChange={e=>{const tl=[...pm.timeline];tl[i]={...tl[i],event:e.target.value};upd('timeline',tl)}}
              className="flex-1 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none"/>
            <input value={t.who} placeholder="Who?" onChange={e=>{const tl=[...pm.timeline];tl[i]={...tl[i],who:e.target.value};upd('timeline',tl)}}
              className="w-28 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none"/>
            <button onClick={()=>upd('timeline',pm.timeline.filter((_,j)=>j!==i))} className="text-slate-600 hover:text-crit px-1">×</button>
          </div>
        ))}
        <Btn size="xs" onClick={addTimeline}><Plus size={11}/> Add entry</Btn>
      </Section>

      {/* 5-Whys */}
      <Section title="5-Whys root cause analysis" icon={Target}>
        <div className="mb-3"><div className="text-[10px] text-slate-500 mb-1">Root cause statement</div>{input(pm.rootCause, v=>upd('rootCause',v), 'The primary root cause of this incident was…', true)}</div>
        {pm.whys.map((w, i) => (
          <div key={i} className="flex gap-2 mb-2 items-start">
            <div className="w-7 h-7 rounded-full bg-brand/15 text-brand text-xs font-bold flex items-center justify-center flex-shrink-0 mt-0.5">{w.level}</div>
            <input value={w.why} placeholder={`Why #${w.level}?`} onChange={e=>{const ws=[...pm.whys];ws[i]={...ws[i],why:e.target.value};upd('whys',ws)}}
              className="flex-1 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none"/>
            <button onClick={()=>upd('whys',pm.whys.filter((_,j)=>j!==i))} className="text-slate-600 hover:text-crit px-1">×</button>
          </div>
        ))}
        <Btn size="xs" onClick={addWhy} disabled={pm.whys.length >= 5}><Plus size={11}/> Add why</Btn>
      </Section>

      {/* Action items */}
      <Section title="Action items" icon={CheckCircle}>
        {pm.actionItems.map((a, i) => (
          <div key={a.id} className="flex gap-2 mb-2 items-start">
            <input type="checkbox" checked={a.done} onChange={e=>{const as=[...pm.actionItems];as[i]={...as[i],done:e.target.checked};upd('actionItems',as)}}
              className="mt-2 flex-shrink-0"/>
            <input value={a.what} placeholder="What needs to be done?" onChange={e=>{const as=[...pm.actionItems];as[i]={...as[i],what:e.target.value};upd('actionItems',as)}}
              className={clsx('flex-1 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none',a.done&&'line-through opacity-50')}/>
            <input value={a.who} placeholder="Owner" onChange={e=>{const as=[...pm.actionItems];as[i]={...as[i],who:e.target.value};upd('actionItems',as)}}
              className="w-28 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none"/>
            <input value={a.by} placeholder="Due date" onChange={e=>{const as=[...pm.actionItems];as[i]={...as[i],by:e.target.value};upd('actionItems',as)}}
              className="w-24 px-2 py-1.5 text-xs bg-surface-2 border border-surface-3 rounded text-slate-300 focus:outline-none"/>
            <button onClick={()=>upd('actionItems',pm.actionItems.filter((_,j)=>j!==i))} className="text-slate-600 hover:text-crit px-1">×</button>
          </div>
        ))}
        <Btn size="xs" onClick={addAction}><Plus size={11}/> Add action</Btn>
      </Section>

      {/* Learnings */}
      <Section title="Learnings & improvements" icon={Users}>
        {input(pm.learnings, v=>upd('learnings',v), 'What did we learn? What can we improve to prevent this?', true)}
      </Section>

      <div className="flex gap-2 justify-end">
        <Btn onClick={()=>upd('status','draft')}>Save draft</Btn>
        <Btn onClick={()=>upd('status','review')}>Submit for review</Btn>
        <Btn variant="primary" onClick={onSave}><CheckCircle size={13}/> Publish</Btn>
      </div>
    </div>
  )
}

export default function PostmortemPage() {
  const [editing, setEditing] = useState<Postmortem | null>(null)
  const [pms, setPMs] = useState<Postmortem[]>([])

  const { data: probData } = useQuery({
    queryKey: ['problems-resolved'],
    queryFn: () => problems.list({ status: 'RESOLVED', limit: 10 }),
    staleTime: 60_000,
  })

  const resolvedProbs = probData?.problems ?? []

  if (editing) return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-3 border-b border-surface-3 flex-shrink-0">
        <button onClick={()=>setEditing(null)} className="text-xs text-slate-500 hover:text-slate-300">← Postmortems</button>
        <span className="text-slate-700">/</span>
        <span className="text-sm font-semibold text-slate-200">{editing.title || 'New postmortem'}</span>
        <span className={clsx('text-[10px] px-2 py-0.5 rounded font-bold ml-2',
          editing.status==='published'?'bg-ok/15 text-ok':editing.status==='review'?'bg-brand/15 text-brand':'bg-surface-3 text-slate-400')}>
          {editing.status}
        </span>
      </div>
      <div className="flex-1 overflow-y-auto p-4">
        <PostmortemEditor pm={editing} onChange={setEditing}
          onSave={()=>{setPMs(prev=>[...prev.filter(p=>p.id!==editing.id),{...editing,id:editing.id||Date.now().toString(),status:'published'}]);setEditing(null)}}/>
      </div>
    </div>
  )

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader title="Postmortems" subtitle="Structured RCA · 5-whys · action items · learnings"
        actions={<Btn variant="primary" onClick={()=>setEditing({...EMPTY_PM,id:Date.now().toString()})}><Plus size={13}/> New postmortem</Btn>}/>
      <div className="flex-1 overflow-y-auto p-4">
        {pms.length === 0 && resolvedProbs.length === 0 ? (
          <EmptyState icon={FileText} title="No postmortems yet"
            description="Create a postmortem for any resolved incident to capture learnings and action items." />
        ) : (
          <div className="space-y-3">
            {pms.map(pm => (
              <div key={pm.id} className="bg-surface-1 border border-surface-3 rounded-xl p-4 cursor-pointer hover:border-brand/40 transition-all"
                onClick={()=>setEditing(pm)}>
                <div className="flex items-center justify-between mb-2">
                  <span className="text-sm font-semibold text-slate-200">{pm.title}</span>
                  <span className={clsx('text-[10px] px-2 py-0.5 rounded font-bold',
                    pm.status==='published'?'bg-ok/15 text-ok':pm.status==='review'?'bg-brand/15 text-brand':'bg-surface-3 text-slate-400')}>
                    {pm.status}
                  </span>
                </div>
                <div className="flex gap-4 text-[10px] text-slate-600">
                  <span>{pm.severity}</span><span>{pm.duration}</span>
                  <span>{pm.actionItems.filter(a=>!a.done).length} open actions</span>
                </div>
              </div>
            ))}
            {resolvedProbs.length > 0 && (
              <div>
                <div className="text-xs font-medium text-slate-500 mb-3">Resolved incidents without postmortem</div>
                {resolvedProbs.map((p:any) => (
                  <div key={p.id} className="bg-surface-1 border border-warn/20 rounded-xl p-4 mb-2 cursor-pointer hover:border-brand/40"
                    onClick={()=>setEditing({...EMPTY_PM,id:Date.now().toString(),incidentId:p.id,title:p.title,severity:p.severity,detectedAt:p.detected_at})}>
                    <div className="flex items-center gap-2 mb-1">
                      <AlertTriangle size={12} className="text-warn"/>
                      <span className="text-xs font-medium text-slate-200">{p.title}</span>
                      <span className="ml-auto text-[10px] text-brand">Start postmortem →</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
