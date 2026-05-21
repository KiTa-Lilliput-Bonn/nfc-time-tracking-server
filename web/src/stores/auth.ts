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

  const isAuthenticated = computed(() => !!token.value && !!user.value)
  const role = computed(() => user.value?.role ?? null)

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
    return data
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

  function logout() {
    token.value = null
    setStoredToken(null)
    persistUser(null)
  }

  return {
    token,
    user,
    role,
    isAuthenticated,
    login,
    logout,
    refreshToken,
    changePassword,
  }
})
