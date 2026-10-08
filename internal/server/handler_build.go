package server

import "github.com/Rhionin/pantry/internal/buildinfo"

// buildInfoResponse is the body of GET /api/build. Commit is the full git SHA
// when the binary was stamped at link time, and "unknown" otherwise. Version
// is "dev" for a local build. CommittedAt, BuiltAt, and Subject are omitted
// when the build was not stamped with them. The document is only that
// identity: no environment, host, or path.
type buildInfoResponse struct {
	Commit      string `json:"commit"`
	CommittedAt string `json:"committedAt,omitempty"`
	BuiltAt     string `json:"builtAt,omitempty"`
	Version     string `json:"version"`
	Subject     string `json:"subject,omitempty"`
}

func handleBuildInfo(Request[struct{}, struct{}]) (buildInfoResponse, error) {
	return buildInfoResponse{
		Commit:      buildinfo.Commit,
		CommittedAt: buildinfo.CommittedAt,
		BuiltAt:     buildinfo.BuiltAt,
		Version:     buildinfo.VisibleVersion(),
		Subject:     buildinfo.VisibleSubject(),
	}, nil
}
