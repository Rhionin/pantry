//go:build !linux

package scanlistener

import (
	"fmt"
	"io"
)

// OpenFunc is a function type for opening evdev devices.
// On non-Linux platforms, this is a stub type.
type OpenFunc func(path string) (io.ReadCloser, bool, error)

// openEvdev returns an error on non-Linux platforms since there is no evdev subsystem.
func openEvdev(path string) (io.ReadCloser, bool, error) {
	return nil, false, fmt.Errorf("evdev device access not supported on this platform")
}

// KeyEvent is a stub type for non-Linux platforms.
// On Linux, this is defined in device.go with actual evdev fields.
type KeyEvent struct {
	Type  uint16
	Code  uint16
	Value int32
}

// IsKeyPress reports whether this record is a key-press we should decode.
// On non-Linux platforms, this always returns false since evdev is not available.
func (e KeyEvent) IsKeyPress() bool {
	return false
}

// readKeyEvent reads a key event from a reader.
// On non-Linux platforms, this returns an error since evdev is not supported.
func readKeyEvent(r io.Reader) (KeyEvent, error) {
	return KeyEvent{}, fmt.Errorf("key event reading not supported on this platform")
}

// evKey and valuePress are constants defined only on Linux.
// These are stubbed here for compilation purposes but should not be used on other platforms.
const (
	evKey      = 0x01
	valuePress = 1
)
