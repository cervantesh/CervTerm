# Phase 14 Slice 14.10 — Dormant iTerm Adapter

Date: 2026-07-22
Base: `1bc904a`

Added the dormant pure-leaf `internal/itermimage` OSC 1337 adapter. It accepts only `File=` with exact `inline=1`, positive `size`, optional cell-only `width` xor `height` in 1..256, and absent/exact `preserveAspectRatio=1`. Unknown, duplicate, external-I/O, name, stretch, pixel, percentage, auto, non-canonical boolean, whitespace and malformed strict-padded-base64 forms are rejected. Only encoded base64 is retained through reservation-backed `CandidateTransfer`; chunk, pane/process, pending-transfer, ID, timeout, cancellation, overflow, close/reset and discard recovery paths release ownership exactly.

The package is dormant and does not modify VT, mux, workers, configuration, or runtime activation.

Validation passed:

- focused package tests and race tests;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import and maturity gates, and diff check;
- all three fuzz targets for 60 seconds each (scanner fragmentation, strict base64, lifecycle);
- independent review, with its exact lexical `inline=1`/`preserveAspectRatio=1` finding fixed and re-reviewed.

Ten single-CPU benchmark samples showed:

- 256 KiB scanner: 1.006–1.171 ms/op, 0 B/op, 0 allocs/op;
- 256 KiB adapter seal: 1.081–1.226 ms/op, 266,272 B/op, 57 allocs/op.

The adapter allocation is the bounded retained transfer plus lifecycle bookkeeping; no second aggregate payload buffer is created.
