package model

type NFCTag struct {
	ID           int    `json:"id"`
	TagUID       string `json:"tag_uid"`
	UserID       int    `json:"user_id"`
	AssignedFrom string `json:"assigned_from"`
}
