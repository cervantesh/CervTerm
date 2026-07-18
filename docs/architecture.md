# CervTerm Architecture

## Decisions

- Language: Go for the MVP, with the explicit assumption that Go may not be the final best tool.
- UI/toolkit: no Fyne, no Gio, no widget toolkit. The MVP uses a thin GLFW/OpenGL frontend.
- Inspiration: Alacritty first (small, fast, layered), with selected WezTerm-inspired local mux and UX capabilities. WezTerm-style domains are explicitly out of scope.
- Performance policy: correctness and boundaries first; measure GC/allocation impact from day one; optimize only with evidence.
- Graphics backend: OpenGL through GLFW remains the only supported backend. Vulkan work is paused indefinitely; see [Rendering backend decision](rendering-backend-decision.md).

## Layering

```text
cmd/cervterm
  -> internal/frontend/glfwgl  window, input, OpenGL projection (optional tag)
  -> internal/mux             pane IDs, split tree, focus, layout, lifecycle and session aggregates
  -> internal/render          renderer-neutral per-pane frame snapshots
  -> internal/pty             local PTY/ConPTY byte transports
  -> internal/vt              escape parser, toolkit-neutral
  -> internal/core            per-pane grid, cells, cursor, attributes and scrollback
  -> internal/metrics         GC/allocation/frame counters
```

The core never imports the mux, renderer, PTY, GLFW, or OpenGL. Each mux pane owns one PTY, VT parser, terminal core and render snapshot. PTY readers enqueue pane-addressed bytes; the GLFW main thread serializes parsing, topology, focus, lifecycle and rendering. The frontend projects positioned panes and routes input; it does not own the split tree or sessions.

## Native in-process mux

```text
Frontend -> Mux Window -> implicit Tab -> SplitTree -> Pane -> local PTY Session -> Terminal Core
```

The mux is process-local and supports native column/row splits, stable split identities and ratios, draggable dividers, focused-pane input, independent scrollback/selection/search/mouse/zoom state, deterministic close/collapse and clipped rendering. GLFW projects pointer and font intent, while `internal/mux` validates ratios and owns pixel/grid geometry using renderer-neutral metrics per pane. Terminal grids update live and PTY resize settles once after divider or pane-zoom interaction. Mixed font sizes share one bounded two-page glyph atlas whose entries are namespaced by raster specification; selecting a pane never clears atlas pages. Visible tabs, multiple local windows, and layout-only workspaces are planned above these ownership boundaries. Domains, a daemon, live detach/reattach, remote sessions, and tmux integration are excluded.

## Bounded font model and fixed-grid projection

Font startup is a transaction: configuration canonicalizes into immutable descriptors, fallback rules, shaping features and metric projection; required primary resources and the first atlas context are prepared before the application adopts configuration or starts the mux/PTY. Parsed faces are singleflight-loaded outside the cache lock, pin-owned by backends, and bounded to 128 faces/256 MiB. Context admission is separately bounded to 64 retained identities with visible pins and deterministic inactive LRU eviction. A rejected or aborted candidate closes only candidate ownership and cannot change the active context, pane grid or PTY size.

`FontEnvironmentKey` includes descriptors, fallback/rules, features, metric projection, base size, pane zoom, DPI and raster controls. `ResolvedFaceKey` adds the selected canonical face, source tier/rule, effective target and synthetic mode. Atlas positive and negative entries use both levels, so styles, fallback choices, shaping settings and metrics cannot alias. Negative raster/run/insertion results share one 8,192-entry generation-scoped budget per context; the GPU atlas remains exactly two 2048² RGBA pages.

Metric projection scales one integer cell width/height per context and shifts raster ink only after natural metrics are known. Rune, cluster, shaped-run, fallback and color paths are placed into that fixed canvas; logical cell advances remain content-independent. Pane zoom/DPI uses the same projected metrics for mux layout, hit testing and deferred PTY resize. Advanced font configuration is restart-scoped; live candidate evaluation performs no font I/O or active mutation.

## Phase 5 appearance ownership and bounds

Appearance configuration remains data owned by `internal/config`; composition/decoding of bounded background layers is isolated in `internal/background`; GLFW applies DPI-aware per-side padding, stable scrollbar gutter, opacity, presentation gating, and native startup requests. Background decode/compose work prepares candidate CPU/GPU resources off the active state, and main-thread adoption preserves the last-known-good surface on failure. Image dimensions, decoded bytes, layer count, cache entries/bytes, and asynchronous work are bounded.

`render.max_fps` is a presentation gate layered over damage-driven redraw and vsync, not a scheduler or renderer choice. Scrollbar fade uses its separately bounded animation FPS and cannot change grid geometry when stable gutter is enabled. Initial rows/columns derive initial client geometry from terminal metrics; decorations/titlebar are platform-capability requests scoped to native window recreation.

