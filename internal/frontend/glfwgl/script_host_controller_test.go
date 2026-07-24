//go:build glfw

package glfwgl

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
	"cervterm/internal/script"
)

var _ script.Host = paneHost{}

type scriptRuntimeConfigHostContract interface {
	RuntimeConfig() config.Config
	ApplyRuntimeConfig(config.Config) error
	RequestConfigReload() bool
}

var _ scriptRuntimeConfigHostContract = paneHost{}

type fakeScriptHostPorts struct {
	log       []string
	pane      termmux.PaneID
	row       int
	text      string
	lines     int
	points    float64
	config    config.Config
	applied   config.Config
	reloaded  bool
	clipboard string
}

func (f *fakeScriptHostPorts) record(name string, pane termmux.PaneID) {
	f.log = append(f.log, name)
	f.pane = pane
}

func (f *fakeScriptHostPorts) scriptHostRuntimeConfig() config.Config {
	f.log = append(f.log, "config")
	return f.config
}

func (f *fakeScriptHostPorts) applyScriptHostRuntimeConfig(next config.Config) error {
	f.log = append(f.log, "apply-config")
	f.applied = next
	return nil
}

func (f *fakeScriptHostPorts) requestScriptHostConfigReload() bool {
	f.log = append(f.log, "reload-config")
	f.reloaded = true
	return true
}

func (f *fakeScriptHostPorts) writeScriptHostInput(pane termmux.PaneID, text string) {
	f.record("write", pane)
	f.text = text
}

func (f *fakeScriptHostPorts) notifyScriptHost(text string) {
	f.log = append(f.log, "notify")
	f.text = text
}

func (f *fakeScriptHostPorts) setScriptHostClipboard(text string) {
	f.log = append(f.log, "set-clipboard")
	f.clipboard = text
}

func (f *fakeScriptHostPorts) scriptHostClipboard() string {
	f.log = append(f.log, "clipboard")
	return f.clipboard
}

func (f *fakeScriptHostPorts) scriptHostFontSize(pane termmux.PaneID) float64 {
	f.record("font-size", pane)
	return 12.5
}

func (f *fakeScriptHostPorts) setScriptHostFontSize(pane termmux.PaneID, points float64) {
	f.record("set-font-size", pane)
	f.points = points
}

func (f *fakeScriptHostPorts) scriptHostSelection(pane termmux.PaneID) string {
	f.record("selection", pane)
	return "selection"
}

func (f *fakeScriptHostPorts) scrollScriptHost(pane termmux.PaneID, lines int) bool {
	f.record("scroll", pane)
	f.lines = lines
	return true
}

func (f *fakeScriptHostPorts) scrollScriptHostToBottom(pane termmux.PaneID) {
	f.record("scroll-bottom", pane)
}

func (f *fakeScriptHostPorts) scriptHostScrollbackLen(pane termmux.PaneID) int {
	f.record("scrollback", pane)
	return 17
}

func (f *fakeScriptHostPorts) scriptHostSize(pane termmux.PaneID) (int, int) {
	f.record("size", pane)
	return 80, 24
}

func (f *fakeScriptHostPorts) scriptHostCursor(pane termmux.PaneID) (int, int) {
	f.record("cursor", pane)
	return 3, 4
}

func (f *fakeScriptHostPorts) scriptHostTitle(pane termmux.PaneID) string {
	f.record("title", pane)
	return "title"
}

func (f *fakeScriptHostPorts) scriptHostCWD(pane termmux.PaneID) string {
	f.record("cwd", pane)
	return "/cwd"
}

func (f *fakeScriptHostPorts) scriptHostLine(pane termmux.PaneID, row int) (string, bool) {
	f.record("line", pane)
	f.row = row
	return "line", true
}

func (f *fakeScriptHostPorts) setScriptHostTitle(pane termmux.PaneID, title string) {
	f.record("set-title", pane)
	f.text = title
}

func (f *fakeScriptHostPorts) scriptHostLineWrapped(pane termmux.PaneID, row int) (bool, bool) {
	f.record("wrapped", pane)
	f.row = row
	return true, true
}

func (f *fakeScriptHostPorts) searchScriptHost(pane termmux.PaneID, query string) bool {
	f.record("search", pane)
	f.text = query
	return true
}

func newFakeScriptHostController(pane termmux.PaneID) scriptHostController {
	return newScriptHostController(pane)
}

