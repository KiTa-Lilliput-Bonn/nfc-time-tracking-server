//go:build !darwin && !windows && !linux

package folderpick

func platformAvailable() bool {
	return false
}

func platformPick(string) (string, error) {
	return "", ErrUnavailable
}
