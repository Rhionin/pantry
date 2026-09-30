package connection_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Rhionin/pantry/internal/cart/connection"
)

func TestGenerateRandomStateYieldsAtLeast32Chars(t *testing.T) {
	state, err := connection.GenerateRandomState(bytes.NewReader(bytes.Repeat([]byte{0x41}, 64)))
	if err != nil {
		t.Fatalf("GenerateRandomState: %v", err)
	}
	if len(state) < 32 {
		t.Errorf("state length: want >= 32, got %d (%q)", len(state), state)
	}
}

func TestGenerateRandomStateDistinctReadsDiffer(t *testing.T) {
	first, err := connection.GenerateRandomState(bytes.NewReader(bytes.Repeat([]byte{0x00}, 32)))
	if err != nil {
		t.Fatalf("first GenerateRandomState: %v", err)
	}
	second, err := connection.GenerateRandomState(bytes.NewReader(bytes.Repeat([]byte{0xFF}, 32)))
	if err != nil {
		t.Fatalf("second GenerateRandomState: %v", err)
	}
	if first == second {
		t.Errorf("distinct entropy produced identical states: %q", first)
	}
}

func TestGenerateRandomStateEncodesInputDeterministically(t *testing.T) {
	input := make([]byte, 32)
	for i := range input {
		input[i] = byte(i)
	}
	got, err := connection.GenerateRandomState(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("GenerateRandomState: %v", err)
	}
	const want = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	if got != want {
		t.Errorf("GenerateRandomState: want %q, got %q", want, got)
	}
}

func TestGenerateRandomStatePropagatesReaderError(t *testing.T) {
	wantErr := errors.New("no entropy")
	_, err := connection.GenerateRandomState(errReader{wantErr})
	if !errors.Is(err, wantErr) {
		t.Errorf("GenerateRandomState error: want %v, got %v", wantErr, err)
	}
}

func TestGenerateRandomStateWithReaderYieldsAtLeast32Chars(t *testing.T) {
	first, err := connection.GenerateRandomStateWithReader()
	if err != nil {
		t.Fatalf("GenerateRandomStateWithReader: %v", err)
	}
	if len(first) < 32 {
		t.Errorf("state length: want >= 32, got %d (%q)", len(first), first)
	}

	second, err := connection.GenerateRandomStateWithReader()
	if err != nil {
		t.Fatalf("second GenerateRandomStateWithReader: %v", err)
	}
	if first == second {
		t.Errorf("crypto/rand produced identical states across calls: %q", first)
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }
