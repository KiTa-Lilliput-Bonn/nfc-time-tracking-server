package logging

import (
	"bytes"
	"io"
	"regexp"
)

// csiSequence matches CSI escape sequences (Farben, Cursor, etc.), die z. B.
// chi's middleware.Logger in TTY-Modus ausgibt.
var csiSequence = regexp.MustCompile("\x1b\\[[\\x30-\\x3f]*[\\x20-\\x2f]*[\\x40-\\x7e]")

// lineAnsiStripWriter puffert bis zum Zeilenende und schreibt nur vollstaendige
// Zeilen in den Ziel-Writer, nachdem ANSI-Steuersequenzen entfernt wurden.
// So kann stderr farbige Zeilen bekommen (gleiche Bytes + ANSI) und die
// Logdatei eine saubere Variante.
type lineAnsiStripWriter struct {
	w   io.Writer
	buf []byte
}

func newLineAnsiStripWriter(w io.Writer) *lineAnsiStripWriter {
	return &lineAnsiStripWriter{w: w}
}

func (s *lineAnsiStripWriter) Write(p []byte) (int, error) {
	orig := len(p)
	s.buf = append(s.buf, p...)
	for {
		idx := bytes.IndexByte(s.buf, '\n')
		if idx < 0 {
			return orig, nil
		}
		line := s.buf[:idx+1]
		s.buf = s.buf[idx+1:]
		clean := csiSequence.ReplaceAll(line, nil)
		if len(clean) > 0 {
			if _, err := s.w.Write(clean); err != nil {
				return 0, err
			}
		}
	}
}

// Flush schreibt einen evtl. verbleibenden Puffer ohne abschliessenden Zeilenumbruch.
func (s *lineAnsiStripWriter) Flush() error {
	if len(s.buf) == 0 {
		return nil
	}
	clean := csiSequence.ReplaceAll(s.buf, nil)
	s.buf = nil
	if len(clean) == 0 {
		return nil
	}
	_, err := s.w.Write(clean)
	return err
}
