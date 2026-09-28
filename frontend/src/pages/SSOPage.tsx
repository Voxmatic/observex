// frontend/src/pages/SSOPage.tsx
// Single Sign-On configuration — SAML 2.0, OIDC, GitHub, Google, Azure AD
// Connects to /api/v1/sso/providers and /api/auth/sso/* endpoints

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { sso, type SSOConfig } from '@/lib/api'
import { PageHeader, Spinner, EmptyState } from '@/components/shared/Layout'
import {
  Shield, Plus, Trash2, CheckCircle, XCircle, AlertTriangle,
  ExternalLink, Copy, ChevronDown, ChevronRight, Settings
} from 'lucide-react'
import { format } from 'date-fns'
import toast from 'react-hot-toast'
import clsx from 'clsx'

// ── Provider metadata ──────────────────────────────────────────────────────────

const PROVIDERS = [
  {
    type: 'saml',
    name: 'SAML 2.0',
    description: 'Works with Okta, Azure AD, Google Workspace, PingFederate, and any SAML 2.0 IdP',
    logo: '🔐',
    fields: [
      { key: 'idp_entity_id',  label: 'IdP Entity ID',       placeholder: 'https://idp.example.com/saml2/metadata', type: 'url'  },
      { key: 'idp_sso_url',    label: 'IdP SSO URL',          placeholder: 'https://idp.example.com/saml2/sso',      type: 'url'  },
      { key: 'idp_cert',       label: 'IdP X.509 Certificate', placeholder: '-----BEGIN CERTIFICATE-----\n…\n-----END CERTIFICATE-----', type: 'textarea' },
      { key: 'sp_entity_id',   label: 'SP Entity ID (optional)', placeholder: 'https://your-observex.com/api/auth/sso/saml/metadata', type: 'url' },
    ],
  },
  {
    type: 'oidc',
    name: 'OpenID Connect',
    description: 'Generic OIDC provider — works with Auth0, Keycloak, Ping Identity, or any OIDC 1.0 server',
    logo: '🔑',
    fields: [
      { key: 'issuer',        label: 'Issuer / Discovery URL', placeholder: 'https://auth.example.com/.well-known/openid-configuration', type: 'url'      },
      { key: 'client_id',     label: 'Client ID',              placeholder: 'your-client-id',     type: 'text'     },
      { key: 'client_secret', label: 'Client Secret',          placeholder: '(stored encrypted)', type: 'password' },
    ],
  },
  {
    type: 'google',
    name: 'Google Workspace',
    description: 'Sign in with Google using OAuth 2.0 / OIDC. Supports domain restriction.',
    logo: '🟦',
    fields: [
      { key: 'client_id',     label: 'Google OAuth Client ID',     placeholder: 'xxx.apps.googleusercontent.com', type: 'text'     },
      { key: 'client_secret', label: 'Google OAuth Client Secret', placeholder: '(stored encrypted)',             type: 'password' },
    ],
  },
  {
    type: 'azure',
    name: 'Azure Active Directory',
    description: 'Sign in with Microsoft Azure AD (Entra ID) via OIDC or SAML.',
    logo: '🔷',
    fields: [
      { key: 'issuer',        label: 'Azure Tenant ID / Issuer', placeholder: 'https://login.microsoftonline.com/{tenant}/v2.0', type: 'url'      },
      { key: 'client_id',     label: 'Application (client) ID',  placeholder: 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx',            type: 'text'     },
      { key: 'client_secret', label: 'Client Secret Value',      placeholder: '(stored encrypted)',                              type: 'password' },
    ],
  },
  {
    type: 'github',
    name: 'GitHub',
    description: 'Sign in with GitHub. Use GitHub Organizations to restrict access.',
    logo: '🐙',
    fields: [
      { key: 'client_id',     label: 'GitHub OAuth App Client ID',     placeholder: 'Iv1.xxxx', type: 'text'     },
      { key: 'client_secret', label: 'GitHub OAuth App Client Secret', placeholder: '(stored encrypted)', type: 'password' },
    ],
  },
]

// ── Helper components ──────────────────────────────────────────────────────────

function StatusBadge({ enabled }: { enabled: boolean }) {
  return enabled
    ? <span className="flex items-center gap-1 text-[11px] font-medium text-ok"><CheckCircle size={11} /> Enabled</span>
    : <span className="flex items-center gap-1 text-[11px] text-slate-500"><XCircle size={11} /> Disabled</span>
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <button onClick={copy}
      className="flex items-center gap-1 text-[11px] text-slate-400 hover:text-slate-200 transition-colors ml-auto">
      <Copy size={11} />
      {copied ? 'Copied!' : `Copy ${label}`}
    </button>
  )
}

