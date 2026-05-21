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
  createHoliday,
  deleteHoliday,
  fetchHolidays,
  generateHolidaysForYear,
} from '@/api/admin'
import type { Holiday } from '@/types/api'
import { formatGermanDate } from '@/utils/dates'

const toast = useToast()

const year = ref(new Date().getFullYear())
const rows = ref<Holiday[]>([])
const loading = ref(false)

const sorted = computed(() =>
  [...rows.value].sort((a, b) => a.holiday_date.localeCompare(b.holiday_date)),
)

async function load() {
  loading.value = true
  try {
    rows.value = await fetchHolidays(year.value)
  } catch {
    toast.add({ severity: 'error', summary: 'Feiertage', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(year, load)

const generating = ref(false)

async function generate() {
  if (!confirm(`NRW-Feiertage für ${year.value} generieren? Bestehende automatische Einträge des Jahres werden ersetzt.`)) {
    return
  }
  generating.value = true
  try {
    await generateHolidaysForYear(year.value)
    toast.add({ severity: 'success', summary: 'Feiertage generiert', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Generieren fehlgeschlagen', life: 10000 })
  } finally {
    generating.value = false
  }
}

const showAdd = ref(false)
const addDate = ref<Date>(new Date())
const addName = ref('')
const saving = ref(false)

function openAdd() {
  addDate.value = new Date(year.value, 0, 1)
  addName.value = ''
  showAdd.value = true
}

async function submitAdd() {
  if (!addName.value.trim()) return
  saving.value = true
  try {
    const y = addDate.value.getFullYear()
    const m = String(addDate.value.getMonth() + 1).padStart(2, '0')
    const d = String(addDate.value.getDate()).padStart(2, '0')
    await createHoliday({ holiday_date: `${y}-${m}-${d}`, name: addName.value.trim() })
    toast.add({ severity: 'success', summary: 'Gespeichert', life: 10000 })
    showAdd.value = false
    const prevY = year.value
    year.value = y
    if (y === prevY) await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

async function remove(h: Holiday) {
  if (!confirm(`Feiertag „${h.name}“ am ${formatGermanDate(h.holiday_date)} löschen?`)) return
  try {
    await deleteHoliday(h.id)
    toast.add({ severity: 'success', summary: 'Gelöscht', life: 10000 })
    await load()
  } catch {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', life: 10000 })
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Feiertage (Admin)</template>
      <template #content>
        <div class="toolbar">
          <label class="lbl">Jahr</label>
          <InputNumber v-model="year" :min="2000" :max="2100" :use-grouping="false" show-buttons />
          <Button
            label="NRW generieren"
            icon="pi pi-refresh"
            severity="secondary"
            outlined
            :loading="generating"
            @click="generate"
          />
          <Button label="Feiertag hinzufügen" icon="pi pi-plus" @click="openAdd" />
        </div>
        <DataTable :value="sorted" :loading="loading" data-key="id" striped-rows>
          <Column field="holiday_date" header="Datum" sortable>
            <template #body="{ data }">{{ formatGermanDate(data.holiday_date) }}</template>
          </Column>
          <Column field="name" header="Name" sortable />
          <Column header="Herkunft">
            <template #body="{ data }">
              <Tag
                :severity="data.auto_generated ? 'info' : 'secondary'"
                :value="data.auto_generated ? 'Automatisch' : 'Manuell'"
              />
            </template>
          </Column>
          <Column header="">
            <template #body="{ data }">
              <Button icon="pi pi-trash" severity="danger" text rounded @click="remove(data)" />
            </template>
          </Column>
        </DataTable>
      </template>
    </Card>

    <Dialog v-model:visible="showAdd" header="Feiertag" modal :style="{ width: '400px' }">
      <div class="form">
        <label>Datum</label>
        <DatePicker v-model="addDate" date-format="dd.mm.yy" show-icon class="w" />
        <label>Name</label>
        <InputText v-model="addName" class="w" />
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
  max-width: 800px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  align-items: center;
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
.w {
  width: 100%;
}
</style>
