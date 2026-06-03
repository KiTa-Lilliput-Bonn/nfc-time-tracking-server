<script setup lang="ts">
import type { CSSProperties } from 'vue'
import { computed, nextTick, onMounted, onUnmounted, ref, unref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Card from 'primevue/card'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import Editor from 'primevue/editor'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import { useToast } from 'primevue/usetoast'

import { fetchEmployees } from '@/api/employees'
import { fetchGroups } from '@/api/groups'
import {
  fetchScheduleExportDefaults,
  fetchScheduleExportExcel,
  importScheduleExcel,
  previewScheduleExcelImport,
  deleteTeamMeeting,
  postTeamMeeting,
  putScheduleWeekNotes,
  putTeamMeeting,
} from '@/api/management'
import { triggerDownload } from '@/api/exportReport'
import ScheduleGrid from '@/components/ScheduleGrid.vue'
import type {
  Employee,
  ScheduleExcelImportReport,
  ScheduleExcelPastPreview,
  TeamMeeting,
  TeamMeetingKind,
  UserGroup,
} from '@/types/api'
import { getApiErrorMessage } from '@/utils/apiError'
import {
  addDays,
  formatGermanDate,
  compareISOWeek,
  isoWeekAndYear,
  mondayOfISOWeek,
  parseTimeHHMM,
  plusOneHourHHMM,
  toISODateLocal,
} from '@/utils/dates'
import { teamMeetingListLabel } from '@/utils/teamMeetingLabel'
import { clearRouteQueryKeys, queryPositiveInt } from '@/utils/leitungDeepLink'

const toast = useToast()
const route = useRoute()
const router = useRouter()

const now = isoWeekAndYear(new Date())
const weekYear = ref(now.year)
const week = ref(now.week)

const employees = ref<Employee[]>([])
const groups = ref<UserGroup[]>([])
const loading = ref(true)

const weekNotes = ref('')
const notesSaving = ref(false)

/** Hinweis nach erfolgreichem Auto-Save kurz einblenden */
const notesSavedFlash = ref(false)
let notesSavedTimer: ReturnType<typeof setTimeout> | null = null

const NOTES_DEBOUNCE_MS = 800
let notesTimer: ReturnType<typeof setTimeout> | null = null

/** Kompaktes Notizen-Layout: muss exakt dieselbe Viewport-Regel wie CSS `@media (max-width: 960px)` nutzen (Firefox vs. Chrome bei innerWidth/visualViewport). */
const NOTES_COMPACT_MEDIA = '(max-width: 960px)'

function readNotesLayoutCompact(): boolean {
  if (typeof window === 'undefined') return false
  if (typeof window.matchMedia !== 'function') return window.innerWidth <= 960
  return window.matchMedia(NOTES_COMPACT_MEDIA).matches
}

function initialShowNotesInline(): boolean {
  if (typeof window === 'undefined') return true
  return !readNotesLayoutCompact()
}

const showNotesInline = ref(initialShowNotesInline())
const notesMobileDialogVisible = ref(false)

function syncNotesLayoutWidth() {
  if (typeof window === 'undefined') return
  const compact = readNotesLayoutCompact()
  showNotesInline.value = !compact
  if (!compact) notesMobileDialogVisible.value = false
  scheduleSyncNotesFabVisualViewport()
}

let notesCompactMq: MediaQueryList | null = null
function onNotesCompactMqChange() {
  syncNotesLayoutWidth()
}

let notesLayoutListenersBound = false

/** FAB an den sichtbaren Bereich (visualViewport) koppeln — verhindert Drift/Verschwinden beim Resizen (Firefox). */
const notesFabWrapStyle = ref<CSSProperties>({})
let notesFabVvRaf = 0
let notesFabVvListenersBound = false

function scheduleSyncNotesFabVisualViewport() {
  if (typeof window === 'undefined') return
  if (notesFabVvRaf !== 0) cancelAnimationFrame(notesFabVvRaf)
  notesFabVvRaf = requestAnimationFrame(() => {
    notesFabVvRaf = 0
    syncNotesFabVisualViewport()
  })
}

function syncNotesFabVisualViewport() {
  if (typeof window === 'undefined') return
  const base: CSSProperties = {
    position: 'fixed',
    zIndex: 1090,
    display: 'flex',
    alignItems: 'flex-end',
    justifyContent: 'flex-end',
    boxSizing: 'border-box',
    margin: 0,
    paddingTop: 0,
    paddingLeft: 'max(0px, env(safe-area-inset-left, 0px))',
    paddingRight: 'max(12px, env(safe-area-inset-right, 0px))',
    paddingBottom: 'max(12px, env(safe-area-inset-bottom, 0px))',
    pointerEvents: 'none',
  }
  const vv = window.visualViewport
  if (!vv || vv.width < 8) {
    notesFabWrapStyle.value = {
      ...base,
      left: 0,
      top: 0,
      width: '100%',
      height: `${window.innerHeight}px`,
    }
    return
  }
  notesFabWrapStyle.value = {
    ...base,
    left: `${vv.offsetLeft}px`,
    top: `${vv.offsetTop}px`,
    width: `${vv.width}px`,
    height: `${vv.height}px`,
  }
}

function bindNotesFabVisualViewportListeners() {
  if (typeof window === 'undefined' || notesFabVvListenersBound) return
  const vv = window.visualViewport
  if (vv) {
    vv.addEventListener('resize', scheduleSyncNotesFabVisualViewport)
    vv.addEventListener('scroll', scheduleSyncNotesFabVisualViewport)
  }
  window.addEventListener('resize', scheduleSyncNotesFabVisualViewport)
  notesFabVvListenersBound = true
}

function unbindNotesFabVisualViewportListeners() {
  if (typeof window === 'undefined' || !notesFabVvListenersBound) return
  const vv = window.visualViewport
  vv?.removeEventListener('resize', scheduleSyncNotesFabVisualViewport)
  vv?.removeEventListener('scroll', scheduleSyncNotesFabVisualViewport)
  window.removeEventListener('resize', scheduleSyncNotesFabVisualViewport)
  notesFabVvListenersBound = false
}
const scheduleGridRef = ref<InstanceType<typeof ScheduleGrid> | null>(null)
const scheduleAutosaveHint = ref('')

const excelInputRef = ref<HTMLInputElement | null>(null)
const excelImportBusy = ref(false)
const lastExcelReport = ref<ScheduleExcelImportReport | null>(null)
const excelPastImportDialogVisible = ref(false)
const pendingPastImportFile = ref<File | null>(null)
const excelPastImportPrompt = ref('')

const excelExportDialogVisible = ref(false)
const excelExportBusy = ref(false)
const exportFromYear = ref(now.year)
const exportFromWeek = ref(now.week)
const exportToYear = ref(now.year)
const exportToWeek = ref(now.week)

/** true, sobald „Bis“ manuell geändert wurde (Fehler nur dann anzeigen). */
const exportRangeEndManuallyEdited = ref(false)

function exportWeekRangeFromTo() {
  return {
    from: { year: exportFromYear.value, week: exportFromWeek.value },
    to: { year: exportToYear.value, week: exportToWeek.value },
  }
}

const exportWeekRangeInvalid = computed(() => {
  const { from, to } = exportWeekRangeFromTo()
  return compareISOWeek(to, from) < 0
})

const exportWeekRangeError = computed(() => {
  if (!exportWeekRangeInvalid.value || !exportRangeEndManuallyEdited.value) return null
  return 'End-KW liegt vor Start-KW.'
})

/** Gleiches Jahr: „Bis“-KW mit „Von“ mitziehen, wenn „Von“ darüber wächst. */
function autoBumpExportToWeekFromStart() {
  if (exportFromYear.value !== exportToYear.value) return
  if (exportFromWeek.value <= exportToWeek.value) return
  exportToWeek.value = exportFromWeek.value
}

watch([exportFromYear, exportFromWeek], () => {
  exportRangeEndManuallyEdited.value = false
  autoBumpExportToWeekFromStart()
})

watch([exportToYear, exportToWeek], () => {
  exportRangeEndManuallyEdited.value = true
})

const teamMeetingDialogVisible = ref(false)
/** 'create' = neue Sitzung anlegen; 'edit' = bestehende bearbeiten */
const teamMeetingDialogMode = ref<'edit' | 'create'>('edit')
const teamMeetingSaving = ref(false)
const selectedTeamMeetingId = ref<number | null>(null)
const meetingEditStart = ref('')
const meetingEditEnd = ref('')
const meetingEndManuallyEdited = ref(false)
const meetingParticipantIds = ref<number[]>([])
const meetingKindForCreate = ref<TeamMeetingKind>('kt')
const meetingDateForCreate = ref('')
const meetingLabelForCreate = ref('')
const meetingEditLabel = ref('')
const meetingEditDate = ref('')

const teamKindOptions = [
  { label: 'KT (Gruppenteam)', value: 'kt' as const },
  { label: 'GT (Gesamtteam)', value: 'gt' as const },
  { label: 'Sonstiges', value: 'other' as const },
]

const scheduleWeekdayShort = ['Mo', 'Di', 'Mi', 'Do', 'Fr'] as const

const meetingWeekdayOptions = computed(() => {
  const mon = mondayOfISOWeek(weekYear.value, week.value)
  return scheduleWeekdayShort.map((short, i) => {
    const iso = toISODateLocal(addDays(mon, i))
    return { label: `${short} · ${formatGermanDate(iso)}`, value: iso }
  })
})

const meetingMondayISO = computed(() => {
  const mon = mondayOfISOWeek(weekYear.value, week.value)
  return toISODateLocal(mon)
})

const meetingMondayLabel = computed(() => formatGermanDate(meetingMondayISO.value))

const teamMeetingWeekHint = computed(() => {
  const kw = `Kalenderwoche ${week.value} (${weekYear.value})`
  if (teamMeetingDialogMode.value === 'create' && meetingKindForCreate.value === 'other') {
    return `${kw} · Wochentag wählbar`
  }
  if (teamMeetingDialogMode.value === 'create') {
    return `${kw} · Montag ${meetingMondayLabel.value}`
  }
  return kw
})

const selectedTeamMeeting = computed(() =>
  weekTeamMeetings.value.find((x) => x.id === selectedTeamMeetingId.value),
)

const editingOtherMeeting = computed(
  () => selectedTeamMeeting.value?.kind === 'other' && selectedTeamMeeting.value?.source !== 'excel',
)

const teamMeetingDialogHeader = computed(() =>
  teamMeetingDialogMode.value === 'create' ? 'Teamsitzung hinzufügen' : 'Teamsitzung bearbeiten',
)

const teamMeetingSaveDisabled = computed(() => {
  if (teamMeetingDialogMode.value === 'create') return false
  return !weekTeamMeetings.value.length
})

const weekTeamMeetings = computed((): TeamMeeting[] => {
  const g = scheduleGridRef.value as null | {
    listTeamMeetings?: () => TeamMeeting[]
  }
  const fn = g?.listTeamMeetings
  if (typeof fn !== 'function') return []
  return fn() ?? []
})

const teamMeetingSelectOptions = computed(() =>
  weekTeamMeetings.value.map((m) => ({
    label: `${teamMeetingListLabel(m, formatGermanDate)} (#${m.id})`,
    value: m.id,
  })),
)

function syncMeetingFormFromSelection() {
  const id = selectedTeamMeetingId.value
  const m = weekTeamMeetings.value.find((x) => x.id === id)
  if (!m) {
    meetingEditStart.value = ''
    meetingEditEnd.value = ''
    meetingEndManuallyEdited.value = false
    meetingEditLabel.value = ''
    meetingEditDate.value = ''
    meetingParticipantIds.value = []
    return
  }
  meetingEditStart.value = m.time_start
  meetingEndManuallyEdited.value = false
  meetingEditEnd.value = m.time_end || (plusOneHourHHMM(meetingEditStart.value) ?? '')
  meetingEditLabel.value = m.label ?? ''
  meetingEditDate.value = m.meeting_date
  meetingParticipantIds.value = [...(m.user_ids ?? [])]
}

watch(teamMeetingDialogVisible, (v) => {
  if (!v) {
    teamMeetingDialogMode.value = 'edit'
    return
  }
  if (teamMeetingDialogMode.value === 'create') {
    selectedTeamMeetingId.value = null
    meetingEditStart.value = '09:00'
    meetingEndManuallyEdited.value = false
    meetingEditEnd.value = plusOneHourHHMM(meetingEditStart.value) ?? '10:00'
    meetingParticipantIds.value = []
    meetingKindForCreate.value = 'kt'
    meetingLabelForCreate.value = ''
    meetingDateForCreate.value = defaultMeetingDateForCreate()
    return
  }
  if (!weekTeamMeetings.value.length) {
    selectedTeamMeetingId.value = null
    meetingEditStart.value = ''
    meetingEditEnd.value = ''
    meetingEndManuallyEdited.value = false
    meetingParticipantIds.value = []
    return
  }
  if (!weekTeamMeetings.value.some((m) => m.id === selectedTeamMeetingId.value)) {
    selectedTeamMeetingId.value = weekTeamMeetings.value[0]?.id ?? null
  }
  syncMeetingFormFromSelection()
})

watch(selectedTeamMeetingId, () => {
  if (teamMeetingDialogVisible.value) syncMeetingFormFromSelection()
})

function toggleMeetingParticipant(uid: number, on: boolean) {
  const set = new Set(meetingParticipantIds.value)
  if (on) set.add(uid)
  else set.delete(uid)
  meetingParticipantIds.value = [...set].sort((a, b) => a - b)
}

/** Alle Mitarbeiter aus der Dienstplan-Liste (aktiv, ohne Superadmin) als Teilnehmer setzen. */
function isDefaultTeamMeetingParticipant(e: Employee): boolean {
  return e.default_team_meeting_participant !== false
}

function addAllMeetingParticipants() {
  mergeMeetingParticipantIds(
    scheduleEmployees.value.filter(isDefaultTeamMeetingParticipant).map((e) => e.id),
  )
}

/** Teilnehmerliste mit weiteren IDs vereinigen (ohne Duplikate). */
function mergeMeetingParticipantIds(ids: number[]) {
  const set = new Set(meetingParticipantIds.value)
  for (const id of ids) {
    if (id > 0) set.add(id)
  }
  meetingParticipantIds.value = [...set].sort((a, b) => a - b)
}

/** Alle Mitarbeiter einer Dienstplan-Sektion (Gruppe / „Ohne Gruppe“) zur Teilnehmerliste hinzufügen. */
function addMeetingParticipantsFromSection(employees: Employee[]) {
  mergeMeetingParticipantIds(
    employees.filter(isDefaultTeamMeetingParticipant).map((e) => e.id),
  )
}

function timeFieldOk(s: string): boolean {
  return parseTimeHHMM(s) != null
}

watch(meetingEditStart, (next) => {
  if (!teamMeetingDialogVisible.value || meetingEndManuallyEdited.value) return
  const auto = plusOneHourHHMM(next)
  if (auto) meetingEditEnd.value = auto
})

function onMeetingEndInput() {
  meetingEndManuallyEdited.value = true
}

function defaultMeetingDateForCreate(): string {
  const g = scheduleGridRef.value as null | { expandedDayIndex?: () => number | null }
  const dayIdx = g?.expandedDayIndex?.()
  if (typeof dayIdx === 'number' && dayIdx >= 0 && dayIdx < meetingWeekdayOptions.value.length) {
    return meetingWeekdayOptions.value[dayIdx]?.value ?? meetingMondayISO.value
  }
  return meetingMondayISO.value
}

async function saveTeamMeetingEdits() {
  if (!timeFieldOk(meetingEditStart.value) || !timeFieldOk(meetingEditEnd.value)) {
    toast.add({
      severity: 'warn',
      summary: 'Teamsitzung',
      detail: 'Beginn und Ende im Format HH:MM angeben.',
      life: 6000,
    })
    return
  }
  if (!meetingParticipantIds.value.length) {
    toast.add({
      severity: 'warn',
      summary: 'Teamsitzung',
      detail: 'Mindestens eine teilnehmende Person auswählen.',
      life: 6000,
    })
    return
  }
  if (teamMeetingDialogMode.value === 'create' && meetingKindForCreate.value === 'other') {
    if (!meetingLabelForCreate.value.trim()) {
      toast.add({
        severity: 'warn',
        summary: 'Teamsitzung',
        detail: 'Bezeichnung für Sonstiges angeben.',
        life: 6000,
      })
      return
    }
    if (!meetingDateForCreate.value) {
      toast.add({
        severity: 'warn',
        summary: 'Teamsitzung',
        detail: 'Wochentag auswählen.',
        life: 6000,
      })
      return
    }
  }
  if (teamMeetingDialogMode.value === 'edit' && editingOtherMeeting.value) {
    if (!meetingEditLabel.value.trim()) {
      toast.add({
        severity: 'warn',
        summary: 'Teamsitzung',
        detail: 'Bezeichnung für Sonstiges angeben.',
        life: 6000,
      })
      return
    }
    if (!meetingEditDate.value) {
      toast.add({
        severity: 'warn',
        summary: 'Teamsitzung',
        detail: 'Wochentag auswählen.',
        life: 6000,
      })
      return
    }
  }
  teamMeetingSaving.value = true
  try {
    if (teamMeetingDialogMode.value === 'create') {
      const base = {
        year: weekYear.value,
        week: week.value,
        kind: meetingKindForCreate.value,
        time_start: meetingEditStart.value.trim(),
        time_end: meetingEditEnd.value.trim(),
        user_ids: meetingParticipantIds.value,
      }
      await postTeamMeeting(
        meetingKindForCreate.value === 'other'
          ? {
              ...base,
              meeting_date: meetingDateForCreate.value,
              label: meetingLabelForCreate.value.trim(),
            }
          : base,
      )
      await scheduleGridRef.value?.reloadFromServer()
      toast.add({ severity: 'success', summary: 'Teamsitzung', detail: 'Angelegt.', life: 5000 })
      teamMeetingDialogVisible.value = false
      return
    }
    const id = selectedTeamMeetingId.value
    if (id == null) return
    const putBody = {
      time_start: meetingEditStart.value.trim(),
      time_end: meetingEditEnd.value.trim(),
      user_ids: meetingParticipantIds.value,
    }
    if (editingOtherMeeting.value) {
      await putTeamMeeting(id, {
        ...putBody,
        meeting_date: meetingEditDate.value,
        label: meetingEditLabel.value.trim(),
      })
    } else {
      await putTeamMeeting(id, putBody)
    }
    await scheduleGridRef.value?.reloadFromServer()
    toast.add({ severity: 'success', summary: 'Teamsitzung', detail: 'Gespeichert.', life: 5000 })
    teamMeetingDialogVisible.value = false
  } catch (err: unknown) {
    toast.add({
      severity: 'error',
      summary: 'Teamsitzung',
      detail: getApiErrorMessage(err) ?? 'Speichern fehlgeschlagen.',
      life: 8000,
    })
  } finally {
    teamMeetingSaving.value = false
  }
}

function teamMeetingLabel(m: TeamMeeting): string {
  return teamMeetingListLabel(m, formatGermanDate)
}

async function deleteSelectedTeamMeeting() {
  const id = selectedTeamMeetingId.value
  if (id == null) return
  const m = weekTeamMeetings.value.find((x) => x.id === id)
  const label = m ? teamMeetingLabel(m) : `Teamsitzung #${id}`
  if (!confirm(`„${label}“ wirklich löschen?`)) return
  teamMeetingSaving.value = true
  try {
    await deleteTeamMeeting(id)
    await scheduleGridRef.value?.reloadFromServer()
    toast.add({ severity: 'success', summary: 'Teamsitzung', detail: 'Gelöscht.', life: 5000 })
    if (!weekTeamMeetings.value.length) {
      teamMeetingDialogVisible.value = false
      return
    }
    if (!weekTeamMeetings.value.some((x) => x.id === selectedTeamMeetingId.value)) {
      selectedTeamMeetingId.value = weekTeamMeetings.value[0]?.id ?? null
    }
    syncMeetingFormFromSelection()
  } catch (err: unknown) {
    toast.add({
      severity: 'error',
      summary: 'Teamsitzung',
      detail: getApiErrorMessage(err) ?? 'Löschen fehlgeschlagen.',
      life: 8000,
    })
  } finally {
    teamMeetingSaving.value = false
  }
}

function openTeamMeetingCreateDialog() {
  teamMeetingDialogMode.value = 'create'
  teamMeetingDialogVisible.value = true
}

function openTeamMeetingEditDialog() {
  teamMeetingDialogMode.value = 'edit'
  teamMeetingDialogVisible.value = true
}

function openTeamMeetingEditForMeeting(id: number) {
  teamMeetingDialogMode.value = 'edit'
  selectedTeamMeetingId.value = id
  teamMeetingDialogVisible.value = true
}

const planActionsDisabled = computed(() => {
  const g = scheduleGridRef.value
  if (!g) return true
  return Boolean(unref(g.loading)) || Boolean(unref(g.copyBusy))
})

function excelPastSignalCount(p: ScheduleExcelPastPreview): number {
  return (
    (p.past_cells_skipped ?? 0) + (p.past_week_notes_skipped ?? 0) + (p.past_team_meetings_skipped ?? 0)
  )
}

function buildExcelPastImportPrompt(p: ScheduleExcelPastPreview): string {
  const parts: string[] = []
  const cells = p.past_cells_skipped ?? 0
  const notes = p.past_week_notes_skipped ?? 0
  const meetings = p.past_team_meetings_skipped ?? 0
  if (cells > 0) {
    parts.push(`${cells} vergangene Zelle${cells === 1 ? '' : 'n'} mit Schichten oder Abwesenheiten`)
  }
  if (notes > 0) {
    parts.push(`${notes} Wochennotiz${notes === 1 ? '' : 'en'} in vergangenen Kalenderwochen`)
  }
  if (meetings > 0) {
    parts.push(`${meetings} Teamsitzung${meetings === 1 ? '' : 'en'} in vergangenen Wochen`)
  }
  if (parts.length === 0) {
    return 'Die Datei enthält Daten in der Vergangenheit. Diese importieren?'
  }
  return `Die Datei enthält ${parts.join(', ')}. Sollen diese vergangenen Daten ebenfalls importiert werden?`
}

function summarizeExcelImport(rep: ScheduleExcelImportReport): string {
  const bits: string[] = []
  bits.push(`${rep.schedules_written} Schicht-Einträge`)
  if (rep.schedules_deleted) bits.push(`${rep.schedules_deleted} Schichten gelöscht`)
  bits.push(`${rep.absences_created} Abwesenheiten neu`)
  bits.push(`${rep.absences_replaced} Abwesenheiten ersetzt`)
  if (rep.week_notes_updated) bits.push(`${rep.week_notes_updated} Wochennotizen aktualisiert`)
  if ((rep.team_meetings_created ?? 0) > 0) {
    bits.push(`${rep.team_meetings_created} Teamsitzung(en)`)
  }
  if (rep.absences_skipped)
    bits.push(`${rep.absences_skipped} Feiertagstage übersprungen (je Werktagsspalte)`)
  if (rep.past_cells_skipped)
    bits.push(`${rep.past_cells_skipped} Vergangenheit ignoriert (nur ab heute)`)
  if (rep.past_week_notes_skipped)
    bits.push(`${rep.past_week_notes_skipped} Wochennotiz(en) in der Vergangenheit nicht gespeichert`)
  if (rep.past_team_meetings_skipped)
    bits.push(`${rep.past_team_meetings_skipped} Teamsitzung(en) in der Vergangenheit nicht importiert`)
  return bits.join(' · ')
}

function excelReportHasTeamMondaySections(rep: ScheduleExcelImportReport): boolean {
  for (const w of rep.weeks ?? []) {
    if ((w.team_monday_sections?.length ?? 0) > 0) return true
  }
  return false
}

function excelReportHasDetails(rep: ScheduleExcelImportReport): boolean {
  return (
    (rep.errors?.length ?? 0) > 0 ||
    (rep.warnings?.length ?? 0) > 0 ||
    (rep.unknown_names?.length ?? 0) > 0 ||
    (rep.past_cells_skipped ?? 0) > 0 ||
    (rep.past_week_notes_skipped ?? 0) > 0 ||
    (rep.past_team_meetings_skipped ?? 0) > 0 ||
    (rep.team_meetings_created ?? 0) > 0 ||
    excelReportHasTeamMondaySections(rep)
  )
}

function openExcelPicker() {
  excelInputRef.value?.click()
}

function isExcelFile(file: File): boolean {
  if (file.name.toLowerCase().endsWith('.xlsx')) return true
  return file.type === 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'
}

function firstExcelFromFileList(list: FileList | null | undefined): File | null {
  if (!list?.length) return null
  for (let i = 0; i < list.length; i++) {
    const f = list.item(i)
    if (f && isExcelFile(f)) return f
  }
  return null
}

function dragEventHasFiles(ev: DragEvent): boolean {
  return Boolean(ev.dataTransfer?.types?.includes('Files'))
}

const excelDropHighlight = ref(false)

function resetExcelDropHighlight() {
  excelDropHighlight.value = false
}

function onWindowDragEnd() {
  resetExcelDropHighlight()
}

function onExcelZoneDragEnter(ev: DragEvent) {
  if (!dragEventHasFiles(ev)) return
  if (planActionsDisabled.value || excelImportBusy.value) return
  ev.preventDefault()
  excelDropHighlight.value = true
}

function onExcelZoneDragLeave(ev: DragEvent) {
  if (!dragEventHasFiles(ev)) return
  const box = ev.currentTarget as HTMLElement
  const rel = ev.relatedTarget as Node | null
  if (rel && box.contains(rel)) return
  resetExcelDropHighlight()
}

function onExcelZoneDragOver(ev: DragEvent) {
  if (!dragEventHasFiles(ev)) return
  if (planActionsDisabled.value || excelImportBusy.value) return
  ev.preventDefault()
  if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'copy'
}

async function onExcelZoneDrop(ev: DragEvent) {
  resetExcelDropHighlight()
  if (!dragEventHasFiles(ev)) return
  if (planActionsDisabled.value || excelImportBusy.value) return
  ev.preventDefault()
  const file = firstExcelFromFileList(ev.dataTransfer?.files)
  if (!file) {
    toast.add({
      severity: 'warn',
      summary: 'Excel-Import',
      detail: 'Bitte eine .xlsx-Datei ablegen.',
      life: 6000,
    })
    return
  }
  await runExcelImport(file)
}

function dismissExcelPastImportDialog() {
  excelPastImportDialogVisible.value = false
  pendingPastImportFile.value = null
  excelPastImportPrompt.value = ''
}

async function runExcelImportAfterPastChoice(includePast: boolean) {
  const file = pendingPastImportFile.value
  if (!file) return
  excelImportBusy.value = true
  try {
    const rep = await importScheduleExcel(file, { include_past: includePast })
    lastExcelReport.value = rep
    await scheduleGridRef.value?.reloadFromServer()
    dismissExcelPastImportDialog()
    const issues = excelReportHasDetails(rep)
    toast.add({
      severity: issues ? 'warn' : 'success',
      summary: 'Excel-Import',
      detail: summarizeExcelImport(rep),
      life: 10000,
    })
  } catch (err: unknown) {
    const serverMsg =
      getApiErrorMessage(err) ??
      (err instanceof Error && err.message.trim() ? err.message : undefined) ??
      (typeof err === 'string' && err.trim() ? err : undefined)
    toast.add({
      severity: 'error',
      summary: 'Excel-Import',
      detail:
        serverMsg ??
        'Import fehlgeschlagen. In den Entwicklertools (F12) → Netzwerk → „import-excel“ / „preview-excel-import“ prüfen.',
      life: 10000,
    })
  } finally {
    excelImportBusy.value = false
  }
}

async function runExcelImport(file: File) {
  excelImportBusy.value = true
  lastExcelReport.value = null
  dismissExcelPastImportDialog()
  try {
    const preview = await previewScheduleExcelImport(file)
    if (excelPastSignalCount(preview) > 0) {
      pendingPastImportFile.value = file
      excelPastImportPrompt.value = buildExcelPastImportPrompt(preview)
      excelPastImportDialogVisible.value = true
      return
    }
    const rep = await importScheduleExcel(file)
    lastExcelReport.value = rep
    await scheduleGridRef.value?.reloadFromServer()
    const issues = excelReportHasDetails(rep)
    toast.add({
      severity: issues ? 'warn' : 'success',
      summary: 'Excel-Import',
      detail: summarizeExcelImport(rep),
      life: 10000,
    })
  } catch (err: unknown) {
    const serverMsg =
      getApiErrorMessage(err) ??
      (err instanceof Error && err.message.trim() ? err.message : undefined) ??
      (typeof err === 'string' && err.trim() ? err : undefined)
    toast.add({
      severity: 'error',
      summary: 'Excel-Import',
      detail:
        serverMsg ??
        'Import fehlgeschlagen. In den Entwicklertools (F12) → Netzwerk → „import-excel“ / „preview-excel-import“ prüfen.',
      life: 10000,
    })
  } finally {
    excelImportBusy.value = false
  }
}

async function openExcelExportDialog() {
  excelExportDialogVisible.value = true
  exportRangeEndManuallyEdited.value = false
  try {
    const d = await fetchScheduleExportDefaults()
    exportFromYear.value = d.start_year
    exportFromWeek.value = d.start_week
    exportToYear.value = d.end_year
    exportToWeek.value = d.end_week
    exportRangeEndManuallyEdited.value = false
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Excel-Export',
      detail: getApiErrorMessage(e) ?? 'Standard-Zeitraum konnte nicht geladen werden.',
      life: 10000,
    })
  }
}

