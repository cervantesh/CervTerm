//go:build glfw

package glfwgl

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestProjectCandidateRectUsesIndependentAxesAndClamps(t *testing.T) {
	rect, err := projectCandidateRect(framebufferCaretRect{x: 100, y: 60, width: 4, height: 20}, 200, 120, 100, 40)
	if err != nil {
		t.Fatal(err)
	}
	if want := (nativeCandidateRect{X: 50, Y: 20, Width: 2, Height: 7}); rect != want {
		t.Fatalf("rect=%#v want=%#v", rect, want)
	}

	rect, err = projectCandidateRect(framebufferCaretRect{x: -5, y: -3, width: 20, height: 12}, 200, 100, 100, 50)
	if err != nil || rect.X != 0 || rect.Y != 0 || rect.Width != 8 || rect.Height != 5 {
		t.Fatalf("clamped=%#v err=%v", rect, err)
	}
}

func TestProjectCandidateRectRejectsDegenerateAndOverflow(t *testing.T) {
	for _, test := range []struct {
		name                 string
		rect                 framebufferCaretRect
		fbW, fbH, winW, winH int
	}{
		{name: "zero framebuffer", rect: framebufferCaretRect{width: 1, height: 1}, fbH: 1, winW: 1, winH: 1},
		{name: "zero window", rect: framebufferCaretRect{width: 1, height: 1}, fbW: 1, fbH: 1, winH: 1},
		{name: "nan", rect: framebufferCaretRect{x: math.NaN(), width: 1, height: 1}, fbW: 1, fbH: 1, winW: 1, winH: 1},
		{name: "infinite edge", rect: framebufferCaretRect{x: math.MaxFloat64, width: math.MaxFloat64, height: 1}, fbW: 1, fbH: 1, winW: 1, winH: 1},
		{name: "outside", rect: framebufferCaretRect{x: 2, y: 2, width: 1, height: 1}, fbW: 1, fbH: 1, winW: 1, winH: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := projectCandidateRect(test.rect, test.fbW, test.fbH, test.winW, test.winH); !errors.Is(err, errCandidateGeometryInvalid) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestPreeditCaretFramebufferRectUsesVisualCaretAndMetrics(t *testing.T) {
	presentation := preparePreeditPresentation(activePreedit("שלום", 4, 0, 4), 20)
	rect, ok := preeditCaretFramebufferRect(presentation, 30, 40, 9, 18, 2)
	if !ok || rect != (framebufferCaretRect{x: 30, y: 40, width: 2, height: 18}) {
		t.Fatalf("rect=%#v ok=%v", rect, ok)
	}
	if _, ok := preeditCaretFramebufferRect(preeditPresentation{}, 0, 0, 9, 18, 2); ok {
		t.Fatal("inactive presentation produced geometry")
	}
}

func TestCandidateGeometryPublisherPublishesOnlyChangesAndRetriesFailure(t *testing.T) {
	var published []nativeCandidateRect
	clears := 0
	fail := true
	var publisher candidateGeometryPublisher
	if err := publisher.setCallbacks(func(rect nativeCandidateRect) error {
		if fail {
			fail = false
			return errors.New("publish failed")
		}
		published = append(published, rect)
		return nil
	}, func() error { clears++; return nil }); err != nil {
		t.Fatal(err)
	}
	rect := nativeCandidateRect{X: 1, Y: 2, Width: 3, Height: 4}
	if err := publisher.publishChanged(rect); err == nil {
		t.Fatal("failed publication succeeded")
	}
	if err := publisher.publishChanged(rect); err != nil {
		t.Fatal(err)
	}
	if err := publisher.publishChanged(rect); err != nil || len(published) != 1 {
		t.Fatalf("duplicate publication=%#v err=%v", published, err)
	}
	publisher.invalidate()
	if err := publisher.publishChanged(rect); err != nil || len(published) != 2 {
		t.Fatalf("invalidated publication=%#v err=%v", published, err)
	}
	if err := publisher.hide(); err != nil || clears != 1 {
		t.Fatalf("hide clears=%d err=%v", clears, err)
	}
	if err := publisher.hide(); err != nil || clears != 1 || !reflect.DeepEqual(published, []nativeCandidateRect{rect, rect}) {
		t.Fatalf("second hide clears=%d published=%#v", clears, published)
	}
}

func TestCandidateGeometryPublisherClearsOldSinkBeforeReplacement(t *testing.T) {
	var publisher candidateGeometryPublisher
	clearAttempts := 0
	if err := publisher.setCallbacks(func(nativeCandidateRect) error { return nil }, func() error {
		clearAttempts++
		if clearAttempts == 1 {
			return errors.New("clear failed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := publisher.publishChanged(nativeCandidateRect{Width: 1, Height: 1}); err != nil {
		t.Fatal(err)
	}
	newPublishes := 0
	if err := publisher.setCallbacks(func(nativeCandidateRect) error { newPublishes++; return nil }, nil); err == nil {
		t.Fatal("replacement ignored old clear failure")
	}
	if err := publisher.setCallbacks(func(nativeCandidateRect) error { newPublishes++; return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if err := publisher.publishChanged(nativeCandidateRect{Width: 2, Height: 2}); err != nil || clearAttempts != 2 || newPublishes != 1 {
		t.Fatalf("clearAttempts=%d newPublishes=%d err=%v", clearAttempts, newPublishes, err)
	}
}
