<script setup lang="ts">
import { computed } from 'vue'

import { clockParts, durationParts } from '@/utils/schedulePlanning'

/**
 * Kompakte Zeit mit hochgestellten Minuten: Schicht „7³⁰–13³⁰“ (start/end) oder Dauer „28³⁰“ (minutes).
 */
const props = defineProps<{
  start?: string
  end?: string
  minutes?: number
}>()

const parts = computed(() => {
  if (props.minutes != null) return [durationParts(props.minutes)]
  return [clockParts(props.start ?? ''), clockParts(props.end ?? '')]
})
</script>

<template>
  <span class="ct"
    ><template v-for="(p, i) in parts" :key="i"
      ><template v-if="i > 0">–</template>{{ p.h }}<sup v-if="p.m">{{ p.m }}</sup></template
    ></span
  >
</template>

<style scoped>
.ct {
  white-space: nowrap;
}

.ct sup {
  font-size: 0.62em;
  line-height: 0;
  vertical-align: 0;
  position: relative;
  top: -0.55em;
  margin-left: 0.04em;
  letter-spacing: -0.01em;
}
</style>
