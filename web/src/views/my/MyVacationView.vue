<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Card from 'primevue/card'

import VacationBalanceBar from '@/components/VacationBalanceBar.vue'

import {
  fetchClosureDaysForMe,
  fetchMeAbsences,
  fetchMeProfile,
  fetchMeVacation,
} from '@/api/me'
import type { Absence, ClosureDay, VacationBalance } from '@/types/api'
import { formatGermanDate, toISODateLocal } from '@/utils/dates'

function normalizeISODate(s: string): string {
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})/)
  return m ? `${m[1]}-${m[2]}-${m[3]}` : String(s)
}

interface OverviewRow {
  dateISO: string
  kind: 'vacation' | 'closure'
  absence?: Absence
  closure?: ClosureDay
}

const balance = ref<VacationBalance | null>(null)
const vacationAbsences = ref<Absence[]>([])
const closuresInYear = ref<ClosureDay[]>([])
const fixedWeekdays = ref<Set<number>>(new Set())
const loading = ref(true)
const err = ref('')

const year = new Date().getFullYear()

const overviewRows = computed<OverviewRow[]>(() => {
  const y = `${year}`
  const vacList = vacationAbsences.value.filter((a) => a.absence_type === 'vacation')
  const vacByDate = new Map<string, Absence>()
  for (const a of vacList) {
    vacByDate.set(normalizeISODate(a.absence_date), a)
  }

  const cloByDate = new Map<string, ClosureDay>()
  for (const c of closuresInYear.value) {
    const iso = normalizeISODate(c.closure_date)
    if (iso.startsWith(y)) {
      cloByDate.set(iso, c)
    }
  }

  const dates = new Set<string>([...vacByDate.keys(), ...cloByDate.keys()])
  const sorted = [...dates].sort()
  const rows: OverviewRow[] = []
  for (const iso of sorted) {
    const vac = vacByDate.get(iso)
    const clo = cloByDate.get(iso)
    if (vac) {
      rows.push({ dateISO: iso, kind: 'vacation', absence: vac, closure: clo })
    } else if (clo) {
      rows.push({ dateISO: iso, kind: 'closure', closure: clo })
    }
  }
  return rows
})

const todayISO = toISODateLocal(new Date())

/** Aufeinanderfolgende Einträge gleicher Art zu Blöcken zusammenfassen (z. B. „03.08.–12.08. · 7 Tage“). */
interface OverviewBlock {
  from: string
  to: string
  kind: 'vacation' | 'closure'
  days: number
  label: string
  note: string
}

function rowLabel(row: OverviewRow): { label: string; note: string; days: number } {
  if (row.kind === 'closure' && row.closure) {
    const d = new Date(`${row.dateISO}T12:00:00`)
    const dow = d.getDay()
    const fix = dow >= 1 && dow <= 5 && fixedWeekdays.value.has(dow)
    return {
      label: `Schließtag: ${row.closure.name}`,
      note: fix ? 'Regulär freier Tag, kein Urlaubsabzug.' : 'Kein Urlaubsabzug.',
      days: 0,
    }
  }
  const a = row.absence
  const half = !!a?.half_day
  return {
    label: half ? 'Urlaub (halber Tag)' : 'Urlaub',
    note: row.closure ? 'Fällt auf einen Schließtag.' : '',
    days: half ? 0.5 : 1,
  }
}

function isNextWorkdayGap(prevISO: string, nextISO: string): boolean {
  const a = new Date(`${prevISO}T12:00:00`)
  const b = new Date(`${nextISO}T12:00:00`)
  for (let d = new Date(a); ; ) {
    d.setDate(d.getDate() + 1)
    const iso = toISODateLocal(d)
    if (iso === nextISO) return true
    if (d > b) return false
    const dow = d.getDay()
    const off = dow === 0 || dow === 6 || fixedWeekdays.value.has(dow)
    if (!off) return false
  }
}

function toBlocks(rows: OverviewRow[]): OverviewBlock[] {
  const out: OverviewBlock[] = []
  for (const row of rows) {
    const l = rowLabel(row)
    const last = out[out.length - 1]
    const mergeable =
      last &&
      row.kind === 'vacation' &&
      last.kind === 'vacation' &&
      l.days === 1 &&
      last.label === 'Urlaub' &&
      !l.note &&
      !last.note &&
      isNextWorkdayGap(last.to, row.dateISO)
    if (mergeable) {
      last.to = row.dateISO
      last.days += 1
      continue
    }
    out.push({ from: row.dateISO, to: row.dateISO, kind: row.kind, days: l.days, label: l.label, note: l.note })
  }
  return out
}

