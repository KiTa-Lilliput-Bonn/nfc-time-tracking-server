<script setup lang="ts">
import { computed } from 'vue'

import type { SaldoDay, TimeCorrection, WorkPeriod } from '@/types/api'
import { formatGermanTime } from '@/utils/dates'
import { balanceClass, formatHoursHM, formatMinutesHM } from '@/utils/hours'
import { correctionByWorkPeriod } from '@/utils/timeTableModel'

/**
 * Tagesliste einer Woche (vor allem für schmale Bildschirme): pro Tag Plan, Stempel und die
 * Rechnung des Servers (gestempelt − Pausenabzug = netto, Gutschrift, Soll, Saldo).
 */
const props = defineProps<{
  days: SaldoDay[]
  periods: WorkPeriod[]
  corrections?: TimeCorrection[]
  scheduleByDate: Record<string, { shift_start: string; shift_end: string }>
  /** Letzter Tag, der ins Stundenkonto zählt (gestern). */
  countedThrough: string
  today: string
  loading?: boolean
}>()

const weekdayDE = ['So', 'Mo', 'Di', 'Mi', 'Do', 'Fr', 'Sa']

const corrMap = computed(() => correctionByWorkPeriod(props.corrections))

interface StampRow {
  key: number
  from: string
  to: string
  open: boolean
  corrected: boolean
  disabled: boolean
  reason: string
}

const stampsByDate = computed(() => {
  const m = new Map<string, StampRow[]>()
  const list = [...props.periods].filter((p) => !p.is_break).sort((a, b) => a.punch_in.localeCompare(b.punch_in))
  for (const p of list) {
    const ds = p.work_date.slice(0, 10)
    const c = corrMap.value.get(p.id)
    const disabled = !!c?.disabled
    const corrected = !!c && !disabled
    const pin = corrected ? c!.corrected_in : p.punch_in
    const pout = corrected ? c!.corrected_out : p.punch_out
    const row: StampRow = {
      key: p.id,
      from: formatGermanTime(pin),
      to: pout ? formatGermanTime(pout) : '',
      open: !pout,
      corrected,
      disabled,
      reason: c?.reason ?? '',
    }
    const arr = m.get(ds) ?? []
    arr.push(row)
    m.set(ds, arr)
  }
  return m
})

const absenceLabels: Record<string, string> = {
  vacation: 'Urlaub',
  sick: 'Krank',
  compensation_day: 'Ausgleichstag',
  other: 'Abwesend',
}

function dayTitle(d: SaldoDay): string {
  const dt = new Date(`${d.date}T12:00:00`)
  return `${weekdayDE[dt.getDay()]} ${String(dt.getDate()).padStart(2, '0')}.${String(dt.getMonth() + 1).padStart(2, '0')}.`
}

function dayTags(d: SaldoDay): string[] {
  const tags: string[] = []
  if (d.date === props.today) tags.push('Heute')
  if (d.holiday_name) tags.push(`Feiertag: ${d.holiday_name}`)
  if (d.closure_name) tags.push(`Schließtag: ${d.closure_name}`)
  if (d.absence_type) {
    const l = absenceLabels[d.absence_type] ?? 'Abwesend'
    tags.push(d.half_day ? `${l} (½ Tag)` : l)
  }
  if (!d.is_workday && !d.holiday_name && !d.closure_name && !d.absence_type) tags.push('Frei')
  return tags
}

function isCounted(d: SaldoDay): boolean {
  return d.date <= props.countedThrough
}

function hasCalc(d: SaldoDay): boolean {
  return d.gross_minutes > 0 || d.credit_hours > 0 || d.target_hours > 0
}

const creditLabel = (d: SaldoDay) => (d.absence_type ? `Gutschrift ${absenceLabels[d.absence_type] ?? ''}`.trim() : 'Gutschrift')

/** Wochensumme nur über gezählte Tage. */
const week = computed(() => {
  let worked = 0
  let target = 0
  let n = 0
  for (const d of props.days) {
    if (!isCounted(d)) continue
    worked += d.net_minutes / 60 + d.credit_hours
    target += d.target_hours
    n++
  }
  return { worked, target, balance: worked - target, n }
})

const weekThroughLabel = computed(() => {
  const counted = props.days.filter(isCounted)
  if (!counted.length) return ''
  const last = counted[counted.length - 1]!
  return counted.length === props.days.length ? 'ganze Woche' : `bis ${dayTitle(last)}`
})
</script>

