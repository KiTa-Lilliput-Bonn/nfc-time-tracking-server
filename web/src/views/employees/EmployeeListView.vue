<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import { createEmployee, fetchWeeklyHours } from '@/api/management'
import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import type { Employee, UserGroup, WeeklyHours } from '@/types/api'
import { toastDetailAfterPasswordClipboard } from '@/utils/clipboard'
import { toISODateLocal } from '@/utils/dates'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const toast = useToast()
const auth = useAuthStore()

const employees = ref<Employee[]>([])
const groups = ref<UserGroup[]>([])
const weeklyMap = ref<Record<number, number | null>>({})
const loading = ref(false)

function groupLabel(groupId: number | null | undefined) {
  if (groupId == null) return '—'
  const g = groups.value.find((x) => x.id === groupId)
  return g?.name ?? `#${groupId}`
}

const statusFilter = ref<'all' | 'active' | 'inactive'>('active')
const statusOptions = [
  { label: 'Aktiv', value: 'active' as const },
  { label: 'Inaktiv', value: 'inactive' as const },
  { label: 'Alle', value: 'all' as const },
]

const query = ref('')
const queryInput = ref<ComponentPublicInstance | null>(null)

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  let list = employees.value
  if (statusFilter.value === 'active') list = list.filter((e) => e.active)
  else if (statusFilter.value === 'inactive') list = list.filter((e) => !e.active)

  if (!q) return list
  return list.filter((e) => {
    const dn = e.display_name?.toLowerCase() ?? ''
    const un = e.username?.toLowerCase() ?? ''
    return dn.includes(q) || un.includes(q)
  })
})

/** Standard-Sortierung Name A→Z — sichtbar am Spaltenkopf (PrimeVue Sort-Icons). */
const sortField = ref('display_name')
const sortOrder = ref<number>(1)

function roleLabel(r: string) {
  if (r === 'leitung') return 'Leitung'
  if (r === 'superadmin') return 'Superadmin'
  return 'Mitarbeiter'
}

function currentWeeklyHours(list: WeeklyHours[]): number | null {
  const today = toISODateLocal(new Date())
  const applicable = list.filter((w) => w.valid_from <= today)
  const pick = (arr: WeeklyHours[]) =>
    [...arr].sort((a, b) => b.valid_from.localeCompare(a.valid_from))[0]
  if (applicable.length) return pick(applicable).hours_per_week
  if (list.length) return pick(list).hours_per_week
  return null
}

async function loadWeekly(ids: number[]) {
  const m: Record<number, number | null> = { ...weeklyMap.value }
  await Promise.all(
    ids.map(async (id) => {
      try {
        const list = await fetchWeeklyHours(id)
        m[id] = currentWeeklyHours(list)
      } catch {
        m[id] = null
      }
    }),
  )
  weeklyMap.value = m
}

