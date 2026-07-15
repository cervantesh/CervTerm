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
### Security

- OSC 52 clipboard write is now OFF by default (was "write"); set `clipboard.osc52 = "write"` to restore. Clipboard reads via OSC 52 remain denied. (#61)
- Bracketed paste strips any embedded `ESC[201~` end marker so a hostile clipboard cannot break out of bracketed mode into command input. (#62)
- The pprof endpoint (`CERVTERM_PPROF`) now refuses non-loopback binds, so `/debug/pprof` is not exposed on the network. (#63)
- The diagnostic log file is created `0600` (dir `0700`) instead of world-readable. (#64)

### Fixed

- Windows: the shell now inherits the full parent environment (PATH, etc.); external programs (git, node, python, npx…) launch correctly, and `shell.env` from config now applies on Windows too. (#70)
- Windows: CervTerm now advertises ANSI color support (`ANSICON`) so console apps that gate coloring on it (Django, colorama/supports-color CLIs) emit color instead of monochrome. CervTerm already rendered ANSI; the apps just weren't emitting it.
- Zoom animates frame by frame while only the ConPTY resize is debounced, so rapid zoom no longer garbles or duplicates scrollback. (#59)
- Scrollback view stays pinned to content while output streams in. (#57)

### Changed

- The window title now honors `window.dynamic_title`; when disabled it stays "CervTerm" instead of reflecting host-controlled OSC 0/2 titles. (#65)
- Split terminal core into smaller files to keep touched files under 500 lines.
- Preserved mouse modifiers during SGR drag reports.
- Expanded README with build/run/configuration and limitation guidance.
- CI artifacts now include generated default config and release documentation/metadata directories.
- Emoji cluster grouping now includes emoji modifiers in addition to combining marks and ZWJ sequences.

### Known limitations

- Full GSUB/GPOS shaping, real SVG outline text/font selection, broader redistributable color-font fixtures, authoritative vttest raw captures, `.syso` resource embedding, installer packaging, and signed releases are not implemented yet.
