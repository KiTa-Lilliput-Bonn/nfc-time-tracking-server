import { api } from '@/api/client'
import type {
  CashAllowance,
  CashBoxDetail,
  CashBoxListItem,
  CashEntry,
  CashEntryInput,
  CashPerson,
  CashReceipt,
} from '@/types/api'

export async function fetchCashBoxes() {
  const { data } = await api.get<{ cash_boxes: CashBoxListItem[] }>('/cash-boxes')
  return data.cash_boxes ?? []
}

export async function fetchCashBox(groupId: number) {
  const { data } = await api.get<CashBoxDetail>(`/cash-boxes/${groupId}`)
  return data
}

export async function putCashKeepers(groupId: number, userIds: number[]) {
  const { data } = await api.put<{ keepers: CashPerson[] }>(`/cash-boxes/${groupId}/keepers`, { user_ids: userIds })
  return data.keepers
}

export async function putCashAllowance(groupId: number, validFrom: string, amountCents: number) {
  const { data } = await api.put<CashAllowance>(`/cash-boxes/${groupId}/allowances`, {
    valid_from: validFrom,
    amount_cents: amountCents,
  })
  return data
}

export async function deleteCashAllowance(groupId: number, id: number) {
  await api.delete(`/cash-boxes/${groupId}/allowances/${id}`)
}

export async function createCashEntry(groupId: number, body: CashEntryInput) {
  const { data } = await api.post<CashEntry>(`/cash-boxes/${groupId}/entries`, body)
  return data
}

export async function updateCashEntry(groupId: number, entryId: number, body: CashEntryInput) {
  const { data } = await api.put<CashEntry>(`/cash-boxes/${groupId}/entries/${entryId}`, body)
  return data
}

export async function deleteCashEntry(groupId: number, entryId: number) {
  await api.delete(`/cash-boxes/${groupId}/entries/${entryId}`)
}

export async function uploadCashReceipts(groupId: number, entryId: number, files: File[]) {
  const form = new FormData()
  for (const f of files) form.append('file', f, f.name)
  const { data } = await api.post<{ receipts: CashReceipt[] }>(
    `/cash-boxes/${groupId}/entries/${entryId}/receipts`,
    form,
  )
  return data.receipts
}

/** Lädt einen Beleg als Blob (die API verlangt den Bearer-Token, ein direkter Link reicht daher nicht). */
export async function fetchCashReceipt(groupId: number, receiptId: number) {
  const { data } = await api.get<Blob>(`/cash-boxes/${groupId}/receipts/${receiptId}`, { responseType: 'blob' })
  return data
}

export async function deleteCashReceipt(groupId: number, receiptId: number) {
  await api.delete(`/cash-boxes/${groupId}/receipts/${receiptId}`)
}
