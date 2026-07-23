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
	ControlStringOSC1337
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

// ControlStringSink receives borrowed selected control-string chunks. It must not retain Chunk
// without copying it and must not call Parser methods reentrantly.
type ControlStringSink func(ControlStringEvent)

type controlStringState struct {
	sink        ControlStringSink
	activeSink  ControlStringSink
	total       int
	chunkLen    int
	dcsPreamble [6]byte
	dcsLen      int
	chunk       [maxControlStringChunk]byte
}

// SetControlStringSink installs the sink captured by subsequently opened selected
// control-string frames. Installing a non-nil sink allocates one reusable bounded
// state block outside the text-only parse path. A nil sink consumes and discards frames.
func (p *Parser) SetControlStringSink(sink ControlStringSink) {
	if sink != nil && p.control == nil {
		p.control = new(controlStringState)
	}
	if p.control != nil {
		p.control.sink = sink
	}
}

// Reset cancels one open selected control-string candidate, drops all partial
// parser state, and returns to ground. Parser callbacks remain installed.
func (p *Parser) Reset() {
	p.preservePublicHoldForReset()
	if kind, ok := p.activeControlKind(); ok {
		p.cancelControlString(kind, false)
	} else {
		p.clearControlString()
	}
	p.state = stateGround
	p.utf8Len = 0
	p.resetCSI()
	p.resetOSC()
	p.clearPublicProjection()
}

// EndOfInput cancels an incomplete control string and drops every other partial
// sequence so the parser can be reused safely.
func (p *Parser) EndOfInput() {
	p.Reset()
}

func (p *Parser) startControlString(kind ControlStringKind) {
	p.clearControlString()
	hasSink := p.control != nil && p.control.sink != nil
	if !hasSink && !p.publicWantsControl(kind) {
		p.state = controlDiscardState(kind)
		return
	}
	if p.control == nil {
		p.control = new(controlStringState)
	}
	p.control.activeSink = p.control.sink
	switch kind {
	case ControlStringDCS:
		p.state = stateDCSPreamble
	case ControlStringOSC1337:
		p.control.total = len("1337;")
		p.state = stateOSC1337
	default:
		p.state = stateAPC
	}
}

func (p *Parser) advanceControlState(b byte) {
	switch p.state {
	case stateAPC, stateDCS:
		p.advanceControlPayload(b)
	case stateAPCEsc, stateDCSEsc:
		p.advanceControlEscape(b)
	case stateDCSPreamble:
		p.advanceDCSPreamble(b)
	case stateDCSPreambleEsc:
		p.advanceUnselectedDCSEscape(b)
	case stateAPCDiscard, stateDCSDiscard:
		p.advanceControlDiscard(b)
	case stateAPCDiscardEsc, stateDCSDiscardEsc:
		p.advanceControlDiscardEscape(b)
	default:
		p.advanceOSC1337State(b)
	}
}

