import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, setStoredToken } from '@/api/client'

export type Role = 'user' | 'leitung' | 'superadmin'

export interface AuthUser {
  id: number
  username: string
  display_name: string
  role: Role
  must_change_password: boolean
}

const USER_KEY = 'nfc_user'
/** Wie die aktuelle Sitzung angemeldet wurde ('sso' oder 'password'). */
const VIA_KEY = 'nfc_auth_via'
export type AuthVia = 'sso' | 'password'

function loadVia(): AuthVia {
  try {
    return localStorage.getItem(VIA_KEY) === 'sso' ? 'sso' : 'password'
  } catch {
    return 'password'
  }
}

function loadUser(): AuthUser | null {
  try {
    const raw = localStorage.getItem(USER_KEY)
    if (!raw) return null
    return JSON.parse(raw) as AuthUser
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('nfc_token'))
  const user = ref<AuthUser | null>(loadUser())
  const via = ref<AuthVia>(loadVia())

  const isAuthenticated = computed(() => !!token.value && !!user.value)
  const role = computed(() => user.value?.role ?? null)

  function persistVia(v: AuthVia) {
    via.value = v
    try {
      localStorage.setItem(VIA_KEY, v)
    } catch {
      /* ignore */
    }
  }

  function persistUser(u: AuthUser | null) {
    user.value = u
    if (u) localStorage.setItem(USER_KEY, JSON.stringify(u))
    else localStorage.removeItem(USER_KEY)
  }

  async function login(username: string, password: string) {
    const { data } = await api.post<{
      token: string
      user: AuthUser
      expires_in_seconds: number
    }>('/auth/login', { username, password })
    token.value = data.token
    setStoredToken(data.token)
    persistUser(data.user)
    persistVia('password')
    return data
  }

  /** Übernimmt das App-Token nach SSO-Login und lädt den Benutzer. */
  async function loginWithToken(newToken: string) {
    setStoredToken(newToken)
    try {
      const { data } = await api.get<AuthUser>('/auth/me')
      token.value = newToken
      persistUser(data)
      persistVia('sso')
      return data
    } catch (e) {
      setStoredToken(null)
      throw e
    }
  }

  async function refreshToken() {
    const { data } = await api.post<{ token: string; expires_in_seconds: number }>(
      '/auth/refresh',
    )
    token.value = data.token
    setStoredToken(data.token)
  }

  async function changePassword(currentPassword: string, newPassword: string) {
    await api.post('/auth/change-password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
    if (user.value) {
      persistUser({ ...user.value, must_change_password: false })
    }
  }

  /** Meldet lokal ab. Liefert true, wenn die Sitzung über SSO angemeldet war. */
  function logout(): boolean {
    const wasSso = via.value === 'sso'
    token.value = null
    setStoredToken(null)
    persistUser(null)
    persistVia('password')
    return wasSso
  }

  return {
    token,
    user,
    role,
    isAuthenticated,
    via,
    login,
    loginWithToken,
    logout,
    refreshToken,
    changePassword,
  }
})
