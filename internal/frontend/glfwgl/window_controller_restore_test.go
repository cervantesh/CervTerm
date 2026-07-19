//go:build glfw

package glfwgl

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"

	"cervterm/internal/layoutrestore"
	"cervterm/internal/layoutstate"
	termmux "cervterm/internal/mux"
	"cervterm/internal/pty"
	"cervterm/internal/windowbounds"
)

type fakeRestoreProjectionFactory struct {
	log        *[]string
	failAt     int
	bindAt     int
	hosts      []*fakeNativeWindow
	geometries []termmux.RestoreWindowGeometry
	bindings   map[int]termmux.WindowID
}

func (f *fakeRestoreProjectionFactory) PrepareRestore(index int) (*nativeProjectionBundle, termmux.RestoreWindowGeometry, error) {
	*f.log = append(*f.log, fmt.Sprintf("prepare:%d", index))
	host := &fakeNativeWindow{id: fmt.Sprintf("restore-%d", index), log: f.log}
	f.hosts = append(f.hosts, host)
	bundle := &nativeProjectionBundle{host: host, handle: func([]termmux.Event) bool { return true }}
	bundle.bind = func(id termmux.WindowID) error {
		*f.log = append(*f.log, fmt.Sprintf("bind:%d:%d", index, id))
		if f.bindings == nil {
			f.bindings = make(map[int]termmux.WindowID)
		}
		f.bindings[index] = id
		if index == f.bindAt {
			return errors.New("injected bind failure")
		}
		return nil
	}
	bundle.unbind = func() error {
		*f.log = append(*f.log, fmt.Sprintf("unbind:%d", index))
		delete(f.bindings, index)
		return nil
	}
	bundle.resources = append(bundle.resources, projectionResourceFunc(func() error {
		*f.log = append(*f.log, fmt.Sprintf("close:%d", index))
		return nil
	}))
	geometry := termmux.RestoreWindowGeometry{Content: termmux.PixelRect{Width: 800 + index, Height: 480}, Metrics: termmux.CellMetrics{CellWidth: 8, CellHeight: 16}}
	f.geometries = append(f.geometries, geometry)
	if index == f.failAt {
		return bundle, geometry, errors.New("injected prepare failure")
	}
	return bundle, geometry, nil
}

func newRestoreProjectionController(t *testing.T, log *[]string) *windowController {
	t.Helper()
	controller := newWindowController(processServices{}, fakeNativePump{log: log})
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	return controller
}

func assertRestoreProjectionPristine(t *testing.T, controller *windowController, hosts []*fakeNativeWindow) {
	t.Helper()
	if controller.restorePending != nil || len(controller.windows) != 0 || len(controller.order) != 0 || controller.active != 0 || controller.current != 0 {
		t.Fatalf("controller leaked restore state: pending=%p windows=%v order=%v", controller.restorePending, controller.windows, controller.order)
	}
	for index, host := range hosts {
		if host.destroyed != 1 {
			t.Fatalf("host %d destroyed=%d", index, host.destroyed)
		}
	}
}

func TestWindowControllerRestorePreparationFailureClosesEveryHiddenCandidate(t *testing.T) {
	for failAt := 0; failAt < 3; failAt++ {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			var log []string
			factory := &fakeRestoreProjectionFactory{log: &log, failAt: failAt, bindAt: -1}
			controller := newRestoreProjectionController(t, &log)
			if candidate, err := controller.prepareRestoreProjections(factory, 3); candidate != nil || err == nil {
				t.Fatalf("candidate=%p err=%v", candidate, err)
			}
			assertRestoreProjectionPristine(t, controller, factory.hosts)
			for _, entry := range log {
				if len(entry) >= 5 && entry[:5] == "show:" {
					t.Fatalf("candidate became visible: %v", log)
				}
			}
		})
	}
}

