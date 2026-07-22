package sixel

import (
	"reflect"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func FuzzSixelTokenizerFragmentation(f *testing.F) {
	f.Add([]byte(validFrame), uint32(0))
	f.Add([]byte(`#1;1;0;0;0"1;1;1;1~`), uint32(7))
	f.Fuzz(func(t *testing.T, source []byte, split uint32) {
		if len(source) > maxFrameBytes+1 {
			source = source[:maxFrameBytes+1]
		}
		var whole, fragmented scanner
		wholeFailure := whole.feed(source)
		if wholeFailure == FailureNone {
			wholeFailure = whole.finish()
		}
		point := int(uint64(split) % uint64(len(source)+1))
		fragmentedFailure := fragmented.feed(source[:point])
		if fragmentedFailure == FailureNone {
			fragmentedFailure = fragmented.feed(source[point:])
		}
		if fragmentedFailure == FailureNone {
			fragmentedFailure = fragmented.finish()
		}
		if wholeFailure != fragmentedFailure || !reflect.DeepEqual(whole, fragmented) {
			t.Fatalf("split=%d differs", point)
		}
		if len(source) <= 4096 {
			var oneByte scanner
			oneFailure := FailureNone
			for _, value := range source {
				if oneFailure = oneByte.feed([]byte{value}); oneFailure != FailureNone {
					break
				}
			}
			if oneFailure == FailureNone {
				oneFailure = oneByte.finish()
			}
			if oneFailure != wholeFailure || !reflect.DeepEqual(oneByte, whole) {
				t.Fatal("one-byte fragmentation differs")
			}
		}
	})
}

func FuzzSixelAdapterLifecycle(f *testing.F) {
	f.Add([]byte(validFrame), uint32(1), false, false)
	f.Add([]byte{0xff, '!', '0', '~'}, uint32(2), true, false)
	f.Fuzz(func(t *testing.T, source []byte, split uint32, cancel, overflow bool) {
		if len(source) > maxFrameBytes+1 {
			source = source[:maxFrameBytes+1]
		}
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapter := NewAdapter(store)
		point := int(uint64(split) % uint64(len(source)+1))
		first := adapter.Advance(time.Now(), DCSEvent{Data: source[:point]})
		if first.Command != nil {
			first.Command.Close()
		}
		out := adapter.Advance(time.Now(), DCSEvent{Data: source[point:], Final: true, Cancelled: cancel, Overflow: overflow})
		if out.Command != nil {
			if out.Failure != FailureNone || out.Command.Image < termimage.MinInternalImageID || out.Command.Placement < termimage.MinInternalPlacementID {
				t.Fatalf("command=%#v failure=%v", out.Command, out.Failure)
			}
			out.Command.Close()
		}
		adapter.Close()
		if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
			t.Fatalf("usage=%#v", store.Usage())
		}
	})
}
