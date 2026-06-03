<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import InputNumber from 'primevue/inputnumber'
import InputSwitch from 'primevue/inputswitch'
import InputText from 'primevue/inputtext'
import Dialog from 'primevue/dialog'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import {
  assignNFCTag,
  deleteFixedNonWorkWeekdays,
  deleteScheduleBound,
  deleteVacationEntitlement,
  fetchFixedNonWorkWeekdays,
  fetchScheduleBound,
  fetchNFCTags,
  fetchVacationEntitlements,
  deleteWeeklyHours,
  fetchWeeklyHours,
  patchEmployee,
  postEmployeeResetPassword,
  putFixedNonWorkWeekdays,
  putScheduleBound,
  putVacationEntitlement,
  putWeeklyHours,
} from '@/api/management'
import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import type {
  Employee,
  FixedNonWorkWeekdays,
  NFCTag,
  ScheduleBoundSetting,
  UserGroup,
  VacationEntitlement,
  WeeklyHours,
} from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { toastDetailAfterPasswordClipboard } from '@/utils/clipboard'
import { formatGermanDate, toISODateLocal } from '@/utils/dates'
import { scheduleBoundForDate } from '@/utils/timeTableModel'
import { canDeleteEntitlementEntry } from '@/utils/entryLock'
import { canManageEmployeeByRole } from '@/utils/roles'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const toast = useToast()
const auth = useAuthStore()

const employeeId = computed(() => Number(route.params.id))

const employee = ref<Employee | null>(null)
const groups = ref<UserGroup[]>([])
const groupDraft = ref<number | null>(null)
const displayName = ref('')
const active = ref(true)
const defaultTeamMeetingParticipant = ref(true)

const groupOptions = computed(() => [
  { label: 'Keine Gruppe', value: null as number | null },
  ...groups.value.map((g) => ({ label: g.name, value: g.id as number | null })),
])

const weeklyList = ref<WeeklyHours[]>([])
const fnwList = ref<FixedNonWorkWeekdays[]>([])
const scheduleBoundList = ref<ScheduleBoundSetting[]>([])
const scheduleBoundDraft = ref(true)
const scheduleBoundValidFrom = ref<Date>(new Date())
const vacationList = ref<VacationEntitlement[]>([])
const nfcTags = ref<NFCTag[]>([])

const whHours = ref(40)
const whValidFrom = ref<Date>(new Date())
const fnwDraft = ref<number[]>([])
const fnwValidFrom = ref<Date>(new Date())
const vacDays = ref(25)
const vacValidFrom = ref<Date>(new Date())

const tagUid = ref('')
const tagFrom = ref<Date>(new Date())

const FNW_WEEKDAY_OPTIONS = [
  { label: 'Mo', value: 1 },
  { label: 'Di', value: 2 },
  { label: 'Mi', value: 3 },
  { label: 'Do', value: 4 },
  { label: 'Fr', value: 5 },
] as const

function formatFnwWeekdays(weekdays: number[]): string {
  const labels: Record<number, string> = { 1: 'Mo', 2: 'Di', 3: 'Mi', 4: 'Do', 5: 'Fr' }
  const sorted = [...weekdays].sort((a, b) => a - b)
  return sorted.map((d) => labels[d] ?? '?').join(', ') || '—'
}

function toggleFnwDay(dow: number, checked: boolean) {
  if (checked) {
    if (!fnwDraft.value.includes(dow)) {
      fnwDraft.value = [...fnwDraft.value, dow].sort((a, b) => a - b)
    }
  } else {
    fnwDraft.value = fnwDraft.value.filter((d) => d !== dow)
  }
}

/** Mindestens 4 Byte-Blöcke (8 Hex-Zeichen), beliebig viele weitere. */
const TAG_UID_PATTERN = /^[0-9A-F]{2}(:[0-9A-F]{2}){3,}$/

function formatTagUidValue(raw: string): string {
  const hex = raw.toUpperCase().replace(/[^0-9A-F]/g, '')
  if (hex.length === 0) return ''
  let out = ''
  for (let i = 0; i < hex.length; i++) {
    out += hex.charAt(i)
    const n = i + 1
    if (n % 2 === 0 && n < hex.length) {
      out += ':'
    }
  }
  return out
}

function caretForHexCount(formatted: string, hexCount: number): number {
  let hex = 0
  for (let i = 0; i < formatted.length; i++) {
    if (formatted[i] !== ':') {
      hex++
      if (hex === hexCount) return i + 1
    }
  }
  return formatted.length
}

