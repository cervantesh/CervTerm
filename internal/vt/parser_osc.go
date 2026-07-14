package vt

import (
	"encoding/base64"
	"net/url"
	"strings"
	"unicode/utf8"

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
		if cwd, ok := parseOSC7Cwd(rest); ok {
			t.SetCwd(cwd)
		}
	case "52":
		p.dispatchOSC52(rest)
	}
}

func parseOSC7Cwd(payload string) (string, bool) {
	if payload == "" {
		return "", false
	}
	u, err := url.Parse(payload)
	if err != nil || !strings.EqualFold(u.Scheme, "file") || u.Opaque != "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return "", false
	}
	path := u.Path // net/url percent-decodes Path while parsing.
	if path == "" || path[0] != '/' || !utf8.ValidString(path) {
		return "", false
	}

	host := u.Hostname()
	if host != "" && !strings.EqualFold(host, "localhost") {
		return `\\` + host + `\` + strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", `\`), true
	}
	if len(path) >= 3 && path[0] == '/' && isASCIIAlpha(path[1]) && path[2] == ':' {
		return strings.ReplaceAll(path[1:], "/", `\`), true
	}
	return path, true
}

func isASCIIAlpha(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
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
