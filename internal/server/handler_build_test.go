package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rhionin/pantry/internal/buildinfo"
	"github.com/go-json-experiment/json"
)

func TestBuildInfoEndpoint(t *testing.T) {
	t.Setenv("PANTRY_SETUP_STATUS", "")
	originalStatusPath := setupStatusPath
	setupStatusPath = filepath.Join(t.TempDir(), "missing.json")
	t.Cleanup(func() { setupStatusPath = originalStatusPath })

	originalCommit := buildinfo.Commit
	originalTime := buildinfo.CommittedAt
	originalBuilt := buildinfo.BuiltAt
	originalVersion := buildinfo.Version
	originalSubject := buildinfo.Subject
	t.Cleanup(func() {
		buildinfo.Commit = originalCommit
		buildinfo.CommittedAt = originalTime
		buildinfo.BuiltAt = originalBuilt
		buildinfo.Version = originalVersion
		buildinfo.Subject = originalSubject
	})

	runHandlerTests(t, []handlerTestCase{
		{
			name: "GET /api/build returns the stamped commit, times, version, and subject",
			setup: func(testEnv) {
				buildinfo.Commit = "0123456789abcdef0123456789abcdef01234567"
				buildinfo.CommittedAt = "2026-10-01T04:32:19Z"
				buildinfo.BuiltAt = "2026-10-08T15:04:00Z"
				buildinfo.Version = "v20261008-0123456"
				buildinfo.Subject = "Show the running commit in a quiet footer"
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				expectedHeaders: map[string]string{
					"Cache-Control": "no-store",
				},
				assertions: []assertion{
					{path: "$.commit", value: "0123456789abcdef0123456789abcdef01234567"},
					{path: "$.committedAt", value: "2026-10-01T04:32:19Z"},
					{path: "$.builtAt", value: "2026-10-08T15:04:00Z"},
					{path: "$.version", value: "v20261008-0123456"},
					{path: "$.subject", value: "Show the running commit in a quiet footer"},
				},
			},
			afterRequest: assertBuildInfoKeys,
		},
		{
			name: "GET /api/build reports dev and omits an unstamped time and subject",
			setup: func(testEnv) {
				buildinfo.Commit = "unknown"
				buildinfo.CommittedAt = ""
				buildinfo.BuiltAt = ""
				buildinfo.Version = ""
				buildinfo.Subject = ""
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{`"commit":"unknown"`, `"version":"dev"`},
			},
			afterRequest: assertBuildInfoOmitsEmptyFields,
		},
		{
			name: "GET /api/build reports the applied setup commit and status",
			setup: func(env testEnv) {
				path := filepath.Join(env.T.TempDir(), "status.json")
				body := `{"setupCommit":"0123456789abcdef0123456789abcdef01234567","setupAppliedAt":"2026-10-09T14:00:00Z","setupStatus":"applied","secret":"nope"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					env.T.Fatalf("write status: %v", err)
				}
				env.T.Setenv("PANTRY_SETUP_STATUS", path)
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.setupCommit", value: "0123456789abcdef0123456789abcdef01234567"},
					{path: "$.setupAppliedAt", value: "2026-10-09T14:00:00Z"},
					{path: "$.setupStatus", value: "applied"},
					{path: "$.secret", absent: true},
				},
				bodyExcludes: []string{"nope"},
			},
			afterRequest: assertBuildInfoKeys,
		},
		{
			name: "GET /api/build reports a rolled-back setup",
			setup: func(env testEnv) {
				path := filepath.Join(env.T.TempDir(), "status.json")
				body := `{"setupCommit":"abcdefabcdefabcdefabcdefabcdefabcdefabcd","setupAppliedAt":"2026-10-09T15:04:05Z","setupStatus":"rolled-back"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					env.T.Fatalf("write status: %v", err)
				}
				env.T.Setenv("PANTRY_SETUP_STATUS", path)
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.setupCommit", value: "abcdefabcdefabcdefabcdefabcdefabcdefabcd"},
					{path: "$.setupStatus", value: "rolled-back"},
				},
			},
			afterRequest: assertBuildInfoKeys,
		},
		{
			name: "GET /api/build omits a setup record that is not a commit or status",
			setup: func(env testEnv) {
				path := filepath.Join(env.T.TempDir(), "status.json")
				body := `{"setupCommit":"../etc/passwd","setupAppliedAt":"yesterday","setupStatus":"applied;rm"}`
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					env.T.Fatalf("write status: %v", err)
				}
				env.T.Setenv("PANTRY_SETUP_STATUS", path)
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				bodyExcludes:   []string{"setupCommit", "setupAppliedAt", "setupStatus", "passwd", "yesterday"},
			},
			afterRequest: assertBuildInfoOmitsEmptyFields,
		},
		{
			name: "GET /api/build ignores a setup record that is not JSON",
			setup: func(env testEnv) {
				path := filepath.Join(env.T.TempDir(), "status.json")
				if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
					env.T.Fatalf("write status: %v", err)
				}
				env.T.Setenv("PANTRY_SETUP_STATUS", path)
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				bodyExcludes:   []string{"setupCommit", "not-json"},
			},
			afterRequest: assertBuildInfoOmitsEmptyFields,
		},
		{
			name: "POST /api/build is not a build lookup",
			httpExchange: httpExchange{
				method:         "POST",
				url:            "/api/build",
				expectedStatus: http.StatusMethodNotAllowed,
			},
			afterRequest: assertBodyExcludesPlaceholder,
		},
	})
}

func assertBuildInfoOmitsEmptyFields(env testEnv) {
	body := readBuildInfoBody(env)
	text := string(body)
	if strings.Contains(text, "committedAt") || strings.Contains(text, "builtAt") || strings.Contains(text, "subject") ||
		strings.Contains(text, "setupCommit") || strings.Contains(text, "setupAppliedAt") || strings.Contains(text, "setupStatus") {
		env.T.Errorf("unstamped build info should omit times, subject, and setup status, got %s", text)
	}
	assertBuildInfoKeySet(env.T, body)
}

func assertBuildInfoKeys(env testEnv) {
	assertContentTypePrefix("application/json")(env)
	assertBuildInfoKeySet(env.T, readBuildInfoBody(env))
}

func readBuildInfoBody(env testEnv) []byte {
	env.T.Helper()
	defer env.Res.Body.Close()
	body, err := io.ReadAll(env.Res.Body)
	if err != nil {
		env.T.Fatalf("read body: %v", err)
	}
	return body
}

// assertBuildInfoKeySet keeps the public document to build identity. A new
// field has to be named here on purpose, so a hostname or path cannot ride
// along.
func assertBuildInfoKeySet(t *testing.T, body []byte) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode body %s: %v", body, err)
	}
	allowed := map[string]bool{
		"commit": true, "committedAt": true, "builtAt": true, "version": true, "subject": true,
		"setupCommit": true, "setupAppliedAt": true, "setupStatus": true,
	}
	for key := range raw {
		if !allowed[key] {
			t.Errorf("GET /api/build included %q", key)
		}
	}
	if _, ok := raw["commit"]; !ok {
		t.Error("GET /api/build omitted commit")
	}
	if _, ok := raw["version"]; !ok {
		t.Error("GET /api/build omitted version")
	}
}
