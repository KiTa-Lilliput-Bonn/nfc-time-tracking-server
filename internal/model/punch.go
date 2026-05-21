package model

import "time"

type RawPunch struct {
	ID         int       `json:"id"`
	PunchTime  time.Time `json:"punch_time"`
	NFCTagUID  string    `json:"nfc_tag_uid"`
	SourceFile string    `json:"source_file"`
	DeviceName string    `json:"device_name"`
	ImportedAt time.Time `json:"imported_at"`
}

// LanSyncPunch is a stored raw punch with resolved server user id (LAN employeeId).
type LanSyncPunch struct {
	UserID int
	Punch  RawPunch
}
