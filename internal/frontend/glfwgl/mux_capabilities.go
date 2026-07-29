//go:build glfw

package glfwgl

import (
	"time"

	"cervterm/internal/core"
	termmux "cervterm/internal/mux"
	"cervterm/internal/termimage"
)

// windowMuxCapability is the projection-local surface. It deliberately omits
// process lifecycle, workspaces, restore, cross-window transfer, and shutdown.
type windowMuxCapability interface {
	Bootstrap(termmux.SpawnSpec, termmux.PixelRect, termmux.CellMetrics) (termmux.TabID, termmux.PaneID, []termmux.Event, error)
	SpawnSplit(termmux.PaneID, termmux.SplitAxis, termmux.SpawnSpec) (termmux.PaneID, []termmux.Event, error)
	Split(termmux.PaneID, termmux.SplitAxis, termmux.SpawnSpec) (termmux.PaneID, []termmux.Event, error)
	FocusPane(termmux.PaneID) ([]termmux.Event, error)
	FocusDirection(termmux.Direction) ([]termmux.Event, error)
	FocusNext(bool) ([]termmux.Event, error)
	Write(termmux.PaneID, []byte) ([]termmux.Event, error)
	FeedFallback(termmux.PaneID, []byte) ([]termmux.Event, error)
	ClosePane(termmux.PaneID) ([]termmux.Event, error)
	SpawnTab(termmux.SpawnSpec, termmux.CellMetrics, string) (termmux.TabID, termmux.PaneID, []termmux.Event, error)
	ActivateTab(termmux.TabID) ([]termmux.Event, error)
	RenameTab(termmux.TabID, string) ([]termmux.Event, error)
	MoveTab(termmux.TabID, int) ([]termmux.Event, error)
	CloseTab(termmux.TabID) ([]termmux.Event, error)
	TransferPane(termmux.PaneID, termmux.TabID, termmux.PaneID, termmux.SplitAxis) ([]termmux.Event, error)
	ResizeCurrentPane(termmux.Direction, int) ([]termmux.Event, error)
	SwapCurrentPane(termmux.Direction) ([]termmux.Event, error)
	MoveCurrentPane(termmux.Direction) ([]termmux.Event, error)
	SetSplitRatio(termmux.SplitID, termmux.SplitRatio) ([]termmux.Event, error)
	Resize(termmux.PixelRect, termmux.CellMetrics) ([]termmux.Event, error)
	ResizeGrid(termmux.PixelRect, termmux.CellMetrics) ([]termmux.Event, error)
	ResizeBounds(termmux.PixelRect) ([]termmux.Event, error)
	ResizePaneGrid(termmux.PaneID, termmux.CellMetrics) ([]termmux.Event, error)
	ApplyResize(termmux.PaneID) ([]termmux.Event, error)
	ScrollViewport(termmux.PaneID, int) (bool, error)
	ScrollViewportToGlobalRow(termmux.PaneID, int) (bool, error)
	SetTitle(termmux.PaneID, string) (bool, error)
	FocusedPane() (termmux.PaneID, bool)
	PaneIDs() []termmux.PaneID
	Layout() (termmux.Layout, error)
	PaneView(termmux.PaneID) (termmux.PaneView, bool)
	Tabs() []termmux.TabView
	ActiveTab() termmux.TabID
	TabForPane(termmux.PaneID) (termmux.TabID, bool)
	AcquireImageResource(termmux.PaneID, termimage.ResourceRef) (termimage.DetachedResource, bool)
	SearchUpward(termmux.PaneID, string, bool, int) (int, int, bool, error)
	GlobalRowToViewport(termmux.PaneID, int) (int, bool)
	Line(termmux.PaneID, int) (string, bool)
	LineWrapped(termmux.PaneID, int) (bool, bool)
	QuickSelectSnapshot(termmux.PaneID, int, int) (termmux.QuickSelectSnapshot, bool)
	QuickSelectSnapshotCurrent(termmux.QuickSelectSnapshot) bool
	SemanticSnapshot(termmux.PaneID) (termmux.SemanticSnapshot, bool)
	SemanticSnapshotCurrent(termmux.SemanticSnapshot) bool
	ValidateSemanticRange(termmux.SemanticSnapshot, core.SemanticRange) error
	SemanticRangeText(termmux.SemanticSnapshot, core.SemanticRange) (string, error)
	ReplyCounters(termmux.PaneID) (termmux.ReplyCounters, bool)
	NextImageDeadline() (time.Time, bool)
	ImageSetupError() error
}

// processCommandCapability is retained only by the serialized process
// controller. It deliberately cannot mint projection capabilities.
type processCommandCapability interface {
	runtimeWindowLifecycle
	restoreWindowLifecycle
	Drain(int) ([]termmux.Event, error)
	ResolveEventAddresses([]termmux.Event) []termmux.Event
	FreshSessionSnapshot() (termmux.FreshSessionSnapshot, error)
	Windows() []termmux.WindowView
	WindowForPane(termmux.PaneID) (termmux.WindowID, bool)
	Workspaces() []termmux.WorkspaceView
	ActiveWorkspace() termmux.WorkspaceView
	TransferPaneBetweenWindows(termmux.PaneTransferRequest) ([]termmux.Event, error)
	TransferTabBetweenWindows(termmux.TabTransferRequest) ([]termmux.Event, error)
	CreateWorkspace(string) (termmux.WorkspaceView, []termmux.Event, error)
	RenameWorkspace(termmux.WorkspaceID, string) ([]termmux.Event, error)
	SwitchWorkspace(termmux.WorkspaceID) ([]termmux.Event, error)
	MoveWindowToWorkspace(termmux.WindowID, termmux.WorkspaceID) ([]termmux.Event, error)
	SetPaletteBase(core.PaletteBase) error
	SetScrollbackCapacity(int) error
	SetHideCursorWhenScrolled(bool) error
	ImageSetupError() error
	Shutdown() error
	Closed() bool
}

// windowCapabilityFactory is the only process service allowed to mint a
// projection-local WindowOwner. The controller attests exact native identity
// before publishing the returned narrow capability to an App.
type windowCapabilityFactory interface {
	ForWindow(termmux.WindowID, termmux.WindowAttestor) (*termmux.WindowOwner, error)
}
