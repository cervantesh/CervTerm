# Project Context Snapshot

## Current Understanding
CervTerm is a Go terminal emulator with a GLFW/OpenGL frontend, VT parser/core terminal model, local PTY integration, and a font rendering backend that handles OpenType, DirectWrite shaping on Windows, and color glyph rendering.

The current architecture concern is Unicode/emoji rendering correctness. Recent work made 60+ emoji samples render visually in `docs/assets/cervterm-emoji-inspection.png`, but the implementation still contains manual Unicode ranges and host-font-specific handling. The project now needs an architecture direction that scales beyond individual emoji fixes.

## Confirmed Facts
- Language/runtime: Go 1.24 module `cervterm`.
- Terminal cell width currently lives in `internal/core/width.go`.
- Render cluster collection currently lives in `internal/frontend/glfwgl/cluster.go`.
- Font fallback and emoji-family selection currently live in `internal/fontglyph/backend.go`.
- Segoe UI Emoji compatibility rendering currently uses COLRv0 preference and canvas fitting in `internal/fontglyph/color_colr_render.go`.
- Verification includes `go test ./...`, GLFW-tagged tests, and Go-based visual/capture smoke helpers under `scripts/`.

## Architecture Direction So Far
CervTerm should not maintain per-emoji hand patches. Unicode behavior must be modeled at the level of generated Unicode properties, grapheme clusters, and cluster display width. Font fallback and color rendering should operate on clusters rather than individual runes.

## Assumptions
- Primary target for current emoji rendering work is Windows using Segoe UI Emoji, but the architecture should allow other emoji fonts and platforms.
- The project prioritizes correctness/testability over minimizing a small generated Unicode table.
- Generated data should be reproducible and pinned to a Unicode version.

## Missing Information
- Target Unicode version for generated tables.
- Whether generated tables should be committed or generated at build/test time.
- Whether bundled emoji fallback fonts are acceptable later; currently no bundled full emoji font is assumed.

## Questions
1. Confirm Unicode version policy before generating tables, e.g. latest stable Unicode at implementation time.
2. Confirm whether generated Unicode tables may be committed under `internal/unicodeprops` or equivalent.
