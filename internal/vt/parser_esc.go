package vt

import "cervterm/internal/core"

func (p *Parser) dispatchESC(t *core.Terminal, b byte) {
	p.state = stateGround
	switch b {
	case 0x1b:
		// ESC restarts an escape sequence; repeated introducers must not return
		// to ground and expose a following control-string payload as text.
		p.state = stateEsc
	case '[':
		p.resetCSI()
		p.state = stateCSI
	case ']':
		p.resetOSC()
		p.state = stateOSC
	case '_':
		p.startControlString(ControlStringAPC)
	case 'P':
		p.startControlString(ControlStringDCS)
	case '(':
		p.state = stateEscG0
	case ')':
		p.state = stateEscG1
	case '7':
		t.SaveCursor()
	case '8':
		t.RestoreCursor()
	case 'D':
		t.Index()
	case 'E':
		t.NextLine()
	case 'H':
		t.SetTabStop()
	case 'M':
		t.ReverseIndex()
	case '=':
		t.SetApplicationKeypadMode(true)
	case '>':
		t.SetApplicationKeypadMode(false)
	case 'c':
		p.Reset()
		t.Reset()
	}
}

func (p *Parser) designateCharset(t *core.Terminal, slot int, b byte) {
	switch b {
	case 'B':
		t.DesignateCharset(slot, core.CharsetASCII)
	case '0':
		t.DesignateCharset(slot, core.CharsetDECSpecial)
	}
}
