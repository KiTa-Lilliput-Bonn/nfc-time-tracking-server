package entrylock

import "time"

const MutableWindow = 24 * time.Hour

// IsMutable reports whether an entry may still be changed by non-superadmin actors.
func IsMutable(createdAt time.Time, now time.Time) bool {
	return !now.After(createdAt.UTC().Add(MutableWindow))
}
