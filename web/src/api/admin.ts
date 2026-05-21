import { api } from '@/api/client'
import type {
  ApiPairedClient,
  BackupBrowseResult,
  BackupStatus,
  Employee,
  GenerateAndroidApiClientResponse,
  Holiday,
  LanEmployeeSyncAllResult,
  LanEmployeeSyncResult,
  Setting,
} from '@/types/api'

export async function fetchAdminUsers() {
  const { data } = await api.get<{ users: Employee[] }>('/users')
  return data.users
}

export async function createAdminUser(body: {
  username: string
  display_name: string
  role: string
}) {
  const { data } = await api.post<{ user: Employee; temporary_password: string }>('/users', body)
  return data
}

export async function patchAdminUser(
  id: number,
  body: { display_name?: string; role?: string; active?: boolean },
) {
  const { data } = await api.patch<Employee>(`/users/${id}`, body)
  return data
}

export async function fetchHolidays(year: number) {
  const { data } = await api.get<{ holidays: Holiday[] }>('/holidays', { params: { year } })
  return data.holidays
}

export async function generateHolidaysForYear(year: number) {
  await api.post('/holidays/generate', {}, { params: { year } })
}

export async function createHoliday(body: { holiday_date: string; name: string }) {
  const { data } = await api.post<Holiday>('/holidays', body)
  return data
}

export async function deleteHoliday(id: number) {
  await api.delete(`/holidays/${id}`)
}

export async function fetchSettings() {
  const { data } = await api.get<{ settings: Setting[] }>('/settings')
  return data.settings
}

export async function putSetting(key: string, value: string) {
  const { data } = await api.put<Setting>(`/settings/${key}`, { value })
  return data
}

export async function fetchBackupStatus() {
  const { data } = await api.get<BackupStatus>('/admin/backup/status')
  return data
}

export async function postBackupPickFolder(body?: { initial_path?: string }) {
  const { data } = await api.post<{ path: string; cancelled: boolean }>(
    '/admin/backup/pick-folder',
    body ?? {},
    { timeout: 600_000 },
  )
  return data
}

export async function fetchBackupBrowse(opts?: { path?: string; roots?: boolean }) {
  const params: Record<string, string> = {}
  if (opts?.roots) params.roots = '1'
  else if (opts?.path?.trim()) params.path = opts.path.trim()
  const { data } = await api.get<BackupBrowseResult>('/admin/backup/browse', { params })
  return data
}

export async function putBackupConfig(body: {
  enabled: boolean
  interval_minutes: number
  use_restic: boolean
  target_path: string
}) {
  const { data } = await api.put<BackupStatus>('/admin/backup/config', body)
  return data
}

export async function postBackupInitRestic(body?: { repo_path?: string }) {
  const { data } = await api.post<{ password: string; message: string }>('/admin/backup/init-restic', body ?? {})
  return data
}

export async function postBackupRunNow() {
  const { data } = await api.post<BackupStatus>('/admin/backup/run-now', {})
  return data
}

export async function fetchAndroidApiClients() {
  const { data } = await api.get<{ clients: ApiPairedClient[] }>('/android-api/clients')
  return data.clients ?? []
}

/** Server erzeugt Client + temporäres Pairing-Token für den QR-Code. */
export async function generateAndroidApiClient(body: { label?: string }) {
  const { data } = await api.post<GenerateAndroidApiClientResponse>('/android-api/clients/generate', body)
  return data
}

export async function deleteAndroidApiClient(id: string) {
  await api.delete(`/android-api/clients/${encodeURIComponent(id)}`)
}

/** Ruft die LAN-API der App für ein konfiguriertes Ziel auf (GET /v1/employees, POST Upsert /v1/employee-ids, DELETE deaktivierte /v1/employee-ids/:id). */
export async function syncLanEmployeeIds(targetId: string): Promise<LanEmployeeSyncResult> {
  const { data } = await api.post<LanEmployeeSyncResult>('/android-lan/sync-employee-ids', {
    target_id: targetId,
  })
  return {
    target_id: data.target_id,
    label: data.label,
    lan_base_url: data.lan_base_url ?? '',
    app_employee_ids_after: data.app_employee_ids_after ?? [],
    created: data.created ?? [],
    updated: data.updated ?? [],
    skipped_already_in_app: data.skipped_already_in_app ?? [],
    removed_from_app: data.removed_from_app ?? [],
    failures: data.failures ?? [],
  }
}

/** Gleicher Abgleich für alle konfigurierten LAN-Ziele. */
export async function syncLanEmployeeIdsAll(): Promise<LanEmployeeSyncAllResult> {
  const { data } = await api.post<LanEmployeeSyncAllResult>('/android-lan/sync-employee-ids-all', {})
  const results = (data.results ?? []).map((r) => ({
    ...r,
    app_employee_ids_after: r.app_employee_ids_after ?? [],
    created: r.created ?? [],
    updated: r.updated ?? [],
    skipped_already_in_app: r.skipped_already_in_app ?? [],
    removed_from_app: r.removed_from_app ?? [],
    failures: r.failures ?? [],
  }))
  return {
    results,
    summary: data.summary ?? {
      targets: 0,
      created_total: 0,
      updated_total: 0,
      skipped_total: 0,
      removed_total: 0,
      failures_total: 0,
    },
  }
}
