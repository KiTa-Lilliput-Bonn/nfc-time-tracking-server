<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import { useToast } from 'primevue/usetoast'

import CashEntryDialog from '@/components/cash/CashEntryDialog.vue'
import ReceiptViewerDialog from '@/components/cash/ReceiptViewerDialog.vue'
import { fetchEmployees } from '@/api/employees'
import {
  deleteCashAllowance,
  deleteCashEntry,
  deleteCashOpening,
  downloadCashExport,
  fetchCashBox,
  putCashAllowance,
  putCashKeepers,
  putCashOpening,
  type CashExportFormat,
} from '@/api/groupCash'
import type { CashBoxDetail, CashEntry, CashEntryKind, CashReceipt, Employee } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { formatGermanDate, toISODateLocal } from '@/utils/dates'
import { centsToInput, formatEuro, monthLabel, parseEuroToCents } from '@/utils/money'

const route = useRoute()
const toast = useToast()

const groupId = computed(() => Number(route.params.groupId))
const box = ref<CashBoxDetail | null>(null)
const loading = ref(true)
const err = ref('')

async function load() {
  err.value = ''
  try {
    box.value = await fetchCashBox(groupId.value)
  } catch (e) {
    err.value = getApiErrorMessage(e) ?? 'Gruppenkasse konnte nicht geladen werden.'
  } finally {
    loading.value = false
  }
}

watch(groupId, () => {
  loading.value = true
  void load()
}, { immediate: true })

/* Buchungen nach Jahr */
const years = computed(() => {
  const set = new Set<string>([String(new Date().getFullYear())])
  for (const e of box.value?.entries ?? []) set.add(e.entry_date.slice(0, 4))
  return [...set].sort().reverse()
})
const year = ref(String(new Date().getFullYear()))
const visibleEntries = computed(() => (box.value?.entries ?? []).filter((e) => e.entry_date.startsWith(year.value)))
const yearTotals = computed(() => {
  let income = 0
  let expense = 0
  let fromSavings = 0
  for (const e of visibleEntries.value) {
    if (e.kind === 'income') income += e.amount_cents
    else if (e.source === 'savings') fromSavings += e.amount_cents
    else expense += e.amount_cents
  }
  return { income, expense, fromSavings }
})

const sourceLabel: Record<string, string> = {
  allowance: 'Monatsbetrag',
  savings: 'Aus Ansparkonto',
  other: 'Einnahme',
}

function entryTitle(e: CashEntry) {
  if (e.kind === 'expense') return e.description
  if (e.source === 'allowance') return `Monatsbetrag ${monthLabel(e.for_month)}`
  if (e.source === 'savings') return 'Entnahme aus dem Ansparkonto'
  return e.description
}

function entryNote(e: CashEntry) {
  if (e.kind === 'income' && e.source !== 'other' && e.description) return e.description
  return ''
}

/* Buchung anlegen / bearbeiten */
const entryDialog = ref(false)
const entryKind = ref<CashEntryKind>('expense')
const editingEntry = ref<CashEntry | null>(null)

function openNew(kind: CashEntryKind) {
  editingEntry.value = null
  entryKind.value = kind
  entryDialog.value = true
}

function openEdit(e: CashEntry) {
  editingEntry.value = e
  entryKind.value = e.kind
  entryDialog.value = true
}

async function removeEntry(e: CashEntry) {
  const extra = e.receipts.length ? ` Die ${e.receipts.length} Beleg(e) werden mitgelöscht.` : ''
  if (!confirm(`Buchung „${entryTitle(e)}“ über ${formatEuro(e.amount_cents)} löschen?${extra}`)) return
  try {
    await deleteCashEntry(groupId.value, e.id)
    await load()
  } catch (er) {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', detail: getApiErrorMessage(er), life: 10000 })
  }
}

/* Export */
const exporting = ref<CashExportFormat | null>(null)

async function exportLedger(format: CashExportFormat) {
  if (!box.value) return
  exporting.value = format
  try {
    await downloadCashExport(groupId.value, box.value.group_name, year.value, format)
  } catch {
    toast.add({ severity: 'error', summary: 'Export fehlgeschlagen', life: 10000 })
  } finally {
    exporting.value = null
  }
}

