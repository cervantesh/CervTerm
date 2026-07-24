---
template_version: 1
date: 2026-07-23T19:16:06-04:00
author: cervantesh
commit: e9f9b2c0666f1392c8a1b74ca6b6cb0be261b000
branch: review/architecture-maturity-audit
repository: cervantesh/CervTerm
target: .
target_kind: module
layer_count: 7
phases: []
unresolved_finding_count: 0
status: ready
tags: [architecture-review, cervterm, maturity, clean-code, grasp, dependency-graph, domain-leaks]
last_updated: 2026-07-23T19:16:06-04:00
last_updated_by: cervantesh
last_updated_note: "Final report-only assessment; implementation plan intentionally omitted by developer request"
---

# Architecture review — CervTerm maturity, Clean Code, GRASP, and domain leakage

This review audits clean `origin/main` after Phase 15, not the stale dirty feature branch from which the request originated. It covers all 435 non-test, non-generated production Go files under `cmd/` and `internal/`; tests, fuzzers, import guards, architecture docs, ADRs, and qualification artifacts are supporting evidence. Round 1 uses an independent constructive team and an independent adversarial team; Round 2 cross-examines, verifies, weakens, or falsifies every material claim before synthesis.

---

## Conventions

Each finding records grounded `file:line` evidence, current and desired state, a concrete improvement, severity, effort, blast radius, class, status, dependencies, and a cross-cutting tag. Statuses are `open`, `accepted`, `rejected`, `deferred`, or `withdrawn`.

### Layers (top → down)

| # | Layer | Production files | Primary scope |
|---|---|---:|---|
| 0 | Process entry and composition | 9 | `cmd/cervterm` |
| 1 | GLFW/native frontend and UI projection | 133 | `internal/frontend`, `accessibility`, `ime`, `modal`, `quickselect`, `selection`, `windowbounds` |
| 2 | Application API, configuration, scripting, actions, policies | 73 | `config`, `script`, `action`, `bellpolicy`, `linkpolicy`, `notificationpolicy` |
| 3 | Runtime orchestration, mux, workspace and persistence | 57 | `mux`, `layoutrestore`, `layoutstate` |
| 4 | Terminal/platform adapters and projections | 115 | `vt`, `pty`, `render`, `input`, protocols, GPU/fonts/background/theme |
| 5 | Central domain model and primitives | 44 | `core`, `termimage`, `fontdesc`, Unicode primitives |
| 6 | Cross-cutting utilities | 4 | `applog`, `buildinfo`, `metrics`, `workscheduler` |

---

## Methodology principles

### M1 — Ownership must be executable

**Origin:** C07, promoted during developer triage together with C11, C16, and C38.

**Rule.** A comment saying “owner thread only” is not an ownership boundary. Mutable aggregate state needs a capability, assertion, serialized mailbox, or API shape that makes wrong-thread use detectably invalid. Keep lock-free owner-thread performance; change public seams that silently accept concurrent misuse.

### M2 — Repair behavior before decomposing controllers

**Origin:** Round 2 separated actual multi-window, accessibility, input, and policy defects from the broader `App`/`Mux` size critique.

**Rule.** Land observable correctness and ownership repairs before structural package splits. Decomposition must preserve existing transactions, tests, performance baselines, and default-off guarantees rather than mixing behavior changes with moves.

### M3 — Do not DRY away semantic boundaries

**Origin:** C06 was weakened because persistence DTOs, resolved restore plans, and runtime topology have similar shapes but different trust and lifetime contracts.

**Rule.** Duplicate representation is acceptable where it protects on-disk, trust, or ownership boundaries. Consolidate catalogs and translations only when one concept truly has one lifecycle; retain explicit adapters between semantically distinct models.

### M4 — Closed vocabularies need one authority

**Origin:** C02 and C29 survived both rounds as change-amplification debt.

**Rule.** Configuration leaves, actions, event variants, and protocol outcomes should have one declarative authority that derives validation, codecs, diagnostics, discovery, and tests. Generated or table-driven projection is preferred to parallel hand-maintained switches.

