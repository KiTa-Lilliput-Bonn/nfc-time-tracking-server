<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import QRCode from 'qrcode'
import { useToast } from 'primevue/usetoast'

import {
  fetchAndroidApiClients,
  fetchSettings,
  deleteAndroidApiClient,
  generateAndroidApiClient,
  putSetting,
  syncLanEmployeeIds,
  syncLanEmployeeIdsAll,
} from '@/api/admin'
import type {
  AndroidLanTargetSetting,
  ApiPairedClient,
  GenerateAndroidApiClientResponse,
  LanEmployeeSyncAllResult,
  LanEmployeeSyncResult,
  PairedLanDeviceRow,
} from '@/types/api'

const toast = useToast()

const devices = ref<PairedLanDeviceRow[]>([])
const loading = ref(false)
const linkPairingLoading = ref(false)
const apiTestUiKey = ref<'dialog' | string | null>(null)
const deletingClientId = ref<string | null>(null)
const savingDevices = ref(false)
const newLabel = ref('')
const qrDataUrl = ref('')
const qrDialogVisible = ref(false)
const lastPayloadJson = ref('')
const qrDialogDeviceId = ref<string | null>(null)
let pairPollTimer: ReturnType<typeof setInterval> | undefined

function stopPairPoll() {
  if (pairPollTimer !== undefined) {
    clearInterval(pairPollTimer)
    pairPollTimer = undefined
  }
}

function startPairPoll() {
  stopPairPoll()
  const deviceId = qrDialogDeviceId.value
  if (!deviceId || !qrDialogVisible.value) return
  pairPollTimer = setInterval(async () => {
    try {
      await reloadDevices()
      const row = devices.value.find((d) => d.id === deviceId)
      if (row?.host.trim()) {
        stopPairPoll()
        toast.add({
          severity: 'success',
          summary: 'App verbunden',
          detail: `LAN-Adresse ${row.host}:${row.port} wurde automatisch eingetragen.`,
          life: 10000,
        })
      }
    } catch {
      /* ignore transient poll errors */
    }
  }, 2500)
}

watch(qrDialogVisible, (visible) => {
  if (visible) startPairPoll()
  else stopPairPoll()
})

const devColHeader = {
  label: { width: '14%', minWidth: '8rem' },
  host: { width: '32%', minWidth: '12rem' },
  port: { width: '5.5rem', minWidth: '5.5rem', maxWidth: '6rem' },
  secret: { width: '8%', minWidth: '5rem' },
  status: { width: '8%', minWidth: '5rem' },
  actions: { width: '28%', minWidth: '12rem' },
} as const
const devColBody = {
  label: { width: '14%', minWidth: '8rem', verticalAlign: 'middle' },
  host: { width: '32%', minWidth: '12rem', verticalAlign: 'middle' },
  port: { width: '5.5rem', minWidth: '5.5rem', maxWidth: '6rem', verticalAlign: 'middle' },
  secret: { width: '8%', minWidth: '5rem', verticalAlign: 'middle' },
  status: { width: '8%', minWidth: '5rem', verticalAlign: 'middle' },
  actions: { width: '28%', minWidth: '12rem', verticalAlign: 'middle' },
} as const

const syncLoadingId = ref<string | null>(null)
const syncAllLoading = ref(false)
const syncResult = ref<LanEmployeeSyncResult | null>(null)
const syncAllResult = ref<LanEmployeeSyncAllResult | null>(null)

const devicesWithHost = computed(() => devices.value.filter((d) => d.host.trim()))

const pairingUrlFromPayload = computed(() => {
  try {
    const o = JSON.parse(lastPayloadJson.value || '{}') as { u?: string }
    return o.u?.trim() || '—'
  } catch {
    return '—'
  }
})

function activeDevice(d: PairedLanDeviceRow) {
  return !d.revoked_at_utc
}

function settingVal(settings: { key: string; value: string }[], key: string, fallback: string) {
  return settings.find((s) => s.key === key)?.value ?? fallback
}

