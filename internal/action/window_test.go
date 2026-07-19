package action

import (
	"reflect"
	"testing"
)

func TestWindowActionsCodecRoundTrip(t *testing.T) {
	actions := []Action{
		NewWindow{}, CloseWindow{WindowID: 2}, FocusWindow{WindowID: 3},
		MoveTabToWindow{WindowID: 4, TabID: 5, Position: 1},
		MovePaneToWindow{WindowID: 6, PaneID: 7, Axis: SplitRows},
	}
	for _, action := range actions {
		t.Run(string(action.ID()), func(t *testing.T) {
			envelope := Envelope{Action: action, Target: TargetFocused}
			encoded, err := Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := Unmarshal(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, envelope) {
				t.Fatalf("decoded=%#v want=%#v", decoded, envelope)
			}
		})
	}
}

func TestWindowActionsRejectInvalidTargets(t *testing.T) {
	cases := []Action{CloseWindow{}, FocusWindow{}, MoveTabToWindow{WindowID: 1}, MoveTabToWindow{WindowID: 1, TabID: 1, Position: -1}, MovePaneToWindow{WindowID: 1, Axis: SplitRows}, MovePaneToWindow{WindowID: 1, PaneID: 1, Axis: "diagonal"}}
	for _, action := range cases {
		if err := action.Validate(); err == nil {
			t.Fatalf("%T accepted", action)
		}
	}
}

func TestWindowActionRegistryMetadata(t *testing.T) {
	for _, id := range []ID{IDNewWindow, IDCloseWindow, IDFocusWindow, IDMoveTabToWindow, IDMovePaneToWindow} {
		d, ok := DefaultRegistry().Lookup(id)
		if !ok || d.Category != CategoryWindow || !d.Serializable || d.TriggerPolicy != pressOnly {
			t.Fatalf("%s=%#v", id, d)
		}
		wantDiscoverable := id != IDMoveTabToWindow && id != IDMovePaneToWindow
		if d.Discoverable != wantDiscoverable {
			t.Fatalf("%s discoverable=%v", id, d.Discoverable)
		}
	}
}

func TestWindowContextValidation(t *testing.T) {
	valid := Context{Source: SourceScript, OriginWindow: Ref{Kind: RefWindow, ID: 1}, FocusedWindow: Ref{Kind: RefWindow, ID: 2}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.OriginWindow = Ref{Kind: RefPane, ID: 1}
	if err := valid.Validate(); err == nil {
		t.Fatal("invalid window ref accepted")
	}
}
