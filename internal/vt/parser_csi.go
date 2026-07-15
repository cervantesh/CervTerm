package vt

import (
	"fmt"

	"cervterm/internal/core"
)

func (p *Parser) dispatchCSI(t *core.Terminal, action byte) {
	// `>`-prefixed sequences other than DA2 (e.g. XTMODKEYS `CSI > 4;2 m`)
	// must not fall through to the plain dispatch table.
	if p.csiGT && action != 'c' {
		return
	}
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
		t.SetCursor(t.CursorRow()+p.param(0, 1)-originRowOffset(t), 0)
	case 'F':
		t.SetCursor(t.CursorRow()-p.param(0, 1)-originRowOffset(t), 0)
	case 'G':
		t.SetCursor(t.CursorRow(), p.param(0, 1)-1)
	case 'H', 'f':
		t.SetCursor(p.param(0, 1)-1, p.param(1, 1)-1)
	case 'J':
		p.dispatchEraseDisplay(t)
	case 'K':
		p.dispatchEraseLine(t)
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
	case 'I':
		t.CursorForwardTabs(p.param(0, 1))
	case 'Z':
		t.CursorBackwardTabs(p.param(0, 1))
	case 'c':
		p.dispatchDeviceAttributes()
	case 'd':
		t.SetCursor(p.param(0, 1)-1, t.CursorCol())
	case 'h', 'l':
		p.dispatchMode(t, action == 'h')
	case 'm':
		p.dispatchSGR(t)
	case 'n':
		p.dispatchDSR(t)
	case 'g':
		p.dispatchTBC(t)
	case 'q':
		if p.csiInter == ' ' {
			t.SetCursorStyle(core.CursorStyle(p.param(0, 0)))
		}
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

func originRowOffset(t *core.Terminal) int {
	if !t.OriginMode() {
		return 0
	}
	top, _ := t.ScrollRegion()
	return top
}

func (p *Parser) dispatchEraseDisplay(t *core.Terminal) {
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
}

func (p *Parser) dispatchEraseLine(t *core.Terminal) {
	switch p.param(0, 0) {
	case 0:
		t.ClearToEndOfLine()
	case 1:
		t.ClearToBeginningOfLine()
	case 2:
		t.ClearLine(t.CursorRow())
	}
}

func (p *Parser) dispatchDeviceAttributes() {
	if p.csiGT {
		p.reply("\x1b[>1;10;0c")
		return
	}
	if !p.csiPrivate {
		p.reply("\x1b[?62;22c")
	}
}

func (p *Parser) dispatchDSR(t *core.Terminal) {
	switch p.param(0, 0) {
	case 5:
		if !p.csiPrivate {
			p.reply("\x1b[0n")
		}
	case 6:
		row, col := t.CursorReport()
		prefix := ""
		if p.csiPrivate {
			prefix = "?"
		}
		p.reply(fmt.Sprintf("\x1b[%s%d;%dR", prefix, row, col))
	}
}

func (p *Parser) dispatchTBC(t *core.Terminal) {
	switch p.param(0, 0) {
	case 0:
		t.ClearTabStop()
	case 3:
		t.ClearAllTabStops()
	}
}

func (p *Parser) reply(s string) {
	if p.Reply != nil {
		p.Reply([]byte(s))
	}
}

func (p *Parser) dispatchMode(t *core.Terminal, enabled bool) {
	if !p.csiPrivate {
		for i := 0; i < p.paramCount; i++ {
			if p.params[i] == 4 {
				t.SetInsertMode(enabled)
			}
		}
		return
	}
	for i := 0; i < p.paramCount; i++ {
		switch p.params[i] {
		case 1:
			t.SetApplicationCursorMode(enabled)
		case 6:
			t.SetOriginMode(enabled)
		case 7:
			t.SetAutoWrapMode(enabled)
		case 25:
			t.SetCursorVisible(enabled)
		case 47:
			t.SetAlternateScreenModeWithOptions(enabled, false, true, false)
		case 1000, 1002, 1003, 1004, 1006:
			t.SetMouseMode(p.params[i], enabled)
		case 2004:
			t.SetBracketedPasteMode(enabled)
		case 1047:
			t.SetAlternateScreenModeWithOptions(enabled, false, true, true)
		case 1048:
			if enabled {
				t.SaveCursor()
			} else {
				t.RestoreCursor()
			}
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
		case v == 2:
			t.SetDim(true)
		case v == 3:
			t.SetItalic(true)
		case v == 4:
			t.SetUnderline(true)
		case v == 5:
			t.SetBlink(true)
		case v == 7:
			t.SetInverse(true)
		case v == 9:
			t.SetStrikethrough(true)
		case v == 22:
			t.SetBold(false)
			t.SetDim(false)
		case v == 23:
			t.SetItalic(false)
		case v == 24:
			t.SetUnderline(false)
		case v == 25:
			t.SetBlink(false)
		case v == 27:
			t.SetInverse(false)
		case v == 29:
			t.SetStrikethrough(false)
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
