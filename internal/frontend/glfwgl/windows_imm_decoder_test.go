//go:build glfw

package glfwgl

import (
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
	"time"
	"unicode/utf16"

	"cervterm/internal/ime"
)

type fakeIMMAPI struct {
	log            []string
	data           map[uint32][]byte
	cursors        []int32
	short          map[uint32]bool
	queryErr       map[uint32]error
	readErr        map[uint32]error
	negative       map[uint32]bool
	acquireErr     error
	releaseErr     error
	candidateErr   error
	panicRead      bool
	panicRelease   bool
	panicCandidate bool
}

func (api *fakeIMMAPI) Acquire(uintptr) (uintptr, error) {
	api.log = append(api.log, "acquire")
	if api.acquireErr != nil {
		return 0, api.acquireErr
	}
	return 7, nil
}
func (api *fakeIMMAPI) Release(uintptr, uintptr) error {
	api.log = append(api.log, "release")
	if api.panicRelease {
		panic("release panic")
	}
	return api.releaseErr
}
func (api *fakeIMMAPI) Read(_ uintptr, index uint32, destination []byte) (int32, error) {
	if api.panicRead {
		panic("read panic")
	}
	if index == gcsCursorPos {
		api.log = append(api.log, "cursor")
		if err := api.queryErr[index]; err != nil {
			return 0, err
		}
		if len(api.cursors) == 0 {
			return 0, nil
		}
		value := api.cursors[0]
		api.cursors = api.cursors[1:]
		return value, nil
	}
	phase := "query"
	if destination != nil {
		phase = "read"
	}
	api.log = append(api.log, phase+":"+immIndexName(index))
	data := api.data[index]
	if destination == nil {
		if err := api.queryErr[index]; err != nil {
			return 0, err
		}
		if api.negative[index] {
			return -1, nil
		}
		return int32(len(data)), nil
	}
	if err := api.readErr[index]; err != nil {
		return 0, err
	}
	copy(destination, data)
	if api.short[index] {
		return int32(len(data) - 1), nil
	}
	return int32(len(data)), nil
}
func (api *fakeIMMAPI) SetCandidate(_ uintptr, rect nativeCandidateRect, visible bool) error {
	if api.panicCandidate {
		panic("candidate panic")
	}
	if visible {
		api.log = append(api.log, "candidate:show")
	} else {
		api.log = append(api.log, "candidate:clear")
	}
	_ = rect
	return api.candidateErr
}

func immIndexName(index uint32) string {
	switch index {
	case gcsResultStr:
		return "result"
	case gcsCompStr:
		return "preedit"
	case gcsCompAttr:
		return "attributes"
	default:
		return "unknown"
	}
}

func utf16Bytes(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, len(units)*2)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(data[index*2:], unit)
	}
	return data
}

func newFakeIMMDecoder(api *fakeIMMAPI, log *[]string) *immDecoder {
	generation := uint64(0)
	return &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start: func() (uint64, error) {
			generation++
			*log = append(*log, "start")
			return generation, nil
		},
		Update: func(current uint64, update ime.NativeUpdate) error {
			*log = append(*log, "update")
			if current != generation || string(utf16.Decode(update.UTF16)) != "新" || update.CursorUTF16 != 1 {
				return errors.New("unexpected update")
			}
			return nil
		},
		Commit: func(current uint64, units []uint16) (string, error) {
			*log = append(*log, "commit")
			if current != generation || string(utf16.Decode(units)) != "済" {
				return "", errors.New("unexpected commit")
			}
			return "済", nil
		},
		Cancel: func(reason ime.CancelReason) error {
			*log = append(*log, "cancel:"+immCancelReasonName(reason))
			return nil
		},
		ArmEcho: func(current uint64, text string, _ time.Time) bool {
			*log = append(*log, "echo")
			return current != 0 && text == "済"
		},
		Now: func() time.Time { return time.Unix(10, 0) },
	}}
}

