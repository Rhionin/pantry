package server

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Rhionin/pantry/internal/buildinfo"
)

func TestBuildInfoEndpoint(t *testing.T) {
	originalCommit := buildinfo.Commit
	originalTime := buildinfo.CommittedAt
	originalSubject := buildinfo.Subject
	t.Cleanup(func() {
		buildinfo.Commit = originalCommit
		buildinfo.CommittedAt = originalTime
		buildinfo.Subject = originalSubject
	})

	runHandlerTests(t, []handlerTestCase{
		{
			name: "GET /api/build returns the stamped commit, time, and subject",
			setup: func(testEnv) {
				buildinfo.Commit = "0123456789abcdef0123456789abcdef01234567"
				buildinfo.CommittedAt = "2026-10-01T04:32:19Z"
				buildinfo.Subject = "Show the running commit in a quiet footer"
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				assertions: []assertion{
					{path: "$.commit", value: "0123456789abcdef0123456789abcdef01234567"},
					{path: "$.committedAt", value: "2026-10-01T04:32:19Z"},
					{path: "$.subject", value: "Show the running commit in a quiet footer"},
				},
			},
			afterRequest: assertContentTypePrefix("application/json"),
		},
		{
			name: "GET /api/build omits an unstamped time and subject",
			setup: func(testEnv) {
				buildinfo.Commit = "unknown"
				buildinfo.CommittedAt = ""
				buildinfo.Subject = ""
			},
			httpExchange: httpExchange{
				method:         "GET",
				url:            "/api/build",
				expectedStatus: http.StatusOK,
				bodyContains:   []string{`"commit":"unknown"`},
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
	defer env.Res.Body.Close()
	body, err := io.ReadAll(env.Res.Body)
	if err != nil {
		env.T.Fatalf("read body: %v", err)
	}
	text := string(body)
	if strings.Contains(text, "committedAt") || strings.Contains(text, "subject") {
		env.T.Errorf("unstamped build info should omit time and subject, got %s", text)
	}
}
