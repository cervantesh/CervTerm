package vt

import (
	"encoding/base64"
	"fmt"
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

func (p *Parser) selectOSC1337() bool {
	if len(p.osc) != 5 || p.osc[0] != '1' || p.osc[1] != '3' || p.osc[2] != '3' || p.osc[3] != '7' || p.osc[4] != ';' {
		return false
	}
	p.resetOSC()
	p.startControlString(ControlStringOSC1337)
	return true
}

func (p *Parser) dispatchOSC(t *core.Terminal) {
	if p.oscTruncated {
		// A truncated string must be dropped whole, never half-decoded.
		return
	}
	payload := string(p.osc)
	code, rest, ok := strings.Cut(payload, ";")
	if !ok {
		switch payload {
		case "104", "110", "111":
			code, rest = payload, ""
		default:
			return
		}
	}
	switch code {
	case "0", "2":
		t.SetTitle(rest)
	case "4":
		p.dispatchOSC4(t, rest)
	case "7":
		if cwd, ok := parseOSC7Cwd(rest); ok {
			t.SetCwd(cwd)
		}
	case "8":
		p.dispatchOSC8(t, rest)
	case "9":
		p.dispatchOSCNotification(t, "", rest)
	case "133", "633":
		p.dispatchOSCSemantic(t, code, rest)
	case "10":
		p.dispatchOSCDefaultColor(t, rest, true)
	case "11":
		p.dispatchOSCDefaultColor(t, rest, false)
	case "52":
		p.dispatchOSC52(rest)
	case "104":
		dispatchOSC104(t, rest)
	case "110":
		if rest == "" {
			t.ResetPaletteFG()
		}
	case "111":
		if rest == "" {
			t.ResetPaletteBG()
		}
	case "777":
		command, remainder, found := strings.Cut(rest, ";")
		if !found || command != "notify" {
			return
		}
		title, body, found := strings.Cut(remainder, ";")
		if !found {
			return
		}
		p.dispatchOSCNotification(t, title, body)
	}
}

func (p *Parser) dispatchOSC8(t *core.Terminal, rest string) {
	params, uri, ok := strings.Cut(rest, ";")
	if !ok {
		return
	}
	if uri == "" {
		t.CloseHyperlink()
		return
	}
	if len(params) > core.MaxHyperlinkParamsBytes || len(uri) > core.MaxHyperlinkURIBytes || !validOSC8Text(uri) {
		return
	}
	// This slice stores metadata only. Scheme allowlisting belongs to the explicit
	// user-activation policy at the frontend side-effect boundary.
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme == "" {
		return
	}
	explicitID := ""
	if params != "" {
		for _, part := range strings.Split(params, ":") {
			key, value, found := strings.Cut(part, "=")
			if !found || key == "" || value == "" || !validOSC8Text(key) || !validOSC8Text(value) {
				return
			}
			if key == "id" {
				if explicitID != "" {
					return
				}
				explicitID = value
			}
		}
	}
	t.OpenHyperlink(uri, explicitID)
}

func validOSC8Text(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func (p *Parser) dispatchOSCNotification(t *core.Terminal, title, body string) {
	if len(title) > core.MaxNotificationTitleBytes || len(body) > core.MaxNotificationBodyBytes || body == "" {
		return
	}
	if !validOSC8Text(title) || !validOSC8Text(body) {
		return
	}
	t.RequestNotification(title, body)
}

const maxSemanticOSCBytes = 1024

func (p *Parser) dispatchOSCSemantic(t *core.Terminal, code, rest string) {
	if rest == "" || len(rest) > maxSemanticOSCBytes || !validOSC8Text(rest) {
		return
	}
	marker, _, _ := strings.Cut(rest, ";")
	if len(marker) != 1 {
		return
	}
	var kind core.SemanticKind
	switch marker {
	case "A":
		kind = core.SemanticPrompt
	case "B":
		kind = core.SemanticInput
	case "C":
		kind = core.SemanticOutput
	case "D":
		kind = core.SemanticNone
	case "E":
		if code != "633" {
			return
		}
		kind = core.SemanticInput
	default:
		return
	}
	t.SetSemanticKind(kind)
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

type oscPaletteOp struct {
	index uint8
	color core.RGB
	query bool
}

func (p *Parser) dispatchOSC4(t *core.Terminal, payload string) {
	fields := strings.Split(payload, ";")
	if len(fields) < 2 || len(fields)%2 != 0 || len(fields)/2 > 256 {
		return
	}

	var ops [256]oscPaletteOp
	count := len(fields) / 2
	for i := 0; i < count; i++ {
		index, ok := parsePaletteIndex(fields[i*2])
		if !ok {
			return
		}
		spec := fields[i*2+1]
		ops[i].index = index
		if spec == "?" {
			ops[i].query = true
			continue
		}
		color, ok := parseOSCColor(spec)
		if !ok {
			return
		}
		ops[i].color = color
	}

	for i := 0; i < count; i++ {
		op := ops[i]
		if op.query {
			p.reply(fmt.Sprintf("\x1b]4;%d;%s\x1b\\", op.index, formatOSCColor(t.EffectivePaletteIndex(op.index))))
			continue
		}
		t.SetPaletteIndex(op.index, op.color)
	}
}

func (p *Parser) dispatchOSCDefaultColor(t *core.Terminal, payload string, foreground bool) {
	if payload == "?" {
		color := t.EffectivePaletteBG()
		code := 11
		if foreground {
			color = t.EffectivePaletteFG()
			code = 10
		}
		p.reply(fmt.Sprintf("\x1b]%d;%s\x1b\\", code, formatOSCColor(color)))
		return
	}

	color, ok := parseOSCColor(payload)
	if !ok {
		return
	}
	if foreground {
		t.SetPaletteFG(color)
	} else {
		t.SetPaletteBG(color)
	}
}

func dispatchOSC104(t *core.Terminal, payload string) {
	if payload == "" {
		t.ResetPaletteIndexes()
		return
	}
	fields := strings.Split(payload, ";")
	if len(fields) > 256 {
		return
	}
	var indexes [256]uint8
	for i, field := range fields {
		index, ok := parsePaletteIndex(field)
		if !ok {
			return
		}
		indexes[i] = index
	}
	for i := range fields {
		t.ResetPaletteIndex(indexes[i])
	}
}

func parsePaletteIndex(value string) (uint8, bool) {
	if value == "" || len(value) > 3 {
		return 0, false
	}
	index := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
		index = index*10 + int(value[i]-'0')
	}
	if index > 255 {
		return 0, false
	}
	return uint8(index), true
}

func parseOSCColor(spec string) (core.RGB, bool) {
	if len(spec) == 7 && spec[0] == '#' {
		r, okR := parseHexComponent(spec[1:3])
		g, okG := parseHexComponent(spec[3:5])
		b, okB := parseHexComponent(spec[5:7])
		if !okR || !okG || !okB {
			return core.RGB{}, false
		}
		return core.RGB{R: uint8(r), G: uint8(g), B: uint8(b)}, true
	}
	if !strings.HasPrefix(spec, "rgb:") {
		return core.RGB{}, false
	}
	components := strings.Split(spec[4:], "/")
	if len(components) != 3 {
		return core.RGB{}, false
	}
	var bytes [3]uint8
	for i, component := range components {
		if len(component) < 1 || len(component) > 4 {
			return core.RGB{}, false
		}
		value, ok := parseHexComponent(component)
		if !ok {
			return core.RGB{}, false
		}
		maximum := uint32(1)<<(4*len(component)) - 1
		bytes[i] = uint8((value*255 + maximum/2) / maximum)
	}
	return core.RGB{R: bytes[0], G: bytes[1], B: bytes[2]}, true
}

func parseHexComponent(value string) (uint32, bool) {
	var result uint32
	for i := 0; i < len(value); i++ {
		var digit byte
		switch {
		case value[i] >= '0' && value[i] <= '9':
			digit = value[i] - '0'
		case value[i] >= 'a' && value[i] <= 'f':
			digit = value[i] - 'a' + 10
		case value[i] >= 'A' && value[i] <= 'F':
			digit = value[i] - 'A' + 10
		default:
			return 0, false
		}
		result = result*16 + uint32(digit)
	}
	return result, true
}

func formatOSCColor(color core.RGB) string {
	return fmt.Sprintf("rgb:%04X/%04X/%04X", uint16(color.R)*0x101, uint16(color.G)*0x101, uint16(color.B)*0x101)
}
