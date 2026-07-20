package ime

import (
	"errors"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestControllerLifecycleCommitAndDetachedSnapshots(t *testing.T) {
	var controller Controller
	target := Target{Kind: TargetPane, ID: 42, Activation: 7}
	generation, err := controller.Start(target)
	if err != nil || generation == 0 {
		t.Fatalf("start generation=%d err=%v", generation, err)
	}
	text := "A😀e\u0301"
	units := utf16.Encode([]rune(text))
	attributes := []byte{AttributeInput, AttributeTargetConverted, AttributeTargetConverted, AttributeInput, AttributeInput}
	if err := controller.Update(generation, NativeUpdate{UTF16: units, CursorUTF16: 3, Attributes: attributes}); err != nil {
		t.Fatal(err)
	}
	units[0] = 'Z'
	attributes[1] = AttributeInput
	snapshot := controller.Snapshot()
	if !snapshot.Active || snapshot.Target != target || snapshot.Text != text || snapshot.CursorRune != 2 || snapshot.TargetRuneSpan != (Span{Start: 1, End: 2}) {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	snapshot.Runes[0] = 'Z'
	if fresh := controller.Snapshot(); fresh.Text != text || fresh.Runes[0] != 'A' {
		t.Fatalf("snapshot aliased controller: %#v", fresh)
	}

	commitText := "日本語😀"
	commit, err := controller.Commit(generation, utf16.Encode([]rune(commitText)))
	if err != nil {
		t.Fatal(err)
	}
	if commit.Target != target || commit.Generation != generation || commit.Text != commitText || string(commit.Runes) != commitText {
		t.Fatalf("commit=%#v", commit)
	}
	finished := controller.Snapshot()
	if finished.Active || finished.Target != (Target{}) || finished.Text != "" || len(finished.Runes) != 0 || finished.LastCancel != CancelNone || finished.Revision != 3 {
		t.Fatalf("finished=%#v", finished)
	}
}

func TestControllerRejectsStaleAndOutOfOrderEvents(t *testing.T) {
	var controller Controller
	if err := controller.Update(1, NativeUpdate{}); !errors.Is(err, ErrInactive) {
		t.Fatalf("inactive update=%v", err)
	}
	generation, err := controller.Start(Target{Kind: TargetModal, ID: 3, Activation: 9})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Start(Target{Kind: TargetPane, ID: 1, Activation: 1}); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("double start=%v", err)
	}
	if err := controller.Update(generation+1, NativeUpdate{}); !errors.Is(err, ErrInvalidGeneration) {
		t.Fatalf("stale update=%v", err)
	}
	if _, err := controller.Commit(generation+1, []uint16{'x'}); !errors.Is(err, ErrInvalidGeneration) {
		t.Fatalf("stale commit=%v", err)
	}
	if err := controller.Cancel(generation, CancelFocusLost); err != nil {
		t.Fatal(err)
	}
	cancelled := controller.Snapshot()
	if cancelled.Active || cancelled.LastCancel != CancelFocusLost || cancelled.Revision != 2 {
		t.Fatalf("cancelled=%#v", cancelled)
	}
	if err := controller.Cancel(generation, CancelExplicit); !errors.Is(err, ErrInactive) {
		t.Fatalf("second cancel=%v", err)
	}
	next, err := controller.Start(Target{Kind: TargetSearch, ID: 4, Activation: 10})
	if err != nil || next == generation {
		t.Fatalf("next generation=%d err=%v", next, err)
	}
	if controller.Snapshot().LastCancel != CancelNone {
		t.Fatal("new start retained cancellation reason")
	}
}

func TestControllerFailedUpdatesAndCommitsAreAtomic(t *testing.T) {
	var controller Controller
	generation, err := controller.Start(Target{Kind: TargetPane, ID: 1, Activation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Update(generation, NativeUpdate{UTF16: []uint16{'o', 'k'}, CursorUTF16: 2}); err != nil {
		t.Fatal(err)
	}
	before := controller.Snapshot()
	invalid := []struct {
		name   string
		update NativeUpdate
		err    error
	}{
		{name: "surrogate", update: NativeUpdate{UTF16: []uint16{0xD800}, CursorUTF16: 0}, err: ErrInvalidUTF16},
		{name: "cursor", update: NativeUpdate{UTF16: utf16.Encode([]rune("😀")), CursorUTF16: 1}, err: ErrInvalidCursor},
		{name: "attribute length", update: NativeUpdate{UTF16: []uint16{'x', 'y'}, CursorUTF16: 1, Attributes: []byte{AttributeInput}}, err: ErrInvalidAttributes},
		{name: "attribute value", update: NativeUpdate{UTF16: []uint16{'x'}, CursorUTF16: 1, Attributes: []byte{99}}, err: ErrInvalidAttributes},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			err := controller.Update(generation, test.update)
			if !errors.Is(err, test.err) {
				t.Fatalf("err=%v want=%v", err, test.err)
			}
			if after := controller.Snapshot(); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed update mutated state\nbefore=%#v\n after=%#v", before, after)
			}
		})
	}
	if _, err := controller.Commit(generation, []uint16{0xDC00}); !errors.Is(err, ErrInvalidUTF16) {
		t.Fatalf("invalid commit=%v", err)
	}
	if after := controller.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("failed commit mutated state\nbefore=%#v\n after=%#v", before, after)
	}
}

