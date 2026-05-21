// Package lanemployeesync pushes active employees (user id, display name, NFC tag) to the
// Flutter app's LAN API (GET /v1/employees, POST /v1/employee-ids, DELETE inactive). Used by
// the admin HTTP handler and by the stamps poll scheduler.
package lanemployeesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/bootstrap"
	"nfc-time-tracking-server/internal/lanhost"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store"
)

// Service coordinates LAN employee sync against the Flutter app.
type Service struct {
	Users    store.UserStore
	NFCTags  store.NFCTagStore
	Settings store.SettingsStore
	Clients  store.ApiPairedClientStore
	HTTP     *http.Client
}

// NewService constructs a LAN employee sync service.
func NewService(users store.UserStore, tags store.NFCTagStore, settings store.SettingsStore, clients store.ApiPairedClientStore) *Service {
	return &Service{
		Users:    users,
		NFCTags:  tags,
		Settings: settings,
		Clients:  clients,
	}
}

func (s *Service) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 45 * time.Second}
}

// SetHTTPClient overrides the HTTP client (tests).
func (s *Service) SetHTTPClient(c *http.Client) {
	s.HTTP = c
}

// SecretForAPIClient resolves the paired API client for a target's api_client_id.
func (s *Service) SecretForAPIClient(ctx context.Context, clientID string) (*model.ApiPairedClient, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil, errors.New("api_client_id missing on target")
	}
	c, err := s.Clients.GetByID(ctx, clientID)
	if err != nil || c == nil {
		return nil, errors.New("api client not found")
	}
	if c.RevokedAtUTC != nil && strings.TrimSpace(*c.RevokedAtUTC) != "" {
		return nil, errors.New("api client revoked")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return nil, errors.New("api client has no secret")
	}
	return c, nil
}

type appEmployeeRecord struct {
	Name   string
	NfcTag string
}

// Execute runs employee sync for one LAN base URL and bearer secret.
func (s *Service) Execute(ctx context.Context, base, secret string) (map[string]interface{}, int, string) {
	return s.executeLanEmployeeSync(ctx, base, secret)
}

// SyncAll runs sync for every configured LAN target. If logErrors is true, logs per-target
// failures (used from the stamps poll background job). Returns the same payload shape as
// POST /android-lan/sync-employee-ids-all.
func (s *Service) SyncAll(ctx context.Context, logErrors bool) (map[string]interface{}, error) {
	targets, err := bootstrap.LanTargetsFromSettings(ctx, s.Settings)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]interface{}, 0, len(targets))
	createdTotal, updatedTotal, skippedTotal, removedTotal, failTotal := 0, 0, 0, 0, 0

	for _, t := range targets {
		row := map[string]interface{}{
			"target_id": t.ID,
			"label":     t.Label,
		}
		if err := lanhost.ValidateAndroidLANHostContext(ctx, t.Host); err != nil {
			row["error"] = err.Error()
			if logErrors {
				log.Printf("lanemployeesync: target %s: %v", t.ID, err)
			}
			results = append(results, row)
			continue
		}
		c, err := s.SecretForAPIClient(ctx, t.APIClientID)
		if err != nil {
			row["error"] = err.Error()
			if logErrors {
				log.Printf("lanemployeesync: target %s: %v", t.ID, err)
			}
			results = append(results, row)
			continue
		}
		base := t.LanBaseURL()
		secret := strings.TrimSpace(c.Secret)
		out, status, msg := s.executeLanEmployeeSync(ctx, base, secret)
		if status != http.StatusOK {
			row["error"] = msg
			if logErrors {
				log.Printf("lanemployeesync: target %s: %s", t.ID, msg)
			}
			results = append(results, row)
			continue
		}
		for k, v := range out {
			row[k] = v
		}
		createdTotal += sliceLen(out["created"])
		updatedTotal += sliceLen(out["updated"])
		skippedTotal += sliceLen(out["skipped_already_in_app"])
		removedTotal += sliceLen(out["removed_from_app"])
		failTotal += sliceLen(out["failures"])
		results = append(results, row)
	}
	return map[string]interface{}{
		"results": results,
		"summary": map[string]int{
			"targets":        len(targets),
			"created_total":  createdTotal,
			"updated_total":  updatedTotal,
			"skipped_total":  skippedTotal,
			"removed_total":  removedTotal,
			"failures_total": failTotal,
		},
	}, nil
}