<template>
  <div class="wdl" :class="{ dim: loading }">
    <div v-if="week.n > 0" class="week-sum" data-testid="week-summary">
      <div class="week-sum-title">Wochensumme ({{ weekThroughLabel }})</div>
      <div class="week-sum-row">
        <span>Ist {{ formatHoursHM(week.worked) }}</span>
        <span>Soll {{ formatHoursHM(week.target) }}</span>
        <strong :class="balanceClass(week.balance)">{{ formatHoursHM(week.balance, { signed: true }) }}</strong>
      </div>
    </div>

    <ul class="days">
      <li
        v-for="d in days"
        :key="d.date"
        class="day"
        :class="{ 'day--today': d.date === today, 'day--future': d.date > today }"
        data-testid="day-row"
      >
        <div class="day-head">
          <span class="day-title">{{ dayTitle(d) }}</span>
          <span class="tags">
            <span v-for="t in dayTags(d)" :key="t" class="tag">{{ t }}</span>
          </span>
        </div>

        <div v-if="scheduleByDate[d.date]" class="line muted">
          Geplant {{ scheduleByDate[d.date]!.shift_start }}–{{ scheduleByDate[d.date]!.shift_end }}
        </div>
        <ul v-if="stampsByDate.get(d.date)?.length" class="stamps">
          <li v-for="s in stampsByDate.get(d.date)" :key="s.key" :class="{ 'stamp--disabled': s.disabled }">
            Gestempelt {{ s.from }}–{{ s.open ? 'läuft' : s.to }}
            <span v-if="s.corrected" class="corr">korrigiert<template v-if="s.reason">: {{ s.reason }}</template></span>
            <span v-if="s.disabled" class="corr">zählt nicht<template v-if="s.reason">: {{ s.reason }}</template></span>
          </li>
        </ul>

        <dl v-if="hasCalc(d) && d.date <= today" class="calc">
          <template v-if="d.gross_minutes > 0">
            <div>
              <dt>Arbeitszeit</dt>
              <dd>{{ formatMinutesHM(d.gross_minutes) }}</dd>
            </div>
            <div v-if="d.stamped_break_minutes > 0">
              <dt>Ausgestempelte Pause</dt>
              <dd class="muted">{{ formatMinutesHM(d.stamped_break_minutes) }}, angerechnet</dd>
            </div>
            <div v-if="d.deduction_minutes > 0">
              <dt>Pausenabzug</dt>
              <dd>−{{ formatMinutesHM(d.deduction_minutes) }}</dd>
            </div>
            <div class="calc-sum">
              <dt>Netto</dt>
              <dd>{{ formatMinutesHM(d.net_minutes) }}</dd>
            </div>
          </template>
          <div v-if="d.credit_hours > 0">
            <dt>{{ creditLabel(d) }}</dt>
            <dd>+{{ formatHoursHM(d.credit_hours) }}</dd>
          </div>
          <div>
            <dt>Soll</dt>
            <dd>{{ formatHoursHM(d.target_hours) }}</dd>
          </div>
          <div class="calc-sum">
            <dt>Saldo</dt>
            <dd :class="isCounted(d) ? balanceClass(d.balance_hours) : ''">
              {{ formatHoursHM(d.balance_hours, { signed: true }) }}
            </dd>
          </div>
        </dl>
        <p v-if="d.date === today && hasCalc(d)" class="note">Vorläufig. Der Tag zählt ab morgen im Stundenkonto.</p>
        <p v-if="d.open_period && d.date < today" class="note warn">
          Ausstempeln fehlt. Dieser Block wird nicht gezählt, bitte bei der Leitung melden.
        </p>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.wdl.dim {
  opacity: 0.55;
}
.week-sum {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.65rem 0.9rem;
  margin-bottom: 0.75rem;
}
.week-sum-title {
  font-size: 0.8rem;
  color: #64748b;
  margin-bottom: 0.2rem;
}
.week-sum-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem 1rem;
  align-items: baseline;
  font-variant-numeric: tabular-nums;
}
.week-sum-row strong {
  margin-left: auto;
  font-size: 1.1rem;
}
.days {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}
.day {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.65rem 0.9rem;
}
.day--today {
  border-color: #3b82f6;
  box-shadow: inset 3px 0 0 #3b82f6;
}
.day--future {
  background: #f8fafc;
}
.day-head {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: center;
  gap: 0.35rem 0.75rem;
  margin-bottom: 0.3rem;
}
.day-title {
  font-weight: 700;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem;
}
.tag {
  font-size: 0.75rem;
  background: #eef2ff;
  color: #3730a3;
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
}
.line,
.stamps {
  font-size: 0.9rem;
}
.stamps {
  list-style: none;
  margin: 0.1rem 0 0;
  padding: 0;
}
.stamp--disabled {
  text-decoration: line-through;
  color: #94a3b8;
}
.corr {
  display: inline-block;
  margin-left: 0.35rem;
  font-size: 0.78rem;
  color: #b45309;
}
.calc {
  margin: 0.45rem 0 0;
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  font-size: 0.88rem;
}
.calc > div {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
}
.calc dt {
  color: #475569;
}
.calc dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
  text-align: right;
}
.calc-sum {
  font-weight: 600;
  border-top: 1px solid #f1f5f9;
  padding-top: 0.1rem;
}
.calc-sum dt {
  color: #0f172a;
}
.note {
  margin: 0.35rem 0 0;
  font-size: 0.8rem;
  color: #64748b;
}
.note.warn {
  color: #b45309;
}
.muted {
  color: #64748b;
}
.pos {
  color: #15803d;
}
.neg {
  color: #b91c1c;
}
</style>
