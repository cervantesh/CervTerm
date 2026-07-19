package mux

// EventKind classifies a pane-addressed state transition observed by the
// main-thread owner.
type EventKind uint8

const (
	PaneStarted EventKind = iota + 1
	PaneOutput
	PaneDirty
	PaneTitleChanged
	PaneCWDChanged
	PaneBell
	PaneExited
	PaneFocused
	PaneGeometryChanged
	PaneWriteFailed
	PaneResizeFailed
	PaneCloseFailed
	PaneClosed
	PaneTransferred
	TabEmpty // compatibility alias emitted when the final pane closes
	TabSpawned
	TabActivated
	TabRenamed
	TabMoved
	TabRevisionChanged
	TabClosed
	WindowTabsEmpty
	WindowCreated
	WindowActivated
	WindowRenamed
	WindowClosed
)

// Event contains values only; it never exposes a mutable terminal, parser, PTY,
// renderer, or toolkit object.
type Event struct {
	Kind         EventKind
	Window       WindowID
	SourceWindow WindowID
	Pane         PaneID
	Tab          TabID
	SourceTab    TabID
	Data         []byte
	Text         string
	Geometry     PaneGeometry
	Err          error
	Revision     uint64
}

type ingressRecord struct {
	pane  PaneID
	owner *pane
	data  []byte
	err   error
}
