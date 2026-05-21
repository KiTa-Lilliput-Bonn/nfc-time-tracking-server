import { api } from '@/api/client'

export function triggerDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

export async function fetchExportCsvText(employeeId: number, from: string, to: string) {
  const { data } = await api.get<string>('/export/csv', {
    params: { employee: employeeId, from, to },
    responseType: 'text',
    transformResponse: (r) => r,
  })
  return data
}

export async function fetchExportCsvBlob(employeeId: number, from: string, to: string) {
  const { data } = await api.get<Blob>('/export/csv', {
    params: { employee: employeeId, from, to },
    responseType: 'blob',
  })
  return data
}

export async function fetchExportPdfBlob(employeeId: number, month: number, year: number) {
  const { data } = await api.get<Blob>('/export/pdf', {
    params: { employee: employeeId, month, year },
    responseType: 'blob',
  })
  return data
}
