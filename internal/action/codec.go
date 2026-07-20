package action

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
)

const (
	MaxJSONDepth = MaxSequenceDepth
	MaxJSONBytes = 1 << 20
)

type codecBudget struct{ nodes int }

func (b *codecBudget) take() error {
	b.nodes++
	if b.nodes > MaxActionNodes {
		return fmt.Errorf("action graph exceeds maximum nodes %d", MaxActionNodes)
	}
	return nil
}

type Codec struct{ registry *Registry }

func NewCodec(registry *Registry) (*Codec, error) {
	if registry == nil {
		return nil, errors.New("action codec registry is required")
	}
	return &Codec{registry: registry}, nil
}

var (
	defaultCodecOnce sync.Once
	defaultCodec     *Codec
)

func mustCodec(registry *Registry) *Codec {
	codec, err := NewCodec(registry)
	if err != nil {
		panic(err)
	}
	return codec
}

func actionCodec() *Codec {
	defaultCodecOnce.Do(func() { defaultCodec = mustCodec(DefaultRegistry()) })
	return defaultCodec
}

func Marshal(envelope Envelope) ([]byte, error) { return actionCodec().Marshal(envelope) }
func Unmarshal(data []byte) (Envelope, error)   { return actionCodec().Unmarshal(data) }
func (e Envelope) MarshalJSON() ([]byte, error) { return Marshal(e) }

func (e *Envelope) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("cannot unmarshal action into nil envelope")
	}
	decoded, err := Unmarshal(data)
	if err != nil {
		return err
	}
	*e = decoded
	return nil
}

func (c *Codec) Marshal(envelope Envelope) ([]byte, error) {
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	return c.marshalEnvelope(envelope, 0, &codecBudget{})
}

func (c *Codec) Unmarshal(data []byte) (Envelope, error) {
	if len(data) > MaxJSONBytes {
		return Envelope{}, fmt.Errorf("action JSON has %d bytes; maximum is %d", len(data), MaxJSONBytes)
	}
	return c.unmarshalEnvelope(data, 0, &codecBudget{})
}

type envelopeWire struct {
	Type   ID              `json:"type"`
	Target TargetSelector  `json:"target"`
	Args   json.RawMessage `json:"args"`
}

type emptyArgs struct{}
type scrollArgs struct {
	Unit   ScrollUnit `json:"unit"`
	Amount int        `json:"amount"`
}
type scrollToPromptArgs struct {
	Delta int `json:"delta"`
}
type copySemanticZoneArgs struct {
	Zone SemanticZone `json:"zone"`
}
type zoomArgs struct {
	Mode   ZoomMode `json:"mode"`
	Amount float64  `json:"amount"`
}
type splitPaneArgs struct {
	Axis SplitAxis `json:"axis"`
}
type focusPaneArgs struct {
	Direction Direction `json:"direction"`
}
type resizePaneArgs struct {
	Direction Direction `json:"direction"`
	Delta     int       `json:"delta"`
}
type directionalPaneArgs struct {
	Direction Direction `json:"direction"`
}
type multipleArgs struct {
	Actions []json.RawMessage `json:"actions"`
}

func (c *Codec) marshalEnvelope(envelope Envelope, depth int, budget *codecBudget) ([]byte, error) {
	if depth > MaxJSONDepth {
		return nil, fmt.Errorf("action nesting exceeds maximum depth %d", MaxJSONDepth)
	}
	if err := budget.take(); err != nil {
		return nil, err
	}
	id, err := actionIdentity(envelope.Action)
	if err != nil {
		return nil, err
	}
	item, ok := c.registry.lookupRegistration(id)
	if !ok {
		return nil, fmt.Errorf("action %q is not registered in this codec", id)
	}
	if !item.descriptor.Serializable {
		return nil, fmt.Errorf("action %q: %w", id, ErrNotSerializable)
	}
	encodedArgs, err := item.codec.encode(envelope.Action, c, depth, budget)
	if err != nil {
		return nil, fmt.Errorf("action %q arguments: %w", id, err)
	}
	return json.Marshal(envelopeWire{Type: id, Target: envelope.Target, Args: encodedArgs})
}

