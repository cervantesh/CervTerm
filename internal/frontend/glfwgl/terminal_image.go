//go:build glfw

package glfwgl

import (
	"fmt"
	"math"

	"cervterm/internal/frontend/gpu"
	"cervterm/internal/termimage"

	"github.com/go-gl/gl/v2.1/gl"
)

type terminalImageGL interface {
	createTexture(width, height int32, rgba []byte) (uint32, error)
	deleteTexture(texture uint32)
	bindTexture(texture uint32)
	setTexture2DEnabled(enabled bool)
	drawQuad(destination gpu.ImageRect, u0, v0, u1, v1, opacity float32)
	restoreBlend()
}

type nativeTerminalImageGL struct{}

func (nativeTerminalImageGL) createTexture(width, height int32, rgba []byte) (uint32, error) {
	for gl.GetError() != gl.NO_ERROR {
	}
	var texture uint32
	gl.GenTextures(1, &texture)
	if texture == 0 {
		return 0, fmt.Errorf("gpu: OpenGL did not allocate a terminal image texture")
	}
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, width, height, 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba))
	if code := gl.GetError(); code != gl.NO_ERROR {
		gl.DeleteTextures(1, &texture)
		return 0, fmt.Errorf("gpu: OpenGL terminal image upload failed: 0x%x", code)
	}
	return texture, nil
}

func (nativeTerminalImageGL) deleteTexture(texture uint32) {
	gl.DeleteTextures(1, &texture)
}

func (nativeTerminalImageGL) bindTexture(texture uint32) {
	gl.BindTexture(gl.TEXTURE_2D, texture)
}

func (nativeTerminalImageGL) setTexture2DEnabled(enabled bool) {
	if enabled {
		gl.Enable(gl.TEXTURE_2D)
		return
	}
	gl.Disable(gl.TEXTURE_2D)
}

func (nativeTerminalImageGL) drawQuad(destination gpu.ImageRect, u0, v0, u1, v1, opacity float32) {
	gl.Color4f(1, 1, 1, opacity)
	gl.Begin(gl.QUADS)
	gl.TexCoord2f(u0, v0)
	gl.Vertex2f(destination.X, destination.Y)
	gl.TexCoord2f(u1, v0)
	gl.Vertex2f(destination.X+destination.Width, destination.Y)
	gl.TexCoord2f(u1, v1)
	gl.Vertex2f(destination.X+destination.Width, destination.Y+destination.Height)
	gl.TexCoord2f(u0, v1)
	gl.Vertex2f(destination.X, destination.Y+destination.Height)
	gl.End()
}

