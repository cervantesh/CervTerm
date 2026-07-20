# Phase 12.1 — Pure bounded accessibility model

## Scope

Adds `internal/accessibility`, a platform-neutral immutable semantic document foundation. It has no frontend, mux, PTY, GLFW, Win32 or UI Automation dependency and performs no I/O.

## Contracts

- Provider ID, caller generation and a private per-document instance make ranges stale after every replacement, including accidental same-generation reuse.
- Node identities are one-projection values. Pane/input/item nodes require nonzero activation identity; parents must precede children and IDs are unique.
- Documents are limited to 512 explicit rows, 16,384 UAX #29 graphemes, 1 MiB aggregate UTF-8 across names/text and 256 nodes. Truncation is explicit and caret/selection endpoints clamp to retained content.
- Embedded CR/LF is rejected because row and soft-wrap boundaries are explicit. Soft-wrapped rows do not create a logical newline.
- Text ranges use half-open logical grapheme offsets. Rectangle lists remain detached and omit logical newlines.
- All returned node, row and rectangle slices are detached copies. Concurrent readers cannot mutate the document.
- Invalid identity, UTF-8, bounds, ranges or exhausted generation input returns no partial document.

## Evidence

Tests cover detached Unicode snapshots, UAX #29 decomposed Hangul, combining/CJK rows, soft wraps, invalid UTF-8/newlines/identities/topology/focus/activation, finite and cardinality-checked rectangles, row/grapheme/byte/node limits, truncation/clamping, stale provider/generation/document instances, endpoint comparison, concurrent reads and static forbidden-import checks.

## Gates

```text
go test ./... -count=1
go test ./internal/accessibility -race -count=1
go vet ./internal/accessibility
go run ./scripts/check-maturity-gates.go
git diff --check
```
