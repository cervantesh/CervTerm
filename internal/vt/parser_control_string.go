package vt

const (
	maxControlStringLen   = 256 * 1024
	maxControlStringChunk = 16 * 1024
)

// ControlStringKind identifies a framed terminal control-string transport.
type ControlStringKind uint8

const (
	ControlStringAPC ControlStringKind = iota + 1
	ControlStringDCS
)

// ControlStringEvent is delivered synchronously while parsing a control string.
// Chunk aliases parser-owned storage and is valid only until the sink returns.
// A terminal outcome has Final set; cancellation and overflow also set Cancelled.
type ControlStringEvent struct {
	Kind      ControlStringKind
	Chunk     []byte
	Final     bool
	Cancelled bool
	Overflow  bool
}

// ControlStringSink receives borrowed APC/DCS chunks. It must not retain Chunk
// without copying it and must not call Parser methods reentrantly.
type ControlStringSink func(ControlStringEvent)

type controlStringState struct {
	sink       ControlStringSink
	activeSink ControlStringSink
	total      int
	chunkLen   int
	chunk      [maxControlStringChunk]byte
}

// SetControlStringSink installs the sink captured by subsequently opened APC/DCS
// frames. Installing a non-nil sink allocates one reusable bounded state block
// outside the text-only parse path. A nil sink consumes and discards frames.
func (p *Parser) SetControlStringSink(sink ControlStringSink) {
	if sink != nil && p.control == nil {
		p.control = new(controlStringState)
	}
	if p.control != nil {
		p.control.sink = sink
	}
}

// Reset cancels one open APC/DCS candidate, drops all partial parser state, and
// returns to ground. Parser callbacks remain installed.
func (p *Parser) Reset() {
	if kind, ok := p.activeControlKind(); ok {
		p.cancelControlString(kind, false)
	} else {
		p.clearControlString()
	}
	p.state = stateGround
	p.utf8Len = 0
	p.resetCSI()
	p.resetOSC()
}

// EndOfInput cancels an incomplete control string and drops every other partial
// sequence so the parser can be reused safely.
func (p *Parser) EndOfInput() {
	p.Reset()
}

func (p *Parser) startControlString(kind ControlStringKind) {
	p.clearControlString()
	if p.control == nil || p.control.sink == nil {
		p.state = controlDiscardState(kind)
		return
	}
	p.control.activeSink = p.control.sink
	p.state = controlPayloadState(kind)
}

func (p *Parser) advanceControlState(b byte) {
	switch p.state {
	case stateAPC, stateDCS:
		p.advanceControlPayload(b)
	case stateAPCEsc, stateDCSEsc:
		p.advanceControlEscape(b)
	case stateAPCDiscard, stateDCSDiscard:
		p.advanceControlDiscard(b)
	case stateAPCDiscardEsc, stateDCSDiscardEsc:
		p.advanceControlDiscardEscape(b)
	default:
		p.state = stateGround
	}
}

func (p *Parser) advanceControlPayload(b byte) {
	kind, ok := p.activeControlKind()
	if !ok {
		p.state = stateGround
		return
	}
	switch b {
	case 0x18, 0x1a:
		p.cancelControlString(kind, false)
	case 0x1b:
		p.state = controlEscapeState(kind)
	default:
		p.appendControlByte(kind, b)
	}
}

func (p *Parser) advanceControlEscape(b byte) {
	kind, ok := p.activeControlKind()
	if !ok {
		p.state = stateGround
		return
	}
	switch b {
	case '\\':
		p.finishControlString(kind)
	case 0x18, 0x1a:
		p.cancelControlString(kind, false)
	case 0x1b:
		if p.appendControlByte(kind, 0x1b) {
			p.state = controlEscapeState(kind)
		} else {
			// The previous ESC overflowed, but the current ESC still begins a
			// possible overlapping ST while the rejected frame is discarded.
			p.state = controlDiscardEscapeState(kind)
		}
	default:
		if p.appendControlByte(kind, 0x1b) && p.appendControlByte(kind, b) {
			p.state = controlPayloadState(kind)
		}
	}
}

