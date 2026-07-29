package mux

import (
	"context"
	"errors"
	"sync"

	"cervterm/internal/core"
	"cervterm/internal/itermimage"
	"cervterm/internal/kitty"
	"cervterm/internal/pty"
	"cervterm/internal/render"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
	"cervterm/internal/vt"
)

// PaneState describes the lifecycle of one local terminal aggregate.
type PaneState uint8

const (
	PaneStateStarting PaneState = iota + 1
	PaneStateRunning
	PaneStateExited
	PaneStateClosing
	PaneStateClosed
	PaneStateFailed
)

type pane struct {
	id             PaneID
	state          PaneState
	ownerStamp     ownerStamp
	activeScope    mutationScope
	terminal       *core.Terminal
	parser         vt.Parser
	imageStore     *termimage.Store
	kittyAdapter   *kitty.Adapter
	kittyOutcomes  []kitty.Outcome
	kittyEvents    []Event
	sixelAdapter   *sixel.Adapter
	sixelOutcomes  []sixel.Outcome
	itermAdapter   *itermimage.Adapter
	itermOutcomes  []itermimage.Outcome
	session        pty.Session
	launch         FreshLaunch
	snapshot       render.Snapshot
	captureOptions render.CaptureOptions
	geometry       PaneGeometry
	contentGen     uint64
	reflowGen      uint64
	viewportGen    uint64

	replies     replyQueue
	desiredSize pty.Size
	appliedSize pty.Size
	resizeErr   error

	title               string
	cwd                 string
	bellCount           int
	notificationSeq     uint64
	notificationScratch []core.NotificationRequest

	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func newPane(id PaneID, cols, rows int, scrollbackCapacity *int, hideCursorWhenScrolled *bool) *pane {
	terminal := core.NewTerminal(cols, rows)
	if scrollbackCapacity != nil {
		terminal = core.NewTerminalWithHistory(cols, rows, *scrollbackCapacity)
	}
	hideCursor := true
	if hideCursorWhenScrolled != nil {
		hideCursor = *hideCursorWhenScrolled
	}
	p := &pane{
		id:             id,
		state:          PaneStateStarting,
		terminal:       terminal,
		captureOptions: render.CaptureOptions{HideCursorWhenScrolled: hideCursor, PaneObject: uint64(id)},
		done:           make(chan struct{}),
	}
	p.parser.Reply = func(data []byte) { p.queueReply(data) }
	p.capture()
	return p
}

func (p *pane) capture() {
	render.CaptureWithOptions(&p.snapshot, p.terminal, p.captureOptions)
	p.title = p.snapshot.Title
	p.cwd = p.snapshot.Cwd
	p.bellCount = p.snapshot.BellCount
}

func (p *pane) setFreshLaunch(spec SpawnSpec) {
	p.launch = FreshLaunch{TargetID: spec.TargetID, Program: spec.Options.ShellProgram, Args: append([]string(nil), spec.Options.ShellArgs...), CWD: spec.Options.WorkingDirectory}
}

func (p *pane) startReader(ctx context.Context, incoming chan<- ingressRecord, wake func(), readers *sync.WaitGroup) {
	if p.session == nil {
		return
	}
	readers.Add(1)
	p.launchReader(ctx, incoming, wake, readers)
}

// launchReader starts a reader whose WaitGroup slot has already been reserved.
func (p *pane) launchReader(ctx context.Context, incoming chan<- ingressRecord, wake func(), readers *sync.WaitGroup) {
	reader := p.session.Reader()
	go func() {
		defer readers.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				record := ingressRecord{pane: p.id, owner: p, stamp: p.ownerStamp, data: append([]byte(nil), buf[:n]...)}
				if !enqueueIngress(ctx, p.done, incoming, record, wake) {
					return
				}
			}
			if err != nil {
				enqueueIngress(ctx, p.done, incoming, ingressRecord{pane: p.id, owner: p, stamp: p.ownerStamp, err: err}, wake)
				return
			}
		}
	}()
}

