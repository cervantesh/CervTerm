# Changelog

All notable changes to CervTerm will be documented in this file.

The format is based on Keep a Changelog, and this project uses an experimental pre-1.0 versioning scheme.

## [Unreleased]

### Added

- Native in-process panes with horizontal and vertical splits, focused input, directional navigation, independent terminal sessions, and deterministic close/collapse behavior.

### Fixed

- Keep selection, search, links, scrollback, mouse reporting, resize events, and Lua callbacks isolated to their originating pane.
- Generate Teal Lua output beside its source file instead of the current working directory.
- Center DirectWrite glyph advances correctly.
