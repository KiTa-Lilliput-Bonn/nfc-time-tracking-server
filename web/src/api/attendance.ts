import { api } from '@/api/client'

export type ChildNoticeReason = 'vacation' | 'sick' | 'other'

export interface ChildNotice {
  id: number
  child_id: number
  date_from: string
  date_to: string
  reason: ChildNoticeReason
  /** Kommt erst ab (HH:MM); null = normal */
  arrive_from: string | null
  /** Wird schon um (HH:MM) abgeholt; null = normal */
  leave_at: string | null
  note: string
}

export type ChildNoticeInput = Omit<ChildNotice, 'id' | 'child_id'>

export interface AttendanceChild {
  id: number
  group_id: number
  first_name: string
  last_name: string
  arrived_at: string | null
  left_at: string | null
  notice: ChildNotice | null
  /** Gemeldete Abwesenheiten ab morgen */
  upcoming: number
}

export interface AttendanceGroup {
  id: number
  name: string
  children: AttendanceChild[]
}

export interface AttendanceDay {
  date: string
  today: string
  groups: AttendanceGroup[]
}

export interface AttendanceAccess {
  groups: { id: number; name: string }[]
  can_manage: boolean
  /** Startansicht: eigene Gruppe; 0 = alle Gruppen */
  default_group_id: number
  is_group_account: boolean
  today: string
}

export interface EvacuationData {
  date: string
  time: string
  groups: { id: number; name: string; children: { id: number; first_name: string; last_name: string }[] }[]
  staff: { id: number; display_name: string; group_name: string }[]
}

export interface Child {
  id: number
  group_id: number
  first_name: string
  last_name: string
  active: boolean
}

export interface GroupAccount {
  id: number
  group_id: number
  username: string
  active: boolean
}

export async function fetchAttendanceAccess() {
  const { data } = await api.get<AttendanceAccess>('/attendance/access')
  return data
}

export async function fetchAttendance(date: string) {
  const { data } = await api.get<AttendanceDay>('/attendance', { params: { date } })
  return data
}

export async function putAttendanceDay(childId: number, date: string, arrivedAt: string | null, leftAt: string | null) {
  await api.put(`/attendance/children/${childId}/days/${date}`, { arrived_at: arrivedAt, left_at: leftAt })
}

export async function fetchChildNotices(childId: number) {
  const { data } = await api.get<ChildNotice[]>(`/attendance/children/${childId}/notices`)
  return data
}

export async function createChildNotice(childId: number, input: ChildNoticeInput) {
  const { data } = await api.post<ChildNotice>(`/attendance/children/${childId}/notices`, input)
  return data
}

export async function updateChildNotice(id: number, input: ChildNoticeInput) {
  const { data } = await api.put<ChildNotice>(`/attendance/notices/${id}`, input)
  return data
}

export async function deleteChildNotice(id: number) {
  await api.delete(`/attendance/notices/${id}`)
}

export async function fetchEvacuation() {
  const { data } = await api.get<EvacuationData>('/attendance/evacuation')
  return data
}

export async function fetchChildren() {
  const { data } = await api.get<Child[]>('/children')
  return data
}

export async function createChild(input: { group_id: number; first_name: string; last_name: string }) {
  const { data } = await api.post<Child>('/children', input)
  return data
}

export async function patchChild(id: number, input: Partial<Omit<Child, 'id'>>) {
  const { data } = await api.patch<Child>(`/children/${id}`, input)
  return data
}

export async function deleteChild(id: number) {
  await api.delete(`/children/${id}`)
}

export async function fetchGroupAccounts() {
  const { data } = await api.get<GroupAccount[]>('/group-accounts')
  return data
}

export async function createGroupAccount(groupId: number, username: string) {
  const { data } = await api.post<{ account: GroupAccount; password: string }>('/group-accounts', {
    group_id: groupId,
    username,
  })
  return data
}

export async function patchGroupAccount(id: number, input: { username?: string; active?: boolean }) {
  const { data } = await api.patch<GroupAccount>(`/group-accounts/${id}`, input)
  return data
}

export async function resetGroupAccountPassword(id: number) {
  const { data } = await api.post<{ password: string }>(`/group-accounts/${id}/reset-password`)
  return data.password
}

export async function deleteGroupAccount(id: number) {
  await api.delete(`/group-accounts/${id}`)
}

export const NOTICE_REASON_LABEL: Record<ChildNoticeReason, string> = {
  vacation: 'Urlaub',
  sick: 'Krank',
  other: 'Sonstiges',
}

/** Kurzer Text zu einer Meldung, z. B. „Urlaub“, „Krank · kommt ab 10:30“. */
export function noticeShort(n: ChildNotice): string {
  // Bei „kommt später/früher abholen“ sagt der Grund „Sonstiges“ nichts; dann die Notiz zeigen.
  const partial = !!(n.arrive_from || n.leave_at)
  const parts = partial && n.reason === 'other' ? [] : [NOTICE_REASON_LABEL[n.reason]]
  if (n.arrive_from) parts.push(`kommt ab ${n.arrive_from}`)
  if (n.leave_at) parts.push(`Abholung ${n.leave_at}`)
  if (partial && n.reason === 'other' && n.note) parts.push(n.note.length > 40 ? `${n.note.slice(0, 40)}…` : n.note)
  return parts.join(' · ')
}

export function noticeFullDay(n: ChildNotice): boolean {
  return !n.arrive_from && !n.leave_at
}

export function childName(c: { first_name: string; last_name: string }): string {
  return c.last_name ? `${c.first_name} ${c.last_name}` : c.first_name
}

const pad = (n: number) => String(n).padStart(2, '0')

export function localYmd(d = new Date()): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

export function nowClock(d = new Date()): string {
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

export function addDays(ymd: string, n: number): string {
  const [y, m, d] = ymd.split('-').map(Number)
  return localYmd(new Date(y!, m! - 1, d! + n))
}

const WEEKDAYS = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa']

/** „Do 08.10.“ */
export function shortDay(ymd: string): string {
  const [y, m, d] = ymd.split('-').map(Number)
  const dt = new Date(y!, m! - 1, d!)
  return `${WEEKDAYS[dt.getDay()]} ${pad(d!)}.${pad(m!)}.`
}

/** „08.10.–12.10.“ bzw. „Do 08.10.“ für einen Tag */
export function rangeLabel(from: string, to: string): string {
  if (from === to) return shortDay(from)
  return `${shortDay(from)} – ${shortDay(to)}`
}
