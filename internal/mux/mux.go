package mux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"cervterm/internal/core"
	"cervterm/internal/pty"
	"cervterm/internal/render"
)

// Options configures process-local mux orchestration.
type Options struct {
	IngressCapacity int
	Wake            func()
	SetClipboard    func(PaneID, string)
}

// PaneView is an immutable copy of the pane state exposed to frontends.
type PaneView struct {
	ID                PaneID
	State             PaneState
	Geometry          PaneGeometry
	Snapshot          render.Snapshot
	DesiredSize       pty.Size
	AppliedSize       pty.Size
	ResizeErr         error
	DisplayOffset     int
	ScrollbackLines   int
	AlternateScreen   bool
	BracketedPaste    bool
	FocusEvents       bool
	ApplicationCursor bool
	MouseMode         core.MouseMode
}

// Mux owns the implicit tab model and every pane session aggregate. Its methods
// are called by one main-thread owner; only reader goroutines touch ingress.
type Mux struct {
	factory      SessionFactory
	options      Options
	model        *Model
	panes        map[PaneID]*pane
	closed       map[PaneID]struct{}
	incoming     chan ingressRecord
	ctx          context.Context
	cancel       context.CancelFunc
	readers      sync.WaitGroup
	bootstrapped bool
	bounds       PixelRect
	metrics      CellMetrics
}

