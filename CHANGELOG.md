# Changelog

All notable changes to CervTerm will be documented in this file.

The format is based on Keep a Changelog, and this project uses an experimental pre-1.0 versioning scheme.

## [Unreleased]

### Added

- Native in-process panes with horizontal and vertical splits, focused input, directional navigation, independent terminal sessions, and deterministic close/collapse behavior.
- Resize adjacent panes live by dragging their divider with the mouse while preserving minimum terminal dimensions.
- Zoom the focused pane independently while sharing one bounded multi-size glyph atlas across all panes.
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

### Fixed

- Keep selection, search, links, scrollback, mouse reporting, resize events, and Lua callbacks isolated to their originating pane.
- Allow pane-bound Lua callbacks to read, update, and reload runtime configuration through their originating frontend host.
- Preserve the released mouse button in SGR reports so mouse-aware terminal applications do not remain stuck dragging.
- Bound and back off divider-settlement retries so a persistent PTY resize failure cannot leave the application spinning or repeatedly notifying.
- Generate Teal Lua output beside its source file instead of the current working directory.
- Center DirectWrite glyph advances correctly.
