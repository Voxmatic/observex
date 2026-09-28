// frontend/src/pages/OnCallPage.tsx
// On-call schedules · escalation policies · alert routing

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { oncall, users as usersApi } from '@/lib/api'
import { PageHeader, Spinner, Btn, EmptyState } from '@/components/shared/Layout'
import { Clock, Users, Shield, Plus, ChevronRight, Check, X } from 'lucide-react'
import { format, formatDistanceToNow, isPast, isFuture } from 'date-fns'
import clsx from 'clsx'
import toast from 'react-hot-toast'

type ActiveTab = 'schedules' | 'policies' | 'routing'

// ── Schedule card ─────────────────────────────────────────────────────────────

function ScheduleCard({ schedule }: { schedule: any }) {
  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="text-sm font-medium text-slate-200">{schedule.name}</div>
          {schedule.description && (
            <div className="text-xs text-slate-500 mt-0.5">{schedule.description}</div>
          )}
        </div>
        <span className="text-[10px] bg-surface-3 text-slate-400 px-2 py-0.5 rounded font-mono">
          {schedule.timezone}
        </span>
      </div>

      {/* Current on-call indicator */}
      <div className="bg-ok/10 border border-ok/20 rounded-lg px-3 py-2 mb-3">
        <div className="text-[10px] text-ok font-medium mb-0.5">Currently on-call</div>
        <div className="text-xs text-slate-300">
          {schedule.current_oncall ?? 'No rotation active'}
        </div>
      </div>

      {/* Next rotation */}
      {schedule.next_rotation && (
        <div className="text-[10px] text-slate-600">
          Next: {schedule.next_rotation} in {schedule.next_in}
        </div>
      )}
    </div>
  )
}

// ── Escalation policy card ────────────────────────────────────────────────────

function PolicyCard({ policy }: { policy: any }) {
  const steps = policy.steps ?? []
  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl p-4">
      <div className="text-sm font-medium text-slate-200 mb-1">{policy.name}</div>
      {policy.description && (
        <div className="text-xs text-slate-500 mb-3">{policy.description}</div>
      )}
      <div className="space-y-2">
        {steps.map((step: any, i: number) => (
          <div key={i} className="flex items-start gap-3">
            <div className="w-5 h-5 rounded-full bg-brand/15 text-brand flex items-center justify-center text-[9px] font-bold flex-shrink-0 mt-0.5">
              {i + 1}
            </div>
            <div className="flex-1">
              <div className="text-xs text-slate-300">
                {step.delay_min === 0 ? 'Immediately' : `After ${step.delay_min}m`}
              </div>
              <div className="text-[10px] text-slate-500 mt-0.5">
                {(step.targets ?? []).map((t: string) =>
                  t.replace('user:', '').replace('schedule:', '📅 ')
                ).join(', ') || 'No targets'}
              </div>
            </div>
          </div>
        ))}
        {steps.length === 0 && (
          <div className="text-xs text-slate-600">No escalation steps defined</div>
        )}
      </div>
    </div>
  )
}

// ── Default data (used when API has no data yet) ──────────────────────────────

const DEFAULT_SCHEDULES = [
  {
    id: 'demo-1',
    name: 'Platform engineering',
    description: '24/7 on-call for production infrastructure',
    timezone: 'UTC',
    current_oncall: 'alice.kim@co.com',
    next_rotation: 'ben.r@co.com',
    next_in: '2d 6h',
  },
  {
    id: 'demo-2',
    name: 'Backend services',
    description: 'Business hours coverage',
    timezone: 'America/New_York',
    current_oncall: 'carol.z@co.com',
    next_rotation: 'alice.kim@co.com',
    next_in: '5d',
  },
]

const DEFAULT_POLICIES = [
  {
    id: 'policy-1',
    name: 'Default escalation',
    description: 'Page on-call → team → manager',
    steps: [
      { delay_min: 0,  targets: ['schedule:demo-1'] },
      { delay_min: 15, targets: ['schedule:demo-2'] },
      { delay_min: 30, targets: ['user:admin@observex.io'] },
    ],
  },
  {
    id: 'policy-2',
    name: 'Critical fast-track',
    description: 'For CRITICAL severity incidents',
    steps: [
      { delay_min: 0, targets: ['schedule:demo-1', 'schedule:demo-2'] },
      { delay_min: 5, targets: ['user:admin@observex.io'] },
    ],
  },
]

