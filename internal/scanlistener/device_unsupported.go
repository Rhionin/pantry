//go:build !linux

package scanlistener

import (
	"fmt"
	"io"
)

// openEvdev returns an error on non-Linux platforms since there is no evdev subsystem.
func openEvdev(path string) (io.ReadCloser, bool, error) {
	return nil, false, fmt.Errorf("evdev device access not supported on this platform")
}
