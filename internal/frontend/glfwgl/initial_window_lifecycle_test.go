package glfwgl

import (
	"errors"
	"reflect"
	"testing"
)

type fakeInitialWindow struct {
	log *[]string
}

func (w *fakeInitialWindow) Show()               { *w.log = append(*w.log, "show") }
func (w *fakeInitialWindow) Hide()               { *w.log = append(*w.log, "hide") }
func (w *fakeInitialWindow) MakeContextCurrent() { *w.log = append(*w.log, "current") }
func (w *fakeInitialWindow) Focus()              { *w.log = append(*w.log, "focus") }

type fakeInitialReveal struct {
	log           *[]string
	supported     bool
	concealErrors []error
	concealCalls  int
	restoreErr    error
}

func (r *fakeInitialReveal) Conceal() (bool, error) {
	*r.log = append(*r.log, "conceal")
	var err error
	if r.concealCalls < len(r.concealErrors) {
		err = r.concealErrors[r.concealCalls]
	}
	r.concealCalls++
	return r.supported, err
}

func (r *fakeInitialReveal) Restore() error {
	*r.log = append(*r.log, "restore")
	return r.restoreErr
}

type fakeInitialCompositor struct {
	log *[]string
	err error
}

func (c fakeInitialCompositor) Flush() error {
	*c.log = append(*c.log, "compose")
	return c.err
}

func TestInitialWindowLifecycleCloaksVisiblePresentationBeforeRevealAndFocus(t *testing.T) {
	var log []string
	reveal := &fakeInitialReveal{log: &log, supported: true}
	window, err := runInitialWindowLifecycle(
		func() (*fakeInitialWindow, error) {
			log = append(log, "create-hidden")
			return &fakeInitialWindow{log: &log}, nil
		},
		func(*fakeInitialWindow) error { log = append(log, "prepare"); return nil },
		func(*fakeInitialWindow) error { log = append(log, "first-present", "finish"); return nil },
		func(*fakeInitialWindow) error { log = append(log, "visible-present", "finish"); return nil },
		func(*fakeInitialWindow) initialWindowReveal { return reveal },
		fakeInitialCompositor{log: &log},
	)
	if err != nil || window == nil {
		t.Fatalf("window=%v err=%v", window, err)
	}
	want := []string{"create-hidden", "prepare", "first-present", "finish", "conceal", "show", "current", "visible-present", "finish", "compose", "restore", "focus"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("lifecycle order=%v want=%v", log, want)
	}
}

func TestInitialWindowLifecycleUnsupportedRevealUsesOrderedFallback(t *testing.T) {
	var log []string
	_, err := runInitialWindowLifecycle(
		func() (*fakeInitialWindow, error) {
			log = append(log, "create-hidden")
			return &fakeInitialWindow{log: &log}, nil
		},
		func(*fakeInitialWindow) error { log = append(log, "prepare"); return nil },
		func(*fakeInitialWindow) error { log = append(log, "first-present"); return nil },
		func(*fakeInitialWindow) error { log = append(log, "visible-present"); return nil },
		func(*fakeInitialWindow) initialWindowReveal { return unsupportedInitialWindowReveal{} },
		fakeInitialCompositor{log: &log},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"create-hidden", "prepare", "first-present", "show", "current", "visible-present", "compose", "focus"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("fallback order=%v want=%v", log, want)
	}
}