func New(factory SessionFactory, options Options) *Mux {
	if factory == nil {
		factory = LocalSessionFactory()
	}
	if options.IngressCapacity <= 0 {
		options.IngressCapacity = 256
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Mux{
		factory:  factory,
		options:  options,
		model:    NewModel(),
		panes:    make(map[PaneID]*pane),
		closed:   make(map[PaneID]struct{}),
		incoming: make(chan ingressRecord, options.IngressCapacity),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Bootstrap starts the sole model leaf. A spawn failure preserves a failed leaf
// with a diagnostic banner so the window can remain usable for diagnostics.
func (m *Mux) Bootstrap(spec SpawnSpec, content PixelRect, metrics CellMetrics) (TabID, PaneID, []Event, error) {
	if m.bootstrapped {
		return 0, 0, nil, ErrAlreadyBootstrapped
	}
	layout, err := m.model.Layout(content, metrics)
	if err != nil {
		return 0, 0, nil, err
	}
	if len(layout.Panes) != 1 {
		return 0, 0, nil, invariantError("bootstrap layout has %d panes", len(layout.Panes))
	}
	geometry := effectiveGeometry(layout.Panes[0])
	p := newPane(geometry.Pane, geometry.Cols, geometry.Rows)
	p.geometry = geometry
	if m.options.SetClipboard != nil {
		p.parser.SetClipboard = func(text string) { m.options.SetClipboard(p.id, text) }
	}
	m.panes[p.id] = p
	m.bounds, m.metrics = content, metrics
	m.bootstrapped = true

	rows, cols := terminalSize(geometry)
	session, spawnErr := m.factory.Spawn(rows, cols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		p.state = PaneStateFailed
		p.parser.Advance(p.terminal, []byte("Local PTY unavailable: "+spawnErr.Error()+"\r\n"))
		p.capture()
		return m.model.TabID(), p.id, []Event{
			{Kind: PaneStarted, Pane: p.id},
			{Kind: PaneFocused, Pane: p.id},
			{Kind: PaneGeometryChanged, Pane: p.id, Geometry: geometry},
			{Kind: PaneDirty, Pane: p.id},
		}, spawnErr
	}
	p.session = session
	p.state = PaneStateRunning
	p.desiredSize = pty.Size{Rows: rows, Cols: cols}
	p.appliedSize = p.desiredSize
	p.capture()
	p.startReader(m.ctx, m.incoming, m.options.Wake, &m.readers)
	return m.model.TabID(), p.id, []Event{
		{Kind: PaneStarted, Pane: p.id},
		{Kind: PaneFocused, Pane: p.id},
		{Kind: PaneGeometryChanged, Pane: p.id, Geometry: geometry},
	}, nil
}

func (m *Mux) FocusedPane() (PaneID, bool) {
	id := m.model.FocusedPane()
	return id, id != 0
}

func (m *Mux) PaneIDs() []PaneID { return m.model.PaneIDs() }

func (m *Mux) Layout() (Layout, error) { return m.model.Layout(m.bounds, m.metrics) }

func (m *Mux) PaneView(id PaneID) (PaneView, bool) {
	p, ok := m.panes[id]
	if !ok {
		return PaneView{}, false
	}
	view := PaneView{
		ID: id, State: p.state, Geometry: p.geometry,
		DesiredSize: p.desiredSize, AppliedSize: p.appliedSize, ResizeErr: p.resizeErr,
		Snapshot:      p.snapshot,
		DisplayOffset: p.terminal.DisplayOffset(), ScrollbackLines: p.terminal.ScrollbackLines(),
		AlternateScreen: p.terminal.AlternateScreenMode(), BracketedPaste: p.terminal.BracketedPasteMode(),
		FocusEvents: p.terminal.FocusEventsMode(), ApplicationCursor: p.terminal.ApplicationCursorMode(),
		MouseMode: p.terminal.MouseMode(),
	}
	view.Snapshot.Cells = append(view.Snapshot.Cells[:0:0], p.snapshot.Cells...)
	return view, true
}

// Split spawns a new session before committing topology, so a spawn failure is
// atomic with respect to tree, focus, geometry, and ID allocation.
func (m *Mux) Split(target PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	if !m.bootstrapped {
		return 0, nil, ErrEmptyModel
	}
	if !validAxis(axis) {
		return 0, nil, ErrInvalidAxis
	}
	layout, err := m.model.Layout(m.bounds, m.metrics)
	if err != nil {
		return 0, nil, err
	}
	var targetGeometry PaneGeometry
	found := false
	for _, geometry := range layout.Panes {
		if geometry.Pane == target {
			targetGeometry, found = geometry, true
			break
		}
	}
	if !found {
		return 0, nil, ErrPaneNotFound
	}
	_, _, newRect := splitPixelRect(targetGeometry.Pixels, axis, DefaultSplitRatio)
	cols, rows := cellGeometry(newRect, m.metrics)
	if cols < MinPaneCols || rows < MinPaneRows {
		return 0, nil, ErrSplitTooSmall
	}

	predictedID := m.model.nextPaneID
	newPane := newPane(predictedID, cols, rows)
	if m.options.SetClipboard != nil {
		newPane.parser.SetClipboard = func(text string) { m.options.SetClipboard(newPane.id, text) }
	}
	ptyRows, ptyCols := terminalSize(PaneGeometry{Pane: predictedID, Pixels: newRect, Cols: cols, Rows: rows})
	session, spawnErr := m.factory.Spawn(ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		return 0, nil, fmt.Errorf("spawn split pane: %w", spawnErr)
	}
	newPane.session = session
	newPane.state = PaneStateRunning
	newPane.desiredSize = pty.Size{Rows: ptyRows, Cols: ptyCols}
	newPane.appliedSize = newPane.desiredSize

	createdID, err := m.model.Split(target, axis, m.bounds, m.metrics)
	if err != nil {
		_ = newPane.close()
		return 0, nil, err
	}
	if createdID != predictedID {
		_ = newPane.close()
		return 0, nil, invariantError("model allocated pane %d after predicting %d", createdID, predictedID)
	}
	m.panes[createdID] = newPane
	resizeEvents, resizeErr := m.Resize(m.bounds, m.metrics)
	newPane.capture()
	newPane.startReader(m.ctx, m.incoming, m.options.Wake, &m.readers)
	events := []Event{{Kind: PaneStarted, Pane: createdID}, {Kind: PaneFocused, Pane: createdID}}
	events = append(events, resizeEvents...)
	return createdID, events, resizeErr
}

func (m *Mux) FocusPane(id PaneID) ([]Event, error) {
	if err := m.model.Focus(id); err != nil {
		return nil, err
	}
	return []Event{{Kind: PaneFocused, Pane: id}}, nil
}

func (m *Mux) FocusDirection(direction Direction) ([]Event, error) {
	id, err := m.model.FocusDirection(direction, m.bounds, m.metrics)
	if err != nil {
		return nil, err
	}
	return []Event{{Kind: PaneFocused, Pane: id}}, nil
}

func (m *Mux) FocusNext(reverse bool) ([]Event, error) {
	if reverse {
		ids := m.model.PaneIDs()
		if len(ids) == 0 {
			return nil, ErrEmptyModel
		}
		focused := m.model.FocusedPane()
		for i, id := range ids {
			if id == focused {
				previous := ids[(i-1+len(ids))%len(ids)]
				if err := m.model.Focus(previous); err != nil {
					return nil, err
				}
				return []Event{{Kind: PaneFocused, Pane: previous}}, nil
			}
		}
		return nil, invariantError("focused pane %d is not active", focused)
	}
	id, err := m.model.FocusNext()
	if err != nil {
		return nil, err
	}
	return []Event{{Kind: PaneFocused, Pane: id}}, nil
}

func (m *Mux) Write(id PaneID, data []byte) ([]Event, error) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return nil, ErrPaneNotFound
	}
	if p.state != PaneStateRunning || p.session == nil {
		return nil, ErrPaneNotRunning
	}
	n, err := p.session.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return []Event{{Kind: PaneWriteFailed, Pane: id, Err: err}}, err
	}
	return nil, nil
}