func TestWindowControllerRestoreBindFailureRollsBackWholeBatch(t *testing.T) {
	var log []string
	factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: 1}
	controller := newRestoreProjectionController(t, &log)
	candidate, err := controller.prepareRestoreProjections(factory, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.publishRestoreProjections(candidate, []termmux.WindowID{2, 3, 4}); err == nil {
		t.Fatal("bind failure accepted")
	}
	assertRestoreProjectionPristine(t, controller, factory.hosts)
	if len(factory.bindings) != 0 {
		t.Fatalf("bindings leaked: %v", factory.bindings)
	}
}

func TestWindowControllerRestorePublishIsHiddenAndAbortable(t *testing.T) {
	var log []string
	factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: -1}
	controller := newRestoreProjectionController(t, &log)
	candidate, err := controller.prepareRestoreProjections(factory, 2)
	if err != nil {
		t.Fatal(err)
	}
	geometries, err := controller.restoreGeometries(candidate)
	if err != nil || !reflect.DeepEqual(geometries, factory.geometries) {
		t.Fatalf("geometries=%#v err=%v", geometries, err)
	}
	geometries[0].Content.Width = 1
	again, _ := controller.restoreGeometries(candidate)
	if again[0].Content.Width == 1 {
		t.Fatal("geometry slice aliases candidate")
	}
	if err := controller.publishRestoreProjections(candidate, []termmux.WindowID{2, 3}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controller.order, []termmux.WindowID{2, 3}) || len(controller.windows) != 2 || controller.windows[2].visible || controller.windows[3].visible || controller.active != 0 {
		t.Fatalf("published state order=%v windows=%v active=%d", controller.order, controller.windows, controller.active)
	}
	if err := controller.abortRestoreProjections(candidate, nil); err != nil {
		t.Fatal(err)
	}
	assertRestoreProjectionPristine(t, controller, factory.hosts)
}

func TestWindowControllerRestoreActivationCommitsOwnership(t *testing.T) {
	var log []string
	factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: -1}
	controller := newRestoreProjectionController(t, &log)
	candidate, err := controller.prepareRestoreProjections(factory, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.publishRestoreProjections(candidate, []termmux.WindowID{2, 3}); err != nil {
		t.Fatal(err)
	}
	if err := controller.activateRestoreProjections(candidate, nil); err != nil {
		t.Fatal(err)
	}
	if controller.restorePending != nil || !candidate.committed {
		t.Fatal("restore ownership not committed")
	}
	if err := controller.abortRestoreProjections(candidate, nil); !errors.Is(err, errRestoreProjectionTransaction) {
		t.Fatalf("abort committed=%v", err)
	}
	for _, id := range []termmux.WindowID{2, 3} {
		if err := controller.closeProjection(id); err != nil {
			t.Fatal(err)
		}
	}
}

type fakeRestoreWindows struct {
	log        *[]string
	prepareErr error
	idsErr     error
	commitErr  error
	aborts     int
	candidate  *termmux.RestoreCandidate
	geometries []termmux.RestoreWindowGeometry
}

func (f *fakeRestoreWindows) PrepareRestore(_ layoutrestore.Blueprint, geometries []termmux.RestoreWindowGeometry) (*termmux.RestoreCandidate, error) {
	*f.log = append(*f.log, "mux-prepare")
	f.geometries = append([]termmux.RestoreWindowGeometry(nil), geometries...)
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	f.candidate = &termmux.RestoreCandidate{}
	return f.candidate, nil
}
func (f *fakeRestoreWindows) RestoreWindowIDs(candidate *termmux.RestoreCandidate) ([]termmux.WindowID, error) {
	*f.log = append(*f.log, "mux-ids")
	if candidate != f.candidate {
		return nil, errors.New("wrong candidate")
	}
	if f.idsErr != nil {
		return nil, f.idsErr
	}
	return []termmux.WindowID{2, 3}, nil
}
func (f *fakeRestoreWindows) CommitRestore(candidate *termmux.RestoreCandidate) ([]termmux.Event, error) {
	*f.log = append(*f.log, "mux-commit")
	if candidate != f.candidate {
		return nil, errors.New("wrong candidate")
	}
	if f.commitErr != nil {
		return nil, f.commitErr
	}
	return []termmux.Event{{Kind: termmux.WindowActivated, Window: 2}}, nil
}
func (f *fakeRestoreWindows) AbortRestore(candidate *termmux.RestoreCandidate) error {
	*f.log = append(*f.log, "mux-abort")
	if candidate != f.candidate {
		return errors.New("wrong candidate")
	}
	f.aborts++
	return nil
}

