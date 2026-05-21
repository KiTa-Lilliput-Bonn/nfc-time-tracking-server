<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'
import { nextTick, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import { createAdminUser, fetchAdminUsers, patchAdminUser } from '@/api/admin'
import { postEmployeeResetPassword } from '@/api/management'
import type { Employee } from '@/types/api'
import { toastDetailAfterPasswordClipboard } from '@/utils/clipboard'
import { useAuthStore } from '@/stores/auth'

const toast = useToast()
const auth = useAuthStore()

const users = ref<Employee[]>([])
const loading = ref(false)

const roleOptions = [
  { label: 'Mitarbeiter', value: 'user' },
  { label: 'Leitung', value: 'leitung' },
  { label: 'Superadmin', value: 'superadmin' },
]

function roleLabel(r: string) {
  const o = roleOptions.find((x) => x.value === r)
  return o?.label ?? r
}

async function load() {
  loading.value = true
  try {
    users.value = await fetchAdminUsers()
  } catch {
    toast.add({ severity: 'error', summary: 'Benutzer', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(load)

const showCreate = ref(false)
const creating = ref(false)
const newUsername = ref('')
const newDisplayName = ref('')
const newRole = ref<'leitung' | 'user'>('leitung')
const tempPassword = ref('')
const createdSummary = ref<{ username: string; display_name: string } | null>(null)

const closeAfterUserCreateRef = ref<ComponentPublicInstance | null>(null)

watch(tempPassword, (pw) => {
  if (!pw) return
  nextTick(() => {
    requestAnimationFrame(() => {
      const el = closeAfterUserCreateRef.value?.$el
      if (el instanceof HTMLElement) el.focus()
    })
  })
})

const createRoleOptions = [
  { label: 'Leitung', value: 'leitung' as const },
  { label: 'Mitarbeiter', value: 'user' as const },
]

function resetCreateUserDialog() {
  newUsername.value = ''
  newDisplayName.value = ''
  newRole.value = 'leitung'
  tempPassword.value = ''
  createdSummary.value = null
}

function openCreate() {
  resetCreateUserDialog()
  showCreate.value = true
}

async function submitCreate() {
  if (creating.value) return
  if (!newUsername.value.trim() || !newDisplayName.value.trim()) return
  creating.value = true
  tempPassword.value = ''
  createdSummary.value = null
  try {
    const res = await createAdminUser({
      username: newUsername.value.trim(),
      display_name: newDisplayName.value.trim(),
      role: newRole.value,
    })
    createdSummary.value = {
      username: res.user.username,
      display_name: res.user.display_name,
    }
    tempPassword.value = res.temporary_password
    const createDetail = await toastDetailAfterPasswordClipboard(res.temporary_password)
    toast.add({ severity: 'success', summary: 'Angelegt', detail: createDetail, life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Anlegen fehlgeschlagen', life: 10000 })
  } finally {
    creating.value = false
  }
}

const showEdit = ref(false)
const editRow = ref<Employee | null>(null)
const editDisplayName = ref('')
const editRole = ref('')
const editActive = ref(true)
const saving = ref(false)

function openEdit(u: Employee) {
  editRow.value = u
  editDisplayName.value = u.display_name
  editRole.value = u.role
  editActive.value = u.active
  showEdit.value = true
}

function roleOptionsFor(u: Employee | null) {
  if (!u) return roleOptions
  if (u.role === 'superadmin') return roleOptions
  return roleOptions.filter((o) => o.value !== 'superadmin')
}

async function saveEdit() {
  if (!editRow.value) return
  saving.value = true
  try {
    await patchAdminUser(editRow.value.id, {
      display_name: editDisplayName.value.trim(),
      role: editRole.value,
      active: editActive.value,
    })
    toast.add({ severity: 'success', summary: 'Gespeichert', life: 10000 })
    cancelEdit()
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

function cancelEdit() {
  showEdit.value = false
  editRow.value = null
}

const pwResetDialogVisible = ref(false)
const pwResetStep = ref<'confirm' | 'result'>('confirm')
const pwResetting = ref(false)
const pwResetTemp = ref('')

function onPwResetDialogHide() {
  pwResetStep.value = 'confirm'
  pwResetTemp.value = ''
}

function openPwResetFromEdit() {
  if (!editRow.value) return
  pwResetStep.value = 'confirm'
  pwResetTemp.value = ''
  pwResetDialogVisible.value = true
}

async function submitAdminPwReset() {
  if (!editRow.value || pwResetting.value) return
  pwResetting.value = true
  try {
    const res = await postEmployeeResetPassword(editRow.value.id)
    const pw = res.temporary_password
    pwResetTemp.value = pw
    pwResetStep.value = 'result'
    const resetDetail = await toastDetailAfterPasswordClipboard(pw)
    toast.add({
      severity: 'success',
      summary: 'Passwort zurückgesetzt',
      detail: resetDetail,
      life: 10000,
    })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Zurücksetzen fehlgeschlagen', life: 10000 })
  } finally {
    pwResetting.value = false
  }
}

const selfId = () => auth.user?.id
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Benutzer (Admin)</template>
      <template #content>
        <div class="toolbar">
          <Button label="Benutzer anlegen" icon="pi pi-plus" @click="openCreate" />
        </div>
        <DataTable :value="users" :loading="loading" data-key="id" striped-rows>
          <Column field="display_name" header="Name" sortable />
          <Column field="username" header="Benutzername" sortable />
          <Column header="Rolle" field="role" sortable>
            <template #body="{ data }">{{ roleLabel(data.role) }}</template>
          </Column>
          <Column header="Aktiv">
            <template #body="{ data }">
              <Tag :severity="data.active ? 'success' : 'secondary'" :value="data.active ? 'Ja' : 'Nein'" />
            </template>
          </Column>
          <Column header="">
            <template #body="{ data }">
              <Button label="Bearbeiten" size="small" text @click="openEdit(data)" />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Dialog
      v-model:visible="showCreate"
      :header="tempPassword ? 'Benutzer angelegt' : 'Neuer Benutzer'"
      modal
      :style="{ width: '440px' }"
      @hide="resetCreateUserDialog"
    >
      <template v-if="!tempPassword">
        <p class="hint">Leitung oder Mitarbeiter anlegen (Superadmin nur bei Erstinstallation).</p>
        <form id="admin-user-create-form" class="form" @submit.prevent="submitCreate">
          <label for="admin-new-username">Benutzername</label>
          <InputText
            id="admin-new-username"
            v-model="newUsername"
            class="w"
            autocomplete="off"
            autofocus
          />
          <label for="admin-new-displayname">Anzeigename</label>
          <InputText id="admin-new-displayname" v-model="newDisplayName" class="w" />
          <label>Rolle</label>
          <Select
            v-model="newRole"
            :options="createRoleOptions"
            option-label="label"
            option-value="value"
            class="w"
          />
        </form>
      </template>
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
          <Button type="button" label="Abbrechen" severity="secondary" text @click="showCreate = false" />
          <Button
            type="submit"
            form="admin-user-create-form"
            label="Anlegen"
            :loading="creating"
            :disabled="!newUsername.trim() || !newDisplayName.trim()"
          />
        </template>
        <Button v-else ref="closeAfterUserCreateRef" label="Schließen" @click="showCreate = false" />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="showEdit"
      header="Benutzer bearbeiten"
      modal
      :style="{ width: '440px' }"
      @hide="editRow = null"
    >
      <div v-if="editRow" class="form">
        <p v-if="editRow.id === selfId()" class="warn">Das ist Ihr eigener Account.</p>
        <label>Anzeigename</label>
        <InputText v-model="editDisplayName" class="w" />
        <label>Rolle</label>
        <Select
          v-model="editRole"
          :options="roleOptionsFor(editRow)"
          option-label="label"
          option-value="value"
          class="w"
        />
        <label class="row">
          <span>Aktiv</span>
          <input v-model="editActive" type="checkbox" class="cb" />
        </label>
        <div class="pw-reset-wrap">
          <Button
            label="Passwort zurücksetzen"
            icon="pi pi-key"
            severity="secondary"
            outlined
            type="button"
            @click="openPwResetFromEdit"
          />
        </div>
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="cancelEdit" />
        <Button label="Speichern" :loading="saving" @click="saveEdit" />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="pwResetDialogVisible"
      :header="pwResetStep === 'confirm' ? 'Passwort zurücksetzen' : 'Neues Einmalpasswort'"
      modal
      :style="{ width: '440px' }"
      @hide="onPwResetDialogHide"
    >
      <template v-if="pwResetStep === 'confirm' && editRow">
        <p class="pw-reset-lead">
          Das bisherige Passwort von <strong>{{ editRow.display_name }}</strong> funktioniert danach nicht mehr. Es wird
          ein neues Einmalpasswort erzeugt; die Person muss bei der nächsten Anmeldung ein neues Passwort wählen.
        </p>
      </template>
      <div v-else-if="pwResetStep === 'result' && editRow" class="pw-result">
        <p class="pw-reset-lead">Bitte Zugangsdaten sicher übermitteln — der Account ist bereits aktualisiert.</p>
        <div class="pw-result-row">
          <span class="pw-result-label">Benutzername</span>
          <span class="pw-result-value">{{ editRow.username }}</span>
        </div>
        <div class="pw-result-row">
          <span class="pw-result-label">Anzeigename</span>
          <span class="pw-result-value">{{ editRow.display_name }}</span>
        </div>
        <div class="pw-box">
          <span class="pw-result-label">Einmalpasswort</span>
          <strong class="pw-box-value">{{ pwResetTemp }}</strong>
        </div>
      </div>
      <template #footer>
        <template v-if="pwResetStep === 'confirm'">
          <Button label="Abbrechen" severity="secondary" text @click="pwResetDialogVisible = false" />
          <Button label="Zurücksetzen" :loading="pwResetting" @click="submitAdminPwReset" />
        </template>
        <Button v-else label="Schließen" @click="pwResetDialogVisible = false" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 960px;
}
.toolbar {
  margin-bottom: 1rem;
}
.hint {
  font-size: 0.9rem;
  color: #64748b;
  margin: 0 0 0.75rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.form label {
  font-size: 0.8rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.35rem;
}
.cb {
  width: 1.1rem;
  height: 1.1rem;
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
.warn {
  background: #fef9c3;
  padding: 0.5rem;
  border-radius: 6px;
  font-size: 0.85rem;
  margin: 0 0 0.5rem;
}
.pw-reset-wrap {
  margin-top: 0.75rem;
}
.pw-reset-lead {
  margin: 0;
  font-size: 0.9rem;
  color: #475569;
  line-height: 1.45;
}
.pw-result {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.pw-result-row {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.pw-result-label {
  font-size: 0.75rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.pw-result-value {
  font-size: 1rem;
  font-weight: 500;
  color: #0f172a;
}
.pw-box {
  margin: 0;
  padding: 0.65rem 0.75rem;
  background: #fef3c7;
  border-radius: 6px;
  font-size: 0.9rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.pw-box-value {
  font-size: 1.05rem;
  letter-spacing: 0.04em;
}
</style>
