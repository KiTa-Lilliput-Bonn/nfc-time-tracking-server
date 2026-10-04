<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Dialog from 'primevue/dialog'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { useToast } from 'primevue/usetoast'

import { approveRequest, fetchRequests, rejectRequest } from '@/api/requests'
import { usePendingRequests } from '@/stores/pendingRequests'
import { useAuthStore } from '@/stores/auth'
import type { ChangeRequest } from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import {
  requestKindLabel,
  requestStatusLabel,
  requestStatusSeverity,
  requestSummary,
} from '@/utils/changeRequests'
import { formatGermanDateTime } from '@/utils/dates'

const toast = useToast()
const auth = useAuthStore()
const pendingStore = usePendingRequests()

const view = ref<'pending' | 'decided'>('pending')
const viewOptions = [
  { label: 'Offen', value: 'pending' },
  { label: 'Erledigt', value: 'decided' },
]
const requests = ref<ChangeRequest[]>([])
const loading = ref(true)
const err = ref('')
const busyId = ref<number | null>(null)

const rejectTarget = ref<ChangeRequest | null>(null)
const rejectComment = ref('')
const rejectVisible = computed({
  get: () => rejectTarget.value != null,
  set: (v: boolean) => {
    if (!v) rejectTarget.value = null
  },
})

const myId = computed(() => auth.user?.id ?? 0)

async function load() {
  loading.value = true
  err.value = ''
  try {
    requests.value = await fetchRequests(view.value)
  } catch {
    err.value = 'Anträge konnten nicht geladen werden.'
    requests.value = []
  } finally {
    loading.value = false
  }
  void pendingStore.refresh()
}

watch(view, () => void load())

async function approve(r: ChangeRequest) {
  busyId.value = r.id
  try {
    await approveRequest(r.id)
    toast.add({ severity: 'success', summary: 'Genehmigt', detail: `${requestKindLabel[r.kind]} für ${r.user_display_name} ist jetzt wirksam.`, life: 6000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Genehmigen nicht möglich', detail: getApiErrorMessage(e), life: 12000 })
  } finally {
    busyId.value = null
  }
}

function openReject(r: ChangeRequest) {
  rejectComment.value = ''
  rejectTarget.value = r
}

async function confirmReject() {
  const r = rejectTarget.value
  if (!r) return
  if (!rejectComment.value.trim()) {
    toast.add({ severity: 'warn', summary: 'Bitte einen Kommentar für die Ablehnung angeben.', life: 8000 })
    return
  }
  busyId.value = r.id
  try {
    await rejectRequest(r.id, rejectComment.value.trim())
    rejectTarget.value = null
    toast.add({ severity: 'success', summary: 'Abgelehnt', life: 6000 })
    await load()
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Ablehnen fehlgeschlagen', detail: getApiErrorMessage(e), life: 12000 })
  } finally {
    busyId.value = null
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <SelectButton v-model="view" :options="viewOptions" option-label="label" option-value="value" :allow-empty="false" />
    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="loading" class="muted">Laden…</p>
    <p v-else-if="!requests.length" class="muted">
      {{ view === 'pending' ? 'Keine offenen Anträge.' : 'Noch keine entschiedenen Anträge.' }}
    </p>
    <Card v-for="r in requests" v-else :key="r.id" class="req" data-testid="leitung-request">
      <template #content>
        <div class="head">
          <div>
            <strong>{{ r.user_display_name }}</strong>
            <span class="muted"> · {{ requestKindLabel[r.kind] }}</span>
          </div>
          <Tag :value="requestStatusLabel[r.status]" :severity="requestStatusSeverity[r.status]" />
        </div>
        <div class="summary">{{ requestSummary(r) }}</div>
        <div v-if="r.reason" class="reason">Grund: {{ r.reason }}</div>
        <div class="muted small">Gestellt am {{ formatGermanDateTime(r.created_at) }}</div>
        <div v-if="r.status !== 'pending'" class="muted small">
          {{ requestStatusLabel[r.status] }}
          <template v-if="r.decided_by_name"> von {{ r.decided_by_name }}</template>
          <template v-if="r.decided_at"> am {{ formatGermanDateTime(r.decided_at) }}</template>
          <div v-if="r.decision_comment" class="comment">„{{ r.decision_comment }}“</div>
        </div>
        <div v-if="r.status === 'pending'" class="actions">
          <template v-if="r.user_id === myId">
            <span class="muted small">Eigener Antrag, muss von einer anderen Leitung entschieden werden.</span>
          </template>
          <template v-else>
            <Button
              label="Ablehnen"
              severity="danger"
              outlined
              size="small"
              :disabled="busyId === r.id"
              data-testid="request-reject"
              @click="openReject(r)"
            />
            <Button
              label="Genehmigen"
              severity="success"
              size="small"
              :loading="busyId === r.id"
              data-testid="request-approve"
              @click="approve(r)"
            />
          </template>
        </div>
      </template>
    </Card>

    <Dialog v-model:visible="rejectVisible" header="Antrag ablehnen" modal :style="{ width: '440px', maxWidth: '95vw' }">
      <p v-if="rejectTarget" class="dlg-sub">
        {{ rejectTarget.user_display_name }} · {{ requestKindLabel[rejectTarget.kind] }}<br />
        {{ requestSummary(rejectTarget) }}
      </p>
      <label for="reject-comment" class="lbl">Kommentar für die Mitarbeiterin / den Mitarbeiter (Pflicht)</label>
      <Textarea id="reject-comment" v-model="rejectComment" rows="3" auto-resize class="w" data-testid="reject-comment" />
      <template #footer>
        <Button label="Abbrechen" severity="secondary" text @click="rejectTarget = null" />
        <Button
          label="Ablehnen"
          severity="danger"
          :disabled="!rejectComment.trim()"
          :loading="busyId === rejectTarget?.id"
          data-testid="reject-confirm"
          @click="confirmReject"
        />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 720px;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}
.summary {
  margin-top: 0.4rem;
  font-size: 1.05rem;
}
.reason {
  margin-top: 0.25rem;
  color: #334155;
}
.actions {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  margin-top: 0.75rem;
  flex-wrap: wrap;
}
.comment {
  font-style: italic;
  color: #334155;
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
.dlg-sub {
  margin: 0 0 0.75rem;
  color: #334155;
}
.lbl {
  display: block;
  font-size: 0.85rem;
  color: #64748b;
  margin-bottom: 0.4rem;
}
.w {
  width: 100%;
}
</style>