async function runExcelExport() {
  const rangeErr = exportWeekRangeError.value
  if (rangeErr) {
    toast.add({
      severity: 'error',
      summary: 'Excel-Export',
      detail: rangeErr,
      life: 10000,
    })
    return
  }
  excelExportBusy.value = true
  try {
    const blob = await fetchScheduleExportExcel(
      exportFromYear.value,
      exportFromWeek.value,
      exportToYear.value,
      exportToWeek.value,
    )
    const filename = `Dienstplan-KW${exportFromWeek.value}-${exportFromYear.value}-bis-KW${exportToWeek.value}-${exportToYear.value}.xlsx`
    triggerDownload(blob, filename)
    excelExportDialogVisible.value = false
    toast.add({
      severity: 'success',
      summary: 'Excel-Export',
      detail: 'Download gestartet.',
      life: 4000,
    })
  } catch (e) {
    toast.add({
      severity: 'error',
      summary: 'Excel-Export',
      detail: getApiErrorMessage(e) ?? 'Export fehlgeschlagen.',
      life: 10000,
    })
  } finally {
    excelExportBusy.value = false
  }
}

async function onExcelFileChange(ev: Event) {
  const el = ev.target as HTMLInputElement
  const file = el.files?.[0]
  el.value = ''
  if (!file) return
  await runExcelImport(file)
}

