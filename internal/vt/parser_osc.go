package vt

import (
	"encoding/base64"
	"strings"

	"cervterm/internal/core"
)

// maxOSCLen bounds OSC accumulation; OSC 52 clipboard payloads are routinely
// multiple kilobytes of base64, so the cap must be generous.
const maxOSCLen = 64 * 1024

func (p *Parser) resetOSC() {
	p.osc = p.osc[:0]
	p.oscTruncated = false
}

func (p *Parser) appendOSC(b byte) {
	if len(p.osc) >= maxOSCLen {
		p.oscTruncated = true
		return
	}
	p.osc = append(p.osc, b)
}

func (p *Parser) dispatchOSC(t *core.Terminal) {
	if p.oscTruncated {
		// A truncated string must be dropped whole, never half-decoded.
		return
	}
	payload := string(p.osc)
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
