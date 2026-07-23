# Phase 14 Slice 14.7 — Dormant Sixel Adapter Validation

Date: 2026-07-22
Base: `dcce5a2`

## Scope

Added a pure dormant `internal/sixel` leaf. It incrementally validates the accepted Sixel token subset and transfers borrowed DCS chunks into one reservation-backed sealed `termimage.CandidateTransfer`. It performs no decode, mux routing, configuration, terminal mutation, reply, or rendering work.

## Evidence

Passed:

- focused unit and race tests for `internal/sixel`
- tokenizer and adapter fuzz targets, each for at least 60 seconds
- full tagged/untagged tests and vet
- full race plus tagged focused race
- maturity, Phase 13 import, Phase 14 import, and diff checks
- independent slice/security review after classification and transparent-canvas fixes

Coverage includes every valid-frame split, one-byte fuzz fragmentation, exact grammar boundaries, transparent declared canvases, truncation, malformed/HLS forms, cancellation/overflow classification, exact and +1 256 KiB frames, 16 KiB chunks, 4,096 chunks, pane pending limits, expiry/reset/close, discard recovery, high-half IDs, sealed payload identity, and exact reservation rollback.

## Performance

Ten one-second single-CPU runs:

- `BenchmarkSixelTokenizer256KiB`: 0 B/op and 0 allocs/op in every run.
- `BenchmarkSixelAdapterSeal256KiB`: 264,984 B/op and 53 allocs/op in every run. This is one bounded retained frame plus chunk/lease ownership; there is no second full-frame adapter copy.

The package is dormant and introduces no disabled-path allocation or wake.
