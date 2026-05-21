package store

import (
	"errors"
	"fmt"
	"time"
)

var ErrNFCTagAssigned = errors.New("nfc tag assigned")

type NFCTagAssignedError struct {
	DisplayName  string
	AssignedFrom string
}

func (e *NFCTagAssignedError) Error() string {
	from := e.AssignedFrom
	if t, err := time.Parse("2006-01-02", e.AssignedFrom); err == nil {
		from = t.Format("02.01.2006")
	}
	return fmt.Sprintf("NFC-Tag ist bereits %s zugeordnet (gültig ab %s).", e.DisplayName, from)
}

func (e *NFCTagAssignedError) Is(target error) bool {
	return target == ErrNFCTagAssigned
}
