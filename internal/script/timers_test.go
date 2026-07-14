package script

import (
	"strings"
	"testing"
	"time"

	"cervterm/internal/config"
)

func TestTimerAfterFiresOnceAndRemovesItself(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  keys = { { key = "a", action = function(term)
    cervterm.after(10, function(term) term:notify("fired") end)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if _, ok := rt.NextTimerDeadline(); !ok {
		t.Fatal("after should schedule a timer")
	}
	// Not yet due.
	rt.FireDueTimers(time.Now(), host)
	if len(host.notices) != 0 {
		t.Fatalf("timer fired early: %#v", host.notices)
	}
	// Past the deadline: fires once and removes itself.
	rt.FireDueTimers(time.Now().Add(time.Second), host)
	if strings.Join(host.notices, "|") != "fired" {
		t.Fatalf("notices = %#v", host.notices)
	}
	if _, ok := rt.NextTimerDeadline(); ok {
		t.Fatal("one-shot timer should be gone after firing")
	}
}

func TestTimerEveryReschedulesFromNow(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  keys = { { key = "a", action = function(term)
    cervterm.every(100, function(term) term:notify("tick") end)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	base := time.Now().Add(time.Second)
	rt.FireDueTimers(base, host)
	deadline, ok := rt.NextTimerDeadline()
	if !ok {
		t.Fatal("every timer should still be scheduled")
	}
	if got := deadline.Sub(base); got != 100*time.Millisecond {
		t.Fatalf("reschedule delta = %v, want 100ms", got)
	}
	// Fires again next time it is due.
	rt.FireDueTimers(deadline, host)
	if strings.Join(host.notices, "|") != "tick|tick" {
		t.Fatalf("notices = %#v", host.notices)
	}
}

func TestTimerCancel(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  keys = { { key = "a", action = function(term)
    local id = cervterm.every(10, function(term) term:notify("tick") end)
    cervterm.cancel(id)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if _, ok := rt.NextTimerDeadline(); ok {
		t.Fatal("cancel should remove the timer")
	}
	rt.FireDueTimers(time.Now().Add(time.Second), host)
	if len(host.notices) != 0 {
		t.Fatalf("cancelled timer fired: %#v", host.notices)
	}
}

func TestTimerSelfCancelMidFire(t *testing.T) {
	// A repeating handler cancels its own id: it fires this pass, then is gone.
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
local id
return {
  keys = { { key = "a", action = function(term)
    id = cervterm.every(10, function(term)
      term:notify("once")
      cervterm.cancel(id)
    end)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	rt.FireDueTimers(time.Now().Add(time.Second), host)
	rt.FireDueTimers(time.Now().Add(2*time.Second), host)
	if strings.Join(host.notices, "|") != "once" {
		t.Fatalf("self-cancelling timer notices = %#v", host.notices)
	}
	if _, ok := rt.NextTimerDeadline(); ok {
		t.Fatal("self-cancelled timer should be gone")
	}
}

func TestTimerDueOrderingByDeadline(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
return {
  keys = { { key = "a", action = function(term)
    cervterm.after(200, function(term) term:notify("late") end)
    cervterm.after(50, function(term) term:notify("early") end)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// The soonest deadline is the 50ms timer, not the 200ms one added first.
	deadline, ok := rt.NextTimerDeadline()
	if !ok {
		t.Fatal("expected a scheduled timer")
	}
	for _, e := range rt.timers.entries {
		if deadline.After(e.deadline) {
			t.Fatal("NextTimerDeadline must return the soonest deadline")
		}
	}
	// Firing well past both fires both; collection is in insertion order.
	rt.FireDueTimers(time.Now().Add(time.Second), host)
	if strings.Join(host.notices, "|") != "late|early" {
		t.Fatalf("notices = %#v", host.notices)
	}
}

func TestTimerEveryWatchdogNoticeDedupedUntilSuccess(t *testing.T) {
	path := writeScriptConfig(t, `local cervterm = require("cervterm")
fail = true
return {
  keys = { { key = "a", action = function(term)
    cervterm.every(10, function(term)
      if fail then error("boom") end
      term:notify("ok")
    end)
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	// Repeated failures notify once, not once per tick.
	for i := 0; i < 3; i++ {
		rt.FireDueTimers(time.Now().Add(time.Duration(i+1)*time.Second), host)
	}
	if len(host.notices) != 1 || !strings.Contains(host.notices[0], "boom") {
		t.Fatalf("expected one deduped error notice, got %#v", host.notices)
	}
	// Flip to success, then fail again: a fresh failure notifies once more.
	rt.state.DoString("fail = false")
	rt.FireDueTimers(time.Now().Add(10*time.Second), host)
	rt.state.DoString("fail = true")
	rt.FireDueTimers(time.Now().Add(20*time.Second), host)
	if got := strings.Join(host.notices, "|"); strings.Count(got, "boom") != 2 || !strings.Contains(got, "ok") {
		t.Fatalf("notices after recovery = %#v", host.notices)
	}
}

func TestSetFontSizeClampedAtLuaBoundary(t *testing.T) {
	path := writeScriptConfig(t, `return {
  keys = { { key = "z", action = function(term)
    term:set_font_size(2)     -- below min -> 6
    term:set_font_size(100)   -- above max -> 72
    term:set_font_size(14)    -- in range
    term:notify(string.format("%.0f", term:font_size()))
  end } },
}`)
	_, rt, err := Load(path, config.Defaults())
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer rt.Close()
	host := &fakeHost{fontSize: 10}
	if err := rt.Dispatch(0, host); err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	want := []float64{6, 72, 14}
	if len(host.fontSizes) != len(want) {
		t.Fatalf("recorded font sizes = %#v", host.fontSizes)
	}
	for i, w := range want {
		if host.fontSizes[i] != w {
			t.Fatalf("font size[%d] = %v, want %v", i, host.fontSizes[i], w)
		}
	}
	if strings.Join(host.notices, "") != "14" {
		t.Fatalf("font_size readback = %#v", host.notices)
	}
}
