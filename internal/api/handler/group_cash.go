package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"nfc-time-tracking-server/internal/api/middleware"
	"nfc-time-tracking-server/internal/api/response"
	"nfc-time-tracking-server/internal/audit"
	"nfc-time-tracking-server/internal/model"
	"nfc-time-tracking-server/internal/service/groupcash"
	"nfc-time-tracking-server/internal/store"
)

// GroupCashHandler serves the group cash boxes (Gruppenkassen). Every group has one box.
// Kassenwarte of a group may book entries, receipts and the monthly allowance; Leitung and
// superadmin read all boxes and assign the Kassenwarte.
type GroupCashHandler struct {
	Cash   store.GroupCashStore
	Groups store.GroupStore
	Users  store.UserStore
	Audit  *audit.Logger
	// Now is overridable in tests; nil means time.Now.
	Now func() time.Time
}

const (
	maxReceiptBytes        = 15 << 20
	maxReceiptsPerUpload   = 10
	maxReceiptUploadBytes  = maxReceiptsPerUpload*maxReceiptBytes + 1<<20
	maxCashAmountCents     = 10_000_000
	maxCashDescriptionRune = 500
)

func (h *GroupCashHandler) now() time.Time {
	if h.Now != nil {
		return h.Now().In(time.Local)
	}
	return time.Now().In(time.Local)
}

func isLeitungRole(role string) bool {
	return role == string(model.RoleLeitung) || role == string(model.RoleSuperadmin)
}

type cashAccess struct {
	group   *model.Group
	canEdit bool
}

// access resolves the box of URL param {groupId} and the caller's rights. It writes the error response
// and returns nil when the caller may not read the box.
func (h *GroupCashHandler) access(w http.ResponseWriter, r *http.Request) *cashAccess {
	groupID, err := strconv.Atoi(chi.URLParam(r, "groupId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid group id")
		return nil
	}
	g, err := h.Groups.GetByID(r.Context(), groupID)
	if err != nil || g == nil {
		response.Error(w, http.StatusNotFound, "Gruppe nicht gefunden")
		return nil
	}
	keeper, err := h.Cash.IsKeeper(r.Context(), groupID, middleware.UserID(r))
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return nil
	}
	if !keeper && !isLeitungRole(middleware.Role(r)) {
		response.Error(w, http.StatusForbidden, "Kein Zugriff auf diese Gruppenkasse")
		return nil
	}
	return &cashAccess{group: g, canEdit: keeper}
}

func (h *GroupCashHandler) editAccess(w http.ResponseWriter, r *http.Request) *cashAccess {
	a := h.access(w, r)
	if a == nil {
		return nil
	}
	if !a.canEdit {
		response.Error(w, http.StatusForbidden, "Nur Kassenwarte dieser Gruppe dürfen buchen")
		return nil
	}
	return a
}

type cashPerson struct {
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
}

func (h *GroupCashHandler) userNames(r *http.Request) (map[int]string, error) {
	users, err := h.Users.List(r.Context(), false)
	if err != nil {
		return nil, err
	}
	names := make(map[int]string, len(users))
	for _, u := range users {
		names[u.ID] = u.DisplayName
	}
	return names, nil
}

