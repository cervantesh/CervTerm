package vt

import (
	"bytes"
	"reflect"
	"testing"

	"cervterm/internal/core"
)

type publicOutputMask struct {
	kitty bool
	sixel bool
	iterm bool
}

type publicParseResult struct {
	output  []byte
	events  []capturedControlEvent
	text    string
	maxHold int
}

func TestPublicOutputProjectionExactSelectionEverySplit(t *testing.T) {
	kitty := []byte("\x1b_GSELECTED_KITTY\x1b\\")
	sixel := []byte("\x1bP0;0;0qSELECTED_SIXEL\x1b\\")
	iterm := []byte("\x1b]1337;SELECTED_ITERM\x07")
	input := append([]byte("A"), kitty...)
	input = append(input, 'B')
	input = append(input, sixel...)
	input = append(input, 'C')
	input = append(input, iterm...)
	input = append(input, 'D')
	want := []byte("ABCD")
	mask := publicOutputMask{kitty: true, sixel: true, iterm: true}

	unsplit := parsePublicOutput(t, input, nil, mask)
	if !bytes.Equal(unsplit.output, want) {
		t.Fatalf("output=%q want=%q", unsplit.output, want)
	}
	if bytes.Contains(unsplit.output, []byte("SELECTED_")) {
		t.Fatalf("selected marker leaked: %q", unsplit.output)
	}
	legacyEvents, legacyText := parseLegacyOracle(t, input, nil)
	if !reflect.DeepEqual(unsplit.events, legacyEvents) || unsplit.text != legacyText {
		t.Fatalf("public parse changed parser oracle: events=%#v/%#v text=%q/%q", unsplit.events, legacyEvents, unsplit.text, legacyText)
	}
	for split := 0; split <= len(input); split++ {
		got := parsePublicOutput(t, input, []int{split}, mask)
		if !bytes.Equal(got.output, want) || !reflect.DeepEqual(got.events, unsplit.events) || got.text != unsplit.text {
			t.Fatalf("split=%d output=%q events=%#v text=%q", split, got.output, got.events, got.text)
		}
	}
	oneByte := make([]int, 0, len(input)-1)
	for split := 1; split < len(input); split++ {
		oneByte = append(oneByte, split)
	}
	got := parsePublicOutput(t, input, oneByte, mask)
	if !bytes.Equal(got.output, want) || !reflect.DeepEqual(got.events, unsplit.events) || got.text != unsplit.text {
		t.Fatalf("one-byte fragmentation output=%q events=%#v text=%q", got.output, got.events, got.text)
	}
	if got.maxHold > 16 {
		t.Fatalf("projection hold=%d exceeds 16", got.maxHold)
	}
}

func TestPublicOutputProjectionMixedAdvanceThenAdvancePublic(t *testing.T) {
	tests := []struct {
		name    string
		partial []byte
		rest    []byte
		kind    ControlStringKind
		payload []byte
	}{
		{name: "APC", partial: []byte("\x1b_"), rest: []byte("GAPC_MIXED_MARKER\x1b\\TAIL"), kind: ControlStringAPC, payload: []byte("GAPC_MIXED_MARKER")},
		{name: "DCS", partial: []byte("\x1bP0;0"), rest: []byte("qDCS_MIXED_MARKER\x1b\\TAIL"), kind: ControlStringDCS, payload: []byte("DCS_MIXED_MARKER")},
		{name: "OSC", partial: []byte("\x1b]133"), rest: []byte("7;OSC_MIXED_MARKER\x07TAIL"), kind: ControlStringOSC1337, payload: []byte("OSC_MIXED_MARKER")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			term := core.NewTerminal(40, 2)
			var parser Parser
			var events []capturedControlEvent
			parser.SetControlStringSink(captureControlEvents(&events))
			parser.SetPublicOutputRedaction(true, true, true)

			parser.Advance(term, test.partial)
			if out := parser.AdvancePublic(term, test.rest); !bytes.Equal(out, []byte("TAIL")) {
				t.Fatalf("selected continuation output=%q", out)
			}
			if out := parser.AdvancePublic(term, []byte("NEXT")); !bytes.Equal(out, []byte("NEXT")) {
				t.Fatalf("subsequent output=%q", out)
			}
			if len(events) != 1 || events[0].Kind != test.kind || !events[0].Final || events[0].Cancelled || events[0].Overflow || !bytes.Equal(events[0].Chunk, test.payload) {
				t.Fatalf("events=%#v want final kind=%d payload=%q", events, test.kind, test.payload)
			}
			if text := []byte(term.PlainText()); bytes.Contains(text, []byte("MIXED_MARKER")) {
				t.Fatalf("selected marker corrupted terminal text: %q", text)
			}
			if out := parser.EndOfInputPublic(); len(out) != 0 {
				t.Fatalf("EOF output=%q", out)
			}
		})
	}
}

