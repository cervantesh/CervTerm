---
date: 2026-07-23T20:35:00-04:00
author: cervantesh
commit: e9f9b2c0666f1392c8a1b74ca6b6cb0be261b000
branch: review/architecture-maturity-audit
repository: cervantesh/CervTerm
topic: architecture-maturity-remediation
phase_count: 8
work_package_count: 8
execution_stage_count: 10
status: ready
tags: [architecture, clean-code, grasp, ownership, domain-boundaries, remediation]
parent: .rpiv/artifacts/architecture-reviews/2026-07-23_cervterm-maturity-clean-code-grasp-domain-leaks.md
---

# Architecture Maturity Remediation Implementation Plan

## Objective

Resolve all 30 accepted findings and continue evidence-driven remediation until every independently re-scored dimension reaches at least 8.0. Per developer direction, begin with behavior-preserving decomposition of `App`, `Mux`, and `fontglyph`; then repair ownership, multi-window correctness, trust/security, lifecycle, closed vocabularies, and smaller defects.

The implementation is strictly one slice at a time. Every slice starts from the newly merged `origin/main`, uses its own branch and PR, and merges only after focused and repository-wide gates pass.

## Inputs Used

- Architecture review: `docs/architecture-maturity/accepted-findings.md`, canonical LF SHA-256 `ba4e12b929960dc753ab3b6546038b11753b1dc1c099f94c860d3e97b1b22c4a` (source artifact: `.rpiv/artifacts/architecture-reviews/2026-07-23_cervterm-maturity-clean-code-grasp-domain-leaks.md`)
- Production baseline: `e9f9b2c` (`v0.10.0-beta.1` merge baseline)
- Architecture: `docs/architecture.md`
- Guardrails: `.architecture-ai-project-advisor/.asi/projects/cervterm/guardrails.md`, canonical LF SHA-256 `c418ef9cd47e1d50a49c23ea2d748657b88fee3d1f1c954066d6113ebba3d654`, plus preserved Phase 15 qualification boundaries
- ADRs: existing ownership, persistence, external-effects, native-host, and terminal-image decisions
- Precedents: typed actions/config PRs #121–143, native-window PRs #183–191, Phase 13/14 VT/image work, App search-controller extraction PRs #79–82, font PRs #149–154

## Fixed Decisions

1. **Current main only.** Never implement from `fix/windows-version-resource-from-tag` (`61ece0e`) or another dirty/stale worktree.
2. **Developer-prioritized architectural concentration first.** After Phase 0, execute the `App`, `Mux`, and `fontglyph` decomposition sub-slices before other findings. These initial moves are behavior-preserving only; correctness and ownership changes remain separate later slices.
3. **Behavior before movement.** No commit combines an observable correction with package/file movement.
4. **One atomic delivery unit per branch/PR.** Unrelated findings never mix. Effort-L findings may use sequential suffix PRs (`a`, `b`, `c`) when one compatibility facade protects multiple subsystem moves.
5. **Commit before large change.** Every effort-L slice begins with a passing characterization/golden/benchmark commit before adding a seam or moving code.
6. **No weakened boundaries.** Preserve v1/v2 compatibility, default-off experimental features, renderer-selection exclusion, redaction, resource caps, stable IDs, fresh-session persistence, and main-thread GLFW/OpenGL ownership.
7. **ADRs before direction changes.** Owner capability, shared multi-window configuration, generated configuration/action authorities, VT framing projection, and module extraction require preflight/ADR acceptance before their implementation slice.
8. **Score floor is evidence, not aspiration.** The goal is incomplete until a fresh two-team/two-round audit, using the same rubric and clean current main, scores every dimension ≥8.0; any lower dimension creates additional one-by-one remediation slices.

## Commit and Merge Protocol

For every slice:

1. Fetch `origin/main`; verify clean status and that the previous slice is merged.
2. Create `arch/<finding-id>-<topic>` from current `origin/main`.
3. Run the slice baseline tests before edits.
4. Use these atomic commit classes:
   - **T** — characterization/golden/fuzz/benchmark tests only.
   - **A** — additive type/interface/capability; old path remains authoritative.
   - **B** — behavior correction plus regression tests; no moves/renames.
   - **M** — mechanical move/split/package/import update; no behavior change.
   - **W** — wire existing behavior through an already-tested seam.
   - **G** — generated parity, guards, docs, and obsolete-code removal.
5. Never commit red tests. Never amend a merged slice.
6. Push, open PR, wait for Windows/Linux CI and CodeQL, then use a true merge commit (neither squash nor rebase-and-merge) so the PR boundary and `T/A/B/M/W/G` commits remain independently revertible; delete the branch.
7. Rebase the next slice from the new `origin/main`; never stack unmerged architecture slices.

