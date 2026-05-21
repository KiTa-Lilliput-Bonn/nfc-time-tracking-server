package logging

import (
	"bytes"
	"testing"
)

func TestLineAnsiStripWriter_stripsCSI(t *testing.T) {
	var out bytes.Buffer
	s := newLineAnsiStripWriter(&out)
	if _, err := s.Write([]byte("plain\x1b[32m green\x1b[0m\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := out.String(); got != "plain green\n" {
		t.Errorf("got %q want %q", got, "plain green\n")
	}
}

func TestLineAnsiStripWriter_FlushPartialLine(t *testing.T) {
	var out bytes.Buffer
	s := newLineAnsiStripWriter(&out)
	if _, err := s.Write([]byte("no newline yet")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no flush before newline, got %q", out.String())
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := out.String(); got != "no newline yet" {
		t.Errorf("got %q want %q", got, "no newline yet")
	}
}