const scheduleEmployees = computed(() =>
  employees.value.filter((e) => e.active && e.role !== 'superadmin'),
)

watch(loading, (busy) => {
  if (!busy) void nextTick(() => syncNotesLayoutWidth())
})

/** Schmale Ansicht: Notiz-Dialog-Button (Desktop: Notizen in der rechten Spalte). */
const showNotesBubbleVisible = computed(
  () => !loading.value && scheduleEmployees.value.length > 0 && !showNotesInline.value,
)

watch(showNotesBubbleVisible, (vis) => {
  if (vis) void nextTick(() => scheduleSyncNotesFabVisualViewport())
})

const notesFabButtonStyle = {
  pointerEvents: 'auto' as const,
  width: '3.5rem',
  minWidth: '3.5rem',
  height: '3.5rem',
  padding: '0',
  fontSize: '1.25rem',
}

/** Dienstplan-Zeilen nach Benutzergruppe; „Ohne Gruppe“ für nicht oder unbekannt zugewiesene IDs. */
const scheduleSections = computed((): { title: string; employees: Employee[] }[] => {
  const list = scheduleEmployees.value
  const sortedGroups = [...groups.value]
  const known = new Set(sortedGroups.map((g) => g.id))
  const sections: { title: string; employees: Employee[] }[] = []

  for (const g of sortedGroups) {
    const emps = list
      .filter((e) => e.group_id === g.id)
      .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
    if (emps.length) sections.push({ title: g.name, employees: emps })
  }

  const orphan = list
    .filter((e) => e.group_id == null || !known.has(e.group_id))
    .sort((a, b) => a.display_name.localeCompare(b.display_name, 'de'))
  if (orphan.length) sections.push({ title: 'Ohne Gruppe', employees: orphan })

  return sections
})

