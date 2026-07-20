//go:build glfw

package glfwgl

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	backgroundcore "cervterm/internal/background"
	"cervterm/internal/config"
)

func writeBackgroundPNG(t *testing.T, dir, name string, value color.RGBA) string {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			pixels.SetRGBA(x, y, value)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareBackgroundCPUResolvesRelativeImagesAndWatchesDigest(t *testing.T) {
	dir := t.TempDir()
	path := writeBackgroundPNG(t, dir, "wall.png", color.RGBA{R: 255, A: 255})
	cfg := config.Defaults()
	cfg.Background.Layers = []config.BackgroundLayer{
		{Kind: "linear_gradient", Opacity: 1, Colors: []string{"#000000", "#0000ff"}, Angle: 90},
		{Kind: "image", Opacity: 0.5, Path: "wall.png", Fit: "stretch", HorizontalAlign: "center", VerticalAlign: "center"},
	}
	prepared, err := prepareBackgroundCPU(cfg, dir, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.surface.Bounds() != image.Rect(0, 0, 4, 3) {
		t.Fatalf("bounds = %v", prepared.surface.Bounds())
	}
	canonical, _ := filepath.EvalSymlinks(path)
	canonical, _ = filepath.Abs(canonical)
	if len(prepared.watchPaths) != 1 || !strings.EqualFold(prepared.watchPaths[0], filepath.Clean(canonical)) {
		t.Fatalf("watch paths = %#v want %q", prepared.watchPaths, canonical)
	}
	if prepared.watchHashes[prepared.watchPaths[0]] == ([32]byte{}) {
		t.Fatal("missing digest")
	}
}

func TestBackgroundLayerBaseUsesWinningSource(t *testing.T) {
	records := []config.ProvenanceRecord{{Path: "background.layers", Winner: config.ProvenanceOrigin{CanonicalSource: filepath.Join("root", "theme", "config.lua")}}}
	if got := backgroundLayerBase(records, "fallback.lua"); got != filepath.Join("root", "theme") {
		t.Fatalf("base = %q", got)
	}
}

func TestLayeredBackgroundResizePublishesNewestGeneration(t *testing.T) {
	dir := t.TempDir()
	writeBackgroundPNG(t, dir, "wall.png", color.RGBA{G: 255, A: 255})
	cfg := config.Defaults()
	cfg.Background.Layers = []config.BackgroundLayer{{Kind: "image", Opacity: 1, Path: "wall.png", Fit: "stretch", HorizontalAlign: "center", VerticalAlign: "center"}}
	renderer := &replaceRecordingRenderer{}
	old := &recordingBackgroundSurface{}
	a := &App{cfg: cfg, r: renderer, backgroundSurface: old, configPath: filepath.Join(dir, "cervterm.lua")}
	a.requestBackgroundResize(8, 6)
	deadline := time.Now().Add(2 * time.Second)
	for a.backgroundSurfaceWidth != 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		a.applyPreparedBackgroundResize()
	}
	if a.backgroundSurfaceWidth != 8 || a.backgroundSurfaceHeight != 6 || a.backgroundSurface != renderer.preparedSurface {
		t.Fatalf("resize state = %dx%d %#v", a.backgroundSurfaceWidth, a.backgroundSurfaceHeight, a.backgroundSurface)
	}
	if old.closed != 1 {
		t.Fatalf("old close count = %d", old.closed)
	}
	firstGeneration := a.backgroundGeneration
	a.contentScaleX, a.contentScaleY = 2, 2
	a.requestBackgroundResize(8, 6)
	if a.backgroundGeneration == firstGeneration {
		t.Fatal("DPI-only change reused stale background generation")
	}
	deadline = time.Now().Add(2 * time.Second)
	for a.configReloadAsync.resizeWorkers != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		a.applyPreparedBackgroundResize()
	}
	if a.configReloadAsync.activeBackgroundDPI != 192 {
		t.Fatalf("active DPI = %v", a.configReloadAsync.activeBackgroundDPI)
	}
}

func TestBackgroundGPUTransferBudgetCountsActiveAndCandidate(t *testing.T) {
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	if bytes, err := backgroundGPUTransferBytes(0, pixel); err != nil || bytes != 4 {
		t.Fatalf("single pixel bytes=%d err=%v", bytes, err)
	}
	if _, err := backgroundGPUTransferBytes(backgroundcore.MaxAggregateGPUBytes-3, pixel); err == nil {
		t.Fatal("active plus candidate GPU residency exceeded aggregate budget")
	}
}

func TestInitialBackgroundDependenciesJoinStartupFreshnessAndWatchSet(t *testing.T) {
	dir := t.TempDir()
	imagePath := writeBackgroundPNG(t, dir, "startup.png", color.RGBA{R: 255, A: 255})
	cfg := config.Defaults()
	cfg.Background.Layers = []config.BackgroundLayer{{Kind: "image", Opacity: 1, Path: "startup.png", Fit: "stretch", HorizontalAlign: "center", VerticalAlign: "center"}}
	prepared, err := prepareBackgroundCPU(cfg, dir, 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	primary := filepath.Join(dir, "cervterm.lua")
	app := &App{configWatch: newConfigWatchState(primary), configWatchHashes: make(map[string][32]byte)}
	app.registerInitialBackgroundDependencies(prepared)
	found := false
	for _, watched := range app.configWatch.activePaths {
		found = found || strings.EqualFold(watched, imagePath)
	}
	if !found {
		t.Fatalf("startup active paths = %v", app.configWatch.activePaths)
	}
	if watchHashesChanged(app.configWatchHashes) {
		t.Fatal("unchanged startup image reported stale")
	}
	writeBackgroundPNG(t, dir, "startup.png", color.RGBA{G: 255, A: 255})
	if !watchHashesChanged(app.configWatchHashes) {
		t.Fatal("startup image mutation escaped pre-Teal freshness gate")
	}
	base := time.Date(2026, 7, 18, 3, 0, 0, 0, time.UTC)
	if app.configWatch.poll(base) {
		t.Fatal("startup image watcher fired before debounce")
	}
	if !app.configWatch.poll(base.Add(300 * time.Millisecond)) {
		t.Fatal("startup image edit did not queue reload")
	}
}
