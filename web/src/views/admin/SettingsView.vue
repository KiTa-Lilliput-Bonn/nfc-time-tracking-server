<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputGroup from 'primevue/inputgroup'
import InputGroupAddon from 'primevue/inputgroupaddon'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import AndroidApiPairingSection from '@/components/AndroidApiPairingSection.vue'
import BackupPathPickerDialog from '@/components/BackupPathPickerDialog.vue'
import { copyTextToClipboard } from '@/utils/clipboard'
import {
  fetchBackupStatus,
  fetchSettings,
  postBackupInitRestic,
  postBackupPickFolder,
  postBackupRunNow,
  putBackupConfig,
  putSetting,
} from '@/api/admin'
import type { BreakRule } from '@/types/api'

const toast = useToast()

const loading = ref(false)
const saving = ref(false)
const backupLoading = ref(false)
const backupSaving = ref(false)
const backupInitLoading = ref(false)
const backupRunLoading = ref(false)

const roundingOptions = [
  { label: '5 Minuten', value: 5 },
  { label: '10 Minuten', value: 10 },
  { label: '15 Minuten', value: 15 },
  { label: '30 Minuten', value: 30 },
]

const rounding = ref(15)
const breakRules = ref<BreakRule[]>([])

const stampsPollIntervalSeconds = ref(300)

/** Must match internal/service/backup.MinIntervalMinutes */
const backupMinIntervalMinutes = 15

const backupEnabled = ref(false)
const backupIntervalMinutes = ref(60)
const backupIntervalEmpty = computed(() => backupIntervalMinutes.value == null)
const backupIntervalTooLow = computed(
  () =>
    backupIntervalMinutes.value != null && backupIntervalMinutes.value < backupMinIntervalMinutes,
)
const backupIntervalInvalid = computed(() => backupIntervalEmpty.value || backupIntervalTooLow.value)
const backupUseRestic = ref(false)
const backupTargetPath = ref('')
const backupLastSuccess = ref('')
const backupLastError = ref('')
const backupResticInitialized = ref(false)
const backupHasResticPassword = ref(false)
const backupResticBinaryPresent = ref(false)
const backupUseResticLoaded = ref(false)
const backupResticInitializedLoaded = ref(false)
const backupFolderPickerAvailable = ref(false)

const pathPickerVisible = ref(false)
const pathPickLoading = ref(false)

const showPasswordDialog = ref(false)
const initPassword = ref('')
const initMessage = ref('')
const passwordClipboardCopied = ref(false)

function settingVal(settings: { key: string; value: string }[], key: string, fallback: string) {
  return settings.find((s) => s.key === key)?.value ?? fallback
}

