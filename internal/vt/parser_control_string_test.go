package vt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/core"
)

type capturedControlEvent struct {
	Kind      ControlStringKind
	Chunk     []byte
	Final     bool
	Cancelled bool
	Overflow  bool
}

func TestControlStringsFrameAPCAndDCS(t *testing.T) {
	tests := []struct {
		name       string
		introducer byte
		kind       ControlStringKind
	}{
		{name: "APC", introducer: '_', kind: ControlStringAPC},
		{name: "DCS", introducer: 'P', kind: ControlStringDCS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := append([]byte{0x1b, test.introducer}, []byte("alpha\a\x1bxomega\x1b\\OK")...)
			events, text := parseControlInput(t, input, nil)
			if len(events) != 1 {
				t.Fatalf("events = %#v, want one", events)
			}
			wantPayload := []byte("alpha\a\x1bxomega")
			if event := events[0]; event.Kind != test.kind || !event.Final || event.Cancelled || event.Overflow || !bytes.Equal(event.Chunk, wantPayload) {
				t.Fatalf("event = %#v, want final %v payload %q", event, test.kind, wantPayload)
			}
			if !strings.HasPrefix(text, "OK") || strings.Contains(text, "alpha") {
				t.Fatalf("plain text leaked control payload: %q", text)
			}
		})
	}
}

func TestControlStringEverySplitMatchesUnsplit(t *testing.T) {
	input := []byte("\x1b_apc\x1bxmid\x1b\x1b\\tail")
	wantEvents, wantText := parseControlInput(t, input, nil)
	for split := 0; split <= len(input); split++ {
		events, text := parseControlInput(t, input, []int{split})
		if !reflect.DeepEqual(events, wantEvents) || text != wantText {
			t.Fatalf("split %d mismatch\nevents: %#v\nwant: %#v\ntext: %q want %q", split, events, wantEvents, text, wantText)
		}
	}
	oneByte := make([]int, 0, len(input)-1)
	for i := 1; i < len(input); i++ {
		oneByte = append(oneByte, i)
	}
	events, text := parseControlInput(t, input, oneByte)
	if !reflect.DeepEqual(events, wantEvents) || text != wantText {
		t.Fatalf("one-byte chunks mismatch: %#v, %q", events, text)
	}
}

func TestRepeatedESCStillIntroducesControlString(t *testing.T) {
	for _, introducer := range []byte{'_', 'P'} {
		input := []byte{0x1b, 0x1b, introducer}
		input = append(input, []byte("secret\x1b\\OK")...)
		events, text := parseControlInput(t, input, nil)
		if len(events) != 1 || string(events[0].Chunk) != "secret" || !events[0].Final {
			t.Fatalf("introducer %q events = %#v", introducer, events)
		}
		if !strings.HasPrefix(text, "OK") || strings.Contains(text, "secret") {
			t.Fatalf("introducer %q leaked text %q", introducer, text)
		}
	}
}

func TestExactCapOverlappingSTOverflowRecoversAcrossSplits(t *testing.T) {
	input := append([]byte("\x1bP"), bytes.Repeat([]byte{'x'}, maxControlStringLen)...)
	input = append(input, 0x1b, 0x1b, '\\', 'O', 'K')
	wantEvents, wantText := parseControlInput(t, input, nil)
	if len(wantEvents) == 0 || !wantEvents[len(wantEvents)-1].Overflow || !strings.HasPrefix(wantText, "OK") {
		t.Fatalf("events=%#v text=%q", wantEvents, wantText)
	}
	for _, split := range []int{1, 2, maxControlStringLen + 1, maxControlStringLen + 2, maxControlStringLen + 3, maxControlStringLen + 4, maxControlStringLen + 5} {
		events, text := parseControlInput(t, input, []int{split})
		if !reflect.DeepEqual(events, wantEvents) || text != wantText {
			t.Fatalf("split %d mismatch: events=%#v text=%q", split, events, text)
		}
	}
}