function parseTargetsJson(raw: string): AndroidLanTargetSetting[] {
  try {
    const v = JSON.parse(raw || '[]') as unknown
    if (!Array.isArray(v)) return []
    const out: AndroidLanTargetSetting[] = []
    for (const x of v) {
      if (!x || typeof x !== 'object') continue
      const o = x as Record<string, unknown>
      const id = String(o.id ?? '').trim()
      const host = String(o.host ?? '').trim()
      const apiClientId = String(o.api_client_id ?? '').trim()
      let port = 8787
      if (typeof o.port === 'number' && Number.isFinite(o.port)) port = Math.trunc(o.port)
      else if (typeof o.port === 'string') {
        const p = parseInt(o.port, 10)
        if (Number.isFinite(p)) port = p
      }
      const label = String(o.label ?? '').trim()
      if (!id) continue
      out.push({ id, host, port, api_client_id: apiClientId, label })
    }
    return out
  } catch {
    return []
  }
}

function findTargetForClient(
  targets: AndroidLanTargetSetting[],
  clientId: string,
): AndroidLanTargetSetting | undefined {
  const byId = targets.find((t) => t.id === clientId)
  if (byId) return byId
  return targets.find((t) => t.api_client_id === clientId)
}

function mergeDevices(clients: ApiPairedClient[], targets: AndroidLanTargetSetting[]): PairedLanDeviceRow[] {
  return clients.map((c) => {
    const t = findTargetForClient(targets, c.id)
    const label = (c.label?.trim() || t?.label?.trim() || '').trim()
    return {
      id: c.id,
      label,
      host: t?.host?.trim() ?? '',
      port: t?.port ?? 8787,
      secret: c.secret ?? '',
      created_at_utc: c.created_at_utc,
      revoked_at_utc: c.revoked_at_utc,
    }
  })
}

function targetsPayloadFromDevices(list: PairedLanDeviceRow[]): AndroidLanTargetSetting[] {
  return list
    .filter((d) => d.host.trim())
    .map((d) => ({
      id: d.id,
      host: d.host.trim(),
      port: d.port,
      api_client_id: d.id,
      label: d.label?.trim() ?? '',
    }))
}

function validateDeviceRow(d: PairedLanDeviceRow, summary: string): boolean {
  if (!d.id.trim()) {
    toast.add({ severity: 'warn', summary, detail: 'Geräte-ID fehlt.', life: 8000 })
    return false
  }
  if (!d.host.trim()) {
    toast.add({ severity: 'warn', summary, detail: 'Bitte Host/IP ausfüllen.', life: 8000 })
    return false
  }
  if (d.port == null || d.port < 1 || d.port > 65535) {
    toast.add({ severity: 'warn', summary, detail: 'Port muss zwischen 1 und 65535 liegen.', life: 8000 })
    return false
  }
  if (!d.secret.trim()) {
    toast.add({ severity: 'warn', summary, detail: 'Kein API-Secret für dieses Gerät.', life: 8000 })
    return false
  }
  if (!activeDevice(d)) {
    toast.add({ severity: 'warn', summary, detail: 'Gerät ist gesperrt.', life: 8000 })
    return false
  }
  return true
}

type SaveDevicesOpts = { quiet?: boolean; requireHostForId?: string }

async function saveDevices(opts?: SaveDevicesOpts): Promise<boolean> {
  const summary = 'Gepaarte Geräte'
  if (opts?.requireHostForId) {
    const row = devices.value.find((d) => d.id === opts.requireHostForId)
    if (!row || !validateDeviceRow(row, summary)) return false
  } else {
    for (const d of devices.value) {
      if (!d.host.trim()) continue
      if (!validateDeviceRow(d, summary)) return false
    }
  }

  const payload = targetsPayloadFromDevices(devices.value)
  savingDevices.value = true
  try {
    await putSetting('android_lan_targets', JSON.stringify(payload))
    if (!opts?.quiet) {
      toast.add({ severity: 'success', summary, detail: 'Gespeichert.', life: 8000 })
    }
    await reloadDevices()
    return true
  } catch {
    toast.add({ severity: 'error', summary, detail: 'Speichern fehlgeschlagen.', life: 10000 })
    return false
  } finally {
    savingDevices.value = false
  }
}

async function reloadDevices() {
  const [settings, clientList] = await Promise.all([fetchSettings(), fetchAndroidApiClients()])
  const targets = parseTargetsJson(settingVal(settings, 'android_lan_targets', '[]'))
  devices.value = mergeDevices(clientList ?? [], targets)
}

