# Implementation Plan — WezTerm-Inspired Parity

## Objective
Close the requested CervTerm parity gaps through small, reversible epics. The functional baseline for this roadmap is `origin/main` at `7d64cc9`.

## Non-goals
- Renderer/backend selection or backend migration.
- Local, SSH, WSL, serial, or remote domains.
- Daemon, persistence of live processes, detach/reattach, tmux compatibility.
- Plugin marketplace or unrestricted external code.

Named workspaces remain local and in-process. Persistence stores layout/config only; it never stores running processes or credentials.

## Required ADR Gates
- [ADR-0002](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0002-version-config-composition-and-provenance.md): config composition, precedence, provenance, and schema migration.
- [ADR-0003](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0003-use-a-typed-action-model.md): typed actions, targeting, serialization, and modal precedence.
- [ADR-0004](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0004-own-tabs-windows-and-workspaces-in-process.md): ownership of tabs, native windows, and local workspaces.
- [ADR-0005](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0005-introduce-a-native-host-seam-for-ime-and-accessibility.md): native host seam for IME and accessibility.
- [ADR-0006](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0006-bound-terminal-image-lifetime-and-resources.md): image transfer, storage, placement, lifetime, and resource budgets.
- [ADR-0007](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0007-persist-workspace-layouts-not-live-processes.md): layout-only workspace persistence and migrations.
- [ADR-0008](../.architecture-ai-project-advisor/.asi/projects/cervterm/decisions/0008-gate-terminal-originated-external-side-effects.md): trust policy for links, notifications, shell metadata, and external side effects.

## Delivery Checkpoints
1. **Visual parity:** Phases 0–5.
2. **Discoverable local UX:** Phases 6–9.
3. **Shell/native integration:** Phases 10–12.
4. **Terminal graphics:** Phases 13–14.
5. **Release hardening:** Phase 15.

## Dependency Summary
- Phase 1 blocks all configurable actions and discoverable UI.
- Phase 2 blocks all new public configuration.
- Phases 3 and 4 can develop internals in parallel, but config integration must merge serially.
- Phase 8 blocks Phase 9.
- Phase 11 establishes the host seam used by Phase 12.
- Phase 13 blocks Phase 14.
- Phase 10 design may overlap tab work after ADR-0008 is accepted.

---

## Phase 0 — Baseline and Contracts
**Scope:** Refresh architecture/spec/roadmap from current main; record support and performance baselines.

**Work**
- Correct stale docs about mux, pane zoom, opacity, blur, scrollbar, scripting, reload, fonts, OSC 7/52, and links.
- Add a machine-readable feature matrix and deprecation policy.
- Capture startup time/memory, atlas memory, idle wakeups, parser throughput, and installed-package smoke results.
- Create Proposed ADR-0002 through ADR-0008 with decision questions and acceptance evidence.

**Success:** current capabilities and non-goals are explicit; later phases reference ADRs and measurable baselines.

**Verify:** `go test ./...`, GLFW-tagged tests, maturity gates, current Windows package smoke.

**Rollback:** documentation/artifact-only.

## Phase 1 — Typed Action Engine
**Scope:** One toolkit-neutral action foundation for current keyboard/Lua commands and future mouse, palette, quick-select, launch-menu, pane, tab, window, and workspace consumers.

