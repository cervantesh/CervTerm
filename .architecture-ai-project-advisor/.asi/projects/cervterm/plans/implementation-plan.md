# Phase 13 Implementation Plan — Bounded Image Model and Kitty Graphics

Date: 2026-07-20
Status: Approved after independent verification (PASS)
Baseline: Phase 12 close-out `e1ed32c`; Slice 13.0a starts from clean local `dev` at `65f9000` after mandatory Hermes-only Agentic Stack onboarding (verify the latest merged `dev` before every later worktree)
Authority: Phase 13 context/preflight/guardrails, accepted ADR 0014, reviewed Phase 13 feature design

## Delivery policy

Execute strictly in order. Each slice starts from the latest locally merged `dev`, uses a clean dedicated worktree/branch, receives focused and full validation plus independent review, is committed once, merged locally into `dev`, and only then advances. Do not touch the dirty primary worktree `fix/windows-version-resource-from-tag` at `61ece0e`.

No slice may add renderer selection, enlarge/change `core.Cell`, activate Sixel/iTerm/animation/external transports, or make Kitty default-on.

## Dependency chain

```text
13.0a tracked architecture gate
 -> 13.0b invariant/performance harness
 -> 13.1 APC/DCS framing
 -> 13.2 budget/store foundations
 -> 13.3 atomic placement primitives
 -> 13.4 edit/scroll/history lifecycle
 -> 13.5 reflow/alternate/reset lifecycle
 -> 13.6 detached render projection
 -> 13.7 mux store/reply lifecycle
 -> 13.8 strict dormant config
 -> 13.9 Kitty adapter/chunk model
 -> 13.10 bounded decoders
 -> 13.11 async mux integration
 -> 13.12 optional GL capability
 -> 13.13 context texture cache
 -> 13.14 draw/damage
 -> 13.15 default-off activation
 -> 13.16 qualification/close-out
```

## 13.0a — Track architecture authority

Branch: `docs/parity-phase-13-image-architecture`

Files: tracked `.architecture-ai-project-advisor/.asi/{active-project.json,projects/index.json,projects/cervterm/context.md,projects/cervterm/context.json,projects/cervterm/guardrails.md,projects/cervterm/preflight.md,projects/cervterm/decisions/0006-*.md,projects/cervterm/decisions/0014-*.md,projects/cervterm/decisions/README.md,projects/cervterm/designs/feature-design.md,projects/cervterm/plans/implementation-plan.md}`. The external `.t50-project-flow` state is planning input, not a second tracked authority in this slice.

Work:
- Copy the accepted ADR 0014, revised design, plan, context, guardrails and preflight into the existing tracked advisor tree in a clean worktree from `dev`.
- Mark tracked ADR-0006 `Superseded` by ADR 0014 without erasing history and rebuild the decision index.
- Set tracked active/context metadata and `projects/index.json` summary/stage to Phase 13 `plan`; validate every changed JSON file.
- No production/test behavior change and no tracked `.t50-project-flow` duplicate tree.

Success criteria:
- One tracked authoritative set of caps/lifecycle/subset rules; no stale Phase 11/12 context.
- `test -f` finds ADR 0014/design/plan; `grep` confirms 0014 `Accepted`, 0006 `Superseded`, phase/stage `13`/`plan`, and the Phase 13 objective in `projects/index.json`; `python -m json.tool` validates active-project, project-index, and context JSON.
- `go test ./... -count=1`, `go test -tags glfw ./... -count=1`, `go run ./scripts/check-maturity-gates.go`, and `git diff --check` pass; `git diff --name-only` contains only the tracked advisor tree.

Exact Slice 13.0a Git Bash gate:

```bash
set -e
ROOT=.architecture-ai-project-advisor/.asi
PROJECT=$ROOT/projects/cervterm
test -f "$PROJECT/decisions/0014-bound-terminal-image-lifetime-transports-and-resources.md"
test -f "$PROJECT/designs/feature-design.md"
test -f "$PROJECT/plans/implementation-plan.md"
grep -q '^Accepted$' "$PROJECT/decisions/0014-bound-terminal-image-lifetime-transports-and-resources.md"
grep -q '^Superseded$' "$PROJECT/decisions/0006-bound-terminal-image-lifetime-and-resources.md"
grep -q '"stage": "plan"' "$ROOT/active-project.json"
grep -q '"phase": "13"' "$ROOT/projects/index.json"
grep -q 'bounded protocol-neutral image model' "$ROOT/projects/index.json"
python -m json.tool "$ROOT/active-project.json" >/dev/null
python -m json.tool "$ROOT/projects/index.json" >/dev/null
python -m json.tool "$PROJECT/context.json" >/dev/null
test -z "$(git diff --name-only | grep -v '^\.architecture-ai-project-advisor/')"
go test ./... -count=1
go test -tags glfw ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

Rollback: revert documentation/state commit only.

## 13.0b — Pin invariants and baselines

Branch: `test/parity-phase-13-image-baseline`

Files: `internal/core/attr_test.go`, new `internal/vt/parser_benchmark_test.go`, `internal/render/snapshot_test.go`, new row-grid disabled-draw benchmark under `internal/frontend/glfwgl`, new `scripts/check-phase13-imports.go`, new `scripts/capture-phase13-benchmark.go`, new `scripts/compare-phase13-baseline.go`, raw `docs/validation/phase-13-baseline.txt`, raw `docs/validation/phase-13-gl-baseline.txt`, and explanatory `docs/validation/phase-13-baseline.md`.

Work:
- Assert `unsafe.Sizeof(core.Cell{}) == 32`.
- Add repeatable text-only parser/core/snapshot benchmarks, a context-free row-grid draw benchmark, and a disabled idle/frame measurement procedure. The frame-level disabled-image dispatch seam does not exist yet and is introduced and baselined in 13.14.
- Use `capture-phase13-benchmark.go` to warm and record the exact ten-sample untagged and `-tags glfw` commands into the two `.txt` artifacts with a machine-readable method/environment/source-harness digest; record production baseline commit, OS/CPU, Go version, `GOMAXPROCS`, sample count and disabled idle/frame evidence in the Markdown report.
- Enforce with non-optional `check-phase13-imports.go`: `termimage` may not import core/render/mux/frontend and non-frontend packages may not import GLFW/OpenGL once the package exists.
- `compare-phase13-baseline.go` consumes exactly ten samples per benchmark, computes the mandatory median/allocations gate itself, fails any matching disabled/text-only benchmark above 3% median or with new allocations, and additionally invokes installed `benchstat` when available. Absence of that optional presentation tool cannot weaken or bypass the self-contained hard gate; installing it requires separate user approval.

Success criteria: no production behavior diff; both exact baseline captures are recorded and reproducible; idle evidence is either freshly captured or explicitly carried forward only when `cmd/` and `internal/` are proven byte-identical to the source evidence commit; both raw `.txt` artifacts parse through the comparison script; the row-grid GL benchmark runs without requiring a live window/context; the import script passes before `termimage` exists and automatically starts enforcing production imports when added; no existing test expectation changes.

## 13.1 — Generic bounded APC/DCS framing

Branch: `feat/parity-phase-13-control-strings`

Files: `internal/vt/parser.go`, `parser_esc.go`, new `parser_control_string.go`, `internal/core/vt_features.go`, tests/fuzz/retained corpus, control-string benchmarks, portable raw baseline/report, and the `control` capture-suite registration.

API: `ControlStringKind`, borrowed `ControlStringEvent`, `SetControlStringSink`, `Parser.Reset`, `Parser.EndOfInput`.

Work: dedicated APC/DCS/escape/discard states; 256 KiB frame and 16 KiB chunk; ST finalize, CAN/SUB cancel, overflow discard through terminator, nil sink discard, reset/EOF cancellation; preserve OSC; establish zero-allocation discard/overflow first-result baselines.
- Mandatory parser fuzz findings are in-scope safety work: saturate CSI parameters at the supported `uint16` geometry limit and make forward/backward tab traversal stop at the terminal boundary instead of repeating no-op work.

Success criteria:
- Every split boundary, ESC/ST ambiguity, malformed/cancel/overflow/reset/EOF returns to ground text without payload leakage.
- Existing OSC/VT tests byte-identical; disabled benchmark <=3% median regression and zero new allocation.
- Fuzz `FuzzControlStringFraming` and existing parser fuzz >=60 s before merge.

Rollback: remove inert framing/API; the independently qualified CSI overflow/loop hardening may remain.

## 13.2 — Bounded process/pane store foundations

Branch: `feat/parity-phase-13-image-store`

New package: `internal/termimage/{doc,types,limits,budget,store}.go` and tests.

Work:
- Define pane-scoped IDs/generations, immutable hard and lowerable operational limits, checked atomic process reservations for encoded bytes, decoded bytes, images, **placements (1,024/pane and 4,096/process)** and pending transfer counts, owner-thread store epochs, pending candidates and detached acquisition copies.
- Implement `BeginTransfer`, exactly-once candidate close, `Acquire`, `Reset`, `Close`; no core/render/mux imports.
- Reserve every byte/count/placement before retention; image-ID reuse increments generation.

Success criteria:
- Exact rollback under cancel/failure/reset/close and concurrent worker lease return.
- Pane/global encoded/decoded/image/placement/transfer caps reject before retention; checked arithmetic cannot wrap; a failed placement reservation cannot retain a resource or alter counters.
- Race tests and import-boundary script pass; inert package changes no runtime path.

## 13.3 — Dormant placement contracts and prepared transaction

Branch: `feat/parity-phase-13-image-transaction`

Files: new `internal/core/images.go`, internal core/terminal/type tests, termimage placement types/tests.

Work:
- Implement concrete primary/alternate coordinates, nil-or-validated pixel crop, span 1..256 and exact delete-selector combination rules.
- Add nil-by-default private store/sidecars and test-only constructors; do **not** expose production attach/commit/place/delete APIs yet.
- Prepare complete replacement store state and replacement sidecar slices, including pane/global placement reservations, then provide an infallible private owner-thread two-pointer publication helper.
- Fault-inject every preparation allocation/validation/reservation seam; test transmit-only, replace, placement and deletion as private state transitions.

Success criteria:
- Production cannot create a placement after this slice; the incomplete lifecycle is unreachable outside same-package tests.
- Any injected failure leaves prior resource/generation/placements/counters unchanged; readers in tests observe old or new pair.
- Invalid crop/selectors reject atomically; `Cell` remains 32 bytes; text-only terminal allocates no image state.

Mandatory independent review: transaction ownership/atomicity and API boundary.

## 13.4 — Complete dormant edit/scroll/history lifecycle

Branch: `feat/parity-phase-13-image-screen-lifecycle`

Files: private `internal/core/images.go`, screen/edit/scroll/scrollback-capacity paths and same-package focused tests.

Work:
- Keep all image creation/publication entry points private/unreachable from production.
- Erase/overwrite overlap deletes whole placement; insert/delete chars/lines and partial scroll shift contained placements; boundary crossings delete.
- Full-screen upward scroll moves into history; ring eviction/capacity reduction releases placements; ED3 clears history placements only; preserve scrolled-back projection.

Success criteria: production still cannot attach/create image state; exhaustive boundary truth tables, ring-wrap/orphan/resource tests, unchanged text scrollback behavior, and bounded 1,024-placement operations pass.

## 13.5 — Complete lifecycle, then publish core image API

Branch: `feat/parity-phase-13-image-reflow`

Files: core resize/screen/terminal, `internal/vt/parser_esc.go` RIS/reset seam, image API and lifecycle tests. Mux pane close remains deferred to 13.7.

Work:
- Extract private reusable `reflowMap` from the existing old/new physical-cell stream and use it for cursor/boundary/placement top-left.
- Preserve cell span/delete evicted anchors; isolate alternate placements, discard on exit and top-anchor resize crop.
- RIS cancels the parser candidate and performs prepared `ResetImages`/store epoch reset.
- Only after every mutation path is integrated, publish `AttachImageStore`, `CommitImage`, `DeleteImages`, `ResetImages`, and `ImageProjection`. No production caller exists until 13.7/13.11.

Success criteria: repeated narrow/wide reflow deterministic; primary survives alternate; alternate never enters history; RIS returns reservations to zero; text resize/reset goldens unchanged; one final lifecycle matrix covers overwrite, every erase/edit/scroll/evict/reflow/alternate/reset transition before the public API is accepted.

Mandatory independent lifecycle and public-API review.

## 13.6 — Detached render/mux image projection

Branch: `feat/parity-phase-13-image-snapshot`

Files: render snapshot/damage, mux `PaneView` detachment and tests.

Work: add detached placement descriptors and `ImageGeneration`; reuse internal capture capacity but deep-copy public pane views; keep row hashes text-only; add image-only damage identity.

Success criteria: post-capture backing mutations cannot change snapshot; returned views cannot mutate pane; no pixels/store/GL handles; text-only capture zero image allocations and <=3% regression.

## 13.7 — Mux store lifecycle, acquisition and bounded shared replies

Branch: `feat/parity-phase-13-image-mux`

Files: mux options/pane/lifecycle/restore/transfer, new mux image files/tests.

Work:
- Mux owns optional process budget; image-enabled test options attach one store per pane. Default options remain literal nil.
- Transfer pane/store unchanged; pane close, failed restore and shutdown close exactly once, completing the close lifecycle deferred by 13.5.
- Add pane+generation checked detached `AcquireImageResource`.
- Replace every direct `pendingReplies` append with a shared sequenced `queueReply`; account completed bytes plus reserved async slots under 64 KiB/pane, deduct on drain, cap image slots/frames at 512 B and expose fixed counters. Existing synchronous replies receive monotonically increasing sequence IDs.

Success criteria: default creates no image infrastructure; transfer preserves CPU identity; stale acquisition false; restore/close releases all reservations; existing DA/DSR/OSC replies retain order/content under the shared bound; queue plus reserved slots cannot exceed 64 KiB.

## 13.8 — Strict v2 dormant configuration

Branch: `feat/parity-phase-13-image-config`

Files: config schema/document/validation/diff/template/Teal/publication/diagnostics/doctor and tests.

Contract: default-off `graphics.kitty.enabled`; lower-only restart-scoped encoded/decoded/image/placement/GPU limits.

Work: complete defaults, strict decode, includes/profiles/unset/provenance, candidate diagnostics, diff/restart classification, Lua/Teal/template/doctor. Do not pass config to mux/frontend yet.

Success criteria: v1/default unchanged; raised/invalid limits fail candidate; reload reports restart required without partial activation; provenance points to winning source.

## 13.9 — Kitty command/chunk/reply adapter

Branch: `feat/parity-phase-13-kitty-adapter`

New package/files: `internal/kitty/{doc,header,adapter,reply}.go`, fixtures/tests/fuzz.

Work: parse APC `G`; actions transmit/transmit+place/place/delete/query; direct transport only; fixed redacted replies/quiet policy; 8 pending/pane, 32 process, 4,096 chunks. Each candidate records last activity and exposes pure `NextExpiry()`/`Expire(now)`; 10 s of silence cancels and releases it even without another input byte. Reject external transports, animation, placeholders and unknown/conflicting fields.

Success criteria: header ordering/chunk boundaries deterministic; malformed/finalized request commits nothing and returns at most one reply; calling `Expire` after a silent 10 s releases every reservation and yields one fixed timeout outcome; earlier calls are no-ops; no value/payload echo; adapter fuzz >=60 s.

## 13.10 — Bounded raw/zlib/PNG decoders

Branch: `feat/parity-phase-13-kitty-decode`

Files: `internal/kitty/{decode,worker}.go`, tests/fuzz/benchmarks.

Work: direct base64 RGB24/RGBA32, zlib raw and PNG; validate format/dimensions/stride/pixels and reserve output before allocation; immutable candidate result only. 4,096², 16,777,216 pixels, 64 MiB/image hard caps.

Success criteria: truncated/invalid inputs and decompression bombs roll back exactly; no unchecked dimension allocation; no core/mux/frontend imports; fuzz >=60 s; security review has no blocker.

Mandatory independent decoder/security review.

## 13.11 — Async Kitty scheduler, commit and reply ordering

Branch: `feat/parity-phase-13-kitty-runtime`

Files: mux drain/advance/pane, new mux Kitty integration, parser EOF hookup, app-wide owner-loop next-deadline/wake seam outside projection visibility filtering, and tests.

Work:
- Connect sink only for enabled test options.
- Bounded queue 32, two workers/process, one active/pane, deadline from final seal; reject new work when saturated.
- Assign every parsed reply request a sequence. Async Kitty reserves a bounded 512 B slot; later synchronous DA/DSR/OSC replies remain buffered until the earlier slot completes/times out, then shared drain emits contiguous sequence order.
- Expose mux `NextImageDeadline` across **all panes/windows**, including hidden workspaces/projections. `App.processNextWakeTimeout` takes the minimum mux deadline before/independently of its visible-projection loop, and the top-level owner tick expires/drains due candidates before rendering. Thus hidden silent transfers release without polling; projection visibility never filters resource deadlines.
- Pane/epoch/generation revalidation in owner drain; atomically commit prepared resource+optional placement; only then complete the reply slot and dirty.
- Late/stale/closed outcomes release leases exactly; workers never mutate model/session/GL.

Success criteria: deterministic wire request/reply/commit order including `Kitty -> DSR` and `Kitty -> OSC` interleavings; a hidden-workspace/hidden-projection silent transfer expires while PTY/UI are otherwise idle; visible filtering cannot suppress the mux deadline; no idle wake when no deadline exists; failed replacement preserves old generation; close/reset/restore cannot receive stale commit; race/shutdown/saturation/deadline tests pass.

Mandatory concurrency, ordering and timer/wake review.

## 13.12 — Optional OpenGL terminal-image capability

Branch: `feat/parity-phase-13-image-gl-capability`

Files: new GPU-neutral terminal-image capability/types, glRenderer implementation, compile-only backend assertions and fake tests.

Work: optional `TerminalImageRenderer`, `ImageTexture.Close`, checked RGBA upload/draw. Keep base Renderer and backend selection unchanged; no glyph-atlas reuse.

Success criteria: only OpenGL claims capability; upload rejects malformed length/stride; fake tests verify create/draw/close and GL-state restoration; compile tests pass all tags.

## 13.13 — Projection/context-local texture cache

Branch: `feat/parity-phase-13-image-cache`

Files: new GLFW image cache/tests/benchmarks, projection bundle integration.

Work: key pane object+resource generation; 512 entries/256 MiB hard caps; deterministic ordered-prefix visible selection fitting both caps; one-frame pins; unpinned LRU; acquire copy only on miss. Retry each resource generation at most three times with fixed 100 ms, 500 ms and 2 s delays; only a new generation or context recreation resets attempts. Register the earliest retry with the same owner-loop deadline seam only while enabled/pending. Register cache close before renderer destruction and make owning context current in normal/failure teardown.

Success criteria: pins never exceed caps/underflow/evict; overflow omissions deterministic; retry count/delays cannot spin or wake while idle; GL handles never cross context on pane transfer; stale generation, upload failure, model independence and teardown are tested.

Mandatory GL lifetime/context review.

## 13.14 — Pane-clipped drawing and damage

Branch: `feat/parity-phase-13-image-draw`

Files: GLFW draw/damage/controller and new image draw tests/goldens.

Order: background -> negative-z images -> text -> zero/positive-z images -> cursor/preedit/application overlays -> pane chrome.

Work: use existing pane clip, crop and opacity; conservative affected-pane damage on generation change; upload completion damages only references; missing texture deterministic omission.

Success criteria: z-order/clip/multi-pane goldens; image-only repaint with unchanged row hashes; introduce a context-free frame-level `BenchmarkPhase13DisabledFrame` at the actual image-dispatch seam and establish its first-result budget; disabled frame dispatch has no cache lookup/allocation/idle cadence, while the carried row-grid draw benchmark remains <=3%.

## 13.15 — Default-off transactional activation

Branch: `feat/parity-phase-13-kitty-opt-in`

Files: startup/projection/restore factories, bundle/app/main wiring and tests.

Work: map strict config into optional mux/cache settings only when enabled; create each budget/store/scheduler/cache exactly once across initial/child/restored projections; rollback closes in reverse order. Keep default false and do not advertise when disabled.

Success criteria: default/v1 literal nil behavior; enabled test startup/restore succeeds atomically; every injected failure leaves no partial owner; changes require restart; operational rollback is `enabled=false` + restart.

Mandatory activation/rollback review.

## 13.16 — Qualification and close-out

Branch: `docs/parity-phase-13-image-closeout`

Files: README/CHANGELOG/SUPPORT, architecture/spec/getting-started/manual-verification/troubleshooting/parity roadmap/support matrix, CI fuzz smoke, Phase 13 qualification/closeout reports.

Work:
- Document exact experimental default-off subset, limits, restart behavior and rejected transports.
- Populate `docs/validation/phase-13-qualification.md` with enumerated rows and evidence links for: RGB, RGBA, zlib, PNG, chunking, replace/place/delete/query, malformed/overflow/cancel/EOF, scroll/history/ED3, erase/edit, repeated reflow, alternate, pane/tab/window transfer, restore, close, deadline/saturation, context loss/upload failure, bounds/reservation rollback, text-only allocation/performance and disabled idle cadence.
- Record exact automated command/output or Windows/OpenGL manual steps/result for every row; no `N/A` without an exclusion citation.
- Run independent final diff review and architecture drift check. Support claim remains experimental/default-off; no claim for excluded transports/animation/Sixel/iTerm.

Success criteria: every enumerated row is Pass with reproducible evidence or the feature remains unclaimed/disabled; support matrix matches evidence; the exact 13.16 gate block below passes; Phase 14 can target termimage without changing Cell or GL ownership. Any failure blocks the close-out merge and support claim; create a named repair slice from current `dev`, revise this plan if scope changes, rerun all final gates, and only then retry 13.16.

Exact 13.16 close-out gates:

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go test -race -tags glfw ./internal/frontend/glfwgl ./internal/frontend/gpu ./internal/mux -count=1
go run ./scripts/check-maturity-gates.go
go run ./scripts/check-phase13-imports.go
git diff --check
go test ./internal/vt -run '^$' -fuzz=FuzzControlStringFraming -fuzztime=60s
go test ./internal/vt -run '^$' -fuzz=FuzzParserAdvanceDoesNotPanic -fuzztime=60s
go test ./internal/kitty -run '^$' -fuzz=FuzzKittyAdapter -fuzztime=60s
go test ./internal/kitty -run '^$' -fuzz=FuzzKittyDecode -fuzztime=60s
go run ./scripts/capture-phase13-benchmark.go -suite text -out phase13-final.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-baseline.txt phase13-final.txt
go run ./scripts/capture-phase13-benchmark.go -suite glfw -out phase13-gl-final.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-gl-baseline.txt phase13-gl-final.txt
go run ./scripts/capture-phase13-benchmark.go -suite control -out phase13-control-final.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-control-string-baseline.txt phase13-control-final.txt
```

