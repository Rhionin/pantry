package buildinfo

import (
	"encoding/base64"
	"testing"

	"pgregory.net/rapid"
)

func TestDecodeSubjectStampRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		subject := rapid.String().Draw(t, "subject")
		stamp := base64.StdEncoding.EncodeToString([]byte(subject))
		if got := DecodeSubjectStamp(stamp); got != subject {
			t.Fatalf("DecodeSubjectStamp(%q) = %q, want %q", stamp, got, subject)
		}
	})
}

func TestDecodeSubjectStampRejectsEmptyAndInvalid(t *testing.T) {
	if got := DecodeSubjectStamp(""); got != "" {
		t.Fatalf("empty stamp = %q, want empty", got)
	}
	if got := DecodeSubjectStamp("not base64!!!"); got != "" {
		t.Fatalf("invalid stamp = %q, want empty", got)
	}
}

func TestVisibleSubjectPrefersDirectValue(t *testing.T) {
	originalSubject := Subject
	originalStamp := subjectStamp
	t.Cleanup(func() {
		Subject = originalSubject
		subjectStamp = originalStamp
	})

	Subject = "direct subject"
	subjectStamp = base64.StdEncoding.EncodeToString([]byte("stamped subject"))
	if got := VisibleSubject(); got != "direct subject" {
		t.Fatalf("VisibleSubject() = %q, want the direct subject", got)
	}
}

func TestVisibleSubjectDecodesStamp(t *testing.T) {
	originalSubject := Subject
	originalStamp := subjectStamp
	t.Cleanup(func() {
		Subject = originalSubject
		subjectStamp = originalStamp
	})

	Subject = ""
	subjectStamp = base64.StdEncoding.EncodeToString([]byte(`Say "hi" it's deployed`))
	if got := VisibleSubject(); got != `Say "hi" it's deployed` {
		t.Fatalf("VisibleSubject() = %q", got)
	}
}
