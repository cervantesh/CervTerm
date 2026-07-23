package vt

import (
	"testing"

	"cervterm/internal/core"
)

func BenchmarkPhase13TextOnlyParser(b *testing.B) {
	payload := []byte("\x1b[32mhello world\x1b[0m\r\n")
	term := core.NewTerminal(120, 40)
	var parser Parser
	phase13PrewarmScrollback(term)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Advance(term, payload)
	}
}

func BenchmarkPhase13TextOnlyCoreReuse(b *testing.B) {
	payload := []byte("hello world\r\n")
	term := core.NewTerminal(120, 40)
	var parser Parser
	phase13PrewarmScrollback(term)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Advance(term, payload)
	}
}

func phase13PrewarmScrollback(term *core.Terminal) {
	for i := 0; i < term.Rows()+1; i++ {
		term.NewLine()
	}
}
