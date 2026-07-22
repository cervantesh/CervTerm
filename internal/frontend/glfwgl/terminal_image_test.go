//go:build glfw

package glfwgl

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"cervterm/internal/frontend/gpu"
	"cervterm/internal/termimage"
)

type terminalImageDrawCall struct {
	destination    gpu.ImageRect
	u0, v0, u1, v1 float32
	opacity        float32
}

type fakeTerminalImageGL struct {
	calls         []string
	nextTexture   uint32
	createErr     error
	createWidth   int32
	createHeight  int32
	createRGBA    []byte
	deleted       []uint32
	bound         []uint32
	textureStates []bool
	draws         []terminalImageDrawCall
	restored      int
}

func (f *fakeTerminalImageGL) createTexture(width, height int32, rgba []byte) (uint32, error) {
	f.calls = append(f.calls, "create")
	f.createWidth, f.createHeight = width, height
	f.createRGBA = append(f.createRGBA[:0], rgba...)
	return f.nextTexture, f.createErr
}

func (f *fakeTerminalImageGL) deleteTexture(texture uint32) {
	f.calls = append(f.calls, "delete")
	f.deleted = append(f.deleted, texture)
}

func (f *fakeTerminalImageGL) bindTexture(texture uint32) {
	f.calls = append(f.calls, "bind")
	f.bound = append(f.bound, texture)
}

func (f *fakeTerminalImageGL) setTexture2DEnabled(enabled bool) {
	f.calls = append(f.calls, "texture-state")
	f.textureStates = append(f.textureStates, enabled)
}

func (f *fakeTerminalImageGL) drawQuad(destination gpu.ImageRect, u0, v0, u1, v1, opacity float32) {
	f.calls = append(f.calls, "draw")
	f.draws = append(f.draws, terminalImageDrawCall{
		destination: destination,
		u0:          u0,
		v0:          v0,
		u1:          u1,
		v1:          v1,
		opacity:     opacity,
	})
}

func (f *fakeTerminalImageGL) restoreBlend() {
	f.calls = append(f.calls, "restore-blend")
	f.restored++
}

func (f *fakeTerminalImageGL) resetCalls() {
	f.calls = nil
	f.deleted = nil
	f.bound = nil
	f.textureStates = nil
	f.draws = nil
	f.restored = 0
}

func validDetachedResource() termimage.DetachedResource {
	return termimage.DetachedResource{
		Ref:    termimage.ResourceRef{Image: 1, Generation: 2},
		Width:  2,
		Height: 2,
		Stride: 8,
		RGBA: []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			9, 10, 11, 12, 13, 14, 15, 16,
		},
	}
}

func validImageTextureKey() gpu.ImageTextureKey {
	return gpu.ImageTextureKey{PaneObject: 7, Resource: validDetachedResource().Ref}
}

func TestPrepareTerminalImageRejectsMalformedResourceBeforeGLCalls(t *testing.T) {
	valid := validDetachedResource()
	tests := []struct {
		name   string
		mutate func(*termimage.DetachedResource)
	}{
		{name: "zero image identity", mutate: func(resource *termimage.DetachedResource) { resource.Ref.Image = 0 }},
		{name: "zero generation", mutate: func(resource *termimage.DetachedResource) { resource.Ref.Generation = 0 }},
		{name: "zero width", mutate: func(resource *termimage.DetachedResource) { resource.Width = 0 }},
		{name: "dimension above hard cap", mutate: func(resource *termimage.DetachedResource) { resource.Width = termimage.HardImageDimension + 1 }},
		{name: "non-exact stride", mutate: func(resource *termimage.DetachedResource) { resource.Stride++ }},
		{name: "short RGBA", mutate: func(resource *termimage.DetachedResource) { resource.RGBA = resource.RGBA[:len(resource.RGBA)-1] }},
		{name: "long RGBA", mutate: func(resource *termimage.DetachedResource) { resource.RGBA = append(resource.RGBA, 17) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := valid
			resource.RGBA = append([]byte(nil), valid.RGBA...)
			test.mutate(&resource)
			fake := &fakeTerminalImageGL{nextTexture: 31}
			renderer := &glRenderer{imageGL: fake}
			if texture, err := renderer.PrepareTerminalImage(validImageTextureKey(), resource); err == nil || texture != nil {
				t.Fatalf("PrepareTerminalImage() = (%v, %v), want nil error result", texture, err)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("GL calls before validation completed: %v", fake.calls)
			}
		})
	}
}

