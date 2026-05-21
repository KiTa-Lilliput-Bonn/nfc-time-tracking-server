<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { fetchEmployees } from '@/api/employees'
import { createGroup, deleteGroup, fetchGroups, patchGroup, putGroupOrder } from '@/api/groups'
import { patchEmployee } from '@/api/management'
import type { Employee, UserGroup } from '@/types/api'
import { canManageEmployeeByRole } from '@/utils/roles'
import { useAuthStore } from '@/stores/auth'

const toast = useToast()
const auth = useAuthStore()

const groups = ref<UserGroup[]>([])
const employees = ref<Employee[]>([])
const loading = ref(false)

const selectedGroup = ref<UserGroup | null>(null)
const addMemberId = ref<number | null>(null)

const showEdit = ref(false)
const editing = ref<UserGroup | null>(null)
const editName = ref('')
const saving = ref(false)
const reorderSaving = ref(false)

function memberCount(groupId: number) {
  return employees.value.filter((e) => e.group_id === groupId).length
}

const members = computed(() => {
  const g = selectedGroup.value
  if (!g) return []
  return employees.value
    .filter(
      (e) =>
        e.group_id === g.id && canManageEmployeeByRole(auth.role, e.role),
    )
    .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
})

const addableOptions = computed(() => {
  const g = selectedGroup.value
  if (!g) return []
  return employees.value
    .filter(
      (e) =>
        canManageEmployeeByRole(auth.role, e.role) &&
        e.group_id !== g.id,
    )
    .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
    .map((e) => ({
      label: `${e.display_name}${e.group_id != null && e.group_id !== g.id ? ' (andere Gruppe)' : ''}`,
      value: e.id,
    }))
})

