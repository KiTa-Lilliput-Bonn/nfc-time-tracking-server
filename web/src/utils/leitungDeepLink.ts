import type { Router } from 'vue-router'
import { parseISODate } from '@/utils/dates'

export function queryString(q: unknown): string {
  if (typeof q === 'string') return q.trim()
  if (Array.isArray(q) && typeof q[0] === 'string') return q[0].trim()
  return ''
}

export function queryPositiveInt(q: unknown): number | null {
  const s = queryString(q)
  if (!s) return null
  const n = Number.parseInt(s, 10)
  if (!Number.isFinite(n) || n <= 0) return null
  return n
}

export function queryISODate(q: unknown): Date | null {
  const s = queryString(q)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return null
  const d = parseISODate(s)
  return Number.isNaN(d.getTime()) ? null : d
}

export function clearRouteQueryKeys(router: Router, keys: string[]) {
  const q = { ...router.currentRoute.value.query }
  let changed = false
  for (const k of keys) {
    if (k in q) {
      delete q[k]
      changed = true
    }
  }
  if (changed) {
    void router.replace({ query: q })
  }
}