---

## Maturity scorecard

| Dimension | Reconciled score | Evidence-based interpretation |
|---|---:|---|
| Architecture maturity | 7.4/10 | Strong explicit ownership narratives, rollback paths, bounds, and platform seams; controller concentration remains. |
| Clean Code | 6.0/10 | Package/file decomposition is extensive, but `App`, `Mux`, config catalogs, reload/draw flows, and fontglyph retain high change concentration. |
| GRASP | 6.4/10 | Information Expert, Creator, Indirection, and Protected Variations are strong; Controller, Low Coupling, and High Cohesion are uneven. |
| Dependency graph | 8.1/10 | 37 production packages, 68 internal edges, zero import SCCs, 36/37 root-reachable; several near-cycles and high-instability hubs. |
| Domain isolation | 7.0/10 | Core/protocol/image leaves are mostly disciplined; quick-select/mux, persistence launch intent, events, and concrete DTOs leak across boundaries. |
| Ownership/transactions | 7.5/10 | Prepare/commit/rollback and resource budgets are unusually mature; owner-thread enforcement and a few failure paths need hardening. |
| Test/guardrail maturity | 8.8/10 | Import guards, invariants, race/fuzz/property tests, maturity gates, and Phase 15 qualification materially reduce risk. |
| **Overall** | **7.3/10** | Mature beta architecture suitable for incremental hardening; not yet a low-change-amplification 1.0 structure. |

Team A’s constructive scores clustered around 7.0–8.1. Team B deliberately scored Clean Code/GRASP at 4.3–5.9. Round 2 reconciled 40 claims: 30 accepted findings, 11 deferred boundaries/debts, and one falsified/withdrawn claim.

---

## Layer 0 — Process entry and composition

### L0-01 — Duplicate tagged bootstrap policy

**Evidence:** `cmd/cervterm/main.go:16-79`; `cmd/cervterm/main_glfw.go:22-136`.

Headless and GLFW mains repeat common flags and diagnostic routing, but diverge legitimately after composition. Extract only the common bootstrap/early-command model; preserve build-specific startup.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** internal
- **Class:** polish
- **Status:** **deferred** — keep behind correctness and ownership phases
- **Depends on:** none
- **Cross-cut tag:** `T1-controller-concentration`

### Layer 0 — tally

| Status | Count |
|---|---:|
| accepted | 0 |
| deferred | 1 |
| withdrawn | 0 |

---

## Layer 1 — GLFW/native frontend and UI projection

### L1-01 — `App` is an overextended controller

**Evidence:** `internal/frontend/glfwgl/app.go:23-159`; `action_executor.go:17`; `app_draw.go:61`; `reload.go:95`.

`App` owns the correct frontend projection boundary but also centralizes action policy, render scratch, reload, scripting, accessibility, IME, native windows, and mux event interpretation across hundreds of methods. Extract cohesive controllers after behavioral repairs, retaining `App` as a thin composition facade.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — decompose by stable ownership concern
- **Depends on:** L1-02, L1-03, L1-04, L1-05, L1-06
- **Cross-cut tag:** `T1-controller-concentration`

### L1-02 — Runtime configuration can diverge across windows

**Evidence:** `internal/frontend/glfwgl/window_controller_runtime.go:137-160`; `reload.go:429-437`; `app_host.go:22-25`.

Projection-local setters can commit without advancing the generation used to synchronize siblings. Introduce one process-owned shared-config transaction/revision and fan out detached effective state atomically to every projection.

- **Severity:** Medium
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — repair shared multi-window publication
- **Depends on:** L3-02
- **Cross-cut tag:** `T3-multi-window-consistency`

### L1-03 — Accessibility word units return rows

**Evidence:** `internal/accessibility/range.go:281-295`.

`TextUnitWord`, format, and line use the same row segments. Implement bounded word segmentation over the accessibility document and add UIA range tests for punctuation, Unicode, soft wraps, and empty rows.