func sliceLen(v interface{}) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		return rv.Len()
	}
	return 0
}

func (s *Service) executeLanEmployeeSync(ctx context.Context, base, secret string) (map[string]interface{}, int, string) {
	appEmployees, status, getBody, err := s.loadAppEmployees(ctx, base, secret)
	if err != nil {
		return nil, status, err.Error()
	}
	if status != http.StatusOK {
		msg := strings.TrimSpace(string(getBody))
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return nil, http.StatusBadGateway, "app GET /v1/employees: " + msg
	}

	candidates, err := s.NFCTags.ListActiveUserIDsWithOpenNFCTag(ctx)
	if err != nil {
		return nil, http.StatusInternalServerError, "nfc query failed"
	}

	type row struct {
		UserID int    `json:"user_id"`
		IDStr  string `json:"employee_id"`
		Name   string `json:"name"`
		NfcUID string `json:"nfc_tag_uid"`
	}
	type removedRow struct {
		UserID int    `json:"user_id"`
		IDStr  string `json:"employee_id"`
	}
	created := make([]row, 0)
	updated := make([]row, 0)
	skipped := make([]int, 0)
	removed := make([]removedRow, 0)
	failures := make([]map[string]string, 0)

	for _, uid := range candidates {
		idStr := strconv.Itoa(uid)

		u, err := s.Users.GetByID(ctx, uid)
		if err != nil || u == nil {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": "user not found"})
			continue
		}
		tagUID, err := s.NFCTags.LatestOpenTagUID(ctx, uid)
		if err != nil {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": "nfc tag lookup failed"})
			continue
		}
		if strings.TrimSpace(tagUID) == "" {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": "no active nfc tag"})
			continue
		}

		displayName := strings.TrimSpace(u.DisplayName)
		payload := map[string]string{
			"id":       idStr,
			"name":     displayName,
			"nfcTagId": tagUID,
		}

		appRec, inApp := appEmployees[idStr]
		if inApp && appRecordMatchesServer(appRec, displayName, tagUID) {
			skipped = append(skipped, uid)
			continue
		}

		st, respTxt, postErr := s.postEmployeeUpsert(ctx, base, secret, payload)
		if postErr != nil {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": postErr.Error()})
			continue
		}

		switch st {
		case http.StatusCreated:
			created = append(created, row{UserID: uid, IDStr: idStr, Name: displayName, NfcUID: tagUID})
			appEmployees[idStr] = appEmployeeRecord{Name: displayName, NfcTag: tagUID}
		case http.StatusOK:
			if respTxt == "unchanged" {
				skipped = append(skipped, uid)
				continue
			}
			if respTxt == "ok" {
				updated = append(updated, row{UserID: uid, IDStr: idStr, Name: displayName, NfcUID: tagUID})
				appEmployees[idStr] = appEmployeeRecord{Name: displayName, NfcTag: tagUID}
				continue
			}
			failures = append(failures, map[string]string{"employee_id": idStr, "error": "unexpected POST body: " + respTxt})
		default:
			failures = append(failures, map[string]string{"employee_id": idStr, "error": fmt.Sprintf("HTTP %d", st)})
		}
	}

	appIDsSorted := make([]string, 0, len(appEmployees))
	for id := range appEmployees {
		appIDsSorted = append(appIDsSorted, id)
	}
	sort.Strings(appIDsSorted)
	for _, idStr := range appIDsSorted {
		uid, err := strconv.Atoi(idStr)
		if err != nil {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": "invalid employee_id"})
			continue
		}
		u, err := s.Users.GetByID(ctx, uid)
		if err != nil || u == nil {
			continue
		}
		if u.Active {
			continue
		}
		st, delErr := s.deleteLanEmployee(ctx, base, secret, idStr)
		if delErr != nil {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": delErr.Error()})
			continue
		}
		if st != http.StatusNoContent && st != http.StatusNotFound {
			failures = append(failures, map[string]string{"employee_id": idStr, "error": fmt.Sprintf("HTTP %d", st)})
			continue
		}
		delete(appEmployees, idStr)
		removed = append(removed, removedRow{UserID: uid, IDStr: idStr})
	}

	appList := make([]string, 0, len(appEmployees))
	for id := range appEmployees {
		appList = append(appList, id)
	}
	sort.Strings(appList)

	return map[string]interface{}{
		"lan_base_url":           base,
		"app_employee_ids_after": appList,
		"created":                created,
		"updated":                updated,
		"skipped_already_in_app": skipped,
		"removed_from_app":       removed,
		"failures":               failures,
	}, http.StatusOK, ""
}

