<script setup lang="ts">
import { computed } from 'vue'

import CompactTime from '@/components/schedule/CompactTime.vue'
import type { KibizTotals } from '@/types/api'
import { KIBIZ_TOLERANCE_MIN, kibizShortfall } from '@/utils/kibiz'

/** KiBiz-Zeile einer Gruppe: Kinder (antippbar), Fachkraftstunden und Personalstunden insgesamt, geplant / nötig. */
const props = defineProps<{
  totals: KibizTotals
  childrenTotal: number
  /** z. B. „18–20“ wenn die Tage der Woche unterschiedlich sind */
  childrenLabel?: string
  adjusted: boolean
  hasPattern: boolean
  testid: string
}>()

const emit = defineEmits<{ children: [] }>()

const short = computed(() => kibizShortfall(props.totals))
const fkShort = computed(() => short.value.fachkraft >= KIBIZ_TOLERANCE_MIN)
const totalShort = computed(() => short.value.total >= KIBIZ_TOLERANCE_MIN)
const planned = computed(() => props.totals.planned_fachkraft_min + props.totals.planned_ergaenzung_min)
</script>

<template>
  <div class="kb" :data-testid="testid">
    <button
      type="button"
      class="kids"
      :class="{ adj: adjusted, none: !hasPattern && !childrenTotal }"
      :data-testid="testid + '-children'"
      @click="emit('children')"
    >
      <i class="pi pi-users" aria-hidden="true" />
      <template v-if="!hasPattern && !childrenTotal">Kinder eintragen</template>
      <template v-else>{{ childrenLabel || childrenTotal }} Kinder<template v-if="adjusted"> ✎</template></template>
    </button>
    <template v-if="totals.need_total_min > 0">
      <span class="v" :class="{ short: fkShort }" :data-testid="testid + '-fk'">
        <template v-if="fkShort">⚠ </template>Fachkraft
        <b><CompactTime :minutes="totals.planned_fachkraft_min" /></b> /
        <CompactTime :minutes="totals.need_fachkraft_min" />&nbsp;h
      </span>
      <span class="v" :class="{ short: totalShort }" :data-testid="testid + '-total'">
        <template v-if="totalShort">⚠ </template>Gesamt
        <b><CompactTime :minutes="planned" /></b> /
        <CompactTime :minutes="totals.need_total_min" />&nbsp;h
      </span>
    </template>
  </div>
</template>

<style scoped>
.kb {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem 0.7rem;
  font-size: 0.8rem;
  color: #475569;
  margin: 0.15rem 0 0.45rem;
  font-variant-numeric: tabular-nums;
}
.kids {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 999px;
  padding: 0.2rem 0.6rem;
  font: inherit;
  color: #1e293b;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
}
.kids.adj {
  border-color: #f59e0b;
  background: #fffbeb;
}
.kids.none {
  border-style: dashed;
  color: #64748b;
}
.kids .pi {
  font-size: 0.75rem;
}
.v b {
  color: #0f172a;
}
.v.short {
  color: #b45309;
  font-weight: 600;
}
.v.short b {
  color: #b45309;
}
</style>
