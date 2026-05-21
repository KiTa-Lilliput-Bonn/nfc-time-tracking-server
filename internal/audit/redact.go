package audit

import (
	"strings"
)

// RedactSettingValue masks sensitive setting values for audit summaries.
func RedactSettingValue(key, value string) string {
	lower := strings.ToLower(key)
	for _, frag := range []string{"password", "secret", "token"} {
		if strings.Contains(lower, frag) {
			return "[redacted]"
		}
	}
	return value
}
