package input

import "testing"

func TestEncodeSGRMouse(t *testing.T) {
	tests := []struct {
		name  string
		event MouseEvent
		want  string
	}{
		{name: "left press", event: MouseEvent{Button: MouseLeft, Action: MousePress, Row: 2, Col: 4, SGR: true}, want: "\x1b[<0;5;3M"},
		{name: "left release", event: MouseEvent{Button: MouseLeft, Action: MouseRelease, Row: 2, Col: 4, SGR: true}, want: "\x1b[<3;5;3m"},
		{name: "drag with ctrl shift", event: MouseEvent{Button: MouseLeft, Action: MouseMove, Row: 1, Col: 1, Mods: ModCtrl | ModShift, SGR: true}, want: "\x1b[<52;2;2M"},
		{name: "wheel up", event: MouseEvent{Button: MouseWheelUp, Action: MousePress, Row: 0, Col: 0, SGR: true}, want: "\x1b[<64;1;1M"},
		{name: "wheel down alt", event: MouseEvent{Button: MouseWheelDown, Action: MousePress, Row: 0, Col: 0, Mods: ModAlt, SGR: true}, want: "\x1b[<73;1;1M"},
		{name: "no-button motion", event: MouseEvent{Button: MouseNone, Action: MouseMove, Row: 2, Col: 4, SGR: true}, want: "\x1b[<35;5;3M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := EncodeMouse(tt.event)
			if !ok || string(got) != tt.want {
				t.Fatalf("want %q ok=true, got %q ok=%v", tt.want, string(got), ok)
			}
		})
	}
}

func TestEncodeMouseRejectsUnsupportedModes(t *testing.T) {
	if got, ok := EncodeMouse(MouseEvent{Button: MouseLeft, SGR: true, Row: -1}); ok || got != nil {
		t.Fatalf("negative coordinates should be rejected, got %q ok=%v", string(got), ok)
	}
}

func TestEncodeMouseLegacy(t *testing.T) {
	tests := []struct {
		name  string
		event MouseEvent
		want  string
	}{
		{"press", MouseEvent{Button: MouseLeft, Action: MousePress, Row: 0, Col: 0}, "\x1b[M !!"},
		{"release", MouseEvent{Button: MouseLeft, Action: MouseRelease, Row: 0, Col: 0}, "\x1b[M#!!"},
		{"wheel clamp", MouseEvent{Button: MouseWheelDown, Action: MousePress, Row: 300, Col: 300}, "\x1b[Ma\xfe\xfe"},
		{"motion", MouseEvent{Button: MouseLeft, Action: MouseMove, Row: 1, Col: 2, Mods: ModShift}, "\x1b[MD#\""},
		{"no-button motion", MouseEvent{Button: MouseNone, Action: MouseMove, Row: 1, Col: 2}, "\x1b[MC#\""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := EncodeMouse(tt.event)
			if !ok || string(got) != tt.want {
				t.Fatalf("want %q ok=true, got %q ok=%v", tt.want, string(got), ok)
			}
		})
	}
}