func TestPublicOutputProjectionPublicThenSilentNonselectedPreservesPublishedHold(t *testing.T) {
	term := core.NewTerminal(20, 1)
	var parser Parser
	parser.SetPublicOutputRedaction(true, true, true)
	if out := parser.AdvancePublic(term, []byte("\x1b")); len(out) != 0 {
		t.Fatalf("partial output=%q", out)
	}
	parser.Advance(term, []byte("X")) // This call intentionally discards X, but not the prior public ESC.
	if out := parser.AdvancePublic(term, []byte("Z")); !bytes.Equal(out, []byte("\x1bZ")) {
		t.Fatalf("resumed output=%q want=%q", out, []byte("\x1bZ"))
	}
}

func TestPublicOutputProjectionPreservesNonselectedAndDisabledBytes(t *testing.T) {
	kitty := []byte("\x1b_GKITTY_SECRET\x1b\\")
	sixel := []byte("\x1bPqSIXEL_SECRET\x1b\\")
	iterm := []byte("\x1b]1337;ITERM_SECRET\x1b\\")
	for bits := 0; bits < 8; bits++ {
		mask := publicOutputMask{kitty: bits&1 != 0, sixel: bits&2 != 0, iterm: bits&4 != 0}
		input := append([]byte("a"), kitty...)
		input = append(input, 'b')
		input = append(input, sixel...)
		input = append(input, 'c')
		input = append(input, iterm...)
		input = append(input, 'd')
		want := []byte("a")
		if !mask.kitty {
			want = append(want, kitty...)
		}
		want = append(want, 'b')
		if !mask.sixel {
			want = append(want, sixel...)
		}
		want = append(want, 'c')
		if !mask.iterm {
			want = append(want, iterm...)
		}
		want = append(want, 'd')
		got := parsePublicOutput(t, input, everyByteSplit(len(input)), mask)
		if !bytes.Equal(got.output, want) {
			t.Fatalf("mask=%03b output=%q want=%q", bits, got.output, want)
		}
	}

	mask := publicOutputMask{kitty: true, sixel: true, iterm: true}
	for _, input := range [][]byte{
		[]byte("plain UTF-8: café\r\n"),
		[]byte("\x1b_not-kitty-G\x1b\\"),
		[]byte("\x1bP1qnot-sixel\x1b\\"),
		[]byte("\x1bP0;1qnot-sixel\x1b\\"),
		[]byte("\x1b]1337:not-iterm\x07"),
		[]byte("\x1b]01337;not-iterm\x1b\\"),
		[]byte("\x1bXstill-public\x1b\\"),
		[]byte("\x1b^still-public\x1b\\"),
	} {
		got := parsePublicOutput(t, input, everyByteSplit(len(input)), mask)
		if !bytes.Equal(got.output, input) {
			t.Fatalf("input=%q output=%q", input, got.output)
		}
	}
}