/* Anfangsbestand */
const openingDialog = ref(false)
const openingDate = ref('')
const openingCash = ref('')
const openingSavings = ref('')
const openingSaving = ref(false)
const openingYearVisible = computed(() => box.value?.summary.opening_date?.startsWith(year.value) ?? false)

function openOpening() {
  const s = box.value?.summary
  const firstEntry = box.value?.entries.at(-1)?.entry_date
  openingDate.value = s?.opening_date ?? firstEntry ?? toISODateLocal(new Date())
  openingCash.value = s?.opening_date ? centsToInput(s.opening_cash_cents) : ''
  openingSavings.value = s?.opening_date ? centsToInput(s.opening_savings_cents) : ''
  openingDialog.value = true
}

async function saveOpening() {
  const cash = parseEuroToCents(openingCash.value || '0')
  const savings = parseEuroToCents(openingSavings.value || '0')
  if (cash == null || savings == null) {
    toast.add({ severity: 'warn', summary: 'Bitte gültige Beträge eingeben, z. B. 42,50.', life: 8000 })
    return
  }
  if (!openingDate.value) {
    toast.add({ severity: 'warn', summary: 'Bitte den Stichtag angeben.', life: 8000 })
    return
  }
  openingSaving.value = true
  try {
    await putCashOpening(groupId.value, { date: openingDate.value, cash_cents: cash, savings_cents: savings })
    openingDialog.value = false
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Speichern nicht möglich', detail: getApiErrorMessage(e), life: 10000 })
  } finally {
    openingSaving.value = false
  }
}

async function removeOpening() {
  if (!confirm('Anfangsbestand löschen? Kassenstand und Ansparkonto werden ohne ihn neu berechnet.')) return
  try {
    await deleteCashOpening(groupId.value)
    openingDialog.value = false
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', detail: getApiErrorMessage(e), life: 10000 })
  }
}

/* Belege ansehen */
const viewing = ref<CashReceipt | null>(null)

/* Monatsanspruch */
const allowanceFrom = ref(toISODateLocal(new Date()).slice(0, 7))
const allowanceAmount = ref('')
const allowanceSaving = ref(false)

async function saveAllowance() {
  const cents = parseEuroToCents(allowanceAmount.value)
  if (cents == null) {
    toast.add({ severity: 'warn', summary: 'Bitte einen gültigen Betrag eingeben, z. B. 100,00.', life: 8000 })
    return
  }
  if (!allowanceFrom.value) {
    toast.add({ severity: 'warn', summary: 'Bitte den Monat angeben, ab dem der Betrag gilt.', life: 8000 })
    return
  }
  allowanceSaving.value = true
  try {
    await putCashAllowance(groupId.value, allowanceFrom.value, cents)
    allowanceAmount.value = ''
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', detail: getApiErrorMessage(e), life: 10000 })
  } finally {
    allowanceSaving.value = false
  }
}

async function removeAllowance(id: number, from: string) {
  if (!confirm(`Anspruch ab ${monthLabel(from)} löschen? Das Ansparkonto wird neu berechnet.`)) return
  try {
    await deleteCashAllowance(groupId.value, id)
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Löschen fehlgeschlagen', detail: getApiErrorMessage(e), life: 10000 })
  }
}

const allowancesDesc = computed(() => [...(box.value?.allowances ?? [])].reverse())
const months = computed(() => box.value?.summary.months ?? [])
const showAllMonths = ref(false)
const visibleMonths = computed(() => (showAllMonths.value ? months.value : months.value.slice(0, 6)))

/* Kassenwarte (Leitung) */
const keeperDialog = ref(false)
const keeperIds = ref<number[]>([])
const employees = ref<Employee[]>([])
const keeperSaving = ref(false)

const keeperOptions = computed(() =>
  employees.value
    .filter((e) => e.active && e.role !== 'superadmin')
    .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
    .map((e) => ({ label: e.display_name, value: e.id })),
)

async function openKeepers() {
  keeperIds.value = box.value?.keepers.map((k) => k.id) ?? []
  keeperDialog.value = true
  if (!employees.value.length) {
    try {
      employees.value = await fetchEmployees()
    } catch {
      toast.add({ severity: 'error', summary: 'Mitarbeitende konnten nicht geladen werden', life: 10000 })
    }
  }
}

