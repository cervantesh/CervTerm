//go:build glfw

package glfwgl

import (
	"log"

	"cervterm/internal/config"
)

func effectiveTextRasterFor(cfg config.Config) string {
	if cfg.EffectiveBackgroundAlpha() < 0xff && cfg.Render.TextRaster == "subpixel" {
		return "go"
	}
	return cfg.Render.TextRaster
}

func (a *App) effectiveTextRaster() string { return effectiveTextRasterFor(a.cfg) }

// applyWindowAppearance runs only on the GLFW thread. Config validation has
// already made native opacity and transparent-background alpha mutually
// exclusive, so this function never invokes GLFW's undefined combination.
func (a *App) applyWindowAppearance() {
	if a.window == nil {
		return
	}
	translucentBackground := a.cfg.EffectiveBackgroundAlpha() < 0xff
	if translucentBackground {
		a.window.SetOpacity(1)
		if !a.transparentFramebuffer && !a.transparencyWarned {
			a.transparencyWarned = true
			log.Printf("transparent framebuffer unavailable; background alpha may render opaque")
			a.Notify("transparent framebuffer unavailable; using compositor fallback")
		}
	} else {
		a.window.SetOpacity(float32(a.cfg.Window.Opacity))
	}

	provider := a.blurProvider
	if provider == nil {
		provider = unsupportedBlurProvider{name: "uninitialized"}
	}
	result := provider.Apply(BlurRequest{
		Enabled:                         a.cfg.Window.Blur,
		TranslucentBackground:           translucentBackground,
		TransparentFramebufferAvailable: a.transparentFramebuffer,
	})
	a.blurProviderName = provider.Name()
	a.blurStatus = result.Status
	if result.Status == BlurDisabled || result.Status == BlurActive {
		a.blurWarned = false
		return
	}
	if a.blurWarned && a.blurWarnedStatus == result.Status {
		return
	}

	a.blurWarned = true
	a.blurWarnedStatus = result.Status
	if result.Status == BlurIncompatible {
		log.Printf("background blur disabled: %v", result.Err)
	} else {
		log.Printf("background blur unavailable from %s (%s): %v", provider.Name(), result.Status, result.Err)
	}
	if translucentBackground {
		a.Notify("background blur unavailable; transparency remains active")
	} else {
		a.Notify("background blur unavailable")
	}
}
