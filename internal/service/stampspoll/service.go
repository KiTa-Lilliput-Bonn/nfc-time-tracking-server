package stampspoll

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/importer"
	"nfc-time-tracking-server/internal/service/lanemployeesync"
	"nfc-time-tracking-server/internal/store"
)

// WatermarkSettingKey returns the settings key for incremental stamps fetch for a LAN target.
func WatermarkSettingKey(targetID string) string {
	return "stamps_poll_watermark_utc_" + strings.TrimSpace(targetID)
}

type targetHealth struct {
	reachable    bool
	lastErr      string
	lastCheckUTC time.Time
}

// Service polls the Flutter LAN API for stamps and updates work periods.
type Service struct {
	settings   store.SettingsStore
	apiClients store.ApiPairedClientStore
	punch      store.PunchStore
	periods    store.WorkPeriodStore
	tags       store.NFCTagStore
	claims     store.CompensationDayClaimStore
	users      store.UserStore
	http       *http.Client

	employeeLAN *lanemployeesync.Service

	mu           sync.Mutex
	targetHealth map[string]*targetHealth // keyed by AndroidLanTarget.ID
}

type pollTarget struct {
	Target bootstrap.AndroidLanTarget
	Bearer string
}

// NewService constructs a stamps poll service. employeeLAN may be nil (tests); when set, RunPoll also syncs employees to each LAN target.
func NewService(settings store.SettingsStore, apiClients store.ApiPairedClientStore, punch store.PunchStore, periods store.WorkPeriodStore, tags store.NFCTagStore, claims store.CompensationDayClaimStore, users store.UserStore, employeeLAN *lanemployeesync.Service) *Service {
	return &Service{
		settings:     settings,
		apiClients:   apiClients,
		punch:        punch,
		periods:      periods,
		tags:         tags,
		claims:       claims,
		users:        users,
		employeeLAN:  employeeLAN,
		http:         &http.Client{Timeout: 45 * time.Second},
		targetHealth: make(map[string]*targetHealth),
	}
}

// SetHTTPClient overrides the HTTP client (tests).
func (s *Service) SetHTTPClient(c *http.Client) {
	if c != nil {
		s.http = c
	}
}

func (s *Service) ensureHealth(id string) *targetHealth {
	if s.targetHealth == nil {
		s.targetHealth = make(map[string]*targetHealth)
	}
	h, ok := s.targetHealth[id]
	if !ok {
		h = &targetHealth{reachable: true}
		s.targetHealth[id] = h
	}
	return h
}

func (s *Service) noteTargetCheck(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	th := s.ensureHealth(id)
	th.lastCheckUTC = time.Now().UTC()
	if err != nil {
		th.lastErr = err.Error()
		th.reachable = false
	} else {
		th.lastErr = ""
		th.reachable = true
	}
}

func apiClientRevoked(c *model.ApiPairedClient) bool {
	if c == nil || c.RevokedAtUTC == nil {
		return false
	}
	return strings.TrimSpace(*c.RevokedAtUTC) != ""
}

func (s *Service) bearerForAPIClient(ctx context.Context, apiClientID string) string {
	id := strings.TrimSpace(apiClientID)
	if id == "" || s.apiClients == nil {
		return ""
	}
	c, err := s.apiClients.GetByID(ctx, id)
	if err != nil || c == nil || apiClientRevoked(c) {
		return ""
	}
	return strings.TrimSpace(c.Secret)
}

func (s *Service) preparePollTargets(ctx context.Context) ([]pollTarget, error) {
	list, err := bootstrap.LanTargetsFromSettings(ctx, s.settings)
	if err != nil {
		return nil, err
	}
	out := make([]pollTarget, 0, len(list))
	for _, t := range list {
		b := s.bearerForAPIClient(ctx, t.APIClientID)
		if strings.TrimSpace(b) == "" || strings.TrimSpace(t.Host) == "" {
			continue
		}
		out = append(out, pollTarget{Target: t, Bearer: b})
	}
	return out, nil
}

func (s *Service) pollEnabled(ctx context.Context) bool {
	if bootstrap.StampsPollIntervalSeconds(ctx, s.settings) <= 0 {
		return false
	}
	pts, err := s.preparePollTargets(ctx)
	if err != nil {
		return false
	}
	return len(pts) > 0
}

func lanBase(t bootstrap.AndroidLanTarget) string {
	return t.LanBaseURL()
}

