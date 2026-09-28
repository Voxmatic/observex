import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { users as usersApi, teams as teamsApi, org as orgApi, apiKeys, audit } from '@/lib/api'
import { PageHeader, Btn, Spinner } from '@/components/shared/Layout'
import { Users, Shield, Key, Globe, Bell, Trash2, Plus, X, Check, Copy } from 'lucide-react'
import { useAuth } from '@/store/auth'
import { formatDistanceToNow, format } from 'date-fns'
import clsx from 'clsx'
import toast from 'react-hot-toast'

type Tab = 'users' | 'teams' | 'apikeys' | 'namespaces' | 'audit' | 'org'

const TABS: { id: Tab; icon: React.ElementType; label: string }[] = [
  { id: 'org',        icon: Globe,   label: 'Organisation' },
  { id: 'users',      icon: Users,   label: 'Users' },
  { id: 'teams',      icon: Shield,  label: 'Teams' },
  { id: 'apikeys',    icon: Key,     label: 'API Keys' },
  { id: 'namespaces', icon: Globe,   label: 'Namespaces' },
  { id: 'audit',      icon: Bell,    label: 'Audit log' },
]

// ── Org tab ─────────────────────────────────────────────────────────────────

function OrgTab() {
  const { org } = useAuth()
  if (!org) return null
  return (
    <div className="space-y-4 max-w-xl">
      <div className="bg-surface-2 rounded-xl p-4 space-y-3">
        {[
          ['Name', org.name],
          ['Display name', org.display_name],
          ['Plan', org.plan],
          ['Users', `${org.user_count ?? '?'} / ${org.max_users === 0 ? '∞' : org.max_users}`],
          ['Teams', `${org.team_count ?? '?'} / ${org.max_teams === 0 ? '∞' : org.max_teams}`],
        ].map(([k, v]) => (
          <div key={k} className="flex items-center justify-between py-1.5 border-b border-surface-3 last:border-0">
            <span className="text-xs text-slate-500">{k}</span>
            <span className="text-sm text-slate-300 capitalize">{v}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Users tab ────────────────────────────────────────────────────────────────

function UsersTab() {
  const qc = useQueryClient()
  const [showInvite, setShowInvite] = useState(false)
  const [inviteEmail, setInviteEmail] = useState('')
  const [inviteRole, setInviteRole]   = useState('viewer')
  const { data, isLoading } = useQuery({ queryKey: ['users-list'], queryFn: () => usersApi.list() })
  const invite = useMutation({
    mutationFn: () => orgApi.sendInvitation({ email: inviteEmail, role: inviteRole }),
    onSuccess: () => { toast.success(`Invitation sent to ${inviteEmail}`); setShowInvite(false); setInviteEmail('') },
    onError: () => toast.error('Failed to send invitation'),
  })
  const del = useMutation({
    mutationFn: (id: string) => usersApi.delete(id),
    onSuccess: () => { toast.success('User deleted'); qc.invalidateQueries({ queryKey: ['users-list'] }) },
  })

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Btn variant="primary" onClick={() => setShowInvite(true)}><Plus size={13} /> Invite user</Btn>
      </div>

      {isLoading && <Spinner />}

      <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
        <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600 bg-surface-2 border-b border-surface-3 px-4 py-2" style={{ gridTemplateColumns: '1fr 120px 80px 100px 40px' }}>
          <span>User</span><span>Role</span><span>Status</span><span>Last login</span><span />
        </div>
        {(data?.users ?? []).map(u => (
          <div key={u.id} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 hover:bg-surface-2/30 text-sm" style={{ gridTemplateColumns: '1fr 120px 80px 100px 40px' }}>
            <div>
              <div className="text-slate-200">{u.name}</div>
              <div className="text-xs text-slate-600">{u.email}</div>
            </div>
            <span className="text-xs text-slate-400 capitalize">{u.role}</span>
            <span className={clsx('text-xs', u.is_active ? 'text-ok' : 'text-slate-500')}>{u.is_active ? 'Active' : 'Inactive'}</span>
            <span className="text-xs text-slate-600">{u.last_login_at ? formatDistanceToNow(new Date(u.last_login_at), { addSuffix: true }) : '—'}</span>
            <button onClick={() => del.mutate(u.id)} className="text-slate-600 hover:text-crit transition-colors"><Trash2 size={12} /></button>
          </div>
        ))}
      </div>

      {showInvite && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-sm bg-surface-1 border border-surface-3 rounded-2xl p-5 animate-slide-up">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-semibold text-slate-200">Invite user</h2>
              <button onClick={() => setShowInvite(false)}><X size={15} className="text-slate-500" /></button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Email</label>
                <input value={inviteEmail} onChange={e => setInviteEmail(e.target.value)} type="email" autoFocus
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Role</label>
                <select value={inviteRole} onChange={e => setInviteRole(e.target.value)}
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                  {['admin', 'editor', 'viewer'].map(r => <option key={r}>{r}</option>)}
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <Btn onClick={() => setShowInvite(false)}>Cancel</Btn>
              <Btn variant="primary" disabled={!inviteEmail || invite.isPending} onClick={() => invite.mutate()}>
                <Check size={13} /> Send invite
              </Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── API Keys tab ─────────────────────────────────────────────────────────────

function APIKeysTab() {
  const qc = useQueryClient()
  const [showNew, setShowNew]   = useState(false)
  const [newName, setNewName]   = useState('')
  const [newRole, setNewRole]   = useState('viewer')
  const [revealed, setRevealed] = useState<string | null>(null)

  const { data, isLoading } = useQuery({ queryKey: ['api-keys'], queryFn: () => apiKeys.list() })
  const create = useMutation({
    mutationFn: () => apiKeys.create({ name: newName, role: newRole }),
    onSuccess: (res) => { setRevealed(res.key); qc.invalidateQueries({ queryKey: ['api-keys'] }); setShowNew(false); setNewName('') },
    onError: () => toast.error('Failed to create API key'),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => apiKeys.revoke(id),
    onSuccess: () => { toast.success('Key revoked'); qc.invalidateQueries({ queryKey: ['api-keys'] }) },
  })

  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Btn variant="primary" onClick={() => setShowNew(true)}><Plus size={13} /> New key</Btn>
      </div>

      {revealed && (
        <div className="bg-ok/10 border border-ok/30 rounded-xl p-4 animate-fade-in">
          <div className="text-xs text-ok font-medium mb-2">Save this key — it won't be shown again</div>
          <div className="flex items-center gap-2">
            <code className="flex-1 text-xs font-mono bg-surface-2 px-3 py-2 rounded-lg text-slate-300 break-all">{revealed}</code>
            <Btn size="xs" onClick={() => { navigator.clipboard.writeText(revealed); toast.success('Copied') }}><Copy size={11} /></Btn>
            <Btn size="xs" onClick={() => setRevealed(null)}><X size={11} /></Btn>
          </div>
        </div>
      )}

      {isLoading && <Spinner />}

      <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
        {(data?.api_keys ?? []).map(k => (
          <div key={k.id} className="flex items-center gap-3 px-4 py-3 border-b border-surface-3/50 hover:bg-surface-2/30 text-sm">
            <div className="flex-1 min-w-0">
              <div className="text-slate-200">{k.name}</div>
              <div className="text-xs text-slate-600 font-mono">{k.key_prefix}•••</div>
            </div>
            <span className="text-xs text-slate-500 capitalize">{k.role}</span>
            <span className="text-xs text-slate-600">{k.last_used_at ? formatDistanceToNow(new Date(k.last_used_at), { addSuffix: true }) : 'never used'}</span>
            <button onClick={() => revoke.mutate(k.id)} className="text-slate-600 hover:text-crit transition-colors"><Trash2 size={12} /></button>
          </div>
        ))}
        {(data?.api_keys ?? []).length === 0 && !isLoading && (
          <div className="px-4 py-8 text-center text-xs text-slate-600">No API keys</div>
        )}
      </div>

      {showNew && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-sm bg-surface-1 border border-surface-3 rounded-2xl p-5 animate-slide-up">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-semibold text-slate-200">New API key</h2>
              <button onClick={() => setShowNew(false)}><X size={15} className="text-slate-500" /></button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Name</label>
                <input value={newName} onChange={e => setNewName(e.target.value)} autoFocus placeholder="CI/CD pipeline"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Role</label>
                <select value={newRole} onChange={e => setNewRole(e.target.value)}
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                  {['viewer', 'editor', 'admin'].map(r => <option key={r}>{r}</option>)}
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <Btn onClick={() => setShowNew(false)}>Cancel</Btn>
              <Btn variant="primary" disabled={!newName || create.isPending} onClick={() => create.mutate()}>
                <Check size={13} /> Create
              </Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Audit Log tab ────────────────────────────────────────────────────────────

function AuditTab() {
  const { data, isLoading } = useQuery({
    queryKey: ['audit-log'],
    queryFn: () => audit.list({ limit: 100 }),
    refetchInterval: 30_000,
  })

  const actionColor = (action: string) => ({
    create: 'text-ok', update: 'text-brand', delete: 'text-crit',
    login: 'text-info', logout: 'text-slate-400', login_failed: 'text-warn',
  }[action] ?? 'text-slate-400')

  return (
    <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
      <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600 bg-surface-2 border-b border-surface-3 px-4 py-2"
        style={{ gridTemplateColumns: '1fr 80px 100px 80px 120px' }}>
        <span>User</span><span>Action</span><span>Resource</span><span>IP</span><span>Time</span>
      </div>
      {isLoading && <div className="flex justify-center p-6"><Spinner /></div>}
      {(data?.entries ?? []).map(e => (
        <div key={e.id} className="grid items-center px-4 py-2 border-b border-surface-3/50 hover:bg-surface-2/30 text-xs"
          style={{ gridTemplateColumns: '1fr 80px 100px 80px 120px' }}>
          <span className="text-slate-300 truncate">{e.user_email}</span>
          <span className={clsx('font-mono', actionColor(e.action))}>{e.action}</span>
          <span className="text-slate-500 truncate">{e.resource} {e.resource_id ? `#${e.resource_id.slice(0, 6)}` : ''}</span>
          <span className="text-slate-600 font-mono">{e.ip_address}</span>
          <span className="text-slate-600">{format(new Date(e.created_at), 'MMM dd HH:mm:ss')}</span>
        </div>
      ))}
      {!isLoading && (data?.entries ?? []).length === 0 && (
        <div className="px-4 py-8 text-center text-xs text-slate-600">No audit entries</div>
      )}
    </div>
  )
}

// ── Teams tab ─────────────────────────────────────────────────────────────────

function TeamsTab() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<string | null>(null)
  const [showNew, setShowNew]   = useState(false)
  const [newName, setNewName]   = useState('')
  const [newDesc, setNewDesc]   = useState('')
  const [addEmail, setAddEmail] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['teams'],
    queryFn: () => teamsApi.list(),
  })
  const { data: detail } = useQuery({
    queryKey: ['team', selected],
    queryFn: () => teamsApi.get(selected!),
    enabled: !!selected,
  })

  const create = useMutation({
    mutationFn: () => teamsApi.create({ name: newName, description: newDesc }),
    onSuccess: () => {
      toast.success('Team created')
      qc.invalidateQueries({ queryKey: ['teams'] })
      setShowNew(false); setNewName(''); setNewDesc('')
    },
    onError: () => toast.error('Failed to create team'),
  })
  const del = useMutation({
    mutationFn: (id: string) => teamsApi.delete(id),
    onSuccess: () => {
      toast.success('Team deleted')
      qc.invalidateQueries({ queryKey: ['teams'] })
      if (selected && data?.teams.find(t => t.id === selected)) setSelected(null)
    },
  })
  const addMember = useMutation({
    mutationFn: () => teamsApi.addMember(selected!, { email: addEmail, role: 'viewer' }),
    onSuccess: () => {
      toast.success('Member added')
      qc.invalidateQueries({ queryKey: ['team', selected] })
      setAddEmail('')
    },
    onError: () => toast.error('Failed to add member'),
  })
  const removeMember = useMutation({
    mutationFn: (userId: string) => teamsApi.removeMember(selected!, userId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['team', selected] }),
  })

  return (
    <div className="flex gap-4 h-full">
      {/* Team list */}
      <div className="w-48 flex-shrink-0 space-y-1.5">
        <Btn variant="primary" className="w-full" onClick={() => setShowNew(true)}>
          <Plus size={13} /> New team
        </Btn>
        {isLoading && <Spinner />}
        {(data?.teams ?? []).map(t => (
          <button key={t.id}
            onClick={() => setSelected(t.id)}
            className={clsx(
              'w-full text-left px-3 py-2 rounded-lg text-sm transition-colors',
              selected === t.id
                ? 'bg-brand/15 text-brand border border-brand/30'
                : 'text-slate-400 hover:bg-surface-2 hover:text-slate-200 border border-transparent'
            )}>
            <div className="font-medium truncate">{t.name}</div>
            <div className="text-[10px] text-slate-600 mt-0.5">
              {(t.namespaces ?? []).length} namespace{(t.namespaces ?? []).length !== 1 ? 's' : ''}
            </div>
          </button>
        ))}
        {!isLoading && (data?.teams ?? []).length === 0 && (
          <div className="text-xs text-slate-600 px-1 pt-2">No teams yet</div>
        )}
      </div>

      {/* Team detail */}
      <div className="flex-1 min-w-0">
        {!selected ? (
          <div className="flex items-center justify-center h-40 text-sm text-slate-600">
            Select a team to manage members
          </div>
        ) : (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-medium text-slate-200">{detail?.team?.name}</h3>
                {detail?.team?.description && (
                  <p className="text-xs text-slate-500 mt-0.5">{detail.team.description}</p>
                )}
              </div>
              <Btn size="xs" variant="danger" onClick={() => del.mutate(selected)}>
                <Trash2 size={11} /> Delete team
              </Btn>
            </div>

            {/* Members */}
            <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
              <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                              bg-surface-2 border-b border-surface-3 px-4 py-2"
                style={{ gridTemplateColumns: '1fr 80px 120px 32px' }}>
                <span>Member</span><span>Role</span><span>Joined</span><span />
              </div>
              {(detail?.members ?? []).map(m => (
                <div key={m.user_id} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-sm"
                  style={{ gridTemplateColumns: '1fr 80px 120px 32px' }}>
                  <div>
                    <div className="text-slate-200">{m.user_name ?? '—'}</div>
                    <div className="text-xs text-slate-600">{m.email}</div>
                  </div>
                  <span className="text-xs text-slate-400 capitalize">{m.role}</span>
                  <span className="text-xs text-slate-600">
                    {m.joined_at ? formatDistanceToNow(new Date(m.joined_at), { addSuffix: true }) : '—'}
                  </span>
                  <button onClick={() => removeMember.mutate(m.user_id)}
                    className="text-slate-600 hover:text-crit transition-colors">
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
              {(detail?.members ?? []).length === 0 && (
                <div className="px-4 py-6 text-center text-xs text-slate-600">No members</div>
              )}
            </div>

            {/* Add member */}
            <div className="flex gap-2">
              <input value={addEmail} onChange={e => setAddEmail(e.target.value)}
                placeholder="member@example.com"
                onKeyDown={e => { if (e.key === 'Enter' && addEmail) addMember.mutate() }}
                className="flex-1 px-3 py-2 text-sm bg-surface-1 border border-surface-3 rounded-lg text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50" />
              <Btn variant="primary" disabled={!addEmail || addMember.isPending} onClick={() => addMember.mutate()}>
                <Plus size={13} /> Add
              </Btn>
            </div>
          </div>
        )}
      </div>

      {/* New team modal */}
      {showNew && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-sm bg-surface-1 border border-surface-3 rounded-2xl p-5 animate-slide-up">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-semibold text-slate-200">New team</h2>
              <button onClick={() => setShowNew(false)}><X size={15} className="text-slate-500" /></button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Name</label>
                <input value={newName} onChange={e => setNewName(e.target.value)} autoFocus placeholder="platform-eng"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Description</label>
                <input value={newDesc} onChange={e => setNewDesc(e.target.value)} placeholder="Optional"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none focus:border-brand/60" />
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <Btn onClick={() => setShowNew(false)}>Cancel</Btn>
              <Btn variant="primary" disabled={!newName || create.isPending} onClick={() => create.mutate()}>
                <Check size={13} /> Create
              </Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Namespaces tab ────────────────────────────────────────────────────────────

function NamespacesTab() {
  const qc = useQueryClient()
  const [showGrant, setShowGrant] = useState(false)
  const [ns, setNs]               = useState('')
  const [teamId, setTeamId]       = useState('')
  const [access, setAccess]       = useState('read')

  const { data: permsData, isLoading } = useQuery({
    queryKey: ['ns-perms'],
    queryFn: () => orgApi.listNsPerms(),
  })
  const { data: teamsData } = useQuery({
    queryKey: ['teams'],
    queryFn: () => teamsApi.list(),
  })

  const grant = useMutation({
    mutationFn: () => orgApi.grantNs({ namespace: ns, team_id: teamId, access_level: access }),
    onSuccess: () => {
      toast.success('Namespace access granted')
      qc.invalidateQueries({ queryKey: ['ns-perms'] })
      setShowGrant(false); setNs(''); setTeamId('')
    },
    onError: () => toast.error('Failed to grant access'),
  })
  const revoke = useMutation({
    mutationFn: ({ team, namespace }: { team: string; namespace: string }) =>
      orgApi.revokeNs(team, namespace),
    onSuccess: () => {
      toast.success('Access revoked')
      qc.invalidateQueries({ queryKey: ['ns-perms'] })
    },
  })

  const perms = permsData?.permissions ?? []

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-xs text-slate-500">
          Control which teams can read or write specific Kubernetes namespaces.
        </p>
        <Btn variant="primary" onClick={() => setShowGrant(true)}>
          <Plus size={13} /> Grant access
        </Btn>
      </div>

      {isLoading && <div className="flex justify-center py-6"><Spinner /></div>}

      <div className="bg-surface-1 border border-surface-3 rounded-xl overflow-hidden">
        <div className="grid text-[10px] font-semibold uppercase tracking-wider text-slate-600
                        bg-surface-2 border-b border-surface-3 px-4 py-2"
          style={{ gridTemplateColumns: '1fr 1fr 80px 32px' }}>
          <span>Namespace</span><span>Team</span><span>Access</span><span />
        </div>
        {perms.map((p, i) => (
          <div key={i} className="grid items-center px-4 py-2.5 border-b border-surface-3/50 text-sm"
            style={{ gridTemplateColumns: '1fr 1fr 80px 32px' }}>
            <span className="font-mono text-slate-300 text-xs">{p.namespace}</span>
            <span className="text-xs text-slate-400">{p.team_name ?? p.team_id}</span>
            <span className={clsx('text-xs font-medium',
              p.access_level === 'write' ? 'text-warn' : 'text-ok')}>
              {p.access_level}
            </span>
            <button
              onClick={() => revoke.mutate({ team: p.team_id, namespace: p.namespace })}
              className="text-slate-600 hover:text-crit transition-colors">
              <Trash2 size={12} />
            </button>
          </div>
        ))}
        {!isLoading && perms.length === 0 && (
          <div className="px-4 py-8 text-center text-xs text-slate-600">
            No namespace grants — all teams see all namespaces by default.
          </div>
        )}
      </div>

      {showGrant && (
        <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-sm bg-surface-1 border border-surface-3 rounded-2xl p-5 animate-slide-up">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-sm font-semibold text-slate-200">Grant namespace access</h2>
              <button onClick={() => setShowGrant(false)}><X size={15} className="text-slate-500" /></button>
            </div>
            <div className="space-y-3">
              <div>
                <label className="block text-xs text-slate-500 mb-1">Namespace</label>
                <input value={ns} onChange={e => setNs(e.target.value)} autoFocus placeholder="production"
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 font-mono focus:outline-none focus:border-brand/60" />
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Team</label>
                <select value={teamId} onChange={e => setTeamId(e.target.value)}
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                  <option value="">Select a team…</option>
                  {(teamsData?.teams ?? []).map(t => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs text-slate-500 mb-1">Access level</label>
                <select value={access} onChange={e => setAccess(e.target.value)}
                  className="w-full px-3 py-2 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-300 focus:outline-none">
                  <option value="read">read — view metrics, logs, traces</option>
                  <option value="write">write — read + create SLOs and alerts</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end gap-2 mt-4">
              <Btn onClick={() => setShowGrant(false)}>Cancel</Btn>
              <Btn variant="primary" disabled={!ns || !teamId || grant.isPending} onClick={() => grant.mutate()}>
                <Check size={13} /> Grant
              </Btn>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Main ─────────────────────────────────────────────────────────────────────

export default function SettingsPage() {
  const [tab, setTab] = useState<Tab>('org')
  const { user } = useAuth()

  const content = {
    org:        <OrgTab />,
    users:      <UsersTab />,
    teams:      <TeamsTab />,
    apikeys:    <APIKeysTab />,
    namespaces: <NamespacesTab />,
    audit:      <AuditTab />,
  }[tab]

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader title="Settings" subtitle={`Logged in as ${user?.email}`} />

      <div className="flex-1 flex overflow-hidden">
        {/* Left nav */}
        <div className="w-44 border-r border-surface-3 py-3 flex-shrink-0">
          {TABS.map(({ id, icon: Icon, label }) => (
            <button key={id} onClick={() => setTab(id)}
              className={clsx('w-full flex items-center gap-2.5 px-4 py-2 text-sm transition-colors', tab === id ? 'text-brand bg-brand/10' : 'text-slate-500 hover:text-slate-300 hover:bg-surface-2')}>
              <Icon size={14} />{label}
            </button>
          ))}
        </div>

        {/* Content */}
        <div className="flex-1 overflow-y-auto p-5">{content}</div>
      </div>
    </div>
  )
}
