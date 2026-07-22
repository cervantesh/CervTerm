//go:build glfw

package glfwgl

import (
	"fmt"
	"reflect"
	"testing"

	"cervterm/internal/core"
	"cervterm/internal/frontend/gpu"
	termmux "cervterm/internal/mux"
	"cervterm/internal/render"
	"cervterm/internal/termimage"
)

type terminalImageRecordingTexture struct {
	label string
	key   gpu.ImageTextureKey
}

func (*terminalImageRecordingTexture) Close() error { return nil }

type terminalImageRecordingDraw struct {
	key                 gpu.ImageTextureKey
	destination, source gpu.ImageRect
	opacity             float32
}

type terminalImageRecordingRenderer struct {
	events *[]string
	draws  []terminalImageRecordingDraw
}

func (*terminalImageRecordingRenderer) PrepareTerminalImage(gpu.ImageTextureKey, termimage.DetachedResource) (gpu.ImageTexture, error) {
	return nil, fmt.Errorf("unexpected terminal image preparation")
}

func (r *terminalImageRecordingRenderer) DrawTerminalImage(texture gpu.ImageTexture, destination, source gpu.ImageRect, opacity float32) error {
	recorded, ok := texture.(*terminalImageRecordingTexture)
	if !ok {
		return fmt.Errorf("unexpected texture type %T", texture)
	}
	if r.events != nil {
		*r.events = append(*r.events, "image:"+recorded.label)
	}
	r.draws = append(r.draws, terminalImageRecordingDraw{
		key: recorded.key, destination: destination, source: source, opacity: opacity,
	})
	return nil
}

func terminalImageTestPlacement(key gpu.ImageTextureKey, z int16, row int64, col uint32, cols, rows uint16, opacity uint8, crop *termimage.PixelRect) render.ImagePlacement {
	return render.ImagePlacement{
		PaneObject: key.PaneObject,
		Placement: termimage.Placement{
			Resource: key.Resource,
			Anchor:   termimage.CellAnchor{Row: row, Col: col},
			Cols:     cols,
			Rows:     rows,
			Crop:     crop,
			Z:        z,
			Opacity:  opacity,
		},
	}
}

func terminalImageTestCache(renderer *terminalImageRecordingRenderer, textures map[gpu.ImageTextureKey]*terminalImageRecordingTexture, sizes map[gpu.ImageTextureKey]terminalImageTextureMetadata) *terminalImageCache {
	entries := make(map[gpu.ImageTextureKey]*terminalImageCacheEntry, len(textures))
	for key, texture := range textures {
		metadata := sizes[key]
		entries[key] = &terminalImageCacheEntry{
			texture: texture, width: metadata.Width, height: metadata.Height,
		}
	}
	return &terminalImageCache{renderer: renderer, entries: entries}
}