// LanTargetHealth is per-device LAN status for the web UI.
type LanTargetHealth struct {
	ID           string     `json:"id"`
	Label        string     `json:"label,omitempty"`
	Mode         string     `json:"mode"` // disabled | ok | down
	Reachable    bool       `json:"reachable"`
	LastError    string     `json:"last_error,omitempty"`
	LastCheckUTC *time.Time `json:"last_check_utc,omitempty"`
}

// LanHealthPayload is returned by GET /android-lan/health-status.
type LanHealthPayload struct {
	Mode         string            `json:"mode"` // disabled | ok | down
	Reachable    bool              `json:"reachable"`
	LastError    string            `json:"last_error,omitempty"`
	LastCheckUTC *time.Time        `json:"last_check_utc,omitempty"`
	Targets      []LanTargetHealth `json:"targets"`
}

// LanHealth returns current LAN reachability for UI polling.
func (s *Service) LanHealth(ctx context.Context) LanHealthPayload {
	interval := bootstrap.StampsPollIntervalSeconds(ctx, s.settings)
	allTargets, err := bootstrap.LanTargetsFromSettings(ctx, s.settings)
	if err != nil {
		return LanHealthPayload{Mode: "disabled", Reachable: true, Targets: nil}
	}
	if interval <= 0 || len(allTargets) == 0 {
		return LanHealthPayload{Mode: "disabled", Reachable: true, Targets: []LanTargetHealth{}}
	}

	bearerByID := make(map[string]string, len(allTargets))
	for _, t := range allTargets {
		bearerByID[t.ID] = s.bearerForAPIClient(ctx, t.APIClientID)
	}

	s.mu.Lock()
	targetsOut := make([]LanTargetHealth, 0, len(allTargets))
	var maxCheck time.Time
	allReach := true
	firstErr := ""

	for _, t := range allTargets {
		bearer := bearerByID[t.ID]
		th := s.ensureHealth(t.ID)
		reach := th.reachable
		lastErr := th.lastErr
		checkUTC := th.lastCheckUTC

		mode := "ok"
		if strings.TrimSpace(bearer) == "" {
			mode = "disabled"
			reach = true
		} else if !reach {
			mode = "down"
			allReach = false
			if firstErr == "" && strings.TrimSpace(lastErr) != "" {
				firstErr = lastErr
			}
		}

		var lastCheckPtr *time.Time
		if !checkUTC.IsZero() {
			c := checkUTC
			lastCheckPtr = &c
			if c.After(maxCheck) {
				maxCheck = c
			}
		}

		targetsOut = append(targetsOut, LanTargetHealth{
			ID:           t.ID,
			Label:        t.Label,
			Mode:         mode,
			Reachable:    reach,
			LastError:    lastErr,
			LastCheckUTC: lastCheckPtr,
		})
	}
	s.mu.Unlock()

	aggMode := "ok"
	if !allReach {
		aggMode = "down"
	}
	var aggLastCheck *time.Time
	if !maxCheck.IsZero() {
		mc := maxCheck
		aggLastCheck = &mc
	}

	return LanHealthPayload{
		Mode:         aggMode,
		Reachable:    allReach,
		LastError:    firstErr,
		LastCheckUTC: aggLastCheck,
		Targets:      targetsOut,
	}
}

// ProbeHealth calls GET /health on one LAN device (no bearer).
func (s *Service) ProbeHealth(ctx context.Context, t bootstrap.AndroidLanTarget) error {
	base := lanBase(t)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/health", nil)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return err
	}
	res, err := s.http.Do(req)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return err
	}
	if res.StatusCode != http.StatusOK {
		e := fmt.Errorf("health: HTTP %d", res.StatusCode)
		s.noteTargetCheck(t.ID, e)
		return e
	}
	if strings.TrimSpace(string(body)) != "ok" {
		e := fmt.Errorf("health: unexpected body %q", strings.TrimSpace(string(body)))
		s.noteTargetCheck(t.ID, e)
		return e
	}
	s.noteTargetCheck(t.ID, nil)
	return nil
}

// RunPoll fetches stamps for all configured pollable targets.
func (s *Service) RunPoll(ctx context.Context) error {
	pts, err := s.preparePollTargets(ctx)
	if err != nil {
		return err
	}
	for _, pt := range pts {
		_ = s.runPollOne(ctx, pt)
	}
	s.pushTodayStampsToAllLanTargets(ctx, pts)
	if s.employeeLAN != nil {
		if _, err := s.employeeLAN.SyncAll(ctx, true); err != nil {
			log.Printf("lanemployeesync: SyncAll: %v", err)
		}
	}
	if len(pts) > 0 {
		log.Printf("stampspoll: RunPoll finished (%d LAN target(s); GET /v1/stamps, LAN push POST, employee sync)", len(pts))
	}
	return nil
}

