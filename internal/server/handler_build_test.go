package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Rhionin/pantry/internal/buildinfo"
	"github.com/go-json-experiment/json"
)

func TestBuildInfoEndpoint(t *testing.T) {
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
	if strings.Contains(text, "committedAt") || strings.Contains(text, "builtAt") || strings.Contains(text, "subject") {
		env.T.Errorf("unstamped build info should omit times and subject, got %s", text)
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