func TestIMMDecoderCombinedResultThenPreeditOrderingAndPairing(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{
		gcsResultStr: utf16Bytes("済"), gcsCompStr: utf16Bytes("新"), gcsCompAttr: {ime.AttributeTargetConverted},
	}, cursors: []int32{1, 1}}
	var callbacks []string
	decoder := newFakeIMMDecoder(api, &callbacks)
	if handled, err := decoder.handleMessage(wmIMEStartComposition, 0); !handled || err != nil {
		t.Fatalf("start handled=%v err=%v", handled, err)
	}
	flags := uintptr(gcsResultStr | gcsCompStr | gcsCompAttr | gcsCursorPos)
	if handled, err := decoder.handleMessage(wmIMEComposition, flags); !handled || err != nil {
		t.Fatalf("composition handled=%v err=%v", handled, err)
	}
	if want := []string{"start", "commit", "echo", "start", "update"}; !reflect.DeepEqual(callbacks, want) {
		t.Fatalf("callbacks=%v want=%v", callbacks, want)
	}
	if want := []string{"acquire", "query:result", "read:result", "cursor", "query:preedit", "read:preedit", "query:attributes", "read:attributes", "cursor", "release"}; !reflect.DeepEqual(api.log, want) {
		t.Fatalf("api log=%v want=%v", api.log, want)
	}
	if !decoder.active || decoder.generation != 2 {
		t.Fatalf("decoder=%#v", decoder)
	}
	if _, err := decoder.handleMessage(wmIMEEndComposition, 0); err != nil || decoder.active {
		t.Fatalf("end err=%v active=%v", err, decoder.active)
	}
}

func TestIMMDecoderRejectsMalformedPayloadBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name string
		api  *fakeIMMAPI
	}{
		{name: "odd UTF16", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: {1}}}},
		{name: "short read", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}, short: map[uint32]bool{gcsResultStr: true}}},
		{name: "cursor drift", api: &fakeIMMAPI{data: map[uint32][]byte{gcsCompStr: utf16Bytes("新")}, cursors: []int32{0, 1}}},
		{name: "attribute drift", api: &fakeIMMAPI{data: map[uint32][]byte{gcsCompStr: utf16Bytes("新"), gcsCompAttr: {1, 2}}, cursors: []int32{1, 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var callbacks []string
			decoder := newFakeIMMDecoder(test.api, &callbacks)
			if _, err := decoder.handleMessage(wmIMEStartComposition, 0); err != nil {
				t.Fatal(err)
			}
			flags := uintptr(gcsResultStr)
			if test.name == "cursor drift" || test.name == "attribute drift" {
				flags = gcsCompStr | gcsCompAttr | gcsCursorPos
			}
			if _, err := decoder.handleMessage(wmIMEComposition, flags); !errors.Is(err, errIMMReadInvalid) {
				t.Fatalf("err=%v", err)
			}
			if decoder.active || callbacks[len(callbacks)-1] != "cancel:malformed" || immContainsString(callbacks, "commit") || immContainsString(callbacks, "update") {
				t.Fatalf("decoder=%#v callbacks=%v", decoder, callbacks)
			}
			if test.api.log[0] != "acquire" || test.api.log[len(test.api.log)-1] != "release" {
				t.Fatalf("context log=%v", test.api.log)
			}
		})
	}
}

func TestIMMDecoderPanicContainmentAndCandidatePairing(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{}}
	decoder := &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start:  func() (uint64, error) { panic("boom") },
		Cancel: func(ime.CancelReason) error { return nil },
	}}
	if handled, err := decoder.handleMessage(wmIMEStartComposition, 0); !handled || !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	if err := decoder.publishCandidate(nativeCandidateRect{X: 1, Y: 2, Width: 3, Height: 4}); err != nil {
		t.Fatal(err)
	}
	if err := decoder.clearCandidate(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"acquire", "candidate:show", "release", "acquire", "candidate:clear", "release"}; !reflect.DeepEqual(api.log, want) {
		t.Fatalf("candidate log=%v", api.log)
	}
}

func immContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func immCancelReasonName(reason ime.CancelReason) string {
	if reason == ime.CancelMalformed {
		return "malformed"
	}
	if reason == ime.CancelExplicit {
		return "explicit"
	}
	return "other"
}

func TestDormantIMMDecoderRoutesOnceAndArmsEcho(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	app.initCompositionCoordinator()
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}}
	decoder := newDormantIMMDecoder(app, 11, api)
	decoder.callbacks.Now = func() time.Time { return time.Unix(20, 0) }
	if _, err := decoder.handleMessage(wmIMEStartComposition, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); err != nil {
		t.Fatal(err)
	}
	if got := factory.sessions[0].text(); got != "済" {
		t.Fatalf("routed=%q", got)
	}
	if !app.charSuppression.consume('済', time.Unix(20, 0).Add(time.Millisecond)) {
		t.Fatal("native result echo was not armed")
	}
	if app.charSuppression.consume('済', time.Unix(20, 0).Add(2*time.Millisecond)) {
		t.Fatal("native result echo suppressed more than once")
	}
}

