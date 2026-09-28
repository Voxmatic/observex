import { useState, FormEvent } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { auth } from '@/lib/api'
import { useAuth } from '@/store/auth'
import { Eye, EyeOff, Activity } from 'lucide-react'
import toast from 'react-hot-toast'

export default function LoginPage() {
  const navigate = useNavigate()
  const { setToken, loadMe, setDemoSession } = useAuth()
  const [email, setEmail]       = useState('')
  const [password, setPassword] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [loading, setLoading]   = useState(false)
  const [mode, setMode]         = useState<'login' | 'forgot'>('login')
  const [sent, setSent]         = useState(false)

  const handleLogin = async (e: FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      const res = await auth.login(email, password)
      setToken(res.token)
      await loadMe()
      navigate('/')
    } catch (err: any) {
      toast.error(err?.response?.data?.error ?? 'Invalid credentials')
    } finally {
      setLoading(false)
    }
  }

  const handleForgot = async (e: FormEvent) => {
    e.preventDefault()
    setLoading(true)
    try {
      await auth.forgotPassword(email)
      setSent(true)
    } catch {
      setSent(true) // always show success to avoid email enumeration
    } finally {
      setLoading(false)
    }
  }

  const openDemo = () => {
    setDemoSession()
    navigate('/')
  }

  return (
    <div className="min-h-screen bg-surface-0 flex items-center justify-center p-4">
      {/* Background grid */}
      <div className="fixed inset-0 opacity-[0.03]"
        style={{ backgroundImage: 'linear-gradient(hsl(217 91% 60%) 1px, transparent 1px), linear-gradient(90deg, hsl(217 91% 60%) 1px, transparent 1px)', backgroundSize: '40px 40px' }} />

      <div className="w-full max-w-sm relative animate-fade-in">
        {/* Logo */}
        <div className="flex items-center gap-3 mb-8">
          <div className="w-9 h-9 rounded-xl bg-brand/20 border border-brand/30 flex items-center justify-center">
            <Activity size={18} className="text-brand" />
          </div>
          <span className="font-display font-bold text-xl text-gradient">ObserveX</span>
        </div>

        <div className="bg-surface-1 rounded-2xl border border-surface-3 p-7">
          {mode === 'login' ? (
            <>
              <h1 className="text-lg font-semibold text-slate-100 mb-1">Sign in</h1>
              <p className="text-sm text-slate-500 mb-6">to your observability platform</p>

              <form onSubmit={handleLogin} className="space-y-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1.5">Email</label>
                  <input
                    type="email" required autoFocus
                    value={email} onChange={e => setEmail(e.target.value)}
                    placeholder="admin@observex.io"
                    className="w-full px-3 py-2.5 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-200 placeholder-slate-600 focus:outline-none focus:border-brand/60 transition-colors"
                  />
                </div>

                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1.5">Password</label>
                  <div className="relative">
                    <input
                      type={showPass ? 'text' : 'password'} required
                      value={password} onChange={e => setPassword(e.target.value)}
                      placeholder="••••••••"
                      className="w-full px-3 py-2.5 pr-10 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-200 placeholder-slate-600 focus:outline-none focus:border-brand/60 transition-colors"
                    />
                    <button type="button" onClick={() => setShowPass(!showPass)}
                      className="absolute right-3 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300">
                      {showPass ? <EyeOff size={15} /> : <Eye size={15} />}
                    </button>
                  </div>
                </div>

                <button type="submit" disabled={loading}
                  className="w-full py-2.5 bg-brand hover:bg-brand-glow text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
                  {loading ? 'Signing in…' : 'Sign in'}
                </button>
              </form>

              <button onClick={() => setMode('forgot')}
                className="mt-4 w-full text-center text-xs text-slate-500 hover:text-slate-300 transition-colors">
                Forgot password?
              </button>

              <button type="button" onClick={openDemo}
                className="mt-3 w-full py-2.5 bg-surface-2 hover:bg-surface-3 border border-surface-3 text-slate-200 text-sm font-medium rounded-lg transition-colors">
                Open frontend demo
              </button>
            </>
          ) : (
            <>
              <h1 className="text-lg font-semibold text-slate-100 mb-1">Reset password</h1>
              <p className="text-sm text-slate-500 mb-6">We'll send a reset link to your email</p>

              {sent ? (
                <div className="text-center py-4">
                  <div className="text-sm text-ok mb-2">✓ If that email exists, a reset link has been sent.</div>
                  <button onClick={() => { setMode('login'); setSent(false) }}
                    className="text-xs text-slate-400 hover:text-slate-200 transition-colors">
                    Back to sign in
                  </button>
                </div>
              ) : (
                <form onSubmit={handleForgot} className="space-y-4">
                  <div>
                    <label className="block text-xs font-medium text-slate-400 mb-1.5">Email</label>
                    <input
                      type="email" required autoFocus
                      value={email} onChange={e => setEmail(e.target.value)}
                      className="w-full px-3 py-2.5 text-sm bg-surface-2 border border-surface-3 rounded-lg text-slate-200 placeholder-slate-600 focus:outline-none focus:border-brand/60"
                    />
                  </div>
                  <button type="submit" disabled={loading}
                    className="w-full py-2.5 bg-brand hover:bg-brand-glow text-white text-sm font-medium rounded-lg transition-colors disabled:opacity-50">
                    {loading ? 'Sending…' : 'Send reset link'}
                  </button>
                  <button type="button" onClick={() => setMode('login')}
                    className="w-full text-center text-xs text-slate-500 hover:text-slate-300">
                    Back to sign in
                  </button>
                </form>
              )}
            </>
          )}
        </div>

        <p className="mt-4 text-center text-xs text-slate-600">
          Default: admin@observex.io / observex
        </p>
      </div>
    </div>
  )
}
