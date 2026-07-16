package vt

import (
	"os"
	"path/filepath"
	"testing"

	"cervterm/internal/core"
)

func TestParserTextAndSGR(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("hello\r\n\x1b[31mred"))
	got := term.PlainText()
	want := "hello\nred\n\n"
	if got != want {
		t.Fatalf("plain text mismatch\nwant: %q\n got: %q", want, got)
	}
	cell := copyCells(term)[1*term.Cols()]
	if cell.Attr.FG != core.ANSIColor(1) {
		t.Fatalf("expected red fg, got %#v", cell.Attr.FG)
	}
}

func TestParserBrightAndDefaultSGR(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var p Parser

	p.Advance(term, []byte("\x1b[94mF\x1b[103mB\x1b[39;49mD"))
	cells := copyCells(term)

	if cells[0].Attr.FG != core.ANSIColor(12) {
		t.Fatalf("expected bright blue fg, got %#v", cells[0].Attr.FG)
	}
	if cells[1].Attr.FG != core.ANSIColor(12) || cells[1].Attr.BG != core.ANSIColor(11) {
		t.Fatalf("expected bright blue on bright yellow, got %#v", cells[1].Attr)
	}
	if cells[2].Attr.FG != core.DefaultFG || cells[2].Attr.BG != core.DefaultBG {
		t.Fatalf("expected default colors after reset, got %#v", cells[2].Attr)
	}
}

func TestParserExtendedSGRColors(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var p Parser

	p.Advance(term, []byte("\x1b[38;5;196mR\x1b[48;5;21mB\x1b[38;2;12;34;56mT\x1b[48;2;200;150;100mQ"))
	cells := copyCells(term)

	if cells[0].Attr.FG != (core.RGB{R: 255, G: 0, B: 0}) {
		t.Fatalf("expected 256-color red fg, got %#v", cells[0].Attr.FG)
	}
	if cells[1].Attr.BG != (core.RGB{R: 0, G: 0, B: 255}) {
		t.Fatalf("expected 256-color blue bg, got %#v", cells[1].Attr.BG)
	}
	if cells[2].Attr.FG != (core.RGB{R: 12, G: 34, B: 56}) {
		t.Fatalf("expected truecolor fg, got %#v", cells[2].Attr.FG)
	}
	if cells[3].Attr.BG != (core.RGB{R: 200, G: 150, B: 100}) {
		t.Fatalf("expected truecolor bg, got %#v", cells[3].Attr.BG)
	}
}

func TestParserAdditionalSGRAttributes(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var p Parser
	p.Advance(term, []byte("\x1b[3;4;7;9mX\x1b[23;24;27;29mY"))
	cells := copyCells(term)
	if !cells[0].Attr.Italic || !cells[0].Attr.Underline || !cells[0].Attr.Inverse || !cells[0].Attr.Strikethrough {
		t.Fatalf("expected all additional attrs on first cell, got %#v", cells[0].Attr)
	}
	if cells[1].Attr.Italic || cells[1].Attr.Underline || cells[1].Attr.Inverse || cells[1].Attr.Strikethrough {
		t.Fatalf("expected attrs reset on second cell, got %#v", cells[1].Attr)
	}
}

func TestParserSplitUTF8AcrossAdvanceCalls(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	bytes := []byte("é")
	p.Advance(term, bytes[:1])
	if got := term.PlainText(); got != "\n\n\n" {
		t.Fatalf("incomplete UTF-8 should not print yet, got %q", got)
	}
	p.Advance(term, bytes[1:])
	if got := term.PlainText(); got != "é\n\n\n" {
		t.Fatalf("split UTF-8 mismatch: %q", got)
	}
}

func TestParserSplitUTF8BeforeASCII(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	bytes := []byte("好")
	p.Advance(term, bytes[:2])
	p.Advance(term, append(bytes[2:], 'x'))
	if got := term.PlainText(); got != "好x\n\n\n" {
		t.Fatalf("split UTF-8 before ASCII mismatch: %q", got)
	}
}

func TestParserOSCTitleBEL(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("\x1b]0;cervterm demo\x07"))
	if term.Title() != "cervterm demo" {
		t.Fatalf("unexpected title: %q", term.Title())
	}
}

func TestParserOSCTitleStringTerminator(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("\x1b]2;shell title\x1b\\"))
	if term.Title() != "shell title" {
		t.Fatalf("unexpected title: %q", term.Title())
	}
}

