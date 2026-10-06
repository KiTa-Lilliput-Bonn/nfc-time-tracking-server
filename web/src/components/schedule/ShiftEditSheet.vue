<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'

import type { BreakRule, TeamMeeting } from '@/types/api'
import {
  clockToMinutes,
  formatHoursMinutes,
  minutesToClock,
  shiftGrossMinutes,
  shiftNetMinutes,
  shortShiftLabel,
  type ShiftPattern,
} from '@/utils/schedulePlanning'
import { teamMeetingBarLabel } from '@/utils/teamMeetingLabel'

/** Was am Tag gilt; `blocked` = keine Planung möglich (Feiertag, fix frei, ganztägig Urlaub/Ausgleich). */
export interface ShiftSheetDay {
  blocked: boolean
  /** Kurzer Hinweis wie „Urlaub“, „krank“, „Feiertag · Tag der Deutschen Einheit“ */
  note: string
}

const props = defineProps<{
  employeeName: string
  dateLabel: string
  groupName: string
  start: string
  end: string
  day: ShiftSheetDay
  quickPicks: ShiftPattern[]
  /** Schicht am gleichen Wochentag der Vorwoche */
  lastWeek: { start: string; end: string } | null
  lastWeekLabel: string
  meetings: TeamMeeting[]
  breakRules: BreakRule[]
  /** Geplante Minuten der Woche ohne diesen Tag */
  weekOtherMinutes: number
  /** Soll-Minuten der Woche (0 = keine Wochenstunden hinterlegt) */
  weekTargetMinutes: number
  prevLabel: string
  nextLabel: string
  saving: boolean
}>()

const visible = defineModel<boolean>('visible', { default: false })

const emit = defineEmits<{
  save: [start: string, end: string]
  clear: []
  nav: [dir: -1 | 1, start: string, end: string]
}>()

const startVal = ref('')
const endVal = ref('')
const active = ref<'start' | 'end'>('start')

const hasShift = computed(() => Boolean(props.start.trim() && props.end.trim()))

function reset() {
  if (hasShift.value) {
    startVal.value = minutesToClock(clockToMinutes(props.start) ?? 0)
    endVal.value = minutesToClock(clockToMinutes(props.end) ?? 0)
  } else {
    const first = props.quickPicks[0] ?? props.lastWeek
    startVal.value = first?.start ?? '08:00'
    endVal.value = first?.end ?? '16:00'
  }
  active.value = 'start'
}

watch(
  () => [visible.value, props.employeeName, props.dateLabel, props.start, props.end] as const,
  ([v]) => {
    if (v) reset()
  },
  { immediate: true },
)

function step(deltaMin: number) {
  const target = active.value === 'start' ? startVal : endVal
  const cur = clockToMinutes(target.value) ?? 8 * 60
  target.value = minutesToClock(Math.max(0, Math.min(23 * 60 + 45, cur + deltaMin)))
}

function pick(p: { start: string; end: string }) {
  startVal.value = p.start
  endVal.value = p.end
}

function isPicked(p: { start: string; end: string }) {
  return p.start === startVal.value && p.end === endVal.value
}

const valid = computed(() => shiftGrossMinutes(startVal.value, endVal.value) > 0)
const netMin = computed(() => shiftNetMinutes(startVal.value, endVal.value, props.breakRules))
const grossMin = computed(() => shiftGrossMinutes(startVal.value, endVal.value))
const breakMin = computed(() => grossMin.value - netMin.value)
const weekAfter = computed(() => props.weekOtherMinutes + (valid.value ? netMin.value : 0))
const weekStatus = computed(() => {
  if (props.weekTargetMinutes <= 0) return ''
  const diff = weekAfter.value - props.weekTargetMinutes
  if (Math.abs(diff) < 1) return 'ok'
  return diff > 0 ? 'over' : 'under'
})

const changed = computed(() => {
  if (!hasShift.value) return true
  return (
    clockToMinutes(startVal.value) !== clockToMinutes(props.start) ||
    clockToMinutes(endVal.value) !== clockToMinutes(props.end)
  )
})

const warnings = computed(() => {
  const out: string[] = []
  if (!valid.value) {
    out.push('Das Ende muss nach dem Beginn liegen.')
    return out
  }
  if (grossMin.value > 10 * 60) out.push('Die Schicht ist länger als 10 Stunden.')
  const s = clockToMinutes(startVal.value)!
  const e = clockToMinutes(endVal.value)!
  for (const m of props.meetings) {
    const ms = clockToMinutes(m.time_start)
    const me = clockToMinutes(m.time_end)
    if (ms == null || me == null) continue
    if (ms < s || me > e) out.push(`${teamMeetingBarLabel(m)} liegt nicht ganz in der Schicht.`)
  }
  return out
})

