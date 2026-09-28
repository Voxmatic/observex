// frontend/src/pages/IntegrationsPage.tsx
// Integration hub — connect ObserveX to Slack, PagerDuty, OpsGenie, Teams,
// AWS CloudWatch, GCP Cloud Monitoring, and native ObserveX Agent enrollment.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { integrations } from '@/lib/api'
import { PageHeader, Btn, Spinner, EmptyState } from '@/components/shared/Layout'
import { Plus, Check, X, Zap, RefreshCw, ExternalLink, AlertTriangle } from 'lucide-react'
import clsx from 'clsx'
import toast from 'react-hot-toast'

// ── Integration catalog ───────────────────────────────────────────────────────

const CATALOG = [
  {
    id: 'slack', name: 'Slack', category: 'Notifications',
    logo: '💬', desc: 'Send alert notifications and incident updates to Slack channels.',
    fields: [{ key: 'webhook_url', label: 'Incoming webhook URL', placeholder: 'https://hooks.slack.com/services/…', type: 'url' },
             { key: 'channel', label: 'Default channel', placeholder: '#alerts', type: 'text' }],
    docs: 'https://api.slack.com/messaging/webhooks',
  },
  {
    id: 'pagerduty', name: 'PagerDuty', category: 'On-call',
    logo: '🔔', desc: 'Route critical alerts to PagerDuty for automated on-call escalation.',
    fields: [{ key: 'routing_key', label: 'Integration key (Events API v2)', placeholder: '…', type: 'password' }],
    docs: 'https://developer.pagerduty.com/docs/events-api-v2',
  },
  {
    id: 'opsgenie', name: 'OpsGenie', category: 'On-call',
    logo: '🚨', desc: 'Create OpsGenie alerts with full incident context and auto-close.',
    fields: [{ key: 'api_key', label: 'OpsGenie API key', placeholder: '…', type: 'password' },
             { key: 'team', label: 'Default team', placeholder: 'platform-eng', type: 'text' }],
    docs: 'https://support.atlassian.com/opsgenie/docs/api-key-management/',
  },
  {
    id: 'teams', name: 'Microsoft Teams', category: 'Notifications',
    logo: '🟦', desc: 'Post adaptive cards to Teams channels on incident open/resolve.',
    fields: [{ key: 'webhook_url', label: 'Incoming webhook URL', placeholder: 'https://…webhook.office.com/…', type: 'url' }],
    docs: 'https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors',
  },
  {
    id: 'observex-agent', name: 'ObserveX Agent Enrollment', category: 'Metrics',
    logo: '⚙️', desc: 'Issue SaaS enrollment tokens for native host, process, container, log, trace, profile, topology, and security collection.',
    fields: [{ key: 'tenant_url', label: 'Tenant ingest URL', placeholder: 'https://ingest.observex.io', type: 'url' },
             { key: 'enrollment_token', label: 'Enrollment token', placeholder: '…', type: 'password' }],
    docs: '/docs/observex-agent-saas-playbook',
  },
  {
    id: 'aws', name: 'AWS CloudWatch', category: 'Cloud',
    logo: '🌩', desc: 'Pull CloudWatch metrics (EC2, RDS, Lambda, ECS) into ObserveX dashboards.',
    fields: [{ key: 'access_key_id', label: 'Access Key ID', placeholder: 'AKIA…', type: 'text' },
             { key: 'secret_access_key', label: 'Secret Access Key', placeholder: '…', type: 'password' },
             { key: 'region', label: 'Region', placeholder: 'us-east-1', type: 'text' }],
    docs: 'https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/',
  },
  {
    id: 'gcp', name: 'Google Cloud Monitoring', category: 'Cloud',
    logo: '🌐', desc: 'Import GCP metrics from Cloud Monitoring (Stackdriver) into dashboards.',
    fields: [{ key: 'project_id', label: 'GCP Project ID', placeholder: 'my-project', type: 'text' },
             { key: 'service_account_json', label: 'Service account JSON', placeholder: '{"type":"service_account",…}', type: 'textarea' }],
    docs: 'https://cloud.google.com/monitoring/docs',
  },
  {
    id: 'github', name: 'GitHub', category: 'Code',
    logo: '🐙', desc: 'Auto-create deploy markers on GitHub releases and pull request merges.',
    fields: [{ key: 'token', label: 'Personal access token', placeholder: 'ghp_…', type: 'password' },
             { key: 'org', label: 'Organization', placeholder: 'myorg', type: 'text' }],
    docs: 'https://docs.github.com/en/rest/deployments',
  },
  {
    id: 'jira', name: 'Jira', category: 'Collaboration',
    logo: '📋', desc: 'Auto-create Jira tickets for critical incidents with full context.',
    fields: [{ key: 'url', label: 'Jira URL', placeholder: 'https://myteam.atlassian.net', type: 'url' },
             { key: 'token', label: 'API token', placeholder: '…', type: 'password' },
             { key: 'project_key', label: 'Project key', placeholder: 'OPS', type: 'text' }],
    docs: 'https://developer.atlassian.com/cloud/jira/platform/rest/v3/',
  },
]

