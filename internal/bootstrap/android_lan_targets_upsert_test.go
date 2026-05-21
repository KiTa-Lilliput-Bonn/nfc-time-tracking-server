package bootstrap

import "testing"

func TestUpsertAndroidLanTarget_replaceAndAppend(t *testing.T) {
	existing := []AndroidLanTarget{
		{ID: "a", Host: "10.0.0.1", Port: 8787, APIClientID: "a", Label: "old"},
	}
	updated, err := UpsertAndroidLanTarget(existing, AndroidLanTarget{
		ID: "a", Host: "10.0.0.2", Port: 9000, APIClientID: "a", Label: "new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || updated[0].Host != "10.0.0.2" || updated[0].Port != 9000 {
		t.Fatalf("unexpected: %+v", updated)
	}

	appended, err := UpsertAndroidLanTarget(updated, AndroidLanTarget{
		ID: "b", Host: "10.0.0.3", Port: 8787, APIClientID: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(appended) != 2 {
		t.Fatalf("want 2 targets, got %d", len(appended))
	}
}
