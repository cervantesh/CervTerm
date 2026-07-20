package accessibility

import (
	"sync"
	"testing"
)

func eventDocument(t *testing.T, generation uint64, rootName, text string, caret int, selection Span, focus NodeID, bounds Rect) Document {
	t.Helper()
	root := NodeID{Kind: NodeKindWindow, Projection: 90, Object: 1}
	pane := NodeID{Kind: NodeKindPane, Projection: 90, Object: 2, Activation: 1}
	document, err := NewDocument(DocumentDraft{ProviderID: 7, Generation: generation, Focus: focus, Nodes: []NodeDraft{
		{ID: root, Role: RoleWindow, Name: rootName},
		{ID: pane, Parent: root, Role: RoleTerminal, Name: "terminal", Rows: []RowDraft{{Text: text, Bounds: []Rect{bounds}}}, Caret: &caret, Selection: &selection},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestSemanticSchedulerTruthTableOrderingAndPayloadPrivacy(t *testing.T) {
	root := NodeID{Kind: NodeKindWindow, Projection: 90, Object: 1}
	pane := NodeID{Kind: NodeKindPane, Projection: 90, Object: 2, Activation: 1}
	previous := eventDocument(t, 1, "old", "a", 0, Span{Start: 0, End: 0}, pane, Rect{Width: 8, Height: 16})
	next := eventDocument(t, 2, "new", "b", 1, Span{Start: 0, End: 1}, root, Rect{X: 1, Width: 8, Height: 16})
	scheduler := NewSemanticScheduler(true)
	scheduler.BeginCycle()
	scheduler.QueueTransition(previous, next, IntentDocument|IntentTopology|IntentText|IntentCaret|IntentSelection|IntentFocus)
	scheduler.QueueAnnouncement(7, 2, AnnouncementBell)
	scheduler.QueueAnnouncement(7, 2, AnnouncementNotification)
	events := scheduler.Drain()
	want := []EventKind{EventDocumentInvalidated, EventTopologyChanged, EventTextChanged, EventCaretChanged, EventSelectionChanged, EventFocusChanged, EventAnnouncement, EventAnnouncement}
	if len(events) != len(want) {
		t.Fatalf("events=%#v", events)
	}
	for index := range want {
		if events[index].Kind != want[index] || events[index].ProviderID != 7 || events[index].Generation != 2 {
			t.Fatalf("event[%d]=%#v", index, events[index])
		}
	}
	if events[6].Announcement != AnnouncementBell || events[7].Announcement != AnnouncementNotification || events[2].Node != pane {
		t.Fatalf("metadata events=%#v", events)
	}
	if second := scheduler.Drain(); len(second) != 0 {
		t.Fatalf("published twice: %#v", second)
	}
}

func TestSemanticSchedulerSuppressesRepaintAndCoalescesBursts(t *testing.T) {
	pane := NodeID{Kind: NodeKindPane, Projection: 90, Object: 2, Activation: 1}
	first := eventDocument(t, 1, "window", "a", 0, Span{}, pane, Rect{Width: 8, Height: 16})
	second := eventDocument(t, 2, "window", "b", 0, Span{}, pane, Rect{Width: 8, Height: 16})
	third := eventDocument(t, 3, "window", "c", 0, Span{}, pane, Rect{Width: 8, Height: 16})
	scheduler := NewSemanticScheduler(true)
	scheduler.BeginCycle()
	scheduler.QueueTransition(first, second, IntentNone)
	if events := scheduler.Drain(); len(events) != 0 {
		t.Fatalf("repaint events=%#v", events)
	}
	scheduler.BeginCycle()
	scheduler.QueueTransition(first, second, IntentText)
	scheduler.QueueTransition(second, third, IntentText)
	events := scheduler.Drain()
	if len(events) != 1 || events[0].Kind != EventTextChanged || events[0].Generation != 3 {
		t.Fatalf("burst=%#v", events)
	}
}

func TestSemanticSchedulerInactiveIdleAndShutdown(t *testing.T) {
	scheduler := NewSemanticScheduler(false)
	for cycle := 0; cycle < 10; cycle++ {
		scheduler.BeginCycle()
		scheduler.QueueAnnouncement(1, 1, AnnouncementBell)
		scheduler.Drain()
	}
	stats := scheduler.Stats()
	if stats.Publications != 0 || stats.Events != 0 {
		t.Fatalf("disabled stats=%#v", stats)
	}
	scheduler.SetActive(true)
	scheduler.BeginCycle()
	if events := scheduler.Drain(); len(events) != 0 {
		t.Fatalf("idle events=%#v", events)
	}
	if stats := scheduler.Stats(); stats.Publications != 0 {
		t.Fatalf("idle stats=%#v", stats)
	}
	scheduler.Close()
	if scheduler.BeginCycle() {
		t.Fatal("closed scheduler began cycle")
	}
}

func TestSemanticSchedulerOverflowCollapsesToInvalidation(t *testing.T) {
	root := NodeID{Kind: NodeKindWindow, Projection: 91, Object: 1}
	makeDocument := func(generation uint64, text string, caret int, selection Span) Document {
		nodes := []NodeDraft{{ID: root, Role: RoleWindow, Name: "window"}}
		for index := 0; index < MaxNodes-1; index++ {
			id := NodeID{Kind: NodeKindPane, Projection: 91, Object: uint64(index + 2), Activation: 1}
			nodes = append(nodes, NodeDraft{ID: id, Parent: root, Role: RoleTerminal, Name: "pane", Rows: []RowDraft{{Text: text}}, Caret: &caret, Selection: &selection})
		}
		document, err := NewDocument(DocumentDraft{ProviderID: 8, Generation: generation, Nodes: nodes})
		if err != nil {
			t.Fatal(err)
		}
		return document
	}
	previous := makeDocument(1, "a", 0, Span{})
	next := makeDocument(2, "b", 1, Span{Start: 0, End: 1})
	scheduler := NewSemanticScheduler(true)
	scheduler.BeginCycle()
	scheduler.QueueTransition(previous, next, IntentText|IntentCaret|IntentSelection)
	events := scheduler.Drain()
	if len(events) != 1 || events[0].Kind != EventDocumentInvalidated || scheduler.Stats().Overflows != 1 {
		t.Fatalf("overflow events=%#v stats=%#v", events, scheduler.Stats())
	}
}

func TestSemanticSchedulerShutdownRace(t *testing.T) {
	scheduler := NewSemanticScheduler(true)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				scheduler.BeginCycle()
				scheduler.QueueAnnouncement(1, uint64(iteration+1), AnnouncementBell)
				scheduler.Drain()
			}
		}()
	}
	scheduler.Close()
	wait.Wait()
	if scheduler.BeginCycle() {
		t.Fatal("scheduler revived after close")
	}
}

func TestSemanticSchedulerCycleBoundaryDropsUndrainedAndPostDrainEvents(t *testing.T) {
	scheduler := NewSemanticScheduler(true)
	scheduler.BeginCycle()
	scheduler.QueueAnnouncement(1, 1, AnnouncementBell)
	scheduler.BeginCycle()
	if events := scheduler.Drain(); len(events) != 0 {
		t.Fatalf("undrained leaked=%#v", events)
	}
	scheduler.BeginCycle()
	scheduler.QueueAnnouncement(1, 2, AnnouncementBell)
	if events := scheduler.Drain(); len(events) != 1 {
		t.Fatalf("first drain=%#v", events)
	}
	scheduler.QueueAnnouncement(1, 2, AnnouncementNotification)
	scheduler.BeginCycle()
	if events := scheduler.Drain(); len(events) != 0 {
		t.Fatalf("post-drain leaked=%#v", events)
	}
}

func TestSemanticSchedulerCoalescesFocusAndInvalidatesTruncationChange(t *testing.T) {
	root := NodeID{Kind: NodeKindWindow, Projection: 90, Object: 1}
	pane := NodeID{Kind: NodeKindPane, Projection: 90, Object: 2, Activation: 1}
	first := eventDocument(t, 1, "window", "a", 0, Span{}, pane, Rect{Width: 8, Height: 16})
	second := eventDocument(t, 2, "window", "a", 0, Span{}, root, Rect{Width: 8, Height: 16})
	third := eventDocument(t, 3, "window", "a", 0, Span{}, pane, Rect{Width: 8, Height: 16})
	scheduler := NewSemanticScheduler(true)
	scheduler.BeginCycle()
	scheduler.QueueTransition(first, second, IntentFocus)
	scheduler.QueueTransition(second, third, IntentFocus)
	events := scheduler.Drain()
	if len(events) != 1 || events[0].Kind != EventFocusChanged || events[0].Node != pane || events[0].Generation != 3 {
		t.Fatalf("focus=%#v", events)
	}
	truncated := third
	truncated.generation = 4
	truncated.truncated = true
	scheduler.BeginCycle()
	scheduler.QueueTransition(third, truncated, IntentText)
	events = scheduler.Drain()
	if len(events) != 1 || events[0].Kind != EventDocumentInvalidated {
		t.Fatalf("truncation=%#v", events)
	}
}
