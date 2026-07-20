//go:build glfw && !windows

package glfwgl

import (
	"errors"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type unsupportedNotificationEffectSink struct{}

func newPlatformNotificationEffectSink(*glfw.Window) notificationEffectSink {
	return unsupportedNotificationEffectSink{}
}

func (unsupportedNotificationEffectSink) Notify(string, string) error {
	return errors.New("native notification adapter unavailable")
}

func (unsupportedNotificationEffectSink) Close() error { return nil }
