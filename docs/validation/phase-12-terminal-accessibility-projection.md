# Phase 12.2 — Terminal accessibility projection

## Scope

Adds a pure terminal viewport projector in `internal/accessibility` plus a dormant GLFW adapter that converts detached render snapshots into the pure DTO. No provider, event publication or native API is introduced.

## Contracts

- Only the detached visible `Rows × Cols` cell array enters the projector. History depth, hidden primary/alternate state, title, CWD, hyperlinks/URIs, semantic-zone payload and process metadata are structurally absent from the DTO.
- Logical UAX #29 grapheme order defines document text, caret and half-open selection offsets. Explicit wrap flags suppress logical newlines after soft-wrapped rows.
- Core cells are detached into base-plus-combining text. Rune-zero padding and wide continuations do not emit text; wide lead spans and multi-cell graphemes retain their logical cell interval.
- BiDi input is a checked visual-to-logical permutation. Text remains logical while each grapheme rectangle unions its contributing visual cells.
- Geometry uses the supplied pane pixel origin, adopted pane-local cell metrics and half-open pane clip. Fully clipped graphemes retain a valid zero rectangle.
- Cursor on a continuation maps to its lead grapheme. A scrolled viewport (`DisplayOffset > 0`) exposes visible historical text but no live caret. Inclusive cell selections normalize and become half-open grapheme spans.
- Rows, cells, cell spans, UTF-8, explicit blank/continuation/barrier structure, dimensions, permutations and finite geometry are validated before publication. Input is capped at 262,144 visible cells before the adapter allocates its detached DTO. Projection truncation feeds the immutable document truncation flag.

## Evidence

Deterministic JSON goldens cover Unicode/combining/CJK, hard versus soft wraps, continuation cursor, reverse multiline selection, logical mixed-BiDi text with visual rectangles, and alternate/scrolled visible-only privacy. Focused tests cover wide-cell clipping, malformed dimensions/permutations/UTF-8/newlines/spans/continuations, detached render values, hyperlink URI/title/CWD exclusion, combining/wide/BiDi conversion and snapshot-shape rejection.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
