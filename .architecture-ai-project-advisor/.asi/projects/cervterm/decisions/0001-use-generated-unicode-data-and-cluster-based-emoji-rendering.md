# ADR: Use generated Unicode data and cluster-based emoji rendering

## Status
Accepted

## Date
2026-07-10

## Context
CervTerm renders terminal text through a core cell model, a GLFW/OpenGL frontend, and a font backend with DirectWrite shaping and color glyph support. Recent emoji work fixed visible defects in a 60+ emoji grid, including fragmented Segoe UI Emoji COLR rendering and BMP emoji width issues such as `✍️`, `☕`, `✈️`, `⚽`, `⭐`, and `❤️`.

Those fixes proved the direction but also exposed a scaling risk: the emoji universe is too large and changes too often to handle via one-off codepoint patches. Terminal correctness depends on multiple Unicode concepts that are broader than individual emoji samples:

- grapheme cluster segmentation,
- Extended Pictographic / Emoji / Emoji Presentation properties,
- variation selectors, especially VS15/VS16,
- emoji modifiers and modifier bases,
- regional indicators and keycap sequences,
- East Asian Width and terminal cell width,
- per-cluster font fallback and shaping.

Current evidence in the repo:

- `internal/core/width.go` computes rune width using handwritten ranges.
- `internal/frontend/glfwgl/cluster.go` collects render clusters using handwritten emoji/ZWJ/regional indicator logic.
- `internal/fontglyph/backend.go` chooses Segoe UI Emoji for emoji clusters using handwritten checks.
- `internal/fontglyph/color_colr_render.go` contains Segoe-specific COLRv0 compatibility and canvas fitting.

## Decision
CervTerm will move Unicode/emoji correctness from per-symbol patches to a generated Unicode-data architecture.

1. Add a small internal Unicode properties package, e.g. `internal/unicodeprops`, generated from pinned Unicode data files.
2. Generate and commit property tables needed by the terminal renderer:
   - `Emoji`,
   - `Emoji_Presentation`,
   - `Extended_Pictographic`,
   - `Emoji_Modifier`,
   - `Emoji_Modifier_Base`,
   - `Regional_Indicator`,
   - `Variation_Selector`,
   - `East_Asian_Width` categories relevant to terminal width.
3. Replace handwritten emoji range checks in core/frontend/fontglyph with calls to the generated property package.
4. Introduce cluster-level display width as the renderer-facing contract:
   - text/combining clusters generally width 1,
   - emoji presentation clusters generally width 2,
   - regional-indicator pairs width 2,
   - keycap clusters width 2,
   - ZWJ emoji sequences width 2,
   - combining marks and modifiers remain width 0 inside their cluster.
5. Keep per-cluster font fallback:
   - emoji clusters prefer an emoji color font such as Segoe UI Emoji on Windows,
   - normal text remains in the configured monospace font,
   - fallback must be selected for the whole cluster, not individual runes.
6. Keep current Segoe UI Emoji COLRv0 compatibility as a Windows font-specific rendering compatibility layer, but isolate it as font-backend behavior rather than terminal semantics.
7. Maintain screenshot-based visual verification, but treat it as a category suite rather than a list of every emoji.

## Alternatives Considered
1. Continue patching individual emoji as users find defects.
   - Rejected. It does not scale and causes Unicode drift.
2. Depend entirely on DirectWrite to solve segmentation, width, and fallback.
   - Rejected. DirectWrite helps shaping/fallback, but terminal cell width and parser/render cluster boundaries are CervTerm behavior.
3. Vendor a full emoji shaping/rendering library immediately.
   - Deferred. It may be useful later, but generated Unicode tables plus existing DirectWrite/OpenType paths are a smaller next step.
4. Bundle Noto Color Emoji to solve missing flags/platform differences.
   - Deferred. Bundling fonts has licensing, size, and platform packaging implications. It can be revisited if native fonts are insufficient.

## Tradeoffs
- Positive: Unicode behavior becomes reproducible, testable, and less dependent on ad-hoc patches.
- Positive: cluster width and font fallback become explicit architecture seams.
- Negative: generated tables add maintenance and a Unicode-version update workflow.
- Negative: terminal width behavior still has policy choices where terminal conventions differ.
- Negative: native font limitations remain; e.g. Segoe UI Emoji may not render national flags as colored flags on some Windows configurations.

## Positive Consequences
- Easier to add coverage for whole emoji categories.
- Better separation between terminal semantics and font-rendering compatibility.
- Safer future upgrades when Unicode adds new emoji.
- Fewer user-visible regressions from adding one-off range checks.

## Negative Consequences
- Requires generator script and pinned source provenance.
- Requires review discipline: no new manual emoji exceptions without explicit justification.
- The first implementation may still need compatibility shims for specific fonts/platforms.

## Risks
- Incorrect generated data could break width handling broadly.
- Unicode version mismatch with host fonts can produce clusters the font cannot render.
- Visual correctness still depends on installed fonts and graphics backend.
- Flags remain a known platform/font limitation unless a flag-capable emoji font is available.

## Reversal Signals
Change this recommendation if:

- a mature Go terminal text shaping/width library fully covers CervTerm needs with better maintenance economics,
- DirectWrite/CoreText/Pango integration can safely own segmentation, width, and fallback cross-platform,
- generated tables become too large or costly for the project goals,
- bundled fonts become required for product consistency.

## Impact on Behavior Model
Text rendering behavior should be modeled around clusters, not runes. The terminal state can still store cells, but renderer-facing behavior must understand that a visible glyph may span multiple codepoints and terminal columns.

## Impact on Decision Model
This is a rendering semantics decision, not a user/business policy. The main policy-like rule is architectural: new emoji behavior must be expressed through Unicode properties and cluster tests, not arbitrary one-off codepoint patches.

## Impact on Execution Model
Implementation should proceed as a controlled refactor:

1. Add generator and property package.
2. Add cluster width API and tests.
3. Replace existing handwritten checks incrementally.
4. Preserve current screenshot verification and add category snapshots.
5. Keep existing Segoe compatibility path until a better renderer/fallback architecture supersedes it.

## Follow-up Work
- Add `scripts/generate-unicode-props.go` or equivalent generator.
- Add `internal/unicodeprops` generated package with provenance comments.
- Add cluster segmenter or integrate a tested grapheme segmentation package if acceptable.
- Add tests for BMP emoji + VS16, keycaps, modifiers, ZWJ, regional indicators, combining marks, and East Asian Width.
- Add architecture fitness checks preventing new handwritten emoji exceptions.
- Revisit font bundling only if native font fallback cannot meet product expectations.
