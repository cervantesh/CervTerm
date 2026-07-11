//go:build glfw

package glfwgl

import (
	"testing"

	"cervterm/internal/input"

	"github.com/go-gl/glfw/v3.3/glfw"
)

func TestMouseModsFromGLFW(t *testing.T) {
	mods := mouseModsFromGLFW(glfw.ModShift | glfw.ModAlt | glfw.ModControl)
	want := input.ModShift | input.ModAlt | input.ModCtrl
	if mods != want {
		t.Fatalf("mouseModsFromGLFW = %v, want %v", mods, want)
	}
}
