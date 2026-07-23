package mux

import (
	"context"
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
				record := ingressRecord{pane: p.id, owner: p, data: append([]byte(nil), buf[:n]...)}
				if !enqueueIngress(ctx, p.done, incoming, record, wake) {
					return
				}
			}
			if err != nil {
				enqueueIngress(ctx, p.done, incoming, ingressRecord{pane: p.id, owner: p, err: err}, wake)
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

func (p *pane) close() error {
	p.closeOnce.Do(func() {
		p.state = PaneStateClosing
		close(p.done)
		if p.kittyAdapter != nil {
			p.kittyAdapter.Close()
		}
		if p.sixelAdapter != nil {
			p.sixelAdapter.Close()
		}
		if p.itermAdapter != nil {
			p.itermAdapter.Close()
		}
		for index := range p.sixelOutcomes {
			if p.sixelOutcomes[index].Command != nil {
				p.sixelOutcomes[index].Command.Close()
			}
		}
		for index := range p.itermOutcomes {
			if p.itermOutcomes[index].Command != nil {
				p.itermOutcomes[index].Command.Close()
			}
		}
		p.kittyOutcomes = nil
		p.kittyEvents = nil
		p.sixelOutcomes = nil
		p.itermOutcomes = nil
		p.terminal.CloseImageStore()
		p.clearReplies()
		if p.session != nil {
			p.closeErr = p.session.Close()
		}
		p.state = PaneStateClosed
	})
	return p.closeErr
}
