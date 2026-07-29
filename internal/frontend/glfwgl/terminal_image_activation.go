//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"

	"cervterm/internal/config"
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

func imagesEnabled(graphics config.GraphicsConfig) bool {
	return graphics.Kitty.Enabled || graphics.Sixel.Enabled || graphics.ITerm.Enabled
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
	if !imagesEnabled(a.cfg.Graphics) {
		return options
	}
	limits := a.cfg.Graphics.Limits
	options.ImageLimits = &termimage.Limits{
		EncodedBytes: limits.EncodedBytesPerPane,
		DecodedBytes: limits.DecodedBytesPerPane,
		Images:       limits.ImageCountPerPane,
		Placements:   limits.PlacementCountPerPane,
	}
	options.KittyEnabled = a.cfg.Graphics.Kitty.Enabled
	options.SixelEnabled = a.cfg.Graphics.Sixel.Enabled
	options.ITermEnabled = a.cfg.Graphics.ITerm.Enabled
	return options
}

// initMux creates the process owner locally, then transfers its two narrow
// interfaces to the process controller. App retains only its attested window
// capability; restored startup has no projection capability until bind.
func (a *App) initMux() error {
	if a == nil {
		return errors.New("initialize mux: nil app")
	}
	if a.host == nil || a.host.services.commands != nil || a.host.services.windowCapabilities != nil || a.mux != nil {
		return errors.New("initialize mux: already initialized")
	}
	owner := termmux.NewOwner(nil, a.muxOptions())
	if err := owner.ImageSetupError(); err != nil {
		return errors.Join(fmt.Errorf("initialize mux images: %w", err), owner.Shutdown())
	}
	if err := owner.SetPaletteBase(configuredPaletteBase(a.cfg.Colors)); err != nil {
		return errors.Join(err, owner.Shutdown())
	}
	if err := a.host.installProcessServices(owner, owner); err != nil {
		return errors.Join(err, owner.Shutdown())
	}
	if a.windowID == 0 {
		return nil
	}
	projection := a.host.windows[a.windowID]
	if projection == nil || projection.app != a {
		return fmt.Errorf("initialize mux: %w", errWindowProjectionMissing)
	}
	window, identity, err := a.host.acquireWindowCapability(a.windowID, a, projection.host)
	if err != nil {
		return err
	}
	a.mux = window
	a.windowIdentity = identity
	delete(a.host.boundOrigins, identity)
	return nil
}

func (a *App) rollbackInitializedMux(cause error) error {
	if a == nil || a.host == nil || a.host.services.commands == nil {
		return cause
	}
	if a.host.primary != a {
		return errors.Join(cause, termmux.ErrWrongOwner)
	}
	projectionErr := a.host.closeProjectionLoop()
	if a.host.inLoop {
		return errors.Join(cause, projectionErr)
	}
	shutdownErr := a.host.shutdownServices(a)
	if a.host.processClosed() {
		a.mux = nil
		a.windowIdentity = termmux.WindowIdentity{}
	}
	return errors.Join(cause, projectionErr, shutdownErr)
}

// prepareTerminalImageCache creates one projection/context-local cache. Callers
// invoke it only after the projection renderer and atlas exist with the owning
// GL context current, then register close immediately in acquisition order.
func (a *App) prepareTerminalImageCache() error {
	if a == nil {
		return errors.New("prepare terminal image cache: nil app")
	}
	if !imagesEnabled(a.cfg.Graphics) {
		return nil
	}
	if a.terminalImageCache != nil {
		return errors.New("prepare terminal image cache: already initialized")
	}
	renderer, ok := a.r.(gpu.TerminalImageRenderer)
	if !ok {
		return errors.New("prepare terminal image cache: renderer capability is unavailable")
	}
	if a.mux == nil && (a.controller == nil || !a.controller.processReady()) {
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
		if a.mux == nil {
			return termimage.DetachedResource{}, false
		}
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
		return err
	}
	if err := commit(); err != nil {
		return errors.Join(err, a.closeTerminalImageCache())
	}
	return nil
}
