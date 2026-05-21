package bootstrap

import "testing"

func TestParseAndroidLanTargetsJSON_sortAndValidate(t *testing.T) {
	raw := `[{"id":"b","host":"10.0.0.1","port":9000,"api_client_id":"c1"},{"id":"a","host":"10.0.0.2","port":8787,"api_client_id":"c2"}]`
	got, err := ParseAndroidLanTargetsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len %d", len(got))
	}
	// sorted by host
	if got[0].Host != "10.0.0.1" || got[1].Host != "10.0.0.2" {
		t.Fatalf("sort: %+v", got)
	}
}

func TestParseAndroidLanTargetsJSON_invalidPort(t *testing.T) {
	raw := `[{"id":"x","host":"127.0.0.1","port":0,"api_client_id":"c"}]`
	_, err := ParseAndroidLanTargetsJSON(raw)
	if err == nil {
		t.Fatal("expected error")
	}
}
