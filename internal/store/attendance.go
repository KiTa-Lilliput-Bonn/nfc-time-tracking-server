package store

import (
	"context"
	"errors"

	"nfc-time-tracking-server/internal/model"
)

// ErrNotFound: Datensatz existiert nicht (Anwesenheitsliste).
var ErrNotFound = errors.New("not found")

// ErrGroupHasChildren: Gruppe kann nicht gelöscht werden, weil ihr noch Kinder zugeordnet sind.
var ErrGroupHasChildren = errors.New("group has children")

// AttendanceStore speichert Kinder, Anwesenheit, Meldungen und Gruppenaccounts.
type AttendanceStore interface {
	ListChildren(ctx context.Context, includeInactive bool) ([]model.Child, error)
	GetChild(ctx context.Context, id int) (*model.Child, error)
	CreateChild(ctx context.Context, c *model.Child) error
	UpdateChild(ctx context.Context, c *model.Child) error
	DeleteChild(ctx context.Context, id int) error

	ListAttendance(ctx context.Context, date string) (map[int]model.ChildAttendance, error)
	PutAttendance(ctx context.Context, a model.ChildAttendance) error

	ListNoticesForDate(ctx context.Context, date string) ([]model.ChildNotice, error)
	ListNoticesEndingFrom(ctx context.Context, from string) ([]model.ChildNotice, error)
	ListNoticesForChild(ctx context.Context, childID int, from string) ([]model.ChildNotice, error)
	GetNotice(ctx context.Context, id int) (*model.ChildNotice, error)
	CreateNotice(ctx context.Context, n *model.ChildNotice) error
	UpdateNotice(ctx context.Context, n *model.ChildNotice) error
	DeleteNotice(ctx context.Context, id int) error

	ListPresentStaff(ctx context.Context, date string) ([]int, error)

	ListGroupAccounts(ctx context.Context) ([]model.GroupAccount, error)
	GetGroupAccount(ctx context.Context, id int) (*model.GroupAccount, error)
	GetGroupAccountByGroup(ctx context.Context, groupID int) (*model.GroupAccount, error)
	GetGroupAccountByUsername(ctx context.Context, username string) (*model.GroupAccount, error)
	CreateGroupAccount(ctx context.Context, a *model.GroupAccount) error
	UpdateGroupAccount(ctx context.Context, a *model.GroupAccount) error
	SetGroupAccountPassword(ctx context.Context, id int, hash string) error
	DeleteGroupAccount(ctx context.Context, id int) error
}
