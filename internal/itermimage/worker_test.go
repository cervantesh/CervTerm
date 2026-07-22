package itermimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cervterm/internal/termimage"
)

func workerPNG(t testing.TB, width, height int, noisy bool) ([]byte, []byte) {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := color.NRGBA{R: uint8(x*17 + y*3), G: uint8(x*5 + y*29), B: uint8(x*11 + y*7), A: 0xff}
			if !noisy {
				value = color.NRGBA{R: 0x19, G: 0x37, B: 0x5b, A: 0xd3}
			}
			pixels.SetNRGBA(x, y, value)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes(), append([]byte(nil), pixels.Pix...)
}

func sealedWorkerCommand(t testing.TB, store *termimage.Store, raw []byte, axis SizingAxis, cells uint16) Command {
	t.Helper()
	metadata := "File=inline=1;size=" + strconv.Itoa(len(raw))
	switch axis {
	case SizingWidth:
		metadata += ";width=" + strconv.Itoa(int(cells))
	case SizingHeight:
		metadata += ";height=" + strconv.Itoa(int(cells))
	}
	input := []byte(metadata + ":" + base64.StdEncoding.EncodeToString(raw))
	adapter := NewAdapter(store)
	defer adapter.Close()
	var out Outcome
	for len(input) != 0 {
		size := min(len(input), int(termimage.HardControlChunkBytes))
		out = adapter.Advance(time.Now(), OSCEvent{Data: input[:size], Final: size == len(input)})
		if out.Failure != FailureNone {
			t.Fatalf("adapter failure: %#v", out)
		}
		input = input[size:]
	}
	if out.Command == nil {
		t.Fatal("adapter did not seal command")
	}
	command := *out.Command
	out.Command.Transfer = nil
	return command
}

func directWorkerCommand(t testing.TB, store *termimage.Store, encoded []byte, metadata Metadata) Command {
	t.Helper()
	imageID, err := store.AllocateInternalImageID()
	if err != nil {
		t.Fatal(err)
	}
	placementID, err := store.AllocateInternalPlacementID()
	if err != nil {
		t.Fatal(err)
	}
	return directWorkerCommandWithHeader(t, store, encoded, metadata, imageID, imageID, placementID)
}

func directWorkerCommandWithHeader(t testing.TB, store *termimage.Store, encoded []byte, metadata Metadata, commandImage, headerImage termimage.ImageID, placement termimage.PlacementID) Command {
	t.Helper()
	transfer, err := store.BeginTransfer(termimage.Header{Transfer: termimage.TransferID(commandImage), Image: headerImage})
	if err != nil {
		t.Fatal(err)
	}
	for len(encoded) != 0 {
		size := min(len(encoded), int(termimage.HardControlChunkBytes))
		if err = transfer.Append(encoded[:size]); err != nil {
			transfer.Close()
			t.Fatal(err)
		}
		encoded = encoded[size:]
	}
	if err = transfer.Seal(); err != nil {
		transfer.Close()
		t.Fatal(err)
	}
	return Command{Image: commandImage, Placement: placement, Metadata: metadata, Transfer: transfer}
}

func runWorker(t testing.TB, store *termimage.Store, command Command, spec DecodeSpec, ctx context.Context) *DecodeResult {
	t.Helper()
	job, failure := NewDecodeJob(store, command, spec)
	if failure != FailureNone {
		t.Fatalf("new job failure: %v", failure)
	}
	return job.Run(ctx)
}