func (c *Codec) unmarshalEnvelope(data []byte, depth int, budget *codecBudget) (Envelope, error) {
	if depth > MaxJSONDepth {
		return Envelope{}, fmt.Errorf("action nesting exceeds maximum depth %d", MaxJSONDepth)
	}
	if err := budget.take(); err != nil {
		return Envelope{}, err
	}
	var wire envelopeWire
	if err := decodeObject(data, &wire); err != nil {
		return Envelope{}, fmt.Errorf("action envelope: %w", err)
	}
	if len(wire.Args) == 0 {
		return Envelope{}, fmt.Errorf("action %q arguments are required", wire.Type)
	}
	item, ok := c.registry.lookupRegistration(wire.Type)
	if !ok {
		return Envelope{}, fmt.Errorf("unknown action %q", wire.Type)
	}
	if !item.descriptor.Serializable {
		return Envelope{}, fmt.Errorf("action %q: %w", wire.Type, ErrNotSerializable)
	}
	action, err := item.codec.decode(wire.Args, c, depth, budget)
	if err != nil {
		return Envelope{}, fmt.Errorf("action %q arguments: %w", wire.Type, err)
	}
	envelope := Envelope{Action: action, Target: wire.Target}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func simpleCodec(value Action) codecOps {
	return codecOps{
		encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
			got, err := actionIdentity(action)
			if err != nil {
				return nil, err
			}
			if got != value.ID() {
				return nil, fmt.Errorf("codec for %q received %q", value.ID(), got)
			}
			return json.Marshal(emptyArgs{})
		},
		decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
			if err := decodeObject(data, &emptyArgs{}); err != nil {
				return nil, err
			}
			return value, nil
		},
	}
}

var scrollCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(Scroll)
		if !ok {
			return nil, fmt.Errorf("expected Scroll, got %T", action)
		}
		return json.Marshal(scrollArgs{Unit: value.Unit, Amount: value.Amount})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args scrollArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return Scroll{Unit: args.Unit, Amount: args.Amount}, nil
	},
}

var zoomCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(Zoom)
		if !ok {
			return nil, fmt.Errorf("expected Zoom, got %T", action)
		}
		return json.Marshal(zoomArgs{Mode: value.Mode, Amount: value.Amount})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args zoomArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return Zoom{Mode: args.Mode, Amount: args.Amount}, nil
	},
}

var splitPaneCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(SplitPane)
		if !ok {
			return nil, fmt.Errorf("expected SplitPane, got %T", action)
		}
		return json.Marshal(splitPaneArgs{Axis: value.Axis})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args splitPaneArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return SplitPane{Axis: args.Axis}, nil
	},
}

var focusPaneCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(FocusPane)
		if !ok {
			return nil, fmt.Errorf("expected FocusPane, got %T", action)
		}
		return json.Marshal(focusPaneArgs{Direction: value.Direction})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args focusPaneArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return FocusPane{Direction: args.Direction}, nil
	},
}

func directionalPaneCodec(name string, makeAction func(Direction) Action) codecOps {
	return codecOps{
		encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
			direction := Direction("")
			switch value := action.(type) {
			case SwapPane:
				direction = value.Direction
			case MovePane:
				direction = value.Direction
			default:
				return nil, fmt.Errorf("expected %s, got %T", name, action)
			}
			return json.Marshal(directionalPaneArgs{Direction: direction})
		},
		decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
			var args directionalPaneArgs
			if err := decodeObject(data, &args); err != nil {
				return nil, err
			}
			return makeAction(args.Direction), nil
		},
	}
}

var resizePaneCodec = codecOps{
	encode: func(action Action, _ *Codec, _ int, _ *codecBudget) (json.RawMessage, error) {
		value, ok := action.(ResizePane)
		if !ok {
			return nil, fmt.Errorf("expected ResizePane, got %T", action)
		}
		return json.Marshal(resizePaneArgs{Direction: value.Direction, Delta: value.Delta})
	},
	decode: func(data json.RawMessage, _ *Codec, _ int, _ *codecBudget) (Action, error) {
		var args resizePaneArgs
		if err := decodeObject(data, &args); err != nil {
			return nil, err
		}
		return ResizePane{Direction: args.Direction, Delta: args.Delta}, nil
	},
}