The architecture intentionally exposes no renderer selector. `internal/core`, `internal/vt`, `internal/render`, `internal/mux`, `internal/config`, and `internal/background` remain free of GLFW/OpenGL imports; only the build-tagged frontend projects these policies through the existing `gpu.Renderer` seam.

## Candidate configuration source graph

`internal/config.BuildSourceGraph` is the candidate-only foundation for Phase 2 composition. It consumes one fresh caller-owned candidate Lua state, canonicalizes local Lua/Teal source identity (including filesystem aliases), evaluates one primary plus declarative includes exactly once in that state, and emits deterministic depth-first post-order nodes and dependency edges. A failed candidate state is discarded, and a state cannot be submitted for a second build. Depth, source-count, per-file byte, and aggregate-byte limits reject a candidate before it can affect active configuration.

Primary evaluation occurs once before include traversal so it can declare edges. Includes and their nested `require`/`dofile`/`loadfile` calls run under a declarative guard: they may return values and typed actions but cannot register timers, status entries, or overlays. The instrumented standard loaders record canonical local module dependencies and v2 rejects replacement/custom loaders that would make reload completeness unknowable.

Teal sources check and generate into a per-candidate owned staging directory (including beneath a caller-supplied staging parent); the graph reserves their eventual adjacent Lua paths and rejects source/derived-output collisions without changing adjacent files. Candidate staging is removed when the graph closes. After composition and final validation, `PublishStagedTeal` may publish all staged outputs through a separate candidate-only transaction: absent targets are created, unmarked legacy outputs are adopted only when byte-identical, and existing markers must name the same canonical Teal source. Foreign/unowned, nonregular, hardlinked, duplicate, or explicit-module-colliding targets reject before commit.

Publication prepares every same-directory temp before mutation, removes only stale CervTerm temp files older than 24 hours, rechecks path identity/mode/bytes before commit and immediately before each replacement, then publishes the ownership marker before its output. Unix replacements and rollback removals sync the parent directory; Windows uses write-through `MoveFileEx`. Injected or real failures restore prior bytes/modes or remove newly-created files in reverse order. The rollback contract is byte/content and permission-mode exact; ACLs, xattrs, timestamps, ownership, and hardlink identity are deliberately outside it, and pre-existing hardlinks are rejected. Configuration remains trusted local code rather than a security boundary.

`internal/config.ComposeSourceGraph` consumes that graph in deterministic post-order and builds a new root table in the same Lua state. Records merge recursively, `shell.env` merges by key, lists replace, event function slots replace independently, and `cervterm.config.unset` suppresses lower layers while allowing a higher value to win later. A 100,000-node/list-entry ceiling bounds composition. Provenance is retained per fixed schema leaf, map key, list, and event function with source versions and a low-to-high overwrite chain; it stores no raw values and marks sensitive paths.

Named `environments` and `profiles` are strict partial documents. Same-name declarations remain source-local and apply in graph order after ordinary include/primary fields, so the chosen environment wins over the base and the chosen profile wins over the environment without losing per-source provenance. The pure candidate selector resolves environment override, `CERVTERM_ENV` input, configured default, then exact GOOS fallback; profile resolution uses override, `CERVTERM_PROFILE` input, then configured default. Missing explicit/configured selections fail, while an absent GOOS match selects nothing.

Candidate `CLIOverride` values apply left-to-right after the selected profile. Paths resolve against schema metadata; booleans/numbers/integers/lists use JSON and schema-known strings may be unquoted. Records, callbacks, bindings, composition metadata, and sensitive `shell.env` paths are rejected before values are decoded. Provenance records only the argument index and path, never the raw argument value. The decoder is pure and has no command-line wiring in this slice; final cross-field validation remains the candidate bundle caller's responsibility.

`internal/script.BuildCandidateBundle` now creates the ownership unit needed for transfer: one validated composed `Config`, the single candidate Lua state and effective bindings/events plus primary timers/status/overlays, selection/provenance, dependency graph/staging, and deferred idempotent Teal publication. Every source's legacy fail-fast scripting surfaces validate before effective merge. Build failures close the Lua state and staging; bundle accessors detach mutable data; `Close` releases runtime then graph exactly once. Bundle lifecycle is serialized on the main thread.

`script.LoadVersioned` is now the executable/reload dispatch seam. It evaluates the selected source exactly once and chooses from the authored version: omitted/v1 retains the single-source runtime and marker-free `tl gen` contract, while explicit v2 retains the whole candidate bundle. Dependency-capture wrappers are removed without undoing v1 user replacements, and v1 keeps last-return semantics. A v2-owned Teal-to-v1 transition journals generated output and marker bytes until frontend activation succeeds.

Frontend live application is split into fallible `prepareLiveConfig` and mechanically infallible `commitLiveConfig`. Startup prepares GLFW/GL/font resources, publishes staged v2 Teal, commits the candidate runtime, then spawns the PTY. Reload prepares every raster resource without creating pane UI state, publishes Teal, swaps config/runtime/bundle on the main thread, and only then closes the old owner. Publication/resource faults preserve the last-known-good active state; v2-to-v1 journal rollback restores external artifacts as well.

