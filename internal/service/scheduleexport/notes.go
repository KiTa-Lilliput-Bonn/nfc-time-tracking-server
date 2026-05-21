package scheduleexport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

var (
	reBr          = regexp.MustCompile(`(?i)<br\s*/?>`)
	reBlockEnd   = regexp.MustCompile(`(?i)</\s*(p|div|li|h[1-6])\s*>`)
	reBlockStart = regexp.MustCompile(`(?i)<\s*(p|div|li|h[1-6])(\s[^>]*)?>`)
	reStrong      = regexp.MustCompile(`(?i)</?strong>`)
	reEm          = regexp.MustCompile(`(?i)</?em>`)
	reB           = regexp.MustCompile(`(?i)</?b>`)
	reI           = regexp.MustCompile(`(?i)</?i>`)
	reU           = regexp.MustCompile(`(?i)</?u>`)
	reStyleAttr   = regexp.MustCompile(`(?i)style\s*=\s*["']([^"']*)["']`)
	reColorInStyle = regexp.MustCompile(`(?i)color\s*:\s*([^;]+)`)
	reRGBColor    = regexp.MustCompile(`(?i)rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)`)
	reStrip       = regexp.MustCompile(`<[^>]+>`)
)

// htmlToRichRuns wandelt Quill-HTML in Excel-Rich-Text-Runs um (fett, kursiv, unterstrichen, Farbe).
func htmlToRichRuns(html string) []excelize.RichTextRun {
	html = strings.TrimSpace(html)
	if html == "" {
		return nil
	}
	s := prepareNotesHTML(html)
	if strings.TrimSpace(s) == "" {
		return nil
	}

	segments := parseInlineHTML(s)
	if len(segments) == 0 {
		plain := decodeHTMLEntities(reStrip.ReplaceAllString(s, ""))
		if strings.TrimSpace(plain) == "" {
			return nil
		}
		return []excelize.RichTextRun{{Text: plain, Font: defaultNotesFont()}}
	}

	runs := make([]excelize.RichTextRun, 0, len(segments))
	for _, seg := range segments {
		font := defaultNotesFont()
		applyInlineStyle(font, seg.style)
		runs = append(runs, excelize.RichTextRun{Text: seg.text, Font: font})
	}
	return runs
}

func defaultNotesFont() *excelize.Font {
	return &excelize.Font{Size: 8, Family: fontFamilyArial}
}

type inlineStyle struct {
	bold, italic, underline bool
	color                   string // Excel RRGGBB
}

type textSegment struct {
	text  string
	style inlineStyle
}

func applyInlineStyle(f *excelize.Font, st inlineStyle) {
	f.Bold = st.bold
	f.Italic = st.italic
	if st.underline {
		f.Underline = "single"
	}
	if st.color != "" {
		f.Color = st.color
	}
}

func parseInlineHTML(s string) []textSegment {
	stack := []inlineStyle{{}}
	var out []textSegment
	pos := 0
	for pos < len(s) {
		idx := strings.Index(s[pos:], "<")
		if idx < 0 {
			appendInlineText(&out, stack, s[pos:])
			break
		}
		idx += pos
		if idx > pos {
			appendInlineText(&out, stack, s[pos:idx])
		}
		end := strings.Index(s[idx:], ">")
		if end < 0 {
			appendInlineText(&out, stack, s[idx:])
			break
		}
		tag := s[idx : idx+end+1]
		stack = applyInlineTag(stack, tag)
		pos = idx + end + 1
	}
	return out
}

func appendInlineText(out *[]textSegment, stack []inlineStyle, raw string) {
	text := decodeHTMLEntities(raw)
	if text == "" {
		return
	}
	*out = append(*out, textSegment{text: text, style: stack[len(stack)-1]})
}

func pushInlineStyle(stack []inlineStyle, mod func(inlineStyle) inlineStyle) []inlineStyle {
	cur := stack[len(stack)-1]
	next := mod(cur)
	return append(stack, next)
}

func popInlineStyle(stack []inlineStyle) []inlineStyle {
	if len(stack) > 1 {
		return stack[:len(stack)-1]
	}
	return stack
}

