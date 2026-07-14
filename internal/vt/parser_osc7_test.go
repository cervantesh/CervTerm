package vt

import (
	"testing"

	"cervterm/internal/core"
)

func TestParserOSC7WorkingDirectory(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		term    string
		want    string
	}{
		{name: "BEL terminator", payload: "file:///home/alice", term: "\x07", want: "/home/alice"},
		{name: "ST terminator", payload: "file:///srv/project", term: "\x1b\\", want: "/srv/project"},
		{name: "percent encoded space", payload: "file:///home/my%20project", term: "\x07", want: "/home/my project"},
		{name: "percent encoded UTF-8", payload: "file:///home/caf%C3%A9", term: "\x07", want: "/home/café"},
		{name: "Windows drive path", payload: "file:///C:/Users/alice/work", term: "\x07", want: `C:\Users\alice\work`},
		{name: "UNC host", payload: "file://server/share/project", term: "\x07", want: `\\server\share\project`},
		{name: "invalid URI ignored", payload: "file:///bad%ZZpath", term: "\x07", want: "/previous"},
		{name: "empty payload ignored", payload: "", term: "\x07", want: "/previous"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := core.NewTerminal(8, 1)
			term.SetCwd("/previous")
			var p Parser
			p.Advance(term, []byte("\x1b]7;"+tt.payload+tt.term))
			if got := term.Cwd(); got != tt.want {
				t.Fatalf("cwd = %q, want %q", got, tt.want)
			}
		})
	}
}
