// Package buildinfo reports the identity of the running binary.
//
// Release images stamp Commit at link time with the same full git SHA the
// container is tagged with (ghcr.io/rhionin/pantry:<sha>). Local builds leave
// the default.
package buildinfo

// Commit is the full git SHA of this binary, or "unknown" when the build was
// not stamped. It is a variable so -ldflags -X can replace it. Release images
// set it from the Docker COMMIT_HASH build-arg, which is the same SHA the
// image tag uses.
var Commit = "unknown"