const DEFAULT_ROUTES = [
  { id: 'r1', matchers: [{ label: 'severity', op: '=', value: 'CRITICAL' }], policy: 'Critical fast-track', group_wait: '30s', repeat: '1h' },
  { id: 'r2', matchers: [{ label: 'severity', op: '=', value: 'HIGH' }],     policy: 'Default escalation',  group_wait: '2m',  repeat: '4h' },
  { id: 'r3', matchers: [],                                                    policy: 'Default escalation',  group_wait: '5m',  repeat: '4h' },
]

// ── Main page ─────────────────────────────────────────────────────────────────

export default function OnCallPage() {
  const [tab, setTab] = useState<ActiveTab>('schedules')

  const { data: schedulesData, isLoading: schLoading } = useQuery({
    queryKey: ['oncall-schedules'],
    queryFn: () => oncall.schedules(),
    staleTime: 60_000,
  })
  const { data: policiesData, isLoading: polLoading } = useQuery({
    queryKey: ['escalation-policies'],
    queryFn: () => oncall.policies(),
    staleTime: 60_000,
  })

  const schedules = (schedulesData as any)?.schedules ?? DEFAULT_SCHEDULES
  const policies  = (policiesData as any)?.policies  ?? DEFAULT_POLICIES

  const TABS: { id: ActiveTab; label: string; icon: React.ElementType }[] = [
    { id: 'schedules', label: 'On-call schedules', icon: Clock },
    { id: 'policies',  label: 'Escalation policies', icon: Shield },
    { id: 'routing',   label: 'Alert routing', icon: ChevronRight },
  ]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="On-call & Escalation"
        subtitle="Who gets paged · how fast · with what escalation path"
        actions={
          <Btn variant="primary" size="sm">
            <Plus size={13} /> New schedule
          </Btn>
        }
      />

      <div className="flex border-b border-surface-3 px-4 flex-shrink-0">
        {TABS.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)}
            className={clsx('flex items-center gap-1.5 px-3 py-2.5 text-xs font-medium border-b-2 -mb-px transition-colors',
              tab === t.id ? 'border-brand text-brand' : 'border-transparent text-slate-500 hover:text-slate-300')}>
            <t.icon size={12} />{t.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {tab === 'schedules' && (
          <div className="space-y-3">
            {schLoading ? <Spinner /> : schedules.map((s: any) => (
              <ScheduleCard key={s.id} schedule={s} />
            ))}
            {schedules.length === 0 && (
              <EmptyState icon={Clock} title="No schedules"
                description="Create an on-call schedule to start routing alerts to the right people." />
            )}
          </div>
        )}

        {tab === 'policies' && (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {polLoading ? <Spinner /> : policies.map((p: any) => (
              <PolicyCard key={p.id} policy={p} />
            ))}
          </div>
        )}

        {tab === 'routing' && (
          <div className="space-y-3">
            <div className="text-xs text-slate-500 mb-4">
              Routes are evaluated top-to-bottom. The first matching route wins.
              Empty matchers = catch-all default route.
            </div>
            <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
              <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                              bg-surface-2 border-b border-surface-3 px-4 py-2"
                style={{ gridTemplateColumns: '28px 1fr 1fr 80px 80px' }}>
                <span>#</span><span>Matchers</span><span>Escalation policy</span>
                <span>Group wait</span><span>Repeat</span>
              </div>
              {DEFAULT_ROUTES.map((r, i) => (
                <div key={r.id} className="grid items-center px-4 py-3 border-b border-surface-3/50 text-xs hover:bg-surface-2/30"
                  style={{ gridTemplateColumns: '28px 1fr 1fr 80px 80px' }}>
                  <span className="text-slate-600 font-mono">{i + 1}</span>
                  <div>
                    {r.matchers.length === 0 ? (
                      <span className="text-slate-500 italic">catch-all (default)</span>
                    ) : (
                      r.matchers.map((m, mi) => (
                        <span key={mi} className="inline-flex items-center gap-1 mr-1.5">
                          <span className="font-mono text-slate-300">{m.label}</span>
                          <span className="text-slate-600">{m.op}</span>
                          <span className="bg-brand/15 text-brand px-1.5 py-0.5 rounded text-[9px] font-mono">{m.value}</span>
                        </span>
                      ))
                    )}
                  </div>
                  <span className="text-slate-300 font-medium">{r.policy}</span>
                  <span className="text-slate-500 font-mono">{r.group_wait}</span>
                  <span className="text-slate-500 font-mono">{r.repeat}</span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
