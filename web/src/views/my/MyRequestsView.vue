<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'

import { fetchMeCorrections, fetchMeTimes } from '@/api/me'
import { createMyRequest, fetchMyRequests, withdrawMyRequest } from '@/api/requests'
import type { ChangeRequest, ChangeRequestKind, TimeCorrection, WorkPeriod } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import {
  requestKindLabel,
  requestStatusLabel,
  requestStatusSeverity,
  requestSummary,
} from '@/utils/changeRequests'
import {
  buildCorrectionTimeInstants,
  formatGermanDateTime,
  formatGermanTime,
  isoToTimeInputValue,
  toISODateLocal,
} from '@/utils/dates'
import { correctionByWorkPeriod, isPeriodDisabled } from '@/utils/timeTableModel'

const route = useRoute()
const toast = useToast()

const kindOptions: { label: string; value: ChangeRequestKind }[] = [
  { label: 'Zeit korrigieren', value: 'time_correction' },
  { label: 'Zeit nachtragen', value: 'time_entry' },
  { label: 'Urlaub', value: 'vacation' },
]

function kindFromQuery(): ChangeRequestKind {
  const q = route.query.type
  if (q === 'vacation' || q === 'time_entry' || q === 'time_correction') return q
  return 'time_correction'
}

const today = toISODateLocal(new Date())
const kind = ref<ChangeRequestKind>(kindFromQuery())
const workDate = ref(typeof route.query.date === 'string' ? route.query.date : today)
const dateFrom = ref(today)
const dateTo = ref(today)
const halfDay = ref(false)
const timeIn = ref('08:00')
const timeOut = ref('16:00')
const reason = ref('')
const submitting = ref(false)

const dayPeriods = ref<WorkPeriod[]>([])
const dayCorrections = ref<TimeCorrection[]>([])
const selectedWpId = ref<number | null>(null)
const dayLoading = ref(false)

const requests = ref<ChangeRequest[]>([])
const listLoading = ref(true)
const listErr = ref('')

const corrByWp = computed(() => correctionByWorkPeriod(dayCorrections.value))

const pendingWpIds = computed(
  () => new Set(requests.value.filter((r) => r.status === 'pending' && r.work_period_id).map((r) => r.work_period_id!)),
)

interface PeriodOption {
  id: number
  label: string
  disabled: boolean
  note?: string
}

const periodOptions = computed<PeriodOption[]>(() =>
  dayPeriods.value
    .filter((p) => !p.is_break)
    .map((p) => {
      const c = corrByWp.value.get(p.id)
      const a = c && !isPeriodDisabled(c) ? c.corrected_in : p.punch_in
      const b = c && !isPeriodDisabled(c) ? c.corrected_out : p.punch_out
      const label = `${formatGermanTime(a)} – ${b ? formatGermanTime(b) : 'offen'}`
      if (isPeriodDisabled(c)) return { id: p.id, label, disabled: true, note: 'von der Leitung deaktiviert' }
      if (pendingWpIds.value.has(p.id)) return { id: p.id, label, disabled: true, note: 'Antrag offen' }
      return { id: p.id, label, disabled: false }
    }),
)

function selectPeriod(id: number) {
  const p = dayPeriods.value.find((x) => x.id === id)
  if (!p) return
  selectedWpId.value = id
  const c = corrByWp.value.get(id)
  const eff = c && !isPeriodDisabled(c) ? c : undefined
  timeIn.value = isoToTimeInputValue(eff ? eff.corrected_in : p.punch_in)
  const out = eff ? eff.corrected_out : p.punch_out
  timeOut.value = out ? isoToTimeInputValue(out) : '16:00'
}

async function loadDay() {
  if (kind.value !== 'time_correction' || !workDate.value) return
  dayLoading.value = true
  selectedWpId.value = null
  try {
    const [t, c] = await Promise.all([
      fetchMeTimes(workDate.value, workDate.value),
      fetchMeCorrections(workDate.value, workDate.value),
    ])
    dayPeriods.value = t.work_periods
    dayCorrections.value = c.corrections
    const first = periodOptions.value.find((o) => !o.disabled)
    if (first) selectPeriod(first.id)
  } catch {
    dayPeriods.value = []
    dayCorrections.value = []
  } finally {
    dayLoading.value = false
  }
}

