package vt

import (
	"bytes"

	"cervterm/internal/core"
)

const maxPublicOutputHold = 16

type publicOutputMode uint8

const (
	publicOutputIdle publicOutputMode = iota
	publicOutputESCProbe
	publicOutputAPCProbe
	publicOutputDCSProbe
	publicOutputOSCProbe
	publicOutputSelectedAPC
	publicOutputSelectedDCS
	publicOutputSelectedOSC
)

type publicOutputProjection struct {
	kitty bool
	sixel bool
	iterm bool

	mode       publicOutputMode
	hold       [maxPublicOutputHold]byte
	holdLen    int
	holdPublic uint16

	deferred    [maxPublicOutputHold]byte
	deferredLen int
}

// SetPublicOutputRedaction selects which parser-recognized image transports are
// omitted by AdvancePublic. It does not change Advance or the control-string sink.
func (p *Parser) SetPublicOutputRedaction(kitty, sixel, iterm bool) {
	p.public.kitty = kitty
	p.public.sixel = sixel
	p.public.iterm = iterm
	if (kitty || sixel || iterm) && p.control == nil {
		// Public selection must follow the same APC/DCS/OSC states even when a
		// caller does not install a payload sink. The block is fixed and reused.
		p.control = new(controlStringState)
	}
}

// AdvancePublic advances the terminal through the existing parser and returns
// the exact input projection with enabled selected image frames removed. The
// returned bytes may alias data when no projection decision required a copy.
func (p *Parser) AdvancePublic(t *core.Terminal, data []byte) []byte {
	return p.advanceProjected(t, data, true)
}

func (p *Parser) advanceProjected(t *core.Terminal, data []byte, publish bool) []byte {
	output := publicAdvanceOutput{input: data, publish: publish}
	if publish {
		p.emitPublicDeferred(&output)
	}
	if len(data) == 0 {
		return output.result()
	}
	if !p.public.configured() && p.public.mode == publicOutputIdle && p.public.holdLen == 0 {
		p.advanceCore(t, data)
		output.pass(0, len(data))
		return output.result()
	}

	for index := 0; index < len(data); {
		// Preserve the parser's text fast path and zero-allocation projection for
		// ordinary text. Only stateful bytes are observed individually.
		if p.public.mode == publicOutputIdle && p.state == stateGround {
			nextESC := bytes.IndexByte(data[index:], 0x1b)
			if nextESC < 0 {
				p.advanceCore(t, data[index:])
				output.pass(index, len(data))
				break
			}
			if nextESC > 0 {
				end := index + nextESC
				p.advanceCore(t, data[index:end])
				output.pass(index, end)
				index = end
				continue
			}
		}

		before := p.state
		p.advanceCore(t, data[index:index+1])
		p.projectPublicByte(&output, index, before, p.state, data[index])
		index++
	}
	return output.result()
}

// EndOfInputPublic flushes reset-deferred and currently undecided public bytes,
// drops a partial selected frame, and applies normal EndOfInput parser cleanup.
func (p *Parser) EndOfInputPublic() []byte {
	output := make([]byte, 0, p.public.deferredLen+p.public.holdLen)
	output = append(output, p.public.deferred[:p.public.deferredLen]...)
	p.clearPublicDeferred()
	if !p.public.selected() {
		for index := 0; index < p.public.holdLen; index++ {
			if p.public.holdPublic&(uint16(1)<<uint(index)) != 0 {
				output = append(output, p.public.hold[index])
			}
		}
	}
	// Clear projection state before EndOfInput calls Reset so already exposed
	// undecided bytes are not deferred a second time.
	p.clearPublicProjection()
	p.EndOfInput()
	return output
}

func (p *Parser) projectPublicByte(output *publicAdvanceOutput, index int, before, after, b byte) {
	switch p.public.mode {
	case publicOutputESCProbe:
		p.projectPublicESC(output, index, after, b)
	case publicOutputAPCProbe:
		p.projectPublicAPC(output, index, before, after, b)
	case publicOutputDCSProbe:
		p.projectPublicDCS(output, index, before, after, b)
	case publicOutputOSCProbe:
		p.projectPublicOSC(output, index, after, b)
	case publicOutputSelectedAPC, publicOutputSelectedDCS, publicOutputSelectedOSC:
		output.diverge(index)
		if !publicSelectedState(p.public.mode, after) {
			p.public.mode = publicOutputIdle
		}
	default:
		if before == stateGround && b == 0x1b && after == stateEsc && p.public.configured() {
			p.public.mode = publicOutputESCProbe
			p.holdPublicByte(output, index, b)
			return
		}
		output.pass(index, index+1)
	}
}

