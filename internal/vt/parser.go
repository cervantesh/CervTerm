package vt

import (
	"unicode/utf8"

	"cervterm/internal/core"
)

type Parser struct {
	state      byte
	params     [16]int
	paramCount int
	cur        int
	hasCur     bool
	csiPrivate bool

	osc    [512]byte
	oscLen int

	utf8Buf [utf8.UTFMax]byte
	utf8Len int
}

const (
	stateGround byte = iota
	stateEsc
	stateCSI
	stateOSC
	stateOSCEsc
)

func (p *Parser) Advance(t *core.Terminal, data []byte) {
	for len(data) > 0 {
		if p.utf8Len > 0 {
			p.appendPendingUTF8(t, data[0])
			data = data[1:]
			continue
		}

		b := data[0]
		if p.state == stateGround && b >= 0x20 && b != 0x7f {
			if b < utf8.RuneSelf {
				t.PutRune(rune(b))
				data = data[1:]
				continue
			}
			if !utf8.FullRune(data) {
				p.utf8Len = copy(p.utf8Buf[:], data)
				return
			}
			r, n := utf8.DecodeRune(data)
			if r != utf8.RuneError || n > 1 {
				t.PutRune(r)
			}
			data = data[n:]
			continue
		}
		p.advanceByte(t, b)
		data = data[1:]
	}
}

func (p *Parser) appendPendingUTF8(t *core.Terminal, b byte) {
	if p.utf8Len >= len(p.utf8Buf) {
		p.utf8Len = 0
		return
	}
	p.utf8Buf[p.utf8Len] = b
	p.utf8Len++

	buf := p.utf8Buf[:p.utf8Len]
	if !utf8.FullRune(buf) {
		return
	}
	r, n := utf8.DecodeRune(buf)
	if r != utf8.RuneError || n > 1 {
		t.PutRune(r)
	}
	p.utf8Len = 0
}

func (p *Parser) advanceByte(t *core.Terminal, b byte) {
	switch p.state {
	case stateGround:
		switch b {
		case 0x1b:
			p.state = stateEsc
		case '\r':
			t.CarriageReturn()
		case '\n':
			t.NewLine()
		case '\b':
			t.Backspace()
		case '\t':
			t.Tab()
		}
	case stateEsc:
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
		switch b {
		case '7':
			t.SaveCursor()
		case '8':
			t.RestoreCursor()
		case '=':
			t.SetApplicationKeypadMode(true)
		case '>':
			t.SetApplicationKeypadMode(false)
		case 'c':
			t.Reset()
		}
		p.state = stateGround
	case stateCSI:
		if b >= '0' && b <= '9' {
			p.cur = p.cur*10 + int(b-'0')
			p.hasCur = true
			return
		}
		if b == '?' && p.paramCount == 0 && !p.hasCur {
			p.csiPrivate = true
			return
		}
		if b == ';' {
			p.pushParam()
			return
		}
		p.pushParam()
		p.dispatchCSI(t, b)
		p.state = stateGround
	case stateOSC:
		if b == 0x07 {
			p.dispatchOSC(t)
			p.state = stateGround
			return
		}
		if b == 0x1b {
			p.state = stateOSCEsc
			return
		}
		p.appendOSC(b)
	case stateOSCEsc:
		if b == '\\' {
			p.dispatchOSC(t)
			p.state = stateGround
			return
		}
		p.appendOSC(0x1b)
		p.appendOSC(b)
		p.state = stateOSC
	}
}

func (p *Parser) resetCSI() {
	p.paramCount = 0
	p.cur = 0
	p.hasCur = false
	p.csiPrivate = false
	for i := range p.params {
		p.params[i] = 0
	}
}

func (p *Parser) resetOSC() {
	p.oscLen = 0
}

func (p *Parser) appendOSC(b byte) {
	if p.oscLen >= len(p.osc) {
		return
	}
	p.osc[p.oscLen] = b
	p.oscLen++
}

func (p *Parser) dispatchOSC(t *core.Terminal) {
	if p.oscLen < 2 || p.osc[1] != ';' {
		return
	}
	if p.osc[0] != '0' && p.osc[0] != '2' {
		return
	}
	t.SetTitle(string(p.osc[2:p.oscLen]))
}

func (p *Parser) pushParam() {
	if p.paramCount >= len(p.params) {
		p.cur = 0
		p.hasCur = false
		return
	}
	if p.hasCur {
		p.params[p.paramCount] = p.cur
	} else {
		p.params[p.paramCount] = 0
	}
	p.paramCount++
	p.cur = 0
	p.hasCur = false
}

func (p *Parser) param(i, fallback int) int {
	if i >= p.paramCount || p.params[i] == 0 {
		return fallback
	}
	return p.params[i]
}

