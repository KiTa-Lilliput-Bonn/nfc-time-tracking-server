<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputSwitch from 'primevue/inputswitch'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import {
  createEmployeeAbsence,
  deleteEmployeeAbsence,
  fetchEmployeeAbsences,
  fetchEmployeeCompensationDayClaims,
  fetchClosureDays,
  fetchHolidays,
  fetchTeamOverview,
  waiveEmployeeCompensationDayClaim,
} from '@/api/management'
import { fetchEmployees } from '@/api/employees'
import type { Absence, ClosureDay, CompensationDayClaim, Employee } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { friendlyAbsenceCreateError } from '@/utils/absenceErrors'
import { endOfMonth, formatGermanDate, parseISODate, startOfMonth, toISODateLocal } from '@/utils/dates'
import { focusSelectFilterOnShow, openSelectDropdown } from '@/utils/selectFilterFocus'
import {
  countSkippedNonWorkdays,
  enumerateInclusiveCalendarISO,
  enumerateWorkdayISO,
  holidayDateSetForRange,
  normalizeISODate,
  vacationDisplayGapOnlySkippable,
} from '@/utils/workdays'

function germanWeekdayLong(d: Date): string {
  return new Intl.DateTimeFormat('de-DE', { weekday: 'long' }).format(d)
}

const toast = useToast()

const employees = ref<Employee[]>([])
const from = ref<Date>(startOfMonth(new Date()))
const to = ref<Date>(endOfMonth(new Date()))
const filterEmployeeId = ref<number | null>(null)

interface Row extends Absence {
  employeeName: string
}

const rows = ref<Row[]>([])
const loading = ref(false)
const closures = ref<ClosureDay[]>([])
const holidayDates = ref<Set<string>>(new Set())

const waiveEmployeeId = ref<number | null>(null)
const waiveClaims = ref<CompensationDayClaim[]>([])
const waiveLoading = ref(false)
const waivingClaimId = ref<number | null>(null)
/** User-IDs mit mindestens einem offenen Ausgleichstag-Anspruch (wie Team-Übersicht). */
const waiveEligibleIds = ref<number[]>([])
const waiveOverviewLoading = ref(false)

const waiveEmployeeOptions = computed(() => {
  const allow = new Set(waiveEligibleIds.value)
  return employees.value
    .filter((e) => allow.has(e.id))
    .map((e) => ({ label: e.display_name, value: e.id }))
})

async function refreshWaiveEligibleIds() {
  waiveOverviewLoading.value = true
  try {
    const data = await fetchTeamOverview(new Date().getFullYear())
    waiveEligibleIds.value = data.rows
      .filter((r) => r.compensation_day_claims_open > 0)
      .map((r) => r.id)
  } catch {
    waiveEligibleIds.value = []
    toast.add({
      severity: 'error',
      summary: 'Ausgleichstag-Ansprüche',
      detail: 'Die Übersicht für verfügbare Ansprüche konnte nicht geladen werden.',
      life: 10000,
    })
  } finally {
    waiveOverviewLoading.value = false
  }
}

function syncWaiveEmployeeSelection() {
  const opts = waiveEmployeeOptions.value
  if (opts.length === 0) {
    waiveEmployeeId.value = null
    return
  }
  if (waiveEmployeeId.value != null && opts.some((o) => o.value === waiveEmployeeId.value)) {
    return
  }
  waiveEmployeeId.value = opts[0].value
}

const employeeOptions = computed(() => [
  { label: 'Alle', value: null as number | null },
  ...employees.value.map((e) => ({ label: e.display_name, value: e.id })),
])

const filteredRows = computed(() => {
  if (filterEmployeeId.value == null) return rows.value
  return rows.value.filter((r) => r.user_id === filterEmployeeId.value)
})
/** Standard-Sortierung: Datum absteigend (PrimeVue Pfeile im Header). */
const sortField = ref('from')
const sortOrder = ref<number>(-1)

const closureDateSet = computed(() => new Set(closures.value.map((c) => normalizeISODate(c.closure_date))))