const tagUidValid = computed(() => TAG_UID_PATTERN.test(tagUid.value))

function onTagUidInput(e: Event) {
  const el = e.target as HTMLInputElement
  const oldCaret = el.selectionStart ?? el.value.length
  const hexBeforeCaret = el.value
    .slice(0, oldCaret)
    .toUpperCase()
    .replace(/[^0-9A-F]/g, '')
    .length
  const formatted = formatTagUidValue(el.value)
  tagUid.value = formatted
  const newCaret = caretForHexCount(formatted, hexBeforeCaret)
  nextTick(() => {
    el.value = formatted
    el.setSelectionRange(newCaret, newCaret)
  })
}

const openHours = ref(0)
const openVac = ref(0)

const saving = ref(false)

const canManageEmployee = computed(() =>
  employee.value ? canManageEmployeeByRole(auth.role, employee.value.role) : false,
)

const pwResetDialogVisible = ref(false)
const pwResetStep = ref<'confirm' | 'result'>('confirm')
const pwResetting = ref(false)
const pwResetTemp = ref('')

function openPwResetDialog() {
  pwResetStep.value = 'confirm'
  pwResetTemp.value = ''
  pwResetDialogVisible.value = true
}

function onPwResetDialogHide() {
  pwResetStep.value = 'confirm'
  pwResetTemp.value = ''
}

async function submitPwReset() {
  if (!employee.value || pwResetting.value) return
  pwResetting.value = true
  try {
    const res = await postEmployeeResetPassword(employee.value.id)
    const pw = res.temporary_password
    pwResetTemp.value = pw
    pwResetStep.value = 'result'
    const detail = await toastDetailAfterPasswordClipboard(pw)
    toast.add({
      severity: 'success',
      summary: 'Passwort zurückgesetzt',
      detail,
      life: 10000,
    })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Zurücksetzen fehlgeschlagen', life: 10000 })
  } finally {
    pwResetting.value = false
  }
}

async function load() {
  const [list, grp] = await Promise.all([fetchEmployees(), fetchGroups()])
  groups.value = grp
  employee.value = list.find((e) => e.id === employeeId.value) ?? null
  if (!employee.value) {
    router.replace('/employees')
    return
  }
  displayName.value = employee.value.display_name
  active.value = employee.value.active
  defaultTeamMeetingParticipant.value = employee.value.default_team_meeting_participant !== false
  groupDraft.value = employee.value.group_id ?? null
  openHours.value = employee.value.opening_hours_balance ?? 0
  openVac.value = employee.value.opening_vacation_days ?? 0
  const [wh, fnw, sb, vac, tags] = await Promise.all([
    fetchWeeklyHours(employee.value.id),
    fetchFixedNonWorkWeekdays(employee.value.id),
    fetchScheduleBound(employee.value.id),
    fetchVacationEntitlements(employee.value.id),
    fetchNFCTags(employee.value.id),
  ])
  weeklyList.value = wh
  fnwList.value = fnw
  scheduleBoundList.value = sb
  vacationList.value = vac
  nfcTags.value = tags
  const today = toISODateLocal(new Date())
  scheduleBoundDraft.value = scheduleBoundForDate(sb, today)
}

onMounted(load)
watch(employeeId, load)

watch(
  () => employee.value,
  (e) => {
    if (e && !canManageEmployeeByRole(auth.role, e.role)) {
      router.replace(`/employees/${e.id}`)
    }
  },
  { immediate: true },
)