func (h *GroupCashHandler) keepers(r *http.Request, groupID int, names map[int]string) ([]cashPerson, error) {
	ids, err := h.Cash.ListKeepers(r.Context(), groupID)
	if err != nil {
		return nil, err
	}
	out := make([]cashPerson, 0, len(ids))
	for _, id := range ids {
		out = append(out, cashPerson{ID: id, DisplayName: names[id]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out, nil
}

func (h *GroupCashHandler) summary(r *http.Request, groupID int) (groupcash.Summary, []model.CashAllowance, []model.CashEntry, error) {
	allowances, err := h.Cash.ListAllowances(r.Context(), groupID)
	if err != nil {
		return groupcash.Summary{}, nil, nil, err
	}
	entries, err := h.Cash.ListEntries(r.Context(), groupID)
	if err != nil {
		return groupcash.Summary{}, nil, nil, err
	}
	return groupcash.Compute(allowances, entries, groupcash.MonthOf(h.now())), allowances, entries, nil
}

type cashBoxListItem struct {
	GroupID   int               `json:"group_id"`
	GroupName string            `json:"group_name"`
	Keepers   []cashPerson      `json:"keepers"`
	CanEdit   bool              `json:"can_edit"`
	Summary   groupcash.Summary `json:"summary"`
}

// List returns the boxes the caller may see: all for Leitung/superadmin, otherwise the boxes they keep.
func (h *GroupCashHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := middleware.UserID(r)
	kept, err := h.Cash.ListKeeperGroups(ctx, uid)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	keeperOf := make(map[int]bool, len(kept))
	for _, id := range kept {
		keeperOf[id] = true
	}
	groups, err := h.Groups.List(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	names, err := h.userNames(r)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	lead := isLeitungRole(middleware.Role(r))
	out := []cashBoxListItem{}
	for _, g := range groups {
		if !lead && !keeperOf[g.ID] {
			continue
		}
		ks, err := h.keepers(r, g.ID, names)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "query failed")
			return
		}
		sum, _, _, err := h.summary(r, g.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "query failed")
			return
		}
		sum.Months = nil
		out = append(out, cashBoxListItem{GroupID: g.ID, GroupName: g.Name, Keepers: ks, CanEdit: keeperOf[g.ID], Summary: sum})
	}
	response.JSON(w, http.StatusOK, map[string]any{"cash_boxes": out})
}

type cashEntryResponse struct {
	model.CashEntry
	BalanceAfterCents int64               `json:"balance_after_cents"`
	CreatedByName     string              `json:"created_by_name,omitempty"`
	UpdatedByName     string              `json:"updated_by_name,omitempty"`
	Receipts          []model.CashReceipt `json:"receipts"`
}

type cashBoxDetail struct {
	GroupID          int                   `json:"group_id"`
	GroupName        string                `json:"group_name"`
	Keepers          []cashPerson          `json:"keepers"`
	CanEdit          bool                  `json:"can_edit"`
	CanManageKeepers bool                  `json:"can_manage_keepers"`
	Allowances       []model.CashAllowance `json:"allowances"`
	Summary          groupcash.Summary     `json:"summary"`
	// Entries: newest first, each with the cash balance after the entry.
	Entries []cashEntryResponse `json:"entries"`
}

func (h *GroupCashHandler) Get(w http.ResponseWriter, r *http.Request) {
	a := h.access(w, r)
	if a == nil {
		return
	}
	names, err := h.userNames(r)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	ks, err := h.keepers(r, a.group.ID, names)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	sum, allowances, entries, err := h.summary(r, a.group.ID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	receipts, err := h.Cash.ListReceipts(r.Context(), a.group.ID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	byEntry := map[int][]model.CashReceipt{}
	for _, rc := range receipts {
		byEntry[rc.EntryID] = append(byEntry[rc.EntryID], rc)
	}
	out := make([]cashEntryResponse, len(entries))
	var bal int64
	for i, e := range entries {
		bal += e.SignedCents()
		er := cashEntryResponse{CashEntry: e, BalanceAfterCents: bal, Receipts: byEntry[e.ID]}
		if er.Receipts == nil {
			er.Receipts = []model.CashReceipt{}
		}
		if e.CreatedBy != nil {
			er.CreatedByName = names[*e.CreatedBy]
		}
		if e.UpdatedBy != nil {
			er.UpdatedByName = names[*e.UpdatedBy]
		}
		// Neueste zuerst.
		out[len(entries)-1-i] = er
	}
	if allowances == nil {
		allowances = []model.CashAllowance{}
	}
	response.JSON(w, http.StatusOK, cashBoxDetail{
		GroupID: a.group.ID, GroupName: a.group.Name, Keepers: ks, CanEdit: a.canEdit,
		CanManageKeepers: isLeitungRole(middleware.Role(r)),
		Allowances:       allowances, Summary: sum, Entries: out,
	})
}

type putKeepersBody struct {
	UserIDs []int `json:"user_ids"`
}

// PutKeepers replaces the Kassenwarte of a box (Leitung/superadmin only, enforced by the router).
func (h *GroupCashHandler) PutKeepers(w http.ResponseWriter, r *http.Request) {
	a := h.access(w, r)
	if a == nil {
		return
	}
	var body putKeepersBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	seen := map[int]bool{}
	ids := []int{}
	for _, id := range body.UserIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		u, err := h.Users.GetByID(r.Context(), id)
		if err != nil || u == nil {
			response.Error(w, http.StatusBadRequest, "Unbekannte Person")
			return
		}
		if !u.Active || u.Role == model.RoleSuperadmin {
			response.Error(w, http.StatusBadRequest, fmt.Sprintf("%s kann nicht Kassenwart sein", u.DisplayName))
			return
		}
		ids = append(ids, id)
	}
	if err := h.Cash.SetKeepers(r.Context(), a.group.ID, ids); err != nil {
		response.Error(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityCashKeepers, EntityID: auditID(a.group.ID),
		Summary: audit.JSONSummary(map[string]any{"group_id": a.group.ID, "user_ids": ids}),
	})
	names, _ := h.userNames(r)
	ks, _ := h.keepers(r, a.group.ID, names)
	response.JSON(w, http.StatusOK, map[string]any{"keepers": ks})
}

type allowanceBody struct {
	ValidFrom   string `json:"valid_from"`
	AmountCents int64  `json:"amount_cents"`
}

// PutAllowance sets the monthly allowance from valid_from (YYYY-MM) on.
func (h *GroupCashHandler) PutAllowance(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	var body allowanceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !groupcash.ValidMonth(body.ValidFrom) {
		response.Error(w, http.StatusBadRequest, "Bitte einen Monat angeben (gültig ab)")
		return
	}
	if body.AmountCents < 0 || body.AmountCents > maxCashAmountCents {
		response.Error(w, http.StatusBadRequest, "Ungültiger Betrag")
		return
	}
	uid := middleware.UserID(r)
	al := &model.CashAllowance{GroupID: a.group.ID, ValidFrom: body.ValidFrom, AmountCents: body.AmountCents, CreatedBy: &uid}
	if err := h.Cash.UpsertAllowance(r.Context(), al); err != nil {
		response.Error(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityCashAllowance, EntityID: auditID(al.ID),
		Summary: audit.JSONSummary(map[string]any{"group_id": a.group.ID, "valid_from": al.ValidFrom, "amount_cents": al.AmountCents}),
	})
	response.JSON(w, http.StatusOK, al)
}

func (h *GroupCashHandler) DeleteAllowance(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	id, err := strconv.Atoi(chi.URLParam(r, "allowanceId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return
	}
	ok, err := h.Cash.DeleteAllowance(r.Context(), a.group.ID, id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Löschen fehlgeschlagen")
		return
	}
	if !ok {
		response.Error(w, http.StatusNotFound, "not found")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityCashAllowance, EntityID: auditID(id),
		Summary: audit.JSONSummary(map[string]any{"group_id": a.group.ID}),
	})
	w.WriteHeader(http.StatusNoContent)
}

type cashEntryBody struct {
	Kind        model.CashEntryKind    `json:"kind"`
	Source      model.CashIncomeSource `json:"source"`
	ForMonth    string                 `json:"for_month"`
	EntryDate   string                 `json:"entry_date"`
	AmountCents int64                  `json:"amount_cents"`
	Description string                 `json:"description"`
}

// normalize validates the body and returns the entry fields; the error text is user-facing.
func (b cashEntryBody) normalize(today time.Time) (model.CashEntry, error) {
	e := model.CashEntry{Kind: b.Kind, Source: b.Source, ForMonth: strings.TrimSpace(b.ForMonth),
		EntryDate: strings.TrimSpace(b.EntryDate), AmountCents: b.AmountCents, Description: strings.TrimSpace(b.Description)}
	d, err := time.ParseInLocation("2006-01-02", e.EntryDate, time.Local)
	if err != nil {
		return e, fmt.Errorf("Bitte ein Datum angeben")
	}
	if d.After(today) {
		return e, fmt.Errorf("Das Datum darf nicht in der Zukunft liegen")
	}
	if e.AmountCents <= 0 || e.AmountCents > maxCashAmountCents {
		return e, fmt.Errorf("Bitte einen Betrag größer 0 angeben")
	}
	if utf8.RuneCountInString(e.Description) > maxCashDescriptionRune {
		return e, fmt.Errorf("Die Beschreibung ist zu lang")
	}
	switch e.Kind {
	case model.CashExpense:
		e.Source, e.ForMonth = "", ""
		if e.Description == "" {
			return e, fmt.Errorf("Bitte angeben, wofür das Geld ausgegeben wurde")
		}
	case model.CashIncome:
		switch e.Source {
		case model.CashSourceAllowance:
			if e.ForMonth == "" {
				e.ForMonth = e.EntryDate[:7]
			}
			if !groupcash.ValidMonth(e.ForMonth) {
				return e, fmt.Errorf("Bitte den Monat des Monatsbetrags angeben")
			}
		case model.CashSourceSavings:
			e.ForMonth = ""
		case model.CashSourceOther:
			e.ForMonth = ""
			if e.Description == "" {
				return e, fmt.Errorf("Bitte angeben, woher das Geld kommt")
			}
		default:
			return e, fmt.Errorf("Bitte die Herkunft der Einnahme wählen")
		}
	default:
		return e, fmt.Errorf("Bitte Ausgabe oder Einnahme wählen")
	}
	return e, nil
}

// checkAgainstBox validates the entry against allowance and savings, excluding entry excludeID.
func (h *GroupCashHandler) checkAgainstBox(r *http.Request, groupID int, e model.CashEntry, excludeID int) (int, error) {
	allowances, err := h.Cash.ListAllowances(r.Context(), groupID)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("query failed")
	}
	entries, err := h.Cash.ListEntries(r.Context(), groupID)
	if err != nil {
		return http.StatusInternalServerError, fmt.Errorf("query failed")
	}
	others := entries[:0]
	for _, o := range entries {
		if o.ID != excludeID {
			others = append(others, o)
		}
	}
	if err := groupcash.CheckEntry(allowances, others, e, groupcash.MonthOf(h.now())); err != nil {
		return http.StatusConflict, err
	}
	return 0, nil
}

func cashEntryAudit(e *model.CashEntry) string {
	return audit.JSONSummary(map[string]any{
		"group_id": e.GroupID, "kind": e.Kind, "source": e.Source, "for_month": e.ForMonth,
		"entry_date": e.EntryDate, "amount_cents": e.AmountCents, "description": e.Description,
	})
}

func (h *GroupCashHandler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	var body cashEntryBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	e, err := body.normalize(h.now())
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	e.GroupID = a.group.ID
	if status, err := h.checkAgainstBox(r, a.group.ID, e, 0); err != nil {
		response.Error(w, status, err.Error())
		return
	}
	uid := middleware.UserID(r)
	e.CreatedBy = &uid
	if err := h.Cash.CreateEntry(r.Context(), &e); err != nil {
		response.Error(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionCreate, EntityType: audit.EntityCashEntry, EntityID: auditID(e.ID), Summary: cashEntryAudit(&e),
	})
	response.JSON(w, http.StatusCreated, e)
}

// entryOfBox loads URL param {entryId} and checks it belongs to the box.
func (h *GroupCashHandler) entryOfBox(w http.ResponseWriter, r *http.Request, a *cashAccess) *model.CashEntry {
	id, err := strconv.Atoi(chi.URLParam(r, "entryId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	e, err := h.Cash.GetEntry(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return nil
	}
	if e == nil || e.GroupID != a.group.ID {
		response.Error(w, http.StatusNotFound, "Buchung nicht gefunden")
		return nil
	}
	return e
}

func (h *GroupCashHandler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	old := h.entryOfBox(w, r, a)
	if old == nil {
		return
	}
	var body cashEntryBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	e, err := body.normalize(h.now())
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	e.ID, e.GroupID, e.CreatedBy, e.CreatedAt = old.ID, old.GroupID, old.CreatedBy, old.CreatedAt
	if status, err := h.checkAgainstBox(r, a.group.ID, e, old.ID); err != nil {
		response.Error(w, status, err.Error())
		return
	}
	uid := middleware.UserID(r)
	e.UpdatedBy = &uid
	if err := h.Cash.UpdateEntry(r.Context(), &e); err != nil {
		response.Error(w, http.StatusInternalServerError, "Speichern fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionUpdate, EntityType: audit.EntityCashEntry, EntityID: auditID(e.ID), Summary: cashEntryAudit(&e),
	})
	response.JSON(w, http.StatusOK, e)
}

func (h *GroupCashHandler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	e := h.entryOfBox(w, r, a)
	if e == nil {
		return
	}
	if err := h.Cash.DeleteEntry(r.Context(), e.ID); err != nil {
		response.Error(w, http.StatusInternalServerError, "Löschen fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityCashEntry, EntityID: auditID(e.ID), Summary: cashEntryAudit(e),
	})
	w.WriteHeader(http.StatusNoContent)
}

// detectReceiptType returns the media type of an allowed receipt file (PDF or photo), sniffed from content.
func detectReceiptType(data []byte) (string, bool) {
	ct := http.DetectContentType(data)
	switch ct {
	case "application/pdf", "image/jpeg", "image/png", "image/webp", "image/gif":
		return ct, true
	}
	// HEIC/HEIF (iPhone-Fotos) erkennt DetectContentType nicht.
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		switch string(data[8:12]) {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1":
			return "image/heic", true
		}
	}
	return "", false
}

func receiptFilename(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "Beleg"
	}
	if utf8.RuneCountInString(name) > 120 {
		name = string([]rune(name)[:120])
	}
	return name
}

