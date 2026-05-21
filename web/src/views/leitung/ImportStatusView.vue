<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import { fetchAndroidLanHealthStatus, postAndroidLanSyncStampsRange } from '@/api/management'
import type { AndroidLanHealthStatus } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { addDays, parseISODate, toISODateLocal } from '@/utils/dates'

const toast = useToast()

const health = ref<AndroidLanHealthStatus | null>(null)
let pollTimer: number | undefined

const today = new Date()
const fromDate = ref(toISODateLocal(addDays(today, -7)))
const toDate = ref(toISODateLocal(today))
const syncBusy = ref(false)

const inclusiveDayCount = computed(() => {
  const a = parseISODate(fromDate.value)
  const b = parseISODate(toDate.value)
  if (Number.isNaN(a.getTime()) || Number.isNaN(b.getTime())) return 0
  return Math.floor((b.getTime() - a.getTime()) / 86400000) + 1
})

const aggSummary = computed(() => {
  const h = health.value
  if (!h) return '—'
  if (h.mode === 'disabled') return 'LAN-Import ist deaktiviert (kein Intervall oder keine Ziele).'
  if (h.mode === 'ok') return 'Alle erreichbaren Ziele melden OK.'
  return h.last_error?.trim() || 'Mindestens ein Ziel ist nicht erreichbar.'
})

async function pollHealthOnce() {
  try {
    health.value = await fetchAndroidLanHealthStatus()
  } catch {
    health.value = {
      mode: 'down',
      reachable: false,
      last_error: 'Status konnte nicht geladen werden.',
    }
  }
}

function scheduleNextPoll() {
  if (pollTimer !== undefined) {
    clearTimeout(pollTimer)
    pollTimer = undefined
  }
  void pollHealthOnce().finally(() => {
    const h = health.value
    let delayMs = 45_000
    if (h?.mode === 'down') delayMs = 10_000
    else if (h?.mode === 'disabled') delayMs = 60_000
    pollTimer = window.setTimeout(scheduleNextPoll, delayMs)
  })
}

onMounted(() => {
  scheduleNextPoll()
})

onUnmounted(() => {
  if (pollTimer !== undefined) {
    clearTimeout(pollTimer)
    pollTimer = undefined
  }
})

function severityForMode(m: string): 'success' | 'warn' | 'danger' | 'secondary' {
  if (m === 'ok') return 'success'
  if (m === 'disabled') return 'secondary'
  return 'danger'
}

