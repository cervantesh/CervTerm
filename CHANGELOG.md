# Changelog

All notable changes to CervTerm will be documented in this file.

The format is based on Keep a Changelog, and this project uses an experimental pre-1.0 versioning scheme.

## [Unreleased]

### Added

- Product roadmap and implementation plan.
- Lua configuration loading and Teal check/gen integration.
- VT scroll regions, insert/delete char/line operations, cursor visibility, autowrap, application cursor/keypad modes, and SGR mouse mode tracking.
- Expanded key encoding and SGR mouse event encoding.
- Unicode width handling for CJK/wide cells and combining marks.
- OpenType-rendered embedded Go Mono glyph atlas.
- GitHub Actions CI for tests, GLFW compile check, and Windows artifact build.

### Changed

- Split terminal core into smaller files to keep touched files under 500 lines.

### Known limitations

- Manual interactive validation remains pending.
- System font discovery and installer packaging are not implemented yet.
