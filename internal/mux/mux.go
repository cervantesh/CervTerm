package mux

import (
	"errors"
	"fmt"
	"io"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/pty"
	"cervterm/internal/render"
	"cervterm/internal/termimage"
)

type Options struct {
	IngressCapacity        int
	Wake                   func()
	SetClipboard           func(PaneID, string)
	ScrollbackCapacity     *int
	HideCursorWhenScrolled *bool
	ImageLimits            *termimage.Limits
	KittyEnabled           bool
	SixelEnabled           bool
	ITermEnabled           bool
	// ImageDiagnostic receives fixed privacy-safe Sixel and iTerm failure data.
	// Callback panics are contained and never change runtime failure handling.
	ImageDiagnostic func(ImageDiagnostic)
	// Now may be called by decode workers and must be safe for concurrent use.
	Now func() time.Time
}

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

type Mux struct {
	sessions       *localSessionRegistry
	options        Options
	model          *Model
	imageBudget    *termimage.ProcessBudget
	imageLimits    termimage.Limits
	imageSetupErr  error
	imageScheduler *imageDecodeScheduler
	kittyPending   map[uint64]kittyDecodeOwner
	kittyNextToken uint64
	sixelPending   map[uint64]sixelDecodeOwner
	sixelNextToken uint64
	itermPending   map[uint64]itermDecodeOwner
	itermNextToken uint64
	bootstrapped   bool
	bounds         PixelRect
	paneMetrics    map[PaneID]CellMetrics
	paletteBase    core.PaletteBase
	windowFault    func(string) error // package-private deterministic failure injection
	pending        *RestoreCandidate
}