const CATEGORIES = ['All', ...Array.from(new Set(CATALOG.map(c => c.category)))]

// ── Integration card ──────────────────────────────────────────────────────────

function IntegrationCard({ integration, onConfigure, onTest, onDelete, configured, testing }: {
  integration: typeof CATALOG[0]
  onConfigure: () => void
  onTest: () => void
  onDelete: () => void
  configured: boolean
  testing: boolean
}) {
  return (
    <div className={clsx('bg-surface-1 border rounded-xl p-4 transition-all',
      configured ? 'border-ok/25' : 'border-surface-3')}>
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl bg-surface-2 flex items-center justify-center text-xl">
            {integration.logo}
          </div>
          <div>
            <div className="text-sm font-semibold text-slate-200">{integration.name}</div>
            <div className="text-[10px] bg-surface-3 text-slate-400 px-1.5 py-0.5 rounded inline-block mt-0.5">
              {integration.category}
            </div>
          </div>
        </div>
        {configured && (
          <div className="flex items-center gap-1 text-[10px] text-ok">
            <Check size={10} /> Connected
          </div>
        )}
      </div>

      <p className="text-xs text-slate-500 leading-relaxed mb-4">{integration.desc}</p>

      <div className="flex items-center gap-2">
        <Btn size="xs" variant={configured ? 'ghost' : 'primary'} onClick={onConfigure}>
          {configured ? 'Reconfigure' : 'Connect'}
        </Btn>
        {configured && (
          <>
            <Btn size="xs" variant="ghost" onClick={onTest} disabled={testing}>
              {testing ? <><RefreshCw size={10} className="animate-spin" /> Testing…</> : '▶ Test'}
            </Btn>
            <Btn size="xs" variant="danger" onClick={onDelete}><X size={10} /></Btn>
          </>
        )}
        <a href={integration.docs} target="_blank" rel="noopener"
          className="ml-auto text-[10px] text-slate-600 hover:text-slate-400 flex items-center gap-0.5">
          Docs <ExternalLink size={9} />
        </a>
      </div>
    </div>
  )
}

// ── Configure modal ───────────────────────────────────────────────────────────