func TestTerminalImagePaneDispatchGoldenPreservesClipAndSemanticLayerOrder(t *testing.T) {
	pane := termmux.PaneID(7)
	geometry := termmux.PaneGeometry{
		Pane: pane, Pixels: termmux.PixelRect{X: 10, Y: 20, Width: 80, Height: 64}, Cols: 10, Rows: 4,
	}
	keys := []gpu.ImageTextureKey{
		cacheKey(70, 1, 1),
		cacheKey(70, 2, 1),
		cacheKey(70, 3, 1),
		cacheKey(70, 4, 1),
	}
	labels := []string{"negative-back", "positive-front", "negative-near", "zero-front"}
	snapshot := render.Snapshot{Cols: 10, Rows: 4, PaneObject: 70, ImageGeneration: 1}
	snapshot.Images = []render.ImagePlacement{
		terminalImageTestPlacement(keys[0], -2, 0, 0, 1, 1, 255, nil),
		terminalImageTestPlacement(keys[1], 2, 0, 0, 1, 1, 255, nil),
		terminalImageTestPlacement(keys[2], -1, 0, 0, 1, 1, 255, nil),
		terminalImageTestPlacement(keys[3], 0, 0, 0, 1, 1, 255, nil),
	}

	var frame terminalImageFrame
	frame.appendPane(geometry, snapshot, 8, 16)
	frame.finalizeKeys()
	var events []string
	renderer := &terminalImageRecordingRenderer{events: &events}
	textures := make(map[gpu.ImageTextureKey]*terminalImageRecordingTexture, len(keys))
	sizes := make(map[gpu.ImageTextureKey]terminalImageTextureMetadata, len(keys))
	for index, key := range keys {
		textures[key] = &terminalImageRecordingTexture{label: labels[index], key: key}
		sizes[key] = terminalImageTextureMetadata{Width: 1, Height: 1}
	}
	cache := terminalImageTestCache(renderer, textures, sizes)
	frame.resolve(cache)

	dispatchTerminalImagePaneDraw(func(stage terminalImagePaneDrawStage) {
		switch stage {
		case terminalImagePanePushClip:
			clip := terminalImagePaneClip(geometry)
			events = append(events, fmt.Sprintf("push-clip:%d,%d,%d,%d", clip.X, clip.Y, clip.Width, clip.Height))
		case terminalImagePaneBackground:
			events = append(events, "semantic-background")
		case terminalImagePaneNegativeImages:
			frame.drawPane(cache, pane, true)
		case terminalImagePaneText:
			events = append(events, "semantic-text")
		case terminalImagePaneNonNegativeImages:
			frame.drawPane(cache, pane, false)
		case terminalImagePaneOverlays:
			events = append(events, "application-overlays")
		case terminalImagePanePopClip:
			events = append(events, "pop-clip")
		}
	})

	want := []string{
		"push-clip:10,20,80,64",
		"semantic-background",
		"image:negative-back",
		"image:negative-near",
		"semantic-text",
		"image:zero-front",
		"image:positive-front",
		"application-overlays",
		"pop-clip",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("pane draw golden:\n got: %v\nwant: %v", events, want)
	}
}

func TestTerminalImageFrameResolvesCropFullSourceOpacityAndDestinationGeometry(t *testing.T) {
	pane := termmux.PaneID(3)
	geometry := termmux.PaneGeometry{
		Pane: pane, Pixels: termmux.PixelRect{X: 5, Y: 7, Width: 160, Height: 160}, Cols: 20, Rows: 10,
	}
	fullKey := cacheKey(30, 1, 1)
	cropKey := cacheKey(30, 2, 1)
	crop := termimage.PixelRect{X: 2, Y: 3, Width: 4, Height: 5}
	snapshot := render.Snapshot{Cols: 20, Rows: 10, PaneObject: 30, ImageGeneration: 1}
	snapshot.Images = []render.ImagePlacement{
		terminalImageTestPlacement(fullKey, 0, 1, 2, 3, 2, 255, nil),
		terminalImageTestPlacement(cropKey, 1, 3, 4, 2, 1, 128, &crop),
	}

	var frame terminalImageFrame
	frame.appendPane(geometry, snapshot, 8, 16)
	frame.finalizeKeys()
	renderer := &terminalImageRecordingRenderer{}
	cache := terminalImageTestCache(renderer,
		map[gpu.ImageTextureKey]*terminalImageRecordingTexture{
			fullKey: {label: "full", key: fullKey},
			cropKey: {label: "crop", key: cropKey},
		},
		map[gpu.ImageTextureKey]terminalImageTextureMetadata{
			fullKey: {Width: 13, Height: 9},
			cropKey: {Width: 20, Height: 10},
		},
	)
	frame.resolve(cache)
	frame.drawPane(cache, pane, false)

	want := []terminalImageRecordingDraw{
		{
			key:         fullKey,
			destination: gpu.ImageRect{X: 21, Y: 23, Width: 24, Height: 32},
			source:      gpu.ImageRect{Width: 13, Height: 9},
			opacity:     1,
		},
		{
			key:         cropKey,
			destination: gpu.ImageRect{X: 37, Y: 55, Width: 16, Height: 16},
			source:      gpu.ImageRect{X: 2, Y: 3, Width: 4, Height: 5},
			opacity:     float32(128) / float32(255),
		},
	}
	if !reflect.DeepEqual(renderer.draws, want) {
		t.Fatalf("resolved draws:\n got: %#v\nwant: %#v", renderer.draws, want)
	}
}

