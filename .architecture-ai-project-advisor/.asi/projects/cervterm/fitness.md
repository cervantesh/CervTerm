# Architecture Fitness Functions

## Context
CervTerm must scale Unicode/emoji rendering beyond per-symbol fixes. The accepted architecture direction is ADR-0001: generated Unicode data + grapheme/emoji cluster rendering + per-cluster font fallback.

## Fitness Functions

| ID | Function | Type | Target | How to Check | Failure Signal | Owner |
|---|---|---|---|---|---|---|
| AF-001 | Emoji semantics use generated Unicode properties, not scattered handwritten ranges | static/review | `internal/core`, `internal/frontend/glfwgl`, `internal/fontglyph` | grep for new raw emoji ranges outside generated package and approved compatibility shims | new `0x1F...`, `0x2600`, `0x27BF`, `0x2B50` checks outside `internal/unicodeprops` or documented font shim | architecture |
| AF-002 | Renderer forms grapheme/emoji clusters before font fallback | unit | cluster collector / future segmenter | tests for `❤️`, `✍️`, `👩‍💻`, `🧑🏽‍🚀`, `1️⃣`, `🇦🇷`, combining marks | cluster split into individual codepoints or wrong `CellSpan` | renderer |
| AF-003 | Cluster display width is computed at cluster level | unit | core/render boundary | tests for BMP emoji + VS16, ZWJ, keycap, regional indicators, modifiers | emoji presentation cluster occupies 1 cell or modifiers consume cells | core |
| AF-004 | Emoji clusters use emoji font fallback as a whole cluster | unit/integration | `internal/fontglyph` | tests assert emoji clusters choose Segoe UI Emoji or configured emoji fallback on Windows | mixed monospace/emoji font within one cluster, tofu, or monochrome fallback when color font exists | font backend |
| AF-005 | Segoe UI Emoji compatibility remains isolated to font backend | review/static | `internal/fontglyph` | review direct Segoe/COLR compatibility code stays inside font backend | terminal core/frontend knows about Segoe-specific behavior | architecture |
| AF-006 | Visual emoji suite covers categories, not individual bug reports only | manual/integration | screenshot harness | capture includes faces, BMP+VS16, hands+modifiers, ZWJ, keycaps, flags, food, transport, symbols | screenshot lacks category coverage or only tests previously reported examples | QA |
| AF-007 | Unicode data provenance is pinned and reproducible | static/test | generator + generated package | generated files include Unicode version/source URL/hash; generator has deterministic output test | generated tables have no version/provenance or cannot be regenerated | build |
| AF-008 | Unicode upgrades are explicit | process/review | changelog/ADR/tests | Unicode version bumps must mention changed data and rerun category tests | silent Unicode data drift | release |

## Suggested Automation

- Static checks:
  - Add a lightweight grep/script that fails on raw emoji range checks outside `internal/unicodeprops` and documented font compatibility shims.
  - Ensure generated files contain `Code generated` and Unicode version metadata.
- Unit tests:
  - Cluster segmentation tests for combining marks, ZWJ, modifiers, regional indicators, keycaps, BMP+VS16.
  - Width tests for cluster-level display width.
  - Font fallback tests for emoji cluster font selection.
- Integration tests:
  - Backend raster tests for representative emoji categories.
  - Optional PTY/capture smoke for the emoji grid.
- Code review checks:
  - No one-off emoji patches unless they are font compatibility shims with a failing test and comment.
- CI gates:
  - `go test ./...`
  - `go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1`

## Minimum Set Before Implementation

- [ ] Create `internal/unicodeprops` package with generated data or a clearly chosen external dependency.
- [ ] Add generator/provenance for Unicode data.
- [ ] Replace handwritten checks in `internal/core/width.go`, `internal/frontend/glfwgl/cluster.go`, and `internal/fontglyph/backend.go`.
- [ ] Add cluster-width tests for the categories listed above.
- [ ] Preserve current 60+ emoji screenshot capture as visual regression evidence.

## Notes
The current code may keep temporary handwritten ranges while the generated property package is built. After the generated package lands, those ranges become technical debt and should be removed or explicitly documented as font-specific compatibility shims.