func TestScriptHostControllerRepresentsCompletePaneHostSurface(t *testing.T) {
	ports := &fakeScriptHostPorts{config: config.Defaults(), clipboard: "clip"}
	controller := newFakeScriptHostController(7)

	if controller.runtimeConfig(ports).Font.Size != ports.config.Font.Size || controller.applyRuntimeConfig(ports, config.Defaults()) != nil || !controller.requestConfigReload(ports) {
		t.Fatal("config surface did not delegate")
	}
	controller.writeInput(ports, "input")
	controller.notify(ports, "notice")
	controller.setClipboard(ports, "next")
	if controller.clipboard(ports) != "next" || controller.fontSize(ports) != 12.5 {
		t.Fatal("notification/font read surface did not delegate")
	}
	controller.setFontSize(ports, 13)
	if controller.selectionText(ports) != "selection" || !controller.scroll(ports, 4) {
		t.Fatal("selection surface did not delegate")
	}
	controller.scrollToBottom(ports)
	if controller.scrollbackLen(ports) != 17 {
		t.Fatal("scrollback surface did not delegate")
	}
	cols, rows := controller.size(ports)
	cursorRow, cursorCol := controller.cursor(ports)
	if cols != 80 || rows != 24 || cursorRow != 3 || cursorCol != 4 || controller.title(ports) != "title" || controller.cwd(ports) != "/cwd" {
		t.Fatal("view surface did not delegate")
	}
	controller.setTitle(ports, "renamed")
	if line, ok := controller.line(ports, 9); line != "line" || !ok {
		t.Fatal("line surface did not delegate")
	}
	if wrapped, ok := controller.lineWrapped(ports, 10); !wrapped || !ok || !controller.search(ports, "needle") {
		t.Fatal("mutation surface did not delegate")
	}
	if ports.pane != 7 || ports.points != 13 || !ports.reloaded {
		t.Fatalf("pane=%d points=%v reloaded=%v", ports.pane, ports.points, ports.reloaded)
	}
	want := []string{
		"config", "apply-config", "reload-config", "write", "notify", "set-clipboard", "clipboard",
		"font-size", "set-font-size", "selection", "scroll", "scroll-bottom", "scrollback",
		"size", "cursor", "title", "cwd", "set-title", "line", "wrapped", "search",
	}
	if !reflect.DeepEqual(ports.log, want) {
		t.Fatalf("script host trace = %v, want %v", ports.log, want)
	}
}

func TestScriptHostControllerConfigCrossesDetachedInBothDirections(t *testing.T) {
	ports := &fakeScriptHostPorts{config: config.Defaults()}
	ports.config.Shell.Args = []string{"source"}
	ports.config.Shell.Env = map[string]string{"SOURCE": "1"}
	controller := newFakeScriptHostController(1)

	snapshot := controller.runtimeConfig(ports)
	snapshot.Shell.Args[0] = "changed"
	snapshot.Shell.Env["SOURCE"] = "2"
	if ports.config.Shell.Args[0] != "source" || ports.config.Shell.Env["SOURCE"] != "1" {
		t.Fatal("RuntimeConfig exposed port-owned mutable config")
	}

	next := config.Defaults()
	next.Shell.Args = []string{"apply"}
	next.Shell.Env = map[string]string{"APPLY": "1"}
	if err := controller.applyRuntimeConfig(ports, next); err != nil {
		t.Fatal(err)
	}
	next.Shell.Args[0] = "changed"
	next.Shell.Env["APPLY"] = "2"
	if ports.applied.Shell.Args[0] != "apply" || ports.applied.Shell.Env["APPLY"] != "1" {
		t.Fatal("ApplyRuntimeConfig retained caller-owned mutable config")
	}
}

func TestScriptHostControllerNoOpRoutesDoNotAllocate(t *testing.T) {
	ports := &fakeScriptHostPorts{log: make([]string, 0, 1)}
	controller := newFakeScriptHostController(0)
	allocs := testing.AllocsPerRun(1000, func() {
		ports.log = ports.log[:0]
		controller.writeInput(ports, "ignored")
		if controller.search(ports, "") {
			panic("empty search matched")
		}
	})
	if allocs != 0 || len(ports.log) != 0 {
		t.Fatalf("no-op allocations=%v trace=%v", allocs, ports.log)
	}
}

type controllerFieldExpectation struct {
	name string
	typ  reflect.Type
}

func assertControllerPortStructure(t *testing.T, controller reflect.Type, fields []controllerFieldExpectation, ports []reflect.Type, budget int) {
	t.Helper()
	if controller.NumField() != len(fields) {
		t.Fatalf("%s fields = %d, listed = %d", controller.Name(), controller.NumField(), len(fields))
	}
	methodCount := 0
	for _, port := range ports {
		if port.Kind() != reflect.Interface || port.NumMethod() == 0 || port.NumMethod() > 5 {
			t.Errorf("%s has invalid method count %d", port.Name(), port.NumMethod())
		}
		methodCount += port.NumMethod()
		for methodIndex := 0; methodIndex < port.NumMethod(); methodIndex++ {
			method := port.Method(methodIndex)
			for input := 0; input < method.Type.NumIn(); input++ {
				assertScriptNativeControllerType(t, port.Name()+"."+method.Name, method.Type.In(input))
			}
			for output := 0; output < method.Type.NumOut(); output++ {
				assertScriptNativeControllerType(t, port.Name()+"."+method.Name, method.Type.Out(output))
			}
		}
	}
	if methodCount != budget {
		t.Fatalf("aggregate %s port methods = %d, budget = %d", controller.Name(), methodCount, budget)
	}
	for fieldIndex, expected := range fields {
		field := controller.Field(fieldIndex)
		if field.Name != expected.name || field.Type != expected.typ {
			t.Errorf("%s field %d = %s %s, want %s %s", controller.Name(), fieldIndex, field.Name, field.Type, expected.name, expected.typ)
		}
		assertScriptNativeControllerType(t, controller.Name()+"."+field.Name, field.Type)
	}
}

