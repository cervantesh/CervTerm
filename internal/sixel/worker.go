package sixel

import (
	"context"
	"io"
	"sync/atomic"

	"cervterm/internal/termimage"
)

const (
	maxDimension    = 4096
	maxPixels       = 16_777_216
	maxDecodedBytes = 64 * 1024 * 1024
	maxOperations   = 4_194_304
	maxCellSpan     = 256
)

type RGBA struct{ R, G, B, A uint8 }
type Palette [256]RGBA
type DecodeSpec struct {
	CellPixelWidth, CellPixelHeight uint32
	Palette                         Palette
}
type Span struct{ Cols, Rows uint32 }
type DecodeResult struct {
	Candidate *termimage.DecodedCandidate
	Span      Span
	Failure   Failure
}

func (r *DecodeResult) Close() {
	if r != nil && r.Candidate != nil {
		r.Candidate.Close()
		r.Candidate = nil
	}
}

type DecodeJob struct {
	store   *termimage.Store
	command Command
	spec    DecodeSpec
	state   atomic.Uint32
}

func NewDecodeJob(store *termimage.Store, command Command, spec DecodeSpec) (*DecodeJob, Failure) {
	if store == nil || command.Transfer == nil || command.Image < termimage.MinInternalImageID || command.Placement < termimage.MinInternalPlacementID || spec.CellPixelWidth == 0 || spec.CellPixelHeight == 0 {
		command.Close()
		return nil, FailureInvalid
	}
	return &DecodeJob{store: store, command: command, spec: spec}, FailureNone
}
func (j *DecodeJob) Close() {
	if j == nil || !j.state.CompareAndSwap(0, 2) {
		return
	}
	j.command.Close()
}
func (j *DecodeJob) Run(ctx context.Context) *DecodeResult {
	if j == nil || !j.state.CompareAndSwap(0, 1) {
		return &DecodeResult{Failure: FailureCancelled}
	}
	transfer := j.command.Transfer
	j.command.Transfer = nil
	payload, err := transfer.TakeSealedPayload(j.store)
	if err != nil {
		transfer.Close()
		return &DecodeResult{Failure: FailureCancelled}
	}
	defer payload.Close()
	if err = ctx.Err(); err != nil {
		return &DecodeResult{Failure: FailureCancelled}
	}
	scratch, err := j.store.ReserveDecodeScratch(payload.EncodedLen())
	if err != nil {
		return &DecodeResult{Failure: mapStoreError(err)}
	}
	defer scratch.Close()
	data, err := io.ReadAll(io.LimitReader(payload.Reader(), maxFrameBytes+1))
	if err != nil || len(data) > maxFrameBytes {
		return &DecodeResult{Failure: FailureLimit}
	}
	plan, failure := validateDecode(ctx, data, j.command.Raster, j.spec)
	if failure != FailureNone {
		return &DecodeResult{Failure: failure}
	}
	if err = ctx.Err(); err != nil {
		return &DecodeResult{Failure: FailureCancelled}
	}
	candidate, err := j.store.NewDecodedCandidate(j.command.Image, plan.width, plan.height)
	if err != nil {
		return &DecodeResult{Failure: mapStoreError(err)}
	}
	if failure = renderDecode(ctx, data, plan, j.spec.Palette, candidate); failure != FailureNone {
		candidate.Close()
		return &DecodeResult{Failure: failure}
	}
	if err = ctx.Err(); err != nil {
		candidate.Close()
		return &DecodeResult{Failure: FailureCancelled}
	}
	if err = candidate.SealWrites(); err != nil {
		candidate.Close()
		return &DecodeResult{Failure: FailureFailed}
	}
	if candidate.Epoch() != payload.Epoch() || !candidate.ValidFor(j.store) {
		candidate.Close()
		return &DecodeResult{Failure: FailureCancelled}
	}
	return &DecodeResult{Candidate: candidate, Span: plan.span}
}

type decodePlan struct {
	width, height uint32
	span          Span
}

func validateDecode(ctx context.Context, data []byte, raster Raster, spec DecodeSpec) (decodePlan, Failure) {
	if raster.Width == 0 || raster.Height == 0 || raster.Width > maxDimension || raster.Height > maxDimension {
		return decodePlan{}, FailureLimit
	}
	pixels := uint64(raster.Width) * uint64(raster.Height)
	if pixels > maxPixels || pixels*4 > maxDecodedBytes {
		return decodePlan{}, FailureLimit
	}
	cols := (uint64(raster.Width) + uint64(spec.CellPixelWidth) - 1) / uint64(spec.CellPixelWidth)
	rows := (uint64(raster.Height) + uint64(spec.CellPixelHeight) - 1) / uint64(spec.CellPixelHeight)
	if cols == 0 || rows == 0 || cols > maxCellSpan || rows > maxCellSpan {
		return decodePlan{}, FailureLimit
	}
	plan := decodePlan{width: raster.Width, height: raster.Height, span: Span{Cols: uint32(cols), Rows: uint32(rows)}}
	walker := decodeWalker{width: plan.width, height: plan.height, limitOnly: true, ctx: ctx}
	declared, failure := walkTokens(data, &walker)
	if failure != FailureNone {
		return decodePlan{}, failure
	}
	if declared != raster {
		return decodePlan{}, FailureInvalid
	}
	return plan, FailureNone
}
func renderDecode(ctx context.Context, data []byte, plan decodePlan, palette Palette, candidate *termimage.DecodedCandidate) Failure {
	walker := decodeWalker{width: plan.width, height: plan.height, palette: palette, candidate: candidate, ctx: ctx}
	_, failure := walkTokens(data, &walker)
	return failure
}
