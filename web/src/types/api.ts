/** Work period from API (punch times ISO8601). */
export interface WorkPeriod {
  id: number
  user_id: number
  work_date: string
  punch_in: string
  punch_out: string | null
  is_break: boolean
  source: string
}

export interface MonthBalance {
  year: number
  month: number
  worked_hours: number
  target_hours: number
  balance_hours: number
  carryover: number
  total_balance: number
  /** Erster Tag des Stundenkontos (YYYY-MM-DD). */
  account_start?: string
  /** Tatsächlich gezählter Zeitraum dieses Monats (leer, wenn nichts gezählt). */
  counted_from?: string
  counted_through?: string
  /** Laufender Monat: gezählt nur bis gestern. */
  is_partial?: boolean
  /** Monat beginnt nach gestern: noch nichts gezählt. */
  is_future?: boolean
}

/** GET /me/hours-account: Stand des Stundenkontos bis einschließlich gestern. */
export interface HoursAccount {
  balance_hours: number
  account_start: string
  counted_through: string
  opening_hours: number
}

/** Ein Tag aus GET /me/days (gleiche Rechnung wie der Saldo). */
export interface SaldoDay {
  date: string
  gross_minutes: number
  stamped_break_minutes: number
  deduction_minutes: number
  net_minutes: number
  open_period: boolean
  credit_hours: number
  target_hours: number
  balance_hours: number
  absence_type: string | null
  half_day: boolean
  holiday_name?: string
  closure_name?: string
  is_workday: boolean
}

/** Gesamt = Startsaldo + Übertrag + Anspruch; Rest = Gesamt − genommen; frei = Rest − geplant. */
export interface VacationBalance {
  year: number
  /** Startsaldo aus dem Import. */
  carried_over: number
  /** Übertrag aus den Vorjahren. */
  carryover: number
  /** Anspruch im aktuellen Jahr. */
  entitlement: number
  total: number
  /** Genommene Urlaubstage im aktuellen Jahr bis einschließlich heute. */
  taken: number
  remaining: number
  /** Alle eingetragenen Urlaubstage nach heute. */
  planned: number
  free: number
}

export interface Schedule {
  id: number
  user_id: number
  schedule_date: string
  shift_start: string
  shift_end: string
}

export type TeamMeetingKind = 'kt' | 'gt' | 'other'

export interface TeamMeeting {
  id: number
  iso_week_year: number
  iso_week: number
  meeting_date: string
  kind: TeamMeetingKind
  label?: string
  time_start: string
  time_end: string
  source: string
  section_index: number
  user_ids: number[]
}

/** Antwort von POST /schedules/import-excel */
export interface ScheduleExcelImportTimeSpan {
  start: string
  end: string
}

export interface ScheduleExcelImportTeamMondaySection {
  monday: string
  raw_line?: string
  no_meetings: boolean
  group_team?: ScheduleExcelImportTimeSpan
  all_team?: ScheduleExcelImportTimeSpan
  employees?: string[]
}

export interface ScheduleExcelImportWeekReport {
  iso_year: number
  iso_week: number
  notes_written: boolean
  times_written: number
  times_cleared: number
  absences_created: number
  absences_replaced: number
  past_cells_skipped?: number
  team_monday_sections?: ScheduleExcelImportTeamMondaySection[]
}

export interface ScheduleExcelImportReport {
  weeks: ScheduleExcelImportWeekReport[]
  schedules_written: number
  schedules_deleted: number
  absences_created: number
  absences_replaced: number
  absences_skipped: number
  week_notes_updated: number
  past_cells_skipped?: number
  past_week_notes_skipped?: number
  past_team_meetings_skipped?: number
  team_meetings_created?: number
  unknown_names: string[]
  errors: string[]
  warnings: string[]
}

/** Antwort von POST /schedules/preview-excel-import */
export type ScheduleExcelPastPreview = Pick<
  ScheduleExcelImportReport,
  'past_cells_skipped' | 'past_week_notes_skipped' | 'past_team_meetings_skipped'
