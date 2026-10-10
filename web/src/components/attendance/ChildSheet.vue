<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import { useToast } from 'primevue/usetoast'

import {
  NOTICE_REASON_LABEL,
  ageLabel,
  childName,
  createChildNotice,
  deleteChildNotice,
  fetchChildNotices,
  noticeShort,
  nowClock,
  putAttendanceDay,
  rangeLabel,
  shortDay,
  updateChildNotice,
  type AttendanceChild,
  type ChildNotice,
  type ChildNoticeReason,
} from '@/api/attendance'
import { useNarrowViewport } from '@/composables/useNarrowViewport'
import { getApiErrorMessage } from '@/utils/apiError'

/** Ein Kind bearbeiten: Kommen/Gehen des gewählten Tages korrigieren und Fehlen vorab melden. */
const props = defineProps<{
  child: AttendanceChild | null
  groupName: string
  date: string
  today: string
}>()

const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [] }>()

const toast = useToast()
const narrow = useNarrowViewport()

const arrived = ref('')
const left = ref('')
const savingTimes = ref(false)

const notices = ref<ChildNotice[]>([])
const loadingNotices = ref(false)

type Kind = 'absent' | 'late' | 'early'
const editing = ref<ChildNotice | 'new' | null>(null)
const fReason = ref<ChildNoticeReason>('sick')
const fFrom = ref('')
const fTo = ref('')
const fKind = ref<Kind>('absent')
const fTime = ref('')
const fNote = ref('')
const savingNotice = ref(false)

const canEditTimes = computed(() => props.date <= props.today)
const isToday = computed(() => props.date === props.today)
const timesChanged = computed(
  () => (arrived.value || null) !== props.child?.arrived_at || (left.value || null) !== props.child?.left_at,
)

watch(
  () => [visible.value, props.child?.id] as const,
  async ([v]) => {
    if (!v || !props.child) return
    arrived.value = props.child.arrived_at ?? ''
    left.value = props.child.left_at ?? ''
    editing.value = null
    await loadNotices()
  },
  { immediate: true },
)

async function loadNotices() {
  if (!props.child) return
  loadingNotices.value = true
  try {
    notices.value = await fetchChildNotices(props.child.id)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Laden fehlgeschlagen', detail: getApiErrorMessage(e), life: 5000 })
  } finally {
    loadingNotices.value = false
  }
}

async function saveTimes() {
  if (!props.child) return
  savingTimes.value = true
  try {
    await putAttendanceDay(props.child.id, props.date, arrived.value || null, left.value || null)
    emit('changed')
    visible.value = false
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nicht gespeichert', detail: getApiErrorMessage(e), life: 5000 })
  } finally {
    savingTimes.value = false
  }
}

function clearTimes() {
  arrived.value = ''
  left.value = ''
}

function startNew() {
  editing.value = 'new'
  fReason.value = 'sick'
  fFrom.value = props.date < props.today ? props.today : props.date
  fTo.value = fFrom.value
  fKind.value = 'absent'
  fTime.value = ''
  fNote.value = ''
}

function startEdit(n: ChildNotice) {
  editing.value = n
  fReason.value = n.reason
  fFrom.value = n.date_from
  fTo.value = n.date_to
  fKind.value = n.arrive_from ? 'late' : n.leave_at ? 'early' : 'absent'
  fTime.value = n.arrive_from ?? n.leave_at ?? ''
  fNote.value = n.note
}

watch(fFrom, (v) => {
  if (v && (!fTo.value || fTo.value < v)) fTo.value = v
})

const noticeValid = computed(
  () => !!fFrom.value && !!fTo.value && fTo.value >= fFrom.value && (fKind.value === 'absent' || !!fTime.value),
)

async function saveNotice() {
  if (!props.child || !editing.value) return
  savingNotice.value = true
  const input = {
    reason: fReason.value,
    date_from: fFrom.value,
    date_to: fTo.value,
    arrive_from: fKind.value === 'late' ? fTime.value : null,
    leave_at: fKind.value === 'early' ? fTime.value : null,
    note: fNote.value,
  }
  try {
    if (editing.value === 'new') await createChildNotice(props.child.id, input)
    else await updateChildNotice(editing.value.id, input)
    editing.value = null
    await loadNotices()
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nicht gespeichert', detail: getApiErrorMessage(e), life: 5000 })
  } finally {
    savingNotice.value = false
  }
}

async function removeNotice(n: ChildNotice) {
  if (!window.confirm(`Meldung „${noticeShort(n)}“ (${rangeLabel(n.date_from, n.date_to)}) löschen?`)) return
  try {
    await deleteChildNotice(n.id)
    if (editing.value !== 'new' && editing.value?.id === n.id) editing.value = null
    await loadNotices()
    emit('changed')
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nicht gelöscht', detail: getApiErrorMessage(e), life: 5000 })
  }
}

