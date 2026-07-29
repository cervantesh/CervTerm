# ADR: Establish explicit process and mux owner capabilities

## Status

Accepted

## Date

2026-07-23

## Relationship

Extends ADR-0004. ADR-0004 remains authoritative for in-process topology/identity; this ADR adds executable ownership and wrong-owner semantics.

## Context

Executable bootstrap, mux mutation and native-window projection currently rely on concrete/global reach. Wrong-owner calls can silently mutate state and obstruct multi-window correctness.

## Decision

Introduce one process owner that creates and closes shared mux/config/runtime resources. `NewOwner` locks the creating goroutine to its native OS thread and records the exact identity through the shared import-light `internal/ownerthread` leaf (`GetCurrentThreadId` on Windows, `gettid` on Linux, `thread_selfid` on Darwin). The frontend controller independently captures that same native identity at loop start and pairs it with a monotonic loop epoch; every native path rechecks both before calling GLFW or OpenGL.

Every accepted process or window mutation receives a generation-bound, one-dispatch nonce scope. Process methods can reach only private scope-required mux sinks. `WindowOwner` additionally binds exact non-reusable `WindowIdentity{ID, Incarnation}` and mints its scope only after detached native-thread, active-loop-epoch, and current-context attestation. Scope copies expire when the dispatch leaves.

Frontend child projections retain a typed `projectionMessageRouter` made only of explicit function message ports. They do not retain the concrete `windowController`, process service storage, `Owner`, or `Mux`. Process commands and window capability minting remain separate narrow interfaces owned by the root controller.

Owner-thread work uses prepare/commit/abort preflight, idempotent close and reverse acquisition unwind. Fallible pane/store/session close validation completes before registry, topology, terminal sidecar, or accounting detach. `termimage.PreparedStoreState` transitions reacquire a fresh StoreOwner scope; close preparation holds an atomic reservation while Commit/Abort reacquire a fresh scope. Private sinks revalidate owner, generation, nonce, activity, thread, store, epoch, and publication state before mutation. Wrong-owner, wrong-thread, wrong-origin/incarnation, stale-generation, expired-scope, busy-owner and closed-owner requests fail without mutation. No capability exposes usable `*Mux` mutation authority through functions, methods, variables, fields, closures, interfaces, aliases, generic containers, or composite literals.

## Consequences

The owner fast path adds native-thread and atomic nonce checks. Closure therefore requires source/binary-bound, ten-sample-per-side physical ABBA evidence with no allocation increase and no median regression above 3%, in addition to recursive typed actual-source family guards, permanent locator/bypass fixtures, exact private sink/body inventories, complete zero-mutation fingerprints, and full validation. The comparison baseline is immutable pre-slice production `9fe0bd0287ed402fe90b58a5fba7d6413897c1af` plus only the hashed benchmark harness; W `27070f32395be7803b6fc11388a08a5c32d3d3a1` contains owner production changes and is rejected as a benchmark baseline. A failing or unrecorded evidence family keeps L3-02 open even when implementation tests pass.

## Rejected alternatives

A package global, passing `*Mux`/`*App` everywhere, goroutine-ID ownership, pointer possession without an ephemeral dispatch scope, a window capability without current-context attestation, or silently redirecting wrong-owner calls.

## Rollback

Revert consumers, wiring and additive capability in reverse order while retaining characterization tests.