interface ViewRow {
  kind: 'range' | 'single'
  rowKey: string
  user_id: number
  employeeName: string
  absence_type: string
  half_day: boolean
  vacation_days?: number
  from: string
  to?: string
  ids: number[]
}

const viewRows = computed<ViewRow[]>(() => {
  const list = filteredRows.value
    .map((r) => ({ ...r, absence_date: normalizeISODate(r.absence_date) }))
    .sort((a, b) => b.absence_date.localeCompare(a.absence_date) || (b.id ?? 0) - (a.id ?? 0))

  const singles: ViewRow[] = []
  const vacByUser = new Map<number, Row[]>()
  for (const r of list) {
    if (r.absence_type === 'vacation' && !r.half_day) {
      const arr = vacByUser.get(r.user_id) ?? []
      arr.push(r)
      vacByUser.set(r.user_id, arr)
    } else {
      singles.push({
        kind: 'single',
        rowKey: `a-${r.id}`,
        user_id: r.user_id,
        employeeName: r.employeeName,
        absence_type: r.absence_type,
        half_day: r.half_day,
        vacation_days: r.absence_type === 'vacation' ? (r.half_day ? 0.5 : 1) : undefined,
        from: r.absence_date,
        ids: [r.id],
      })
    }
  }

  const ranges: ViewRow[] = []
  const skipBase = new Set<string>([...closureDateSet.value, ...holidayDates.value])
  const fixedByUser = new Map<number, Set<number>>()
  for (const e of employees.value) {
    fixedByUser.set(e.id, new Set(e.fixed_non_work_weekdays ?? []))
  }

  for (const [uid, arr0] of vacByUser.entries()) {
    const fixedSet = fixedByUser.get(uid) ?? new Set<number>()
    const arr = arr0.slice().sort((a, b) => a.absence_date.localeCompare(b.absence_date))
    let curFrom = ''
    let curTo = ''
    let curIds: number[] = []
    let curLen = 0
    let curName = ''

    function flush() {
      if (!curLen) return
      ranges.push({
        kind: curLen > 1 ? 'range' : 'single',
        rowKey: curLen > 1 ? `vr-${uid}-${curFrom}-${curTo}` : `a-${curIds[0]}`,
        user_id: uid,
        employeeName: curName,
        absence_type: 'vacation',
        half_day: false,
        vacation_days: curIds.length,
        from: curFrom,
        to: curLen > 1 ? curTo : undefined,
        ids: [...curIds],
      })
      curFrom = ''
      curTo = ''
      curIds = []
      curLen = 0
      curName = ''
    }

    for (const r of arr) {
      const iso = r.absence_date
      if (!curLen) {
        curFrom = iso
        curTo = iso
        curIds = [r.id]
        curLen = 1
        curName = r.employeeName
        continue
      }
      if (vacationDisplayGapOnlySkippable(curTo, iso, skipBase, fixedSet)) {
        curTo = iso
        curIds.push(r.id)
        curLen++
        continue
      }
      flush()
      curFrom = iso
      curTo = iso
      curIds = [r.id]
      curLen = 1
      curName = r.employeeName
    }
    flush()
  }

  return [...ranges, ...singles].sort(
    (a, b) => b.from.localeCompare(a.from) || a.employeeName.localeCompare(b.employeeName, 'de'),
  )
})

const typeOptions = [
  { label: 'Krank', value: 'sick' as const },
  { label: 'Urlaub', value: 'vacation' as const },
  { label: 'Ausgleichstag', value: 'compensation_day' as const },
  { label: 'Sonstiges', value: 'other' as const },
]

async function loadWaiveClaims() {
  if (waiveEmployeeId.value == null) {
    waiveClaims.value = []
    return
  }
  waiveLoading.value = true
  try {
    waiveClaims.value = await fetchEmployeeCompensationDayClaims(waiveEmployeeId.value, 'open')
  } catch {
    waiveClaims.value = []
    toast.add({
      severity: 'error',
      summary: 'Ausgleichstag-Ansprüche',
      detail: 'Die Anspruchsliste konnte nicht geladen werden.',
      life: 10000,
    })
  } finally {
    waiveLoading.value = false
  }
}