func New(factory SessionFactory, options Options) *Mux {
	if factory == nil {
		factory = LocalSessionFactory()
	}
	if options.IngressCapacity <= 0 {
		options.IngressCapacity = 256
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if !options.KittyEnabled && !options.SixelEnabled && !options.ITermEnabled {
		options.ImageLimits = nil
	}
	sessions := newLocalSessionRegistry(factory, options.IngressCapacity, options.Wake)
	mux := &Mux{
		sessions: sessions, options: options, model: NewModel(),
		paneMetrics: make(map[PaneID]CellMetrics), paletteBase: core.DefaultPaletteBase(),
	}
	if options.ImageLimits != nil {
		limits, err := termimage.ValidateLimits(*options.ImageLimits)
		if err != nil {
			mux.imageSetupErr = err
		} else {
			mux.imageLimits = limits
			mux.imageBudget = termimage.NewProcessBudget()
			mux.imageScheduler = newImageDecodeScheduler(options.Wake, options.Now)
			if options.KittyEnabled {
				mux.kittyPending = make(map[uint64]kittyDecodeOwner)
			}
			if options.SixelEnabled {
				mux.sixelPending = make(map[uint64]sixelDecodeOwner)
			}
			if options.ITermEnabled {
				mux.itermPending = make(map[uint64]itermDecodeOwner)
			}
		}
	}
	return mux
}

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
	if err := m.sessions.reserve(geometry.Pane); err != nil {
		return 0, 0, nil, err
	}
	defer m.sessions.release(geometry.Pane)
	p := m.createPane(geometry.Pane, geometry.Cols, geometry.Rows)
	p.setFreshLaunch(spec)
	p.terminal.SetPaletteBase(m.paletteBase)
	p.geometry = geometry
	if m.options.SetClipboard != nil {
		p.parser.SetClipboard = func(text string) { m.options.SetClipboard(p.id, text) }
	}
	if err := m.sessions.register(p); err != nil {
		_ = p.close()
		return 0, 0, nil, err
	}
	m.bounds = content
	m.paneMetrics[p.id] = metrics
	m.bootstrapped = true

	rows, cols := terminalSize(geometry)
	session, spawnErr := m.sessions.spawn(rows, cols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		p.state = PaneStateFailed
		p.parser.Advance(p.terminal, []byte("Local PTY unavailable: "+spawnErr.Error()+"\r\n"))
		p.contentGen++
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
	if err := m.sessions.start(p.id); err != nil {
		detached := m.sessions.detach(p.id)
		if detached.owned {
			_ = detached.pane.close()
		}
		return 0, 0, nil, err
	}
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

func (m *Mux) Layout() (Layout, error) {
	return m.model.LayoutWithMetrics(m.bounds, m.resolveMetrics)
}

func (m *Mux) PaneView(id PaneID) (PaneView, bool) {
	if m.restorePanePending(id) {
		return PaneView{}, false
	}
	p, ok := m.sessions.lookup(id)
	if !ok {
		return PaneView{}, false
	}
	view := PaneView{
		ID: id, State: p.state, Geometry: p.geometry,
		DesiredSize: p.desiredSize, AppliedSize: p.appliedSize, ResizeErr: p.resizeErr,
		Snapshot:      detachedPaneSnapshot(p.snapshot),
		DisplayOffset: p.terminal.DisplayOffset(), ScrollbackLines: p.terminal.ScrollbackLines(),
		AlternateScreen: p.terminal.AlternateScreenMode(), BracketedPaste: p.terminal.BracketedPasteMode(),
		FocusEvents: p.terminal.FocusEventsMode(), ApplicationCursor: p.terminal.ApplicationCursorMode(),
		MouseMode: p.terminal.MouseMode(),
	}
	return view, true
}
func (m *Mux) SpawnSplit(origin PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	target := origin
	if !m.bootstrapped {
		return 0, nil, ErrEmptyModel
	}
	if !validAxis(axis) {
		return 0, nil, ErrInvalidAxis
	}
	layout, err := m.model.LayoutWithMetrics(m.bounds, m.resolveMetrics)
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
	targetMetrics, ok := m.resolveMetrics(target)
	if !ok {
		return 0, nil, ErrPaneNotFound
	}
	_, _, newRect := splitPixelRect(targetGeometry.Pixels, axis, DefaultSplitRatio)
	cols, rows := cellGeometry(newRect, targetMetrics)
	if cols < MinPaneCols || rows < MinPaneRows {
		return 0, nil, ErrSplitTooSmall
	}

	predictedID := m.model.nextPaneID
	if err := m.sessions.reserve(predictedID); err != nil {
		return 0, nil, err
	}
	defer m.sessions.release(predictedID)
	newPane := m.createPane(predictedID, cols, rows)
	newPane.setFreshLaunch(spec)
	newPane.terminal.SetPaletteBase(m.paletteBase)
	if m.options.SetClipboard != nil {
		newPane.parser.SetClipboard = func(text string) { m.options.SetClipboard(newPane.id, text) }
	}
	ptyRows, ptyCols := terminalSize(PaneGeometry{Pane: predictedID, Pixels: newRect, Cols: cols, Rows: rows})
	session, spawnErr := m.sessions.spawn(ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		_ = newPane.close()
		return 0, nil, fmt.Errorf("spawn split pane: %w", spawnErr)
	}
	newPane.session = session
	newPane.state = PaneStateRunning
	newPane.desiredSize = pty.Size{Rows: ptyRows, Cols: ptyCols}
	newPane.appliedSize = newPane.desiredSize

	resolveSplitMetrics := func(id PaneID) (CellMetrics, bool) {
		if id == predictedID {
			return targetMetrics, true
		}
		return m.resolveMetrics(id)
	}
	if err := m.sessions.register(newPane); err != nil {
		_ = newPane.close()
		return 0, nil, err
	}
	if err := m.sessions.start(newPane.id); err != nil {
		detached := m.sessions.detach(newPane.id)
		if detached.owned {
			_ = detached.pane.close()
		}
		return 0, nil, err
	}
	createdID, err := m.model.SplitWithMetrics(target, axis, m.bounds, resolveSplitMetrics)
	if err != nil {
		detached := m.sessions.detach(newPane.id)
		if detached.owned {
			_ = detached.pane.close()
		}
		return 0, nil, err
	}
	if createdID != predictedID {
		_, _ = m.model.Close(createdID)
		detached := m.sessions.detach(newPane.id)
		if detached.owned {
			_ = detached.pane.close()
		}
		return 0, nil, invariantError("model allocated pane %d after predicting %d", createdID, predictedID)
	}
	m.paneMetrics[createdID] = targetMetrics
	resizeEvents, resizeErr := m.resizeBoundsAndApply(m.bounds)
	newPane.capture()
	events := []Event{{Kind: PaneStarted, Pane: createdID}, {Kind: PaneFocused, Pane: createdID}}
	events = append(events, resizeEvents...)
	return createdID, m.ResolveEventAddresses(events), resizeErr
}

func (m *Mux) FocusPane(id PaneID) ([]Event, error) {
	if err := m.model.Focus(id); err != nil {
		return nil, err
	}
	return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: id}}), nil
}

func (m *Mux) FocusDirection(direction Direction) ([]Event, error) {
	id, err := m.model.FocusDirectionWithMetrics(direction, m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: id}}), nil
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
				return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: previous}}), nil
			}
		}
		return nil, invariantError("focused pane %d is not active", focused)
	}
	id, err := m.model.FocusNext()
	if err != nil {
		return nil, err
	}
	return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: id}}), nil
}