- **Severity:** Medium
- **Effort:** M
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — restore truthful word semantics
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L1-04 — UIA focusability equals current focus

**Evidence:** `internal/frontend/glfwgl/windows_uia_provider.go:307-313`.

`IsKeyboardFocusable` and `HasKeyboardFocus` use the same predicate. Separate capability from current state and test non-focused but focusable terminal/input nodes.

- **Severity:** Medium
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — fix provider semantics
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L1-05 — Accessibility geometry mixes coordinate spaces

**Evidence:** `projection_accessibility_factory_windows.go:34-43`; `candidate_geometry.go:38`; `candidate_geometry_glfw.go:39-40`.

The projection combines window position with framebuffer dimensions without the conversion used elsewhere. Centralize framebuffer→window→screen conversion and qualify mixed-DPI/multi-monitor rectangles.

- **Severity:** Medium
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — use one checked coordinate projection
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L1-06 — Drawing, input, and reload rely on temporal protocols

**Evidence:** `app_draw.go:99-176`; `app_callbacks.go:33-154`; `reload.go:95-220,252-418`.

Centralized ordering is intentional, but pane scratch swapping, hard-coded input precedence, and long staged reload scripts make future extensions fragile. Introduce explicit pane render context, composable input routes, and typed reload states after current behavior is pinned.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — split temporal protocols without changing precedence
- **Depends on:** L1-02, L3-02
- **Cross-cut tag:** `T1-controller-concentration`

### L1-07 — Selection extraction trusts rectangular storage

**Evidence:** `internal/selection/selection.go:35-56,99`.

Dimensions are validated but `len(cells) >= rows*cols` is not. Return a bounded error/empty result for malformed input and fuzz the public helper.

- **Severity:** Low
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — make malformed snapshots fail soft
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L1-08 — Persisted monitor identity is order-dependent

**Evidence:** `startup_restore_glfw.go:66-72`; `layout_persistence_export_glfw.go:42`.

The synthesized ID is order-dependent, but current persistence does not write `MonitorHint`; the proposed failure path is unreachable.

- **Severity:** None
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **withdrawn** — falsified by both Round-2 teams
- **Depends on:** none

### Layer 1 — tally

| Status | Count |
|---|---:|
| accepted | 7 |
| deferred | 0 |
| withdrawn | 1 |

---

## Layer 2 — Application API, configuration, scripting, actions, policies

### L2-01 — Configuration uses parallel manual catalogs

**Evidence:** `internal/config/config.go:18-200`; `document.go:88-182`; `lua.go:64-182`; `diff.go:40-202`; `runtime_scope.go:252-470`.

Tests constrain drift, but each leaf still requires coordinated edits across defaults, schema, decode, validation, diff, runtime scope, template, and diagnostics. Expand schema metadata into the single declarative authority and derive repetitive projections.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — consolidate without weakening strict v1/v2 behavior
- **Depends on:** L2-02, L2-03, L2-06
- **Cross-cut tag:** `T2-single-authority`

### L2-02 — Lua scrollbar API omits authoritative v2 fields

**Evidence:** `internal/config/config.go:97-102`; `internal/script/api.go:277-314`.

Expose and decode `mode`, `stable_gutter`, and `animation_fps` consistently with strict v2/runtime semantics; retain legacy `enabled` compatibility.

- **Severity:** Medium
- **Effort:** S
- **Blast radius:** public-API
- **Class:** polish
- **Status:** **accepted** — close scripting API parity
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L2-03 — Quick-select derived state can go stale

**Evidence:** `internal/config/config.go:158-162,303-309`; `document_apply.go:82-84`; `frontend/glfwgl/quick_select.go:30`.

Authored rules and exported compiled rules are independently mutable. Make compiled rules private/derived during validation or expose an immutable prepared value.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — enforce derived-state invariant
- **Depends on:** none
- **Cross-cut tag:** `T2-single-authority`

### L2-04 — Script timers and status lack aggregate bounds

