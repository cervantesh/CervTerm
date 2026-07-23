//go:build glfw

package glfwgl

import (
	"os"
	"path/filepath"
	"testing"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
	"cervterm/internal/termimage"
)

func TestEmptyPaneOutputKeepsActivityButSkipsLuaOutputCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cervterm.lua")
	source := `local n=0; return { events={ output=function(term, data) n=n+1; term:notify(tostring(n)..":"..data) end } }`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtime, err := script.Load(path, config.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	limits := termimage.DefaultLimits()
	m := termmux.New(failingTestFactory{}, termmux.Options{ImageLimits: &limits, KittyEnabled: true})
	_, pane, bootstrapEvents, _ := m.Bootstrap(termmux.SpawnSpec{}, termmux.PixelRect{Width: 20, Height: 4}, termmux.CellMetrics{CellWidth: 1, CellHeight: 1})
	t.Cleanup(func() { _ = m.Shutdown() })
	app := &App{
		cfg: cfg, scriptRT: runtime, mux: m, focusedPane: pane,
		paneUI: make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int),
		cellW: 1, cellH: 1,
	}
	app.handleMuxEvents(bootstrapEvents)
	app.syncFocusedProjection()

	private := []byte("\x1b_GPRIVATE_MARKER\x1b\\")
	beforeBytes := app.meter.Snapshot().Bytes
	events, err := m.FeedFallback(pane, private)
	if err != nil {
		t.Fatal(err)
	}
	foundEmpty := false
	for _, event := range events {
		if event.Kind == termmux.PaneOutput {
			foundEmpty = len(event.Data) == 0
		}
	}
	if !foundEmpty || !app.applyMuxEvents(events) {
		t.Fatalf("selected frame did not produce consumed empty activity event: %#v", events)
	}
	if app.notice != "" {
		t.Fatalf("empty PaneOutput fired Lua callback: notice=%q", app.notice)
	}
	if got := app.meter.Snapshot().Bytes - beforeBytes; got != uint64(len(private)) {
		t.Fatalf("bytes-read metric=%d want raw ingress=%d", got, len(private))
	}
	events, err = m.FeedFallback(pane, []byte("public"))
	if err != nil {
		t.Fatal(err)
	}
	app.applyMuxEvents(events)
	if app.notice != "1:public" {
		t.Fatalf("nonempty public output callback notice=%q", app.notice)
	}
}
