import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Spinner } from '@/components/shared/Layout'
import { Book, Plus, Play, Trash2, Share2, Search, Clock } from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import clsx from 'clsx'

const CELL_ICONS: Record<string, string> = { markdown:'📝', metrics:'📊', logs:'📋', traces:'🔍', free_text:'✏️' }

function CellBadge({ type }: { type: string }) {
  const colors: Record<string, string> = { markdown:'bg-slate-700 text-slate-300', metrics:'bg-indigo-500/20 text-indigo-300', logs:'bg-emerald-500/20 text-emerald-300', traces:'bg-purple-500/20 text-purple-300' }
  return (
    <span className={clsx('text-[9px] font-bold px-1.5 py-0.5 rounded uppercase tracking-wide', colors[type] ?? 'bg-slate-700 text-slate-400')}>
      {CELL_ICONS[type]} {type}
    </span>
  )
}

export default function NotebooksPage() {
  const qc = useQueryClient()
  const [activeNb, setActiveNb] = useState<any>(null)
  const [creating, setCreating] = useState(false)
  const [newTitle, setNewTitle] = useState('')
  const [search, setSearch] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['notebooks'],
    queryFn: () => http.get('/api/v1/notebooks').then(r => r.data),
    refetchInterval: 60_000,
  })

  const { data: activeData } = useQuery({
    queryKey: ['notebook', activeNb?.id],
    queryFn: () => http.get(`/api/v1/notebooks/${activeNb.id}`).then(r => r.data),
    enabled: !!activeNb?.id,
  })

  const create = useMutation({
    mutationFn: () => http.post('/api/v1/notebooks', { title: newTitle, cells: [], tags: [] }),
    onSuccess: (data: any) => { qc.invalidateQueries({ queryKey: ['notebooks'] }); setCreating(false); setNewTitle(''); setActiveNb(data) },
  })

  const execute = useMutation({
    mutationFn: ({ cellId, type, query }: any) => http.post(`/api/v1/notebooks/${activeNb.id}/execute`, { cell_id: cellId, type, query }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['notebook', activeNb?.id] }),
  })

  const notebooks: any[] = (data?.notebooks ?? []).filter((n: any) => !search || n.title?.toLowerCase().includes(search.toLowerCase()))

  return (
    <div className="flex h-[calc(100vh-96px)] bg-[#070c18] text-slate-200 gap-4 p-4">
      {/* Sidebar */}
      <div className="w-72 flex-shrink-0 bg-[#111827] border border-[#1e2433] rounded-xl flex flex-col overflow-hidden">
        <div className="p-3 border-b border-[#1e2433]">
          <div className="flex items-center gap-2 bg-[#0d1423] border border-[#1e2433] rounded-lg px-2.5 py-1.5 mb-2">
            <Search size={12} className="text-slate-500" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search notebooks" className="bg-transparent text-[12px] text-slate-300 outline-none flex-1 placeholder-slate-600" />
          </div>
          <button onClick={() => setCreating(true)}
            className="w-full flex items-center justify-center gap-2 bg-[#6366f1] hover:bg-[#4f46e5] text-white text-[12px] font-semibold py-2 rounded-lg transition-colors">
            <Plus size={13} /> New Notebook
          </button>
        </div>

        {creating && (
          <div className="p-3 border-b border-[#1e2433] bg-[#0d1423]">
            <input autoFocus value={newTitle} onChange={e => setNewTitle(e.target.value)} onKeyDown={e => e.key === 'Enter' && create.mutate()}
              placeholder="Notebook title..." className="w-full bg-[#111827] border border-[#1e2433] rounded-lg px-2.5 py-1.5 text-[12px] text-slate-300 outline-none mb-2" />
            <div className="flex gap-2">
              <button onClick={() => create.mutate()} disabled={!newTitle} className="flex-1 bg-[#6366f1] text-white text-[11px] py-1.5 rounded-lg font-semibold">Create</button>
              <button onClick={() => setCreating(false)} className="flex-1 bg-[#1e2433] text-slate-400 text-[11px] py-1.5 rounded-lg">Cancel</button>
            </div>
          </div>
        )}

        <div className="flex-1 overflow-y-auto py-2">
          {isLoading ? <div className="flex justify-center py-8"><Spinner /></div> :
            notebooks.map((nb: any) => (
              <div key={nb.id} onClick={() => setActiveNb(nb)}
                className={clsx('px-3 py-2.5 mx-2 rounded-lg cursor-pointer mb-1 transition-colors', activeNb?.id === nb.id ? 'bg-[#6366f1]/20 border border-[#6366f1]/30' : 'hover:bg-[#1a2235]')}>
                <div className="flex items-start gap-2">
                  <Book size={13} className={activeNb?.id === nb.id ? 'text-[#818cf8]' : 'text-slate-500'} />
                  <div className="flex-1 min-w-0">
                    <div className={clsx('text-[12px] font-medium truncate', activeNb?.id === nb.id ? 'text-[#818cf8]' : 'text-slate-300')}>{nb.title}</div>
                    <div className="flex items-center gap-1 mt-0.5">
                      <Clock size={9} className="text-slate-600" />
                      <span className="text-[10px] text-slate-600">{nb.updated_at ? formatDistanceToNow(new Date(nb.updated_at), { addSuffix: true }) : ''}</span>
                    </div>
                    {nb.tags?.length > 0 && (
                      <div className="flex gap-1 mt-1 flex-wrap">
                        {nb.tags.slice(0, 3).map((t: string) => (
                          <span key={t} className="text-[9px] bg-[#1e2433] text-slate-500 px-1 py-0.5 rounded">{t}</span>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ))
          }
        </div>
      </div>

      {/* Main notebook area */}
      {activeNb ? (
        <div className="flex-1 bg-[#111827] border border-[#1e2433] rounded-xl flex flex-col overflow-hidden">
          {/* Notebook header */}
          <div className="flex items-center gap-3 px-5 py-3 border-b border-[#1e2433]">
            <h2 className="text-[15px] font-bold text-slate-100 flex-1">{activeData?.title ?? activeNb.title}</h2>
            <button className="flex items-center gap-1.5 text-[11px] text-slate-400 border border-[#1e2433] px-3 py-1.5 rounded-lg hover:border-[#2a3447]">
              <Share2 size={12} /> Share
            </button>
            <button className="flex items-center gap-1.5 text-[11px] bg-[#6366f1] text-white px-3 py-1.5 rounded-lg font-semibold hover:bg-[#4f46e5]">
              <Play size={12} /> Run All
            </button>
          </div>

          {/* Cells */}
          <div className="flex-1 overflow-y-auto p-5 space-y-4">
            {(activeData?.cells ?? []).map((cell: any) => (
              <div key={cell.id} className="bg-[#0d1423] border border-[#1e2433] rounded-xl overflow-hidden">
                <div className="flex items-center justify-between px-4 py-2 border-b border-[#1e2433] bg-[#080d1b]">
                  <CellBadge type={cell.type} />
                  <button onClick={() => execute.mutate({ cellId: cell.id, type: cell.type, query: cell.query })}
                    className="flex items-center gap-1 text-[10px] text-[#818cf8] hover:text-[#6366f1]">
                    <Play size={10} /> Run
                  </button>
                </div>
                <div className="p-4">
                  {cell.type === 'markdown' ? (
                    <div className="text-[13px] text-slate-300 leading-relaxed whitespace-pre-wrap">{cell.content}</div>
                  ) : (
                    <>
                      {cell.query && (
                        <div className="font-mono text-[11px] text-[#818cf8] bg-[#080d1b] rounded-lg px-3 py-2 mb-3 border border-[#1e2433] overflow-x-auto whitespace-nowrap">{cell.query}</div>
                      )}
                      {cell.content && <div className="text-[12px] text-slate-400">{cell.content}</div>}
                    </>
                  )}
                </div>
              </div>
            ))}

            {/* Add cell buttons */}
            <div className="flex gap-2 pt-2">
              {['markdown', 'metrics', 'logs', 'traces'].map(type => (
                <button key={type} className="flex items-center gap-1.5 text-[11px] text-slate-500 border border-dashed border-[#2a3447] px-3 py-2 rounded-lg hover:border-[#6366f1] hover:text-slate-300 transition-colors">
                  <Plus size={11} /> {CELL_ICONS[type]} {type}
                </button>
              ))}
            </div>
          </div>
        </div>
      ) : (
        <div className="flex-1 bg-[#111827] border border-[#1e2433] rounded-xl flex flex-col items-center justify-center text-slate-500">
          <Book size={48} className="mb-4 opacity-30" />
          <p className="text-base font-medium mb-2">Select a notebook or create one</p>
          <p className="text-sm text-slate-600">Combine metrics, logs, traces and markdown in one investigation workspace</p>
        </div>
      )}
    </div>
  )
}