async function loadList() {
  listLoading.value = true
  listErr.value = ''
  try {
    requests.value = await fetchMyRequests()
  } catch {
    listErr.value = 'Anträge konnten nicht geladen werden.'
  } finally {
    listLoading.value = false
  }
}

watch([kind, workDate], () => void loadDay())
watch(dateFrom, (v) => {
  if (!dateTo.value || dateTo.value < v) dateTo.value = v
})
const singleDay = computed(() => dateFrom.value === dateTo.value)
watch(singleDay, (v) => {
  if (!v) halfDay.value = false
})

const reasonRequired = computed(() => kind.value !== 'vacation')

function warn(summary: string) {
  toast.add({ severity: 'warn', summary, life: 8000 })
}

async function submit() {
  if (reasonRequired.value && !reason.value.trim()) {
    warn('Bitte einen Grund angeben.')
    return
  }
  let body: Parameters<typeof createMyRequest>[0]
  if (kind.value === 'vacation') {
    if (!dateFrom.value || !dateTo.value) {
      warn('Bitte Zeitraum angeben.')
      return
    }
    body = { kind: 'vacation', date_from: dateFrom.value, date_to: dateTo.value, half_day: halfDay.value, reason: reason.value.trim() }
  } else {
    const inst = buildCorrectionTimeInstants(workDate.value, timeIn.value, timeOut.value)
    if (!inst) {
      warn('Gehen muss am selben Tag nach Kommen liegen.')
      return
    }
    if (kind.value === 'time_correction') {
      if (selectedWpId.value == null) {
        warn('Bitte den Eintrag auswählen, der korrigiert werden soll.')
        return
      }
      body = { kind: 'time_correction', work_period_id: selectedWpId.value, punch_in: inst.corrected_in, punch_out: inst.corrected_out, reason: reason.value.trim() }
    } else {
      body = { kind: 'time_entry', work_date: workDate.value, punch_in: inst.corrected_in, punch_out: inst.corrected_out, reason: reason.value.trim() }
    }
  }
  submitting.value = true
  try {
    await createMyRequest(body)
    toast.add({
      severity: 'success',
      summary: 'Antrag gestellt',
      detail: 'Die Änderung gilt erst, wenn die Leitung zustimmt.',
      life: 8000,
    })
    reason.value = ''
    await loadList()
    await loadDay()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Antrag nicht möglich', detail: getApiErrorMessage(e), life: 10000 })
  } finally {
    submitting.value = false
  }
}

async function withdraw(r: ChangeRequest) {
  if (!confirm('Antrag zurückziehen?')) return
  try {
    await withdrawMyRequest(r.id)
    await loadList()
    await loadDay()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Zurückziehen fehlgeschlagen', detail: getApiErrorMessage(e), life: 10000 })
  }
}