func readReceipt(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > maxReceiptBytes {
		return nil, fmt.Errorf("%s ist größer als %d MB", fh.Filename, maxReceiptBytes>>20)
	}
	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("%s konnte nicht gelesen werden", fh.Filename)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReceiptBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s konnte nicht gelesen werden", fh.Filename)
	}
	if len(data) > maxReceiptBytes {
		return nil, fmt.Errorf("%s ist größer als %d MB", fh.Filename, maxReceiptBytes>>20)
	}
	return data, nil
}

// UploadReceipts attaches one or more files (multipart field "file", repeatable) to an entry.
func (h *GroupCashHandler) UploadReceipts(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	e := h.entryOfBox(w, r, a)
	if e == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptUploadBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "Upload zu groß oder ungültig")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		response.Error(w, http.StatusBadRequest, "Bitte mindestens eine Datei auswählen")
		return
	}
	if len(files) > maxReceiptsPerUpload {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("Höchstens %d Dateien auf einmal", maxReceiptsPerUpload))
		return
	}
	type upload struct {
		name, ct string
		data     []byte
	}
	uploads := make([]upload, 0, len(files))
	for _, fh := range files {
		data, err := readReceipt(fh)
		if err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		ct, ok := detectReceiptType(data)
		if !ok {
			response.Error(w, http.StatusBadRequest, fmt.Sprintf("%s ist weder PDF noch Foto", fh.Filename))
			return
		}
		uploads = append(uploads, upload{name: receiptFilename(fh.Filename), ct: ct, data: data})
	}
	uid := middleware.UserID(r)
	out := make([]model.CashReceipt, 0, len(uploads))
	for _, u := range uploads {
		rc := &model.CashReceipt{EntryID: e.ID, Filename: u.name, ContentType: u.ct, UploadedBy: &uid}
		if err := h.Cash.CreateReceipt(r.Context(), rc, u.data); err != nil {
			response.Error(w, http.StatusInternalServerError, "Beleg konnte nicht gespeichert werden")
			return
		}
		logAudit(h.Audit, r.Context(), audit.Entry{
			Action: audit.ActionCreate, EntityType: audit.EntityCashReceipt, EntityID: auditID(rc.ID),
			Summary: audit.JSONSummary(map[string]any{"entry_id": e.ID, "filename": rc.Filename, "size_bytes": rc.SizeBytes}),
		})
		out = append(out, *rc)
	}
	response.JSON(w, http.StatusCreated, map[string]any{"receipts": out})
}