func (p *Parser) projectPublicESC(output *publicAdvanceOutput, index int, after, b byte) {
	switch {
	case b == 0x1b && after == stateEsc:
		// The parser restarts ESC. The old introducer is public; the new one is
		// the only byte still capable of opening an enabled selected frame.
		output.diverge(index)
		p.emitPublicHold(output)
		p.holdPublicByte(output, index, b)
	case b == '_' && p.public.kitty && (after == stateAPC || after == stateAPCDiscard):
		p.public.mode = publicOutputAPCProbe
		p.holdPublicByte(output, index, b)
	case b == 'P' && p.public.sixel && after == stateDCSPreamble:
		p.public.mode = publicOutputDCSProbe
		p.holdPublicByte(output, index, b)
	case b == ']' && p.public.iterm && after == stateOSC:
		p.public.mode = publicOutputOSCProbe
		p.holdPublicByte(output, index, b)
	default:
		p.holdPublicByte(output, index, b)
		p.emitPublicHold(output)
		p.public.mode = publicOutputIdle
	}
}

func (p *Parser) projectPublicAPC(output *publicAdvanceOutput, index int, before, after, b byte) {
	p.holdPublicByte(output, index, b)
	switch before {
	case stateAPC, stateAPCDiscard:
		switch b {
		case 0x18, 0x1a:
			p.emitPublicHold(output)
			p.public.mode = publicOutputIdle
		case 0x1b:
			// Wait for ST. A non-ST continuation makes the held ESC the first body byte.
		default:
			if b == 'G' {
				p.clearPublicHold()
				p.public.mode = publicOutputSelectedAPC
				if !publicSelectedState(p.public.mode, after) {
					p.public.mode = publicOutputIdle
				}
				return
			}
			p.emitPublicHold(output)
			p.public.mode = publicOutputIdle
		}
	case stateAPCEsc, stateAPCDiscardEsc:
		// ST/cancel closes an empty nonselected APC. Every other continuation
		// exposes the held ESC as the first body byte, so it is not Kitty.
		p.emitPublicHold(output)
		p.public.mode = publicOutputIdle
	default:
		p.emitPublicHold(output)
		p.public.mode = publicOutputIdle
	}
}

func (p *Parser) projectPublicDCS(output *publicAdvanceOutput, index int, before, after, b byte) {
	p.holdPublicByte(output, index, b)
	if before == stateDCSPreamble && after == stateDCS {
		// advanceDCSPreamble reached an exact classifyDCSPreamble selection.
		p.clearPublicHold()
		p.public.mode = publicOutputSelectedDCS
		return
	}
	if after == stateDCSPreamble || after == stateDCSPreambleEsc {
		return
	}
	p.emitPublicHold(output)
	p.public.mode = publicOutputIdle
}

func (p *Parser) projectPublicOSC(output *publicAdvanceOutput, index int, after byte, b byte) {
	p.holdPublicByte(output, index, b)
	if after == stateOSC1337 {
		// selectOSC1337 made the exact five-byte selection.
		p.clearPublicHold()
		p.public.mode = publicOutputSelectedOSC
		return
	}
	if (after == stateOSC || after == stateOSCEsc) && len(p.osc) < 5 && !p.oscTruncated {
		return
	}
	p.emitPublicHold(output)
	p.public.mode = publicOutputIdle
}

func (p *Parser) holdPublicByte(output *publicAdvanceOutput, index int, b byte) {
	output.diverge(index)
	if p.public.holdLen == len(p.public.hold) {
		// Exact selectors decide well before this bound. Preserve bytes rather
		// than retaining or dropping data if a future parser change violates it.
		p.emitPublicHold(output)
		output.pass(index, index+1)
		p.public.mode = publicOutputIdle
		return
	}
	holdIndex := p.public.holdLen
	p.public.hold[holdIndex] = b
	if output.publish {
		p.public.holdPublic |= uint16(1) << uint(holdIndex)
	}
	p.public.holdLen++
}

