import { describe, expect, it } from 'vitest'

import { canDeleteEntitlementEntry, isEntryMutable } from './entryLock'

describe('entryLock', () => {
  it('isEntryMutable within 24h', () => {
    const now = Date.parse('2026-05-17T12:00:00Z')
    const created = '2026-05-17T10:00:00Z'
    expect(isEntryMutable(created, now)).toBe(true)
  })

  it('isEntryMutable after 24h', () => {
    const now = Date.parse('2026-05-18T10:00:01Z')
    const created = '2026-05-17T10:00:00Z'
    expect(isEntryMutable(created, now)).toBe(false)
  })

  it('superadmin can always delete', () => {
    expect(canDeleteEntitlementEntry('superadmin', { mutable: false })).toBe(true)
  })

  it('leitung can always delete', () => {
    expect(canDeleteEntitlementEntry('leitung', { mutable: true })).toBe(true)
    expect(canDeleteEntitlementEntry('leitung', { mutable: false })).toBe(true)
  })
})