const scheduleSectionsWithEmployees = computed(() =>
  scheduleSections.value.filter((s) => s.employees.length > 0),
)

/** Vor KW-/Jahreswechsel: ausstehende Notizen sofort für die alte Woche speichern */
watch([weekYear, week], async (_, ow) => {
  if (notesTimer) {
    clearTimeout(notesTimer)
    notesTimer = null
  }
  if (!ow) return
  notesSaving.value = true
  try {
    await putScheduleWeekNotes(ow[0], ow[1], weekNotes.value)
  } catch {
    toast.add({
      severity: 'error',
      summary: 'Notizen',
      detail: 'Konnte vor dem Wechsel der Kalenderwoche nicht gespeichert werden.',
      life: 10000,
    })
  } finally {
    notesSaving.value = false
  }
})

watch(weekNotes, () => {
  if (!scheduleEmployees.value.length) return
  if (notesTimer) clearTimeout(notesTimer)
  notesTimer = setTimeout(async () => {
    notesTimer = null
    notesSaving.value = true
    try {
      await putScheduleWeekNotes(weekYear.value, week.value, weekNotes.value)
      if (notesSavedTimer) clearTimeout(notesSavedTimer)
      notesSavedFlash.value = true
      notesSavedTimer = setTimeout(() => {
        notesSavedTimer = null
        notesSavedFlash.value = false
      }, 1600)
    } catch {
      toast.add({
        severity: 'error',
        summary: 'Notizen',
        detail: 'Automatisches Speichern fehlgeschlagen.',
        life: 10000,
      })
    } finally {
      notesSaving.value = false
    }
  }, NOTES_DEBOUNCE_MS)
})

