# Phase 14 Close-out Report

Date: 2026-07-22
Production base: `0f619f7`

## Result

**Automated gates: PASS. All 13 relevant 60-second fuzz targets: PASS. Real Windows/GLFW/OpenGL matrix: UNRUN. Support disposition: experimental, default-off, restart-scoped, support claim none.**

Phase 14 closes the bounded Sixel DCS and iTerm OSC 1337 direct-inline PNG implementation without claiming broad protocol or platform conformance. The close-out includes the parser-coupled public-output security closure found during final adversarial review; defaults and support claims remain unchanged.

## Exact gate evidence

Executed from `C:/dev/golang-terminal-emulator-parity-phase-14-closeout` on `windows/amd64`:

| Gate | Result |
|---|---|
| `go test ./internal/vt ./internal/sixel ./internal/itermimage ./internal/termimage ./internal/core ./internal/mux -count=1` | PASS |
| `go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1` | PASS |
| `go test ./... -count=1` | PASS |
| `go test -tags glfw ./... -count=1` | PASS |
| `go vet -unsafeptr=false ./...` | PASS |
| `go vet -unsafeptr=false -tags glfw ./...` | PASS |
| `go test -race ./... -count=1` | PASS |
| `go test -race -tags glfw ./internal/frontend/glfwgl ./internal/frontend/gpu ./internal/workscheduler ./internal/kitty ./internal/sixel ./internal/itermimage ./internal/termimage ./internal/core ./internal/vt ./internal/mux ./cmd/cervterm -count=1` | PASS |
| `go run ./scripts/check-maturity-gates.go` | PASS |
| `go run ./scripts/check-phase13-imports.go` | PASS |
| `go run ./scripts/check-phase14-imports.go` | PASS |
| JSON parse/schema-value checks for `docs/parity-support-matrix.json` | PASS |
| local Markdown relative-link/fragment validation for changed docs | PASS |
| `git diff --stat --check` | PASS |
| all 13 exact fuzz targets listed in [`phase-14-qualification.md`](phase-14-qualification.md), 60 seconds each | PASS |

Unknown is not treated as pass. No real-GUI command was run; all manual rows remain UNRUN.

## Preserved parser/public-output security fix

The pre-existing dirty implementation changes make mux `PaneOutput` parser-coupled rather than a raw ingress copy:

- `Parser.AdvancePublic`/`EndOfInputPublic` share terminal parser decisions and omit enabled selected Kitty/Sixel/iTerm envelopes across arbitrary fragmentation.
- Disabled and unselected control strings remain public; undecided bytes flush safely at EOF, while a selected partial frame is dropped.
- Selected OSC 1337 now obeys the 256 KiB aggregate framing cap, including overflow/discard/recovery behavior.
- Unselected Sixel DCS preambles produce no selected-transfer cancellation diagnostic.
- The mux and GLFW Lua output route use only projected bytes; selected-only ingress can preserve pane activity with an empty public payload.
- Dual-oracle and selected-envelope fuzzing, mux/GLFW integration tests, and all full/race gates pass.

The parser projection and its transport/mux/frontend tests are part of this close-out and are covered by every final verification gate.

## Exact behavior closed out

### Grammar and transport

Sixel accepts only 7-bit `ESC P q|0q|0;0q|0;0;0q ... ST`, one `"1;1;W;H` declaration before pixels, and the bounded data/repeat/RGB palette/carriage/new-band forms. iTerm accepts only 7-bit `OSC 1337;File=...:<strict padded base64>` ending in BEL or ST with exact lexical `inline=1`, positive decoded `size`, one PNG/EOF, and optional one cell-only axis preserving aspect ratio.

C1 forms, HLS, duplicate/unknown fields, `name`, broad sizing/multipart forms, JPEG/GIF, animation, Sixel scrolling/DECSDM, cursor effects, external file/path/URL/temporary/shared-memory/download/write modes, renderer selection, and non-OpenGL rendering remain excluded. Rejection is atomic and performs no external I/O.

### Ownership, limits, scheduler, and lifecycle

Kitty, Sixel, and iTerm share the Phase 13 pane/process transfer, encoded, decoded, image, placement, snapshot, and GL-context budgets. One FIFO scheduler has two workers, queue capacity 32, and one outstanding job per pane; queue wait counts toward the 250 ms acceptance deadline. Config may lower exposed limits but cannot raise or multiply them per protocol.

Sixel/iTerm capture a canonical cursor-neutral anchor at frame termination, emit no replies or reply slots, and use monotonic non-reused internal IDs in `0x80000000..0xffffffff`. The owner atomically commits one ephemeral resource plus placement and retires the resource exactly when its final placement retires. Kitty low-half wire IDs and durable resource semantics remain unchanged.

### Activation and rollback

`graphics.sixel.enabled` and `graphics.iterm.enabled` are independent strict-v2, default-false, restart-scoped flags. `imagesEnabled = kitty || sixel || iterm` creates one shared image owner only when required; all-disabled remains literal nil. Initial, child, and restored projections prepare one distinct cache per current GL context, publish old-or-new state, and unwind provisional resources in reverse acquisition order.

