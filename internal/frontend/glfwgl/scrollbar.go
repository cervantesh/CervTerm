//go:build glfw

package glfwgl

import (
	"image/color"
	"math"
	"time"

	"cervterm/internal/config"

	"github.com/go-gl/glfw/v3.3/glfw"
)

type rectF struct{ x, y, w, h float32 }

func (r rectF) contains(x, y float32) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

type scrollbarGeometry struct {
	slot        rectF
	track       rectF
	thumb       rectF
	history     int
	offset      int
	visibleRows int
}

func calculateScrollbarGeometry(frameWidth, frameHeight, paddingY, cellHeight, scale float32, cfg config.ScrollbarConfig, visibleRows, history, offset int) scrollbarGeometry {
	if scale <= 0 {
		scale = 1
	}
	reserved := float32(cfg.ReservedWidthPX) * scale
	margin := float32(cfg.MarginPX) * scale
	width := float32(cfg.WidthPX) * scale
	g := scrollbarGeometry{
		slot:    rectF{x: max(float32(0), frameWidth-reserved), y: 0, w: max(float32(0), reserved), h: max(float32(0), frameHeight)},
		history: max(0, history), offset: max(0, min(offset, history)), visibleRows: max(1, visibleRows),
	}
	trackHeight := min(max(float32(0), float32(visibleRows)*cellHeight), max(float32(0), frameHeight-paddingY))
	g.track = rectF{x: g.slot.x + margin, y: max(float32(0), paddingY), w: min(width, max(float32(0), g.slot.w-2*margin)), h: trackHeight}
	if g.history == 0 || g.track.w <= 0 || g.track.h <= 0 {
		return g
	}
	totalRows := float32(g.history + g.visibleRows)
	thumbHeight := g.track.h * float32(g.visibleRows) / totalRows
	thumbHeight = max(thumbHeight, float32(cfg.MinThumbPX)*scale)
	thumbHeight = min(thumbHeight, g.track.h)
	travel := max(float32(0), g.track.h-thumbHeight)
	position := float32(1)
	if g.history > 0 {
		position = float32(g.history-g.offset) / float32(g.history)
	}
	g.thumb = rectF{x: g.track.x, y: g.track.y + travel*position, w: g.track.w, h: thumbHeight}
	return g
}

func scrollbarOffsetForThumb(g scrollbarGeometry, thumbTop float32) int {
	if g.history <= 0 {
		return 0
	}
	travel := g.track.h - g.thumb.h
	if travel <= 0 {
		return 0
	}
	ratio := (thumbTop - g.track.y) / travel
	ratio = max(float32(0), min(float32(1), ratio))
	return int(math.Round(float64((1 - ratio) * float32(g.history))))
}

type pointerOwner uint8

const (
	pointerOwnerNone pointerOwner = iota
	pointerOwnerTerminal
	pointerOwnerScrollbar
)

type scrollbarState struct {
	hovered            bool
	pressed            bool
	dragging           bool
	dragGrabOffset     float32
	owner              pointerOwner
	lastActivity       time.Time
	lastPaintedOpacity float32
}

func scrollbarMode(cfg config.ScrollbarConfig) string {
	if cfg.Mode != "" {
		return cfg.Mode
	}
	if cfg.Enabled {
		return "scrolling"
	}
	return "never"
}

func scrollbarEnabled(cfg config.ScrollbarConfig) bool {
	return cfg.Enabled && scrollbarMode(cfg) != "never"
}

func scrollbarGutterWidth(cfg config.ScrollbarConfig, scale float32) float32 {
	stable := cfg.StableGutter
	if cfg.Mode == "" {
		stable = true
	}
	if !scrollbarEnabled(cfg) || !stable {
		return 0
	}
	return float32(cfg.ReservedWidthPX) * max(float32(1), scale)
}

func (a *App) scrollbarReservedWidth() float32 {
	return scrollbarGutterWidth(a.cfg.Scrollbar, a.uiScale)
}

func paneScrollbarGeometry(frameWidth int, paneY, paneHeight int, paddingY, cellHeight, scale float32, cfg config.ScrollbarConfig, visibleRows, history, offset int) scrollbarGeometry {
	g := calculateScrollbarGeometry(float32(frameWidth), float32(paneHeight), paddingY, cellHeight, scale, cfg, visibleRows, history, offset)
	yOffset := float32(paneY)
	g.slot.y += yOffset
	g.track.y += yOffset
	g.thumb.y += yOffset
	return g
}

func (a *App) currentScrollbarGeometry() scrollbarGeometry {
	if a.window == nil || !scrollbarEnabled(a.cfg.Scrollbar) {
		return scrollbarGeometry{}
	}
	_, view, ok := a.focusedView()
	if !ok {
		return scrollbarGeometry{}
	}
	w, h := a.window.GetFramebufferSize()
	track := a.windowGeometry(w, h).ScrollbarTrack
	pane := view.Geometry.Pixels
	return paneScrollbarGeometry(track.X+track.Width, pane.Y, pane.Height, 0, a.cellH, a.uiScale, a.cfg.Scrollbar, view.Snapshot.Rows, view.ScrollbackLines, view.DisplayOffset)
}

