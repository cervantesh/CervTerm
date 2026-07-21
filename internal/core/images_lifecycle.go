package core

import (
	"math"

	"cervterm/internal/termimage"
)

type imageCellRect struct {
	top, bottom int64
	left, right uint32
}

func placementRect(placement termimage.Placement) imageCellRect {
	return imageCellRect{
		top: placement.Anchor.Row, bottom: placement.Anchor.Row + int64(placement.Rows),
		left: placement.Anchor.Col, right: placement.Anchor.Col + uint32(placement.Cols),
	}
}

func imageRectsIntersect(left, right imageCellRect) bool {
	return left.top < right.bottom && right.top < left.bottom && left.left < right.right && right.left < left.right
}

func imageRectContains(outer, inner imageCellRect) bool {
	return inner.top >= outer.top && inner.bottom <= outer.bottom && inner.left >= outer.left && inner.right <= outer.right
}

func (t *Terminal) mutateImagePlacements(alternate bool, transform func(termimage.Placement) (termimage.Placement, bool)) bool {
	if t == nil || t.imageStore == nil || t.imageSidecars == nil {
		return false
	}
	if t.imageSidecars.generation == math.MaxUint64 {
		t.closeImages()
		return true
	}
	source := t.imageSidecars.primary
	if alternate {
		source = t.imageSidecars.alternate
	}
	if len(source) == 0 {
		return false
	}
	var next []imagePlacement
	var retired []*termimage.PlacementReservation
	for index, entry := range source {
		placement, keep := transform(entry.placement)
		changed := !keep || placement != entry.placement
		if changed && next == nil {
			next = make([]imagePlacement, 0, len(source))
			next = append(next, source[:index]...)
		}
		if next == nil {
			continue
		}
		if !keep {
			retired = append(retired, entry.lease)
			continue
		}
		entry.placement = placement
		next = append(next, entry)
	}
	if next == nil {
		return false
	}
	sidecars := &imageSidecars{
		primary: t.imageSidecars.primary, alternate: t.imageSidecars.alternate,
		generation: t.imageSidecars.generation + 1,
	}
	if alternate {
		sidecars.alternate = next
	} else {
		sidecars.primary = next
	}
	t.imageSidecars = sidecars
	for _, lease := range retired {
		lease.Close()
	}
	return true
}

func (t *Terminal) activeImageGlobalRow(liveRow int) int64 {
	if t.alternateScreen {
		return int64(liveRow)
	}
	return int64(t.scrollbackRows + liveRow)
}

func (t *Terminal) eraseImageLiveRect(top, bottom, left, right int) {
	if top >= bottom || left >= right {
		return
	}
	rect := imageCellRect{top: t.activeImageGlobalRow(top), bottom: t.activeImageGlobalRow(bottom), left: uint32(left), right: uint32(right)}
	t.mutateImagePlacements(t.alternateScreen, func(placement termimage.Placement) (termimage.Placement, bool) {
		return placement, !imageRectsIntersect(placementRect(placement), rect)
	})
}

func (t *Terminal) moveImagesInRect(alternate bool, affected, movable imageCellRect, rowDelta int64, colDelta int32) {
	t.mutateImagePlacements(alternate, func(placement termimage.Placement) (termimage.Placement, bool) {
		rect := placementRect(placement)
		if !imageRectsIntersect(rect, affected) {
			return placement, true
		}
		if !imageRectContains(movable, rect) {
			return placement, false
		}
		placement.Anchor.Row += rowDelta
		if colDelta < 0 {
			placement.Anchor.Col -= uint32(-colDelta)
		} else {
			placement.Anchor.Col += uint32(colDelta)
		}
		return placement, true
	})
}

func (t *Terminal) editImageChars(insert bool, row, col, count int) {
	if count <= 0 || col >= t.cols {
		return
	}
	globalRow := t.activeImageGlobalRow(row)
	affected := imageCellRect{top: globalRow, bottom: globalRow + 1, left: uint32(col), right: uint32(t.cols)}
	movable := affected
	delta := int32(count)
	if insert {
		movable.right = uint32(t.cols - count)
	} else {
		movable.left = uint32(col + count)
		delta = -delta
	}
	t.moveImagesInRect(t.alternateScreen, affected, movable, 0, delta)
}

