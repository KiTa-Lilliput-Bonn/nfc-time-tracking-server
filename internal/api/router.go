package api

import (
	golog "log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
	"github.com/mattn/go-isatty"

	"nfc-time-tracking-server/internal/api/handler"
	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/config"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/apipairing"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/service/export"
	"nfc-time-tracking-server/internal/service/lanemployeesync"
	"nfc-time-tracking-server/internal/service/stampspoll"
	"nfc-time-tracking-server/internal/store"
	"nfc-time-tracking-server/internal/store/sqlite"
)

// Deps bundles dependencies for the HTTP API.
type Deps struct {
	UserStore             store.UserStore
	GroupStore            store.GroupStore
	Auth                  *authsvc.Service
	WorkPeriods           store.WorkPeriodStore
	Corrections           store.CorrectionStore
	Absences              store.AbsenceStore
	CompensationDayClaims store.CompensationDayClaimStore
	WeeklyHours           store.WeeklyHoursStore
	FixedNonWorkWeekdays  store.FixedNonWorkWeekdaysStore
	ScheduleBound         store.ScheduleBoundStore
	VacationEnt           store.VacationEntitlementStore
	NFCTags               store.NFCTagStore
	Schedules             store.ScheduleStore
	TeamMeetings          store.TeamMeetingStore
	ClosureDays           store.ClosureDayStore
	Holidays              store.HolidayStore
	Settings              store.SettingsStore

	ApiPairedClients   store.ApiPairedClientStore
	ApiPairingSessions store.ApiPairingSessionStore
	Server             config.ServerConfig

	Stamps          *stampspoll.Service
	LanEmployeeSync *lanemployeesync.Service
	Export          export.Data
	Backup          *backup.Service
	Audit           *audit.Logger
	AuditStore      *sqlite.AuditStore
}

// loginRateLimitPerMinute begrenzt Login-Versuche pro Client-IP (Brute-Force). IP kommt von chi RealIP / X-Forwarded-For.
const loginRateLimitPerMinute = 20

// pairRegisterRateLimitPerMinute begrenzt Pairing-Register-Versuche pro Client-IP.
const pairRegisterRateLimitPerMinute = 10

