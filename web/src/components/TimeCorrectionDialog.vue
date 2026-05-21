<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'
import { createMeCorrection } from '@/api/me'
import { createCorrection } from '@/api/management'
import type { TimeCorrection, WorkPeriod } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { correctionByWorkPeriod, pickPrimaryWorkPeriod } from '@/utils/timeTableModel'
import {
  buildCorrectionTimeInstants,
  formatGermanDate,
  formatGermanTime,
  isoToTimeInputValue,
  parseTimeHHMM,
} from '@/utils/dates'

const props = defineProps<{
  visible: boolean
  dialogDate: string
  /** Korrigierbare Arbeitsperioden des Tages (ohne Pausen). */
  candidates: WorkPeriod[]
  /** Beim Öffnen z. B. aus Kalender: diesen Eintrag vorauswählen (muss in `candidates` sein). */
  initialWorkPeriodId?: number | null
  periods: WorkPeriod[]
  corrections?: TimeCorrection[]
  rowCorrection: { mode: 'self' } | { mode: 'employee'; employeeId: number }
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  saved: []
}>()

const toast = useToast()

const selWpId = ref<number | null>(null)
const corrIn = ref('')
const corrOut = ref('')
const corrReason = ref('')
const saving = ref(false)

const corrByWorkPeriod = computed(() => correctionByWorkPeriod(props.corrections))

const wpOptions = computed(() => {
  if (selWpId.value == null) return [] as { label: string; value: number }[]
  return props.candidates.map((p) => ({
    value: p.id,
    label: workPeriodLabel(p, corrByWorkPeriod.value.get(p.id)),
  }))
})

function workPeriodLabel(p: WorkPeriod, c?: TimeCorrection) {
  const a = c ? c.corrected_in : p.punch_in
  const b = c ? c.corrected_out : p.punch_out
  return `${formatGermanTime(a)} → ${b ? formatGermanTime(b) : '—'}`
}

function fillFromSelection() {
  if (selWpId.value == null) {
    corrIn.value = ''
    corrOut.value = ''
    return
  }
  const p = props.periods.find((x) => x.id === selWpId.value)
  if (!p) {
    corrIn.value = ''
    corrOut.value = ''
    return
  }
  const c = corrByWorkPeriod.value.get(p.id)
  corrIn.value = isoToTimeInputValue(c ? c.corrected_in : p.punch_in)
  const outSrc = c ? c.corrected_out : p.punch_out
  corrOut.value = outSrc ? isoToTimeInputValue(outSrc) : isoToTimeInputValue(p.punch_in)
}

function syncOpen() {
  if (!props.visible || !props.candidates.length) return
  const init = props.initialWorkPeriodId
  const byInit = init != null ? props.candidates.find((c) => c.id === init) : undefined
  const primary = byInit ?? pickPrimaryWorkPeriod(props.candidates)
  selWpId.value = primary?.id ?? props.candidates[0]!.id
  corrReason.value = ''
  fillFromSelection()
}

watch(
  () => [props.visible, props.initialWorkPeriodId] as const,
  () => {
    if (props.visible) syncOpen()
  },
)

watch(selWpId, () => {
  if (!props.visible) return
  fillFromSelection()
})

function close() {
  emit('update:visible', false)
}

async function submitCorrect() {
  if (selWpId.value == null) return
  if (!corrReason.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Grund erforderlich', life: 10000 })
    return
  }
  const p = props.periods.find((x) => x.id === selWpId.value)
  if (!p) return
  const a = parseTimeHHMM(corrIn.value)
  const b = parseTimeHHMM(corrOut.value)
  if (!a || !b) {
    toast.add({ severity: 'warn', summary: 'Bitte gültige Uhrzeiten (HH:MM) eingeben', life: 10000 })
    return
  }
  if (a.h === b.h && a.m === b.m) {
    toast.add({ severity: 'warn', summary: 'Kommen und Gehen dürfen nicht dieselbe Uhrzeit haben.', life: 10000 })
    return
  }
  const inst = buildCorrectionTimeInstants(p.work_date, corrIn.value, corrOut.value)
  if (!inst) {
    toast.add({
      severity: 'warn',
      summary: 'Ungültige Uhrzeiten',
      detail: 'Am selben Kalendertag muss Gehen zeitlich nach Kommen liegen (z. B. Kommen 08:00, Gehen 16:00).',
      life: 10000,
    })
    return
  }
  saving.value = true
  try {
    const body = {
      work_period_id: selWpId.value,
      corrected_in: inst.corrected_in,
      corrected_out: inst.corrected_out,
      reason: corrReason.value.trim(),
    }
    if (props.rowCorrection.mode === 'self') {
      await createMeCorrection(body)
    } else {
      await createCorrection(props.rowCorrection.employeeId, body)
    }
    toast.add({ severity: 'success', summary: 'Korrektur gespeichert', life: 10000 })
    close()
    emit('saved')
  } catch (e) {
    const detail = getApiErrorMessage(e)
    toast.add({
      severity: 'error',
      summary: 'Korrektur fehlgeschlagen',
      ...(detail ? { detail, life: 10000 } : { life: 10000 }),
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog
    :visible="visible"
    header="Zeit korrigieren"
    modal
    :style="{ width: '460px' }"
    @update:visible="emit('update:visible', $event)"
  >
    <p v-if="dialogDate" class="dlg-sub">Datum: {{ formatGermanDate(dialogDate) }}</p>
    <div v-if="wpOptions.length > 1" class="form">
      <label>Eintrag</label>
      <Select
        v-model="selWpId"
        :options="wpOptions"
        option-label="label"
        option-value="value"
        class="w"
      />
    </div>
    <div class="form">
      <label>Kommen (Uhrzeit)</label>
      <input v-model="corrIn" type="time" step="60" class="p-inputtext p-component w" />
      <label>Gehen (Uhrzeit)</label>
      <input v-model="corrOut" type="time" step="60" class="p-inputtext p-component w" />
      <label>Grund (Pflicht)</label>
      <InputText v-model="corrReason" class="w" />
    </div>
    <template #footer>
      <Button label="Abbrechen" severity="secondary" text @click="close" />
      <Button label="Speichern" :loading="saving" :disabled="selWpId == null" @click="submitCorrect" />
    </template>
  </Dialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.75rem;
}
.form label {
  font-size: 0.85rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.dlg-sub {
  margin: 0 0 0.25rem;
  color: #334155;
  font-size: 0.9rem;
}
</style>