func TestTerminalImageFrameDrawsOnlyRequestedPane(t *testing.T) {
	firstPane, secondPane := termmux.PaneID(1), termmux.PaneID(2)
	firstKey := cacheKey(101, 1, 1)
	secondKey := cacheKey(202, 1, 1)
	var frame terminalImageFrame
	frame.appendPane(
		termmux.PaneGeometry{Pane: firstPane, Pixels: termmux.PixelRect{Width: 20, Height: 10}},
		render.Snapshot{
			Cols: 2, Rows: 1, PaneObject: 101, ImageGeneration: 1,
			Images: []render.ImagePlacement{terminalImageTestPlacement(firstKey, -1, 0, 0, 1, 1, 255, nil)},
		},
		10, 10,
	)
	frame.appendPane(
		termmux.PaneGeometry{Pane: secondPane, Pixels: termmux.PixelRect{X: 20, Width: 20, Height: 10}},
		render.Snapshot{
			Cols: 2, Rows: 1, PaneObject: 202, ImageGeneration: 1,
			Images: []render.ImagePlacement{terminalImageTestPlacement(secondKey, 0, 0, 0, 1, 1, 255, nil)},
		},
		10, 10,
	)
	frame.finalizeKeys()
	renderer := &terminalImageRecordingRenderer{}
	cache := terminalImageTestCache(renderer,
		map[gpu.ImageTextureKey]*terminalImageRecordingTexture{
			firstKey:  {label: "first", key: firstKey},
			secondKey: {label: "second", key: secondKey},
		},
		map[gpu.ImageTextureKey]terminalImageTextureMetadata{
			firstKey:  {Width: 1, Height: 1},
			secondKey: {Width: 1, Height: 1},
		},
	)
	frame.resolve(cache)

	frame.drawPane(cache, firstPane, true)
	frame.drawPane(cache, firstPane, false)
	if got := terminalImageDrawKeys(renderer.draws); !reflect.DeepEqual(got, []gpu.ImageTextureKey{firstKey}) {
		t.Fatalf("first pane draws = %v", got)
	}
	renderer.draws = renderer.draws[:0]
	frame.drawPane(cache, secondPane, true)
	frame.drawPane(cache, secondPane, false)
	if got := terminalImageDrawKeys(renderer.draws); !reflect.DeepEqual(got, []gpu.ImageTextureKey{secondKey}) {
		t.Fatalf("second pane draws = %v", got)
	}
	renderer.draws = renderer.draws[:0]
	frame.drawPane(cache, termmux.PaneID(999), true)
	frame.drawPane(cache, termmux.PaneID(999), false)
	if len(renderer.draws) != 0 {
		t.Fatalf("missing pane drew textures: %#v", renderer.draws)
	}
}

func terminalImageDrawKeys(draws []terminalImageRecordingDraw) []gpu.ImageTextureKey {
	keys := make([]gpu.ImageTextureKey, len(draws))
	for index := range draws {
		keys[index] = draws[index].key
	}
	return keys
}

func TestTerminalImageFrameCollectsDuplicateKeysDeterministically(t *testing.T) {
	duplicate := cacheKey(20, 1, 1)
	middle := cacheKey(20, 2, 1)
	first := cacheKey(10, 1, 1)
	build := func(reverse bool) []gpu.ImageTextureKey {
		t.Helper()
		var frame terminalImageFrame
		appendPane20 := func() {
			frame.appendPane(
				termmux.PaneGeometry{Pane: 20, Pixels: termmux.PixelRect{Width: 10, Height: 10}},
				render.Snapshot{
					Cols: 1, Rows: 1, PaneObject: 20, ImageGeneration: 1,
					Images: []render.ImagePlacement{
						terminalImageTestPlacement(duplicate, 2, 0, 0, 1, 1, 255, nil),
						terminalImageTestPlacement(middle, -1, 0, 0, 1, 1, 255, nil),
						terminalImageTestPlacement(duplicate, -2, 0, 0, 1, 1, 255, nil),
					},
				},
				10, 10,
			)
		}
		appendPane10 := func() {
			frame.appendPane(
				termmux.PaneGeometry{Pane: 10, Pixels: termmux.PixelRect{Width: 10, Height: 10}},
				render.Snapshot{
					Cols: 1, Rows: 1, PaneObject: 10, ImageGeneration: 1,
					Images: []render.ImagePlacement{terminalImageTestPlacement(first, -2, 0, 0, 1, 1, 255, nil)},
				},
				10, 10,
			)
		}
		if reverse {
			appendPane20()
			appendPane10()
		} else {
			appendPane10()
			appendPane20()
		}
		frame.finalizeKeys()
		return append([]gpu.ImageTextureKey(nil), frame.keys...)
	}

	want := []gpu.ImageTextureKey{first, duplicate, middle}
	if got := build(false); !reflect.DeepEqual(got, want) {
		t.Fatalf("forward keys = %v, want %v", got, want)
	}
	if got := build(true); !reflect.DeepEqual(got, want) {
		t.Fatalf("reverse keys = %v, want %v", got, want)
	}
}

