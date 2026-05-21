package scheduleimport

import (
	"fmt"
	"html"
	"strings"

	"github.com/xuri/excelize/v2"
)

// mergeAnchorCell mappt jede Zelle auf die linke obere Ecke ihres Merge-Bereichs (falls vorhanden).
func mergeAnchorCell(f *excelize.File, sheet string) (map[string]string, error) {
	mcs, err := f.GetMergeCells(sheet)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, mc := range mcs {
		if len(mc) < 1 {
			continue
		}
		ref := strings.TrimSpace(mc[0])
		if ref == "" {
			continue
		}
		parts := strings.Split(ref, ":")
		if len(parts) == 1 {
			ref = parts[0] + ":" + parts[0]
			parts = strings.Split(ref, ":")
		}
		if len(parts) != 2 {
			continue
		}
		topLeft := strings.TrimSpace(parts[0])
		c1, r1, err := excelize.CellNameToCoordinates(topLeft)
		if err != nil {
			continue
		}
		c2, r2, err := excelize.CellNameToCoordinates(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		x1, x2 := c1, c2
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		y1, y2 := r1, r2
		if y1 > y2 {
			y1, y2 = y2, y1
		}
		for x := x1; x <= x2; x++ {
			for y := y1; y <= y2; y++ {
				cell, err := excelize.CoordinatesToCellName(x, y)
				if err != nil {
					continue
				}
				out[strings.ToUpper(cell)] = topLeft
			}
		}
	}
	return out, nil
}

// collectNotesRichHTML liest Spalte I bis zur ersten „Datum“-Zeile und wandelt
// Excel-Richtext in einfachen HTML für die Wochennotizen (Quill-kompatibel).
func collectNotesRichHTML(f *excelize.File, sheet string, weekHeaderRow0 int, rowCount int) (string, error) {
	anchors, err := mergeAnchorCell(f, sheet)
	if err != nil {
		return "", err
	}

	var blocks []string
	for rr := weekHeaderRow0; rr < rowCount; rr++ {
		bCell, err := excelize.CoordinatesToCellName(2, rr+1)
		if err != nil {
			return "", err
		}
		b, err := f.GetCellValue(sheet, bCell)
		if err != nil {
			return "", err
		}
		if strings.EqualFold(strings.TrimSpace(b), "Datum") {
			break
		}

		iCell, err := excelize.CoordinatesToCellName(9, rr+1)
		if err != nil {
			return "", err
		}
		if master, ok := anchors[strings.ToUpper(iCell)]; ok {
			if strings.ToUpper(master) != strings.ToUpper(iCell) {
				continue
			}
		}

		plain, err := f.GetCellValue(sheet, iCell)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(plain) == "" {
			continue
		}

		runs, err := f.GetCellRichText(sheet, iCell)
		if err != nil {
			return "", err
		}
		block, err := richRunsToHTML(f, runs, plain)
		if err != nil {
			return "", err
		}
		if block != "" {
			blocks = append(blocks, block)
		}
	}
	return strings.Join(blocks, ""), nil
}

func richRunsToHTML(f *excelize.File, runs []excelize.RichTextRun, plainFallback string) (string, error) {
	if len(runs) == 0 {
		return paragraphFromPlain(plainFallback), nil
	}
	var inner strings.Builder
	for _, r := range runs {
		if r.Text == "" {
			continue
		}
		frag := html.EscapeString(r.Text)
		frag = strings.ReplaceAll(frag, "\r\n", "\n")
		frag = strings.ReplaceAll(frag, "\r", "\n")
		parts := strings.Split(frag, "\n")
		for i, p := range parts {
			if i > 0 {
				inner.WriteString("<br>")
			}
			if p == "" {
				continue
			}
			wrapped := wrapRunWithFontHTML(f, p, r.Font)
			inner.WriteString(wrapped)
		}
	}
	s := inner.String()
	if strings.TrimSpace(s) == "" {
		return paragraphFromPlain(plainFallback), nil
	}
	return "<p>" + s + "</p>", nil
}

func paragraphFromPlain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	esc := html.EscapeString(s)
	esc = strings.ReplaceAll(esc, "\r\n", "\n")
	esc = strings.ReplaceAll(esc, "\r", "\n")
	esc = strings.ReplaceAll(esc, "\n", "<br>")
	return "<p>" + esc + "</p>"
}

func wrapRunWithFontHTML(f *excelize.File, escapedSegment string, font *excelize.Font) string {
	if font == nil {
		return escapedSegment
	}
	out := escapedSegment
	if font.VertAlign == "superscript" {
		out = "<sup>" + out + "</sup>"
	} else if font.VertAlign == "subscript" {
		out = "<sub>" + out + "</sub>"
	}
	if font.Strike {
		out = "<s>" + out + "</s>"
	}
	if font.Bold {
		out = "<strong>" + out + "</strong>"
	}
	if font.Italic {
		out = "<em>" + out + "</em>"
	}
	if font.Underline != "" && font.Underline != "none" {
		out = "<u>" + out + "</u>"
	}

	var style []string
	if font.Size > 0 {
		style = append(style, fmt.Sprintf("font-size:%gpt", font.Size))
	}
	if ff := strings.TrimSpace(font.Family); ff != "" {
		style = append(style, "font-family:"+quoteCSSFontFamily(ff))
	}
	if c := fontColorCSS(f, font); c != "" {
		style = append(style, "color:"+c)
	}
	if len(style) > 0 {
		out = fmt.Sprintf(`<span style="%s">%s</span>`, strings.Join(style, ";"), out)
	}
	return out
}

func quoteCSSFontFamily(name string) string {
	if strings.ContainsAny(name, `"'`) || strings.Contains(name, ",") {
		escaped := strings.ReplaceAll(name, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return name
}

func fontColorCSS(f *excelize.File, font *excelize.Font) string {
	if font == nil {
		return ""
	}
	base := f.GetBaseColor(font.Color, font.ColorIndexed, font.ColorTheme)
	if base == "" {
		return ""
	}
	full := excelize.ThemeColor(base, font.ColorTint)
	if len(full) == 8 {
		full = strings.TrimPrefix(full, "FF")
	}
	if len(full) != 6 {
		return ""
	}
	// Standard‑„Automatikfarbe“ / Schwarz liefert oft #000000 — ohne eigenes span,
	// damit die Ausgabe nicht unnötig aufbläht und näher am Original bleibt.
	if strings.EqualFold(full, "000000") {
		return ""
	}
	return "#" + full
}
