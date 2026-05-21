//go:build darwin

package folderpick

import "testing"

func TestAppleScriptQuote(t *testing.T) {
	got := appleScriptQuote(`/tmp/foo"bar\baz`)
	want := `"/tmp/foo\"bar\\baz"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