Large slices use distinct commits `T → A → M → W → G`; `M` and `W` are never combined. If a behavior defect is discovered during `M`, revert the move and repair behavior in a separate branch first.

Every slice records two relations: `execution_predecessor` (the previously merged PR) and `semantic_depends_on` (findings/contracts required for formal closure). Its PR description carries an explicit changed-path allowlist; CI rejects paths outside that allowlist except generated artifacts declared in advance.

A characterization that preserves a known defect is named `KnownDefect_<finding-id>_*`, cites its expiry slice, and must be replaced by the corrected invariant when that slice merges. It may never be treated as desired behavior.

## Common Verification Gate

Required before every merge:

```text
go run ./scripts/check-maturity-gates.go
go test ./...
go vet -unsafeptr=false ./...
go test -tags glfw ./internal/frontend/glfwgl ./cmd/cervterm -count=1
go run ./scripts/check-phase15-recovery.go -race
```

Additionally:

- Mux/ownership: `go test -race ./internal/mux ./internal/termimage ./internal/layoutstate -count=1`
- Config/actions: `go test ./internal/config ./internal/script ./internal/action -count=1`
- Accessibility: package tests, tagged UIA/candidate-geometry tests, and Windows real-GUI evidence where geometry changes
- PTY: race tests plus Windows ConPTY and Linux PTY CI
- VT: all parser/public-output tests plus `FuzzParserAdvanceDoesNotPanic`, `FuzzControlStringFraming`, `FuzzPublicOutputProjectionDualOracle`, `FuzzPublicOutputProjectionSelectedEnvelope`, `FuzzSixelDCSTransportSelection`, and `FuzzOSC1337SelectedTransport`; 2-second smoke per PR and 60-second runs before security close-out
- Font/frontend decomposition: pinned allocation/performance captures with no unexplained material regression

## Authoritative Execution Order — Revision 2

This order supersedes the numeric order of the detailed work-package sections below. Numeric slice IDs remain stable for traceability to the reviewed plan and findings.

| Stage | Execute in this exact order | Why now |
|---:|---|---|
| 0 | Phase 0 baseline/preflight/ADRs | Pin behavior, performance and architecture before movement. |
| 1 | `6.3a → 6.3b → 6.3c` (`App` preparatory) | Developer-selected first work; private same-package delegation only. L1-01 remains partial. |
| 2 | `6.2a → 6.2b → 6.2c` (`Mux` preparatory) | Private delegation under existing Mux entry points. L3-01 remains partial. |
| 3 | `5.5a → 5.5b → 5.5c` (`fontglyph`) | Complete conditionally under the approved package DAG and face/cache ownership ADR. |
| 4 | `3.1 → 3.2 → 3.3 → 3.4 → 3.5` | Executable ownership, bootstrap, critical per-window geometry, shared config, global policy. |
| 5 | `5.4 → 4.3 → 4.7 → 4.9 → 4.10 → 4.8 → 5.1 → 6.2d` | Security/trust/lifecycle/events, then formal thin-Mux closure. |
| 6 | `4.2 → 4.1 → 2.1 → 2.4 → 2.2 → 2.3 → 4.4 → 4.5 → 4.6` | Medium user-visible and API correctness. |
| 7 | `5.2 → 5.3 → 6.1a → 6.1b → 6.1c → 6.3d` | Single authorities, temporal contracts, then formal thin-App closure. |
| 8 | `1.1 → 1.2 → 1.3` | Lowest-risk accounting/fail-soft/dead-surface cleanup. |
| 9 | Phase 7 score-closure loop | Re-audit; create new slices for any dimension below 8.0. |

## Work-package Overview

| Package | Goal | Findings | Complexity | Architectural risk |
|---:|---|---:|---|---|
| 0 | Baseline, ADR and branch discipline | gates only | S | Low |
| 1 | Tiny fail-soft/accounting cleanup | 3 | S | Low |
| 2 | Small API and mux correctness | 4 | S–M | Low–Medium |
| 3 | Executable ownership and urgent multi-window repair | 5 | M–L | High/Critical |
| 4 | Medium lifecycle, trust and transaction repairs | 10 | M | Medium–High |
| 5 | Closed vocabularies and security-sensitive structure | 5 | L | High/Critical |
| 6 | Controller and subsystem decomposition | 3 | L | High |
| 7 | Independent score closure | dynamic | M–L | Evidence-gated |

---

## Phase 0 — Baseline and Architecture Gates

### Scope

Create the implementation baseline before production edits.

### Steps

