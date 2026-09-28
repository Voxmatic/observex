// ErrorsInboxPage.tsx — New Relic parity: unified cross-service error grouping
import { useState } from 'react'
import { Bug, Users, Clock, ChevronRight, XCircle, CheckCircle, Eye, GitBranch } from 'lucide-react'

const ERRORS = [
  { id: 'err-001', fingerprint: 'NullPointerException in UserService.getProfile()', count: 1842, users: 312, service: 'user-service', version: 'v2.4.0', firstSeen: '2h ago', lastSeen: '12s ago', status: 'unresolved', severity: 'CRITICAL',
    stack: `java.lang.NullPointerException: Cannot invoke "User.getProfile()" because "user" is null
  at UserService.getProfile(UserService.java:142)
  at UserController.handleGetProfile(UserController.java:67)
  at HttpHandler.dispatch(HttpHandler.java:231)`, assignee: null },
  { id: 'err-002', fingerprint: 'TimeoutError: Database connection timeout after 30000ms', count: 847, users: 198, service: 'checkout-service', version: 'v2.5.0', firstSeen: '23m ago', lastSeen: '3s ago', status: 'unresolved', severity: 'HIGH',
    stack: `TimeoutError: Database connection timeout after 30000ms
  at ConnectionPool.acquire(pool.js:84)
  at CheckoutRepository.findOrder(checkout.repo.js:156)
  at CheckoutService.processOrder(checkout.svc.js:89)`, assignee: 'sarah.chen' },
  { id: 'err-003', fingerprint: 'PaymentGateway: DECLINED — insufficient_funds', count: 284, users: 284, service: 'payment-service', version: 'v3.1.2', firstSeen: '1h ago', lastSeen: '1m ago', status: 'unresolved', severity: 'MEDIUM',
    stack: `PaymentError: DECLINED — insufficient_funds
  at PaymentGateway.charge(gateway.js:44)
  at PaymentService.processPayment(payment.svc.js:112)`, assignee: 'ops-team' },
  { id: 'err-004', fingerprint: 'Redis ECONNREFUSED 127.0.0.1:6379', count: 128, users: 0, service: 'ml-inference', version: 'v1.8.3', firstSeen: '3h ago', lastSeen: '45m ago', status: 'resolved', severity: 'HIGH',
    stack: `Error: connect ECONNREFUSED 127.0.0.1:6379
  at TCPConnectWrap.afterConnect
  at RedisClient.connect(redis.js:211)`, assignee: 'platform-team' },
  { id: 'err-005', fingerprint: '404 Not Found: GET /api/v1/users/:id/recommendations', count: 74, users: 74, service: 'recommendation-svc', version: 'v2.0.1', firstSeen: '5h ago', lastSeen: '18m ago', status: 'ignored', severity: 'LOW',
    stack: `NotFoundError: Resource not found
  at Router.handle(router.js:67)
  at RecommendationController.getForUser(rec.ctrl.js:33)`, assignee: null },
]

const sevC: Record<string, string> = { CRITICAL: '#ff4d6a', HIGH: '#f5a623', MEDIUM: '#a855f7', LOW: '#546a88' }
const statC: Record<string, string> = { unresolved: '#ff4d6a', resolved: '#0fcf8a', ignored: '#546a88' }

