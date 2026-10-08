<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Button from 'primevue/button'
import { useToast } from 'primevue/usetoast'

import ChildSheet from '@/components/attendance/ChildSheet.vue'
import {
  addDays,
  childName,
  fetchAttendance,
  fetchAttendanceAccess,
  localYmd,
  noticeFullDay,
  noticeShort,
  nowClock,
  putAttendanceDay,
  shortDay,
  type AttendanceAccess,
  type AttendanceChild,
  type AttendanceDay,
} from '@/api/attendance'
import { getApiErrorMessage } from '@/utils/apiError'

const router = useRouter()
const toast = useToast()

const access = ref<AttendanceAccess | null>(null)
const day = ref<AttendanceDay | null>(null)
const loading = ref(false)
const loadError = ref('')
const date = ref(localYmd())
const today = computed(() => day.value?.today ?? access.value?.today ?? localYmd())

/** 0 = alle Gruppen. Start immer in der eigenen Gruppe (Gruppenaccount bzw. zugewiesene Gruppe). */
const groupId = ref(0)

type Filter = 'all' | 'present' | 'expected' | 'gone' | 'absent'
const filter = ref<Filter>('all')

type Status = 'present' | 'expected' | 'gone' | 'absent'

function status(c: AttendanceChild): Status {
  if (c.left_at) return 'gone'
  if (c.arrived_at) return 'present'
  if (c.notice && noticeFullDay(c.notice)) return 'absent'
  return 'expected'
}

const visibleGroups = computed(() => {
  const groups = day.value?.groups ?? []
  if (groups.length <= 1 || !groupId.value) return groups
  const one = groups.filter((g) => g.id === groupId.value)
  return one.length ? one : groups
})

const allChildren = computed(() => visibleGroups.value.flatMap((g) => g.children))

const counts = computed(() => {
  const c = { all: 0, present: 0, expected: 0, gone: 0, absent: 0 }
  for (const ch of allChildren.value) {
    c.all++
    c[status(ch)]++
  }
  return c
})

const FILTERS: { key: Filter; label: string }[] = [
  { key: 'all', label: 'Alle' },
  { key: 'present', label: 'Da' },
  { key: 'expected', label: 'Noch nicht da' },
  { key: 'gone', label: 'Gegangen' },
  { key: 'absent', label: 'Fehlt' },
]

function shown(c: AttendanceChild) {
  return filter.value === 'all' || status(c) === filter.value
}

const isToday = computed(() => date.value === today.value)
const isFuture = computed(() => date.value > today.value)
const multiGroup = computed(() => (day.value?.groups.length ?? 0) > 1)

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    day.value = await fetchAttendance(date.value)
    loadError.value = ''
  } catch (e) {
    loadError.value = getApiErrorMessage(e) ?? 'Laden fehlgeschlagen'
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    access.value = await fetchAttendanceAccess()
    date.value = access.value.today
    groupId.value = access.value.default_group_id
  } catch (e) {
    loadError.value = getApiErrorMessage(e) ?? 'Kein Zugriff'
    return
  }
  await load()
})

watch(date, () => void load())

// Mehrere Geräte tragen gleichzeitig ein (Flur-Tablet, Handys): regelmäßig nachladen.
const sheetOpen = ref(false)
let timer: number | undefined
onMounted(() => {
  timer = window.setInterval(() => {
    if (!sheetOpen.value && document.visibilityState === 'visible' && day.value) void load(true)
  }, 30_000)
})
onUnmounted(() => window.clearInterval(timer))

function shiftDay(n: number) {
  date.value = addDays(date.value, n)
}

const dateLabel = computed(() => {
  if (isToday.value) return `Heute, ${shortDay(date.value)}`
  if (date.value === addDays(today.value, 1)) return `Morgen, ${shortDay(date.value)}`
  if (date.value === addDays(today.value, -1)) return `Gestern, ${shortDay(date.value)}`
  return shortDay(date.value)
})

const sheetChild = ref<AttendanceChild | null>(null)
const sheetGroupName = ref('')
function openSheet(c: AttendanceChild) {
  sheetChild.value = c
  sheetGroupName.value = day.value?.groups.find((g) => g.id === c.group_id)?.name ?? ''
  sheetOpen.value = true
}