1. Refresh T50 architecture description so it reflects the current multi-window/mux/image system rather than the obsolete single-window model.
2. Record preflight for the 30-finding program.
3. Accept ADRs before their dependent phases:
   - executable mux owner capability and wrong-owner behavior;
   - process-owned shared-config transaction and per-window mux geometry ownership;
   - generated/declarative config and action authority boundaries;
   - parser framing event/public-output projection direction;
   - decomposition constraints for `App`, `Mux`, and `fontglyph`.
   - `fontglyph` package DAG: root facade may import `discovery`, `cache`, `shape`, `raster`, `platform`, and `internal/face`; `discovery→fontdesc`; `cache→face`; `shape/raster/platform→face,fontdesc,unicodecluster,unicodeprops`; `face→fontdesc`. These are the complete allowed edges; public subsystem packages never import the facade or one another, while exact private `internal/face` and stable-leaf imports are allowed. `cache.Lease` owns exactly one pin and closes idempotently; native handles close through the face owner.
   - versioned scoring protocol at `docs/architecture-scoring-protocol.md`, canonical LF SHA-256 `1dd499835cd9dbff0c7bb78eae0aaeb18318fa8af8216293d9d5ead2bff68be2`; scored-file manifest SHA-256 `fb3f0e052fe56eacd07d99e091879987e423a59f66815a2697313679e28c836c`. It defines per-item 0/1/2 anchors, platform/tag inclusion, canonical commands, exact `Overall`, blind team independence, lower-team acceptance and no upward reconciliation.
4. Capture immutable baseline evidence under `docs/validation/architecture-maturity-phase0/` with `scripts/capture-architecture-maturity-baseline.go`:
   - full/common gate;
   - all-tracked-production-file package graph (37 packages, 71 local edges across every GOOS/tagged file, zero cycles);
   - Phase 15 performance captures;
   - honest two-window geometry/lifecycle trace plus explicit record that shared process-config generation is absent and owned by L1-02/ADR-0018;
   - accessibility and public-output goldens.
5. Commit architecture artifacts and baseline tests separately before execution Stage 1.
6. Re-run architecture preflight immediately before Phases 3, 5, and 6; assumptions from Phase 0 are not automatically valid after intervening merges.

### Success Criteria

- [ ] Clean implementation worktree at current `origin/main`.
- [ ] Architecture context reflects current production shape.
- [ ] Required ADRs accepted with rollback and stop conditions.
- [ ] Baseline commands and measurements pass and are reproducible.
- [ ] No production behavior changed.

### Rollback

Revert only the planning/baseline commit. No production state is affected.

---

## Phase 1 — Tiny Fail-soft and Accounting Repairs

Each slice is an independent branch and PR.

### Slice 1.1 — L4-06 transactional background budget

- **Files:** `internal/background/budget.go`, focused tests.
- **Change:** roll back encoded-byte reservation when decoded reservation fails; exact reusable-budget state test.
- **Commits:** `T → B`.
- **Risk/complexity:** Low / S.
- **Rollback:** revert behavior commit.

### Slice 1.2 — L1-07 malformed selection snapshots

- **Files:** `internal/selection/selection.go`, unit/fuzz tests.
- **Change:** checked `rows*cols` and backing-length validation before slicing; fail soft without panic.
- **Commits:** `T → B`.
- **Risk/complexity:** Low / S.

### Slice 1.3 — L4-04 disconnected theme package

- **Files:** `internal/theme/*`, import/maturity guard tests.
- **Change:** delete superseded package after proving no production consumer; do not introduce another palette authority.
- **Commits:** `T → G`.
- **Risk/complexity:** Low / S.

### Phase 1 Success Criteria

- [ ] Every slice merged separately.
- [ ] Malformed selection never panics.
- [ ] Failed background pin restores exact prior accounting.
- [ ] Production dependency graph remains acyclic and `theme` ambiguity is removed.
- [ ] Common gate passes after each merge.

---

## Phase 2 — Small API and Mux Correctness

### Slice 2.1 — L1-04 UIA focus semantics

- **Files:** `internal/frontend/glfwgl/windows_uia_provider.go`, UIA provider/helper tests.
- Separate `IsKeyboardFocusable` capability from `HasKeyboardFocus` state.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S.
- Test non-focused but focusable terminal/input nodes.

### Slice 2.2 — L4-05 Ctrl-V ownership

- **Files:** `internal/input/encoder.go`, `internal/input/*_test.go`, `internal/frontend/glfwgl/action_bindings.go`, binding tests.
- Remove paste policy from the encoder; frontend bindings alone intercept paste. Unbound Ctrl-V emits `0x16`.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S.

### Slice 2.3 — L2-02 Lua scrollbar parity

