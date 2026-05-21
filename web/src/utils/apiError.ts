import type { AxiosError } from 'axios'

function extractFromResponseData(data: unknown): string | undefined {
  if (data == null) return undefined

  if (typeof data === 'string') {
    const t = data.trim()
    if (!t) return undefined
    if (t.startsWith('{') || t.startsWith('[')) {
      try {
        const parsed = JSON.parse(t) as Record<string, unknown>
        return extractFromPlainObject(parsed)
      } catch {
        if (t.startsWith('<')) {
          return 'Die Antwort ist HTML statt JSON (oft Reverse-Proxy oder Fehlerseite). Details in den Entwicklertools unter „Netzwerk“.'
        }
        return t.length > 500 ? `${t.slice(0, 500)}…` : t
      }
    }
    return t.length > 500 ? `${t.slice(0, 500)}…` : t
  }

  if (typeof data === 'object' && !Array.isArray(data)) {
    return extractFromPlainObject(data as Record<string, unknown>)
  }

  return undefined
}

function extractFromPlainObject(o: Record<string, unknown>): string | undefined {
  for (const key of ['error', 'message', 'detail'] as const) {
    const v = o[key]
    if (typeof v === 'string') {
      const s = v.trim()
      if (s) return s
    }
  }
  return undefined
}

/**
 * Liefert eine lesbare Fehlermeldung aus einer Axios-/HTTP-Antwort,
 * z. B. `{ "error": "…" }` vom Go-Backend, auch wenn der Body als String oder HTML ankommt.
 */
export function getApiErrorMessage(err: unknown): string | undefined {
  const ax = err as AxiosError<unknown> | undefined

  const fromData = ax?.response?.data !== undefined ? extractFromResponseData(ax.response.data) : undefined
  if (fromData) return fromData

  const status = ax?.response?.status
  const statusText = ax?.response?.statusText
  if (status != null) {
    return `HTTP ${status}${statusText ? ` ${statusText}` : ''}`
  }

  if (ax?.message && typeof ax.message === 'string') {
    const m = ax.message.trim()
    if (m) return m
  }

  return undefined
}