func TestParserIgnoresUnsupportedOSC(t *testing.T) {
	term := core.NewTerminal(20, 4)
	term.SetTitle("old")
	var p Parser
	p.Advance(term, []byte("\x1b]9;new\x07"))
	if term.Title() != "old" {
		t.Fatalf("unsupported OSC changed title: %q", term.Title())
	}
}

func TestParserGroundBellIncrementsCount(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("a\x07b\x07"))
	if term.BellCount() != 2 {
		t.Fatalf("bell count = %d, want 2", term.BellCount())
	}
	if cellsFirstRow(term) != "ab" {
		t.Fatalf("BEL should not print: %q", cellsFirstRow(term))
	}
}

func TestParserOSCTerminatorBELIsNotABell(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("\x1b]0;t\x07"))
	if term.BellCount() != 0 {
		t.Fatalf("OSC-terminating BEL counted as bell: %d", term.BellCount())
	}
}

func cellsFirstRow(term *core.Terminal) string {
	cells := make([]core.Cell, term.Cols()*term.Rows())
	term.CopyView(cells)
	out := ""
	for i := 0; i < term.Cols(); i++ {
		if cells[i].Rune == ' ' || cells[i].Rune == 0 {
			break
		}
		out += string(cells[i].Rune)
	}
	return out
}

func TestParserBracketedPasteMode(t *testing.T) {
	term := core.NewTerminal(20, 4)
	var p Parser
	p.Advance(term, []byte("\x1b[?2004h"))
	if !term.BracketedPasteMode() {
		t.Fatalf("bracketed paste should be enabled")
	}
	p.Advance(term, []byte("\x1b[?2004l"))
	if term.BracketedPasteMode() {
		t.Fatalf("bracketed paste should be disabled")
	}
}

func TestParserAlternateScreenMode(t *testing.T) {
	term := core.NewTerminal(10, 2)
	var p Parser
	p.Advance(term, []byte("primary"))
	p.Advance(term, []byte("\x1b[?1049halt"))
	if !term.AlternateScreenMode() {
		t.Fatalf("alternate screen should be active")
	}
	if got := term.PlainText(); got != "alt\n" {
		t.Fatalf("alternate screen text mismatch: %q", got)
	}

	p.Advance(term, []byte("\x1b[?1049l"))
	if term.AlternateScreenMode() {
		t.Fatalf("alternate screen should be inactive")
	}
	if got := term.PlainText(); got != "primary\n" {
		t.Fatalf("primary screen text mismatch: %q", got)
	}
}

func TestParserEraseModes(t *testing.T) {
	term := core.NewTerminal(4, 3)
	var p Parser
	p.Advance(term, []byte("abcd\r\nefgh\r\nijkl"))
	p.Advance(term, []byte("\x1b[2;2H\x1b[J"))
	if got := term.PlainText(); got != "abcd\ne\n" {
		t.Fatalf("CSI J should erase below cursor, got %q", got)
	}

	term = core.NewTerminal(6, 1)
	p = Parser{}
	p.Advance(term, []byte("abcdef\x1b[1;3H\x1b[1K"))
	if got := term.PlainText(); got != "   def" {
		t.Fatalf("CSI 1K should erase left of cursor, got %q", got)
	}

	term = core.NewTerminal(4, 2)
	p = Parser{}
	p.Advance(term, []byte("one\r\ntwo\r\ntri"))
	if term.ScrollbackLines() == 0 {
		t.Fatalf("expected scrollback before CSI 3J")
	}
	before := term.PlainText()
	p.Advance(term, []byte("\x1b[3J"))
	if term.ScrollbackLines() != 0 {
		t.Fatalf("CSI 3J should clear scrollback, got %d lines", term.ScrollbackLines())
	}
	if got := term.PlainText(); got != before {
		t.Fatalf("CSI 3J should not clear viewport, got %q want %q", got, before)
	}
}

func TestParserEraseCharactersForCMDCompletion(t *testing.T) {
	term := core.NewTerminal(40, 1)
	var p Parser

	p.Advance(term, []byte(">type a-very-long-completion-name.txt"))
	// ConPTY rewrites a shorter cmd.exe completion at column 2, then emits ECH
	// for the cells that belonged to the previous, longer candidate.
	p.Advance(term, []byte("\x1b[1;2Htype b.txt\x1b[27X"))

	if got := term.PlainText(); got != ">type b.txt" {
		t.Fatalf("CSI X left stale completion text: %q", got)
	}
	if row, col := term.CursorRow(), term.CursorCol(); row != 0 || col != 11 {
		t.Fatalf("CSI X moved cursor to (%d,%d)", row, col)
	}
}

