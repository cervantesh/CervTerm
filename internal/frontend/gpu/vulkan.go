//go:build vulkan

// Vulkan backend — STUB. Build with `-tags vulkan`. Nothing is implemented yet;
// this file lays out the standard Vulkan setup as a phase-by-phase skeleton to
// fill in. Do Phase 0 (route the whole frontend through gpu.Renderer, see
// doc.go) before wiring this in, or it renders nothing.
//
// Suggested binding: github.com/vulkan-go/vulkan (add to go.mod in Phase 1).
// GLFW creates the surface via glfw.Window.CreateWindowSurface and reports the
// required instance extensions via glfw.GetVulkanGetInstanceProcAddress /
// GetRequiredInstanceExtensions.

package gpu

import "image/color"

// vulkanRenderer holds the Vulkan objects a terminal backend needs. Every field
// is a TODO; the types are left as comments so this compiles without the binding
// until Phase 1 adds it.
type vulkanRenderer struct {
	widthPx, heightPx int

	// TODO Phase 1 — device & surface:
	//   instance       vk.Instance
	//   debugMessenger vk.DebugUtilsMessengerEXT   // validation layers (debug builds)
	//   surface        vk.Surface                  // from glfw CreateWindowSurface
	//   physicalDevice vk.PhysicalDevice
	//   device         vk.Device
	//   graphicsQueue  vk.Queue
	//   presentQueue   vk.Queue
	//
	// TODO Phase 2 — swapchain & frame plumbing:
	//   swapchain      vk.Swapchain
	//   images         []vk.Image
	//   imageViews     []vk.ImageView
	//   format         vk.Format
	//   extent         vk.Extent2D
	//   renderPass     vk.RenderPass
	//   framebuffers   []vk.Framebuffer
	//   commandPool    vk.CommandPool
	//   commandBuffers []vk.CommandBuffer
	//   imageAvailable []vk.Semaphore              // per frame-in-flight
	//   renderFinished []vk.Semaphore
	//   inFlight       []vk.Fence
	//   currentFrame   int
	//
	// TODO Phase 3 — pipelines & geometry (immediate→retained: build vertex
	// buffers per frame; there is no glBegin/End in Vulkan):
	//   pipelineSolid  vk.Pipeline                 // SPIR-V: colored quad
	//   pipelineGlyph  vk.Pipeline                 // SPIR-V: textured, tinted quad
	//   pipelineLayout vk.PipelineLayout
	//   vertexBuffers  []buffer                    // per frame-in-flight, dynamic
	//
	// TODO Phase 4 — atlas as textures:
	//   atlasPages     map[int]texture             // image + view + memory
	//   sampler        vk.Sampler
	//   descriptorPool vk.DescriptorPool
	//   descriptorSets []vk.DescriptorSet          // per atlas page
}

// NewVulkanRenderer will run the full init chain (createInstance → …
// → createSyncObjects) and return a ready Renderer. It errors until built out.
func NewVulkanRenderer(widthPx, heightPx int) (Renderer, error) {
	return nil, errNotImplemented
}

// --- Init phases (fill these in; each is a well-known Vulkan setup step) ------

func (r *vulkanRenderer) createInstance() error       { return errNotImplemented } // app info, required + validation extensions
func (r *vulkanRenderer) setupDebugMessenger() error  { return errNotImplemented } // debug builds only
func (r *vulkanRenderer) createSurface() error        { return errNotImplemented } // glfw.CreateWindowSurface
func (r *vulkanRenderer) pickPhysicalDevice() error   { return errNotImplemented } // graphics+present queue families, swapchain support
func (r *vulkanRenderer) createLogicalDevice() error  { return errNotImplemented } // logical device + queues
func (r *vulkanRenderer) createSwapchain() error      { return errNotImplemented } // surface format, present mode (vsync), extent
func (r *vulkanRenderer) createImageViews() error     { return errNotImplemented }
func (r *vulkanRenderer) createRenderPass() error     { return errNotImplemented }
func (r *vulkanRenderer) createFramebuffers() error   { return errNotImplemented }
func (r *vulkanRenderer) createPipelines() error      { return errNotImplemented } // load SPIR-V, solid + glyph pipelines
func (r *vulkanRenderer) createCommandPool() error    { return errNotImplemented }
func (r *vulkanRenderer) createVertexBuffers() error  { return errNotImplemented } // dynamic, host-visible, per frame-in-flight
func (r *vulkanRenderer) createAtlasResources() error { return errNotImplemented } // sampler, descriptor pool/layout
func (r *vulkanRenderer) createSyncObjects() error    { return errNotImplemented } // semaphores + fences
func (r *vulkanRenderer) recreateSwapchain() error    { return errNotImplemented } // on resize / out-of-date

// --- Renderer interface (stubbed) ---------------------------------------------

func (r *vulkanRenderer) Resize(widthPx, heightPx int) {
	r.widthPx, r.heightPx = widthPx, heightPx
	// TODO: mark swapchain out-of-date; recreateSwapchain() before next frame.
}

// PARTIAL-REDRAW HAZARD (must solve before this backend is correct): the frontend
// repaints only damaged rows and relies on the previous frame's pixels surviving in
// the drawable. Swapchain images ROTATE — an acquired image holds an OLDER or
// undefined frame, not the one just presented. A naive BeginFrame that render-passes
// straight into the acquired image will corrupt partial frames. Fix: keep a persistent
// offscreen color target, draw every frame (full or partial) into it, and blit/copy it
// to the acquired swapchain image before present. Clear() clears THAT target, not the
// swapchain image.
func (r *vulkanRenderer) BeginFrame(widthPx, heightPx int) {
	// TODO: acquire next image (wait inFlight fence), begin command buffer, begin
	// render pass (no clear), set viewport/scissor to (widthPx, heightPx), reset
	// the per-frame vertex writer.
}

func (r *vulkanRenderer) Clear(c color.RGBA) {
	// TODO: clear the current render target/attachment to c (e.g. vkCmdClearAttachments).
}

func (r *vulkanRenderer) FillRect(x, y, w, h float32, c color.RGBA) {
	// TODO: append two triangles (a quad) with color c to the solid vertex batch.
}

func (r *vulkanRenderer) DrawGlyph(page int, mode GlyphMode, x, y, w, h, skew float32, u0, v0, u1, v1 float32, c color.RGBA) {
	// TODO: append a textured quad (page's descriptor set) to the glyph batch,
	// selecting the blend/tint per mode (mask/color/subpixel).
}

func (r *vulkanRenderer) ConfigureAtlas(pageCount, sizePx int) {
	// TODO: (re)create pageCount sizePx×sizePx images + views + descriptor sets.
}

func (r *vulkanRenderer) UploadAtlasRegion(page, x, y, w, h int, rgba []byte) {
	// TODO: staging buffer → vkCmdCopyBufferToImage into page at (x,y);
	// transition to shader-read.
}

func (r *vulkanRenderer) ClearAtlasPage(page int) {
	// TODO: drain in-flight draws (fence/vkDeviceWaitIdle), then clear page's image.
}

func (r *vulkanRenderer) EndFrame() {
	// TODO: flush vertex buffers, record draw calls (bind pipeline+descriptors,
	// draw solid batch then glyph batch), end render pass + command buffer,
	// submit (wait imageAvailable, signal renderFinished, fence), queue present.
}

func (r *vulkanRenderer) Destroy() {
	// TODO: vkDeviceWaitIdle, then destroy every object in reverse creation order.
}