// ── Provider config form ───────────────────────────────────────────────────────

function ProviderForm({ providerMeta, existing, onSave, onCancel }: {
  providerMeta: typeof PROVIDERS[0]
  existing?: SSOConfig
  onSave: (data: Partial<SSOConfig>) => void
  onCancel: () => void
}) {
  const [form, setForm] = useState<Record<string, string>>(() => {
    const init: Record<string, string> = {
      type: providerMeta.type,
      auto_provision: existing?.auto_provision ? 'true' : 'true',
      default_role: existing?.default_role ?? 'viewer',
      email_attr: existing?.email_attr ?? 'email',
      name_attr: existing?.name_attr ?? 'displayName',
    }
    providerMeta.fields.forEach(f => {
      init[f.key] = (existing as any)?.[f.key] ?? ''
    })
    return init
  })

  const set = (k: string, v: string) => setForm(f => ({ ...f, [k]: v }))

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const payload: Partial<SSOConfig> = {
      type: form.type as SSOConfig['type'],
      enabled: true,
      auto_provision: form.auto_provision === 'true',
      default_role: form.default_role,
      email_attr: form.email_attr,
      name_attr: form.name_attr,
    }
    providerMeta.fields.forEach(f => {
      if (form[f.key]) (payload as any)[f.key] = form[f.key]
    })
    onSave(payload)
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Provider fields */}
      {providerMeta.fields.map(field => (
        <div key={field.key}>
          <label className="block text-xs font-medium text-slate-400 mb-1">{field.label}</label>
          {field.type === 'textarea' ? (
            <textarea
              value={form[field.key] ?? ''}
              onChange={e => set(field.key, e.target.value)}
              placeholder={field.placeholder}
              rows={4}
              className="w-full px-3 py-2 text-xs font-mono bg-surface-0 border border-surface-3 rounded-lg
                         text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50 resize-y"
            />
          ) : (
            <input
              type={field.type}
              value={form[field.key] ?? ''}
              onChange={e => set(field.key, e.target.value)}
              placeholder={field.placeholder}
              className="w-full px-3 py-2 text-sm bg-surface-0 border border-surface-3 rounded-lg
                         text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50"
            />
          )}
        </div>
      ))}

      {/* Attribute mapping */}
      <div className="border-t border-surface-2 pt-4 space-y-3">
        <h4 className="text-xs font-medium text-slate-400 uppercase tracking-wide">Attribute Mapping</h4>
        <div className="grid grid-cols-2 gap-3">
          {[
            { key: 'email_attr', label: 'Email attribute',  placeholder: 'email' },
            { key: 'name_attr',  label: 'Name attribute',   placeholder: 'displayName' },
          ].map(f => (
            <div key={f.key}>
              <label className="block text-xs text-slate-500 mb-1">{f.label}</label>
              <input value={form[f.key] ?? ''} onChange={e => set(f.key, e.target.value)}
                placeholder={f.placeholder}
                className="w-full px-2 py-1.5 text-sm bg-surface-0 border border-surface-3 rounded
                           text-slate-300 placeholder-slate-600 focus:outline-none focus:border-brand/50" />
            </div>
          ))}
        </div>
      </div>

      {/* Auto-provision */}
      <div className="flex items-center justify-between bg-surface-0 rounded-lg px-4 py-3">
        <div>
          <div className="text-sm text-slate-300 font-medium">Auto-provision users</div>
          <div className="text-xs text-slate-500 mt-0.5">Create ObserveX accounts automatically on first SSO login</div>
        </div>
        <button type="button"
          onClick={() => set('auto_provision', form.auto_provision === 'true' ? 'false' : 'true')}
          className={clsx(
            'w-10 h-6 rounded-full transition-colors relative',
            form.auto_provision === 'true' ? 'bg-brand' : 'bg-surface-3'
          )}>
          <span className={clsx('absolute top-1 w-4 h-4 rounded-full bg-white transition-transform',
            form.auto_provision === 'true' ? 'translate-x-5' : 'translate-x-1')} />
        </button>
      </div>

      {/* Default role */}
      <div>
        <label className="block text-xs font-medium text-slate-400 mb-1">Default role for new users</label>
        <select value={form.default_role} onChange={e => set('default_role', e.target.value)}
          className="w-full px-3 py-2 text-sm bg-surface-0 border border-surface-3 rounded-lg text-slate-300
                     focus:outline-none focus:border-brand/50">
          <option value="viewer">Viewer</option>
          <option value="editor">Editor</option>
          <option value="admin">Admin</option>
        </select>
      </div>

      <div className="flex items-center gap-3 pt-2">
        <button type="submit"
          className="flex-1 py-2 bg-brand/20 hover:bg-brand/30 text-brand text-sm font-medium rounded-lg border border-brand/30 transition-colors">
          {existing ? 'Save changes' : 'Enable provider'}
        </button>
        <button type="button" onClick={onCancel}
          className="px-4 py-2 text-sm text-slate-400 hover:text-slate-200 transition-colors">
          Cancel
        </button>
      </div>
    </form>
  )
}

