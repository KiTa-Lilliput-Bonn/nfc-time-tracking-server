package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"nfc-time-tracking-server/internal/api"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/config"
	"nfc-time-tracking-server/internal/logging"
	"nfc-time-tracking-server/internal/model"
	authsvc "nfc-time-tracking-server/internal/service/auth"
	"nfc-time-tracking-server/internal/service/backup"
	"nfc-time-tracking-server/internal/service/compensationday"
	"nfc-time-tracking-server/internal/service/export"
	"nfc-time-tracking-server/internal/service/holidaysync"
	"nfc-time-tracking-server/internal/service/lanemployeesync"
	"nfc-time-tracking-server/internal/service/stampspoll"
	"nfc-time-tracking-server/internal/store/sqlite"
	"nfc-time-tracking-server/internal/web"
)

func main() {
	cfgPath := os.Getenv("NFC_CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Printf("Could not load config from %s, using defaults: %v", cfgPath, err)
		cfg = config.Defaults()
	}
	cfg.ApplyEnv()

	if err := bootstrap.ValidateTestModeForStartup(); err != nil {
		log.Fatalf("test mode: %v", err)
	}
	testMode, testModeOn := bootstrap.ActiveTestMode()

	logCloser, err := logging.Setup(cfg.Logging)
	if err != nil {
		log.Fatalf("logging setup: %v", err)
	}
	defer logCloser.Close()

	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0o755); err != nil {
		log.Fatalf("create database directory: %v", err)
	}

	jwtSecret, err := bootstrap.ResolveJWTSecret(cfg.Auth.JWTSecret, cfg.Database.Path)
	if err != nil {
		log.Fatalf("jwt secret: %v", err)
	}

	expiryH := cfg.Auth.TokenExpiryHours
	if expiryH <= 0 {
		expiryH = 8
	}
	authService := authsvc.New(jwtSecret, expiryH)

	db, err := sqlite.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	users := sqlite.NewUserStore(db)
	holidays := sqlite.NewHolidayStore(db)
	ctx := context.Background()

	n, err := users.Count(ctx)
	if err != nil {
		log.Fatalf("count users: %v", err)
	}
	if n == 0 {
		mustChange := true
		pw := authsvc.GenerateRandomPassword(16)
		if testModeOn {
			pw = testMode.AdminPassword
			mustChange = false
		}
		hash, err := authService.HashPassword(pw)
		if err != nil {
			log.Fatalf("hash bootstrap password: %v", err)
		}
		u := &model.User{
			Username:           "admin",
			PasswordHash:       hash,
			DisplayName:        "Administrator",
			Role:               model.RoleSuperadmin,
			Active:             true,
			MustChangePassword: mustChange,
		}
		if err := users.Create(ctx, u); err != nil {
			log.Fatalf("create bootstrap admin: %v", err)
		}
		if testModeOn {
			log.Printf("bootstrap: created superadmin user %q (test mode)", u.Username)
		} else {
			log.Printf("bootstrap: created superadmin user %q with one-time password: %s", u.Username, pw)
		}
	}

	holidayCalLoc, holidayBerlin := holidaysync.HolidayCalendarLocation()
	if !holidayBerlin {
		log.Printf("holidays: Europe/Berlin unavailable, using UTC for calendar years")
	}
	syncNRWHolidays := func() {
		var years []int
		if testModeOn {
			years = bootstrap.TestModeHolidayYears()
		} else {
			years = holidaysync.CurrentAndNextCalendarYears(time.Now(), holidayCalLoc)
		}
		if err := holidaysync.EnsureNRWHolidays(ctx, holidays, years); err != nil {
			log.Printf("holidays ensure %v: %v", years, err)
		}
	}
	syncNRWHolidays()
	if !testModeOn {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				syncNRWHolidays()
			}
		}()
	}

	workPeriods := sqlite.NewWorkPeriodStore(db)
	schedules := sqlite.NewScheduleStore(db)
	teamMeetings := sqlite.NewTeamMeetingStore(db)
	absences := sqlite.NewAbsenceStore(db)
	corrections := sqlite.NewCorrectionStore(db)
	compensationDayClaims := sqlite.NewCompensationDayClaimStore(db)
	closures := sqlite.NewClosureDayStore(db)
	weeklyHours := sqlite.NewWeeklyHoursStore(db)
	fixedNonWorkWeekdays := sqlite.NewFixedNonWorkWeekdaysStore(db)
	scheduleBound := sqlite.NewScheduleBoundStore(db)
	settings := sqlite.NewSettingsStore(db)
	punches := sqlite.NewPunchStore(db)
	nfcTags := sqlite.NewNFCTagStore(db)

	if err := bootstrap.SeedBackupTargetPath(ctx, settings, cfg.BackupTargetPath); err != nil {
		log.Printf("bootstrap backup target path: %v", err)
	}

	if err := compensationday.BootstrapScanUsers(ctx, users, fixedNonWorkWeekdays, workPeriods, corrections, compensationDayClaims); err != nil {
		log.Printf("bootstrap compensation day claims: %v", err)
	}

	apiClients := sqlite.NewApiPairedClientStore(db)
	apiPairingSessions := sqlite.NewApiPairingSessionStore(db)
	lanEmpSync := lanemployeesync.NewService(users, nfcTags, settings, apiClients)
	stampsSvc := stampspoll.NewService(settings, apiClients, punches, workPeriods, nfcTags, compensationDayClaims, fixedNonWorkWeekdays, users, lanEmpSync)
	stampsSvc.StartScheduler(ctx)
	stampsSvc.StartRecoveryLoop(ctx)
	if iv := bootstrap.StampsPollIntervalSeconds(ctx, settings); iv > 0 {
		if targets, err := bootstrap.LanTargetsFromSettings(ctx, settings); err == nil && len(targets) > 0 {
			log.Printf("lan stamps: scheduled poll every %d s for %d LAN target(s) (from system settings)", iv, len(targets))
		}
	}

	auditStore := sqlite.NewAuditStore(db)
	auditLog := &audit.Logger{Store: auditStore}
	if err := auditLog.RunRetention(ctx); err != nil {
		log.Printf("audit retention: %v", err)
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := auditLog.RunRetention(context.Background()); err != nil {
				log.Printf("audit retention: %v", err)
			}
		}
	}()

	backupSvc := &backup.Service{
		Settings:     settings,
		DB:           db,
		AuditStore:   auditStore,
		DatabasePath: cfg.Database.Path,
	}
	go func() {
		run := func() {
			runCtx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
			defer cancel()
			if err := backupSvc.RunScheduled(runCtx); err != nil {
				log.Printf("scheduled backup: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(45 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()

	apiHandler := api.NewRouter(api.Deps{
		UserStore:          users,
		GroupStore:         sqlite.NewGroupStore(db),
		Auth:               authService,
		Server:             cfg.Server,
		ApiPairedClients:   apiClients,
		ApiPairingSessions: apiPairingSessions,
		LanEmployeeSync:       lanEmpSync,
		WorkPeriods:           workPeriods,
		Corrections:           corrections,
		Absences:              absences,
		CompensationDayClaims: compensationDayClaims,
		WeeklyHours:           weeklyHours,
		FixedNonWorkWeekdays:  fixedNonWorkWeekdays,
		ScheduleBound:         scheduleBound,
		VacationEnt:           sqlite.NewVacationEntitlementStore(db),
		NFCTags:               nfcTags,
		Schedules:             schedules,
		TeamMeetings:          teamMeetings,
		ClosureDays:           closures,
		Holidays:              holidays,
		Settings:              settings,
		Stamps:                stampsSvc,
		Backup:                backupSvc,
		Audit:                 auditLog,
		AuditStore:            auditStore,
		Export: export.Data{
			WorkPeriods: workPeriods,
			Schedules:   schedules,
			Absences:    absences,
			Holidays:    holidays,
			Closures:    closures,
			WeeklyHours: weeklyHours,
			FixedNonWorkWeekdays: fixedNonWorkWeekdays,
			ScheduleBound:        scheduleBound,
			Settings:             settings,
			Users:       users,
		},
	})

	var handler http.Handler = apiHandler
	if staticFS, err := web.FS(); err == nil {
		handler = api.WithSPA(apiHandler, staticFS)
	} else {
		log.Printf("web UI embed: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("NFC Time Tracking Server listening on http://%s", addr)
	srv := &http.Server{Addr: addr, Handler: handler}
	runPlatformUI(srv, cfg)
}