- **Files:** `internal/script/api.go`, script API tests, `internal/config/config.go`, scrollbar schema/runtime/template tests.
- Expose/decode `mode`, `stable_gutter`, and `animation_fps`; preserve legacy `enabled` and strict v2 scopes.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S; public API.

### Slice 2.4 — L3-05 topology revisions

- **Files:** `internal/mux/topology.go`, `tree.go`, `model_tabs.go`, topology/ratio/stale-confirmation tests.
- Increment affected tab/window revisions for every structural and ratio mutation.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S.

### Phase 2 Success Criteria

- [ ] Accessibility, input and Lua public behavior have focused regressions.
- [ ] Every topology mutation advances truthful revisions.
- [ ] Common gate and Windows tagged tests pass after each slice.

---

## Phase 3 — Executable Ownership and Urgent Multi-window Repair

This is the deliberate risk exception. The owner seam is complex but must precede the sole High-severity geometry defect.

### Slice 3.1 — L3-02 executable owner-thread use

- **ADR gate:** owner capability/serialized mutation direction accepted.
- **Files/areas:** `internal/mux/mux.go`, new `owner.go`, `session_registry.go`, image scheduling/store integration, `internal/termimage/store.go`, frontend mux adapter and race tests.
- **Change:** wrong-owner mutations fail detectably; owner path stays lock-free; cover every mutation family under race tests.
- **Commits:** `T → A → W → G`.
- **Risk/complexity:** High / L.
- **Stop:** owner identity remains documentary or requires off-thread GLFW/OpenGL.

### Slice 3.2 — L3-04 bootstrap publication transaction

- **Depends on:** 3.1.
- **Files:** `internal/mux/mux.go`, `session_registry.go`, `session_factory.go`, bootstrap/fault tests.
- Prepare ID/session/reader ownership before publishing bootstrap fields; exact zero-state rollback.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S.

### Slice 3.3 — L3-03 per-window mux geometry

- **Depends on:** 3.1 and 3.2; no unrelated slice may intervene.
- **Files:** `internal/mux/mux.go`, `resize.go`, window/tab model and transfer files, `internal/frontend/glfwgl/app_loop.go`, window/zoom/divider/transfer tests.
- Replace one global bounds field with stable `WindowID`-addressed geometry/layout.
- Required operation matrix for two differently sized windows: active/inactive resize, split and ratio mutation, close/collapse, pane zoom, cross-window pane/tab transfer, directional focus geometry, workspace switch, PTY deferred settlement, restore, and final-window close.
- **Commits:** `T → A → B → G`; **Risk/complexity:** Critical / L.
- **Manual gate:** two-window real-GUI smoke with different sizes and mixed DPI.

### Slice 3.4 — L1-02 atomic shared configuration

- **Depends on:** 3.1 and 3.3.
- **Files:** `internal/frontend/glfwgl/window_controller_runtime.go`, `reload.go`, `app_host.go`, `window_controller.go`, config runtime-scope integration and multi-projection tests.
- Process-owned config revision/transaction; prepare all projection resources before atomic publication; rollback every projection on failure. Runtime setters use the same coordinator.
- **Commits:** `T → A → B → G`; **Risk/complexity:** High / M.

### Slice 3.5 — L3-06 global pane-policy reach

- **Depends on:** 3.4 so all-window policy publication shares the authoritative config revision.
- **Files:** `internal/mux/viewport.go`, model/registry pane enumeration, frontend live-config application, inactive-tab/window policy tests.
- Apply scrollback/cursor policy to every existing pane while keeping effective config/revision synchronized.
- **Commits:** `T → B`; **Risk/complexity:** Medium / S.

### Phase 3 Success Criteria

- [ ] Wrong-owner calls are detected in tests.
- [ ] Two windows cannot overwrite geometry or effective config.
- [ ] Failed bootstrap/config preparation is old-or-new with exact cleanup.
- [ ] Every existing pane reflects the authoritative global policy and config revision across inactive tabs/windows.
- [ ] Mux race suite, tagged frontend suite, common gate, and GUI qualification pass.

### Rollback

Revert the complete slice merge. Never retain half of owner, geometry, or shared-config transactions.

---

## Phase 4 — Medium Lifecycle, Trust and Transaction Repairs

### Slice 4.1 — L1-03 truthful accessibility words

- **Files:** `internal/accessibility/range.go`, range tests, Windows native text-range adapter/tests.
- Bounded Unicode-aware word segmentation distinct from rows/lines; punctuation, marks, wraps and empty-row tests.
- **Commits:** `T → A → B`; **Risk/complexity:** Medium / M.

### Slice 4.2 — L1-05 checked coordinate projection

- **Files:** candidate-geometry helpers, GLFW conversion adapter, Windows accessibility projection factory, coordinate/ABI/GUI tests.
- Central framebuffer→window→screen conversion shared by accessibility and candidate geometry.
- **Commits:** `T → A → B`; **Risk/complexity:** High native / M.