**Evidence:** `internal/script/timers.go:31-34,93-107`; `status.go:10-39`.

Add count, minimum-period, ID, and returned-text budgets with deterministic rejection/coalescing so trusted local scripts cannot monopolize the owner loop.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — bound resource ownership
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L2-05 — Action extension remains change-amplifying

**Evidence:** `internal/action/action.go`; `registry.go`; `internal/script/actions.go`; `frontend/glfwgl/action_executor.go`; `command_palette.go`.

The registry centralizes metadata/codecs, but Lua exposure, execution, and discovery defaults remain parallel switches. Make one descriptor/handler registration authority while preserving typed serializable actions.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — reduce closed-vocabulary shotgun surgery
- **Depends on:** none
- **Cross-cut tag:** `T2-single-authority`

### L2-06 — Config decoding and diagnostics are not uniformly strict/deterministic

**Evidence:** `internal/config/config.go:439-466`; `lua.go:64-182`.

Sort map-backed validation errors and stop discarding quick-select preparation/type errors at reusable decoding seams. Keep legacy compatibility explicit rather than silently partial.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** public-API
- **Class:** polish
- **Status:** **accepted** — make reusable decoder contracts honest
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L2-07 — Teal cleanup lacks live publisher ownership

**Evidence:** `internal/config/teal_publish.go:275-300`.

Age/name-based cleanup can remove another process’s still-live temp. Add ownership metadata or avoid cross-process cleanup; retain atomic publication and stale-file recovery.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** on-disk
- **Class:** redesign
- **Status:** **accepted** — harden publication ownership
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L2-08 — Config→quickselect→mux near-cycle

**Evidence:** `config/config.go:151-160`; `quickselect/engine.go:11,96`.

The coupling is real but currently acyclic and internal. Defer an independent quick-select snapshot/value package until a concrete mux/config cycle or API extraction warrants it.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — deliberate low-risk coupling
- **Depends on:** none
- **Cross-cut tag:** `T6-domain-leakage`

### L2-09 — Candidate activation is a documented borrowed-runtime contract

**Evidence:** `internal/script/candidate_bundle.go:86-113,243-256`.

Multiple handles can borrow the same runtime, but the bundle lifetime/exclusive-owner contract is explicit and activation is idempotent per handle. Revisit only with linear ownership tooling or evidence of misuse.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — protected by current transaction tests
- **Depends on:** L3-02

### L2-10 — Legacy and v2 loaders have different side-effect contracts

**Evidence:** `config/load.go:12-31`; `script/runtime.go:39-48`; `script/versioned_load.go:120-145`.

The difference is compatibility policy, not hidden production drift. Document it at public seams and defer unification until legacy v1 retirement is approved.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** on-disk
- **Class:** redesign
- **Status:** **deferred** — compatibility boundary
- **Depends on:** none

### L2-11 — Script host/overlay boundary is broad but bounded

**Evidence:** `script/api.go:13-32,243-255`; `script/overlays.go`; `frontend/glfwgl/app_overlay.go`.

Actual painting stays frontend-owned and optional capabilities fail closed. Defer interface segregation until a second host implementation creates pressure.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — no current behavioral defect
- **Depends on:** none

### Layer 2 — tally

| Status | Count |
|---|---:|
| accepted | 7 |
| deferred | 4 |
| withdrawn | 0 |

---

## Layer 3 — Runtime orchestration, mux, workspace and persistence

### L3-01 — `Mux` is an overextended controller

**Evidence:** `internal/mux/mux.go:49-69,376-440`; protocol scheduler and restore files.

Mux correctly owns aggregate identity/lifecycle, but directly coordinates transport, parsing, render snapshots, persistence, three protocols, clocks, and events. Extract session ingress, protocol scheduling, and restore coordinators behind mux-owned ports.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — retain mux as topology/lifecycle facade
- **Depends on:** L3-02, L3-03, L3-04, L3-08, L3-09, L3-10
- **Cross-cut tag:** `T1-controller-concentration`

