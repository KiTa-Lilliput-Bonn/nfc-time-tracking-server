package audit

import "time"

const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// Entity types for audit summaries.
const (
	EntityUser                 = "user"
	EntityEmployee             = "employee"
	EntityTimeCorrection       = "time_correction"
	EntityWorkPeriod           = "work_period"
	EntityAbsence              = "absence"
	EntityWeeklyHours          = "weekly_hours"
	EntityVacationEntitlement  = "vacation_entitlement"
	EntityFixedNonWorkWeekdays = "fixed_non_work_weekdays"
	EntityScheduleBound        = "schedule_bound"
	EntityNFCTag               = "nfc_tag"
	EntityCompensationDayClaim = "compensation_day_claim"
	EntityGroup                = "group"
	EntitySchedule             = "schedule"
	EntityScheduleWeekNotes    = "schedule_week_notes"
	EntityTeamMeeting          = "team_meeting"
	EntityScheduleImport       = "schedule_import"
	EntityHoliday              = "holiday"
	EntityClosureDay           = "closure_day"
	EntitySetting              = "setting"
	EntityShiftAlertDismissal  = "shift_alert_dismissal"
	EntityAPIPairedClient      = "api_paired_client"
	EntityLanStampsSync        = "lan_stamps_sync"
	EntityChangeRequest        = "change_request"
	EntityCashKeepers          = "cash_keepers"
	EntityCashAllowance        = "cash_allowance"
	EntityCashOpening          = "cash_opening"
	EntityCashEntry            = "cash_entry"
	EntityCashReceipt          = "cash_receipt"
	EntityKibizPlanning        = "kibiz_planning"
	EntityChild                = "child"
	EntityChildAttendance      = "child_attendance"
	EntityChildNotice          = "child_notice"
	EntityGroupAccount         = "group_account"
)

// Event is a persisted audit log row.
type Event struct {
	ID            int64     `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	ActorUserID   *int      `json:"actor_user_id,omitempty"`
	ActorRole     string    `json:"actor_role"`
	Action        string    `json:"action"`
	EntityType    string    `json:"entity_type"`
	EntityID      string    `json:"entity_id"`
	TargetUserID  *int      `json:"target_user_id,omitempty"`
	Summary       string    `json:"summary"`
	PrevHash      []byte    `json:"prev_hash"`
	EventHash     []byte    `json:"event_hash"`
}

// Entry is input for appending one audit event.
type Entry struct {
	ActorUserID  *int
	ActorRole    string
	Action       string
	EntityType   string
	EntityID     string
	TargetUserID *int
	Summary      string // compact JSON
}

// ListFilter narrows audit event queries.
type ListFilter struct {
	From         *time.Time
	To           *time.Time
	EntityType   string
	ActorUserID  *int
	TargetUserID *int
	Limit        int
	Offset       int
}

// VerifyResult is returned by chain verification.
type VerifyResult struct {
	OK        bool   `json:"ok"`
	Checked   int    `json:"checked"`
	BrokenID  *int64 `json:"broken_id,omitempty"`
	GenesisOK bool   `json:"genesis_ok"`
}

// Anchor records the chain tip before a retention purge.
type Anchor struct {
	ID              int64     `json:"id"`
	AnchoredAt      time.Time `json:"anchored_at"`
	LastDeletedID   int64     `json:"last_deleted_id"`
	LastEventHash   []byte    `json:"last_event_hash"`
}

// Tip is the latest chain head for external backup comparison.
type Tip struct {
	LastID     int64  `json:"last_id"`
	EventHash  []byte `json:"event_hash"`
	WrittenAt  time.Time `json:"written_at"`
}