### Slice 4.3 — L3-07 persisted launch-intent trust boundary

- **Files:** `internal/vt/parser_osc.go`, `internal/mux/fresh_session.go`, layout persistence exporter/model/validation and migration tests.
- Keep OSC CWD as observed metadata; persist configured launch intent by default; preserve on-disk schema.
- **Commits:** `T → A → B`; **Risk/complexity:** High security/on-disk / M.

### Slice 4.4 — L2-06 strict deterministic config decoding

- **Files:** `internal/config/config.go`, `lua.go`, `diagnostic.go`, strict/legacy/determinism tests.
- Sort map-backed diagnostics; propagate type/quick-select errors; preserve tested legacy compatibility.
- **Commits:** `T → B`; **Risk/complexity:** Medium public API / M.

### Slice 4.5 — L2-03 immutable quick-select derived state

- **Files:** config model, document apply/validation, quick-select frontend consumer and clone/mutation tests.
- Make compiled rules private/prepared; expose detached immutable access.
- **Commits:** `T → A → B`; **Risk/complexity:** Medium / M.

### Slice 4.6 — L2-04 bounded script timers/status

- **Files:** `internal/script/timers.go`, `status.go`, runtime integration and bound/coalescing tests.
- Maximum counts, minimum periods, bounded IDs/text and deterministic rejection/coalescing.
- **Commits:** `T → A → B`; **Risk/complexity:** Medium / M.

### Slice 4.7 — L2-07 Teal publication ownership

- **Files:** `internal/config/teal_publish.go`, platform publication helpers and concurrent-publisher/fault tests.
- Remove age-only cross-process deletion; publisher-owned cleanup preserves atomic replacement and rollback.
- **Commits:** `T → A → B`; **Risk/complexity:** High on-disk / M.

### Slice 4.8 — L3-09 typed protocol outcomes and one clock

- **Files:** shared mux image scheduler, Kitty/Sixel/iTerm mux adapters, protocol adapters, `termimage.Store`, fake-clock/race tests.
- Replace erased `any` results; propagate one injected clock through queue, adapter, store and deadline checks.
- **Commits:** `T → A → W → G`; **Risk/complexity:** High / M.

### Slice 4.9 — L3-10 close/store serialization

- **Files:** mux/model tab-close transaction, `internal/layoutstate/store.go`, close/store fault and same-path tests.
- Exact rollback after detach failure; canonical-path in-process coordination; external processes remain documented atomic last-writer-wins.
- **Commits:** `T → A → B`; **Risk/complexity:** High on-disk / M.

### Slice 4.10 — L4-03 observable PTY completion

- **Files:** PTY session interface, Unix/Windows implementations, mux session lifecycle and platform tests.
- Single-consumer wait/exit result; deterministic natural-exit vs close; idempotent cleanup.
- **Commits:** `T → A → B`; **Risk/complexity:** High cross-platform / M.

### Phase 4 Success Criteria

- [ ] Accessibility and geometry are semantically correct and GUI-qualified.
- [ ] Terminal output cannot silently determine future persisted launch CWD.
- [ ] Config and quick-select invariants are deterministic and immutable.
- [ ] Script/protocol/store/PTY ownership is bounded and race-clean.
- [ ] On-disk operations remain backwards-compatible and old-or-new.

---

## Phase 5 — Closed Vocabularies and Security-sensitive Structure

Every slice is effort L and must begin with a separate passing `T` commit.

### Slice 5.1 — L3-08 typed event vocabulary

- **Files:** mux event types/addressing, every producer, frontend/script consumers, producer/consumer coverage matrix.
- Typed payload variants/constructors capture stable addresses at creation; generic fields are removed only after all consumers migrate.
- **Commits:** `T → A → W → G`; **Risk/complexity:** High / L.

### Slice 5.2 — L2-05 single action authority

- **Files:** action registry/types/codecs, Lua actions, frontend executor, command palette, generator/generated parity tests.
- Deterministic declarative action specification derives metadata/codecs/discovery/coverage and Lua exposure; handlers remain typed and handwritten.
- **Commits:** `T → A → W → G`; **Risk/complexity:** High / L.

### Slice 5.3 — L2-01 one configuration-leaf catalog

- **Files:** config model/schema/Lua/diff/runtime/template/diagnostics plus new catalog/generator/generated files and frozen v1/v2 goldens.
- Schema catalog becomes authority for repetitive projections; complex and legacy fields retain explicit adapters.
- **Commits:** `T → A → W` per projection `→ G`; **Risk/complexity:** High compatibility / L.