watch(waiveEmployeeId, () => {
  void loadWaiveClaims()
})

async function load() {
  loading.value = true
  try {
    employees.value = await fetchEmployees()
    await refreshWaiveEligibleIds()
    syncWaiveEmployeeSelection()
    closures.value = await fetchClosureDays().catch((): ClosureDay[] => [])
    try {
      holidayDates.value = await holidayDateSetForRange(from.value, to.value, fetchHolidays)
    } catch {
      holidayDates.value = new Set()
    }
    const f = toISODateLocal(from.value)
    const t = toISODateLocal(to.value)
    const next: Row[] = []
    for (const e of employees.value) {
      try {
        const abs = await fetchEmployeeAbsences(e.id, f, t)
        for (const a of abs) next.push({ ...a, employeeName: e.display_name })
      } catch {
        /* skip */
      }
    }
    next.sort((a, b) => b.absence_date.localeCompare(a.absence_date))
    rows.value = next
    await loadWaiveClaims()
  } catch {
    toast.add({ severity: 'error', summary: 'Abwesenheiten', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch([from, to], load)

const showAdd = ref(false)
const addEmpId = ref<number | null>(null)
const addDateFrom = ref<Date>(new Date())
const addDateTo = ref<Date>(new Date())
const addType = ref<'sick' | 'vacation' | 'other' | 'compensation_day'>('vacation')
const addHalf = ref(false)
const saving = ref(false)
const openCompensationDayClaims = ref<CompensationDayClaim[]>([])
const addEmpSelect = ref<any>(null)

const showVacationEdit = ref(false)
const editingVacationRow = ref<ViewRow | null>(null)
const editFrom = ref<Date>(new Date())
const editTo = ref<Date>(new Date())
const savingVacationEdit = ref(false)

function syncAddDateToFromStart() {
  const d = addDateFrom.value
  addDateTo.value = new Date(d.getFullYear(), d.getMonth(), d.getDate())
}

watch(addType, (t) => {
  if (t === 'compensation_day') {
    addHalf.value = false
  }
  syncAddDateToFromStart()
})

watch(addDateFrom, () => {
  syncAddDateToFromStart()
})

watch([addEmpId, addType], async () => {
  if (addType.value === 'compensation_day') {
    addHalf.value = false
  }
  if (addType.value !== 'compensation_day' || addEmpId.value == null) {
    openCompensationDayClaims.value = []
    return
  }
  try {
    openCompensationDayClaims.value = await fetchEmployeeCompensationDayClaims(addEmpId.value, 'open')
  } catch {
    openCompensationDayClaims.value = []
  }
})

function openAdd() {
  addEmpId.value = employees.value[0]?.id ?? null
  const today = new Date()
  addDateFrom.value = new Date(today)
  addDateTo.value = new Date(today)
  addType.value = 'vacation'
  addHalf.value = false
  showAdd.value = true
  // Dialog transition + overlay init; open dropdown so filter is immediately usable.
  setTimeout(() => openSelectDropdown(addEmpSelect.value), 50)
}

function normalizeAddRange(): [Date, Date] {
  const a = new Date(addDateFrom.value.getFullYear(), addDateFrom.value.getMonth(), addDateFrom.value.getDate())
  const b = new Date(addDateTo.value.getFullYear(), addDateTo.value.getMonth(), addDateTo.value.getDate())
  if (a.getTime() <= b.getTime()) return [a, b]
  return [b, a]
}

/** Urlaub: nur Werktage ohne Feiertage/Schließtage/fix frei. */
function enumerateVacationWorkdayISO(from: Date, to: Date, holidaySet: Set<string>, closureSet: Set<string>): string[] {
  const emp = employees.value.find((e) => e.id === addEmpId.value)
  const fixed = new Set(emp?.fixed_non_work_weekdays ?? [])
  return enumerateWorkdayISO(from, to, holidaySet, closureSet, fixed)
}

async function submitAdd() {
  if (addEmpId.value == null) return

  const [df, dt] = normalizeAddRange()
  const isoSingle = toISODateLocal(df)

  if (addType.value === 'compensation_day') {
    const emp = employees.value.find((e) => e.id === addEmpId.value)
    const fixed = new Set(emp?.fixed_non_work_weekdays ?? [])
    const dow = df.getDay()
    const pretty = formatGermanDate(isoSingle)
    const weekday = germanWeekdayLong(df)
    if (fixed.has(dow)) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — das ist ein fester freier Wochentag (Dienstplan).`,
        life: 10000,
      })
      return
    }
    if (addHalf.value) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: 'Halbe Ausgleichstage sind nicht möglich.',
        life: 10000,
      })
      return
    }
    if (dow === 0 || dow === 6) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — das ist ein Wochenende.`,
        life: 10000,
      })
      return
    }
    try {
      const hol = await fetchHolidays(df.getFullYear())
      const hit = hol.find((h) => h.holiday_date === isoSingle)
      if (hit) {
        toast.add({
          severity: 'error',
          summary: 'Ausgleichstag nicht möglich',
          detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — „${hit.name}“ ist ein gesetzlicher Feiertag.`,
          life: 10000,
        })
        return
      }
      const claims = await fetchEmployeeCompensationDayClaims(addEmpId.value, 'open')
      if (claims.length === 0) {
        toast.add({
          severity: 'error',
          summary: 'Ausgleichstag nicht möglich',
          detail: 'Für diesen Mitarbeiter ist kein offener Ausgleichstag-Anspruch vorhanden.',
          life: 10000,
        })
        return
      }
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag',
        detail: 'Die Ausgleichstag-Ansprüche konnten nicht geprüft werden. Bitte erneut versuchen.',
        life: 10000,
      })
      return
    }
    saving.value = true
    try {
      await createEmployeeAbsence(addEmpId.value, {
        absence_date: isoSingle,
        absence_type: 'compensation_day',
        half_day: false,
      })
      toast.add({ severity: 'success', summary: 'Eingetragen', life: 10000 })
      showAdd.value = false
      await load()
    } catch (e) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail:
          friendlyAbsenceCreateError({
            apiMessage: getApiErrorMessage(e),
            isoDate: isoSingle,
            absenceLabel: 'Ausgleichstag',
          }) ??
          'Die Abwesenheit konnte nicht gespeichert werden. Bitte Eingabe prüfen und erneut versuchen.',
        life: 10000,
      })
    } finally {
      saving.value = false
    }
    return
  }

  let datesToBook: string[] = []

  if (addType.value === 'vacation') {
    let holidaySet: Set<string>
    try {
      holidaySet = await holidayDateSetForRange(df, dt, fetchHolidays)
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Feiertage',
        detail: 'Die Feiertagsliste konnte nicht geladen werden. Bitte erneut versuchen oder Administrator informieren.',
        life: 10000,
      })
      return
    }
    const allCal = enumerateInclusiveCalendarISO(df, dt)
    datesToBook = enumerateVacationWorkdayISO(df, dt, holidaySet, closureDateSet.value)
    if (datesToBook.length === 0) {
      toast.add({
        severity: 'error',
        summary: 'Urlaub nicht möglich',
        detail:
          'Im gewählten Zeitraum gibt es keinen Werktag ohne Feiertag/Schließtag/fix frei — nur Wochenenden und/oder übersprungene Tage.',
        life: 10000,
      })
      return
    }
    const emp = employees.value.find((e) => e.id === addEmpId.value)
    const fixedSet = new Set(emp?.fixed_non_work_weekdays ?? [])
    const skipped = countSkippedNonWorkdays(allCal, holidaySet, closureDateSet.value, fixedSet)
    if (addHalf.value && datesToBook.length !== 1) {
      toast.add({
        severity: 'error',
        summary: 'Urlaub nicht möglich',
        detail: 'Halbtags-Urlaub ist nur an einem einzelnen Arbeitstag möglich.',
        life: 10000,
      })
      return
    }
    saving.value = true
    try {
      let ok = 0
      for (const iso of datesToBook) {
        await createEmployeeAbsence(addEmpId.value, {
          absence_date: iso,
          absence_type: 'vacation',
          half_day: addHalf.value,
        })
        ok++
      }
      const summary = ok === 1 ? 'Urlaub eingetragen' : `${ok} Urlaubstage eingetragen`
      toast.add({ severity: 'success', summary, life: 10000 })
      if (skipped > 0) {
        toast.add({
          severity: 'info',
          summary: 'Hinweis',
          detail: `${skipped} Kalendertag(e) waren Wochenende, Feiertag, Schließtag oder fix frei und wurden nicht als Urlaub gebucht.`,
          life: 10000,
        })
      }
      showAdd.value = false
      await load()
    } catch (e) {
      toast.add({
        severity: 'error',
        summary: 'Urlaub nicht möglich',
        detail:
          friendlyAbsenceCreateError({
            apiMessage: getApiErrorMessage(e),
            isoDate: datesToBook[0] ?? toISODateLocal(df),
            absenceLabel: 'Urlaub',
          }) ??
          'Die Abwesenheit konnte nicht gespeichert werden. Bitte Eingabe prüfen und erneut versuchen.',
        life: 10000,
      })
    } finally {
      saving.value = false
    }
    return
  }

  datesToBook = enumerateInclusiveCalendarISO(df, dt)
  if (addHalf.value && datesToBook.length !== 1) {
    toast.add({
      severity: 'error',
      summary: 'Halbtag nicht möglich',
      detail: 'Halbtags-Abwesenheit ist nur möglich, wenn „Von“ und „Bis“ derselbe Kalendertag sind.',
      life: 10000,
    })
    return
  }

  saving.value = true
  try {
    let ok = 0
    for (const iso of datesToBook) {
      await createEmployeeAbsence(addEmpId.value, {
        absence_date: iso,
        absence_type: addType.value,
        half_day: addHalf.value,
      })
      ok++
    }
    const summary = ok === 1 ? 'Eingetragen' : `${ok} Abwesenheiten eingetragen`
    toast.add({ severity: 'success', summary, life: 10000 })
    showAdd.value = false
    await load()
  } catch (e) {
    const apiMsg = getApiErrorMessage(e)
    const singleISO = datesToBook.length === 1 ? datesToBook[0]! : toISODateLocal(df)
    const detail =
      friendlyAbsenceCreateError({
        apiMessage: apiMsg,
        isoDate: singleISO,
        absenceLabel: addType.value === 'sick' ? 'Krankmeldung' : 'Abwesenheit',
      }) ?? 'Die Abwesenheit konnte nicht gespeichert werden. Bitte Eingabe prüfen und erneut versuchen.'
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      detail,
      life: 10000,
    })
  } finally {
    saving.value = false
  }
}

async function waiveClaim(claim: CompensationDayClaim) {
  if (waiveEmployeeId.value == null) return
  const pretty = formatGermanDate(claim.work_date)
  if (
    !confirm(
      `Anspruch für den gearbeiteten ${pretty} wirklich streichen?\n\nDer Verzicht entfernt nur den Anspruch auf einen freien Ausgleichstag. Überstunden aus der Wochenendarbeit bleiben unverändert.`,
    )
  ) {
    return
  }
  waivingClaimId.value = claim.id
  try {
    await waiveEmployeeCompensationDayClaim(waiveEmployeeId.value, claim.id)
    toast.add({ severity: 'success', summary: 'Anspruch gestrichen', life: 10000 })
    await refreshWaiveEligibleIds()
    syncWaiveEmployeeSelection()
    await loadWaiveClaims()
    if (addEmpId.value === waiveEmployeeId.value && addType.value === 'compensation_day') {
      try {
        openCompensationDayClaims.value = await fetchEmployeeCompensationDayClaims(
          waiveEmployeeId.value,
          'open',
        )
      } catch {
        openCompensationDayClaims.value = []
      }
    }
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Verzicht fehlgeschlagen',
      detail: getApiErrorMessage(e) ?? 'Der Anspruch konnte nicht gestrichen werden.',
      life: 10000,
    })
  } finally {
    waivingClaimId.value = null
  }
}

function openVacationEdit(row: ViewRow) {
  if (row.absence_type !== 'vacation' || row.half_day) return
  editingVacationRow.value = row
  editFrom.value = parseISODate(row.from)
  editTo.value = parseISODate(row.to ?? row.from)
  showVacationEdit.value = true
}

async function submitVacationEdit() {
  const row = editingVacationRow.value
  if (!row) return
  const df = new Date(editFrom.value.getFullYear(), editFrom.value.getMonth(), editFrom.value.getDate())
  const dt = new Date(editTo.value.getFullYear(), editTo.value.getMonth(), editTo.value.getDate())
  if (df.getTime() > dt.getTime()) {
    toast.add({ severity: 'error', summary: 'Ungültiger Zeitraum', detail: '„Bis“ muss nach „Von“ liegen.', life: 10000 })
    return
  }
  let holidaySet: Set<string>
  try {
    holidaySet = await holidayDateSetForRange(df, dt, fetchHolidays)
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Feiertage',
      detail: 'Die Feiertagsliste konnte nicht geladen werden. Bitte erneut versuchen.',
      life: 10000,
    })
    return
  }
  const datesToBook = enumerateVacationWorkdayISO(df, dt, holidaySet, closureDateSet.value)
  if (datesToBook.length === 0) {
    toast.add({
      severity: 'error',
      summary: 'Urlaub nicht möglich',
      detail: 'Im gewählten Zeitraum gibt es keinen Werktag ohne Feiertag/Schließtag.',
      life: 10000,
    })
    return
  }

  savingVacationEdit.value = true
  try {
    for (const id of row.ids) {
      await deleteEmployeeAbsence(row.user_id, id)
    }
    for (const iso of datesToBook) {
      await createEmployeeAbsence(row.user_id, {
        absence_date: iso,
        absence_type: 'vacation',
        half_day: false,
      })
    }
    toast.add({ severity: 'success', summary: 'Urlaubszeitraum gespeichert', life: 10000 })
    showVacationEdit.value = false
    editingVacationRow.value = null
    await load()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      detail:
        friendlyAbsenceCreateError({
          apiMessage: getApiErrorMessage(e),
          isoDate: datesToBook[0] ?? row.from,
          absenceLabel: 'Urlaub',
        }) ?? 'Der Urlaubszeitraum konnte nicht gespeichert werden.',
      life: 10000,
    })
  } finally {
    savingVacationEdit.value = false
  }
}

async function removeRow(row: ViewRow) {
  const msg = row.kind === 'range' ? 'Urlaubszeitraum löschen?' : 'Abwesenheit löschen?'
  if (!confirm(msg)) return
  try {
    for (const id of row.ids) {
      await deleteEmployeeAbsence(row.user_id, id)
    }
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  }
}

function typeLabel(t: string) {
  if (t === 'sick') return 'Krank'
  if (t === 'vacation') return 'Urlaub'
  if (t === 'compensation_day') return 'Ausgleichstag'
  return 'Sonstiges'
}
</script>

<template>
  <div class="page">
    <div class="page-grid">
      <div class="page-main">
    <Card>
      <template #title>Abwesenheiten</template>
      <template #content>
        <div class="toolbar">
          <div class="range">
            <label class="lbl">Von</label>
            <DatePicker v-model="from" date-format="dd.mm.yy" show-icon />
            <label class="lbl">Bis</label>
            <DatePicker v-model="to" date-format="dd.mm.yy" show-icon />
          </div>
          <Select
            v-model="filterEmployeeId"
            :options="employeeOptions"
            option-label="label"
            option-value="value"
            placeholder="Mitarbeiter"
            class="filt"
            filter
            @show="focusSelectFilterOnShow"
          />
          <Button label="Abwesenheit eintragen" icon="pi pi-plus" data-testid="absences-add-btn" @click="openAdd" />
        </div>
        <div class="absences-table-wrap">
          <DataTable
            v-model:sort-field="sortField"
            v-model:sort-order="sortOrder"
            :value="viewRows"
            :loading="loading"
            data-key="rowKey"
            striped-rows
            table-style="min-width: 44rem"
            data-testid="absences-table"
          >
          <Column field="from" header="Datum" sortable>
            <template #body="{ data }: { data: ViewRow }">
              <span v-if="data.to">{{ formatGermanDate(data.from) }} – {{ formatGermanDate(data.to) }}</span>
              <span v-else>{{ formatGermanDate(data.from) }}</span>
            </template>
          </Column>
          <Column field="employeeName" header="Mitarbeiter" sortable />
          <Column header="Art">
            <template #body="{ data }: { data: ViewRow }">{{ typeLabel(data.absence_type) }}</template>
          </Column>
          <Column
            field="vacation_days"
            header="Urlaubstage"
            sortable
            style="min-width: 6.5rem; width: 6.5rem"
          >
            <template #body="{ data }: { data: ViewRow }">
              <span v-if="data.absence_type === 'vacation' && data.vacation_days != null">
                {{ String(data.vacation_days).replace('.', ',') }}
              </span>
              <span v-else class="muted-cell">—</span>
            </template>
          </Column>
          <Column header="Halbtag">
            <template #body="{ data }: { data: ViewRow }">{{ data.half_day ? 'Ja' : 'Nein' }}</template>
          </Column>
          <Column header="" style="width: 6.5rem">
            <template #body="{ data }: { data: ViewRow }">
              <template v-if="data.absence_type === 'vacation' && !data.half_day">
                <Button icon="pi pi-pencil" text rounded aria-label="Bearbeiten" @click="openVacationEdit(data)" />
                <Button icon="pi pi-trash" severity="danger" text rounded aria-label="Löschen" @click="removeRow(data)" />
              </template>
              <template v-else>
                <Button icon="pi pi-trash" severity="danger" text rounded aria-label="Löschen" @click="removeRow(data)" />
              </template>
            </template>
          </Column>
          </DataTable>
        </div>
      </template>
    </Card>
      </div>

      <aside class="page-aside" aria-label="Ausgleichstag-Ansprüche">
    <Card class="claims-card">
      <template #title>Offene Ausgleichstag-Ansprüche</template>
      <template #content>
        <p class="claims-intro">
          Hier können offene Ansprüche aus Wochenendarbeit verworfen werden. Der Verzicht entfernt nur den freien Tag;
          die geleisteten Stunden bleiben im Überstundenkonto.
        </p>
        <label class="lbl block-label">Mitarbeiter</label>
        <p v-if="waiveOverviewLoading" class="hint waive-status">Mitarbeitende mit offenen Ansprüchen werden ermittelt …</p>
        <p
          v-else-if="waiveEmployeeOptions.length === 0"
          class="hint waive-status"
        >
          Aktuell gibt es keine Mitarbeitenden mit einem verwertbaren offenen Ausgleichstag-Anspruch.
        </p>
        <Select
          v-else
          v-model="waiveEmployeeId"
          :options="waiveEmployeeOptions"
          option-label="label"
          option-value="value"
          placeholder="Mitarbeiter wählen"
          class="filt waive-select"
          :disabled="waiveEmployeeOptions.length === 0"
          filter
          @show="focusSelectFilterOnShow"
        />
        <DataTable
          :value="waiveClaims"
          :loading="waiveLoading"
          data-key="id"
          striped-rows
          class="waive-table"
        >
          <Column field="work_date" header="Gearbeiteter Tag" sortable>
            <template #body="{ data }: { data: CompensationDayClaim }">
              {{ formatGermanDate(data.work_date) }}
            </template>
          </Column>
          <Column header="" style="width: 9rem">
            <template #body="{ data }: { data: CompensationDayClaim }">
              <Button
                label="Verzichten"
                severity="secondary"
                size="small"
                :loading="waivingClaimId === data.id"
                :disabled="waiveEmployeeId == null"
                @click="waiveClaim(data)"
              />
            </template>
          </Column>
        </DataTable>
        <p
          v-if="waiveEmployeeId != null && !waiveLoading && waiveClaims.length === 0"
          class="hint empty-hint"
        >
          Keine offenen Ansprüche für diesen Mitarbeiter.
        </p>
      </template>
    </Card>
      </aside>
    </div>

    <Dialog v-model:visible="showVacationEdit" header="Urlaubszeitraum" modal :style="{ width: '420px' }">
      <div class="form">
        <label>Von</label>
        <DatePicker v-model="editFrom" date-format="dd.mm.yy" show-icon class="w" />
        <label>Bis</label>
        <DatePicker v-model="editTo" date-format="dd.mm.yy" show-icon class="w" :min-date="editFrom" />
        <p class="hint">Wochenenden, Feiertage und Schließtage im Zeitraum werden beim Urlaub automatisch übersprungen.</p>
      </div>
      <template #footer>
        <Button label="Abbrechen" text @click="showVacationEdit = false" />
        <Button label="Speichern" :loading="savingVacationEdit" @click="submitVacationEdit" />
      </template>
    </Dialog>

    <Dialog v-model:visible="showAdd" header="Abwesenheit" modal :style="{ width: '440px' }">
      <div class="form">
        <label>Mitarbeiter</label>
        <Select
          ref="addEmpSelect"
          v-model="addEmpId"
          :options="employees.map((e) => ({ label: e.display_name, value: e.id }))"
          option-label="label"
          option-value="value"
          class="w"
          filter
          @show="focusSelectFilterOnShow"
        />
        <template v-if="addType === 'compensation_day'">
          <label>Datum</label>
          <DatePicker v-model="addDateFrom" date-format="dd.mm.yy" show-icon class="w" />
        </template>
        <template v-else>
          <label>Von</label>
          <DatePicker v-model="addDateFrom" date-format="dd.mm.yy" show-icon class="w" />
          <label>Bis</label>
          <DatePicker
            v-model="addDateTo"
            date-format="dd.mm.yy"
            show-icon
            class="w"
            :min-date="addDateFrom"
          />
          <p v-if="addType === 'vacation'" class="hint">
            Wochenenden und gesetzliche Feiertage im Zeitraum werden beim Urlaub automatisch übersprungen.
          </p>
        </template>
        <label>Art</label>
        <Select
          v-model="addType"
          :options="typeOptions"
          option-label="label"
          option-value="value"
          class="w"
        />
        <p v-if="addType === 'compensation_day'" class="hint">
          Offene Ausgleichstag-Ansprüche: {{ openCompensationDayClaims.length }}
        </p>
        <label class="row">
          <span>Halber Tag</span>
          <InputSwitch v-model="addHalf" :disabled="addType === 'compensation_day'" />
        </label>
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showAdd = false" />
        <Button
          label="Speichern"
          :loading="saving"
          :disabled="addEmpId == null"
          data-testid="absences-save"
          @click="submitAdd"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: min(1280px, 100%);
  width: 100%;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}
.page-grid {
  display: flex;
  flex-direction: row;
  align-items: flex-start;
  gap: 1.25rem;
}
.page-main {
  flex: 1;
  min-width: 0;
}
.page-aside {
  flex: 0 0 21rem;
  max-width: 100%;
  position: sticky;
  top: calc(var(--layout-top-inset, 0px) + 0.5rem);
  align-self: flex-start;
}
@media (max-width: 960px) {
  .page-grid {
    flex-direction: column;
  }
  .page-aside {
    flex: 1 1 auto;
    width: 100%;
    position: static;
  }
}
.claims-card .claims-intro {
  margin: 0 0 0.75rem;
  font-size: 0.9rem;
  color: #475569;
  line-height: 1.45;
}
.block-label {
  display: block;
  margin-bottom: 0.35rem;
}
.waive-select {
  width: 100%;
  max-width: 100%;
  margin-bottom: 0.75rem;
}
.waive-table {
  margin-top: 0.25rem;
}
.page-aside :deep(.p-card) {
  width: 100%;
}
.empty-hint {
  margin-top: 0.5rem;
}
.waive-status {
  margin: 0 0 0.5rem;
}
.absences-table-wrap {
  width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.muted-cell {
  color: #94a3b8;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: flex-end;
  margin-bottom: 1rem;
}
.range {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}
.lbl {
  font-size: 0.85rem;
  color: #64748b;
}
.filt {
  min-width: 200px;
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
.row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 0.5rem;
}
.w {
  width: 100%;
}
.hint {
  margin: 0;
  font-size: 0.8rem;
  color: #64748b;
}
</style>