func TestCompositionFocusLossClearsDormantNativeEcho(t *testing.T) {
	app := &App{}
	app.initCompositionCoordinator()
	now := time.Unix(30, 0)
	if !app.charSuppression.armIMEEcho(1, "x", now) {
		t.Fatal("failed to arm echo")
	}
	app.compositionNativeFocusChanged(false)
	if app.charSuppression.consume('x', now.Add(time.Millisecond)) {
		t.Fatal("focus loss retained stale native echo")
	}
}

func TestDormantIMMDecoderMalformedSurrogateCancelsWithoutRouting(t *testing.T) {
	app, factory := newRecordingActionApp(t)
	app.initCompositionCoordinator()
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: {0x00, 0xd8}}}
	decoder := newDormantIMMDecoder(app, 11, api)
	if _, err := decoder.handleMessage(wmIMEStartComposition, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); !errors.Is(err, ime.ErrInvalidUTF16) {
		t.Fatalf("malformed surrogate err=%v", err)
	}
	if decoder.active || app.composition.snapshot().Active || factory.sessions[0].text() != "" {
		t.Fatalf("decoderActive=%v snapshot=%#v text=%q", decoder.active, app.composition.snapshot(), factory.sessions[0].text())
	}
}

func TestIMMDecoderRejectsOversizedLengthBeforeSecondRead(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: make([]byte, 2*ime.MaxCommitUTF16Units+2)}}
	var callbacks []string
	decoder := newFakeIMMDecoder(api, &callbacks)
	if _, err := decoder.handleMessage(wmIMEStartComposition, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); !errors.Is(err, errIMMReadInvalid) {
		t.Fatalf("oversized err=%v", err)
	}
	if immContainsString(api.log, "read:result") {
		t.Fatalf("oversized payload reached second read: %v", api.log)
	}
}

func TestIMMDecoderContainsCancelCallbackPanic(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: {1}}}
	decoder := &immDecoder{hwnd: 11, api: api, active: true, generation: 1, callbacks: immCallbacks{
		Cancel: func(ime.CancelReason) error { panic("cancel panic") },
	}}
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); !errors.Is(err, errIMMCallbackPanic) || !errors.Is(err, errIMMReadInvalid) {
		t.Fatalf("contained cancel panic err=%v", err)
	}
	if decoder.active {
		t.Fatal("decoder remained active after contained cancel panic")
	}
}

func TestIMMDecoderCommitFailureCancelsWhileStillOwned(t *testing.T) {
	commitErr := errors.New("commit failed")
	cancelled := 0
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}}
	decoder := &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start:  func() (uint64, error) { return 1, nil },
		Commit: func(uint64, []uint16) (string, error) { return "", commitErr },
		Cancel: func(ime.CancelReason) error { cancelled++; return nil },
	}}
	if _, err := decoder.handleMessage(wmIMEStartComposition, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); !errors.Is(err, commitErr) {
		t.Fatalf("commit err=%v", err)
	}
	if cancelled != 1 || decoder.active {
		t.Fatalf("cancelled=%d active=%v", cancelled, decoder.active)
	}
}

func TestIMMDecoderStartPanicCancelsPartiallyStartedOwner(t *testing.T) {
	cancelled := 0
	decoder := &immDecoder{callbacks: immCallbacks{
		Start:  func() (uint64, error) { panic("after underlying start") },
		Cancel: func(ime.CancelReason) error { cancelled++; return nil },
	}}
	if _, err := decoder.handleMessage(wmIMEStartComposition, 0); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("start panic err=%v", err)
	}
	if cancelled != 1 || decoder.active {
		t.Fatalf("cancelled=%d active=%v", cancelled, decoder.active)
	}
}

func TestIMMDecoderValidatesCombinedPayloadBeforeCommit(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済"), gcsCompStr: {1}}, cursors: []int32{0}}
	commits := 0
	decoder := &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start:  func() (uint64, error) { return 1, nil },
		Commit: func(uint64, []uint16) (string, error) { commits++; return "済", nil },
		Cancel: func(ime.CancelReason) error { return nil },
	}}
	_, _ = decoder.handleMessage(wmIMEStartComposition, 0)
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr|gcsCompStr); !errors.Is(err, errIMMReadInvalid) {
		t.Fatalf("combined malformed err=%v", err)
	}
	if commits != 0 {
		t.Fatalf("result committed before malformed preedit validation: %d", commits)
	}
}