func TestPublicOutputProjectionReviewRepros(t *testing.T) {
	selectedKitty := "\x1b_GKITTY_MARKER\x1b\\"
	selectedSixel := "\x1bP0;0qSIXEL_MARKER\x1b\\"
	selectedITerm := "\x1b]1337;ITERM_MARKER\x07"
	mask := publicOutputMask{kitty: true, sixel: true, iterm: true}
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{name: "SOS then selected", input: []byte("\x1bXpublic" + selectedKitty + "tail"), want: []byte("\x1bXpublictail")},
		{name: "PM then selected", input: []byte("\x1b^public" + selectedSixel + "tail"), want: []byte("\x1b^publictail")},
		{name: "APC CAN then Kitty", input: []byte("\x1b_\x18" + selectedKitty + "tail"), want: []byte("\x1b_\x18tail")},
		{name: "partial DCS cancel then selected", input: []byte("\x1bP0\x18" + selectedSixel + "tail"), want: []byte("\x1bP0\x18tail")},
		{name: "partial selected OSC cancel then selected", input: []byte("\x1b]1337;partial\x1a" + selectedITerm + "tail"), want: []byte("tail")},
		{name: "selected Kitty CAN", input: []byte("\x1b_GKITTY_CANCEL\x18tail"), want: []byte("tail")},
		{name: "selected Sixel SUB", input: []byte("\x1bP0qSIXEL_CANCEL\x1atail"), want: []byte("tail")},
		{name: "selected iTerm escape CAN", input: []byte("\x1b]1337;ITERM_CANCEL\x1b\x18tail"), want: []byte("tail")},
		{name: "generic OSC CAN SUB ESC nesting", input: []byte("\x1b]2;a\x18b\x1ac\x1bxnested\x1b\\" + selectedITerm + "tail"), want: []byte("\x1b]2;a\x18b\x1ac\x1bxnested\x1b\\tail")},
		{name: "CSI ESC cannot introduce APC", input: []byte("\x1b[31\x1b_GNOT_SELECTED\x1b\\tail"), want: []byte("\x1b[31\x1b_GNOT_SELECTED\x1b\\tail")},
		{name: "charset ESC cannot introduce APC", input: []byte("\x1b(\x1b_GNOT_SELECTED\x1b\\tail"), want: []byte("\x1b(\x1b_GNOT_SELECTED\x1b\\tail")},
		{name: "charset then selected", input: []byte("\x1b(0public" + selectedKitty + "tail"), want: []byte("\x1b(0publictail")},
		{name: "back-to-back mixed", input: []byte("a" + selectedKitty + selectedSixel + selectedITerm + "z"), want: []byte("az")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			unsplit := parsePublicOutput(t, test.input, nil, mask)
			if !bytes.Equal(unsplit.output, test.want) {
				t.Fatalf("output=%q want=%q", unsplit.output, test.want)
			}
			for split := 0; split <= len(test.input); split++ {
				got := parsePublicOutput(t, test.input, []int{split}, mask)
				if !bytes.Equal(got.output, test.want) || !reflect.DeepEqual(got.events, unsplit.events) || got.text != unsplit.text {
					t.Fatalf("split=%d output=%q events=%#v text=%q", split, got.output, got.events, got.text)
				}
			}
		})
	}
}

