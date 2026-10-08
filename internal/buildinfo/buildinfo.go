// Package buildinfo reports the identity of the running binary.
//
// Release images stamp these strings at link time from the same commit the
// container is tagged with (ghcr.io/rhionin/pantry:<sha>). Local builds leave
// the defaults: Commit is "unknown", Version is "dev", and the times and
// subject stay empty so callers can omit them.
package buildinfo

import "encoding/base64"

// Commit is the full git SHA of this binary, or "unknown" when the build was
// not stamped. It is a variable so -ldflags -X can replace it.
var Commit = "unknown"

// CommittedAt is the commit timestamp in ISO 8601, or empty when the build
// was not stamped. It is a variable so -ldflags -X can replace it.
var CommittedAt = ""

// BuiltAt is when the image was built, in ISO 8601 UTC, or empty for a build
// that was not stamped. It is a variable so -ldflags -X can replace it.
var BuiltAt = ""

// Version is the release label. CI stamps vYYYYMMDD plus a 7-character SHA,
// the same formula as the annotated tag. A local build leaves "dev".
var Version = "dev"

// Subject is the commit subject line. Tests and callers set it directly.
// Release images leave it empty and stamp subjectStamp instead, because a
// free-text subject cannot be spliced into -ldflags without quoting hazards.
var Subject = ""

// subjectStamp is the base64-encoded commit subject set by -ldflags. VisibleSubject
// decodes it when Subject itself was not set.
var subjectStamp = ""

// VisibleSubject returns the commit subject: Subject when set, otherwise the
// decoded link-time stamp. An empty or invalid stamp yields an empty string
// so the API can omit the field.
func VisibleSubject() string {
	if Subject != "" {
		return Subject
	}
	return DecodeSubjectStamp(subjectStamp)
}

// VisibleVersion returns the release label. An empty stamp is "dev" so a
// local binary still answers instead of omitting the field.
func VisibleVersion() string {
	if Version == "" {
		return "dev"
	}
	return Version
}

// DecodeSubjectStamp decodes a base64 commit subject. Empty and invalid
// stamps decode to an empty string.
func DecodeSubjectStamp(stamp string) string {
	if stamp == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(stamp)
	if err != nil {
		return ""
	}
	return string(decoded)
}
