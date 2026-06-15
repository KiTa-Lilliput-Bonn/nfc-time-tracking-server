<script setup lang="ts">
import { onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import { useToast } from 'primevue/usetoast'

import { fetchShiftAlertConfig, putShiftAlertConfig } from '@/api/management'
import { getApiErrorMessage } from '@/utils/apiError'

const emit = defineEmits<{
  saved: []
}>()

const toast = useToast()

const loading = ref(false)
const saving = ref(false)
const maxHours = ref(11)
const lateEndCheckEnabled = ref(true)
const lateEndTime = ref('21:00')

async function load() {
  loading.value = true
  try {
    const cfg = await fetchShiftAlertConfig()
    maxHours.value = cfg.max_hours
    const late = (cfg.late_end_time ?? '').trim()
    lateEndCheckEnabled.value = late !== ''
    lateEndTime.value = late || '21:00'
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Einstellungen konnten nicht geladen werden',
      detail: getApiErrorMessage(e),
      life: 5000,
    })
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await putShiftAlertConfig({
      max_hours: maxHours.value ?? 0,
      late_end_time: lateEndCheckEnabled.value ? lateEndTime.value.trim() : '',
    })
    toast.add({ severity: 'success', summary: 'Einstellungen gespeichert', life: 3000 })
    emit('saved')
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Speichern fehlgeschlagen',
      detail: getApiErrorMessage(e),
      life: 5000,
    })
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <Card class="shift-alert-settings" data-testid="shift-alert-settings">
    <template #title>Schwellwerte für Warnungen</template>
    <template #content>
      <p v-if="loading" class="muted">Laden…</p>
      <div v-else class="fields">
        <div class="field">
          <label for="shift-alert-max-hours">Maximale Tagesarbeitszeit (Stunden)</label>
          <InputNumber
            id="shift-alert-max-hours"
            v-model="maxHours"
            :min="0"
            :max="24"
            :min-fraction-digits="0"
            :max-fraction-digits="1"
            input-id="shift-alert-max-hours-input"
            data-testid="shift-alert-max-hours"
          />
          <p class="hint">0 = Prüfung deaktiviert</p>
        </div>
        <div class="field">
          <div class="late-end-toggle">
            <Checkbox v-model="lateEndCheckEnabled" binary input-id="shift-alert-late-end-enabled" />
            <label for="shift-alert-late-end-enabled">Spätes Ende prüfen</label>
          </div>
          <label for="shift-alert-late-end">Uhrzeit</label>
          <input
            id="shift-alert-late-end"
            v-model="lateEndTime"
            type="time"
            step="60"
            class="p-inputtext p-component time-input"
            :disabled="!lateEndCheckEnabled"
            data-testid="shift-alert-late-end"
          />
          <p class="hint">Haken entfernen = Prüfung deaktiviert</p>
        </div>
        <Button
          label="Speichern"
          icon="pi pi-save"
          size="small"
          :loading="saving"
          data-testid="shift-alert-settings-save"
          @click="save"
        />
      </div>
    </template>
  </Card>
</template>

<style scoped>
.fields {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 22rem;
}
.field label {
  display: block;
  margin-bottom: 0.35rem;
  font-weight: 500;
}
.late-end-toggle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}
.late-end-toggle label {
  margin: 0;
  font-weight: 500;
  cursor: pointer;
}
.time-input {
  width: 100%;
  box-sizing: border-box;
}
.hint {
  margin: 0.25rem 0 0;
  font-size: 0.85rem;
  color: var(--p-text-muted-color);
}
.muted {
  color: var(--p-text-muted-color);
}
</style>
