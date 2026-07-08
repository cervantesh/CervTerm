package input

import "testing"

func TestEncodePrintableRune(t *testing.T) {
	got, ok := Encode(Event{Rune: 'é'})
	if !ok {
		t.Fatalf("expected printable rune to encode")
	}
	if string(got) != "é" {
		t.Fatalf("unexpected encoded rune: %q", string(got))
	}
}

func TestEncodeControlKeys(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{name: "enter", event: Event{Key: KeyEnter}, want: "\r"},
		{name: "backspace", event: Event{Key: KeyBackspace}, want: "\x7f"},
		{name: "tab", event: Event{Key: KeyTab}, want: "\t"},
		{name: "escape", event: Event{Key: KeyEscape}, want: "\x1b"},
		{name: "up", event: Event{Key: KeyUp}, want: "\x1b[A"},
		{name: "down", event: Event{Key: KeyDown}, want: "\x1b[B"},
		{name: "right", event: Event{Key: KeyRight}, want: "\x1b[C"},
		{name: "left", event: Event{Key: KeyLeft}, want: "\x1b[D"},
		{name: "home", event: Event{Key: KeyHome}, want: "\x1b[H"},
		{name: "end", event: Event{Key: KeyEnd}, want: "\x1b[F"},
		{name: "insert", event: Event{Key: KeyInsert}, want: "\x1b[2~"},
		{name: "delete", event: Event{Key: KeyDelete}, want: "\x1b[3~"},
		{name: "page up", event: Event{Key: KeyPageUp}, want: "\x1b[5~"},
		{name: "page down", event: Event{Key: KeyPageDown}, want: "\x1b[6~"},
		{name: "f1", event: Event{Key: KeyF1}, want: "\x1bOP"},
		{name: "f4", event: Event{Key: KeyF4}, want: "\x1bOS"},
		{name: "f5", event: Event{Key: KeyF5}, want: "\x1b[15~"},
		{name: "f12", event: Event{Key: KeyF12}, want: "\x1b[24~"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Encode(tt.event)
			if !ok {
				t.Fatalf("expected event to encode")
			}
			if string(got) != tt.want {
				t.Fatalf("want %q, got %q", tt.want, string(got))
			}
		})
	}
}

func TestEncodeModifiedKeys(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{name: "shift up", event: Event{Key: KeyUp, Mods: ModShift}, want: "\x1b[1;2A"},
		{name: "alt right", event: Event{Key: KeyRight, Mods: ModAlt}, want: "\x1b[1;3C"},
		{name: "ctrl left", event: Event{Key: KeyLeft, Mods: ModCtrl}, want: "\x1b[1;5D"},
		{name: "ctrl shift page up", event: Event{Key: KeyPageUp, Mods: ModCtrl | ModShift}, want: "\x1b[5;6~"},
		{name: "alt f4", event: Event{Key: KeyF4, Mods: ModAlt}, want: "\x1b[1;3S"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Encode(tt.event)
			if !ok || string(got) != tt.want {
				t.Fatalf("want %q ok=true, got %q ok=%v", tt.want, string(got), ok)
			}
		})
	}
}

func TestEncodeApplicationCursorMode(t *testing.T) {
	got, ok := EncodeWithMode(Event{Key: KeyUp}, Mode{ApplicationCursor: true})
	if !ok || string(got) != "\x1bOA" {
		t.Fatalf("application cursor up mismatch: %q ok=%v", string(got), ok)
	}
	got, ok = EncodeWithMode(Event{Key: KeyUp, Mods: ModCtrl}, Mode{ApplicationCursor: true})
	if !ok || string(got) != "\x1b[1;5A" {
		t.Fatalf("modified application cursor should use CSI, got %q ok=%v", string(got), ok)
	}
}

func TestEncodeCtrlAndAltLetters(t *testing.T) {
	got, ok := Encode(Event{Rune: 'c', Mods: ModCtrl})
	if !ok || string(got) != "\x03" {
		t.Fatalf("want ETX, got %q ok=%v", string(got), ok)
	}
	got, ok = Encode(Event{Rune: 'D', Mods: ModCtrl})
	if !ok || string(got) != "\x04" {
		t.Fatalf("want EOT, got %q ok=%v", string(got), ok)
	}
	got, ok = Encode(Event{Rune: 'x', Mods: ModAlt})
	if !ok || string(got) != "\x1bx" {
		t.Fatalf("want alt-prefixed x, got %q ok=%v", string(got), ok)
	}
}

func TestEncodeIgnoresUnsupportedEvents(t *testing.T) {
	if got, ok := Encode(Event{}); ok || got != nil {
		t.Fatalf("expected empty event to be ignored, got %q ok=%v", string(got), ok)
	}
	if got, ok := Encode(Event{Rune: 'v', Mods: ModCtrl}); ok || got != nil {
		t.Fatalf("ctrl+v is reserved for paste by frontend, got %q ok=%v", string(got), ok)
	}
}

func TestEncodePaste(t *testing.T) {
	if got := EncodePaste("hello", false); string(got) != "hello" {
		t.Fatalf("raw paste mismatch: %q", string(got))
	}
	got := EncodePaste("hello", true)
	want := "\x1b[200~hello\x1b[201~"
	if string(got) != want {
		t.Fatalf("bracketed paste mismatch: want %q got %q", want, string(got))
	}
}

func TestClipboardShortcut(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  ClipboardAction
	}{
		{name: "ctrl shift c copies", event: Event{Rune: 'c', Mods: ModCtrl | ModShift}, want: ClipboardCopy},
		{name: "ctrl shift v pastes", event: Event{Rune: 'v', Mods: ModCtrl | ModShift}, want: ClipboardPaste},
		{name: "uppercase c copies", event: Event{Rune: 'C', Mods: ModCtrl | ModShift}, want: ClipboardCopy},
		{name: "plain ctrl c is terminal", event: Event{Rune: 'c', Mods: ModCtrl}, want: ClipboardNone},
		{name: "plain ctrl v reserved elsewhere", event: Event{Rune: 'v', Mods: ModCtrl}, want: ClipboardNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClipboardShortcut(tt.event); got != tt.want {
				t.Fatalf("want %v got %v", tt.want, got)
			}
		})
	}
}
