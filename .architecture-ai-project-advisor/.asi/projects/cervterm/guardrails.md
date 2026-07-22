# CervTerm Phase 14 Guardrails — Sixel and iTerm Inline Images

Date: 2026-07-23
Authority: ADR-0014 and ADR-0016

## Required

- [ ] `core.Cell` remains text-only and exactly 32 bytes.
- [ ] Phase 14 uses the existing pane/process image budgets, owner transaction, screen lifecycle, detached snapshots and context-local GL cache.
- [ ] Sixel/iTerm leaves import no core/render/mux/frontend/config/VT package and no filesystem/network/process/unsafe facility.
- [ ] Encoded/decoded/pixel/image/placement/chunk/operation/worker resources retain immutable hard caps and exact ownership rollback.
- [ ] Parser overflow/cancel/reset/EOF discards atomically through the correct terminator and leaks no payload as text.
- [ ] Workers mutate no parser/core/mux/session/reply/GL state; owner revalidates and commits.
- [ ] Cursor remains unchanged; primary anchors include scrollback, alternate anchors are top-relative.
- [ ] Internal high-half IDs cannot collide with or be addressed by Kitty low-half wire IDs.
- [ ] Ephemeral resources retire atomically when their final placement retires; Kitty resources remain durable.
- [ ] All protocols share one bounded scheduler; pending limits are not multiplied.
- [ ] Sixel/iTerm emit no replies and cannot disturb Kitty/terminal reply order.
- [ ] `imagesEnabled` creates one shared model/cache only when Kitty, Sixel or iTerm is enabled; all-disabled remains literal nil/no idle work.
- [ ] Every slice is independently tested, reviewed, committed and locally merged.

## Forbidden

- [ ] Renderer/backend selection or GPU handles crossing contexts.
- [ ] Image identity/pointers in `core.Cell` or mutable pixels in snapshots.
- [ ] File/path/URL/temp/shared-memory/download/write transports.
- [ ] Protocol-specific worker pools, budgets, stores or GL caches.
- [ ] Sixel cursor scrolling/DECSDM, iTerm cursor effects, animation or invented replies.
- [ ] Forced decoder-preemption claims, autonomous model timers or early pane-slot release.
- [ ] Partial publication, success-before-commit, prior-generation replacement on failure, payload/metadata logging.
- [ ] Stable support claims before automated and real Windows/OpenGL qualification.

## Merge gates

- [ ] Focused tests and >=60 s touched fuzz once target exists.
- [ ] Full untagged/tagged tests and vet; full race plus tagged focused race.
- [ ] Maturity/import checks and `git diff --check`.
- [ ] Relevant portable ten-run baseline: no new disabled allocations/wakes and <=3% unexplained median regression.
- [ ] Independent boundary review has no blocker.

## Stop conditions

Stop for architecture review on unchecked arithmetic, payload leakage, external I/O need, off-owner mutation, undefined lifecycle, ownership leak, cap widening/multiplication, hidden activation/support, renderer changes or any required gate failure.
