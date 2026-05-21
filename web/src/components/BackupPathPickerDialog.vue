<script setup lang="ts">
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import { useToast } from 'primevue/usetoast'

import { fetchBackupBrowse } from '@/api/admin'
import type { BackupBrowseResult } from '@/types/api'

const visible = defineModel<boolean>('visible', { default: false })

const props = defineProps<{
  initialPath?: string
}>()

const emit = defineEmits<{
  select: [path: string]
}>()

const toast = useToast()
const loading = ref(false)
const browse = ref<BackupBrowseResult | null>(null)

async function load(opts?: { path?: string; roots?: boolean }) {
  loading.value = true
  try {
    browse.value = await fetchBackupBrowse(opts)
  } catch (e: unknown) {
    const msg =
      e && typeof e === 'object' && 'response' in e
        ? String((e as { response?: { data?: { error?: string } } }).response?.data?.error ?? '')
        : ''
    toast.add({
      severity: 'error',
      summary: 'Verzeichnis',
      detail: msg || 'Ordnerliste konnte nicht geladen werden.',
      life: 10000,
    })
    browse.value = null
  } finally {
    loading.value = false
  }
}

async function openBrowse() {
  const start = props.initialPath?.trim()
  if (start) await load({ path: start })
  else await load()
}

watch(visible, (v) => {
  if (v) void openBrowse()
})

function goUp() {
  if (!browse.value) return
  if (!browse.value.parent) void load({ roots: true })
  else void load({ path: browse.value.parent })
}

function enterDir(path: string) {
  void load({ path })
}

function selectCurrent() {
  const p = browse.value?.path?.trim()
  if (!p) {
    toast.add({ severity: 'warn', summary: 'Verzeichnis', detail: 'Bitte einen Ordner auswählen.', life: 6000 })
    return
  }
  emit('select', p)
  visible.value = false
}

function selectDir(path: string) {
  emit('select', path)
  visible.value = false
}
</script>

<template>
  <Dialog
    v-model:visible="visible"
    modal
    header="Zielpfad wählen"
    :style="{ width: 'min(560px, 95vw)' }"
  >
    <p class="muted small">Ordner auf dem Server — Doppelklick öffnet, „Auswählen“ übernimmt den aktuellen Ordner.</p>
    <div v-if="loading" class="muted">Laden…</div>
    <template v-else-if="browse">
      <div class="path-bar mono">{{ browse.path || 'Laufwerke / Stamm' }}</div>
      <div class="toolbar">
        <Button
          label="Nach oben"
          icon="pi pi-arrow-up"
          severity="secondary"
          text
          size="small"
          :disabled="!browse.parent"
          @click="goUp"
        />
        <Button label="Diesen Ordner auswählen" icon="pi pi-check" size="small" :disabled="!browse.path" @click="selectCurrent" />
      </div>
      <ul v-if="browse.entries.length" class="dir-list">
        <li
          v-for="entry in browse.entries"
          :key="entry.path"
          class="dir-item"
          @dblclick="enterDir(entry.path)"
        >
          <span class="pi pi-folder dir-icon" aria-hidden="true" />
          <span class="dir-name">{{ entry.name }}</span>
          <Button icon="pi pi-check" text rounded size="small" title="Auswählen" @click="selectDir(entry.path)" />
        </li>
      </ul>
      <p v-else class="muted small">Keine Unterordner.</p>
    </template>
    <template #footer>
      <Button label="Abbrechen" severity="secondary" text @click="visible = false" />
    </template>
  </Dialog>
</template>

<style scoped>
.path-bar {
  padding: 0.5rem 0.65rem;
  background: var(--p-surface-100);
  border-radius: 6px;
  font-size: 0.85rem;
  word-break: break-all;
  margin-bottom: 0.5rem;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  margin-bottom: 0.75rem;
}
.dir-list {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: min(320px, 50vh);
  overflow-y: auto;
  border: 1px solid var(--p-surface-200);
  border-radius: 6px;
}
.dir-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35rem 0.5rem;
  cursor: default;
}
.dir-item:hover {
  background: var(--p-surface-50);
}
.dir-icon {
  color: var(--p-primary-color);
}
.dir-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.muted {
  color: var(--p-text-muted-color);
}
.small {
  font-size: 0.875rem;
}
.mono {
  font-family: ui-monospace, monospace;
}
</style>
