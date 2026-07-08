package core

import "testing"

func TestTerminalApplicationInputModes(t *testing.T) {
	term := NewTerminal(4, 1)
	if term.ApplicationCursorMode() {
		t.Fatalf("application cursor should default disabled")
	}
	if term.ApplicationKeypadMode() {
		t.Fatalf("application keypad should default disabled")
	}

	term.SetApplicationCursorMode(true)
	term.SetApplicationKeypadMode(true)
	if !term.ApplicationCursorMode() || !term.ApplicationKeypadMode() {
		t.Fatalf("application modes should be enabled")
	}

	term.Reset()
	if term.ApplicationCursorMode() || term.ApplicationKeypadMode() {
		t.Fatalf("reset should disable application input modes")
	}
}

func TestTerminalMouseModes(t *testing.T) {
	term := NewTerminal(4, 1)
	if term.MouseMode().ReportsMouse() {
		t.Fatalf("mouse reporting should default disabled")
	}

	term.SetMouseMode(1000, true)
	term.SetMouseMode(1006, true)
	mode := term.MouseMode()
	if !mode.NormalTracking || !mode.SGR || !mode.ReportsMouse() {
		t.Fatalf("normal SGR mouse mode should be enabled: %#v", mode)
	}

	term.Reset()
	if term.MouseMode().ReportsMouse() || term.MouseMode().SGR {
		t.Fatalf("reset should disable mouse modes: %#v", term.MouseMode())
	}
}
