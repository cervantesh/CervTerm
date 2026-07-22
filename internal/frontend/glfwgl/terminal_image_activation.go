//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"

	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/termimage"
)

type terminalImageCacheFactory func(
	gpu.TerminalImageRenderer,
	terminalImageAcquire,
	terminalImageCacheLimits,
) (*terminalImageCache, error)

func defaultTerminalImageCacheFactory(
	renderer gpu.TerminalImageRenderer,
	acquire terminalImageAcquire,
	limits terminalImageCacheLimits,
) (*terminalImageCache, error) {
	return newTerminalImageCache(renderer, acquire, limits)
}

func (a *App) muxOptions() termmux.Options {
	historyCapacity := a.cfg.Scrolling.History
	hideCursorWhenScrolled := a.cfg.Scrolling.HideCursorWhenScrolled
	options := termmux.Options{
		ScrollbackCapacity:     &historyCapacity,
		HideCursorWhenScrolled: &hideCursorWhenScrolled,
		Wake:                   a.wakeMainLoop,
		SetClipboard: func(_ termmux.PaneID, text string) {
			if a.window != nil && a.cfg.Clipboard.OSC52 == "write" {
				a.window.SetClipboardString(text)
			}
		},
	}
	if !a.cfg.Graphics.Kitty.Enabled {
		return options
	}
	limits := a.cfg.Graphics.Limits
	options.ImageLimits = &termimage.Limits{
		EncodedBytes: limits.EncodedBytesPerPane,
		DecodedBytes: limits.DecodedBytesPerPane,
		Images:       limits.ImageCountPerPane,
		Placements:   limits.PlacementCountPerPane,
	}
	options.KittyEnabled = true
	return options
}

// initMux publishes the one process mux only after optional image limits have
// been validated and their process-owned budget/scheduler are usable.
func (a *App) initMux() error {
	if a == nil {
		return errors.New("initialize mux: nil app")
	}
	if a.mux != nil {
		return errors.New("initialize mux: already initialized")
	}
	candidate := termmux.New(nil, a.muxOptions())
	if err := candidate.ImageSetupError(); err != nil {
		return errors.Join(fmt.Errorf("initialize mux images: %w", err), candidate.Shutdown())
	}
	candidate.SetPaletteBase(configuredPaletteBase(a.cfg.Colors))
	a.mux = candidate
	return nil
}

func (a *App) rollbackInitializedMux(cause error) error {
	if a == nil || a.mux == nil {
		return cause
	}
	candidate := a.mux
	a.mux = nil
	return errors.Join(cause, candidate.Shutdown())
}

// prepareTerminalImageCache creates one projection/context-local cache. Callers
// invoke it only after the projection renderer and atlas exist with the owning
// GL context current, then register close immediately in acquisition order.
func (a *App) prepareTerminalImageCache() error {
	if a == nil {
		return errors.New("prepare terminal image cache: nil app")
	}
	if !a.cfg.Graphics.Kitty.Enabled {
		return nil
	}
	if a.terminalImageCache != nil {
		return errors.New("prepare terminal image cache: already initialized")
	}
	renderer, ok := a.r.(gpu.TerminalImageRenderer)
	if !ok {
		return errors.New("prepare terminal image cache: renderer capability is unavailable")
	}
	if a.mux == nil {
		return errors.New("prepare terminal image cache: mux is not initialized")
	}
	limits, err := validateTerminalImageCacheLimits(terminalImageCacheLimits{
		Entries: termimage.HardGPUEntriesPerContext,
		Bytes:   a.cfg.Graphics.Limits.GPUBytesPerContext,
	})
	if err != nil {
		return fmt.Errorf("prepare terminal image cache: %w", err)
	}
	factory := a.terminalImageCacheFactory
	if factory == nil {
		factory = defaultTerminalImageCacheFactory
	}
	cache, err := factory(renderer, func(key gpu.ImageTextureKey) (termimage.DetachedResource, bool) {
		return a.mux.AcquireImageResource(termmux.PaneID(key.PaneObject), key.Resource)
	}, limits)
	if err != nil {
		if cache != nil {
			err = errors.Join(err, cache.Close())
		}
		return fmt.Errorf("prepare terminal image cache: %w", err)
	}
	if cache == nil {
		return errors.New("prepare terminal image cache: factory returned nil cache")
	}
	a.terminalImageCache = cache
	return nil
}

func (a *App) activateInitialTerminalImages(commit func() error) error {
	if commit == nil {
		return errors.New("activate terminal images: nil commit")
	}
	if err := a.initMux(); err != nil {
		return err
	}
	if err := a.prepareTerminalImageCache(); err != nil {
		return a.rollbackInitializedMux(err)
	}
	if err := commit(); err != nil {
		closeErr := a.closeTerminalImageCache()
		return a.rollbackInitializedMux(errors.Join(err, closeErr))
	}
	return nil
}
