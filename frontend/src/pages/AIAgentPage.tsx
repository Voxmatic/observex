// AIAgentPage.tsx — Full AI Monitoring Agent UI
// Shows real-time decisions, action engine, multi-model support, NL interface, audit trail

import { useState, useEffect, useRef } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { http } from '@/lib/api'
import { Send, CheckCircle, XCircle, Brain, Zap, Shield, AlertTriangle, Activity, Settings, RefreshCw, Terminal } from 'lucide-react'
import toast from 'react-hot-toast'

// Static demo data matching our agent's decisions
const DECISIONS = [
  {
    id: 'dec-001', status: 'PENDING', analysisType: 'RULE', confidence: 0.94, timestamp: '19:47:23',
    rootCause: 'postgres-primary', blastRadius: ['user-service', 'checkout-service', 'api-gateway', 'payment-service'],
    analysis: '[Rule: R001] DB Connection Pool Exhaustion — Confidence: 94%\nRoot cause: postgres-primary connection pool 200/200 at capacity. All downstream services timing out waiting for available connections. No recent deployment detected.',
    actions: [
      { id: 'act-001-1', type: 'notify_oncall', target: 'oncall-db', confidence: 0.94, status: 'PENDING', reason: 'DB connection pool exhausted — immediate human intervention required', params: { urgency: 'high', runbook: 'db-connection-recovery' } },
      { id: 'act-001-2', type: 'open_incident', target: 'INC-843', confidence: 0.99, status: 'PENDING', reason: 'Critical infrastructure incident', params: { severity: 'critical' } },
      { id: 'act-001-3', type: 'capture_snapshot', target: 'postgres-primary', confidence: 0.99, status: 'PENDING', reason: 'Capture diagnostic snapshot for post-mortem', params: {} },
    ],
  },
  {
    id: 'dec-002', status: 'APPROVED', analysisType: 'RULE', confidence: 0.88, timestamp: '19:45:11',
    rootCause: 'ml-inference', blastRadius: ['recommendation-svc'],
    analysis: '[Rule: R002] CPU Saturation → Scale Deployment\nCPU sustained at 94% for >10 minutes. Queue depth 247 pending requests. Scaling from 2→4 replicas will reduce P99 by ~60%.',
    actions: [
      { id: 'act-002-1', type: 'scale_deployment', target: 'ml-inference', confidence: 0.88, status: 'EXECUTED', reason: 'CPU at 94% (threshold 85%) — scale up to handle load', params: { replicas: '+2', max_replicas: 10 }, outcome: 'Scaled ml-inference from 2→4 replicas at 19:45:23. New pods starting.' },
    ],
  },
  {
    id: 'dec-003', status: 'EXECUTED', analysisType: 'RULE', confidence: 0.91, timestamp: '18:56:00',
    rootCause: 'api-gateway', blastRadius: [],
    analysis: '[Rule: R004] Post-Deploy Regression → Rollback\nError rate spiked +420% within 12 minutes of api-gateway v2.4.1 deployment. Classic regression pattern. Automatic rollback to v2.4.0.',
    actions: [
      { id: 'act-003-1', type: 'rollback_deployment', target: 'api-gateway', confidence: 0.91, status: 'EXECUTED', reason: 'Error rate spiked within 15 minutes of deployment — likely regression', params: { versions_back: 1 }, outcome: 'Rolled back api-gateway from v2.4.1 to v2.4.0. Error rate stabilizing.' },
    ],
  },
  {
    id: 'dec-004', status: 'REJECTED', analysisType: 'LLM', confidence: 0.72, timestamp: '17:30:00',
    rootCause: 'notification-svc', blastRadius: [],
    analysis: 'LLM Analysis: notification-svc showing elevated latency (84ms vs 40ms baseline). May indicate upstream Kafka broker issue. Monitoring recommended before action.',
    actions: [
      { id: 'act-004-1', type: 'restart_pod', target: 'notification-svc', confidence: 0.72, status: 'REJECTED', reason: 'Latency 2x baseline — preemptive restart', params: {}, outcome: 'Rejected by engineer — monitoring instead' },
    ],
  },
]

