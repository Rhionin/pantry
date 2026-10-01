package server

import "github.com/Rhionin/pantry/internal/buildinfo"

// buildInfoResponse is the body of GET /api/build. Commit is the full git SHA
// when the binary was stamped at link time, and "unknown" otherwise.
type buildInfoResponse struct {
	Commit string `json:"commit"`
}

func handleBuildInfo(Request[struct{}, struct{}]) (buildInfoResponse, error) {
	return buildInfoResponse{Commit: buildinfo.Commit}, nil
}