/** Letzte Schnellaktion zum Rückgängigmachen. */
const undo = ref<{ child: AttendanceChild; label: string; prev: [string | null, string | null] } | null>(null)
let undoTimer: number | undefined
const busy = ref<number | null>(null)

async function quick(c: AttendanceChild) {
  if (!isToday.value) {
    openSheet(c)
    return
  }
  const prev: [string | null, string | null] = [c.arrived_at, c.left_at]
  const now = nowClock()
  const next: [string | null, string | null] = c.arrived_at ? [c.arrived_at, now] : [now, null]
  busy.value = c.id
  try {
    await putAttendanceDay(c.id, date.value, next[0], next[1])
    c.arrived_at = next[0]
    c.left_at = next[1]
    undo.value = { child: c, label: `${c.first_name}: ${c.left_at ? 'gegangen' : 'gekommen'} ${now}`, prev }
    window.clearTimeout(undoTimer)
    undoTimer = window.setTimeout(() => (undo.value = null), 8000)
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Nicht gespeichert', detail: getApiErrorMessage(e), life: 5000 })
  } finally {
    busy.value = null
  }
}

async function doUndo() {
  const u = undo.value
  if (!u) return
  undo.value = null
  try {
    await putAttendanceDay(u.child.id, date.value, u.prev[0], u.prev[1])
    u.child.arrived_at = u.prev[0]
    u.child.left_at = u.prev[1]
  } catch (e) {
    toast.add({ severity: 'error', summary: 'Rückgängig fehlgeschlagen', detail: getApiErrorMessage(e), life: 5000 })
  }
}

function statusText(c: AttendanceChild): string {
  switch (status(c)) {
    case 'present':
      return `da seit ${c.arrived_at}`
    case 'gone':
      return `${c.arrived_at} – ${c.left_at}`
    case 'absent':
      return c.notice ? noticeShort(c.notice) : 'fehlt'
    default:
      return isFuture.value ? 'erwartet' : 'noch nicht da'
  }
}

/** Hinweis bei teilweisem Fehlen (kommt später, früher abholen). */
function partialHint(c: AttendanceChild): string {
  if (!c.notice || noticeFullDay(c.notice)) return ''
  return noticeShort(c.notice)
}

function actionLabel(c: AttendanceChild): string {
  return c.arrived_at ? 'Gegangen' : 'Gekommen'
}
</script>

