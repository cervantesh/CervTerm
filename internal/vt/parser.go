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
	csiGT      bool
	csiInter   byte

	osc          []byte
	oscTruncated bool

	utf8Buf [utf8.UTFMax]byte
	utf8Len int

	// Reply, when set, receives bytes to send back to the application.
	Reply func([]byte)
	// SetClipboard, when set, receives OSC 52 clipboard writes.
	SetClipboard func(string)
}

const (
	stateGround byte = iota
	stateEsc
	stateCSI
	stateOSC
	stateOSCEsc
	stateEscG0
	stateEscG1
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
		case 0x0e:
			t.SelectCharset(1)
		case 0x0f:
			t.SelectCharset(0)
		case '\r':
			t.CarriageReturn()
		case '\n':
			t.NewLine()
		case '\b':
			t.Backspace()
		case '\t':
			t.Tab()
		case 0x07:
			t.Bell()
		}
	case stateEsc:
		p.dispatchESC(t, b)
	case stateCSI:
		if b >= 0x20 && b <= 0x2f {
			p.csiInter = b
			return
		}
		if b >= '0' && b <= '9' {
			p.cur = p.cur*10 + int(b-'0')
			p.hasCur = true
			return
		}
		if b == '?' && p.paramCount == 0 && !p.hasCur {
			p.csiPrivate = true
			return
		}
		if b == '>' && p.paramCount == 0 && !p.hasCur {
			p.csiGT = true
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
	case stateEscG0:
		p.designateCharset(t, 0, b)
		p.state = stateGround
	case stateEscG1:
		p.designateCharset(t, 1, b)
		p.state = stateGround
	}
}

func (p *Parser) resetCSI() {
	p.paramCount = 0
	p.cur = 0
	p.hasCur = false
	p.csiPrivate = false
	p.csiGT = false
	p.csiInter = 0
	for i := range p.params {
		p.params[i] = 0
	}
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
