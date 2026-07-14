package glfwgl

import (
	"testing"
	"time"
)

func TestNextWake(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	blinkPeriod := 1000 * time.Millisecond // half-period = 500ms

	tests := []struct {
		name        string
		now         time.Time
		blinkActive bool
		blinkStart  time.Time
		blinkPeriod time.Duration
		noticeUntil time.Time
		statsShown  bool
		timer       time.Time
		want        time.Duration
	}{
		{
			name:        "idle no blink no notice no stats caps at maxWake",
			now:         base,
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        500 * time.Millisecond,
		},
		{
			name:        "blink at phase boundary waits a full half-period",
			now:         base,
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        500 * time.Millisecond,
		},
		{
			name:        "blink mid-phase waits to the next boundary",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        200 * time.Millisecond,
		},
		{
			name:        "blink mid-phase in second half wraps the modulo",
			now:         base.Add(700 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        300 * time.Millisecond,
		},
		{
			name:        "notice sooner than blink boundary wins",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			noticeUntil: base.Add(400 * time.Millisecond), // 100ms out < 200ms blink
			want:        100 * time.Millisecond,
		},
		{
			name:        "expired notice does not shorten the wait",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			noticeUntil: base.Add(100 * time.Millisecond), // already past
			want:        500 * time.Millisecond,
		},
		{
			name:        "stats shown caps at 500ms",
			now:         base,
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			statsShown:  true,
			want:        500 * time.Millisecond,
		},
		{
			name:        "result clamps up to minWake",
			now:         base.Add(500 * time.Millisecond), // exactly on boundary from below
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			noticeUntil: base.Add(500*time.Millisecond + 100*time.Microsecond), // 100us out
			want:        1 * time.Millisecond,
		},
		{
			name:        "zero period guards against divide and ignores blink",
			now:         base.Add(123 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: 0,
			want:        500 * time.Millisecond,
		},
		{
			name:        "negative period guards and ignores blink",
			now:         base.Add(123 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: -1000 * time.Millisecond,
			want:        500 * time.Millisecond,
		},
		{
			name:        "blink inactive ignores blink even with valid period",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        500 * time.Millisecond,
		},
		{
			name:        "timer sooner than blink boundary wins",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,                    // 200ms to next blink boundary
			timer:       base.Add(350 * time.Millisecond), // 50ms out
			want:        50 * time.Millisecond,
		},
		{
			name:        "past-due timer clamps to minWake",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			timer:       base.Add(100 * time.Millisecond), // already past
			want:        1 * time.Millisecond,
		},
		{
			name:        "zero timer deadline leaves the wait unchanged",
			now:         base,
			blinkActive: false,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,
			want:        500 * time.Millisecond,
		},
		{
			name:        "timer later than blink boundary does not extend the wait",
			now:         base.Add(300 * time.Millisecond),
			blinkActive: true,
			blinkStart:  base,
			blinkPeriod: blinkPeriod,                    // 200ms to next blink boundary
			timer:       base.Add(900 * time.Millisecond), // 600ms out, ignored
			want:        200 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextWake(tc.now, tc.blinkActive, tc.blinkStart, tc.blinkPeriod, tc.noticeUntil, tc.statsShown, tc.timer)
			if got != tc.want {
				t.Fatalf("nextWake = %v, want %v", got, tc.want)
			}
			if got < minWake || got > maxWake {
				t.Fatalf("nextWake = %v out of [%v, %v]", got, minWake, maxWake)
			}
		})
	}
}