func TestITermDecodeWorkerGoldenPixelsAndSpans(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		axis          SizingAxis
		cells         uint16
		cellW, cellH  uint32
		want          Span
	}{
		{name: "intrinsic ceil", width: 13, height: 17, axis: SizingIntrinsic, cellW: 8, cellH: 16, want: Span{Cols: 2, Rows: 2}},
		{name: "width aspect ceil", width: 7, height: 5, axis: SizingWidth, cells: 3, cellW: 2, cellH: 3, want: Span{Cols: 3, Rows: 2}},
		{name: "height aspect ceil", width: 5, height: 7, axis: SizingHeight, cells: 3, cellW: 3, cellH: 2, want: Span{Cols: 2, Rows: 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			raw, wantRGBA := workerPNG(t, test.width, test.height, true)
			command := sealedWorkerCommand(t, store, raw, test.axis, test.cells)
			result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: test.cellW, CellPixelHeight: test.cellH}, context.Background())
			if result.Failure != FailureNone || result.Candidate == nil || result.Span != test.want {
				t.Fatalf("result=%#v want span=%#v", result, test.want)
			}
			if got := result.Candidate.RGBA(); !bytes.Equal(got, wantRGBA) {
				t.Fatalf("RGBA mismatch: got %d bytes want %d", len(got), len(wantRGBA))
			}
			if width, height, stride := result.Candidate.Dimensions(); width != uint32(test.width) || height != uint32(test.height) || stride != uint32(test.width*4) {
				t.Fatalf("dimensions=%dx%d stride=%d", width, height, stride)
			}
			if !result.Candidate.WritesSealed() || result.Candidate.WriteRGBAAt(0, []byte{0}) == nil {
				t.Fatal("candidate is not immutable")
			}
			copyOfPixels := result.Candidate.RGBA()
			copyOfPixels[0] ^= 0xff
			if bytes.Equal(copyOfPixels, result.Candidate.RGBA()) {
				t.Fatal("RGBA accessor aliased candidate ownership")
			}
			wantUsage := termimage.Usage{DecodedBytes: uint64(test.width * test.height * 4), Images: 1}
			if store.Usage() != wantUsage || process.Usage() != wantUsage {
				t.Fatalf("transfer or scratch retained: pane=%#v process=%#v want=%#v", store.Usage(), process.Usage(), wantUsage)
			}
			result.Close()
			result.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func TestITermDecodeWorkerRejectsHostileAlphabetAndPadding(t *testing.T) {
	raw, _ := pngWithPadding(t)
	valid := base64.StdEncoding.EncodeToString(raw)
	invalidAlphabet := []byte(valid)
	invalidAlphabet[len(invalidAlphabet)/2] = '!'
	cases := map[string][]byte{
		"alphabet":       invalidAlphabet,
		"newline":        append(append([]byte(nil), valid[:4]...), append([]byte("\r\n\r\n"), valid[4:]...)...),
		"missing pad":    []byte(strings.TrimSuffix(valid, "=")),
		"extra pad":      []byte(valid + "===="),
		"interior pad":   append([]byte{'='}, []byte(valid[1:])...),
		"nonzero padbit": []byte(corruptPadBits(t, valid)),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			command := directWorkerCommand(t, store, encoded, validWorkerMetadata(uint64(len(raw))))
			result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16}, context.Background())
			if result.Failure != FailureInvalid || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func pngWithPadding(t testing.TB) ([]byte, []byte) {
	t.Helper()
	for width := 1; width <= 8; width++ {
		raw, rgba := workerPNG(t, width, 1, true)
		if len(raw)%3 != 0 {
			return raw, rgba
		}
	}
	t.Fatal("could not generate padded PNG fixture")
	return nil, nil
}

func corruptPadBits(t testing.TB, encoded string) string {
	t.Helper()
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := []byte(encoded)
	index := -1
	mask := byte(0)
	switch {
	case strings.HasSuffix(encoded, "=="):
		index, mask = len(result)-3, 0x0f
	case strings.HasSuffix(encoded, "="):
		index, mask = len(result)-2, 0x03
	default:
		t.Fatal("fixture has no padding")
	}
	value := strings.IndexByte(alphabet, result[index])
	if value < 0 || byte(value)&mask != 0 {
		t.Fatal("fixture padding bits are not canonical")
	}
	result[index] = alphabet[value|1]
	return string(result)
}

func validWorkerMetadata(size uint64) Metadata {
	return Metadata{Size: size, Axis: SizingIntrinsic, PreserveAspectRatio: true}
}

func TestITermDecodeWorkerRequiresExactDeclaredSizeAndOnePNG(t *testing.T) {
	raw, _ := workerPNG(t, 2, 2, true)
	second, _ := workerPNG(t, 1, 1, false)
	cases := []struct {
		name string
		raw  []byte
		size uint64
	}{
		{name: "declared short", raw: raw, size: uint64(len(raw) - 1)},
		{name: "declared long", raw: raw, size: uint64(len(raw) + 1)},
		{name: "trailing byte exact size", raw: append(append([]byte(nil), raw...), 0), size: uint64(len(raw) + 1)},
		{name: "trailing PNG exact size", raw: append(append([]byte(nil), raw...), second...), size: uint64(len(raw) + len(second))},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			encoded := []byte(base64.StdEncoding.EncodeToString(test.raw))
			command := directWorkerCommand(t, store, encoded, validWorkerMetadata(test.size))
			result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1}, context.Background())
			if result.Failure != FailureInvalid || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}
}

