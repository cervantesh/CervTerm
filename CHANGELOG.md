# Changelog

All notable changes to CervTerm will be documented in this file.

The format is based on Keep a Changelog, and this project uses an experimental pre-1.0 versioning scheme.

## [Unreleased]

### Added

- Native in-process panes with horizontal and vertical splits, focused input, directional navigation, independent terminal sessions, and deterministic close/collapse behavior.
- Resize adjacent panes live by dragging their divider with the mouse while preserving minimum terminal dimensions.

### Fixed

- Keep selection, search, links, scrollback, mouse reporting, resize events, and Lua callbacks isolated to their originating pane.
- Preserve the released mouse button in SGR reports so mouse-aware terminal applications do not remain stuck dragging.
- Bound and back off divider-settlement retries so a persistent PTY resize failure cannot leave the application spinning or repeatedly notifying.
- Generate Teal Lua output beside its source file instead of the current working directory.
- Center DirectWrite glyph advances correctly.