### Slice 5.4 — L4-01 parser sink and framing-derived redaction

- **Files:** VT parser/control-string/OSC/public-output files, mux projection consumers, parser/redaction tests and every named fuzz oracle.
- Add command sink without byte changes, expose framing events, prove dual-oracle parity, remove shadow FSM last.
- **Commits:** `T → A → W → B → G`; **Risk/complexity:** Critical security / L.

### Slice 5.5a — L4-02 font discovery/index/cache extraction

- **Files:** `fontindex.go`, `font_cache.go`, face loading/cache tests and new compatibility package/facade.
- **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 5.5b — L4-02 font resolution and shaping extraction

- **Files:** face resolver, fallback/rules, run shaping, backend facade and shaping/identity tests.
- **Depends on:** 5.5a; **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 5.5c — L4-02 raster/color/native extraction

- **Files:** raster paths, COLR/SVG/bitmap paint, DirectWrite adapters, backend facade and platform/performance tests.
- **Depends on:** 5.5b; **Commits:** `T → A → M → W → G`; separate branch/PR.
- Every 5.5 PR preserves cache budgets, glyph outputs, startup memory and frame/allocation baselines.

### Phase 5 Success Criteria

- [ ] One authority drives config/actions/events without weakening strict diagnostics or compatibility.
- [ ] `go generate` produces a clean tree and complete coverage matrices.
- [ ] Public-output bytes/redaction remain byte-exact under every fuzz oracle.
- [ ] Font caches, budgets, glyph outputs, startup memory and frame performance remain within pinned baselines.

### Stop Conditions

Stop immediately for redaction leakage, v1/v2 drift, generated/manual divergence, platform font regression, allocation increase, or a new package cycle.

---

## Phase 6 — Controller and Subsystem Decomposition

These early `App`/`Mux` slices are preparatory parity-only extractions. They do not close L1-01/L3-01 and may not transfer final ownership, add exported ports, bypass `App`/`Mux`, copy mutable state, or retain `*App`/`*Mux` as a service locator. Formal closure happens in 6.3d/6.2d after semantic dependencies merge.

### Slice 6.1a — L1-06 per-pane render context

- **Files:** `app_draw.go`, pane UI state, render context and trace/golden tests.
- Pin draw ordering, add additive context, wire one pane path, remove scratch swap only after parity.
- **Commits:** `T → A → W → G`; separate branch/PR.

### Slice 6.1b — L1-06 ordered input routes

- **Files:** callbacks, action bindings, mouse/tab/modal routes and precedence trace tests.
- **Depends on:** 6.1a; **Commits:** `T → A → W → G`; separate branch/PR.

### Slice 6.1c — L1-06 typed reload states

- **Files:** reload entry/prepare/commit/rollback files, config watcher/runtime ownership and fault tests.
- **Depends on:** 6.1b; **Commits:** `T → A → W → G`; separate branch/PR.

### Slice 6.2a — L3-01 session-ingress coordinator

- **Files:** mux drain/ingress, session registry, parser/reply event tests.
- **execution_predecessor:** 6.3c. **semantic_depends_on:** none for preparation; L3-02/L3-03/L3-04/L3-08/L3-09/L3-10 for closure. Preserve current ingress behavior, including tagged known defects. **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 6.2b — L3-01 protocol-scheduling coordinator

- **Files:** shared image scheduler and mux protocol dispatch, with existing stores/adapters unchanged.
- **execution_predecessor:** 6.2a. Extract current scheduling behavior unchanged; L3-09 repairs type erasure/clock ownership later. **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 6.2c — L3-01 restore coordinator

- **Files:** mux restore/fresh-session transaction and frontend restore adapter tests.
- **execution_predecessor:** 6.2b. Preserve restore publication semantics. **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 6.2d — L3-01 formal thin-Mux closure

- **execution_predecessor:** 5.1. **semantic_depends_on:** L3-02, L3-03, L3-04, L3-08, L3-09, L3-10 and preparatory 6.2a-c.
- Remove temporary delegation shims; verify `Mux` retains topology, identity and lifecycle only. Session ingress, protocol scheduling and restore coordination remain private typed collaborators and cannot expose bypass entry points.
- **Commits:** `T → A → M → W → G`; separate branch/PR. L3-01 closes only here.

### Slice 6.3a — L1-01 action/input controllers

- **Files:** `app.go`, action executor, callbacks/bindings and new same-package controllers.
- **execution_predecessor:** Phase 0. **semantic_depends_on:** L1-02 through L1-06 for closure only. Preserve input/action behavior and tag known-defect goldens with expiry slices. **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 6.3b — L1-01 render/reload controllers

