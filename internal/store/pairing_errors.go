package store

import "errors"

var (
	ErrPairingSessionNotFound = errors.New("pairing session not found")
	ErrPairingSessionConsumed = errors.New("pairing session already consumed")
	ErrPairingSessionExpired  = errors.New("pairing session expired")
)
