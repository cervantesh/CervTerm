package glfwgl

import "time"

// Wake bounds. maxWake doubles as the idle heartbeat and the self-heal cap for
// a lost PostEmptyEvent; minWake stops a tight blink from busy-looping.
const (
	minWake = 1 * time.Millisecond
	maxWake = 500 * time.Millisecond
)

// nextWake returns how long the event wait may sleep before a time-driven
// redraw or timer is due. The time-driven wakeups are cursor blink, transient
// notice expiry, the stats HUD refresh, and the next script timer deadline;
// everything else wakes the loop via an OS event or PostEmptyEvent. The result
// is clamped to [minWake, maxWake] so a missed wake self-heals within maxWake
// and a tight blink never spins.
//
// timerDeadline is the soonest script timer deadline (zero when there are no
// timers, which leaves the wait unchanged — no timers cost nothing). A past-due
// deadline yields a negative remainder that the final clamp pins to minWake, so
// an overdue timer fires on the very next iteration.
//
// This function is pure (no glfw/gl state) so the default `go test ./...` suite
// covers the boundary math.
func nextWake(now time.Time, blinkActive bool, blinkStart time.Time,
	blinkPeriod time.Duration, noticeUntil time.Time, statsShown bool,
	timerDeadline time.Time) time.Duration {
	wake := maxWake

	// Blink flips at every half-period boundary. Compute the time to the next
	// boundary from (now - blinkStart) modulo the half-period rather than
	// accumulating deltas, so the phase never drifts.
	if blinkActive && blinkPeriod > 0 {
		if half := blinkPeriod / 2; half > 0 {
			rem := (now.Sub(blinkStart)) % half
			if rem < 0 {
				rem += half
			}
			if until := half - rem; until < wake {
				wake = until
			}
		}
	}

	// A pending notice must clear on schedule even with no input.
	if noticeUntil.After(now) {
		if until := noticeUntil.Sub(now); until < wake {
			wake = until
		}
	}

	// The stats HUD refreshes on the FPS window; cap the sleep so its numbers
	// keep ticking. (maxWake already equals this window, but keep it explicit so
	// the intent survives a future change to maxWake.)
	if statsShown && wake > maxWake {
		wake = maxWake
	}

	// A due script timer bounds the sleep exactly like blink/notice do. A zero
	// deadline means no timers are scheduled, so the wait is untouched.
	if !timerDeadline.IsZero() {
		if until := timerDeadline.Sub(now); until < wake {
			wake = until
		}
	}

	if wake < minWake {
		wake = minWake
	}
	if wake > maxWake {
		wake = maxWake
	}
	return wake
}