- **Files:** draw/frame/reload ownership and new same-package controllers.
- **execution_predecessor:** 6.3a. Preserve draw/reload behavior and ownership; **Commits:** `T → A → M → W → G`; separate branch/PR.

### Slice 6.3c — L1-01 scripting/native capability controllers

- **Files:** script host/events, accessibility/IME/native projection lifecycle, `app.go` facade.
- **execution_predecessor:** 6.3b. Preserve script/native lifecycle; **Commits:** `T → A → M → W → G`; separate branch/PR.
- After 6.3c, L1-01 remains **partial**; `App` stays authoritative until 6.3d.

### Slice 6.3d — L1-01 formal thin-App closure

- **execution_predecessor:** 6.1c. **semantic_depends_on:** L1-02, L1-03, L1-04, L1-05, L1-06 and preparatory 6.3a-c.
- Remove temporary forwarding/shims; verify `App` retains composition and native lifetime only. Extracted controllers use narrow owned ports and no generic service locator.
- **Commits:** `T → A → M → W → G`; separate branch/PR. L1-01 closes only here.
- Final criterion: `App` retains composition/lifetime coordination only; no extracted controller becomes a service locator.

### Phase 6 Success Criteria

- [ ] Early 6.3a-c and 6.2a-c are explicitly partial/preparatory; formal L1-01/L3-01 closure is evidenced only by 6.3d/6.2d after semantic dependencies.
- [ ] No behavior trace, import direction, resource count, allocation or performance baseline changes unexplained.
- [ ] No new interface exists without at least two concrete consumers or a clear Protected Variations boundary.
- [ ] Common, race, fuzz, recovery, package and real-GUI gates pass.

---

## Phase 7 — Independent Score Closure Loop

### Baseline and required floor

| Dimension | Baseline | Required final |
|---|---:|---:|
| Architecture maturity | 7.4 | ≥8.0 |
| Clean Code | 6.0 | ≥8.0 |
| GRASP responsibility assignment | 6.4 | ≥8.0 |
| Dependency graph hygiene | 8.1 | ≥8.0 (must not regress) |
| Domain isolation | 7.0 | ≥8.0 |
| Ownership and transactions | 7.5 | ≥8.0 |
| Test and guardrail maturity | 8.8 | ≥8.0 (must not regress) |
| Overall | 7.3 | ≥8.0 |

### Procedure

1. Audit all non-test, non-generated production Go under `cmd/` and `internal/`, including every newly added package, with two blind independent Round-1 teams and Round-2 cross-examination.
2. Use versioned anchors for each 0–10 dimension. `Overall` is the arithmetic mean of the seven component dimensions before Overall, reported to one decimal without rounding upward for acceptance.
3. Each team publishes a final score for every row. Acceptance requires the **minimum of the two teams’ final scores** for every component and Overall to be ≥8.0; scores are never averaged across teams, reconciled upward, weighted away, or rounded up.
4. Ground every score in `file:line`, package-graph metrics, tests, performance, ownership and domain-boundary evidence.
5. If any row is below 8.0, or any accepted finding at any severity remains open, create new one-by-one slices ordered by current architectural risk.
6. Repeat implementation and re-audit until every row passes and all 30 accepted findings plus every newly accepted finding are closed.

### Success Criteria

- [ ] Minimum final score across both teams is ≥8.0 for Architecture maturity, Clean Code, GRASP, Dependency graph hygiene, Domain isolation, Ownership/transactions, Test/guardrail maturity, and Overall.
- [ ] Zero package cycles and no regression in package instability or domain direction without accepted ADR.
- [ ] All 30 accepted findings and every newly accepted finding are closed, including Low severity.
- [ ] CI, CodeQL, race, fuzz, recovery, package, performance and required GUI gates pass on the scored commit.
- [ ] Final report identifies commit, environment, scoring-protocol version/hash, inclusion rule, both teams, both rounds, per-team scores and evidence.

---

## Ordering Constraints

```text
Phase 0 baseline/ADRs
  ↓
App preparatory: 6.3a → 6.3b → 6.3c (L1-01 partial)
  ↓
Mux preparatory: 6.2a → 6.2b → 6.2c (L3-01 partial)
  ↓
fontglyph: 5.5a → 5.5b → 5.5c
  ↓
Ownership/multi-window: 3.1 → 3.2 → 3.3 → 3.4 → 3.5
  ↓
Security/trust/lifecycle: 5.4 → 4.3 → 4.7 → 4.9 → 4.10 → 4.8 → 5.1 → 6.2d
  ↓
Medium correctness: 4.2 → 4.1 → 2.1 → 2.4 → 2.2 → 2.3 → 4.4 → 4.5 → 4.6
  ↓
Authorities/temporal: 5.2 → 5.3 → 6.1a → 6.1b → 6.1c → 6.3d
  ↓
Small cleanup: 1.1 → 1.2 → 1.3
  ↓
Phase 7 independent score loop until every dimension ≥8.0
```

