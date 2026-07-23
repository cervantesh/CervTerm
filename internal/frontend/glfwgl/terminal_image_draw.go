//go:build glfw

package glfwgl

import (
	"math"
	"sort"
	"time"

	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	"cervterm/internal/termimage"
)

const terminalImageBufferAge = 2

type terminalImagePaneFrame struct {
	pane       termmux.PaneID
	paneObject uint64
	generation uint64
	geometry   termmux.PaneGeometry
	snapshot   render.Snapshot
	drawStart  int
	drawEnd    int
}

type terminalImageDrawDescriptor struct {
	key         gpu.ImageTextureKey
	z           int16
	renderOrder int
	destination gpu.ImageRect
	crop        termimage.PixelRect
	hasCrop     bool
	source      gpu.ImageRect
	opacity     float32
	texture     gpu.ImageTexture
	ready       bool
}

type terminalImageKeyCandidate struct {
	key         gpu.ImageTextureKey
	z           int16
	renderOrder int
}

type terminalImageFrame struct {
	panes      []terminalImagePaneFrame
	draws      []terminalImageDrawDescriptor
	candidates []terminalImageKeyCandidate
	keys       []gpu.ImageTextureKey
	seen       map[gpu.ImageTextureKey]struct{}
}

func (f *terminalImageFrame) reset() {
	f.panes = f.panes[:0]
	f.draws = f.draws[:0]
	f.candidates = f.candidates[:0]
	f.keys = f.keys[:0]
	if f.seen != nil {
		clear(f.seen)
	}
}

func (f *terminalImageFrame) appendPane(geometry termmux.PaneGeometry, snapshot render.Snapshot, cellW, cellH float32) {
	pane := terminalImagePaneFrame{
		pane:       geometry.Pane,
		paneObject: snapshot.PaneObject,
		generation: snapshot.ImageGeneration,
		geometry:   geometry,
		snapshot:   snapshot,
		drawStart:  len(f.draws),
	}
	if geometry.Pixels.Empty() || cellW <= 0 || cellH <= 0 || !finiteImageValue(cellW) || !finiteImageValue(cellH) {
		pane.drawEnd = len(f.draws)
		f.panes = append(f.panes, pane)
		return
	}
	for order, image := range snapshot.Images {
		placement := image.Placement
		if image.PaneObject == 0 || image.PaneObject != snapshot.PaneObject ||
			placement.Resource.Image == 0 || placement.Resource.Generation == 0 ||
			!terminalImagePlacementVisible(placement, snapshot.Cols, snapshot.Rows) {
			continue
		}
		descriptor := terminalImageDrawDescriptor{
			key: gpu.ImageTextureKey{
				PaneObject: image.PaneObject,
				Resource:   placement.Resource,
			},
			z:           placement.Z,
			renderOrder: order,
			destination: gpu.ImageRect{
				X:      float32(geometry.Pixels.X) + float32(placement.Anchor.Col)*cellW,
				Y:      float32(geometry.Pixels.Y) + float32(placement.Anchor.Row)*cellH,
				Width:  float32(placement.Cols) * cellW,
				Height: float32(placement.Rows) * cellH,
			},
			opacity: float32(placement.Opacity) / float32(math.MaxUint8),
		}
		if placement.Crop != nil {
			descriptor.crop = *placement.Crop
			descriptor.hasCrop = true
		}
		f.draws = append(f.draws, descriptor)
		f.candidates = append(f.candidates, terminalImageKeyCandidate{
			key: descriptor.key, z: descriptor.z, renderOrder: descriptor.renderOrder,
		})
	}
	pane.drawEnd = len(f.draws)
	sort.SliceStable(f.draws[pane.drawStart:pane.drawEnd], func(left, right int) bool {
		return terminalImageDrawLess(
			f.draws[pane.drawStart+left],
			f.draws[pane.drawStart+right],
		)
	})
	f.panes = append(f.panes, pane)
}