function ConfigureModal({ integration, onSave, onClose }: {
  integration: typeof CATALOG[0]
  onSave: (config: Record<string, string>) => void
  onClose: () => void
}) {
  const [values, setValues] = useState<Record<string, string>>({})

  return (
    <div className="fixed inset-0 bg-black/60 z-50 flex items-center justify-center p-4">
      <div className="w-full max-w-md bg-surface-1 border border-surface-3 rounded-2xl p-5">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <span className="text-xl">{integration.logo}</span>
            <h2 className="text-sm font-semibold text-slate-200">Configure {integration.name}</h2>
          </div>
          <button onClick={onClose}><X size={15} className="text-slate-500" /></button>
        </div>

        <div className="space-y-3">
          {integration.fields.map(field => (
            <div key={field.key}>
              <label className="block text-xs text-slate-500 mb-1">{field.label}</label>
              {field.type === 'textarea' ? (
                <textarea
                  value={values[field.key] ?? ''}
                  onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
                  placeholder={field.placeholder}
                  rows={3}
                  className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 font-mono focus:outline-none focus:border-brand/60 resize-none"
                />
              ) : (
                <input
                  type={field.type}
                  value={values[field.key] ?? ''}
                  onChange={e => setValues(v => ({ ...v, [field.key]: e.target.value }))}
                  placeholder={field.placeholder}
                  className="w-full px-3 py-2 text-xs bg-surface-2 border border-surface-3 rounded-lg text-slate-300 font-mono focus:outline-none focus:border-brand/60"
                />
              )}
            </div>
          ))}
        </div>

        <div className="mt-2 p-3 bg-surface-2 rounded-lg border border-surface-3 text-[10px] text-slate-600">
          <AlertTriangle size={9} className="inline mr-1" />
          Credentials are encrypted at rest using AES-256. Never stored in plain text.
        </div>

        <div className="flex justify-end gap-2 mt-4">
          <Btn onClick={onClose}>Cancel</Btn>
          <Btn variant="primary" onClick={() => onSave(values)}>
            <Check size={12} /> Save & connect
          </Btn>
        </div>
      </div>
    </div>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function IntegrationsPage() {
  const qc = useQueryClient()
  const [category, setCategory] = useState('All')
  const [configuring, setConfiguring] = useState<typeof CATALOG[0] | null>(null)
  const [testing, setTesting] = useState<string | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['integrations'],
    queryFn: () => integrations.list(),
    staleTime: 60_000,
  })

  const create = useMutation({
    mutationFn: (d: any) => integrations.create(d),
    onSuccess: () => {
      toast.success('Integration connected')
      qc.invalidateQueries({ queryKey: ['integrations'] })
      setConfiguring(null)
    },
    onError: () => toast.error('Failed to connect'),
  })

  const del = useMutation({
    mutationFn: (id: string) => integrations.delete(id),
    onSuccess: () => {
      toast.success('Integration removed')
      qc.invalidateQueries({ queryKey: ['integrations'] })
    },
  })

  const testIntegration = async (id: string) => {
    setTesting(id)
    try {
      await integrations.test(id)
      toast.success('Test message sent successfully')
    } catch {
      toast.error('Test failed — check configuration')
    } finally {
      setTesting(null)
    }
  }

  const connectedIds = new Set((data?.integrations ?? []).map((i: any) => i.type))

  const filtered = CATALOG.filter(c => category === 'All' || c.category === category)

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <PageHeader
        title="Integrations"
        subtitle="Connect ObserveX to your notification, on-call, cloud, and code platforms"
      />

      {/* Category filter */}
      <div className="flex gap-1 px-4 py-3 border-b border-surface-3 flex-wrap flex-shrink-0">
        {CATEGORIES.map(cat => (
          <button key={cat} onClick={() => setCategory(cat)}
            className={clsx('px-3 py-1.5 text-xs rounded-lg transition-colors',
              category === cat ? 'bg-brand/20 text-brand' : 'text-slate-500 hover:text-slate-300')}>
            {cat}
          </button>
        ))}
        <div className="ml-auto flex items-center gap-2 text-[10px] text-slate-600">
          <Zap size={10} className="text-ok" />
          {connectedIds.size} of {CATALOG.length} connected
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-4">
        {isLoading && <div className="flex justify-center py-12"><Spinner size={20} /></div>}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {filtered.map(integration => (
            <IntegrationCard
              key={integration.id}
              integration={integration}
              configured={connectedIds.has(integration.id)}
              testing={testing === integration.id}
              onConfigure={() => setConfiguring(integration)}
              onTest={() => testIntegration(integration.id)}
              onDelete={() => del.mutate(integration.id)}
            />
          ))}
        </div>
      </div>

      {configuring && (
        <ConfigureModal
          integration={configuring}
          onSave={config => create.mutate({ type: configuring.id, config })}
          onClose={() => setConfiguring(null)} />
      )}
    </div>
  )
}
