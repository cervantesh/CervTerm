package itermimage

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"sync/atomic"

	"cervterm/internal/termimage"
)

const maxCellPixelDimension uint32 = termimage.HardImageDimension

type DecodeSpec struct {
	CellPixelWidth, CellPixelHeight uint32
}

type Span struct {
	Cols, Rows uint32
}

type DecodeResult struct {
	Candidate *termimage.DecodedCandidate
	Span      Span
	Failure   Failure
}

func (r *DecodeResult) Close() {
	if r == nil || r.Candidate == nil {
		return
	}
	r.Candidate.Close()
	r.Candidate = nil
}

type DecodeJob struct {
	store   *termimage.Store
	command Command
	spec    DecodeSpec
	state   atomic.Uint32
}

func NewDecodeJob(store *termimage.Store, command Command, spec DecodeSpec) (*DecodeJob, Failure) {
	failure := validateJob(store, command, spec)
	if failure != FailureNone {
		command.Close()
		return nil, failure
	}
	return &DecodeJob{store: store, command: command, spec: spec}, FailureNone
}

func validateJob(store *termimage.Store, command Command, spec DecodeSpec) Failure {
	if store == nil || command.Transfer == nil || command.Image < termimage.MinInternalImageID || command.Placement < termimage.MinInternalPlacementID ||
		spec.CellPixelWidth == 0 || spec.CellPixelHeight == 0 {
		return FailureInvalid
	}
	if spec.CellPixelWidth > maxCellPixelDimension || spec.CellPixelHeight > maxCellPixelDimension {
		return FailureLimit
	}
	return validateMetadata(command.Metadata)
}

func validateMetadata(metadata Metadata) Failure {
	if metadata.Size == 0 || !metadata.PreserveAspectRatio {
		return FailureInvalid
	}
	switch metadata.Axis {
	case SizingIntrinsic:
		if metadata.Cells != 0 {
			return FailureInvalid
		}
	case SizingWidth, SizingHeight:
		if metadata.Cells == 0 {
			return FailureInvalid
		}
		if metadata.Cells > termimage.HardPlacementSpan {
			return FailureLimit
		}
	default:
		return FailureInvalid
	}
	return FailureNone
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
	if ctx == nil {
		return &DecodeResult{Failure: FailureInvalid}
	}
	if err = ctx.Err(); err != nil {
		return &DecodeResult{Failure: FailureCancelled}
	}
	header := payload.Header()
	if header.Image != j.command.Image || header.Transfer != termimage.TransferID(j.command.Image) || payload.EncodedLen() == 0 || payload.EncodedLen()%4 != 0 {
		return &DecodeResult{Failure: FailureInvalid}
	}

	var decodedReaders []*countingReader
	candidate, err := termimage.DecodePNG(ctx, j.store, j.command.Image, func() (io.Reader, error) {
		reader, openErr := strictBase64Reader(ctx, payload)
		if openErr != nil {
			return nil, openErr
		}
		counted := &countingReader{reader: reader}
		decodedReaders = append(decodedReaders, counted)
		return counted, nil
	})
	if err != nil {
		return &DecodeResult{Failure: mapDecodeError(err)}
	}
	if len(decodedReaders) != 2 || decodedReaders[1].overflow || decodedReaders[1].count != j.command.Metadata.Size {
		candidate.Close()
		return &DecodeResult{Failure: FailureInvalid}
	}
	width, height, stride := candidate.Dimensions()
	if stride == 0 {
		candidate.Close()
		return &DecodeResult{Failure: FailureInvalid}
	}
	span, failure := deriveSpan(width, height, j.command.Metadata, j.spec)
	if failure != FailureNone {
		candidate.Close()
		return &DecodeResult{Failure: failure}
	}
	if err = ctx.Err(); err != nil || candidate.Epoch() != payload.Epoch() || !candidate.ValidFor(j.store) || !candidate.WritesSealed() {
		candidate.Close()
		return &DecodeResult{Failure: FailureCancelled}
	}
	return &DecodeResult{Candidate: candidate, Span: span}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(dst []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(dst)
}

type base64AlphabetReader struct {
	reader io.Reader
}

func (r *base64AlphabetReader) Read(dst []byte) (int, error) {
	n, err := r.reader.Read(dst)
	for _, value := range dst[:n] {
		if !isBase64Byte(value) {
			return 0, termimage.ErrCandidateInvalid
		}
	}
	return n, err
}

func isBase64Byte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '+' || value == '/' || value == '='
}

