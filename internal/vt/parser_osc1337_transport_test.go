package vt

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"cervterm/internal/core"
)

func TestOSC1337SelectedTransportBELAndSTAcrossEverySplit(t *testing.T) {
	payload := []byte("File=inline=1;size=4:AAAA")
	for name, terminator := range map[string][]byte{"BEL": {0x07}, "ST": {0x1b, '\\'}} {
		t.Run(name, func(t *testing.T) {
			input := append([]byte("\x1b]1337;"), payload...)
			input = append(input, terminator...)
			input = append(input, []byte("OK")...)
			wantEvents, wantText := parseControlInput(t, input, nil)
			if got := joinControlChunks(wantEvents); !bytes.Equal(got, payload) {
				t.Fatalf("payload=%q want=%q events=%#v", got, payload, wantEvents)
			}
			if terminalControlEventsForKind(wantEvents, ControlStringOSC1337) != 1 || wantEvents[len(wantEvents)-1].Cancelled || wantEvents[len(wantEvents)-1].Overflow || !strings.HasPrefix(wantText, "OK") {
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

func TestOSC1337WholeFrameCapOverflowRedactionAndRecovery(t *testing.T) {
	const selector = "1337;"
	exactPayloadLen := maxControlStringLen - len(selector)
	for name, terminator := range map[string][]byte{"BEL": {0x07}, "ST": {0x1b, '\\'}} {
		t.Run(name, func(t *testing.T) {
			exactPayload := bytes.Repeat([]byte{'x'}, exactPayloadLen)
			exact := append([]byte("\x1b]"+selector), exactPayload...)
			exact = append(exact, terminator...)
			events, _ := parseControlInput(t, exact, nil)
			assertControlEventBounds(t, events)
			if got := joinControlChunks(events); !bytes.Equal(got, exactPayload) || terminalControlEventsForKind(events, ControlStringOSC1337) != 1 || events[len(events)-1].Cancelled || events[len(events)-1].Overflow {
				t.Fatalf("exact cap bytes=%d events=%#v", len(got), events)
			}

			overflowPayload := bytes.Repeat([]byte{'y'}, exactPayloadLen+1)
			overflow := append([]byte("A\x1b]"+selector), overflowPayload...)
			overflow = append(overflow, []byte("HIDDEN")...)
			overflow = append(overflow, terminator...)
			overflow = append(overflow, 'Z')
			events, text := parseControlInput(t, overflow, nil)
			assertControlEventBounds(t, events)
			terminal, overflowEvents := 0, 0
			for _, event := range events {
				if event.Final {
					terminal++
					if event.Cancelled && event.Overflow {
						overflowEvents++
					}
				}
			}
			if terminal != 1 || overflowEvents != 1 || !strings.HasPrefix(text, "AZ") || strings.Contains(text, "HIDDEN") {
				t.Fatalf("overflow terminal=%d overflow=%d text=%q events=%#v", terminal, overflowEvents, text, events)
			}

			term := core.NewTerminal(20, 1)
			var parser Parser
			var publicEvents []capturedControlEvent
			parser.SetControlStringSink(captureControlEvents(&publicEvents))
			parser.SetPublicOutputRedaction(false, false, true)
			public := append([]byte(nil), parser.AdvancePublic(term, overflow)...)
			public = append(public, parser.EndOfInputPublic()...)
			if !bytes.Equal(public, []byte("AZ")) || !reflect.DeepEqual(publicEvents, events) {
				t.Fatalf("public=%q events=%#v want events=%#v", public, publicEvents, events)
			}
		})
	}
}

func TestOSC1337ExactSelectionPreservesNonselectedOSC(t *testing.T) {
	for _, input := range []string{
		"\x1b]1337\a",
		"\x1b]13370;File=inline=1:AAAA\a",
		"\x1b]1337:File=inline=1:AAAA\x1b\\",
		"\x1b]01337;File=inline=1:AAAA\a",
	} {
		events, _ := parseControlInput(t, []byte(input), nil)
		if len(events) != 0 {
			t.Fatalf("nonselected %q events=%#v", input, events)
		}
	}
	term := core.NewTerminal(20, 1)
	var parser Parser
	var events []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&events))
	parser.Advance(term, []byte("\x1b]2;ordinary title\a"))
	if term.Title() != "ordinary title" || len(events) != 0 {
		t.Fatalf("title=%q events=%#v", term.Title(), events)
	}
	// C1 OSC remains outside the selected 7-bit transport.
	parser.Advance(term, append([]byte{0x9d}, []byte("1337;File=x\a")...))
	if len(events) != 0 {
		t.Fatalf("C1 OSC events=%#v", events)
	}
}

func TestOSC1337EscapeAmbiguityAndTerminators(t *testing.T) {
	for name, suffix := range map[string][]byte{
		"ESC payload then ST": {'a', 0x1b, 'x', 0x1b, '\\'},
		"overlapping ST":      {'a', 0x1b, 0x1b, '\\'},
		"ESC then BEL":        {'a', 0x1b, 0x07},
	} {
		t.Run(name, func(t *testing.T) {
			input := append([]byte("\x1b]1337;"), suffix...)
			events, _ := parseControlInput(t, input, nil)
			if terminalControlEventsForKind(events, ControlStringOSC1337) != 1 || events[len(events)-1].Cancelled {
				t.Fatalf("events=%#v", events)
			}
			want := suffix[:len(suffix)-2]
			if name == "ESC then BEL" {
				want = suffix[:len(suffix)-1]
			}
			if !bytes.Equal(joinControlChunks(events), want) {
				t.Fatalf("payload=%q want=%q", joinControlChunks(events), want)
			}
		})
	}
}

func TestOSC1337CancellationResetEOFAndNilSink(t *testing.T) {
	for name, terminator := range map[string][]byte{"CAN": {0x18}, "SUB": {0x1a}, "escape CAN": {0x1b, 0x18}} {
		t.Run(name, func(t *testing.T) {
			input := append([]byte("\x1b]1337;partial"), terminator...)
			input = append(input, 'Z')
			events, text := parseControlInput(t, input, nil)
			if len(events) != 1 || events[0].Kind != ControlStringOSC1337 || !events[0].Final || !events[0].Cancelled || events[0].Overflow || !strings.HasPrefix(text, "Z") {
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
			parser.Advance(term, []byte("\x1b]1337;partial"))
			finish.call(&parser)
			finish.call(&parser)
			if len(events) != 1 || !events[0].Final || !events[0].Cancelled {
				t.Fatalf("events=%#v", events)
			}
		})
	}
	for _, terminator := range [][]byte{{0x07}, {0x1b, '\\'}} {
		term := core.NewTerminal(20, 1)
		var parser Parser
		input := append([]byte("\x1b]1337;secret"), terminator...)
		input = append(input, []byte("OK")...)
		parser.Advance(term, input)
		if parser.control != nil || !strings.HasPrefix(term.PlainText(), "OK") || strings.Contains(term.PlainText(), "secret") {
			t.Fatalf("control=%p text=%q", parser.control, term.PlainText())
		}
	}
}

func TestOSC1337SinkIsCapturedAtSelection(t *testing.T) {
	term := core.NewTerminal(20, 1)
	var parser Parser
	var first, second []capturedControlEvent
	parser.SetControlStringSink(captureControlEvents(&first))
	parser.Advance(term, []byte("\x1b]1337;first"))
	parser.SetControlStringSink(captureControlEvents(&second))
	parser.Advance(term, []byte("\a\x1b]1337;second\a"))
	if string(joinControlChunks(first)) != "first" || string(joinControlChunks(second)) != "second" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func FuzzOSC1337SelectedTransport(f *testing.F) {
	f.Add([]byte("File=inline=1;size=4:AAAA"), uint32(0), false)
	f.Add([]byte{0x1b, 0x07, 0xff}, uint32(7), true)
	f.Fuzz(func(t *testing.T, source []byte, split uint32, useST bool) {
		raw := source
		if len(raw) > 1024 {
			raw = raw[:1024]
		}
		rawInput := append([]byte("\x1b]1337;"), raw...)
		if useST {
			rawInput = append(rawInput, 0x1b, '\\')
		} else {
			rawInput = append(rawInput, 0x07)
		}
		rawInput = append(rawInput, 'Z')
		rawEvents, rawText := parseControlInput(t, rawInput, nil)
		rawSplits := make([]int, 0, len(rawInput)-1)
		for point := 1; point < len(rawInput); point++ {
			rawSplits = append(rawSplits, point)
		}
		fragmentedEvents, fragmentedText := parseControlInput(t, rawInput, rawSplits)
		if !reflect.DeepEqual(fragmentedEvents, rawEvents) || fragmentedText != rawText {
			t.Fatal("raw one-byte fragmentation differs")
		}
		assertControlEventBounds(t, fragmentedEvents)

		maxPayload := maxControlStringLen - len("1337;")
		if len(source) > maxPayload+1 {
			source = source[:maxPayload+1]
		}
		payload := make([]byte, len(source))
		for index, value := range source {
			payload[index] = 0x20 + value%0x5f
		}
		input := append([]byte("\x1b]1337;"), payload...)
		if useST {
			input = append(input, 0x1b, '\\')
		} else {
			input = append(input, 0x07)
		}
		input = append(input, 'Z')
		wantEvents, wantText := parseControlInput(t, input, nil)
		point := int(uint64(split) % uint64(len(input)+1))
		gotEvents, gotText := parseControlInput(t, input, []int{point})
		if !reflect.DeepEqual(gotEvents, wantEvents) || gotText != wantText {
			t.Fatalf("split=%d differs", point)
		}
		assertControlEventBounds(t, gotEvents)
		if terminalControlEventsForKind(gotEvents, ControlStringOSC1337) != 1 {
			t.Fatalf("events=%#v", gotEvents)
		}
		terminal := gotEvents[len(gotEvents)-1]
		if len(payload) <= maxPayload {
			if terminal.Cancelled || terminal.Overflow || !bytes.Equal(joinControlChunks(gotEvents), payload) {
				t.Fatalf("bounded events=%#v", gotEvents)
			}
		} else if !terminal.Cancelled || !terminal.Overflow {
			t.Fatalf("overflow events=%#v", gotEvents)
		}
	})
}