async function refreshAll() {
  loading.value = true
  try {
    await reloadDevices()
  } catch {
    toast.add({ severity: 'error', summary: 'Android-API', detail: 'Laden fehlgeschlagen.', life: 10000 })
  } finally {
    loading.value = false
  }
}

async function runLanEmployeeSyncForTarget(targetId: string) {
  if (!targetId) return
  syncLoadingId.value = targetId
  syncResult.value = null
  syncAllResult.value = null
  try {
    const saved = await saveDevices({ quiet: true, requireHostForId: targetId })
    if (!saved) return
    const res = await syncLanEmployeeIds(targetId)
    syncResult.value = res
    const n = res.created?.length ?? 0
    const u = res.updated?.length ?? 0
    const skip = res.skipped_already_in_app?.length ?? 0
    const rm = res.removed_from_app?.length ?? 0
    const f = res.failures?.length ?? 0
    toast.add({
      severity: f ? 'warn' : 'success',
      summary: 'App-Mitarbeiter',
      detail:
        `${n} neu, ${u} aktualisiert, ${skip} unverändert` +
        (rm ? `, ${rm} in App entfernt (deaktiviert)` : '') +
        (f ? `, ${f} Fehler` : '.'),
      life: 10000,
    })
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Synchronisation',
      detail: 'Server konnte die App-LAN-API nicht erreichen oder die Anfrage schlug fehl.',
      life: 12000,
    })
  } finally {
    syncLoadingId.value = null
  }
}

async function runLanEmployeeSyncAll() {
  if (!devicesWithHost.value.length) return
  syncAllLoading.value = true
  syncResult.value = null
  syncAllResult.value = null
  try {
    const saved = await saveDevices({ quiet: true })
    if (!saved) return
    const res = await syncLanEmployeeIdsAll()
    syncAllResult.value = res
    const s = res.summary
    toast.add({
      severity: s.failures_total ? 'warn' : 'success',
      summary: 'Alle Apps',
      detail:
        `${s.targets} Ziel(e): ${s.created_total} neu, ${s.updated_total} aktualisiert, ${s.skipped_total} unverändert` +
        ((s.removed_total ?? 0) ? `, ${s.removed_total} in App entfernt (deaktiviert)` : '') +
        (s.failures_total ? `, ${s.failures_total} Fehler` : '.'),
      life: 12000,
    })
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Synchronisation',
      detail: 'Mindestens ein Ziel konnte nicht abgeglichen werden.',
      life: 12000,
    })
  } finally {
    syncAllLoading.value = false
  }
}

async function linkWithApp() {
  linkPairingLoading.value = true
  try {
    const generated: GenerateAndroidApiClientResponse = await generateAndroidApiClient({
      label: newLabel.value.trim(),
    })
    const client = generated.client
    if (!generated.pairing_token?.trim()) {
      toast.add({
        severity: 'error',
        summary: 'Android-API',
        detail: 'Pairing-Token fehlt in der Server-Antwort.',
        life: 10000,
      })
      return
    }
    toast.add({
      severity: 'success',
      summary: 'Android-API',
      detail: 'QR erzeugt. In der App unter LAN-API → QR scannen; Host/Port werden automatisch übernommen.',
      life: 10000,
    })
    await reloadDevices()
    const existing = devices.value.find((d) => d.id === client.id)
    if (!existing) {
      devices.value = [
        ...devices.value,
        {
          id: client.id,
          label: client.label?.trim() ?? '',
          host: '',
          port: 8787,
          secret: client.secret,
          created_at_utc: client.created_at_utc,
          revoked_at_utc: client.revoked_at_utc,
        },
      ]
    }
    qrDialogDeviceId.value = client.id
    const pairingBaseUrl = (generated.pairing_base_url || '').trim()
    const isLoopbackUrl =
      !pairingBaseUrl ||
      /^https?:\/\/(localhost|127\.0\.0\.1)(:\d+)?\/?$/i.test(pairingBaseUrl) ||
      /^https?:\/\/\[::1\](:\d+)?\/?$/i.test(pairingBaseUrl)
    if (isLoopbackUrl) {
      toast.add({
        severity: 'error',
        summary: 'Server-URL',
        detail:
          'Keine LAN-Adresse für den QR-Code ermittelt. Server neu starten, Netzwerk prüfen oder in config.yaml pairing_advertise_host setzen (z. B. 192.168.1.10).',
        life: 15000,
      })
      return
    }
    const payload = {
      t: generated.pairing_token,
      c: client.id,
      n: client.label || '',
      u: pairingBaseUrl,
    }
    lastPayloadJson.value = JSON.stringify(payload)
    qrDataUrl.value = await QRCode.toDataURL(lastPayloadJson.value, { margin: 2, width: 260 })
    qrDialogVisible.value = true
    newLabel.value = ''
  } catch {
    toast.add({ severity: 'error', summary: 'Android-API', detail: 'Verknüpfung fehlgeschlagen.', life: 10000 })
  } finally {
    linkPairingLoading.value = false
  }
}

