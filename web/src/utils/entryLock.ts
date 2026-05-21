const MUTABLE_MS = 24 * 60 * 60 * 1000

export function isEntryMutable(createdAt: string, now = Date.now()): boolean {
  const t = new Date(createdAt).getTime()
  if (Number.isNaN(t)) return false
  return now <= t + MUTABLE_MS
}

export function canDeleteEntitlementEntry(
  role: string | null | undefined,
  row: { mutable?: boolean; created_at?: string },
): boolean {
  if (role === 'superadmin') return true
  if (row.mutable === true) return true
  if (row.mutable === false) return false
  if (row.created_at) return isEntryMutable(row.created_at)
  return false
}
