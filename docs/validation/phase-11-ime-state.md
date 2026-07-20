# Phase 11.1 Validation — Pure IME Composition State

## Contract

- `internal/ime` is pure and toolkit/native/PTY/mux/core/render independent.
- Zero-value `Controller` owns strict `Inactive -> Started -> Updated* -> Committed/Cancelled -> Inactive` transitions with nonzero stable target identity, generation and monotonic revision.
- Failed start/update/commit/cancel operations are atomic. Generation/revision exhaustion fails closed rather than reusing stale tokens.
- Snapshots and commit deliveries detach all slices; caller-owned UTF-16/attribute buffers are never retained.
- Preedit is bounded before decoding/allocation to 16 Ki UTF-16 units, then to 16 KiB UTF-8 and 4096 runes. Commit is bounded before decoding to 64 Ki UTF-16 units and after decoding to 64 KiB/runes.
- UTF-16 decoding rejects unpaired surrogates and caret offsets inside pairs. Composition attributes must be known, exactly one byte per code unit, consistent across surrogate pairs, and contain at most one contiguous target run.
- Cursor and target spans normalize to Unicode extended-grapheme boundaries. The focused `github.com/clipperhouse/uax29/v2/graphemes` dependency is Unicode 17/UAX #29 conformance-tested and is used only by the pure IME package; terminal-wide cluster behavior is not changed in this slice.
- Cancellation reasons are explicit and retained only as last-transition metadata. No frontend callback or native API is wired.

## Focused evidence

`internal/ime` tests cover lifecycle, stable/stale generations, target validation, atomic failures, detached ownership, counter exhaustion, predecode/exact bounds, malformed UTF-16/cursor/attributes, split-surrogate attributes, disjoint target runs, and cluster boundaries for Hangul Jamo, CRLF, Indic conjuncts, ZWJ emoji and regional-indicator flags.

## Required gates

```text
go test ./internal/ime -count=1
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

## Executed evidence

On Windows on 2026-07-20 from local `dev` base `13f94e3`, focused IME tests, full default/GLFW suites, default/GLFW vet, race, maturity gates and diff check passed. Independent review findings on grapheme conformance, disjoint attributes, predecode bounds, counters, surrogate attributes, API units and cancellation precedence were repaired; final re-review found no blocker.

## Rollback

Remove `internal/ime` and its single grapheme dependency. No production caller, configuration, native hook or persisted state exists in Slice 11.1.
