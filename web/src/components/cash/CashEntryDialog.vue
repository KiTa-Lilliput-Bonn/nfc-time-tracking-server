<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'

import { createCashEntry, deleteCashReceipt, updateCashEntry, uploadCashReceipts } from '@/api/groupCash'
import type { CashEntry, CashEntryInput, CashEntryKind, CashIncomeSource, CashReceipt, CashSummary } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import { toISODateLocal } from '@/utils/dates'
import { centsToInput, formatEuro, monthLabel, parseEuroToCents } from '@/utils/money'
import { prepareReceiptFile } from '@/utils/receiptImage'

const props = defineProps<{
  visible: boolean
  groupId: number
  summary: CashSummary
  /** Neue Buchung dieser Art, oder bestehende Buchung bearbeiten. */
  kind: CashEntryKind
  entry: CashEntry | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

const toast = useToast()
const today = toISODateLocal(new Date())
const currentMonth = today.slice(0, 7)

const kindOptions: { label: string; value: CashEntryKind }[] = [
  { label: 'Ausgabe', value: 'expense' },
  { label: 'Einnahme', value: 'income' },
]
const sourceOptions: { label: string; value: CashIncomeSource }[] = [
  { label: 'Monatsbetrag', value: 'allowance' },
  { label: 'Aus Ansparkonto', value: 'savings' },
  { label: 'Sonstige', value: 'other' },
]

const kind = ref<CashEntryKind>('expense')
const source = ref<CashIncomeSource>('allowance')
const forMonth = ref(currentMonth)
const entryDate = ref(today)
const amount = ref('')
const description = ref('')
const existingReceipts = ref<CashReceipt[]>([])
const removedReceiptIds = ref<number[]>([])
const pendingFiles = ref<File[]>([])
const saving = ref(false)

const cameraInput = ref<HTMLInputElement | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

watch(
  () => props.visible,
  (open) => {
    if (!open) return
    const e = props.entry
    kind.value = e ? e.kind : props.kind
    source.value = e?.source || 'allowance'
    forMonth.value = e?.for_month || currentMonth
    entryDate.value = e?.entry_date || today
    amount.value = e ? centsToInput(e.amount_cents) : ''
    description.value = e?.description ?? ''
    existingReceipts.value = e ? [...e.receipts] : []
    removedReceiptIds.value = []
    pendingFiles.value = []
  },
)

const isIncome = computed(() => kind.value === 'income')
const descriptionRequired = computed(() => kind.value === 'expense' || source.value === 'other')
const title = computed(() => {
  const what = kind.value === 'expense' ? 'Ausgabe' : 'Einnahme'
  return props.entry ? `${what} bearbeiten` : `${what} eintragen`
})

/** Hinweis zu Monatsanspruch bzw. Ansparkonto (bei Bearbeitung ohne den eigenen alten Betrag). */
const sourceHint = computed(() => {
  if (!isIncome.value) return ''
  const own = props.entry
  if (source.value === 'allowance') {
    if (forMonth.value !== props.summary.current_month) {
      return `Monatsbetrag für ${monthLabel(forMonth.value)}. Was für einen Monat nicht abgerufen wurde, liegt im Ansparkonto.`
    }
    let open = props.summary.current_open_cents
    if (own?.source === 'allowance' && own.for_month === forMonth.value) open += own.amount_cents
    return `Für ${monthLabel(forMonth.value)} stehen noch ${formatEuro(Math.max(0, open))} von ${formatEuro(props.summary.current_allowance_cents)} zu.`
  }
  if (source.value === 'savings') {
    let avail = props.summary.savings_cents
    if (own?.source === 'savings') avail += own.amount_cents
    return `Im Ansparkonto verfügbar: ${formatEuro(Math.max(0, avail))}.`
  }
  return 'Zum Beispiel Spenden oder Elternbeiträge. Sie zählen nicht zum Monatsanspruch.'
})

function close() {
  emit('update:visible', false)
}

function warn(summary: string) {
  toast.add({ severity: 'warn', summary, life: 8000 })
}

function onFilesPicked(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  pendingFiles.value.push(...files)
  input.value = ''
}

function removePending(i: number) {
  pendingFiles.value.splice(i, 1)
}

function removeExisting(r: CashReceipt) {
  existingReceipts.value = existingReceipts.value.filter((x) => x.id !== r.id)
  removedReceiptIds.value.push(r.id)
}

function sizeLabel(bytes: number) {
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / 1024 / 1024).toFixed(1).replace('.', ',')} MB`
}

async function save() {
  const cents = parseEuroToCents(amount.value)
  if (cents == null || cents <= 0) {
    warn('Bitte einen gültigen Betrag eingeben, z. B. 12,50.')
    return
  }
  if (!entryDate.value) {
    warn('Bitte ein Datum angeben.')
    return
  }
  if (descriptionRequired.value && !description.value.trim()) {
    warn(kind.value === 'expense' ? 'Bitte angeben, wofür das Geld ausgegeben wurde.' : 'Bitte angeben, woher das Geld kommt.')
    return
  }
  const body: CashEntryInput = {
    kind: kind.value,
    source: isIncome.value ? source.value : '',
    for_month: isIncome.value && source.value === 'allowance' ? forMonth.value : '',
    entry_date: entryDate.value,
    amount_cents: cents,
    description: description.value.trim(),
  }
  saving.value = true
  let entryId = props.entry?.id
  try {
    if (entryId) {
      await updateCashEntry(props.groupId, entryId, body)
    } else {
      entryId = (await createCashEntry(props.groupId, body)).id
    }
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Speichern nicht möglich', detail: getApiErrorMessage(e), life: 10000 })
    saving.value = false
    return
  }
  try {
    for (const id of removedReceiptIds.value) await deleteCashReceipt(props.groupId, id)
    if (pendingFiles.value.length) {
      const prepared = await Promise.all(pendingFiles.value.map(prepareReceiptFile))
      await uploadCashReceipts(props.groupId, entryId, prepared)
    }
    toast.add({ severity: 'success', summary: 'Gespeichert', life: 4000 })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Buchung gespeichert, Beleg nicht',
      detail: getApiErrorMessage(e),
      life: 12000,
    })
  } finally {
    saving.value = false
  }
  emit('saved')
  close()
}
</script>

<template>
  <Dialog
    :visible="visible"
    modal
    :header="title"
    :style="{ width: 'min(560px, 96vw)' }"
    :breakpoints="{ '600px': '100vw' }"
    @update:visible="emit('update:visible', $event)"
  >
    <div class="form">
      <SelectButton
        v-if="!entry"
        v-model="kind"
        :options="kindOptions"
        option-label="label"
        option-value="value"
        :allow-empty="false"
        data-testid="cash-entry-kind"
      />

      <template v-if="isIncome">
        <span class="label">Herkunft</span>
        <SelectButton
          v-model="source"
          :options="sourceOptions"
          option-label="label"
          option-value="value"
          :allow-empty="false"
          class="wrap"
          data-testid="cash-entry-source"
        />
        <p class="hint" data-testid="cash-entry-hint">{{ sourceHint }}</p>
        <template v-if="source === 'allowance'">
          <label for="ce-month">Für Monat</label>
          <input id="ce-month" v-model="forMonth" type="month" :max="currentMonth" class="p-inputtext p-component w" />
        </template>
      </template>

      <div class="two">
        <div>
          <label for="ce-date">Datum</label>
          <input id="ce-date" v-model="entryDate" type="date" :min="summary.opening_date" :max="today" class="p-inputtext p-component w" />
        </div>
        <div>
          <label for="ce-amount">Betrag (€)</label>
          <InputText id="ce-amount" v-model="amount" inputmode="decimal" placeholder="0,00" class="w" autocomplete="off" />
        </div>
      </div>

      <label for="ce-desc">{{ descriptionRequired ? (isIncome ? 'Woher? (Pflicht)' : 'Wofür? (Pflicht)') : 'Bemerkung (optional)' }}</label>
      <Textarea id="ce-desc" v-model="description" rows="2" auto-resize class="w" maxlength="500" />

      <span class="label">Belege (PDF oder Foto)</span>
      <ul v-if="existingReceipts.length || pendingFiles.length" class="files">
        <li v-for="r in existingReceipts" :key="`r${r.id}`">
          <span class="pi" :class="r.content_type === 'application/pdf' ? 'pi-file-pdf' : 'pi-image'" aria-hidden="true" />
          <span class="fname">{{ r.filename }}</span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Beleg entfernen" @click="removeExisting(r)" />
        </li>
        <li v-for="(f, i) in pendingFiles" :key="`p${i}`" data-testid="cash-pending-file">
          <span class="pi" :class="f.type === 'application/pdf' ? 'pi-file-pdf' : 'pi-image'" aria-hidden="true" />
          <span class="fname">{{ f.name }}</span>
          <span class="muted small">{{ sizeLabel(f.size) }} · neu</span>
          <Button icon="pi pi-times" text rounded size="small" severity="secondary" aria-label="Datei entfernen" @click="removePending(i)" />
        </li>
      </ul>
      <div class="file-buttons">
        <Button label="Foto aufnehmen" icon="pi pi-camera" severity="secondary" outlined size="small" @click="cameraInput?.click()" />
        <Button label="Datei wählen" icon="pi pi-paperclip" severity="secondary" outlined size="small" @click="fileInput?.click()" />
        <input ref="cameraInput" type="file" accept="image/*" capture="environment" hidden @change="onFilesPicked" />
        <input
          ref="fileInput"
          type="file"
          accept="application/pdf,image/*"
          multiple
          hidden
          data-testid="cash-file-input"
          @change="onFilesPicked"
        />
      </div>
    </div>

    <template #footer>
      <Button label="Abbrechen" severity="secondary" text @click="close" />
      <Button label="Speichern" icon="pi pi-check" :loading="saving" data-testid="cash-entry-save" @click="save" />
    </template>
  </Dialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.form label,
.label {
  font-size: 0.85rem;
  color: #64748b;
  margin-top: 0.35rem;
}
.w {
  width: 100%;
}
.wrap {
  flex-wrap: wrap;
}
.hint {
  margin: 0.15rem 0 0;
  font-size: 0.85rem;
  color: #475569;
}
.two {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
}
.two > div {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.files {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.files li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.25rem 0.25rem 0.25rem 0.6rem;
}
.fname {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.8rem;
}
</style>