Operational rollback disables only the affected protocol and restarts. Code rollback order remains activation -> config -> mux routes -> workers/adapters/shared PNG codec -> scheduler -> parser transports/public projection -> ephemeral lifecycle -> ID partition -> canonical anchor.

## Diagnostics allowlist and payload non-leakage

The exact diagnostic schema is four fields: `Protocol`, `Reason`, `Count`, and `Duration`. Protocol values are `sixel` and `iterm`; reason values are `invalid`, `unsupported`, `limit`, `timeout`, `cancelled`, `failed`, `stale`, and `busy`. Success emits nothing.

Automated tests replay adapter, validation, limit, timeout, cancellation, submission, stale, decode, commit, and expiry failures. Reflection tests reject added fields or names containing identity/payload surfaces. Parser projection tests use explicit payload markers and prove those markers never appear in enabled `PaneOutput`/Lua output, while disabled/unselected controls remain byte-identical. No payload, pixels, metadata names, base64, internal IDs, paths, URLs, or terminal text are permitted in diagnostics.

## Performance disposition

- Phase 14 baseline text/parser/control and disabled GL paths remained zero-allocation; the actual disabled image-frame baseline was 7.13 ns/op.
- Shared scheduler median was about 315 ns/op at 0 B/op and 0 allocs/op.
- Final Slice 14.15 all-disabled frame median was 7.721 ns/op, 0 B/op, 0 allocs/op; paired clean-base difference was +2.17%, inside the 3% gate.
- After the parser public-output closure, the final all-disabled frame median was 7.104 ns/op versus 7.074 ns/op on clean `0f619f7` (+0.43%), still 0 B/op/0 allocs/op.
- Sixel tokenizer and iTerm scanner retained 0 B/op/0 allocs/op. Adapter/worker allocations are bounded retained-transfer, scratch, PNG conversion, and immutable-candidate ownership documented in the slice artifacts.
- The parser-coupled plain-text public projection ten-run medians were 6,898.5 ns/op all-disabled and 6,744 ns/op all-enabled, both 0 B/op/0 allocs/op. The difference is treated as host noise; no speedup claim is made.

See [`phase-14-qualification.md`](phase-14-qualification.md#performance-references) and the linked slice artifacts for reproducible details.

## ADR-0014 / ADR-0016 drift audit

| Authority | Final evidence | Drift |
|---|---|---|
| ADR-0014 keeps `core.Cell` text-only/32 bytes and GL handles context-local | Phase 14 import/maturity gates pass; snapshots contain detached references only; caches remain projection/context-owned | None |
| ADR-0014 requires bounded streaming, discard recovery, shared budgets, owner publication, and exact rollback | DCS/OSC framing, all 13 fuzz targets, budget/lifecycle/activation tests, and reverse rollback pass | None |
| ADR-0014 excludes external transports and payload-bearing diagnostics | Positive import allowlist and exact grammar reject external I/O; diagnostic/public-output leakage tests pass | None |
| ADR-0016 requires one shared FIFO/two-worker scheduler, one active pane job, queue 32, and queue-inclusive deadline | Scheduler/mixed-runtime tests and race gates pass | None |
| ADR-0016 requires cursor-neutral no-reply Phase 14 placement | Mux Sixel/iTerm and mixed reply-order tests pass | None |
| ADR-0016 partitions low-half Kitty IDs from internal high-half IDs | Namespace boundary/exhaustion/addressing tests pass | None |
| ADR-0016 requires ephemeral final-placement retirement | Core lifecycle truth table and mux runtime tests pass | None |
| ADR-0016 requires independent default-off restart flags and all-disabled nil behavior | Config/provenance/diff/doctor plus all eight activation-mask tests pass | None |
| ADR-0016 requires exact narrow Sixel/iTerm grammars | tokenizer/scanner/worker tests and grammar fuzz targets pass | None |

**Drift detected: No. ADR update required: No. Guardrail update required: No.** The parser-coupled `PaneOutput` projection is a security closure of the existing no-payload-leak rule; `PaneOutput.BytesRead` preserves raw PTY ingress accounting while `Data` carries only the public projection. It does not widen protocol behavior, ownership, side effects, or support claims.

## Manual and support boundary

Every Phase 14 manual row is **UNRUN**, including Windows 11 `amd64`, Windows 11 `arm64`, Linux real GLFW/OpenGL, and macOS real GLFW/OpenGL. No manual PASS, real-driver context-loss PASS, platform qualification, or stable support is claimed.

The machine-readable disposition is intentionally:

- `status`: `experimental`
- `platform`: `windows-glfw-opengl`
- `default_enabled`: `false`
- `manual_qualification`: `unrun`
- `support_claim`: `none`

A future status proposal requires attached real-GUI evidence for the manual matrix. Partial execution remains experimental/default-off; any security, cursor/reply, ownership, rollback, context-isolation, or external-I/O failure blocks promotion.
