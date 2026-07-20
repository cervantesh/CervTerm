//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/core"
)

func TestCollectRenderClusterCombiningMark(t *testing.T) {
	cells := []core.Cell{core.NewCellWithCombining('e', core.Attr{}, '\u0301')}
	cluster, ok := collectRenderCluster(cells, 1, 0, 0)
	if !ok {
		t.Fatalf("expected combining cluster")
	}
	if cluster.Text != "e\u0301" || cluster.CellSpan != 1 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterZWJEmojiSequence(t *testing.T) {
	cells := []core.Cell{
		core.NewCellWithCombining('\U0001F468', core.Attr{}, zeroWidthJoiner),
		{WideContinuation: true},
		{Rune: '\U0001F469'},
		{WideContinuation: true},
	}
	cluster, ok := collectRenderCluster(cells, 4, 0, 0)
	if !ok {
		t.Fatalf("expected zwj emoji cluster")
	}
	if cluster.Text != "\U0001F468\u200d\U0001F469" || cluster.CellSpan != 4 {
		t.Fatalf("cluster = %#v", cluster)
	}
	if _, ok := collectRenderCluster(cells, 4, 0, 1); ok {
		t.Fatalf("wide continuation must not start a cluster")
	}
}

func TestCollectRenderClusterEmojiModifier(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.PutRune('👍')
	term.PutRune('\U0001F3FD')
	cluster, ok := collectRenderCluster(copyCells(term), term.Cols(), 0, 0)
	if !ok {
		t.Fatalf("expected emoji modifier cluster")
	}
	if cluster.Text != "👍\U0001F3FD" || cluster.CellSpan != 2 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterSingleEmoji(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.PutRune('😀')
	cluster, ok := collectRenderCluster(copyCells(term), term.Cols(), 0, 0)
	if !ok {
		t.Fatalf("expected single emoji cluster")
	}
	if cluster.Text != "😀" || cluster.CellSpan != 2 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterRegionalIndicatorPair(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.PutRune('🇦')
	term.PutRune('🇷')
	cluster, ok := collectRenderCluster(copyCells(term), term.Cols(), 0, 0)
	if !ok {
		t.Fatalf("expected regional indicator flag cluster")
	}
	if cluster.Text != "🇦🇷" || cluster.CellSpan != 2 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterKeycapUsesClusterWidth(t *testing.T) {
	term := core.NewTerminal(4, 1)
	term.PutRune('1')
	term.PutRune('\ufe0f')
	term.PutRune('\u20e3')
	cluster, ok := collectRenderCluster(copyCells(term), term.Cols(), 0, 0)
	if !ok {
		t.Fatalf("expected keycap emoji cluster")
	}
	if cluster.Text != "1️⃣" || cluster.CellSpan != 2 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterTagFlag(t *testing.T) {
	const england = "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F"
	term := core.NewTerminal(4, 1)
	for _, r := range england {
		term.PutRune(r)
	}
	cluster, ok := collectRenderCluster(copyCells(term), term.Cols(), 0, 0)
	if !ok {
		t.Fatalf("expected tag flag emoji cluster")
	}
	if cluster.Text != england || cluster.CellSpan != 2 {
		t.Fatalf("cluster = %#v", cluster)
	}
}

func TestCollectRenderClusterRejectsPlainASCII(t *testing.T) {
	cells := []core.Cell{{Rune: 'A'}}
	if cluster, ok := collectRenderCluster(cells, 1, 0, 0); ok {
		t.Fatalf("plain ASCII should not be pre-shaped, got %#v", cluster)
	}
}

// TestCollectRenderClusterFastPathsBareASCII pins the render-path selection the
// allocation fix depends on: bare digits/#/* (emoji candidates, but no combining
// marks) take the fast per-rune path, while a keycap sequence — whose marks
// attach as width-0 combining — must still cluster.
func TestCollectRenderClusterFastPathsBareASCII(t *testing.T) {
	for _, r := range []rune{'0', '5', '9', '#', '*'} {
		cells := []core.Cell{{Rune: r}, {Rune: 'x'}}
		if cluster, ok := collectRenderCluster(cells, 2, 0, 0); ok {
			t.Fatalf("bare %q should use the fast per-rune path, got cluster %#v", r, cluster)
		}
	}
	keycap := []core.Cell{core.NewCellWithCombining('1', core.Attr{}, '️', '⃣')}
	if _, ok := collectRenderCluster(keycap, 1, 0, 0); !ok {
		t.Fatalf("keycap sequence (digit + U+20E3) must still cluster")
	}
}

// copyCells returns a defensive copy of the current screen cells for assertions
// (replaces the removed core.Terminal.Cells() accessor).
func copyCells(t *core.Terminal) []core.Cell {
	c := make([]core.Cell, t.Cols()*t.Rows())
	t.CopyView(c)
	return c
}
