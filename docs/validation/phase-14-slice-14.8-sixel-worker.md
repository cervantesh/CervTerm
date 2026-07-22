# Phase 14 Slice 14.8 — Dormant Sixel Worker Validation

Date: 2026-07-22
Base: `307305b`

Added the dormant two-pass Sixel worker over sealed adapter transfers. It reserves decode scratch, revalidates grammar/declaration metadata, enforces 4,096 dimensions, 16,777,216 pixels, 64 MiB RGBA, 4,194,304 operations, out-of-canvas drawing and checked 1..256 cell spans. Rendering uses a detached 256-entry palette and returns a write-sealed immutable candidate.

Passed focused/full/tagged tests, vet, full race and tagged focused race, maturity/import/diff gates, independent review, and a 60-second worker fuzz run. Goldens cover palette rendering, transparent canvas, non-divisible span rounding, dimension/span/drawing/operation bombs, direct grammar bypass, cancellation, metadata mismatch and exact ownership rollback.

`BenchmarkSixelDecodeWorker256x64` ran ten one-second single-CPU samples: 125,726–154,000 ns/op, 71,088 B/op and 29 allocs/op in every run. The fixed allocation includes the 65,536-byte immutable candidate plus bounded transfer/scratch ownership; the dormant slice changes no disabled production path.
