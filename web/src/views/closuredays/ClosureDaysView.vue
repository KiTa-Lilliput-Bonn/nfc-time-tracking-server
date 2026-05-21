<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import DatePicker from 'primevue/datepicker'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import {
  createClosureDay,
  deleteClosureDay,
  fetchClosureDays,
  fetchHolidays,
} from '@/api/management'
import type { ClosureDay, Holiday } from '@/types/api'
import { formatGermanDate, toISODateLocal } from '@/utils/dates'

const toast = useToast()

const year = ref(new Date().getFullYear())
const closures = ref<ClosureDay[]>([])
const holidays = ref<Holiday[]>([])
const loading = ref(false)

function normalizeISODate(s: string): string {
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})/)
  return m ? `${m[1]}-${m[2]}-${m[3]}` : String(s)
}
function parseISODateSafe(s: string): Date {
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})/)
  if (!m) return new Date(NaN)
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
}

interface UnifiedRow {
  kind: 'holiday' | 'closure'
  rowKey: string
  date: string
  dateTo?: string
  name: string
  closureIds?: number[]
}

const rows = computed<UnifiedRow[]>(() => {
  const y = `${year.value}-`
  const out: UnifiedRow[] = []
  const holidaySet = new Set(holidays.value.map((h) => normalizeISODate(h.holiday_date)))
  for (const h of holidays.value) {
    const iso = normalizeISODate(h.holiday_date)
    out.push({
      kind: 'holiday',
      rowKey: `h-${h.id}`,
      date: iso,
      name: h.name,
    })
  }

  const list = closures.value
    .filter((c) => c.closure_date.startsWith(y))
    .sort((a, b) => a.closure_date.localeCompare(b.closure_date) || a.id - b.id)

  function expectedNextWorkdayISO(iso: string): string {
    // Nächster Werktag nach iso; Feiertage gelten als "Lücke" und werden übersprungen,
    // damit ein Zeitraum trotzdem als zusammenhängend angezeigt bleibt.
    const d = new Date(`${iso}T00:00:00`)
    while (true) {
      const wd = d.getDay() // 0=So..6=Sa
      const add = wd === 5 ? 3 : 1 // Fr -> Mo, sonst +1
      d.setDate(d.getDate() + add)
      const nextISO = toISODateLocal(d)
      const nwd = d.getDay()
      if (nwd === 0 || nwd === 6) continue
      if (holidaySet.has(nextISO)) continue
      return nextISO
    }
  }

  // Schließtage zu Zeiträumen zusammenfassen:
  // - gleicher Name
  // - nur Werktage (Mo–Fr)
  // - zusammenhängend, Wochenende darf "übersprungen" werden (Fr -> Mo)
  // - Feiertage unterbrechen immer
  let curName = ''
  let curFrom = ''
  let curTo = ''
  let curIds: number[] = []
  let curLen = 0

  function flush() {
    if (!curLen) return
    out.push({
      kind: 'closure',
      rowKey: curLen === 1 ? `c-${curIds[0]}` : `cr-${curFrom}-${curTo}-${curName}`,
      date: curFrom,
      dateTo: curLen === 1 ? undefined : curTo,
      name: curName,
      closureIds: [...curIds],
    })
    curName = ''
    curFrom = ''
    curTo = ''
    curIds = []
    curLen = 0
  }

  for (const c of list) {
    const iso = normalizeISODate(c.closure_date)
    // Schließtag auf Feiertag: soll nicht existieren — nicht anzeigen und Zeitraum nicht unterbrechen.
    if (holidaySet.has(iso)) continue
    // Nur Werktage in Zeiträumen
    const d = new Date(`${iso}T00:00:00`)
    const wd = d.getDay()
    const isWeekend = wd === 0 || wd === 6
    if (isWeekend) {
      flush()
      out.push({
        kind: 'closure',
        rowKey: `c-${c.id}`,
        date: iso,
        name: c.name,
        closureIds: [c.id],
      })
      continue
    }

    if (!curLen) {
      curName = c.name
      curFrom = iso
      curTo = iso
      curIds = [c.id]
      curLen = 1
      continue
    }

    const expected = expectedNextWorkdayISO(curTo)
    const canJoin = c.name === curName && iso === expected
    if (canJoin) {
      curTo = iso
      curIds.push(c.id)
      curLen++
      continue
    }
    flush()
    curName = c.name
    curFrom = iso
    curTo = iso
    curIds = [c.id]
    curLen = 1
  }
  flush()

  out.sort((a, b) => a.date.localeCompare(b.date) || a.name.localeCompare(b.name, 'de'))
  return out
})

