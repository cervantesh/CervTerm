package core

type MouseMode struct {
	NormalTracking      bool
	ButtonEventTracking bool
	SGR                 bool
}

func (m MouseMode) ReportsMouse() bool {
	return m.NormalTracking || m.ButtonEventTracking
}

func (t *Terminal) ResetAttr()                      { t.attr = Attr{FG: DefaultFG, BG: DefaultBG} }
func (t *Terminal) SetBold(v bool)                  { t.attr.Bold = v }
func (t *Terminal) SetFG(c RGB)                     { t.attr.FG = c }
func (t *Terminal) SetBG(c RGB)                     { t.attr.BG = c }
func (t *Terminal) BracketedPasteMode() bool        { return t.bracketedPaste }
func (t *Terminal) SetBracketedPasteMode(v bool)    { t.bracketedPaste = v }
func (t *Terminal) AlternateScreenMode() bool       { return t.alternateScreen }
func (t *Terminal) CursorVisible() bool             { return t.cursorVisible }
func (t *Terminal) SetCursorVisible(v bool)         { t.cursorVisible = v }
func (t *Terminal) AutoWrapMode() bool              { return t.autoWrap }
func (t *Terminal) ApplicationCursorMode() bool     { return t.applicationCursor }
func (t *Terminal) SetApplicationCursorMode(v bool) { t.applicationCursor = v }
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
	case 1006:
		t.mouseMode.SGR = enabled
	}
}