func TestPublicOutputProjectionOverflowResetAndEOF(t *testing.T) {
	mask := publicOutputMask{kitty: true, sixel: true, iterm: true}
	payload := bytes.Repeat([]byte{'x'}, maxControlStringLen+1)
	input := append([]byte("A\x1bPq"), payload...)
	input = append(input, []byte("HIDDEN\x1b\\Z")...)
	got := parsePublicOutput(t, input, []int{1, 2, 3, maxControlStringLen, len(input) - 2}, mask)
	if !bytes.Equal(got.output, []byte("AZ")) {
		t.Fatalf("overflow output=%q", got.output)
	}

	t.Run("EOF flushes undecided DCS", func(t *testing.T) {
		term := core.NewTerminal(20, 1)
		var parser Parser
		parser.SetPublicOutputRedaction(true, true, true)
		if out := parser.AdvancePublic(term, []byte("A\x1bP0;0")); !bytes.Equal(out, []byte("A")) {
			t.Fatalf("advance=%q", out)
		}
		if out := parser.EndOfInputPublic(); !bytes.Equal(out, []byte("\x1bP0;0")) {
			t.Fatalf("EOF=%q", out)
		}
	})
	t.Run("EOF flushes undecided OSC and ESC", func(t *testing.T) {
		for _, input := range [][]byte{[]byte("\x1b]133"), []byte("\x1b")} {
			term := core.NewTerminal(20, 1)
			var parser Parser
			parser.SetPublicOutputRedaction(true, true, true)
			if out := parser.AdvancePublic(term, input); len(out) != 0 {
				t.Fatalf("input=%q advance=%q", input, out)
			}
			if out := parser.EndOfInputPublic(); !bytes.Equal(out, input) {
				t.Fatalf("input=%q EOF=%q", input, out)
			}
		}
	})
	t.Run("EOF drops selected partial", func(t *testing.T) {
		term := core.NewTerminal(20, 1)
		var parser Parser
		parser.SetPublicOutputRedaction(true, true, true)
		if out := parser.AdvancePublic(term, []byte("A\x1b_GSELECTED_PARTIAL")); !bytes.Equal(out, []byte("A")) {
			t.Fatalf("advance=%q", out)
		}
		if out := parser.EndOfInputPublic(); len(out) != 0 {
			t.Fatalf("EOF leaked=%q", out)
		}
	})
	t.Run("Reset defers undecided public bytes", func(t *testing.T) {
		for _, input := range [][]byte{[]byte("\x1b"), []byte("\x1b]133"), []byte("\x1bP0;0")} {
			term := core.NewTerminal(20, 1)
			var parser Parser
			parser.SetPublicOutputRedaction(true, true, true)
			if out := parser.AdvancePublic(term, input); len(out) != 0 {
				t.Fatalf("input=%q advance=%q", input, out)
			}
			parser.Reset()
			if out := parser.AdvancePublic(term, []byte("Z")); !bytes.Equal(out, append(append([]byte(nil), input...), 'Z')) {
				t.Fatalf("input=%q after reset=%q", input, out)
			}
			if parser.public.deferredLen != 0 || len(parser.public.deferred) != maxPublicOutputHold {
				t.Fatalf("input=%q deferred len=%d capacity=%d", input, parser.public.deferredLen, len(parser.public.deferred))
			}
		}
	})
	t.Run("Reset exposes deferred bytes at public EOF", func(t *testing.T) {
		term := core.NewTerminal(20, 1)
		var parser Parser
		parser.SetPublicOutputRedaction(true, true, true)
		input := []byte("\x1bP0;0")
		if out := parser.AdvancePublic(term, input); len(out) != 0 {
			t.Fatalf("advance=%q", out)
		}
		parser.Reset()
		if out := parser.EndOfInputPublic(); !bytes.Equal(out, input) {
			t.Fatalf("EOF=%q want=%q", out, input)
		}
	})
	t.Run("Reset still drops selected partial", func(t *testing.T) {
		for _, input := range [][]byte{[]byte("\x1b_GSELECTED_PARTIAL"), []byte("\x1bPqSELECTED_PARTIAL"), []byte("\x1b]1337;SELECTED_PARTIAL")} {
			term := core.NewTerminal(20, 1)
			var parser Parser
			parser.SetPublicOutputRedaction(true, true, true)
			if out := parser.AdvancePublic(term, input); len(out) != 0 {
				t.Fatalf("input=%q advance=%q", input, out)
			}
			parser.Reset()
			if out := parser.AdvancePublic(term, []byte("Z")); !bytes.Equal(out, []byte("Z")) {
				t.Fatalf("input=%q after reset=%q", input, out)
			}
		}
	})
}

func FuzzPublicOutputProjectionDualOracle(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("plain"),
		[]byte("\x1bXpublic\x1b_Gsecret\x1b\\tail"),
		[]byte("\x1b_\x18\x1b_Gsecret\x1b\\"),
		[]byte("\x1bP0\x18\x1bPqsecret\x1b\\"),
		[]byte("\x1b]2;a\x18b\x1ac\x1bx\x1b\\"),
		[]byte("\x1b[31\x1b_Gnot-selected\x1b\\"),
	} {
		f.Add(seed, uint8(7), uint32(1))
	}
	f.Fuzz(func(t *testing.T, input []byte, bits uint8, splitSeed uint32) {
		if len(input) > 4096 {
			input = input[:4096]
		}
		mask := publicOutputMask{kitty: bits&1 != 0, sixel: bits&2 != 0, iterm: bits&4 != 0}
		legacyEvents, legacyText := parseLegacyOracle(t, input, nil)
		unsplit := parsePublicOutput(t, input, nil, mask)
		if !reflect.DeepEqual(unsplit.events, legacyEvents) || unsplit.text != legacyText {
			t.Fatalf("public parser differs from legacy oracle")
		}
		point := 0
		if len(input) > 0 {
			point = int(uint64(splitSeed) % uint64(len(input)+1))
		}
		fragmented := parsePublicOutput(t, input, []int{point}, mask)
		if !bytes.Equal(fragmented.output, unsplit.output) || !reflect.DeepEqual(fragmented.events, unsplit.events) || fragmented.text != unsplit.text {
			t.Fatalf("split=%d projection differs", point)
		}
		oneByte := parsePublicOutput(t, input, everyByteSplit(len(input)), mask)
		if !bytes.Equal(oneByte.output, unsplit.output) || !reflect.DeepEqual(oneByte.events, unsplit.events) || oneByte.text != unsplit.text {
			t.Fatal("one-byte projection differs")
		}
		if oneByte.maxHold > 16 {
			t.Fatalf("projection hold=%d", oneByte.maxHold)
		}
	})
}