### L3-02 — Owner-thread use is not executable

**Evidence:** `internal/mux/mux.go:49-69`; `session_registry.go:14-32`; `termimage/store.go:72-110,187-195`.

Locks protect membership and termimage capabilities protect mutation, but public aggregate operations do not detect wrong-thread calls. Introduce an owner capability/assertion or serialized command seam and race-test every mutation family.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — promoted by developer triage
- **Depends on:** none
- **Cross-cut tag:** `T4-ownership-transactions`

### L3-03 — One mux bounds value governs multiple windows

**Evidence:** `internal/mux/mux.go:64`; `resize.go:53-59`; `frontend/glfwgl/app_loop.go:362-377`.

Every projection overwrites global bounds while windows have independent dimensions. Store geometry per mux window/tab projection and address layout/resize by stable window ID.

- **Severity:** High
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — highest-priority architectural correctness defect
- **Depends on:** L3-02
- **Cross-cut tag:** `T3-multi-window-consistency`

### L3-04 — Bootstrap publishes before fallible reader start

**Evidence:** `internal/mux/mux.go:138-169`; `session_registry.go:124-160`.

The failure is rare but leaves published bootstrap fields after a failed start. Prepare all state and reader ownership first, then commit atomically or restore exact zero state.

- **Severity:** Low
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — promoted with ownership risks
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L3-05 — Topology mutations omit revisions

**Evidence:** `internal/mux/topology.go:156-175`; `tree.go:30-47`.

Increment affected tab/window revisions for topology and ratio commits and test stale confirmations/persistence/activity consumers across every structural mutation.

- **Severity:** Medium
- **Effort:** S
- **Blast radius:** cross-module
- **Class:** polish
- **Status:** **accepted** — repair revision contract
- **Depends on:** none
- **Cross-cut tag:** `T3-multi-window-consistency`

### L3-06 — Global pane policies update only the active tab

**Evidence:** `internal/mux/viewport.go:30-60`; `model.go:188-192`.

Iterate all registry/model panes or explicitly scope and rename the APIs. Existing inactive panes must converge to current scrollback/cursor policy.

- **Severity:** Medium
- **Effort:** S
- **Blast radius:** cross-module
- **Class:** polish
- **Status:** **accepted** — fix policy reach
- **Depends on:** none
- **Cross-cut tag:** `T3-multi-window-consistency`

### L3-07 — Terminal-controlled CWD becomes persisted launch intent

**Evidence:** `internal/vt/parser_osc.go:59-62`; `internal/mux/fresh_session.go:110-115`; `layoutstate/validation.go`.

OSC-derived state crosses a trust/lifetime boundary into future process launch. Persist configured launch intent by default; require an explicit sanitized/consented policy before adopting terminal CWD.

- **Severity:** Medium
- **Effort:** M
- **Blast radius:** on-disk
- **Class:** redesign
- **Status:** **accepted** — close domain/trust leak
- **Depends on:** none
- **Cross-cut tag:** `T6-domain-leakage`

### L3-08 — Event union permits invalid payload/address combinations

**Evidence:** `internal/mux/events.go:46-64`; `event_address.go:3-20`; `mux_tabs.go:115`.

Replace generic optional fields with typed event variants or enforce a per-kind validator at every producer boundary. Preserve stable addressing at event creation rather than reconstructing from mutable topology.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — strengthen event vocabulary
- **Depends on:** L3-02
- **Cross-cut tag:** `T6-domain-leakage`

### L3-09 — Protocol scheduler erases types and splits clock ownership

**Evidence:** `kitty_decode_scheduler.go:30-46,273-315`; `sixel/adapter.go:25-27`; `itermimage/adapter.go:22-24`; `termimage/store.go`.

Use typed protocol results and one injected clock propagated through adapter/store/deadline evaluation; consolidate shared completion/error policy without conflating protocol grammar.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — promoted by ownership triage
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L3-10 — Close/store serialization has partial ownership gaps

