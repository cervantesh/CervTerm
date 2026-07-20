package accessibility

import (
	"reflect"
	"sync"
)

const MaxSemanticEvents = 256

type SemanticIntent uint16

const (
	IntentNone      SemanticIntent = 0
	IntentDocument  SemanticIntent = 1 << 0
	IntentTopology  SemanticIntent = 1 << 1
	IntentText      SemanticIntent = 1 << 2
	IntentCaret     SemanticIntent = 1 << 3
	IntentSelection SemanticIntent = 1 << 4
	IntentFocus     SemanticIntent = 1 << 5
)

type EventKind uint8

const (
	EventNone EventKind = iota
	EventDocumentInvalidated
	EventTopologyChanged
	EventTextChanged
	EventCaretChanged
	EventSelectionChanged
	EventFocusChanged
	EventAnnouncement
)

type AnnouncementKind uint8

const (
	AnnouncementNone AnnouncementKind = iota
	AnnouncementBell
	AnnouncementNotification
)

type SemanticEvent struct {
	Kind         EventKind
	ProviderID   uint64
	Generation   uint64
	Node         NodeID
	Announcement AnnouncementKind
}

type SchedulerStats struct {
	Cycles       uint64
	Publications uint64
	Events       uint64
	Overflows    uint64
}

type semanticEventKey struct {
	kind         EventKind
	node         NodeID
	announcement AnnouncementKind
}

type SemanticScheduler struct {
	mu         sync.Mutex
	active     bool
	closed     bool
	published  bool
	providerID uint64
	generation uint64
	pending    map[semanticEventKey]SemanticEvent
	order      []semanticEventKey
	overflow   bool
	stats      SchedulerStats
}

func NewSemanticScheduler(active bool) *SemanticScheduler {
	return &SemanticScheduler{active: active, pending: make(map[semanticEventKey]SemanticEvent, MaxSemanticEvents)}
}

func (scheduler *SemanticScheduler) BeginCycle() bool {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed {
		return false
	}
	scheduler.clearLocked()
	scheduler.published = false
	scheduler.stats.Cycles++
	return scheduler.active
}

func (scheduler *SemanticScheduler) SetActive(active bool) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed {
		return
	}
	scheduler.active = active
	if !active {
		scheduler.clearLocked()
	}
}

func (scheduler *SemanticScheduler) QueueTransition(previous, next Document, intents SemanticIntent) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed || !scheduler.active || intents == IntentNone || next.providerID == 0 {
		return
	}
	scheduler.providerID, scheduler.generation = next.providerID, next.generation
	if previous.providerID == 0 || previous.providerID != next.providerID {
		scheduler.queueLocked(SemanticEvent{Kind: EventDocumentInvalidated, ProviderID: next.providerID, Generation: next.generation})
		return
	}
	if previous.truncated != next.truncated {
		scheduler.queueLocked(SemanticEvent{Kind: EventDocumentInvalidated, ProviderID: next.providerID, Generation: next.generation})
	}
	if intents&IntentDocument != 0 && documentGeometryChanged(previous, next) {
		scheduler.queueLocked(SemanticEvent{Kind: EventDocumentInvalidated, ProviderID: next.providerID, Generation: next.generation})
	}
	if intents&IntentTopology != 0 && documentTopologyChanged(previous, next) {
		scheduler.queueLocked(SemanticEvent{Kind: EventTopologyChanged, ProviderID: next.providerID, Generation: next.generation})
	}
	if intents&IntentText != 0 || intents&IntentCaret != 0 || intents&IntentSelection != 0 {
		queueNodeDiffsLocked(scheduler, previous, next, intents)
	}
	if intents&IntentFocus != 0 && previous.focus != next.focus {
		scheduler.queueLocked(SemanticEvent{Kind: EventFocusChanged, ProviderID: next.providerID, Generation: next.generation, Node: next.focus})
	}
}

func (scheduler *SemanticScheduler) QueueAnnouncement(providerID, generation uint64, kind AnnouncementKind) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed || !scheduler.active || providerID == 0 || generation == 0 || (kind != AnnouncementBell && kind != AnnouncementNotification) {
		return
	}
	scheduler.providerID, scheduler.generation = providerID, generation
	scheduler.queueLocked(SemanticEvent{Kind: EventAnnouncement, ProviderID: providerID, Generation: generation, Announcement: kind})
}

