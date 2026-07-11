package vt

import "cervterm/internal/core"

func (p *Parser) dispatchESC(t *core.Terminal, b byte) {
	if b == '[' {
		p.resetCSI()
		p.state = stateCSI
		return
	}
	if b == ']' {
		p.resetOSC()
		p.state = stateOSC
		return
	}
	if b == '(' {
		p.state = stateEscG0
		return
	}
	if b == ')' {
		p.state = stateEscG1
		return
	}
	switch b {
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
		t.Reset()
	}
	p.state = stateGround
}

func (p *Parser) designateCharset(t *core.Terminal, slot int, b byte) {
	switch b {
	case 'B':
		t.DesignateCharset(slot, core.CharsetASCII)
	case '0':
		t.DesignateCharset(slot, core.CharsetDECSpecial)
	}
}
