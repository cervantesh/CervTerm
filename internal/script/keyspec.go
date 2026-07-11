package script

import (
	"fmt"
	"strings"
)

type Mod uint8

const (
	ModCtrl Mod = 1 << iota
	ModAlt
	ModShift
	ModSuper
)

type Spec struct {
	Key  string
	Mods Mod
}

var validKeys = map[string]struct{}{}

func init() {
	for ch := 'a'; ch <= 'z'; ch++ {
		validKeys[string(ch)] = struct{}{}
	}
	for ch := '0'; ch <= '9'; ch++ {
		validKeys[string(ch)] = struct{}{}
	}
	for i := 1; i <= 12; i++ {
		validKeys[fmt.Sprintf("f%d", i)] = struct{}{}
	}
	for _, key := range []string{
		"enter", "tab", "escape", "space", "backspace", "delete", "insert",
		"home", "end", "pageup", "pagedown", "up", "down", "left", "right",
		"minus", "equal", "comma", "period", "slash", "backslash",
		"semicolon", "apostrophe", "grave",
	} {
		validKeys[key] = struct{}{}
	}
}

func ParseSpec(key, mods string) (Spec, error) {
	name := strings.ToLower(strings.TrimSpace(key))
	if _, ok := validKeys[name]; !ok {
		return Spec{}, fmt.Errorf("unknown key %q", key)
	}
	spec := Spec{Key: name}
	for _, token := range strings.Split(mods, "+") {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		switch token {
		case "ctrl":
			spec.Mods |= ModCtrl
		case "alt":
			spec.Mods |= ModAlt
		case "shift":
			spec.Mods |= ModShift
		case "super", "cmd", "win":
			spec.Mods |= ModSuper
		default:
			return Spec{}, fmt.Errorf("unknown mod %q", token)
		}
	}
	return spec, nil
}

func (s Spec) String() string {
	var parts []string
	if s.Mods&ModCtrl != 0 {
		parts = append(parts, "ctrl")
	}
	if s.Mods&ModAlt != 0 {
		parts = append(parts, "alt")
	}
	if s.Mods&ModShift != 0 {
		parts = append(parts, "shift")
	}
	if s.Mods&ModSuper != 0 {
		parts = append(parts, "super")
	}
	parts = append(parts, s.Key)
	return strings.Join(parts, "+")
}