func applyInlineTag(stack []inlineStyle, tag string) []inlineStyle {
	lower := strings.ToLower(tag)
	closing := strings.HasPrefix(lower, "</")

	switch {
	case reStrong.MatchString(lower) || reB.MatchString(lower):
		if closing {
			return popInlineStyle(stack)
		}
		return pushInlineStyle(stack, func(p inlineStyle) inlineStyle {
			p.bold = true
			return p
		})
	case reEm.MatchString(lower) || reI.MatchString(lower):
		if closing {
			return popInlineStyle(stack)
		}
		return pushInlineStyle(stack, func(p inlineStyle) inlineStyle {
			p.italic = true
			return p
		})
	case reU.MatchString(lower):
		if closing {
			return popInlineStyle(stack)
		}
		return pushInlineStyle(stack, func(p inlineStyle) inlineStyle {
			p.underline = true
			return p
		})
	case strings.HasPrefix(lower, "</span"):
		// Nur Farb-Span schließen (nicht jedes </span> einen fremden Stack-Eintrag entfernen).
		if len(stack) > 1 && stack[len(stack)-1].color != "" {
			return popInlineStyle(stack)
		}
	case strings.HasPrefix(lower, "<span"):
		if c := extractStyleColor(tag); c != "" {
			return pushInlineStyle(stack, func(p inlineStyle) inlineStyle {
				p.color = c
				return p
			})
		}
	}
	return stack
}

func extractStyleColor(tag string) string {
	m := reStyleAttr.FindStringSubmatch(tag)
	if len(m) < 2 {
		return ""
	}
	cm := reColorInStyle.FindStringSubmatch(m[1])
	if len(cm) < 2 {
		return ""
	}
	return parseCSSColorToExcel(strings.TrimSpace(cm[1]))
}

func parseCSSColorToExcel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := reRGBColor.FindStringSubmatch(s); len(m) == 4 {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		return rgbToExcel(r, g, b)
	}
	if strings.HasPrefix(s, "#") {
		h := strings.TrimPrefix(s, "#")
		if len(h) == 3 {
			var expanded strings.Builder
			for i := 0; i < 3; i++ {
				expanded.WriteByte(h[i])
				expanded.WriteByte(h[i])
			}
			h = expanded.String()
		}
		if len(h) == 6 {
			return strings.ToUpper(h)
		}
		if len(h) == 8 {
			return strings.ToUpper(h[2:])
		}
	}
	return ""
}

func rgbToExcel(r, g, b int) string {
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	return fmt.Sprintf("%02X%02X%02X", clamp(r), clamp(g), clamp(b))
}

// prepareNotesHTML wandelt Quill-Blöcke in Zeilenumbrüche (\n) für Excel-Richtext um.
func prepareNotesHTML(html string) string {
	s := html
	s = reBr.ReplaceAllString(s, "\n")
	s = reBlockEnd.ReplaceAllString(s, "\n")
	s = reBlockStart.ReplaceAllString(s, "")
	return strings.TrimRight(s, "\n")
}

func decodeHTMLEntities(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// applyNotesMerge fügt Spalte I von startRow bis endRow zusammen (nur I, B–G bleiben einzelne Zeilen).
func applyNotesMerge(f *excelize.File, sheet string, startRow, endRow int, notesHTML string, notesStyleID int) error {
	if endRow < startRow {
		endRow = startRow
	}
	topLeft, err := excelize.CoordinatesToCellName(9, startRow)
	if err != nil {
		return err
	}
	bottomRight, err := excelize.CoordinatesToCellName(9, endRow)
	if err != nil {
		return err
	}
	if endRow > startRow {
		if err := f.MergeCell(sheet, topLeft, bottomRight); err != nil {
			return err
		}
	}
	if err := f.SetCellStyle(sheet, topLeft, bottomRight, notesStyleID); err != nil {
		return err
	}
	runs := htmlToRichRuns(notesHTML)
	if len(runs) > 0 {
		if err := f.SetCellRichText(sheet, topLeft, runs); err != nil {
			return err
		}
	} else {
		if err := f.SetCellValue(sheet, topLeft, ""); err != nil {
			return err
		}
	}
	return nil
}