No slices run in parallel. Each arrow means the previous PR is merged into current main and all required gates pass before the next branch is created.

## Global Rollback Policy

- Use commit-preserving merges for architecture slices; squash merges are forbidden because they erase `T/A/B/M/W/G` rollback boundaries.
- Behavioral slice: revert its merge commit as one unit.
- Structural slice: revert in reverse commit order (`G`, wiring, move, additive seam); keep `T` if useful.
- Dependency rollback is reverse-topological: revert every merged dependent slice before reverting its prerequisite.
- On-disk slice: rollback only after proving existing files remain readable and failed writes retain exact old state.
- Generated authority: revert consumers before generated/catalog authority.
- If a move exposes a behavior defect, revert the move and repair behavior in a separate branch.

## Global Stop Conditions

Stop the current slice and do not merge if:

- the worktree contains unrelated changes;
- any required gate fails;
- a baseline-known defect regresses before its owning slice, or remains after that owning slice merges;
- before Slice 3.1, wrong-owner behavior changes beyond the Phase 0 baseline; after Slice 3.1, any wrong-owner mutation remains silent;
- before Slices 3.3/3.4, window geometry/config diverges beyond the pinned baseline; after each owning slice, any sibling divergence remains;
- publication/recovery is partial;
- sensitive values or paths leak;
- persisted state becomes unreadable;
- v1/v2 behavior, support claims or default-off flags broaden;
- redaction/parser fuzz parity changes;
- allocation or performance regression is unexplained;
- a structural commit requires behavior changes;
- a new durable ownership, persistence, side-effect or package-direction decision lacks current preflight/ADR approval.

## Final Close-out

After execution Stage 8 (`1.3`) and before Stage 9 scoring:

1. Run complete Windows/Linux CI and CodeQL.
2. Run every named VT/public-output fuzzer for at least 60 seconds and archive corpus/crash results.
3. Run Phase 15 recovery/redaction with race detection.
4. Repeat performance captures against Phase 0.
5. Execute Windows two-window/mixed-DPI/UIA/IME/close/restore/config GUI matrix.
6. Run architecture drift review: zero cycles, one authority per vocabulary, owner seam non-bypassable, defaults/persistence unchanged.
7. Enforce separate facade metrics: `App` retains composition/native-lifetime coordination only; `Mux` retains topology, identity, and lifecycle only. No extracted controller imports more packages than its source concern, no controller is stored/accessed as a generic service locator, and moved code has byte/trace/performance parity.
8. Re-score all eight rows — Architecture maturity, Clean Code, GRASP, Dependency graph hygiene, Domain isolation, Ownership/transactions, Test/guardrail maturity, and Overall — with the frozen two-team/two-round protocol.

## Independent Review Resolution

- **Blocker resolved:** stop conditions are baseline-scoped until their owning slice, then absolute.
- **Concern resolved:** bootstrap transaction now precedes per-window geometry.
- **Concern resolved:** global pane-policy reach now follows shared-config publication.
- **Concern resolved:** every slice names expected production/test/generated areas.
- **Concern resolved:** `fontglyph`, `Mux`, `App`, and temporal-protocol findings use multiple sequential suffix PRs.
- **Concern resolved:** true merge commits and reverse-topological rollback are mandatory.
- **Concern resolved:** critical geometry has a complete two-window operation matrix.
- **Suggestions applied:** repeat preflight at Phases 3/5/6; name VT fuzz durations and measurable thin-facade criteria.

Coverage verification confirms all 30 accepted IDs remain mapped exactly once. Revision-2 adversarial review requirements — preparatory-versus-formal App/Mux closure, reproducible score protocol, exact fuzz targets, explicit fontglyph DAG/ownership, distinct M/W commits, path allowlists, known-defect expiry and reproducible guardrails — are incorporated. Final recheck reports no blockers or concerns.

## Implementation Handoff

- **First production slice:** execution Stage 1, Slice 6.3a — L1-01 preparatory action/input controller extraction.
- **Before source edits:** finish Phase 0 architecture/preflight/ADR/scoring baseline and commit it.
- **Expected first branch:** `arch/l1-01a-app-action-input-prep` from current clean `origin/main`.
- **First focused command:** `go test -tags glfw ./internal/frontend/glfwgl -run 'Action|Input|Callback|Binding' -count=1`.
- **First merge requirement:** characterization `T`, additive seam `A`, mechanical move `M`, wiring `W`, cleanup `G`; common gate + CI/CodeQL; merge commit; then Stage 1 Slice 6.3b from updated main.