function onSave() {
  if (!valid.value || props.saving) return
  emit('save', startVal.value, endVal.value)
}

function onNav(dir: -1 | 1) {
  emit('nav', dir, startVal.value, endVal.value)
}

/** Navigation speichert nur, wenn sich etwas an einer bestehenden Schicht geändert hat. */
defineExpose({ changed, valid })
</script>

<template>
  <Drawer
    v-model:visible="visible"
    position="bottom"
    class="shift-sheet"
    :show-close-icon="false"
    :block-scroll="true"
    :pt="{ root: { 'data-testid': 'shift-sheet' } }"
  >
    <template #container="{ closeCallback }">
      <div class="sheet">
        <button type="button" class="grab" aria-label="Schließen" @click="closeCallback" />
        <div class="head">
          <strong data-testid="shift-sheet-name">{{ employeeName }}</strong>
          <span>{{ dateLabel }}<template v-if="groupName"> · {{ groupName }}</template></span>
        </div>
        <p class="sub">
          <template v-if="day.note">{{ day.note }} · </template>
          <template v-if="!hasShift && !day.blocked">Noch nichts geplant</template>
          <template v-else-if="hasShift">Geplant {{ shortShiftLabel(start, end) }}</template>
          <template v-if="weekTargetMinutes > 0 && !day.blocked">
            · Woche ohne diesen Tag {{ formatHoursMinutes(weekOtherMinutes) }} von
            {{ formatHoursMinutes(weekTargetMinutes) }}
          </template>
        </p>

        <template v-if="day.blocked">
          <p class="blocked" data-testid="shift-sheet-blocked">
            An diesem Tag wird nicht geplant. Abwesenheiten änderst du unter „Abwesenheiten“.
          </p>
        </template>
        <template v-else>
          <div class="times">
            <label class="tbox" :class="{ on: active === 'start' }" @click="active = 'start'">
              <small>Beginn</small>
              <input
                v-model="startVal"
                type="time"
                step="900"
                data-testid="shift-start"
                aria-label="Beginn"
                @focus="active = 'start'"
              />
            </label>
            <span class="dash">–</span>
            <label class="tbox" :class="{ on: active === 'end' }" @click="active = 'end'">
              <small>Ende</small>
              <input
                v-model="endVal"
                type="time"
                step="900"
                data-testid="shift-end"
                aria-label="Ende"
                @focus="active = 'end'"
              />
            </label>
          </div>
          <div class="steps" :aria-label="active === 'start' ? 'Beginn verstellen' : 'Ende verstellen'">
            <button type="button" @click="step(-60)">−1 h</button>
            <button type="button" @click="step(-15)">−15 min</button>
            <button type="button" @click="step(15)">+15 min</button>
            <button type="button" @click="step(60)">+1 h</button>
          </div>

          <template v-if="quickPicks.length || lastWeek">
            <div class="h6">Schnellwahl</div>
            <div class="chips">
              <button
                v-for="p in quickPicks"
                :key="p.start + p.end"
                type="button"
                class="chip"
                :class="{ on: isPicked(p) }"
                data-testid="shift-pick"
                @click="pick(p)"
              >
                {{ shortShiftLabel(p.start, p.end) }}
              </button>
              <button
                v-if="lastWeek && !quickPicks.some((p) => p.start === lastWeek!.start && p.end === lastWeek!.end)"
                type="button"
                class="chip"
                :class="{ on: isPicked(lastWeek) }"
                @click="pick(lastWeek)"
              >
                wie {{ lastWeekLabel }} ({{ shortShiftLabel(lastWeek.start, lastWeek.end) }})
              </button>
            </div>
          </template>

          <div class="sum" data-testid="shift-sum">
            <span v-if="valid">
              {{ formatHoursMinutes(netMin) }}<template v-if="breakMin > 0"> (nach {{ breakMin }} min Pause)</template>
            </span>
            <span v-else>–</span>
            <strong v-if="weekTargetMinutes > 0" :class="weekStatus">
              Woche {{ formatHoursMinutes(weekAfter) }} / {{ formatHoursMinutes(weekTargetMinutes) }}
              <template v-if="weekStatus === 'ok'">✓</template>
            </strong>
            <strong v-else>Woche {{ formatHoursMinutes(weekAfter) }}</strong>
          </div>
          <ul v-if="warnings.length" class="warn">
            <li v-for="w in warnings" :key="w">{{ w }}</li>
          </ul>

          <div class="btns">
            <Button
              v-if="hasShift"
              label="Frei"
              severity="danger"
              outlined
              class="btn-free"
              data-testid="shift-clear"
              :disabled="saving"
              @click="emit('clear')"
            />
            <Button
              label="Speichern"
              class="btn-save"
              data-testid="shift-save"
              :disabled="!valid || saving || !changed"
              :loading="saving"
              @click="onSave"
            />
          </div>
        </template>

        <div class="nav">
          <button v-if="prevLabel" type="button" data-testid="shift-prev" @click="onNav(-1)">‹ {{ prevLabel }}</button>
          <span v-else />
          <button v-if="nextLabel" type="button" data-testid="shift-next" @click="onNav(1)">{{ nextLabel }} ›</button>
        </div>
      </div>
    </template>
  </Drawer>
