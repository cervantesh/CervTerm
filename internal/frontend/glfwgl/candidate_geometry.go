//go:build glfw

package glfwgl

import (
	"errors"
	"math"
)

var errCandidateGeometryInvalid = errors.New("candidate geometry is invalid")
var errCandidateGeometryCallbackPanic = errors.New("candidate geometry callback panic")

type framebufferCaretRect struct {
	x, y, width, height float64
}

type nativeCandidateRect struct {
	X, Y, Width, Height int
}

func preeditCaretFramebufferRect(presentation preeditPresentation, x, y, cellWidth, cellHeight, caretWidth float32) (framebufferCaretRect, bool) {
	if !presentation.active || presentation.cells <= 0 || cellWidth <= 0 || cellHeight <= 0 || caretWidth <= 0 {
		return framebufferCaretRect{}, false
	}
	caretX := float64(x + preeditCaretOffset(presentation, cellWidth, caretWidth))
	return framebufferCaretRect{x: caretX, y: float64(y), width: float64(caretWidth), height: float64(cellHeight)}, true
}

func projectCandidateRect(rect framebufferCaretRect, framebufferWidth, framebufferHeight, windowWidth, windowHeight int) (nativeCandidateRect, error) {
	if framebufferWidth <= 0 || framebufferHeight <= 0 || windowWidth <= 0 || windowHeight <= 0 ||
		!finitePositive(rect.width) || !finitePositive(rect.height) || !finite(rect.x) || !finite(rect.y) {
		return nativeCandidateRect{}, errCandidateGeometryInvalid
	}
	right, bottom := rect.x+rect.width, rect.y+rect.height
	if !finite(right) || !finite(bottom) {
		return nativeCandidateRect{}, errCandidateGeometryInvalid
	}
	ratioX := float64(windowWidth) / float64(framebufferWidth)
	ratioY := float64(windowHeight) / float64(framebufferHeight)
	left := math.Floor(rect.x * ratioX)
	top := math.Floor(rect.y * ratioY)
	projectedRight := math.Ceil(right * ratioX)
	projectedBottom := math.Ceil(bottom * ratioY)
	if !finite(left) || !finite(top) || !finite(projectedRight) || !finite(projectedBottom) {
		return nativeCandidateRect{}, errCandidateGeometryInvalid
	}
	left = min(float64(windowWidth), max(float64(0), left))
	top = min(float64(windowHeight), max(float64(0), top))
	projectedRight = min(float64(windowWidth), max(left, projectedRight))
	projectedBottom = min(float64(windowHeight), max(top, projectedBottom))
	if left > math.MaxInt || top > math.MaxInt || projectedRight > math.MaxInt || projectedBottom > math.MaxInt {
		return nativeCandidateRect{}, errCandidateGeometryInvalid
	}
	result := nativeCandidateRect{X: int(left), Y: int(top), Width: int(projectedRight - left), Height: int(projectedBottom - top)}
	if result.Width <= 0 || result.Height <= 0 {
		return nativeCandidateRect{}, errCandidateGeometryInvalid
	}
	return result, nil
}

func finite(value float64) bool         { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func finitePositive(value float64) bool { return finite(value) && value > 0 }

type candidateGeometryPublisher struct {
	publish    func(nativeCandidateRect) error
	clear      func() error
	last       nativeCandidateRect
	valid      bool
	wasVisible bool
	frame      uint64
	presented  uint64
}

func (publisher *candidateGeometryPublisher) setCallbacks(publish func(nativeCandidateRect) error, clear func() error) error {
	if publisher.wasVisible {
		if err := publisher.hide(); err != nil {
			return err
		}
	}
	publisher.publish = publish
	publisher.clear = clear
	publisher.valid = false
	publisher.wasVisible = false
	publisher.frame = 0
	publisher.presented = 0
	return nil
}

func (publisher *candidateGeometryPublisher) detachCallbacks() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.Join(err, errCandidateGeometryCallbackPanic)
		}
		publisher.publish, publisher.clear = nil, nil
		publisher.last = nativeCandidateRect{}
		publisher.valid, publisher.wasVisible = false, false
		publisher.frame, publisher.presented = 0, 0
	}()
	return publisher.hide()
}

func (publisher *candidateGeometryPublisher) publishChanged(rect nativeCandidateRect) error {
	if publisher.publish == nil {
		return nil
	}
	if publisher.valid && publisher.last == rect {
		return nil
	}
	if err := publisher.publish(rect); err != nil {
		return err
	}
	publisher.last = rect
	publisher.valid = true
	publisher.wasVisible = true
	return nil
}

func (publisher *candidateGeometryPublisher) beginFrame() {
	publisher.frame++
	if publisher.frame == 0 {
		publisher.frame = 1
		publisher.presented = 0
	}
}

func (publisher *candidateGeometryPublisher) markPresented() {
	publisher.presented = publisher.frame
}

func (publisher *candidateGeometryPublisher) presentedThisFrame() bool {
	return publisher.frame != 0 && publisher.presented == publisher.frame
}

func (publisher *candidateGeometryPublisher) invalidate() {
	publisher.valid = false
}

func (publisher *candidateGeometryPublisher) hide() error {
	publisher.valid = false
	if !publisher.wasVisible || publisher.clear == nil {
		publisher.wasVisible = false
		return nil
	}
	if err := publisher.clear(); err != nil {
		return err
	}
	publisher.wasVisible = false
	return nil
}