**Evidence:** `mux_tabs.go:119-147`; `model_tabs.go:145-161`; `layoutstate/store.go:65-92`.

Existing prevalidation narrows the window, but unexpected detach failure and independent same-path stores are not serialized as one transaction. Add fault-injected exact rollback and per-path/process coordination or explicit last-writer policy.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** on-disk
- **Class:** redesign
- **Status:** **accepted** — promoted by ownership triage
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L3-11 — Shutdown leaves model-only operations callable

**Evidence:** `mux/mux_shutdown.go:3-11`; `mux.go:307-312`.

This is a low-risk lifecycle smell under current teardown ownership. Defer a terminal `closed` state until public reuse or post-shutdown callers appear.

- **Severity:** Low
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **deferred** — no demonstrated production caller
- **Depends on:** L3-02

### L3-12 — Persistence/runtime DTO duplication protects boundaries

**Evidence:** `layoutstate/model.go`; `layoutrestore/restore.go`; `mux/fresh_session.go`; `layout_persistence_export_glfw.go`.

The similar graphs encode persisted, normalized, and runtime lifecycles. Keep explicit translation; only extract leaf value objects proven semantically identical. Mux exposure of PTY/render values is likewise internal and detached.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — deliberate anti-coupling boundary
- **Depends on:** none
- **Cross-cut tag:** `T6-domain-leakage`

### Layer 3 — tally

| Status | Count |
|---|---:|
| accepted | 10 |
| deferred | 2 |
| withdrawn | 0 |

---

## Layer 4 — Terminal/platform adapters and projections

### L4-01 — VT concrete binding and redaction FSM amplify security-sensitive change

**Evidence:** `internal/vt/parser.go:64-78`; `parser_public_output.go:11-36`.

Introduce a narrow terminal command sink and derive public-output projection from parser framing events instead of shadow state. Preserve byte-for-byte disabled/unselected behavior and existing fuzz oracles.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — reduce synchronized security state machines
- **Depends on:** none
- **Cross-cut tag:** `T6-domain-leakage`

### L4-02 — `fontglyph` contains multiple architectural subsystems

**Evidence:** `fontindex.go:61-119`; `backend.go:65-101`; `directwrite_bridge_windows.go`; `color_colr_render.go`.

Split discovery/index, parsed-face cache, resolution/policy, shaping, rasterization/color paint, and platform adapters behind stable `fontdesc` identities. Preserve cache budgets and performance baselines.

- **Severity:** Medium
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — reduce package-level coupling
- **Depends on:** none
- **Cross-cut tag:** `T1-controller-concentration`

### L4-03 — PTY lifecycle omits process completion semantics

**Evidence:** `internal/pty/session.go:16-21`; `session_unix.go:49-53`.

Add a single-consumer wait/exit result contract and deterministic close-vs-natural-exit semantics; retain platform-specific idempotent cleanup.

- **Severity:** Medium
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **accepted** — make lifecycle observable
- **Depends on:** L3-02
- **Cross-cut tag:** `T4-ownership-transactions`

### L4-04 — Theme package is disconnected

**Evidence:** `internal/theme/palette.go`; production graph has no inbound import.

Delete it if superseded by core/config palette resolution, or adopt it as the single palette abstraction. Do not maintain a test-only production package.

- **Severity:** Low
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — decide and remove dead ambiguity
- **Depends on:** none
- **Cross-cut tag:** `T2-single-authority`

### L4-05 — Ctrl-V policy leaks into low-level encoding

**Evidence:** `internal/input/encoder.go:140-149`; `frontend/glfwgl/action_bindings.go:83-86`.

The encoder suppresses Ctrl-V independently of frontend paste bindings. Move paste policy entirely to input routing so rebinding can intentionally emit `0x16`.

- **Severity:** Medium
- **Effort:** S
- **Blast radius:** cross-module
- **Class:** polish
- **Status:** **accepted** — restore layer ownership
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L4-06 — Background budget charges survive failed pin preparation