async function saveKeepers() {
  keeperSaving.value = true
  try {
    await putCashKeepers(groupId.value, keeperIds.value)
    keeperDialog.value = false
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', detail: getApiErrorMessage(e), life: 10000 })
  } finally {
    keeperSaving.value = false
  }
}
</script>

<template>
  <div class="page">
    <p v-if="loading" class="muted">Laden…</p>
    <p v-else-if="err" class="err">{{ err }}</p>
    <template v-else-if="box">
      <div class="head">
        <div>
          <RouterLink to="/cash-boxes" class="back">← Alle Gruppenkassen</RouterLink>
          <h2 class="name" data-testid="cash-box-name">{{ box.group_name }}</h2>
          <p class="muted small">
            Kassenwarte:
            <template v-if="box.keepers.length">{{ box.keepers.map((k) => k.display_name).join(', ') }}</template>
            <template v-else>noch keine</template>
            <Button
              v-if="box.can_manage_keepers"
              label="ändern"
              link
              size="small"
              class="inline-btn"
              data-testid="cash-keepers-edit"
              @click="openKeepers"
            />
          </p>
          <p v-if="!box.can_edit" class="readonly small">Nur Lesezugriff</p>
        </div>
        <div v-if="box.can_edit" class="actions">
          <Button label="Ausgabe" icon="pi pi-minus" severity="danger" outlined data-testid="cash-new-expense" @click="openNew('expense')" />
          <Button label="Einnahme" icon="pi pi-plus" severity="success" outlined data-testid="cash-new-income" @click="openNew('income')" />
        </div>
      </div>

      <div class="tiles">
        <div class="tile">
          <span class="tile-label">Kassenstand</span>
          <span class="tile-value" :class="{ neg: box.summary.balance_cents < 0 }" data-testid="cash-balance">
            {{ formatEuro(box.summary.balance_cents) }}
          </span>
        </div>
        <div class="tile">
          <span class="tile-label">Ansparkonto</span>
          <span class="tile-value" data-testid="cash-savings">{{ formatEuro(box.summary.savings_cents) }}</span>
          <span class="tile-sub">nicht abgerufene Monatsbeträge</span>
        </div>
        <div class="tile">
          <span class="tile-label">Monatsbetrag {{ monthLabel(box.summary.current_month) }}</span>
          <span class="tile-value">{{ formatEuro(box.summary.current_open_cents) }}</span>
          <span class="tile-sub">
            noch offen von {{ formatEuro(box.summary.current_allowance_cents) }}
          </span>
        </div>
      </div>

      <Card>
        <template #title>
          <div class="card-title">
            <span>Buchungen</span>
            <Select v-model="year" :options="years" class="year" aria-label="Jahr" />
          </div>
        </template>
        <template #content>
          <div class="export">
            <span class="muted small">Kassenbuch {{ year }}:</span>
            <Button label="PDF" icon="pi pi-file-pdf" size="small" severity="secondary" outlined :loading="exporting === 'pdf'" data-testid="cash-export-pdf" @click="exportLedger('pdf')" />
            <Button label="Excel (CSV)" icon="pi pi-table" size="small" severity="secondary" outlined :loading="exporting === 'csv'" data-testid="cash-export-csv" @click="exportLedger('csv')" />
            <Button label="ZIP mit Belegen" icon="pi pi-download" size="small" severity="secondary" outlined :loading="exporting === 'zip'" data-testid="cash-export-zip" @click="exportLedger('zip')" />
          </div>
          <p v-if="!visibleEntries.length && !openingYearVisible" class="muted">Keine Buchungen in {{ year }}.</p>
          <template v-if="visibleEntries.length">
            <p class="muted small totals">
              {{ year }}: Einnahmen {{ formatEuro(yearTotals.income) }} · Ausgaben {{ formatEuro(yearTotals.expense) }}
              <template v-if="yearTotals.fromSavings">
                · direkt aus Ansparkonto {{ formatEuro(yearTotals.fromSavings) }}
              </template>
            </p>
            <ul class="entries">
              <li v-for="e in visibleEntries" :key="e.id" class="entry" data-testid="cash-entry">
                <div class="entry-main">
                  <div class="entry-text">
                    <span class="entry-title">{{ entryTitle(e) }}</span>
                    <span class="muted small">
                      {{ formatGermanDate(e.entry_date) }}
                      <template v-if="e.kind === 'income'"> · {{ sourceLabel[e.source] }}</template>
                      <template v-else-if="e.source === 'savings'"> · direkt aus Ansparkonto</template>
                      <template v-if="e.created_by_name"> · {{ e.created_by_name }}</template>
                    </span>
                    <span v-if="entryNote(e)" class="muted small">{{ entryNote(e) }}</span>
                  </div>
                  <div class="entry-amount">
                    <span :class="e.kind === 'income' ? 'pos' : e.source === 'savings' ? 'savings' : 'neg'">
                      {{ e.kind === 'income' ? '+' : '−' }}{{ formatEuro(e.amount_cents) }}
                    </span>
                    <span v-if="e.kind === 'expense' && e.source === 'savings'" class="muted small">aus Ansparkonto</span>
                    <span v-else class="muted small">Stand {{ formatEuro(e.balance_after_cents) }}</span>
                  </div>
                </div>
                <div class="entry-foot">
                  <div class="receipts">
                    <button
                      v-for="r in e.receipts"
                      :key="r.id"
                      type="button"
                      class="receipt"
                      data-testid="cash-receipt"
                      @click="viewing = r"
                    >
                      <span class="pi" :class="r.content_type === 'application/pdf' ? 'pi-file-pdf' : 'pi-image'" aria-hidden="true" />
                      {{ r.filename }}
                    </button>
                    <Tag v-if="e.kind === 'expense' && !e.receipts.length" value="Beleg fehlt" severity="warn" />
                  </div>
                  <div v-if="box.can_edit" class="entry-actions">
                    <Button icon="pi pi-pencil" text rounded size="small" aria-label="Bearbeiten" data-testid="cash-entry-edit" @click="openEdit(e)" />
                    <Button icon="pi pi-trash" text rounded size="small" severity="danger" aria-label="Löschen" @click="removeEntry(e)" />
                  </div>
                </div>
              </li>
            </ul>
          </template>
          <div v-if="openingYearVisible" class="opening-row" data-testid="cash-opening-row">
            <span>Anfangsbestand am {{ formatGermanDate(box.summary.opening_date!) }}</span>
            <span>{{ formatEuro(box.summary.opening_cash_cents) }}</span>
          </div>
        </template>
      </Card>

      <Card>
        <template #title>Anfangsbestand</template>
        <template #content>
          <p v-if="box.summary.opening_date" class="opening-text">
            Am {{ formatGermanDate(box.summary.opening_date) }} lagen
            <strong>{{ formatEuro(box.summary.opening_cash_cents) }}</strong> in der Kasse und
            <strong>{{ formatEuro(box.summary.opening_savings_cents) }}</strong> im Ansparkonto.
          </p>
          <p v-else class="muted small opening-text">
            Wenn die Kasse schon vor der Erfassung hier Geld hatte, tragen Sie den Bestand von Kasse und Ansparkonto
            zum Stichtag ein. Buchungen beginnen dann am Stichtag.
          </p>
          <Button
            v-if="box.can_edit"
            :label="box.summary.opening_date ? 'Anfangsbestand ändern' : 'Anfangsbestand eintragen'"
            size="small"
            severity="secondary"
            outlined
            data-testid="cash-opening-edit"
            @click="openOpening"
          />
        </template>
      </Card>

      <Card>
        <template #title>Monatsanspruch und Ansparkonto</template>
        <template #content>
          <p class="muted small explain">
            Was der Kasse in einem Monat zusteht, aber nicht als Monatsbetrag ausgezahlt wird, geht am Monatsende ins
            Ansparkonto. Aus dem Ansparkonto kann über „Einnahme → Aus Ansparkonto“ in die Kasse gebucht oder eine Ausgabe
            direkt bezahlt werden („Ausgabe → Direkt aus Ansparkonto“).
          </p>

          <h3 class="sub">Anspruch pro Monat</h3>
          <p v-if="!allowancesDesc.length" class="muted small">Noch kein Monatsanspruch eingetragen.</p>
          <ul v-else class="allowances">
            <li v-for="a in allowancesDesc" :key="a.id" data-testid="cash-allowance">
              <span>ab {{ monthLabel(a.valid_from) }}: <strong>{{ formatEuro(a.amount_cents) }}</strong></span>
              <Button
                v-if="box.can_edit"
                icon="pi pi-trash"
                text
                rounded
                size="small"
                severity="secondary"
                aria-label="Anspruch löschen"
                @click="removeAllowance(a.id, a.valid_from)"
              />
            </li>
          </ul>
          <div v-if="box.can_edit" class="allowance-form">
            <div>
              <label for="al-from">Gültig ab</label>
              <input id="al-from" v-model="allowanceFrom" type="month" class="p-inputtext p-component w" />
            </div>
            <div>
              <label for="al-amount">Betrag pro Monat (€)</label>
              <InputText id="al-amount" v-model="allowanceAmount" inputmode="decimal" placeholder="100,00" class="w" />
            </div>
            <Button label="Speichern" :loading="allowanceSaving" data-testid="cash-allowance-save" @click="saveAllowance" />
          </div>

          <template v-if="months.length">
            <h3 class="sub">Verlauf</h3>
            <div class="table-wrap">
              <table class="months">
                <thead>
                  <tr>
                    <th>Monat</th>
                    <th>Anspruch</th>
                    <th>Ausgezahlt</th>
                    <th>Ins Ansparkonto</th>
                    <th>Entnommen</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in visibleMonths" :key="m.month" :class="{ current: m.current }">
                    <td>{{ monthLabel(m.month) }}<span v-if="m.current" class="muted small"> (läuft)</span></td>
                    <td>{{ formatEuro(m.allowance_cents) }}</td>
                    <td>{{ formatEuro(m.paid_cents) }}</td>
                    <td>{{ m.current ? '–' : formatEuro(m.saved_cents) }}</td>
                    <td>{{ m.withdrawn_cents ? formatEuro(m.withdrawn_cents) : '–' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Button
              v-if="months.length > 6"
              :label="showAllMonths ? 'Weniger anzeigen' : `Alle ${months.length} Monate anzeigen`"
              link
              size="small"
              @click="showAllMonths = !showAllMonths"
            />
          </template>
        </template>
      </Card>

      <CashEntryDialog
        v-model:visible="entryDialog"
        :group-id="box.group_id"
        :summary="box.summary"
        :kind="entryKind"
        :entry="editingEntry"
        @saved="load"
      />
      <ReceiptViewerDialog :group-id="box.group_id" :receipt="viewing" @close="viewing = null" />

      <Dialog
        v-model:visible="openingDialog"
        modal
        header="Anfangsbestand"
        :style="{ width: 'min(480px, 96vw)' }"
        :breakpoints="{ '600px': '100vw' }"
      >
        <div class="opening-form">
          <p class="muted small">
            Bestand am Stichtag, bevor die Kasse hier geführt wurde. Monatsbeträge ab dem Monat des Stichtags laufen
            weiter ins Ansparkonto.
          </p>
          <label for="op-date">Stichtag</label>
          <input id="op-date" v-model="openingDate" type="date" :max="toISODateLocal(new Date())" class="p-inputtext p-component w" />
          <label for="op-cash">Bargeld in der Kasse (€)</label>
          <InputText id="op-cash" v-model="openingCash" inputmode="decimal" placeholder="0,00" class="w" />
          <label for="op-savings">Guthaben im Ansparkonto (€)</label>
          <InputText id="op-savings" v-model="openingSavings" inputmode="decimal" placeholder="0,00" class="w" />
        </div>
        <template #footer>
          <Button v-if="box.summary.opening_date" label="Löschen" severity="danger" text @click="removeOpening" />
          <Button label="Abbrechen" severity="secondary" text @click="openingDialog = false" />
          <Button label="Speichern" :loading="openingSaving" data-testid="cash-opening-save" @click="saveOpening" />
        </template>
      </Dialog>

      <Dialog
        v-model:visible="keeperDialog"
        modal
        header="Kassenwarte"
        :style="{ width: 'min(480px, 96vw)' }"
      >
        <p class="muted small">Kassenwarte dürfen in dieser Kasse buchen und Belege hochladen.</p>
        <MultiSelect
          v-model="keeperIds"
          :options="keeperOptions"
          option-label="label"
          option-value="value"
          filter
          display="chip"
          placeholder="Personen wählen"
          class="w"
          data-testid="cash-keepers-select"
        />
        <template #footer>
          <Button label="Abbrechen" severity="secondary" text @click="keeperDialog = false" />
          <Button label="Speichern" :loading="keeperSaving" data-testid="cash-keepers-save" @click="saveKeepers" />
        </template>
      </Dialog>
    </template>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 960px;
}
.head {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  align-items: flex-end;
  gap: 0.75rem;
}
.back {
  font-size: 0.85rem;
  color: #4f46e5;
  text-decoration: none;
}
.name {
  margin: 0.25rem 0;
  font-size: 1.35rem;
  color: #0f172a;
}
.inline-btn {
  padding: 0 0.25rem;
}
.readonly {
  display: inline-block;
  margin: 0.25rem 0 0;
  padding: 0.1rem 0.5rem;
  border-radius: 999px;
  background: #f1f5f9;
  color: #475569;
}
.actions {
  display: flex;
  gap: 0.5rem;
}
.tiles {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0.75rem;
}
.tile {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 0.75rem 1rem;
}
.tile-label {
  font-size: 0.8rem;
  color: #64748b;
}
.tile-value {
  font-size: 1.35rem;
  font-weight: 600;
  color: #0f172a;
}
.tile-value.neg {
  color: #b91c1c;
}
.savings {
  color: #92400e;
}
.tile-sub {
  font-size: 0.75rem;
  color: #94a3b8;
}
.card-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.year {
  min-width: 6.5rem;
}
.totals {
  margin: 0 0 0.5rem;
}
.entries {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}
.entry {
  border-top: 1px solid #e2e8f0;
  padding: 0.6rem 0;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.entry:first-child {
  border-top: none;
}
.entry-main {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
}
.entry-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.entry-title {
  font-weight: 600;
  color: #0f172a;
  word-break: break-word;
}
.entry-amount {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  white-space: nowrap;
  font-weight: 600;
}
.pos {
  color: #15803d;
}
.neg {
  color: #b91c1c;
}
.entry-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}
.receipts {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  min-width: 0;
}
.receipt {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  max-width: 14rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border: 1px solid #cbd5e1;
  border-radius: 999px;
  background: #f8fafc;
  padding: 0.2rem 0.6rem;
  font: inherit;
  font-size: 0.8rem;
  cursor: pointer;
}
.receipt:hover {
  background: #eef2ff;
  border-color: #a5b4fc;
}
.entry-actions {
  display: flex;
  flex-shrink: 0;
}
.export {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}
.opening-row {
  display: flex;
  justify-content: space-between;
  border-top: 1px dashed #cbd5e1;
  padding-top: 0.6rem;
  margin-top: 0.25rem;
  color: #475569;
  font-weight: 600;
}
.opening-text {
  margin-top: 0;
}
.opening-form {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.opening-form label {
  font-size: 0.85rem;
  color: #64748b;
  margin-top: 0.35rem;
}
.explain {
  margin-top: 0;
}
.sub {
  font-size: 0.95rem;
  margin: 1rem 0 0.5rem;
  color: #0f172a;
}
.allowances {
  list-style: none;
  margin: 0;
  padding: 0;
}
.allowances li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  max-width: 24rem;
}
.allowance-form {
  display: grid;
  grid-template-columns: 1fr 1fr auto;
  gap: 0.75rem;
  align-items: end;
  margin-top: 0.5rem;
  max-width: 36rem;
}
.allowance-form > div {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.allowance-form label {
  font-size: 0.85rem;
  color: #64748b;
}
.w {
  width: 100%;
}
.table-wrap {
  overflow-x: auto;
}
.months {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
}
.months th,
.months td {
  text-align: right;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid #e2e8f0;
  white-space: nowrap;
}
.months th:first-child,
.months td:first-child {
  text-align: left;
}
.months th {
  font-weight: 600;
  color: #475569;
  font-size: 0.8rem;
}
.months tr.current td {
  background: #f8fafc;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
}
.err {
  color: #b91c1c;
}
@media (max-width: 600px) {
  .tiles {
    grid-template-columns: 1fr 1fr;
  }
  .tile:last-child {
    grid-column: span 2;
  }
  .actions {
    width: 100%;
  }
  .actions > * {
    flex: 1;
  }
  .allowance-form {
    grid-template-columns: 1fr 1fr;
  }
  .allowance-form > :last-child {
    grid-column: span 2;
  }
}
</style>