func terminalImagePlacementVisible(placement termimage.Placement, cols, rows int) bool {
	if cols <= 0 || rows <= 0 || placement.Cols == 0 || placement.Rows == 0 {
		return false
	}
	rowSpan := int64(placement.Rows)
	if placement.Anchor.Row > math.MaxInt64-rowSpan {
		return false
	}
	rowBottom := placement.Anchor.Row + rowSpan
	colRight := uint64(placement.Anchor.Col) + uint64(placement.Cols)
	return placement.Anchor.Row < int64(rows) && rowBottom > 0 &&
		uint64(placement.Anchor.Col) < uint64(cols) && colRight > 0
}

func terminalImageDrawLess(left, right terminalImageDrawDescriptor) bool {
	if left.z != right.z {
		return left.z < right.z
	}
	if left.renderOrder != right.renderOrder {
		return left.renderOrder < right.renderOrder
	}
	return imageTextureKeyLess(left.key, right.key)
}

func terminalImageCandidateLess(left, right terminalImageKeyCandidate) bool {
	if left.z != right.z {
		return left.z < right.z
	}
	if left.renderOrder != right.renderOrder {
		return left.renderOrder < right.renderOrder
	}
	return imageTextureKeyLess(left.key, right.key)
}

func (f *terminalImageFrame) finalizeKeys() {
	sort.SliceStable(f.candidates, func(left, right int) bool {
		return terminalImageCandidateLess(f.candidates[left], f.candidates[right])
	})
	if len(f.candidates) != 0 && f.seen == nil {
		f.seen = make(map[gpu.ImageTextureKey]struct{}, len(f.candidates))
	}
	for _, candidate := range f.candidates {
		if _, duplicate := f.seen[candidate.key]; duplicate {
			continue
		}
		f.seen[candidate.key] = struct{}{}
		f.keys = append(f.keys, candidate.key)
	}
}

func (f *terminalImageFrame) resolve(cache *terminalImageCache) {
	for index := range f.draws {
		draw := &f.draws[index]
		draw.texture = nil
		draw.source = gpu.ImageRect{}
		draw.ready = false
		texture, metadata, ok := cache.textureMetadata(draw.key)
		if !ok {
			continue
		}
		if draw.hasCrop {
			if !terminalImageCropWithin(draw.crop, metadata.Width, metadata.Height) {
				continue
			}
			draw.source = gpu.ImageRect{
				X: float32(draw.crop.X), Y: float32(draw.crop.Y),
				Width: float32(draw.crop.Width), Height: float32(draw.crop.Height),
			}
		} else {
			draw.source = gpu.ImageRect{Width: float32(metadata.Width), Height: float32(metadata.Height)}
		}
		draw.texture = texture
		draw.ready = true
	}
}

func terminalImageCropWithin(crop termimage.PixelRect, width, height uint32) bool {
	return crop.Width != 0 && crop.Height != 0 && crop.X <= width && crop.Y <= height &&
		crop.Width <= width-crop.X && crop.Height <= height-crop.Y
}

func (f *terminalImageFrame) paneFrame(pane termmux.PaneID) (*terminalImagePaneFrame, bool) {
	for index := range f.panes {
		if f.panes[index].pane == pane {
			return &f.panes[index], true
		}
	}
	return nil, false
}

func (f *terminalImageFrame) references(key gpu.ImageTextureKey) bool {
	for index := range f.draws {
		if f.draws[index].key == key {
			return true
		}
	}
	return false
}

func (f *terminalImageFrame) drawPane(cache *terminalImageCache, pane termmux.PaneID, negative bool) {
	paneFrame, ok := f.paneFrame(pane)
	if !ok {
		return
	}
	for index := paneFrame.drawStart; index < paneFrame.drawEnd; index++ {
		draw := &f.draws[index]
		if !draw.ready || (draw.z < 0) != negative {
			continue
		}
		cache.draw(draw.texture, draw.destination, draw.source, draw.opacity)
	}
}

