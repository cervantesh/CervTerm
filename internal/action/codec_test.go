package action

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCodecRoundTripsBuiltInActions(t *testing.T) {
	multiple, err := NewMultiple(
		focused(FocusPane{Direction: FocusLeft}),
		Envelope{Action: Scroll{Unit: ScrollLine, Amount: 3}, Target: TargetOrigin},
	)
	if err != nil {
		t.Fatal(err)
	}
	tests := []Envelope{
		focused(CopySelection{}),
		focused(CopySemanticZone{Zone: SemanticZoneOutput}),
		focused(SelectSemanticZone{Zone: SemanticZoneInput}),
		focused(PasteClipboard{}),
		focused(ToggleSearch{}),
		focused(ToggleStats{}),
		focused(ActivateCommandPalette{}),
		focused(ActivateQuickSelect{}),
		focused(ActivateLaunchMenu{}),
		focused(ReloadConfig{}),
		focused(ClosePane{}),
		focused(Scroll{Unit: ScrollPage, Amount: -1}),
		focused(ScrollToPrompt{Delta: -1}),
		focused(Zoom{Mode: ZoomDelta, Amount: 1.5}),
		focused(Zoom{Mode: ZoomReset}),
		focused(SplitPane{Axis: SplitColumns}),
		focused(FocusPane{Direction: FocusDown}),
		focused(ResizePane{Direction: FocusRight, Delta: 3}),
		focused(SwapPane{Direction: FocusLeft}),
		focused(MovePane{Direction: FocusUp}),
		focused(multiple),
	}

	for _, want := range tests {
		t.Run(string(want.Action.ID()), func(t *testing.T) {
			encoded, err := Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Unmarshal(encoded)
			if err != nil {
				t.Fatalf("Unmarshal(%s): %v", encoded, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip = %#v, want %#v\nJSON: %s", got, want, encoded)
			}

			standard, err := json.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			var viaJSON Envelope
			if err := json.Unmarshal(standard, &viaJSON); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(viaJSON, want) {
				t.Fatalf("encoding/json round trip = %#v, want %#v", viaJSON, want)
			}
		})
	}
}

func TestCodecCanonicalScrollJSON(t *testing.T) {
	encoded, err := Marshal(Envelope{
		Action: Scroll{Unit: ScrollPage, Amount: -1},
		Target: TargetFocused,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"scroll","target":"focused","args":{"unit":"page","amount":-1}}`
	if string(encoded) != want {
		t.Fatalf("Marshal() = %s, want %s", encoded, want)
	}
}

func TestCodecRejectsInvalidJSONAndActions(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "not object", data: `[]`, want: "JSON object"},
		{name: "unknown envelope field", data: `{"type":"copy_selection","target":"focused","args":{},"extra":1}`, want: "unknown field"},
		{name: "case varied envelope field", data: `{"TYPE":"copy_selection","target":"focused","args":{}}`, want: "unknown field"},
		{name: "duplicate envelope field", data: `{"type":"copy_selection","type":"close_pane","target":"focused","args":{}}`, want: "duplicate field"},
		{name: "trailing value", data: `{"type":"copy_selection","target":"focused","args":{}} {}`, want: "multiple JSON"},
		{name: "unknown action", data: `{"type":"launch_domain","target":"focused","args":{}}`, want: "unknown action"},
		{name: "missing args", data: `{"type":"copy_selection","target":"focused"}`, want: "arguments are required"},
		{name: "args not object", data: `{"type":"copy_selection","target":"focused","args":[]}`, want: "JSON object"},
		{name: "unknown args", data: `{"type":"copy_selection","target":"focused","args":{"force":true}}`, want: "unknown field"},
		{name: "case varied arg", data: `{"type":"scroll","target":"focused","args":{"Unit":"page","amount":1}}`, want: "unknown field"},
		{name: "duplicate arg", data: `{"type":"scroll","target":"focused","args":{"unit":"page","unit":"line","amount":1}}`, want: "duplicate field"},
		{name: "multiple args not array", data: `{"type":"multiple","target":"focused","args":{"actions":{}}}`, want: "JSON array"},
		{name: "multiple unknown arg", data: `{"type":"multiple","target":"focused","args":{"children":[]}}`, want: "unknown field"},
		{name: "multiple duplicate arg", data: `{"type":"multiple","target":"focused","args":{"actions":[],"actions":[]}}`, want: "duplicate field"},
		{name: "invalid target", data: `{"type":"copy_selection","target":"pane-4","args":{}}`, want: "target"},
		{name: "invalid scroll", data: `{"type":"scroll","target":"focused","args":{"unit":"page","amount":0}}`, want: "must not be zero"},
		{name: "invalid split", data: `{"type":"split_pane","target":"focused","args":{"axis":"diagonal"}}`, want: "axis"},
		{name: "callback", data: `{"type":"callback","target":"focused","args":{}}`, want: "not serializable"},
		{name: "invalid resize direction", data: `{"type":"resize_pane","target":"focused","args":{"direction":"next","delta":1}}`, want: "direction"},
		{name: "invalid resize delta", data: `{"type":"resize_pane","target":"focused","args":{"direction":"left","delta":0}}`, want: "delta"},
		{name: "oversized resize delta", data: `{"type":"resize_pane","target":"focused","args":{"direction":"left","delta":1025}}`, want: "delta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Unmarshal() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestCodecRejectsRuntimeCallback(t *testing.T) {
	_, err := Marshal(focused(Callback{BindingIndex: 3, Label: "custom"}))
	if !errors.Is(err, ErrNotSerializable) {
		t.Fatalf("Marshal(callback) error = %v, want ErrNotSerializable", err)
	}
}

func TestCodecBoundsNesting(t *testing.T) {
	envelope := focused(CopySelection{})
	encoded := `{"type":"copy_selection","target":"focused","args":{}}`
	for range MaxJSONDepth + 1 {
		envelope = focused(Multiple{actions: []Envelope{envelope}})
		encoded = fmt.Sprintf(`{"type":"multiple","target":"focused","args":{"actions":[%s]}}`, encoded)
	}
	if _, err := Marshal(envelope); err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("Marshal(deep) error = %v", err)
	}
	if _, err := Unmarshal([]byte(encoded)); err == nil || !strings.Contains(err.Error(), "maximum depth") {
		t.Fatalf("Unmarshal(deep) error = %v", err)
	}
}

func TestCodecBoundsInputBytesCardinalityAndNodes(t *testing.T) {
	if _, err := Unmarshal(bytes.Repeat([]byte{' '}, MaxJSONBytes+1)); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Fatalf("oversized JSON error = %v", err)
	}
	simple := `{"type":"copy_selection","target":"focused","args":{}}`
	tooMany := fmt.Sprintf(`{"type":"multiple","target":"focused","args":{"actions":[%s]}}`, strings.Join(makeCopies(simple, MaxSequenceActions+1), ","))
	if _, err := Unmarshal([]byte(tooMany)); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversized sequence error = %v", err)
	}
	group := fmt.Sprintf(`{"type":"multiple","target":"focused","args":{"actions":[%s]}}`, strings.Join(makeCopies(simple, MaxSequenceActions), ","))
	graph := fmt.Sprintf(`{"type":"multiple","target":"focused","args":{"actions":[%s]}}`, strings.Join(makeCopies(group, MaxSequenceActions), ","))
	if _, err := Unmarshal([]byte(graph)); err == nil || !strings.Contains(err.Error(), "maximum nodes") {
		t.Fatalf("oversized graph error = %v", err)
	}
}

func makeCopies(value string, count int) []string {
	values := make([]string, count)
	for i := range values {
		values[i] = value
	}
	return values
}

func TestCodecRequiresRegistry(t *testing.T) {
	_, err := NewCodec(nil)
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("NewCodec(nil) error = %v", err)
	}
}

func TestCodecRegistryIsAuthoritative(t *testing.T) {
	copyRegistration, _ := DefaultRegistry().lookupRegistration(IDCopySelection)
	registry, err := newRegistry(copyRegistration)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := NewCodec(registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.Marshal(focused(Scroll{Unit: ScrollLine, Amount: 1})); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("Marshal with limited registry error = %v", err)
	}
	data := []byte(`{"type":"scroll","target":"focused","args":{"unit":"line","amount":1}}`)
	if _, err := codec.Unmarshal(data); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("Unmarshal with limited registry error = %v", err)
	}
}