func (nativeTerminalImageGL) restoreBlend() {
	gl.Enable(gl.BLEND)
	gl.BlendFuncSeparate(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA, gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
}

type glImageTexture struct {
	renderer      *glRenderer
	texture       uint32
	width, height uint32
	key           gpu.ImageTextureKey
}

func (t *glImageTexture) Close() error {
	if t == nil || t.texture == 0 {
		return nil
	}
	if t.renderer == nil {
		return fmt.Errorf("gpu: terminal image texture has no owner")
	}
	texture := t.texture
	t.texture = 0
	if t.renderer.boundTexture == texture {
		t.renderer.boundTexture = 0
	}
	t.renderer.terminalImageAPI().deleteTexture(texture)
	return nil
}

func (r *glRenderer) terminalImageAPI() terminalImageGL {
	if r.imageGL != nil {
		return r.imageGL
	}
	return nativeTerminalImageGL{}
}

func validateTerminalImageResource(key gpu.ImageTextureKey, resource termimage.DetachedResource) (int32, int32, error) {
	if key.PaneObject == 0 || key.Resource != resource.Ref {
		return 0, 0, fmt.Errorf("gpu: terminal image texture key is invalid")
	}
	if resource.Ref.Image == 0 || resource.Ref.Generation == 0 {
		return 0, 0, fmt.Errorf("gpu: terminal image resource identity is invalid")
	}
	stride, byteCount, err := termimage.CheckedRGBABytes(resource.Width, resource.Height)
	if err != nil {
		return 0, 0, fmt.Errorf("gpu: invalid terminal image resource: %w", err)
	}
	if resource.Stride != stride {
		return 0, 0, fmt.Errorf("gpu: terminal image stride is %d, want %d", resource.Stride, stride)
	}
	if uint64(len(resource.RGBA)) != byteCount {
		return 0, 0, fmt.Errorf("gpu: terminal image RGBA length is %d, want %d", len(resource.RGBA), byteCount)
	}
	return int32(resource.Width), int32(resource.Height), nil
}

func (r *glRenderer) PrepareTerminalImage(key gpu.ImageTextureKey, resource termimage.DetachedResource) (gpu.ImageTexture, error) {
	if r == nil {
		return nil, fmt.Errorf("gpu: terminal image renderer is nil")
	}
	width, height, err := validateTerminalImageResource(key, resource)
	if err != nil {
		return nil, err
	}
	previousTexture := r.boundTexture
	texture, err := r.terminalImageAPI().createTexture(width, height, resource.RGBA)
	if err != nil || texture == 0 {
		if previousTexture != 0 {
			r.terminalImageAPI().bindTexture(previousTexture)
		}
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("gpu: OpenGL did not create a terminal image texture")
	}
	r.boundTexture = texture
	return &glImageTexture{
		renderer: r,
		texture:  texture,
		width:    resource.Width,
		height:   resource.Height,
		key:      key,
	}, nil
}

func finiteImageValue(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func validateTerminalImageDraw(texture *glImageTexture, destination, source gpu.ImageRect, opacity float32) error {
	if !finiteImageValue(destination.X) || !finiteImageValue(destination.Y) ||
		!finiteImageValue(destination.Width) || !finiteImageValue(destination.Height) ||
		destination.Width <= 0 || destination.Height <= 0 {
		return fmt.Errorf("gpu: terminal image destination rectangle is invalid")
	}
	sourceRight, sourceBottom := source.X+source.Width, source.Y+source.Height
	if !finiteImageValue(source.X) || !finiteImageValue(source.Y) ||
		!finiteImageValue(source.Width) || !finiteImageValue(source.Height) ||
		!finiteImageValue(sourceRight) || !finiteImageValue(sourceBottom) ||
		source.X < 0 || source.Y < 0 || source.Width <= 0 || source.Height <= 0 ||
		sourceRight > float32(texture.width) || sourceBottom > float32(texture.height) {
		return fmt.Errorf("gpu: terminal image source rectangle is invalid")
	}
	if !finiteImageValue(opacity) || opacity < 0 || opacity > 1 {
		return fmt.Errorf("gpu: terminal image opacity is invalid")
	}
	return nil
}

func (r *glRenderer) DrawTerminalImage(texture gpu.ImageTexture, destination, source gpu.ImageRect, opacity float32) error {
	if r == nil {
		return fmt.Errorf("gpu: terminal image renderer is nil")
	}
	imageTexture, ok := texture.(*glImageTexture)
	if !ok || imageTexture == nil {
		return fmt.Errorf("gpu: invalid terminal image texture")
	}
	if imageTexture.renderer != r {
		return fmt.Errorf("gpu: terminal image texture belongs to another renderer")
	}
	if imageTexture.texture == 0 {
		return fmt.Errorf("gpu: terminal image texture is closed")
	}
	if err := validateTerminalImageDraw(imageTexture, destination, source, opacity); err != nil {
		return err
	}
	if opacity == 0 {
		return nil
	}
	api := r.terminalImageAPI()
	if !r.texEnabled {
		api.setTexture2DEnabled(true)
		r.texEnabled = true
	}
	if r.boundTexture != imageTexture.texture {
		api.bindTexture(imageTexture.texture)
		r.boundTexture = imageTexture.texture
	}
	u0 := source.X / float32(imageTexture.width)
	v0 := source.Y / float32(imageTexture.height)
	u1 := (source.X + source.Width) / float32(imageTexture.width)
	v1 := (source.Y + source.Height) / float32(imageTexture.height)
	api.drawQuad(destination, u0, v0, u1, v1, opacity)
	api.restoreBlend()
	return nil
}
