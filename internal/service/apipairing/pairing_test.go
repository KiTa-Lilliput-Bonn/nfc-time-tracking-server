package apipairing

import (
	"context"
	"testing"

	"nfc-time-tracking-server/internal/model"
)

type stubStore struct {
	clients []model.ApiPairedClient
	err     error
}

func (s *stubStore) ListAuthorizedSecrets(ctx context.Context) ([]model.ApiPairedClient, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.clients, nil
}

func TestIsAuthorizedBearer(t *testing.T) {
	svc := New(&stubStore{
		clients: []model.ApiPairedClient{
			{ID: "a", Secret: "secret-one"},
			{ID: "b", Secret: "other"},
		},
	})
	ctx := context.Background()
	if !svc.IsAuthorizedBearer(ctx, "secret-one") {
		t.Fatal("expected match")
	}
	if svc.IsAuthorizedBearer(ctx, "wrong") {
		t.Fatal("expected no match")
	}
	if svc.IsAuthorizedBearer(ctx, "") {
		t.Fatal("empty token")
	}
}

func TestNewSecret(t *testing.T) {
	a, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) < 16 || a == b {
		t.Fatalf("unexpected secrets: %q %q", a, b)
	}
}

func TestBuildClient(t *testing.T) {
	c, err := BuildClient("abc", "lbl", " sec ")
	if err != nil {
		t.Fatal(err)
	}
	if c.Secret != "sec" || c.Label != "lbl" {
		t.Fatalf("unexpected trim: %+v", c)
	}
	_, err = BuildClient("", "", "x")
	if err == nil {
		t.Fatal("expected error empty id")
	}
}
