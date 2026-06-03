<script setup lang="ts">
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'

import { createManualWorkPeriod } from '@/api/management'
import { getApiErrorMessage } from '@/utils/apiError'
import { parseISODate, parseTimeHHMM, plusOneHourHHMM, toISODateLocal } from '@/utils/dates'

const visible = defineModel<boolean>('visible', { required: true })

const props = defineProps<{
  employeeId: number
  employeeName?: string
  workDate: string
  shiftStart?: string
  shiftEnd?: string
}>()

const emit = defineEmits<{
  saved: []
}>()

const toast = useToast()
const saving = ref(false)
const manDate = ref<Date>(new Date())
const manIn = ref('08:00')
const manOut = ref('16:00')
const manOutManuallyEdited = ref(false)

function endOfLocalToday(): Date {
  const d = new Date()
  return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999)
}

const manualDateMax = ref(endOfLocalToday())

function applyPrefill() {
  manDate.value = parseISODate(props.workDate)
  manualDateMax.value = endOfLocalToday()
  const start = props.shiftStart?.trim() ?? ''
  const end = props.shiftEnd?.trim() ?? ''
  if (start) {
    manIn.value = start
    manOutManuallyEdited.value = Boolean(end)
    manOut.value = end || plusOneHourHHMM(start) || '09:00'
  } else {
    manIn.value = '08:00'
    manOutManuallyEdited.value = false
    manOut.value = plusOneHourHHMM(manIn.value) ?? '09:00'
  }
}

watch(
  () => visible.value,
  (open) => {
    if (open) applyPrefill()
  },
)

watch(manIn, (next) => {
  if (!visible.value || manOutManuallyEdited.value) return
  const auto = plusOneHourHHMM(next)
  if (auto) manOut.value = auto
})

function onManOutInput() {
  manOutManuallyEdited.value = true
}

function timeOnDateToISO(d: Date, hhmm: string): string {
  const p = parseTimeHHMM(hhmm)
  if (!p) return new Date(NaN).toISOString()
  const local = new Date(d.getFullYear(), d.getMonth(), d.getDate(), p.h, p.m, 0, 0)
  return local.toISOString()
}

async function submitManual() {
  saving.value = true
  try {
    const workYmd = toISODateLocal(manDate.value)
    const todayYmd = toISODateLocal(new Date())
    if (workYmd > todayYmd) {
      toast.add({
        severity: 'warn',
        summary: 'Zukünftige Tage sind nicht erlaubt',
        detail: 'Manuelle Zeiten nur für heute oder die Vergangenheit.',
        life: 10000,
      })
      return
    }
    const punchInISO = timeOnDateToISO(manDate.value, manIn.value)
    const punchOutISO = timeOnDateToISO(manDate.value, manOut.value)
    if (Number.isNaN(new Date(punchInISO).getTime()) || Number.isNaN(new Date(punchOutISO).getTime())) {
      toast.add({ severity: 'warn', summary: 'Bitte gültige Uhrzeiten eingeben', life: 10000 })
      return
    }
    if (new Date(punchOutISO).getTime() <= new Date(punchInISO).getTime()) {
      toast.add({ severity: 'warn', summary: 'Gehen muss nach Kommen liegen', life: 10000 })
      return
    }
    await createManualWorkPeriod(props.employeeId, {
      work_date: workYmd,
      punch_in: punchInISO,
      punch_out: punchOutISO,
    })
    toast.add({ severity: 'success', summary: 'Manuelle Zeit gespeichert', life: 10000 })
    visible.value = false
    emit('saved')
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" header="Manuelle Arbeitszeit" modal :style="{ width: '440px' }">
    <p v-if="employeeName" class="dlg-sub">{{ employeeName }}</p>
    <div class="form">
      <label>Datum</label>
      <DatePicker v-model="manDate" date-format="dd.mm.yy" show-icon class="w" :max-date="manualDateMax" />
      <label>Kommen</label>
      <input v-model="manIn" type="time" step="60" class="p-inputtext p-component w" />
      <label>Gehen</label>
      <input v-model="manOut" type="time" step="60" class="p-inputtext p-component w" @input="onManOutInput" />
    </div>
    <template #footer>
      <Button label="Abbrechen" severity="secondary" text @click="visible = false" />
      <Button
        label="Speichern"
        :loading="saving"
        data-testid="manual-work-period-save"
        @click="submitManual"
      />
    </template>
  </Dialog>
</template>

<style scoped>
.dlg-sub {
  margin: 0 0 0.75rem;
  font-size: 0.9rem;
  color: var(--p-text-muted-color);
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.w {
  width: 100%;
}
</style>
