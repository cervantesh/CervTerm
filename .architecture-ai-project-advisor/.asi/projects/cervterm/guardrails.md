# CervTerm Phase 13 Guardrails — Terminal Images

Date: 2026-07-20

## Required

- [ ] `core.Cell` remains text-only and exactly 32 bytes.
- [ ] Every encoded/decoded/pixel/image/placement/chunk/reply/worker/CPU/GPU resource has immutable hard caps and exact rollback ownership.
- [ ] User configuration can lower but never raise hard caps and is restart-scoped.
- [ ] APC/DCS overflow/cancel/reset/EOF discards candidates atomically and returns to ordinary text without payload leakage.
- [ ] Resource plus optional placement commit is one owner-thread transaction with one success/failure reply outcome.
- [ ] Worker decode mutates no parser/core/mux/session/GL state; stale/late results only release reservations.
- [ ] Primary/history and alternate placements are separate and every edit/erase/scroll/evict/reflow/reset/close mutation has tested semantics.
- [ ] Render/mux snapshots expose only detached descriptors and stable IDs/generations, never pixel aliases/store pointers/GL handles.
- [ ] GL texture caches are projection/context local; visible refs pin for one frame; upload/delete occurs with the owning context current.
- [ ] Disabled/default/v1 paths allocate no store/workers/cache, advertise no Kitty support and add no idle wake/frame cadence.
- [ ] Direct-data transport only; diagnostics/replies never echo payload/metadata values.
- [ ] Every slice is independently testable, reviewed, committed and locally merged before the next.

## Forbidden without new ADR/design

- [ ] Renderer/backend selection changes.
- [ ] Image fields/pointers/IDs in `core.Cell`.
- [ ] Filesystem, temp-file, shared-memory or arbitrary-path Kitty transport.
- [ ] Animation/frame composition, Sixel or iTerm adapter activation.
- [ ] Forced in-process decoder preemption claims; bounds/concurrency and late-result rejection are the security mechanism.
- [ ] GPU handles crossing panes’ destination projections/contexts.
- [ ] Partial resource publication, reply-before-commit, or replacement of prior generation on failed decode/upload.

## Merge gates

- [ ] Focused tests and fuzz for the touched boundary.
- [ ] `go test ./... -count=1`; `go test -tags glfw ./... -count=1`.
- [ ] `go vet -unsafeptr=false ./...`; tagged equivalent.
- [ ] `go test -race ./... -count=1`; tagged focused race for GLFW slices.
- [ ] `go run ./scripts/check-maturity-gates.go`; `git diff --check`.
- [ ] Touched production files remain within maturity line limits.
- [ ] Text-only benchmark/allocation/idle comparison stays within the approved ceiling.
- [ ] Independent slice review has no unresolved blocker.

## Stop conditions

Stop for architecture review on unchecked dimension math, unbounded queue/retention, undefined lifecycle transition, off-thread model/GL mutation, mutable snapshot alias, hidden support advertisement, or any requirement to weaken ADR 0014.