function applyScheduleWeekFromQuery() {
  const y = queryPositiveInt(route.query.year)
  const w = queryPositiveInt(route.query.week)
  if (y != null && w != null && w >= 1 && w <= 53) {
    weekYear.value = y
    week.value = w
  }
  if (route.query.year != null || route.query.week != null) {
    clearRouteQueryKeys(router, ['year', 'week'])
  }
}

onMounted(async () => {
  applyScheduleWeekFromQuery()
  window.addEventListener('dragend', onWindowDragEnd)
  bindNotesFabVisualViewportListeners()
  syncNotesLayoutWidth()
  syncNotesFabVisualViewport()
  if (typeof window.matchMedia === 'function') {
    notesCompactMq = window.matchMedia(NOTES_COMPACT_MEDIA)
    notesCompactMq.addEventListener('change', onNotesCompactMqChange)
  }
  notesLayoutListenersBound = true
  loading.value = true
  try {
    const [emp, grp] = await Promise.all([fetchEmployees(), fetchGroups()])
    employees.value = emp
    groups.value = grp
  } finally {
    loading.value = false
    await nextTick()
    syncNotesLayoutWidth()
    syncNotesFabVisualViewport()
  }
})

onUnmounted(() => {
  window.removeEventListener('dragend', onWindowDragEnd)
  if (notesFabVvRaf !== 0) {
    cancelAnimationFrame(notesFabVvRaf)
    notesFabVvRaf = 0
  }
  unbindNotesFabVisualViewportListeners()
  if (notesLayoutListenersBound) {
    notesCompactMq?.removeEventListener('change', onNotesCompactMqChange)
    notesCompactMq = null
    notesLayoutListenersBound = false
  }
  if (notesTimer) clearTimeout(notesTimer)
  if (notesSavedTimer) clearTimeout(notesSavedTimer)
  resetExcelDropHighlight()
})

function thisWeek() {
  const n = isoWeekAndYear(new Date())
  weekYear.value = n.year
  week.value = n.week
}
</script>

