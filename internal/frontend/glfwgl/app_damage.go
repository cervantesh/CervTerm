//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/render"
	termsel "cervterm/internal/selection"
)

type damageState struct {
	valid             bool
	framebufferWidth  int
	framebufferHeight int
	contentScaleX     float32
	contentScaleY     float32
	cols              int
	displayOffset     int
	alternateScreen   bool
	selectionActive   bool
	selectionStart    termsel.Point
	selectionEnd      termsel.Point
	showStats         bool
	noticeVisible     bool
	searching         bool
	searchHasMatch    bool
	searchMatchRow    int
	searchMatchCol    int
	searchMatchLen    int
	statusSeq         int
	statusGeometry    statusGeometry
	overlaySeq        int
	background        color.RGBA
	damagedRows       []bool
	rowsDrawn         int
}

func (a *App) prepareDamage(w, h, displayOffset int, alternateScreen, noticeVisible bool, background color.RGBA) (bool, []bool) {
	rows := a.snap.Rows
	if cap(a.rowHashes) < rows {
		a.rowHashes = make([]uint64, rows)
	} else {
		a.rowHashes = a.rowHashes[:rows]
	}
	render.HashRows(a.rowHashes, a.snap.Cells, a.snap.Cols)

	stateChanged := !a.damage.valid ||
		w != a.damage.framebufferWidth || h != a.damage.framebufferHeight ||
		a.contentScaleX != a.damage.contentScaleX || a.contentScaleY != a.damage.contentScaleY ||
		a.snap.Cols != a.damage.cols || displayOffset != a.damage.displayOffset ||
		alternateScreen != a.damage.alternateScreen ||
		a.selectionActive != a.damage.selectionActive ||
		a.selectionStart != a.damage.selectionStart || a.selectionEnd != a.damage.selectionEnd ||
		a.showStats != a.damage.showStats || noticeVisible != a.damage.noticeVisible ||
		a.searching != a.damage.searching || a.searchHasMatch != a.damage.searchHasMatch ||
		a.searchMatchRow != a.damage.searchMatchRow || a.searchMatchCol != a.damage.searchMatchCol ||
		a.searchMatchLen != a.damage.searchMatchLen ||
		a.status.seq != a.damage.statusSeq || a.status.geometry != a.damage.statusGeometry ||
		a.overlays.seq != a.damage.overlaySeq ||
		background != a.damage.background
	historySizeMismatch := (len(a.prevHashes) > 0 && len(a.prevHashes) != rows) ||
		(len(a.prevPrevHashes) > 0 && len(a.prevPrevHashes) != rows)
	// The interactive search bar overlay is global state (like selection and the
	// stats panel), so an open bar forces a full-frame repaint every frame (trap
	// 3). The scriptable term:search highlight (searching == false) does not: its
	// appear/move/clear transitions are caught by stateChanged above (the match
	// coords are tracked there), and between transitions the highlight persists
	// via normal row damage, so a scripted search never pins full-frame redraws.
	global := stateChanged || historySizeMismatch || a.selectionActive || a.cfg.Render.Bidi || a.showStats || noticeVisible || a.searching || a.cfg.Render.Damage == "frame"
	fullRedraw := global || len(a.prevHashes) == 0 || len(a.prevPrevHashes) == 0
	if global {
		a.prevHashes = a.prevHashes[:0]
		a.prevPrevHashes = a.prevPrevHashes[:0]
	}

	if cap(a.damage.damagedRows) < rows {
		a.damage.damagedRows = make([]bool, rows)
	} else {
		a.damage.damagedRows = a.damage.damagedRows[:rows]
		clear(a.damage.damagedRows)
	}
	damaged := a.damage.damagedRows
	if fullRedraw {
		for row := range damaged {
			damaged[row] = true
		}
	} else {
		for row, hash := range a.rowHashes {
			damaged[row] = hash != a.prevHashes[row] || hash != a.prevPrevHashes[row]
		}
		// Mark the cursor rows of both prior rendered frames, not just the last:
		// the back buffer we are drawing into may be the N-2 image, whose cursor
		// sat at prevCursorRow. Omitting it leaves a ghost cursor (e.g. after the
		// startup banner) on the alternating buffer.
		markDamagedRow(damaged, a.lastCursorRow)
		markDamagedRow(damaged, a.prevCursorRow)
		markDamagedRow(damaged, a.snap.CursorRow)
	}
	// A steady status overlay does not force a full frame, but any frame that is
	// drawn must repaint the terminal rows beneath it before redrawing the band.
	if a.status.geometry.visible {
		for row := a.status.geometry.firstRow; row <= a.status.geometry.lastRow; row++ {
			markDamagedRow(damaged, row)
		}
	}
	// Same rationale for overlays: a steady visible overlay does not force a full
	// frame, but its covered rows must repaint the terminal beneath before the
	// overlay recomposites (a seq change already forced this frame full above).
	a.markOverlayDamage(damaged, rows)
	return fullRedraw, damaged
}

func (a *App) recordDamageFrame(w, h, displayOffset int, alternateScreen, noticeVisible bool, background color.RGBA, rowsDrawn int) {
	a.prevPrevHashes, a.prevHashes, a.rowHashes = a.prevHashes, a.rowHashes, a.prevPrevHashes
	a.prevCursorRow, a.lastCursorRow = a.lastCursorRow, a.snap.CursorRow
	a.damage.valid = true
	a.damage.framebufferWidth, a.damage.framebufferHeight = w, h
	a.damage.contentScaleX, a.damage.contentScaleY = a.contentScaleX, a.contentScaleY
	a.damage.cols = a.snap.Cols
	a.damage.displayOffset, a.damage.alternateScreen = displayOffset, alternateScreen
	a.damage.selectionActive = a.selectionActive
	a.damage.selectionStart, a.damage.selectionEnd = a.selectionStart, a.selectionEnd
	a.damage.showStats, a.damage.noticeVisible = a.showStats, noticeVisible
	a.damage.searching, a.damage.searchHasMatch = a.searching, a.searchHasMatch
	a.damage.searchMatchRow, a.damage.searchMatchCol = a.searchMatchRow, a.searchMatchCol
	a.damage.searchMatchLen = a.searchMatchLen
	a.damage.statusSeq, a.damage.statusGeometry = a.status.seq, a.status.geometry
	a.damage.overlaySeq = a.overlays.seq
	a.damage.background = background
	a.damage.rowsDrawn = rowsDrawn
}

func markDamagedRow(rows []bool, row int) {
	if row >= 0 && row < len(rows) {
		rows[row] = true
	}
}