func (s *Service) runPollOne(ctx context.Context, pt pollTarget) error {
	t := pt.Target
	wmKey := WatermarkSettingKey(t.ID)
	wm, _ := s.settings.Get(ctx, wmKey)
	wm = strings.TrimSpace(wm)
	q := url.Values{}
	if wm != "" {
		q.Set("from", wm)
	}
	_, err := s.runPollOneWithQuery(ctx, pt, q, wmKey, true)
	return err
}

// runPollOneWithQuery performs GET /v1/stamps with the given query string (e.g. from=watermark or from/to range).
// When updateWatermark is true and watermarkKey is non-empty, stamps_poll_watermark is advanced from response stamps.
func (s *Service) runPollOneWithQuery(ctx context.Context, pt pollTarget, query url.Values, watermarkKey string, updateWatermark bool) (inserted int, err error) {
	t := pt.Target
	base := lanBase(t)
	u, err := url.Parse(strings.TrimRight(base, "/") + "/v1/stamps")
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return 0, err
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(pt.Bearer))
	res, err := s.http.Do(req)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return 0, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return 0, err
	}
	if res.StatusCode != http.StatusOK {
		e := fmt.Errorf("stamps: HTTP %d", res.StatusCode)
		s.noteTargetCheck(t.ID, e)
		return 0, e
	}
	wires, err := ParseLANStampsResponse(body)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return 0, err
	}
	var maxTS time.Time
	hasWatermark := false
	for _, w := range wires {
		ts, err := parseTimestampUTC(w.Timestamp)
		if err != nil {
			continue
		}
		if !hasWatermark || ts.After(maxTS) {
			maxTS = ts
			hasWatermark = true
		}
	}
	var raw []model.RawPunch
	for _, w := range wires {
		empStr, err := employeeIDString(w.EmployeeID)
		if err != nil {
			continue
		}
		uid, err := strconv.Atoi(strings.TrimSpace(empStr))
		if err != nil || uid <= 0 {
			continue
		}
		ts, err := parseTimestampUTC(w.Timestamp)
		if err != nil {
			continue
		}
		tagUID, err := s.tags.TagUIDForUserAt(ctx, uid, ts)
		if err != nil || strings.TrimSpace(tagUID) == "" {
			continue
		}
		rp, err := WireToRawPunch(w, tagUID)
		if err != nil {
			continue
		}
		raw = append(raw, rp)
	}
	inserted, err = s.punch.InsertBatch(ctx, raw)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return inserted, err
	}
	_, err = importer.RecomputeAffectedDaysFromPunches(ctx, s.punch, s.periods, s.tags, s.claims, s.users, raw)
	if err != nil {
		s.noteTargetCheck(t.ID, err)
		return inserted, err
	}
	if updateWatermark && hasWatermark && strings.TrimSpace(watermarkKey) != "" {
		_ = s.settings.Set(ctx, watermarkKey, maxTS.UTC().Format(time.RFC3339Nano))
	}
	s.noteTargetCheck(t.ID, nil)
	return inserted, nil
}

// StartScheduler runs RunPoll on a timer; mirrors FTP importer pattern.
func (s *Service) StartScheduler(ctx context.Context) {
	go func() {
		idleWhenDisabled := 60 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !s.pollEnabled(context.Background()) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(idleWhenDisabled):
				}
				continue
			}
			interval := bootstrap.StampsPollIntervalSeconds(context.Background(), s.settings)
			if interval <= 0 {
				interval = 300
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(interval) * time.Second):
				_ = s.RunPoll(context.Background())
			}
		}
	}()
}

// StartRecoveryLoop probes GET /health periodically for each unreachable pollable target.
func (s *Service) StartRecoveryLoop(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !s.pollEnabled(context.Background()) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(60 * time.Second):
				}
				continue
			}

			pts, err := s.preparePollTargets(context.Background())
			if err != nil || len(pts) == 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(60 * time.Second):
				}
				continue
			}

			s.mu.Lock()
			var down []bootstrap.AndroidLanTarget
			allUp := true
			for _, pt := range pts {
				th := s.ensureHealth(pt.Target.ID)
				if !th.reachable {
					allUp = false
					down = append(down, pt.Target)
				}
			}
			s.mu.Unlock()

			if allUp {
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Second):
				}
				continue
			}

			for _, t := range down {
				_ = s.ProbeHealth(context.Background(), t)
				select {
				case <-ctx.Done():
					return
				case <-time.After(120 * time.Millisecond):
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()
}
