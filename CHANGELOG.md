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

- Default Lua config template generation via `--print-default-config`.
- Parser fuzz smoke target and replay-style VT golden fixture.
- vttest-oriented compatibility checklist.
- Unix PTY implementation compile-verified for Linux.
- Renderer-neutral color glyph backend support for bitmap color glyphs, COLRv0/CPAL, broad COLRv1 paints/composites/variations, SVG glyph extraction/rasterization, and initial cluster handling.
- Optional system emoji color-font rasterization fixture when a known system emoji font is installed.
- SVG text/tspan layout handling with font-size, text-anchor, and dominant-baseline support.
- Redistributable SVG text fixture table and vttest capture workflow notes/script.
- Windows packaging metadata: manifest, version info, resource script, SVG icon source, and generated `.ico`.
### Changed

- Split terminal core into smaller files to keep touched files under 500 lines.
- Preserved mouse modifiers during SGR drag reports.
- Expanded README with build/run/configuration and limitation guidance.
- CI artifacts now include generated default config and release documentation/metadata directories.
- Emoji cluster grouping now includes emoji modifiers in addition to combining marks and ZWJ sequences.

### Known limitations

- Full GSUB/GPOS shaping, real SVG outline text/font selection, broader redistributable color-font fixtures, authoritative vttest raw captures, `.syso` resource embedding, installer packaging, and signed releases are not implemented yet.