func enqueueIngress(ctx context.Context, done <-chan struct{}, incoming chan<- ingressRecord, record ingressRecord, wake func()) bool {
	select {
	case incoming <- record:
		if wake != nil {
			wake()
		}
		return true
	case <-done:
		return false
	case <-ctx.Done():
		return false
	}
}

type preparedPaneClose struct {
	pane     *pane
	images   *core.PreparedImageStoreClose
	finished bool
}

func (p *pane) prepareClose() (*preparedPaneClose, error) {
	if p == nil || p.terminal == nil {
		return nil, ErrInvariant
	}
	images, err := p.terminal.PrepareCloseImageStore()
	if err != nil {
		return nil, err
	}
	return &preparedPaneClose{pane: p, images: images}, nil
}

func (p *preparedPaneClose) abort() error {
	if p == nil || p.finished {
		return nil
	}
	if err := p.images.Abort(); err != nil {
		return err
	}
	p.finished = true
	return nil
}

func (p *preparedPaneClose) commit() error {
	if p == nil || p.pane == nil {
		return ErrInvariant
	}
	if p.finished {
		return p.pane.closeErr
	}
	if err := p.images.Commit(); err != nil {
		return err
	}
	p.finished = true
	p.pane.imageStore = nil
	p.pane.closeOnce.Do(func() {
		p.pane.state = PaneStateClosing
		close(p.pane.done)
		if p.pane.kittyAdapter != nil {
			p.pane.kittyAdapter.Close()
		}
		if p.pane.sixelAdapter != nil {
			p.pane.sixelAdapter.Close()
		}
		if p.pane.itermAdapter != nil {
			p.pane.itermAdapter.Close()
		}
		for index := range p.pane.sixelOutcomes {
			if p.pane.sixelOutcomes[index].Command != nil {
				p.pane.sixelOutcomes[index].Command.Close()
			}
		}
		for index := range p.pane.itermOutcomes {
			if p.pane.itermOutcomes[index].Command != nil {
				p.pane.itermOutcomes[index].Command.Close()
			}
		}
		p.pane.kittyOutcomes = nil
		p.pane.kittyEvents = nil
		p.pane.sixelOutcomes = nil
		p.pane.itermOutcomes = nil
		p.pane.clearReplies()
		if p.pane.session != nil {
			p.pane.closeErr = p.pane.session.Close()
		}
		p.pane.state = PaneStateClosed
	})
	return p.pane.closeErr
}

func preparePaneClosures(panes []*pane) ([]*preparedPaneClose, error) {
	prepared := make([]*preparedPaneClose, 0, len(panes))
	for _, p := range panes {
		closeState, err := p.prepareClose()
		if err != nil {
			var abortErrors []error
			for index := len(prepared) - 1; index >= 0; index-- {
				abortErrors = append(abortErrors, prepared[index].abort())
			}
			return nil, errors.Join(err, errors.Join(abortErrors...))
		}
		prepared = append(prepared, closeState)
	}
	return prepared, nil
}

func abortPaneClosures(prepared []*preparedPaneClose) error {
	var abortErrors []error
	for index := len(prepared) - 1; index >= 0; index-- {
		abortErrors = append(abortErrors, prepared[index].abort())
	}
	return errors.Join(abortErrors...)
}

func (p *pane) close() error {
	prepared, err := p.prepareClose()
	if err != nil {
		return err
	}
	return prepared.commit()
}

func (m *Mux) detachAndClosePane(scope mutationScope, id PaneID, expected *pane, tombstone bool) error {
	if err := scope.valid(m); err != nil {
		return err
	}
	prepared, err := expected.prepareClose()
	if err != nil {
		return err
	}
	var detached detachResult
	if tombstone {
		detached = m.sessions.detachScoped(scope, id)
	} else {
		detached = m.sessions.abortScoped(scope, id, expected)
	}
	if !detached.owned || detached.pane != expected {
		return errors.Join(invariantError("pane %d changed ownership before close", id), prepared.abort())
	}
	return prepared.commit()
}
