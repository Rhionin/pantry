package scanlistener

import "testing"

func TestParseSource(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   Source
		wantOK bool
	}{
		{"empty string", "", SourceDevice, true},
		{"explicit device", "device", SourceDevice, true},
		{"explicit stdin", "stdin", SourceStdin, true},
		{"unrecognized value", "invalid", SourceDevice, false},
		{"uppercase", "DEVICE", SourceDevice, false},
		{"mixed case", "Device", SourceDevice, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseSource(tt.input)
			if got != tt.want {
				t.Errorf("ParseSource(%q) = %v, want %v", tt.input, got, tt.want)
			}
			if ok != tt.wantOK {
				t.Errorf("ParseSource(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}