// ── Configured provider card ───────────────────────────────────────────────────

function ConfiguredCard({ config, onDelete, onEdit }: {
  config: SSOConfig
  onDelete: () => void
  onEdit: () => void
}) {
  const meta = PROVIDERS.find(p => p.type === config.type)
  const qc = useQueryClient()
  const testMutation = useMutation({
    mutationFn: () => sso.test(config.type),
    onSuccess: (d: any) => toast.success(d.message ?? 'Test passed'),
    onError: () => toast.error('Test failed'),
  })

  return (
    <div className="bg-surface-1 border border-surface-2 rounded-xl p-5">
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-3">
          <span className="text-2xl">{meta?.logo ?? '🔐'}</span>
          <div>
            <div className="text-sm font-medium text-slate-200">{meta?.name ?? config.type.toUpperCase()}</div>
            <StatusBadge enabled={config.enabled} />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => testMutation.mutate()}
            className="px-2 py-1 text-xs bg-surface-2 hover:bg-surface-3 rounded text-slate-400 hover:text-slate-200 transition-colors">
            {testMutation.isPending ? '…' : 'Test'}
          </button>
          <button onClick={onEdit}
            className="px-2 py-1 text-xs bg-surface-2 hover:bg-surface-3 rounded text-slate-400 hover:text-slate-200 transition-colors">
            Edit
          </button>
          <button onClick={onDelete}
            className="px-2 py-1 text-xs bg-crit/10 hover:bg-crit/20 rounded text-crit transition-colors">
            <Trash2 size={11} />
          </button>
        </div>
      </div>

      <div className="space-y-1 text-xs text-slate-500">
        {config.idp_sso_url && <div><span className="text-slate-400">IdP URL:</span> <span className="font-mono">{config.idp_sso_url}</span></div>}
        {config.issuer      && <div><span className="text-slate-400">Issuer:</span> <span className="font-mono">{config.issuer}</span></div>}
        {config.client_id   && <div><span className="text-slate-400">Client ID:</span> <span className="font-mono">{config.client_id}</span></div>}
        <div><span className="text-slate-400">Auto-provision:</span> {config.auto_provision ? 'Yes' : 'No'} · Default role: <span className="capitalize">{config.default_role}</span></div>
        <div><span className="text-slate-400">Added:</span> {format(new Date(config.created_at), 'MMM d, yyyy')}</div>
      </div>
    </div>
  )
}

// ── SP info box ────────────────────────────────────────────────────────────────

function SPInfoBox() {
  const baseURL = window.location.origin
  const items = [
    { label: 'ACS URL (SAML callback)',  value: `${baseURL}/api/auth/sso/saml/callback` },
    { label: 'SP Metadata URL',          value: `${baseURL}/api/auth/sso/saml/metadata` },
    { label: 'OIDC Redirect URI',        value: `${baseURL}/api/auth/sso/oidc/callback` },
  ]
  return (
    <div className="bg-surface-1 border border-brand/20 rounded-xl p-5">
      <h3 className="text-sm font-medium text-slate-200 mb-3 flex items-center gap-2">
        <Settings size={14} className="text-brand" />
        Service Provider (SP) Configuration
      </h3>
      <p className="text-xs text-slate-500 mb-3">
        Use these URLs when configuring your Identity Provider.
      </p>
      <div className="space-y-2">
        {items.map(item => (
          <div key={item.label} className="flex items-center gap-2 bg-surface-0 rounded-lg px-3 py-2">
            <div className="flex-1 min-w-0">
              <div className="text-[10px] text-slate-500 uppercase tracking-wide">{item.label}</div>
              <div className="text-xs font-mono text-slate-300 truncate">{item.value}</div>
            </div>
            <CopyButton value={item.value} label="URL" />
          </div>
        ))}
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function SSOPage() {
  const [adding, setAdding] = useState<string | null>(null)
  const [editing, setEditing] = useState<SSOConfig | null>(null)
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['sso-config'],
    queryFn: () => sso.getConfig(),
  })

  const configs: SSOConfig[] = data?.configs ?? []

  const upsertMutation = useMutation({
    mutationFn: (d: Partial<SSOConfig>) => sso.upsert(d),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['sso-config'] })
      setAdding(null); setEditing(null)
      toast.success('SSO provider saved')
    },
    onError: (e: any) => toast.error(e?.response?.data?.error ?? 'Save failed'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => sso.delete(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['sso-config'] })
      toast.success('Provider removed')
    },
  })

  const configuredTypes = new Set(configs.map(c => c.type))

  return (
    <div className="space-y-6">
      <PageHeader
        title="Single Sign-On"
        subtitle="Configure SAML 2.0, OIDC, and OAuth providers for your organisation"
        />

      {isLoading && <div className="text-center py-12"><Spinner /></div>}

      {!isLoading && (
        <div className="grid grid-cols-3 gap-6">
          {/* Left: configured + add new */}
          <div className="col-span-2 space-y-5">

            {/* Configured providers */}
            {configs.length > 0 && (
              <div>
                <h3 className="text-xs font-medium text-slate-500 uppercase tracking-wide mb-3">
                  Configured providers
                </h3>
                <div className="space-y-3">
                  {configs.map(cfg => (
                    <ConfiguredCard
                      key={cfg.id}
                      config={cfg}
                      onDelete={() => deleteMutation.mutate(cfg.id)}
                      onEdit={() => { setEditing(cfg); setAdding(null) }}
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Edit form */}
            {editing && (
              <div className="bg-surface-1 border border-brand/20 rounded-xl p-5">
                <h3 className="text-sm font-medium text-slate-200 mb-4 flex items-center gap-2">
                  <Settings size={14} className="text-brand" />
                  Edit {PROVIDERS.find(p => p.type === editing.type)?.name}
                </h3>
                <ProviderForm
                  providerMeta={PROVIDERS.find(p => p.type === editing.type)!}
                  existing={editing}
                  onSave={d => upsertMutation.mutate(d)}
                  onCancel={() => setEditing(null)}
                />
              </div>
            )}

            {/* Add new provider */}
            {!editing && (
              <div>
                <h3 className="text-xs font-medium text-slate-500 uppercase tracking-wide mb-3">
                  Add provider
                </h3>
                <div className="space-y-2">
                  {PROVIDERS.filter(p => !configuredTypes.has(p.type as any)).map(provider => (
                    <div key={provider.type}>
                      <button
                        onClick={() => setAdding(adding === provider.type ? null : provider.type)}
                        className="w-full flex items-center gap-3 px-4 py-3 bg-surface-1 border border-surface-2
                                   hover:border-brand/30 rounded-xl transition-colors text-left"
                      >
                        <span className="text-xl">{provider.logo}</span>
                        <div className="flex-1">
                          <div className="text-sm font-medium text-slate-200">{provider.name}</div>
                          <div className="text-xs text-slate-500">{provider.description}</div>
                        </div>
                        {adding === provider.type
                          ? <ChevronDown size={14} className="text-brand flex-shrink-0" />
                          : <Plus size={14} className="text-slate-500 flex-shrink-0" />}
                      </button>

                      {adding === provider.type && (
                        <div className="mt-2 bg-surface-1 border border-brand/20 rounded-xl p-5">
                          <ProviderForm
                            providerMeta={provider}
                            onSave={d => upsertMutation.mutate(d)}
                            onCancel={() => setAdding(null)}
                          />
                        </div>
                      )}
                    </div>
                  ))}

                  {PROVIDERS.every(p => configuredTypes.has(p.type as any)) && (
                    <p className="text-xs text-slate-500 text-center py-4">
                      All supported providers are configured.
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>

          {/* Right: SP info + docs */}
          <div className="space-y-4">
            <SPInfoBox />

            <div className="bg-surface-1 border border-surface-2 rounded-xl p-5 space-y-3">
              <h3 className="text-sm font-medium text-slate-200">Documentation</h3>
              {[
                { label: 'Okta SAML setup guide', href: 'https://developer.okta.com/docs/guides/build-sso-integration/saml2/main/' },
                { label: 'Azure AD OIDC guide',   href: 'https://learn.microsoft.com/en-us/azure/active-directory/develop/v2-protocols-oidc' },
                { label: 'Google OAuth 2.0',      href: 'https://developers.google.com/identity/protocols/oauth2' },
                { label: 'GitHub OAuth apps',     href: 'https://docs.github.com/en/apps/oauth-apps' },
              ].map(link => (
                <a key={link.href} href={link.href} target="_blank" rel="noopener noreferrer"
                  className="flex items-center gap-2 text-xs text-brand hover:text-brand/80 transition-colors">
                  <ExternalLink size={11} />
                  {link.label}
                </a>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