func twoWindowRestoreBlueprint(t *testing.T) layoutrestore.Blueprint {
	t.Helper()
	launch := layoutstate.Launch{Program: "shell"}
	pane := func() layoutstate.Node { value := launch; return layoutstate.Node{Type: "pane", Launch: &value} }
	document := layoutstate.Document{Version: 1, Workspaces: []layoutstate.Workspace{{Name: "default", ActiveWindow: 0, Windows: []layoutstate.Window{
		{Title: "one", Bounds: layoutstate.Bounds{Width: 800, Height: 480}, ActiveTab: 0, Tabs: []layoutstate.Tab{{FocusedLeaf: 0, Root: pane()}}},
		{Title: "two", Bounds: layoutstate.Bounds{Width: 640, Height: 400}, ActiveTab: 0, Tabs: []layoutstate.Tab{{FocusedLeaf: 0, Root: pane()}}},
	}}}}
	plan, err := layoutstate.NewPlan(document)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, err := layoutrestore.Prepare(plan, layoutrestore.Options{
		DefaultLaunch: layoutrestore.Launch{Program: "shell"},
		Monitors:      []windowbounds.Monitor{{ID: "test", WorkArea: windowbounds.Rect{Width: 1920, Height: 1080}, ScaleX: 1, ScaleY: 1, Primary: true}},
		Policy:        windowbounds.Policy{FallbackWidth: 800, FallbackHeight: 480, MinWidth: 100, MinHeight: 100, ChromeHeight: 30, MinVisibleChromeX: 20, MinVisibleChromeY: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	return blueprint
}

func TestWindowControllerRestoreStartupCoordinatesNativeAndMuxTransactions(t *testing.T) {
	var log []string
	factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: -1}
	windows := &fakeRestoreWindows{log: &log}
	controller := newRestoreProjectionController(t, &log)
	controller.setRestoreWindows(windows)
	if err := controller.restoreStartupProjections(twoWindowRestoreBlueprint(t), factory); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controller.order, []termmux.WindowID{2, 3}) || len(controller.windows) != 2 || windows.aborts != 0 || len(windows.geometries) != 2 {
		t.Fatalf("order=%v windows=%d aborts=%d geometries=%v", controller.order, len(controller.windows), windows.aborts, windows.geometries)
	}
	wantStages := []string{"mux-prepare", "mux-ids", "bind:0:2", "bind:1:3", "mux-commit"}
	var stages []string
	for _, entry := range log {
		if entry == "mux-prepare" || entry == "mux-ids" || entry == "mux-commit" || len(entry) >= 5 && entry[:5] == "bind:" {
			stages = append(stages, entry)
		}
	}
	if !reflect.DeepEqual(stages, wantStages) {
		t.Fatalf("stages=%v log=%v", stages, log)
	}
	ids := append([]termmux.WindowID(nil), controller.order...)
	for _, id := range ids {
		if err := controller.closeProjection(id); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWindowControllerRestoreStartupFailuresLeaveFreshFallbackSeam(t *testing.T) {
	failure := errors.New("injected mux failure")
	cases := []struct {
		name      string
		configure func(*fakeRestoreProjectionFactory, *fakeRestoreWindows)
	}{
		{name: "prepare", configure: func(_ *fakeRestoreProjectionFactory, w *fakeRestoreWindows) { w.prepareErr = failure }},
		{name: "ids", configure: func(_ *fakeRestoreProjectionFactory, w *fakeRestoreWindows) { w.idsErr = failure }},
		{name: "bind", configure: func(f *fakeRestoreProjectionFactory, _ *fakeRestoreWindows) { f.bindAt = 0 }},
		{name: "commit", configure: func(_ *fakeRestoreProjectionFactory, w *fakeRestoreWindows) { w.commitErr = failure }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var log []string
			factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: -1}
			windows := &fakeRestoreWindows{log: &log}
			tc.configure(factory, windows)
			controller := newRestoreProjectionController(t, &log)
			controller.setRestoreWindows(windows)
			if err := controller.restoreStartupProjections(twoWindowRestoreBlueprint(t), factory); err == nil {
				t.Fatal("failure accepted")
			}
			assertRestoreProjectionPristine(t, controller, factory.hosts)
			if len(factory.bindings) != 0 {
				t.Fatalf("bindings leaked: %v", factory.bindings)
			}
			if tc.name == "prepare" && windows.aborts != 0 {
				t.Fatalf("abort before candidate=%d", windows.aborts)
			}
			if tc.name != "prepare" && windows.aborts != 1 {
				t.Fatalf("aborts=%d log=%v", windows.aborts, log)
			}
		})
	}
}

type projectionRestoreSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	once   sync.Once
}