const RULES = [
  { id: 'R001', name: 'DB Connection Pool Exhaustion', confidence: 94, action: 'notify_oncall + open_incident', triggers: 47, active: true },
  { id: 'R002', name: 'CPU Saturation → Scale', confidence: 88, action: 'scale_deployment', triggers: 12, active: true },
  { id: 'R003', name: 'OOM Kill → Restart', confidence: 97, action: 'restart_pod', triggers: 8, active: true },
  { id: 'R004', name: 'Post-Deploy Regression → Rollback', confidence: 91, action: 'rollback_deployment', triggers: 3, active: true },
  { id: 'R005', name: 'Error Rate Cascade → Incident', confidence: 85, action: 'open_incident', triggers: 28, active: true },
  { id: 'R006', name: 'Memory Leak → Predictive Restart', confidence: 82, action: 'capture_snapshot + restart', triggers: 5, active: true },
  { id: 'R007', name: 'CrashLoopBackOff → Investigate', confidence: 96, action: 'capture_snapshot + notify', triggers: 4, active: true },
]

const MODELS = [
  { id: 'claude-haiku', name: 'Claude Haiku 4.5', provider: 'Anthropic', status: 'active', latency: '380ms', cost: '$0.25/1M in', icon: '🟣', primary: true },
  { id: 'claude-sonnet', name: 'Claude Sonnet 4.6', provider: 'Anthropic', status: 'available', latency: '1240ms', cost: '$3/1M in', icon: '🟣', primary: false },
  { id: 'llama3', name: 'Llama3 (Local)', provider: 'Ollama', status: 'available', latency: '890ms', cost: 'FREE', icon: '🦙', primary: false },
  { id: 'gpt-4o-mini', name: 'GPT-4o Mini', provider: 'OpenAI', status: 'available', latency: '420ms', cost: '$0.15/1M in', icon: '🟢', primary: false },
]

const actionColor: Record<string, string> = {
  notify_oncall: '#f5a623',
  open_incident: '#ff4d6a',
  scale_deployment: '#0fcf8a',
  restart_pod: '#a855f7',
  rollback_deployment: '#3d9bff',
  capture_snapshot: '#6c72ff',
  run_playbook: '#00d4ff',
  silence_alert: '#546a88',
}

const statusColors: Record<string, string> = {
  PENDING: '#f5a623', APPROVED: '#6c72ff', EXECUTED: '#0fcf8a', REJECTED: '#ff4d6a', FAILED: '#ff4d6a'
}

type Tab = 'decisions' | 'rules' | 'models' | 'chat' | 'config'

