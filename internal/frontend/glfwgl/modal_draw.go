//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/modal"
)

func (a *App) drawModal(w, h int, chrome chromeColors) {
	if !a.modal.Active() {
		return
	}
	state := a.modal.Snapshot()
	columns := min(80, max(20, a.cols-4))
	rows := min(18, max(4, a.rows-4))
	layout := modal.ListLayout(state, modal.LayoutGeometry{Columns: columns, Rows: rows, VisibleRows: rows - 2})
	a.paint(modalDrawList(layout, columns, rows, w, h, a.cellW, a.cellH, a.uiScale, chrome.background, chrome.accent, chrome.muted))
	a.drawModalPreedit(w, h, columns, rows, chrome)
}

func modalDrawList(layout modal.Layout, columns, rows, winW, winH int, cellW, cellH, scale float32, background, accent, muted color.RGBA) []drawCmd {
	if len(layout.Commands) == 0 {
		return nil
	}
	pad := 6 * scale
	width := float32(columns)*cellW + 2*pad
	height := float32(rows)*cellH + 2*pad
	x := (float32(winW) - width) / 2
	y := (float32(winH) - height) / 3
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	cmds := make([]drawCmd, 0, len(layout.Commands)*2+2)
	cmds = append(cmds, drawCmd{kind: cmdRect, x: x, y: y, w: width, h: height, col: background}, drawCmd{kind: cmdRect, x: x, y: y, w: width, h: max(1, scale), col: accent})
	for _, row := range layout.Commands {
		ry := y + pad + float32(row.Row)*cellH
		textColor := accent
		if row.Kind == modal.RowHelp {
			textColor = muted
		}
		if row.Kind == modal.RowError {
			textColor = color.RGBA{R: 255, G: 110, B: 110, A: 255}
		}
		if row.Selected {
			cmds = append(cmds, drawCmd{kind: cmdRect, x: x + pad/2, y: ry, w: width - pad, h: cellH, col: color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: 48}})
		}
		cmds = append(cmds, drawCmd{kind: cmdText, x: x + pad, y: ry, text: row.Text, col: textColor})
	}
	return cmds
}
