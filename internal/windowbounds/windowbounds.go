package windowbounds

import (
	"errors"
	"math"
	"math/big"
	"sort"

	"cervterm/internal/layoutstate"
)

const MaxMonitors = 64

type Rect struct {
	X, Y, Width, Height int
}

type Monitor struct {
	ID, Name       string
	WorkArea       Rect
	ScaleX, ScaleY float64
	Primary        bool
}

type Policy struct {
	FallbackWidth, FallbackHeight int
	MinWidth, MinHeight           int
	ChromeHeight                  int
	MinVisibleChromeX             int
	MinVisibleChromeY             int
}

type Outcome string

const (
	Kept              Outcome = "kept"
	Clamped           Outcome = "clamped"
	FallbackInvalid   Outcome = "fallback_invalid"
	FallbackOffscreen Outcome = "fallback_offscreen"
)

type Plan struct {
	Bounds      layoutstate.Bounds
	MonitorID   string
	ScaleX      float64
	ScaleY      float64
	Outcome     Outcome
	HintMatched bool
}

type preparedMonitor struct {
	Monitor
	right, bottom int
}

type preparedRect struct {
	Rect
	right, bottom int
}

func Recover(saved layoutstate.Bounds, monitors []Monitor, policy Policy) (Plan, error) {
	prepared, err := validateMonitors(monitors)
	if err != nil {
		return Plan{}, err
	}
	if err := validatePolicy(policy); err != nil {
		return Plan{}, err
	}

	savedRect, valid := prepareRect(Rect{X: saved.X, Y: saved.Y, Width: saved.Width, Height: saved.Height})
	if !valid {
		selected := defaultMonitor(prepared)
		return fallback(selected, policy, FallbackInvalid, false), nil
	}

	selected, matched, intersects := selectMonitor(savedRect, saved.MonitorHint, prepared)
	if !matched && !intersects {
		return fallback(selected, policy, FallbackOffscreen, false), nil
	}

	bounds := clamp(saved, selected, policy)
	outcome := Clamped
	if bounds.X == saved.X && bounds.Y == saved.Y && bounds.Width == saved.Width && bounds.Height == saved.Height {
		outcome = Kept
	}
	return Plan{Bounds: bounds, MonitorID: selected.ID, ScaleX: selected.ScaleX, ScaleY: selected.ScaleY, Outcome: outcome, HintMatched: matched}, nil
}