</template>

<style scoped>
.sheet {
  padding: 0.5rem 1.1rem calc(1.25rem + env(safe-area-inset-bottom, 0px));
  max-height: 88dvh;
  overflow-y: auto;
}
.grab {
  display: block;
  width: 44px;
  height: 5px;
  border: none;
  border-radius: 3px;
  background: #cbd5e1;
  margin: 0.25rem auto 0.9rem;
  padding: 0;
  cursor: pointer;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.head strong {
  font-size: 1.15rem;
}
.head span {
  font-size: 0.85rem;
  color: #64748b;
}
.sub {
  font-size: 0.82rem;
  color: #64748b;
  margin: 0.2rem 0 0.9rem;
}
.blocked {
  background: #f1f5f9;
  border-radius: 10px;
  padding: 0.75rem;
  font-size: 0.9rem;
  color: #334155;
}
.times {
  display: flex;
  align-items: center;
  gap: 0.6rem;
}
.dash {
  color: #94a3b8;
}
.tbox {
  flex: 1;
  min-width: 0;
  border: 1.5px solid #e2e8f0;
  border-radius: 14px;
  text-align: center;
  padding: 0.4rem 0.25rem 0.3rem;
  cursor: pointer;
}
.tbox.on {
  border-color: #10b981;
  background: #ecfdf5;
}
.tbox small {
  display: block;
  font-size: 0.72rem;
  color: #64748b;
}
.tbox input {
  border: none;
  background: transparent;
  font: inherit;
  font-size: 1.6rem;
  font-weight: 700;
  min-width: 0;
  font-variant-numeric: tabular-nums;
  text-align: center;
  width: 100%;
  color: #0f172a;
  padding: 0;
}
.tbox input:focus {
  outline: none;
}
.steps {
  display: flex;
  justify-content: center;
  gap: 0.4rem;
  margin-top: 0.5rem;
}
.steps button,
.chip {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 999px;
  padding: 0.4rem 0.7rem;
  font-size: 0.85rem;
  color: #1e293b;
  cursor: pointer;
}
.steps button {
  border-radius: 8px;
  padding: 0.35rem 0.6rem;
}
.h6 {
  font-size: 0.7rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #94a3b8;
  margin-top: 0.9rem;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  margin-top: 0.4rem;
}
.chip.on {
  background: #10b981;
  border-color: #10b981;
  color: #fff;
}
.sum {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  background: #f8fafc;
  border-radius: 10px;
  padding: 0.55rem 0.75rem;
  font-size: 0.85rem;
  margin-top: 0.9rem;
  font-variant-numeric: tabular-nums;
}
.sum .ok {
  color: #047857;
}
.sum .over {
  color: #b45309;
}
.warn {
  margin: 0.6rem 0 0;
  padding: 0.5rem 0.75rem 0.5rem 1.75rem;
  background: #fff7ed;
  border: 1px solid #fed7aa;
  border-radius: 10px;
  color: #9a3412;
  font-size: 0.82rem;
}
.btns {
  display: flex;
  gap: 0.5rem;
  margin-top: 0.9rem;
}
.btn-free {
  flex: 1;
}
.btn-save {
  flex: 2;
}
.nav {
  display: flex;
  justify-content: space-between;
  margin-top: 0.9rem;
}
.nav button {
  border: none;
  background: none;
  color: #059669;
  font-weight: 600;
  font-size: 0.88rem;
  padding: 0.3rem 0;
  cursor: pointer;
}
</style>

<style>
.p-drawer-bottom .p-drawer.shift-sheet {
  height: auto;
  max-height: 92dvh;
  border-radius: 20px 20px 0 0;
}
</style>