var swapPaneCodec = directionalPaneCodec("SwapPane", func(direction Direction) Action {
	return SwapPane{Direction: direction}
})

var movePaneCodec = directionalPaneCodec("MovePane", func(direction Direction) Action {
	return MovePane{Direction: direction}
})

var multipleCodec = codecOps{
	encode: func(action Action, codec *Codec, depth int, budget *codecBudget) (json.RawMessage, error) {
		value, ok := action.(Multiple)
		if !ok {
			return nil, fmt.Errorf("expected Multiple, got %T", action)
		}
		children := make([]json.RawMessage, len(value.actions))
		for i, child := range value.actions {
			encoded, err := codec.marshalEnvelope(child, depth+1, budget)
			if err != nil {
				return nil, fmt.Errorf("child %d: %w", i, err)
			}
			children[i] = encoded
		}
		return json.Marshal(multipleArgs{Actions: children})
	},
	decode: func(data json.RawMessage, codec *Codec, depth int, budget *codecBudget) (Action, error) {
		rawActions, err := decodeMultipleActions(data)
		if err != nil {
			return nil, err
		}
		if len(rawActions) == 0 {
			return nil, errors.New("multiple requires at least one action")
		}
		children := make([]Envelope, len(rawActions))
		for i, raw := range rawActions {
			child, err := codec.unmarshalEnvelope(raw, depth+1, budget)
			if err != nil {
				return nil, fmt.Errorf("child %d: %w", i, err)
			}
			children[i] = child
		}
		return Multiple{actions: children}, nil
	},
}

func decodeMultipleActions(data []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	first, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return nil, errors.New("must be a JSON object")
	}
	seenActions := false
	var actions []json.RawMessage
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("JSON object key must be a string")
		}
		if key != "actions" {
			return nil, fmt.Errorf("unknown field %q", key)
		}
		if seenActions {
			return nil, fmt.Errorf("duplicate field %q", key)
		}
		seenActions = true
		start, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if delimiter, ok := start.(json.Delim); !ok || delimiter != '[' {
			return nil, errors.New("actions must be a JSON array")
		}
		for decoder.More() {
			if len(actions) >= MaxSequenceActions {
				return nil, fmt.Errorf("multiple has more than %d actions; maximum is %d", MaxSequenceActions, MaxSequenceActions)
			}
			var raw json.RawMessage
			if err := decoder.Decode(&raw); err != nil {
				return nil, err
			}
			actions = append(actions, raw)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if err := requireEOF(decoder); err != nil {
		return nil, err
	}
	if !seenActions {
		return nil, errors.New("missing field \"actions\"")
	}
	return actions, nil
}

func decodeObject(data []byte, dst any) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("must be a JSON object")
	}
	allowed, err := jsonFieldNames(dst)
	if err != nil {
		return err
	}
	if err := scanObjectKeys(trimmed, allowed); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func jsonFieldNames(dst any) (map[string]struct{}, error) {
	typeOf := reflect.TypeOf(dst)
	if typeOf == nil || typeOf.Kind() != reflect.Pointer || typeOf.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("JSON destination must be a pointer to struct, got %T", dst)
	}
	typeOf = typeOf.Elem()
	fields := make(map[string]struct{}, typeOf.NumField())
	for i := 0; i < typeOf.NumField(); i++ {
		name := typeOf.Field(i).Tag.Get("json")
		if comma := bytes.IndexByte([]byte(name), ','); comma >= 0 {
			name = name[:comma]
		}
		if name != "" && name != "-" {
			fields[name] = struct{}{}
		}
	}
	return fields, nil
}

func scanObjectKeys(data []byte, allowed map[string]struct{}) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := first.(json.Delim); !ok || delimiter != '{' {
		return errors.New("must be a JSON object")
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("JSON object key must be a string")
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown field %q", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = struct{}{}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func requireEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
