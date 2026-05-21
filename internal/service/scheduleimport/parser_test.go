package scheduleimport

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseXLSX_MinimalBlock(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "B1", "16.03. - 20.03.2026")
	_ = f.SetCellValue(sheet, "I1", "Testnotiz")
	_ = f.SetCellValue(sheet, "B2", "Datum")
	_ = f.SetCellValue(sheet, "C2", "16.03.")
	_ = f.SetCellValue(sheet, "D2", "17.03.")
	_ = f.SetCellValue(sheet, "E2", "18.03.")
	_ = f.SetCellValue(sheet, "F2", "19.03.")
	_ = f.SetCellValue(sheet, "G2", "20.03.")
	_ = f.SetCellValue(sheet, "B3", "Anna Tester")
	_ = f.SetCellValue(sheet, "C3", "8.30-16.30 B")
	_ = f.SetCellValue(sheet, "B4", "GT irgendwas")
	_ = f.SetCellValue(sheet, "B5", "Datum")
	_ = f.SetCellValue(sheet, "C5", "16.03.")
	_ = f.SetCellValue(sheet, "D5", "17.03.")
	_ = f.SetCellValue(sheet, "E5", "18.03.")
	_ = f.SetCellValue(sheet, "F5", "19.03.")
	_ = f.SetCellValue(sheet, "G5", "20.03.")
	_ = f.SetCellValue(sheet, "B6", "Anna Tester")
	_ = f.SetCellValue(sheet, "D6", "U")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	w := parsed.Weeks[0]
	if w.Notes != "<p>Testnotiz</p>" {
		t.Fatalf("notes: %q", w.Notes)
	}
	if len(w.Rows) != 2 {
		t.Fatalf("employee rows: %d", len(w.Rows))
	}
	if w.Rows[1].Cells[1] != "U" {
		t.Fatalf("second block vacation cell: %q", w.Rows[1].Cells[1])
	}
}

func TestParseXLSX_ColumnIRichText(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "B1", "16.03. - 20.03.2026")
	err := f.SetCellRichText(sheet, "I1", []excelize.RichTextRun{
		{Text: "Hallo", Font: &excelize.Font{Bold: true}},
		{Text: " Welt", Font: &excelize.Font{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellValue(sheet, "B2", "Datum")
	_ = f.SetCellValue(sheet, "C2", "16.03.")
	_ = f.SetCellValue(sheet, "D2", "17.03.")
	_ = f.SetCellValue(sheet, "E2", "18.03.")
	_ = f.SetCellValue(sheet, "F2", "19.03.")
	_ = f.SetCellValue(sheet, "G2", "20.03.")
	_ = f.SetCellValue(sheet, "B3", "Anna Tester")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	got := parsed.Weeks[0].Notes
	want := "<p><strong>Hallo</strong> Welt</p>"
	if got != want {
		t.Fatalf("notes: %q want %q", got, want)
	}
}

func TestParseXLSX_ColumnIMergedNoDup(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "B1", "16.03. - 20.03.2026")
	if err := f.MergeCell(sheet, "I1", "I3"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellRichText(sheet, "I1", []excelize.RichTextRun{
		{Text: "Nur einmal", Font: &excelize.Font{Bold: true}},
	}); err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellValue(sheet, "B4", "Datum")
	_ = f.SetCellValue(sheet, "C4", "16.03.")
	_ = f.SetCellValue(sheet, "D4", "17.03.")
	_ = f.SetCellValue(sheet, "E4", "18.03.")
	_ = f.SetCellValue(sheet, "F4", "19.03.")
	_ = f.SetCellValue(sheet, "G4", "20.03.")
	_ = f.SetCellValue(sheet, "B5", "Anna Tester")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	got := parsed.Weeks[0].Notes
	want := "<p><strong>Nur einmal</strong></p>"
	if got != want {
		t.Fatalf("notes: %q want %q", got, want)
	}
}

func TestParseXLSX_TeamMondayKT(t *testing.T) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)

	_ = f.SetCellValue(sheet, "B1", "16.03. - 20.03.2026")
	_ = f.SetCellValue(sheet, "B2", "KT 10:00 - 11:00")
	_ = f.SetCellValue(sheet, "B3", "Datum")
	_ = f.SetCellValue(sheet, "C3", "16.03.")
	_ = f.SetCellValue(sheet, "D3", "17.03.")
	_ = f.SetCellValue(sheet, "E3", "18.03.")
	_ = f.SetCellValue(sheet, "F3", "19.03.")
	_ = f.SetCellValue(sheet, "G3", "20.03.")
	_ = f.SetCellValue(sheet, "B4", "Anna Tester")
	_ = f.SetCellValue(sheet, "C4", "8:00-16:00")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseXLSX(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Weeks) != 1 {
		t.Fatalf("weeks: %d", len(parsed.Weeks))
	}
	w := parsed.Weeks[0]
	if len(w.TeamMondaySections) != 1 {
		t.Fatalf("team sections: %d", len(w.TeamMondaySections))
	}
	line := w.TeamMondaySections[0].Line
	if line.Kind != TeamMeetingLineScheduled || line.KTStart != "10:00" || line.KTEnd != "11:00" {
		t.Fatalf("team line: %+v", line)
	}
	if len(w.TeamMondaySections[0].EmployeeRawNames) != 1 || w.TeamMondaySections[0].EmployeeRawNames[0] != "Anna Tester" {
		t.Fatalf("employees: %+v", w.TeamMondaySections[0].EmployeeRawNames)
	}
}