func appRecordMatchesServer(rec appEmployeeRecord, displayName, tagUID string) bool {
	if strings.TrimSpace(rec.Name) != strings.TrimSpace(displayName) {
		return false
	}
	return strings.TrimSpace(rec.NfcTag) == strings.TrimSpace(tagUID)
}

func (s *Service) loadAppEmployees(ctx context.Context, base, secret string) (map[string]appEmployeeRecord, int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v1/employees", nil)
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)

	res, err := s.httpClient().Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, nil, errors.New("lan request failed: " + err.Error())
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, res.StatusCode, nil, errors.New("read body: " + err.Error())
	}
	if res.StatusCode != http.StatusOK {
		return nil, res.StatusCode, body, nil
	}
	m, err := parseAppEmployeesJSON(body)
	if err != nil {
		return nil, http.StatusBadGateway, body, errors.New("invalid JSON from app: " + err.Error())
	}
	return m, http.StatusOK, body, nil
}

func parseAppEmployeesJSON(raw []byte) (map[string]appEmployeeRecord, error) {
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	m := make(map[string]appEmployeeRecord, len(arr))
	for _, obj := range arr {
		id := normalizeJSONScalarToEmployeeID(obj["id"])
		if id == "" {
			continue
		}
		name := ""
		if obj["name"] != nil {
			name = strings.TrimSpace(fmt.Sprint(obj["name"]))
		}
		nfc := ""
		if obj["nfcTagId"] != nil {
			nfc = strings.TrimSpace(fmt.Sprint(obj["nfcTagId"]))
		}
		m[id] = appEmployeeRecord{Name: name, NfcTag: nfc}
	}
	return m, nil
}

func normalizeJSONScalarToEmployeeID(v interface{}) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func (s *Service) postEmployeeUpsert(ctx context.Context, base, secret string, payload map[string]string) (int, string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v1/employee-ids", bytes.NewReader(b))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.httpClient().Do(req)
	if err != nil {
		return 0, "", errors.New("lan POST failed: " + err.Error())
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res.StatusCode, "", err
	}
	txt := strings.TrimSpace(string(body))
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		msg := txt
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return res.StatusCode, txt, errors.New(msg)
	}
	return res.StatusCode, txt, nil
}

func (s *Service) deleteLanEmployee(ctx context.Context, base, secret, idStr string) (int, error) {
	delURL := strings.TrimRight(base, "/") + "/v1/employee-ids/" + url.PathEscape(idStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)

	res, err := s.httpClient().Do(req)
	if err != nil {
		return 0, errors.New("lan DELETE failed: " + err.Error())
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return res.StatusCode, err
	}
	if res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotFound {
		return res.StatusCode, nil
	}
	txt := strings.TrimSpace(string(body))
	if txt == "" {
		txt = fmt.Sprintf("HTTP %d", res.StatusCode)
	}
	return res.StatusCode, errors.New(txt)
}
