package serverurl

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"nfc-time-tracking-server/internal/config"
)

func TestPairingBaseURL_fromRequestLANHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/android-api/clients/generate", nil)
	req.Host = "192.168.1.10:8080"
	got := PairingBaseURL(req, config.ServerConfig{Host: "0.0.0.0", Port: 8080})
	if got != "http://192.168.1.10:8080" {
		t.Fatalf("got %q", got)
	}
}

func TestPairingBaseURL_localhostUsesDiscovery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8080"
	got := PairingBaseURL(req, config.ServerConfig{Host: "0.0.0.0", Port: 8080})
	if got == "" {
		t.Skip("no private IPv4 on this host")
	}
	if IsLoopbackURL(got) {
		t.Fatalf("must not return loopback URL, got %q", got)
	}
	if got[:7] != "http://" {
		t.Fatalf("want http base, got %q", got)
	}
}

func TestPairingBaseURL_configOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8080"
	got := PairingBaseURL(req, config.ServerConfig{
		Host:                 "0.0.0.0",
		Port:                 8080,
		PairingAdvertiseHost: "192.168.50.2",
	})
	if got != "http://192.168.50.2:8080" {
		t.Fatalf("got %q", got)
	}
}

func TestPairingBaseURL_boundPrivateListenHost(t *testing.T) {
	got := PairingBaseURL(nil, config.ServerConfig{Host: "10.0.0.5", Port: 9000})
	if got != "http://10.0.0.5:9000" {
		t.Fatalf("got %q", got)
	}
}

func TestPairingBaseURL_tlsDefaultPort(t *testing.T) {
	srv := config.ServerConfig{Host: "192.168.0.2", Port: 443}
	srv.TLS.Enabled = true
	got := PairingBaseURL(nil, srv)
	if got != "https://192.168.0.2" {
		t.Fatalf("got %q", got)
	}
}

func TestIsLoopbackURL(t *testing.T) {
	if !IsLoopbackURL("http://localhost:8080") {
		t.Fatal("expected loopback")
	}
	if IsLoopbackURL("http://192.168.1.1:8080") {
		t.Fatal("expected non-loopback")
	}
}