func (p *Parser) dispatchCSI(t *core.Terminal, action byte) {
	switch action {
	case '@':
		t.InsertChars(p.param(0, 1))
	case 'A':
		t.MoveCursor(-p.param(0, 1), 0)
	case 'B':
		t.MoveCursor(p.param(0, 1), 0)
	case 'C':
		t.MoveCursor(0, p.param(0, 1))
	case 'D':
		t.MoveCursor(0, -p.param(0, 1))
	case 'E':
		t.SetCursor(t.CursorRow()+p.param(0, 1), 0)
	case 'F':
		t.SetCursor(t.CursorRow()-p.param(0, 1), 0)
	case 'G':
		t.SetCursor(t.CursorRow(), p.param(0, 1)-1)
	case 'H', 'f':
		t.SetCursor(p.param(0, 1)-1, p.param(1, 1)-1)
	case 'J':
		switch p.param(0, 0) {
		case 0:
			t.ClearToEndOfScreen()
		case 1:
			t.ClearToBeginningOfScreen()
		case 2:
			t.Clear()
		case 3:
			t.ClearScrollback()
		}
	case 'K':
		switch p.param(0, 0) {
		case 0:
			t.ClearToEndOfLine()
		case 1:
			t.ClearToBeginningOfLine()
		case 2:
			t.ClearLine(t.CursorRow())
		}
	case 'L':
		t.InsertLines(p.param(0, 1))
	case 'M':
		t.DeleteLines(p.param(0, 1))
	case 'P':
		t.DeleteChars(p.param(0, 1))
	case 'S':
		t.ScrollUp(p.param(0, 1))
	case 'T':
		t.ScrollDown(p.param(0, 1))
	case 'd':
		t.SetCursor(p.param(0, 1)-1, t.CursorCol())
	case 'h', 'l':
		p.dispatchMode(t, action == 'h')
	case 'm':
		p.dispatchSGR(t)
	case 'r':
		if p.paramCount == 0 {
			t.ResetScrollRegion()
			return
		}
		t.SetScrollRegion(p.param(0, 1)-1, p.param(1, t.Rows())-1)
	case 's':
		t.SaveCursor()
	case 'u':
		t.RestoreCursor()
	}
}

func (p *Parser) dispatchMode(t *core.Terminal, enabled bool) {
	if !p.csiPrivate {
		return
	}
	for i := 0; i < p.paramCount; i++ {
		switch p.params[i] {
		case 1:
			t.SetApplicationCursorMode(enabled)
		case 7:
			t.SetAutoWrapMode(enabled)
		case 25:
			t.SetCursorVisible(enabled)
		case 1000, 1002, 1006:
			t.SetMouseMode(p.params[i], enabled)
		case 2004:
			t.SetBracketedPasteMode(enabled)
		case 1049:
			t.SetAlternateScreenMode(enabled)
		}
	}
}

func (p *Parser) dispatchSGR(t *core.Terminal) {
	if p.paramCount == 0 {
		t.ResetAttr()
		return
	}
	for i := 0; i < p.paramCount; i++ {
		v := p.params[i]
		switch {
		case v == 0:
			t.ResetAttr()
		case v == 1:
			t.SetBold(true)
		case v == 22:
			t.SetBold(false)
		case v >= 30 && v <= 37:
			t.SetFG(core.ANSIColor(v - 30))
		case v == 38:
			consumed := p.dispatchExtendedColor(t, true, i)
			i += consumed
		case v == 39:
			t.SetFG(core.DefaultFG)
		case v >= 40 && v <= 47:
			t.SetBG(core.ANSIColor(v - 40))
		case v == 48:
			consumed := p.dispatchExtendedColor(t, false, i)
			i += consumed
		case v == 49:
			t.SetBG(core.DefaultBG)
		case v >= 90 && v <= 97:
			t.SetFG(core.ANSIColor(8 + v - 90))
		case v >= 100 && v <= 107:
			t.SetBG(core.ANSIColor(8 + v - 100))
		}
	}
}

func (p *Parser) dispatchExtendedColor(t *core.Terminal, foreground bool, start int) int {
	if start+1 >= p.paramCount {
		return 0
	}
	mode := p.params[start+1]
	switch mode {
	case 5:
		if start+2 >= p.paramCount {
			return 1
		}
		color := core.ANSI256Color(p.params[start+2])
		if foreground {
			t.SetFG(color)
		} else {
			t.SetBG(color)
		}
		return 2
	case 2:
		if start+4 >= p.paramCount {
			return 1
		}
		color := core.RGB{R: sgrByte(p.params[start+2]), G: sgrByte(p.params[start+3]), B: sgrByte(p.params[start+4])}
		if foreground {
			t.SetFG(color)
		} else {
			t.SetBG(color)
		}
		return 4
	default:
		return 0
	}
}

func sgrByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
