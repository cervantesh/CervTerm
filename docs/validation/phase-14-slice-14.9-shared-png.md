# Phase 14 Slice 14.9 — Shared Bounded PNG Codec

Date: 2026-07-22
Base: `cb9dc24`

Extracted Kitty PNG decoding into protocol-neutral `termimage.DecodePNG`. The codec uses fresh-reader two-pass validation, checked RGBA/scratch reservations, context-aware readers, exact EOF, immutable candidates and exact rollback. Kitty now delegates through its existing strict base64 reader and retains existing error/reply behavior; later iTerm code can reuse the codec without importing Kitty.

Focused and full tagged/untagged tests, vet, full race, tagged focused race, maturity/import and diff gates passed. Shared codec tests cover exact RGBA, two reader opens, trailing data, cancellation identity, factory failure and zero ownership leakage. Independent review passed after adding cooperative context readers around both PNG passes.
