//go:build glfw

package glfwgl

import "errors"

var (
	errBlurUnsupported                   = errors.New("background blur is unsupported on this platform")
	errBlurIncompatible                  = errors.New("background blur is incompatible with the transparent framebuffer")
	errBlurRequiresTranslucentBackground = errors.New("background blur requires a translucent background")
	errBlurTransparentFramebufferMissing = errors.New("background blur requires an available transparent framebuffer")
)

// BlurStatus describes the result of applying the current blur request.
type BlurStatus uint8

const (
	BlurDisabled BlurStatus = iota
	BlurActive
	BlurUnsupported
	BlurIncompatible
	BlurFailed
)

func (s BlurStatus) String() string {
	switch s {
	case BlurDisabled:
		return "disabled"
	case BlurActive:
		return "active"
	case BlurUnsupported:
		return "unsupported"
	case BlurIncompatible:
		return "incompatible"
	case BlurFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// BlurRequest contains only compositor-facing appearance state. Providers must
// not inspect terminal configuration or mutate renderer state.
type BlurRequest struct {
	Enabled                         bool
	TranslucentBackground           bool
	TransparentFramebufferAvailable bool
}

// BlurResult reports the effective provider state after Apply returns.
type BlurResult struct {
	Status BlurStatus
	Err    error
}

// BlurProvider owns one platform blur implementation. All methods are called
// on the locked GLFW/OS thread; implementations must degrade without panicking.
type BlurProvider interface {
	Name() string
	Apply(BlurRequest) BlurResult
	Close() error
}

type blurCompatibility func(BlurRequest) error

type nativeBlurProvider struct {
	name          string
	set           func(bool) error
	compatibility blurCompatibility
}

func (p *nativeBlurProvider) Name() string { return p.name }

func windowsMaterialCompatibility(request BlurRequest) error {
	if request.TranslucentBackground {
		return errBlurIncompatible
	}
	return nil
}

func transparentCompositorCompatibility(request BlurRequest) error {
	if !request.TranslucentBackground {
		return errBlurRequiresTranslucentBackground
	}
	if !request.TransparentFramebufferAvailable {
		return errBlurTransparentFramebufferMissing
	}
	return nil
}

func (p *nativeBlurProvider) compatibilityError(request BlurRequest) error {
	if p.compatibility != nil {
		return p.compatibility(request)
	}
	// Preserve the existing Windows constructor atomically. New providers always
	// set an explicit policy; nil remains the legacy Windows material policy.
	return windowsMaterialCompatibility(request)
}

func blurErrorResult(err error) BlurResult {
	status := BlurFailed
	if errors.Is(err, errBlurUnsupported) {
		status = BlurUnsupported
	}
	return BlurResult{Status: status, Err: err}
}

func (p *nativeBlurProvider) Apply(request BlurRequest) BlurResult {
	if !request.Enabled {
		if err := p.set(false); err != nil {
			return blurErrorResult(err)
		}
		return BlurResult{Status: BlurDisabled}
	}
	if err := p.compatibilityError(request); err != nil {
		if disableErr := p.set(false); disableErr != nil {
			return blurErrorResult(disableErr)
		}
		return BlurResult{Status: BlurIncompatible, Err: err}
	}
	if err := p.set(true); err != nil {
		return blurErrorResult(err)
	}
	return BlurResult{Status: BlurActive}
}

func (p *nativeBlurProvider) Close() error { return p.set(false) }

type failedBlurProvider struct {
	name string
	err  error
}

func (p failedBlurProvider) Name() string { return p.name }

func (p failedBlurProvider) Apply(request BlurRequest) BlurResult {
	if !request.Enabled {
		return BlurResult{Status: BlurDisabled}
	}
	return blurErrorResult(p.err)
}

func (failedBlurProvider) Close() error { return nil }

type unsupportedBlurProvider struct {
	name string
}

func (p unsupportedBlurProvider) Name() string { return p.name }

func (p unsupportedBlurProvider) Apply(request BlurRequest) BlurResult {
	if request.Enabled {
		return BlurResult{Status: BlurUnsupported, Err: errBlurUnsupported}
	}
	return BlurResult{Status: BlurDisabled}
}

func (unsupportedBlurProvider) Close() error { return nil }
