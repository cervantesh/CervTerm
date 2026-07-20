//go:build glfw

package glfwgl

import (
	"testing"
	"time"

	termaction "cervterm/internal/action"
	"cervterm/internal/script"
)

func keySpec(t *testing.T, key, mods string) script.Spec {
	t.Helper()
	spec, err := script.ParseSpec(key, mods)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func testKeyTableSet(t *testing.T) script.BindingSet {
	t.Helper()
	leader := keySpec(t, "a", "ctrl")
	return script.BindingSet{
		Leader: &script.Leader{Spec: leader, TimeoutMS: 100},
		Root: []script.Binding{
			{Spec: keySpec(t, "p", ""), ToTable: "pane"},
			{Spec: keySpec(t, "x", ""), Action: actionEnvelope(termaction.ToggleStats{})},
		},
		Tables: []script.KeyTable{
			{Name: "next", OneShot: true, TimeoutMS: 300, Bindings: []script.Binding{{Spec: keySpec(t, "z", ""), Action: actionEnvelope(termaction.ToggleStats{})}}},
			{Name: "pane", OneShot: false, TimeoutMS: 200, Bindings: []script.Binding{
				{Spec: keySpec(t, "h", ""), Action: actionEnvelope(termaction.ToggleStats{})},
				{Spec: keySpec(t, "n", ""), ToTable: "next"},
			}},
		},
	}
}

func TestKeyTableStateLeaderRepeatTimeoutMismatchAndEscape(t *testing.T) {
	set := testKeyTableSet(t)
	now := time.Unix(100, 0)
	var state keyTableState
	leader := set.Leader.Spec
	if got := state.step(set, leader, false, now, 7); !got.consume || state.mode != keyTableLeader || state.origin != 7 {
		t.Fatalf("leader result=%#v state=%#v", got, state)
	}
	deadline := state.deadline
	if got := state.step(set, leader, true, now.Add(10*time.Millisecond), 9); !got.consume || state.deadline != deadline || state.origin != 7 {
		t.Fatalf("leader repeat result=%#v state=%#v", got, state)
	}
	if got := state.step(set, keySpec(t, "q", ""), false, now.Add(20*time.Millisecond), 9); !got.consume || state.mode != keyTableInactive {
		t.Fatalf("mismatch result=%#v state=%#v", got, state)
	}
	state.step(set, leader, false, now, 7)
	if got := state.step(set, keySpec(t, "escape", ""), false, now.Add(time.Millisecond), 7); !got.consume || state.mode != keyTableInactive {
		t.Fatalf("escape result=%#v state=%#v", got, state)
	}
	state.step(set, leader, false, now, 7)
	if got := state.step(set, keySpec(t, "x", ""), false, now.Add(101*time.Millisecond), 7); got.consume || state.mode != keyTableInactive {
		t.Fatalf("expired result=%#v state=%#v", got, state)
	}
}

func TestKeyTableStateTransitionsPersistentRefreshAndOneShot(t *testing.T) {
	set := testKeyTableSet(t)
	now := time.Unix(200, 0)
	var state keyTableState
	state.step(set, set.Leader.Spec, false, now, 11)
	if got := state.step(set, keySpec(t, "p", ""), false, now.Add(time.Millisecond), 99); !got.consume || got.binding != nil || state.mode != keyTableNamed || state.table != "pane" || state.origin != 11 {
		t.Fatalf("root transition result=%#v state=%#v", got, state)
	}
	firstDeadline := state.deadline
	got := state.step(set, keySpec(t, "h", ""), false, now.Add(50*time.Millisecond), 99)
	if !got.consume || got.binding == nil || got.origin != 11 || state.mode != keyTableNamed || !state.deadline.After(firstDeadline) {
		t.Fatalf("persistent action result=%#v state=%#v", got, state)
	}
	if got := state.step(set, keySpec(t, "n", ""), false, now.Add(60*time.Millisecond), 99); !got.consume || state.table != "next" {
		t.Fatalf("nested transition result=%#v state=%#v", got, state)
	}
	if got := state.step(set, keySpec(t, "z", ""), false, now.Add(70*time.Millisecond), 99); got.binding == nil || got.origin != 11 || state.mode != keyTableInactive {
		t.Fatalf("one-shot action result=%#v state=%#v", got, state)
	}
}

func TestKeyTableStateCancelIsIdempotent(t *testing.T) {
	state := keyTableState{mode: keyTableNamed, table: "pane", deadline: time.Now(), origin: 12}
	state.cancel()
	state.cancel()
	if state != (keyTableState{}) {
		t.Fatalf("cancelled state=%#v", state)
	}
}
