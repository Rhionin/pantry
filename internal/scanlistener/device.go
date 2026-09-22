//go:build linux

package scanlistener

import (
	"encoding/binary"
	"io"
	"log"

	"golang.org/x/sys/unix"
)

// Linux input event types and values we act on. Only EV_KEY presses matter:
// a barcode scanner's release and auto-repeat events carry no information we
// need, and EV_MSC/EV_SYN records interleave with every keystroke.
const (
	evKey      = 0x01
	valuePress = 1

	eventSize = 24 // sizeof(struct input_event) on 64-bit Linux
)

// KeyEvent is one decoded input_event record.
type KeyEvent struct {
	Type  uint16
	Code  uint16
	Value int32
}

// IsKeyPress reports whether this record is a key-press we should decode,
// filtering out non-EV_KEY records and key release/auto-repeat.
func (e KeyEvent) IsKeyPress() bool {
	return e.Type == evKey && e.Value == valuePress
}

// readKeyEvent reads exactly one input_event record. It returns io.EOF only
// when the reader is exhausted at a record boundary; a short read mid-record
// is io.ErrUnexpectedEOF, since a truncated record means we lost framing and
// cannot trust subsequent offsets.
func readKeyEvent(r io.Reader) (KeyEvent, error) {
	var buf [eventSize]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return KeyEvent{}, err
	}
	if n != eventSize {
		return KeyEvent{}, io.ErrUnexpectedEOF
	}

	return KeyEvent{
		Type:  binary.LittleEndian.Uint16(buf[16:18]),
		Code:  binary.LittleEndian.Uint16(buf[18:20]),
		Value: int32(binary.LittleEndian.Uint32(buf[20:24])),
	}, nil
}

// OpenFunc opens the Scanner_Device at path for reading and reports whether
// Exclusive_Grab was obtained. A false grabbed with a nil error is a usable
// device that other consumers can also read (Requirement 3.2).
type OpenFunc func(path string) (dev io.ReadCloser, grabbed bool, err error)

// openEvdev opens the evdev device at path and requests Exclusive_Grab.
// EVIOCGRAB is defined in <linux/ioctl.h> but may not be exported by all
// versions of golang.org/x/sys/unix. We use the constant value directly.
const eviocgrab = 0x40044590

func openEvdev(path string) (io.ReadCloser, bool, error) {
	f, err := unix.Open(path, unix.O_RDONLY, 0)
	if err != nil {
		return nil, false, err
	}

	// Request exclusive grab to prevent the scanner's keystrokes from also
	// appearing at the console (Requirement 3.1).
	if err := unix.IoctlSetInt(f, eviocgrab, 1); err != nil {
		log.Printf("scan listener: EVIOCGRAB failed on %s: %v, continuing without grab", path, err)
		// Don't fail — capture still works without the grab (Requirement 3.2)
		return &fdReader{f: f}, false, nil
	}

	return &fdReader{f: f}, true, nil
}

// fdReader wraps a file descriptor for io.ReadCloser interface.
type fdReader struct {
	f int
}

func (r *fdReader) Read(p []byte) (int, error) {
	return unix.Read(r.f, p)
}

func (r *fdReader) Close() error {
	return unix.Close(r.f)
}