func TestIMMDecoderRejectsCommitTextMismatchWithoutEcho(t *testing.T) {
	echoes := 0
	api := &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}}
	decoder := &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start:   func() (uint64, error) { return 1, nil },
		Commit:  func(uint64, []uint16) (string, error) { return "wrong", nil },
		Cancel:  func(ime.CancelReason) error { return nil },
		ArmEcho: func(uint64, string, time.Time) bool { echoes++; return true },
	}}
	_, _ = decoder.handleMessage(wmIMEStartComposition, 0)
	if _, err := decoder.handleMessage(wmIMEComposition, gcsResultStr); !errors.Is(err, errIMMCommitMismatch) {
		t.Fatalf("mismatch err=%v", err)
	}
	if echoes != 0 {
		t.Fatalf("mismatched commit armed echo: %d", echoes)
	}
}

func TestIMMDecoderContextAndNativeFailurePaths(t *testing.T) {
	queryErr := errors.New("query")
	readErr := errors.New("read")
	releaseErr := errors.New("release")
	for _, test := range []struct {
		name string
		api  *fakeIMMAPI
		want error
	}{
		{name: "acquire", api: &fakeIMMAPI{acquireErr: queryErr}, want: errIMMContextUnavailable},
		{name: "query", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}, queryErr: map[uint32]error{gcsResultStr: queryErr}}, want: errIMMReadInvalid},
		{name: "negative length", api: &fakeIMMAPI{negative: map[uint32]bool{gcsResultStr: true}}, want: errIMMReadInvalid},
		{name: "read", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}, readErr: map[uint32]error{gcsResultStr: readErr}}, want: errIMMReadInvalid},
		{name: "negative cursor", api: &fakeIMMAPI{data: map[uint32][]byte{}, cursors: []int32{-1}}, want: errIMMReadInvalid},
		{name: "release", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}, releaseErr: releaseErr}, want: releaseErr},
		{name: "read panic", api: &fakeIMMAPI{panicRead: true}, want: errIMMCallbackPanic},
		{name: "release panic", api: &fakeIMMAPI{data: map[uint32][]byte{gcsResultStr: utf16Bytes("済")}, panicRelease: true}, want: errIMMCallbackPanic},
	} {
		t.Run(test.name, func(t *testing.T) {
			var callbacks []string
			decoder := newFakeIMMDecoder(test.api, &callbacks)
			_, _ = decoder.handleMessage(wmIMEStartComposition, 0)
			flags := uintptr(gcsResultStr)
			if test.name == "negative cursor" {
				flags = gcsCompStr
			}
			if _, err := decoder.handleMessage(wmIMEComposition, flags); !errors.Is(err, test.want) {
				t.Fatalf("failure err=%v want=%v log=%v", err, test.want, test.api.log)
			}
			if test.name != "acquire" && test.api.log[len(test.api.log)-1] != "release" {
				t.Fatalf("context not released: %v", test.api.log)
			}
		})
	}
}

func TestIMMDecoderContainsCandidatePanicAfterReleaseAttempt(t *testing.T) {
	api := &fakeIMMAPI{panicCandidate: true}
	decoder := &immDecoder{hwnd: 11, api: api}
	if err := decoder.publishCandidate(nativeCandidateRect{Width: 1, Height: 1}); !errors.Is(err, errIMMCallbackPanic) {
		t.Fatalf("candidate panic err=%v", err)
	}
	if want := []string{"acquire", "release"}; !reflect.DeepEqual(api.log, want) {
		t.Fatalf("candidate panic ownership=%v", api.log)
	}
}

func TestIMMDecoderValidatesCombinedAttributeSemanticsBeforeCommit(t *testing.T) {
	api := &fakeIMMAPI{data: map[uint32][]byte{
		gcsResultStr: utf16Bytes("済"), gcsCompStr: utf16Bytes("新"), gcsCompAttr: {0xff},
	}, cursors: []int32{1, 1}}
	commits := 0
	decoder := &immDecoder{hwnd: 11, api: api, callbacks: immCallbacks{
		Start:  func() (uint64, error) { return 1, nil },
		Commit: func(uint64, []uint16) (string, error) { commits++; return "済", nil },
		Cancel: func(ime.CancelReason) error { return nil },
	}}
	_, _ = decoder.handleMessage(wmIMEStartComposition, 0)
	flags := uintptr(gcsResultStr | gcsCompStr | gcsCompAttr | gcsCursorPos)
	if _, err := decoder.handleMessage(wmIMEComposition, flags); !errors.Is(err, ime.ErrInvalidAttributes) {
		t.Fatalf("combined attribute err=%v", err)
	}
	if commits != 0 {
		t.Fatalf("result committed before attribute validation: %d", commits)
	}
}
