//go:build glfw

package glfwgl

import (
	"strings"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
)

// App's script.Host implementation: the terminal surface Lua handlers see.

func (a *App) WriteInput(s string) {
	a.writeInput(s)
}

// Notify runs on the main thread only (script dispatch and drain paths), so it
// may set the redraw flag directly to paint the notice promptly.
func (a *App) Notify(msg string) {
	a.notice = msg
	a.noticeUntil = time.Now().Add(4 * time.Second)
	a.requestRedraw()
}

func (a *App) Selection() string {
	if !a.selectionActive {
		return ""
	}
	// Script handlers run on the main thread between frames, never inside
	// draw(), so reusing a.snap is safe while the terminal snapshot is captured
	// under a.mu.
	a.mu.Lock()
	render.Capture(&a.snap, a.term)
	a.mu.Unlock()
	return termsel.Text(a.snap, termsel.Range{Start: a.selectionStart, End: a.selectionEnd})
}

func (a *App) SetClipboard(text string) {
	if a.window != nil {
		a.window.SetClipboardString(text)
	}
}

func (a *App) Clipboard() string {
	if a.window == nil {
		return ""
	}
	return a.window.GetClipboardString()
}

func (a *App) Scroll(lines int) bool {
	a.mu.Lock()
	moved := a.term.ScrollViewport(lines)
	a.mu.Unlock()
	if moved {
		a.requestRedraw()
	}
	return moved
}

func (a *App) ScrollToBottom() {
	a.mu.Lock()
	moved := a.term.ScrollViewport(-a.term.ScrollbackLines())
	a.mu.Unlock()
	if moved {
		a.requestRedraw()
	}
}

func (a *App) ScrollbackLen() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.ScrollbackLines()
}

// flushReplies sends queued parser replies to the PTY outside a.mu. Main
// thread only.
func (a *App) flushReplies() {
	if len(a.pendingReplies) == 0 {
		return
	}
	replies := a.pendingReplies
	a.pendingReplies = nil
	if a.pty == nil {
		return
	}
	for _, b := range replies {
		_, _ = a.pty.Write(b)
	}
}

// Size, Cursor, Title, and Line expose read-only terminal state to Lua handlers.
// They are called on the main loop thread while the handler runs; the lock guards
// against future concurrent access and matches the other term accessors.

func (a *App) Size() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.Cols(), a.term.Rows()
}

func (a *App) Cursor() (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.CursorRow(), a.term.CursorCol()
}

func (a *App) Title() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.Title()
}

func (a *App) SetTitle(title string) {
	a.mu.Lock()
	changed := a.term.Title() != title
	if changed {
		a.term.SetTitle(title)
	}
	a.mu.Unlock()
	if changed {
		// Re-arm terminal event processing so this follows the same title update
		// and script dispatch path as an OSC 0/2 title change.
		a.termEventsPending = true
		a.requestRedraw()
	}
}

func (a *App) Line(row int) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cols, rows := a.term.Cols(), a.term.Rows()
	if row < 0 || row >= rows {
		return "", false
	}
	cells := make([]core.Cell, cols*rows)
	a.term.CopyView(cells)
	start := row * cols
	last := start + cols - 1
	for last >= start && (cells[last].Rune == ' ' || cells[last].Rune == 0) {
		last--
	}
	var b strings.Builder
	for i := start; i <= last; i++ {
		if cells[i].WideContinuation {
			continue
		}
		r := cells[i].Rune
		if r == 0 {
			r = ' '
		}
		b.WriteRune(r)
		for _, c := range cells[i].Combining {
			b.WriteRune(c)
		}
	}
	return b.String(), true
}

func (a *App) LineWrapped(row int) (bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.LineWrapped(row)
}
