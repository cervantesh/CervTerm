package vt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestSixelDCSAcceptsExactPreamblesAcrossEverySplit(t *testing.T) {
	for _, preamble := range []string{"q", "0q", "0;0q", "0;0;0q"} {
		t.Run(preamble, func(t *testing.T) {
			payload := []byte("\"1;1;4;2#0;2;100;0;0~-$\a\x1bx")
			input := append([]byte("\x1bP"+preamble), payload...)
			input = append(input, []byte("\x1b\\OK")...)
			wantEvents, wantText := parseControlInput(t, input, nil)
			if got := joinControlChunks(wantEvents); !bytes.Equal(got, payload) {
				t.Fatalf("payload=%q want=%q events=%#v", got, payload, wantEvents)
			}
			if len(wantEvents) != 1 || wantEvents[0].Kind != ControlStringDCS || !wantEvents[0].Final || wantEvents[0].Cancelled || wantEvents[0].Overflow || !strings.HasPrefix(wantText, "OK") {
				t.Fatalf("events=%#v text=%q", wantEvents, wantText)
			}
			for split := 0; split <= len(input); split++ {
				gotEvents, gotText := parseControlInput(t, input, []int{split})
				if !reflect.DeepEqual(gotEvents, wantEvents) || gotText != wantText {
					t.Fatalf("split=%d events=%#v text=%q", split, gotEvents, gotText)
				}
			}
		})
	}
}

func TestSixelDCSRejectsEveryNonSelectedPreambleAtomically(t *testing.T) {
	for _, preamble := range []string{"", "1q", ";q", "0;1q", "0;0;1q", "0;0;0;0q", "?q", "0 q", "0;0p"} {
		t.Run(strings.ReplaceAll(preamble, ";", "_"), func(t *testing.T) {
			input := []byte("\x1bP" + preamble + "hidden\x1b\\OK")
			events, text := parseControlInput(t, input, nil)
			if len(events) != 1 || events[0].Kind != ControlStringDCS || !events[0].Final || !events[0].Cancelled || events[0].Overflow || len(events[0].Chunk) != 0 {
				t.Fatalf("unselected preamble events: %#v", events)
			}
			if !strings.HasPrefix(text, "OK") || strings.Contains(text, "hidden") {
				t.Fatalf("discard recovery text=%q", text)
			}
			for split := 0; split <= len(input); split++ {
				gotEvents, gotText := parseControlInput(t, input, []int{split})
				if !reflect.DeepEqual(gotEvents, events) || gotText != text {
					t.Fatalf("split=%d events=%#v text=%q", split, gotEvents, gotText)
				}
			}
		})
	}
	// C1 DCS is not a selected control-string introducer.
	events, _ := parseControlInput(t, append([]byte{0x90}, []byte("qhidden\x1b\\")...), nil)
	if len(events) != 0 {
		t.Fatalf("C1 DCS emitted events: %#v", events)
	}
}

func TestSixelDCSWholeFrameAndChunkBounds(t *testing.T) {
	for _, preamble := range []string{"q", "0q", "0;0q", "0;0;0q"} {
		t.Run(preamble, func(t *testing.T) {
			payload := bytes.Repeat([]byte{'x'}, maxControlStringLen-len(preamble))
			input := append([]byte("\x1bP"+preamble), payload...)
			input = append(input, 0x1b, '\\')
			events, _ := parseControlInput(t, input, nil)
			if !bytes.Equal(joinControlChunks(events), payload) || terminalControlEvents(events) != 1 || events[len(events)-1].Overflow {
				t.Fatalf("exact bound events=%d bytes=%d", len(events), len(joinControlChunks(events)))
			}
			assertControlEventBounds(t, events)

			over := append([]byte("\x1bP"+preamble), bytes.Repeat([]byte{'x'}, maxControlStringLen-len(preamble)+1)...)
			over = append(over, []byte("hidden\x1b\\OK")...)
			events, text := parseControlInput(t, over, nil)
			if terminalControlEvents(events) != 1 || !events[len(events)-1].Cancelled || !events[len(events)-1].Overflow || !strings.HasPrefix(text, "OK") {
				t.Fatalf("overflow events=%#v text=%q", events, text)
			}
			assertControlEventBounds(t, events)
		})
	}
}