async function removeDevice(id: string) {
  if (!confirm('Dieses Gerät endgültig löschen? Es verliert den Server-Zugriff und das LAN-Ziel wird entfernt.')) return
  deletingClientId.value = id
  try {
    await deleteAndroidApiClient(id)
    devices.value = devices.value.filter((d) => d.id !== id)
    await saveDevices({ quiet: true })
    toast.add({ severity: 'success', summary: 'Gepaarte Geräte', detail: 'Gerät gelöscht.', life: 8000 })
  } catch {
    toast.add({ severity: 'error', summary: 'Gepaarte Geräte', detail: 'Löschen fehlgeschlagen.', life: 10000 })
    await reloadDevices()
  } finally {
    deletingClientId.value = null
  }
}

function deviceById(id: string | null): PairedLanDeviceRow | null {
  if (!id) return null
  return devices.value.find((d) => d.id === id) ?? null
}

function buildLanStampsUrl(row: Pick<PairedLanDeviceRow, 'host' | 'port'>): string | null {
  const host = row.host.trim()
  const port = row.port
  if (!host || port == null || port < 1 || port > 65535) return null
  return `http://${host}:${port}/v1/stamps`
}

async function testStamps(secret: string, uiKey: 'dialog' | string, row: PairedLanDeviceRow) {
  const url = buildLanStampsUrl(row)
  if (!url || !secret.trim()) {
    toast.add({
      severity: 'warn',
      summary: 'API-Test',
      detail: 'Bitte Host und Port für dieses Gerät eintragen.',
      life: 12000,
    })
    return
  }
  const hostPort = `${row.host.trim()}:${row.port}`
  if (apiTestUiKey.value !== null) {
    toast.add({
      severity: 'info',
      summary: 'API-Test',
      detail: 'Es läuft bereits ein Test — bitte warten.',
      life: 4000,
    })
    return
  }
  apiTestUiKey.value = uiKey
  const ac = new AbortController()
  const timeoutMs = 10_000
  let tid: number | undefined

  const fetchP = fetch(url, {
    headers: { Authorization: `Bearer ${secret.trim()}` },
    signal: ac.signal,
  })
  const timeoutP = new Promise<Response>((_, reject) => {
    tid = window.setTimeout(() => {
      ac.abort()
      reject(new DOMException('Timeout', 'TimeoutError'))
    }, timeoutMs)
  })

  try {
    const res = await Promise.race([fetchP, timeoutP])
    if (res.ok) {
      toast.add({
        severity: 'success',
        summary: 'API-Test',
        detail: `GET ${url} → ${res.status}`,
        life: 12000,
      })
    } else {
      toast.add({
        severity: 'error',
        summary: 'API-Test',
        detail: `${hostPort}: HTTP ${res.status}`,
        life: 12000,
      })
    }
  } catch (err: unknown) {
    const timedOut = err instanceof DOMException && err.name === 'TimeoutError'
    const aborted =
      !timedOut &&
      ((err instanceof DOMException && err.name === 'AbortError') ||
        (err instanceof Error && err.name === 'AbortError'))
    toast.add({
      severity: 'error',
      summary: 'API-Test',
      detail:
        timedOut || aborted
          ? `Keine Antwort innerhalb von ${timeoutMs / 1000} Sekunden (${hostPort}).`
          : 'Netzwerkfehler oder CORS (Browser ↔ Gerät).',
      life: 12000,
    })
  } finally {
    if (tid !== undefined) window.clearTimeout(tid)
    void fetchP.catch(() => {})
    if (apiTestUiKey.value === uiKey) apiTestUiKey.value = null
  }
}

