import { api } from '@/api/client'
import type { Employee } from '@/types/api'

export async function fetchEmployees() {
  const { data } = await api.get<{ employees: Employee[] }>('/employees')
  return data.employees
}
