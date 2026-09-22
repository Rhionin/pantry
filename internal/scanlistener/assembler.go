package scanlistener

// lineAssembler accumulates decoded characters into a Scan_Line and reports a
// completed barcode when Enter arrives.
type lineAssembler struct {
	buf      []rune
	shift    bool
	unmapped int
}

// feed processes one Key_Event. It returns ok=true exactly once per Enter,
// with the accumulated line; every other event returns ok=false.
func (a *lineAssembler) feed(e KeyEvent) (line string, ok bool) {
	// Shift key handling: Value=1 is press (set shift), Value=0 is release (clear shift)
	// Key code 0x36 is KEY_RIGHTSHIFT
	if e.Type == evKey && e.Code == 0x36 {
		if e.Value == valuePress {
			a.shift = true
		} else {
			a.shift = false
		}
		return "", false
	}

	if !e.IsKeyPress() {
		return "", false
	}

	rune, kind := decodeKey(e.Code, a.shift)
	switch kind {
	case keyPrintable:
		a.buf = append(a.buf, rune)

	case keyEnter:
		line = string(a.buf)
		a.buf = a.buf[:0] // clear buffer
		return line, true

	case keyShift:
		// Shift key codes have Value=1 on press, Value=0 on release
		// This case should not be reached due to early handling above
		if e.Value == valuePress {
			a.shift = true
		} else {
			a.shift = false
		}

	case keyUnmapped:
		a.unmapped++
	}

	return "", false
}

// reset discards a partially accumulated Scan_Line. Called on every
// reconnect so characters from two device connections can never be spliced
// into one barcode (Requirement 2.3).
func (a *lineAssembler) reset() {
	a.buf = a.buf[:0]
	a.shift = false
}

// UnmappedCount returns the count of discarded unmapped keycodes.
func (a *lineAssembler) UnmappedCount() int {
	return a.unmapped
}
