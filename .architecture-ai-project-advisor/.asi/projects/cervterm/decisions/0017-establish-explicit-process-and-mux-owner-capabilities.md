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

Introduce one process owner that creates and closes shared mux/config/runtime resources. Expose a narrow, typed mux-owner capability to window/application controllers. Every mutating request carries stable window/pane identity and is revalidated by the owner. Wrong-owner, stale-generation and closed-owner calls fail without mutation. Existing entry points remain compatibility adapters until all callers migrate.

Owner-thread work uses prepare/commit/rollback, idempotent close and reverse acquisition unwind. No owner capability exposes the concrete mux as a service locator.

## Consequences

Multi-window commands become explicit and testable. Bootstrap and close paths gain an additive seam and additional fault/race tests. Preparatory Mux/App extraction cannot claim this decision implemented.

## Rejected alternatives

A package global, passing `*Mux`/`*App` everywhere, or silently redirecting wrong-owner calls.

## Rollback

Revert consumers, wiring and additive capability in reverse order while retaining characterization tests.