func TestTerminalImageFrameDeterministicallyOmitsMissingTexture(t *testing.T) {
	pane := termmux.PaneID(4)
	missing := cacheKey(40, 1, 1)
	ready := cacheKey(40, 2, 1)
	var frame terminalImageFrame
	frame.appendPane(
		termmux.PaneGeometry{Pane: pane, Pixels: termmux.PixelRect{Width: 20, Height: 10}},
		render.Snapshot{
			Cols: 2, Rows: 1, PaneObject: 40, ImageGeneration: 1,
			Images: []render.ImagePlacement{
				terminalImageTestPlacement(missing, -1, 0, 0, 1, 1, 255, nil),
				terminalImageTestPlacement(ready, 0, 0, 1, 1, 1, 255, nil),
			},
		},
		10, 10,
	)
	frame.finalizeKeys()
	renderer := &terminalImageRecordingRenderer{}
	cache := terminalImageTestCache(renderer,
		map[gpu.ImageTextureKey]*terminalImageRecordingTexture{
			ready: {label: "ready", key: ready},
		},
		map[gpu.ImageTextureKey]terminalImageTextureMetadata{
			ready: {Width: 1, Height: 1},
		},
	)
	frame.resolve(cache)
	frame.drawPane(cache, pane, true)
	frame.drawPane(cache, pane, false)

	if got := terminalImageDrawKeys(renderer.draws); !reflect.DeepEqual(got, []gpu.ImageTextureKey{ready}) {
		t.Fatalf("draws with missing texture = %v", got)
	}
	if frame.draws[0].key != missing || frame.draws[0].ready || frame.draws[0].texture != nil || frame.draws[0].source != (gpu.ImageRect{}) {
		t.Fatalf("missing descriptor resolved nondeterministically: %#v", frame.draws[0])
	}
}

func TestTerminalImageDamageTracksGenerationPerPaneForBothBackBuffersWithoutRowHashChanges(t *testing.T) {
	firstKey := cacheKey(101, 1, 1)
	secondKey := cacheKey(202, 1, 1)
	cells := []core.Cell{{Rune: 'a'}, {Rune: 'b'}, {Rune: 'c'}, {Rune: 'd'}}
	firstSnapshot := render.Snapshot{
		Cols: 2, Rows: 2, Cells: cells, PaneObject: 101, ImageGeneration: 1,
		Images: []render.ImagePlacement{terminalImageTestPlacement(firstKey, 0, 0, 0, 1, 1, 255, nil)},
	}
	secondSnapshot := render.Snapshot{
		Cols: 2, Rows: 2, Cells: cells, PaneObject: 202, ImageGeneration: 7,
		Images: []render.ImagePlacement{terminalImageTestPlacement(secondKey, 0, 0, 0, 1, 1, 255, nil)},
	}
	frame := terminalImageDamageTestFrame(firstSnapshot, secondSnapshot)
	var damage terminalImageDamageState

	damage.observe(&frame, nil)
	if damage.pendingFor(101) != 2 || damage.pendingFor(202) != 2 {
		t.Fatalf("initial pane damage = %d/%d", damage.pendingFor(101), damage.pendingFor(202))
	}
	damage.drawn(frame.panes)
	damage.observe(&frame, nil)
	damage.drawn(frame.panes)
	if damage.pending() {
		t.Fatalf("initial damage did not drain: %#v", damage.panes)
	}

	beforeHashes := make([]uint64, firstSnapshot.Rows)
	afterHashes := make([]uint64, firstSnapshot.Rows)
	render.HashRows(beforeHashes, firstSnapshot.Cells, firstSnapshot.Cols)
	firstSnapshot.ImageGeneration++
	firstSnapshot.Images[0] = terminalImageTestPlacement(cacheKey(101, 1, 2), 0, 0, 0, 1, 1, 255, nil)
	render.HashRows(afterHashes, firstSnapshot.Cells, firstSnapshot.Cols)
	if !reflect.DeepEqual(beforeHashes, afterHashes) {
		t.Fatalf("image-only mutation changed row hashes: %v -> %v", beforeHashes, afterHashes)
	}

	frame = terminalImageDamageTestFrame(firstSnapshot, secondSnapshot)
	damage.observe(&frame, nil)
	if damage.pendingFor(101) != 2 || damage.pendingFor(202) != 0 {
		t.Fatalf("generation damage leaked panes = %d/%d", damage.pendingFor(101), damage.pendingFor(202))
	}
	damage.drawn(frame.panes)
	if damage.pendingFor(101) != 1 {
		t.Fatalf("first back-buffer repaint left %d frames", damage.pendingFor(101))
	}
	damage.observe(&frame, nil)
	damage.drawn(frame.panes)
	if damage.pendingFor(101) != 0 || damage.pending() {
		t.Fatalf("second back-buffer repaint did not drain: %#v", damage.panes)
	}
}