function testDialogDevice() {
  const row = deviceById(qrDialogDeviceId.value)
  if (!row) {
    toast.add({ severity: 'warn', summary: 'API-Test', detail: 'Gerät nicht gefunden.', life: 8000 })
    return
  }
  void testStamps(row.secret, 'dialog', row)
}

onMounted(() => {
  void refreshAll()
})

onUnmounted(() => {
  stopPairPoll()
})
</script>

<template>
  <Card class="pair-card">
    <template #title>Android-API / LAN-Pairing</template>
    <template #content>
      <p class="muted small intro">
        Pro Gerät: <strong>API-Key</strong> (Secret) und <strong>LAN-Adresse</strong> (Host/Port) in einer Zeile.
        Der Server pollt im unter „Systemeinstellungen“ eingestellten Intervall
        <code>GET /v1/stamps</code> auf jedem gespeicherten Ziel. „Geräte speichern“ schreibt die LAN-Konfiguration nach
        <code>android_lan_targets</code> (1:1: Ziel-ID = Client-ID).
      </p>

      <div class="link-row">
        <div class="label-field">
          <label class="lbl" for="nfc-new-label">Bezeichnung (optional, Feld <code>n</code> im QR-JSON)</label>
          <InputText
            id="nfc-new-label"
            v-model="newLabel"
            class="w-full"
            placeholder="z. B. Büro-Tablet"
            autocomplete="off"
          />
        </div>
        <Button label="Mit der App verknüpfen" icon="pi pi-qrcode" :loading="linkPairingLoading" @click="linkWithApp" />
      </div>

      <p class="muted small">
        Der QR-Code enthält <code>t</code>, <code>c</code>, optional <code>n</code> und die Server-URL <code>u</code>.
        Die Admin-Oberfläche darf über <code>localhost</code> laufen — <code>u</code> wird vom Server als LAN-Adresse
        ermittelt. Nach dem Scan meldet die App ihre eigene IP/Port an und trägt sie hier automatisch ein.
      </p>

      <div v-if="qrDataUrl && !qrDialogVisible" class="qr-reopen">
        <Button label="QR-Code anzeigen" icon="pi pi-qrcode" text size="small" @click="qrDialogVisible = true" />
      </div>

      <Dialog
        v-model:visible="qrDialogVisible"
        header="QR-Code für die App"
        modal
        :closable="true"
        :style="{ width: 'min(380px, 96vw)' }"
      >
        <div class="qr-dialog-body">
          <p class="muted small">
            In der App unter LAN-API mit <strong>QR scannen</strong> einlesen. Die Verbindung wird automatisch
            hergestellt; Host und Port erscheinen in der Tabelle, sobald die App sich gemeldet hat.
          </p>
          <img :src="qrDataUrl" alt="Pairing QR" class="qr-img" width="260" height="260" />
          <p v-if="lastPayloadJson" class="muted small server-url-line">
            Server-URL im QR (<code>u</code>): <strong>{{ pairingUrlFromPayload }}</strong>
          </p>
          <details class="payload-details">
            <summary class="muted small">JSON-Nutzlast</summary>
            <pre class="payload-pre">{{ lastPayloadJson }}</pre>
          </details>
        </div>
        <template #footer>
          <Button label="Schließen" severity="secondary" text @click="qrDialogVisible = false" />
          <Button
            label="API testen"
            icon="pi pi-cloud-download"
            :loading="apiTestUiKey === 'dialog'"
            :disabled="!deviceById(qrDialogDeviceId) || !buildLanStampsUrl(deviceById(qrDialogDeviceId)!)"
            @click="testDialogDevice"
          />
        </template>
      </Dialog>

      <section class="lan-sec">
        <div class="table-head">
          <h3 class="subh">Gepaarte Geräte</h3>
          <Button label="Aktualisieren" icon="pi pi-refresh" text size="small" :loading="loading" @click="refreshAll" />
        </div>
        <div class="tbl-actions">
          <Button
            type="button"
            label="Geräte speichern"
            icon="pi pi-save"
            :loading="savingDevices"
            :disabled="!devices.length"
            @click="saveDevices()"
          />
        </div>
        <div v-if="loading && !devices.length" class="muted">Laden…</div>
        <div v-else class="targets-table-wrap">
          <DataTable :value="devices ?? []" data-key="id" size="small" class="tbl targets-tbl">
            <Column
              field="label"
              header="Bezeichnung"
              header-class="col-label"
              :header-style="devColHeader.label"
              :body-style="devColBody.label"
            >
              <template #body="{ data }">
                <InputText v-model="data.label" class="cell-input" placeholder="optional" />
              </template>
            </Column>
            <Column
              field="host"
              header="Host / IP"
              header-class="col-host"
              :header-style="devColHeader.host"
              :body-style="devColBody.host"
            >
              <template #body="{ data }">
                <InputText v-model="data.host" class="cell-input" placeholder="192.168.…" />
              </template>
            </Column>
            <Column
              field="port"
              header="Port"
              header-class="col-port"
              :header-style="devColHeader.port"
              :body-style="devColBody.port"
            >
              <template #body="{ data }">
                <InputNumber v-model="data.port" :min="1" :max="65535" :use-grouping="false" class="cell-input" />
              </template>
            </Column>
            <Column
              header="Secret"
              header-class="col-secret"
              :header-style="devColHeader.secret"
              :body-style="devColBody.secret"
            >
              <template #body="{ data }">
                <span class="secret-mask">{{ data.secret ? '••••••••' : '—' }}</span>
              </template>
            </Column>
            <Column
              header="Status"
              header-class="col-status"
              :header-style="devColHeader.status"
              :body-style="devColBody.status"
            >
              <template #body="{ data }">
                <span :class="activeDevice(data) ? 'ok' : 'revoked'">
                  {{ activeDevice(data) ? 'aktiv' : 'gesperrt' }}
                </span>
              </template>
            </Column>
            <Column
              header=""
              header-class="col-actions"
              :header-style="devColHeader.actions"
              :body-style="devColBody.actions"
            >
              <template #body="{ data }">
                <div class="row-btns">
                  <Button
                    label="Abgleich"
                    size="small"
                    icon="pi pi-cloud-upload"
                    :loading="syncLoadingId === data.id"
                    :disabled="!data.id || !data.host?.trim() || !data.secret || !activeDevice(data)"
                    @click="runLanEmployeeSyncForTarget(data.id)"
                  />
                  <Button
                    label="API-Test"
                    size="small"
                    severity="secondary"
                    :loading="apiTestUiKey === `row-${data.id}`"
                    :disabled="!buildLanStampsUrl(data) || !data.secret"
                    @click="testStamps(data.secret, `row-${data.id}`, data)"
                  />
                  <Button
                    icon="pi pi-trash"
                    size="small"
                    severity="danger"
                    text
                    rounded
                    :loading="deletingClientId === data.id"
                    @click="removeDevice(data.id)"
                  />
                </div>
              </template>
            </Column>
          </DataTable>
        </div>
        <p v-if="!loading && !devices.length" class="muted small">
          Noch keine Geräte — „Mit der App verknüpfen“ nutzen.
        </p>
        <div class="sync-all-row">
          <Button
            label="Mit allen Apps abgleichen"
            icon="pi pi-cloud-upload"
            :loading="syncAllLoading"
            :disabled="!devicesWithHost.length"
            @click="runLanEmployeeSyncAll"
          />
        </div>
        <div v-if="syncResult" class="sync-out">
          <p class="muted small">
            <code>{{ syncResult.lan_base_url }}</code> — nach Sync:
            {{ syncResult.app_employee_ids_after?.length ?? 0 }} IDs in der App.
          </p>
          <DataTable
            v-if="(syncResult.updated?.length ?? 0) > 0"
            :value="syncResult.updated ?? []"
            size="small"
            class="tbl"
            data-key="employee_id"
          >
            <Column header="Aktualisiert">
              <template #body="{ data }">{{ data.name }} ({{ data.employee_id }}) — Tag {{ data.nfc_tag_uid }}</template>
            </Column>
          </DataTable>
          <DataTable
            v-if="(syncResult.removed_from_app?.length ?? 0) > 0"
            :value="syncResult.removed_from_app ?? []"
            size="small"
            class="tbl"
            data-key="employee_id"
          >
            <Column header="In App entfernt (Server deaktiviert)">
              <template #body="{ data }">ID {{ data.employee_id }} (User {{ data.user_id }})</template>
            </Column>
          </DataTable>
          <DataTable
            v-if="(syncResult.failures?.length ?? 0) > 0"
            :value="syncResult.failures ?? []"
            size="small"
            class="tbl"
          >
            <Column field="employee_id" header="Mitarbeiter-ID" />
            <Column field="error" header="Fehler" />
          </DataTable>
        </div>
        <div v-if="syncAllResult" class="sync-out">
          <p class="muted small">
            Letzter Mehrfach-Abgleich: {{ syncAllResult.results?.length ?? 0 }} Ergebnis(se).
          </p>
          <DataTable :value="syncAllResult.results ?? []" size="small" class="tbl" data-key="target_id">
            <Column field="target_id" header="Ziel" />
            <Column field="error" header="Fehler">
              <template #body="{ data }">{{ data.error || '—' }}</template>
            </Column>
            <Column header="Entfernt (deakt.)">
              <template #body="{ data }">{{ (data.removed_from_app?.length ?? 0) || '—' }}</template>
            </Column>
            <Column header="URL">
              <template #body="{ data }"><code class="small-code">{{ data.lan_base_url || '—' }}</code></template>
            </Column>
          </DataTable>
        </div>
      </section>
    </template>
  </Card>
