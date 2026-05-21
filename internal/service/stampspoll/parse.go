package stampspoll

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"nfc-time-tracking-server/internal/model"
)

const (
	stampMinUtcOffsetMinutes = -840
	stampMaxUtcOffsetMinutes = 840
)

type lanStampWire struct {
	EmployeeID         interface{} `json:"employeeId"`
	Timestamp          string      `json:"timestamp"`
	TimeZoneIana       *string     `json:"timeZoneIana"`
	UtcOffsetMinutes   interface{} `json:"utcOffsetMinutes"`
}

// ParseLANStampsResponse decodes JSON array from GET /v1/stamps and returns raw punches (nfc_tag_uid filled by caller).
func ParseLANStampsResponse(body []byte) ([]lanStampWire, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, fmt.Errorf("stamps json: %w", err)
	}
	out := make([]lanStampWire, 0, len(arr))
	for i, raw := range arr {
		var w lanStampWire
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, fmt.Errorf("stamp item %d: %w", i, err)
		}
		out = append(out, w)
	}
	return out, nil
}

func employeeIDString(v interface{}) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", fmt.Errorf("missing employeeId")
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return "", fmt.Errorf("empty employeeId")
		}
		return s, nil
	case float64:
		return strconv.FormatInt(int64(x), 10), nil
	default:
		s := strings.TrimSpace(fmt.Sprint(x))
		if s == "" {
			return "", fmt.Errorf("empty employeeId")
		}
		return s, nil
	}
}

func parseTimestampUTC(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("missing timestamp")
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 timestamp, got %q", s)
}

func validateIanaAndOffset(ianaRaw string, offsetRaw interface{}, lineCtx string) error {
	ianaRaw = strings.TrimSpace(ianaRaw)
	if ianaRaw != "" {
		if strings.ContainsAny(ianaRaw, ";\n\r") {
			return fmt.Errorf("invalid timeZoneIana in %s", lineCtx)
		}
	}
	if offsetRaw == nil {
		return nil
	}
	var offStr string
	switch x := offsetRaw.(type) {
	case string:
		offStr = strings.TrimSpace(x)
	case float64:
		offStr = strconv.Itoa(int(x))
	default:
		offStr = strings.TrimSpace(fmt.Sprint(x))
	}
	if offStr == "" {
		return nil
	}
	off, err := strconv.Atoi(offStr)
	if err != nil {
		return fmt.Errorf("invalid utcOffsetMinutes in %s: %w", lineCtx, err)
	}
	if off < stampMinUtcOffsetMinutes || off > stampMaxUtcOffsetMinutes {
		return fmt.Errorf("utcOffsetMinutes out of range in %s", lineCtx)
	}
	return nil
}

// WireToRawPunch validates wire row and builds RawPunch with nfcTagUID; employeeId must be server user id string.
func WireToRawPunch(w lanStampWire, nfcTagUID string) (model.RawPunch, error) {
	emp, err := employeeIDString(w.EmployeeID)
	if err != nil {
		return model.RawPunch{}, err
	}
	iana := ""
	if w.TimeZoneIana != nil {
		iana = *w.TimeZoneIana
	}
	if err := validateIanaAndOffset(iana, w.UtcOffsetMinutes, emp); err != nil {
		return model.RawPunch{}, err
	}
	ts, err := parseTimestampUTC(w.Timestamp)
	if err != nil {
		return model.RawPunch{}, err
	}
	if strings.TrimSpace(nfcTagUID) == "" {
		return model.RawPunch{}, fmt.Errorf("no nfc tag for employee %s at %s", emp, ts.UTC().Format(time.RFC3339Nano))
	}
	return model.RawPunch{
		PunchTime:  ts,
		NFCTagUID:  nfcTagUID,
		SourceFile: "lan_stamps",
		DeviceName: "lan_api",
	}, nil
}
