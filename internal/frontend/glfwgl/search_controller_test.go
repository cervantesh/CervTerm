//go:build glfw

package glfwgl

import (
	"sync"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/vt"
)

// newTestSearch builds a searchController wired to a real terminal, a private
// mutex, and a redraw counter — no App, window, or GL needed. This isolation is
// the point of extracting the controller: its dependencies are explicit.
func newTestSearch(t *testing.T, lines string) (*searchController, *int) {
	t.Helper()
	term := core.NewTerminal(20, 4)
	var parser vt.Parser
	parser.Advance(term, []byte(lines))
	redraws := 0
	sc := &searchController{}
	sc.init(term, &sync.Mutex{}, func() { redraws++ })
	return sc, &redraws
}

func TestSearchControllerFindsAndMisses(t *testing.T) {
	sc, redraws := newTestSearch(t, "alpha\r\nbeta\r\ngamma\r\n")

	sc.query = []rune("beta")
	before := *redraws
	sc.next()
	if !sc.hasMatch {
		t.Fatalf("expected a match for %q", string(sc.query))
	}
	if sc.matchLen != 4 {
		t.Fatalf("matchLen = %d, want 4", sc.matchLen)
	}
	if *redraws <= before {
		t.Fatal("next() must request a redraw")
	}

	// A query with no match clears hasMatch.
	sc.query = []rune("zzz")
	sc.next()
	if sc.hasMatch {
		t.Fatal("expected no match for zzz")
	}
}

func TestSearchControllerEmptyQueryIsNoOp(t *testing.T) {
	sc, _ := newTestSearch(t, "hello\r\n")
	sc.hasMatch = true
	sc.query = sc.query[:0]
	sc.next()
	if sc.hasMatch {
		t.Fatal("empty query must clear hasMatch and not search")
	}
}

func TestSearchControllerQueryEditing(t *testing.T) {
	sc, _ := newTestSearch(t, "x\r\n")

	sc.appendRune('a')
	sc.appendRune('b')
	sc.appendRune('\x01') // control rune ignored
	sc.appendRune(0x7f)   // DEL ignored
	if got := string(sc.query); got != "ab" {
		t.Fatalf("query = %q, want %q", got, "ab")
	}
	sc.backspace()
	if got := string(sc.query); got != "a" {
		t.Fatalf("after backspace query = %q, want %q", got, "a")
	}

	sc.open()
	if !sc.active || len(sc.query) != 0 || sc.hasMatch {
		t.Fatalf("open() must activate, clear the query, and drop any match")
	}
	sc.close()
	if sc.active || sc.hasMatch {
		t.Fatalf("close() must deactivate and drop any match")
	}
}