func (m *Mux) Write(id PaneID, data []byte) ([]Event, error) {
	p, ok := m.sessions.lookup(id)
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
		return m.ResolveEventAddresses([]Event{{Kind: PaneWriteFailed, Pane: id, Err: err}}), err
	}
	return nil, nil
}

func (m *Mux) FeedFallback(id PaneID, data []byte) ([]Event, error) {
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return nil, ErrPaneNotFound
	}
	if p.state != PaneStateFailed || p.session != nil {
		return nil, ErrPaneNotRunning
	}
	return m.advancePane(p, data), nil
}

func (m *Mux) Drain(limit int) []Event {
	var events []Event
	events = append(events, m.expireImages(m.options.Now())...)
	for count := 0; limit <= 0 || count < limit; count++ {
		var imageReady <-chan struct{}
		if m.imageScheduler != nil {
			imageReady = m.imageScheduler.ready()
		}
		select {
		case <-imageReady:
			completion, ok := m.imageScheduler.takeCompletion()
			if !ok {
				continue
			}
			events = append(events, m.applyImageCompletion(completion)...)
		case record := <-m.sessions.incoming:
			p, ok := m.sessions.lookup(record.pane)
			if !ok || p != record.owner || p.state == PaneStateClosed || p.state == PaneStateClosing {
				continue
			}
			if len(record.data) > 0 {
				events = append(events, m.advancePane(p, record.data)...)
			}
			if record.err != nil && p.state == PaneStateRunning {
				public := p.parser.EndOfInputPublic()
				if len(public) > 0 {
					events = append(events, Event{Kind: PaneOutput, Pane: p.id, Data: public})
				}
				if p.kittyAdapter != nil {
					p.kittyAdapter.Close()
					p.kittyAdapter = nil
				}
				if p.sixelAdapter != nil {
					p.sixelAdapter.Close()
					p.sixelAdapter = nil
				}
				if p.itermAdapter != nil {
					p.itermAdapter.Close()
					p.itermAdapter = nil
				}
				events = append(events, p.kittyEvents...)
				p.kittyEvents = nil
				events = append(events, m.processKittyOutcomes(p)...)
				m.processSixelOutcomes(p)
				m.processITermOutcomes(p)
				p.state = PaneStateExited
				tab := m.model.tabForPane(p.id)
				exit := Event{Kind: PaneExited, Pane: p.id}
				if tab != nil {
					exit.Tab = tab.id
					tab.revision++
				}
				if !errors.Is(record.err, io.EOF) {
					exit.Err = record.err
				}
				events = append(events, exit)
				if tab != nil {
					events = append(events, Event{Kind: TabRevisionChanged, Tab: tab.id, Revision: tab.revision})
				}
			}
		default:
			return m.ResolveEventAddresses(events)
		}
	}
	return m.ResolveEventAddresses(events)
}

func (m *Mux) ClosePane(id PaneID) ([]Event, error) {
	p, ok := m.sessions.lookup(id)
	if !ok {
		if m.sessions.wasClosed(id) {
			return nil, nil
		}
		return nil, ErrPaneNotFound
	}
	window, _ := m.WindowForPane(id)
	workspace, _ := m.WorkspaceForWindow(window)
	result, modelErr := m.model.Close(id)
	if modelErr != nil || !result.Closed {
		return nil, modelErr
	}
	detached := m.sessions.detach(id)
	if !detached.owned || detached.pane != p {
		return nil, invariantError("pane %d model detached without registry ownership", id)
	}
	delete(m.paneMetrics, id)
	closeErr := detached.pane.close()
	var events []Event
	if closeErr != nil {
		events = append(events, Event{Kind: PaneCloseFailed, Tab: result.Tab, Pane: id, Err: closeErr})
	}
	events = append(events, Event{Kind: PaneClosed, Tab: result.Tab, Pane: id})
	if result.TabClosed {
		events = append(events, Event{Kind: TabClosed, Tab: result.Tab})
	}
	if result.Focused != 0 {
		events = append(events, Event{Kind: PaneFocused, Tab: m.model.TabID(), Pane: result.Focused})
	}
	var resizeErr error
	if result.Empty {
		events = append(events, Event{Kind: WindowTabsEmpty, Tab: result.Tab}, Event{Kind: TabEmpty, Tab: result.Tab})
	} else {
		var resizeEvents []Event
		resizeEvents, resizeErr = m.resizeBoundsAndApply(m.bounds)
		events = append(events, resizeEvents...)
	}
	for i := range events {
		if events[i].Window == 0 {
			events[i].Window = window
		}
		if events[i].Workspace == 0 {
			events[i].Workspace = workspace
		}
	}
	return m.ResolveEventAddresses(events), errors.Join(closeErr, resizeErr)
}
