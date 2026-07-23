# Phase 14 Sixel/iTerm Qualification

Date: 2026-07-22
Production base: `0f619f7`

## Disposition

**Automated qualification: PASS. Real Windows/GLFW/OpenGL qualification: UNRUN. Support claim: none.**

Phase 14 delivers independently enabled, experimental, default-off, restart-scoped bounded Sixel and iTerm inline-image subsets. This report qualifies parser recovery, exact grammar, decode, shared limits/scheduling, owner publication, lifecycle, activation rollback, public-output privacy, import/no-I/O boundaries, and disabled-path performance. It does not establish broad protocol conformance, a real-GUI/platform pass, or a default-on decision.

Operational rollback is independent: set `graphics.sixel.enabled=false` and/or `graphics.iterm.enabled=false`, then restart.

## Exact qualified subset

- Sixel transport: 7-bit `ESC P q|0q|0;0q|0;0;0q`, ST only, one `"1;1;W;H` declaration before pixels, and only `?`–`~`, `!N<char>`, `#N`, `#N;2;R;G;B`, `$`, and `-`.
- iTerm transport: 7-bit `OSC 1337;File=<fields>:<strict padded base64>` ending in BEL or ST; exactly lexical `inline=1`, positive exact decoded `size`, one PNG with EOF, optional cell-only `width` xor `height` in `1..256`, and absent or exact `preserveAspectRatio=1`.
- Both protocols: cursor-neutral anchor captured at frame termination, no reply/reply slot, internal high-half IDs, create-only ephemeral resource/placement commit, final-placement resource retirement, shared Phase 13 stores/budgets/snapshots/GL-context caches, one shared FIFO scheduler, and fixed privacy-safe diagnostics.
- Public observers: parser-coupled `PaneOutput` projection removes only enabled selected Kitty/Sixel/iTerm envelopes across arbitrary fragmentation, cancellation, overflow, reset, and EOF. Disabled or unselected controls remain public.