func (scheduler *SemanticScheduler) Drain() []SemanticEvent {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.closed || !scheduler.active || scheduler.published || (!scheduler.overflow && len(scheduler.order) == 0) {
		return nil
	}
	scheduler.published = true
	var events []SemanticEvent
	if scheduler.overflow {
		events = []SemanticEvent{{Kind: EventDocumentInvalidated, ProviderID: scheduler.providerID, Generation: scheduler.generation}}
	} else {
		events = make([]SemanticEvent, 0, len(scheduler.order))
		for _, key := range scheduler.order {
			events = append(events, scheduler.pending[key])
		}
	}
	scheduler.stats.Publications++
	scheduler.stats.Events += uint64(len(events))
	scheduler.clearLocked()
	return events
}

func (scheduler *SemanticScheduler) Close() {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.closed = true
	scheduler.active = false
	scheduler.clearLocked()
}

func (scheduler *SemanticScheduler) Stats() SchedulerStats {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.stats
}

func (scheduler *SemanticScheduler) queueLocked(event SemanticEvent) {
	if scheduler.overflow {
		return
	}
	keyNode := event.Node
	if event.Kind == EventFocusChanged {
		keyNode = NodeID{}
	}
	key := semanticEventKey{kind: event.Kind, node: keyNode, announcement: event.Announcement}
	if _, exists := scheduler.pending[key]; exists {
		scheduler.pending[key] = event
		return
	}
	if len(scheduler.order) == MaxSemanticEvents {
		scheduler.overflow = true
		scheduler.stats.Overflows++
		scheduler.pending = make(map[semanticEventKey]SemanticEvent, MaxSemanticEvents)
		scheduler.order = scheduler.order[:0]
		return
	}
	scheduler.pending[key] = event
	scheduler.order = append(scheduler.order, key)
}

func (scheduler *SemanticScheduler) clearLocked() {
	clear(scheduler.pending)
	scheduler.order = scheduler.order[:0]
	scheduler.overflow = false
}

func queueNodeDiffsLocked(scheduler *SemanticScheduler, previous, next Document, intents SemanticIntent) {
	for _, node := range next.nodes {
		oldIndex, exists := previous.index[node.id]
		if !exists {
			continue
		}
		old := previous.nodes[oldIndex]
		if intents&IntentText != 0 && old.text != node.text {
			scheduler.queueLocked(SemanticEvent{Kind: EventTextChanged, ProviderID: next.providerID, Generation: next.generation, Node: node.id})
		}
		if intents&IntentCaret != 0 && (old.hasCaret != node.hasCaret || old.caret != node.caret) {
			scheduler.queueLocked(SemanticEvent{Kind: EventCaretChanged, ProviderID: next.providerID, Generation: next.generation, Node: node.id})
		}
		if intents&IntentSelection != 0 && (old.hasSelect != node.hasSelect || old.selection != node.selection) {
			scheduler.queueLocked(SemanticEvent{Kind: EventSelectionChanged, ProviderID: next.providerID, Generation: next.generation, Node: node.id})
		}
	}
}

func documentTopologyChanged(previous, next Document) bool {
	if len(previous.nodes) != len(next.nodes) {
		return true
	}
	for index := range next.nodes {
		left, right := previous.nodes[index], next.nodes[index]
		if left.id != right.id || left.parent != right.parent || left.role != right.role || left.name != right.name {
			return true
		}
	}
	return false
}

func documentGeometryChanged(previous, next Document) bool {
	if previous.truncated != next.truncated || len(previous.nodes) != len(next.nodes) {
		return true
	}
	for index := range next.nodes {
		left, right := previous.nodes[index], next.nodes[index]
		if left.id != right.id || len(left.rows) != len(right.rows) {
			return true
		}
		for row := range right.rows {
			if left.rows[row].softWrapped != right.rows[row].softWrapped || !reflect.DeepEqual(left.rows[row].bounds, right.rows[row].bounds) {
				return true
			}
		}
	}
	return false
}
