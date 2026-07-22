package itermimage

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func FuzzITermScannerFragmentation(f *testing.F) {
	f.Add([]byte(validInlineFile), uint32(0))
	f.Add([]byte("File=height=256;inline=1;size=1;preserveAspectRatio=1:AA=="), uint32(17))
	f.Add([]byte("File=inline=1;size=1:AB=="), uint32(23))
	f.Add([]byte("File=inline=0;size=1:AAAA"), uint32(8))
	f.Fuzz(func(t *testing.T, source []byte, split uint32) {
		if len(source) > 64*1024 {
			source = source[:64*1024]
		}

		var whole scanner
		wholePayload, wholeFailure := whole.feed(source)
		if wholeFailure == FailureNone {
			wholeFailure = whole.finish()
		}

		point := int(uint64(split) % uint64(len(source)+1))
		var fragmented scanner
		var fragmentedPayload []byte
		firstPayload, fragmentedFailure := fragmented.feed(source[:point])
		fragmentedPayload = append(fragmentedPayload, firstPayload...)
		if fragmentedFailure == FailureNone {
			secondPayload, failure := fragmented.feed(source[point:])
			fragmentedPayload = append(fragmentedPayload, secondPayload...)
			fragmentedFailure = failure
		}
		if fragmentedFailure == FailureNone {
			fragmentedFailure = fragmented.finish()
		}
		if wholeFailure != fragmentedFailure || !reflect.DeepEqual(whole, fragmented) {
			t.Fatalf("split=%d whole=%v fragmented=%v", point, wholeFailure, fragmentedFailure)
		}
		if wholeFailure == FailureNone && !bytes.Equal(wholePayload, fragmentedPayload) {
			t.Fatalf("split=%d payload whole=%q fragmented=%q", point, wholePayload, fragmentedPayload)
		}

		if len(source) <= 4096 {
			var oneByte scanner
			var onePayload []byte
			oneFailure := FailureNone
			for _, value := range source {
				payload, failure := oneByte.feed([]byte{value})
				onePayload = append(onePayload, payload...)
				if failure != FailureNone {
					oneFailure = failure
					break
				}
			}
			if oneFailure == FailureNone {
				oneFailure = oneByte.finish()
			}
			if oneFailure != wholeFailure || !reflect.DeepEqual(oneByte, whole) {
				t.Fatalf("one-byte differs whole=%v one=%v", wholeFailure, oneFailure)
			}
			if wholeFailure == FailureNone && !bytes.Equal(onePayload, wholePayload) {
				t.Fatalf("one-byte payload=%q whole=%q", onePayload, wholePayload)
			}
		}
	})
}

func FuzzITermStrictBase64(f *testing.F) {
	f.Add([]byte("AAAA"), uint32(0))
	f.Add([]byte("AA=="), uint32(2))
	f.Add([]byte("AB=="), uint32(1))
	f.Add([]byte("YWJjZA=="), uint32(7))
	f.Fuzz(func(t *testing.T, encoded []byte, split uint32) {
		if len(encoded) > 4096 {
			encoded = encoded[:4096]
		}
		point := int(uint64(split) % uint64(len(encoded)+1))

		var scan base64Scanner
		failure := FailureNone
		for _, value := range encoded[:point] {
			if failure = scan.feedByte(value); failure != FailureNone {
				break
			}
		}
		if failure == FailureNone {
			for _, value := range encoded[point:] {
				if failure = scan.feedByte(value); failure != FailureNone {
					break
				}
			}
		}
		if failure == FailureNone {
			failure = scan.finish()
		}

		_, strictErr := base64.StdEncoding.Strict().DecodeString(string(encoded))
		wantValid := len(encoded) != 0 && !bytes.ContainsAny(encoded, " \t\r\n\v\f") && strictErr == nil
		if gotValid := failure == FailureNone; gotValid != wantValid {
			t.Fatalf("encoded=%q failure=%v strictErr=%v", encoded, failure, strictErr)
		}
	})
}

func FuzzITermAdapterLifecycle(f *testing.F) {
	f.Add([]byte(validInlineFile), uint32(1), false, false)
	f.Add([]byte("File=inline=1;size=1:AB=="), uint32(7), false, false)
	f.Add([]byte("File=inline=1;size=1:AAAA"), uint32(20), true, false)
	f.Fuzz(func(t *testing.T, source []byte, split uint32, cancel, overflow bool) {
		maximum := int(2*termimage.HardControlChunkBytes + 1)
		if len(source) > maximum {
			source = source[:maximum]
		}
		process := termimage.NewProcessBudget()
		store := termimage.NewStore(process, termimage.DefaultLimits())
		adapter := NewAdapter(store)
		point := int(uint64(split) % uint64(len(source)+1))
		first := adapter.Advance(time.Now(), OSCEvent{Data: source[:point]})
		if first.Command != nil {
			first.Command.Close()
			t.Fatal("command emitted before final event")
		}
		out := adapter.Advance(time.Now(), OSCEvent{
			Data: source[point:], Final: true, Cancelled: cancel, Overflow: overflow,
		})
		if out.Command != nil {
			if out.Failure != FailureNone || out.Command.Image < termimage.MinInternalImageID || out.Command.Placement < termimage.MinInternalPlacementID {
				t.Fatalf("command=%#v failure=%v", out.Command, out.Failure)
			}
			payload, _, _, err := out.Command.Transfer.SealedEncodedCopy(store)
			if err != nil || len(payload) == 0 || bytes.ContainsAny(payload, " \t\r\n") {
				t.Fatalf("payload=%q err=%v", payload, err)
			}
			out.Command.Close()
		}
		adapter.Close()
		if store.Usage() != (termimage.Usage{}) || process.Usage() != (termimage.Usage{}) {
			t.Fatalf("usage pane=%#v process=%#v", store.Usage(), process.Usage())
		}
	})
}
