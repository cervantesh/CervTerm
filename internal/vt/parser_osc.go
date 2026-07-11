package vt

import (
	"encoding/base64"
	"strings"

	"cervterm/internal/core"
)

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
	payload := string(p.osc[:p.oscLen])
	code, rest, ok := strings.Cut(payload, ";")
	if !ok {
		return
	}
	switch code {
	case "0", "2":
		t.SetTitle(rest)
	case "7":
		t.SetWorkingDirectoryURL(rest)
	case "52":
		p.dispatchOSC52(rest)
	}
}

func (p *Parser) dispatchOSC52(rest string) {
	_, data, ok := strings.Cut(rest, ";")
	if !ok || data == "?" || p.SetClipboard == nil {
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return
	}
	p.SetClipboard(string(decoded))
}
