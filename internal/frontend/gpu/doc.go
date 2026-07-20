// Package gpu defines a backend-neutral rendering seam for CervTerm's terminal
// surface, so OpenGL (today), Vulkan, Metal, and WebGPU can be swapped behind
// one Renderer interface without changing the frontend draw path.
//
// STATUS: Renderer and the OpenGL adapter are live. Vulkan, Metal, and WebGPU
// are build-tagged STUBS: their methods return errNotImplemented or record
// nothing, so default and -tags glfw builds remain unaffected:
//
//	go build -tags vulkan ./internal/frontend/gpu/   # Vulkan stub
//	go build -tags webgpu ./internal/frontend/gpu/   # WebGPU stub
//	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -tags metal ./internal/frontend/gpu/
//
// Phase 0 is complete: terminal rows, cursor, overlays, chrome, glyphs, atlas
// uploads/resets, and presentation all route through Renderer. A real alternate
// backend still needs startup selection plus its API-specific implementation.
//
// Alternate backends must preserve partial redraws with a persistent offscreen
// target because swapchain/drawable/surface images rotate. BeginFrame never
// clears; Clear affects that persistent target only on a frontend full redraw.
package gpu