The complete normative grammar, bounds, sizing formulae, exclusions, and rollback are in [`docs/spec.md`](../spec.md#phase-14-experimental-sixel-and-iterm-inline-images).

## Automated qualification matrix

| Requirement | Result | Evidence |
|---|---|---|
| Selected Sixel DCS framing, exact preamble, ST/CAN/SUB/reset/EOF recovery, 16 KiB chunks, 256 KiB frame, no text leakage | PASS | `internal/vt/parser_control_string.go`, `internal/vt/parser_sixel_transport_test.go`, `FuzzSixelDCSTransportSelection` |
| Selected OSC 1337 framing, BEL/ST, overlap/overflow recovery, parser aggregate bound, no text leakage | PASS | `internal/vt/parser_control_string.go`, `internal/vt/parser_osc1337_transport_test.go`, `FuzzOSC1337SelectedTransport` |
| Generic parser/control framing remains panic-free and fragmentation invariant | PASS | `FuzzParserAdvanceDoesNotPanic`, `FuzzControlStringFraming` |
| Parser-coupled public projection matches the legacy parser oracle and removes exactly selected envelopes | PASS | `internal/vt/parser_public_output.go`, `internal/vt/parser_public_output_test.go`, dual-oracle/envelope fuzz targets |
| Mux `PaneOutput` and GLFW Lua output callbacks receive no enabled selected image payload; disabled/unselected data remains public | PASS | `internal/mux/mux_public_output_test.go`, `internal/frontend/glfwgl/app_mux_public_output_test.go` |
| Sixel tokenizer exact raster/RGB/repeat/band grammar, fragmentation, discard, and sealed-transfer ownership | PASS | `internal/sixel/token.go`, `internal/sixel/adapter_test.go`, tokenizer/adapter fuzz targets, [`phase-14-slice-14.7-sixel-adapter.md`](phase-14-slice-14.7-sixel-adapter.md) |
| Sixel two-pass decode, checked operations/dimensions/pixels/RGBA/span, palette detachment, cancellation, and rollback | PASS | `internal/sixel/worker_test.go`, `FuzzSixelDecodeWorker`, [`phase-14-slice-14.8-sixel-worker.md`](phase-14-slice-14.8-sixel-worker.md) |
| iTerm metadata scanner and strict padded base64 reject duplicates, unknowns, whitespace, names, broad sizing, and external-I/O forms | PASS | `internal/itermimage/adapter_test.go`, scanner/base64/adapter fuzz targets, [`phase-14-slice-14.10-iterm-adapter.md`](phase-14-slice-14.10-iterm-adapter.md) |
| iTerm exact decoded size, one PNG/EOF, checked intrinsic/one-axis span, cancellation, bombs, and rollback | PASS | `internal/itermimage/worker_test.go`, `FuzzITermDecodeWorker`, [`phase-14-slice-14.11-iterm-worker.md`](phase-14-slice-14.11-iterm-worker.md) |
| One FIFO scheduler, two workers, one outstanding job/pane, queue 32, inclusive 8/32 pending caps, queue-inclusive 250 ms acceptance | PASS | `internal/workscheduler`, `internal/mux/kitty_decode_scheduler_test.go`, [`phase-14-slice-14.6-scheduler.md`](phase-14-slice-14.6-scheduler.md) |
| Cursor remains unchanged; Sixel/iTerm emit no replies or reply slots and cannot reorder Kitty/DSR/OSC replies | PASS | `internal/mux/mux_sixel_test.go`, `internal/mux/mux_iterm_test.go`, [`phase-14-slice-14.13-iterm-mixed-runtime.md`](phase-14-slice-14.13-iterm-mixed-runtime.md) |
| Internal IDs stay in `0x80000000..0xffffffff`, do not wrap/reuse, and are not Kitty-addressable | PASS | `internal/termimage/internal_id_test.go`, mux completion revalidation tests |
| Ephemeral resource survives until and retires with its final placement across edit/erase/scroll/history/ED3/reflow/alternate/delete/reset/close | PASS | `internal/core/images_ephemeral_test.go`, mux Sixel/iTerm lifecycle tests |
| All eight protocol masks share owners/limits/caches correctly; all-disabled remains literal nil | PASS | `internal/mux/mux_iterm_test.go`, `internal/frontend/glfwgl/terminal_image_activation_test.go`, [`phase-14-slice-14.15-production-activation.md`](phase-14-slice-14.15-production-activation.md) |
| Initial/child/restore failures publish old-or-new and unwind provisional caches/projections/mux ownership in reverse order | PASS | `internal/frontend/glfwgl/terminal_image_activation_test.go`, restore rollback tests, Slice 14.15 evidence |
| Diagnostic surface is exactly `Protocol`, `Reason`, `Count`, `Duration`; fixed reasons replay without payload/pixels/metadata/base64/IDs | PASS | `internal/mux/image_diagnostic.go`, `internal/mux/image_diagnostic_test.go` |
| Sixel/iTerm leaf packages cannot use filesystem/network/process/unsafe or cross architecture boundaries | PASS | `go run ./scripts/check-phase14-imports.go`; positive standard-library allowlist plus only `internal/termimage` |
| Real Windows/OpenGL image rendering and teardown | **UNRUN** | Manual matrix only; no support claim |

## Required 60-second fuzz matrix

Each target below was run as its own `go test` fuzz invocation with `-run='^$'`, exact `-fuzz` selection, `-fuzztime=60s`, and `-parallel=1` on `windows/amd64`. All 13 exited successfully against the parser/public-projection security fix.

| Area | Package / target | Result |
|---|---|---|
| Control framing | `./internal/vt` / `FuzzControlStringFraming` | PASS (60 s) |
| Generic parser | `./internal/vt` / `FuzzParserAdvanceDoesNotPanic` | PASS (60 s) |
| Sixel DCS transport | `./internal/vt` / `FuzzSixelDCSTransportSelection` | PASS (60 s) |
| OSC 1337 transport | `./internal/vt` / `FuzzOSC1337SelectedTransport` | PASS (60 s) |
| Public projection dual oracle | `./internal/vt` / `FuzzPublicOutputProjectionDualOracle` | PASS (60 s) |
| Public projection selected envelope | `./internal/vt` / `FuzzPublicOutputProjectionSelectedEnvelope` | PASS (60 s) |
| Sixel tokenizer | `./internal/sixel` / `FuzzSixelTokenizerFragmentation` | PASS (60 s) |
| Sixel adapter | `./internal/sixel` / `FuzzSixelAdapterLifecycle` | PASS (60 s) |
| Sixel worker | `./internal/sixel` / `FuzzSixelDecodeWorker` | PASS (60 s) |
| iTerm scanner | `./internal/itermimage` / `FuzzITermScannerFragmentation` | PASS (60 s) |
| iTerm strict base64 | `./internal/itermimage` / `FuzzITermStrictBase64` | PASS (60 s) |
| iTerm adapter | `./internal/itermimage` / `FuzzITermAdapterLifecycle` | PASS (60 s) |
| iTerm worker | `./internal/itermimage` / `FuzzITermDecodeWorker` | PASS (60 s) |

## Shared hard limits and execution budgets

| Resource | Bound |
|---|---:|
| selected logical control-string frame / borrowed chunk | 256 KiB / 16 KiB |
| pending transfers | 8 per pane / 32 process-wide |
| encoded residency | 8 MiB per pane / 32 MiB process-wide |
| chunks / transfer lifetime | 4,096 / 10 s |
| decoded image / pane / process | 64 MiB / 64 MiB / 256 MiB |
| dimensions / pixels / Sixel operations | 4,096 per axis / 16,777,216 / 4,194,304 |
| images | 256 per pane / 1,024 process-wide |
| placements / span | 1,024 per pane / 4,096 process-wide / 256 cells per axis |
| scheduler | one outstanding job per pane / two workers / FIFO queue 32 |
| acceptance deadline | 250 ms including queue wait; prevents commit, not CPU preemption |
| GL context cache | 512 textures / 256 MiB per context |

Configuration may only lower the exposed shared limits. It cannot multiply a limit per protocol.

## Performance references

- Phase 14 authority baseline: text snapshot 8,988 ns/op, core reuse 2,961 ns/op, parser 2,969 ns/op; control discard 149,872 ns/op and overflow 1,757,069.5 ns/op; all were 0 B/op and 0 allocs/op. See [`phase-14-baseline.md`](phase-14-baseline.md).
- Shared scheduler: about 315 ns/op, 0 B/op, 0 allocs/op. See [`phase-14-slice-14.6-scheduler.md`](phase-14-slice-14.6-scheduler.md).
- Sixel tokenizer remained 0 B/op/0 allocs/op; the bounded 256 KiB adapter retained one frame/ownership block. The 256x64 worker measured 125,726–154,000 ns/op with its immutable 65,536-byte candidate. See Slices 14.7 and 14.8.
- iTerm 256 KiB scanner measured 1.006–1.171 ms/op at 0 B/op/0 allocs/op; adapter allocation is the bounded retained transfer. The 256x64 PNG worker measured 306,559–448,767 ns/op including standard-library PNG/color conversion and its immutable candidate. See Slices 14.10 and 14.11.
- Final production activation's all-disabled frame median was 7.721 ns/op, 0 B/op, 0 allocs/op; paired clean-base difference +2.17%, inside the 3% gate. See Slice 14.15.
- After parser-coupled public projection landed, the final all-disabled frame median was 7.104 ns/op versus 7.074 ns/op on clean `0f619f7` (+0.43%), still 0 B/op/0 allocs/op.
- Parser-coupled plain-text public projection, ten 2-second single-CPU samples: all-disabled median 6,898.5 ns/op and all-enabled median 6,744 ns/op; both 0 B/op and 0 allocs/op. The cross-run difference is treated as host noise, not a speed claim; the relevant result is bounded zero-allocation plain-text projection.

## Diagnostics and external-I/O boundary

Allowed protocols are exactly `sixel` and `iterm`. Allowed reasons are exactly `invalid`, `unsupported`, `limit`, `timeout`, `cancelled`, `failed`, `stale`, and `busy`. Allowed data fields are exactly `Protocol`, `Reason`, `Count`, and `Duration`. Success emits no diagnostic.

No payload bytes, decoded pixels, base64, metadata/field names, pane/resource/transfer/placement IDs, paths, URLs, or terminal text may enter a Phase 14 diagnostic. Callback panics are contained and cannot change runtime disposition.

The Sixel/iTerm leaf import gate permits only an explicit standard-library set and `internal/termimage`; it rejects filesystem, network, process, unsafe, cross-layer, and third-party dependencies. The grammars reject every external file/path/URL/download/write form. No external I/O is performed.

## Manual qualification boundary

Every row in [`docs/manual-verification.md`](../manual-verification.md#phase-14-experimental-sixel-and-iterm-images--real-gui-qualification) is **UNRUN**, including Windows `amd64`, Windows `arm64`, Linux GUI, and macOS GUI. Automated Windows tests, fake GL tests, compilation, fuzzing, and benchmarks are not substituted for a real GLFW/OpenGL run.

Consequently `docs/parity-support-matrix.json` remains `status=experimental`, `platform=windows-glfw-opengl`, `default_enabled=false`, `manual_qualification=unrun`, and `support_claim=none`.
