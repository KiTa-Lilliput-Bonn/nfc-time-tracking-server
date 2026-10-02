<script setup lang="ts">
import { computed } from 'vue'

import type { VacationBalance } from '@/types/api'
import { formatDays } from '@/utils/hours'

const props = defineProps<{
  balance: VacationBalance | null
  /** Herleitung (Startsaldo + Übertrag + Anspruch) anzeigen. */
  showBreakdown?: boolean
}>()

/** Segmente relativ zum Gesamtanspruch: genommen | geplant | frei. */
const bar = computed(() => {
  const v = props.balance
  if (!v) return null
  const total = v.total
  const taken = v.taken
  const planned = v.planned
  const free = Math.max(0, v.free)
  if (total <= 0) {
    return { pctTaken: 0, pctPlanned: 0, pctFree: 0 }
  }
  let a = (100 * taken) / total
  let b = (100 * planned) / total
  let c = (100 * free) / total
  const sum = a + b + c
  if (sum > 100.001) {
    a = (a / sum) * 100
    b = (b / sum) * 100
    c = (c / sum) * 100
  }
  return { pctTaken: a, pctPlanned: b, pctFree: c }
})

const ariaLabel = computed(() => {
  const v = props.balance
  if (!v) return ''
  return `Urlaub ${v.year}: ${formatDays(v.total)} Tage gesamt, ${formatDays(v.taken)} genommen, ${formatDays(v.planned)} geplant, ${formatDays(v.free)} frei`
})
</script>

<template>
  <div v-if="!balance" class="muted">Keine Urlaubsdaten.</div>
  <div v-else class="vac">
    <dl v-if="showBreakdown" class="calc" data-testid="vacation-breakdown">
      <div v-if="balance.carried_over !== 0" class="calc-row">
        <dt>Startsaldo (Übernahme)</dt>
        <dd>{{ formatDays(balance.carried_over) }}</dd>
      </div>
      <div class="calc-row">
        <dt>Übertrag aus {{ balance.year - 1 }}</dt>
        <dd>{{ formatDays(balance.carryover) }}</dd>
      </div>
      <div class="calc-row">
        <dt>Anspruch {{ balance.year }}</dt>
        <dd>+ {{ formatDays(balance.entitlement) }}</dd>
      </div>
      <div class="calc-row calc-sum">
        <dt>Gesamt</dt>
        <dd>{{ formatDays(balance.total) }}</dd>
      </div>
      <div class="calc-row">
        <dt>Genommen {{ balance.year }}</dt>
        <dd>− {{ formatDays(balance.taken) }}</dd>
      </div>
      <div class="calc-row calc-sum">
        <dt>Rest</dt>
        <dd>{{ formatDays(balance.remaining) }}</dd>
      </div>
      <div class="calc-row">
        <dt>Schon geplant</dt>
        <dd>− {{ formatDays(balance.planned) }}</dd>
      </div>
      <div class="calc-row calc-sum calc-free">
        <dt>Noch frei verplanbar</dt>
        <dd data-testid="vacation-free">{{ formatDays(balance.free) }} Tage</dd>
      </div>
    </dl>
    <p v-else class="headline">
      <strong data-testid="vacation-free">{{ formatDays(balance.free) }}</strong> Tage noch frei
      <span class="sub">von {{ formatDays(balance.total) }} Tagen {{ balance.year }}</span>
    </p>

    <div v-if="bar" class="vacation-bar" role="img" :aria-label="ariaLabel">
      <div v-if="bar.pctTaken > 0" class="seg seg--taken" :style="{ width: bar.pctTaken + '%' }" />
      <div v-if="bar.pctPlanned > 0" class="seg seg--planned" :style="{ width: bar.pctPlanned + '%' }" />
      <div v-if="bar.pctFree > 0" class="seg seg--free" :style="{ width: bar.pctFree + '%' }" />
    </div>
    <ul class="legend">
      <li><span class="dot dot--taken" />Genommen {{ formatDays(balance.taken) }}</li>
      <li><span class="dot dot--planned" />Geplant {{ formatDays(balance.planned) }}</li>
      <li><span class="dot dot--free" />Frei {{ formatDays(balance.free) }}</li>
    </ul>
    <p v-if="balance.free < -0.05" class="sub warn">
      Es sind mehr Urlaubstage eingetragen als verfügbar. Bitte mit der Leitung klären.
    </p>
  </div>
</template>

<style scoped>
.muted {
  color: #64748b;
}
.sub {
  font-size: 0.85rem;
  color: #64748b;
}
.headline {
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
}
.headline strong {
  font-size: 1.35rem;
}
.headline .sub {
  display: block;
  margin-top: 0.15rem;
}
.calc {
  margin: 0 0 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  max-width: 22rem;
}
.calc-row {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  font-size: 0.92rem;
}
.calc-row dt {
  color: #475569;
}
.calc-row dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
}
.calc-sum {
  border-top: 1px solid #e2e8f0;
  padding-top: 0.2rem;
  font-weight: 600;
}
.calc-sum dt {
  color: #0f172a;
}
.calc-free dd {
  color: #15803d;
}
.vacation-bar {
  display: flex;
  width: 100%;
  height: 0.85rem;
  border-radius: 6px;
  overflow: hidden;
  background: #e2e8f0;
  margin-bottom: 0.5rem;
}
.seg {
  height: 100%;
  min-width: 0;
}
.seg--taken,
.dot--taken {
  background: #3b82f6;
}
.seg--planned,
.dot--planned {
  background: #f59e0b;
}
.seg--free,
.dot--free {
  background: #22c55e;
}
.legend {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem 1rem;
  font-size: 0.85rem;
  color: #334155;
}
.legend li {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}
.dot {
  width: 0.7rem;
  height: 0.7rem;
  border-radius: 3px;
  display: inline-block;
}
.warn {
  margin: 0.5rem 0 0;
  color: #b45309;
}
</style>