func (p *Parser) advanceOSC1337State(b byte) {
	switch p.state {
	case stateOSC1337:
		p.advanceOSC1337Payload(b)
	case stateOSC1337Esc:
		p.advanceOSC1337Escape(b)
	case stateOSC1337Discard:
		p.advanceOSC1337Discard(b)
	case stateOSC1337DiscardEsc:
		p.advanceOSC1337DiscardEscape(b)
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

func (p *Parser) advanceOSC1337Payload(b byte) {
	switch b {
	case 0x07:
		p.finishControlString(ControlStringOSC1337)
	case 0x18, 0x1a:
		p.cancelControlString(ControlStringOSC1337, false)
	case 0x1b:
		p.state = stateOSC1337Esc
	default:
		p.appendOSC1337Byte(b)
	}
}

func (p *Parser) advanceOSC1337Escape(b byte) {
	switch b {
	case '\\':
		p.finishControlString(ControlStringOSC1337)
	case 0x07:
		if p.appendOSC1337Byte(0x1b) {
			p.finishControlString(ControlStringOSC1337)
		} else {
			// BEL terminates the overflow discard immediately.
			p.state = stateGround
		}
	case 0x18, 0x1a:
		p.cancelControlString(ControlStringOSC1337, false)
	case 0x1b:
		if p.appendOSC1337Byte(0x1b) {
			p.state = stateOSC1337Esc
		} else {
			// The prior ESC overflowed, while this ESC may begin an
			// overlapping ST for the rejected frame.
			p.state = stateOSC1337DiscardEsc
		}
	default:
		if p.appendOSC1337Byte(0x1b) && p.appendOSC1337Byte(b) {
			p.state = stateOSC1337
		}
	}
}

func (p *Parser) advanceDCSPreamble(b byte) {
	switch b {
	case 0x18, 0x1a:
		p.abandonUnselectedDCS(stateGround)
		return
	case 0x1b:
		p.state = stateDCSPreambleEsc
		return
	}
	control := p.control
	if control.dcsLen >= len(control.dcsPreamble) {
		p.abandonUnselectedDCS(stateDCSDiscard)
		return
	}
	control.dcsPreamble[control.dcsLen] = b
	control.dcsLen++
	control.total++
	selected, prefix := classifyDCSPreamble(control.dcsPreamble[:control.dcsLen])
	if selected {
		p.state = stateDCS
		return
	}
	if !prefix {
		p.abandonUnselectedDCS(stateDCSDiscard)
	}
}

func classifyDCSPreamble(value []byte) (selected, prefix bool) {
	switch len(value) {
	case 1:
		return value[0] == 'q', value[0] == '0'
	case 2:
		return value[0] == '0' && value[1] == 'q', value[0] == '0' && value[1] == ';'
	case 3:
		return false, value[0] == '0' && value[1] == ';' && value[2] == '0'
	case 4:
		return value[0] == '0' && value[1] == ';' && value[2] == '0' && value[3] == 'q', value[0] == '0' && value[1] == ';' && value[2] == '0' && value[3] == ';'
	case 5:
		return false, value[0] == '0' && value[1] == ';' && value[2] == '0' && value[3] == ';' && value[4] == '0'
	case 6:
		return value[0] == '0' && value[1] == ';' && value[2] == '0' && value[3] == ';' && value[4] == '0' && value[5] == 'q', false
	default:
		return false, false
	}
}

func (p *Parser) advanceUnselectedDCSEscape(b byte) {
	switch b {
	case '\\', 0x18, 0x1a:
		p.abandonUnselectedDCS(stateGround)
	case 0x1b:
		p.abandonUnselectedDCS(stateDCSDiscardEsc)
	default:
		p.abandonUnselectedDCS(stateDCSDiscard)
	}
}

func (p *Parser) abandonUnselectedDCS(next byte) {
	// No selected DCS payload was delivered, so there is no selected transfer
	// to cancel and no adapter diagnostic to emit.
	p.clearControlString()
	p.state = next
}

func (p *Parser) appendControlByte(kind ControlStringKind, b byte) bool {
	control := p.control
	if control.total >= maxControlStringLen {
		p.overflowControlString(kind)
		return false
	}
	if control.activeSink == nil {
		control.total++
		return true
	}
	if control.chunkLen == len(control.chunk) {
		p.emitControlChunk(kind)
	}
	control.chunk[control.chunkLen] = b
	control.chunkLen++
	control.total++
	return true
}

func (p *Parser) appendOSC1337Byte(b byte) bool {
	control := p.control
	if control.total >= maxControlStringLen {
		p.overflowControlString(ControlStringOSC1337)
		return false
	}
	if control.activeSink == nil {
		control.total++
		return true
	}
	if control.chunkLen == len(control.chunk) {
		p.emitControlChunk(ControlStringOSC1337)
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
	if control.activeSink != nil {
		control.activeSink(ControlStringEvent{Kind: kind, Chunk: chunk})
	}
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

func (p *Parser) advanceOSC1337Discard(b byte) {
	switch b {
	case 0x07, 0x18, 0x1a:
		p.state = stateGround
	case 0x1b:
		p.state = stateOSC1337DiscardEsc
	}
}

func (p *Parser) advanceOSC1337DiscardEscape(b byte) {
	switch b {
	case '\\', 0x07, 0x18, 0x1a:
		p.state = stateGround
	case 0x1b:
		p.state = stateOSC1337DiscardEsc
	default:
		p.state = stateOSC1337Discard
	}
}

func (p *Parser) clearControlString() {
	if p.control == nil {
		return
	}
	p.control.activeSink = nil
	p.control.total = 0
	p.control.chunkLen = 0
	p.control.dcsLen = 0
}

func (p *Parser) activeControlKind() (ControlStringKind, bool) {
	switch p.state {
	case stateAPC, stateAPCEsc:
		return ControlStringAPC, true
	case stateDCS, stateDCSEsc:
		return ControlStringDCS, true
	case stateOSC1337, stateOSC1337Esc:
		return ControlStringOSC1337, true
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
	case stateOSC1337Discard, stateOSC1337DiscardEsc:
		return ControlStringOSC1337, true
	default:
		return 0, false
	}
}

func controlPayloadState(kind ControlStringKind) byte {
	switch kind {
	case ControlStringAPC:
		return stateAPC
	case ControlStringOSC1337:
		return stateOSC1337
	default:
		return stateDCS
	}
}

func controlEscapeState(kind ControlStringKind) byte {
	switch kind {
	case ControlStringAPC:
		return stateAPCEsc
	case ControlStringOSC1337:
		return stateOSC1337Esc
	default:
		return stateDCSEsc
	}
}

func controlDiscardState(kind ControlStringKind) byte {
	switch kind {
	case ControlStringAPC:
		return stateAPCDiscard
	case ControlStringOSC1337:
		return stateOSC1337Discard
	default:
		return stateDCSDiscard
	}
}

func controlDiscardEscapeState(kind ControlStringKind) byte {
	switch kind {
	case ControlStringAPC:
		return stateAPCDiscardEsc
	case ControlStringOSC1337:
		return stateOSC1337DiscardEsc
	default:
		return stateDCSDiscardEsc
	}
}
