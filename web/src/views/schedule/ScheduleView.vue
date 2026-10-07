<script setup lang="ts">
import { defineAsyncComponent, ref } from 'vue'
import Button from 'primevue/button'

import { useNarrowViewport } from '@/composables/useNarrowViewport'

const ScheduleEditorView = defineAsyncComponent(() => import('@/views/schedule/ScheduleEditorView.vue'))
const ScheduleMobileView = defineAsyncComponent(() => import('@/views/schedule/ScheduleMobileView.vue'))

/** Auf dem Handy die eigene Ansicht; die Tabelle bleibt über das Menü erreichbar (Wahl wird gemerkt). */
const PREFER_TABLE_KEY = 'nfc.schedule.preferTable'

const narrow = useNarrowViewport()
const preferTable = ref(readPreferTable())

function readPreferTable(): boolean {
  try {
    return localStorage.getItem(PREFER_TABLE_KEY) === '1'
  } catch {
    return false
  }
}

function setPreferTable(v: boolean) {
  preferTable.value = v
  try {
    if (v) localStorage.setItem(PREFER_TABLE_KEY, '1')
    else localStorage.removeItem(PREFER_TABLE_KEY)
  } catch {
    /* nur Komfort */
  }
}
</script>

<template>
  <ScheduleMobileView v-if="narrow && !preferTable" @open-table="setPreferTable(true)" />
  <template v-else>
    <div class="to-mobile">
      <Button
        v-if="narrow"
        label="Zur Handy-Ansicht"
        icon="pi pi-mobile"
        size="small"
        text
        data-testid="schedule-to-mobile"
        @click="setPreferTable(false)"
      />
      <RouterLink :to="{ name: 'schedule-basis' }" class="basis-link" data-testid="schedule-basis-link">
        <i class="pi pi-sliders-h" /> Planungsgrundlagen (Kinder, Qualifikation, KiBiz)
      </RouterLink>
    </div>
    <ScheduleEditorView />
  </template>
</template>

<style scoped>
.basis-link {
  font-size: 0.85rem;
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}
.to-mobile {
  margin: -0.5rem 0 0.5rem;
  display: flex;
  align-items: center;
}
</style>
