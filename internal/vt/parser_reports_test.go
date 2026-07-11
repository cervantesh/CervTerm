package vt

import (
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestParserDeviceReports(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"da1 default", "\x1b[c", "\x1b[?62;22c"},
		{"da1 zero", "\x1b[0c", "\x1b[?62;22c"},
		{"da2 default", "\x1b[>c", "\x1b[>1;10;0c"},
		{"da2 zero", "\x1b[>0c", "\x1b[>1;10;0c"},
		{"dsr status", "\x1b[5n", "\x1b[0n"},
		{"decxcpr", "\x1b[?6n", "\x1b[?1;1R"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := core.NewTerminal(8, 4)
			var got strings.Builder
			parser := Parser{Reply: func(b []byte) { got.Write(b) }}
			parser.Advance(term, []byte(tt.in))
			if got.String() != tt.want {
				t.Fatalf("reply = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestParserCPRReflectsCursor(t *testing.T) {
	term := core.NewTerminal(8, 4)
	var got strings.Builder
	parser := Parser{Reply: func(b []byte) { got.Write(b) }}
	parser.Advance(term, []byte("\x1b[3;4H\x1b[6n"))
	if got.String() != "\x1b[3;4R" {
		t.Fatalf("reply = %q", got.String())
	}
}

func TestParserReplyNilAndUnknownDSRIgnored(t *testing.T) {
	term := core.NewTerminal(8, 4)
	var parser Parser
	parser.Advance(term, []byte("\x1b[c"))
	parser.Reply = func(b []byte) { t.Fatalf("unexpected reply %q", string(b)) }
	parser.Advance(term, []byte("\x1b[9n"))
}
