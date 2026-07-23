# ADR: Bound terminal image lifetime and resources

## Status
Superseded

## Date
2026-07-16

## Superseded By
ADR 0014 — Bound terminal image lifetime, transports, and resources. ADR 0014 accepts the concrete ownership, budget, lifecycle, parser, reply, decode, snapshot, GPU-cache, qualification, and rollback contracts required by this proposal.

## Context
Kitty APC, Sixel DCS, and iTerm OSC image protocols can carry large compressed payloads and create placements that interact with scrolling, erasure, alternate screen, resize, pane clipping, damage, and GPU textures.

## Decision to Make
Choose the protocol-neutral image store, placement/lifetime rules, replies, and security budgets before Phases 13–14.

## Candidate Direction
Keep `core.Cell` text-only. Store pane-owned images and placements adjacent to screen state, expose renderer-neutral snapshot references, stream-decode under encoded/decoded byte, pixel, count, time, and per-pane/global limits, and cache bounded GPU textures.

## Constraints
- Malformed or compressed input cannot allocate or block without bound.
- Placements have deterministic scroll/erase/resize/alternate-screen/delete behavior.
- Every pane clips its placements.
- Text-only workloads show negligible regression.
- Animation is deferred unless separately accepted.

## Evidence Required for Acceptance
Resource budget table, placement state machine, cache eviction policy, parser recovery behavior, fuzz plan, and protocol reply policy.

## Reversal Signals
A shared model cannot preserve protocol semantics, or secure decoding requires a separately approved dependency/process boundary.
