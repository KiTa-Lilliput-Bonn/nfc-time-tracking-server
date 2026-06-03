<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputSwitch from 'primevue/inputswitch'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import {
  createEmployeeAbsence,
  fetchClosureDays,
  fetchEmployeeCompensationDayClaims,
  fetchFixedNonWorkWeekdays,
  fetchHolidays,
} from '@/api/management'
import type { CompensationDayClaim, FixedNonWorkWeekdays } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { friendlyAbsenceCreateError } from '@/utils/absenceErrors'
import { formatGermanDate, parseISODate } from '@/utils/dates'
import { isFixedNonWorkDayISO } from '@/utils/workdays'

const visible = defineModel<boolean>('visible', { required: true })

const props = defineProps<{
  employeeId: number
  displayName: string
  absenceDate: string
}>()

const emit = defineEmits<{
  saved: []
}>()

const toast = useToast()
const saving = ref(false)
const addType = ref<'sick' | 'vacation' | 'other' | 'compensation_day'>('sick')
const addHalf = ref(false)
const fnwRows = ref<FixedNonWorkWeekdays[]>([])
const openCompensationDayClaims = ref<CompensationDayClaim[]>([])

const typeOptions = [
  { label: 'Krank', value: 'sick' as const },
  { label: 'Urlaub', value: 'vacation' as const },
  { label: 'Ausgleichstag', value: 'compensation_day' as const },
  { label: 'Sonstiges', value: 'other' as const },
]

const dateLabel = computed(() => formatGermanDate(props.absenceDate))

function germanWeekdayLong(d: Date): string {
  return new Intl.DateTimeFormat('de-DE', { weekday: 'long' }).format(d)
}

async function loadContext() {
  try {
    fnwRows.value = await fetchFixedNonWorkWeekdays(props.employeeId)
  } catch {
    fnwRows.value = []
  }
  if (addType.value === 'compensation_day') {
    try {
      openCompensationDayClaims.value = await fetchEmployeeCompensationDayClaims(props.employeeId, 'open')
    } catch {
      openCompensationDayClaims.value = []
    }
  } else {
    openCompensationDayClaims.value = []
  }
}

watch(
  () => visible.value,
  (open) => {
    if (open) {
      addType.value = 'sick'
      addHalf.value = false
      void loadContext()
    }
  },
)

watch(addType, (t) => {
  if (t === 'compensation_day') {
    addHalf.value = false
    void loadContext()
  } else {
    openCompensationDayClaims.value = []
  }
})

async function submitAdd() {
  const iso = props.absenceDate
  const df = parseISODate(iso)

  if (addType.value === 'compensation_day') {
    const pretty = formatGermanDate(iso)
    const weekday = germanWeekdayLong(df)
    if (isFixedNonWorkDayISO(iso, fnwRows.value)) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: `Am ${weekday}, ${pretty}, kann kein Ausgleichstag gebucht werden — fester freier Wochentag.`,
        life: 10000,
      })
      return
    }
    const dow = df.getDay()
    if (dow === 0 || dow === 6) {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag nicht möglich',
        detail: `Am ${weekday}, ${pretty}, ist Wochenende.`,
        life: 10000,
      })
      return
    }
    try {
      const hol = await fetchHolidays(df.getFullYear())
      const hit = hol.find((h) => h.holiday_date === iso)
      if (hit) {
        toast.add({
          severity: 'error',
          summary: 'Ausgleichstag nicht möglich',
          detail: `Am ${weekday}, ${pretty}, ist Feiertag („${hit.name}“).`,
          life: 10000,
        })
        return
      }
      if (openCompensationDayClaims.value.length === 0) {
        const claims = await fetchEmployeeCompensationDayClaims(props.employeeId, 'open')
        if (claims.length === 0) {
          toast.add({
            severity: 'error',
            summary: 'Ausgleichstag nicht möglich',
            detail: 'Kein offener Ausgleichstag-Anspruch vorhanden.',
            life: 10000,
          })
          return
        }
      }
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Ausgleichstag',
        detail: 'Ansprüche konnten nicht geprüft werden.',
        life: 10000,
      })
      return
    }
  }

  if (addType.value === 'vacation') {
    try {
      const closures = await fetchClosureDays()
      const closureSet = new Set(closures.map((c) => c.closure_date.slice(0, 10)))
      if (closureSet.has(iso)) {
        toast.add({
          severity: 'error',
          summary: 'Urlaub nicht möglich',
          detail: 'An diesem Tag ist Schließtag.',
          life: 10000,
        })
        return
      }
      const hol = await fetchHolidays(df.getFullYear())
      if (hol.some((h) => h.holiday_date === iso)) {
        toast.add({
          severity: 'error',
          summary: 'Urlaub nicht möglich',
          detail: 'An diesem Tag ist Feiertag.',
          life: 10000,
        })
        return
      }
      if (isFixedNonWorkDayISO(iso, fnwRows.value)) {
        toast.add({
          severity: 'error',
          summary: 'Urlaub nicht möglich',
          detail: 'Fester freier Wochentag.',
          life: 10000,
        })
        return
      }
      const dow = df.getDay()
      if (dow === 0 || dow === 6) {
        toast.add({
          severity: 'error',
          summary: 'Urlaub nicht möglich',
          detail: 'Wochenende.',
          life: 10000,
        })
        return
      }
    } catch {
      toast.add({ severity: 'error', summary: 'Prüfung fehlgeschlagen', life: 10000 })
      return
    }
  }

  const absenceLabel =
    addType.value === 'sick'
      ? 'Krankmeldung'
      : addType.value === 'vacation'
        ? 'Urlaub'
        : addType.value === 'compensation_day'
          ? 'Ausgleichstag'
          : 'Abwesenheit'

  saving.value = true
  try {
    await createEmployeeAbsence(props.employeeId, {
      absence_date: iso,
      absence_type: addType.value,
      half_day: addHalf.value,
    })
    toast.add({ severity: 'success', summary: 'Eingetragen', life: 10000 })
    visible.value = false
    emit('saved')
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      detail:
        friendlyAbsenceCreateError({
          apiMessage: getApiErrorMessage(e),
          isoDate: iso,
          absenceLabel,
        }) ?? 'Die Abwesenheit konnte nicht gespeichert werden.',
      life: 10000,
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" header="Abwesenheit" modal :style="{ width: '440px' }">
    <div class="form">
      <label>Mitarbeiter*in</label>
      <p class="readonly">{{ displayName }}</p>
      <label>Datum</label>
      <p class="readonly">{{ dateLabel }}</p>
      <label>Art</label>
      <Select v-model="addType" :options="typeOptions" option-label="label" option-value="value" class="w" />
      <p v-if="addType === 'compensation_day'" class="hint">
        Offene Ausgleichstag-Ansprüche: {{ openCompensationDayClaims.length }}
      </p>
      <label class="row">
        <span>Halber Tag</span>
        <InputSwitch v-model="addHalf" :disabled="addType === 'compensation_day'" />
      </label>
    </div>
    <template #footer>
      <Button label="Abbrechen" severity="secondary" text @click="visible = false" />
      <Button label="Speichern" :loading="saving" data-testid="gap-absence-save" @click="submitAdd" />
    </template>
  </Dialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.readonly {
  margin: 0;
  font-weight: 600;
}
.hint {
  margin: 0;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}
.w {
  width: 100%;
}
</style>