async function onSync() {
  const n = inclusiveDayCount.value
  if (n <= 0) {
    toast.add({
      severity: 'warn',
      summary: 'Ungültiger Zeitraum',
      detail: '„Von“ muss vor oder gleich „Bis“ liegen.',
      life: 6000,
    })
    return
  }
  if (n > 14) {
    toast.add({
      severity: 'warn',
      summary: 'Zeitraum zu groß',
      detail: 'Es sind höchstens 14 Kalendertage (inklusive Start- und Endtag) erlaubt.',
      life: 8000,
    })
    return
  }
  syncBusy.value = true
  try {
    const res = await postAndroidLanSyncStampsRange({ from: fromDate.value, to: toDate.value })
    const failedPulls = res.targets.filter((t) => t.pull_error).length
    const detail =
      failedPulls > 0
        ? `Bei ${failedPulls} ${failedPulls === 1 ? 'Gerät' : 'Geräten'} gab es eine Rückmeldung — siehe die Tabelle oben.`
        : 'Alle beteiligten Geräte wurden für den gewählten Zeitraum durchlaufen.'
    toast.add({
      severity: failedPulls > 0 ? 'warn' : 'success',
      summary: 'Synchronisation abgeschlossen',
      detail,
      life: 10_000,
    })
    await pollHealthOnce()
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Synchronisation fehlgeschlagen',
      detail: getApiErrorMessage(e),
      life: 12_000,
    })
  } finally {
    syncBusy.value = false
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Importstatus (LAN / Stempel)</template>
      <template #subtitle>
        Erreichbarkeit der konfigurierten Android-Geräte. Die automatische Aktualisierung läuft weiter im
        Hintergrund; diese Seite fragt den Status nur hier ab.
      </template>
      <template #content>
        <p v-if="health" class="agg-line">
          <Tag :severity="severityForMode(health.mode)" :value="health.mode.toUpperCase()" class="agg-tag" />
          <span>{{ aggSummary }}</span>
        </p>
        <p v-else class="muted">Lade Status …</p>
        <div class="toolbar">
          <Button type="button" label="Jetzt aktualisieren" severity="secondary" @click="pollHealthOnce" />
        </div>
        <DataTable
          v-if="health?.targets?.length"
          :value="health.targets"
          data-key="id"
          striped-rows
          size="small"
          class="mt-table"
        >
          <Column field="label" header="Bezeichnung">
            <template #body="{ data }">
              {{ data.label || data.id }}
            </template>
          </Column>
          <Column field="id" header="Ziel-ID" />
          <Column field="mode" header="Modus" />
          <Column field="reachable" header="Erreichbar">
            <template #body="{ data }">
              {{ data.reachable ? 'Ja' : 'Nein' }}
            </template>
          </Column>
          <Column field="last_error" header="Letzte Meldung">
            <template #body="{ data }">
              <span class="err-cell">{{ data.last_error || '—' }}</span>
            </template>
          </Column>
          <Column field="last_check_utc" header="Letzte Prüfung (UTC)">
            <template #body="{ data }">
              {{ data.last_check_utc || '—' }}
            </template>
          </Column>
        </DataTable>
        <p v-else-if="health && !health.targets?.length" class="muted">Keine LAN-Ziele konfiguriert.</p>
      </template>
    </Card>

    <Card class="card-spaced">
      <template #title>Manueller Zeitraum-Abgleich</template>
      <template #subtitle>
        Wählen Sie den <strong>ersten</strong> und <strong>letzten Kalendertag</strong> des Zeitraums, den Sie nachholen
        möchten. Dann holt der Server die Stempelungen von den Stempel-Terminals und gleicht sie mit der
        Zeiterfassung ab; fehlende Einträge werden dabei übernommen. Auf die Geräte wird der gleiche Zeitraum
        zurückgespielt, damit überall derselbe Stand ist. Es gehen höchstens <strong>14 Tage</strong> auf einmal
        (erster und letzter Tag zählen mit). Beteiligt sind nur die Terminals, die bereits für den
        automatischen Abruf eingerichtet sind — der <strong>tägliche automatische Abruf</strong> läuft normal weiter
        und wird dadurch <strong>nicht zurückgesetzt</strong>.
      </template>
      <template #content>
        <div class="range-row">
          <label class="field">
            <span>Von</span>
            <input v-model="fromDate" type="date" class="date-inp" />
          </label>
          <label class="field">
            <span>Bis</span>
            <input v-model="toDate" type="date" class="date-inp" />
          </label>
          <Button
            type="button"
            label="Synchronisieren"
            icon="pi pi-sync"
            :loading="syncBusy"
            :disabled="syncBusy"
            @click="onSync"
          />
        </div>
        <p class="hint">Aktuell {{ inclusiveDayCount }} Tag(e) — höchstens 14 möglich.</p>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.page {
  max-width: 960px;
}
.card-spaced {
  margin-top: 1.25rem;
}
.agg-line {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin: 0 0 0.75rem;
}
.agg-tag {
  text-transform: uppercase;
}
.toolbar {
  margin-bottom: 0.5rem;
}
.mt-table {
  margin-top: 0.5rem;
}
.muted {
  color: #64748b;
  margin: 0 0 0.75rem;
}
.range-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 1rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.85rem;
  color: #475569;
}
.date-inp {
  padding: 0.4rem 0.5rem;
  border: 1px solid #cbd5e1;
  border-radius: 6px;
  font-size: 0.95rem;
}
.err-cell {
  color: #b91c1c;
  font-size: 0.85rem;
}
.hint {
  margin: 0.75rem 0 0;
  font-size: 0.85rem;
  color: #64748b;
}
</style>
