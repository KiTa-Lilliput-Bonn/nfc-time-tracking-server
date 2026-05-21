import { api } from '@/api/client'
import type { UserGroup } from '@/types/api'

export async function fetchGroups() {
  const { data } = await api.get<{ groups: UserGroup[] }>('/groups')
  return data.groups ?? []
}

export async function createGroup(body: { name: string }) {
  const { data } = await api.post<UserGroup>('/groups', body)
  return data
}

export async function patchGroup(id: number, body: { name: string }) {
  const { data } = await api.patch<UserGroup>(`/groups/${id}`, body)
  return data
}

export async function deleteGroup(id: number) {
  await api.delete(`/groups/${id}`)
}

/** Alle Gruppen-IDs in gewünschter Reihenfolge (vollständig). */
export async function putGroupOrder(ids: number[]) {
  await api.put('/groups/order', { ids })
}