func TestControlStringChunkAndFrameBoundaries(t *testing.T) {
	for _, size := range []int{0, maxControlStringChunk - 1, maxControlStringChunk, maxControlStringChunk + 1, maxControlStringLen} {
		t.Run(stringSize(size), func(t *testing.T) {
			payload := bytes.Repeat([]byte{'x'}, size)
			input := append([]byte("\x1b_"), payload...)
			input = append(input, 0x1b, '\\')
			events, text := parseControlInput(t, input, nil)
			var got []byte
			terminal := 0
			for _, event := range events {
				if len(event.Chunk) > maxControlStringChunk {
					t.Fatalf("chunk len = %d", len(event.Chunk))
				}
				got = append(got, event.Chunk...)
				if event.Final {
					terminal++
				}
			}
			if !bytes.Equal(got, payload) || terminal != 1 || !events[len(events)-1].Final {
				t.Fatalf("size %d: events=%d bytes=%d terminal=%d", size, len(events), len(got), terminal)
			}
			if strings.TrimSpace(text) != "" {
				t.Fatalf("payload leaked: %q", text)
			}
		})
	}
}

func TestControlStringOverflowCancelsOnceAndDiscardsThroughST(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, maxControlStringLen+1)
	input := append([]byte("\x1bP"), payload...)
	input = append(input, []byte("hidden\x1b\\OK")...)
	events, text := parseControlInput(t, input, nil)
	terminal := 0
	for _, event := range events {
		if event.Final {
			terminal++
			if !event.Cancelled || !event.Overflow || len(event.Chunk) != 0 {
				t.Fatalf("overflow event = %#v", event)
			}
		}
	}
	if terminal != 1 {
		t.Fatalf("terminal events = %d, events=%#v", terminal, events)
	}
	if !strings.HasPrefix(text, "OK") || strings.Contains(text, "hidden") {
		t.Fatalf("discard recovery text = %q", text)
	}
}

func TestControlStringOverflowDiscardTerminatorsAndResetDoNotDuplicate(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, maxControlStringLen+1)
	for name, terminator := range map[string][]byte{
		"CAN":            {0x18},
		"SUB":            {0x1a},
		"overlapping ST": {0x1b, 0x1b, '\\'},
	} {
		t.Run(name, func(t *testing.T) {
			input := append([]byte("\x1b_"), payload...)
			input = append(input, terminator...)
			input = append(input, 'Z')
			events, text := parseControlInput(t, input, nil)
			terminal := 0
			for _, event := range events {
				if event.Final {
					terminal++
				}
			}
			if terminal != 1 || !strings.HasPrefix(text, "Z") {
				t.Fatalf("events=%#v text=%q", events, text)
			}
		})
	}

	term := core.NewTerminal(10, 1)
	var parser Parser
	var events []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&events))
	input := append([]byte("\x1bP"), payload...)
	parser.Advance(term, input)
	parser.Reset()
	parser.EndOfInput()
	parser.Advance(term, []byte("Z"))
	if len(events) == 0 || !events[len(events)-1].Overflow {
		t.Fatalf("overflow events = %#v", events)
	}
	terminal := 0
	for _, event := range events {
		if event.Final {
			terminal++
		}
	}
	if terminal != 1 || !strings.HasPrefix(term.PlainText(), "Z") {
		t.Fatalf("terminal=%d text=%q", terminal, term.PlainText())
	}
}

func TestControlStringCANAndSUBCancelFromPayloadAndEscape(t *testing.T) {
	for _, input := range [][]byte{
		[]byte("\x1b_partial\x18OK"),
		[]byte("\x1bPpartial\x1b\x1aOK"),
	} {
		events, text := parseControlInput(t, input, nil)
		if len(events) != 1 || !events[0].Final || !events[0].Cancelled || events[0].Overflow {
			t.Fatalf("cancel events = %#v", events)
		}
		if !strings.HasPrefix(text, "OK") || strings.Contains(text, "partial") {
			t.Fatalf("cancel recovery text = %q", text)
		}
	}
}

func TestControlStringNilSinkStillDiscards(t *testing.T) {
	term := core.NewTerminal(40, 2)
	var parser Parser
	parser.Advance(term, []byte("\x1b_secret\x1b\\OK"))
	if text := term.PlainText(); !strings.HasPrefix(text, "OK") || strings.Contains(text, "secret") {
		t.Fatalf("nil sink leaked payload: %q", text)
	}
}