const REASONS: ChildNoticeReason[] = ['sick', 'vacation', 'other']
</script>

<template>
  <Drawer
    v-model:visible="visible"
    :position="narrow ? 'bottom' : 'right'"
    class="child-sheet"
    :class="{ 'child-sheet--side': !narrow }"
    :show-close-icon="false"
    :block-scroll="true"
    :pt="{ root: { 'data-testid': 'child-sheet' } }"
  >
    <template #container="{ closeCallback }">
      <div v-if="child" class="sheet">
        <button v-if="narrow" type="button" class="grab" aria-label="Schließen" @click="closeCallback" />
        <div class="head">
          <div>
            <strong>{{ childName(child) }}</strong>
            <span class="sub"
              >{{ groupName }}<template v-if="ageLabel(child.birth_month, date)"> · {{ ageLabel(child.birth_month, date) }}</template>
              · {{ shortDay(date) }}<template v-if="isToday"> (heute)</template></span>
          </div>
          <button type="button" class="x" aria-label="Schließen" @click="closeCallback">
            <span class="pi pi-times" aria-hidden="true" />
          </button>
        </div>

        <section v-if="canEditTimes" class="block">
          <h3>Kommen und Gehen</h3>
          <div class="times">
            <label>
              <span>Gekommen</span>
              <div class="time-row">
                <input v-model="arrived" type="time" data-testid="sheet-arrived" />
                <button v-if="isToday" type="button" class="now" @click="arrived = nowClock()">Jetzt</button>
              </div>
            </label>
            <label>
              <span>Gegangen</span>
              <div class="time-row">
                <input v-model="left" type="time" :disabled="!arrived" data-testid="sheet-left" />
                <button v-if="isToday" type="button" class="now" :disabled="!arrived" @click="left = nowClock()">
                  Jetzt
                </button>
              </div>
            </label>
          </div>
          <div class="btns">
            <Button
              v-if="child.arrived_at || child.left_at"
              label="Zeiten löschen"
              severity="secondary"
              outlined
              size="small"
              :disabled="savingTimes"
              @click="clearTimes"
            />
            <Button
              label="Zeiten speichern"
              size="small"
              data-testid="sheet-save-times"
              :disabled="!timesChanged || savingTimes"
              :loading="savingTimes"
              @click="saveTimes"
            />
          </div>
        </section>

        <section class="block">
          <h3>Fehlen melden</h3>
          <p class="hint">Urlaub, Krankheit oder Termine, auch für künftige Tage. Auch „kommt später“ oder „wird früher abgeholt“.</p>
          <ul v-if="notices.length" class="notices">
            <li v-for="n in notices" :key="n.id" data-testid="sheet-notice">
              <div class="n-main">
                <b>{{ rangeLabel(n.date_from, n.date_to) }}</b>
                <span :class="['tag', `tag--${n.reason}`]">{{ noticeShort(n) }}</span>
                <small v-if="n.note">{{ n.note }}</small>
              </div>
              <button type="button" class="icon" aria-label="Bearbeiten" @click="startEdit(n)">
                <span class="pi pi-pencil" aria-hidden="true" />
              </button>
              <button type="button" class="icon" aria-label="Löschen" @click="removeNotice(n)">
                <span class="pi pi-trash" aria-hidden="true" />
              </button>
            </li>
          </ul>
          <p v-else-if="!loadingNotices && !editing" class="empty">Nichts gemeldet.</p>

          <div v-if="editing" class="form" data-testid="notice-form">
            <div class="seg" role="radiogroup" aria-label="Grund">
              <button
                v-for="r in REASONS"
                :key="r"
                type="button"
                :class="{ on: fReason === r }"
                :data-testid="`notice-reason-${r}`"
                @click="fReason = r"
              >
                {{ NOTICE_REASON_LABEL[r] }}
              </button>
            </div>
            <div class="dates">
              <label><span>Von</span><input v-model="fFrom" type="date" data-testid="notice-from" /></label>
              <label><span>Bis</span><input v-model="fTo" type="date" :min="fFrom" data-testid="notice-to" /></label>
            </div>
            <div class="seg" role="radiogroup" aria-label="Art">
              <button type="button" :class="{ on: fKind === 'absent' }" data-testid="notice-kind-absent" @click="fKind = 'absent'">
                Fehlt ganz
              </button>
              <button type="button" :class="{ on: fKind === 'late' }" data-testid="notice-kind-late" @click="fKind = 'late'">
                Kommt später
              </button>
              <button type="button" :class="{ on: fKind === 'early' }" data-testid="notice-kind-early" @click="fKind = 'early'">
                Früher abholen
              </button>
            </div>
            <label v-if="fKind !== 'absent'" class="one">
              <span>{{ fKind === 'late' ? 'Kommt ab' : 'Wird abgeholt um' }}</span>
              <input v-model="fTime" type="time" data-testid="notice-time" />
            </label>
            <label class="one">
              <span>Notiz (optional)</span>
              <input v-model="fNote" type="text" maxlength="500" placeholder="z. B. Arzttermin" />
              <small class="note-hint">Bitte keine Diagnosen oder Krankheiten eintragen.</small>
            </label>
            <div class="btns">
              <Button label="Abbrechen" severity="secondary" text size="small" @click="editing = null" />
              <Button
                label="Meldung speichern"
                size="small"
                data-testid="notice-save"
                :disabled="!noticeValid || savingNotice"
                :loading="savingNotice"
                @click="saveNotice"
              />
            </div>
          </div>
          <Button
            v-else
            label="Fehlen melden"
            icon="pi pi-plus"
            outlined
            size="small"
            class="add"
            data-testid="notice-add"
            @click="startNew"
          />
        </section>
      </div>
    </template>
  </Drawer>
