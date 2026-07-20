package mux

import (
	"context"
	"io"
	"sync"

	"cervterm/internal/core"
	"cervterm/internal/pty"
	"cervterm/internal/render"
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
	session        pty.Session
	launch         FreshLaunch
	snapshot       render.Snapshot
	captureOptions render.CaptureOptions
	geometry       PaneGeometry
	contentGen     uint64
	reflowGen      uint64
	viewportGen    uint64

	pendingReplies [][]byte
	desiredSize    pty.Size
	appliedSize    pty.Size
	resizeErr      error

	title     string
	cwd       string
	bellCount int

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
		captureOptions: render.CaptureOptions{HideCursorWhenScrolled: hideCursor},
		done:           make(chan struct{}),
	}
	p.parser.Reply = func(data []byte) {
		p.pendingReplies = append(p.pendingReplies, append([]byte(nil), data...))
	}
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
		if p.session != nil {
			p.closeErr = p.session.Close()
		}
		p.state = PaneStateClosed
	})
	return p.closeErr
}

func (p *pane) flushReplies() []Event {
	if len(p.pendingReplies) == 0 {
		return nil
	}
	replies := p.pendingReplies
	p.pendingReplies = nil
	if p.session == nil {
		return nil
	}
	var events []Event
	for _, reply := range replies {
		n, err := p.session.Write(reply)
		if err == nil && n != len(reply) {
			err = io.ErrShortWrite
		}
		if err != nil {
			events = append(events, Event{Kind: PaneWriteFailed, Pane: p.id, Err: err})
		}
	}
	return events
}
