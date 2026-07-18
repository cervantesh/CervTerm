package action

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func focused(action Action) Envelope {
	return Envelope{Action: action, Target: TargetFocused}
}

func TestActionsValidate(t *testing.T) {
	valid := []Action{
		CopySelection{}, PasteClipboard{}, ToggleSearch{}, ToggleStats{}, ActivateCommandPalette{}, ActivateQuickSelect{}, ActivateLaunchMenu{}, ReloadConfig{}, ClosePane{},
		Scroll{Unit: ScrollLine, Amount: 1}, Scroll{Unit: ScrollPage, Amount: -2}, Scroll{Unit: ScrollBuffer, Amount: 1},
		Zoom{Mode: ZoomDelta, Amount: -1}, Zoom{Mode: ZoomReset},
		SplitPane{Axis: SplitColumns}, SplitPane{Axis: SplitRows},
		FocusPane{Direction: FocusLeft}, FocusPane{Direction: FocusRight}, FocusPane{Direction: FocusUp}, FocusPane{Direction: FocusDown},
		Multiple{actions: []Envelope{focused(CopySelection{})}},
		Callback{BindingIndex: 0},
	}
	for _, action := range valid {
		t.Run(string(action.ID()), func(t *testing.T) {
			if err := action.Validate(); err != nil {
				t.Fatalf("Validate() failed: %v", err)
			}
		})
	}
}

func TestActionValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		action Action
		want   string
	}{
		{name: "scroll unit", action: Scroll{Unit: "pixel", Amount: 1}, want: "unit"},
		{name: "scroll zero", action: Scroll{Unit: ScrollLine}, want: "zero"},
		{name: "buffer amount", action: Scroll{Unit: ScrollBuffer, Amount: 2}, want: "-1 or 1"},
		{name: "zoom mode", action: Zoom{Mode: "absolute", Amount: 1}, want: "mode"},
		{name: "zoom zero delta", action: Zoom{Mode: ZoomDelta}, want: "must not be zero"},
		{name: "zoom reset amount", action: Zoom{Mode: ZoomReset, Amount: 1}, want: "must be zero"},
		{name: "zoom nan", action: Zoom{Mode: ZoomDelta, Amount: math.NaN()}, want: "finite"},
		{name: "split axis", action: SplitPane{Axis: "diagonal"}, want: "axis"},
		{name: "focus direction", action: FocusPane{Direction: "next"}, want: "direction"},
		{name: "empty multiple", action: Multiple{}, want: "at least one"},
		{name: "bad multiple child", action: Multiple{actions: []Envelope{focused(Scroll{})}}, want: "action 0"},
		{name: "callback index", action: Callback{BindingIndex: -1}, want: "must not be negative"},
	}
	tooMany := make([]Envelope, MaxSequenceActions+1)
	for i := range tooMany {
		tooMany[i] = focused(CopySelection{})
	}
	tests = append(tests, struct {
		name   string
		action Action
		want   string
	}{name: "multiple limit", action: Multiple{actions: tooMany}, want: "maximum"})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestEnvelopeValidationAndConstruction(t *testing.T) {
	envelope, err := New(Scroll{Unit: ScrollPage, Amount: 1}, TargetOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Target != TargetOrigin || envelope.Action.ID() != IDScroll {
		t.Fatalf("New() = %#v", envelope)
	}

	validScroll := &Scroll{Unit: ScrollLine, Amount: 1}
	var nilScroll *Scroll
	tests := []struct {
		name string
		env  Envelope
		want string
	}{
		{name: "nil action", env: Envelope{Target: TargetFocused}, want: "required"},
		{name: "bad target", env: Envelope{Action: CopySelection{}, Target: "pane-7"}, want: "target"},
		{name: "bad action", env: focused(Scroll{}), want: "scroll unit"},
		{name: "action pointer", env: focused(validScroll), want: "concrete value"},
		{name: "typed nil action", env: focused(nilScroll), want: "concrete value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOpaqueReferences(t *testing.T) {
	for _, ref := range []Ref{{Kind: RefPane, ID: 1}, {Kind: RefTab, ID: 2}, {Kind: RefWindow, ID: 3}} {
		if !ref.Valid() {
			t.Fatalf("ref should be valid: %#v", ref)
		}
	}
	for _, ref := range []Ref{{Kind: RefPane}, {Kind: "domain", ID: 1}} {
		if ref.Valid() {
			t.Fatalf("ref should be invalid: %#v", ref)
		}
	}
}

func TestMultipleOwnsChildren(t *testing.T) {
	input := []Envelope{focused(CopySelection{})}
	multiple, err := NewMultiple(input...)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = focused(ClosePane{})
	got := multiple.Actions()
	if got[0].Action.ID() != IDCopySelection {
		t.Fatalf("constructor retained caller slice: %#v", got)
	}
	got[0] = focused(ClosePane{})
	if multiple.Actions()[0].Action.ID() != IDCopySelection {
		t.Fatal("Actions returned mutable internal storage")
	}
}

func TestContextValidationAndResolution(t *testing.T) {
	context := Context{
		Source:  SourceKeyboard,
		Origin:  Ref{Kind: RefPane, ID: 1},
		Focused: Ref{Kind: RefPane, ID: 2},
	}
	if err := context.Validate(); err != nil {
		t.Fatal(err)
	}
	ref, err := context.Resolve(TargetFocused)
	if err != nil || ref.ID != 2 {
		t.Fatalf("Resolve(focused) = %#v, %v", ref, err)
	}
	ref, err = context.Resolve(TargetOrigin)
	if err != nil || ref.ID != 1 {
		t.Fatalf("Resolve(origin) = %#v, %v", ref, err)
	}
	if _, err := (Context{Source: SourceKeyboard}).Resolve(TargetFocused); !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("missing focused target error = %v", err)
	}
	if err := (Context{Source: "domain"}).Validate(); err == nil {
		t.Fatal("invalid source passed validation")
	}
	if err := (Context{Source: SourceScript, Origin: Ref{Kind: "domain", ID: 1}}).Validate(); err == nil {
		t.Fatal("invalid origin passed validation")
	}
	if err := (Context{Source: SourceScript, Origin: Ref{Kind: RefPane}}).Validate(); err == nil {
		t.Fatal("partially populated zero-ID origin passed validation")
	}
	if err := (Context{Source: SourceScript, Focused: Ref{Kind: "domain"}}).Validate(); err == nil {
		t.Fatal("malformed zero-ID focused reference passed validation")
	}
}

func TestExecutionErrorUnwrapsCause(t *testing.T) {
	err := &ExecutionError{ActionID: IDFocusPane, Class: ErrorTarget, Err: ErrTargetUnavailable}
	if !errors.Is(err, ErrTargetUnavailable) {
		t.Fatalf("ExecutionError does not unwrap cause: %v", err)
	}
	if !strings.Contains(err.Error(), string(IDFocusPane)) || !strings.Contains(err.Error(), string(ErrorTarget)) {
		t.Fatalf("ExecutionError message = %q", err)
	}
}

func TestMultipleBoundsAggregateNodes(t *testing.T) {
	leaves := make([]Envelope, MaxSequenceActions)
	for i := range leaves {
		leaves[i] = focused(CopySelection{})
	}
	group, err := NewMultiple(leaves...)
	if err != nil {
		t.Fatal(err)
	}
	groups := make([]Envelope, MaxSequenceActions)
	for i := range groups {
		groups[i] = focused(group)
	}
	if _, err := NewMultiple(groups...); err == nil || !strings.Contains(err.Error(), "maximum nodes") {
		t.Fatalf("NewMultiple(large graph) error = %v", err)
	}
}
