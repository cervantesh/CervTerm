# Changelog

All notable changes to CervTerm will be documented in this file.

The format is based on Keep a Changelog, and this project uses an experimental pre-1.0 versioning scheme.

## [Unreleased]

### Added

- Native in-process panes with horizontal and vertical splits, focused input, directional navigation, independent terminal sessions, and deterministic close/collapse behavior.
- Resize adjacent panes live by dragging their divider with the mouse while preserving minimum terminal dimensions.
- Zoom the focused pane independently while sharing one bounded multi-size glyph atlas across all panes.
- Retained top/bottom tab bar with bounded live configuration, active-visible overflow, Unicode-safe labels, add/close controls, and authoritative window/scrollbar geometry reservation.
- Closed typed tab actions for spawn, stable-ID activation/move/rename/close, relative navigation, cross-tab pane transfer, and the retained tab switcher across JSON, Lua, Teal, and the command palette.
- Stable-ID tab close confirmation with revision invalidation, lifecycle-aware running-pane checks, tab-switcher retention across reorder, and one-shot background activity badges.
- Configurable RGBA backgrounds, live appearance reload, a reserved scrollbar, and experimental native blur providers for macOS AppKit and KDE X11/Wayland with transparent fallback.
- A phased WezTerm-inspired parity roadmap, machine-readable support matrix, reproducible performance baseline tool, configuration compatibility policy, and proposed architecture decision gates.
- A toolkit-neutral typed action model and frontend executor with deterministic registry metadata, semantic targets, strict bounded JSON serialization, sequences, callback metadata, trigger policies, and typed built-in key dispatch.
- Typed Lua and Teal key actions, validated action composition, optional discovery labels, and legacy callback execution through the shared action executor and watchdog.
- Versioned configuration documents with strict v2 field/type diagnostics, exact unversioned v1 compatibility, presence tracking, and in-memory migration scaffolding.
- A candidate-only canonical configuration source graph with bounded include traversal, dependency capture, declarative side-effect guards, and transactional Teal staging.
- A candidate-only schema composition engine with deterministic record/map/list/function merge, immutable unset tombstones, bounded node counts, and value-free provenance chains.
- Candidate-only named environment/profile declarations with deterministic selection precedence, same-name merge, and environment-then-profile provenance layers.
- A candidate-only typed CLI override engine with schema capabilities, sensitive-path rejection, ordered post-profile application, and argument-index provenance.
- Candidate-only transactional Teal output publication with ownership markers, byte-identical legacy adoption, commit-time identity checks, durable atomic replacement, and reverse rollback.
- A candidate-only ownership bundle for validated composed configuration, Lua runtime surfaces, provenance/selection, dependency graph/staging, and deferred Teal publication.
- Exact-once version-aware config loading and atomic explicit-v2 startup/reload activation with prepared frontend resources, bundle ownership transfer, and v2-to-v1 Teal artifact rollback.
- Dependency-aware hot reload for the complete evaluated config graph, including include/module aliases, deletion and symlink retarget detection, coalesced debounce, and generation-safe acknowledgement.
- Schema-owned live/new-pane/new-window/recreate/restart classifications with detached desired/effective state, exact pending diagnostics, live cursor reload, and desired shell settings for future panes.
- Durable process-local `ConfigScopeID` patches for live Lua setters, with shared typed decoding, reload revalidation, explicit clearing, stale-scope rejection, and runtime provenance chains.
- Executable v2 environment/profile selection and repeatable typed `--config-override` inputs, snapshotted consistently across startup and reload.
- Failed config attempts now watch the latest discovered and missing dependencies alongside the last successful graph, enabling automatic repair recovery with bounded repeated notices.
- Read-only `--explain-config`/field filters and composed doctor diagnostics with deterministic provenance, graph reporting, sensitive-value redaction, and no frontend or Teal publication side effects.
- Logical default/indexed/truecolor cell attributes and a live configurable 16-color ANSI palette resolved during rendering, allowing existing scrollback to recolor without reparsing.
- Live sparse `colors.indexed_colors` overrides for xterm indices 16–255, with per-key composition/provenance and algorithmic fallback.
- Local v2 `color_schemes` catalogs and live `color_scheme` selection with deterministic composition, inline color precedence, provenance, diagnostics, Teal types, and atomic reload.
- Six live semantic chrome colors for application surfaces, muted text, accents, pane dividers, search matches, and error state, available in inline colors and local named schemes.
- Bounded pane-local OSC 4/10/11 palette set/query and OSC 104/110/111 reset support, with canonical replies, live base-palette reload, and logical scrollback reprojection.
- Deterministic v2 font descriptors with real normal/bold/italic/bold-italic selection, bounded parsed resources, and safe legacy shorthand fallback.
- Lazy whole-cluster fallback and symbol rules with ordered primary/fallback/embedded resolution and bounded font discovery.
- OpenType feature projection and fixed-grid line-height, cell-width, baseline, and glyph offsets with pane-safe cache identities and redacted diagnostics.
- Phase 5 appearance and window controls: per-side padding; independent text/background opacity; bounded solid, gradient, and image layers; scrollbar visibility/stable-gutter/fade-FPS policy; `render.max_fps`; and initial rows/columns plus native decoration/titlebar requests. Renderer selection remains explicitly excluded.
- Bounded leader chords, named key tables, exact typed mouse bindings with exclusive gesture capture, and transactional pane resize/swap/move actions, while preserving legacy flat `keys` callbacks.
- A retained command palette for discoverable typed actions and labeled bindings, with runtime-safe callback invalidation, complete modal input capture, and damage-driven idle rendering.
- Process-owned native windows and named local workspaces with stable-ID transfer/actions, retained switching, and opt-in atomic layout-only persistence that restores fresh sessions with safe monitor/config fallback.
- Bounded pane-local OSC 8 hyperlink metadata with 32-byte cell identity, safe referenced-entry retention, primary/alternate isolation, reflow/scrollback preservation, detached render/mux projection, and fresh explicit-click activation through a centralized absolute HTTP(S) policy; no automatic URL opening.
- Bounded quick select for visible HTTP(S) links and compiled custom regex rules, with prefix-free labels, copy/open actions, and stale-generation rejection.
- A bounded retained launch menu with argv-only local process descriptors, sensitive environment provenance, deterministic environment merging, and spawn-before-topology commit.

### Fixed

- Keep selection, search, links, scrollback, mouse reporting, resize events, and Lua callbacks isolated to their originating pane.
- Allow pane-bound Lua callbacks to read, update, and reload runtime configuration through their originating frontend host.
- Preserve the released mouse button in SGR reports so mouse-aware terminal applications do not remain stuck dragging.
- Bound and back off divider-settlement retries so a persistent PTY resize failure cannot leave the application spinning or repeatedly notifying.
- Generate Teal Lua output beside its source file instead of the current working directory.
- Center DirectWrite glyph advances correctly.