async function saveOpening() {
  if (!employee.value) return
  saving.value = true
  try {
    await patchEmployee(employee.value.id, {
      opening_hours_balance: openHours.value,
      opening_vacation_days: openVac.value,
    })
    toast.add({ severity: 'success', summary: 'Startsaldo gespeichert', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function saveProfile() {
  if (!employee.value) return
  saving.value = true
  try {
    const body: {
      display_name: string
      active: boolean
      default_team_meeting_participant: boolean
      role?: string
      group_id?: number | null
    } = {
      display_name: displayName.value.trim(),
      active: active.value,
      default_team_meeting_participant: defaultTeamMeetingParticipant.value,
    }
    if (auth.role === 'superadmin' && roleDraft.value !== employee.value.role) {
      body.role = roleDraft.value
    }
    const prevG = employee.value.group_id ?? null
    if (groupDraft.value !== prevG) {
      body.group_id = groupDraft.value
    }
    await patchEmployee(employee.value.id, body)
    toast.add({ severity: 'success', summary: 'Gespeichert', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

const roleDraft = ref('user')

watch(employee, (e) => {
  if (e) roleDraft.value = e.role
})

async function saveWeekly() {
  if (!employee.value) return
  saving.value = true
  try {
    await putWeeklyHours(employee.value.id, {
      hours_per_week: whHours.value,
      valid_from: toISODateLocal(whValidFrom.value),
    })
    toast.add({ severity: 'success', summary: 'Wochenstunden gespeichert', life: 10000 })
    weeklyList.value = await fetchWeeklyHours(employee.value.id)
  } catch {
    toast.add({ severity: 'error', summary: 'Fehler Wochenstunden', life: 10000 })
  } finally {
    saving.value = false
  }
}

function canDeleteWeeklyRow(row: WeeklyHours) {
  return canDeleteEntitlementEntry(auth.role, row)
}

async function removeWeekly(row: WeeklyHours) {
  if (!employee.value) return
  const hint = `${row.hours_per_week} h ab ${formatGermanDate(row.valid_from)}`
  if (!confirm(`Wochenstunden-Eintrag löschen (${hint})?`)) return
  saving.value = true
  try {
    await deleteWeeklyHours(employee.value.id, row.id)
    toast.add({ severity: 'success', summary: 'Eintrag gelöscht', life: 10000 })
    weeklyList.value = await fetchWeeklyHours(employee.value.id)
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Löschen fehlgeschlagen',
      detail: 'Einträge sind 24 Stunden nach Anlage nicht mehr löschbar.',
      life: 10000,
    })
  } finally {
    saving.value = false
  }
}

async function saveScheduleBound() {
  if (!employee.value) return
  saving.value = true
  try {
    await putScheduleBound(employee.value.id, {
      schedule_bound: scheduleBoundDraft.value,
      valid_from: toISODateLocal(scheduleBoundValidFrom.value),
    })
    scheduleBoundList.value = await fetchScheduleBound(employee.value.id)
    toast.add({ severity: 'success', summary: 'Dienstplan-Bindung gespeichert', life: 10000 })
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

function canDeleteScheduleBoundRow(row: ScheduleBoundSetting) {
  return canDeleteEntitlementEntry(auth.role, row)
}

async function removeScheduleBound(row: ScheduleBoundSetting) {
  if (!employee.value || !canDeleteScheduleBoundRow(row)) return
  saving.value = true
  try {
    await deleteScheduleBound(employee.value.id, row.id)
    scheduleBoundList.value = await fetchScheduleBound(employee.value.id)
    const today = toISODateLocal(new Date())
    scheduleBoundDraft.value = scheduleBoundForDate(scheduleBoundList.value, today)
    toast.add({ severity: 'success', summary: 'Eintrag gelöscht', life: 10000 })
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

function scheduleBoundLabel(bound: boolean): string {
  return bound ? 'Ja' : 'Nein'
}

const scheduleBoundHelpText =
  'Wenn aktiviert, zählen erfasste Zeiten erst ab dem geplanten Schichtbeginn. Ein neuer Eintrag gilt ab dem gewählten Kalendertag und ersetzt den vorherigen Stand. Einträge können 24 Stunden nach Anlage nur noch vom Superadmin gelöscht werden.'

async function saveFixedNonWorkWeekdays() {
  if (!employee.value) return
  saving.value = true
  try {
    await putFixedNonWorkWeekdays(employee.value.id, {
      weekdays: [...fnwDraft.value],
      valid_from: toISODateLocal(fnwValidFrom.value),
    })
    toast.add({ severity: 'success', summary: 'Feste freie Wochentage gespeichert', life: 10000 })
    fnwList.value = await fetchFixedNonWorkWeekdays(employee.value.id)
  } catch {
    toast.add({ severity: 'error', summary: 'Fehler feste freie Wochentage', life: 10000 })
  } finally {
    saving.value = false
  }
}

function canDeleteFnwRow(row: FixedNonWorkWeekdays) {
  return canDeleteEntitlementEntry(auth.role, row)
}

async function removeFnw(row: FixedNonWorkWeekdays) {
  if (!employee.value) return
  const hint = `${formatFnwWeekdays(row.weekdays)} ab ${formatGermanDate(row.valid_from)}`
  if (!confirm(`Eintrag löschen (${hint})?`)) return
  saving.value = true
  try {
    await deleteFixedNonWorkWeekdays(employee.value.id, row.id)
    toast.add({ severity: 'success', summary: 'Eintrag gelöscht', life: 10000 })
    fnwList.value = await fetchFixedNonWorkWeekdays(employee.value.id)
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Löschen fehlgeschlagen',
      detail: 'Einträge sind 24 Stunden nach Anlage nicht mehr löschbar.',
      life: 10000,
    })
  } finally {
    saving.value = false
  }
}

function onVacValidFromCal(v: Date | Date[] | (Date | null)[] | null | undefined) {
  if (v == null || Array.isArray(v)) return
  vacValidFrom.value = v
}

async function saveVacation() {
  if (!employee.value) return
  saving.value = true
  try {
    await putVacationEntitlement(employee.value.id, {
      days_per_year: vacDays.value,
      valid_from: toISODateLocal(vacValidFrom.value),
    })
    toast.add({ severity: 'success', summary: 'Urlaubsanspruch gespeichert', life: 10000 })
    vacationList.value = await fetchVacationEntitlements(employee.value.id)
  } catch {
    toast.add({ severity: 'error', summary: 'Fehler Urlaub', life: 10000 })
  } finally {
    saving.value = false
  }
}

function canDeleteVacationRow(row: VacationEntitlement) {
  return canDeleteEntitlementEntry(auth.role, row)
}

async function removeVacation(row: VacationEntitlement) {
  if (!employee.value) return
  const hint = `${row.days_per_year} Tage ab ${formatGermanDate(row.valid_from)}`
  if (!confirm(`Urlaubsanspruch-Eintrag löschen (${hint})?`)) return
  saving.value = true
  try {
    await deleteVacationEntitlement(employee.value.id, row.id)
    toast.add({ severity: 'success', summary: 'Eintrag gelöscht', life: 10000 })
    vacationList.value = await fetchVacationEntitlements(employee.value.id)
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Löschen fehlgeschlagen',
      detail: 'Einträge sind 24 Stunden nach Anlage nicht mehr löschbar.',
      life: 10000,
    })
  } finally {
    saving.value = false
  }
}

async function assignTag() {
  if (!employee.value || !tagUidValid.value) return
  saving.value = true
  try {
    await assignNFCTag(employee.value.id, {
      tag_uid: tagUid.value,
      assigned_from: toISODateLocal(tagFrom.value),
    })
    tagUid.value = ''
    toast.add({ severity: 'success', summary: 'NFC-Tag zugewiesen', life: 10000 })
    nfcTags.value = await fetchNFCTags(employee.value.id)
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'NFC-Zuweisung fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}

function onTagUidEnter() {
  if (!tagUidValid.value || saving.value) return
  void assignTag()
}
</script>

<template>
  <div v-if="employee" class="page">
    <div class="page-head">
      <h2 class="name">{{ displayName || employee.display_name }}</h2>
      <p v-if="employee.username" class="name-sub">{{ employee.username }}</p>
    </div>

    <Card>
      <template #title>Mitarbeiter bearbeiten</template>
      <template #content>
        <div class="form">
          <label>Anzeigename</label>
          <InputText v-model="displayName" class="w" />
          <label class="row">
            <span>Aktiv</span>
            <InputSwitch v-model="active" />
          </label>
          <label class="row">
            <span>Nimmt standardmäßig an Teamsitzungen teil</span>
            <InputSwitch v-model="defaultTeamMeetingParticipant" />
          </label>
          <div class="schedule-bound-block">
            <label class="row">
              <span class="schedule-bound-label">
                An Dienstplan gebunden
                <i
                  class="pi pi-question-circle sb-help-icon"
                  tabindex="0"
                  aria-label="Hilfe: An Dienstplan gebunden"
                  v-tooltip.bottom="{ value: scheduleBoundHelpText, class: 'schedule-bound-help-tooltip' }"
                />
              </span>
              <InputSwitch v-model="scheduleBoundDraft" />
            </label>
            <div class="row2 mt">
              <div class="field">
                <label>Gültig ab</label>
                <DatePicker v-model="scheduleBoundValidFrom" date-format="dd.mm.yy" show-icon />
              </div>
              <div class="field field--action">
                <Button label="Speichern" :loading="saving" @click="saveScheduleBound" />
              </div>
            </div>
            <div v-if="scheduleBoundList.length" class="table-scroll mt">
              <DataTable :value="scheduleBoundList" data-key="id" size="small" striped-rows>
                <Column header="Gebunden">
                  <template #body="{ data }">{{ scheduleBoundLabel(data.schedule_bound) }}</template>
                </Column>
                <Column field="valid_from" header="Gültig ab">
                  <template #body="{ data }">{{ formatGermanDate(data.valid_from) }}</template>
                </Column>
                <Column header="" :exportable="false" style="width: 3.25rem; min-width: 3.25rem">
                  <template #body="{ data }">
                    <Button
                      v-if="canDeleteScheduleBoundRow(data)"
                      icon="pi pi-trash"
                      severity="danger"
                      text
                      rounded
                      :disabled="saving"
                      aria-label="Dienstplan-Bindung löschen"
                      @click="removeScheduleBound(data)"
                    />
                  </template>
                </Column>
              </DataTable>
            </div>
          </div>
          <template v-if="auth.role === 'superadmin'">
            <label>Rolle (nur Superadmin)</label>
            <select v-model="roleDraft" class="sel">
              <option value="user">Mitarbeiter</option>
              <option value="leitung">Leitung</option>
            </select>
          </template>
          <label>Gruppe</label>
          <Select
            v-model="groupDraft"
            :options="groupOptions"
            option-label="label"
            option-value="value"
            placeholder="Gruppe wählen"
            class="w"
            show-clear
          />
          <Button label="Profil speichern" :loading="saving" @click="saveProfile" />
          <div v-if="canManageEmployee" class="pw-reset-row">
            <Button
              label="Passwort zurücksetzen"
              icon="pi pi-key"
              severity="secondary"
              outlined
              type="button"
              @click="openPwResetDialog"
            />
          </div>
        </div>
      </template>
    </Card>

    <Dialog
      v-model:visible="pwResetDialogVisible"
      :header="pwResetStep === 'confirm' ? 'Passwort zurücksetzen' : 'Neues Einmalpasswort'"
      modal
      :style="{ width: '440px' }"
      @hide="onPwResetDialogHide"
    >
      <template v-if="pwResetStep === 'confirm'">
        <p class="pw-reset-lead">
          Das bisherige Passwort von <strong>{{ employee.display_name }}</strong> funktioniert danach nicht mehr. Es
          wird ein neues Einmalpasswort erzeugt; die Person muss bei der nächsten Anmeldung ein neues Passwort wählen.
        </p>
      </template>
      <div v-else-if="pwResetStep === 'result'" class="pw-result">
        <p class="pw-reset-lead">Bitte Zugangsdaten sicher übermitteln — der Account ist bereits aktualisiert.</p>
        <div class="pw-result-row">
          <span class="pw-result-label">Benutzername</span>
          <span class="pw-result-value">{{ employee.username }}</span>
        </div>
        <div class="pw-result-row">
          <span class="pw-result-label">Anzeigename</span>
          <span class="pw-result-value">{{ employee.display_name }}</span>
        </div>
        <div class="pw-box">
          <span class="pw-result-label">Einmalpasswort</span>
          <strong class="pw-box-value">{{ pwResetTemp }}</strong>
        </div>
      </div>
      <template #footer>
        <template v-if="pwResetStep === 'confirm'">
          <Button label="Abbrechen" severity="secondary" text @click="pwResetDialogVisible = false" />
          <Button label="Zurücksetzen" :loading="pwResetting" @click="submitPwReset" />
        </template>
        <Button v-else label="Schließen" @click="pwResetDialogVisible = false" />
      </template>
    </Dialog>

    <Card id="startsaldo">
      <template #title>Startsaldo (Import / Alt-System)</template>
      <template #content>
        <p class="muted">
          Optional: Übernommener Stand aus einem früheren System. Stunden: werden in die Jahressaldo-Berechnung
          (Ist−Soll) einbezogen. Urlaub: addiert zu Anspruch abzüglich erfasster Urlaubstage.
        </p>
        <div class="row2">
          <div class="field">
            <label>Stundensaldo (h)</label>
            <InputNumber
              v-model="openHours"
              :min="-500"
              :max="2000"
              :step="0.25"
              :max-fraction-digits="2"
            />
          </div>
          <div class="field">
            <label>Urlaub (Tage)</label>
            <InputNumber
              v-model="openVac"
              :min="-30"
              :max="200"
              :step="0.5"
              :max-fraction-digits="1"
            />
          </div>
          <div class="field field--action">
            <Button label="Startsaldo speichern" :loading="saving" @click="saveOpening" />
          </div>
        </div>
      </template>
    </Card>

    <Card>
      <template #title>Wochenstunden</template>
      <template #content>
        <p class="muted">
          Ein neuer Eintrag gilt ab dem gewählten Kalendertag und beendet den vorherigen Block automatisch. Einträge
          können 24 Stunden nach Anlage nur noch vom Superadmin gelöscht werden.
        </p>
        <div class="row2">
          <div class="field">
            <label>Stunden / Woche</label>
            <InputNumber v-model="whHours" :min="0" :max="80" :step="0.5" />
          </div>
          <div class="field">
            <label>Gültig ab</label>
            <DatePicker v-model="whValidFrom" date-format="dd.mm.yy" show-icon />
          </div>
          <div class="field field--action">
            <Button label="Wochenstunden speichern" :loading="saving" @click="saveWeekly" />
          </div>
        </div>
        <div class="table-scroll mt">
          <DataTable :value="weeklyList" data-key="id" size="small" striped-rows>
            <Column field="hours_per_week" header="Std." />
            <Column field="valid_from" header="Gültig ab">
              <template #body="{ data }">{{ formatGermanDate(data.valid_from) }}</template>
            </Column>
            <Column header="" :exportable="false" style="width: 3.25rem; min-width: 3.25rem">
              <template #body="{ data }">
                <Button
                  v-if="canDeleteWeeklyRow(data)"
                  icon="pi pi-trash"
                  severity="danger"
                  text
                  rounded
                  :disabled="saving"
                  aria-label="Wochenstunden löschen"
                  @click="removeWeekly(data)"
                />
              </template>
            </Column>
          </DataTable>
        </div>
      </template>
    </Card>

    <Card>
      <template #title>Feste freie Wochentage</template>
      <template #content>
        <p class="muted">
          Reguläre freie Wochentage (Mo–Fr) ohne Schichtplanung und ohne Urlaubsabzug an Schließtagen. Ein neuer
          Eintrag gilt ab dem gewählten Kalendertag. Einträge können 24 Stunden nach Anlage nur noch vom Superadmin
          gelöscht werden.
        </p>
        <div class="fnw-days">
          <label v-for="opt in FNW_WEEKDAY_OPTIONS" :key="opt.value" class="fnw-day">
            <Checkbox
              :binary="true"
              :model-value="fnwDraft.includes(opt.value)"
              @update:model-value="(v: boolean) => toggleFnwDay(opt.value, Boolean(v))"
            />
            <span>{{ opt.label }}</span>
          </label>
        </div>
        <div class="row2 mt">
          <div class="field">
            <label>Gültig ab</label>
            <DatePicker v-model="fnwValidFrom" date-format="dd.mm.yy" show-icon />
          </div>
          <div class="field field--action">
            <Button label="Speichern" :loading="saving" @click="saveFixedNonWorkWeekdays" />
          </div>
        </div>
        <div class="table-scroll mt">
          <DataTable :value="fnwList" data-key="id" size="small" striped-rows>
            <Column header="Wochentage">
              <template #body="{ data }">{{ formatFnwWeekdays(data.weekdays) }}</template>
            </Column>
            <Column field="valid_from" header="Gültig ab">
              <template #body="{ data }">{{ formatGermanDate(data.valid_from) }}</template>
            </Column>
            <Column header="" :exportable="false" style="width: 3.25rem; min-width: 3.25rem">
              <template #body="{ data }">
                <Button
                  v-if="canDeleteFnwRow(data)"
                  icon="pi pi-trash"
                  severity="danger"
                  text
                  rounded
                  :disabled="saving"
                  aria-label="Feste freie Wochentage löschen"
                  @click="removeFnw(data)"
                />
              </template>
            </Column>
          </DataTable>
        </div>
      </template>
    </Card>

    <Card>
      <template #title>Urlaubsanspruch</template>
      <template #content>
        <p class="muted">
          Ein neuer Eintrag gilt ab dem gewählten Kalendertag und beendet den vorherigen Block automatisch. Der
          Jahresanspruch wird anteilig nach Restmonaten und Resttagen im Monat (je Zwölftel) berechnet und auf halbe
          Urlaubstage aufgerundet. Einträge können 24 Stunden nach Anlage nur noch vom Superadmin gelöscht werden.
        </p>
        <div class="row2">
          <div class="field">
            <label>Tage / Jahr</label>
            <InputNumber v-model="vacDays" :min="0" :max="60" />
          </div>
          <div class="field">
            <label>Gültig ab</label>
            <DatePicker
              v-model="vacValidFrom"
              date-format="dd.mm.yy"
              show-icon
              @update:model-value="onVacValidFromCal"
            />
          </div>
          <div class="field field--action">
            <Button label="Urlaub speichern" :loading="saving" @click="saveVacation" />
          </div>
        </div>
        <div class="table-scroll mt">
          <DataTable :value="vacationList" data-key="id" size="small" striped-rows>
            <Column field="days_per_year" header="Tage" />
            <Column field="valid_from" header="Gültig ab">
              <template #body="{ data }">{{ formatGermanDate(data.valid_from) }}</template>
            </Column>
            <Column header="" :exportable="false" style="width: 3.25rem; min-width: 3.25rem">
              <template #body="{ data }">
                <Button
                  v-if="canDeleteVacationRow(data)"
                  icon="pi pi-trash"
                  severity="danger"
                  text
                  rounded
                  :disabled="saving"
                  aria-label="Urlaubsanspruch löschen"
                  @click="removeVacation(data)"
                />
              </template>
            </Column>
          </DataTable>
        </div>
      </template>
    </Card>

    <Card>
      <template #title>NFC-Tags</template>
      <template #content>
        <div class="row2">
          <div class="field">
            <label>Tag-UID</label>
            <InputText
              :model-value="tagUid"
              class="w"
              placeholder="z. B. A1:B2:C3:D4 (mind. 4 Blöcke)"
              inputmode="text"
              autocapitalize="characters"
              autocomplete="off"
              spellcheck="false"
              @input="onTagUidInput"
              @keydown.enter.prevent="onTagUidEnter"
            />
          </div>
          <div class="field">
            <label>Zugewiesen ab</label>
            <DatePicker v-model="tagFrom" date-format="dd.mm.yy" show-icon />
          </div>
          <div class="field field--action">
            <Button label="Tag zuweisen" :loading="saving" :disabled="!tagUidValid" @click="assignTag" />
          </div>
        </div>
        <p class="muted">
          Ein neuer Eintrag gilt ab dem gewählten Kalendertag und ersetzt den vorherigen Tag für spätere Tage. Derselbe
          Tag kann nicht gleichzeitig zwei aktiven Mitarbeitern zugeordnet werden.
        </p>
        <DataTable :value="nfcTags" size="small" striped-rows class="mt" data-key="id">
          <Column field="tag_uid" header="UID" />
          <Column field="assigned_from" header="Zugewiesen ab">
            <template #body="{ data }">{{ formatGermanDate(data.assigned_from) }}</template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Button label="Zurück zur Übersicht" severity="secondary" text @click="router.push('/employees')" />
  </div>
</template>

<style scoped>
.page {
  max-width: 720px;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}
.page-head {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}
.name {
  margin: 0;
  font-size: 1.25rem;
  color: #0f172a;
  font-weight: 700;
}
.name-sub {
  margin: 0;
  color: #64748b;
  font-size: 0.9rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 400px;
}
.form label,
.field label,
.row2 label {
  font-size: 0.8rem;
  color: #64748b;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 12rem;
}
.field--action {
  align-self: stretch;
  justify-content: flex-end;
}
.row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.row2 {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: flex-end;
}
.w {
  width: 100%;
  max-width: 360px;
}
.sel {
  padding: 0.5rem;
  border-radius: 6px;
  border: 1px solid #cbd5e1;
  max-width: 360px;
}
.schedule-bound-block {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.25rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--p-content-border-color, #dee2e6);
}

.schedule-bound-label {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.sb-help-icon {
  font-size: 0.95rem;
  color: #64748b;
  cursor: help;
}

:global(.schedule-bound-help-tooltip) {
  max-width: 22rem;
  white-space: normal;
  line-height: 1.4;
}

.muted {
  font-size: 0.85rem;
  color: #64748b;
  margin: 0 0 0.75rem;
}
.table-scroll {
  max-width: 100%;
  overflow-x: auto;
}
.mt {
  margin-top: 0.75rem;
}
.fnw-days {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem 1.25rem;
}
.fnw-day {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.95rem;
  color: #334155;
  cursor: pointer;
}
.pw-reset-row {
  margin-top: 0.35rem;
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