func (a *App) scrollbarGeometryForSnapshot(w, h int) scrollbarGeometry {
	if !scrollbarEnabled(a.cfg.Scrollbar) {
		return scrollbarGeometry{}
	}
	_, view, ok := a.focusedView()
	if !ok {
		return scrollbarGeometry{}
	}
	track := a.windowGeometry(w, h).ScrollbarTrack
	pane := view.Geometry.Pixels
	return paneScrollbarGeometry(track.X+track.Width, pane.Y, pane.Height, 0, a.cellH, a.uiScale, a.cfg.Scrollbar, a.snap.Rows, a.snap.HistoryRows, a.snap.DisplayOffset)
}

func (a *App) scrollViewport(lines int) bool {
	moved, err := a.mux.ScrollViewport(a.focusedPane, lines)
	if err != nil {
		return false
	}
	if moved {
		a.scrollbar.lastActivity = time.Now()
		a.requestAccessibilityRedraw()
		a.markScrollEvent()
	}
	return moved
}

func (a *App) setViewportOffset(target int) bool {
	_, view, ok := a.focusedView()
	if !ok {
		return false
	}
	return a.scrollViewport(target - view.DisplayOffset)
}

func (a *App) handleScrollbarButton(button glfw.MouseButton, action glfw.Action, x, y float32) bool {
	if !scrollbarEnabled(a.cfg.Scrollbar) {
		return false
	}
	g := a.currentScrollbarGeometry()
	if action == glfw.Press {
		if !g.slot.contains(x, y) {
			a.scrollbar.owner = pointerOwnerTerminal
			return false
		}
		a.scrollbar.owner = pointerOwnerScrollbar
		a.scrollbar.hovered = true
		a.scrollbar.lastActivity = time.Now()
		if button == glfw.MouseButtonLeft && g.history > 0 {
			a.scrollbar.pressed = true
			if g.thumb.contains(x, y) {
				a.scrollbar.dragging = true
				a.scrollbar.dragGrabOffset = y - g.thumb.y
			} else if g.track.contains(x, y) {
				if a.cfg.Scrollbar.TrackClick == "jump" {
					a.setViewportOffset(scrollbarOffsetForThumb(g, y-g.thumb.h/2))
				} else {
					page := max(1, int(float64(g.visibleRows)*a.cfg.Scrollbar.PageStep))
					if y < g.thumb.y {
						a.scrollViewport(page)
					} else {
						a.scrollViewport(-page)
					}
				}
			}
		}
		a.requestRedraw()
		return true
	}
	if action == glfw.Release {
		if a.scrollbar.owner == pointerOwnerScrollbar {
			a.scrollbar.owner = pointerOwnerNone
			a.scrollbar.pressed = false
			a.scrollbar.dragging = false
			a.scrollbar.hovered = g.slot.contains(x, y)
			a.scrollbar.lastActivity = time.Now()
			a.requestRedraw()
			return true
		}
		if a.scrollbar.owner == pointerOwnerTerminal {
			a.scrollbar.owner = pointerOwnerNone
		}
	}
	return false
}

func (a *App) handleScrollbarMove(x, y float32) bool {
	if !scrollbarEnabled(a.cfg.Scrollbar) {
		return false
	}
	g := a.currentScrollbarGeometry()
	if a.scrollbar.owner == pointerOwnerTerminal {
		return false
	}
	if a.scrollbar.owner == pointerOwnerScrollbar {
		if a.scrollbar.dragging {
			a.setViewportOffset(scrollbarOffsetForThumb(g, y-a.scrollbar.dragGrabOffset))
		}
		return true
	}
	hovered := g.slot.contains(x, y)
	if hovered != a.scrollbar.hovered {
		a.scrollbar.hovered = hovered
		a.scrollbar.lastActivity = time.Now()
		a.requestRedraw()
	}
	return hovered
}

func (a *App) handleScrollbarWheel(yoff float64, x, y float32) bool {
	if !scrollbarEnabled(a.cfg.Scrollbar) || a.scrollbar.owner == pointerOwnerTerminal {
		return false
	}
	g := a.currentScrollbarGeometry()
	if !g.slot.contains(x, y) {
		return false
	}
	rows := scrollRowsFromWheelDelta(yoff, a.cfg.Scrolling.WheelMultiplier)
	if rows != 0 {
		a.scrollViewport(rows)
		a.scrollbar.lastActivity = time.Now()
	}
	return true
}

