import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { User, Org, Role, AccessLevel } from '@/types'
import { auth as authApi } from '@/lib/api'

interface AuthState {
  token: string | null
  user: User | null
  org: Org | null
  effectiveRole: Role | null
  namespaceAccess: Record<string, AccessLevel>
  isLoading: boolean
  setToken: (token: string) => void
  setDemoSession: () => void
  loadMe: () => Promise<void>
  logout: () => Promise<void>
}

const demoUser: User = {
  id: 'demo-user',
  email: 'admin@observex.io',
  name: 'ObserveX Demo Admin',
  role: 'admin',
  is_active: true,
  created_at: new Date().toISOString(),
}

const demoOrg: Org = {
  id: 'demo-org',
  name: 'observex-demo',
  display_name: 'ObserveX Demo',
  plan: 'enterprise',
  max_users: 250,
  max_teams: 50,
  max_dashboards: 500,
  owner_id: 'demo-user',
  settings: {},
  is_active: true,
  user_count: 1,
  team_count: 1,
}

export const useAuth = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      org: null,
      effectiveRole: null,
      namespaceAccess: {},
      isLoading: false,

      setToken: (token) => {
        localStorage.setItem('observex_token', token)
        set({ token })
      },

      setDemoSession: () => {
        localStorage.setItem('observex_token', 'demo-token')
        set({
          token: 'demo-token',
          user: demoUser,
          org: demoOrg,
          effectiveRole: 'admin',
          namespaceAccess: { default: 'admin', production: 'admin', staging: 'admin' },
          isLoading: false,
        })
      },

      loadMe: async () => {
        if (get().token === 'demo-token') {
          set({
            user: demoUser,
            org: demoOrg,
            effectiveRole: 'admin',
            namespaceAccess: { default: 'admin', production: 'admin', staging: 'admin' },
            isLoading: false,
          })
          return
        }
        set({ isLoading: true })
        try {
          const ctx = await authApi.me()
          set({
            user: ctx.user,
            org: ctx.org,
            effectiveRole: ctx.effective_role,
            namespaceAccess: ctx.namespace_access,
            isLoading: false,
          })
        } catch {
          set({ token: null, user: null, org: null, isLoading: false })
          localStorage.removeItem('observex_token')
        }
      },

      logout: async () => {
        try { await authApi.logout() } catch { /* ignore */ }
        localStorage.removeItem('observex_token')
        set({ token: null, user: null, org: null, effectiveRole: null, namespaceAccess: {} })
      },
    }),
    { name: 'observex-auth', partialize: s => ({
      token: s.token,
      user: s.user,
      org: s.org,
      effectiveRole: s.effectiveRole,
      namespaceAccess: s.namespaceAccess,
    }) }
  )
)