func TestITermDecodeWorkerRejectsPNGBombsAndRollsBack(t *testing.T) {
	for name, raw := range map[string][]byte{
		"dimension": pngConfigOnly(4097, 1),
		"scratch":   pngConfigOnly(4096, 4096),
	} {
		t.Run(name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			command := directWorkerCommand(t, store, []byte(base64.StdEncoding.EncodeToString(raw)), validWorkerMetadata(uint64(len(raw))))
			result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 16, CellPixelHeight: 16}, context.Background())
			if result.Failure != FailureLimit || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}

	t.Run("pane scratch budget", func(t *testing.T) {
		process := termimage.NewProcessBudget()
		limits := termimage.DefaultLimits()
		limits.DecodedBytes = 1024
		store := termimage.NewStore(process, limits)
		raw, _ := workerPNG(t, 64, 64, false)
		command := sealedWorkerCommand(t, store, raw, SizingIntrinsic, 0)
		result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16}, context.Background())
		if result.Failure != FailureLimit || result.Candidate != nil {
			t.Fatalf("result=%#v", result)
		}
		result.Close()
		requireNoUsage(t, process, store)
	})
}

func pngConfigOnly(width, height uint32) []byte {
	var result bytes.Buffer
	result.Write([]byte("\x89PNG\r\n\x1a\n"))
	data := make([]byte, 13)
	binary.BigEndian.PutUint32(data[0:4], width)
	binary.BigEndian.PutUint32(data[4:8], height)
	data[8], data[9] = 8, 6
	writePNGChunk(&result, "IHDR", data)
	return result.Bytes()
}

func writePNGChunk(dst *bytes.Buffer, name string, data []byte) {
	_ = binary.Write(dst, binary.BigEndian, uint32(len(data)))
	dst.WriteString(name)
	dst.Write(data)
	checksum := crc32.NewIEEE()
	_, _ = checksum.Write([]byte(name))
	_, _ = checksum.Write(data)
	_ = binary.Write(dst, binary.BigEndian, checksum.Sum32())
}

func TestITermDecodeWorkerAspectBoundsAndCheckedArithmetic(t *testing.T) {
	validCases := []struct {
		name string
		axis SizingAxis
		want Span
	}{
		{name: "width inclusive 256", axis: SizingWidth, want: Span{Cols: 256, Rows: 256}},
		{name: "height inclusive 256", axis: SizingHeight, want: Span{Cols: 256, Rows: 256}},
	}
	for _, test := range validCases {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			raw, _ := workerPNG(t, 1, 1, false)
			result := runWorker(t, store, sealedWorkerCommand(t, store, raw, test.axis, 256), DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1}, context.Background())
			if result.Failure != FailureNone || result.Span != test.want {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}

	rejectCases := []struct {
		name          string
		width, height int
		axis          SizingAxis
		cells         uint16
	}{
		{name: "intrinsic cols", width: 257, height: 1, axis: SizingIntrinsic},
		{name: "width derived rows", width: 1, height: 2, axis: SizingWidth, cells: 256},
		{name: "height derived cols", width: 2, height: 1, axis: SizingHeight, cells: 256},
	}
	for _, test := range rejectCases {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			raw, _ := workerPNG(t, test.width, test.height, false)
			result := runWorker(t, store, sealedWorkerCommand(t, store, raw, test.axis, test.cells), DecodeSpec{CellPixelWidth: 1, CellPixelHeight: 1}, context.Background())
			if result.Failure != FailureLimit || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}

	if _, ok := checkedProduct(math.MaxUint64, 2); ok {
		t.Fatal("multiply overflow accepted")
	}
	if _, ok := checkedCeilDivide(1, 0); ok {
		t.Fatal("zero divisor accepted")
	}
}

func TestITermDecodeWorkerCancellationAndExactOwnership(t *testing.T) {
	raw, _ := workerPNG(t, 256, 256, true)
	for _, checks := range []int32{1, 5, 20, 100} {
		t.Run(strconv.Itoa(int(checks)), func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			command := sealedWorkerCommand(t, store, raw, SizingIntrinsic, 0)
			ctx := newCancelAfterContext(checks)
			result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16}, ctx)
			if result.Failure != FailureCancelled || result.Candidate != nil {
				t.Fatalf("result=%#v", result)
			}
			result.Close()
			requireNoUsage(t, process, store)
		})
	}

	t.Run("close before run", func(t *testing.T) {
		process, store, _ := newAdapterTestStore()
		command := sealedWorkerCommand(t, store, raw, SizingIntrinsic, 0)
		job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16})
		if failure != FailureNone {
			t.Fatal(failure)
		}
		job.Close()
		job.Close()
		if result := job.Run(context.Background()); result.Failure != FailureCancelled || result.Candidate != nil {
			t.Fatalf("result=%#v", result)
		}
		requireNoUsage(t, process, store)
	})

	t.Run("reset before run", func(t *testing.T) {
		process, store, _ := newAdapterTestStore()
		command := sealedWorkerCommand(t, store, raw, SizingIntrinsic, 0)
		job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16})
		if failure != FailureNone {
			t.Fatal(failure)
		}
		store.Reset()
		result := job.Run(context.Background())
		if result.Failure != FailureCancelled || result.Candidate != nil {
			t.Fatalf("result=%#v", result)
		}
		result.Close()
		requireNoUsage(t, process, store)
	})
}

type cancelAfterContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining atomic.Int32
}

func newCancelAfterContext(checks int32) *cancelAfterContext {
	ctx, cancel := context.WithCancel(context.Background())
	result := &cancelAfterContext{Context: ctx, cancel: cancel}
	result.remaining.Store(checks)
	return result
}

func (c *cancelAfterContext) Err() error {
	if c.remaining.Add(-1) <= 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestITermDecodeWorkerRejectsMetadataHeaderAndMetricBypass(t *testing.T) {
	raw, _ := workerPNG(t, 1, 1, false)
	encoded := []byte(base64.StdEncoding.EncodeToString(raw))
	metadataCases := []struct {
		name string
		meta Metadata
		want Failure
	}{
		{name: "zero size", meta: Metadata{Axis: SizingIntrinsic, PreserveAspectRatio: true}, want: FailureInvalid},
		{name: "preserve false", meta: Metadata{Size: uint64(len(raw)), Axis: SizingIntrinsic}, want: FailureInvalid},
		{name: "intrinsic cells", meta: Metadata{Size: uint64(len(raw)), Axis: SizingIntrinsic, Cells: 1, PreserveAspectRatio: true}, want: FailureInvalid},
		{name: "width zero", meta: Metadata{Size: uint64(len(raw)), Axis: SizingWidth, PreserveAspectRatio: true}, want: FailureInvalid},
		{name: "width above bound", meta: Metadata{Size: uint64(len(raw)), Axis: SizingWidth, Cells: 257, PreserveAspectRatio: true}, want: FailureLimit},
		{name: "unknown axis", meta: Metadata{Size: uint64(len(raw)), Axis: SizingAxis(99), PreserveAspectRatio: true}, want: FailureInvalid},
	}
	for _, test := range metadataCases {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			command := directWorkerCommand(t, store, encoded, test.meta)
			job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16})
			if job != nil || failure != test.want {
				t.Fatalf("job=%#v failure=%v want=%v", job, failure, test.want)
			}
			command.Close()
			requireNoUsage(t, process, store)
		})
	}

	metricCases := []struct {
		name   string
		width  uint32
		height uint32
		want   Failure
	}{
		{name: "zero width", height: 1, want: FailureInvalid},
		{name: "zero height", width: 1, want: FailureInvalid},
		{name: "wide bound", width: maxCellPixelDimension + 1, height: 1, want: FailureLimit},
		{name: "high bound", width: 1, height: maxCellPixelDimension + 1, want: FailureLimit},
	}
	for _, test := range metricCases {
		t.Run(test.name, func(t *testing.T) {
			process, store, _ := newAdapterTestStore()
			command := directWorkerCommand(t, store, encoded, validWorkerMetadata(uint64(len(raw))))
			job, failure := NewDecodeJob(store, command, DecodeSpec{CellPixelWidth: test.width, CellPixelHeight: test.height})
			if job != nil || failure != test.want {
				t.Fatalf("job=%#v failure=%v want=%v", job, failure, test.want)
			}
			requireNoUsage(t, process, store)
		})
	}

	t.Run("header image mismatch", func(t *testing.T) {
		process, store, _ := newAdapterTestStore()
		commandImage, _ := store.AllocateInternalImageID()
		headerImage, _ := store.AllocateInternalImageID()
		placement, _ := store.AllocateInternalPlacementID()
		command := directWorkerCommandWithHeader(t, store, encoded, validWorkerMetadata(uint64(len(raw))), commandImage, headerImage, placement)
		result := runWorker(t, store, command, DecodeSpec{CellPixelWidth: 8, CellPixelHeight: 16}, context.Background())
		if result.Failure != FailureInvalid || result.Candidate != nil {
			t.Fatalf("result=%#v", result)
		}
		result.Close()
		requireNoUsage(t, process, store)
	})
}

func TestITermDecodeWorkerCapturesSpecAndMetadataByValue(t *testing.T) {
	process, store, _ := newAdapterTestStore()
	raw, _ := workerPNG(t, 2, 2, true)
	command := sealedWorkerCommand(t, store, raw, SizingIntrinsic, 0)
	spec := DecodeSpec{CellPixelWidth: 2, CellPixelHeight: 2}
	job, failure := NewDecodeJob(store, command, spec)
	if failure != FailureNone {
		t.Fatal(failure)
	}
	command.Metadata = Metadata{}
	command.Image = 0
	spec.CellPixelWidth = 0
	result := job.Run(context.Background())
	if result.Failure != FailureNone || result.Candidate == nil || result.Span != (Span{Cols: 1, Rows: 1}) {
		t.Fatalf("result=%#v", result)
	}
	result.Close()
	requireNoUsage(t, process, store)
}
