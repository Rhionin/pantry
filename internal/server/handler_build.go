package server

import (
	"encoding/json"
	"os"
	"regexp"
	"time"

	"github.com/Rhionin/pantry/internal/buildinfo"
)

// setupSHAPattern is a full git commit. Shorter or non-hex text is omitted
// so the public build document cannot echo a path or a command.
var setupSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// setupStatusPath is the host file the updater publishes. Tests point it
// at a temp file; production reads the read-only container mount.
var setupStatusPath = "/etc/pantry/setup-state/status.json"

// buildInfoResponse is the body of GET /api/build. Commit is the full git SHA
// when the binary was stamped at link time, and "unknown" otherwise. Version
// is "dev" for a local build. CommittedAt, BuiltAt, and Subject are omitted
// when the build was not stamped with them.
//
// SetupCommit, SetupAppliedAt, and SetupStatus are the host updater's record
// of the setup tree it last applied. They are omitted until that record
// exists. The document still has no environment, host, or path.
type buildInfoResponse struct {
	Commit         string `json:"commit"`
	CommittedAt    string `json:"committedAt,omitempty"`
	BuiltAt        string `json:"builtAt,omitempty"`
	Version        string `json:"version"`
	Subject        string `json:"subject,omitempty"`
	SetupCommit    string `json:"setupCommit,omitempty"`
	SetupAppliedAt string `json:"setupAppliedAt,omitempty"`
	SetupStatus    string `json:"setupStatus,omitempty"`
}

func handleBuildInfo(Request[struct{}, struct{}]) (buildInfoResponse, error) {
	setupCommit, setupAppliedAt, setupStatus := currentSetupStatus()
	return buildInfoResponse{
		Commit:         buildinfo.Commit,
		CommittedAt:    buildinfo.CommittedAt,
		BuiltAt:        buildinfo.BuiltAt,
		Version:        buildinfo.VisibleVersion(),
		Subject:        buildinfo.VisibleSubject(),
		SetupCommit:    setupCommit,
		SetupAppliedAt: setupAppliedAt,
		SetupStatus:    setupStatus,
	}, nil
}

// currentSetupStatus reads the file the root updater publishes. A missing
// or unreadable file, and any value that is not a commit, a timestamp, or
// a known status, is left blank so the field is omitted.
func currentSetupStatus() (commit, appliedAt, status string) {
	path := os.Getenv("PANTRY_SETUP_STATUS")
	if path == "" {
		path = setupStatusPath
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", "", ""
	}
	var doc struct {
		SetupCommit    string `json:"setupCommit"`
		SetupAppliedAt string `json:"setupAppliedAt"`
		SetupStatus    string `json:"setupStatus"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", "", ""
	}
	if setupSHAPattern.MatchString(doc.SetupCommit) {
		commit = doc.SetupCommit
	}
	if _, err := time.Parse(time.RFC3339, doc.SetupAppliedAt); err == nil {
		appliedAt = doc.SetupAppliedAt
	}
	switch doc.SetupStatus {
	case "applied", "rolled-back", "failed", "refused":
		status = doc.SetupStatus
	}
	return commit, appliedAt, status
}
