package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	apimw "nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/store/sqlite"
)

type cashFixture struct {
	router  http.Handler
	db      *sqlite.DB
	group   *model.Group
	other   *model.Group
	keeper  *model.User
	staff   *model.User
	lead    *model.User
	usersBy map[int]*model.User
}

func newCashFixture(t *testing.T) *cashFixture {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	users := sqlite.NewUserStore(db)
	groups := sqlite.NewGroupStore(db)
	f := &cashFixture{db: db, usersBy: map[int]*model.User{}}
	f.keeper = &model.User{Username: "kw", PasswordHash: "x", DisplayName: "Kassenwartin", Role: model.RoleUser, Active: true}
	f.staff = &model.User{Username: "st", PasswordHash: "x", DisplayName: "Andere", Role: model.RoleUser, Active: true}
	f.lead = &model.User{Username: "ld", PasswordHash: "x", DisplayName: "Leitung", Role: model.RoleLeitung, Active: true}
	for _, u := range []*model.User{f.keeper, f.staff, f.lead} {
		if err := users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		f.usersBy[u.ID] = u
	}
	f.group = &model.Group{Name: "Mäuse"}
	f.other = &model.Group{Name: "Bären"}
	for _, g := range []*model.Group{f.group, f.other} {
		if err := groups.Create(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	h := &GroupCashHandler{
		Cash: sqlite.NewGroupCashStore(db), Groups: groups, Users: users,
		Now: func() time.Time { return time.Date(2026, 4, 15, 10, 0, 0, 0, time.Local) },
	}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			u := f.usersBy[atoiOr0(req.Header.Get("X-User"))]
			ctx := context.WithValue(req.Context(), apimw.CtxUserID, u.ID)
			ctx = context.WithValue(ctx, apimw.CtxRole, string(u.Role))
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/cash-boxes", h.List)
	r.Get("/cash-boxes/{groupId}", h.Get)
	r.With(apimw.RequireRole(string(model.RoleLeitung), string(model.RoleSuperadmin))).Put("/cash-boxes/{groupId}/keepers", h.PutKeepers)
	r.Put("/cash-boxes/{groupId}/allowances", h.PutAllowance)
	r.Put("/cash-boxes/{groupId}/opening", h.PutOpening)
	r.Get("/cash-boxes/{groupId}/export", h.Export)
	r.Post("/cash-boxes/{groupId}/entries", h.CreateEntry)
	r.Put("/cash-boxes/{groupId}/entries/{entryId}", h.UpdateEntry)
	r.Delete("/cash-boxes/{groupId}/entries/{entryId}", h.DeleteEntry)
	r.Post("/cash-boxes/{groupId}/entries/{entryId}/receipts", h.UploadReceipts)
	r.Get("/cash-boxes/{groupId}/receipts/{receiptId}", h.GetReceipt)
	r.Delete("/cash-boxes/{groupId}/receipts/{receiptId}", h.DeleteReceipt)
	f.router = r
	return f
}

func atoiOr0(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func (f *cashFixture) do(t *testing.T, actor *model.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("X-User", itoa(actor.ID))
	rr := httptest.NewRecorder()
	f.router.ServeHTTP(rr, req)
	return rr
}

func itoa(n int) string { return strconv.Itoa(n) }

func (f *cashFixture) box(id int) string { return "/cash-boxes/" + itoa(id) }

func TestGroupCash_AccessAndSavingsFlow(t *testing.T) {
	f := newCashFixture(t)
	box := f.box(f.group.ID)

	// Ohne Zuweisung: kein Zugriff für Mitarbeitende, Leitung liest.
	if rr := f.do(t, f.keeper, http.MethodGet, box, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("keeper before assignment: %d", rr.Code)
	}
	if rr := f.do(t, f.keeper, http.MethodPut, box+"/keepers", map[string]any{"user_ids": []int{f.keeper.ID}}); rr.Code != http.StatusForbidden {
		t.Fatalf("staff must not assign keepers: %d", rr.Code)
	}
	if rr := f.do(t, f.lead, http.MethodPut, box+"/keepers", map[string]any{"user_ids": []int{f.keeper.ID}}); rr.Code != http.StatusOK {
		t.Fatalf("assign keepers: %d %s", rr.Code, rr.Body.String())
	}

	// Leitung darf lesen, aber nicht buchen.
	if rr := f.do(t, f.lead, http.MethodPost, box+"/entries", map[string]any{
		"kind": "expense", "entry_date": "2026-04-01", "amount_cents": 100, "description": "x",
	}); rr.Code != http.StatusForbidden {
		t.Fatalf("Leitung must not book: %d", rr.Code)
	}
	// Andere Mitarbeitende sehen die Kasse nicht.
	if rr := f.do(t, f.staff, http.MethodGet, box, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("other staff: %d", rr.Code)
	}
	// Kassenwart einer Gruppe ist nicht Kassenwart der anderen.
	if rr := f.do(t, f.keeper, http.MethodGet, f.box(f.other.ID), nil); rr.Code != http.StatusForbidden {
		t.Fatalf("other box: %d", rr.Code)
	}

	if rr := f.do(t, f.keeper, http.MethodPut, box+"/allowances", map[string]any{"valid_from": "2026-01", "amount_cents": 10000}); rr.Code != http.StatusOK {
		t.Fatalf("allowance: %d %s", rr.Code, rr.Body.String())
	}
	post := func(body map[string]any) *httptest.ResponseRecorder {
		return f.do(t, f.keeper, http.MethodPost, box+"/entries", body)
	}
	if rr := post(map[string]any{"kind": "income", "source": "allowance", "for_month": "2026-01", "entry_date": "2026-01-05", "amount_cents": 10000}); rr.Code != http.StatusCreated {
		t.Fatalf("january payout: %d %s", rr.Code, rr.Body.String())
	}
	// Mehr als der Anspruch für April geht nicht als Monatsbetrag.
	if rr := post(map[string]any{"kind": "income", "source": "allowance", "for_month": "2026-04", "entry_date": "2026-04-02", "amount_cents": 30000}); rr.Code != http.StatusConflict {
		t.Fatalf("april over allowance: %d %s", rr.Code, rr.Body.String())
	}
	if rr := post(map[string]any{"kind": "income", "source": "allowance", "entry_date": "2026-04-02", "amount_cents": 10000}); rr.Code != http.StatusCreated {
		t.Fatalf("april payout (month from date): %d %s", rr.Code, rr.Body.String())
	}
	if rr := post(map[string]any{"kind": "income", "source": "savings", "entry_date": "2026-04-02", "amount_cents": 20001}); rr.Code != http.StatusConflict {
		t.Fatalf("savings over balance: %d", rr.Code)
	}
	if rr := post(map[string]any{"kind": "income", "source": "savings", "entry_date": "2026-04-02", "amount_cents": 20000}); rr.Code != http.StatusCreated {
		t.Fatalf("savings withdrawal: %d %s", rr.Code, rr.Body.String())
	}
	if rr := post(map[string]any{"kind": "expense", "entry_date": "2026-04-03", "amount_cents": 100}); rr.Code != http.StatusBadRequest {
		t.Fatalf("expense needs description: %d", rr.Code)
	}
	if rr := post(map[string]any{"kind": "expense", "entry_date": "2026-04-16", "amount_cents": 100, "description": "x"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("future date: %d", rr.Code)
	}
	rr := post(map[string]any{"kind": "expense", "entry_date": "2026-04-03", "amount_cents": 25050, "description": "Ausflug Zoo"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("expense: %d %s", rr.Code, rr.Body.String())
	}
	var expense model.CashEntry
	_ = json.NewDecoder(rr.Body).Decode(&expense)

	var detail cashBoxDetail
	rr = f.do(t, f.lead, http.MethodGet, box, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("lead read: %d", rr.Code)
	}
	_ = json.NewDecoder(rr.Body).Decode(&detail)
	if detail.CanEdit || !detail.CanManageKeepers {
		t.Fatalf("lead rights: %+v", detail)
	}
	if detail.Summary.SavingsCents != 0 || detail.Summary.BalanceCents != 10000+10000+20000-25050 {
		t.Fatalf("summary: %+v", detail.Summary)
	}
	if len(detail.Entries) != 4 || detail.Entries[0].ID != expense.ID || detail.Entries[0].BalanceAfterCents != detail.Summary.BalanceCents {
		t.Fatalf("entries: %+v", detail.Entries)
	}

	// Liste: Kassenwart sieht nur seine Kasse, Leitung alle.
	var list struct {
		CashBoxes []cashBoxListItem `json:"cash_boxes"`
	}
	_ = json.NewDecoder(f.do(t, f.keeper, http.MethodGet, "/cash-boxes", nil).Body).Decode(&list)
	if len(list.CashBoxes) != 1 || !list.CashBoxes[0].CanEdit {
		t.Fatalf("keeper list: %+v", list)
	}
	_ = json.NewDecoder(f.do(t, f.lead, http.MethodGet, "/cash-boxes", nil).Body).Decode(&list)
	if len(list.CashBoxes) != 2 || list.CashBoxes[0].CanEdit {
		t.Fatalf("lead list: %+v", list)
	}
	_ = json.NewDecoder(f.do(t, f.staff, http.MethodGet, "/cash-boxes", nil).Body).Decode(&list)
	if len(list.CashBoxes) != 0 {
		t.Fatalf("staff list: %+v", list)
	}

	// Gruppe mit Buchungen kann nicht gelöscht werden.
	if err := sqlite.NewGroupStore(f.db).Delete(context.Background(), f.group.ID); err == nil {
		t.Fatal("group with cash entries must not be deletable")
	}
	if err := sqlite.NewGroupStore(f.db).Delete(context.Background(), f.other.ID); err != nil {
		t.Fatalf("group without entries: %v", err)
	}
}

func multipartFiles(t *testing.T, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(data)
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func TestGroupCash_Receipts(t *testing.T) {
	f := newCashFixture(t)
	box := f.box(f.group.ID)
	f.do(t, f.lead, http.MethodPut, box+"/keepers", map[string]any{"user_ids": []int{f.keeper.ID}})
	rr := f.do(t, f.keeper, http.MethodPost, box+"/entries", map[string]any{
		"kind": "expense", "entry_date": "2026-04-03", "amount_cents": 1999, "description": "Bastelmaterial",
	})
	var e model.CashEntry
	_ = json.NewDecoder(rr.Body).Decode(&e)

	upload := func(actor *model.User, files map[string][]byte) *httptest.ResponseRecorder {
		body, ct := multipartFiles(t, files)
		req := httptest.NewRequest(http.MethodPost, box+"/entries/"+itoa(e.ID)+"/receipts", body)
		req.Header.Set("Content-Type", ct)
		req.Header.Set("X-User", itoa(actor.ID))
		rr := httptest.NewRecorder()
		f.router.ServeHTTP(rr, req)
		return rr
	}
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF\n")
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F', 0}, bytes.Repeat([]byte{0}, 64)...)

	if rr := upload(f.lead, map[string][]byte{"a.pdf": pdf}); rr.Code != http.StatusForbidden {
		t.Fatalf("Leitung upload: %d", rr.Code)
	}
	if rr := upload(f.keeper, map[string][]byte{"evil.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)}); rr.Code != http.StatusBadRequest {
		t.Fatalf("svg must be refused: %d", rr.Code)
	}
	rr = upload(f.keeper, map[string][]byte{"Kassenbon.pdf": pdf, "foto.jpg": jpeg})
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", rr.Code, rr.Body.String())
	}
	var up struct {
		Receipts []model.CashReceipt `json:"receipts"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&up)
	if len(up.Receipts) != 2 {
		t.Fatalf("receipts: %+v", up)
	}

	var detail cashBoxDetail
	_ = json.NewDecoder(f.do(t, f.lead, http.MethodGet, box, nil).Body).Decode(&detail)
	if len(detail.Entries[0].Receipts) != 2 {
		t.Fatalf("detail receipts: %+v", detail.Entries[0])
	}
	var pdfID int
	for _, r := range up.Receipts {
		if r.ContentType == "application/pdf" {
			pdfID = r.ID
		}
	}
	rr = f.do(t, f.lead, http.MethodGet, box+"/receipts/"+itoa(pdfID), nil)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/pdf" || !bytes.Equal(rr.Body.Bytes(), pdf) {
		t.Fatalf("download: %d %q", rr.Code, rr.Header().Get("Content-Type"))
	}
	// Über die falsche Kasse ist der Beleg nicht erreichbar.
	if rr := f.do(t, f.lead, http.MethodGet, f.box(f.other.ID)+"/receipts/"+itoa(pdfID), nil); rr.Code != http.StatusNotFound {
		t.Fatalf("receipt via other box: %d", rr.Code)
	}
	if rr := f.do(t, f.staff, http.MethodGet, box+"/receipts/"+itoa(pdfID), nil); rr.Code != http.StatusForbidden {
		t.Fatalf("staff download: %d", rr.Code)
	}
	if rr := f.do(t, f.keeper, http.MethodDelete, box+"/receipts/"+itoa(pdfID), nil); rr.Code != http.StatusNoContent {
		t.Fatalf("delete receipt: %d", rr.Code)
	}
	// Buchung löschen entfernt die übrigen Belege mit.
	if rr := f.do(t, f.keeper, http.MethodDelete, box+"/entries/"+itoa(e.ID), nil); rr.Code != http.StatusNoContent {
		t.Fatalf("delete entry: %d", rr.Code)
	}
	var n int
	_ = f.db.DB.QueryRow(`SELECT COUNT(*) FROM cash_receipts`).Scan(&n)
	if n != 0 {
		t.Fatalf("orphan receipts: %d", n)
	}
}

func TestGroupCash_OpeningAndExport(t *testing.T) {
	f := newCashFixture(t)
	box := f.box(f.group.ID)
	f.do(t, f.lead, http.MethodPut, box+"/keepers", map[string]any{"user_ids": []int{f.keeper.ID}})

	if rr := f.do(t, f.lead, http.MethodPut, box+"/opening", map[string]any{"date": "2026-03-01", "cash_cents": 1000}); rr.Code != http.StatusForbidden {
		t.Fatalf("Leitung must not set opening: %d", rr.Code)
	}
	if rr := f.do(t, f.keeper, http.MethodPut, box+"/opening", map[string]any{"date": "2026-03-01", "cash_cents": 4250, "savings_cents": 30000}); rr.Code != http.StatusOK {
		t.Fatalf("opening: %d %s", rr.Code, rr.Body.String())
	}
	if rr := f.do(t, f.keeper, http.MethodPost, box+"/entries", map[string]any{
		"kind": "expense", "entry_date": "2026-02-28", "amount_cents": 100, "description": "zu früh",
	}); rr.Code != http.StatusConflict {
		t.Fatalf("entry before opening: %d", rr.Code)
	}
	rr := f.do(t, f.keeper, http.MethodPost, box+"/entries", map[string]any{
		"kind": "income", "source": "savings", "entry_date": "2026-04-02", "amount_cents": 30000,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("withdraw opening savings: %d %s", rr.Code, rr.Body.String())
	}
	var e model.CashEntry
	_ = json.NewDecoder(rr.Body).Decode(&e)
	// Stichtag nach der ersten Buchung geht nicht.
	if rr := f.do(t, f.keeper, http.MethodPut, box+"/opening", map[string]any{"date": "2026-04-10", "cash_cents": 0}); rr.Code != http.StatusConflict {
		t.Fatalf("opening after entries: %d", rr.Code)
	}

	var detail cashBoxDetail
	_ = json.NewDecoder(f.do(t, f.lead, http.MethodGet, box, nil).Body).Decode(&detail)
	if detail.Summary.BalanceCents != 34250 || detail.Summary.SavingsCents != 0 || detail.Summary.OpeningDate != "2026-03-01" {
		t.Fatalf("summary: %+v", detail.Summary)
	}
	if detail.Entries[0].BalanceAfterCents != 34250 {
		t.Fatalf("running balance must start at opening: %+v", detail.Entries[0])
	}

	body, ct := multipartFiles(t, map[string][]byte{"bon.pdf": []byte("%PDF-1.4\n%%EOF\n")})
	req := httptest.NewRequest(http.MethodPost, box+"/entries/"+itoa(e.ID)+"/receipts", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-User", itoa(f.keeper.ID))
	f.router.ServeHTTP(httptest.NewRecorder(), req)

	rr = f.do(t, f.lead, http.MethodGet, box+"/export?year=2026&format=csv", nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Anfangsbestand;;;;42,50") {
		t.Fatalf("csv: %d %s", rr.Code, rr.Body.String())
	}
	rr = f.do(t, f.lead, http.MethodGet, box+"/export?year=2026&format=zip", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("zip: %d", rr.Code)
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	if strings.Join(names, "|") != "Kassenbuch_Mäuse_2026.pdf|Kassenbuch_Mäuse_2026.csv|Belege/001_2026-04-02_bon.pdf" {
		t.Fatalf("zip entries: %v", names)
	}
	if rr := f.do(t, f.staff, http.MethodGet, box+"/export?year=2026&format=pdf", nil); rr.Code != http.StatusForbidden {
		t.Fatalf("staff export: %d", rr.Code)
	}
}