## Required gates after every slice

Documentation-only 13.0a runs its exact gates above. Every code/test slice 13.0b–13.15 runs the common block below; 13.16 runs its stricter exact close-out block above:

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
go run ./scripts/check-phase13-imports.go
git diff --check
go test ./internal/vt -run '^$' -fuzz=FuzzParserAdvanceDoesNotPanic -fuzztime=5s
go run ./scripts/capture-phase13-benchmark.go -suite text -out phase13-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-baseline.txt phase13-candidate.txt
```

13.1 establishes the control-string baseline. Slices 13.2–13.15 also run:

```text
go run ./scripts/capture-phase13-benchmark.go -suite control -out phase13-control-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-control-string-baseline.txt phase13-control-candidate.txt
```

GLFW slices also run:

```text
go test -race -tags glfw ./internal/frontend/glfwgl ./internal/frontend/gpu ./internal/mux -count=1
go run ./scripts/capture-phase13-benchmark.go -suite glfw -out phase13-gl-candidate.txt
go run ./scripts/compare-phase13-baseline.go docs/validation/phase-13-gl-baseline.txt phase13-gl-candidate.txt
```

A slice adds focused tests/fuzz for its boundary; new fuzz targets run >=10 s during development and >=60 s before that slice merge, 13.15, and 13.16. If a target does not exist yet, the always-present parser fuzz is the mandatory smoke. Every touched production file satisfies maturity line limits. Results are recorded in the slice validation note.

## Performance gate

13.0b records existing text-only parser/core/snapshot/GL-disabled baselines. APC discard/rejected-overflow baselines begin in 13.1 and compare against that slice for later regressions. New enabled-only benchmarks establish a first-result budget in their introducing slice, then become regression baselines. The same machine/toolchain/config, a fixed `-cpu=1`, a 5 s warm-up, 2 s benchmark intervals, ten samples, and the self-contained median/allocation gate are mandatory; installed `benchstat` adds supplemental evidence but is not allowed to bypass or weaken the hard gate.

Hard acceptance: `core.Cell` exactly 32 bytes; disabled steady-state zero image allocations; matching disabled parser/core/snapshot/draw median regression <=3%; disabled idle wake/frame count no increase; CPU/GPU/count reservations never exceed ADR/config caps.

## Global stop conditions

Stop and return to architecture/design for any Cell image field, unchecked allocation, unreserved retention, payload-to-text leakage, off-thread model/GL mutation, mutable snapshot alias, cross-context GL handle, hidden/default support advertising, external transport/animation/Sixel/iTerm/renderer selection, or unresolvable gate/review failure.

## Rollback order

Operational: `graphics.kitty.enabled=false`, restart. Code: reverse close-out/docs -> activation -> draw -> cache/capability -> runtime/decoder/adapter -> dormant config -> mux projection/lifecycle -> core lifecycle/transaction -> store. Baseline harness/import checks may remain as invariants or be reverted independently. Generic APC/DCS framing may remain only after independent fuzz/performance qualification. Each branch may be reverted independently because no earlier slice exposes behavior that requires a later lifecycle slice.