func FuzzPublicOutputProjectionSelectedEnvelope(f *testing.F) {
	for _, seed := range []struct {
		payload   []byte
		protocol  uint8
		prefix    uint8
		splitSeed uint32
		useST     bool
	}{
		{payload: []byte("kitty"), protocol: 0, prefix: 3, splitSeed: 1},
		{payload: []byte("sixel"), protocol: 1, prefix: 5, splitSeed: 17, useST: true},
		{payload: []byte("iterm"), protocol: 2, prefix: 7, splitSeed: 99},
	} {
		f.Add(seed.payload, seed.protocol, seed.prefix, seed.splitSeed, seed.useST)
	}

	f.Fuzz(func(t *testing.T, source []byte, protocol, prefixIndex uint8, splitSeed uint32, useST bool) {
		if len(source) > 4096 {
			source = source[:4096]
		}
		marker := []byte("SELECTED_ENVELOPE_MARKER")
		payload := append([]byte(nil), marker...)
		for _, value := range source {
			payload = append(payload, 0x20+value%0x5f)
		}

		prefixes := [][]byte{
			nil,
			[]byte("lead:"),
			[]byte("\x1bXknown-SOS:"),
			[]byte("\x1b^known-PM:"),
			[]byte("\x1b_\x18"),
			[]byte("\x1bP0\x18"),
			[]byte("\x1b]2;known-title\x07"),
			[]byte("\x1b[31m"),
			[]byte("\x1b[31\x1bXCSI-recovery:"),
			[]byte("\x1b(\x1bXcharset-recovery:"),
		}
		suffixes := [][]byte{
			[]byte(":tail"),
			[]byte("\x1b[0m:tail"),
			[]byte("\x1bP1qPUBLIC_DCS\x1b\\:tail"),
			[]byte("\x1b]2;after\x07:tail"),
		}
		prefix := prefixes[int(prefixIndex)%len(prefixes)]
		suffix := suffixes[int(splitSeed>>24)%len(suffixes)]

		var frame, sinkPayload []byte
		var kind ControlStringKind
		switch protocol % 3 {
		case 0:
			frame = append([]byte("\x1b_G"), payload...)
			frame = append(frame, 0x1b, '\\')
			sinkPayload = append([]byte{'G'}, payload...)
			kind = ControlStringAPC
		case 1:
			preambles := []string{"q", "0q", "0;0q", "0;0;0q"}
			preamble := preambles[int(protocol>>2)%len(preambles)]
			frame = append([]byte("\x1bP"+preamble), payload...)
			frame = append(frame, 0x1b, '\\')
			sinkPayload = payload
			kind = ControlStringDCS
		default:
			frame = append([]byte("\x1b]1337;"), payload...)
			if useST {
				frame = append(frame, 0x1b, '\\')
			} else {
				frame = append(frame, 0x07)
			}
			sinkPayload = payload
			kind = ControlStringOSC1337
		}

		input := append(append(append([]byte(nil), prefix...), frame...), suffix...)
		wantPublic := append(append([]byte(nil), prefix...), suffix...)
		mask := publicOutputMask{kitty: true, sixel: true, iterm: true}
		splits := selectedEnvelopeFragmentation(len(input), splitSeed)
		got := parsePublicOutput(t, input, splits, mask)
		if !bytes.Equal(got.output, wantPublic) {
			t.Fatalf("protocol=%d output=%q want=%q", protocol%3, got.output, wantPublic)
		}
		if bytes.Contains(got.output, marker) {
			t.Fatalf("protocol=%d selected marker leaked: %q", protocol%3, got.output)
		}

		var selectedPayload []byte
		selectedFinal := 0
		for _, event := range got.events {
			if event.Kind != kind || event.Cancelled || event.Overflow {
				continue
			}
			selectedPayload = append(selectedPayload, event.Chunk...)
			if event.Final {
				selectedFinal++
			}
		}
		if selectedFinal != 1 || !bytes.Equal(selectedPayload, sinkPayload) {
			t.Fatalf("protocol=%d selected final=%d payload=%q want=%q events=%#v", protocol%3, selectedFinal, selectedPayload, sinkPayload, got.events)
		}
		if got.maxHold > maxPublicOutputHold {
			t.Fatalf("projection hold=%d", got.maxHold)
		}

		unsplit := parsePublicOutput(t, input, nil, mask)
		if !bytes.Equal(got.output, unsplit.output) || !reflect.DeepEqual(got.events, unsplit.events) || got.text != unsplit.text {
			t.Fatalf("protocol=%d fragmented projection differs", protocol%3)
		}
	})
}

