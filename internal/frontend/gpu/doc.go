// Package gpu defines a backend-neutral rendering seam for CervTerm's terminal
// surface, so the OpenGL path (today), a Vulkan path, and a Metal path can be
// swapped behind one interface (Renderer) without the frontend knowing which GPU
// API is in use.
//
// STATUS: scaffolding. Renderer is defined; the Vulkan and Metal implementations
// here are STUBS (every method returns/records nothing or errNotImplemented).
// They are gated behind build tags so the default and -tags glfw builds are
// unaffected:
//
//	go build -tags vulkan ./internal/frontend/gpu/   # compiles the Vulkan stub
//	GOOS=darwin go build -tags metal ./...           # compiles the Metal stub
//
// PREREQUISITE (Phase 0, backend-agnostic, do this first): the glfwgl frontend
// currently calls gl.* directly in the hot grid path (drawRow, atlas glyph
// quads, drawCursor) and only routes the chrome through a draw-list. To make ANY
// alternate backend real, the WHOLE render must go through gpu.Renderer:
//
//  1. Implement Renderer with the existing OpenGL calls (an "glRenderer" that
//     wraps fillRect / the atlas quad emit / viewport+ortho+clear+swap).
//  2. Convert drawRow and the glyph emit to produce Renderer calls (or vertex
//     data) instead of gl.Begin/Vertex/End. The draw-list work already done for
//     the chrome is the template — the grid needs the same immediate→retained
//     shift, because Vulkan/Metal have no glBegin/End; you build vertex buffers.
//  3. Pick the backend at startup (config or build tag) via a factory.
//
// Only after Phase 0 do the Vulkan/Metal implementations below plug in and
// render the actual terminal. Until then they render nothing.
package gpu
