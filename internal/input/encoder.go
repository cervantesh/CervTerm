package input

import (
	"fmt"
	"strings"
	"unicode"
)

type Key int

const (
	KeyUnknown Key = iota
	KeyEnter
	KeyBackspace
	KeyTab
	KeyEscape
	KeyUp
	KeyDown
	KeyRight
	KeyLeft
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyDelete
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
)

type Mod uint8

const (
	ModNone Mod = 0
	ModCtrl Mod = 1 << iota
	ModAlt
	ModShift
)

type Event struct {
	Rune rune
	Key  Key
	Mods Mod
}

type Mode struct {
	ApplicationCursor bool
}

type ClipboardAction int

const (
	ClipboardNone ClipboardAction = iota
	ClipboardCopy
	ClipboardPaste
)

func Encode(event Event) ([]byte, bool) {
	return EncodeWithMode(event, Mode{})
}

func EncodeWithMode(event Event, mode Mode) ([]byte, bool) {
	if event.Rune != 0 {
		return encodeRune(event)
	}

	switch event.Key {
	case KeyEnter:
		return []byte("\r"), true
	case KeyBackspace:
		return []byte("\x7f"), true
	case KeyTab:
		if event.Mods == ModShift {
			// Xterm/VT back-tab (CBT). Interactive TUIs such as Pi distinguish
			// this from the plain HT byte used by Tab.
			return []byte("\x1b[Z"), true
		}
		return []byte("\t"), true
	case KeyEscape:
		return []byte("\x1b"), true
	case KeyUp:
		return encodeArrow('A', event.Mods, mode.ApplicationCursor), true
	case KeyDown:
		return encodeArrow('B', event.Mods, mode.ApplicationCursor), true
	case KeyRight:
		return encodeArrow('C', event.Mods, mode.ApplicationCursor), true
	case KeyLeft:
		return encodeArrow('D', event.Mods, mode.ApplicationCursor), true
	case KeyHome:
		return encodeCSIKey("H", event.Mods), true
	case KeyEnd:
		return encodeCSIKey("F", event.Mods), true
	case KeyInsert:
		return encodeTildeKey(2, event.Mods), true
	case KeyDelete:
		return encodeTildeKey(3, event.Mods), true
	case KeyPageUp:
		return encodeTildeKey(5, event.Mods), true
	case KeyPageDown:
		return encodeTildeKey(6, event.Mods), true
	case KeyF1:
		return encodeSS3OrCSI('P', event.Mods), true
	case KeyF2:
		return encodeSS3OrCSI('Q', event.Mods), true
	case KeyF3:
		return encodeSS3OrCSI('R', event.Mods), true
	case KeyF4:
		return encodeSS3OrCSI('S', event.Mods), true
	case KeyF5:
		return encodeTildeKey(15, event.Mods), true
	case KeyF6:
		return encodeTildeKey(17, event.Mods), true
	case KeyF7:
		return encodeTildeKey(18, event.Mods), true
	case KeyF8:
		return encodeTildeKey(19, event.Mods), true
	case KeyF9:
		return encodeTildeKey(20, event.Mods), true
	case KeyF10:
		return encodeTildeKey(21, event.Mods), true
	case KeyF11:
		return encodeTildeKey(23, event.Mods), true
	case KeyF12:
		return encodeTildeKey(24, event.Mods), true
	default:
		return nil, false
	}
}

func encodeRune(event Event) ([]byte, bool) {
	if event.Mods&ModCtrl != 0 {
		r := unicode.ToLower(event.Rune)
		if r >= 'a' && r <= 'z' {
			if r == 'v' {
				return nil, false
			}
			return []byte{byte(r - 'a' + 1)}, true
		}
		return nil, false
	}
	if event.Mods&ModAlt != 0 {
		return append([]byte("\x1b"), string(event.Rune)...), true
	}
	return []byte(string(event.Rune)), true
}

func encodeArrow(final byte, mods Mod, applicationCursor bool) []byte {
	if mods == ModNone && applicationCursor {
		return []byte{0x1b, 'O', final}
	}
	return encodeCSIKey(string(final), mods)
}

func encodeCSIKey(final string, mods Mod) []byte {
	mod := xtermModifier(mods)
	if mod == 1 {
		return []byte("\x1b[" + final)
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%s", mod, final))
}

func encodeTildeKey(code int, mods Mod) []byte {
	mod := xtermModifier(mods)
	if mod == 1 {
		return []byte(fmt.Sprintf("\x1b[%d~", code))
	}
	return []byte(fmt.Sprintf("\x1b[%d;%d~", code, mod))
}

func encodeSS3OrCSI(final byte, mods Mod) []byte {
	mod := xtermModifier(mods)
	if mod == 1 {
		return []byte{0x1b, 'O', final}
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%c", mod, final))
}

func xtermModifier(mods Mod) int {
	value := 1
	if mods&ModShift != 0 {
		value += 1
	}
	if mods&ModAlt != 0 {
		value += 2
	}
	if mods&ModCtrl != 0 {
		value += 4
	}
	return value
}

func EncodePaste(text string, bracketed bool) []byte {
	if !bracketed {
		return []byte(text)
	}
	text = strings.ReplaceAll(text, "\x1b[201~", "")
	out := make([]byte, 0, len(text)+12)
	out = append(out, "\x1b[200~"...)
	out = append(out, text...)
	out = append(out, "\x1b[201~"...)
	return out
}

func ClipboardShortcut(event Event) ClipboardAction {
	if event.Mods&(ModCtrl|ModShift) != (ModCtrl | ModShift) {
		return ClipboardNone
	}
	switch unicode.ToLower(event.Rune) {
	case 'c':
		return ClipboardCopy
	case 'v':
		return ClipboardPaste
	default:
		return ClipboardNone
	}
}
