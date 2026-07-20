//go:build glfw

package glfwgl

import (
	"log"
	"time"

	"cervterm/internal/config"
	"cervterm/internal/core"
	"cervterm/internal/notificationpolicy"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type notificationEffectSink interface {
	Notify(title, body string) error
	Close() error
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
		a.notificationState.sink = newPlatformNotificationEffectSink(a.window)
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

func (a *App) closeNotificationEffectSink() error {
	if a.notificationState.sink == nil {
		return nil
	}
	err := a.notificationState.sink.Close()
	if err == nil {
		a.notificationState.sink = nil
	}
	return err
}

func (a *App) applyNotificationConfigChange(old config.NotificationConfig) {
	if old == a.cfg.Notification {
		return
	}
	if a.notificationState.gate != nil {
		a.notificationState.gate.Reset()
	}
	var cleanupErr error
	if old.Enabled && !a.cfg.Notification.Enabled {
		cleanupErr = a.closeNotificationEffectSink()
	}
	a.notificationState.unsupportedWarned = false
	if cleanupErr != nil {
		a.notificationState.unsupportedWarned = true
		log.Print("native notification cleanup failed")
	}
}