</template>

<style scoped>
.pair-card {
  margin-top: 1.5rem;
}
.lan-sec {
  margin-top: 1rem;
  padding: 1rem;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 8px;
}
.tbl-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}
.targets-table-wrap {
  width: 100%;
  max-width: 100%;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}
.targets-tbl {
  margin-bottom: 0.75rem;
  width: 100%;
}
.targets-tbl :deep(.p-datatable-table) {
  table-layout: fixed;
  width: 100%;
}
.targets-tbl :deep(.p-datatable-thead > tr > th),
.targets-tbl :deep(.p-datatable-tbody > tr > td) {
  box-sizing: border-box;
}
.targets-tbl :deep(.p-datatable-tbody > tr > td) {
  min-width: 0;
}
.cell-input {
  width: 100%;
  min-width: 0;
  max-width: 100%;
  box-sizing: border-box;
}
.targets-tbl :deep(.cell-input.p-inputnumber) {
  width: 100%;
  min-width: 0;
  max-width: 100%;
}
.targets-tbl :deep(.cell-input .p-inputtext) {
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
}
@media (max-width: 640px) {
  .targets-tbl :deep(.row-btns) {
    justify-content: flex-start;
  }
}
.sync-all-row {
  margin-top: 0.75rem;
}
.small-code {
  font-size: 0.75rem;
  word-break: break-all;
}
.subh {
  margin: 0;
  font-size: 1rem;
}
.intro {
  margin-bottom: 1rem;
}
.muted {
  color: #64748b;
}
.small {
  font-size: 0.85rem;
  margin: 0 0 0.75rem;
}
.sync-out {
  margin-top: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid #bbf7d0;
}
.tbl {
  margin-top: 0.5rem;
}
.link-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 1rem;
  margin-bottom: 1rem;
}
.label-field {
  flex: 1;
  min-width: 220px;
}
.lbl {
  display: block;
  font-size: 0.85rem;
  color: #475569;
  margin-bottom: 0.35rem;
}
.w-full {
  width: 100%;
}
.qr-reopen {
  margin: 0 0 1rem;
}
.qr-dialog-body {
  padding-top: 0.25rem;
}
.qr-img {
  display: block;
  margin: 0.5rem auto;
}
.payload-details {
  margin: 0.5rem 0 0;
}
.payload-pre {
  margin: 0.5rem 0 0;
  padding: 0.75rem;
  font-size: 0.75rem;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  overflow-x: auto;
  word-break: break-all;
}
.table-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}
.secret-mask {
  letter-spacing: 0.05em;
}
.ok {
  color: #15803d;
  font-weight: 600;
}
.revoked {
  color: #b91c1c;
  font-weight: 600;
}
.row-btns {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  justify-content: flex-end;
}
code {
  background: #f1f5f9;
  padding: 0.1em 0.35em;
  border-radius: 4px;
  font-size: 0.9em;
}
</style>
