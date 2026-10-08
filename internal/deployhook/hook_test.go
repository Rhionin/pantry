package deployhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Vector shared with deploy/check-deploy-hook.sh. The workflow signs the
// same way openssl does: timestamp, a dot, then the raw body.
const (
	vectorSecret = "test-secret"
	vectorTS     = "1700000000"
	vectorBody   = `{"sha":"abc1234","ref":"master"}`
	vectorSig    = "c6afd39562c3fe40be64d17011d04ee5969a3008270e555483176e963d36f018"
)

func TestSignatureMatchesOpenSSLVector(t *testing.T) {
	if !signatureValid(vectorSecret, vectorTS, []byte(vectorBody), vectorSig) {
		t.Fatal("signature does not match the openssl vector")
	}
	if signatureValid(vectorSecret, vectorTS, []byte(vectorBody), "00"+vectorSig[2:]) {
		t.Fatal("a different signature was accepted")
	}
	if signatureValid(vectorSecret, vectorTS, []byte(vectorBody+" "), vectorSig) {
		t.Fatal("a tampered body was accepted")
	}
	upper := strings.ToUpper(vectorSig)
	if !signatureValid(vectorSecret, vectorTS, []byte(vectorBody), upper) {
		t.Fatal("uppercase hex was rejected")
	}
}

func TestAcceptsValidSignature(t *testing.T) {
	h, path := testHandler(t)
	rec := post(t, h, vectorTS, vectorSig, vectorBody)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "accepted" {
		t.Fatalf("status field = %q", got["status"])
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "sha=abc1234") || !strings.Contains(string(text), "ref=master") {
		t.Fatalf("trigger = %q", text)
	}
}

func TestRejectsBadSignature(t *testing.T) {
	h, path := testHandler(t)
	rec := post(t, h, vectorTS, strings.Repeat("ab", 32), vectorBody)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("trigger file err = %v", err)
	}
}

func TestRejectsExpiredTimestamp(t *testing.T) {
	h, path := testHandler(t)
	// Ten minutes before the handler clock. The HMAC is valid; the age is not.
	old := strconv.FormatInt(h.now().Add(-10*time.Minute).Unix(), 10)
	body := vectorBody
	sig := sign(h.Secret, old, body)
	rec := post(t, h, old, sig, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired request wrote a trigger, err = %v", err)
	}
}

func TestRejectsFutureTimestamp(t *testing.T) {
	h, _ := testHandler(t)
	ahead := strconv.FormatInt(h.now().Add(2*time.Minute).Unix(), 10)
	body := vectorBody
	rec := post(t, h, ahead, sign(h.Secret, ahead, body), body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestRejectsReplayOfAcceptedRequest(t *testing.T) {
	h, path := testHandler(t)
	first := post(t, h, vectorTS, vectorSig, vectorBody)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, body %s", first.Code, first.Body.String())
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second := post(t, h, vectorTS, vectorSig, vectorBody)
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replay status = %d, body %s", second.Code, second.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("replay rewrote the trigger\nbefore %q\nafter %q", before, after)
	}
}

func TestSecondTriggerReplacesFile(t *testing.T) {
	h, path := testHandler(t)
	now := h.now()
	firstTS := strconv.FormatInt(now.Unix(), 10)
	secondTS := strconv.FormatInt(now.Add(time.Second).Unix(), 10)
	firstBody := `{"sha":"aaaaaaaa","ref":"master"}`
	secondBody := `{"sha":"bbbbbbbb","ref":"master"}`
	if rec := post(t, h, firstTS, sign(h.Secret, firstTS, firstBody), firstBody); rec.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", rec.Code)
	}
	if rec := post(t, h, secondTS, sign(h.Secret, secondTS, secondBody), secondBody); rec.Code != http.StatusAccepted {
		t.Fatalf("second status = %d body %s", rec.Code, rec.Body.String())
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "sha=bbbbbbbb") {
		t.Fatalf("trigger = %q", text)
	}
	if strings.Contains(string(text), "sha=aaaaaaaa") {
		t.Fatalf("old trigger left in place: %q", text)
	}
}

func TestRejectsWhenSecretUnset(t *testing.T) {
	dir := t.TempDir()
	h := New("", filepath.Join(dir, "request"))
	h.Now = func() time.Time { return time.Unix(1700000000, 0) }
	rec := post(t, h, vectorTS, vectorSig, vectorBody)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestWriteFailureReleasesReplayClaim(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing", "request")
	h := New(vectorSecret, missing)
	h.Now = func() time.Time { return time.Unix(1700000000, 0) }
	first := post(t, h, vectorTS, vectorSig, vectorBody)
	if first.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body %s", first.Code, first.Body.String())
	}
	if err := os.MkdirAll(filepath.Dir(missing), 0o755); err != nil {
		t.Fatal(err)
	}
	second := post(t, h, vectorTS, vectorSig, vectorBody)
	if second.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, body %s", second.Code, second.Body.String())
	}
}

func TestPrunesExpiredReplayKeys(t *testing.T) {
	h, _ := testHandler(t)
	start := h.now()
	h.Now = func() time.Time { return start }
	body := vectorBody
	firstTS := strconv.FormatInt(start.Unix(), 10)
	if rec := post(t, h, firstTS, sign(h.Secret, firstTS, body), body); rec.Code != http.StatusAccepted {
		t.Fatalf("first status = %d", rec.Code)
	}
	later := start.Add(6 * time.Minute)
	h.Now = func() time.Time { return later }
	secondTS := strconv.FormatInt(later.Unix(), 10)
	if rec := post(t, h, secondTS, sign(h.Secret, secondTS, body), body); rec.Code != http.StatusAccepted {
		t.Fatalf("second status = %d body %s", rec.Code, rec.Body.String())
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.seen) != 1 {
		t.Fatalf("replay cache size = %d, want 1", len(h.seen))
	}
}

func TestRejectsOtherMethodsAndLargeBodies(t *testing.T) {
	h, _ := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/deploy-hook", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec.Code)
	}
	huge := strings.Repeat("a", maxBody+1)
	req = httptest.NewRequest(http.MethodPost, "/api/deploy-hook", strings.NewReader(huge))
	req.Header.Set(HeaderTimestamp, vectorTS)
	req.Header.Set(HeaderSignature, vectorSig)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large status = %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestAllowsClockSkew(t *testing.T) {
	h, path := testHandler(t)
	ahead := strconv.FormatInt(h.now().Add(30*time.Second).Unix(), 10)
	body := vectorBody
	rec := post(t, h, ahead, sign(h.Secret, ahead, body), body)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func testHandler(t *testing.T) (*Handler, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "request")
	h := New(vectorSecret, path)
	h.Now = func() time.Time { return time.Unix(1700000000, 0) }
	return h, path
}

func post(t *testing.T, h *Handler, ts, sig, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/deploy-hook", strings.NewReader(body))
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, sig)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sign(secret, ts, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte{'.'})
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