func TestParserSaveRestoreCursor(t *testing.T) {
	term := core.NewTerminal(6, 2)
	var p Parser
	p.Advance(term, []byte("\x1b[1;3H\x1b7\x1b[2;5H\x1b8X"))
	if got := term.PlainText(); got != "  X\n" {
		t.Fatalf("ESC save/restore cursor mismatch: %q", got)
	}

	term = core.NewTerminal(6, 2)
	p = Parser{}
	p.Advance(term, []byte("\x1b[1;4H\x1b[s\x1b[2;6H\x1b[uY"))
	if got := term.PlainText(); got != "   Y\n" {
		t.Fatalf("CSI save/restore cursor mismatch: %q", got)
	}
}

func TestParserScrollRegionAndInsertDelete(t *testing.T) {
	term := core.NewTerminal(4, 4)
	var p Parser
	p.Advance(term, []byte("aaaa\x1b[2;1Hbbbb\x1b[3;1Hcccc\x1b[4;1Hdddd"))
	p.Advance(term, []byte("\x1b[2;3r\x1b[3;1H\n"))
	if got := term.PlainText(); got != "aaaa\ncccc\n\ndddd" {
		t.Fatalf("CSI r regional scroll mismatch: %q", got)
	}

	term = core.NewTerminal(6, 1)
	p = Parser{}
	p.Advance(term, []byte("abcdef\x1b[1;3H\x1b[2@"))
	if got := term.PlainText(); got != "ab  cd" {
		t.Fatalf("CSI @ insert chars mismatch: %q", got)
	}
	p.Advance(term, []byte("\x1b[1;2H\x1b[3P"))
	if got := term.PlainText(); got != "acd" {
		t.Fatalf("CSI P delete chars mismatch: %q", got)
	}
}

func TestParserInsertDeleteLines(t *testing.T) {
	term := core.NewTerminal(4, 4)
	var p Parser
	p.Advance(term, []byte("aaaa\x1b[2;1Hbbbb\x1b[3;1Hcccc\x1b[4;1Hdddd"))
	p.Advance(term, []byte("\x1b[2;4r\x1b[2;1H\x1b[L"))
	if got := term.PlainText(); got != "aaaa\n\nbbbb\ncccc" {
		t.Fatalf("CSI L insert lines mismatch: %q", got)
	}
	p.Advance(term, []byte("\x1b[3;1H\x1b[M"))
	if got := term.PlainText(); got != "aaaa\n\ncccc\n" {
		t.Fatalf("CSI M delete lines mismatch: %q", got)
	}
}

func TestParserCursorVisibilityAutowrapAndMovement(t *testing.T) {
	term := core.NewTerminal(4, 3)
	var p Parser
	p.Advance(term, []byte("\x1b[?25l"))
	if term.CursorVisible() {
		t.Fatalf("CSI ?25l should hide cursor")
	}
	p.Advance(term, []byte("\x1b[?25h"))
	if !term.CursorVisible() {
		t.Fatalf("CSI ?25h should show cursor")
	}

	p.Advance(term, []byte("\x1b[?7labcdX"))
	if got := term.PlainText(); got != "abcX\n\n" {
		t.Fatalf("CSI ?7l should disable autowrap, got %q", got)
	}
	p.Advance(term, []byte("\x1b[?7h\x1b[2;2HX\x1b[EY\x1b[FZ\x1b[3GQ\x1b[3dR"))
	if term.CursorRow() != 2 || term.CursorCol() != 3 {
		t.Fatalf("unexpected final cursor position %d,%d", term.CursorRow(), term.CursorCol())
	}
}

