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

var defaultMuxPaletteBase = core.DefaultPaletteBase()

type Mux struct {
	owner              *ownerState
	sessions           *localSessionRegistry
	sessionIngress     sessionIngressController[sessionIngressRecordAdapter, muxSessionIngressOperationAdapter]
	protocolScheduling protocolSchedulingController[
		muxProtocolSchedulingDispatchOperationAdapter,
		muxProtocolSchedulingApplyOperationAdapter,
	]
	restoreCoordinator restoreCoordinator[
		muxRestorePreparationOperationAdapter,
		muxRestorePublicationOperationAdapter,
	]
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
	paletteBase    *core.PaletteBase
	windowFault    func(string) error // package-private deterministic failure injection
	pending        *RestoreCandidate
}

func newMux(factory SessionFactory, options Options) *Mux {
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
		sessions:       sessions,
		sessionIngress: newSessionIngressController[sessionIngressRecordAdapter, muxSessionIngressOperationAdapter](),
		protocolScheduling: newProtocolSchedulingController[
			muxProtocolSchedulingDispatchOperationAdapter,
			muxProtocolSchedulingApplyOperationAdapter,
		](),
		restoreCoordinator: newRestoreCoordinator[
			muxRestorePreparationOperationAdapter,
			muxRestorePublicationOperationAdapter,
		](),
		options: options, model: NewModel(),
		paneMetrics: make(map[PaneID]CellMetrics), paletteBase: &defaultMuxPaletteBase,
	}
	sessions.owner = mux
	if options.ImageLimits != nil {
		limits, err := termimage.ValidateLimits(*options.ImageLimits)
		if err != nil {
			mux.imageSetupErr = err
		} else {
			mux.imageLimits = limits
			mux.imageBudget = termimage.NewProcessBudget()
			mux.imageScheduler = newImageDecodeScheduler(options.Wake, options.Now)
			mux.imageScheduler.owner = mux
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

func (m *Mux) bootstrap(scope mutationScope, spec SpawnSpec, content PixelRect, metrics CellMetrics) (TabID, PaneID, []Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return 0, 0, nil, err
	}
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
	if err := m.sessions.reserveScoped(scope, geometry.Pane); err != nil {
		return 0, 0, nil, err
	}
	defer m.sessions.releaseScoped(scope, geometry.Pane)
	p := m.createPane(scope, geometry.Pane, geometry.Cols, geometry.Rows)
	p.setFreshLaunch(spec)
	p.terminal.SetPaletteBase(*m.paletteBase)
	p.geometry = geometry
	if m.options.SetClipboard != nil {
		p.parser.SetClipboard = func(text string) { m.options.SetClipboard(p.id, text) }
	}
	if err := m.sessions.registerScoped(scope, p); err != nil {
		return 0, 0, nil, errors.Join(err, p.close())
	}
	m.bounds = content
	m.paneMetrics[p.id] = metrics
	m.bootstrapped = true

	rows, cols := terminalSize(geometry)
	session, spawnErr := m.sessions.spawnScoped(scope, rows, cols, spec.Options)
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
	if err := m.sessions.startScoped(scope, p.id); err != nil {
		return 0, 0, nil, errors.Join(err, m.detachAndClosePane(scope, p.id, p, true))
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
	if err := validateBounds(m.bounds); err != nil {
		return Layout{}, err
	}
	tab := m.model.activeTab()
	if tab == nil || tab.root == nil {
		return Layout{}, nil
	}
	if !tab.root.isLeaf() {
		return layoutRoot(tab.root, m.bounds, m.resolveMetrics)
	}
	metrics, ok := m.paneMetrics[tab.root.pane]
	if !ok {
		return Layout{}, ErrPaneNotFound
	}
	if err := validateCellMetrics(metrics); err != nil {
		return Layout{}, err
	}
	cols, rows := cellGeometry(m.bounds, metrics)
	panes := make([]PaneGeometry, 1)
	panes[0] = PaneGeometry{Pane: tab.root.pane, Pixels: m.bounds, Cols: cols, Rows: rows}
	return Layout{Panes: panes, Compressed: cols < MinPaneCols || rows < MinPaneRows}, nil
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
func (m *Mux) spawnSplit(scope mutationScope, origin PaneID, axis SplitAxis, spec SpawnSpec) (PaneID, []Event, error) {
	if err := scope.validPaneOrigin(m, origin); err != nil {
		return 0, nil, err
	}
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
	if err := m.sessions.reserveScoped(scope, predictedID); err != nil {
		return 0, nil, err
	}
	defer m.sessions.releaseScoped(scope, predictedID)
	newPane := m.createPane(scope, predictedID, cols, rows)
	newPane.setFreshLaunch(spec)
	newPane.terminal.SetPaletteBase(*m.paletteBase)
	if m.options.SetClipboard != nil {
		newPane.parser.SetClipboard = func(text string) { m.options.SetClipboard(newPane.id, text) }
	}
	ptyRows, ptyCols := terminalSize(PaneGeometry{Pane: predictedID, Pixels: newRect, Cols: cols, Rows: rows})
	session, spawnErr := m.sessions.spawnScoped(scope, ptyRows, ptyCols, spec.Options)
	if spawnErr != nil {
		if session != nil {
			_ = session.Close()
		}
		return 0, nil, errors.Join(fmt.Errorf("spawn split pane: %w", spawnErr), newPane.close())
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
	if err := m.sessions.registerScoped(scope, newPane); err != nil {
		return 0, nil, errors.Join(err, newPane.close())
	}
	if err := m.sessions.startScoped(scope, newPane.id); err != nil {
		return 0, nil, errors.Join(err, m.detachAndClosePane(scope, newPane.id, newPane, true))
	}
	createdID, err := m.model.SplitWithMetrics(target, axis, m.bounds, resolveSplitMetrics)
	if err != nil {
		return 0, nil, errors.Join(err, m.detachAndClosePane(scope, newPane.id, newPane, true))
	}
	if createdID != predictedID {
		preparedClose, prepareErr := newPane.prepareClose()
		if prepareErr != nil {
			return 0, nil, prepareErr
		}
		_, modelCloseErr := m.model.Close(createdID)
		detached := m.sessions.detachScoped(scope, newPane.id)
		if !detached.owned || detached.pane != newPane {
			return 0, nil, errors.Join(invariantError("model allocated pane %d after predicting %d", createdID, predictedID), modelCloseErr, preparedClose.abort())
		}
		return 0, nil, errors.Join(invariantError("model allocated pane %d after predicting %d", createdID, predictedID), modelCloseErr, preparedClose.commit())
	}
	m.paneMetrics[createdID] = targetMetrics
	resizeEvents, resizeErr := m.resizeBoundsAndApply(scope, m.bounds)
	newPane.capture()
	events := []Event{{Kind: PaneStarted, Pane: createdID}, {Kind: PaneFocused, Pane: createdID}}
	events = append(events, resizeEvents...)
	return createdID, m.ResolveEventAddresses(events), resizeErr
}

func (m *Mux) focusPane(scope mutationScope, id PaneID) ([]Event, error) {
	if err := scope.validPaneOrigin(m, id); err != nil {
		return nil, err
	}
	if err := m.model.Focus(id); err != nil {
		return nil, err
	}
	return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: id}}), nil
}

func (m *Mux) focusDirection(scope mutationScope, direction Direction) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	id, err := m.model.FocusDirectionWithMetrics(direction, m.bounds, m.resolveMetrics)
	if err != nil {
		return nil, err
	}
	return m.ResolveEventAddresses([]Event{{Kind: PaneFocused, Pane: id}}), nil
}

