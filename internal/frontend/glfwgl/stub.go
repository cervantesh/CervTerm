//go:build !glfw

package glfwgl

import "errors"

// Run is unavailable in headless builds. Use `-tags glfw` to compile the
// optional GLFW/OpenGL frontend.
func Run() error {
	return errors.New("glfw frontend requires building with -tags glfw")
}

type glyphAtlas struct{}

func newGlyphAtlas() (*glyphAtlas, error) { return &glyphAtlas{}, nil }

func (a *glyphAtlas) drawRune(r rune, x, y, scale float32) {}