<template>
  <div
    class="page page-excel-drop"
    @dragenter="onExcelZoneDragEnter"
    @dragleave="onExcelZoneDragLeave"
    @dragover="onExcelZoneDragOver"
    @drop="onExcelZoneDrop"
  >
    <div
      v-show="excelDropHighlight"
      class="page-excel-drop-overlay"
      aria-hidden="true"
    >
      <span class="page-excel-drop-overlay-text">Excel-Datei (.xlsx) hier ablegen</span>
    </div>
    <Card>
      <template #title>Dienstplan</template>
      <template #subtitle>
        Gruppierung nach Benutzergruppe (Reihenfolge der Gruppen unter „Gruppen“ per Pfeiltasten); ohne Zuordnung unter
        „Ohne Gruppe“. Excel-Import über „Excel importieren“ oder .xlsx-Datei irgendwo auf diese Seite ziehen.
      </template>
      <template #content>
        <div class="toolbar">
          <div class="fields">
            <label>
              <span class="lbl">Kalenderjahr</span>
              <InputNumber
                v-model="weekYear"
                :min="2000"
                :max="2100"
                :use-grouping="false"
                show-buttons
                input-id="schedule-week-year"
                data-testid="schedule-week-year"
              />
            </label>
            <label>
              <span class="lbl">KW</span>
              <InputNumber
                v-model="week"
                :min="1"
                :max="53"
                show-buttons
                input-id="schedule-week"
                data-testid="schedule-week"
              />
            </label>
          </div>
          <Button label="Springe zu aktueller Woche" severity="secondary" outlined @click="thisWeek" />
        </div>
        <p v-if="!loading && scheduleEmployees.length === 0" class="muted">
          Keine aktiven Mitarbeiter für den Dienstplan.
        </p>
        <div v-else class="editor-layout">
          <div class="schedule-left">
            <div class="schedule-plan-actions">
              <input
                ref="excelInputRef"
                type="file"
                accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                hidden
                @change="onExcelFileChange"
              />
              <Button
                label="Vorwoche kopieren"
                severity="secondary"
                outlined
                :disabled="planActionsDisabled"
                @click="scheduleGridRef?.copyPreviousWeek()"
              />
              <Button
                label="Excel importieren"
                severity="secondary"
                outlined
                :disabled="planActionsDisabled || excelImportBusy"
                :loading="excelImportBusy"
                @click="openExcelPicker"
              />
              <Button
                label="Excel exportieren"
                severity="secondary"
                outlined
                :disabled="planActionsDisabled || excelExportBusy"
                :loading="excelExportBusy"
                @click="openExcelExportDialog"
              />
              <Button
                label="Teamsitzung hinzufügen"
                severity="secondary"
                outlined
                :disabled="planActionsDisabled"
                @click="openTeamMeetingCreateDialog"
              />
              <Button
                label="Teamsitzung bearbeiten"
                severity="secondary"
                outlined
                :disabled="planActionsDisabled"
                @click="openTeamMeetingEditDialog"
              />
              <span
                class="autosave-hint"
                :class="{ 'autosave-hint--empty': !scheduleAutosaveHint }"
                aria-live="polite"
              >
                {{ scheduleAutosaveHint || '\u00a0' }}
              </span>
            </div>
            <div v-if="lastExcelReport && excelReportHasDetails(lastExcelReport)" class="import-report">
              <p class="import-report-title">Import — Hinweise</p>
              <ul v-if="lastExcelReport.unknown_names?.length" class="import-report-list">
                <li v-for="n in lastExcelReport.unknown_names" :key="'u-' + n">
                  Unbekannter Name (nicht importiert): {{ n }}
                </li>
              </ul>
              <ul v-if="lastExcelReport.errors?.length" class="import-report-list import-report-list--err">
                <li v-for="(e, i) in lastExcelReport.errors" :key="'e-' + i">{{ e }}</li>
              </ul>
              <ul v-if="lastExcelReport.warnings?.length" class="import-report-list import-report-list--warn">
                <li v-for="(w, i) in lastExcelReport.warnings" :key="'w-' + i">{{ w }}</li>
              </ul>
              <template v-for="(wk, wi) in lastExcelReport.weeks ?? []" :key="'tmw-' + wi">
                <ul
                  v-if="(wk.team_monday_sections?.length ?? 0) > 0"
                  class="import-report-list import-report-list--muted"
                >
                  <li v-for="(sec, si) in wk.team_monday_sections ?? []" :key="'tms-' + wi + '-' + si">
                    KW {{ wk.iso_week }}/{{ wk.iso_year }} · Montag:
                    <template v-if="sec.no_meetings">kein team</template>
                    <template v-else>
                      <span v-if="sec.group_team">{{ sec.group_team.start }}–{{ sec.group_team.end }} (KT)</span>
                      <span v-if="sec.group_team && sec.all_team"> · </span>
                      <span v-if="sec.all_team">{{ sec.all_team.start }}–{{ sec.all_team.end }} (GT)</span>
                    </template>
                    <span v-if="sec.raw_line"> — {{ sec.raw_line }}</span>
                  </li>
                </ul>
              </template>
            </div>
            <div class="schedule-main">
              <ScheduleGrid
                ref="scheduleGridRef"
                v-model:week-notes="weekNotes"
                :sections="scheduleSections"
                :week-year="weekYear"
                :week="week"
                @autosave-hint="scheduleAutosaveHint = $event"
                @edit-team-meeting="openTeamMeetingEditForMeeting"
              />
            </div>
          </div>
          <aside v-if="showNotesInline" class="notes-aside">
            <span class="notes-label">Notizen zur Kalenderwoche</span>
            <div class="notes-editor">
              <Editor
                v-model="weekNotes"
                class="notes-rich-editor"
                :formats="['bold', 'italic', 'underline', 'color']"
                placeholder="Interne Hinweise zu dieser Woche …"
                :editor-style="{
                  minHeight: '12rem',
                  maxHeight: 'min(55vh, 520px)',
                  overflowY: 'auto',
                }"
              >
                <template #toolbar>
                  <span class="ql-formats">
                    <button type="button" class="ql-bold" aria-label="Fett" />
                    <button type="button" class="ql-italic" aria-label="Kursiv" />
                    <button type="button" class="ql-underline" aria-label="Unterstreichen" />
                  </span>
                  <span class="ql-formats">
                    <select class="ql-color" aria-label="Textfarbe" />
                  </span>
                </template>
              </Editor>
            </div>
            <p v-if="notesSaving" class="notes-status">Notizen werden gespeichert …</p>
            <p v-else-if="notesSavedFlash" class="notes-status notes-status--ok">Notizen gespeichert</p>
          </aside>
        </div>
      </template>
    </Card>

    <Teleport to="body">
      <div v-if="showNotesBubbleVisible" class="schedule-editor-notes-fab" :style="notesFabWrapStyle">
        <Button
          type="button"
          class="schedule-editor-notes-fab-btn"
          icon="pi pi-comment"
          rounded
          severity="primary"
          :style="notesFabButtonStyle"
          aria-label="Kalenderwochen-Notizen"
          @click="notesMobileDialogVisible = true"
        />
      </div>
    </Teleport>

    <Dialog
      v-model:visible="notesMobileDialogVisible"
      class="schedule-notes-mobile-dialog"
      header="Notizen zur Kalenderwoche"
      :modal="true"
      :draggable="false"
      position="top"
      :pt="{
        mask: { class: 'schedule-notes-mobile-dialog-mask' },
      }"
    >
      <div class="notes-editor notes-editor--dialog">
        <Editor
          v-model="weekNotes"
          class="notes-rich-editor"
          :formats="['bold', 'italic', 'underline', 'color']"
          placeholder="Interne Hinweise zu dieser Woche …"
          :editor-style="{
            minHeight: '8rem',
            maxHeight: 'min(52dvh, 360px)',
            overflowY: 'auto',
          }"
        >
          <template #toolbar>
            <span class="ql-formats">
              <button type="button" class="ql-bold" aria-label="Fett" />
              <button type="button" class="ql-italic" aria-label="Kursiv" />
              <button type="button" class="ql-underline" aria-label="Unterstreichen" />
            </span>
            <span class="ql-formats">
              <select class="ql-color" aria-label="Textfarbe" />
            </span>
          </template>
        </Editor>
      </div>
      <p v-if="notesSaving" class="notes-status">Notizen werden gespeichert …</p>
      <p v-else-if="notesSavedFlash" class="notes-status notes-status--ok">Notizen gespeichert</p>
    </Dialog>

    <Dialog
      v-model:visible="excelExportDialogVisible"
      class="schedule-export-dialog"
      header="Dienstplan exportieren"
      :modal="true"
      :style="{ width: 'min(560px, 96vw)' }"
    >
      <p class="muted export-dialog-hint">
        Kalenderwochen für den Excel-Export (Layout wie Import-Vorlage).
      </p>
      <div class="export-kw-grid">
        <fieldset class="export-kw-block">
          <legend>Von</legend>
          <label>
            <span class="lbl">Jahr</span>
            <InputNumber
              v-model="exportFromYear"
              :min="2000"
              :max="2100"
              :use-grouping="false"
              show-buttons
              input-id="export-from-year"
            />
          </label>
          <label>
            <span class="lbl">KW</span>
            <InputNumber
              v-model="exportFromWeek"
              :min="1"
              :max="53"
              show-buttons
              input-id="export-from-week"
            />
          </label>
        </fieldset>
        <fieldset class="export-kw-block">
          <legend>Bis</legend>
          <label>
            <span class="lbl">Jahr</span>
            <InputNumber
              v-model="exportToYear"
              :min="2000"
              :max="2100"
              :use-grouping="false"
              show-buttons
              input-id="export-to-year"
            />
          </label>
          <label>
            <span class="lbl">KW</span>
            <InputNumber
              v-model="exportToWeek"
              :min="1"
              :max="53"
              show-buttons
              input-id="export-to-week"
            />
          </label>
        </fieldset>
      </div>
      <p v-if="exportWeekRangeError" class="export-range-error" role="alert">
        {{ exportWeekRangeError }}
      </p>
      <template #footer>
        <Button
          label="Abbrechen"
          severity="secondary"
          text
          type="button"
          @click="excelExportDialogVisible = false"
        />
        <Button
          label="Herunterladen"
          type="button"
          :disabled="exportWeekRangeInvalid"
          :loading="excelExportBusy"
          @click="runExcelExport"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="excelPastImportDialogVisible"
      class="schedule-past-import-dialog"
      header="Vergangenheit importieren?"
      :modal="true"
      :closable="!excelImportBusy"
      :style="{ width: 'min(520px, 96vw)' }"
      @hide="dismissExcelPastImportDialog"
    >
      <p class="muted export-dialog-hint">{{ excelPastImportPrompt }}</p>
      <p class="muted export-dialog-hint">
        Es wurde noch nichts importiert — der Dienstplan wird erst nach Ihrer Auswahl geschrieben.
      </p>
      <p class="muted export-dialog-hint">
        Standard: Vergangenheitsdaten werden nicht übernommen (nur Einträge ab heute).
      </p>
      <template #footer>
        <Button
          label="Ignorieren"
          severity="secondary"
          type="button"
          :disabled="excelImportBusy"
          autofocus
          @click="runExcelImportAfterPastChoice(false)"
        />
        <Button
          label="Vergangenheit importieren"
          type="button"
          :loading="excelImportBusy"
          @click="runExcelImportAfterPastChoice(true)"
        />
      </template>
    </Dialog>

    <Dialog
      v-model:visible="teamMeetingDialogVisible"
      :header="teamMeetingDialogHeader"
      :modal="true"
      :style="{ width: 'min(520px, 96vw)' }"
    >
      <p class="tm-week-hint muted">{{ teamMeetingWeekHint }}</p>
      <template v-if="teamMeetingDialogMode === 'create'">
        <div class="tm-dialog">
          <label class="tm-field">
            <span class="tm-lbl">Art</span>
            <Select
              v-model="meetingKindForCreate"
              :options="teamKindOptions"
              option-label="label"
              option-value="value"
              class="tm-select"
            />
          </label>
          <label v-if="meetingKindForCreate === 'other'" class="tm-field">
            <span class="tm-lbl">Bezeichnung</span>
            <InputText v-model="meetingLabelForCreate" placeholder="z. B. Fortbildung" class="tm-input" />
          </label>
          <label v-if="meetingKindForCreate === 'other'" class="tm-field">
            <span class="tm-lbl">Wochentag</span>
            <Select
              v-model="meetingDateForCreate"
              :options="meetingWeekdayOptions"
              option-label="label"
              option-value="value"
              class="tm-select"
            />
          </label>
          <div class="tm-row">
            <label class="tm-field">
              <span class="tm-lbl">Beginn</span>
              <input v-model="meetingEditStart" type="time" step="60" class="p-inputtext p-component tm-input" />
            </label>
            <label class="tm-field">
              <span class="tm-lbl">Ende</span>
              <input
                v-model="meetingEditEnd"
                type="time"
                step="60"
                class="p-inputtext p-component tm-input"
                @input="onMeetingEndInput"
              />
            </label>
          </div>
          <div v-if="scheduleSectionsWithEmployees.length" class="tm-groups">
            <span class="tm-lbl">Gruppen</span>
            <div class="tm-group-btns">
              <Button
                v-for="(sec, si) in scheduleSectionsWithEmployees"
                :key="'tm-gr-c-' + si + '-' + sec.title"
                :label="'+ ' + sec.title"
                size="small"
                severity="secondary"
                text
                type="button"
                @click="addMeetingParticipantsFromSection(sec.employees)"
              />
            </div>
          </div>
          <div class="tm-participant-head">
            <span class="tm-lbl">Teilnehmer</span>
            <Button
              label="Alle hinzufügen"
              size="small"
              severity="secondary"
              text
              type="button"
              :disabled="!scheduleEmployees.length"
              @click="addAllMeetingParticipants"
            />
          </div>
          <div class="tm-participants">
            <label v-for="emp in scheduleEmployees" :key="'tmpc-' + emp.id" class="tm-participant">
              <Checkbox
                :binary="true"
                :model-value="meetingParticipantIds.includes(emp.id)"
                @update:model-value="(v: boolean) => toggleMeetingParticipant(emp.id, Boolean(v))"
              />
              <span>{{ emp.display_name }}</span>
            </label>
          </div>
        </div>
      </template>
      <template v-else>
        <p v-if="!weekTeamMeetings.length" class="muted">
          Für diese Kalenderwoche sind keine Teamsitzungen geladen (z. B. noch kein Import oder keine Einträge).
        </p>
        <div v-else class="tm-dialog">
          <label class="tm-field">
            <span class="tm-lbl">Sitzung</span>
            <Select
              v-model="selectedTeamMeetingId"
              :options="teamMeetingSelectOptions"
              option-label="label"
              option-value="value"
              placeholder="Auswählen"
              class="tm-select"
            />
          </label>
          <label v-if="editingOtherMeeting" class="tm-field">
            <span class="tm-lbl">Bezeichnung</span>
            <InputText v-model="meetingEditLabel" placeholder="z. B. Fortbildung" class="tm-input" />
          </label>
          <label v-if="editingOtherMeeting" class="tm-field">
            <span class="tm-lbl">Wochentag</span>
            <Select
              v-model="meetingEditDate"
              :options="meetingWeekdayOptions"
              option-label="label"
              option-value="value"
              class="tm-select"
            />
          </label>
          <div class="tm-row">
            <label class="tm-field">
              <span class="tm-lbl">Beginn</span>
              <input v-model="meetingEditStart" type="time" step="60" class="p-inputtext p-component tm-input" />
            </label>
            <label class="tm-field">
              <span class="tm-lbl">Ende</span>
              <input
                v-model="meetingEditEnd"
                type="time"
                step="60"
                class="p-inputtext p-component tm-input"
                @input="onMeetingEndInput"
              />
            </label>
          </div>
          <div v-if="scheduleSectionsWithEmployees.length" class="tm-groups">
            <span class="tm-lbl">Gruppen</span>
            <div class="tm-group-btns">
              <Button
                v-for="(sec, si) in scheduleSectionsWithEmployees"
                :key="'tm-gr-e-' + si + '-' + sec.title"
                :label="'+ ' + sec.title"
                size="small"
                severity="secondary"
                text
                type="button"
                @click="addMeetingParticipantsFromSection(sec.employees)"
              />
            </div>
          </div>
          <div class="tm-participant-head">
            <span class="tm-lbl">Teilnehmer</span>
            <Button
              label="Alle hinzufügen"
              size="small"
              severity="secondary"
              text
              type="button"
              :disabled="!scheduleEmployees.length"
              @click="addAllMeetingParticipants"
            />
          </div>
          <div class="tm-participants">
            <label v-for="emp in scheduleEmployees" :key="'tmp-' + emp.id" class="tm-participant">
              <Checkbox
                :binary="true"
                :model-value="meetingParticipantIds.includes(emp.id)"
                @update:model-value="(v: boolean) => toggleMeetingParticipant(emp.id, Boolean(v))"
              />
              <span>{{ emp.display_name }}</span>
            </label>
          </div>
        </div>
      </template>
      <template #footer>
        <div class="tm-dialog-footer">
          <Button
            v-if="teamMeetingDialogMode === 'edit' && selectedTeamMeetingId != null && weekTeamMeetings.length"
            label="Löschen"
            icon="pi pi-trash"
            severity="danger"
            text
            :disabled="teamMeetingSaving"
            @click="deleteSelectedTeamMeeting"
          />
          <div class="tm-dialog-footer-actions">
            <Button label="Abbrechen" severity="secondary" text @click="teamMeetingDialogVisible = false" />
            <Button
              :label="teamMeetingDialogMode === 'create' ? 'Anlegen' : 'Speichern'"
              :disabled="teamMeetingSaveDisabled || teamMeetingSaving"
              :loading="teamMeetingSaving"
              @click="saveTeamMeetingEdits"
            />
          </div>
        </div>
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.page {
  max-width: 1320px;
  position: relative;
}
.page-excel-drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border-radius: 6px;
  border: 2px dashed #2563eb;
  background: rgba(239, 246, 255, 0.92);
  pointer-events: none;
}
.page-excel-drop-overlay-text {
  font-size: 1rem;
  font-weight: 600;
  color: #1e3a8a;
  text-align: center;
  padding: 0 1rem;
}
.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 1rem;
  margin-bottom: 1rem;
}
.editor-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
  gap: 1rem 1.25rem;
  align-items: start;
}
.schedule-left {
  grid-column: 1;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-width: 0;
}
.schedule-plan-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.autosave-hint {
  flex: 0 0 14rem;
  font-size: 0.8rem;
  color: #64748b;
  white-space: nowrap;
}
.autosave-hint--empty {
  visibility: hidden;
}
.schedule-main {
  min-width: 0;
}
@media (max-width: 960px) {
  .editor-layout {
    grid-template-columns: 1fr;
  }
  /* Notizspalte nur ab 961px sinnvoll; bei schmalem Viewport ausblenden (auch wenn JS-Ref hängt). */
  .notes-aside {
    display: none !important;
  }
}
.notes-aside {
  grid-column: 2;
  position: sticky;
  top: calc(var(--layout-top-inset, 0px) + 0.75rem);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}