const plannedBlocks = computed(() => toBlocks(overviewRows.value.filter((r) => r.dateISO > todayISO)))
const pastBlocks = computed(() => toBlocks(overviewRows.value.filter((r) => r.dateISO <= todayISO)).reverse())

function blockDate(b: OverviewBlock): string {
  if (b.from === b.to) return formatGermanDate(b.from)
  return `${formatGermanDate(b.from).slice(0, 6)}–${formatGermanDate(b.to)}`
}

function blockDays(b: OverviewBlock): string {
  if (b.kind === 'closure') return ''
  if (b.days === 0.5) return '½ Tag'
  return b.days === 1 ? '1 Tag' : `${b.days} Tage`
}

onMounted(async () => {
  loading.value = true
  err.value = ''
  try {
    const [vb, abs, cls, prof] = await Promise.all([
      fetchMeVacation(),
      fetchMeAbsences(`${year}-01-01`, `${year + 1}-12-31`),
      fetchClosureDaysForMe(),
      fetchMeProfile(),
    ])
    balance.value = vb
    vacationAbsences.value = abs.absences.filter((a) => a.absence_type === 'vacation')
    closuresInYear.value = cls
    fixedWeekdays.value = new Set(prof.fixed_non_work_weekdays ?? [])
  } catch {
    err.value = 'Urlaubsdaten konnten nicht geladen werden.'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page">
    <p v-if="err" class="err">{{ err }}</p>
    <div v-if="loading" class="muted">Laden…</div>
    <template v-else>
      <RouterLink to="/my/requests?type=vacation" class="request-link">Urlaub beantragen</RouterLink>
      <Card v-if="balance">
        <template #title>Urlaub {{ balance.year }}</template>
        <template #content>
          <VacationBalanceBar :balance="balance" show-breakdown />
          <p class="hint">
            Der Übertrag ist der Rest aus den Vorjahren. „Geplant“ sind alle eingetragenen Urlaubstage nach heute.
          </p>
        </template>
      </Card>
      <Card>
        <template #title>Geplant</template>
        <template #content>
          <p v-if="!plannedBlocks.length" class="muted">Kein Urlaub geplant.</p>
          <ul v-else class="blocks" data-testid="vacation-planned">
            <li v-for="b in plannedBlocks" :key="b.from" :class="['block', `block--${b.kind}`]">
              <div class="block-main">
                <span class="block-date">{{ blockDate(b) }}</span>
                <span class="block-days">{{ blockDays(b) }}</span>
              </div>
              <div class="block-label">{{ b.label }}</div>
              <div v-if="b.note" class="block-note">{{ b.note }}</div>
            </li>
          </ul>
        </template>
      </Card>
      <Card>
        <template #title>Genommen {{ year }}</template>
        <template #content>
          <p v-if="!pastBlocks.length" class="muted">Noch kein Urlaub genommen.</p>
          <ul v-else class="blocks" data-testid="vacation-taken">
            <li v-for="b in pastBlocks" :key="b.from" :class="['block', `block--${b.kind}`]">
              <div class="block-main">
                <span class="block-date">{{ blockDate(b) }}</span>
                <span class="block-days">{{ blockDays(b) }}</span>
              </div>
              <div class="block-label">{{ b.label }}</div>
              <div v-if="b.note" class="block-note">{{ b.note }}</div>
            </li>
          </ul>
        </template>
      </Card>
    </template>
  </div>
</template>

<style scoped>
.request-link {
  align-self: flex-start;
  color: #4f46e5;
}
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 40rem;
}
.hint {
  margin: 0.75rem 0 0;
  font-size: 0.85rem;
  color: #64748b;
}
.blocks {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}
.block {
  padding: 0.55rem 0;
  border-bottom: 1px solid #e2e8f0;
}
.block:last-child {
  border-bottom: none;
}
.block-main {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  font-weight: 600;
}
.block-days {
  color: #475569;
  font-weight: 500;
  white-space: nowrap;
}
.block-label {
  font-size: 0.9rem;
  color: #334155;
}
.block--closure .block-label {
  color: #047857;
}
.block-note {
  font-size: 0.8rem;
  color: #64748b;
}
.err {
  color: #b91c1c;
}
.muted {
  color: #64748b;
  margin: 0;
}
</style>