**Status:** Complete. The typed model, frontend executor, Lua/Teal contract, and final input-pipeline cleanup landed in PRs [#123](https://github.com/cervantesh/CervTerm/pull/123) through [#126](https://github.com/cervantesh/CervTerm/pull/126). See [Phase 1 validation evidence](validation/phase-1-typed-action-engine.md).

**Work**
- Accept ADR-0003.
- Add `internal/action` with typed arguments, metadata/labels, target resolution, context, validation, and serialization.
- Port current copy/paste/search/scroll/zoom/reload/split/focus/close actions first; pane resize/swap/move actions remain Phase 6 scope.
- Retain Lua callbacks through an explicit bounded callback action and watchdog.
- Define precedence: active modal > safety/reload > user binding > built-in > PTY encoding.
- Add action sequences and explicit stop-on-error behavior.

**Success:** all existing shortcuts behave identically; built-ins no longer depend on GLFW key values; registry enumeration can feed UI.

**Tests:** table-driven validation/dispatch/target tests; exact key/mouse bytes; callback timeout; modal input leakage; race tests.

**Rollback:** temporary adapter may invoke old handlers until parity passes.

## Phase 2 — Versioned Composed Configuration
**Scope:** schema versions, includes/modules, profiles/environment/CLI/window overrides, provenance, migrations, dependency-aware reload.

**Status:** In progress. Slices 1–6 provide versioned/composed candidate configuration, selection/CLI layers, provenance, and transactional Teal publication. Slice 7 provides the candidate bundle ownership unit for validated Config, Lua runtime surfaces, selection/provenance, dependency graph/staging, and deferred publication with deterministic cleanup. Public activation remains gated on splitting frontend live-resource preparation from infallible bundle transfer; runtime scopes, graph watching, and desired/effective application remain later slices.

**Work**
- Apply accepted ADR-0002.
- Establish precedence: defaults < includes < primary < selected environment < selected profile < CLI < per-window runtime override.
- Add canonical-path source graph, include-cycle detection, deterministic deep-merge rules, and field provenance.
- Add migration functions and golden fixtures; preserve v1 shorthand.
- Watch the primary file and dependencies; build/validate a complete candidate before atomic swap.
- Compute live-safe versus restart-required changes.
- Add `--explain-config`/doctor diagnostics.
- Update Go schema, Lua loader, Teal types, template, validation, docs, and tests together.

**Success:** existing config remains valid or migrates clearly; failed reload preserves prior runtime/bindings; cycles/path errors identify source locations; provenance explains every winner.

**Tests:** precedence matrix, cycles, canonicalization, unknown fields, migration goldens, Teal output location, debounce/concurrent writes, reload rollback.

**Rollback:** keep composition behind schema v2 until evidence is complete.

## Phase 3 — Complete Themes and Palettes
**Scope:** named schemes, ANSI 16, indexed overrides, semantic chrome colors.

**Work**
- Introduce an application/theme palette; retain logical default/indexed/truecolor attributes in terminal core.
- Add foreground/background/cursor/selection, ANSI 16, indexed overrides, chrome/accent tokens.
- Resolve palette only during projection/rendering.
- Support local named schemes and atomic live reload.
- Route status/search/scrollbar/divider/tab colors through semantic tokens.

**Success:** the user's Shades of Purple palette is reproduced exactly; ANSI/reset/inverse/dim/truecolor remain correct; failed reload retains old colors.

**Tests:** palette goldens, SGR reset/inverse/dim, config/Teal/template/reload, screenshots/manual checklist.

**Rollback:** optional fields default to the current palette.

## Phase 4 — Font Descriptors, Fallback, and Metrics
**Scope:** fallback stack, face rules, weights/styles/stretch, line height, offsets, feature rules.

**Work**
- Preserve `font.family` shorthand while adding ordered descriptors.
- Add regular/bold/italic/bold-italic face selection, weight/style/stretch, and deterministic ranking.
- Prevent Regular from resolving to ExtraLight when a regular face exists.
- Add lazy ordered fallback and optional Unicode-range/symbol rules.
- Add OpenType features, line-height/cell-width, baseline/glyph offsets while preserving the fixed grid.
- Expand atlas/shaping cache keys and DPI/pane-zoom rebuild logic.
- Diagnose chosen faces and fallbacks; verify installed package paths.

**Success:** JetBrainsMono Nerd Font selects the intended faces; real styles replace synthetic ones when available; zoom/DPI/fallback remain cache-safe; sibling pane zoom is unaffected.

**Tests:** font matching fixtures, metric/centering/baseline tests, ligature cursor splits, fallback memory, multi-size atlas reuse, installed-package smoke.

**Rollback:** legacy shorthand maps to one descriptor with current metrics.

## Phase 5 — Appearance and Window Controls
**Scope:** per-side padding, separate text/background opacity, backgrounds/gradients, decorations, FPS policy, scrollbar visibility.

**Work**
- Add left/right/top/bottom padding and correct hit-testing/grid calculations.
- Separate text and background opacity while preserving existing translucency/blur invariants.
- Add solid/gradient/image background layers with scale/fit/alignment and bounded decode/cache.
- Add supported decoration/titlebar/initial-size options with platform capability diagnostics.
- Add animation/max-FPS and idle/on-demand policy; do not add renderer selection.
- Expand scrollbar modes: always, hover, scrolling, never, with stable gutter option.

**Success:** current config remains visually identical by default; live-safe fields reload; recreation-required fields are diagnosed; idle mode keeps zero-frame behavior.

**Tests:** layout/hit-testing, opacity combinations, image limits, FPS timing, scrollbar policies, platform capability tests.

**Rollback:** each feature is independently optional.

## Phase 6 — Advanced Keyboard and Mouse Bindings
**Scope:** WezTerm-style ergonomic actions without copying domain actions.

**Work**
- Add key tables/modes, leader and bounded multi-chord sequences.
- Add typed mouse bindings for press/release/drag/wheel/click-count and modifier matching.
- Add copy-on-select, right-click paste, platform primary-selection capability, open/copy link, pane resize/swap/move, and tab/window actions.
- Define terminal mouse-reporting versus UI-action precedence and override modifiers.
- Add clipboard image actions only where the OS adapter supports them safely.

**Success:** no duplicate input; mouse release identity/modifiers survive normalization; modal and terminal mouse modes have deterministic precedence.

**Tests:** byte-level key/mouse cases, chord timeouts, drag/click counts, PTY-vs-UI routing, clipboard capability failures.

**Rollback:** existing bindings remain default; advanced tables opt in.

## Phase 7 — Command Palette, Quick Select, and Launch Menu
**Scope:** discoverable retained-mode UX backed by the action registry.

**Work**
- Extract reusable modal controller/list/filter/chrome primitives from search.
- Command palette enumerates labeled permitted actions and bindings.
- Quick select overlays labels for hyperlinks and configured regex matches.
- Launch menu runs explicitly configured local commands/cwd/env only.
- Define switcher extension points; pane/tab/window switchers land with Phases 8–9 after those models exist.

**Success:** modal UI captures all input before PTY; focus restores correctly; overlays repaint only when damaged; unsafe/unavailable actions are not offered.

**Tests:** rune-safe filtering, label assignment, no PTY leakage, focus restoration, zero-frame idle, launch argument/environment quoting.

**Rollback:** each modal has a separate feature flag/binding.

## Phase 8 — Visible Tabs and Tab Bar
**Scope:** promote the implicit tab into a mux-owned tab collection.

**Work**
- Accept the tab-ownership portion of ADR-0004.
- Add stable TabID, ordered tabs, active tab, tab lifecycle, rename/reorder, pane move between tabs.
- Preserve split-tree and independent pane state/lifecycle.
- Add tab bar model/layout/hit-testing/rendering, configurable position/visibility/style and overflow.
- Add typed create/focus/relative-focus/move/rename/close/move-pane actions and close confirmation policy.
- Preserve a one-tab hidden-bar compatibility path.

**Success:** one-pane/one-tab behavior is unchanged; closing/moving panes never leaks PTYs; inactive tabs do not receive input or unnecessary rendering.

**Tests:** pure mux invariants, tab lifecycle/focus fallback, pane transfer, hit-testing/overflow, close ordering, race/leak tests.

**Rollback:** hidden one-tab mode can remain the default for one release.

## Phase 9 — Multiple Windows and Local Workspaces
**Scope:** multiple native windows in one process, named local workspaces, layout-only persistence.

**Work**
- Complete ADR-0004 and accept ADR-0007.
- Add process-level window controller; each native window projects one mux window.
- Support new/close/focus window and moving tabs/panes between windows.
- Add named workspace membership/switching/renaming.
- Persist versioned window bounds, tab order, split ratios, cwd/launch descriptors, and appearance overrides only.
- Restore by spawning new local sessions; never serialize processes, PTY handles, credentials, or scrollback by default.
- Keep native/OpenGL calls on the OS thread.

**Success:** windows have independent focus/chrome; moves preserve pane identity/session ownership; corrupt/old layouts fail safely; restore never claims live-session continuity.

**Tests:** model invariants, fake host/session lifecycle, persistence migrations/corruption, monitor/DPI bounds recovery, OS-thread assertions.

**Rollback:** disable restore and fall back to one fresh window; persisted file is non-authoritative.

## Phase 10 — Shell Semantics, Links, Bell, and Notifications
**Scope:** OSC 8, OSC 133/633, prompt navigation, command selection, custom links, trusted notifications.

**Work**
- Accept ADR-0008.
- Add OSC 8 hyperlink identity/lifetime and configurable safe hyperlink rules.
- Add semantic prompt/command/output zones tied to logical rows and propagated via snapshots/events.
- Add previous/next prompt, select command/output, copy command/output actions.
- Add bell policies: disabled, audible, visual, taskbar, notification, with throttling.
- Add native notification requests through allow/deny, rate-limit, focus, and URI validation policy.
- Expose read-only semantic metadata to Lua; terminal output cannot directly execute arbitrary commands.

**Success:** metadata survives scroll/resize/alternate-screen rules; malicious OSC cannot launch arbitrary code or spam; existing OSC 7/52 behavior remains intact.

**Tests:** BEL/ST parsing, malformed/oversized payloads, row mutations, alt screen, trust/rate limits, shell E2E on supported shells.

**Rollback:** side effects default disabled or prompt-gated; semantic parsing can remain internal.

## Phase 11 — IME and Preedit
**Scope:** composition lifecycle, preedit display, candidate positioning, committed-text routing.

**Work**
- Accept ADR-0005 and add a narrow native host interface around GLFW.
- Model start/update/commit/cancel, preedit selection/caret, candidate rectangle, focus loss.
- Render preedit as frontend overlay anchored to the focused pane cursor.
- Send only committed text to input encoder; suppress duplicate character callbacks.
- Ship Windows adapter first, then platform adapters/explicit capability reporting.

**Success:** preedit never reaches PTY; commits are grapheme-safe and exactly once; candidate location tracks pane/zoom/DPI; modal UI and focus changes cancel/transfer deterministically.

**Tests:** composition state machine, Unicode/graphemes, duplicate suppression, pane focus/zoom/DPI, native manual matrix.

**Rollback:** adapter can be disabled, preserving current character callbacks.

## Phase 12 — Accessibility
**Scope:** screen-reader representation and events for windows, tabs, panes, terminal content, cursor, selection, and semantic zones.

**Work**
- Extend the accepted ADR-0005 contract with roles/names/focus/value/text-range/event mapping.
- Build immutable accessibility snapshots from render/core/mux state.
- Add bounded/coalesced events for text, caret, selection, focus, tabs, panes, bells/notifications.
- Add Windows adapter first; define macOS/Linux capability roadmap.
- Provide privacy controls for exposing scrollback and sensitive alternate-screen content.

**Success:** navigation order matches window/tab/pane topology; screen readers receive focused content/caret changes without reading every repaint; no frontend toolkit dependency enters core/mux.

**Tests:** snapshot goldens, event coalescing, text ranges/graphemes, focus topology, privacy policy, manual Narrator/NVDA matrix.

**Rollback:** native adapter is optional; snapshot model remains testable.

## Phase 13 — Image Model and Kitty Graphics
**Scope:** bounded protocol-neutral image store/placements plus Kitty APC implementation.

**Work**
- Accept ADR-0006.
- Add streaming APC/DCS support with hard encoded/decoded byte, pixel, count, time, and per-pane/global budgets.
- Keep `core.Cell` text-only; store image resources/placements adjacent to screen state.
- Define IDs, placement anchors, scroll/erase/resize/alternate-screen/delete/z-order semantics.
- Add renderer-neutral snapshot references and pane-clipped GPU texture cache/damage.
- Implement Kitty transmit/place/delete/query/replies incrementally; defer animation unless explicitly accepted.

**Success:** protocol fixtures render correctly; scroll/erase/resize behavior is deterministic; oversized/malformed input cannot cause unbounded allocation or UI stalls; text-only workloads show negligible regression.

**Tests:** parser golden/fuzz, budget/timeout/decompression-bomb tests, placement lifecycle, snapshots, clipping/damage/cache eviction, performance benchmarks.

**Rollback:** protocol disabled by default until conformance/security gates pass; parser safely ignores unsupported commands.

## Phase 14 — Sixel and iTerm Inline Images
**Scope:** adapters onto the Phase 13 shared model.

**Work**
- Implement streaming Sixel decoder/palette/raster attributes with limits.
- Implement iTerm OSC 1337 inline-file image subset; reject unsafe file/path operations and unsupported metadata clearly.
- Normalize decoded images and placements into the shared store.
- Publish supported-subset documentation and fixtures.

**Success:** both protocols share lifecycle/cache/budgets; text parsing recovers after malformed/truncated streams; no remote file read or arbitrary write is possible.

**Tests:** reference fixtures, fuzz/truncation, palette/size/aspect cases, budget enforcement, cross-protocol placement parity.

**Rollback:** independent feature flags per protocol.

## Phase 15 — Integration, Performance, Migration, and Release
**Scope:** cross-feature hardening and staged delivery.

**Work**
- Run config migrations against real user examples and publish before/after templates.
- Add compatibility matrix and `doctor` output for platform-specific decorations, clipboard, IME, accessibility, notifications, and image support.
- Benchmark startup, memory, idle CPU, input latency, resize, font rebuild, many tabs/windows, semantic metadata, and image transfers against Phase 0.
- Run Windows daily-driver smoke plus macOS/Linux qualification matrices.
- Add crash recovery for invalid config/layout/image cache; ensure logs redact sensitive data.
- Ship checkpoint releases with opt-in gates for the riskiest features; remove temporary adapters only after one stable release.

**Success:** full tests/race/vet/maturity/package gates pass; no blocker security/accessibility findings; measured regressions are within accepted budgets; docs clearly state exclusions.

**Verification commands:**
- `go test ./... -count=1`
- `go test -tags glfw ./... -count=1`
- `go test -race ./internal/... -count=1` in supported subsets
- `go vet -unsafeptr=false ./...`
- `go run ./scripts/check-maturity-gates.go`
- package and installed-binary smoke scripts

**Rollback:** release flags and compatibility adapters allow checkpoint-level rollback without config loss.

---

## Recommended Issue/PR Boundaries
- One tracking epic per phase.
- One ADR PR before each architectural phase.
- Configuration phases follow their ADR slices; every PR updates all applicable Go/Lua/Teal/template/reload/test/docs contracts, with a final evidence PR only for validation and status closeout.
- Tabs: mux model PR, action integration PR, tab bar PR, move/lifecycle hardening PR.
- Windows/workspaces: host controller PR, move actions PR, persistence PR.
- Shell integration: OSC 8 PR, semantic zones PR, actions/UI PR, notification policy PR.
- IME/accessibility: host seam PR, Windows adapter PR, later platform adapter PRs.
- Images: parser/model PR, budgets/fuzz PR, snapshot/renderer PR, then one protocol PR at a time.

## Stop Conditions
Stop and return to architecture review if:
- a phase introduces domain/daemon/remote-session abstractions;
- mux ownership moves into GLFW;
- preedit enters PTY before commit;
- terminal output can invoke external side effects without policy;
- image data enters `core.Cell` or bypasses resource budgets;
- multi-window support requires off-thread GLFW/OpenGL calls;
- config reload cannot preserve the last valid runtime;
- renderer selection becomes a prerequisite.

## Immediate Next Action
Implement Phase 2 slice 1 only: presence-aware raw v1/v2 documents, schema metadata, strict v2 validation, in-memory migrations, and compatibility goldens. Do not add includes until the v1 compatibility gate passes.
