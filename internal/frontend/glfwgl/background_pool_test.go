//go:build glfw

package glfwgl

import (
	"image/color"
	"path/filepath"
	"sync"
	"testing"

	"cervterm/internal/config"
)

func TestBackgroundResourcePoolPinsSharedSourceAndReleasesCompositions(t *testing.T) {
	dir := t.TempDir()
	writeBackgroundPNG(t, dir, "shared.png", color.RGBA{R: 255, A: 255})
	cfg := config.Defaults()
	layer := config.BackgroundLayer{Kind: "image", Opacity: 1, Path: "shared.png", Fit: "stretch", HorizontalAlign: "center", VerticalAlign: "center"}
	cfg.Background.Layers = []config.BackgroundLayer{layer, layer}
	pool := newBackgroundResourcePool()
	first, err := pool.prepare(cfg, dir, 8, 6, 96)
	if err != nil {
		t.Fatal(err)
	}
	if pool.cache.ResidentBytes() == 0 || pool.composedBytes == 0 {
		t.Fatalf("residency cache=%d composed=%d", pool.cache.ResidentBytes(), pool.composedBytes)
	}
	second, err := pool.prepare(cfg, dir, 8, 6, 96)
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	first.Close()
	second.Close()
	if pool.composedBytes != 0 {
		t.Fatalf("composition residency after release = %d", pool.composedBytes)
	}
	if err := pool.close(); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundResourcePoolConcurrentGenerationsAreBoundedAndRaceSafe(t *testing.T) {
	dir := t.TempDir()
	writeBackgroundPNG(t, dir, "shared.png", color.RGBA{G: 255, A: 255})
	cfg := config.Defaults()
	cfg.Background.Layers = []config.BackgroundLayer{{Kind: "image", Opacity: 1, Path: filepath.Base("shared.png"), Fit: "contain", HorizontalAlign: "center", VerticalAlign: "center"}}
	pool := newBackgroundResourcePool()
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prepared, err := pool.prepare(cfg, dir, 16, 12, 120)
			if err == nil {
				prepared.Close()
			}
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if pool.composedBytes != 0 {
		t.Fatalf("composition residency after concurrent release = %d", pool.composedBytes)
	}
	if err := pool.close(); err != nil {
		t.Fatal(err)
	}
}