func (a *App) scrollbarOpacity(now time.Time, history int) float32 {
	if !scrollbarEnabled(a.cfg.Scrollbar) || history <= 0 {
		return 0
	}
	if a.scrollbar.hovered || a.scrollbar.dragging || a.scrollbar.pressed {
		return 1
	}
	switch scrollbarMode(a.cfg.Scrollbar) {
	case "always":
		return 1
	case "hover":
		return 0
	}
	if a.scrollbar.lastActivity.IsZero() {
		return 0
	}
	fadeStart := a.scrollbar.lastActivity.Add(time.Duration(a.cfg.Scrollbar.AutoHideDelayMS) * time.Millisecond)
	if now.Before(fadeStart) {
		return 1
	}
	fade := time.Duration(a.cfg.Scrollbar.FadeMS) * time.Millisecond
	if fade <= 0 || !now.Before(fadeStart.Add(fade)) {
		return 0
	}
	frame := time.Second / time.Duration(max(1, a.cfg.Scrollbar.AnimationFPS))
	elapsed := now.Sub(fadeStart)
	sample := (elapsed / frame) * frame
	return float32(1 - sample.Seconds()/fade.Seconds())
}

func (a *App) scrollbarWake(now time.Time) (time.Duration, bool) {
	if !scrollbarEnabled(a.cfg.Scrollbar) || scrollbarMode(a.cfg.Scrollbar) != "scrolling" || a.scrollbar.hovered || a.scrollbar.dragging || a.scrollbar.lastActivity.IsZero() {
		return 0, false
	}
	fadeStart := a.scrollbar.lastActivity.Add(time.Duration(a.cfg.Scrollbar.AutoHideDelayMS) * time.Millisecond)
	if now.Before(fadeStart) {
		return fadeStart.Sub(now), true
	}
	fadeEnd := fadeStart.Add(time.Duration(a.cfg.Scrollbar.FadeMS) * time.Millisecond)
	if now.Before(fadeEnd) {
		frame := time.Second / time.Duration(max(1, a.cfg.Scrollbar.AnimationFPS))
		elapsed := now.Sub(fadeStart)
		next := fadeStart.Add((elapsed/frame + 1) * frame)
		if next.After(fadeEnd) {
			next = fadeEnd
		}
		return next.Sub(now), true
	}
	if a.scrollbar.lastPaintedOpacity > 0 {
		return minWake, true
	}
	return 0, false
}

func (a *App) scrollbarNeedsRedraw(now time.Time) bool {
	return a.scrollbarOpacity(now, a.snap.HistoryRows) != a.scrollbar.lastPaintedOpacity
}

func withOpacity(c color.RGBA, opacity float32) color.RGBA {
	c.A = uint8(float32(c.A) * max(float32(0), min(float32(1), opacity)))
	return c
}

func (a *App) drawScrollbar(now time.Time, background color.RGBA, w, h int) {
	if !scrollbarEnabled(a.cfg.Scrollbar) {
		a.scrollbar.lastPaintedOpacity = 0
		return
	}
	g := a.scrollbarGeometryForSnapshot(w, h)
	// The target is persistent: overwrite the complete reserved grid-height slot
	// before drawing a new fade sample so translucent chrome never accumulates.
	a.replaceRect(g.slot.x, g.track.y, g.slot.w, g.track.h, background)
	opacity := a.scrollbarOpacity(now, g.history)
	a.scrollbar.lastPaintedOpacity = opacity
	if opacity <= 0 || g.history <= 0 {
		return
	}
	track := withOpacity(a.chrome.scrollTrack, opacity)
	thumbColor := a.chrome.scrollThumb
	if a.scrollbar.pressed || a.scrollbar.dragging {
		thumbColor = a.chrome.scrollThumbPress
	} else if a.scrollbar.hovered {
		thumbColor = a.chrome.scrollThumbHover
	}
	thumb := withOpacity(thumbColor, opacity)
	radius := float32(a.cfg.Scrollbar.RadiusPX) * max(float32(1), a.uiScale)
	a.fillRoundedRect(g.track, radius, track)
	a.fillRoundedRect(g.thumb, radius, thumb)
}

func (a *App) fillRoundedRect(r rectF, radius float32, c color.RGBA) {
	if r.w <= 0 || r.h <= 0 || c.A == 0 {
		return
	}
	radius = min(radius, min(r.w, r.h)/2)
	if radius <= 0.5 {
		a.fillRect(r.x, r.y, r.w, r.h, c)
		return
	}
	a.fillRect(r.x, r.y+radius, r.w, max(float32(0), r.h-2*radius), c)
	steps := max(1, int(math.Ceil(float64(radius))))
	for i := 0; i < steps; i++ {
		dy := float32(i) + 0.5
		d := radius - dy
		inset := radius - float32(math.Sqrt(float64(max(float32(0), radius*radius-d*d))))
		stripW := max(float32(0), r.w-2*inset)
		a.fillRect(r.x+inset, r.y+float32(i), stripW, 1, c)
		a.fillRect(r.x+inset, r.y+r.h-float32(i)-1, stripW, 1, c)
	}
}