export default function AIAgentPage() {
  const [tab, setTab] = useState<Tab>('decisions')
  const [mode, setMode] = useState<'AUTO' | 'SUGGEST' | 'MANUAL'>('SUGGEST')
  const [chatInput, setChatInput] = useState('')
  const [chatHistory, setChatHistory] = useState<{ role: 'user'|'ai', content: string, ts: string }[]>([
    { role: 'ai', content: '🧠 ObserveX AI Agent online. I\'m monitoring your infrastructure in real-time.\n\n**Current situation:** 2 active problems detected.\n\n• `postgres-primary` — Connection pool exhausted (200/200). Cascade affecting 4 services. Recommended: notify oncall + open incident.\n• `ml-inference` — CPU 94% sustained. Auto-scaled from 2→4 replicas.\n\nWhat would you like to know?', ts: '19:47:00' },
  ])
  const [decisions, setDecisions] = useState(DECISIONS)
  const chatRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight
  }, [chatHistory])

  const approve = (decId: string, actId: string) => {
    setDecisions(d => d.map(dec => dec.id === decId ? {
      ...dec,
      actions: dec.actions.map(a => a.id === actId ? { ...a, status: 'EXECUTED', outcome: `Action executed at ${new Date().toLocaleTimeString()}` } : a)
    } : dec))
    toast.success('Action approved and executed')
  }

  const reject = (decId: string, actId: string) => {
    setDecisions(d => d.map(dec => dec.id === decId ? {
      ...dec,
      actions: dec.actions.map(a => a.id === actId ? { ...a, status: 'REJECTED', outcome: 'Rejected by engineer' } : a)
    } : dec))
    toast('Action rejected')
  }

  const sendChat = () => {
    if (!chatInput.trim()) return
    const userMsg = chatInput.trim()
    setChatInput('')
    const ts = new Date().toLocaleTimeString()
    setChatHistory(h => [...h, { role: 'user', content: userMsg, ts }])
    
    // Simulate AI response
    setTimeout(() => {
      const responses: Record<string, string> = {
        default: '**Analysis:** Based on current signals, the primary concern is the postgres-primary connection pool exhaustion. This is causing cascade failures across 4 downstream services.\n\n**My confidence:** 94% this is not deployment-related (last deploy 3h ago).\n\n**Recommended immediate actions:**\n1. Notify on-call DB team\n2. Temporarily increase max_connections to 250\n3. Identify and kill long-running queries\n4. Consider adding read replica',
        root: '**Root cause (94% confidence):** `postgres-primary` connection pool\n\nThe cascade chain:\n```\npostgres-primary (pool exhausted)\n  └→ user-service (error rate +2800%)\n    └→ checkout-service (cascade failure)\n      └→ api-gateway (error rate +68%)\n```\n\nAnomalous behavior started at 19:24:11 UTC. No recent deployment matches (last at 14:30). Likely causes: query holding connections + traffic spike.',
        scale: '**Scaling recommendation for ml-inference:**\n\n• Current: 2 replicas, CPU 94%\n• Recommended: 4 replicas (HPA max: 10)\n• Expected CPU after scale: ~47%\n• Expected P99 improvement: ~60%\n• Monthly cost increase: +$48/mo\n\n✅ I already auto-scaled this — check Kubernetes workloads.',
      }
      const lower = userMsg.toLowerCase()
      let resp = responses.default
      if (lower.includes('root') || lower.includes('cause')) resp = responses.root
      if (lower.includes('scale') || lower.includes('ml')) resp = responses.scale
      setChatHistory(h => [...h, { role: 'ai', content: resp, ts: new Date().toLocaleTimeString() }])
    }, 800)
  }

  const TABS: { id: Tab; label: string; icon: any }[] = [
    { id: 'decisions', label: 'Decisions', icon: Brain },
    { id: 'rules', label: 'Expert Rules', icon: Zap },
    { id: 'models', label: 'AI Models', icon: Activity },
    { id: 'chat', label: 'Chat Interface', icon: Terminal },
    { id: 'config', label: 'Configuration', icon: Settings },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      {/* Header */}
      <div style={{ padding: '16px 20px 0', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
          <div style={{ width: 42, height: 42, borderRadius: 12, background: 'linear-gradient(135deg, #6c72ff, #a855f7)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 20 }}>🧠</div>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif' }}>AI Monitoring Agent v2.0</div>
            <div style={{ fontSize: 11, color: '#546a88' }}>Causal topology analysis · Multi-model LLM · 7 expert rules · Action engine · Zero data egress mode</div>
          </div>
          {/* Mode selector */}
          <div style={{ display: 'flex', background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, overflow: 'hidden' }}>
            {(['AUTO', 'SUGGEST', 'MANUAL'] as const).map(m => (
              <button key={m} onClick={() => setMode(m)}
                style={{ padding: '6px 14px', border: 'none', background: mode === m ? (m === 'AUTO' ? '#0fcf8a' : m === 'SUGGEST' ? '#6c72ff' : '#f5a623') : 'transparent', color: mode === m ? '#fff' : '#546a88', fontSize: 11, fontWeight: 700, cursor: 'pointer' }}>
                {m}
              </button>
            ))}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 12px', background: 'rgba(15,207,138,.08)', border: '1px solid rgba(15,207,138,.25)', borderRadius: 8 }}>
            <div style={{ width: 7, height: 7, borderRadius: '50%', background: '#0fcf8a', animation: 'pulse 2s infinite' }} />
            <span style={{ fontSize: 11, color: '#0fcf8a', fontWeight: 700 }}>Active · {mode} mode</span>
          </div>
        </div>

        {/* Stats strip */}
        <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
          {[
            { l: 'Decisions Today', v: 14, c: '#8b90ff' },
            { l: 'Auto-Remediated', v: 9, c: '#0fcf8a' },
            { l: 'Escalated', v: 3, c: '#f5a623' },
            { l: 'Avg Confidence', v: '87%', c: '#6c72ff' },
            { l: 'Signals Processed', v: '24.8K', c: '#546a88' },
          ].map(s => (
            <div key={s.l} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 8, padding: '6px 14px' }}>
              <div style={{ fontSize: 9, color: '#546a88', textTransform: 'uppercase', letterSpacing: '.06em' }}>{s.l}</div>
              <div style={{ fontSize: 16, fontWeight: 800, color: s.c, fontFamily: 'Syne, sans-serif' }}>{s.v}</div>
            </div>
          ))}
        </div>

        {/* Tabs */}
        <div style={{ display: 'flex', gap: 0 }}>
          {TABS.map(t => {
            const Icon = t.icon
            return (
              <button key={t.id} onClick={() => setTab(t.id)}
                style={{ padding: '8px 16px', background: 'none', border: 'none', borderBottom: tab === t.id ? '2px solid #6c72ff' : '2px solid transparent', color: tab === t.id ? '#8b90ff' : '#546a88', cursor: 'pointer', fontWeight: 600, fontSize: 12, display: 'flex', alignItems: 'center', gap: 6 }}>
                <Icon size={13} />
                {t.label}
              </button>
            )
          })}
        </div>
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: 'hidden' }}>
        {/* DECISIONS TAB */}
        {tab === 'decisions' && (
          <div style={{ height: '100%', overflowY: 'auto', padding: 20 }}>
            {decisions.map(d => (
              <div key={d.id} style={{ background: d.status === 'PENDING' ? 'rgba(245,166,35,.04)' : 'rgba(17,31,53,.7)', border: `1px solid ${d.status === 'PENDING' ? 'rgba(245,166,35,.4)' : '#1a2d4a'}`, borderRadius: 12, padding: 16, marginBottom: 12 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 10 }}>
                  <span style={{ background: `${statusColors[d.status]}22`, color: statusColors[d.status], fontSize: 10, fontWeight: 800, padding: '2px 8px', borderRadius: 12 }}>{d.status}</span>
                  <span style={{ background: d.analysisType === 'RULE' ? 'rgba(108,114,255,.2)' : 'rgba(168,85,247,.2)', color: d.analysisType === 'RULE' ? '#8b90ff' : '#c084fc', fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 12 }}>{d.analysisType}</span>
                  <span style={{ fontSize: 12, fontWeight: 700, color: '#f0f6ff', flex: 1 }}>Root cause: <span style={{ fontFamily: 'JetBrains Mono, monospace', color: '#ff4d6a' }}>{d.rootCause}</span></span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                    <div style={{ width: 60, height: 5, background: '#0b1628', borderRadius: 3, overflow: 'hidden' }}>
                      <div style={{ height: '100%', width: `${d.confidence*100}%`, background: d.confidence >= 0.85 ? '#0fcf8a' : '#f5a623' }} />
                    </div>
                    <span style={{ fontSize: 11, color: '#8fa8cc' }}>{(d.confidence*100).toFixed(0)}%</span>
                  </div>
                  <span style={{ fontSize: 10, color: '#546a88', fontFamily: 'JetBrains Mono, monospace' }}>{d.timestamp}</span>
                </div>

                {/* Analysis */}
                <div style={{ fontSize: 11, color: '#8fa8cc', lineHeight: 1.6, marginBottom: 10, background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 8, padding: '8px 12px', fontFamily: 'JetBrains Mono, monospace' }}>
                  {d.analysis}
                </div>

                {/* Blast radius */}
                {d.blastRadius.length > 0 && (
                  <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 10 }}>
                    <span style={{ fontSize: 10, color: '#546a88' }}>Blast radius:</span>
                    {d.blastRadius.map(e => (
                      <span key={e} style={{ background: '#182844', color: '#8fa8cc', fontSize: 10, padding: '2px 8px', borderRadius: 8, fontFamily: 'JetBrains Mono, monospace' }}>{e}</span>
                    ))}
                  </div>
                )}

                {/* Actions */}
                {d.actions.map(a => (
                  <div key={a.id} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '8px 12px', background: '#0b1628', border: `1px solid ${actionColor[a.type] || '#1a2d4a'}22`, borderRadius: 8, marginBottom: 5 }}>
                    <span style={{ background: `${actionColor[a.type] || '#546a88'}22`, color: actionColor[a.type] || '#546a88', fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 10, flexShrink: 0 }}>{a.type}</span>
                    <span style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 11, color: '#6c72ff', flexShrink: 0 }}>{a.target}</span>
                    <span style={{ fontSize: 11, color: '#8fa8cc', flex: 1 }}>{a.reason}</span>
                    {a.outcome && <span style={{ fontSize: 11, color: '#0fcf8a', fontStyle: 'italic' }}>{a.outcome}</span>}
                    {a.status === 'PENDING' && (
                      <div style={{ display: 'flex', gap: 6 }}>
                        <button onClick={() => approve(d.id, a.id)} style={{ display: 'flex', alignItems: 'center', gap: 4, background: '#0fcf8a', color: '#fff', border: 'none', padding: '4px 12px', borderRadius: 7, fontSize: 11, fontWeight: 700, cursor: 'pointer' }}>
                          <CheckCircle size={11} /> Approve
                        </button>
                        <button onClick={() => reject(d.id, a.id)} style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'transparent', color: '#ff4d6a', border: '1px solid rgba(255,77,106,.3)', padding: '4px 12px', borderRadius: 7, fontSize: 11, cursor: 'pointer' }}>
                          <XCircle size={11} /> Reject
                        </button>
                      </div>
                    )}
                    {a.status !== 'PENDING' && (
                      <span style={{ background: `${statusColors[a.status]}22`, color: statusColors[a.status], fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 10 }}>{a.status}</span>
                    )}
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}

        {/* RULES TAB */}
        {tab === 'rules' && (
          <div style={{ height: '100%', overflowY: 'auto', padding: 20 }}>
            <div style={{ marginBottom: 14, fontSize: 12, color: '#546a88' }}>
              7 expert rules that run deterministically before LLM. High confidence (82–97%). Fast, no API calls needed.
            </div>
            {RULES.map((r, i) => (
              <div key={r.id} style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '12px 16px', background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 10, marginBottom: 6 }}>
                <div style={{ width: 28, height: 28, borderRadius: 8, background: '#182844', border: '1px solid #254060', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, fontWeight: 700, color: '#6c72ff', fontFamily: 'JetBrains Mono, monospace' }}>{r.id}</div>
                <div style={{ width: 6, height: 6, borderRadius: '50%', background: r.active ? '#0fcf8a' : '#546a88', flexShrink: 0 }} />
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 13, fontWeight: 700, color: '#f0f6ff', marginBottom: 3 }}>{r.name}</div>
                  <div style={{ fontSize: 11, color: '#546a88', fontFamily: 'JetBrains Mono, monospace' }}>→ {r.action}</div>
                </div>
                <div style={{ textAlign: 'right' }}>
                  <div style={{ fontSize: 16, fontWeight: 800, color: r.confidence >= 90 ? '#0fcf8a' : '#f5a623', fontFamily: 'Syne, sans-serif' }}>{r.confidence}%</div>
                  <div style={{ fontSize: 10, color: '#546a88' }}>{r.triggers} triggers</div>
                </div>
                <div style={{ height: 40, width: 1, background: '#1a2d4a' }} />
                <button style={{ background: r.active ? 'rgba(15,207,138,.1)' : '#0b1628', border: `1px solid ${r.active ? 'rgba(15,207,138,.3)' : '#1a2d4a'}`, color: r.active ? '#0fcf8a' : '#546a88', padding: '4px 12px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}>
                  {r.active ? '● Active' : '○ Inactive'}
                </button>
              </div>
            ))}
          </div>
        )}

        {/* MODELS TAB */}
        {tab === 'models' && (
          <div style={{ height: '100%', overflowY: 'auto', padding: 20 }}>
            <div style={{ marginBottom: 14, fontSize: 12, color: '#546a88' }}>
              Multi-model support. Primary model used for all LLM analysis. Expert rules always run first (no API calls needed).
              <span style={{ display: 'inline-block', marginLeft: 8, background: 'rgba(15,207,138,.12)', color: '#0fcf8a', fontSize: 10, padding: '2px 8px', borderRadius: 8 }}>★ Ollama = zero data egress, 100% on-premise</span>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12 }}>
              {MODELS.map(m => (
                <div key={m.id} style={{ background: 'rgba(17,31,53,.7)', border: `1px solid ${m.primary ? 'rgba(108,114,255,.4)' : '#1a2d4a'}`, borderRadius: 12, padding: 16 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
                    <span style={{ fontSize: 24 }}>{m.icon}</span>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 13, fontWeight: 700, color: '#f0f6ff' }}>{m.name}</div>
                      <div style={{ fontSize: 11, color: '#546a88' }}>{m.provider}</div>
                    </div>
                    {m.primary && <span style={{ background: 'rgba(108,114,255,.2)', color: '#8b90ff', fontSize: 10, fontWeight: 700, padding: '2px 8px', borderRadius: 10 }}>PRIMARY</span>}
                    <div style={{ width: 8, height: 8, borderRadius: '50%', background: m.status === 'active' ? '#0fcf8a' : '#546a88' }} />
                  </div>
                  <div style={{ display: 'flex', gap: 12, fontSize: 11 }}>
                    <div><span style={{ color: '#546a88' }}>Latency: </span><span style={{ color: '#c8d8ef', fontFamily: 'JetBrains Mono, monospace' }}>{m.latency}</span></div>
                    <div><span style={{ color: '#546a88' }}>Cost: </span><span style={{ color: m.cost === 'FREE' ? '#0fcf8a' : '#c8d8ef', fontWeight: m.cost === 'FREE' ? 700 : 400 }}>{m.cost}</span></div>
                  </div>
                  {!m.primary && (
                    <button style={{ width: '100%', marginTop: 10, background: '#182844', border: '1px solid #254060', color: '#8fa8cc', padding: '6px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}>Set as Primary</button>
                  )}
                  {m.id === 'llama3' && (
                    <div style={{ marginTop: 8, padding: '6px 10px', background: 'rgba(15,207,138,.07)', border: '1px solid rgba(15,207,138,.2)', borderRadius: 8, fontSize: 10, color: '#0fcf8a' }}>
                      ★ UNIQUE: Zero data egress · All analysis stays on-premise · Ollama runs on your GPU
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* CHAT TAB */}
        {tab === 'chat' && (
          <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
            <div ref={chatRef} style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
              {chatHistory.map((m, i) => (
                <div key={i} style={{ display: 'flex', gap: 10, marginBottom: 12, justifyContent: m.role === 'user' ? 'flex-end' : 'flex-start' }}>
                  {m.role === 'ai' && <div style={{ width: 28, height: 28, borderRadius: 8, background: 'linear-gradient(135deg, #6c72ff, #a855f7)', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, fontSize: 14 }}>🧠</div>}
                  <div style={{ maxWidth: '80%', padding: '10px 14px', borderRadius: m.role === 'ai' ? '4px 12px 12px 12px' : '12px 4px 12px 12px', background: m.role === 'ai' ? 'rgba(108,114,255,.12)' : 'rgba(17,31,53,.9)', border: `1px solid ${m.role === 'ai' ? 'rgba(108,114,255,.25)' : '#254060'}`, fontSize: 12, color: '#c8d8ef', lineHeight: 1.6 }}>
                    <pre style={{ margin: 0, fontFamily: 'inherit', whiteSpace: 'pre-wrap' }}>{m.content}</pre>
                    <div style={{ fontSize: 10, color: '#546a88', marginTop: 5 }}>{m.ts}</div>
                  </div>
                </div>
              ))}
            </div>
            <div style={{ padding: 16, borderTop: '1px solid #1a2d4a', flexShrink: 0 }}>
              <div style={{ display: 'flex', gap: 8 }}>
                <div style={{ flex: 1, display: 'flex', alignItems: 'center', background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '8px 14px', gap: 8 }}>
                  <Terminal size={14} style={{ color: '#546a88', flexShrink: 0 }} />
                  <input value={chatInput} onChange={e => setChatInput(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && sendChat()}
                    placeholder="Ask about incidents, root causes, recommendations..."
                    style={{ background: 'none', border: 'none', outline: 'none', fontSize: 12, color: '#c8d8ef', flex: 1 }} />
                </div>
                <button onClick={sendChat} style={{ background: '#6c72ff', color: '#fff', border: 'none', padding: '8px 16px', borderRadius: 10, cursor: 'pointer' }}>
                  <Send size={14} />
                </button>
              </div>
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                {['What is the root cause?', 'Scale ml-inference', 'Show blast radius', 'Run DB recovery playbook'].map(q => (
                  <button key={q} onClick={() => { setChatInput(q); setTimeout(sendChat, 50) }}
                    style={{ background: '#182844', border: '1px solid #254060', color: '#8fa8cc', padding: '4px 10px', borderRadius: 8, fontSize: 10, cursor: 'pointer', whiteSpace: 'nowrap' }}>{q}</button>
                ))}
              </div>
            </div>
          </div>
        )}

        {/* CONFIG TAB */}
        {tab === 'config' && (
          <div style={{ height: '100%', overflowY: 'auto', padding: 20 }}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
              {[
                { label: 'Agent Mode', value: mode, type: 'select', options: ['AUTO', 'SUGGEST', 'MANUAL'], hint: 'AUTO: executes actions automatically. SUGGEST: shows recommendations. MANUAL: audit-only.' },
                { label: 'Confidence Threshold', value: '85%', type: 'range', hint: 'Minimum confidence to take action or suggest action.' },
                { label: 'Cooldown Period', value: '10 min', type: 'text', hint: 'Minimum time between actions on the same service.' },
                { label: 'Max Actions/Hour', value: '30', type: 'number', hint: 'Safety limit on automated actions per hour.' },
                { label: 'LLM Fallback', value: 'Enabled', type: 'toggle', hint: 'Use LLM when expert rules cannot classify the signal.' },
                { label: 'Zero Egress Mode', value: 'Ollama', type: 'select', options: ['Disabled', 'Ollama', 'On-prem'], hint: '★ UNIQUE: All LLM analysis stays on-premise. No data leaves your network.' },
              ].map(c => (
                <div key={c.label} style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 10, padding: 14 }}>
                  <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 8 }}>{c.label}</div>
                  <div style={{ fontSize: 14, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 6 }}>{c.value}</div>
                  <div style={{ fontSize: 11, color: '#546a88', lineHeight: 1.5 }}>{c.hint}</div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
