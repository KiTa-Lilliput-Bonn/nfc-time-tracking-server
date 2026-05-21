package lanemployeesync

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestExecute_GETEmployeesNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/employees" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("denied"))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + host + ":" + strconv.Itoa(port)

	svc := NewService(nil, nil, nil, nil)
	svc.SetHTTPClient(srv.Client())

	_, status, msg := svc.Execute(context.Background(), base, "any-bearer")
	if status != http.StatusBadGateway {
		t.Fatalf("status: want %d got %d msg=%q", http.StatusBadGateway, status, msg)
	}
	if !strings.Contains(msg, "app GET /v1/employees") {
		t.Fatalf("msg: %q", msg)
	}
}
