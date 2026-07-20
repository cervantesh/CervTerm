package core

type MouseMode struct {
	NormalTracking      bool
	ButtonEventTracking bool
	AnyEventTracking    bool
	SGR                 bool
}

func (m MouseMode) ReportsMouse() bool {
	return m.NormalTracking || m.ButtonEventTracking || m.AnyEventTracking
}

func (t *Terminal) ResetAttr()                      { t.attr = Attr{FG: DefaultColor(), BG: DefaultColor()} }
func (t *Terminal) SetBold(v bool)                  { t.attr.Bold = v }
func (t *Terminal) SetDim(v bool)                   { t.attr.Dim = v }
func (t *Terminal) SetItalic(v bool)                { t.attr.Italic = v }
func (t *Terminal) SetUnderline(v bool)             { t.attr.Underline = v }
func (t *Terminal) SetInverse(v bool)               { t.attr.Inverse = v }
func (t *Terminal) SetStrikethrough(v bool)         { t.attr.Strikethrough = v }
func (t *Terminal) SetBlink(v bool)                 { t.attr.Blink = v }
func (t *Terminal) SetFG(c LogicalColor)            { t.attr.FG = c }
func (t *Terminal) SetBG(c LogicalColor)            { t.attr.BG = c }
func (t *Terminal) BracketedPasteMode() bool        { return t.bracketedPaste }
func (t *Terminal) SetBracketedPasteMode(v bool)    { t.bracketedPaste = v }
func (t *Terminal) AlternateScreenMode() bool       { return t.alternateScreen }
func (t *Terminal) CursorVisible() bool             { return t.cursorVisible }
func (t *Terminal) SetCursorVisible(v bool)         { t.cursorVisible = v }
func (t *Terminal) AutoWrapMode() bool              { return t.autoWrap }
func (t *Terminal) ApplicationCursorMode() bool     { return t.applicationCursor }
func (t *Terminal) SetApplicationCursorMode(v bool) { t.applicationCursor = v }

// ApplicationKeypadMode / SetApplicationKeypadMode hold the DECKPAM/DECKPNM
// keypad mode: the parser (ESC = / ESC >) sets it and it is stored here, but the
// input encoder does not yet consume it. input.Key models no numeric-keypad keys
// (there is no KeyKP0..9, KeyKPEnter, etc.), so there is nothing whose encoding
// this flag could alter. Wiring it up requires first adding those keys — a new
// feature — after which the encoder would branch on this mode. Until then this is
// parsed-and-stored state with no consumer.
func (t *Terminal) ApplicationKeypadMode() bool     { return t.applicationKeypad }
func (t *Terminal) SetApplicationKeypadMode(v bool) { t.applicationKeypad = v }

func (t *Terminal) SetAutoWrapMode(v bool) {
	t.autoWrap = v
	if !v {
		t.wrapNext = false
	}
}

func (t *Terminal) MouseMode() MouseMode { return t.mouseMode }

func (t *Terminal) SetMouseMode(code int, enabled bool) {
	switch code {
	case 1000:
		t.mouseMode.NormalTracking = enabled
	case 1002:
		t.mouseMode.ButtonEventTracking = enabled
	case 1003:
		t.mouseMode.AnyEventTracking = enabled
	case 1004:
		t.SetFocusEventsMode(enabled)
	case 1006:
		t.mouseMode.SGR = enabled
	}
}
