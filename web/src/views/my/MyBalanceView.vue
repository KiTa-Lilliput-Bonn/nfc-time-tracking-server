<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'

import { fetchMeBalance, fetchMeHoursAccount } from '@/api/me'
import type { HoursAccount, MonthBalance } from '@/types/api'
import { formatGermanDate } from '@/utils/dates'
import { balanceClass, formatHoursHM } from '@/utils/hours'

const currentYear = new Date().getFullYear()
const year = ref(currentYear)
const balances = ref<MonthBalance[]>([])
const account = ref<HoursAccount | null>(null)
const loading = ref(false)
const err = ref('')

const monthNames = [
  'Januar',
  'Februar',
  'März',
  'April',
  'Mai',
  'Juni',
  'Juli',
  'August',
  'September',
  'Oktober',
  'November',
  'Dezember',
]

async function loadAccount() {
  try {
    account.value = await fetchMeHoursAccount()
  } catch {
    account.value = null
  }
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const tasks: Promise<MonthBalance>[] = []
    for (let m = 1; m <= 12; m++) {
      tasks.push(fetchMeBalance(m, year.value))
    }
    balances.value = await Promise.all(tasks)
  } catch {
    err.value = 'Salden konnten nicht geladen werden.'
    balances.value = []
  } finally {
    loading.value = false
  }
}

/** Nur Monate mit gezählten Tagen (ab Kontobeginn, bis gestern), neueste zuerst. */
const countedMonths = computed(() => balances.value.filter((b) => !!b.counted_through).slice().reverse())

const accountStartLabel = computed(() => (account.value ? formatGermanDate(account.value.account_start) : ''))
const countedThroughLabel = computed(() => (account.value ? formatGermanDate(account.value.counted_through) : ''))

function monthTitle(b: MonthBalance): string {
  return `${monthNames[b.month - 1]} ${b.year}`
}

function monthRange(b: MonthBalance): string {
  if (!b.counted_from || !b.counted_through) return ''
  if (b.is_partial) return `bis ${formatGermanDate(b.counted_through)}`
  const start = `${b.year}-${String(b.month).padStart(2, '0')}-01`
  if (b.counted_from !== start) return `ab ${formatGermanDate(b.counted_from)}`
  return ''
}

onMounted(() => {
  void loadAccount()
  void load()
})
watch(year, load)
</script>

<template>
  <div class="page">
    <Card v-if="account" class="account-card" data-testid="hours-account">
      <template #content>
        <div class="account-label">Stundenkonto</div>
        <div class="account-value" :class="balanceClass(account.balance_hours)">
          {{ formatHoursHM(account.balance_hours, { signed: true }) }}
        </div>
        <div class="account-meta">Stand: {{ countedThroughLabel }} (der heutige Tag zählt ab morgen mit)</div>
        <div class="account-meta">
          Gezählt seit {{ accountStartLabel }}<template v-if="account.opening_hours !== 0">
            , inklusive Startsaldo {{ formatHoursHM(account.opening_hours, { signed: true }) }}</template
          >
        </div>
      </template>
    </Card>

    <details class="explain">
      <summary>Wie wird gerechnet?</summary>
      <ul>
        <li><strong>Ist</strong>: gestempelte Arbeitszeit nach Pausenabzug plus Gutschriften für Urlaub, Krankheit und Sonstiges.</li>
        <li><strong>Soll</strong>: die vertraglichen Stunden für die Arbeitstage im Zeitraum. Feiertage und Schließtage zählen ohne Abzug.</li>
        <li><strong>Saldo</strong>: Ist minus Soll. Plus bedeutet Überstunden, Minus bedeutet Minusstunden.</li>
        <li><strong>Stand</strong>: Stundenkonto am Ende des Monats (bzw. gestern).</li>
      </ul>
    </details>

    <div class="toolbar">
      <Button icon="pi pi-chevron-left" text rounded severity="secondary" aria-label="Vorheriges Jahr" @click="year--" />
      <span class="year">{{ year }}</span>
      <Button
        icon="pi pi-chevron-right"
        text
        rounded
        severity="secondary"
        aria-label="Nächstes Jahr"
        :disabled="year >= currentYear"
        @click="year++"
      />
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <div v-if="loading" class="muted">Laden…</div>
    <p v-else-if="!countedMonths.length" class="muted">Für {{ year }} gibt es noch keine gezählten Tage.</p>
    <ul v-else class="months">
      <li v-for="b in countedMonths" :key="b.month" class="month" data-testid="balance-card">
        <div class="month-head">
          <span class="month-title">{{ monthTitle(b) }}</span>
          <span v-if="monthRange(b)" class="month-range">{{ monthRange(b) }}</span>
        </div>
        <dl class="month-stats">
          <div>
            <dt>Ist</dt>
            <dd data-testid="balance-worked">{{ formatHoursHM(b.worked_hours) }}</dd>
          </div>
          <div>
            <dt>Soll</dt>
            <dd>{{ formatHoursHM(b.target_hours) }}</dd>
          </div>
          <div>
            <dt>Saldo</dt>
            <dd data-testid="balance-month" :class="balanceClass(b.balance_hours)">
              {{ formatHoursHM(b.balance_hours, { signed: true }) }}
            </dd>
          </div>
          <div>
            <dt>Stand</dt>
            <dd :class="balanceClass(b.total_balance)">{{ formatHoursHM(b.total_balance, { signed: true }) }}</dd>
          </div>
        </dl>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 40rem;
}
.account-label {
  font-size: 0.85rem;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.account-value {
  font-size: 2.25rem;
  font-weight: 700;
  line-height: 1.2;
  margin: 0.2rem 0 0.35rem;
  font-variant-numeric: tabular-nums;
}
.account-meta {
  font-size: 0.85rem;
  color: #475569;
}
.explain {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.6rem 0.9rem;
  font-size: 0.9rem;
  color: #334155;
}
.explain summary {
  cursor: pointer;
  font-weight: 600;
}
.explain ul {
  margin: 0.5rem 0 0;
  padding-left: 1.1rem;
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}
.year {
  font-weight: 600;
  font-size: 1.05rem;
  min-width: 3.5rem;
  text-align: center;
}
.months {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}
.month {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 0.7rem 0.9rem;
}
.month-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 0.75rem;
  margin-bottom: 0.4rem;
}
.month-title {
  font-weight: 600;
}
.month-range {
  font-size: 0.8rem;
  color: #64748b;
}
.month-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0.5rem;
  margin: 0;
}
.month-stats dt {
  font-size: 0.72rem;
  color: #64748b;
}
.month-stats dd {
  margin: 0;
  font-weight: 600;
  font-size: 0.95rem;
  font-variant-numeric: tabular-nums;
}
.pos {
  color: #15803d;
}
.neg {
  color: #b91c1c;
}
.err {
  color: #b91c1c;
  margin: 0;
}
.muted {
  color: #64748b;
  margin: 0;
}
</style>
