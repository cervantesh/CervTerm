//go:build glfw

package glfwgl

import (
	"log"
	"time"

	"cervterm/internal/bellpolicy"
	termmux "cervterm/internal/mux"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type bellEffectSink interface {
	Audible() error
	Taskbar()
}

type bellState struct {
	bellGate        *bellpolicy.Gate
	bellSink        bellEffectSink
	bellVisualUntil time.Time
	bellDelivered   map[termmux.PaneID]int
}

type glfwBellEffectSink struct{ window *glfw.Window }

func (s glfwBellEffectSink) Audible() error { return nativeAudibleBell() }
func (s glfwBellEffectSink) Taskbar() {
	if s.window != nil {
		s.window.RequestAttention()
	}
}

func (a *App) deliverBell(pane termmux.PaneID, effect bool) {
	if a.bellDelivered == nil {
		a.bellDelivered = make(map[termmux.PaneID]int)
	}
	a.bellDelivered[pane]++
	a.ensureScriptLifecycleController().bell(a, a, a, pane)
	if effect {
		a.applyBellEffect()
	}
}

func (a *App) deliverBellEvent(pane termmux.PaneID) {
	if a.mux != nil {
		if view, ok := a.mux.PaneView(pane); ok && a.bellDelivered[pane] >= view.Snapshot.BellCount {
			return
		}
	}
	a.deliverBell(pane, true)
}

// catchUpBellEvents reconstructs bounded controller events dropped before a
// native projection existed from the pane-local monotonic terminal counter.
func (a *App) catchUpBellEvents() {
	if a.mux == nil {
		return
	}
	for _, window := range a.mux.Windows() {
		if a.windowID != 0 && window.ID != a.windowID {
			continue
		}
		for _, tab := range window.Tabs {
			for _, pane := range tab.Panes {
				view, ok := a.mux.PaneView(pane)
				if !ok {
					continue
				}
				for a.bellDelivered[pane] < view.Snapshot.BellCount {
					a.deliverBell(pane, false)
				}
			}
		}
	}
}

func (a *App) applyBellEffect() {
	if a.bellSink == nil && a.window != nil {
		a.bellSink = glfwBellEffectSink{window: a.window}
	}
	focused := a.window != nil && a.window.GetAttrib(glfw.Focused) == glfw.True
	a.applyBellEffectWithFocus(focused)
}

func (a *App) applyBellEffectWithFocus(focused bool) {
	if a.bellGate == nil {
		a.bellGate = bellpolicy.NewGate(nil)
	}
	cfg := bellpolicy.Config{
		Mode:      bellpolicy.Mode(a.cfg.Bell.Mode),
		Unfocused: a.cfg.Bell.Focus == "unfocused",
		Throttle:  time.Duration(a.cfg.Bell.ThrottleMS) * time.Millisecond,
		VisualFor: time.Duration(a.cfg.Bell.VisualDurationMS) * time.Millisecond,
	}
	decision, ok := a.bellGate.Decide(cfg, focused)
	if !ok {
		return
	}
	switch decision.Mode {
	case bellpolicy.Audible:
		if a.bellSink != nil {
			if err := a.bellSink.Audible(); err != nil {
				log.Printf("audible bell unavailable: %v", err)
			}
		}
	case bellpolicy.Visual:
		a.bellVisualUntil = decision.At.Add(decision.VisualFor)
		a.requestRedraw()
	case bellpolicy.Taskbar:
		if a.bellSink != nil {
			a.bellSink.Taskbar()
		}
	}
}