>

export interface Absence {
  id: number
  user_id: number
  absence_date: string
  absence_type: 'sick' | 'vacation' | 'other' | 'compensation_day'
  half_day: boolean
  created_by: number
  created_at: string
}

export type CompensationDayClaimStatus = 'open' | 'used' | 'waived'

export interface CompensationDayClaim {
  id: number
  user_id: number
  work_date: string
  status: CompensationDayClaimStatus
  used_absence_id: number | null
  created_at: string
  updated_at: string
}

export interface UserGroup {
  id: number
  name: string
  /** Reihenfolge für Listen und Dienstplan (0 = oben). */
  sort_order: number
  created_at: string
  updated_at: string
}

export interface Employee {
  id: number
  username: string
  display_name: string
  role: string
  active: boolean
  must_change_password: boolean
  /** Mit einem SSO-Konto (OIDC) verknüpft */
  sso_linked?: boolean
  /** false = nicht per Alle/Gruppe/Excel-Import vorausgewählt; Default ja */
  default_team_meeting_participant?: boolean
  /** Höchstens eine Gruppe; fehlt oder null = keine Zuordnung */
  group_id?: number | null
  /** Stundensaldo Import/Alt-System (h), addiert in die Saldo-Berechnung */
  opening_hours_balance?: number
  /** Urlaub-Startwert (Tage), addiert zu Anspruch − genommen */
  opening_vacation_days?: number
  /** Mo–Fr wie Date.getDay() (1=Mo … 5=Fr), regulär ohne Soll */
  fixed_non_work_weekdays?: number[]
  /** Anlagedatum des Kontos (Zeitstempel). Startsaldo gilt per diesem Tag. */
  created_at?: string
}

export interface TimeCorrection {
  id: number
  work_period_id: number
  corrected_in: string
  corrected_out: string
  reason: string
  corrected_by: number
  created_at: string
  disabled?: boolean
}

export interface WeeklyHours {
  id: number
  user_id: number
  hours_per_week: number
  valid_from: string
  created_at: string
  mutable?: boolean
}

export interface VacationEntitlement {
  id: number
  user_id: number
  days_per_year: number
  valid_from: string
  created_at: string
  mutable?: boolean
}

export interface FixedNonWorkWeekdays {
  id: number
  user_id: number
  weekdays: number[]
  valid_from: string
  created_at: string
  mutable?: boolean
}

export interface ScheduleBoundSetting {
  id: number
  user_id: number
  schedule_bound: boolean
  valid_from: string
  created_at: string
  mutable?: boolean
}

export interface NFCTag {
  id: number
  tag_uid: string
  user_id: number
  assigned_from: string
}

export interface ClosureDay {
  id: number
  closure_date: string
  name: string
  created_by: number
}

export interface Holiday {
  id: number
  holiday_date: string
  name: string
  kind: 'feiertag' | 'brauchtum'
  auto_generated: boolean
}

/** Feiertag für Stundenübersichten inkl. gutgeschriebener Stunden (h). */
export interface HolidayCredit {
  holiday_date: string
  name: string
  credit_hours: number
}

/** Abwesenheit für Stundenübersichten inkl. gutgeschriebener Stunden (h). */
export interface AbsenceCredit {
  absence_date: string
  absence_type: string
  credit_hours: number
  half_day: boolean
}

export interface Setting {
  key: string
  value: string
}

/** GET /admin/backup/browse */
export interface BackupBrowseEntry {
  name: string
  path: string
  is_dir: boolean
}

export interface BackupBrowseResult {
  path: string
  parent: string
  entries: BackupBrowseEntry[]
}

/** GET /admin/backup/status */
export interface BackupStatus {
  enabled: boolean
  interval_minutes: number
  use_restic: boolean
  target_path: string
  restic_initialized: boolean
  has_restic_password: boolean
  restic_binary_present: boolean
  folder_picker_available: boolean
  last_success_utc: string
  last_error: string
}