func newProjectionRestoreSession() *projectionRestoreSession {
	r, w := io.Pipe()
	return &projectionRestoreSession{reader: r, writer: w}
}
func (s *projectionRestoreSession) Reader() io.Reader              { return s.reader }
func (s *projectionRestoreSession) Write(data []byte) (int, error) { return len(data), nil }
func (s *projectionRestoreSession) Resize(pty.Size) error          { return nil }
func (s *projectionRestoreSession) Close() error {
	var err error
	s.once.Do(func() { err = errors.Join(s.writer.Close(), s.reader.Close()) })
	return err
}

type projectionRestoreSessionFactory struct{}

func (projectionRestoreSessionFactory) Spawn(uint16, uint16, pty.Options) (pty.Session, error) {
	return newProjectionRestoreSession(), nil
}

func TestWindowControllerRestoreStartupWithRealMuxPublishesWorkspaceVisibilityAndFocus(t *testing.T) {
	var log []string
	m := termmux.New(projectionRestoreSessionFactory{}, termmux.Options{})
	defer m.Shutdown()
	controller := newWindowController(processServices{mux: m}, fakeNativePump{log: &log})
	if err := controller.startLoop(); err != nil {
		t.Fatal(err)
	}
	factory := &fakeRestoreProjectionFactory{log: &log, failAt: -1, bindAt: -1}
	if err := controller.restoreStartupProjections(twoWindowRestoreBlueprint(t), factory); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(controller.order, []termmux.WindowID{2, 3}) || controller.active != 2 || !controller.projectionVisible(2) || !controller.projectionVisible(3) {
		t.Fatalf("order=%v active=%d visible=%v/%v", controller.order, controller.active, controller.projectionVisible(2), controller.projectionVisible(3))
	}
	views := m.Windows()
	if len(views) != 2 || views[0].ID != 2 || views[1].ID != 3 || !views[0].Active || views[1].Active {
		t.Fatalf("mux windows=%#v", views)
	}
	showCount, focusCount := 0, 0
	for _, entry := range log {
		if len(entry) >= 5 && entry[:5] == "show:" {
			showCount++
		}
		if len(entry) >= 6 && entry[:6] == "focus:" {
			focusCount++
		}
	}
	if showCount != 2 || focusCount != 1 {
		t.Fatalf("show/focus=%d/%d log=%v", showCount, focusCount, log)
	}
	ids := append([]termmux.WindowID(nil), controller.order...)
	for _, id := range ids {
		if err := controller.closeProjection(id); err != nil {
			t.Fatal(err)
		}
	}
}
