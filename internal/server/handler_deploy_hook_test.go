package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDeployHookRouteWritesTrigger(t *testing.T) {
	dir := t.TempDir()
	trigger := filepath.Join(dir, "request")
	const secret = "test-secret"
	handler, _ := setupTestWithContributor(t, nil, WithDeployHook(secret, trigger))

	body := `{"sha":"abc1234","ref":"master"}`
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte{'.'})
	mac.Write([]byte(body))
	req := httptest.NewRequest(http.MethodPost, "/api/deploy-hook", strings.NewReader(body))
	req.Header.Set("X-Pantry-Deploy-Timestamp", ts)
	req.Header.Set("X-Pantry-Deploy-Signature", hex.EncodeToString(mac.Sum(nil)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(trigger)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "sha=abc1234") || !strings.Contains(string(got), "ref=master") {
		t.Fatalf("trigger = %q", got)
	}

	again := httptest.NewRequest(http.MethodPost, "/api/deploy-hook", strings.NewReader(body))
	again.Header = req.Header
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, again)
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, body %s", replay.Code, replay.Body.String())
	}
}

func TestDeployHookRejectsWhenUnset(t *testing.T) {
	handler, _ := setupTestWithDB(t)
	req := httptest.NewRequest(http.MethodPost, "/api/deploy-hook", strings.NewReader(`{}`))
	req.Header.Set("X-Pantry-Deploy-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Pantry-Deploy-Signature", strings.Repeat("ab", 32))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}
