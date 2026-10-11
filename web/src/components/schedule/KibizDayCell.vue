<script setup lang="ts">
import { computed } from 'vue'

import CompactTime from '@/components/schedule/CompactTime.vue'
import type { KibizDay } from '@/types/api'
import { KIBIZ_TOLERANCE_MIN, kibizShortfall } from '@/utils/kibiz'

/** KiBiz-Werte eines Tages in der Wochentabelle (Desktop): Kinder (antippbar), Fachkraft und Gesamt. */
const props = defineProps<{
  day: KibizDay
  hasPattern: boolean
  testid: string
}>()

const emit = defineEmits<{ children: [] }>()

const short = computed(() => kibizShortfall(props.day))
const fkShort = computed(() => short.value.fachkraft >= KIBIZ_TOLERANCE_MIN)
const totalShort = computed(() => short.value.total >= KIBIZ_TOLERANCE_MIN)
const planned = computed(() => props.day.planned_fachkraft_min + props.day.planned_ergaenzung_min)
</script>

<template>
  <div class="kd" :data-testid="testid">
    <span v-if="!day.open" class="closed">geschlossen</span>
    <template v-else>
      <button
        type="button"
        class="kids"
        :class="{ adj: day.adjusted, none: !hasPattern && !day.children_total }"
        :data-testid="testid + '-children'"
        :title="day.adjusted ? 'Weicht vom Wochenmuster ab' : 'Kinderzahl für diesen Tag anpassen'"
        @click="emit('children')"
      >
        <i class="pi pi-users" aria-hidden="true" />
        <template v-if="!hasPattern && !day.children_total">eintragen</template>
        <template v-else>{{ day.children_total }}<template v-if="day.adjusted"> ✎</template></template>
      </button>
      <template v-if="day.need_total_min > 0">
        <span class="v" :class="{ short: fkShort }" :data-testid="testid + '-fk'" title="Fachkraftstunden geplant / nötig">
          <template v-if="fkShort">⚠ </template>FK <b><CompactTime :minutes="day.planned_fachkraft_min" /></b> /
          <CompactTime :minutes="day.need_fachkraft_min" />
        </span>
        <span class="v" :class="{ short: totalShort }" :data-testid="testid + '-total'" title="Personalstunden geplant / nötig">
          <template v-if="totalShort">⚠ </template>Ges. <b><CompactTime :minutes="planned" /></b> /
          <CompactTime :minutes="day.need_total_min" />
        </span>
      </template>
    </template>
  </div>
</template>

<style scoped>
.kd {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 0.15rem;
  font-size: 0.78rem;
  color: #475569;
  font-variant-numeric: tabular-nums;
}
.closed {
  color: #94a3b8;
}
.kids {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
  font: inherit;
  color: #1e293b;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
}
.kids:hover {
  border-color: #94a3b8;
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
  font-size: 0.7rem;
}
.v {
  white-space: nowrap;
}
.v b {
  color: #0f172a;
}
.v.short,
.v.short b {
  color: #b45309;
  font-weight: 600;
}
</style>
