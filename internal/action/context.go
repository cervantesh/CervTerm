package action

import (
	"errors"
	"fmt"
)

type TargetSelector string

const (
	TargetFocused TargetSelector = "focused"
	TargetOrigin  TargetSelector = "origin"
)

type RefKind string

const (
	RefPane   RefKind = "pane"
	RefTab    RefKind = "tab"
	RefWindow RefKind = "window"
)

// Ref is an opaque process-local identity. The frontend owns conversion to mux
// identities; serialized actions never contain a Ref.
type Ref struct {
	Kind RefKind
	ID   uint64
}

func (r Ref) Valid() bool {
	return r.ID != 0 && (r.Kind == RefPane || r.Kind == RefTab || r.Kind == RefWindow)
}

type Source string

const (
	SourceKeyboard Source = "keyboard"
	SourceMouse    Source = "mouse"
	SourcePalette  Source = "palette"
	SourceScript   Source = "script"
)

type Context struct {
	Source        Source
	Origin        Ref
	Focused       Ref
	OriginWindow  Ref
	FocusedWindow Ref
}

func (c Context) Validate() error {
	switch c.Source {
	case SourceKeyboard, SourceMouse, SourcePalette, SourceScript:
	default:
		return fmt.Errorf("action source %q is invalid", c.Source)
	}
	if c.Origin != (Ref{}) && !c.Origin.Valid() {
		return fmt.Errorf("action origin reference is invalid")
	}
	if c.Focused != (Ref{}) && !c.Focused.Valid() {
		return fmt.Errorf("action focused reference is invalid")
	}
	if c.OriginWindow != (Ref{}) && (!c.OriginWindow.Valid() || c.OriginWindow.Kind != RefWindow) {
		return fmt.Errorf("action origin window reference is invalid")
	}
	if c.FocusedWindow != (Ref{}) && (!c.FocusedWindow.Valid() || c.FocusedWindow.Kind != RefWindow) {
		return fmt.Errorf("action focused window reference is invalid")
	}
	return nil
}

// Resolve evaluates a selector at the moment an executor invokes it. Executors
// call it again for each child in Multiple, so a focus action can affect the
// next child's focused target while origin remains fixed in Context.
func (c Context) Resolve(selector TargetSelector) (Ref, error) {
	var ref Ref
	switch selector {
	case TargetFocused:
		ref = c.Focused
	case TargetOrigin:
		ref = c.Origin
	default:
		return Ref{}, fmt.Errorf("target selector %q is invalid", selector)
	}
	if !ref.Valid() {
		return Ref{}, ErrTargetUnavailable
	}
	return ref, nil
}

// Envelope adds semantic target selection to an action value.
type Envelope struct {
	Action Action
	Target TargetSelector
}

func New(action Action, target TargetSelector) (Envelope, error) {
	envelope := Envelope{Action: action, Target: target}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (e Envelope) Validate() error {
	nodes := 0
	return e.validateDepth(0, &nodes)
}

func (e Envelope) validateDepth(depth int, nodes *int) error {
	if e.Action == nil {
		return errors.New("action is required")
	}
	id, err := actionIdentity(e.Action)
	if err != nil {
		return err
	}
	*nodes++
	if *nodes > MaxActionNodes {
		return fmt.Errorf("action graph exceeds maximum nodes %d", MaxActionNodes)
	}
	if e.Target != TargetFocused && e.Target != TargetOrigin {
		return fmt.Errorf("action %q target %q is invalid", id, e.Target)
	}
	if multiple, ok := e.Action.(Multiple); ok {
		err = validateMultiple(multiple, depth, nodes)
	} else {
		err = e.Action.Validate()
	}
	if err != nil {
		return fmt.Errorf("action %q: %w", id, err)
	}
	return nil
}
