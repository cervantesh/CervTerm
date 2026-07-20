//go:build glfw

package glfwgl

import (
	"image/color"

	"cervterm/internal/core"
	"cervterm/internal/frontend/gpu"
	"cervterm/internal/ime"
	"cervterm/internal/modal"
	"cervterm/internal/unicodecluster"
)

func (a *App) drawPreeditAt(presentation preeditPresentation, x, y float32, chrome chromeColors) {
	if !presentation.active || presentation.cells <= 0 {
		return
	}
	width := float32(presentation.cells) * a.cellW
	background := chrome.background
	background.A = 0xee
	a.fillRect(x, y, width, a.cellH, background)
	target := chrome.accent
	target.A = 0x58
	for _, span := range presentation.targetSpans {
		a.fillRect(x+float32(span.start)*a.cellW, y, float32(span.end-span.start)*a.cellW, a.cellH, target)
	}
	drawn := a.drawPreeditRTLRunClusters(presentation, x, y, chrome.accent)
	for index, cluster := range presentation.clusters {
		if drawn[index] {
			continue
		}
		clusterX := x + float32(cluster.cellStart)*a.cellW
		if !a.drawCluster(cluster.text, cluster.cellWidth, clusterX, y, chrome.accent, 1, 0) {
			a.drawPreeditClusterFallback(cluster, clusterX, y, chrome.accent)
		}
	}
	underline := chrome.muted
	underline.A = 0xff
	a.fillRect(x, y+a.cellH-max(1, a.uiScale), width, max(1, a.uiScale), underline)
	caretWidth := max(1, a.uiScale)
	caretX := x + preeditCaretOffset(presentation, a.cellW, caretWidth)
	a.fillRect(caretX, y, caretWidth, a.cellH, chrome.accent)
}

func preeditCaretOffset(presentation preeditPresentation, cellWidth, caretWidth float32) float32 {
	offset := float32(min(presentation.caretCell, presentation.cells)) * cellWidth
	if presentation.caretCell >= presentation.cells {
		offset = max(float32(0), float32(presentation.cells)*cellWidth-caretWidth)
	}
	return offset
}

func (a *App) drawPreeditRTLRunClusters(presentation preeditPresentation, x, y float32, color color.RGBA) []bool {
	drawn := make([]bool, len(presentation.clusters))
	for start := 0; start < len(presentation.clusters); {
		if !preeditClusterRTL(presentation.clusters[start]) {
			start++
			continue
		}
		end := start + 1
		for end < len(presentation.clusters) && preeditClusterRTL(presentation.clusters[end]) {
			end++
		}
		if end-start < 2 {
			start = end
			continue
		}
		text := ""
		cellStart, cellEnd := presentation.cells, 0
		for index := start; index < end; index++ {
			cluster := presentation.clusters[index]
			text += cluster.text
			cellStart = min(cellStart, cluster.cellStart)
			cellEnd = max(cellEnd, cluster.cellStart+cluster.cellWidth)
		}
		if a.drawCluster(text, cellEnd-cellStart, x+float32(cellStart)*a.cellW, y, color, 1, 0) {
			for index := start; index < end; index++ {
				drawn[index] = true
			}
		}
		start = end
	}
	return drawn
}

func (a *App) drawPreeditClusterFallback(cluster preeditCluster, x, y float32, color color.RGBA) {
	usedCells := 0
	lastBaseX := x
	for _, r := range cluster.text {
		width := core.RuneWidth(r)
		drawX := lastBaseX
		if width > 0 {
			if usedCells >= cluster.cellWidth {
				break
			}
			drawX = x + float32(usedCells)*a.cellW
			lastBaseX = drawX
			usedCells += width
		}
		a.drawRune(r, drawX, y, color, 1, 0)
	}
}

func (a *App) currentPreeditSnapshot(kind ime.TargetKind, id uint64) (ime.Snapshot, bool) {
	snapshot := a.composition.snapshot()
	if !snapshot.Active || snapshot.Target.Kind != kind || (id != 0 && snapshot.Target.ID != id) {
		return ime.Snapshot{}, false
	}
	current, err := a.currentCommittedTextTarget()
	if err != nil || current != snapshot.Target {
		return ime.Snapshot{}, false
	}
	return snapshot, true
}

func (a *App) drawTerminalPreedit(paneID uint64, x, y float32, availableCells int) {
	snapshot, ok := a.currentPreeditSnapshot(ime.TargetPane, paneID)
	if !ok {
		return
	}
	presentation := preparePreeditPresentation(snapshot, availableCells)
	a.drawPreeditAt(presentation, x, y, a.chrome)
	a.publishCandidateGeometry(presentation, x, y)
}

func (a *App) drawSearchPreedit(winW, winH int, chrome chromeColors) {
	snapshot, ok := a.currentPreeditSnapshot(ime.TargetSearch, 0)
	if !ok || !a.search.active {
		return
	}
	pad := 6 * a.uiScale
	prefixCells := unicodecluster.DisplayWidthString("buscar: " + string(a.search.query))
	x := pad + float32(prefixCells)*a.cellW
	barY := float32(winH) - (a.cellH + 2*pad)
	y := barY + pad
	available := max(0, int((float32(winW)-x-pad)/a.cellW))
	a.r.PushClip(gpu.ClipRect{X: 0, Y: int(barY), Width: winW, Height: int(a.cellH + 2*pad)})
	presentation := preparePreeditPresentation(snapshot, available)
	a.drawPreeditAt(presentation, x, y, chrome)
	a.r.PopClip()
	a.publishCandidateGeometry(presentation, x, y)
}

func (a *App) drawModalPreedit(winW, winH, columns, rows int, chrome chromeColors) {
	snapshot, ok := a.currentPreeditSnapshot(ime.TargetModal, 0)
	if !ok || !a.modal.Active() {
		return
	}
	pad := 6 * a.uiScale
	width := float32(columns)*a.cellW + 2*pad
	height := float32(rows)*a.cellH + 2*pad
	panelX := max(float32(0), (float32(winW)-width)/2)
	panelY := max(float32(0), (float32(winH)-height)/3)
	x, y := panelX+pad, panelY+pad
	queryCells := modal.CellWidth("> " + string(a.modal.Snapshot().Query))
	x += float32(queryCells) * a.cellW
	available := max(0, columns-queryCells)
	a.r.PushClip(gpu.ClipRect{X: int(panelX), Y: int(panelY), Width: int(width), Height: int(height)})
	presentation := preparePreeditPresentation(snapshot, available)
	a.drawPreeditAt(presentation, x, y, chrome)
	a.r.PopClip()
	a.publishCandidateGeometry(presentation, x, y)
}