export default function ErrorsInboxPage() {
  const [selected, setSelected] = useState(ERRORS[0])
  const [filter, setFilter] = useState<'ALL'|'unresolved'|'resolved'|'ignored'>('ALL')
  const [errors, setErrors] = useState(ERRORS)

  const shown = errors.filter(e => filter === 'ALL' || e.status === filter)
  const setStatus = (id: string, status: string) => setErrors(e => e.map(x => x.id === id ? { ...x, status } : x))

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', background: '#070f1e', color: '#c8d8ef', overflow: 'hidden' }}>
      <div style={{ padding: '16px 20px 12px', borderBottom: '1px solid #1a2d4a', flexShrink: 0 }}>
        <div style={{ fontSize: 20, fontWeight: 800, color: '#f0f6ff', fontFamily: 'Syne, sans-serif', marginBottom: 3 }}>Errors Inbox</div>
        <div style={{ fontSize: 12, color: '#546a88', marginBottom: 12 }}>Unified cross-service error grouping · Triage, assign, resolve · Stack trace correlation</div>
        <div style={{ display: 'flex', gap: 12, marginBottom: 10 }}>
          {[{ l: 'Total Errors', v: errors.reduce((a,e)=>a+e.count,0).toLocaleString(), c: '#ff4d6a' },
            { l: 'Affected Users', v: errors.reduce((a,e)=>a+e.users,0).toLocaleString(), c: '#f5a623' },
            { l: 'Error Groups', v: errors.length, c: '#6c72ff' },
            { l: 'Unresolved', v: errors.filter(e=>e.status==='unresolved').length, c: '#ff4d6a' }].map(k => (
            <div key={k.l} style={{ background: '#0b1628', border: '1px solid #1a2d4a', borderRadius: 10, padding: '7px 14px' }}>
              <div style={{ fontSize: 10, color: '#546a88' }}>{k.l}</div>
              <div style={{ fontSize: 18, fontWeight: 800, color: k.c, fontFamily: 'Syne, sans-serif' }}>{k.v}</div>
            </div>
          ))}
          <div style={{ marginLeft: 'auto', display: 'flex', gap: 6, alignItems: 'center' }}>
            {(['ALL','unresolved','resolved','ignored'] as const).map(f => (
              <button key={f} onClick={() => setFilter(f)} style={{ padding: '5px 12px', borderRadius: 20, border: `1px solid ${filter===f ? statC[f]||'#6c72ff' : '#1a2d4a'}`, background: filter===f ? `${statC[f]||'#6c72ff'}22` : 'transparent', color: filter===f ? statC[f]||'#8b90ff' : '#546a88', fontSize: 11, fontWeight: 600, cursor: 'pointer' }}>
                {f}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
        {/* Error list */}
        <div style={{ width: 360, borderRight: '1px solid #1a2d4a', overflowY: 'auto' }}>
          {shown.map(e => (
            <div key={e.id} onClick={() => setSelected(e)} style={{ padding: '12px 14px', cursor: 'pointer', borderBottom: '1px solid #0d1a2e', borderLeft: `3px solid ${selected.id===e.id ? sevC[e.severity] : 'transparent'}`, background: selected.id===e.id ? 'rgba(108,114,255,.04)' : 'transparent' }}>
              <div style={{ display: 'flex', gap: 6, alignItems: 'flex-start', marginBottom: 6 }}>
                <Bug size={12} style={{ color: sevC[e.severity], marginTop: 1, flexShrink: 0 }} />
                <div style={{ flex: 1, fontSize: 11, fontWeight: 700, color: '#f0f6ff', lineHeight: 1.3, fontFamily: 'JetBrains Mono, monospace' }}>{e.fingerprint}</div>
              </div>
              <div style={{ display: 'flex', gap: 10, fontSize: 10, color: '#546a88' }}>
                <span style={{ color: sevC[e.severity] }}>{e.severity}</span>
                <span style={{ color: '#6c72ff', fontFamily: 'JetBrains Mono, monospace' }}>{e.service}</span>
                <span style={{ color: statC[e.status], fontWeight: 700 }}>{e.status}</span>
                <span style={{ marginLeft: 'auto', display: 'flex', gap: 4, alignItems: 'center' }}><Users size={9}/>{e.users} · <Bug size={9}/>{e.count.toLocaleString()}</span>
              </div>
            </div>
          ))}
        </div>

        {/* Detail */}
        <div style={{ flex: 1, overflowY: 'auto', padding: 20 }}>
          <div style={{ background: `rgba(${selected.severity==='CRITICAL'?'255,77,106':'17,31,53'},.07)`, border: `1px solid ${sevC[selected.severity]}44`, borderRadius: 14, padding: 16, marginBottom: 14 }}>
            <div style={{ display: 'flex', gap: 10, marginBottom: 10 }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 13, fontWeight: 700, color: '#f0f6ff', marginBottom: 6, lineHeight: 1.3 }}>{selected.fingerprint}</div>
                <div style={{ display: 'flex', gap: 10 }}>
                  <span style={{ background: `${sevC[selected.severity]}22`, color: sevC[selected.severity], fontSize: 10, fontWeight: 800, padding: '2px 8px', borderRadius: 10 }}>{selected.severity}</span>
                  <span style={{ color: '#6c72ff', fontSize: 11, fontFamily: 'JetBrains Mono, monospace' }}>{selected.service}</span>
                  <span style={{ color: '#546a88', fontSize: 11 }}>{selected.version}</span>
                  <span style={{ color: statC[selected.status], fontSize: 11, fontWeight: 700 }}>{selected.status}</span>
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8 }}>
                {selected.status !== 'resolved' && <button onClick={() => setStatus(selected.id, 'resolved')} style={{ display: 'flex', gap: 5, alignItems: 'center', background: 'rgba(15,207,138,.1)', border: '1px solid rgba(15,207,138,.25)', color: '#0fcf8a', padding: '6px 12px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}><CheckCircle size={12}/>Resolve</button>}
                {selected.status !== 'ignored' && <button onClick={() => setStatus(selected.id, 'ignored')} style={{ background: '#0b1628', border: '1px solid #1a2d4a', color: '#546a88', padding: '6px 12px', borderRadius: 8, fontSize: 11, cursor: 'pointer' }}>Ignore</button>}
              </div>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10 }}>
              {[{ l: 'Occurrences', v: selected.count.toLocaleString(), c: '#ff4d6a' }, { l: 'Affected Users', v: selected.users.toLocaleString(), c: '#f5a623' }, { l: 'First Seen', v: selected.firstSeen, c: '#8fa8cc' }, { l: 'Last Seen', v: selected.lastSeen, c: '#0fcf8a' }].map(k => (
                <div key={k.l} style={{ background: 'rgba(7,15,30,.5)', border: '1px solid #1a2d4a', borderRadius: 8, padding: '8px 12px' }}>
                  <div style={{ fontSize: 10, color: '#546a88' }}>{k.l}</div>
                  <div style={{ fontSize: 15, fontWeight: 800, color: k.c, fontFamily: 'Syne, sans-serif' }}>{k.v}</div>
                </div>
              ))}
            </div>
          </div>

          {/* Stack trace */}
          <div style={{ background: '#080d1b', border: '1px solid #1a2d4a', borderRadius: 12, overflow: 'hidden', marginBottom: 14 }}>
            <div style={{ padding: '10px 14px', borderBottom: '1px solid #1a2d4a', fontSize: 12, fontWeight: 700, color: '#c8d8ef', background: '#0b1628' }}>Stack Trace</div>
            <pre style={{ padding: 14, margin: 0, fontFamily: 'JetBrains Mono, monospace', fontSize: 11, color: '#ff7a92', lineHeight: 1.7, overflowX: 'auto', whiteSpace: 'pre-wrap' }}>
              {selected.stack}
            </pre>
          </div>

          {/* Assignment */}
          <div style={{ background: 'rgba(17,31,53,.7)', border: '1px solid #1a2d4a', borderRadius: 12, padding: 14 }}>
            <div style={{ fontSize: 12, fontWeight: 700, color: '#c8d8ef', marginBottom: 10 }}>Assignment & Tracking</div>
            <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 10, color: '#546a88', marginBottom: 4 }}>Assignee</div>
                <div style={{ fontSize: 12, color: '#f0f6ff', fontWeight: 600 }}>{selected.assignee || 'Unassigned'}</div>
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 10, color: '#546a88', marginBottom: 4 }}>Service</div>
                <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: '#6c72ff' }}>{selected.service}</div>
              </div>
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 10, color: '#546a88', marginBottom: 4 }}>Version</div>
                <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: '#8fa8cc' }}>{selected.version}</div>
              </div>
              <button style={{ alignSelf: 'flex-end', background: '#6c72ff', color: '#fff', border: 'none', padding: '6px 14px', borderRadius: 8, fontSize: 11, fontWeight: 700, cursor: 'pointer' }}>Create Jira Ticket</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
