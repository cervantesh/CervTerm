package script

import (
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// timerEntry is one scheduled Lua callback. period == 0 marks a one-shot
// (after); a positive period marks a repeating timer (every). errNoted dedupes
// the watchdog-failure notice so a repeatedly-failing every-handler shows its
// error once, not once per tick, until it next succeeds.
type timerEntry struct {
	id       int
	deadline time.Time
	period   time.Duration
	fn       *lua.LFunction
	errNoted bool
}

// timerTable is the runtime's main-thread-only timer store. It is created before
// the Lua state so the cervterm module's after/every/cancel closures can share
// it, then handed to the Runtime. No locking: every mutation (module functions,
// FireDueTimers) happens on the loop thread.
type timerTable struct {
	entries []*timerEntry
	nextID  int
}

func (tt *timerTable) add(deadline time.Time, period time.Duration, fn *lua.LFunction) int {
	tt.nextID++
	tt.entries = append(tt.entries, &timerEntry{id: tt.nextID, deadline: deadline, period: period, fn: fn})
	return tt.nextID
}

func (tt *timerTable) find(id int) *timerEntry {
	for _, e := range tt.entries {
		if e.id == id {
			return e
		}
	}
	return nil
}

func (tt *timerTable) cancel(id int) {
	for i, e := range tt.entries {
		if e.id == id {
			tt.entries = append(tt.entries[:i], tt.entries[i+1:]...)
			return
		}
	}
}

// dueIDs returns the ids of every timer whose deadline is at or before now, in
// insertion order. Collecting ids up front (rather than iterating entries while
// firing) lets a handler cancel or add timers mid-pass without disturbing the
// set already selected for this iteration.
func (tt *timerTable) dueIDs(now time.Time) []int {
	var ids []int
	for _, e := range tt.entries {
		if !e.deadline.After(now) {
			ids = append(ids, e.id)
		}
	}
	return ids
}

func (tt *timerTable) nextDeadline() (time.Time, bool) {
	var best time.Time
	found := false
	for _, e := range tt.entries {
		if !found || e.deadline.Before(best) {
			best, found = e.deadline, true
		}
	}
	return best, found
}

// buildModule returns the cervterm module table exposing after/every/cancel/status.
// Called from PreloadModule during Load; the closures capture the shared
// timer and status tables, which the Runtime then owns.
func buildModule(state *lua.LState, tt *timerTable, statuses *statusTable, overlays *overlayStore) *lua.LTable {
	mod := state.NewTable()
	mod.RawSetString("after", state.NewFunction(func(l *lua.LState) int {
		ms := l.CheckInt(1)
		fn := l.CheckFunction(2)
		id := tt.add(time.Now().Add(time.Duration(ms)*time.Millisecond), 0, fn)
		l.Push(lua.LNumber(id))
		return 1
	}))
	mod.RawSetString("every", state.NewFunction(func(l *lua.LState) int {
		ms := l.CheckInt(1)
		fn := l.CheckFunction(2)
		period := time.Duration(ms) * time.Millisecond
		id := tt.add(time.Now().Add(period), period, fn)
		l.Push(lua.LNumber(id))
		return 1
	}))
	mod.RawSetString("cancel", state.NewFunction(func(l *lua.LState) int {
		tt.cancel(l.CheckInt(1))
		return 0
	}))
	mod.RawSetString("status", state.NewFunction(func(l *lua.LState) int {
		statuses.set(l.CheckString(1), l.CheckString(2))
		return 0
	}))
	mod.RawSetString("overlay", state.NewFunction(func(l *lua.LState) int {
		ov := overlays.get(l.CheckString(1))
		if ov.handle == nil {
			ov.handle = newOverlayHandle(l, overlays, ov)
		}
		l.Push(ov.handle)
		return 1
	}))
	return mod
}

// NextTimerDeadline reports the soonest timer deadline, or false when no timers
// are scheduled. The loop feeds this into nextWake so a timer bounds the event
// wait. Because timers only mutate on this same thread, a timer added inside a
// handler is already in the table by the time the loop recomputes the wait — no
// cross-thread wake (PostEmptyEvent) is needed (trap 1).
func (r *Runtime) NextTimerDeadline() (time.Time, bool) {
	if r.timers == nil {
		return time.Time{}, false
	}
	return r.timers.nextDeadline()
}

// FireDueTimers runs every timer due at now under the shared watchdog. Called
// from the main loop after processTermEvents, never from inside draw().
//
// A repeating timer reschedules from now (deadlines do not accumulate drift); a
// one-shot is removed before it fires. Both happen before the call so a handler
// that cancels its own id mid-fire wins. Due ids are collected up front and
// re-checked before each fire, so a handler cancelling a later-due sibling stops
// it from running this pass.
func (r *Runtime) FireDueTimers(now time.Time, host Host) {
	if r.timers == nil {
		return
	}
	for _, id := range r.timers.dueIDs(now) {
		entry := r.timers.find(id)
		if entry == nil {
			continue // cancelled by an earlier handler in this pass
		}
		fn := entry.fn
		if entry.period > 0 {
			entry.deadline = now.Add(entry.period)
		} else {
			r.timers.cancel(id)
		}
		err := r.callProtected(fmt.Sprintf("timer #%d", id), fn, host)
		entry = r.timers.find(id) // may be gone: one-shot, or self-cancelled repeater
		switch {
		case err == nil:
			if entry != nil {
				entry.errNoted = false
			}
		case entry != nil:
			// Repeating handler failed; keep the schedule but show the notice once
			// until it next succeeds so a broken every-timer does not spam (trap 2).
			if !entry.errNoted {
				entry.errNoted = true
				host.Notify("script error: " + err.Error())
			}
		default:
			host.Notify("script error: " + err.Error())
		}
	}
}
