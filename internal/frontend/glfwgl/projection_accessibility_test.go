//go:build glfw

package glfwgl

import (
	"errors"
	"reflect"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type fakeProjectionAccessibilityLifecycle struct {
	log *[]string
	err error
}

func (lifecycle *fakeProjectionAccessibilityLifecycle) Close() error {
	*lifecycle.log = append(*lifecycle.log, "accessibility")
	return lifecycle.err
}

func TestProjectionAccessibilityFactoryDormantTransferAndOrder(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	var log []string
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		log = append(log, "prepare")
		return &fakeProjectionAccessibilityLifecycle{log: &log}, nil
	}
	before := &compositionBeforeUnbind{
		cancel:     func() error { log = append(log, "cancel"); return nil },
		deactivate: func() error { log = append(log, "deactivate"); return nil },
		restore:    func() error { log = append(log, "restore"); return nil },
		release:    func() error { log = append(log, "release"); return nil },
	}
	if err := prepareProjectionAccessibility(&App{}, new(glfw.Window), before); err != nil {
		t.Fatal(err)
	}
	if err := before.close(); err != nil {
		t.Fatal(err)
	}
	want := []string{"prepare", "cancel", "accessibility", "deactivate", "restore", "release"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("order=%v want=%v", log, want)
	}
}

func TestProjectionAccessibilityFactoryFailureClosesPartial(t *testing.T) {
	original := projectionAccessibilityFactory
	defer func() { projectionAccessibilityFactory = original }()
	injected := errors.New("prepare")
	var log []string
	projectionAccessibilityFactory = func(*App, *glfw.Window, *compositionBeforeUnbind) (projectionAccessibilityLifecycle, error) {
		return &fakeProjectionAccessibilityLifecycle{log: &log}, injected
	}
	if err := prepareProjectionAccessibility(&App{}, new(glfw.Window), &compositionBeforeUnbind{}); !errors.Is(err, injected) {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(log, []string{"accessibility"}) {
		t.Fatalf("cleanup=%v", log)
	}
}