**Evidence:** `internal/background/budget.go:45-66`.

Rollback encoded-byte reservation when decoded reservation fails, even if current candidate budgets are usually discarded. Add reusable-budget fault tests.

- **Severity:** Low
- **Effort:** S
- **Blast radius:** internal
- **Class:** polish
- **Status:** **accepted** — make accounting transactional
- **Depends on:** none
- **Cross-cut tag:** `T5-boundary-correctness`

### L4-07 — Unicode clustering and BiDi are explicit subsets

**Evidence:** `unicodecluster/cluster.go:122-145`; `render/bidi.go:60-66`.

The boundary is documented and BiDi is experimental. Defer full UAX #29/render-cluster semantics until product support scope changes; retain conformance gaps as explicit tests/docs.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — deliberate bounded scope
- **Depends on:** none

### Layer 4 — tally

| Status | Count |
|---|---:|
| accepted | 6 |
| deferred | 1 |
| withdrawn | 0 |

---

## Layer 5 — Central domain model and primitives

### L5-01 — `Terminal` is a large but cohesive aggregate

**Evidence:** `internal/core/types.go:125-174`; image lifecycle/reflow files.

Metadata and image placement must move with terminal mutations, and no external infrastructure leaks in. Defer splitting until a concrete invariant can be moved without cross-aggregate transactions.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — Information Expert currently outweighs size
- **Depends on:** none

### L5-02 — Combining accessor exposes internal backing by contract

**Evidence:** `internal/core/types.go:38-72`; `render/detach.go:10-19`.

Public detached snapshots deep-clone and core mutations are copy-on-write; internal capture consumers are told not to mutate. Defer an allocation/copy trade-off change unless a mutating consumer appears.

- **Severity:** Low
- **Effort:** M
- **Blast radius:** cross-module
- **Class:** polish
- **Status:** **deferred** — Round-2 teams disagreed; no production mutation found
- **Depends on:** none

### L5-03 — Image command unions and fail-closed store shutdown are deliberate

**Evidence:** `core/images_api.go:15-20`; `termimage/types.go:82-90`; `core/images_lifecycle.go:29-44`.

Typed operations would improve invalid-state ergonomics, but current validation and fail-closed cleanup protect text correctness. Defer until protocol work resumes.

- **Severity:** Low
- **Effort:** L
- **Blast radius:** cross-module
- **Class:** redesign
- **Status:** **deferred** — deliberate containment policy
- **Depends on:** none

### Layer 5 — tally

| Status | Count |
|---|---:|
| accepted | 0 |
| deferred | 3 |
| withdrawn | 0 |

---

## Layer 6 — Cross-cutting utilities

No material finding survived Round 2. `applog`, `buildinfo`, `metrics`, and `workscheduler` remain small leaves; scheduler rejection/shutdown cleanup and logging redaction resisted adversarial review.

### Layer 6 — tally

| Status | Count |
|---|---:|
| accepted | 0 |
| deferred | 0 |
| withdrawn | 0 |

---

## Cross-cutting themes

### T1 — Controller concentration (active)

**Findings:** L0-01, L1-01, L1-06, L3-01, L4-02.

Frontend, mux, and font execution remain dependency hubs. The plan repairs behavior/ownership first, then decomposes by stable lifecycle rather than file size.

### T2 — Single declarative authority (active)

**Findings:** L2-01, L2-03, L2-05, L4-04.

Closed vocabularies and derived state are safe but change-amplifying. One authority should derive repetitive projection while preserving explicit compatibility boundaries.

### T3 — Multi-window consistency (active)

**Findings:** L1-02, L3-03, L3-05, L3-06.

The local multi-window model is structurally present, but shared config, geometry, revision, and policy reach are not uniformly window/tab addressed.

### T4 — Executable ownership and transactions (active)

**Findings:** L2-04, L2-07, L3-02, L3-04, L3-09, L3-10, L4-03.

Strong prepare/commit/rollback conventions need enforceable owner identity, one clock, exact failure rollback, and bounded script/process lifecycles.

