package stampspoll

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nfc-time-tracking-server/internal/model"
)

const lanPushTimeZoneIana = "Europe/Berlin"

var (
	berlinOnce sync.Once
	berlinLoc  *time.Location
)

func europeBerlinLocation() *time.Location {
	berlinOnce.Do(func() {
		var err error
		berlinLoc, err = time.LoadLocation(lanPushTimeZoneIana)
		if err != nil {
			log.Printf("stampspoll: %s: %v; LAN push timestamps fall back to UTC", lanPushTimeZoneIana, err)
			berlinLoc = time.UTC
		}
	})
	return berlinLoc
}

type lanPushStampWire struct {
	EmployeeID       string `json:"employeeId"`
	Timestamp        string `json:"timestamp"`
	TimeZoneIana     string `json:"timeZoneIana"`
	UtcOffsetMinutes int    `json:"utcOffsetMinutes"`
}

func utcOffsetMinutesForLocation(t time.Time, loc *time.Location) int {
	tb := t.In(loc)
	_, offset := tb.Zone()
	return offset / 60
}

func buildLanPushStampsJSON(rows []model.LanSyncPunch, loc *time.Location) ([]byte, error) {
	iana := lanPushTimeZoneIana
	if loc == time.UTC {
		iana = "UTC"
	}
	stamps := make([]lanPushStampWire, 0, len(rows))
	for _, row := range rows {
		t := row.Punch.PunchTime.UTC()
		stamps = append(stamps, lanPushStampWire{
			EmployeeID:       strconv.Itoa(row.UserID),
			Timestamp:        t.In(loc).Format(time.RFC3339Nano),
			TimeZoneIana:     iana,
			UtcOffsetMinutes: utcOffsetMinutesForLocation(t, loc),
		})
	}
	return json.Marshal(map[string][]lanPushStampWire{"stamps": stamps})
}

func (s *Service) pushTodayStampsToAllLanTargets(ctx context.Context, pts []pollTarget) {
	if len(pts) == 0 {
		return
	}
	utcDate := time.Now().UTC().Format("2006-01-02")
	body, err := s.lanPushJSONForUtcDate(ctx, utcDate)
	if err != nil {
		log.Printf("stampspoll: list punches for LAN push %s: %v", utcDate, err)
		return
	}
	ok := 0
	for _, pt := range pts {
		if s.pushStampsPOSTOne(ctx, pt, body) {
			ok++
		}
	}
	if len(pts) > 0 {
		log.Printf("stampspoll: LAN push UTC day=%s payload_bytes=%d POST ok=%d/%d",
			utcDate, len(body), ok, len(pts))
	}
}

// lanPushJSONForUtcDate builds the POST /v1/stamps JSON body for one UTC calendar day (YYYY-MM-DD).
func (s *Service) lanPushJSONForUtcDate(ctx context.Context, utcDate string) ([]byte, error) {
	rows, err := s.punch.ListByUTCDateForLanSync(ctx, utcDate)
	if err != nil {
		return nil, err
	}
	loc := europeBerlinLocation()
	return buildLanPushStampsJSON(rows, loc)
}

func (s *Service) pushStampsPOSTOne(ctx context.Context, pt pollTarget, body []byte) bool {
	u := strings.TrimRight(lanBase(pt.Target), "/") + "/v1/stamps"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		log.Printf("stampspoll: LAN push %s new request: %v", pt.Target.ID, err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(pt.Bearer))
	res, err := s.http.Do(req)
	if err != nil {
		log.Printf("stampspoll: LAN push POST %s: %v", pt.Target.ID, err)
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		log.Printf("stampspoll: LAN push POST %s: HTTP %d %s", pt.Target.ID, res.StatusCode, strings.TrimSpace(string(respBody)))
		return false
	}
	return true
}
