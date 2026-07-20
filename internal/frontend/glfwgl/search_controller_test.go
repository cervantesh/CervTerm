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
	sc.init(newLockedTerminal(term, &sync.Mutex{}), func() { redraws++ })
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

// fakeSearchTerminal is a searchTerminal port double: no terminal, no lock. Its
// existence is the point of the port — the controller is now testable without a
// core.Terminal at all.
type fakeSearchTerminal struct {
	row, col int
	ok       bool
	calls    []struct {
		query   string
		hasPrev bool
		prevRow int
	}
}

func (f *fakeSearchTerminal) SearchUpward(query string, hasPrev bool, prevRow int) (int, int, bool) {
	f.calls = append(f.calls, struct {
		query   string
		hasPrev bool
		prevRow int
	}{query, hasPrev, prevRow})
	return f.row, f.col, f.ok
}

// TestSearchControllerDrivesPort pins that next() feeds the port the right
// from-row convention (no prior match on the first jump, prevRow after) and
// records the returned match — exercised entirely through the fake port.
func TestSearchControllerDrivesPort(t *testing.T) {
	fake := &fakeSearchTerminal{row: 7, col: 2, ok: true}
	sc := &searchController{}
	sc.init(fake, func() {})

	sc.query = []rune("hi")
	sc.next()
	if len(fake.calls) != 1 || fake.calls[0].hasPrev {
		t.Fatalf("first next() must query with hasPrev=false; calls=%+v", fake.calls)
	}
	if !sc.hasMatch || sc.matchRow != 7 || sc.matchCol != 2 || sc.matchLen != 2 {
		t.Fatalf("match not recorded: has=%t row=%d col=%d len=%d", sc.hasMatch, sc.matchRow, sc.matchCol, sc.matchLen)
	}

	sc.next()
	if !fake.calls[1].hasPrev || fake.calls[1].prevRow != 7 {
		t.Fatalf("second next() must search above the current match (prevRow=7); got %+v", fake.calls[1])
	}

	fake.ok = false
	sc.next()
	if sc.hasMatch {
		t.Fatal("a miss must clear hasMatch")
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
