package modal

import "testing"

func entries(labels ...string) []Entry {
	out := make([]Entry, len(labels))
	for i, label := range labels {
		out[i] = Entry{ID: label, Label: label}
	}
	return out
}

func TestCoordinatorOpenFilterNavigateCloseAndReplace(t *testing.T) {
	var c Coordinator
	if c.Open(ModeNone, 1, 2, entries("bad")) || c.Open(ModeCommandPalette, 1, 2, nil) {
		t.Fatal("invalid mode and empty modal must not open")
	}
	if !c.Open(ModeCommandPalette, 7, 9, entries("Alpha", "Beta", "alphabet")) {
		t.Fatal("open failed")
	}
	for _, r := range []rune("ALP") {
		c.AppendRune(r)
	}
	s := c.Snapshot()
	if got := len(s.Filtered); got != 2 || s.Filtered[0] != 0 || s.Filtered[1] != 2 {
		t.Fatalf("stable rune filtering = %v", s.Filtered)
	}
	c.Move(100)
	if got := c.Accept(); len(got) != 1 || got[0].Entry.ID != "alphabet" || got[0].Pane != 7 {
		t.Fatalf("accept = %+v", got)
	}
	if !c.Replace(ModeLaunchMenu, entries("one")) {
		t.Fatal("replace failed")
	}
	if s := c.Snapshot(); s.Mode != ModeLaunchMenu || s.OpeningPane != 7 || s.OpeningFocus != 9 || s.Revision <= 1 {
		t.Fatalf("replacement state = %+v", s)
	}
	intents := c.Close()
	if len(intents) != 2 || intents[1].Kind != IntentRestoreFocus || intents[1].Pane != 7 || intents[1].Focus != 9 {
		t.Fatalf("close intents = %+v", intents)
	}
}

func TestCoordinatorBoundsAndRetainedError(t *testing.T) {
	var c Coordinator
	tooMany := make([]Entry, MaxEntries+1)
	if c.Open(ModeQuickSelect, 1, 1, tooMany) {
		t.Fatal("over-cap entries opened")
	}
	if !c.Open(ModeQuickSelect, 1, 1, entries("x", "y")) {
		t.Fatal("open failed")
	}
	for i := 0; i < MaxQueryRunes+10; i++ {
		c.AppendRune('x')
	}
	c.SetError(string(make([]rune, MaxErrorRunes+10)))
	s := c.Snapshot()
	if len(s.Query) != MaxQueryRunes || len([]rune(s.Error)) != MaxErrorRunes {
		t.Fatalf("caps query=%d error=%d", len(s.Query), len([]rune(s.Error)))
	}
	rev := s.Revision
	c.SetError(s.Error)
	if c.Snapshot().Revision != rev {
		t.Fatal("unchanged error changed revision")
	}
}

func TestCoordinatorActivationAndWholeTextAreStableAndAtomic(t *testing.T) {
	var c Coordinator
	if !c.Open(ModeCommandPalette, 7, 9, entries("日本語", "other")) {
		t.Fatal("open failed")
	}
	activation := c.Activation()
	revision := c.Revision()
	if activation == 0 || !c.AppendText(activation, "日本") {
		t.Fatalf("activation=%d append failed", activation)
	}
	if c.Activation() != activation || c.Revision() != revision+1 || string(c.Snapshot().Query) != "日本" {
		t.Fatalf("state=%#v", c.Snapshot())
	}
	before := c.Snapshot()
	if c.AppendText(activation+1, "stale") || c.AppendText(activation, "bad\ntext") || c.AppendText(activation, string(make([]rune, MaxQueryRunes))) {
		t.Fatal("invalid whole-text append succeeded")
	}
	if after := c.Snapshot(); after.Revision != before.Revision || string(after.Query) != string(before.Query) {
		t.Fatalf("invalid append mutated state: before=%#v after=%#v", before, after)
	}
	if !c.Replace(ModeLaunchMenu, entries("new")) || c.Activation() == activation {
		t.Fatalf("replace did not create a new activation: %#v", c.Snapshot())
	}
	if c.AppendText(activation, "old") {
		t.Fatal("replaced activation accepted text")
	}
}

func TestCoordinatorActivationExhaustionFailsClosed(t *testing.T) {
	c := Coordinator{nextActivation: maxActivationID}
	if c.Open(ModeCommandPalette, 1, 1, entries("x")) || c.Active() {
		t.Fatalf("exhausted activation opened: %#v", c.Snapshot())
	}
}
