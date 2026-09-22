package scanlistener

// keyKind categorizes a decoded keycode.
type keyKind int

const (
	keyUnmapped keyKind = iota
	keyPrintable
	keyEnter
	keyShift
)

// decodeKey maps a Linux keycode to the character a US-layout keyboard would
// produce, reporting which category the keycode falls into so the assembler
// can act on Enter and shift without a second lookup.
func decodeKey(code uint16, shift bool) (rune, keyKind) {
	// Digit row (top row of letters shifted to numbers)
	switch code {
	case 0x02: // KEY_1 / EXCLAM
		if shift {
			return '!', keyPrintable
		}
		return '1', keyPrintable
	case 0x03: // KEY_2 / AT
		if shift {
			return '@', keyPrintable
		}
		return '2', keyPrintable
	case 0x04: // KEY_3 / HASH
		if shift {
			return '#', keyPrintable
		}
		return '3', keyPrintable
	case 0x05: // KEY_4 / DOLLAR
		if shift {
			return '$', keyPrintable
		}
		return '4', keyPrintable
	case 0x06: // KEY_5 / PERCENT
		if shift {
			return '%', keyPrintable
		}
		return '5', keyPrintable
	case 0x07: // KEY_6 / CARET
		if shift {
			return '^', keyPrintable
		}
		return '6', keyPrintable
	case 0x08: // KEY_7 / AMPERSAND
		if shift {
			return '&', keyPrintable
		}
		return '7', keyPrintable
	case 0x09: // KEY_8 / ASTERISK
		if shift {
			return '*', keyPrintable
		}
		return '8', keyPrintable
	case 0x0a: // KEY_9 / PARENLEFT
		if shift {
			return '(', keyPrintable
		}
		return '9', keyPrintable
	case 0x0b: // KEY_0 / PARENRIGHT
		if shift {
			return ')', keyPrintable
		}
		return '0', keyPrintable
	case 0x0c: // KEY_MINUS / UNDERSCORE
		if shift {
			return '_', keyPrintable
		}
		return '-', keyPrintable
	case 0x0d: // KEY_EQUAL / PLUS
		if shift {
			return '+', keyPrintable
		}
		return '=', keyPrintable
	}

	// Letter row
	switch code {
	case 0x10: // KEY_Q
		if shift {
			return 'Q', keyPrintable
		}
		return 'q', keyPrintable
	case 0x11: // KEY_W
		if shift {
			return 'W', keyPrintable
		}
		return 'w', keyPrintable
	case 0x12: // KEY_E
		if shift {
			return 'E', keyPrintable
		}
		return 'e', keyPrintable
	case 0x13: // KEY_R
		if shift {
			return 'R', keyPrintable
		}
		return 'r', keyPrintable
	case 0x14: // KEY_T
		if shift {
			return 'T', keyPrintable
		}
		return 't', keyPrintable
	case 0x15: // KEY_Y
		if shift {
			return 'Y', keyPrintable
		}
		return 'y', keyPrintable
	case 0x16: // KEY_U
		if shift {
			return 'U', keyPrintable
		}
		return 'u', keyPrintable
	case 0x17: // KEY_I
		if shift {
			return 'I', keyPrintable
		}
		return 'i', keyPrintable
	case 0x18: // KEY_O
		if shift {
			return 'O', keyPrintable
		}
		return 'o', keyPrintable
	case 0x19: // KEY_P
		if shift {
			return 'P', keyPrintable
		}
		return 'p', keyPrintable
	case 0x1a: // KEY_LEFTBRACE
		if shift {
			return '{', keyPrintable
		}
		return '[', keyPrintable
	case 0x1b: // KEY_RIGHTBRACE
		if shift {
			return '}', keyPrintable
		}
		return ']', keyPrintable
	}

	// Middle row
	switch code {
	case 0x1e: // KEY_A
		if shift {
			return 'A', keyPrintable
		}
		return 'a', keyPrintable
	case 0x1f: // KEY_S
		if shift {
			return 'S', keyPrintable
		}
		return 's', keyPrintable
	case 0x20: // KEY_D
		if shift {
			return 'D', keyPrintable
		}
		return 'd', keyPrintable
	case 0x21: // KEY_F
		if shift {
			return 'F', keyPrintable
		}
		return 'f', keyPrintable
	case 0x22: // KEY_G
		if shift {
			return 'G', keyPrintable
		}
		return 'g', keyPrintable
	case 0x23: // KEY_H
		if shift {
			return 'H', keyPrintable
		}
		return 'h', keyPrintable
	case 0x24: // KEY_J
		if shift {
			return 'J', keyPrintable
		}
		return 'j', keyPrintable
	case 0x25: // KEY_K
		if shift {
			return 'K', keyPrintable
		}
		return 'k', keyPrintable
	case 0x26: // KEY_L
		if shift {
			return 'L', keyPrintable
		}
		return 'l', keyPrintable
	case 0x27: // KEY_SEMICOLON
		if shift {
			return ':', keyPrintable
		}
		return ';', keyPrintable
	case 0x28: // KEY_APOSTROPHE
		if shift {
			return '"', keyPrintable
		}
		return '\'', keyPrintable
	case 0x29: // KEY_GRAVE
		if shift {
			return '~', keyPrintable
		}
		return '`', keyPrintable
	}

	// Lower row
	switch code {
	case 0x2c: // KEY_Z
		if shift {
			return 'Z', keyPrintable
		}
		return 'z', keyPrintable
	case 0x2d: // KEY_X
		if shift {
			return 'X', keyPrintable
		}
		return 'x', keyPrintable
	case 0x2e: // KEY_C
		if shift {
			return 'C', keyPrintable
		}
		return 'c', keyPrintable
	case 0x2f: // KEY_V
		if shift {
			return 'V', keyPrintable
		}
		return 'v', keyPrintable
	case 0x30: // KEY_B
		if shift {
			return 'B', keyPrintable
		}
		return 'b', keyPrintable
	case 0x31: // KEY_N
		if shift {
			return 'N', keyPrintable
		}
		return 'n', keyPrintable
	case 0x32: // KEY_M
		if shift {
			return 'M', keyPrintable
		}
		return 'm', keyPrintable
	case 0x33: // KEY_COMMA
		if shift {
			return '<', keyPrintable
		}
		return ',', keyPrintable
	case 0x34: // KEY_DOT
		if shift {
			return '>', keyPrintable
		}
		return '.', keyPrintable
	case 0x35: // KEY_SLASH
		if shift {
			return '?', keyPrintable
		}
		return '/', keyPrintable
	case 0x36: // KEY_RIGHTSHIFT
		return 0, keyShift
	}

	// Keypad digits (many scanners use these instead of digit row)
	switch code {
	case 0x52: // KEY_KP0
		return '0', keyPrintable
	case 0x53: // KEY_KP1
		return '1', keyPrintable
	case 0x54: // KEY_KP2
		return '2', keyPrintable
	case 0x55: // KEY_KP3
		return '3', keyPrintable
	case 0x56: // KEY_KP4
		return '4', keyPrintable
	case 0x57: // KEY_KP5
		return '5', keyPrintable
	case 0x58: // KEY_KP6
		return '6', keyPrintable
	case 0x59: // KEY_KP7
		return '7', keyPrintable
	case 0x5a: // KEY_KP8
		return '8', keyPrintable
	case 0x5b: // KEY_KP9
		return '9', keyPrintable
	}

	// Space and special keys
	switch code {
	case 0x39: // KEY_SPACE
		return ' ', keyPrintable
	case 0x2a: // KEY_ENTER
		return 0, keyEnter
	case 0x9c: // KEY_ENTER (numpad enter)
		return 0, keyEnter
	}

	// Unmapped: return empty rune with keyUnmapped kind
	return 0, keyUnmapped
}
