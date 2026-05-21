<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { fetchEmployees } from '@/api/employees'
import {
  fetchExportCsvBlob,
  fetchExportCsvText,
  fetchExportPdfBlob,
  triggerDownload,
} from '@/api/exportReport'
import type { Employee } from '@/types/api'
import { parseSemicolonCsv } from '@/utils/csvParse'
import { formatGermanDate, toISODateLocal } from '@/utils/dates'
import { focusSelectFilterOnShow } from '@/utils/selectFilterFocus'

const toast = useToast()

const employees = ref<Employee[]>([])
const employeeId = ref<number | null>(null)
const month = ref(new Date().getMonth() + 1)
const year = ref(new Date().getFullYear())

const allOption = { label: 'Alle Mitarbeiter (nur CSV, nacheinander)', value: -1 as number }

const employeeOptions = computed(() => [
  allOption,
  ...employees.value.map((e) => ({ label: e.display_name, value: e.id })),
])

const fromTo = computed(() => {
  const first = new Date(year.value, month.value - 1, 1)
  const last = new Date(year.value, month.value, 0)
  return { from: toISODateLocal(first), to: toISODateLocal(last) }
})

const previewHeaders = [
  'Datum',
  'WT',
  'Schicht',
  'Ende',
  'Stempel',
  'Brutto',
  'Netto',
  'Soll',
  'Saldo',
  'Hinweise',
]

const previewRows = ref<string[][]>([])
const previewLoading = ref(false)

const previewData = computed(() =>
  previewRows.value.map((cells, i) => {
    const row: Record<string, string> = { _i: String(i) }
    for (let j = 0; j < previewHeaders.length; j++) {
      const h = previewHeaders[j]!
      let val = cells[j] ?? ''
      if (h === 'Datum' && /^\d{4}-\d{2}-\d{2}/.test(val)) {
        val = formatGermanDate(val)
      }
      row[h] = val
    }
    return row
  }),
)

onMounted(async () => {
  employees.value = await fetchEmployees()
  employeeId.value = employees.value[0]?.id ?? null
})

async function loadPreview() {
  const id = employeeId.value
  if (id == null || id < 0) {
    previewRows.value = []
    toast.add({
      severity: 'info',
      summary: 'Vorschau',
      detail: 'Bitte einen einzelnen Mitarbeiter wählen.',
      life: 10000,
    })
    return
  }
  previewLoading.value = true
  previewRows.value = []
  try {
    const text = await fetchExportCsvText(id, fromTo.value.from, fromTo.value.to)
    const grid = parseSemicolonCsv(text)
    if (grid.length <= 1) {
      previewRows.value = []
      return
    }
    previewRows.value = grid.slice(1)
  } catch {
    toast.add({ severity: 'error', summary: 'Vorschau fehlgeschlagen', life: 10000 })
  } finally {
    previewLoading.value = false
  }
}

async function downloadCsv() {
  const id = employeeId.value
  if (id == null) return
  const { from, to } = fromTo.value
  try {
    if (id < 0) {
      const list = employees.value.filter((e) => e.role !== 'superadmin')
      if (!list.length) {
        toast.add({ severity: 'warn', summary: 'Keine Mitarbeiter', life: 10000 })
        return
      }
      for (const e of list) {
        const blob = await fetchExportCsvBlob(e.id, from, to)
        triggerDownload(blob, `export-${e.username}-${from}-${to}.csv`)
      }
      toast.add({
        severity: 'success',
        summary: 'CSV',
        detail: `${list.length} Datei(en) heruntergeladen.`,
        life: 10000,
      })
      return
    }
    const emp = employees.value.find((x) => x.id === id)
    const blob = await fetchExportCsvBlob(id, from, to)
    triggerDownload(blob, `export-${emp?.username ?? id}-${from}-${to}.csv`)
  } catch {
    toast.add({ severity: 'error', summary: 'CSV-Export fehlgeschlagen', life: 10000 })
  }
}

async function downloadPdf() {
  const id = employeeId.value
  if (id == null || id < 0) {
    toast.add({
      severity: 'warn',
      summary: 'PDF',
      detail: 'PDF pro Monat nur für einen Mitarbeiter.',
      life: 10000,
    })
    return
  }
  try {
    const emp = employees.value.find((x) => x.id === id)
    const blob = await fetchExportPdfBlob(id, month.value, year.value)
    triggerDownload(blob, `report-${emp?.username ?? id}-${year.value}-${String(month.value).padStart(2, '0')}.pdf`)
  } catch {
    toast.add({ severity: 'error', summary: 'PDF-Export fehlgeschlagen', life: 10000 })
  }
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Berichte & Export</template>
      <template #content>
        <div class="toolbar">
          <div class="fld">
            <span class="lbl">Mitarbeiter</span>
            <Select
              v-model="employeeId"
              :options="employeeOptions"
              option-label="label"
              option-value="value"
              placeholder="Wählen"
              class="w"
              filter
              @show="focusSelectFilterOnShow"
            />
          </div>
          <div class="fld">
            <span class="lbl">Monat</span>
            <InputNumber v-model="month" :min="1" :max="12" :use-grouping="false" show-buttons />
          </div>
          <div class="fld">
            <span class="lbl">Jahr</span>
            <InputNumber v-model="year" :min="2000" :max="2100" :use-grouping="false" show-buttons />
          </div>
        </div>
        <p class="hint">
          Zeitraum für CSV und Vorschau: erster bis letzter Tag des gewählten Monats. PDF nutzt ebenfalls Monat/Jahr.
        </p>
        <div class="actions">
          <Button label="Vorschau laden" icon="pi pi-table" :loading="previewLoading" @click="loadPreview" />
          <Button label="Export CSV" icon="pi pi-download" severity="secondary" outlined @click="downloadCsv" />
          <Button label="Export PDF" icon="pi pi-file-pdf" severity="help" outlined @click="downloadPdf" />
        </div>

        <h3 class="sub">Vorschau (CSV-Daten)</h3>
        <DataTable
          :value="previewData"
          :loading="previewLoading"
          size="small"
          striped-rows
          scrollable
          scroll-height="400px"
          data-key="_i"
        >
          <Column v-for="h in previewHeaders" :key="h" :field="h" :header="h" />
        </DataTable>
      </template>
    </Card>
  </div>
</template>

<style scoped>
.page {
  max-width: 1100px;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  align-items: flex-end;
}
.fld {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.lbl {
  font-size: 0.75rem;
  color: #64748b;
}
.w {
  min-width: 260px;
}
.hint {
  font-size: 0.85rem;
  color: #64748b;
  margin: 1rem 0;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 1.25rem;
}
.sub {
  font-size: 1rem;
  margin: 0 0 0.5rem;
}
</style>