func TestControlStringResetAndEndOfInputCancelOnceAndReuse(t *testing.T) {
	for _, finish := range []struct {
		name string
		call func(*Parser)
	}{
		{name: "Reset", call: (*Parser).Reset},
		{name: "EndOfInput", call: (*Parser).EndOfInput},
	} {
		t.Run(finish.name, func(t *testing.T) {
			term := core.NewTerminal(40, 2)
			var parser Parser
			var events []capturedControlEvent
			parser.SetControlStringSink(captureControlEvents(&events))
			parser.Advance(term, []byte("\x1b_open"))
			finish.call(&parser)
			finish.call(&parser)
			parser.Advance(term, []byte("OK"))
			if len(events) != 1 || !events[0].Final || !events[0].Cancelled || events[0].Overflow {
				t.Fatalf("events = %#v", events)
			}
			if text := term.PlainText(); !strings.HasPrefix(text, "OK") || strings.Contains(text, "open") {
				t.Fatalf("reuse text = %q", text)
			}
		})
	}
}

func TestControlStringSinkIsCapturedAtFrameStart(t *testing.T) {
	term := core.NewTerminal(20, 1)
	var parser Parser
	var first, second []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&first))
	parser.Advance(term, []byte("\x1b_first"))
	parser.SetControlStringSink(captureControlEvents(&second))
	parser.Advance(term, []byte("\x1b\\"))
	parser.Advance(term, []byte("\x1b_second\x1b\\"))
	if len(first) != 1 || string(first[0].Chunk) != "first" || len(second) != 1 || string(second[0].Chunk) != "second" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func TestMalformedPendingUTF8CannotHideControlIntroducer(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var parser Parser
	var events []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&events))
	parser.Advance(term, []byte{0xc3})
	parser.Advance(term, []byte("\x1b_safe\x1b\\OK"))
	if len(events) != 1 || string(events[0].Chunk) != "safe" {
		t.Fatalf("events = %#v", events)
	}
	if text := term.PlainText(); !strings.HasPrefix(text, "OK") || strings.Contains(text, "safe") {
		t.Fatalf("text = %q", text)
	}
}

func TestMalformedPendingUTF8ReprocessesFollowingASCII(t *testing.T) {
	term := core.NewTerminal(10, 1)
	var parser Parser
	parser.Advance(term, []byte{0xc3})
	parser.Advance(term, []byte("A"))
	if text := term.PlainText(); !strings.HasPrefix(text, "A") {
		t.Fatalf("following ASCII was swallowed: %q", text)
	}
}

func TestOversizedCSIParameterSaturates(t *testing.T) {
	term := core.NewTerminal(12, 5)
	var parser Parser
	parser.Advance(term, []byte("\x1b[0000000000000000000000000000000000000000800000000000000000"))
	if parser.cur != maxCSIParam {
		t.Fatalf("oversized CSI parameter = %d, want saturated %d", parser.cur, maxCSIParam)
	}
	parser.Advance(term, []byte("I"))
	if term.CursorCol() < 0 || term.CursorCol() >= term.Cols() {
		t.Fatalf("cursor col = %d after saturated CHT", term.CursorCol())
	}
	term.SetCursor(0, term.Cols()-1)
	parser.Advance(term, []byte("\x1b[999999999999999999999999Z"))
	if term.CursorCol() != 0 {
		t.Fatalf("saturated CBT cursor col = %d, want 0", term.CursorCol())
	}

	wide := core.NewTerminal(12001, 1)
	parser = Parser{}
	parser.Advance(wide, []byte("\x1b[12000G"))
	if wide.CursorCol() != 11999 {
		t.Fatalf("valid five-digit column = %d, want 11999", wide.CursorCol())
	}
}

func TestCursorForwardTabsAtBoundaryClearsPendingWrap(t *testing.T) {
	term := core.NewTerminal(3, 2)
	var parser Parser
	parser.Advance(term, []byte("abc\x1b[65535IX"))
	if text := term.PlainText(); !strings.HasPrefix(text, "abX") {
		t.Fatalf("CHT at right boundary preserved pending wrap: %q", text)
	}
}

