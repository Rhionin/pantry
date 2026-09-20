package connection

import (
	"crypto/rand"
	"encoding/base64"
	"io"
)

// GenerateRandomState generates a random authorization state string.
// It uses crypto/rand for production security, or an injected io.Reader for tests.
func GenerateRandomState(r io.Reader) (string, error) {
	// Generate at least 32 bytes of entropy for a 32+ character base64 string
	buf := make([]byte, 32)
	_, err := r.Read(buf)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(buf), nil
}

// GenerateRandomStateWithReader is a wrapper that uses crypto/rand by default.
func GenerateRandomStateWithReader() (string, error) {
	return GenerateRandomState(rand.Reader)
}