onMounted(() => {
  void loadList()
  void loadDay()
})
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Neuer Antrag</template>
      <template #content>
        <p class="hint">Änderungen werden erst wirksam, wenn die Leitung den Antrag genehmigt.</p>
        <SelectButton
          v-model="kind"
          :options="kindOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          class="kind-select"
          data-testid="request-kind"
        />

        <div class="form">
          <template v-if="kind !== 'vacation'">
            <label for="rq-date">Datum</label>
            <input id="rq-date" v-model="workDate" type="date" :max="today" class="p-inputtext p-component w" />
          </template>

          <template v-if="kind === 'time_correction'">
            <span class="label">Eintrag</span>
            <p v-if="dayLoading" class="muted">Laden…</p>
            <p v-else-if="!periodOptions.length" class="muted">
              An diesem Tag gibt es keine Stempelzeit. Fehlt sie ganz, bitte „Zeit nachtragen“ wählen.
            </p>
            <div v-else class="periods">
              <button
                v-for="o in periodOptions"
                :key="o.id"
                type="button"
                class="period"
                :class="{ 'period--active': selectedWpId === o.id }"
                :disabled="o.disabled"
                @click="selectPeriod(o.id)"
              >
                {{ o.label }}<span v-if="o.note" class="period-note"> · {{ o.note }}</span>
              </button>
            </div>
          </template>

          <template v-if="kind !== 'vacation'">
            <div class="two">
              <div>
                <label for="rq-in">Kommen</label>
                <input id="rq-in" v-model="timeIn" type="time" step="60" class="p-inputtext p-component w" />
              </div>
              <div>
                <label for="rq-out">Gehen</label>
                <input id="rq-out" v-model="timeOut" type="time" step="60" class="p-inputtext p-component w" />
              </div>
            </div>
          </template>

          <template v-else>
            <div class="two">
              <div>
                <label for="rq-from">Von</label>
                <input id="rq-from" v-model="dateFrom" type="date" class="p-inputtext p-component w" />
              </div>
              <div>
                <label for="rq-to">Bis</label>
                <input id="rq-to" v-model="dateTo" type="date" :min="dateFrom" class="p-inputtext p-component w" />
              </div>
            </div>
            <div v-if="singleDay" class="check">
              <Checkbox v-model="halfDay" input-id="rq-half" binary />
              <label for="rq-half">Halber Tag</label>
            </div>
            <p class="muted small">Wochenenden, Feiertage, Schließtage und Ihre freien Tage werden nicht mitgezählt.</p>
          </template>

          <label for="rq-reason">{{ reasonRequired ? 'Grund (Pflicht)' : 'Bemerkung (optional)' }}</label>
          <Textarea id="rq-reason" v-model="reason" rows="2" auto-resize class="w" />

          <Button
            :label="kind === 'vacation' ? 'Urlaub beantragen' : 'Antrag stellen'"
            :loading="submitting"
            class="submit"
            data-testid="request-submit"
            @click="submit"
          />
        </div>
      </template>
    </Card>

    <Card>
      <template #title>Meine Anträge</template>
      <template #content>
        <p v-if="listErr" class="err">{{ listErr }}</p>
        <p v-else-if="listLoading" class="muted">Laden…</p>
        <p v-else-if="!requests.length" class="muted">Noch keine Anträge.</p>
        <ul v-else class="list">
          <li v-for="r in requests" :key="r.id" class="item" data-testid="my-request">
            <div class="item-head">
              <strong>{{ requestKindLabel[r.kind] }}</strong>
              <Tag :value="requestStatusLabel[r.status]" :severity="requestStatusSeverity[r.status]" />
            </div>
            <div>{{ requestSummary(r) }}</div>
            <div v-if="r.reason" class="muted small">Grund: {{ r.reason }}</div>
            <div class="muted small">Gestellt am {{ formatGermanDateTime(r.created_at) }}</div>
            <div v-if="r.status === 'approved' || r.status === 'rejected'" class="decision" :class="`decision--${r.status}`">
              {{ r.status === 'approved' ? 'Genehmigt' : 'Abgelehnt' }}
              <template v-if="r.decided_by_name"> von {{ r.decided_by_name }}</template>
              <template v-if="r.decided_at"> am {{ formatGermanDateTime(r.decided_at) }}</template>
              <div v-if="r.decision_comment" class="comment">„{{ r.decision_comment }}“</div>
            </div>
            <Button
              v-if="r.status === 'pending'"
              label="Zurückziehen"
              size="small"
              severity="secondary"
              text
              @click="withdraw(r)"
            />
          </li>
        </ul>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 640px;
}
.hint {
  margin: 0 0 0.75rem;
  color: #475569;
}
.kind-select {
  flex-wrap: wrap;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 1rem;
}
.form label,
.label {
  font-size: 0.85rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
}
.two > div {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.check {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.25rem;
}
.periods {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.period {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 6px;
  padding: 0.5rem 0.75rem;
  font: inherit;
  cursor: pointer;
}
.period--active {
  border-color: #4f46e5;
  background: #eef2ff;
  color: #3730a3;
}
.period:disabled {
  cursor: not-allowed;
  color: #94a3b8;
}
.period-note {
  font-size: 0.8rem;
}
.submit {
  margin-top: 0.75rem;
  align-self: flex-start;
}
.list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.item {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  align-items: flex-start;
}
.item-head {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}
.decision {
  margin-top: 0.25rem;
  font-size: 0.9rem;
}
.decision--approved {
  color: #15803d;
}
.decision--rejected {
  color: #b91c1c;
}
.comment {
  color: #334155;
  font-style: italic;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
}
.err {
  color: #b91c1c;
}
</style>