func TestInitialWindowLifecycleFailureHidesAfterShowAndNeverFocuses(t *testing.T) {
	stageErr := errors.New("stage failure")
	rollbackErr := errors.New("rollback failure")
	for _, test := range []struct {
		name          string
		prepareErr    error
		firstErr      error
		concealErrors []error
		visibleErr    error
		composeErr    error
		restoreErr    error
		want          []string
		wantErrs      []error
	}{
		{name: "prepare", prepareErr: stageErr, want: []string{"create-hidden", "prepare"}, wantErrs: []error{stageErr}},
		{name: "first present", firstErr: stageErr, want: []string{"create-hidden", "prepare", "first-present"}, wantErrs: []error{stageErr}},
		{name: "conceal", concealErrors: []error{stageErr}, want: []string{"create-hidden", "prepare", "first-present", "conceal"}, wantErrs: []error{stageErr}},
		{name: "visible present", visibleErr: stageErr, want: []string{"create-hidden", "prepare", "first-present", "conceal", "show", "current", "visible-present", "hide"}, wantErrs: []error{stageErr}},
		{name: "compositor", composeErr: stageErr, want: []string{"create-hidden", "prepare", "first-present", "conceal", "show", "current", "visible-present", "compose", "hide"}, wantErrs: []error{stageErr}},
		{name: "restore rollback", restoreErr: stageErr, want: []string{"create-hidden", "prepare", "first-present", "conceal", "show", "current", "visible-present", "compose", "restore", "conceal", "hide"}, wantErrs: []error{stageErr}},
		{name: "restore rollback double failure", concealErrors: []error{nil, rollbackErr}, restoreErr: stageErr, want: []string{"create-hidden", "prepare", "first-present", "conceal", "show", "current", "visible-present", "compose", "restore", "conceal", "hide"}, wantErrs: []error{stageErr, rollbackErr}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var log []string
			reveal := &fakeInitialReveal{
				log:           &log,
				supported:     true,
				concealErrors: test.concealErrors,
				restoreErr:    test.restoreErr,
			}
			window, err := runInitialWindowLifecycle(
				func() (*fakeInitialWindow, error) {
					log = append(log, "create-hidden")
					return &fakeInitialWindow{log: &log}, nil
				},
				func(*fakeInitialWindow) error { log = append(log, "prepare"); return test.prepareErr },
				func(*fakeInitialWindow) error { log = append(log, "first-present"); return test.firstErr },
				func(*fakeInitialWindow) error { log = append(log, "visible-present"); return test.visibleErr },
				func(*fakeInitialWindow) initialWindowReveal { return reveal },
				fakeInitialCompositor{log: &log, err: test.composeErr},
			)
			if window == nil {
				t.Fatal("lifecycle did not return the created window")
			}
			for _, wantErr := range test.wantErrs {
				if !errors.Is(err, wantErr) {
					t.Fatalf("err=%v does not include %v", err, wantErr)
				}
			}
			if !reflect.DeepEqual(log, test.want) {
				t.Fatalf("failure order=%v want=%v", log, test.want)
			}
			for _, event := range log {
				if event == "focus" {
					t.Fatal("failed lifecycle focused the window")
				}
			}
		})
	}
}

func TestInitialWindowLifecycleUnsupportedFailureHidesImmediately(t *testing.T) {
	stageErr := errors.New("visible present")
	var log []string
	_, err := runInitialWindowLifecycle(
		func() (*fakeInitialWindow, error) { return &fakeInitialWindow{log: &log}, nil },
		func(*fakeInitialWindow) error { return nil },
		func(*fakeInitialWindow) error { return nil },
		func(*fakeInitialWindow) error { log = append(log, "visible-present"); return stageErr },
		func(*fakeInitialWindow) initialWindowReveal { return unsupportedInitialWindowReveal{} },
		noopInitialWindowCompositor{},
	)
	if !errors.Is(err, stageErr) {
		t.Fatal(err)
	}
	want := []string{"show", "current", "visible-present", "hide"}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("fallback failure order=%v want=%v", log, want)
	}
}

func TestCreateWindowHiddenScopesVisibilityAndFocusHints(t *testing.T) {
	createErr := errors.New("create")
	for _, test := range []struct {
		name string
		err  error
	}{{name: "success"}, {name: "failure", err: createErr}} {
		t.Run(test.name, func(t *testing.T) {
			var log []string
			_, err := createWindowHidden(
				func(enabled bool) {
					if enabled {
						log = append(log, "hints-default")
					} else {
						log = append(log, "hidden-unfocused")
					}
				},
				func() (*fakeInitialWindow, error) { log = append(log, "create"); return &fakeInitialWindow{}, test.err },
			)
			if !errors.Is(err, test.err) {
				t.Fatalf("err=%v want=%v", err, test.err)
			}
			want := []string{"hidden-unfocused", "create", "hints-default"}
			if !reflect.DeepEqual(log, want) {
				t.Fatalf("hint order=%v want=%v", log, want)
			}
		})
	}
}
