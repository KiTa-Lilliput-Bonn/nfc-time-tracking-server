<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import InputNumber from 'primevue/inputnumber'

import BalanceCard from '@/components/BalanceCard.vue'
import { fetchMeBalance } from '@/api/me'
import type { MonthBalance } from '@/types/api'

const year = ref(new Date().getFullYear())
const balances = ref<MonthBalance[]>([])
const loading = ref(false)
const err = ref('')

const monthNames = [
  'Jan',
  'Feb',
  'Mär',
  'Apr',
  'Mai',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Okt',
  'Nov',
  'Dez',
]

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

const yearly = computed(() => {
  let w = 0
  let t = 0
  for (const b of balances.value) {
    w += b.worked_hours
    t += b.target_hours
  }
  const bal = w - t
  return { worked: w, target: t, balance: bal }
})

onMounted(load)
watch(year, load)
</script>

<template>
  <div class="page">
    <div class="toolbar">
      <label class="lbl">Jahr</label>
      <InputNumber v-model="year" :min="2000" :max="2100" :use-grouping="false" show-buttons />
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <div v-if="loading" class="muted">Laden…</div>
    <div v-else class="year-summary">
      <h3>Jahresübersicht {{ year }}</h3>
      <p>
        Ist: <strong>{{ yearly.worked.toFixed(2) }} h</strong> · Soll:
        <strong>{{ yearly.target.toFixed(2) }} h</strong> · Saldo:
        <strong :class="yearly.balance >= 0 ? 'pos' : 'neg'">{{ yearly.balance.toFixed(2) }} h</strong>
      </p>
    </div>
    <div class="grid">
      <BalanceCard
        v-for="(b, i) in balances"
        :key="i"
        :balance="b"
        :title="`${monthNames[i]} ${year}`"
      />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.lbl {
  font-size: 0.9rem;
  color: #64748b;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
}
.year-summary {
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 1rem 1.25rem;
}
.year-summary h3 {
  margin: 0 0 0.5rem;
  font-size: 1.1rem;
}
.year-summary p {
  margin: 0;
  color: #334155;
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
}
</style>