async function load() {
  loading.value = true
  try {
    const [grp, emp] = await Promise.all([fetchGroups(), fetchEmployees()])
    groups.value = grp
    employees.value = emp
    if (selectedGroup.value) {
      const id = selectedGroup.value.id
      selectedGroup.value = grp.find((x) => x.id === id) ?? null
    }
  } catch {
    toast.add({ severity: 'error', summary: 'Gruppen', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(load)

const showNew = ref(false)
const newName = ref('')

function openNew() {
  newName.value = ''
  showNew.value = true
}

async function submitNew() {
  if (!newName.value.trim()) return
  saving.value = true
  try {
    await createGroup({ name: newName.value.trim() })
    toast.add({ severity: 'success', summary: 'Gruppe angelegt', life: 10000 })
    showNew.value = false
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Anlegen fehlgeschlagen', detail: 'Name evtl. bereits vergeben.', life: 10000 })
  } finally {
    saving.value = false
  }
}

function openEdit(g: UserGroup) {
  editing.value = g
  editName.value = g.name
  showEdit.value = true
}

async function submitEdit() {
  if (!editing.value || !editName.value.trim()) return
  saving.value = true
  try {
    await patchGroup(editing.value.id, { name: editName.value.trim() })
    toast.add({ severity: 'success', summary: 'Gespeichert', life: 10000 })
    showEdit.value = false
    editing.value = null
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function remove(g: UserGroup) {
  if (!confirm(`Gruppe „${g.name}“ löschen? Zugeordnete Mitarbeitende verlieren die Gruppenzuordnung.`)) return
  saving.value = true
  try {
    await deleteGroup(g.id)
    if (selectedGroup.value?.id === g.id) {
      selectedGroup.value = null
    }
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function addMember() {
  const g = selectedGroup.value
  if (!g || addMemberId.value == null) return
  saving.value = true
  try {
    await patchEmployee(addMemberId.value, { group_id: g.id })
    toast.add({ severity: 'success', summary: 'Zugeordnet', life: 10000 })
    addMemberId.value = null
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Zuordnung fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function removeMember(e: Employee) {
  if (!selectedGroup.value) return
  if (!confirm(`„${e.display_name}“ aus dieser Gruppe entfernen?`)) return
  saving.value = true
  try {
    await patchEmployee(e.id, { group_id: null })
    toast.add({ severity: 'success', summary: 'Entfernt', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Entfernen fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function persistGroupOrder() {
  reorderSaving.value = true
  try {
    await putGroupOrder(groups.value.map((g) => g.id))
    toast.add({ severity: 'success', summary: 'Reihenfolge gespeichert', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Reihenfolge konnte nicht gespeichert werden', life: 10000 })
    await load()
  } finally {
    reorderSaving.value = false
  }
}

function moveGroupUp(index: number) {
  if (index <= 0) return
  const arr = [...groups.value]
  ;[arr[index - 1], arr[index]] = [arr[index], arr[index - 1]]
  groups.value = arr
  void persistGroupOrder()
}

function moveGroupDown(index: number) {
  if (index >= groups.value.length - 1) return
  const arr = [...groups.value]
  ;[arr[index], arr[index + 1]] = [arr[index + 1], arr[index]]
  groups.value = arr
  void persistGroupOrder()
}

function groupRowIndex(g: UserGroup) {
  return groups.value.findIndex((x) => x.id === g.id)
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Gruppen</template>
      <template #subtitle>
        Reihenfolge mit den Pfeilen neben jeder Zeile anpassen (u. a. für den Dienstplan). Gruppe auswählen für
        Mitglieder (max. eine Gruppe pro Person); bereits anderweitig Zugewiesene können hierher verschoben werden.
      </template>
      <template #content>
        <div class="toolbar">
          <Button label="Neue Gruppe" icon="pi pi-plus" @click="openNew" />
        </div>
        <DataTable
          v-model:selection="selectedGroup"
          :value="groups"
          :loading="loading"
          data-key="id"
          selection-mode="single"
          striped-rows
          size="small"
          meta-key-selection
        >
          <Column selection-mode="single" header-style="width: 3rem" />
          <Column header="Position" :sortable="false" style="width: 6.5rem">
            <template #body="{ data }: { data: UserGroup }">
              <Button
                icon="pi pi-arrow-up"
                text
                rounded
                size="small"
                aria-label="Nach oben"
                :disabled="groupRowIndex(data) <= 0 || reorderSaving || loading"
                @click.stop="moveGroupUp(groupRowIndex(data))"
              />
              <Button
                icon="pi pi-arrow-down"
                text
                rounded
                size="small"
                aria-label="Nach unten"
                :disabled="groupRowIndex(data) >= groups.length - 1 || reorderSaving || loading"
                @click.stop="moveGroupDown(groupRowIndex(data))"
              />
            </template>
          </Column>
          <Column field="name" header="Name" />
          <Column header="Mitglieder" :sortable="false" style="width: 7rem">
            <template #body="{ data }: { data: UserGroup }">
              {{ memberCount(data.id) }}
            </template>
          </Column>
          <Column header="" :exportable="false" style="width: 8rem">
            <template #body="{ data }: { data: UserGroup }">
              <Button icon="pi pi-pencil" text rounded aria-label="Bearbeiten" @click.stop="openEdit(data)" />
              <Button
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                aria-label="Löschen"
                @click.stop="remove(data)"
              />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Card v-if="selectedGroup" class="members-card">
      <template #title>Mitglieder: {{ selectedGroup.name }}</template>
      <template #content>
        <div class="add-row">
          <Select
            v-model="addMemberId"
            :options="addableOptions"
            option-label="label"
            option-value="value"
            placeholder="Mitarbeitenden hinzufügen…"
            class="add-select"
            filter
            show-clear
          />
          <Button
            label="Hinzufügen"
            icon="pi pi-user-plus"
            :disabled="addMemberId == null || saving"
            :loading="saving"
            @click="addMember"
          />
        </div>
        <p v-if="addableOptions.length === 0" class="muted">
          Keine weiteren zuordenbaren Mitarbeitenden (alle sind bereits in dieser Gruppe oder nicht verwaltbar).
        </p>
        <DataTable :value="members" data-key="id" striped-rows size="small" class="mt">
          <Column field="display_name" header="Name" sortable>
            <template #body="{ data }: { data: Employee }">
              <RouterLink class="link" :to="`/employees/${data.id}/edit`">{{ data.display_name }}</RouterLink>
            </template>
          </Column>
          <Column field="username" header="Benutzername" sortable />
          <Column header="" :exportable="false" style="width: 4rem">
            <template #body="{ data }: { data: Employee }">
              <Button
                icon="pi pi-times"
                severity="danger"
                text
                rounded
                aria-label="Aus Gruppe entfernen"
                :disabled="saving"
                @click="removeMember(data)"
              />
            </template>
          </Column>
        </DataTable>
        <p v-if="members.length === 0" class="muted mt">Noch keine Mitglieder in dieser Gruppe.</p>
      </template>
    </Card>

    <p v-else-if="!loading && groups.length > 0" class="hint">
      Bitte eine Gruppe in der Tabelle auswählen (Zeile anklicken).
    </p>

    <Dialog v-model:visible="showNew" header="Neue Gruppe" modal :style="{ width: '400px' }">
      <div class="form">
        <label for="group-new-name">Name</label>
        <InputText id="group-new-name" v-model="newName" class="w-full" autocomplete="off" @keyup.enter="submitNew" />
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showNew = false" />
        <Button label="Anlegen" :loading="saving" :disabled="!newName.trim()" @click="submitNew" />
      </template>
    </Dialog>

    <Dialog v-model:visible="showEdit" header="Gruppe bearbeiten" modal :style="{ width: '400px' }">
      <div v-if="editing" class="form">
        <label for="group-edit-name">Name</label>
        <InputText id="group-edit-name" v-model="editName" class="w-full" autocomplete="off" @keyup.enter="submitEdit" />
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showEdit = false" />
        <Button label="Speichern" :loading="saving" :disabled="!editName.trim()" @click="submitEdit" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 960px;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}
.toolbar {
  margin-bottom: 1rem;
}
.members-card {
  scroll-margin-top: 1rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.form label {
  font-size: 0.8rem;
  color: #64748b;
}
.w-full {
  width: 100%;
}
.add-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
  margin-bottom: 1rem;
}
.add-select {
  min-width: min(100%, 320px);
  flex: 1;
}
.muted {
  font-size: 0.85rem;
  color: #64748b;
  margin: 0;
}
.mt {
  margin-top: 0.75rem;
}
.hint {
  font-size: 0.9rem;
  color: #64748b;
  margin: 0;
}
.link {
  color: #2563eb;
  text-decoration: none;
  font-weight: 500;
}
.link:hover {
  text-decoration: underline;
}
</style>
