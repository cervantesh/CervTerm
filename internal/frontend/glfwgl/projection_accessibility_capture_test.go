//go:build glfw

package glfwgl

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectionAccessibilityCaptureIsVisibleOnlyAndRedacted(t *testing.T) {
	app := newMuxTestApp(t, 16, 2)
	app.windowID = 1
	feedTestPane(t, app, []byte("visible-secret"))
	document, ok, err := app.captureAccessibilityDocument(1)
	if err != nil || !ok {
		t.Fatalf("capture ok=%v err=%v", ok, err)
	}
	encoded, _ := json.Marshal(document.Nodes())
	text := string(encoded)
	if !strings.Contains(text, "visible-secret") {
		t.Fatalf("visible document=%s", encoded)
	}
	nativeHidden, ok, err := app.captureAccessibilityDocumentVisibility(2, false)
	if err != nil || !ok || nativeHidden.NodeCount() != 1 {
		t.Fatalf("native-hidden capture nodes=%d ok=%v err=%v", nativeHidden.NodeCount(), ok, err)
	}
	encoded, _ = json.Marshal(nativeHidden.Nodes())
	if strings.Contains(string(encoded), "visible-secret") {
		t.Fatalf("native-hidden document=%s", encoded)
	}
	workspace, events, err := app.mux.CreateWorkspace("hidden")
	if err != nil {
		t.Fatal(err)
	}
	app.handleMuxEvents(events)
	events, err = app.mux.SwitchWorkspace(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	app.handleMuxEvents(events)
	hidden, ok, err := app.captureAccessibilityDocument(3)
	if err != nil || !ok {
		t.Fatalf("hidden capture ok=%v err=%v", ok, err)
	}
	encoded, _ = json.Marshal(hidden.Nodes())
	if strings.Contains(string(encoded), "visible-secret") || hidden.NodeCount() != 1 {
		t.Fatalf("hidden document=%s", encoded)
	}
}