func TestPrepareTerminalImageRejectsInvalidKeyBeforeGLCalls(t *testing.T) {
	resource := validDetachedResource()
	for _, key := range []gpu.ImageTextureKey{{Resource: resource.Ref}, {PaneObject: 7, Resource: termimage.ResourceRef{Image: 9, Generation: 9}}} {
		fake := &fakeTerminalImageGL{nextTexture: 31}
		renderer := &glRenderer{imageGL: fake}
		if texture, err := renderer.PrepareTerminalImage(key, resource); err == nil || texture != nil {
			t.Fatalf("PrepareTerminalImage()=(%v,%v), want rejection", texture, err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("GL calls before key validation: %v", fake.calls)
		}
	}
}

func TestPrepareTerminalImageCreatesStandaloneTexture(t *testing.T) {
	fake := &fakeTerminalImageGL{nextTexture: 41}
	renderer := &glRenderer{imageGL: fake, pages: []uint32{3, 5}, boundTexture: 3}
	resource := validDetachedResource()
	texture, err := renderer.PrepareTerminalImage(validImageTextureKey(), resource)
	if err != nil {
		t.Fatal(err)
	}
	created, ok := texture.(*glImageTexture)
	if !ok {
		t.Fatalf("texture type = %T", texture)
	}
	if created.renderer != renderer || created.texture != 41 || created.width != 2 || created.height != 2 {
		t.Fatalf("created texture = %#v", created)
	}
	if fake.createWidth != 2 || fake.createHeight != 2 || !reflect.DeepEqual(fake.createRGBA, resource.RGBA) {
		t.Fatalf("create upload = %dx%d %v", fake.createWidth, fake.createHeight, fake.createRGBA)
	}
	if !reflect.DeepEqual(fake.calls, []string{"create"}) {
		t.Fatalf("calls = %v", fake.calls)
	}
	if renderer.boundTexture != 41 {
		t.Fatalf("bound texture = %d, want 41", renderer.boundTexture)
	}
	if !reflect.DeepEqual(renderer.pages, []uint32{3, 5}) {
		t.Fatalf("terminal image changed atlas pages: %v", renderer.pages)
	}
}

func TestPrepareTerminalImageReportsTextureCreationFailure(t *testing.T) {
	fake := &fakeTerminalImageGL{nextTexture: 41, createErr: errors.New("upload failed")}
	renderer := &glRenderer{imageGL: fake, boundTexture: 7}
	texture, err := renderer.PrepareTerminalImage(validImageTextureKey(), validDetachedResource())
	if err == nil || texture != nil {
		t.Fatalf("PrepareTerminalImage() = (%v, %v), want failure", texture, err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"create", "bind"}) || !reflect.DeepEqual(fake.bound, []uint32{7}) {
		t.Fatalf("calls = %v", fake.calls)
	}
	if renderer.boundTexture != 7 {
		t.Fatalf("failed create changed bound texture to %d", renderer.boundTexture)
	}
}

func TestDrawTerminalImageUsesNormalizedUVsAndRestoresRendererState(t *testing.T) {
	fake := &fakeTerminalImageGL{}
	renderer := &glRenderer{imageGL: fake, pages: []uint32{4}, boundTexture: 4}
	texture := &glImageTexture{renderer: renderer, texture: 19, width: 8, height: 4}
	destination := gpu.ImageRect{X: -3, Y: 5, Width: 20, Height: 10}
	source := gpu.ImageRect{X: 2, Y: 1, Width: 4, Height: 2}
	if err := renderer.DrawTerminalImage(texture, destination, source, 0.5); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"texture-state", "bind", "draw", "restore-blend"}) {
		t.Fatalf("calls = %v", fake.calls)
	}
	if !reflect.DeepEqual(fake.textureStates, []bool{true}) || !reflect.DeepEqual(fake.bound, []uint32{19}) {
		t.Fatalf("texture state = %v bound = %v", fake.textureStates, fake.bound)
	}
	if len(fake.draws) != 1 {
		t.Fatalf("draw count = %d", len(fake.draws))
	}
	draw := fake.draws[0]
	if draw.destination != destination || draw.u0 != 0.25 || draw.v0 != 0.25 || draw.u1 != 0.75 || draw.v1 != 0.75 || draw.opacity != 0.5 {
		t.Fatalf("draw = %#v", draw)
	}
	if !renderer.texEnabled || renderer.boundTexture != 19 || fake.restored != 1 {
		t.Fatalf("renderer state: texture enabled=%v bound=%d restored=%d", renderer.texEnabled, renderer.boundTexture, fake.restored)
	}
	if !reflect.DeepEqual(renderer.pages, []uint32{4}) {
		t.Fatalf("draw changed atlas pages: %v", renderer.pages)
	}

	fake.resetCalls()
	if err := renderer.DrawTerminalImage(texture, destination, source, 1); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"draw", "restore-blend"}) {
		t.Fatalf("same-texture calls = %v", fake.calls)
	}
}