/** Gepaarter Device-/LAN-API-Client (Secret nur für Superadmin). */
export interface ApiPairedClient {
  id: string
  label: string
  secret: string
  created_at_utc: string
  revoked_at_utc?: string | null
}

/** POST /android-api/clients/generate */
export interface GenerateAndroidApiClientResponse {
  client: ApiPairedClient
  pairing_token: string
  /** LAN-erreichbare Server-URL für QR-Feld u (auch wenn Admin-UI über localhost läuft). */
  pairing_base_url?: string
}

/** GET /android-lan/health-status — LAN-Stamps (ein Eintrag pro konfiguriertem Ziel). */
export interface AndroidLanHealthTargetRow {
  id: string
  label?: string
  mode: 'disabled' | 'ok' | 'down'
  reachable: boolean
  last_error?: string
  last_check_utc?: string
}

export interface AndroidLanHealthStatus {
  mode: 'disabled' | 'ok' | 'down'
  reachable: boolean
  last_error?: string
  last_check_utc?: string
  targets?: AndroidLanHealthTargetRow[]
}

/** POST /android-lan/sync-stamps-range — Berlin calendar dates YYYY-MM-DD, max 14 inclusive days. */
export interface AndroidLanSyncStampsRangeBody {
  from: string
  to: string
}

export interface AndroidLanSyncStampsRangeTargetRow {
  target_id: string
  label?: string
  pull_error?: string
  rows_inserted: number
  push_days_ok: number
  push_days_failed: number
}

export interface AndroidLanSyncStampsRangeResult {
  from: string
  to: string
  utc_days_considered: number
  targets: AndroidLanSyncStampsRangeTargetRow[]
}

/** POST /android-lan/sync-employee-ids — ein Ziel (target_id). */
export interface LanEmployeeSyncResult {
  target_id?: string
  label?: string
  lan_base_url: string
  app_employee_ids_after: string[]
  created: {
    user_id: number
    employee_id: string
    name: string
    nfc_tag_uid: string
  }[]
  updated: {
    user_id: number
    employee_id: string
    name: string
    nfc_tag_uid: string
  }[]
  skipped_already_in_app: number[]
  removed_from_app: {
    user_id: number
    employee_id: string
  }[]
  failures: { employee_id?: string; error: string }[]
}

/** POST /android-lan/sync-employee-ids-all */
export interface LanEmployeeSyncAllResult {
  results: Array<
    LanEmployeeSyncResult & {
      error?: string
    }
  >
  summary: {
    targets: number
    created_total: number
    updated_total: number
    skipped_total: number
    removed_total: number
    failures_total: number
  }
}

/** Eintrag in Einstellung android_lan_targets (JSON-Array). */
export interface AndroidLanTargetSetting {
  id: string
  host: string
  port: number
  api_client_id: string
  label?: string
}

/** Superadmin-UI: gepaarter API-Client und LAN-Endpunkt in einer Zeile. */
export interface PairedLanDeviceRow {
  id: string
  label: string
  host: string
  port: number
  secret: string
  created_at_utc: string
  revoked_at_utc?: string | null
}

export interface BreakRule {
  min_work_hours: number
  break_minutes: number
}

export interface ScheduleGapItem {
  user_id: number
  display_name: string
  schedule_date: string
  shift_start: string
  shift_end: string
  iso_week_year: number
  iso_week: number
}

export interface ScheduleGapsResponse {
  from: string
  through: string
  count: number
  items: ScheduleGapItem[]
}

export type ShiftAlertReason = 'long_duration' | 'late_end'

export interface ShiftAlertItem {
  user_id: number
  display_name: string
  work_date: string
  total_duration_minutes: number
  latest_end: string
  reasons: ShiftAlertReason[]
  iso_week_year: number
  iso_week: number
}

export interface ShiftAlertsResponse {
  from: string
  through: string
  count: number
  items: ShiftAlertItem[]
}

