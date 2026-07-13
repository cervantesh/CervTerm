package core

import "testing"

func TestTerminalBasicEditing(t *testing.T) {
	term := NewTerminal(8, 3)
	for _, r := range "abc" {
		term.PutRune(r)
	}
	term.Backspace()
	term.PutRune('Z')
	term.CarriageReturn()
	term.NewLine()
	term.PutRune('x')

	want := "abZ\nx\n"
	if got := term.PlainText(); got != want {
		t.Fatalf("plain text mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTerminalAutoWrapAtRightEdge(t *testing.T) {
	term := NewTerminal(4, 3)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}

	want := "abcd\nef\n"
	if got := term.PlainText(); got != want {
		t.Fatalf("wrap mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTerminalAutoWrapScrollsAtBottom(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abcdefghij" {
		term.PutRune(r)
	}

	want := "efgh\nij"
	if got := term.PlainText(); got != want {
		t.Fatalf("wrap scroll mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTerminalScrollbackIncludesAutoWrappedLines(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abcdefghijklmn" {
		term.PutRune(r)
	}

	if term.ScrollbackLines() != 2 {
		t.Fatalf("expected 2 wrapped scrollback lines, got %d", term.ScrollbackLines())
	}

	view := make([]Cell, term.Cols()*term.Rows())
	term.ScrollViewport(2)
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got != "abcd\nefgh" {
		t.Fatalf("wrapped scrollback view mismatch: %q", got)
	}
}

func TestTerminalResizeRejoinsOnlyAutoWrappedLines(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}

	term.Resize(6, 2)
	if got := term.PlainText(); got != "abcdef\n" {
		t.Fatalf("auto-wrapped resize mismatch: %q", got)
	}

	term = NewTerminal(4, 2)
	for _, r := range "abcd" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "ef" {
		term.PutRune(r)
	}

	term.Resize(6, 2)
	if got := term.PlainText(); got != "abcd\nef" {
		t.Fatalf("explicit newline resize mismatch: %q", got)
	}
}

func TestTerminalResizePreservesScrollableHistory(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abcdefghijklmn" {
		term.PutRune(r)
	}

	term.Resize(6, 2)
	if term.ScrollbackLines() == 0 {
		t.Fatalf("resize should preserve scrollback history")
	}

	view := make([]Cell, term.Cols()*term.Rows())
	term.ScrollViewport(1)
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got == term.PlainText() {
		t.Fatalf("scrolling after resize should change the viewport, got %q", got)
	}
}

func TestTerminalScrollsAtBottom(t *testing.T) {
	term := NewTerminal(5, 2)
	for _, r := range "one" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "two" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "tri" {
		term.PutRune(r)
	}

	want := "two\ntri"
	if got := term.PlainText(); got != want {
		t.Fatalf("scroll mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestTerminalResizePreservesOverlap(t *testing.T) {
	term := NewTerminal(6, 2)
	for _, r := range "hello" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "world" {
		term.PutRune(r)
	}

	term.Resize(4, 3)
	want := "o\nworl\nd"
	if got := term.PlainText(); got != want {
		t.Fatalf("resize mismatch\nwant: %q\n got: %q", want, got)
	}
	if term.ScrollbackLines() != 1 {
		t.Fatalf("expected reflowed history to preserve one scrollback line, got %d", term.ScrollbackLines())
	}
}

func TestTerminalResizeStartupShrinkDropsTrailingBlankRows(t *testing.T) {
	term := NewTerminal(100, 32)

	term.Resize(77, 23)

	if got := term.ScrollbackLines(); got != 0 {
		t.Fatalf("startup shrink should not add blank scrollback rows, got %d", got)
	}
}

func TestTerminalResizePromptAtTopDoesNotAddHistory(t *testing.T) {
	term := NewTerminal(100, 32)
	for _, r := range "C:\\>" {
		term.PutRune(r)
	}

	term.Resize(77, 23)

	if got := term.ScrollbackLines(); got != 0 {
		t.Fatalf("prompt-at-top shrink should not add history, got %d rows", got)
	}
	if got := term.PlainText(); got[:4] != "C:\\>" {
		t.Fatalf("prompt should remain at the top after shrink, got %q", got)
	}
}

func TestTerminalResizeFullContentPreservesShrinkHistory(t *testing.T) {
	term := NewTerminal(4, 4)
	for row, text := range []string{"aaa", "bbb", "ccc", "ddd"} {
		term.SetCursor(row, 0)
		for _, r := range text {
			term.PutRune(r)
		}
	}

	term.Resize(4, 2)

	if got := term.ScrollbackLines(); got != 2 {
		t.Fatalf("full-content shrink should preserve two history rows, got %d", got)
	}
	if got := term.PlainText(); got != "ccc\nddd" {
		t.Fatalf("full-content shrink visible rows mismatch: %q", got)
	}
	view := make([]Cell, term.Cols()*term.Rows())
	term.ScrollViewport(2)
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got != "aaa\nbbb" {
		t.Fatalf("full-content shrink history mismatch: %q", got)
	}
}

func TestTerminalResizeCursorAtBottomProtectsHistory(t *testing.T) {
	term := NewTerminal(5, 6)
	for row, text := range []string{"one", "two", "tri"} {
		term.SetCursor(row, 0)
		for _, r := range text {
			term.PutRune(r)
		}
	}
	term.SetCursor(5, 0)

	term.Resize(5, 3)

	if got := term.ScrollbackLines(); got != 3 {
		t.Fatalf("cursor-row protection should preserve three history rows, got %d", got)
	}
	if got := term.CursorRow(); got != 2 {
		t.Fatalf("bottom cursor should remain on the bottom visible row, got %d", got)
	}
	view := make([]Cell, term.Cols()*term.Rows())
	term.ScrollViewport(3)
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got != "one\ntwo\ntri" {
		t.Fatalf("cursor-row protection lost history content: %q", got)
	}
}

func TestTerminalResizeDoesNotTrimBlankWrappedRow(t *testing.T) {
	term := NewTerminal(4, 4)
	term.rowWrapped[3] = true

	term.Resize(4, 2)

	if got := term.ScrollbackLines(); got != 2 {
		t.Fatalf("blank wrapped row should stop trimming, got %d history rows", got)
	}
	if !term.rowWrapped[1] {
		t.Fatalf("blank wrapped row should retain its wrapped flag")
	}
}

func TestTerminalResizeGrowThenShrinkRoundTrip(t *testing.T) {
	term := NewTerminal(100, 32)
	for _, r := range "prompt" {
		term.PutRune(r)
	}
	term.Resize(77, 23)
	term.Resize(100, 32)
	term.Resize(77, 23)

	if got := term.ScrollbackLines(); got != 0 {
		t.Fatalf("grow-then-shrink should not invent history, got %d rows", got)
	}
	if got := term.PlainText(); got[:6] != "prompt" {
		t.Fatalf("grow-then-shrink should preserve prompt content, got %q", got)
	}
	if got := term.CursorRow(); got != 0 {
		t.Fatalf("grow-then-shrink should preserve cursor row 0, got %d", got)
	}
}

func TestTerminalEraseLineModes(t *testing.T) {
	term := NewTerminal(6, 2)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}

	term.SetCursor(0, 2)
	term.ClearToEndOfLine()
	if got := term.PlainText(); got != "ab\n" {
		t.Fatalf("erase line right mismatch: %q", got)
	}

	term = NewTerminal(6, 2)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}
	term.SetCursor(0, 2)
	term.ClearToBeginningOfLine()
	if got := term.PlainText(); got != "   def\n" {
		t.Fatalf("erase line left mismatch: %q", got)
	}

	term.ClearLine(0)
	if got := term.PlainText(); got != "\n" {
		t.Fatalf("erase whole line mismatch: %q", got)
	}
}

func TestTerminalEraseDisplayModes(t *testing.T) {
	term := NewTerminal(4, 3)
	for _, r := range "abcd" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "efgh" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "ijkl" {
		term.PutRune(r)
	}

	term.SetCursor(1, 1)
	term.ClearToEndOfScreen()
	if got := term.PlainText(); got != "abcd\ne\n" {
		t.Fatalf("erase display below mismatch: %q", got)
	}

	term = NewTerminal(4, 3)
	for _, r := range "abcd" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "efgh" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "ijkl" {
		term.PutRune(r)
	}
	term.SetCursor(1, 1)
	term.ClearToBeginningOfScreen()
	if got := term.PlainText(); got != "\n  gh\nijkl" {
		t.Fatalf("erase display above mismatch: %q", got)
	}
}

func TestTerminalSaveRestoreCursor(t *testing.T) {
	term := NewTerminal(6, 2)
	term.SetCursor(0, 2)
	term.SaveCursor()
	term.SetCursor(1, 4)
	term.RestoreCursor()
	if term.CursorRow() != 0 || term.CursorCol() != 2 {
		t.Fatalf("expected cursor 0,2 got %d,%d", term.CursorRow(), term.CursorCol())
	}
	term.PutRune('X')
	if got := term.PlainText(); got != "  X\n" {
		t.Fatalf("restore cursor write mismatch: %q", got)
	}
}

func TestTerminalAttributes(t *testing.T) {
	term := NewTerminal(4, 1)
	term.SetFG(ANSIColor(2))
	term.SetBG(ANSIColor(4))
	term.SetBold(true)
	term.PutRune('x')

	cell := term.Cells()[0]
	if cell.Rune != 'x' || cell.Attr.FG != ANSIColor(2) || cell.Attr.BG != ANSIColor(4) || !cell.Attr.Bold {
		t.Fatalf("unexpected cell: %#v", cell)
	}
}

func TestTerminalBracketedPasteMode(t *testing.T) {
	term := NewTerminal(4, 1)
	if term.BracketedPasteMode() {
		t.Fatalf("bracketed paste should default to disabled")
	}
	term.SetBracketedPasteMode(true)
	if !term.BracketedPasteMode() {
		t.Fatalf("bracketed paste should be enabled")
	}
	term.SetBracketedPasteMode(false)
	if term.BracketedPasteMode() {
		t.Fatalf("bracketed paste should be disabled")
	}
}

func TestTerminalAlternateScreenPreservesPrimary(t *testing.T) {
	term := NewTerminal(5, 2)
	writeLine := func(s string) {
		for _, r := range s {
			term.PutRune(r)
		}
		term.CarriageReturn()
		term.NewLine()
	}
	writeLine("one")
	writeLine("two")
	writeLine("tri")

	primaryView := term.PlainText()
	primaryScrollback := term.ScrollbackLines()

	term.SetAlternateScreenMode(true)
	if term.AlternateScreenMode() != true {
		t.Fatalf("alternate screen should be active")
	}
	if term.ScrollbackLines() != 0 {
		t.Fatalf("alternate screen should start without primary scrollback, got %d", term.ScrollbackLines())
	}
	for _, r := range "ALT" {
		term.PutRune(r)
	}
	if got := term.PlainText(); got != "ALT\n" {
		t.Fatalf("alternate screen mismatch: %q", got)
	}

	term.SetAlternateScreenMode(false)
	if term.AlternateScreenMode() {
		t.Fatalf("alternate screen should be inactive")
	}
	if got := term.PlainText(); got != primaryView {
		t.Fatalf("primary screen should be restored\nwant: %q\n got: %q", primaryView, got)
	}
	if term.ScrollbackLines() != primaryScrollback {
		t.Fatalf("primary scrollback should be restored, want %d got %d", primaryScrollback, term.ScrollbackLines())
	}
}

func TestTerminalScrollbackViewport(t *testing.T) {
	term := NewTerminal(5, 2)
	writeLine := func(s string) {
		for _, r := range s {
			term.PutRune(r)
		}
		term.CarriageReturn()
		term.NewLine()
	}
	writeLine("one")
	writeLine("two")
	writeLine("tri")

	if term.ScrollbackLines() != 2 {
		t.Fatalf("expected 2 scrollback lines, got %d", term.ScrollbackLines())
	}
	if term.DisplayOffset() != 0 {
		t.Fatalf("expected bottom display offset")
	}

	view := make([]Cell, term.Cols()*term.Rows())
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got != "tri\n" {
		t.Fatalf("bottom view mismatch: %q", got)
	}

	term.ScrollViewport(2)
	if term.DisplayOffset() != 2 {
		t.Fatalf("expected offset 2, got %d", term.DisplayOffset())
	}
	term.CopyView(view)
	if got := cellsText(view, term.Cols(), term.Rows()); got != "one\ntwo" {
		t.Fatalf("scrolled view mismatch: %q", got)
	}

	term.ScrollViewport(-99)
	if term.DisplayOffset() != 0 {
		t.Fatalf("expected clamped bottom offset, got %d", term.DisplayOffset())
	}
}

func TestTerminalScrollRegionScrollsOnlyRegion(t *testing.T) {
	term := NewTerminal(4, 4)
	for row, text := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		term.SetCursor(row, 0)
		for _, r := range text {
			term.PutRune(r)
		}
	}
	term.SetScrollRegion(1, 2)
	term.SetCursor(2, 0)
	term.NewLine()

	if got := term.PlainText(); got != "aaaa\ncccc\n\ndddd" {
		t.Fatalf("regional scroll mismatch: %q", got)
	}
	if term.ScrollbackLines() != 0 {
		t.Fatalf("regional scroll should not append scrollback, got %d", term.ScrollbackLines())
	}
}

func TestTerminalFullScreenScrollRegionAddsScrollback(t *testing.T) {
	term := NewTerminal(4, 2)
	for _, r := range "abcd" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()
	for _, r := range "efgh" {
		term.PutRune(r)
	}
	term.CarriageReturn()
	term.NewLine()

	if got := term.ScrollbackLines(); got != 1 {
		t.Fatalf("full-screen scroll should append scrollback, got %d", got)
	}
}

func TestTerminalInsertDeleteChars(t *testing.T) {
	term := NewTerminal(6, 1)
	for _, r := range "abcdef" {
		term.PutRune(r)
	}
	term.SetCursor(0, 2)
	term.InsertChars(2)
	if got := term.PlainText(); got != "ab  cd" {
		t.Fatalf("insert chars mismatch: %q", got)
	}

	term.SetCursor(0, 1)
	term.DeleteChars(3)
	if got := term.PlainText(); got != "acd" {
		t.Fatalf("delete chars mismatch: %q", got)
	}
}

func TestTerminalInsertDeleteLinesWithinScrollRegion(t *testing.T) {
	term := NewTerminal(4, 4)
	for row, text := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		term.SetCursor(row, 0)
		for _, r := range text {
			term.PutRune(r)
		}
	}
	term.SetScrollRegion(1, 3)
	term.SetCursor(1, 0)
	term.InsertLines(1)
	if got := term.PlainText(); got != "aaaa\n\nbbbb\ncccc" {
		t.Fatalf("insert lines mismatch: %q", got)
	}

	term.SetCursor(2, 0)
	term.DeleteLines(1)
	if got := term.PlainText(); got != "aaaa\n\ncccc\n" {
		t.Fatalf("delete lines mismatch: %q", got)
	}
}

func TestTerminalCursorVisibleAndAutowrapModes(t *testing.T) {
	term := NewTerminal(3, 2)
	if !term.CursorVisible() {
		t.Fatalf("cursor should default visible")
	}
	term.SetCursorVisible(false)
	if term.CursorVisible() {
		t.Fatalf("cursor should be hidden")
	}
	if !term.AutoWrapMode() {
		t.Fatalf("autowrap should default enabled")
	}
	term.SetAutoWrapMode(false)
	for _, r := range "abcd" {
		term.PutRune(r)
	}
	if got := term.PlainText(); got != "abd\n" {
		t.Fatalf("disabled autowrap should overwrite right edge, got %q", got)
	}
}

func cellsText(cells []Cell, cols, rows int) string {
	out := ""
	for row := 0; row < rows; row++ {
		if row > 0 {
			out += "\n"
		}
		start := row * cols
		end := start + cols
		last := end - 1
		for last >= start && cells[last].Rune == ' ' {
			last--
		}
		for i := start; i <= last; i++ {
			out += string(cells[i].Rune)
		}
	}
	return out
}
