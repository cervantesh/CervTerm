# Architecture Guardrails

Project: CervTerm  
Updated: 2026-07-10

## Unicode / Emoji Rendering Guardrails

- [ ] Do not add one-off emoji codepoint patches unless they are documented as temporary compatibility shims with a failing regression test.
- [ ] Unicode emoji behavior must be derived from generated Unicode property tables or a vetted Unicode library.
- [ ] Grapheme/emoji cluster segmentation must happen before font fallback and glyph rasterization.
- [ ] Display width decisions for emoji presentation sequences must be made at cluster level, not solely at single-rune level.
- [ ] Font fallback must select an emoji font for the whole emoji cluster; do not mix monospace and emoji fonts inside one cluster.
- [ ] Font-specific compatibility hacks, such as Segoe UI Emoji COLRv0 preference, must stay inside `internal/fontglyph` and not leak into terminal core semantics.
- [ ] Generated Unicode data must include source URLs, Unicode version, and deterministic regeneration instructions.
- [ ] Unicode version upgrades must update tests and be mentioned in release notes or changelog.

## Rendering Verification Guardrails

- [ ] `go test ./...` must pass before claiming emoji-rendering completion.
- [ ] GLFW-tagged rendering/font tests must pass before claiming visual rendering completion.
- [ ] The emoji visual screenshot must cover categories, not just reported examples: faces, hands/modifiers, BMP+VS16, food, transport, symbols, ZWJ, keycaps, flags, and combining text.
- [ ] Known host-font limitations, such as national flags on Windows Segoe UI Emoji, must be documented rather than hidden as renderer failures.

## General Project Guardrails

- [ ] Keep terminal semantics in `internal/core` independent from platform font names.
- [ ] Keep platform/font compatibility in `internal/fontglyph`.
- [ ] Avoid architecture-heavy dependencies unless they replace enough handwritten Unicode logic to reduce long-term maintenance.
- [ ] Decisions with durable architecture impact require an ADR.
