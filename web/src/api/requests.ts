import { api } from '@/api/client'
import type { ChangeRequest, ChangeRequestKind } from '@/types/api'

export interface NewChangeRequest {
  kind: ChangeRequestKind
  work_period_id?: number
  work_date?: string
  punch_in?: string
  punch_out?: string
  date_from?: string
  date_to?: string
  half_day?: boolean
  reason: string
}

export async function fetchMyRequests() {
  const { data } = await api.get<{ requests: ChangeRequest[] | null }>('/me/requests')
  return data.requests ?? []
}

export async function createMyRequest(body: NewChangeRequest) {
  const { data } = await api.post<ChangeRequest>('/me/requests', body)
  return data
}

export async function withdrawMyRequest(id: number) {
  await api.post(`/me/requests/${id}/withdraw`)
}

/** Leitung: offene (`pending`) oder entschiedene (`decided`) Anträge. */
export async function fetchRequests(status: 'pending' | 'decided' | 'all' = 'pending') {
  const { data } = await api.get<{ requests: ChangeRequest[] | null }>('/requests', { params: { status } })
  return data.requests ?? []
}

export async function fetchPendingRequestCount() {
  const { data } = await api.get<{ pending: number }>('/requests/pending-count')
  return data.pending
}

export async function approveRequest(id: number, comment = '') {
  const { data } = await api.post<ChangeRequest>(`/requests/${id}/approve`, { comment })
  return data
}

export async function rejectRequest(id: number, comment: string) {
  const { data } = await api.post<ChangeRequest>(`/requests/${id}/reject`, { comment })
  return data
}
