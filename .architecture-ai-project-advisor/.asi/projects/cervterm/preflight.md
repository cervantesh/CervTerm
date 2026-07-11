# Architecture Preflight Check

## Proposed Feature
Replace ad-hoc emoji/rune handling with generated Unicode property tables, grapheme/emoji cluster segmentation, cluster-level width calculation, and per-cluster emoji font fallback.

## Context Baseline
- Active project: CervTerm.
- Relevant ADRs: ADR-0001 Use generated Unicode data and cluster-based emoji rendering.
- Relevant guardrails: avoid per-emoji hardcoding; keep terminal semantics separate from font-specific rendering compatibility.
- Assumptions:
  - Windows/Segoe UI Emoji remains primary test target for current work.
  - Generated Unicode tables may be committed if deterministic and documented.
  - Existing Segoe COLRv0 compatibility can remain as a font-backend shim.

## Layer Impact
- Behavior model:
  - Terminal text rendering behavior shifts from rune-level decisions to cluster-level decisions.
  - Display width is a property of a cluster, not only a single rune.
- Decision model:
  - Architecture rule: Unicode properties and cluster rules decide emoji behavior; individual user-reported emoji patches are discouraged.
  - No business/security policy impact.
- Execution model:
  - Add generation step for Unicode data.
  - Add/replace tests and update rendering path incrementally.
- Infrastructure:
  - No external service required.
  - Optional build/generation script and committed generated Go files.
- AI/tooling:
  - No LLM runtime behavior; this is deterministic rendering logic.

## Required Commands / Handlers
- Not a command-handler/domain feature.
- Required developer commands/scripts:
  - `go run ./scripts/generate-unicode-props.go` or equivalent.
  - `go test ./...`.
  - `go test -tags glfw ./internal/applog ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1`.
  - screenshot capture for visual verification.

## Required Policies
- No runtime authorization policies.
- Architecture policy: raw Unicode emoji ranges should not proliferate outside generated data or documented compatibility shims.

## Required State Transitions / Invariants
- Invariants:
  - Combining marks, variation selectors, ZWJ, keycap combining mark, and emoji modifiers do not occupy their own display cells.
  - Emoji presentation clusters occupy two terminal cells unless explicitly classified otherwise.
  - Regional indicator pairs occupy two cells as one flag cluster.
  - Font fallback is selected for the whole cluster.
  - Terminal core must not depend on Windows-specific font names.

## Side Effects and Reliability
- Outbox needed? No.
- Worker/job needed? No.
- Idempotency needed? Generation should be deterministic; rerunning generator should produce no diff if Unicode version unchanged.
- Retry/compensation needed? No.

## AI Safety
- LLM role: assist with implementation only.
- Forbidden actions: no hidden network downloads during normal build/test; no bundling font assets without license review.
- Human approval: required before bundling large fonts or changing Unicode version policy.
- Audit events: not applicable.

## Tests / Fitness Functions
- Unit:
  - Unicode property lookup tests for representative codepoints.
  - Cluster segmentation tests for `é`, `❤️`, `✍️`, `👩‍💻`, `🧑🏽‍🚀`, `1️⃣`, `🇦🇷`.
  - Width tests for text, CJK, BMP emoji, astral emoji, modifiers, regional indicators.
- Integration:
  - Font backend tests for per-cluster Segoe fallback and color glyph rasterization.
  - GLFW cluster collector tests until replaced by a proper segmenter.
- Policy:
  - Static/review check for new handwritten emoji ranges outside generated package.
- AI evaluation:
  - Not applicable.
- Architecture fitness:
  - AF-001 through AF-008 from `fitness.md`.

## ADR Required?
Yes. Created ADR-0001 because this changes the long-term Unicode rendering architecture and avoids a pattern of one-off emoji fixes.

## Recommendation
Proceed with constraints.

Recommended implementation slices:

1. **Unicode data generator and package**
   - Add generated `internal/unicodeprops` tables with provenance.
   - Do not yet change rendering behavior except tests for lookup correctness.

2. **Cluster segmentation API**
   - Introduce a renderer-independent cluster model, e.g. `internal/unicodecluster`.
   - Support combining marks, VS15/VS16, ZWJ, keycaps, modifiers, regional indicator pairs.

3. **Cluster width API**
   - Move width from rune-only to cluster-aware API.
   - Keep `RuneWidth` for compatibility but make render paths use cluster width.

4. **Replace handwritten checks**
   - Update `internal/core/width.go`, `internal/frontend/glfwgl/cluster.go`, and `internal/fontglyph/backend.go` to use generated properties.

5. **Verification hardening**
   - Keep the 60+ emoji screenshot.
   - Add category tests and static checks to prevent regression into per-emoji patches.

Do not bundle a full emoji font in this slice. Treat that as a separate ADR if native fonts remain insufficient.