func (t *Terminal) moveImagesInLiveRows(top, bottom, sourceTop, sourceBottom, delta int) {
	base := int64(0)
	if !t.alternateScreen {
		base = int64(t.scrollbackRows)
	}
	affectedTop, affectedBottom := base+int64(top), base+int64(bottom+1)
	movableTop, movableBottom := base+int64(sourceTop), base+int64(sourceBottom+1)
	t.mutateImagePlacements(t.alternateScreen, func(placement termimage.Placement) (termimage.Placement, bool) {
		rect := placementRect(placement)
		if rect.bottom <= affectedTop || rect.top >= affectedBottom {
			return placement, true
		}
		if rect.top < movableTop || rect.bottom > movableBottom {
			return placement, false
		}
		placement.Anchor.Row += int64(delta)
		return placement, true
	})
}

func (t *Terminal) dropPrimaryImageRows(count int) {
	if count <= 0 {
		return
	}
	boundary := int64(count)
	t.mutateImagePlacements(false, func(placement termimage.Placement) (termimage.Placement, bool) {
		if placement.Anchor.Row < boundary {
			return placement, false
		}
		placement.Anchor.Row -= boundary
		return placement, true
	})
}

func (t *Terminal) imagesScrollUp(top, bottom, lines int) {
	if t.imageSidecars == nil {
		return
	}
	if !t.alternateScreen && top == 0 && bottom == t.rows-1 {
		newHistory := min(t.scrollbackCapacity, t.scrollbackRows+lines)
		t.dropPrimaryImageRows(t.scrollbackRows + lines - newHistory)
		return
	}
	t.moveImagesInLiveRows(top, bottom, top+lines, bottom, -lines)
}

func (t *Terminal) imagesScrollDown(top, bottom, lines int) {
	if t.imageSidecars == nil {
		return
	}
	t.moveImagesInLiveRows(top, bottom, top, bottom-lines, lines)
}

func (t *Terminal) imagesClearScrollback() {
	if t.alternateScreen || t.scrollbackRows == 0 {
		return
	}
	t.dropPrimaryImageRows(t.scrollbackRows)
}

func (t *Terminal) imagesSetScrollbackCapacity(oldRows, newRows int) {
	t.dropPrimaryImageRows(oldRows - newRows)
}

func (t *Terminal) imagesReflowPrimary(mapping reflowMap, evicted, cols int) {
	if t.imageSidecars == nil {
		return
	}
	t.mutateImagePlacements(false, func(placement termimage.Placement) (termimage.Placement, bool) {
		row, col, ok := mapping.mapCell(int(placement.Anchor.Row), int(placement.Anchor.Col))
		if !ok || row < evicted || col < 0 {
			return placement, false
		}
		col = min(col, cols-1)
		placement.Anchor.Row = int64(row - evicted)
		placement.Anchor.Col = uint32(col)
		return placement, true
	})
}

func (t *Terminal) imagesCropAlternate(cols, rows int) {
	if t.imageSidecars == nil {
		return
	}
	t.mutateImagePlacements(true, func(placement termimage.Placement) (termimage.Placement, bool) {
		return placement, placement.Anchor.Row >= 0 && placement.Anchor.Row < int64(rows) && placement.Anchor.Col < uint32(cols)
	})
}

func (t *Terminal) imagesDiscardAlternate() {
	if t.imageSidecars == nil {
		return
	}
	t.mutateImagePlacements(true, func(placement termimage.Placement) (termimage.Placement, bool) { return placement, false })
}

func (t *Terminal) imageViewportPlacements(dst []termimage.Placement) []termimage.Placement {
	dst = dst[:0]
	if t == nil || t.imageSidecars == nil {
		return dst
	}
	source := t.imageSidecars.primary
	viewportTop := int64(t.ViewportTopGlobalRow())
	if t.alternateScreen {
		source, viewportTop = t.imageSidecars.alternate, 0
	}
	viewport := imageCellRect{top: viewportTop, bottom: viewportTop + int64(t.rows), left: 0, right: uint32(t.cols)}
	for _, entry := range source {
		if !imageRectsIntersect(placementRect(entry.placement), viewport) {
			continue
		}
		placement := entry.placement
		placement.Anchor.Row -= viewportTop
		dst = append(dst, placement)
	}
	return dst
}