func strictBase64Reader(ctx context.Context, payload *termimage.SealedEncodedPayload) (io.Reader, error) {
	if payload == nil || payload.EncodedLen() == 0 || payload.EncodedLen()%4 != 0 {
		return nil, termimage.ErrCandidateInvalid
	}
	encoded := contextReader{ctx: ctx, reader: payload.Reader()}
	alphabet := &base64AlphabetReader{reader: encoded}
	decoded := base64.NewDecoder(base64.StdEncoding.Strict(), alphabet)
	return contextReader{ctx: ctx, reader: decoded}, nil
}

type countingReader struct {
	reader   io.Reader
	count    uint64
	overflow bool
}

func (r *countingReader) Read(dst []byte) (int, error) {
	n, err := r.reader.Read(dst)
	if n < 0 || uint64(n) > math.MaxUint64-r.count {
		r.overflow = true
		return 0, termimage.ErrLimitExceeded
	}
	r.count += uint64(n)
	return n, err
}

func deriveSpan(width, height uint32, metadata Metadata, spec DecodeSpec) (Span, Failure) {
	if width == 0 || height == 0 || spec.CellPixelWidth == 0 || spec.CellPixelHeight == 0 {
		return Span{}, FailureInvalid
	}
	if spec.CellPixelWidth > maxCellPixelDimension || spec.CellPixelHeight > maxCellPixelDimension {
		return Span{}, FailureLimit
	}
	if failure := validateMetadata(metadata); failure != FailureNone {
		return Span{}, failure
	}

	var cols, rows uint64
	var ok bool
	switch metadata.Axis {
	case SizingIntrinsic:
		cols, ok = checkedCeilDivide(uint64(width), uint64(spec.CellPixelWidth))
		if !ok {
			return Span{}, FailureLimit
		}
		rows, ok = checkedCeilDivide(uint64(height), uint64(spec.CellPixelHeight))
	case SizingWidth:
		cols = uint64(metadata.Cells)
		var numerator, denominator uint64
		numerator, ok = checkedProduct(uint64(height), cols, uint64(spec.CellPixelWidth))
		if ok {
			denominator, ok = checkedProduct(uint64(width), uint64(spec.CellPixelHeight))
		}
		if ok {
			rows, ok = checkedCeilDivide(numerator, denominator)
		}
	case SizingHeight:
		rows = uint64(metadata.Cells)
		var numerator, denominator uint64
		numerator, ok = checkedProduct(uint64(width), rows, uint64(spec.CellPixelHeight))
		if ok {
			denominator, ok = checkedProduct(uint64(height), uint64(spec.CellPixelWidth))
		}
		if ok {
			cols, ok = checkedCeilDivide(numerator, denominator)
		}
	default:
		return Span{}, FailureInvalid
	}
	if !ok || cols == 0 || rows == 0 || cols > uint64(termimage.HardPlacementSpan) || rows > uint64(termimage.HardPlacementSpan) {
		return Span{}, FailureLimit
	}
	return Span{Cols: uint32(cols), Rows: uint32(rows)}, FailureNone
}

func checkedProduct(values ...uint64) (uint64, bool) {
	product := uint64(1)
	for _, value := range values {
		if value != 0 && product > math.MaxUint64/value {
			return 0, false
		}
		product *= value
	}
	return product, true
}

func checkedCeilDivide(numerator, denominator uint64) (uint64, bool) {
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

func mapDecodeError(err error) Failure {
	switch {
	case err == nil:
		return FailureNone
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, termimage.ErrClosed), errors.Is(err, termimage.ErrTransferClosed):
		return FailureCancelled
	case errors.Is(err, termimage.ErrLimitExceeded):
		return FailureLimit
	case errors.Is(err, termimage.ErrCandidateInvalid), errors.Is(err, termimage.ErrInvalidChunk):
		return FailureInvalid
	default:
		return FailureFailed
	}
}
