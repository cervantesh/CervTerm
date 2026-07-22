package mux

import (
	"math"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/sixel"
	"cervterm/internal/termimage"
)

func (m *Mux) processSixelOutcomes(p *pane) {
	outcomes := p.sixelOutcomes
	p.sixelOutcomes = nil
	for _, outcome := range outcomes {
		if outcome.Failure != sixel.FailureNone {
			if outcome.Command != nil {
				outcome.Command.Close()
			}
			continue
		}
		if outcome.Command != nil {
			m.submitSixelDecode(p, outcome)
		}
	}
}

func (m *Mux) submitSixelDecode(p *pane, outcome sixel.Outcome) {
	if outcome.Command == nil {
		return
	}
	command := *outcome.Command
	if m.imageScheduler == nil || p == nil || p.imageStore == nil {
		command.Close()
		return
	}
	metrics, ok := m.resolveMetrics(p.id)
	if !ok || validateCellMetrics(metrics) != nil || uint64(metrics.CellWidth) > math.MaxUint32 || uint64(metrics.CellHeight) > math.MaxUint32 {
		command.Close()
		return
	}
	palette := detachedSixelPalette(p.terminal)
	job, failure := sixel.NewDecodeJob(p.imageStore, command, sixel.DecodeSpec{
		CellPixelWidth: uint32(metrics.CellWidth), CellPixelHeight: uint32(metrics.CellHeight), Palette: palette,
	})
	if failure != sixel.FailureNone {
		return
	}
	m.sixelNextToken++
	anchor := p.terminal.ImageCursorAnchor()
	owner := sixelDecodeOwner{
		paneID: p.id, pane: p, model: m.model, store: p.imageStore, storeEpoch: p.imageStore.Epoch(),
		imageGeneration: p.terminal.ImageGeneration(), reflowGen: p.reflowGen, anchorGen: p.terminal.ImageAnchorGeneration(),
		token: m.sixelNextToken, metrics: metrics, image: command.Image, placement: command.Placement,
		raster: command.Raster, acceptUntil: m.options.Now().Add(termimage.HardAcceptanceDeadline), anchor: anchor,
	}
	if err := m.imageScheduler.submitSixel(sixelDecodeWork{owner: owner, job: job}); err != nil {
		return
	}
	m.sixelPending[owner.token] = owner
}

func detachedSixelPalette(terminal *core.Terminal) sixel.Palette {
	var palette sixel.Palette
	if terminal == nil {
		return palette
	}
	for index := range palette {
		rgb := terminal.EffectivePaletteIndex(uint8(index))
		palette[index] = sixel.RGBA{R: rgb.R, G: rgb.G, B: rgb.B, A: 255}
	}
	return palette
}

func (m *Mux) applySixelCompletion(completion sixelDecodeCompletion) []Event {
	owner := completion.Owner
	result := completion.Result
	pendingOwner, pending := m.sixelPending[owner.token]
	if !pending || pendingOwner != owner {
		if result != nil {
			result.Close()
		}
		return nil
	}
	delete(m.sixelPending, owner.token)
	if !m.sixelCompletionCurrent(owner, result, completion.FinishedAt) {
		if result != nil {
			result.Close()
		}
		return nil
	}
	placement := termimage.PlacementSpec{
		ID: owner.placement, Anchor: owner.anchor, Cols: uint16(result.Span.Cols), Rows: uint16(result.Span.Rows), Opacity: 255,
	}
	candidate := result.Candidate
	_, err := owner.pane.terminal.CommitImage(core.ImageCommit{
		Candidate: candidate, Placement: &placement, Retention: termimage.ResourceEphemeral,
	})
	result.Candidate = nil // CommitImage consumes the candidate on success and failure.
	if err != nil {
		return nil
	}
	owner.pane.capture()
	return m.ResolveEventAddresses([]Event{{Kind: PaneDirty, Pane: owner.paneID}})
}

func (m *Mux) sixelCompletionCurrent(owner sixelDecodeOwner, result *sixel.DecodeResult, finishedAt time.Time) bool {
	p, ok := m.sessions.lookup(owner.paneID)
	if !ok || p != owner.pane || m.model != owner.model || m.model.tabForPane(owner.paneID) == nil ||
		p.imageStore != owner.store || owner.store == nil || owner.store.Closed() || owner.store.Epoch() != owner.storeEpoch ||
		p.terminal.ImageGeneration() != owner.imageGeneration || p.terminal.ImageAnchorGeneration() != owner.anchorGen || p.reflowGen != owner.reflowGen ||
		finishedAt.IsZero() || !finishedAt.Before(owner.acceptUntil) || result == nil || result.Failure != sixel.FailureNone || result.Candidate == nil {
		return false
	}
	metrics, ok := m.resolveMetrics(owner.paneID)
	if !ok || metrics != owner.metrics {
		return false
	}
	if owner.image < termimage.MinInternalImageID || owner.placement < termimage.MinInternalPlacementID ||
		result.Candidate.Image() != owner.image || result.Candidate.Epoch() != owner.storeEpoch || !result.Candidate.ValidFor(owner.store) || !result.Candidate.WritesSealed() {
		return false
	}
	width, height, stride := result.Candidate.Dimensions()
	if width != owner.raster.Width || height != owner.raster.Height || uint64(stride) != uint64(width)*4 {
		return false
	}
	span, ok := expectedSixelSpan(owner.raster, owner.metrics)
	return ok && result.Span == span
}

func expectedSixelSpan(raster sixel.Raster, metrics CellMetrics) (sixel.Span, bool) {
	if raster.Width == 0 || raster.Height == 0 || metrics.CellWidth <= 0 || metrics.CellHeight <= 0 ||
		uint64(metrics.CellWidth) > math.MaxUint32 || uint64(metrics.CellHeight) > math.MaxUint32 {
		return sixel.Span{}, false
	}
	cols := (uint64(raster.Width) + uint64(metrics.CellWidth) - 1) / uint64(metrics.CellWidth)
	rows := (uint64(raster.Height) + uint64(metrics.CellHeight) - 1) / uint64(metrics.CellHeight)
	if cols == 0 || rows == 0 || cols > uint64(termimage.HardPlacementSpan) || rows > uint64(termimage.HardPlacementSpan) {
		return sixel.Span{}, false
	}
	return sixel.Span{Cols: uint32(cols), Rows: uint32(rows)}, true
}