func TestSixelDCSPreambleCancellationTerminatesExactlyOnce(t *testing.T) {
	for name, suffix := range map[string][]byte{"CAN": {0x18}, "SUB": {0x1a}, "ST": {0x1b, '\\'}} {
		t.Run(name, func(t *testing.T) {
			events, text := parseControlInput(t, append([]byte("\x1bP0"), append(suffix, 'Z')...), nil)
			if len(events) != 1 || events[0].Kind != ControlStringDCS || !events[0].Final || !events[0].Cancelled || events[0].Overflow || !strings.HasPrefix(text, "Z") {
				t.Fatalf("events=%#v text=%q", events, text)
			}
		})
	}
}

func TestSixelDCSCancellationResetEOFAndNilSink(t *testing.T) {
	for name, suffix := range map[string][]byte{"CAN": {0x18}, "SUB": {0x1a}, "escape CAN": {0x1b, 0x18}} {
		t.Run(name, func(t *testing.T) {
			events, text := parseControlInput(t, append([]byte("\x1bPqpartial"), append(suffix, 'Z')...), nil)
			if len(events) != 1 || !events[0].Final || !events[0].Cancelled || events[0].Overflow || !strings.HasPrefix(text, "Z") {
				t.Fatalf("events=%#v text=%q", events, text)
			}
		})
	}
	for _, finish := range []struct {
		name string
		call func(*Parser)
	}{{"reset", (*Parser).Reset}, {"EOF", (*Parser).EndOfInput}} {
		t.Run(finish.name, func(t *testing.T) {
			term := core.NewTerminal(20, 1)
			var parser Parser
			var events []capturedControlEvent
			parser.SetControlStringSink(captureControlEvents(&events))
			parser.Advance(term, []byte("\x1bPqpartial"))
			finish.call(&parser)
			finish.call(&parser)
			if len(events) != 1 || !events[0].Cancelled || !events[0].Final {
				t.Fatalf("events=%#v", events)
			}
		})
	}
	term := core.NewTerminal(20, 1)
	var parser Parser
	parser.Advance(term, []byte("\x1bPqsecret\x1b\\OK"))
	if parser.control != nil || !strings.HasPrefix(term.PlainText(), "OK") || strings.Contains(term.PlainText(), "secret") {
		t.Fatalf("nil sink allocated or leaked: control=%p text=%q", parser.control, term.PlainText())
	}
}

func FuzzSixelDCSTransportSelection(f *testing.F) {
	f.Add([]byte("sixel"), uint32(0), byte(0))
	f.Add([]byte{0x1b, 0x18, 0xff}, uint32(7), byte(3))
	f.Fuzz(func(t *testing.T, source []byte, split uint32, selector byte) {
		if len(source) > maxControlStringLen+1 {
			source = source[:maxControlStringLen+1]
		}
		payload := make([]byte, len(source))
		for index, value := range source {
			payload[index] = 0x20 + value%0x5f
		}
		source = payload
		preambles := []string{"q", "0q", "0;0q", "0;0;0q", "1q", "0;1q"}
		preamble := preambles[int(selector)%len(preambles)]
		input := append([]byte("\x1bP"+preamble), source...)
		input = append(input, 0x1b, '\\', 'Z')
		wantEvents, wantText := parseControlInput(t, input, nil)
		point := int(uint64(split) % uint64(len(input)+1))
		gotEvents, gotText := parseControlInput(t, input, []int{point})
		if !reflect.DeepEqual(gotEvents, wantEvents) || gotText != wantText {
			t.Fatalf("split=%d differs", point)
		}
		assertControlEventBounds(t, gotEvents)
		if terminalControlEventsForKind(gotEvents, ControlStringDCS) != 1 {
			t.Fatalf("multiple DCS terminal events: %#v", gotEvents)
		}
	})
}

func joinControlChunks(events []capturedControlEvent) []byte {
	var result []byte
	for _, event := range events {
		result = append(result, event.Chunk...)
	}
	return result
}

func terminalControlEvents(events []capturedControlEvent) int {
	count := 0
	for _, event := range events {
		if event.Final {
			count++
		}
	}
	return count
}

func terminalControlEventsForKind(events []capturedControlEvent, kind ControlStringKind) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind && event.Final {
			count++
		}
	}
	return count
}
