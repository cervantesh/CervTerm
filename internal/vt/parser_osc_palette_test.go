package vt

import (
	"fmt"
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestParserOSC4SetsAndQueriesInAuthoredOrder(t *testing.T) {
	term := core.NewTerminal(4, 2)
	base := term.PaletteBase()
	base.Indexed[2] = core.RGB{R: 0x12, G: 0x34, B: 0x56}
	term.SetPaletteBase(base)

	var replies []string
	parser := Parser{Reply: func(reply []byte) { replies = append(replies, string(reply)) }}
	parser.Advance(term, []byte("\x1b]4;2;?;2;#ABCDEF;2;?;3;rgb:1/80/ffff;3;?\x1b\\"))

	wantReplies := []string{
		"\x1b]4;2;rgb:1212/3434/5656\x1b\\",
		"\x1b]4;2;rgb:ABAB/CDCD/EFEF\x1b\\",
		"\x1b]4;3;rgb:1111/8080/FFFF\x1b\\",
	}
	if fmt.Sprint(replies) != fmt.Sprint(wantReplies) {
		t.Fatalf("replies = %q, want %q", replies, wantReplies)
	}
	if got := term.EffectivePaletteIndex(2); got != (core.RGB{R: 0xAB, G: 0xCD, B: 0xEF}) {
		t.Fatalf("index 2 = %#v", got)
	}
	if got := term.EffectivePaletteIndex(3); got != (core.RGB{R: 0x11, G: 0x80, B: 0xFF}) {
		t.Fatalf("index 3 = %#v", got)
	}
}

func TestParserOSCColorFormatsAndProportionalRounding(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want core.RGB
	}{
		{"hash uppercase", "#123ABC", core.RGB{R: 0x12, G: 0x3A, B: 0xBC}},
		{"hash lowercase", "#abcdef", core.RGB{R: 0xAB, G: 0xCD, B: 0xEF}},
		{"one digit", "rgb:0/8/f", core.RGB{R: 0x00, G: 0x88, B: 0xFF}},
		{"two digits", "rgb:01/80/fe", core.RGB{R: 0x01, G: 0x80, B: 0xFE}},
		{"three digits rounds", "rgb:001/800/ffe", core.RGB{R: 0x00, G: 0x80, B: 0xFF}},
		{"four digits rounds", "rgb:0101/8080/fefe", core.RGB{R: 0x01, G: 0x80, B: 0xFE}},
		{"mixed widths", "rgb:f/080/8000", core.RGB{R: 0xFF, G: 0x08, B: 0x80}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := core.NewTerminal(2, 1)
			var parser Parser
			parser.Advance(term, []byte("\x1b]4;7;"+tt.spec+"\x07"))
			if got := term.EffectivePaletteIndex(7); got != tt.want {
				t.Fatalf("color = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParserOSCDefaultColorsSetQueryAndReset(t *testing.T) {
	term := core.NewTerminal(2, 1)
	base := term.PaletteBase()
	base.FG = core.RGB{R: 1, G: 2, B: 3}
	base.BG = core.RGB{R: 4, G: 5, B: 6}
	term.SetPaletteBase(base)

	var reply strings.Builder
	parser := Parser{Reply: func(value []byte) { reply.Write(value) }}
	parser.Advance(term, []byte("\x1b]10;#102030\x07\x1b]11;rgb:f/0/8\x1b\\"))
	parser.Advance(term, []byte("\x1b]10;?\x07\x1b]11;?\x1b\\"))
	if want := "\x1b]10;rgb:1010/2020/3030\x1b\\\x1b]11;rgb:FFFF/0000/8888\x1b\\"; reply.String() != want {
		t.Fatalf("reply = %q, want %q", reply.String(), want)
	}

	parser.Advance(term, []byte("\x1b]110\x07\x1b]111\x1b\\"))
	if term.EffectivePaletteFG() != base.FG || term.EffectivePaletteBG() != base.BG {
		t.Fatalf("reset defaults = %#v, %#v", term.EffectivePaletteFG(), term.EffectivePaletteBG())
	}
}

func TestParserOSC104ResetsSelectedOrAll(t *testing.T) {
	term := core.NewTerminal(2, 1)
	term.SetPaletteIndex(1, core.RGB{R: 1})
	term.SetPaletteIndex(2, core.RGB{G: 2})
	term.SetPaletteIndex(255, core.RGB{B: 3})
	var parser Parser

	parser.Advance(term, []byte("\x1b]104;1;255\x07"))
	overrides := term.PaletteOverrides()
	if overrides.HasIndexed(1) || !overrides.HasIndexed(2) || overrides.HasIndexed(255) {
		t.Fatalf("selected reset overrides = %#v", overrides.IndexedSet)
	}
	parser.Advance(term, []byte("\x1b]104\x1b\\"))
	if got := term.PaletteOverrides().IndexedSet; got != [4]uint64{} {
		t.Fatalf("reset all indexed set = %#v", got)
	}
}

func TestParserOSCPaletteCommandsAreAtomicOnMalformedInput(t *testing.T) {
	tests := []string{
		"4;1;#010203;2;bogus",
		"4;1;?;256;#010203",
		"4;1;#010203;2",
		"4;1;#010203;",
		"10;#010203;extra",
		"11;rgb:1/2",
		"104;1;256",
		"104;1;",
		"110;extra",
		"111;?",
	}
	for _, payload := range tests {
		t.Run(payload, func(t *testing.T) {
			term := core.NewTerminal(2, 1)
			before := term.PaletteOverrides()
			var replies int
			parser := Parser{Reply: func([]byte) { replies++ }}
			parser.Advance(term, []byte("\x1b]"+payload+"\x07"))
			if got := term.PaletteOverrides(); got != before {
				t.Fatalf("malformed command mutated state: %#v", got)
			}
			if replies != 0 {
				t.Fatalf("malformed command emitted %d replies", replies)
			}
		})
	}
}

func TestParserOSC4And104PairLimits(t *testing.T) {
	var valid4 strings.Builder
	valid4.WriteString("\x1b]4")
	for i := 0; i < 256; i++ {
		fmt.Fprintf(&valid4, ";%d;#%02X0000", i, i)
	}
	valid4.WriteByte('\a')
	term := core.NewTerminal(2, 1)
	var parser Parser
	parser.Advance(term, []byte(valid4.String()))
	for i := 0; i < 256; i++ {
		if got := term.EffectivePaletteIndex(uint8(i)); got.R != uint8(i) || got.G != 0 || got.B != 0 {
			t.Fatalf("index %d = %#v", i, got)
		}
	}

	before := term.PaletteOverrides()
	var tooMany4 strings.Builder
	tooMany4.WriteString("\x1b]4")
	for i := 0; i < 257; i++ {
		fmt.Fprintf(&tooMany4, ";%d;#FFFFFF", i%256)
	}
	tooMany4.WriteByte('\a')
	parser.Advance(term, []byte(tooMany4.String()))
	if got := term.PaletteOverrides(); got != before {
		t.Fatal("257 OSC 4 pairs mutated state")
	}

	var valid104 strings.Builder
	valid104.WriteString("\x1b]104")
	for i := 0; i < 256; i++ {
		fmt.Fprintf(&valid104, ";%d", i)
	}
	valid104.WriteByte('\a')
	parser.Advance(term, []byte(valid104.String()))
	if got := term.PaletteOverrides().IndexedSet; got != [4]uint64{} {
		t.Fatalf("256 OSC 104 indexes left overrides %#v", got)
	}

	term.SetPaletteIndex(1, core.RGB{R: 1})
	var tooMany104 strings.Builder
	tooMany104.WriteString("\x1b]104")
	for i := 0; i < 257; i++ {
		fmt.Fprintf(&tooMany104, ";%d", i%256)
	}
	tooMany104.WriteByte('\a')
	parser.Advance(term, []byte(tooMany104.String()))
	if !term.PaletteOverrides().HasIndexed(1) {
		t.Fatal("257 OSC 104 indexes reset state")
	}
}

func TestParserOSCPaletteCollectorTerminationChunkingAndNilReply(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{"BEL", []string{"\x1b]4;9;#123456\x07"}},
		{"ST", []string{"\x1b]4;9;#123456\x1b\\"}},
		{"chunked ST", []string{"\x1b]4;9;#12", "3456\x1b", "\\"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := core.NewTerminal(2, 1)
			var parser Parser
			for _, chunk := range tt.chunks {
				parser.Advance(term, []byte(chunk))
			}
			if got := term.EffectivePaletteIndex(9); got != (core.RGB{R: 0x12, G: 0x34, B: 0x56}) {
				t.Fatalf("color = %#v", got)
			}
			parser.Advance(term, []byte("\x1b]4;9;?\x07\x1b]10;?\x07\x1b]11;?\x07"))
		})
	}
}

func TestParserOSCPaletteRejectsInvalidColorsAndIndexes(t *testing.T) {
	invalidSpecs := []string{
		"", "#12345", "#1234567", "#12GG56", "RGB:1/2/3", "rgb:/2/3",
		"rgb:12345/2/3", "rgb:1/2", "rgb:1/2/3/4", "rgb:1/2/zz",
	}
	for _, spec := range invalidSpecs {
		term := core.NewTerminal(2, 1)
		before := term.PaletteOverrides()
		var parser Parser
		parser.Advance(term, []byte("\x1b]4;1;"+spec+"\x07"))
		if got := term.PaletteOverrides(); got != before {
			t.Fatalf("spec %q mutated state", spec)
		}
	}

	for _, index := range []string{"", "-1", "+1", " 1", "1 ", "256", "999", "1x"} {
		term := core.NewTerminal(2, 1)
		before := term.PaletteOverrides()
		var parser Parser
		parser.Advance(term, []byte("\x1b]4;"+index+";#010203\x07"))
		if got := term.PaletteOverrides(); got != before {
			t.Fatalf("index %q mutated state", index)
		}
	}
}
