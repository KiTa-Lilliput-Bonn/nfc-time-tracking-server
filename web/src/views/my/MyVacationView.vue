<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Card from 'primevue/card'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'

import {
  fetchClosureDaysForMe,
  fetchMeAbsences,
  fetchMeProfile,
  fetchMeVacation,
} from '@/api/me'
import type { Absence, ClosureDay, VacationBalance } from '@/types/api'
import { formatGermanDate } from '@/utils/dates'

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

function artLabel(row: OverviewRow): string {
  if (row.kind === 'closure' && row.closure) {
    const d = new Date(`${row.dateISO}T12:00:00`)
    const dow = d.getDay()
    const fix = dow >= 1 && dow <= 5 && fixedWeekdays.value.has(dow)
    if (fix) {
      return `Schließtag (${row.closure.name}) — bei Ihnen regulär frei, kein Urlaubsabzug`
    }
    return `Schließtag (${row.closure.name})`
  }
  const a = row.absence
  if (!a) return '—'
  const half = a.half_day ? 'Halber Tag' : 'Ganzer Tag'
  if (row.closure) {
    return `${half} · Schließtag`
  }
  return half
}

onMounted(async () => {
  loading.value = true
  err.value = ''
  try {
    const [vb, abs, cls, prof] = await Promise.all([
      fetchMeVacation(),
      fetchMeAbsences(`${year}-01-01`, `${year}-12-31`),
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
      <div v-if="balance" class="cards">
        <Card>
          <template #title>Anspruch {{ balance.year }}</template>
          <template #content>
            <dl class="row">
              <div v-if="balance.carried_over !== 0">
                <dt>Start (Import)</dt>
                <dd>{{ balance.carried_over.toFixed(1) }}</dd>
              </div>
              <div>
                <dt>Tage/Jahr</dt>
                <dd>{{ balance.entitlement.toFixed(1) }}</dd>
              </div>
              <div>
                <dt>Genommen</dt>
                <dd>{{ balance.taken.toFixed(1) }}</dd>
              </div>
              <div>
                <dt>Rest</dt>
                <dd class="highlight">{{ balance.remaining.toFixed(1) }}</dd>
              </div>
            </dl>
          </template>
        </Card>
      </div>
      <Card>
        <template #title>Urlaub &amp; Schließtage {{ year }}</template>
        <template #content>
          <DataTable
            :value="overviewRows"
            size="small"
            :empty-message="'Keine Einträge für dieses Jahr.'"
          >
            <Column field="dateISO" header="Datum" sortable>
              <template #body="{ data }: { data: OverviewRow }">
                {{ formatGermanDate(data.dateISO) }}
              </template>
            </Column>
            <Column header="Art">
              <template #body="{ data }: { data: OverviewRow }">
                {{ artLabel(data) }}
              </template>
            </Column>
          </DataTable>
        </template>
      </Card>
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.cards {
  max-width: 480px;
}
.row {
  display: flex;
  gap: 2rem;
  margin: 0;
}
.row dt {
  font-size: 0.75rem;
  color: #64748b;
  margin: 0 0 0.2rem;
}
.row dd {
  margin: 0;
  font-size: 1.25rem;
  font-weight: 600;
}
.highlight {
  color: #3730a3;
}
.err {
  color: #b91c1c;
}
.muted {
  color: #64748b;
}
</style>
