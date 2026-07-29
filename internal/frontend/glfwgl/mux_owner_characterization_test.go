//go:build glfw

package glfwgl

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
	termmux "cervterm/internal/mux"
)

func TestFrontendRetainsOnlyNarrowMuxCapabilities(t *testing.T) {
	servicesType := reflect.TypeOf(processServices{})
	commandsField, ok := servicesType.FieldByName("commands")
	if !ok {
		t.Fatal("process command field missing")
	}
	processType := reflect.TypeOf((*processCommandCapability)(nil)).Elem()
	if commandsField.Type != processType {
		t.Fatalf("process commands type=%v want=%v", commandsField.Type, processType)
	}
	factoryField, ok := servicesType.FieldByName("windowCapabilities")
	if !ok || factoryField.Type != reflect.TypeOf((*windowCapabilityFactory)(nil)).Elem() {
		t.Fatalf("window capability factory=%v", factoryField.Type)
	}
	appType := reflect.TypeOf(App{})
	if _, ok := appType.FieldByName("owner"); ok {
		t.Fatal("App retains *mux.Owner")
	}
	appField, ok := appType.FieldByName("mux")
	if !ok {
		t.Fatal("app mux field missing")
	}
	windowType := reflect.TypeOf((*windowMuxCapability)(nil)).Elem()
	if appField.Type != windowType || processType == windowType {
		t.Fatalf("projection capability=%v process=%v", appField.Type, processType)
	}
	for index := 0; index < appType.NumField(); index++ {
		field := appType.Field(index)
		if field.Type == reflect.TypeOf((*termmux.Owner)(nil)) || field.Type == processType {
			t.Fatalf("App field %s retains process capability %v", field.Name, field.Type)
		}
	}
}

func TestProjectionCloneCannotRetainOwnerProcessCapabilityOrOwnerClosures(t *testing.T) {
	cfg := config.Defaults()
	primary := &App{
		cfg: cfg, desiredCfg: cfg.Clone(), composedCfg: cfg.Clone(),
		terminalImageCacheFactory: defaultTerminalImageCacheFactory,
		paneUI:                    make(map[termmux.PaneID]*paneUIState), pendingPaneScroll: make(map[termmux.PaneID]int),
		pendingPaneResize: make(map[termmux.PaneID]termmux.PaneGeometry), blinkStart: time.Now(),
	}
	primary.clipboardSetter = func(string) { panic("owner closure reached child") }
	child := newProjectionApp(primary)
	if child.mux != nil || child.windowIdentity != (termmux.WindowIdentity{}) {
		t.Fatalf("child retained projection/process capability: mux=%T identity=%#v", child.mux, child.windowIdentity)
	}
	if child.clipboardSetter != nil {
		t.Fatal("child retained primary closure")
	}
	if child.terminalImageCacheFactory == nil || reflect.ValueOf(child.terminalImageCacheFactory).Pointer() != reflect.ValueOf(defaultTerminalImageCacheFactory).Pointer() {
		t.Fatal("child did not retain the named process-free cache factory")
	}
	value := reflect.ValueOf(child).Elem()
	appType := value.Type()
	for index := 0; index < value.NumField(); index++ {
		fieldType := appType.Field(index)
		if fieldType.Type == reflect.TypeOf((*termmux.Owner)(nil)) || fieldType.Type == reflect.TypeOf((*processCommandCapability)(nil)).Elem() {
			t.Fatalf("child field %s can retain process capability", fieldType.Name)
		}
	}
}

func TestProductionWindowAttestationPinsLoopThreadWindowAndContextWithoutAppClosure(t *testing.T) {
	source, err := os.ReadFile("window_controller_process.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"type detachedWindowAttestation struct",
		"loopThread.Current(a.threadSource)",
		"a.contextCurrent(a.host)",
		"c.services.windowCapabilities.ForWindow(id, proof.attest)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("native-loop attestation missing %q", required)
		}
	}
	for _, forbidden := range []string{"app.owner", "a.app.owner", "func (a *App) processMux"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("attestation retained forbidden path %q", forbidden)
		}
	}
	callbackSource, err := os.ReadFile("app_callbacks.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(callbackSource), "a.controller.withCurrent(a.windowID, callback)") {
		t.Fatal("GLFW callbacks do not activate their exact projection context before mux dispatch")
	}
}