.notes-label {
  font-size: 0.8rem;
  font-weight: 600;
  color: #334155;
}
.notes-editor {
  width: 100%;
  min-width: 0;
}
.notes-editor :deep(.p-editor) {
  width: 100%;
}
.notes-rich-editor :deep(.p-editor-toolbar) {
  border-radius: 6px 6px 0 0;
}
.notes-rich-editor :deep(.p-editor-content) {
  border-radius: 0 0 6px 6px;
}
.notes-status {
  margin: 0;
  font-size: 0.75rem;
  color: #64748b;
}
.notes-status--ok {
  color: #15803d;
}
.fields {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
}
.fields label {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.lbl {
  font-size: 0.75rem;
  color: #64748b;
}
.muted {
  color: #64748b;
}
.import-report {
  margin: 0;
  padding: 0.65rem 0.75rem;
  border-radius: 6px;
  background: #fffbeb;
  border: 1px solid #fcd34d;
  font-size: 0.8rem;
  max-width: min(100%, 720px);
}
.import-report-title {
  margin: 0 0 0.35rem 0;
  font-weight: 600;
  color: #92400e;
}
.import-report-list {
  margin: 0.25rem 0 0 1rem;
  padding: 0;
  color: #78350f;
}
.import-report-list--err {
  color: #991b1b;
}
.import-report-list--warn {
  color: #92400e;
}
.import-report-list--muted {
  margin-top: 0.35rem;
  color: #57534e;
  font-size: 0.78rem;
}
.export-dialog-hint {
  margin: 0 0 0.75rem 0;
  font-size: 0.85rem;
}
.export-range-error {
  margin: 0.75rem 0 0 0;
  font-size: 0.85rem;
  color: #b91c1c;
}
.export-kw-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}
@media (max-width: 520px) {
  .export-kw-grid {
    grid-template-columns: 1fr;
  }
}
.export-kw-block {
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  padding: 0.65rem 0.75rem;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}
.export-kw-block label {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  min-width: 0;
}
.export-kw-block label :deep(.p-inputnumber) {
  width: 100%;
}
.export-kw-block label :deep(.p-inputnumber-input) {
  min-width: 0;
}
.export-kw-block legend {
  font-size: 0.75rem;
  font-weight: 600;
  color: #475569;
  padding: 0 0.25rem;
}
.tm-dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  gap: 0.5rem;
}
.tm-dialog-footer-actions {
  display: flex;
  gap: 0.5rem;
  margin-left: auto;
}
.tm-dialog {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}
.tm-week-hint {
  margin: 0 0 0.75rem 0;
  font-size: 0.82rem;
}
.tm-participant-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.tm-groups {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.tm-group-btns {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}
.tm-field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
.tm-lbl {
  font-size: 0.75rem;
  font-weight: 600;
  color: #475569;
}
.tm-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}
.tm-input {
  width: 7rem;
}
.tm-select {
  width: 100%;
}
.tm-participants {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  max-height: min(40vh, 280px);
  overflow-y: auto;
  padding: 0.25rem 0;
}
.tm-participant {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
}
</style>

