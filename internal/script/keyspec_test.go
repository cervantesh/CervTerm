package script

import (
	"strings"
	"testing"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name string
		key  string
		mods string
		want string
	}{
		{name: "plain letter", key: "P", want: "p"},
		{name: "ordered canonical mods", key: "p", mods: "shift+ctrl", want: "ctrl+shift+p"},
		{name: "aliases", key: "enter", mods: "cmd+win", want: "super+enter"},
		{name: "all mods", key: "f12", mods: "super+alt+shift+ctrl", want: "ctrl+alt+shift+super+f12"},
		{name: "named key", key: "PageDown", mods: "Alt", want: "alt+pagedown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSpec(tt.key, tt.mods)
			if err != nil {
				t.Fatalf("ParseSpec failed: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("String() = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestParseSpecErrors(t *testing.T) {
	tests := []struct {
		name string
		key  string
		mods string
		want string
	}{
		{name: "unknown key", key: "printscreen", want: "printscreen"},
		{name: "unknown mod", key: "p", mods: "hyper", want: "hyper"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSpec(tt.key, tt.mods)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}