func (a *App) beginTerminalImageFrame(now time.Time, layout termmux.Layout) {
	cache := a.terminalImageCache
	if cache == nil {
		return
	}
	frame := &a.terminalImages
	frame.reset()
	if a.mux != nil {
		for _, geometry := range layout.Panes {
			view, ok := a.mux.PaneView(geometry.Pane)
			if !ok {
				continue
			}
			state := a.ensurePaneUI(geometry.Pane)
			frame.appendPane(geometry, view.Snapshot, state.font.cellW, state.font.cellH)
		}
	}
	frame.finalizeKeys()
	// App.draw is entered only inside windowController.withCurrent, so uploads
	// and prior-entry eviction below execute under this cache's owning context.
	result := cache.beginFrame(now, frame.keys)
	frame.resolve(cache)
	a.terminalImageDamage.observe(frame, result.Uploaded)
}

func (a *App) terminalImageSnapshot(pane termmux.PaneID) (render.Snapshot, bool) {
	if a.terminalImageCache != nil {
		frame, ok := a.terminalImages.paneFrame(pane)
		if !ok {
			return render.Snapshot{}, false
		}
		return frame.snapshot, true
	}
	view, ok := a.mux.PaneView(pane)
	if !ok {
		return render.Snapshot{}, false
	}
	return view.Snapshot, true
}

func (a *App) drawTerminalImages(pane termmux.PaneID, negative bool) {
	if a.terminalImageCache == nil {
		return
	}
	a.terminalImages.drawPane(a.terminalImageCache, pane, negative)
}

func (a *App) finishTerminalImageFrame() {
	if a.terminalImageCache == nil {
		return
	}
	a.terminalImageDamage.drawn(a.terminalImages.panes)
}

type terminalImagePaneDamage struct {
	generation uint64
	remaining  uint8
	seen       uint64
}

type terminalImageDamageState struct {
	panes    map[uint64]terminalImagePaneDamage
	sequence uint64
}

func (s *terminalImageDamageState) observe(frame *terminalImageFrame, uploaded []gpu.ImageTextureKey) {
	s.sequence++
	if s.sequence == 0 {
		for paneObject, state := range s.panes {
			state.seen = 0
			s.panes[paneObject] = state
		}
		s.sequence = 1
	}
	for _, pane := range frame.panes {
		if pane.paneObject == 0 {
			continue
		}
		if s.panes == nil {
			s.panes = make(map[uint64]terminalImagePaneDamage)
		}
		state, exists := s.panes[pane.paneObject]
		if !exists {
			state.generation = pane.generation
			if pane.generation != 0 {
				state.remaining = terminalImageBufferAge
			}
		} else if state.generation != pane.generation {
			state.generation = pane.generation
			state.remaining = terminalImageBufferAge
		}
		state.seen = s.sequence
		s.panes[pane.paneObject] = state
	}
	for _, key := range uploaded {
		state, exists := s.panes[key.PaneObject]
		if !exists || state.seen != s.sequence || !frame.references(key) {
			continue
		}
		state.remaining = terminalImageBufferAge
		s.panes[key.PaneObject] = state
	}
	for paneObject, state := range s.panes {
		if state.seen != s.sequence {
			delete(s.panes, paneObject)
		}
	}
}

func (s *terminalImageDamageState) drawn(panes []terminalImagePaneFrame) {
	for _, pane := range panes {
		state, ok := s.panes[pane.paneObject]
		if !ok || state.remaining == 0 {
			continue
		}
		state.remaining--
		s.panes[pane.paneObject] = state
	}
}

func (s *terminalImageDamageState) pending() bool {
	for _, state := range s.panes {
		if state.remaining != 0 {
			return true
		}
	}
	return false
}

func (s *terminalImageDamageState) pendingFor(paneObject uint64) uint8 {
	return s.panes[paneObject].remaining
}

func (a *App) resetTerminalImageDrawState() {
	a.terminalImages = terminalImageFrame{}
	a.terminalImageDamage = terminalImageDamageState{}
}