<style>
/*
 * Teleport(Dialog → body), global: Quill hat große min-content-Breite — ohne min-width:0 wächst .p-dialog.
 * Horizontal: kein margin:auto auf dem Dialog (sonst Zentrierung trotz flex-start; breites min-content ragt asymmetrisch ab).
 * Breite hart an den sichtbaren Viewport gekoppelt (100vw/dvw + Safe-Area), damit Kopf inkl. Schließen-Icon immer sichtbar bleibt.
 * Mask-Klasse per pt — :has() allein kann in älteren WebViews fehlen.
 */
.p-dialog-mask.schedule-notes-mobile-dialog-mask,
.p-dialog-mask:has(.schedule-notes-mobile-dialog) {
  justify-content: center !important;
  align-items: flex-start !important;
  /* Exakt den Viewport abdecken — verhindert seitlichen Versatz, wenn die Seite breiter als das Fenster ist */
  inset: 0 !important;
  width: auto !important;
  max-width: 100vw !important;
  min-width: 0 !important;
  height: auto !important;
  max-height: 100dvh !important;
  overflow-x: hidden;
  overflow-y: auto;
  -webkit-overflow-scrolling: touch;
  padding-left: max(0.5rem, env(safe-area-inset-left, 0px));
  padding-right: max(0.5rem, env(safe-area-inset-right, 0px));
  padding-top: max(0.5rem, env(safe-area-inset-top, 0px));
  padding-bottom: max(0.5rem, env(safe-area-inset-bottom, 0px));
  box-sizing: border-box;
}

.schedule-notes-mobile-dialog.p-dialog {
  box-sizing: border-box;
  flex: 0 1 auto;
  min-width: 0 !important;
  /* Volle Maskenbreite, aber nie breiter als sichtbarer Bereich (Quill min-content sonst > Viewport) */
  width: 100% !important;
  max-width: min(32.5rem, calc(100vw - 1rem)) !important;
  max-width: min(32.5rem, calc(100dvw - 1rem)) !important;
  /* Platz für Topbar/Browser-UI; Safe-Area */
  max-height: min(93vh, calc(100vh - 1rem)) !important;
  max-height: min(
    93dvh,
    calc(100dvh - env(safe-area-inset-top, 0px) - env(safe-area-inset-bottom, 0px) - 0.75rem)
  ) !important;
  margin-top: 0 !important;
  margin-bottom: 0 !important;
  margin-left: 0 !important;
  margin-right: 0 !important;
  overflow: hidden;
}
.schedule-notes-mobile-dialog .p-dialog-header {
  min-width: 0;
  flex-shrink: 1;
}
.schedule-notes-mobile-dialog .p-dialog-title {
  min-width: 0;
  overflow-wrap: anywhere;
}
.schedule-notes-mobile-dialog .p-dialog-content {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  overflow-x: auto;
  overflow-y: auto;
  max-height: min(82vh, calc(100vh - 7.5rem));
  max-height: min(78dvh, calc(100dvh - env(safe-area-inset-top, 0px) - env(safe-area-inset-bottom, 0px) - 7.5rem));
}
.schedule-notes-mobile-dialog .notes-editor--dialog {
  min-width: 0;
  max-width: 100%;
}
.schedule-notes-mobile-dialog .notes-rich-editor {
  min-width: 0;
  max-width: 100%;
}
.schedule-notes-mobile-dialog .notes-rich-editor .p-editor {
  min-width: 0;
  max-width: 100%;
}

.schedule-export-dialog.p-dialog {
  box-sizing: border-box;
  min-width: 0 !important;
  max-width: min(560px, calc(100vw - 1rem)) !important;
  max-width: min(560px, calc(100dvw - 1rem)) !important;
}
.schedule-export-dialog .p-dialog-content {
  min-width: 0;
  overflow-x: auto;
}
.schedule-export-dialog .export-dialog-hint {
  overflow-wrap: anywhere;
}
</style>
