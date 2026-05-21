<script setup lang="ts">
import { computed } from 'vue'

import type { VacationBalance } from '@/types/api'

const props = defineProps<{
  balance: VacationBalance | null
}>()

/** Gesamt-Baseline = Stand 01.01. (Übertrag + Startsaldo + Anspruch); offen = Gesamt − genommen − geplant. */
const vacationBar = computed(() => {
  const v = props.balance
  if (!v) return null
  const planned = v.planned ?? 0
  const total = (v.carryover ?? 0) + v.carried_over + v.entitlement
  if (total <= 0) {
    return {
      total,
      taken: v.taken,
      planned,
      open: Math.max(0, total - v.taken - planned),
      pctTaken: 0,
      pctPlanned: 0,
      pctOpen: 0,
    }
  }
  const taken = v.taken
  const openRaw = total - taken - planned
  let a = (100 * taken) / total
  let b = (100 * planned) / total
  let c = (100 * Math.max(0, openRaw)) / total
  const sum = a + b + c
  if (sum > 100.001) {
    a = (a / sum) * 100
    b = (b / sum) * 100
    c = (c / sum) * 100
  }
  return { total, taken, planned, open: Math.max(0, openRaw), pctTaken: a, pctPlanned: b, pctOpen: c }
})

const vacationBarAriaLabel = computed(() => {
  const b = vacationBar.value
  if (!b) return ''
  return `Urlaub: ${b.total.toFixed(1)} Tage gesamt, ${b.taken.toFixed(1)} genommen, ${b.planned.toFixed(1)} geplant, ${b.open.toFixed(1)} noch offen`
})

const vacationBarTitle = computed(() => {
  const b = vacationBar.value
  if (!b) return ''
  return [
    `Gesamt (Stand 01.01.): ${b.total.toFixed(1)} Tg.`,
    `Genommen: ${b.taken.toFixed(1)} Tg.`,
    `Geplant: ${b.planned.toFixed(1)} Tg.`,
    `Noch offen: ${b.open.toFixed(1)} Tg.`,
  ].join(' · ')
})
</script>

<template>
  <div v-if="!balance" class="muted">Keine Urlaubsdaten.</div>
  <template v-else-if="vacationBar">
    <p class="stat vacation-bar-total">
      Gesamt: <strong>{{ vacationBar.total.toFixed(1) }}</strong> Tage
      <span class="sub vacation-bar-hint">(Stand 01.01.: Übertrag + Startsaldo + Anspruch)</span>
    </p>
    <div v-tooltip.bottom="vacationBarTitle" class="vacation-bar" role="img" :aria-label="vacationBarAriaLabel">
      <div
        v-if="vacationBar.pctTaken > 0"
        class="vacation-bar-seg vacation-bar-seg--taken"
        :style="{ width: vacationBar.pctTaken + '%' }"
      >
        <span v-if="vacationBar.pctTaken >= 12" class="vacation-bar-label">{{ vacationBar.taken.toFixed(1) }}</span>
      </div>
      <div
        v-if="vacationBar.pctPlanned > 0"
        class="vacation-bar-seg vacation-bar-seg--planned"
        :style="{ width: vacationBar.pctPlanned + '%' }"
      >
        <span v-if="vacationBar.pctPlanned >= 12" class="vacation-bar-label">{{ vacationBar.planned.toFixed(1) }}</span>
      </div>
      <div
        v-if="vacationBar.pctOpen > 0"
        class="vacation-bar-seg vacation-bar-seg--open"
        :style="{ width: vacationBar.pctOpen + '%' }"
      >
        <span v-if="vacationBar.pctOpen >= 12" class="vacation-bar-label">{{ vacationBar.open.toFixed(1) }}</span>
      </div>
    </div>
    <p v-if="vacationBar.taken + vacationBar.planned > vacationBar.total + 0.05" class="sub vacation-bar-warn">
      Hinweis: Genommen und geplant übersteigen den Gesamtanspruch rechnerisch — bitte Daten prüfen.
    </p>
  </template>
</template>

<style scoped>
.stat {
  margin: 0 0 0.35rem;
  font-size: 0.95rem;
}
.sub {
  margin: 0;
  font-size: 0.85rem;
  color: #64748b;
}
.muted {
  color: #64748b;
}
.vacation-bar-total {
  margin-bottom: 0.5rem;
}
.vacation-bar-hint {
  display: block;
  margin-top: 0.2rem;
  font-weight: 400;
}
.vacation-bar {
  display: flex;
  width: 100%;
  height: 1.125rem;
  border-radius: 6px;
  overflow: hidden;
  background: #e2e8f0;
  margin-bottom: 0.65rem;
}
.vacation-bar-seg {
  min-width: 0;
  height: 100%;
  transition: width 0.2s ease;
  position: relative;
}
.vacation-bar-label {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  font-size: 0.75rem;
  font-weight: 700;
  color: #ffffff;
  text-shadow:
    0 1px 2px rgba(0, 0, 0, 0.45),
    0 0 1px rgba(0, 0, 0, 0.35);
  pointer-events: none;
}
.vacation-bar-seg--taken {
  background: #3b82f6;
}
.vacation-bar-seg--planned {
  background: #f59e0b;
}
.vacation-bar-seg--open {
  background: #22c55e;
}
.vacation-bar-warn {
  margin-top: 0.5rem;
  color: #b45309;
}
</style>