<template>
  <div class="att">
    <div class="bar">
      <div class="date-nav">
        <button type="button" class="nav-btn" aria-label="Tag zurück" data-testid="att-prev" @click="shiftDay(-1)">
          <span class="pi pi-chevron-left" aria-hidden="true" />
        </button>
        <label class="date-label">
          <span data-testid="att-date">{{ dateLabel }}</span>
          <input v-model="date" type="date" aria-label="Tag wählen" class="date-input" />
        </label>
        <button type="button" class="nav-btn" aria-label="Tag vor" data-testid="att-next" @click="shiftDay(1)">
          <span class="pi pi-chevron-right" aria-hidden="true" />
        </button>
        <button v-if="!isToday" type="button" class="today-btn today-inline" @click="date = today">Heute</button>
      </div>
      <div class="bar-right">
        <button v-if="!isToday" type="button" class="today-btn today-side" @click="date = today">Heute</button>
        <Button
          v-if="access?.can_manage"
          icon="pi pi-users"
          label="Kinder verwalten"
          text
          size="small"
          class="manage-btn"
          @click="router.push({ name: 'attendance-manage' })"
        />
        <Button
          icon="pi pi-exclamation-triangle"
          label="Evakuierung"
          severity="danger"
          size="small"
          data-testid="att-evacuation"
          @click="router.push({ name: 'attendance-evacuation' })"
        />
      </div>
    </div>

    <div v-if="multiGroup" class="groups" role="tablist" aria-label="Gruppe">
      <button
        v-for="g in day?.groups ?? []"
        :key="g.id"
        type="button"
        role="tab"
        :aria-selected="groupId === g.id"
        :class="{ on: groupId === g.id }"
        :data-testid="`att-group-${g.id}`"
        @click="groupId = g.id"
      >
        {{ g.name }}
      </button>
      <button type="button" role="tab" :aria-selected="!groupId" :class="{ on: !groupId }" @click="groupId = 0">
        Alle Gruppen
      </button>
    </div>

    <div class="filters" role="tablist" aria-label="Filter">
      <button
        v-for="f in FILTERS"
        :key="f.key"
        type="button"
        role="tab"
        :aria-selected="filter === f.key"
        :class="['chip', `chip--${f.key}`, { on: filter === f.key }]"
        :data-testid="`att-filter-${f.key}`"
        @click="filter = f.key"
      >
        {{ f.label }} <b>{{ counts[f.key] }}</b>
      </button>
    </div>

    <p v-if="loadError" class="error">{{ loadError }}</p>
    <p v-else-if="loading && !day" class="muted">Lädt …</p>
    <p v-else-if="day && !allChildren.length" class="muted empty">
      Noch keine Kinder eingetragen.<template v-if="access?.can_manage"> Unter „Kinder verwalten“ anlegen.</template>
    </p>

    <section v-for="g in visibleGroups" :key="g.id" class="group">
      <h2 v-if="visibleGroups.length > 1 && g.children.length" class="group-title">
        {{ g.name }}
        <small>{{ g.children.filter((c) => status(c) === 'present').length }} da</small>
      </h2>
      <ul class="cards">
        <template v-for="c in g.children" :key="c.id">
          <li
            v-if="shown(c)"
            :class="['card', `card--${status(c)}`]"
            :data-testid="`att-child-${c.id}`"
            :data-status="status(c)"
          >
            <button type="button" class="card-main" :aria-label="`${childName(c)} bearbeiten`" @click="openSheet(c)">
              <span class="name">{{ childName(c) }}</span>
              <span class="state" data-testid="att-state">{{ statusText(c) }}</span>
              <span v-if="partialHint(c)" class="hint"><span class="pi pi-clock" aria-hidden="true" /> {{ partialHint(c) }}</span>
              <span v-if="c.upcoming" class="hint hint--soon">
                <span class="pi pi-calendar" aria-hidden="true" /> {{ c.upcoming === 1 ? '1 Meldung' : `${c.upcoming} Meldungen` }} demnächst
              </span>
            </button>
            <button
              v-if="!isFuture && status(c) !== 'gone'"
              type="button"
              :class="['act', c.arrived_at ? 'act--leave' : 'act--arrive', { 'act--soft': status(c) === 'absent' }]"
              :disabled="busy === c.id"
              :data-testid="`att-action-${c.id}`"
              @click="quick(c)"
            >
              <span :class="['pi', c.arrived_at ? 'pi-sign-out' : 'pi-sign-in']" aria-hidden="true" />
              {{ actionLabel(c) }}
            </button>
          </li>
        </template>
      </ul>
    </section>

    <div v-if="undo" class="undo" role="status" data-testid="att-undo">
      <span>{{ undo.label }}</span>
      <button type="button" @click="doUndo">Rückgängig</button>
    </div>

    <ChildSheet
      v-model:visible="sheetOpen"
      :child="sheetChild"
      :group-name="sheetGroupName"
      :date="date"
      :today="today"
      @changed="load(true)"
    />
  </div>
</template>