// FeedFallback advances a failed pane without a PTY, preserving the interactive
// diagnostic renderer used on platforms where local session creation is unavailable.
func (m *Mux) FeedFallback(id PaneID, data []byte) ([]Event, error) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return nil, ErrPaneNotFound
	}
	if p.state != PaneStateFailed || p.session != nil {
		return nil, ErrPaneNotRunning
	}
	return m.advancePane(p, data), nil
}

// Drain advances at most limit queued records. A non-positive limit drains the
// channel until it is currently empty.
func (m *Mux) Drain(limit int) []Event {
	var events []Event
	for count := 0; limit <= 0 || count < limit; count++ {
		select {
		case record := <-m.incoming:
			p, ok := m.panes[record.pane]
			if !ok || p.state == PaneStateClosed || p.state == PaneStateClosing {
				continue
			}
			if len(record.data) > 0 {
				events = append(events, m.advancePane(p, record.data)...)
			}
			if record.err != nil && p.state == PaneStateRunning {
				p.state = PaneStateExited
				exit := Event{Kind: PaneExited, Pane: p.id}
				if !errors.Is(record.err, io.EOF) {
					exit.Err = record.err
				}
				events = append(events, exit)
			}
		default:
			return events
		}
	}
	return events
}

func (m *Mux) advancePane(p *pane, data []byte) []Event {
	oldTitle, oldCWD, oldBell := p.title, p.cwd, p.bellCount
	p.parser.Advance(p.terminal, data)
	events := p.flushReplies()
	p.capture()
	events = append(events,
		Event{Kind: PaneOutput, Pane: p.id, Data: append([]byte(nil), data...)},
		Event{Kind: PaneDirty, Pane: p.id},
	)
	if p.title != oldTitle {
		events = append(events, Event{Kind: PaneTitleChanged, Pane: p.id, Text: p.title})
	}
	if p.cwd != oldCWD {
		events = append(events, Event{Kind: PaneCWDChanged, Pane: p.id, Text: p.cwd})
	}
	for bell := oldBell; bell < p.bellCount; bell++ {
		events = append(events, Event{Kind: PaneBell, Pane: p.id})
	}
	return events
}

// ScrollViewport moves one pane's viewport and refreshes its immutable snapshot.
func (m *Mux) ScrollViewport(id PaneID, lines int) (bool, error) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return false, ErrPaneNotFound
	}
	moved := p.terminal.ScrollViewport(lines)
	if moved {
		p.capture()
	}
	return moved, nil
}