func (p *Parser) appendControlByte(kind ControlStringKind, b byte) bool {
	control := p.control
	if control.total >= maxControlStringLen {
		p.overflowControlString(kind)
		return false
	}
	if control.chunkLen == len(control.chunk) {
		p.emitControlChunk(kind)
	}
	control.chunk[control.chunkLen] = b
	control.chunkLen++
	control.total++
	return true
}

func (p *Parser) emitControlChunk(kind ControlStringKind) {
	control := p.control
	chunk := control.chunk[:control.chunkLen]
	control.chunkLen = 0
	control.activeSink(ControlStringEvent{Kind: kind, Chunk: chunk})
}

func (p *Parser) finishControlString(kind ControlStringKind) {
	control := p.control
	chunk := control.chunk[:control.chunkLen]
	sink := control.activeSink
	p.clearControlString()
	p.state = stateGround
	if sink != nil {
		sink(ControlStringEvent{Kind: kind, Chunk: chunk, Final: true})
	}
}

func (p *Parser) cancelControlString(kind ControlStringKind, overflow bool) {
	var sink ControlStringSink
	if p.control != nil {
		sink = p.control.activeSink
	}
	p.clearControlString()
	p.state = stateGround
	if sink != nil {
		sink(ControlStringEvent{Kind: kind, Final: true, Cancelled: true, Overflow: overflow})
	}
}

func (p *Parser) overflowControlString(kind ControlStringKind) {
	sink := p.control.activeSink
	p.clearControlString()
	p.state = controlDiscardState(kind)
	if sink != nil {
		sink(ControlStringEvent{Kind: kind, Final: true, Cancelled: true, Overflow: true})
	}
}

func (p *Parser) advanceControlDiscard(b byte) {
	kind, ok := p.discardControlKind()
	if !ok {
		p.state = stateGround
		return
	}
	switch b {
	case 0x18, 0x1a:
		p.state = stateGround
	case 0x1b:
		p.state = controlDiscardEscapeState(kind)
	}
}

func (p *Parser) advanceControlDiscardEscape(b byte) {
	kind, ok := p.discardControlKind()
	if !ok {
		p.state = stateGround
		return
	}
	switch b {
	case '\\', 0x18, 0x1a:
		p.state = stateGround
	case 0x1b:
		p.state = controlDiscardEscapeState(kind)
	default:
		p.state = controlDiscardState(kind)
	}
}

func (p *Parser) clearControlString() {
	if p.control == nil {
		return
	}
	p.control.activeSink = nil
	p.control.total = 0
	p.control.chunkLen = 0
}

func (p *Parser) activeControlKind() (ControlStringKind, bool) {
	switch p.state {
	case stateAPC, stateAPCEsc:
		return ControlStringAPC, true
	case stateDCS, stateDCSEsc:
		return ControlStringDCS, true
	default:
		return 0, false
	}
}

func (p *Parser) discardControlKind() (ControlStringKind, bool) {
	switch p.state {
	case stateAPCDiscard, stateAPCDiscardEsc:
		return ControlStringAPC, true
	case stateDCSDiscard, stateDCSDiscardEsc:
		return ControlStringDCS, true
	default:
		return 0, false
	}
}

func controlPayloadState(kind ControlStringKind) byte {
	if kind == ControlStringAPC {
		return stateAPC
	}
	return stateDCS
}

func controlEscapeState(kind ControlStringKind) byte {
	if kind == ControlStringAPC {
		return stateAPCEsc
	}
	return stateDCSEsc
}

func controlDiscardState(kind ControlStringKind) byte {
	if kind == ControlStringAPC {
		return stateAPCDiscard
	}
	return stateDCSDiscard
}

func controlDiscardEscapeState(kind ControlStringKind) byte {
	if kind == ControlStringAPC {
		return stateAPCDiscardEsc
	}
	return stateDCSDiscardEsc
}