async function loadSettings() {
  loading.value = true
  try {
    const list = await fetchSettings()
    const r = parseInt(settingVal(list, 'rounding_minutes', '15'), 10)
    rounding.value = [5, 10, 15, 30].includes(r) ? r : 15
    const brRaw = settingVal(
      list,
      'break_rules',
      '[{"min_work_hours":6,"break_minutes":30},{"min_work_hours":9,"break_minutes":45}]',
    )
    try {
      const parsed = JSON.parse(brRaw) as BreakRule[]
      breakRules.value = Array.isArray(parsed) ? parsed : []
    } catch {
      breakRules.value = [
        { min_work_hours: 6, break_minutes: 30 },
        { min_work_hours: 9, break_minutes: 45 },
      ]
    }

    const spRaw = settingVal(list, 'stamps_poll_interval_seconds', '')
    const spIv = spRaw === '' ? 300 : parseInt(spRaw, 10)
    stampsPollIntervalSeconds.value = Number.isFinite(spIv) && spIv >= 0 ? spIv : 300
  } catch {
    toast.add({ severity: 'error', summary: 'Einstellungen', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

async function loadBackup() {
  backupLoading.value = true
  try {
    const st = await fetchBackupStatus()
    backupEnabled.value = st.enabled
    backupIntervalMinutes.value = st.interval_minutes
    backupUseRestic.value = st.use_restic
    backupTargetPath.value = st.target_path
    backupLastSuccess.value = st.last_success_utc
    backupLastError.value = st.last_error
    backupResticInitialized.value = st.restic_initialized
    backupHasResticPassword.value = st.has_restic_password
    backupResticBinaryPresent.value = st.restic_binary_present
    backupUseResticLoaded.value = st.use_restic
    backupResticInitializedLoaded.value = st.restic_initialized
    backupFolderPickerAvailable.value = st.folder_picker_available
  } catch {
    toast.add({ severity: 'error', summary: 'Backup', detail: 'Status laden fehlgeschlagen.', life: 10000 })
  } finally {
    backupLoading.value = false
  }
}

async function load() {
  await Promise.all([loadSettings(), loadBackup()])
}

onMounted(load)

async function saveAll() {
  saving.value = true
  try {
    await putSetting('rounding_minutes', String(rounding.value))
    await putSetting('break_rules', JSON.stringify(breakRules.value))

    await putSetting('stamps_poll_interval_seconds', String(Math.max(0, stampsPollIntervalSeconds.value)))

    toast.add({ severity: 'success', summary: 'Gespeichert', life: 10000 })
    await loadSettings()
  } catch {
    toast.add({ severity: 'error', summary: 'Speichern fehlgeschlagen', life: 10000 })
  } finally {
    saving.value = false
  }
}

function backupApiError(e: unknown): string {
  return e && typeof e === 'object' && 'response' in e
    ? String((e as { response?: { data?: { error?: string } } }).response?.data?.error ?? '')
    : ''
}

async function showResticPasswordDialog(res: { password: string; message: string }) {
  initPassword.value = res.password
  initMessage.value = res.message
  passwordClipboardCopied.value = await copyTextToClipboard(res.password)
  showPasswordDialog.value = true
  await loadBackup()
}

function isPickFolderUnavailable(e: unknown): boolean {
  if (!e || typeof e !== 'object' || !('response' in e)) return false
  return (e as { response?: { status?: number } }).response?.status === 501
}

async function openPathPicker() {
  if (backupFolderPickerAvailable.value) {
    pathPickLoading.value = true
    try {
      const res = await postBackupPickFolder({
        initial_path: backupTargetPath.value.trim() || undefined,
      })
      if (!res.cancelled && res.path) {
        backupTargetPath.value = res.path
      }
      return
    } catch (e: unknown) {
      if (!isPickFolderUnavailable(e)) {
        toast.add({
          severity: 'error',
          summary: 'Ordner wählen',
          detail: backupApiError(e) || 'Systemdialog konnte nicht geöffnet werden.',
          life: 10000,
        })
        return
      }
    } finally {
      pathPickLoading.value = false
    }
  }
  pathPickerVisible.value = true
}

function onBackupPathSelected(path: string) {
  backupTargetPath.value = path
}

async function saveBackupConfig() {
  if (backupIntervalEmpty.value) {
    toast.add({
      severity: 'warn',
      summary: 'Backup',
      detail: 'Bitte ein Intervall in Minuten angeben.',
      life: 10000,
    })
    return
  }
  if (backupIntervalTooLow.value) {
    toast.add({
      severity: 'warn',
      summary: 'Backup',
      detail: `Das Intervall muss mindestens ${backupMinIntervalMinutes} Minuten betragen — kürzere Abstände würden die Datenbank zu häufig sichern.`,
      life: 10000,
    })
    return
  }

  const turningOnRestic = !backupUseResticLoaded.value && backupUseRestic.value
  const wasResticInitialized = backupResticInitializedLoaded.value

  backupSaving.value = true
  try {
    await putBackupConfig({
      enabled: backupEnabled.value,
      interval_minutes: backupIntervalMinutes.value,
      use_restic: backupUseRestic.value,
      target_path: backupTargetPath.value,
    })
    toast.add({ severity: 'success', summary: 'Backup', detail: 'Konfiguration gespeichert.', life: 8000 })
    await loadBackup()

    if (turningOnRestic && !wasResticInitialized) {
      if (!backupTargetPath.value.trim()) {
        toast.add({
          severity: 'warn',
          summary: 'restic',
          detail: 'Zielpfad angeben, um das Repository anzulegen.',
          life: 10000,
        })
      } else if (!backupResticBinaryPresent.value) {
        toast.add({
          severity: 'warn',
          summary: 'restic',
          detail: 'restic-Binary nicht gefunden — Repository konnte nicht angelegt werden.',
          life: 12000,
        })
      } else {
        backupInitLoading.value = true
        try {
          const res = await postBackupInitRestic({ repo_path: backupTargetPath.value.trim() })
          await showResticPasswordDialog(res)
          toast.add({
            severity: 'success',
            summary: 'restic',
            detail: 'Repository wurde angelegt.',
            life: 8000,
          })
        } catch (e: unknown) {
          toast.add({
            severity: 'error',
            summary: 'restic init',
            detail: backupApiError(e) || 'Repository konnte nicht angelegt werden.',
            life: 12000,
          })
        } finally {
          backupInitLoading.value = false
        }
      }
    }
  } catch (e: unknown) {
    toast.add({
      severity: 'error',
      summary: 'Backup',
      detail: backupApiError(e) || 'Speichern fehlgeschlagen.',
      life: 12000,
    })
  } finally {
    backupSaving.value = false
  }
}

async function initResticRepo() {
  if (!backupTargetPath.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Backup', detail: 'Zielpfad (Repository) angeben.', life: 8000 })
    return
  }
  backupInitLoading.value = true
  try {
    const res = await postBackupInitRestic({ repo_path: backupTargetPath.value.trim() })
    await showResticPasswordDialog(res)
  } catch (e: unknown) {
    toast.add({ severity: 'error', summary: 'restic init', detail: backupApiError(e) || 'Fehlgeschlagen.', life: 12000 })
  } finally {
    backupInitLoading.value = false
  }
}

async function runBackupNow() {
  backupRunLoading.value = true
  try {
    await postBackupRunNow()
    toast.add({ severity: 'success', summary: 'Backup', detail: 'Backup ausgeführt.', life: 8000 })
    await loadBackup()
  } catch (e: unknown) {
    const msg = e && typeof e === 'object' && 'response' in e ? String((e as { response?: { data?: { error?: string } } }).response?.data?.error ?? '') : ''
    toast.add({ severity: 'error', summary: 'Backup', detail: msg || 'Fehlgeschlagen.', life: 12000 })
  } finally {
    backupRunLoading.value = false
  }
}

function addRule() {
  breakRules.value.push({ min_work_hours: 6, break_minutes: 15 })
}

function removeRule(i: number) {
  breakRules.value.splice(i, 1)
}
</script>

<template>
  <div class="page">
    <Card>
      <template #title>Systemeinstellungen</template>
      <template #content>
        <div v-if="loading" class="muted">Laden…</div>
        <template v-else>
          <section class="sec">
            <h3 class="h">Rundung (Netto-Arbeitszeit)</h3>
            <Select
              v-model="rounding"
              :options="roundingOptions"
              option-label="label"
              option-value="value"
              class="field"
            />
          </section>

          <section class="sec">
            <h3 class="h">Pausenregeln</h3>
            <p class="muted small">
              Pro zusammenhängendem Arbeitsblock (Brutto, nach Schichtbeginn): ab „Mindest-Arbeitsstunden“ wird
              die angegebene „Pause (Minuten)“ für diesen Block abgezogen — unabhängig von Ausstempel-Pausen
              dazwischen (siehe Backend-Zeitberechnung).
            </p>
            <DataTable :value="breakRules" size="small" class="tbl">
              <Column header="Mindest-Arbeitsstunden">
                <template #body="{ data }">
                  <InputNumber v-model="data.min_work_hours" :min="0" :max="24" :step="0.5" />
                </template>
              </Column>
              <Column header="Pause (Minuten)">
                <template #body="{ data }">
                  <InputNumber v-model="data.break_minutes" :min="0" :max="180" />
                </template>
              </Column>
              <Column header="">
                <template #body="{ index }">
                  <Button icon="pi pi-trash" severity="danger" text rounded @click="removeRule(index)" />
                </template>
              </Column>
            </DataTable>
            <Button label="Regel hinzufügen" icon="pi pi-plus" severity="secondary" text @click="addRule" />
          </section>

          <section class="sec panel stamps-poll">
            <h3 class="h">LAN-Stamps (Android)</h3>
            <p class="muted small">
              Der Server ruft im gleichen Intervall <code>GET /v1/stamps</code> auf <strong>allen</strong> unter
              „Android-API / LAN-Pairing“ konfigurierten Zielen auf. API-Client und Host/Port je Gerät stehen dort in der
              Tabelle <code>android_lan_targets</code>. Intervall <strong>0</strong> schaltet den Abruf aus.
            </p>
            <div class="panel-grid">
              <label class="lbl">Intervall (Sek.)</label>
              <InputNumber
                v-model="stampsPollIntervalSeconds"
                :min="0"
                :max="86400"
                :step="60"
                class="w-full"
              />
            </div>
          </section>

          <Button label="Alle Änderungen speichern" icon="pi pi-save" :loading="saving" @click="saveAll" />
        </template>
      </template>
    </Card>

    <Card class="backup-card">
      <template #title>Datenbank-Backup</template>
      <template #content>
        <div v-if="backupLoading" class="muted">Laden…</div>
        <template v-else>
          <p class="muted small">
            Geplante Sicherung der SQLite-Datenbank. Mit restic wird ein verschlüsseltes Repository im angegebenen
            <strong>absoluten</strong> Verzeichnis angelegt; die Passphrase wird bei der Initialisierung einmalig
            angezeigt und serverseitig für automatische Läufe gespeichert (siehe Server-Dokumentation).
          </p>
          <div class="panel-grid backup-grid">
            <label class="lbl">Backup aktiv</label>
            <div class="chk-row">
              <Checkbox v-model="backupEnabled" :binary="true" input-id="backup-en" />
              <label for="backup-en" class="chk-lbl">Geplante Backups ausführen</label>
            </div>
            <label class="lbl">Intervall (Min.)</label>
            <div class="interval-field">
              <InputNumber
                v-model="backupIntervalMinutes"
                :max="10080"
                :step="15"
                :invalid="backupIntervalInvalid"
                class="w-full"
              />
              <p v-if="backupIntervalEmpty" class="err small interval-hint">
                Bitte ein Backup-Intervall in Minuten angeben.
              </p>
              <p v-else-if="backupIntervalTooLow" class="err small interval-hint">
                Das Backup-Intervall muss mindestens {{ backupMinIntervalMinutes }} Minuten betragen. Kürzere
                Abstände würden die Datenbank zu häufig sichern und sind deshalb nicht erlaubt.
              </p>
            </div>
            <label class="lbl">Zielpfad</label>
            <InputGroup class="w-full path-input-group">
              <InputText
                v-model="backupTargetPath"
                class="w-full mono path-input"
                placeholder="/var/backups/nfc-restic"
                autocomplete="off"
                @click="openPathPicker"
              />
              <InputGroupAddon>
                <Button
                  icon="pi pi-folder"
                  severity="secondary"
                  text
                  :loading="pathPickLoading"
                  :disabled="pathPickLoading"
                  :title="
                    backupFolderPickerAvailable
                      ? 'Systemordner-Dialog (auf dem Server)'
                      : 'Ordner auf dem Server durchsuchen'
                  "
                  @click="openPathPicker"
                />
              </InputGroupAddon>
            </InputGroup>
            <label class="lbl">restic</label>
            <div class="chk-row">
              <Checkbox v-model="backupUseRestic" :binary="true" input-id="backup-r" />
              <label for="backup-r" class="chk-lbl">Verschlüsseltes restic-Repository (Zielpfad = Repo-Verzeichnis)</label>
            </div>
          </div>
          <p class="muted small">
            restic-Binary: {{ backupResticBinaryPresent ? 'gefunden (tools/restic)' : 'nicht gefunden' }} · Repo
            initialisiert: {{ backupResticInitialized ? 'ja' : 'nein' }} · Passwort gespeichert:
            {{ backupHasResticPassword ? 'ja' : 'nein' }}
          </p>
          <p v-if="backupLastSuccess" class="muted small">Letzter erfolgreicher Lauf: {{ backupLastSuccess }}</p>
          <p v-if="backupLastError" class="err small">Letzter Fehler: {{ backupLastError }}</p>
          <div class="btn-row">
            <Button
              label="Backup-Konfiguration speichern"
              icon="pi pi-save"
              severity="secondary"
              :loading="backupSaving"
              @click="saveBackupConfig"
            />
            <Button
              label="restic-Repository anlegen"
              icon="pi pi-key"
              severity="help"
              :loading="backupInitLoading"
              :disabled="!backupResticBinaryPresent"
              @click="initResticRepo"
            />
            <Button
              label="Jetzt sichern"
              icon="pi pi-cloud-upload"
              :loading="backupRunLoading"
              @click="runBackupNow"
            />
          </div>
        </template>
      </template>
    </Card>

    <Dialog
      v-model:visible="showPasswordDialog"
      modal
      header="restic-Repository-Passwort"
      :style="{ width: 'min(520px, 95vw)' }"
    >
      <p class="muted small">{{ initMessage }}</p>
      <pre class="pw-block">{{ initPassword }}</pre>
      <p v-if="passwordClipboardCopied" class="muted small clipboard-hint">
        Passwort wurde in die Zwischenablage kopiert.
      </p>
      <p v-else class="muted small clipboard-hint">
        Passwort konnte nicht automatisch kopiert werden — bitte manuell kopieren.
      </p>
      <p class="muted small">Bitte sicher aufbewahren — wird nicht erneut angezeigt.</p>
      <template #footer>
        <Button label="Schließen" @click="showPasswordDialog = false" />
      </template>
    </Dialog>

    <BackupPathPickerDialog
      v-model:visible="pathPickerVisible"
      :initial-path="backupTargetPath"
      @select="onBackupPathSelected"
    />

    <AndroidApiPairingSection />
  </div>
</template>

<style scoped>
.page {
  width: 100%;
  max-width: min(1120px, 100%);
  margin-inline: auto;
  padding-inline: clamp(0.5rem, 2vw, 1.25rem);
  box-sizing: border-box;
}
.sec {
  margin-bottom: 1.75rem;
}
.h {
  margin: 0 0 0.5rem;
  font-size: 1rem;
}
.field {
  min-width: 220px;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
  margin: 0 0 0.75rem;
}
.tbl {
  margin-bottom: 0.5rem;
}
.panel {
  padding: 1rem;
  background: #f8fafc;
  border-radius: 8px;
  border: 1px solid #e2e8f0;
}
.panel-grid {
  display: grid;
  grid-template-columns: 140px 1fr;
  gap: 0.65rem 1rem;
  align-items: center;
}
.stamps-poll {
  margin-top: 1rem;
}
.lbl {
  font-size: 0.85rem;
  color: #475569;
}
.w-full {
  width: 100%;
}
.backup-card {
  margin-top: 1.25rem;
}
.backup-grid {
  margin-top: 0.75rem;
}
.interval-field {
  width: 100%;
}
.interval-hint {
  margin: 0.35rem 0 0;
}
.chk-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.chk-lbl {
  font-size: 0.9rem;
  color: #334155;
  cursor: pointer;
}
.btn-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 1rem;
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: 0.9rem;
}
.path-input-group {
  width: 100%;
}
.path-input {
  cursor: pointer;
}
.clipboard-hint {
  margin-top: 0.25rem;
}
.pw-block {
  margin: 0.75rem 0;
  padding: 0.75rem 1rem;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 6px;
  font-size: 0.85rem;
  word-break: break-all;
  white-space: pre-wrap;
}
.err {
  color: #b91c1c;
}
</style>