func TestDrawTerminalImageRejectsInvalidDrawBeforeGLCalls(t *testing.T) {
	fake := &fakeTerminalImageGL{}
	renderer := &glRenderer{imageGL: fake}
	texture := &glImageTexture{renderer: renderer, texture: 23, width: 8, height: 4}
	validDestination := gpu.ImageRect{X: 1, Y: 2, Width: 3, Height: 4}
	validSource := gpu.ImageRect{Width: 8, Height: 4}
	tests := []struct {
		name        string
		destination gpu.ImageRect
		source      gpu.ImageRect
		opacity     float32
	}{
		{name: "zero destination width", destination: gpu.ImageRect{Height: 1}, source: validSource, opacity: 1},
		{name: "non-finite destination", destination: gpu.ImageRect{Width: 1, Height: float32(math.Inf(1))}, source: validSource, opacity: 1},
		{name: "negative source", destination: validDestination, source: gpu.ImageRect{X: -1, Width: 1, Height: 1}, opacity: 1},
		{name: "source outside width", destination: validDestination, source: gpu.ImageRect{X: 7, Width: 2, Height: 1}, opacity: 1},
		{name: "source outside height", destination: validDestination, source: gpu.ImageRect{Y: 3, Width: 1, Height: 2}, opacity: 1},
		{name: "non-finite source", destination: validDestination, source: gpu.ImageRect{Width: float32(math.NaN()), Height: 1}, opacity: 1},
		{name: "negative opacity", destination: validDestination, source: validSource, opacity: -0.1},
		{name: "opacity above one", destination: validDestination, source: validSource, opacity: 1.1},
		{name: "non-finite opacity", destination: validDestination, source: validSource, opacity: float32(math.NaN())},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := renderer.DrawTerminalImage(texture, test.destination, test.source, test.opacity); err == nil {
				t.Fatal("DrawTerminalImage() error = nil")
			}
			if len(fake.calls) != 0 {
				t.Fatalf("GL calls before validation completed: %v", fake.calls)
			}
		})
	}
}

type otherImageTexture struct{}

func (*otherImageTexture) Close() error { return nil }

func TestDrawTerminalImageRejectsInvalidClosedAndForeignTextures(t *testing.T) {
	fake := &fakeTerminalImageGL{}
	renderer := &glRenderer{imageGL: fake}
	destination := gpu.ImageRect{Width: 1, Height: 1}
	source := gpu.ImageRect{Width: 1, Height: 1}
	foreignRenderer := &glRenderer{imageGL: &fakeTerminalImageGL{}}
	tests := []struct {
		name    string
		texture gpu.ImageTexture
	}{
		{name: "nil", texture: nil},
		{name: "other implementation", texture: &otherImageTexture{}},
		{name: "closed", texture: &glImageTexture{renderer: renderer, width: 1, height: 1}},
		{name: "foreign owner", texture: &glImageTexture{renderer: foreignRenderer, texture: 1, width: 1, height: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := renderer.DrawTerminalImage(test.texture, destination, source, 1); err == nil {
				t.Fatal("DrawTerminalImage() error = nil")
			}
			if len(fake.calls) != 0 {
				t.Fatalf("invalid texture issued GL calls: %v", fake.calls)
			}
		})
	}
}

func TestDrawTerminalImageZeroOpacityIsValidatedNoOp(t *testing.T) {
	fake := &fakeTerminalImageGL{}
	renderer := &glRenderer{imageGL: fake, boundTexture: 5}
	texture := &glImageTexture{renderer: renderer, texture: 29, width: 2, height: 2}
	rect := gpu.ImageRect{Width: 2, Height: 2}
	if err := renderer.DrawTerminalImage(texture, rect, rect, 0); err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 0 || renderer.texEnabled || renderer.boundTexture != 5 {
		t.Fatalf("zero-opacity state: calls=%v enabled=%v bound=%d", fake.calls, renderer.texEnabled, renderer.boundTexture)
	}
}

func TestTerminalImageTextureCloseIsIdempotentAndInvalidatesBinding(t *testing.T) {
	fake := &fakeTerminalImageGL{}
	renderer := &glRenderer{imageGL: fake, boundTexture: 37}
	texture := &glImageTexture{renderer: renderer, texture: 37, width: 2, height: 2}
	if err := texture.Close(); err != nil {
		t.Fatal(err)
	}
	if texture.texture != 0 || renderer.boundTexture != 0 {
		t.Fatalf("closed texture=%d bound=%d", texture.texture, renderer.boundTexture)
	}
	if !reflect.DeepEqual(fake.calls, []string{"delete"}) || !reflect.DeepEqual(fake.deleted, []uint32{37}) {
		t.Fatalf("close calls=%v deleted=%v", fake.calls, fake.deleted)
	}
	if err := texture.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fake.calls, []string{"delete"}) {
		t.Fatalf("idempotent close calls = %v", fake.calls)
	}
}