func TestControlStringResetDropsIncompleteOSCWithoutDispatch(t *testing.T) {
	term := core.NewTerminal(20, 2)
	var parser Parser
	parser.Advance(term, []byte("\x1b]2;incomplete"))
	parser.Reset()
	parser.Advance(term, []byte("OK"))
	if term.Title() != "" || !strings.HasPrefix(term.PlainText(), "OK") {
		t.Fatalf("title=%q text=%q", term.Title(), term.PlainText())
	}
}

func FuzzControlStringFraming(f *testing.F) {
	f.Add([]byte("kitty"), uint32(0), byte('_'))
	f.Add([]byte{0x1b, 0x1b, '\\', 0x18, 0x1a, 0xff}, uint32(7), byte('P'))
	f.Fuzz(func(t *testing.T, source []byte, split uint32, introducer byte) {
		if len(source) > maxControlStringLen+1 {
			source = source[:maxControlStringLen+1]
		}
		if introducer&1 == 0 {
			introducer = '_'
		} else {
			introducer = 'P'
		}

		// Raw binary input exercises ESC/ST ambiguity, cancellation, malformed UTF-8,
		// and discard recovery. Arbitrary splitting must not change the event stream
		// or terminal projection.
		raw := append([]byte{0x1b, introducer}, source...)
		raw = append(raw, 0x1b, '\\', 'Z')
		wantEvents, wantText := parseControlInput(t, raw, nil)
		point := int(uint64(split) % uint64(len(raw)+1))
		gotEvents, gotText := parseControlInput(t, raw, []int{point})
		if !reflect.DeepEqual(gotEvents, wantEvents) || gotText != wantText {
			t.Fatalf("raw split=%d framing differs", point)
		}
		assertControlEventBounds(t, gotEvents)

		// A payload without in-band terminators must remain entirely invisible and
		// produce exactly one terminal outcome.
		payload := make([]byte, len(source))
		for i, value := range source {
			payload[i] = 0x20 + value%0x5f
		}
		safe := append([]byte{0x1b, introducer}, payload...)
		safe = append(safe, 0x1b, '\\', 'Z')
		wantEvents, wantText = parseControlInput(t, safe, nil)
		point = int(uint64(split) % uint64(len(safe)+1))
		gotEvents, gotText = parseControlInput(t, safe, []int{point})
		if !reflect.DeepEqual(gotEvents, wantEvents) || gotText != wantText {
			t.Fatalf("safe split=%d framing differs", point)
		}
		assertControlEventBounds(t, gotEvents)
		terminal := 0
		for _, event := range gotEvents {
			if event.Final {
				terminal++
			}
		}
		if terminal != 1 || !strings.HasPrefix(gotText, "Z") {
			t.Fatalf("terminal=%d text=%q", terminal, gotText)
		}
	})
}

func assertControlEventBounds(t *testing.T, events []capturedControlEvent) {
	t.Helper()
	for _, event := range events {
		if len(event.Chunk) > maxControlStringChunk {
			t.Fatalf("chunk len = %d", len(event.Chunk))
		}
	}
}

func parseControlInput(t *testing.T, input []byte, splits []int) ([]capturedControlEvent, string) {
	t.Helper()
	term := core.NewTerminal(80, 2)
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
	return events, term.PlainText()
}

func captureControlEvents(events *[]capturedControlEvent) ControlStringSink {
	return func(event ControlStringEvent) {
		*events = append(*events, capturedControlEvent{
			Kind:      event.Kind,
			Chunk:     append([]byte(nil), event.Chunk...),
			Final:     event.Final,
			Cancelled: event.Cancelled,
			Overflow:  event.Overflow,
		})
	}
}

func stringSize(size int) string {
	const digits = "0123456789"
	if size == 0 {
		return "0"
	}
	var reversed [20]byte
	index := len(reversed)
	for size > 0 {
		index--
		reversed[index] = digits[size%10]
		size /= 10
	}
	return string(reversed[index:])
}
