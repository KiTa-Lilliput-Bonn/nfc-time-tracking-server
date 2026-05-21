package audit

import "encoding/json"

// JSONSummary marshals v for audit storage; on error returns "{}".
func JSONSummary(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
