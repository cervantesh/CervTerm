# Phase 14 Slice 14.11 — Dormant iTerm Worker

Date: 2026-07-22
Base: `1ad234e`

Added a dormant iTerm decode job over sealed adapter transfers and the shared bounded PNG codec. The worker applies strict standard padded base64, requires the decoded byte count to equal declared `size`, inherits single-PNG/exact-EOF and checked scratch/RGBA bounds from `termimage.DecodePNG`, validates captured cell pixel metrics, and returns a write-sealed immutable candidate plus a detached placement span.

Span projection uses checked integer products and ceiling division. Intrinsic sizing derives both axes; width-only and height-only sizing derive the other axis while preserving pixel aspect ratio. Every result must remain within 1..256 cells. The worker has no mux, config, runtime, core, or renderer activation.

Validation passed:

- focused package tests and race tests;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import, maturity and diff gates;
- `FuzzITermDecodeWorker` for 60 seconds (626,477 executions, three new interesting inputs, pass);
- independent slice review with no decision or cross-slice violation.

Tests cover strict alphabet/padding, declared-size mismatch, trailing PNG data, PNG/dimension/scratch bombs, intrinsic and one-axis aspect rounding, span overflow, cancellation, header/metadata bypass, immutable output and exact transfer/scratch/candidate rollback.

Ten one-second single-CPU samples of the end-to-end 256x64 decode job measured 306,559–448,767 ns/op, 255,236 B/op and 16,450 allocs/op. The benchmark includes standard-library PNG/color conversion and the 65,536-byte immutable candidate; this slice is dormant and adds no steady allocation, wake, or latency to disabled production paths.