func (p *Parser) emitPublicHold(output *publicAdvanceOutput) {
	for index := 0; index < p.public.holdLen; index++ {
		if p.public.holdPublic&(uint16(1)<<uint(index)) == 0 {
			continue
		}
		if output.publish {
			output.emit(p.public.hold[index : index+1])
		} else {
			p.deferPublicByte(p.public.hold[index])
		}
	}
	p.clearPublicHold()
}

func (p *Parser) preservePublicHoldForReset() {
	if p.public.selected() {
		p.clearPublicHold()
		return
	}
	// AdvancePublic drains deferred bytes before it can create another public
	// hold, so every Reset can preserve the exposed hold in the fixed buffer.
	for index := 0; index < p.public.holdLen; index++ {
		if p.public.holdPublic&(uint16(1)<<uint(index)) != 0 {
			p.deferPublicByte(p.public.hold[index])
		}
	}
	p.clearPublicHold()
}

func (p *Parser) deferPublicByte(value byte) {
	if p.public.deferredLen >= len(p.public.deferred) {
		return // Fixed invariant: AdvancePublic drains before creating another public hold.
	}
	p.public.deferred[p.public.deferredLen] = value
	p.public.deferredLen++
}

func (p *Parser) emitPublicDeferred(output *publicAdvanceOutput) {
	if p.public.deferredLen == 0 {
		return
	}
	output.diverge(0)
	output.emit(p.public.deferred[:p.public.deferredLen])
	p.clearPublicDeferred()
}

func (p *Parser) clearPublicHold() {
	clear(p.public.hold[:p.public.holdLen])
	p.public.holdLen = 0
	p.public.holdPublic = 0
}

func (p *Parser) clearPublicDeferred() {
	clear(p.public.deferred[:p.public.deferredLen])
	p.public.deferredLen = 0
}

func (p *Parser) clearPublicProjection() {
	p.clearPublicHold()
	p.public.mode = publicOutputIdle
}

func (p *Parser) publicWantsControl(kind ControlStringKind) bool {
	switch kind {
	case ControlStringAPC:
		return p.public.kitty
	case ControlStringDCS:
		return p.public.sixel
	case ControlStringOSC1337:
		return p.public.iterm
	default:
		return false
	}
}

func (p *publicOutputProjection) configured() bool {
	return p.kitty || p.sixel || p.iterm
}

func (p *publicOutputProjection) selected() bool {
	return p.mode == publicOutputSelectedAPC || p.mode == publicOutputSelectedDCS || p.mode == publicOutputSelectedOSC
}

func publicSelectedState(mode publicOutputMode, state byte) bool {
	switch mode {
	case publicOutputSelectedAPC:
		return state == stateAPC || state == stateAPCEsc || state == stateAPCDiscard || state == stateAPCDiscardEsc
	case publicOutputSelectedDCS:
		return state == stateDCS || state == stateDCSEsc || state == stateDCSDiscard || state == stateDCSDiscardEsc
	case publicOutputSelectedOSC:
		return state == stateOSC1337 || state == stateOSC1337Esc || state == stateOSC1337Discard || state == stateOSC1337DiscardEsc
	default:
		return false
	}
}

type publicAdvanceOutput struct {
	input    []byte
	output   []byte
	publish  bool
	diverged bool
}

func (o *publicAdvanceOutput) diverge(index int) {
	if o.diverged {
		return
	}
	o.diverged = true
	if o.publish {
		o.output = append(o.output, o.input[:index]...)
	}
}

func (o *publicAdvanceOutput) pass(start, end int) {
	if o.publish && o.diverged && start < end {
		o.output = append(o.output, o.input[start:end]...)
	}
}

func (o *publicAdvanceOutput) emit(data []byte) {
	if o.publish {
		o.output = append(o.output, data...)
	}
}

func (o *publicAdvanceOutput) result() []byte {
	if !o.publish {
		return nil
	}
	if !o.diverged {
		return o.input
	}
	return o.output
}