func NewRouter(d Deps) http.Handler {
	// chi's middleware.DefaultLogger ist per Default an os.Stdout gebunden und
	// erbt daher nicht den per log.SetOutput umgelenkten Output (z. B. die
	// rollierende Logdatei aus internal/logging.Setup). An log.Writer() neu
	// binden, damit Request-Zeilen denselben Weg wie alle anderen Server-Logs
	// nehmen.
	// NoColor nur, wenn stderr kein TTY ist (z. B. Umleitung); sonst Farben
	// im Terminal. ANSI wird fuer die Logdatei in internal/logging separat
	// entfernt (lineAnsiStripWriter).
	noColor := !isatty.IsTerminal(os.Stderr.Fd())
	middleware.DefaultLogger = middleware.RequestLogger(&middleware.DefaultLogFormatter{
		Logger:  golog.New(golog.Writer(), "", golog.LstdFlags),
		NoColor: noColor,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})

		pairingSvc := apipairing.New(d.ApiPairedClients)
		pairingSessions := apipairing.NewSessionService(d.ApiPairingSessions)
		dsh := &handler.DeviceStampsHandler{}
		dph := &handler.DevicePairHandler{
			Sessions: pairingSessions,
			Clients:  d.ApiPairedClients,
			Settings: d.Settings,
		}
		r.With(httprate.LimitByIP(pairRegisterRateLimitPerMinute, time.Minute)).
			Post("/device/pair/register", dph.PostRegister)
		r.Group(func(r chi.Router) {
			r.Use(apimw.BearerPairingAuth(pairingSvc))
			r.Get("/device/v1/stamps", dsh.Stamps)
		})

		ah := &handler.AuthHandler{Users: d.UserStore, Auth: d.Auth}
		r.Route("/auth", func(r chi.Router) {
			r.With(httprate.LimitByIP(loginRateLimitPerMinute, time.Minute)).Post("/login", ah.Login)
			r.Group(func(r chi.Router) {
				r.Use(apimw.AuthJWT(d.Auth))
				r.Post("/change-password", ah.ChangePassword)
				r.Post("/refresh", ah.Refresh)
			})
		})

		me := &handler.MeHandler{
			Users: d.UserStore, WorkPeriods: d.WorkPeriods, WeeklyHours: d.WeeklyHours,
			FixedNonWorkWeekdays: d.FixedNonWorkWeekdays,
			ScheduleBound:        d.ScheduleBound,
			VacationEnt: d.VacationEnt, Absences: d.Absences, CompensationDayClaims: d.CompensationDayClaims,
			Schedules: d.Schedules, TeamMeetings: d.TeamMeetings, Corrections: d.Corrections, Holidays: d.Holidays,
			Audit: d.Audit,
		}
		ch := &handler.ClosureHandler{
			Closures: d.ClosureDays, Holidays: d.Holidays, Users: d.UserStore,
			FixedNonWorkWeekdays: d.FixedNonWorkWeekdays, Absences: d.Absences,
			Audit: d.Audit,
		}
		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(d.Auth))
			r.Get("/me/times", me.Times)
			r.Get("/me/balance", me.Balance)
			r.Get("/me/vacation", me.Vacation)
			r.Get("/me/profile", me.Profile)
			r.Get("/me/schedule-bound", me.GetScheduleBound)
			r.Get("/me/schedule", me.Schedule)
			r.Get("/me/absences", me.ListAbsences)
			r.Get("/me/corrections", me.ListCorrections)
			r.Post("/me/corrections", me.CreateCorrection)
			r.Get("/closure-days", ch.List)
		})

		leitung := []string{string(model.RoleLeitung), string(model.RoleSuperadmin)}
		eh := &handler.EmployeeHandler{
			Users: d.UserStore, Groups: d.GroupStore, Auth: d.Auth, WorkPeriods: d.WorkPeriods,
			Corrections: d.Corrections, Absences: d.Absences, CompensationDayClaims: d.CompensationDayClaims,
			Holidays:    d.Holidays,
			ClosureDays: d.ClosureDays,
			WeeklyHours: d.WeeklyHours, VacationEnt: d.VacationEnt, NFCTags: d.NFCTags,
			FixedNonWorkWeekdays: d.FixedNonWorkWeekdays,
			ScheduleBound:        d.ScheduleBound,
			Schedules: d.Schedules, TeamMeetings: d.TeamMeetings, Audit: d.Audit,
		}
		sh := &handler.ScheduleHandler{
			Schedules:             d.Schedules,
			TeamMeetings:          d.TeamMeetings,
			Users:                 d.UserStore,
			Groups:                d.GroupStore,
			Absences:              d.Absences,
			Holidays:              d.Holidays,
			Closures:              d.ClosureDays,
			CompensationDayClaims: d.CompensationDayClaims,
			Audit:                 d.Audit,
			FixedNonWorkWeekdays:  d.FixedNonWorkWeekdays,
		}
		ex := &handler.ExportHandler{Users: d.UserStore, ExportData: d.Export}
		dh := &handler.DashboardHandler{
			Users: d.UserStore, WorkPeriods: d.WorkPeriods, Corrections: d.Corrections,
			Absences: d.Absences, CompensationDayClaims: d.CompensationDayClaims,
			Holidays: d.Holidays, Closures: d.ClosureDays,
			WeeklyHours: d.WeeklyHours, Settings: d.Settings, VacationEnt: d.VacationEnt,
			FixedNonWorkWeekdays: d.FixedNonWorkWeekdays,
			ScheduleBound:        d.ScheduleBound,
			Schedules:            d.Schedules,
		}
		hh := &handler.HolidayHandler{Holidays: d.Holidays, Audit: d.Audit}
		gh := &handler.GroupHandler{Groups: d.GroupStore, Audit: d.Audit}

		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(d.Auth))
			r.Use(apimw.RequireRole(leitung...))
			r.Get("/dashboard/team-overview", dh.TeamOverview)
			r.Get("/dashboard/schedule-gaps", dh.ScheduleGaps)
			r.Get("/employees", eh.List)
			r.Post("/employees", eh.Create)
			r.Patch("/employees/{id}", eh.Patch)
			r.Post("/employees/{id}/reset-password", eh.ResetPassword)
			r.Get("/groups", gh.List)
			r.Post("/groups", gh.Create)
			r.Put("/groups/order", gh.PutOrder)
			r.Patch("/groups/{id}", gh.Patch)
			r.Delete("/groups/{id}", gh.Delete)
			r.Get("/employees/{id}/times", eh.Times)
			r.Get("/employees/{id}/schedule", eh.Schedule)
			r.Get("/employees/{id}/balance", eh.Balance)
			r.Get("/employees/{id}/vacation", eh.Vacation)
			r.Post("/employees/{id}/work-periods", eh.CreateWorkPeriod)
			r.Delete("/employees/{id}/work-periods/{wpId}", eh.DeleteWorkPeriod)
			r.Post("/employees/{id}/corrections", eh.CreateCorrection)
			r.Get("/employees/{id}/corrections", eh.ListCorrections)
			r.Post("/employees/{id}/absences", eh.CreateAbsence)
			r.Get("/employees/{id}/absences", eh.ListAbsences)
			r.Delete("/employees/{id}/absences/{absenceId}", eh.DeleteAbsence)
			r.Get("/employees/{id}/compensation-day-claims", eh.ListCompensationDayClaims)
			r.Post("/employees/{id}/compensation-day-claims/{claimId}/waive", eh.WaiveCompensationDayClaim)
			r.Put("/employees/{id}/weekly-hours", eh.PutWeeklyHours)
			r.Get("/employees/{id}/weekly-hours", eh.GetWeeklyHours)
			r.Delete("/employees/{id}/weekly-hours/{whId}", eh.DeleteWeeklyHours)
			r.Put("/employees/{id}/vacation-entitlement", eh.PutVacationEntitlement)
			r.Get("/employees/{id}/vacation-entitlement", eh.GetVacationEntitlement)
			r.Delete("/employees/{id}/vacation-entitlement/{veId}", eh.DeleteVacationEntitlement)
			r.Put("/employees/{id}/fixed-non-work-weekdays", eh.PutFixedNonWorkWeekdays)
			r.Get("/employees/{id}/fixed-non-work-weekdays", eh.GetFixedNonWorkWeekdays)
			r.Delete("/employees/{id}/fixed-non-work-weekdays/{fnwId}", eh.DeleteFixedNonWorkWeekdays)
			r.Put("/employees/{id}/schedule-bound", eh.PutScheduleBound)
			r.Get("/employees/{id}/schedule-bound", eh.GetScheduleBound)
			r.Delete("/employees/{id}/schedule-bound/{sbId}", eh.DeleteScheduleBound)
			r.Post("/employees/{id}/nfc-tags", eh.PostNFCTag)
			r.Get("/employees/{id}/nfc-tags", eh.ListNFCTags)

			r.Get("/schedules", sh.ListWeek)
			r.Put("/schedules/week-notes", sh.PutWeekNotes)
			r.Get("/schedules/export-defaults", sh.ExportDefaults)
			r.Get("/schedules/export-excel", sh.ExportExcel)
			r.Post("/schedules/import-excel", sh.ImportExcel)
			r.Post("/schedules/preview-excel-import", sh.PreviewExcelImport)
			r.Post("/schedules", sh.Create)
			r.Put("/schedules/{id}", sh.Update)
			r.Delete("/schedules/{id}", sh.Delete)
			r.Post("/team-meetings", sh.PostTeamMeeting)
			r.Put("/team-meetings/{id}", sh.PutTeamMeeting)
			r.Delete("/team-meetings/{id}", sh.DeleteTeamMeeting)

			r.Post("/closure-days", ch.Create)
			r.Delete("/closure-days/{id}", ch.Delete)

			r.Get("/holidays", hh.List)

			r.Get("/export/csv", ex.CSV)
			r.Get("/export/pdf", ex.PDF)

			lanHealth := &handler.AndroidLanHealthHandler{Stamps: d.Stamps}
			r.Get("/android-lan/health-status", lanHealth.Get)
			lanRange := &handler.AndroidLanSyncStampsRangeHandler{Stamps: d.Stamps, Audit: d.Audit}
			r.Post("/android-lan/sync-stamps-range", lanRange.Post)
		})

		uh := &handler.UsersHandler{Users: d.UserStore, Auth: d.Auth, Audit: d.Audit}
		st := &handler.SettingsHandler{Settings: d.Settings, Audit: d.Audit}

		r.Group(func(r chi.Router) {
			r.Use(apimw.AuthJWT(d.Auth))
			r.Use(apimw.RequireRole(string(model.RoleSuperadmin)))
			r.Get("/users", uh.List)
			r.Post("/users", uh.Create)
			r.Patch("/users/{id}", uh.Patch)

			r.Post("/holidays/generate", hh.Generate)
			r.Post("/holidays", hh.Create)
			r.Delete("/holidays/{id}", hh.Delete)

			r.Get("/settings", st.List)
			r.Put("/settings/{key}", st.Put)

			aah := &handler.AndroidAPIClientsHandler{
				Clients:  d.ApiPairedClients,
				Sessions: pairingSessions,
				Server:   d.Server,
				Audit:    d.Audit,
			}
			r.Post("/android-api/clients/generate", aah.PostGenerate)
			r.Get("/android-api/clients", aah.List)
			r.Delete("/android-api/clients/{id}", aah.Delete)

			lanSync := &handler.AndroidLanEmployeeSyncHandler{Sync: d.LanEmployeeSync}
			r.Post("/android-lan/sync-employee-ids", lanSync.PostSync)
			r.Post("/android-lan/sync-employee-ids-all", lanSync.PostSyncAll)

			bh := &handler.BackupHandler{Backup: d.Backup}
			aud := &handler.AuditHandler{Store: d.AuditStore}
			r.Get("/admin/audit/events", aud.ListEvents)
			r.Get("/admin/audit/verify", aud.Verify)
			r.Get("/admin/audit/anchors", aud.ListAnchors)
			r.Get("/admin/backup/status", bh.GetStatus)
			r.Get("/admin/backup/browse", bh.GetBrowse)
			r.Post("/admin/backup/pick-folder", bh.PostPickFolder)
			r.Put("/admin/backup/config", bh.PutConfig)
			r.Post("/admin/backup/init-restic", bh.PostInitRestic)
			r.Post("/admin/backup/run-now", bh.PostRunNow)
		})
	})

	return r
}