func BenchmarkPublicOutputProjectionText(b *testing.B) {
	payload := []byte("plain text without control introducers\r\n")
	for _, test := range []struct {
		name string
		mask publicOutputMask
	}{
		{name: "all-disabled"},
		{name: "all-enabled", mask: publicOutputMask{kitty: true, sixel: true, iterm: true}},
	} {
		b.Run(test.name, func(b *testing.B) {
			term := core.NewTerminal(120, 40)
			var parser Parser
			parser.SetPublicOutputRedaction(test.mask.kitty, test.mask.sixel, test.mask.iterm)
			phase13PrewarmScrollback(term)
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if out := parser.AdvancePublic(term, payload); len(out) != len(payload) {
					b.Fatalf("output len=%d", len(out))
				}
			}
		})
	}
}

func parsePublicOutput(t testing.TB, input []byte, splits []int, mask publicOutputMask) publicParseResult {
	t.Helper()
	term := core.NewTerminal(160, 4)
	var parser Parser
	var events []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&events))
	parser.SetPublicOutputRedaction(mask.kitty, mask.sixel, mask.iterm)
	result := publicParseResult{}
	start := 0
	for _, split := range splits {
		if split < start || split > len(input) {
			t.Fatalf("invalid split %d after %d for %d bytes", split, start, len(input))
		}
		result.output = append(result.output, parser.AdvancePublic(term, input[start:split])...)
		if parser.public.holdLen > result.maxHold {
			result.maxHold = parser.public.holdLen
		}
		start = split
	}
	result.output = append(result.output, parser.AdvancePublic(term, input[start:])...)
	if parser.public.holdLen > result.maxHold {
		result.maxHold = parser.public.holdLen
	}
	result.output = append(result.output, parser.EndOfInputPublic()...)
	result.events = events
	result.text = term.PlainText()
	return result
}

func parseLegacyOracle(t testing.TB, input []byte, splits []int) ([]capturedControlEvent, string) {
	t.Helper()
	term := core.NewTerminal(160, 4)
	var parser Parser
	var events []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&events))
	start := 0
	for _, split := range splits {
		if split < start || split > len(input) {
			t.Fatalf("invalid split %d after %d for %d bytes", split, start, len(input))
		}
		parser.Advance(term, input[start:split])
		start = split
	}
	parser.Advance(term, input[start:])
	parser.EndOfInput()
	return events, term.PlainText()
}

func selectedEnvelopeFragmentation(length int, seed uint32) []int {
	if length <= 1 {
		return nil
	}
	state := seed | 1
	position := 0
	splits := make([]int, 0, length/8)
	for position < length {
		state = state*1664525 + 1013904223
		position += 1 + int(state%23)
		if position < length {
			splits = append(splits, position)
		}
	}
	return splits
}

func everyByteSplit(length int) []int {
	if length <= 1 {
		return nil
	}
	splits := make([]int, 0, length-1)
	for split := 1; split < length; split++ {
		splits = append(splits, split)
	}
	return splits
}