func TestControllerValidatesTargetsAndCancelReasons(t *testing.T) {
	for _, target := range []Target{{}, {Kind: TargetNone, ID: 1, Activation: 1}, {Kind: TargetPane, Activation: 1}, {Kind: TargetSearch, ID: 1}} {
		var controller Controller
		if _, err := controller.Start(target); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("target=%#v err=%v", target, err)
		}
	}
	var controller Controller
	if err := controller.Cancel(1, CancelNone); !errors.Is(err, ErrInactive) {
		t.Fatalf("inactive invalid cancel err=%v", err)
	}
	generation, _ := controller.Start(Target{Kind: TargetPane, ID: 1, Activation: 1})
	if err := controller.Cancel(generation, CancelNone); !errors.Is(err, ErrInvalidCancelReason) || !controller.Snapshot().Active {
		t.Fatalf("invalid cancel err=%v state=%#v", err, controller.Snapshot())
	}
}

func TestControllerRejectsCounterRolloverAtomically(t *testing.T) {
	target := Target{Kind: TargetPane, ID: 1, Activation: 1}
	controller := Controller{generation: maxCounter}
	if _, err := controller.Start(target); !errors.Is(err, ErrCounterExhausted) || controller.Snapshot().Active {
		t.Fatalf("generation rollover err=%v state=%#v", err, controller.Snapshot())
	}

	controller = Controller{revision: maxCounter}
	if _, err := controller.Start(target); !errors.Is(err, ErrCounterExhausted) || controller.Snapshot().Active {
		t.Fatalf("revision start rollover err=%v state=%#v", err, controller.Snapshot())
	}

	controller = Controller{active: true, target: target, generation: 7, revision: maxCounter}
	before := controller.Snapshot()
	if err := controller.Update(7, NativeUpdate{}); !errors.Is(err, ErrCounterExhausted) {
		t.Fatalf("update rollover err=%v", err)
	}
	if _, err := controller.Commit(7, []uint16{'x'}); !errors.Is(err, ErrCounterExhausted) {
		t.Fatalf("commit rollover err=%v", err)
	}
	if err := controller.Cancel(7, CancelExplicit); !errors.Is(err, ErrCounterExhausted) {
		t.Fatalf("cancel rollover err=%v", err)
	}
	if after := controller.Snapshot(); !reflect.DeepEqual(after, before) {
		t.Fatalf("counter rollover mutated state\nbefore=%#v\n after=%#v", before, after)
	}
}