func TestTerminalImageDamageOnlyAcceptsExactUploadedKeyReferences(t *testing.T) {
	firstKey := cacheKey(301, 1, 3)
	secondKey := cacheKey(302, 2, 4)
	firstSnapshot := render.Snapshot{
		Cols: 1, Rows: 1, Cells: []core.Cell{{Rune: 'x'}}, PaneObject: 301, ImageGeneration: 10,
		Images: []render.ImagePlacement{terminalImageTestPlacement(firstKey, 0, 0, 0, 1, 1, 255, nil)},
	}
	secondSnapshot := render.Snapshot{
		Cols: 1, Rows: 1, Cells: []core.Cell{{Rune: 'y'}}, PaneObject: 302, ImageGeneration: 20,
		Images: []render.ImagePlacement{terminalImageTestPlacement(secondKey, 0, 0, 0, 1, 1, 255, nil)},
	}
	frame := terminalImageDamageTestFrame(firstSnapshot, secondSnapshot)
	var damage terminalImageDamageState
	damage.observe(&frame, nil)
	damage.drawn(frame.panes)
	damage.observe(&frame, nil)
	damage.drawn(frame.panes)

	wrongGeneration := cacheKey(301, 1, 99)
	absentPane := cacheKey(999, 1, 1)
	damage.observe(&frame, []gpu.ImageTextureKey{wrongGeneration, secondKey, absentPane})
	if damage.pendingFor(301) != 0 || damage.pendingFor(302) != 2 {
		t.Fatalf("non-reference upload damaged panes = %d/%d", damage.pendingFor(301), damage.pendingFor(302))
	}
	damage.drawn(frame.panes)
	damage.observe(&frame, nil)
	damage.drawn(frame.panes)
	if damage.pending() {
		t.Fatalf("second pane upload damage did not drain: %#v", damage.panes)
	}

	damage.observe(&frame, []gpu.ImageTextureKey{firstKey})
	if damage.pendingFor(301) != 2 || damage.pendingFor(302) != 0 {
		t.Fatalf("exact reference upload damage = %d/%d", damage.pendingFor(301), damage.pendingFor(302))
	}
}

func terminalImageDamageTestFrame(first, second render.Snapshot) terminalImageFrame {
	var frame terminalImageFrame
	frame.appendPane(
		termmux.PaneGeometry{Pane: 1, Pixels: termmux.PixelRect{Width: first.Cols, Height: first.Rows}},
		first, 1, 1,
	)
	frame.appendPane(
		termmux.PaneGeometry{Pane: 2, Pixels: termmux.PixelRect{X: first.Cols, Width: second.Cols, Height: second.Rows}},
		second, 1, 1,
	)
	frame.finalizeKeys()
	return frame
}