// SearchUpward atomically finds and reveals the next match in one pane.
func (m *Mux) SearchUpward(id PaneID, query string, hasPrev bool, prevRow int) (row, col int, ok bool, err error) {
	p, exists := m.panes[id]
	if !exists || !m.model.paneExists(id) {
		return 0, 0, false, ErrPaneNotFound
	}
	from := p.terminal.ScrollbackLines() + p.terminal.Rows()
	if hasPrev {
		from = prevRow
	}
	row, col, ok = p.terminal.SearchBackward(query, from)
	if ok {
		scrollGlobalRowIntoView(p.terminal, row)
		p.capture()
	}
	return row, col, ok, nil
}

func scrollGlobalRowIntoView(t *core.Terminal, row int) {
	if _, ok := t.GlobalRowToViewport(row); ok {
		return
	}
	targetTop := max(0, row-t.Rows()/2)
	t.ScrollViewport((t.ScrollbackLines() - targetTop) - t.DisplayOffset())
}

func (m *Mux) GlobalRowToViewport(id PaneID, row int) (int, bool) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return 0, false
	}
	return p.terminal.GlobalRowToViewport(row)
}

func (m *Mux) SetTitle(id PaneID, title string) (bool, error) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return false, ErrPaneNotFound
	}
	if p.terminal.Title() == title {
		return false, nil
	}
	p.terminal.SetTitle(title)
	p.capture()
	return true, nil
}

func (m *Mux) Line(id PaneID, row int) (string, bool) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) || row < 0 || row >= p.terminal.Rows() {
		return "", false
	}
	cols, rows := p.terminal.Cols(), p.terminal.Rows()
	cells := make([]core.Cell, cols*rows)
	p.terminal.CopyView(cells)
	start := row * cols
	return core.RowText(cells[start : start+cols]), true
}

func (m *Mux) LineWrapped(id PaneID, row int) (bool, bool) {
	p, ok := m.panes[id]
	if !ok || !m.model.paneExists(id) {
		return false, false
	}
	return p.terminal.LineWrapped(row)
}

func (m *Mux) ClosePane(id PaneID) ([]Event, error) {
	p, ok := m.panes[id]
	if !ok {
		if _, wasClosed := m.closed[id]; wasClosed {
			return nil, nil
		}
		return nil, ErrPaneNotFound
	}
	closeErr := p.close()
	result, modelErr := m.model.Close(id)
	delete(m.panes, id)
	m.closed[id] = struct{}{}
	var events []Event
	if closeErr != nil {
		events = append(events, Event{Kind: PaneCloseFailed, Pane: id, Err: closeErr})
	}
	var resizeErr error
	if result.Closed {
		events = append(events, Event{Kind: PaneClosed, Pane: id})
		if result.Focused != 0 {
			events = append(events, Event{Kind: PaneFocused, Pane: result.Focused})
		}
		if result.Empty {
			events = append(events, Event{Kind: TabEmpty})
		} else {
			var resizeEvents []Event
			resizeEvents, resizeErr = m.Resize(m.bounds, m.metrics)
			events = append(events, resizeEvents...)
		}
	}
	return events, errors.Join(closeErr, modelErr, resizeErr)
}

func (m *Mux) Shutdown() error {
	m.cancel()
	var closeErrors []error
	for _, p := range m.panes {
		if err := p.close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("pane %d close: %w", p.id, err))
		}
	}
	m.readers.Wait()
	return errors.Join(closeErrors...)
}

func effectiveGeometry(geometry PaneGeometry) PaneGeometry {
	geometry.Cols = max(2, geometry.Cols)
	geometry.Rows = max(1, geometry.Rows)
	return geometry
}

func terminalSize(geometry PaneGeometry) (rows, cols uint16) {
	return clampUint16(max(1, geometry.Rows)), clampUint16(max(2, geometry.Cols))
}

func clampUint16(value int) uint16 {
	if value > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(value)
}
