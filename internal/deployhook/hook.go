// Package deployhook accepts a signed request to update the running image.
//
// The Pi is not reachable except through the public site, and the pantry
// process cannot start a host service. A valid request writes a trigger file
// on a host-mounted directory. A systemd path unit on the host sees that
// file and starts the existing update service.
package deployhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// HeaderTimestamp is the unix time, in seconds, covered by the signature.
	HeaderTimestamp = "X-Pantry-Deploy-Timestamp"
	// HeaderSignature is the hex HMAC-SHA256 of timestamp + "." + raw body.
	HeaderSignature = "X-Pantry-Deploy-Signature"

	// DefaultTriggerPath is the container path of the host-mounted trigger file.
	DefaultTriggerPath = "/deploy-trigger/request"

	maxBody = 4096
)

// Handler is the deploy-hook HTTP endpoint.
type Handler struct {
	// Secret is the shared HMAC key. Empty disables the endpoint.
	Secret string
	// TriggerPath is the file the host path unit watches.
	TriggerPath string
	// Now is the clock. Tests set it; production leaves it nil.
	Now func() time.Time
	// MaxAge rejects timestamps older than this. That is the replay window.
	MaxAge time.Duration
	// MaxSkew rejects timestamps this far in the future.
	MaxSkew time.Duration

	mu   sync.Mutex
	seen map[string]time.Time
}

// New returns a handler with the production replay window.
func New(secret, triggerPath string) *Handler {
	if strings.TrimSpace(triggerPath) == "" {
		triggerPath = DefaultTriggerPath
	}
	return &Handler{
		Secret:      secret,
		TriggerPath: triggerPath,
		MaxAge:      5 * time.Minute,
		MaxSkew:     time.Minute,
	}
}

type triggerBody struct {
	SHA string `json:"sha"`
	Ref string `json:"ref"`
}

// ServeHTTP verifies the signature and writes the trigger file.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if strings.TrimSpace(h.Secret) == "" {
		log.Printf("deploy hook: rejected reason=not_configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "deploy hook is not configured"})
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			log.Printf("deploy hook: rejected reason=body")
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request is too large"})
			return
		}
		log.Printf("deploy hook: rejected reason=body")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "could not read the request"})
		return
	}

	now := h.now()
	tsHeader := strings.TrimSpace(r.Header.Get(HeaderTimestamp))
	sigHeader := strings.TrimSpace(r.Header.Get(HeaderSignature))
	reason := h.check(now, tsHeader, sigHeader, body)
	if reason != "" {
		log.Printf("deploy hook: rejected reason=%s", reason)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	// Claim the signature before the write so two identical requests cannot
	// both pass. A failed write releases the claim so the caller can retry.
	key := replayKey(tsHeader, sigHeader)
	if !h.claim(key, now) {
		log.Printf("deploy hook: rejected reason=replay")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	line := triggerLine(body, now)
	if err := writeTrigger(h.TriggerPath, line); err != nil {
		h.forget(key)
		log.Printf("deploy hook: could not write trigger: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not start the update"})
		return
	}
	sha, ref := triggerFields(body)
	log.Printf("deploy hook: accepted sha=%s ref=%s", sha, ref)
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// check reports why the request must be rejected. An empty reason means the
// signature and timestamp are acceptable. Replay of an already-accepted
// signature is a separate claim so a failed trigger write can be retried.
func (h *Handler) check(now time.Time, tsHeader, sigHeader string, body []byte) string {
	if tsHeader == "" || sigHeader == "" {
		return "timestamp"
	}
	unix, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil || unix <= 0 {
		return "timestamp"
	}
	sent := time.Unix(unix, 0)
	if sent.After(now.Add(h.MaxSkew)) {
		return "future"
	}
	if now.Sub(sent) > h.MaxAge {
		return "expired"
	}
	if !signatureValid(h.Secret, tsHeader, body, sigHeader) {
		return "bad_signature"
	}
	return ""
}

func signatureValid(secret, tsHeader string, body []byte, sigHeader string) bool {
	got, err := hex.DecodeString(sigHeader)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsHeader))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), got)
}

func replayKey(tsHeader, sigHeader string) string {
	return tsHeader + "\n" + strings.ToLower(sigHeader)
}

// claim records key until the replay window ends. It returns false when key
// was already accepted.
func (h *Handler) claim(key string, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.seen == nil {
		h.seen = map[string]time.Time{}
	}
	h.prune(now)
	if _, ok := h.seen[key]; ok {
		return false
	}
	h.seen[key] = now
	return true
}

func (h *Handler) forget(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.seen, key)
}

func (h *Handler) prune(now time.Time) {
	if h.seen == nil {
		return
	}
	cutoff := now.Add(-h.MaxAge)
	for key, at := range h.seen {
		if at.Before(cutoff) {
			delete(h.seen, key)
		}
	}
}

func triggerFields(body []byte) (sha, ref string) {
	var payload triggerBody
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", ""
	}
	if isHexToken(payload.SHA) {
		sha = payload.SHA
	}
	if isRef(payload.Ref) {
		ref = payload.Ref
	}
	return sha, ref
}

func triggerLine(body []byte, now time.Time) string {
	sha, ref := triggerFields(body)
	return fmt.Sprintf("sha=%s ref=%s accepted=%s\n", sha, ref, now.UTC().Format(time.RFC3339))
}

func isHexToken(s string) bool {
	if len(s) < 7 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func isRef(s string) bool {
	if s == "" || len(s) > 80 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c == '.' || c == '_' || c == '/' || c == '-':
		default:
			return false
		}
	}
	return true
}

// writeTrigger replaces the trigger file in one rename so a reader never
// sees a partial write. Two close requests each replace the file; the host
// update runs once and pulls again if the file changes mid-run.
func writeTrigger(path, line string) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".request-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmp)
		}
	}()
	if _, err := f.WriteString(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
