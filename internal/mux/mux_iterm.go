package mux

import (
	"math"
	"time"

	"cervterm/internal/core"
	"cervterm/internal/itermimage"
	"cervterm/internal/termimage"
)

func (m *Mux) processITermOutcomes(p *pane) {
	m.protocolScheduling.dispatchITerm(muxProtocolSchedulingDispatchOperationAdapter{mux: m, pane: p})
}

func (a muxProtocolSchedulingDispatchOperationAdapter) dispatchITerm() {
	dispatchITermOperation(a.mux, a.pane)
}

func dispatchITermOperation(m *Mux, p *pane) {
	outcomes := p.itermOutcomes
	p.itermOutcomes = nil
	for _, outcome := range outcomes {
		if outcome.Failure != itermimage.FailureNone {
			if outcome.Command != nil {
				outcome.Command.Close()
			}
			m.emitImageDiagnostic(ImageDiagnosticProtocolITerm, itermDiagnosticReason(outcome.Failure), time.Time{}, time.Time{})
			continue
		}
		if outcome.Command != nil {
			m.submitITermDecode(p, outcome)
		}
	}
}

func (m *Mux) submitITermDecode(p *pane, outcome itermimage.Outcome) {
	if outcome.Command == nil {
		return
	}
	command := *outcome.Command
	startedAt := m.options.Now()
	if m.imageScheduler == nil || p == nil || p.imageStore == nil {
		command.Close()
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonFailed, startedAt)
		return
	}
	metrics, ok := m.resolveMetrics(p.id)
	if !ok {
		command.Close()
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonFailed, startedAt)
		return
	}
	if validateCellMetrics(metrics) != nil {
		command.Close()
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonInvalid, startedAt)
		return
	}
	if uint64(metrics.CellWidth) > math.MaxUint32 || uint64(metrics.CellHeight) > math.MaxUint32 {
		command.Close()
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonLimit, startedAt)
		return
	}
	job, failure := itermimage.NewDecodeJob(p.imageStore, command, itermimage.DecodeSpec{
		CellPixelWidth: uint32(metrics.CellWidth), CellPixelHeight: uint32(metrics.CellHeight),
	})
	if failure != itermimage.FailureNone {
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, itermDiagnosticReason(failure), startedAt)
		return
	}
	m.itermNextToken++
	owner := itermDecodeOwner{
		paneID: p.id, pane: p, model: m.model, store: p.imageStore, storeEpoch: p.imageStore.Epoch(),
		imageGeneration: p.terminal.ImageGeneration(), reflowGen: p.reflowGen, anchorGen: p.terminal.ImageAnchorGeneration(),
		token: m.itermNextToken, metrics: metrics, image: command.Image, placement: command.Placement,
		metadata: command.Metadata, startedAt: startedAt, acceptUntil: startedAt.Add(termimage.HardAcceptanceDeadline), anchor: p.terminal.ImageCursorAnchor(),
	}
	if err := m.imageScheduler.submitITerm(itermDecodeWork{owner: owner, job: job}); err != nil {
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonBusy, startedAt)
		return
	}
	m.itermPending[owner.token] = owner
}

func (m *Mux) applyITermCompletion(completion itermDecodeCompletion) []Event {
	owner := completion.Owner
	result := completion.Result
	pendingOwner, pending := m.itermPending[owner.token]
	if !pending || pendingOwner != owner {
		if result != nil {
			result.Close()
		}
		return nil
	}
	delete(m.itermPending, owner.token)
	if reason, failed := m.itermCompletionFailure(owner, result, completion.FinishedAt); failed {
		if result != nil {
			result.Close()
		}
		m.emitImageDiagnostic(ImageDiagnosticProtocolITerm, reason, owner.startedAt, completion.FinishedAt)
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
		m.emitImageDiagnosticNow(ImageDiagnosticProtocolITerm, ImageDiagnosticReasonFailed, owner.startedAt)
		return nil
	}
	owner.pane.capture()
	return m.ResolveEventAddresses([]Event{{Kind: PaneDirty, Pane: owner.paneID}})
}