async function load() {
  loading.value = true
  try {
    const [emps, grp] = await Promise.all([fetchEmployees(), fetchGroups()])
    employees.value = emps
    groups.value = grp
    await loadWeekly(employees.value.map((e) => e.id))
  } catch {
    toast.add({ severity: 'error', summary: 'Mitarbeiter', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await load()
  nextTick(() => {
    requestAnimationFrame(() => {
      const el = queryInput.value?.$el
      // PrimeVue InputText root is a native input element.
      if (el instanceof HTMLInputElement) el.focus()
      else if (el instanceof HTMLElement) el.querySelector('input')?.focus()
    })
  })
})

const showNew = ref(false)
const creating = ref(false)
const newUsername = ref('')
const newDisplayName = ref('')
const newRole = ref<'user' | 'leitung'>('user')
const tempPassword = ref('')
/** Nach erfolgreichem API-Anlegen — Anzeige getrennt vom Formular, damit kein „nochmal speichern“-Eindruck entsteht */
const createdSummary = ref<{ username: string; display_name: string } | null>(null)

/** PrimeVue-Button leitet autofocus nicht auf das native Button-Element durch — Fokus nach Erfolgs-View setzen. */
const closeAfterEmployeeCreateRef = ref<ComponentPublicInstance | null>(null)

watch(tempPassword, (pw) => {
  if (!pw) return
  nextTick(() => {
    requestAnimationFrame(() => {
      const el = closeAfterEmployeeCreateRef.value?.$el
      if (el instanceof HTMLElement) el.focus()
    })
  })
})

const roleCreateOptions = computed((): { label: string; value: 'user' | 'leitung' }[] => {
  const base: { label: string; value: 'user' | 'leitung' }[] = [{ label: 'Mitarbeiter', value: 'user' }]
  if (auth.role === 'superadmin') base.push({ label: 'Leitung', value: 'leitung' })
  return base
})

function resetNewEmployeeDialog() {
  newUsername.value = ''
  newDisplayName.value = ''
  newRole.value = 'user'
  tempPassword.value = ''
  createdSummary.value = null
}

function openNew() {
  resetNewEmployeeDialog()
  showNew.value = true
}

async function submitNew() {
  if (creating.value) return
  if (!newUsername.value.trim() || !newDisplayName.value.trim()) return
  creating.value = true
  tempPassword.value = ''
  createdSummary.value = null
  try {
    const res = await createEmployee({
      username: newUsername.value.trim(),
      display_name: newDisplayName.value.trim(),
      role: newRole.value,
    })
    createdSummary.value = {
      username: res.user.username,
      display_name: res.user.display_name,
    }
    tempPassword.value = res.temporary_password
    const detail = await toastDetailAfterPasswordClipboard(res.temporary_password)
    toast.add({ severity: 'success', summary: 'Angelegt', detail, life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Anlegen fehlgeschlagen', life: 10000 })
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Mitarbeiter</template>
      <template #content>
        <div class="toolbar">
          <Select
            v-model="statusFilter"
            :options="statusOptions"
            option-label="label"
            option-value="value"
            placeholder="Filter"
            class="filt"
          />
          <InputText
            ref="queryInput"
            v-model="query"
            class="search"
            placeholder="Suchen…"
            type="search"
          />
          <Button label="Neuer Mitarbeiter" icon="pi pi-plus" @click="openNew" />
        </div>
        <DataTable
          v-model:sort-field="sortField"
          v-model:sort-order="sortOrder"
          :value="filtered"
          :loading="loading"
          data-key="id"
          striped-rows
        >
          <Column field="display_name" header="Name" sortable>
            <template #body="{ data }">
              <RouterLink class="link" :to="`/employees/${data.id}`">{{ data.display_name }}</RouterLink>
            </template>
          </Column>
          <Column header="Rolle" sortable field="role">
            <template #body="{ data }">
              {{ roleLabel(data.role) }}
            </template>
          </Column>
          <Column header="Gruppe" sortable field="group_id">
            <template #body="{ data }">
              {{ groupLabel(data.group_id) }}
            </template>
          </Column>
          <Column header="Aktiv" field="active">
            <template #body="{ data }">
              <Tag :severity="data.active ? 'success' : 'secondary'" :value="data.active ? 'Ja' : 'Nein'" />
            </template>
          </Column>
          <Column header="Std./Woche">
            <template #body="{ data }">
              {{
                weeklyMap[data.id] != null ? `${weeklyMap[data.id]} h` : '—'
              }}
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Dialog
      v-model:visible="showNew"
      :header="tempPassword ? 'Mitarbeiter angelegt' : 'Neuer Mitarbeiter'"
      modal
      class="dlg"
      :style="{ width: '420px' }"
      @hide="resetNewEmployeeDialog"
    >
      <form
        v-if="!tempPassword"
        id="employee-create-form"
        class="form"
        @submit.prevent="submitNew"
      >
        <label for="employee-new-username">Benutzername</label>
        <InputText
          id="employee-new-username"
          v-model="newUsername"
          class="w-full"
          autocomplete="off"
          autofocus
        />
        <label for="employee-new-displayname">Anzeigename</label>
        <InputText id="employee-new-displayname" v-model="newDisplayName" class="w-full" />
        <label>Rolle</label>
        <Select
          v-model="newRole"
          :options="roleCreateOptions"
          option-label="label"
          option-value="value"
          class="w-full"
        />
      </form>
      <div v-else-if="createdSummary" class="created-summary">
        <p class="created-lead">Bitte Zugangsdaten notieren — der Account ist bereits gespeichert.</p>
        <div class="created-row">
          <span class="created-label">Anzeigename</span>
          <span class="created-value">{{ createdSummary.display_name }}</span>
        </div>
        <div class="created-row">
          <span class="created-label">Benutzername</span>
          <span class="created-value">{{ createdSummary.username }}</span>
        </div>
        <div class="pw">
          <span class="created-label">Einmalpasswort</span>
          <strong class="pw-value">{{ tempPassword }}</strong>
        </div>
      </div>
      <template #footer>
        <template v-if="!tempPassword">
          <Button type="button" label="Abbrechen" severity="secondary" text @click="showNew = false" />
          <Button
            type="submit"
            form="employee-create-form"
            label="Anlegen"
            :loading="creating"
            :disabled="!newUsername.trim() || !newDisplayName.trim()"
          />
        </template>
        <Button v-else ref="closeAfterEmployeeCreateRef" label="Schließen" @click="showNew = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 1100px;
}
.muted {
  color: #94a3b8;
  font-size: 0.9rem;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-bottom: 1rem;
  align-items: center;
}
.filt {
  min-width: 160px;
}
.search {
  flex: 1;
  min-width: 200px;
  max-width: 360px;
}
.link {
  color: #2563eb;
  text-decoration: none;
  font-weight: 500;
}
.link:hover {
  text-decoration: underline;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.form label {
  font-size: 0.8rem;
  color: #64748b;
  margin-top: 0.35rem;
}
.created-summary {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.created-lead {
  margin: 0;
  font-size: 0.9rem;
  color: #475569;
  line-height: 1.4;
}
.created-row {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.created-label {
  font-size: 0.75rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.created-value {
  font-size: 1rem;
  font-weight: 500;
  color: #0f172a;
}
.pw {
  margin: 0;
  padding: 0.65rem 0.75rem;
  background: #fef3c7;
  border-radius: 6px;
  font-size: 0.9rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.pw-value {
  font-size: 1.05rem;
  letter-spacing: 0.04em;
}
.w-full {
  width: 100%;
}
</style>
