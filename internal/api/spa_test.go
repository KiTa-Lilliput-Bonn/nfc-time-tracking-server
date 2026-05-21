package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWithSPA_servesAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	static := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>app</html>")},
	}
	h := WithSPA(api, static)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("api: got %d", rec.Code)
	}
}

func TestWithSPA_fallbackIndexHTML(t *testing.T) {
	api := http.NotFoundHandler()
	static := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa</html>")},
		"assets/x.js": &fstest.MapFile{
			Data: []byte("console.log(1)"),
		},
	}
	h := WithSPA(api, static)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard route: %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type: %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "spa") {
		t.Fatalf("body: %q", rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/assets/x.js", nil)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("asset: %d", rec2.Code)
	}
}

func TestWithSPA_rejectNonGetOnStatic(t *testing.T) {
	static := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("x")}}
	h := WithSPA(http.NewServeMux(), static)

	req := httptest.NewRequest(http.MethodPost, "/foo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /foo: %d", rec.Code)
	}
}

func TestWithSPA_rootServesIndex(t *testing.T) {
	h := WithSPA(http.NotFoundHandler(), fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<root>")},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<root>") {
		t.Fatalf("root: %d %q", rec.Code, rec.Body.String())
	}
}