### T5 — Boundary correctness (active)

**Findings:** L1-03, L1-04, L1-05, L1-07, L2-02, L2-06, L4-05, L4-06.

Small API/semantic mismatches at accessibility, scripting, config, selection, input, and budget seams create disproportionate user-visible risk and should land first.

### T6 — Domain and trust leakage (active)

**Findings:** L2-08, L3-07, L3-08, L3-12, L4-01.

Most package boundaries are sound, but terminal-controlled launch intent, generic events, concrete parser targets, and quick-select DTO ownership blur trust or domain direction.

---

## Final assessment

### Verdict

**Overall architectural maturity: 7.3/10 — mature beta, structurally sound, not yet low-change-amplification 1.0 architecture.**

CervTerm’s strongest attributes are its acyclic package graph, explicit resource budgets, prepare/commit/rollback discipline, immutable/detached projections, import guards, default-off native/protocol features, and unusually deep race/fuzz/invariant qualification. Its main maturity ceiling is not missing architecture but concentrated orchestration: `App`, `Mux`, config catalogs, VT redaction, and `fontglyph` make correct changes expensive and increase synchronized-edit risk.

### Score summary

| Dimension | Score |
|---|---:|
| Architecture maturity | 7.4/10 |
| Clean Code | 6.0/10 |
| GRASP responsibility assignment | 6.4/10 |
| Dependency graph hygiene | 8.1/10 |
| Domain isolation | 7.0/10 |
| Ownership and transactions | 7.5/10 |
| Test and guardrail maturity | 8.8/10 |
| **Overall** | **7.3/10** |

### Finding distribution

| Disposition | Count |
|---|---:|
| Accepted | 30 |
| Deferred deliberate boundaries/debts | 11 |
| Withdrawn after falsification | 1 |

| Accepted severity | Count |
|---|---:|
| High | 1 |
| Medium | 17 |
| Low | 12 |

### Highest-priority risks

1. **L3-03 — global mux geometry across multiple native windows (High):** independent projections overwrite one shared bounds value.
2. **L1-02/L3-02 — shared configuration and owner-thread enforcement (Medium):** cross-window state can diverge and ownership is partly documentary.
3. **L1-03 through L1-05 — accessibility semantic/coordinate correctness (Medium):** word units, focusability, and DPI geometry are inaccurate.
4. **L3-05 through L3-07 — revision, policy reach, and persisted CWD trust (Medium):** multi-tab consistency and launch intent cross boundaries incorrectly.
5. **L1-01/L2-01/L3-01/L4-02 — controller/catalog concentration (Medium):** the system is well guarded but change-amplifying.

### What resisted the adversarial team

- Zero production import cycles across 37 packages and 68 internal edges.
- `termimage`, accessibility, policy packages, `fontdesc`, and utility leaves retain strong dependency discipline.
- Persistence, config publication, restore, image activation, and native projection paths use explicit provisional ownership and rollback.
- Bounded hostile-input handling, redacted diagnostics, stale-generation checks, and default-off experimental behavior are enforced by tests and scripts.
- Several apparent leaks were correctly weakened as deliberate boundary adapters; monitor persistence claim C22 was withdrawn as unreachable.

### Developer triage

The developer accepted the two-team consensus and explicitly promoted ownership-related claims C07, C11, C16, and C38. The developer requested a scored report only; no implementation roadmap or phased plan is included.

### Final tally

| Layer | Findings | Accepted | Deferred | Withdrawn |
|---|---:|---:|---:|---:|
| L0 | 1 | 0 | 1 | 0 |
| L1 | 8 | 7 | 0 | 1 |
| L2 | 11 | 7 | 4 | 0 |
| L3 | 12 | 10 | 2 | 0 |
| L4 | 7 | 6 | 1 | 0 |
| L5 | 3 | 0 | 3 | 0 |
| L6 | 0 | 0 | 0 | 0 |
| **Total** | **42** | **30** | **11** | **1** |