// receiptOfBox loads URL param {receiptId} and checks it belongs to an entry of the box.
func (h *GroupCashHandler) receiptOfBox(w http.ResponseWriter, r *http.Request, a *cashAccess) *model.CashReceipt {
	id, err := strconv.Atoi(chi.URLParam(r, "receiptId"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid id")
		return nil
	}
	rc, err := h.Cash.GetReceipt(r.Context(), id)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return nil
	}
	if rc != nil {
		if e, err := h.Cash.GetEntry(r.Context(), rc.EntryID); err == nil && e != nil && e.GroupID == a.group.ID {
			return rc
		}
	}
	response.Error(w, http.StatusNotFound, "Beleg nicht gefunden")
	return nil
}

func (h *GroupCashHandler) GetReceipt(w http.ResponseWriter, r *http.Request) {
	a := h.access(w, r)
	if a == nil {
		return
	}
	rc := h.receiptOfBox(w, r, a)
	if rc == nil {
		return
	}
	data, err := h.Cash.ReceiptData(r.Context(), rc.ID)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "query failed")
		return
	}
	w.Header().Set("Content-Type", rc.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(rc.Filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *GroupCashHandler) DeleteReceipt(w http.ResponseWriter, r *http.Request) {
	a := h.editAccess(w, r)
	if a == nil {
		return
	}
	rc := h.receiptOfBox(w, r, a)
	if rc == nil {
		return
	}
	if err := h.Cash.DeleteReceipt(r.Context(), rc.ID); err != nil {
		response.Error(w, http.StatusInternalServerError, "Löschen fehlgeschlagen")
		return
	}
	logAudit(h.Audit, r.Context(), audit.Entry{
		Action: audit.ActionDelete, EntityType: audit.EntityCashReceipt, EntityID: auditID(rc.ID),
		Summary: audit.JSONSummary(map[string]any{"entry_id": rc.EntryID, "filename": rc.Filename}),
	})
	w.WriteHeader(http.StatusNoContent)
}