Composition is active only for explicitly authored v2. Active reload watching follows the complete evaluated graph: primary, declarative includes and aliases, plus evaluation-time local `require`/`dofile`/`loadfile` dependencies. Digests bind canonical identity to exact bytes, selected symlink aliases remain watched, missing files trigger reload, and whole-graph debounce coalesces editor writes. Failed graph/load errors carry detached value-free path expectations rather than parsed diagnostic text. On failure the watcher atomically installs `last successful graph ∪ latest failed-attempt paths`; a later failure replaces the failure-only set and success clears it. A newly discovered set queues one immediate retry to close the evaluation race, while later creation/change is debounced normally. Candidate/resource ownership remains untouched. Identical failure log/UI notices are bounded to one per 30 seconds, but watcher polling and reload eligibility are independent. Modules loaded only later by runtime callbacks remain outside this graph by design.

Terminal cell attributes retain logical color identity as `default`, indexed `0..255`, or literal RGB instead of storing eagerly resolved physical colors. SGR basic/bright and `38/48;5` write indexed identity; truecolor remains literal and reset writes logical default, eliminating the old RGB-sentinel collision. Snapshots copy logical cells unchanged. The GLFW projection builds one dense resolver per frame from configured foreground/background, live `colors.ansi` 0–15, the xterm cube/grayscale fallback for 16–255, then live sparse `colors.indexed_colors` overrides. The sparse comparable config array cannot overlap ANSI ownership; numeric map composition/provenance is sorted per key and unset restores algorithmic fallback. Resolver lookup is O(1) and passed by pointer per row. Palette reload invalidates frame damage through the existing atomic live-config commit, so already-buffered cells recolor without reparsing while truecolor remains invariant.

Every public configuration leaf now has one schema-owned application scope: `live`, `new_pane`, `new_window`, `window_recreate`, or `restart`. The frontend owns detached desired and effective snapshots plus value-free pending path/scope records and the last reload error. Opacity/blur, colors, scrolling, scrollbar, and cursor policy merge into the active window live; shell changes remain desired and are used by future split panes; initial width/height are new-window; conservative resource/cached-policy fields remain restart-scoped. Reload failure preserves both snapshots and pending state, while scoped notices name bounded paths without values.

The single-window frontend now owns one opaque process-local `ConfigScopeID` from startup through `App` teardown. Existing Lua setters adapt typed values into ordered runtime path/value patches decoded by the same schema coercion used for CLI overrides. A setter prepares and commits synchronously; last successful field transaction wins, closed scopes reject mutation, and explicit path/all clearing restores the composed value. Scoped patches do not mutate the composed base, survive successful file reload, and are revalidated against each candidate before publication/transfer; invalid reapplication rejects the whole reload. Value-free runtime records project `LayerRuntime` provenance with the scope ID over the composed winner/overwritten chain.

Public legacy `script.Load` remains available. The executable is the only process-input adapter: it snapshots `--environment`/`CERVTERM_ENV`, `--profile`/`CERVTERM_PROFILE`, exact `runtime.GOOS`, and ordered repeatable `--config-override` arguments into detached `CandidateOptions`. The active v2 bundle transfers that immutable snapshot to the frontend, which reuses it on every reload. Explicit composition flags fail closed without a source, against v1, and on a v2-to-v1 reload; ambient selection variables remain ignored by v1 for compatibility. Raw override values never enter logs/provenance.

`--explain-config`, repeatable `--explain-config-field`, and composed `--doctor` run through a diagnostic-only `LoadVersioned` path before logging, profiling, GLFW, window, frontend, or PTY startup. Candidate bundles expose detached config/provenance and value-free graph snapshots; no source bytes, hashes, Lua functions, raw CLI values, or Teal staging paths render. Schema-sensitive values such as `shell.env` are always `<redacted>`. V2 explanation fails closed for v1/no-source/unknown fields, while doctor keeps explicit v1/no-source support boundaries and returns nonzero on load failure. Diagnostic-only v1 Teal evaluation skips legacy adjacent publication. Pending active-process fields and reload failures are honestly unavailable without IPC/persistence. Future native-window origin/focus mapping remains deferred.

## Verifiable measurements

Run parser/core allocation checks:

```bash
go test ./internal/vt -bench=. -benchmem
go test ./internal/render -bench=. -benchmem
```
go run ./scripts/capture-parity-baseline.go -count 3

Run runtime GC tracing:

```bash
GODEBUG=gctrace=1 go run ./cmd/cervterm
```

MVP overlay shows bytes read, frames, malloc count, heap, GC count, and last GC pause. This is intentionally visible so GC/reuse discussions stay evidence-based.

The current cross-subsystem delivery contract is [`docs/wezterm-parity-roadmap.md`](wezterm-parity-roadmap.md); reproducible measurements are recorded in [`docs/parity-baseline.md`](parity-baseline.md).