export interface ShiftAlertConfig {
  max_hours: number
  late_end_time: string
}

export interface TeamOverviewRow {
  id: number
  display_name: string
  hours_balance: number
  vacation_planned: number
  vacation_free: number
  vacation_remaining_total: number
  vacation_carryover: number
  vacation_entitlement: number
  vacation_taken: number
  /** In Rest gesamt eingerechneter Urlaubs-Startsaldo (wie GET /me/vacation), sonst 0 */
  vacation_opening_days: number
  compensation_day_claims_open: number
}

export type ChangeRequestKind = 'time_correction' | 'time_entry' | 'vacation'
export type ChangeRequestStatus = 'pending' | 'approved' | 'rejected' | 'withdrawn'

/** Antrag von Mitarbeitenden, wird erst nach Freigabe durch die Leitung wirksam. */
export interface ChangeRequest {
  id: number
  user_id: number
  user_display_name: string
  kind: ChangeRequestKind
  status: ChangeRequestStatus
  work_period_id?: number
  work_date?: string
  original_in?: string
  original_out?: string | null
  punch_in?: string
  punch_out?: string
  date_from?: string
  date_to?: string
  half_day: boolean
  vacation_days?: number
  vacation_dates?: string[]
  reason: string
  decided_by?: number
  decided_by_name?: string
  decided_at?: string
  decision_comment: string
  created_at: string
}

/** Gruppenkasse */
export type CashEntryKind = 'expense' | 'income'
/** Herkunft einer Einnahme (Monatsbetrag, aus Ansparkonto, Sonstiges); bei Ausgaben leer (Kasse) oder savings (direkt aus Ansparkonto). */
export type CashIncomeSource = '' | 'allowance' | 'savings' | 'other'

export interface CashPerson {
  id: number
  display_name: string
}

export interface CashAllowance {
  id: number
  group_id: number
  /** YYYY-MM */
  valid_from: string
  amount_cents: number
  created_at: string
}

export interface CashReceipt {
  id: number
  entry_id: number
  filename: string
  content_type: string
  size_bytes: number
  created_at: string
}

export interface CashEntry {
  id: number
  group_id: number
  kind: CashEntryKind
  source: CashIncomeSource
  /** YYYY-MM, nur bei Monatsbetrag */
  for_month: string
  /** YYYY-MM-DD */
  entry_date: string
  amount_cents: number
  description: string
  balance_after_cents: number
  created_by_name?: string
  updated_by_name?: string
  created_at: string
  updated_at: string
  receipts: CashReceipt[]
}

export interface CashMonthRow {
  month: string
  allowance_cents: number
  paid_cents: number
  saved_cents: number
  withdrawn_cents: number
  current: boolean
}

export interface CashSummary {
  /** Anfangsbestand (0 ohne Anfangsbestand) */
  opening_cash_cents: number
  opening_savings_cents: number
  /** YYYY-MM-DD, fehlt ohne Anfangsbestand */
  opening_date?: string
  balance_cents: number
  income_cents: number
  expense_cents: number
  /** Ausgaben direkt aus dem Ansparkonto (ändern den Kassenstand nicht) */
  savings_spent_cents: number
  savings_cents: number
  current_month: string
  current_allowance_cents: number
  current_paid_cents: number
  current_open_cents: number
  months: CashMonthRow[] | null
}

export interface CashBoxListItem {
  group_id: number
  group_name: string
  keepers: CashPerson[]
  can_edit: boolean
  summary: CashSummary
}

export interface CashBoxDetail {
  group_id: number
  group_name: string
  keepers: CashPerson[]
  can_edit: boolean
  can_manage_keepers: boolean
  allowances: CashAllowance[]
  summary: CashSummary
  entries: CashEntry[]
}

export interface CashEntryInput {
  kind: CashEntryKind
  source: CashIncomeSource
  for_month: string
  entry_date: string
  amount_cents: number
  description: string
}
