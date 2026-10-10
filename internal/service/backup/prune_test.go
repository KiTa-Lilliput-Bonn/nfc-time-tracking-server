package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestPrunePlainBackups_KeepsDailyWeeklyMonthly(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local)
	// Ein Jahr lang alle 6 Stunden ein Backup.
	var all []string
	for at := now.AddDate(-1, 0, 0); !at.After(now); at = at.Add(6 * time.Hour) {
		name := fmt.Sprintf("timetracking-backup-%d.db", at.UnixNano())
		all = append(all, name)
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(dir, "notes.txt")
	_ = os.WriteFile(other, []byte("x"), 0o644)

	removed, err := PrunePlainBackups(dir, Keep{Daily: 14, Weekly: 8, Monthly: 6}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) == 0 {
		t.Fatal("nothing removed")
	}
	entries, _ := os.ReadDir(dir)
	var kept []time.Time
	for _, e := range entries {
		if e.Name() == "notes.txt" {
			continue
		}
		var ns int64
		if _, err := fmt.Sscanf(e.Name(), "timetracking-backup-%d.db", &ns); err != nil {
			t.Fatalf("unexpected file %s", e.Name())
		}
		kept = append(kept, time.Unix(0, ns))
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("foreign file must stay")
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].Before(kept[j]) })
	// Älter als ca. 6 Monate darf nichts übrig sein.
	if oldest := kept[0]; oldest.Before(now.AddDate(0, -7, 0)) {
		t.Fatalf("oldest kept backup %v too old", oldest)
	}
	// Die letzten 2 Tage bleiben vollständig (alle 6 h).
	recent := 0
	for _, k := range kept {
		if now.Sub(k) < 48*time.Hour {
			recent++
		}
	}
	if recent != 8 {
		t.Fatalf("recent backups kept %d, want 8", recent)
	}
	// Höchstens 8 (2 Tage) + 14 Tage + 8 Wochen + 6 Monate, viele überschneiden sich.
	if len(kept) > 8+14+8+6 || len(kept) < 14 {
		t.Fatalf("kept %d backups", len(kept))
	}
	if len(kept)+len(removed) != len(all) {
		t.Fatalf("kept %d + removed %d != %d", len(kept), len(removed), len(all))
	}
}

func TestPrunePlainBackups_NoRuleKeepsAll(t *testing.T) {
	keep := SelectKeep([]time.Time{time.Now().AddDate(-2, 0, 0), time.Now().AddDate(-1, 0, 0)}, Keep{}, time.Now())
	// Ohne Regel bleibt nur das neueste über SelectKeep; pruneIfDue ruft bei Keep{} gar nicht erst auf.
	if !keep[1] || keep[0] {
		t.Fatalf("keep %v", keep)
	}
	if !(Keep{}).none() {
		t.Fatal("Keep{} must mean no pruning")
	}
}

func TestResticForgetArgs(t *testing.T) {
	got := ResticForgetArgs(Keep{Daily: 14, Weekly: 0, Monthly: 6})
	want := []string{"forget", "--tag", "nfc-time-tracking", "--group-by", "tags", "--keep-within", "2d",
		"--keep-daily", "14", "--keep-monthly", "6", "--prune"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}
