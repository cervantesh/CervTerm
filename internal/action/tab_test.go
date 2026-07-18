package action

import (
	"reflect"
	"strings"
	"testing"
)

func TestTabActionsValidateAndRoundTrip(t *testing.T) {
	actions := []Action{
		NewTab{}, ActivateTab{TabID: 1}, ActivateTabRelative{Delta: -1},
		MoveTab{TabID: 2, Position: 0}, RenameTab{TabID: 3, Title: "build"},
		CloseTab{TabID: 4}, MovePaneToTab{TabID: 5, Axis: SplitRows}, ActivateTabSwitcher{},
	}
	for _, command := range actions {
		t.Run(string(command.ID()), func(t *testing.T) {
			want := focused(command)
			encoded, err := Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Unmarshal(encoded)
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", encoded, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip = %#v, want %#v", got, want)
			}
		})
	}
}

func TestTabActionValidation(t *testing.T) {
	tests := []struct {
		name   string
		action Action
		want   string
	}{
		{"activate zero ID", ActivateTab{}, "positive"},
		{"relative zero", ActivateTabRelative{}, "must not be zero"},
		{"move zero ID", MoveTab{Position: 1}, "positive"},
		{"move negative position", MoveTab{TabID: 1, Position: -1}, "position"},
		{"rename zero ID", RenameTab{}, "positive"},
		{"close zero ID", CloseTab{}, "positive"},
		{"move pane zero ID", MovePaneToTab{Axis: SplitRows}, "positive"},
		{"move pane axis", MovePaneToTab{TabID: 1, Axis: "diagonal"}, "axis"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTabCodecsAreStrict(t *testing.T) {
	tests := []string{
		`{"type":"activate_tab","target":"focused","args":{"tab_id":1,"extra":true}}`,
		`{"type":"activate_tab","target":"focused","args":{"tab_id":1,"tab_id":2}}`,
		`{"type":"move_tab","target":"focused","args":{"tab_id":1,"position":-1}}`,
		`{"type":"activate_tab_relative","target":"focused","args":{"delta":0}}`,
		`{"type":"move_pane_to_tab","target":"focused","args":{"tab_id":1,"axis":"diagonal"}}`,
	}
	for _, data := range tests {
		if _, err := Unmarshal([]byte(data)); err == nil {
			t.Fatalf("Unmarshal(%s) succeeded", data)
		}
	}
}

func TestTabActionIdentityRejectsPointers(t *testing.T) {
	command := &ActivateTab{TabID: 1}
	if _, err := actionIdentity(command); err == nil || !strings.Contains(err.Error(), "concrete value") {
		t.Fatalf("actionIdentity(pointer) error = %v", err)
	}
}

func TestTabRegistryMetadata(t *testing.T) {
	pressOnlyIDs := []ID{IDNewTab, IDActivateTab, IDMoveTab, IDRenameTab, IDCloseTab, IDMovePaneToTab, IDActivateTabSwitcher}
	for _, id := range pressOnlyIDs {
		d, ok := DefaultRegistry().Lookup(id)
		if !ok || d.Category != CategoryTab || !d.Serializable || !d.Discoverable || d.TriggerPolicy != pressOnly {
			t.Fatalf("%s metadata = %#v", id, d)
		}
		wantTarget := TargetOptional
		if id == IDMovePaneToTab {
			wantTarget = TargetPane
		}
		if d.Target != wantTarget {
			t.Fatalf("%s target = %q, want %q", id, d.Target, wantTarget)
		}
	}
	relative, _ := DefaultRegistry().Lookup(IDActivateTabRelative)
	if relative.Category != CategoryTab || relative.Target != TargetOptional || relative.TriggerPolicy != pressAndRepeat || !relative.Serializable || !relative.Discoverable {
		t.Fatalf("relative metadata = %#v", relative)
	}
}