func (m *Mux) itermCompletionFailure(owner itermDecodeOwner, result *itermimage.DecodeResult, finishedAt time.Time) (ImageDiagnosticReason, bool) {
	if finishedAt.IsZero() {
		return ImageDiagnosticReasonFailed, true
	}
	if !owner.startedAt.IsZero() && finishedAt.Before(owner.startedAt) {
		return ImageDiagnosticReasonStale, true
	}
	if !finishedAt.Before(owner.acceptUntil) {
		return ImageDiagnosticReasonTimeout, true
	}
	if result == nil {
		return ImageDiagnosticReasonFailed, true
	}
	if result.Failure != itermimage.FailureNone {
		return itermDiagnosticReason(result.Failure), true
	}
	if result.Candidate == nil {
		return ImageDiagnosticReasonFailed, true
	}
	p, ok := m.sessions.lookup(owner.paneID)
	if !ok || p != owner.pane || m.model != owner.model || m.model.tabForPane(owner.paneID) == nil ||
		p.imageStore != owner.store || owner.store == nil || owner.store.Closed() || owner.store.Epoch() != owner.storeEpoch ||
		p.terminal.ImageGeneration() != owner.imageGeneration || p.terminal.ImageAnchorGeneration() != owner.anchorGen || p.reflowGen != owner.reflowGen {
		return ImageDiagnosticReasonStale, true
	}
	metrics, ok := m.resolveMetrics(owner.paneID)
	if !ok || metrics != owner.metrics {
		return ImageDiagnosticReasonStale, true
	}
	if owner.image < termimage.MinInternalImageID || owner.placement < termimage.MinInternalPlacementID ||
		result.Candidate.Image() != owner.image || result.Candidate.Epoch() != owner.storeEpoch || !result.Candidate.ValidFor(owner.store) || !result.Candidate.WritesSealed() {
		return ImageDiagnosticReasonFailed, true
	}
	width, height, stride := result.Candidate.Dimensions()
	if width == 0 || height == 0 || uint64(stride) != uint64(width)*4 {
		return ImageDiagnosticReasonFailed, true
	}
	span, ok := expectedITermSpan(width, height, owner.metadata, owner.metrics)
	if !ok || result.Span != span {
		return ImageDiagnosticReasonFailed, true
	}
	return "", false
}

func itermDiagnosticReason(failure itermimage.Failure) ImageDiagnosticReason {
	switch failure {
	case itermimage.FailureInvalid:
		return ImageDiagnosticReasonInvalid
	case itermimage.FailureUnsupported:
		return ImageDiagnosticReasonUnsupported
	case itermimage.FailureLimit:
		return ImageDiagnosticReasonLimit
	case itermimage.FailureTimeout:
		return ImageDiagnosticReasonTimeout
	case itermimage.FailureCancelled:
		return ImageDiagnosticReasonCancelled
	default:
		return ImageDiagnosticReasonFailed
	}
}

func expectedITermSpan(width, height uint32, metadata itermimage.Metadata, metrics CellMetrics) (itermimage.Span, bool) {
	if width == 0 || height == 0 || metrics.CellWidth <= 0 || metrics.CellHeight <= 0 ||
		uint64(metrics.CellWidth) > uint64(termimage.HardImageDimension) || uint64(metrics.CellHeight) > uint64(termimage.HardImageDimension) ||
		metadata.Size == 0 || !metadata.PreserveAspectRatio {
		return itermimage.Span{}, false
	}
	cellWidth, cellHeight := uint64(metrics.CellWidth), uint64(metrics.CellHeight)
	var cols, rows uint64
	var ok bool
	switch metadata.Axis {
	case itermimage.SizingIntrinsic:
		if metadata.Cells != 0 {
			return itermimage.Span{}, false
		}
		cols, ok = checkedITermCeilDivide(uint64(width), cellWidth)
		if ok {
			rows, ok = checkedITermCeilDivide(uint64(height), cellHeight)
		}
	case itermimage.SizingWidth:
		cols = uint64(metadata.Cells)
		var numerator, denominator uint64
		numerator, ok = checkedITermProduct(uint64(height), cols, cellWidth)
		if ok {
			denominator, ok = checkedITermProduct(uint64(width), cellHeight)
		}
		if ok {
			rows, ok = checkedITermCeilDivide(numerator, denominator)
		}
	case itermimage.SizingHeight:
		rows = uint64(metadata.Cells)
		var numerator, denominator uint64
		numerator, ok = checkedITermProduct(uint64(width), rows, cellHeight)
		if ok {
			denominator, ok = checkedITermProduct(uint64(height), cellWidth)
		}
		if ok {
			cols, ok = checkedITermCeilDivide(numerator, denominator)
		}
	default:
		return itermimage.Span{}, false
	}
	if !ok || cols == 0 || rows == 0 || cols > uint64(termimage.HardPlacementSpan) || rows > uint64(termimage.HardPlacementSpan) {
		return itermimage.Span{}, false
	}
	return itermimage.Span{Cols: uint32(cols), Rows: uint32(rows)}, true
}

func checkedITermProduct(values ...uint64) (uint64, bool) {
	product := uint64(1)
	for _, value := range values {
		if value != 0 && product > math.MaxUint64/value {
			return 0, false
		}
		product *= value
	}
	return product, true
}

func checkedITermCeilDivide(numerator, denominator uint64) (uint64, bool) {
	if denominator == 0 {
		return 0, false
	}
	quotient := numerator / denominator
	if numerator%denominator == 0 {
		return quotient, true
	}
	if quotient == math.MaxUint64 {
		return 0, false
	}
	return quotient + 1, true
}
