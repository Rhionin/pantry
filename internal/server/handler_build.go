package server

import "github.com/Rhionin/pantry/internal/buildinfo"

// buildInfoResponse is the body of GET /api/build. Commit is the full git SHA
// when the binary was stamped at link time, and "unknown" otherwise.
// CommittedAt and Subject are omitted when the build was not stamped with them.
type buildInfoResponse struct {
	Commit      string `json:"commit"`
	CommittedAt string `json:"committedAt,omitempty"`
	Subject     string `json:"subject,omitempty"`
}

func handleBuildInfo(Request[struct{}, struct{}]) (buildInfoResponse, error) {
	return buildInfoResponse{
		Commit:      buildinfo.Commit,
		CommittedAt: buildinfo.CommittedAt,
		Subject:     buildinfo.VisibleSubject(),
	}, nil
}