func TestParserApplicationInputModes(t *testing.T) {
	term := core.NewTerminal(4, 1)
	var p Parser

	p.Advance(term, []byte("\x1b[?1h"))
	if !term.ApplicationCursorMode() {
		t.Fatalf("CSI ?1h should enable application cursor mode")
	}
	p.Advance(term, []byte("\x1b[?1l"))
	if term.ApplicationCursorMode() {
		t.Fatalf("CSI ?1l should disable application cursor mode")
	}

	p.Advance(term, []byte("\x1b="))
	if !term.ApplicationKeypadMode() {
		t.Fatalf("ESC = should enable application keypad mode")
	}
	p.Advance(term, []byte("\x1b>"))
	if term.ApplicationKeypadMode() {
		t.Fatalf("ESC > should disable application keypad mode")
	}
}

func TestParserMouseModes(t *testing.T) {
	term := core.NewTerminal(4, 1)
	var p Parser

	p.Advance(term, []byte("\x1b[?1000;1006h"))
	mode := term.MouseMode()
	if !mode.NormalTracking || !mode.SGR {
		t.Fatalf("expected normal SGR mouse mode, got %#v", mode)
	}

	p.Advance(term, []byte("\x1b[?1002h"))
	if !term.MouseMode().ButtonEventTracking {
		t.Fatalf("expected button-event tracking enabled")
	}

	p.Advance(term, []byte("\x1b[?1000;1002;1006l"))
	if term.MouseMode().NormalTracking || term.MouseMode().ButtonEventTracking || term.MouseMode().SGR {
		t.Fatalf("expected mouse modes disabled, got %#v", term.MouseMode())
	}
}

func prewarmScrollback(term *core.Terminal) {
	for i := 0; i < term.Rows()+1; i++ {
		term.NewLine()
	}
}

func BenchmarkParserThroughput(b *testing.B) {
	payload := []byte("\x1b[32mhello world\x1b[0m\r\n")
	term := core.NewTerminal(120, 40)
	var p Parser
	prewarmScrollback(term)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Advance(term, payload)
	}
}

func BenchmarkCoreReuseVsNew(b *testing.B) {
	payload := []byte("hello world\r\n")
	b.Run("reuse", func(b *testing.B) {
		term := core.NewTerminal(120, 40)
		var p Parser
		prewarmScrollback(term)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p.Advance(term, payload)
		}
	})
	b.Run("new-terminal", func(b *testing.B) {
		var p Parser
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			term := core.NewTerminal(120, 40)
			p.Advance(term, payload)
		}
	})
}

func FuzzParserAdvanceDoesNotPanic(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("hello"),
		[]byte("\x1b[2J\x1b[H"),
		[]byte("\x1b[2;4r\x1b[L\x1b[M"),
		[]byte("\x1b[38;2;255;0;0mred"),
		[]byte{0xff, 0xfe, 0x1b, '[', '?', '2', '5', 'l'},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		term := core.NewTerminal(12, 5)
		var p Parser
		p.Advance(term, data)
		if term.CursorRow() < 0 || term.CursorRow() >= term.Rows() {
			t.Fatalf("cursor row out of bounds: %d rows=%d", term.CursorRow(), term.Rows())
		}
		if term.CursorCol() < 0 || term.CursorCol() >= term.Cols() {
			t.Fatalf("cursor col out of bounds: %d cols=%d", term.CursorCol(), term.Cols())
		}
		if len(copyCells(term)) != term.Rows()*term.Cols() {
			t.Fatalf("cell length = %d, want %d", len(copyCells(term)), term.Rows()*term.Cols())
		}
	})
}

func TestParserGoldenRecordings(t *testing.T) {
	tests := []struct {
		name string
		cols int
		rows int
	}{
		{name: "fullscreen-region", cols: 12, rows: 4},
		{name: "ansi-smoke", cols: 80, rows: 24},
		{name: "vttest-startup", cols: 80, rows: 24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata", tt.name+".vt"))
			if err != nil {
				t.Fatalf("read recording: %v", err)
			}
			wantBytes, err := os.ReadFile(filepath.Join("testdata", tt.name+".golden"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			term := core.NewTerminal(tt.cols, tt.rows)
			var p Parser
			p.Advance(term, input)
			if got, want := term.PlainText(), string(wantBytes); got != want {
				t.Fatalf("golden mismatch\nwant: %q\n got: %q", want, got)
			}
		})
	}
}

// copyCells returns a defensive copy of the current screen cells for assertions
// (replaces the removed core.Terminal.Cells() accessor).
func copyCells(t *core.Terminal) []core.Cell {
	c := make([]core.Cell, t.Cols()*t.Rows())
	t.CopyView(c)
	return c
}
