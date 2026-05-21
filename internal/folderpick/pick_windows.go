//go:build windows

package folderpick

import (
	"errors"

	"github.com/sqweek/dialog"
)

func platformAvailable() bool {
	return true
}

func platformPick(startDir string) (string, error) {
	d := dialog.Directory().Title(pickTitle)
	if startDir != "" {
		d.SetStartDir(startDir)
	}
	chosen, err := d.Browse()
	if err != nil {
		if errors.Is(err, dialog.ErrCancelled) {
			return "", ErrCancelled
		}
		return "", err
	}
	return chosen, nil
}
