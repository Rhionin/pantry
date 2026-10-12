package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// postUnknownLength sends a request whose length the server cannot know ahead
// of time. Browsers usually set Content-Length. A chunked client does not.
func postUnknownLength(t *testing.T, handler http.Handler, path, body string) (int64, *http.Response) {
	t.Helper()
	var seen int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.ContentLength
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	if seen != -1 {
		t.Fatalf("server ContentLength = %d, want -1 (chunked)", seen)
	}
	return seen, res
}

func TestChunkedJSONBodyIsApplied(t *testing.T) {
	handler, _ := setupTestWithDB(t)
	_, res := postUnknownLength(t, handler, "/api/products", `{"name":"Chunked Oats"}`)
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", res.StatusCode, payload)
	}
	if !strings.Contains(string(payload), "Chunked Oats") {
		t.Fatalf("body = %s", payload)
	}
}

func TestChunkedOversizedJSONIsRejected(t *testing.T) {
	handler, _ := setupTestWithDB(t)
	body := `{"name":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`
	_, res := postUnknownLength(t, handler, "/api/products", body)
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.StatusCode, payload)
	}
	if !strings.Contains(string(payload), "request body is too large") {
		t.Fatalf("body = %s", payload)
	}
}

func TestChunkedInvalidJSONIsRejected(t *testing.T) {
	handler, _ := setupTestWithDB(t)
	_, res := postUnknownLength(t, handler, "/api/products", `{"name":`)
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.StatusCode, payload)
	}
	if !strings.Contains(string(payload), "invalid request body") {
		t.Fatalf("body = %s", payload)
	}
	if strings.Contains(string(payload), "name is required") {
		t.Fatalf("ignored the body and treated it as empty: %s", payload)
	}
}