func (m *Mux) focusNext(scope mutationScope, reverse bool) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
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

func (m *Mux) write(scope mutationScope, id PaneID, data []byte) ([]Event, error) {
	if err := scope.validPaneOrigin(m, id); err != nil {
		return nil, err
	}
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

func (m *Mux) feedFallback(scope mutationScope, id PaneID, data []byte) ([]Event, error) {
	if err := scope.validPaneOrigin(m, id); err != nil {
		return nil, err
	}
	p, ok := m.sessions.lookup(id)
	if !ok || !m.model.paneExists(id) {
		return nil, ErrPaneNotFound
	}
	if p.state != PaneStateFailed || p.session != nil {
		return nil, ErrPaneNotRunning
	}
	return m.advancePaneScoped(scope, p, data), nil
}

// feedFallbackOwned performs the final live-origin and registry revalidation
// under one already-attested ephemeral dispatch scope. It deliberately avoids
// the public Mux path's second topology walk and registry lock.
func (m *Mux) feedFallbackOwned(scope mutationScope, origin WindowID, id PaneID, data []byte) ([]Event, error) {
	if err := scope.validPaneOrigin(m, id); err != nil {
		return nil, err
	}
	if scope.origin != (WindowIdentity{}) && (scope.origin.ID != origin || scope.origin.Incarnation == 0) {
		return nil, ErrWrongOrigin
	}
	p, ok := m.sessions.lookupOwned(id)
	if !ok {
		return nil, ErrPaneNotFound
	}
	if p.state != PaneStateFailed || p.session != nil {
		return nil, ErrPaneNotRunning
	}
	return m.advancePaneScoped(scope, p, data), nil
}

// resizeOwned reuses one native-thread-attested ephemeral dispatch scope and
// performs the final live active-window check immediately before mutation.
func (m *Mux) resizeOwned(scope mutationScope, origin WindowID, content PixelRect, metrics CellMetrics) ([]Event, error) {
	if err := scope.validActiveOrigin(m); err != nil {
		return nil, err
	}
	if scope.origin != (WindowIdentity{}) && scope.origin.ID != origin {
		return nil, ErrWrongOrigin
	}
	return m.resize(scope, content, metrics)
}

type muxSessionIngressOperationAdapter struct {
	mux   *Mux
	pane  *pane
	scope mutationScope
}

var _ sessionIngressApplyPort = muxSessionIngressOperationAdapter{}

func (a muxSessionIngressOperationAdapter) applySessionIngressData(events []Event, data []byte) []Event {
	if err := a.scope.validPaneOrigin(a.mux, a.pane.id); err != nil {
		return events
	}
	return append(events, a.mux.advancePaneScoped(a.scope, a.pane, data)...)
}

func (a muxSessionIngressOperationAdapter) applySessionIngressEnd(events []Event, err error) []Event {
	if err := a.scope.validPaneOrigin(a.mux, a.pane.id); err != nil {
		return events
	}
	if a.pane.state == PaneStateRunning {
		public := a.pane.parser.EndOfInputPublic()
		if len(public) > 0 {
			events = append(events, Event{Kind: PaneOutput, Pane: a.pane.id, Data: public})
		}
		if a.pane.kittyAdapter != nil {
			a.pane.kittyAdapter.Close()
			a.pane.kittyAdapter = nil
		}
		if a.pane.sixelAdapter != nil {
			a.pane.sixelAdapter.Close()
			a.pane.sixelAdapter = nil
		}
		if a.pane.itermAdapter != nil {
			a.pane.itermAdapter.Close()
			a.pane.itermAdapter = nil
		}
		events = append(events, a.pane.kittyEvents...)
		a.pane.kittyEvents = nil
		events = append(events, a.mux.processKittyOutcomesScoped(a.scope, a.pane)...)
		a.mux.processSixelOutcomesScoped(a.scope, a.pane)
		a.mux.processITermOutcomesScoped(a.scope, a.pane)
		a.pane.state = PaneStateExited
		tab := a.mux.model.tabForPane(a.pane.id)
		exit := Event{Kind: PaneExited, Pane: a.pane.id}
		if tab != nil {
			exit.Tab = tab.id
			tab.revision++
		}
		if !errors.Is(err, io.EOF) {
			exit.Err = err
		}
		events = append(events, exit)
		if tab != nil {
			events = append(events, Event{Kind: TabRevisionChanged, Tab: tab.id, Revision: tab.revision})
		}
	}
	return events
}

func (m *Mux) drain(scope mutationScope, limit int) []Event {
	if err := scope.valid(m); err != nil {
		return nil
	}
	var events []Event
	events = append(events, m.expireImagesScoped(scope, m.options.Now())...)
	for count := 0; limit <= 0 || count < limit; count++ {
		var imageReady <-chan struct{}
		if m.imageScheduler != nil {
			imageReady = m.imageScheduler.ready()
		}
		select {
		case <-imageReady:
			completion, ok := m.imageScheduler.takeCompletionScoped(scope, m)
			if !ok {
				continue
			}
			events = append(events, m.applyImageCompletionScoped(scope, completion)...)
		case record := <-m.sessions.incoming:
			accepted := m.sessions.adaptSessionIngressRecord(record)
			if !accepted.found {
				continue
			}
			operation := muxSessionIngressOperationAdapter{mux: m, pane: accepted.registered, scope: scope}
			events = m.sessionIngress.route(events, accepted, operation, record.data, record.err)
		default:
			return m.ResolveEventAddresses(events)
		}
	}
	return m.ResolveEventAddresses(events)
}

func (m *Mux) closePane(scope mutationScope, id PaneID) ([]Event, error) {
	if err := scope.validPaneOrigin(m, id); err != nil {
		return nil, err
	}
	p, ok := m.sessions.lookup(id)
	if !ok {
		if m.sessions.wasClosed(id) {
			return nil, nil
		}
		return nil, ErrPaneNotFound
	}
	preparedClose, err := p.prepareClose()
	if err != nil {
		return nil, err
	}
	window, _ := m.WindowForPane(id)
	workspace, _ := m.WorkspaceForWindow(window)
	result, modelErr := m.model.Close(id)
	if modelErr != nil || !result.Closed {
		return nil, errors.Join(modelErr, preparedClose.abort())
	}
	detached := m.sessions.detachScoped(scope, id)
	if !detached.owned || detached.pane != p {
		return nil, errors.Join(invariantError("pane %d model detached without registry ownership", id), preparedClose.abort())
	}
	delete(m.paneMetrics, id)
	closeErr := preparedClose.commit()
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
		resizeEvents, resizeErr = m.resizeBoundsAndApply(scope, m.bounds)
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
