import { api } from '@/api/client'

export type PasswordLoginMode = 'all' | 'admins' | 'none'

export interface SsoConfig {
  enabled: boolean
  button_label?: string
  password_login: PasswordLoginMode
}

let cached: Promise<SsoConfig> | null = null

/** SSO-Einstellungen des Servers (öffentlich, einmal pro Seitenaufruf geladen). */
export function fetchSsoConfig(): Promise<SsoConfig> {
  if (!cached) {
    cached = api
      .get<SsoConfig>('/auth/oidc/config')
      .then((r) => r.data)
      .catch(() => {
        cached = null
        return { enabled: false, password_login: 'all' as const }
      })
  }
  return cached
}

/** Startet die Anmeldung beim SSO-Provider (volle Seitennavigation). */
export const SSO_START_URL = '/api/v1/auth/oidc/start'
/** Abmelden beim SSO-Provider (falls unterstützt), danach zurück zur Login-Seite. */
export const SSO_LOGOUT_URL = '/api/v1/auth/oidc/logout'

export function ssoErrorMessage(code: string): string {
  switch (code) {
    case 'no_account':
      return 'Zu diesem SSO-Konto gibt es keinen Benutzer in der Zeiterfassung. Bitte wenden Sie sich an die Leitung.'
    case 'inactive':
      return 'Ihr Konto in der Zeiterfassung ist deaktiviert.'
    case 'conflict':
      return 'Das Konto ist bereits mit einem anderen SSO-Konto verknüpft. Bitte wenden Sie sich an die Leitung.'
    case 'denied':
      return 'Die Anmeldung beim SSO-Anbieter wurde abgebrochen.'
    default:
      return 'Die Anmeldung über SSO ist fehlgeschlagen. Bitte erneut versuchen.'
  }
}
