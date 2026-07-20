package action

import (
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceActionsCodecRoundTrip(t *testing.T) {
	actions := []Action{
		CreateWorkspace{Name: "build"},
		SwitchWorkspace{WorkspaceID: 2},
		RenameWorkspace{WorkspaceID: 2, Name: "ops"},
		MoveWindowToWorkspace{WindowID: 3, WorkspaceID: 2},
		ActivateWorkspaceSwitcher{},
	}
	for _, command := range actions {
		t.Run(string(command.ID()), func(t *testing.T) {
			envelope := Envelope{Action: command, Target: TargetFocused}
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

func TestWorkspaceActionsRejectInvalidArguments(t *testing.T) {
	cases := []Action{
		CreateWorkspace{}, CreateWorkspace{Name: "bad\nname"}, CreateWorkspace{Name: strings.Repeat("x", MaxWorkspaceActionNameBytes+1)},
		SwitchWorkspace{}, RenameWorkspace{Name: "name"}, RenameWorkspace{WorkspaceID: 1},
		MoveWindowToWorkspace{WorkspaceID: 1}, MoveWindowToWorkspace{WindowID: 1},
	}
	for _, command := range cases {
		if err := command.Validate(); err == nil {
			t.Fatalf("%T accepted %#v", command, command)
		}
	}
}

func TestWorkspaceActionRegistryMetadata(t *testing.T) {
	ids := []ID{IDCreateWorkspace, IDSwitchWorkspace, IDRenameWorkspace, IDMoveWindowToWorkspace, IDActivateWorkspaceSwitcher}
	for _, id := range ids {
		descriptor, ok := DefaultRegistry().Lookup(id)
		if !ok || descriptor.Category != CategoryWorkspace || !descriptor.Serializable || descriptor.TriggerPolicy != pressOnly || descriptor.Target != TargetOptional {
			t.Fatalf("%s=%#v", id, descriptor)
		}
		if descriptor.Discoverable != (id == IDActivateWorkspaceSwitcher) {
			t.Fatalf("%s discoverable=%v", id, descriptor.Discoverable)
		}
	}
}
