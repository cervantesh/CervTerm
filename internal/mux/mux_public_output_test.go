package mux

import (
	"bytes"
	"io"
	"testing"

	"cervterm/internal/termimage"
)

func TestPaneOutputRedactsOnlyEnabledSelectedImageFrames(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, _, _ := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)
	input := []byte("A\x1b_GKITTY_MARKER\x1b\\B\x1bPqSIXEL_MARKER\x1b\\C\x1b]1337;ITERM_MARKER\x07D")
	events := m.advancePane(p, input)
	output, found := paneOutputForTest(events)
	if !found || !bytes.Equal(output, []byte("ABCD")) {
		t.Fatalf("PaneOutput found=%v data=%q events=%#v", found, output, events)
	}
	for _, marker := range [][]byte{[]byte("KITTY_MARKER"), []byte("SIXEL_MARKER"), []byte("ITERM_MARKER")} {
		if bytes.Contains(output, marker) {
			t.Fatalf("selected marker leaked: %q", marker)
		}
	}
}

func TestPaneOutputPreservesDisabledProtocolsAndEmptyActivity(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, _, _ := newITermRuntimeMux(t, true, false, false, &limits, nil)
	p, _ := m.sessions.lookup(1)
	disabledSixel := []byte("\x1bPqSIXEL_PUBLIC\x1b\\")
	disabledITerm := []byte("\x1b]1337;ITERM_PUBLIC\x07")
	input := append([]byte("\x1b_GKITTY_PRIVATE\x1b\\"), disabledSixel...)
	input = append(input, disabledITerm...)
	events := m.advancePane(p, input)
	output, found := paneOutputForTest(events)
	want := append(append([]byte(nil), disabledSixel...), disabledITerm...)
	if !found || !bytes.Equal(output, want) {
		t.Fatalf("PaneOutput found=%v data=%q want=%q events=%#v", found, output, want, events)
	}

	events = m.advancePane(p, []byte("\x1b_GANOTHER_PRIVATE\x1b\\"))
	output, found = paneOutputForTest(events)
	if !found || len(output) != 0 {
		t.Fatalf("selected-only input lost empty activity event: found=%v data=%q events=%#v", found, output, events)
	}
}

func TestMuxUnselectedDCSDoesNotEmitSixelDiagnostic(t *testing.T) {
	m, _, _ := defaultSixelRuntimeMux(t)
	p, _ := m.sessions.lookup(1)
	var diagnostics []ImageDiagnostic
	m.options.ImageDiagnostic = func(diagnostic ImageDiagnostic) {
		diagnostics = append(diagnostics, diagnostic)
	}

	unselected := []byte("\x1bP1qUNSELECTED\x1b\\")
	events := m.advancePane(p, unselected)
	if output, found := paneOutputForTest(events); !found || !bytes.Equal(output, unselected) {
		t.Fatalf("unselected output=%q found=%v events=%#v", output, found, events)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("unselected diagnostics=%#v", diagnostics)
	}

	for name, cancel := range map[string]byte{"CAN": 0x18, "SUB": 0x1a} {
		t.Run(name, func(t *testing.T) {
			diagnostics = nil
			m.advancePane(p, append([]byte("\x1bPqSELECTED_PARTIAL"), cancel))
			assertImageDiagnostic(t, diagnostics, ImageDiagnosticProtocolSixel, ImageDiagnosticReasonCancelled)
		})
	}
}

func TestMuxEOFFlushesUndecidedPublicOutputAndDropsSelectedPartial(t *testing.T) {
	limits := termimage.DefaultLimits()
	m, _, _ := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p, _ := m.sessions.lookup(1)

	events := m.advancePane(p, []byte("\x1bP0;0"))
	if output, found := paneOutputForTest(events); !found || len(output) != 0 {
		t.Fatalf("undecided advance output=%q found=%v", output, found)
	}
	m.sessions.incoming <- ingressRecord{pane: p.id, owner: p, err: io.EOF}
	events = m.Drain(8)
	output, found := paneOutputForTest(events)
	if !found || !bytes.Equal(output, []byte("\x1bP0;0")) {
		t.Fatalf("EOF output=%q found=%v events=%#v", output, found, events)
	}

	m2, _, _ := newITermRuntimeMux(t, true, true, true, &limits, nil)
	p2, _ := m2.sessions.lookup(1)
	m2.advancePane(p2, []byte("\x1b_GSELECTED_PARTIAL"))
	m2.sessions.incoming <- ingressRecord{pane: p2.id, owner: p2, err: io.EOF}
	if events := m2.Drain(8); hasNonemptyPaneOutput(events) {
		t.Fatalf("selected partial leaked at EOF: %#v", events)
	}
}

func paneOutputForTest(events []Event) ([]byte, bool) {
	for _, event := range events {
		if event.Kind == PaneOutput {
			return event.Data, true
		}
	}
	return nil, false
}

func hasNonemptyPaneOutput(events []Event) bool {
	for _, event := range events {
		if event.Kind == PaneOutput && len(event.Data) != 0 {
			return true
		}
	}
	return false
}
