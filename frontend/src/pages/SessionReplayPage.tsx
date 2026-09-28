// frontend/src/pages/SessionReplayPage.tsx
// Session Replay — playback recorded user sessions, rage clicks, JS errors
// Requires the ObserveX RUM SDK (observex-rum.js) with recording=true

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { sessionReplay, type SessionReplay } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import {
  Monitor, AlertTriangle, MousePointerClick, Clock, Globe,
  Play, Filter, Search, ChevronRight, Activity, Smartphone, Chrome
} from 'lucide-react'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'

// ── Helpers ───────────────────────────────────────────────────────────────────

function fmtDuration(ms: number) {
  if (ms < 1000) return `${ms}ms`
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function DeviceIcon({ device }: { device?: string }) {
  if (device?.toLowerCase().includes('mobile') || device?.toLowerCase().includes('android') || device?.toLowerCase().includes('iphone'))
    return <Smartphone size={14} className="text-slate-400" />
  return <Monitor size={14} className="text-slate-400" />
}

function BrowserIcon({ browser }: { browser?: string }) {
  const b = browser?.toLowerCase() ?? ''
  if (b.includes('chrome') || b.includes('chromium')) return <Chrome size={14} className="text-slate-400" />
  return <Globe size={14} className="text-slate-400" />
}

// ── Session row ───────────────────────────────────────────────────────────────

function SessionRow({ session, onClick }: { session: SessionReplay; onClick: () => void }) {
  const hasErrors = session.error_count > 0
  const hasRageClicks = session.rage_click_count > 0

  return (
    <tr
      onClick={onClick}
      className="border-b border-surface-2 hover:bg-surface-1/50 cursor-pointer transition-colors"
    >
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <Play size={13} className="text-brand flex-shrink-0" />
          <span className="font-mono text-[11px] text-slate-400">{session.session_id.slice(0, 16)}…</span>
        </div>
      </td>
      <td className="px-4 py-3">
        <span className="text-sm text-slate-300 max-w-[280px] truncate block" title={session.url}>
          {session.url}
        </span>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <DeviceIcon device={session.device} />
          <BrowserIcon browser={session.browser} />
          <span className="text-xs text-slate-400">{session.browser ?? '—'}</span>
        </div>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-1 text-xs text-slate-400">
          <Clock size={11} />
          {fmtDuration(session.duration_ms)}
        </div>
      </td>
      <td className="px-4 py-3">
        <span className="text-xs text-slate-400">{session.event_count}</span>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          {hasErrors && (
            <span className="flex items-center gap-1 text-[11px] font-medium text-crit">
              <AlertTriangle size={11} /> {session.error_count}
            </span>
          )}
          {hasRageClicks && (
            <span className="flex items-center gap-1 text-[11px] font-medium text-warn">
              <MousePointerClick size={11} /> {session.rage_click_count}
            </span>
          )}
          {!hasErrors && !hasRageClicks && (
            <span className="text-[11px] text-ok">Clean</span>
          )}
        </div>
      </td>
      <td className="px-4 py-3">
        <span className="text-xs text-slate-500">
          {formatDistanceToNow(new Date(session.started_at), { addSuffix: true })}
        </span>
      </td>
      <td className="px-4 py-3 text-right">
        <ChevronRight size={14} className="text-slate-500 ml-auto" />
      </td>
    </tr>
  )
}

// ── Session player ─────────────────────────────────────────────────────────────

function SessionPlayer({ session, onClose }: { session: SessionReplay; onClose: () => void }) {
  const { data: eventsData, isLoading } = useQuery({
    queryKey: ['replay-events', session.id],
    queryFn: () => sessionReplay.events(session.id),
  })

  const events = eventsData?.events ?? []

  const eventTypeColor: Record<string, string> = {
    click:       'text-brand',
    rage_click:  'text-warn',
    input:       'text-slate-400',
    scroll:      'text-slate-500',
    navigation:  'text-ok',
    error:       'text-crit',
    xhr:         'text-purple-400',
    console:     'text-slate-400',
  }

  return (
    <div className="fixed inset-0 z-50 bg-surface-0/90 backdrop-blur-sm flex items-start justify-center pt-10 px-4">
      <div className="w-full max-w-5xl bg-surface-1 rounded-xl border border-surface-3 shadow-2xl overflow-hidden">

        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-surface-2">
          <div className="flex items-center gap-3">
            <Play size={15} className="text-brand" />
            <span className="text-sm font-medium text-slate-200">Session Replay</span>
            <span className="font-mono text-[11px] text-slate-500 bg-surface-2 px-2 py-0.5 rounded">
              {session.session_id.slice(0, 20)}
            </span>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200 text-lg leading-none">×</button>
        </div>

        <div className="flex h-[600px]">

          {/* Left: canvas player area */}
          <div className="flex-1 flex flex-col bg-surface-0 border-r border-surface-2">
            {/* Viewport simulation */}
            <div className="flex-1 relative overflow-hidden flex items-center justify-center bg-slate-900/50">
              <div className="border border-surface-3 rounded bg-white/5 flex items-center justify-center"
                   style={{ width: 560, height: 400 }}>
                {/* In production: rrweb player renders here */}
                <div className="text-center px-8">
                  <Monitor size={40} className="mx-auto text-slate-600 mb-3" />
                  <p className="text-sm text-slate-500 font-medium">Session Playback</p>
                  <p className="text-xs text-slate-600 mt-1">{session.url}</p>
                  <p className="text-xs text-slate-700 mt-3 max-w-xs mx-auto leading-relaxed">
                    DOM recording playback requires the rrweb player library.
                    The SDK captures all DOM mutations, mouse movements, and interactions.
                  </p>
                  <div className="mt-4 grid grid-cols-3 gap-3 text-center">
                    <div>
                      <div className="text-lg font-bold text-slate-300">{session.event_count}</div>
                      <div className="text-[10px] text-slate-500">events</div>
                    </div>
                    <div>
                      <div className="text-lg font-bold text-crit">{session.error_count}</div>
                      <div className="text-[10px] text-slate-500">errors</div>
                    </div>
                    <div>
                      <div className="text-lg font-bold text-warn">{session.rage_click_count}</div>
                      <div className="text-[10px] text-slate-500">rage clicks</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {/* Timeline bar */}
            <div className="px-4 py-3 border-t border-surface-2">
              <div className="flex items-center gap-2 text-xs text-slate-500 mb-2">
                <span>{format(new Date(session.started_at), 'HH:mm:ss')}</span>
                <div className="flex-1 h-1 bg-surface-2 rounded-full relative">
                  {/* Error markers */}
                  {events.filter(e => e.type === 'error').map((e, i) => (
                    <div key={i}
                      className="absolute w-1 h-1 bg-crit rounded-full top-0"
                      style={{ left: `${(e.offset_ms / session.duration_ms) * 100}%` }}
                    />
                  ))}
                  {/* Rage click markers */}
                  {events.filter(e => e.type === 'rage_click').map((e, i) => (
                    <div key={i}
                      className="absolute w-1 h-1 bg-warn rounded-full top-0"
                      style={{ left: `${(e.offset_ms / session.duration_ms) * 100}%` }}
                    />
                  ))}
                </div>
                <span>{fmtDuration(session.duration_ms)}</span>
              </div>
              <div className="flex items-center gap-3">
                <button className="flex items-center gap-1.5 px-3 py-1 bg-brand/20 text-brand rounded text-xs font-medium hover:bg-brand/30 transition-colors">
                  <Play size={10} /> Play
                </button>
                <div className="flex items-center gap-2 text-xs text-slate-500">
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-crit inline-block" />JS Error</span>
                  <span className="flex items-center gap-1"><span className="w-2 h-2 rounded-full bg-warn inline-block" />Rage Click</span>
                </div>
              </div>
            </div>
          </div>

          {/* Right: event log */}
          <div className="w-72 flex flex-col">
            <div className="px-4 py-2 border-b border-surface-2 text-xs font-medium text-slate-400 uppercase tracking-wide">
              Events ({events.length})
            </div>
            <div className="flex-1 overflow-y-auto text-xs">
              {isLoading && <div className="p-4 text-center"><Spinner /></div>}
              {!isLoading && events.length === 0 && (
                <div className="p-4 text-slate-500 text-center">No events recorded</div>
              )}
              {events.map((event, i) => (
                <div key={i}
                  className="flex items-start gap-2 px-3 py-1.5 border-b border-surface-2/50 hover:bg-surface-2/30 cursor-pointer">
                  <span className="font-mono text-[10px] text-slate-600 flex-shrink-0 mt-0.5">
                    +{fmtDuration(event.offset_ms ?? 0)}
                  </span>
                  <div className="min-w-0">
                    <span className={clsx('font-medium capitalize', eventTypeColor[event.type] ?? 'text-slate-400')}>
                      {event.type?.replace('_', ' ')}
                    </span>
                    {event.target && (
                      <span className="text-slate-500 ml-1 font-mono truncate block" title={event.target}>
                        {event.target.slice(0, 30)}
                      </span>
                    )}
                    {event.message && (
                      <span className="text-crit block truncate" title={event.message}>
                        {event.message.slice(0, 40)}
                      </span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>

        </div>
      </div>
    </div>
  )
}

// ── Stats bar ─────────────────────────────────────────────────────────────────

function StatCard({ label, value, sub, icon: Icon, color = 'text-slate-200' }: {
  label: string; value: string | number; sub?: string
  icon?: React.ElementType
  color?: string
}) {
  return (
    <div className="bg-surface-1 border border-surface-2 rounded-lg p-4">
      <div className="flex items-center justify-between mb-1">
        <span className="text-xs text-slate-500">{label}</span>
        <Icon size={14} className="text-slate-500" />
      </div>
      <div className={clsx('text-2xl font-bold', color)}>{value}</div>
      {sub && <div className="text-xs text-slate-500 mt-0.5">{sub}</div>}
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function SessionReplayPage() {
  const [selected, setSelected] = useState<SessionReplay | null>(null)
  const [search, setSearch] = useState('')
  const [filterErrors, setFilterErrors] = useState(false)
  const [page, setPage] = useState(0)
  const PAGE_SIZE = 25

  const { data, isLoading, error } = useQuery({
    queryKey: ['session-replay', page, filterErrors, search],
    queryFn: () => sessionReplay.list({
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
      url: search || undefined,
      has_errors: filterErrors || undefined,
    }),
    refetchInterval: 30_000,
  })

  const sessions: SessionReplay[] = data?.sessions ?? []
  const total: number = data?.total ?? 0

  // Summary stats
  const totalErrors      = sessions.reduce((s, r) => s + r.error_count, 0)
  const totalRageClicks  = sessions.reduce((s, r) => s + r.rage_click_count, 0)
  const avgDuration      = sessions.length
    ? Math.round(sessions.reduce((s, r) => s + r.duration_ms, 0) / sessions.length)
    : 0

  return (
    <div className="space-y-5">
      <PageHeader
        title="Session Replay"
        subtitle="Recorded user sessions — playback DOM events, rage clicks, JS errors"
        >
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-500">SDK:</span>
          <code className="text-xs text-ok bg-surface-2 px-2 py-0.5 rounded font-mono">
            new ObserveX({'{'} recording: true {'}'})
          </code>
        </div>
      </PageHeader>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4">
        <StatCard label="Total sessions"  value={total.toLocaleString()}     />
        <StatCard label="JS errors"       value={totalErrors}                color="text-crit" />
        <StatCard label="Rage clicks"     value={totalRageClicks}            color="text-warn" />
        <StatCard label="Avg duration"    value={fmtDuration(avgDuration)}  />
      </div>

      {/* Filters */}
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-sm">
          <Search size={13} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            value={search}
            onChange={e => { setSearch(e.target.value); setPage(0) }}
            placeholder="Filter by URL…"
            className="w-full pl-8 pr-3 py-2 text-sm bg-surface-1 border border-surface-2 rounded-lg
                       text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50"
          />
        </div>
        <button
          onClick={() => { setFilterErrors(f => !f); setPage(0) }}
          className={clsx(
            'flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium border transition-colors',
            filterErrors
              ? 'bg-crit/10 border-crit/30 text-crit'
              : 'bg-surface-1 border-surface-2 text-slate-400 hover:text-slate-200'
          )}
        >
          <AlertTriangle size={12} />
          With errors only
        </button>
        <div className="text-xs text-slate-500 ml-auto">
          {total.toLocaleString()} sessions
        </div>
      </div>

      {/* Table */}
      <div className="bg-surface-1 border border-surface-2 rounded-xl overflow-hidden">
        {isLoading && <div className="p-12 text-center"><Spinner /></div>}
        {error && (
          <EmptyState
            icon={AlertTriangle}
            title="Could not load sessions"
            description="Make sure the RUM SDK is installed with recording enabled."
          />
        )}
        {!isLoading && !error && sessions.length === 0 && (
          <EmptyState
            icon={Play}
            title="No sessions recorded yet"
            description={
              filterErrors
                ? 'No sessions with errors in the current time window.'
                : 'Add the ObserveX RUM SDK with recording: true to start capturing sessions.'
            }
          />
        )}
        {!isLoading && sessions.length > 0 && (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-surface-2 text-xs font-medium text-slate-500 uppercase tracking-wide">
                <th className="px-4 py-2 text-left">Session</th>
                <th className="px-4 py-2 text-left">URL</th>
                <th className="px-4 py-2 text-left">Device</th>
                <th className="px-4 py-2 text-left">Duration</th>
                <th className="px-4 py-2 text-left">Events</th>
                <th className="px-4 py-2 text-left">Issues</th>
                <th className="px-4 py-2 text-left">When</th>
                <th className="px-4 py-2 text-left" />
              </tr>
            </thead>
            <tbody>
              {sessions.map(s => (
                <SessionRow key={s.id} session={s} onClick={() => setSelected(s)} />
              ))}
            </tbody>
          </table>
        )}
        {/* Pagination */}
        {total > PAGE_SIZE && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-surface-2">
            <span className="text-xs text-slate-500">
              Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of {total.toLocaleString()}
            </span>
            <div className="flex items-center gap-2">
              <button
                disabled={page === 0}
                onClick={() => setPage(p => p - 1)}
                className="px-3 py-1 text-xs bg-surface-2 rounded disabled:opacity-40 hover:bg-surface-3"
              >← Prev</button>
              <button
                disabled={(page + 1) * PAGE_SIZE >= total}
                onClick={() => setPage(p => p + 1)}
                className="px-3 py-1 text-xs bg-surface-2 rounded disabled:opacity-40 hover:bg-surface-3"
              >Next →</button>
            </div>
          </div>
        )}
      </div>

      {/* Setup instructions when empty */}
      {!isLoading && sessions.length === 0 && !filterErrors && (
        <div className="bg-surface-1 border border-surface-2 rounded-xl p-6">
          <h3 className="text-sm font-medium text-slate-200 mb-3 flex items-center gap-2">
            <Activity size={14} className="text-brand" />
            Enable Session Recording
          </h3>
          <div className="space-y-3 text-sm text-slate-400">
            <div className="bg-surface-0 rounded-lg p-4 font-mono text-xs text-slate-300">
              <div className="text-slate-500 mb-1">// Add to your app entry point</div>
              <div>{'import { ObserveX } from \'observex-rum\''}</div>
              <div className="mt-1">{'new ObserveX({'}</div>
              <div className="ml-4">{'ingestorURL: "https://your-observex/ingest",'}</div>
              <div className="ml-4 text-ok">{'recording: true,         // enable session replay'}</div>
              <div className="ml-4">{'sampleRate: 0.1,         // record 10% of sessions'}</div>
              <div className="ml-4">{'maskAllInputs: true,     // GDPR: mask sensitive fields'}</div>
              <div>{'})'}
              </div>
            </div>
            <p>Sessions are recorded using a lightweight DOM snapshot algorithm (similar to rrweb).
              All PII is masked by default. Storage: ~50KB per minute of recording.</p>
          </div>
        </div>
      )}

      {/* Player modal */}
      {selected && <SessionPlayer session={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}
