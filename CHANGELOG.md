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

### Fixed

- Keep selection, search, links, scrollback, mouse reporting, resize events, and Lua callbacks isolated to their originating pane.
- Preserve the released mouse button in SGR reports so mouse-aware terminal applications do not remain stuck dragging.
- Bound and back off divider-settlement retries so a persistent PTY resize failure cannot leave the application spinning or repeatedly notifying.
- Generate Teal Lua output beside its source file instead of the current working directory.
- Center DirectWrite glyph advances correctly.