func validateMonitors(monitors []Monitor) ([]preparedMonitor, error) {
	if len(monitors) == 0 || len(monitors) > MaxMonitors {
		return nil, errors.New("windowbounds: monitor count must be between 1 and 64")
	}
	out := make([]preparedMonitor, len(monitors))
	ids := make(map[string]struct{}, len(monitors))
	primaries := 0
	for i, monitor := range monitors {
		if monitor.ID == "" {
			return nil, errors.New("windowbounds: monitor ID is empty")
		}
		if _, exists := ids[monitor.ID]; exists {
			return nil, errors.New("windowbounds: duplicate monitor ID")
		}
		ids[monitor.ID] = struct{}{}
		area, ok := prepareRect(monitor.WorkArea)
		if !ok {
			return nil, errors.New("windowbounds: invalid monitor work area")
		}
		if !finitePositive(monitor.ScaleX) || !finitePositive(monitor.ScaleY) {
			return nil, errors.New("windowbounds: invalid monitor scale")
		}
		if monitor.Primary {
			primaries++
		}
		out[i] = preparedMonitor{Monitor: monitor, right: area.right, bottom: area.bottom}
	}
	if primaries > 1 {
		return nil, errors.New("windowbounds: multiple primary monitors")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func validatePolicy(policy Policy) error {
	values := []int{policy.FallbackWidth, policy.FallbackHeight, policy.MinWidth, policy.MinHeight, policy.ChromeHeight, policy.MinVisibleChromeX, policy.MinVisibleChromeY}
	for _, value := range values {
		if value <= 0 {
			return errors.New("windowbounds: policy values must be positive")
		}
	}
	if policy.MinVisibleChromeX > policy.MinWidth || policy.MinVisibleChromeY > policy.ChromeHeight || policy.ChromeHeight > policy.MinHeight {
		return errors.New("windowbounds: visible chrome policy exceeds minimum window bounds")
	}
	return nil
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func prepareRect(rect Rect) (preparedRect, bool) {
	if rect.Width <= 0 || rect.Height <= 0 {
		return preparedRect{}, false
	}
	right, ok := checkedAdd(rect.X, rect.Width)
	if !ok {
		return preparedRect{}, false
	}
	bottom, ok := checkedAdd(rect.Y, rect.Height)
	if !ok {
		return preparedRect{}, false
	}
	return preparedRect{Rect: rect, right: right, bottom: bottom}, true
}

func checkedAdd(a, b int) (int, bool) {
	if b > 0 && a > int(^uint(0)>>1)-b {
		return 0, false
	}
	if b < 0 && a < -int(^uint(0)>>1)-1-b {
		return 0, false
	}
	return a + b, true
}

func defaultMonitor(monitors []preparedMonitor) preparedMonitor {
	for _, monitor := range monitors {
		if monitor.Primary {
			return monitor
		}
	}
	return monitors[0]
}

func selectMonitor(saved preparedRect, hint string, monitors []preparedMonitor) (preparedMonitor, bool, bool) {
	if hint != "" {
		for _, monitor := range monitors {
			if monitor.ID == hint {
				return monitor, true, intersectionArea(saved, monitor).Sign() > 0
			}
		}
		nameIndex := -1
		for i, monitor := range monitors {
			if monitor.Name == hint {
				if nameIndex >= 0 {
					nameIndex = -2
					break
				}
				nameIndex = i
			}
		}
		if nameIndex >= 0 {
			monitor := monitors[nameIndex]
			return monitor, true, intersectionArea(saved, monitor).Sign() > 0
		}
	}

	bestArea := new(big.Int)
	candidates := make([]preparedMonitor, 0, len(monitors))
	for _, monitor := range monitors {
		area := intersectionArea(saved, monitor)
		comparison := area.Cmp(bestArea)
		if comparison > 0 {
			bestArea.Set(area)
			candidates = candidates[:0]
			candidates = append(candidates, monitor)
		} else if comparison == 0 && area.Sign() > 0 {
			candidates = append(candidates, monitor)
		}
	}
	if bestArea.Sign() > 0 {
		return closest(saved, candidates), false, true
	}
	return defaultMonitor(monitors), false, false
}

func intersectionArea(saved preparedRect, monitor preparedMonitor) *big.Int {
	left := max(saved.X, monitor.WorkArea.X)
	top := max(saved.Y, monitor.WorkArea.Y)
	right := min(saved.right, monitor.right)
	bottom := min(saved.bottom, monitor.bottom)
	if right <= left || bottom <= top {
		return new(big.Int)
	}
	width := new(big.Int).Sub(big.NewInt(int64(right)), big.NewInt(int64(left)))
	height := new(big.Int).Sub(big.NewInt(int64(bottom)), big.NewInt(int64(top)))
	return width.Mul(width, height)
}

func closest(saved preparedRect, monitors []preparedMonitor) preparedMonitor {
	best := monitors[0]
	bestDistance := centerDistance(saved, best)
	for _, monitor := range monitors[1:] {
		distance := centerDistance(saved, monitor)
		comparison := distance.Cmp(bestDistance)
		if comparison < 0 || (comparison == 0 && monitor.Primary && !best.Primary) || (comparison == 0 && monitor.Primary == best.Primary && monitor.ID < best.ID) {
			best, bestDistance = monitor, distance
		}
	}
	return best
}

func centerDistance(saved preparedRect, monitor preparedMonitor) *big.Int {
	sx := twiceCenter(saved.X, saved.Width)
	sy := twiceCenter(saved.Y, saved.Height)
	mx := twiceCenter(monitor.WorkArea.X, monitor.WorkArea.Width)
	my := twiceCenter(monitor.WorkArea.Y, monitor.WorkArea.Height)
	dx := new(big.Int).Sub(sx, mx)
	dy := new(big.Int).Sub(sy, my)
	dx.Mul(dx, dx)
	dy.Mul(dy, dy)
	return dx.Add(dx, dy)
}

func twiceCenter(origin, size int) *big.Int {
	value := new(big.Int).SetInt64(int64(origin))
	value.Lsh(value, 1)
	return value.Add(value, new(big.Int).SetInt64(int64(size)))
}

func fallback(monitor preparedMonitor, policy Policy, outcome Outcome, matched bool) Plan {
	width := min(max(policy.FallbackWidth, policy.MinWidth), monitor.WorkArea.Width)
	height := min(max(policy.FallbackHeight, policy.MinHeight), monitor.WorkArea.Height)
	x := monitor.WorkArea.X + (monitor.WorkArea.Width-width)/2
	y := monitor.WorkArea.Y + (monitor.WorkArea.Height-height)/2
	bounds := layoutstate.Bounds{X: x, Y: y, Width: width, Height: height, MonitorHint: monitor.ID}
	return Plan{Bounds: bounds, MonitorID: monitor.ID, ScaleX: monitor.ScaleX, ScaleY: monitor.ScaleY, Outcome: outcome, HintMatched: matched}
}

func saturatedAdd(value, delta int) int {
	result, ok := checkedAdd(value, delta)
	if ok {
		return result
	}
	if delta < 0 {
		return -int(^uint(0)>>1) - 1
	}
	return int(^uint(0) >> 1)
}

func clamp(saved layoutstate.Bounds, monitor preparedMonitor, policy Policy) layoutstate.Bounds {
	width := min(max(saved.Width, policy.MinWidth), monitor.WorkArea.Width)
	height := min(max(saved.Height, policy.MinHeight), monitor.WorkArea.Height)

	visibleX := min(policy.MinVisibleChromeX, width, monitor.WorkArea.Width)
	minX := saturatedAdd(monitor.WorkArea.X, visibleX-width)
	maxX := monitor.right - visibleX
	chromeHeight := min(policy.ChromeHeight, height)
	visibleY := min(policy.MinVisibleChromeY, chromeHeight, monitor.WorkArea.Height)
	minY := saturatedAdd(monitor.WorkArea.Y, visibleY-chromeHeight)
	maxY := monitor.bottom - visibleY

	return layoutstate.Bounds{
		X:           min(max(saved.X, minX), maxX),
		Y:           min(max(saved.Y, minY), maxY),
		Width:       width,
		Height:      height,
		MonitorHint: monitor.ID,
	}
}
