package vt

import (
	"bytes"
	"testing"

	"cervterm/internal/core"
)

func BenchmarkPhase13ControlStringDiscard(b *testing.B) {
	payload := bytes.Repeat([]byte{'x'}, 32*1024)
	input := append([]byte("\x1b_"), payload...)
	input = append(input, 0x1b, '\\')
	term := core.NewTerminal(80, 24)
	var parser Parser
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Advance(term, input)
	}
}

func BenchmarkPhase13ControlStringOverflow(b *testing.B) {
	payload := bytes.Repeat([]byte{'x'}, maxControlStringLen+1)
	input := append([]byte("\x1bP"), payload...)
	input = append(input, 0x1b, '\\')
	term := core.NewTerminal(80, 24)
	var parser Parser
	parser.SetControlStringSink(func(ControlStringEvent) {})
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Advance(term, input)
	}
}