</template>

<style scoped>
.sheet {
  padding: 0.5rem 1.1rem calc(1.25rem + env(safe-area-inset-bottom, 0px));
  max-height: 90dvh;
  overflow-y: auto;
}
.child-sheet--side .sheet {
  max-height: none;
  height: 100%;
  padding-top: 1.1rem;
}
.grab {
  display: block;
  width: 44px;
  height: 5px;
  border: none;
  border-radius: 3px;
  background: #cbd5e1;
  margin: 0.25rem auto 0.8rem;
  padding: 0;
  cursor: pointer;
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.5rem;
}
.head strong {
  display: block;
  font-size: 1.25rem;
}
.sub {
  font-size: 0.85rem;
  color: #64748b;
}
.x {
  border: none;
  background: #f1f5f9;
  border-radius: 999px;
  width: 2.2rem;
  height: 2.2rem;
  cursor: pointer;
  color: #475569;
}
.block {
  margin-top: 1.1rem;
  padding-top: 0.9rem;
  border-top: 1px solid #e2e8f0;
}
.block h3 {
  margin: 0 0 0.6rem;
  font-size: 0.95rem;
}
.note-hint {
  font-size: 0.75rem;
  color: #64748b;
}
.hint {
  margin: -0.3rem 0 0.6rem;
  font-size: 0.8rem;
  color: #64748b;
}
.times,
.dates {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.6rem;
}
label > span {
  display: block;
  font-size: 0.78rem;
  color: #475569;
  margin-bottom: 0.2rem;
}
.time-row {
  display: flex;
  gap: 0.3rem;
}
input[type='time'],
input[type='date'],
input[type='text'] {
  width: 100%;
  min-width: 0;
  font: inherit;
  font-size: 1rem;
  padding: 0.5rem 0.55rem;
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
}
input:disabled {
  background: #f8fafc;
  color: #94a3b8;
}
.now {
  border: 1px solid #cbd5e1;
  border-radius: 8px;
  background: #fff;
  font: inherit;
  font-size: 0.8rem;
  padding: 0 0.55rem;
  cursor: pointer;
}
.now:disabled {
  opacity: 0.5;
  cursor: default;
}
.btns {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 0.75rem;
}
.notices {
  list-style: none;
  margin: 0 0 0.6rem;
  padding: 0;
}
.notices li {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.45rem 0;
  border-bottom: 1px solid #f1f5f9;
}
.n-main {
  flex: 1;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.2rem 0.5rem;
  min-width: 0;
}
.n-main small {
  width: 100%;
  color: #64748b;
}
.icon {
  border: none;
  background: transparent;
  width: 2.2rem;
  height: 2.2rem;
  border-radius: 8px;
  color: #475569;
  cursor: pointer;
}
.icon:hover {
  background: #f1f5f9;
}
.tag {
  font-size: 0.75rem;
  border-radius: 999px;
  padding: 0.1rem 0.5rem;
  background: #f1f5f9;
  color: #334155;
}
.tag--sick {
  background: #fee2e2;
  color: #991b1b;
}
.tag--vacation {
  background: #e0f2fe;
  color: #075985;
}
.empty {
  font-size: 0.85rem;
  color: #94a3b8;
  margin: 0 0 0.6rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 0.75rem;
}
.seg {
  display: flex;
  background: #e2e8f0;
  border-radius: 10px;
  padding: 3px;
}
.seg button {
  flex: 1;
  border: none;
  background: transparent;
  border-radius: 8px;
  padding: 0.5rem 0.3rem;
  font: inherit;
  font-size: 0.85rem;
  color: #475569;
  cursor: pointer;
}
.seg button.on {
  background: #fff;
  color: #0f172a;
  font-weight: 600;
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.12);
}
.add {
  width: 100%;
}
</style>

<style>
.child-sheet.p-drawer-right {
  width: min(420px, 100vw);
}
</style>
