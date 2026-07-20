//go:build glfw

package glfwgl

import (
	"errors"
	"log"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/notificationpolicy"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type notificationEffectSink interface {
	Notify(title, body string) error
}

type unsupportedNotificationEffectSink struct{}

func (unsupportedNotificationEffectSink) Notify(string, string) error {
	return errors.New("native notification adapter unavailable")
}

type notificationState struct {
	gate              *notificationpolicy.Gate
	sink              notificationEffectSink
	unsupportedWarned bool
	overflowWarned    bool
}

func (a *App) applyNotificationEffect(request core.NotificationRequest, fresh bool) {
	focused := a.window != nil && a.window.GetAttrib(glfw.Focused) == glfw.True
	a.applyNotificationEffectWithFocus(request, fresh, focused)
}

func (a *App) applyNotificationEffectWithFocus(request core.NotificationRequest, fresh, focused bool) {
	if a.notificationState.gate == nil {
		a.notificationState.gate = notificationpolicy.NewGate(nil)
	}
	config := notificationpolicy.Config{
		Enabled:     a.cfg.Notification.Enabled,
		Unfocused:   a.cfg.Notification.Focus == "unfocused",
		MinInterval: time.Duration(a.cfg.Notification.RateLimitMS) * time.Millisecond,
	}
	if !a.notificationState.gate.Allow(config, focused, fresh) {
		return
	}
	if a.notificationState.sink == nil {
		a.notificationState.sink = unsupportedNotificationEffectSink{}
	}
	if err := a.notificationState.sink.Notify(request.Title, request.Body); err != nil && !a.notificationState.unsupportedWarned {
		a.notificationState.unsupportedWarned = true
		log.Print("native notification unavailable")
	}
}

func (a *App) reportNotificationOverflow() {
	if a.notificationState.overflowWarned {
		return
	}
	a.notificationState.overflowWarned = true
	log.Print("notification request overflow; excess requests dropped")
}
