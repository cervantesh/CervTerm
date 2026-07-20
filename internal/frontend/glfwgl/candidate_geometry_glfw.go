//go:build glfw

package glfwgl

func (a *App) setCandidateGeometryCallbacks(publish func(nativeCandidateRect) error, clear func() error) error {
	if err := a.candidateGeometry.setCallbacks(publish, clear); err != nil {
		return err
	}
	a.requestRedraw()
	return nil
}

func (a *App) invalidateCandidateGeometry() {
	a.candidateGeometry.invalidate()
}

func (a *App) beginCandidateGeometryFrame() {
	a.candidateGeometry.beginFrame()
	if !a.composition.snapshot().Active {
		_ = a.candidateGeometry.hide()
	}
}

func (a *App) finishCandidateGeometryFrame() {
	if a.composition.snapshot().Active && a.candidateGeometry.wasVisible && !a.candidateGeometry.presentedThisFrame() {
		_ = a.candidateGeometry.hide()
	}
}

func (a *App) publishCandidateGeometry(presentation preeditPresentation, x, y float32) {
	if a.window == nil {
		return
	}
	caret, ok := preeditCaretFramebufferRect(presentation, x, y, a.cellW, a.cellH, max(1, a.uiScale))
	if !ok {
		_ = a.candidateGeometry.hide()
		return
	}
	framebufferWidth, framebufferHeight := a.window.GetFramebufferSize()
	windowWidth, windowHeight := a.window.GetSize()
	rect, err := projectCandidateRect(caret, framebufferWidth, framebufferHeight, windowWidth, windowHeight)
	if err != nil {
		_ = a.candidateGeometry.hide()
		return
	}
	if err := a.candidateGeometry.publishChanged(rect); err == nil {
		a.candidateGeometry.markPresented()
	}
}
