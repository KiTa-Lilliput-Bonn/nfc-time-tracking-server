<script setup lang="ts">
import Card from 'primevue/card'
import type { MonthBalance } from '@/types/api'

defineProps<{
  balance: MonthBalance
  title?: string
}>()

function fmt(n: number) {
  return `${n >= 0 ? '' : ''}${n.toFixed(2)} h`
}
</script>

<template>
  <Card class="balance-card" data-testid="balance-card">
    <template #title>{{ title ?? `${balance.month}/${balance.year}` }}</template>
    <template #content>
      <dl class="stats">
        <div>
          <dt>Ist</dt>
          <dd data-testid="balance-worked">{{ fmt(balance.worked_hours) }}</dd>
        </div>
        <div>
          <dt>Soll</dt>
          <dd>{{ fmt(balance.target_hours) }}</dd>
        </div>
        <div>
          <dt>Saldo Monat</dt>
          <dd
            data-testid="balance-month"
            :class="balance.balance_hours >= 0 ? 'pos' : 'neg'"
          >
            {{ fmt(balance.balance_hours) }}
          </dd>
        </div>
        <div>
          <dt>Vortrag</dt>
          <dd :class="balance.carryover >= 0 ? 'pos' : 'neg'">{{ fmt(balance.carryover) }}</dd>
        </div>
        <div>
          <dt>Gesamt</dt>
          <dd :class="balance.total_balance >= 0 ? 'pos' : 'neg'">{{ fmt(balance.total_balance) }}</dd>
        </div>
      </dl>
    </template>
  </Card>
</template>

<style scoped>
.balance-card {
  height: 100%;
}
.stats {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem 1rem;
  margin: 0;
}
.stats div {
  margin: 0;
}
dt {
  font-size: 0.75rem;
  color: #64748b;
  margin: 0 0 0.15rem;
}
dd {
  margin: 0;
  font-weight: 600;
  font-size: 1rem;
}
.pos {
  color: #15803d;
}
.neg {
  color: #b91c1c;
}
</style>
