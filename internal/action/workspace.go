package action

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const MaxWorkspaceActionNameBytes = 128

type CreateWorkspace struct{ Name string }
type SwitchWorkspace struct{ WorkspaceID uint64 }
type RenameWorkspace struct {
	WorkspaceID uint64
	Name        string
}
type MoveWindowToWorkspace struct {
	WindowID    uint64
	WorkspaceID uint64
}
type ActivateWorkspaceSwitcher struct{}

func (CreateWorkspace) ID() ID                    { return IDCreateWorkspace }
func (SwitchWorkspace) ID() ID                    { return IDSwitchWorkspace }
func (RenameWorkspace) ID() ID                    { return IDRenameWorkspace }
func (MoveWindowToWorkspace) ID() ID              { return IDMoveWindowToWorkspace }
func (ActivateWorkspaceSwitcher) ID() ID          { return IDActivateWorkspaceSwitcher }
func (CreateWorkspace) action()                   {}
func (SwitchWorkspace) action()                   {}
func (RenameWorkspace) action()                   {}
func (MoveWindowToWorkspace) action()             {}
func (ActivateWorkspaceSwitcher) action()         {}
func (ActivateWorkspaceSwitcher) Validate() error { return nil }

func validateWorkspaceID(id uint64) error {
	if id == 0 {
		return errors.New("workspace ID must be positive")
	}
	return nil
}

func validateWorkspaceName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("workspace name must not be empty")
	}
	if !utf8.ValidString(name) {
		return errors.New("workspace name must be valid UTF-8")
	}
	if len(name) > MaxWorkspaceActionNameBytes {
		return errors.New("workspace name is too long")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("workspace name must not contain control characters")
		}
	}
	return nil
}

func (a CreateWorkspace) Validate() error { return validateWorkspaceName(a.Name) }
func (a SwitchWorkspace) Validate() error { return validateWorkspaceID(a.WorkspaceID) }
func (a RenameWorkspace) Validate() error {
	if err := validateWorkspaceID(a.WorkspaceID); err != nil {
		return err
	}
	return validateWorkspaceName(a.Name)
}
func (a MoveWindowToWorkspace) Validate() error {
	if err := validateWindowID(a.WindowID); err != nil {
		return err
	}
	return validateWorkspaceID(a.WorkspaceID)
}

type workspaceNameArgs struct {
	Name string `json:"name"`
}
type workspaceIDArgs struct {
	WorkspaceID uint64 `json:"workspace_id"`
}
type renameWorkspaceArgs struct {
	WorkspaceID uint64 `json:"workspace_id"`
	Name        string `json:"name"`
}
type moveWindowToWorkspaceArgs struct {
	WindowID    uint64 `json:"window_id"`
	WorkspaceID uint64 `json:"workspace_id"`
}

var createWorkspaceCodec = typedCodec("CreateWorkspace", func(a CreateWorkspace) workspaceNameArgs {
	return workspaceNameArgs{a.Name}
}, func(a workspaceNameArgs) CreateWorkspace { return CreateWorkspace{a.Name} })
var switchWorkspaceCodec = typedCodec("SwitchWorkspace", func(a SwitchWorkspace) workspaceIDArgs {
	return workspaceIDArgs{a.WorkspaceID}
}, func(a workspaceIDArgs) SwitchWorkspace { return SwitchWorkspace{a.WorkspaceID} })
var renameWorkspaceCodec = typedCodec("RenameWorkspace", func(a RenameWorkspace) renameWorkspaceArgs {
	return renameWorkspaceArgs{a.WorkspaceID, a.Name}
}, func(a renameWorkspaceArgs) RenameWorkspace { return RenameWorkspace{a.WorkspaceID, a.Name} })
var moveWindowToWorkspaceCodec = typedCodec("MoveWindowToWorkspace", func(a MoveWindowToWorkspace) moveWindowToWorkspaceArgs {
	return moveWindowToWorkspaceArgs{a.WindowID, a.WorkspaceID}
}, func(a moveWindowToWorkspaceArgs) MoveWindowToWorkspace {
	return MoveWindowToWorkspace{a.WindowID, a.WorkspaceID}
})