async function loadClosures() {
  closures.value = await fetchClosureDays()
}

async function loadHolidays() {
  holidays.value = await fetchHolidays(year.value)
}

async function load() {
  loading.value = true
  try {
    await Promise.all([loadClosures(), loadHolidays()])
  } catch {
    toast.add({ severity: 'error', summary: 'Schließtage', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(load)

watch(year, async () => {
  loading.value = true
  try {
    await loadHolidays()
  } catch {
    toast.add({ severity: 'error', summary: 'Feiertage', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
})

const showAdd = ref(false)
const addFrom = ref<Date>(new Date())
const addTo = ref<Date>(new Date())
const addName = ref('')
const saving = ref(false)
const editingClosureIds = ref<number[] | null>(null)

watch(addFrom, (v) => {
  const a = new Date(v.getFullYear(), v.getMonth(), v.getDate())
  const b = new Date(addTo.value.getFullYear(), addTo.value.getMonth(), addTo.value.getDate())
  if (b.getTime() < a.getTime()) {
    addTo.value = new Date(a)
  }
})

function openAdd() {
  const d = new Date(year.value, 0, 1)
  addFrom.value = new Date(d)
  addTo.value = new Date(d)
  addName.value = ''
  editingClosureIds.value = null
  showAdd.value = true
}

function openEdit(row: UnifiedRow) {
  if (row.kind !== 'closure') return
  if (!row.closureIds?.length) return
  addFrom.value = parseISODateSafe(row.date)
  addTo.value = parseISODateSafe(row.dateTo ?? row.date)
  addName.value = row.name
  editingClosureIds.value = [...row.closureIds]
  showAdd.value = true
}

function normalizeAddRange(): [Date, Date] {
  const a = new Date(addFrom.value.getFullYear(), addFrom.value.getMonth(), addFrom.value.getDate())
  const b = new Date(addTo.value.getFullYear(), addTo.value.getMonth(), addTo.value.getDate())
  if (a.getTime() <= b.getTime()) return [a, b]
  return [b, a]
}

function enumerateWeekdayISO(from: Date, to: Date): string[] {
  const out: string[] = []
  let cur = new Date(from.getFullYear(), from.getMonth(), from.getDate())
  const end = new Date(to.getFullYear(), to.getMonth(), to.getDate())
  while (cur.getTime() <= end.getTime()) {
    const wd = cur.getDay() // 0=So, 6=Sa
    if (wd !== 0 && wd !== 6) out.push(toISODateLocal(cur))
    cur.setDate(cur.getDate() + 1)
  }
  return out
}

async function holidayDateSetForRange(from: Date, to: Date): Promise<Set<string>> {
  const minY = Math.min(from.getFullYear(), to.getFullYear())
  const maxY = Math.max(from.getFullYear(), to.getFullYear())
  const set = new Set<string>()
  for (let y = minY; y <= maxY; y++) {
    // Falls Feiertage noch nicht geladen sind, immer vom Server holen.
    const list = y === year.value && holidays.value.length ? holidays.value : await fetchHolidays(y)
    for (const h of list) set.add(normalizeISODate(h.holiday_date))
  }
  return set
}

async function submitAdd() {
  if (!addName.value.trim()) return
  saving.value = true
  try {
    const name = addName.value.trim()
    const [a, b] = normalizeAddRange()
    const holidaySet = await holidayDateSetForRange(a, b)
    // Werktage im Zeitraum, aber Feiertage werden nicht als Schließtag eingetragen.
    const days = enumerateWeekdayISO(a, b).filter((iso) => !holidaySet.has(normalizeISODate(iso)))
    const prevIds = editingClosureIds.value
    if (prevIds?.length) {
      for (const id of prevIds) {
        try {
          await deleteClosureDay(id)
        } catch {
          /* ignore */
        }
      }
    }
    let ok = 0
    let skipped = 0
    for (const iso of days) {
      try {
        await createClosureDay({ closure_date: iso, name })
        ok++
      } catch {
        skipped++
      }
    }

    if (ok > 0) {
      toast.add({
        severity: 'success',
        summary: ok === 1 ? 'Gespeichert' : `${ok} Schließtage gespeichert`,
        detail: skipped ? `${skipped} übersprungen (bereits vorhanden).` : undefined,
        life: 10000,
      })
    } else {
      toast.add({
        severity: 'warn',
        summary: 'Nichts gespeichert',
        detail: 'Alle Werktage im Zeitraum waren bereits als Schließtag vorhanden.',
        life: 10000,
      })
    }
    showAdd.value = false
    const y = a.getFullYear()
    year.value = y
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function remove(row: UnifiedRow) {
  if (row.kind !== 'closure' || !row.closureIds?.length) return
  const label =
    row.dateTo && row.dateTo !== row.date
      ? `Schließzeitraum „${row.name}“ (${formatGermanDate(row.date)} – ${formatGermanDate(row.dateTo)})`
      : `Schließtag „${row.name}“ (${formatGermanDate(row.date)})`
  if (!confirm(`${label} löschen?`)) return
  try {
    for (const id of row.closureIds) {
      await deleteClosureDay(id)
    }
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await loadClosures()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Schließtage &amp; Feiertage</template>
      <template #content>
        <p class="hint">
          Feiertage (NRW usw.) sind hier eingebunden und werden von der Zeitauswertung berücksichtigt. Bearbeiten
          kann sie nur die Administration; Schließtage (z.&nbsp;B. Betriebsferien) trägst du hier ein.
        </p>
        <div class="toolbar">
          <label class="lbl">Jahr</label>
          <InputNumber v-model="year" :min="2000" :max="2100" :use-grouping="false" show-buttons />
          <Button label="Schließtag hinzufügen" icon="pi pi-plus" @click="openAdd" />
        </div>
        <DataTable
          :value="rows"
          :loading="loading"
          :data-key="'rowKey'"
          striped-rows
          sort-field="date"
          :sort-order="1"
        >
          <Column field="date" header="Datum" sortable>
            <template #body="{ data }: { data: UnifiedRow }">
              <span v-if="data.dateTo">{{ formatGermanDate(data.date) }} – {{ formatGermanDate(data.dateTo) }}</span>
              <span v-else>{{ formatGermanDate(data.date) }}</span>
            </template>
          </Column>
          <Column header="Art" sortable :sort-field="'kind'">
            <template #body="{ data }: { data: UnifiedRow }">
              <Tag v-if="data.kind === 'holiday'" severity="info" value="Feiertag" />
              <Tag v-else severity="secondary" value="Schließtag" />
            </template>
          </Column>
          <Column field="name" header="Bezeichnung" sortable />
          <Column header="">
            <template #body="{ data }: { data: UnifiedRow }">
              <Button
                v-if="data.kind === 'closure' && data.closureIds?.length"
                icon="pi pi-pencil"
                text
                rounded
                aria-label="Bearbeiten"
                @click="openEdit(data)"
              />
              <Button
                v-if="data.kind === 'closure' && data.closureIds?.length"
                icon="pi pi-trash"
                severity="danger"
                text
                rounded
                aria-label="Löschen"
                @click="remove(data)"
              />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Dialog v-model:visible="showAdd" :header="editingClosureIds ? 'Schließzeitraum bearbeiten' : 'Schließzeitraum'" modal :style="{ width: '400px' }">
      <div class="form">
        <label>Von</label>
        <DatePicker v-model="addFrom" date-format="dd.mm.yy" show-icon class="w" />
        <label>Bis</label>
        <DatePicker v-model="addTo" date-format="dd.mm.yy" show-icon class="w" :min-date="addFrom" />
        <p class="hint small-hint">Es werden nur Werktage (Mo–Fr) als Schließtage eingetragen.</p>
        <label>Name</label>
        <InputText v-model="addName" class="w" placeholder="z. B. Betriebsferien" />
      </div>
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="showAdd = false" />
        <Button label="Speichern" :loading="saving" :disabled="!addName.trim()" @click="submitAdd" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 720px;
}
.hint {
  margin: 0 0 1rem;
  font-size: 0.85rem;
  color: #64748b;
  line-height: 1.45;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.75rem 1rem;
  margin-bottom: 1rem;
}
.lbl {
  font-size: 0.85rem;
  color: #64748b;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.form label {
  font-size: 0.8rem;
  color: #64748b;
}
.small-hint {
  margin: 0.15rem 0 0.35rem;
  font-size: 0.8rem;
  color: #64748b;
  line-height: 1.35;
}
.w {
  width: 100%;
}
</style>