<style scoped>
.att {
  max-width: 1200px;
  margin: 0 auto;
  padding-bottom: 4.5rem;
}
.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.75rem;
}
.date-nav {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.nav-btn,
.today-btn {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 10px;
  height: 2.6rem;
  min-width: 2.6rem;
  cursor: pointer;
  color: #0f172a;
  font: inherit;
}
.today-btn {
  padding: 0 0.8rem;
  font-size: 0.85rem;
}
.date-label {
  position: relative;
  font-weight: 700;
  font-size: 1.05rem;
  padding: 0 0.4rem;
  /* Feste Breite, damit die Pfeile beim Blättern nicht springen („Heute“ vs. „Morgen“). */
  width: 12.5rem;
  text-align: center;
  white-space: nowrap;
  cursor: pointer;
}
.date-input {
  position: absolute;
  inset: 0;
  opacity: 0;
  width: 100%;
  cursor: pointer;
}
.bar-right {
  display: flex;
  gap: 0.4rem;
  align-items: center;
}
.today-side {
  display: none;
}
.groups {
  display: flex;
  gap: 0.35rem;
  overflow-x: auto;
  margin-bottom: 0.6rem;
  padding-bottom: 2px;
}
.groups button {
  border: 1px solid #cbd5e1;
  background: #fff;
  border-radius: 999px;
  padding: 0.5rem 1rem;
  font: inherit;
  font-size: 0.95rem;
  white-space: nowrap;
  cursor: pointer;
  color: #334155;
}
.groups button.on {
  background: #0f172a;
  border-color: #0f172a;
  color: #fff;
  font-weight: 600;
}
.filters {
  display: flex;
  gap: 0.35rem;
  overflow-x: auto;
  margin-bottom: 0.9rem;
  padding-bottom: 2px;
}
.chip {
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 10px;
  padding: 0.4rem 0.7rem;
  font: inherit;
  font-size: 0.85rem;
  white-space: nowrap;
  cursor: pointer;
  color: #475569;
}
.chip b {
  margin-left: 0.2rem;
  color: #0f172a;
}
.chip.on {
  border-color: #6366f1;
  background: #eef2ff;
  color: #3730a3;
}
.group-title {
  font-size: 1rem;
  margin: 1rem 0 0.5rem;
  color: #0f172a;
}
.group-title small {
  font-weight: 400;
  color: #64748b;
  margin-left: 0.4rem;
}
.cards {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 0.6rem;
}
.card {
  display: flex;
  align-items: stretch;
  background: #fff;
  border: 1px solid #e2e8f0;
  border-left: 5px solid #cbd5e1;
  border-radius: 12px;
  overflow: hidden;
  min-height: 4.4rem;
}
.card--present {
  border-left-color: #16a34a;
}
.card--gone {
  border-left-color: #94a3b8;
  background: #f8fafc;
}
.card--absent {
  border-left-color: #f59e0b;
  background: #fffbeb;
}
.card-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 0.15rem;
  border: none;
  background: transparent;
  text-align: left;
  padding: 0.6rem 0.8rem;
  font: inherit;
  cursor: pointer;
  color: inherit;
}
.name {
  font-weight: 700;
  font-size: 1.08rem;
  color: #0f172a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}
.card--gone .name {
  color: #64748b;
}
.state {
  font-size: 0.85rem;
  color: #475569;
  font-variant-numeric: tabular-nums;
}
.card--present .state {
  color: #15803d;
  font-weight: 600;
}
.card--absent .state {
  color: #b45309;
  font-weight: 600;
}
.hint {
  font-size: 0.78rem;
  color: #b45309;
}
.hint--soon {
  color: #64748b;
}
.act {
  flex: 0 0 auto;
  width: 7.4rem;
  border: none;
  font: inherit;
  font-weight: 700;
  font-size: 0.95rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  cursor: pointer;
}
.act .pi {
  font-size: 1.15rem;
}
.act--arrive {
  background: #16a34a;
  color: #fff;
}
.act--arrive:hover {
  background: #15803d;
}
.act--leave {
  background: #e0e7ff;
  color: #3730a3;
}
.act--leave:hover {
  background: #c7d2fe;
}
.act--soft {
  background: #fef3c7;
  color: #92400e;
}
.act:disabled {
  opacity: 0.6;
}
.undo {
  position: fixed;
  left: 50%;
  bottom: calc(1rem + env(safe-area-inset-bottom, 0px));
  transform: translateX(-50%);
  background: #0f172a;
  color: #f8fafc;
  border-radius: 12px;
  padding: 0.65rem 0.75rem 0.65rem 1rem;
  display: flex;
  align-items: center;
  gap: 1rem;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.3);
  z-index: 200;
  max-width: calc(100vw - 2rem);
}
.undo button {
  border: none;
  background: transparent;
  color: #a5b4fc;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
}
.muted {
  color: #64748b;
}
.empty {
  background: #fff;
  border: 1px dashed #cbd5e1;
  border-radius: 12px;
  padding: 1.5rem;
  text-align: center;
}
.error {
  color: #b91c1c;
}
@media (max-width: 600px) {
  .manage-btn :deep(.p-button-label) {
    display: none;
  }
  /* Handy: Pfeile links und rechts außen, Datum dazwischen. */
  .date-nav {
    width: 100%;
    gap: 0.5rem;
  }
  .date-label {
    flex: 1;
    width: auto;
  }
  .today-inline {
    display: none;
  }
  .today-side {
    display: inline-block;
  }
  .bar-right {
    margin-left: auto;
  }
  .cards {
    grid-template-columns: 1fr;
  }
  .act {
    width: 6.6rem;
  }
}
</style>