func assertScriptNativeControllerType(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	if typ == reflect.TypeOf(config.Config{}) || typ == reflect.TypeOf(time.Time{}) {
		return // Explicit detached value types; config detachment has mutation tests.
	}
	typeName := strings.ToLower(typ.String())
	packagePath := strings.ToLower(typ.PkgPath())
	for _, forbidden := range []string{
		"*glfwgl.app", "*mux.mux", "*glfw.window", "*script.runtime",
		"candidatebundle", "candidateactivation", "nativeprojectionbundle", "wndproc",
		"gpu.", "prepared", "projectionresource",
	} {
		if strings.Contains(typeName, forbidden) {
			t.Errorf("%s has forbidden owner type %s", name, typ)
		}
	}
	if strings.Contains(packagePath, "github.com/go-gl/gl/") || strings.Contains(packagePath, "/frontend/gpu") {
		t.Errorf("%s has forbidden GL/GPU package type %s", name, typ)
	}
	switch typ.Kind() {
	case reflect.Map, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		t.Errorf("%s has forbidden structural type %s", name, typ)
	case reflect.Interface:
		if typ.NumMethod() == 0 {
			t.Errorf("%s uses any/empty interface %s", name, typ)
		}
	case reflect.Pointer, reflect.Slice, reflect.Array:
		if typ.Kind() == reflect.Slice {
			t.Errorf("%s has mutable list type %s", name, typ)
		}
		assertScriptNativeControllerType(t, name, typ.Elem())
	case reflect.Struct:
		for fieldIndex := 0; fieldIndex < typ.NumField(); fieldIndex++ {
			assertScriptNativeControllerType(t, name+"."+typ.Field(fieldIndex).Name, typ.Field(fieldIndex).Type)
		}
	}
}

func TestPaneHostConstructionAndWarmedStableReadsDoNotAllocate(t *testing.T) {
	app := &App{cfg: config.Defaults()}
	stable := newPaneHost(app, 7)
	if stable.pane != 7 || stable.controller.pane != 7 || !stable.controller.initialized {
		t.Fatalf("stable route pane=%d controller=%+v", stable.pane, stable.controller)
	}
	host := newPaneHost(app, 0)
	if host.pane != 0 || host.controller.pane != 0 || !host.controller.initialized {
		t.Fatalf("zero route pane=%d controller=%+v", host.pane, host.controller)
	}
	if host.Clipboard() != "" || host.FontSize() != app.cfg.Font.Size {
		t.Fatal("unexpected warmed host read")
	}

	allocs := testing.AllocsPerRun(1000, func() {
		stable := newPaneHost(app, 7)
		candidate := paneHost{app: app}
		route := candidate.scriptHostRoute()
		if stable.controller.pane != 7 || !stable.controller.initialized || route.pane != 0 || !route.initialized || candidate.Clipboard() != "" || candidate.FontSize() != app.cfg.Font.Size {
			panic("unexpected warmed host route")
		}
	})
	if allocs != 0 {
		t.Fatalf("newPaneHost, compatibility route, and warmed reads allocations=%v, want 0", allocs)
	}
}

func TestPaneHostZeroPaneLiteralPreservesFocusedFontCompatibility(t *testing.T) {
	app := newMuxTestApp(t, 80, 24)
	app.cfg.Font.Size = 14
	focused, ok := app.focusedFontPane()
	if !ok {
		t.Fatal("focused pane unavailable")
	}
	app.ensurePaneUI(focused).font.fontSize = 18

	host := paneHost{app: app}
	if got := host.FontSize(); got != 18 {
		t.Fatalf("zero-pane literal FontSize=%v, want focused size 18", got)
	}
	host.SetFontSize(18)
	if app.cfg.Font.Size != 14 {
		t.Fatalf("zero-pane literal changed base font size to %v", app.cfg.Font.Size)
	}
	if got := app.ensurePaneUI(focused).font.fontSize; got != 18 {
		t.Fatalf("zero-pane literal changed focused font size to %v", got)
	}
}

func TestScriptHostControllerPortsAndFieldsAreExhaustiveNarrowAndDetached(t *testing.T) {
	paneType := reflect.TypeOf(termmux.PaneID(0))
	ports := []reflect.Type{
		reflect.TypeOf((*scriptHostConfigPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostInputPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostNotificationPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostFontPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostSelectionPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostViewPort)(nil)).Elem(),
		reflect.TypeOf((*scriptHostMutationPort)(nil)).Elem(),
	}
	fields := []controllerFieldExpectation{
		{name: "pane", typ: paneType},
		{name: "initialized", typ: reflect.TypeOf(false)},
	}
	assertControllerPortStructure(t, reflect.TypeOf(scriptHostController{}), fields, ports, scriptHostControllerPortBudget)
}
