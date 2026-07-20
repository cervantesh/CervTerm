# Phase 12.4 — Accessibility semantic event scheduler

## Scope

Adds a dormant bounded semantic-diff scheduler and value-only mux/input intent classifiers. It has no native provider, callback, publication wiring, timer or continuous frame cadence.

## Contracts

- Repaint-only damage carries no semantic intent and produces no diff or event.
- Document geometry, topology, text, caret, selection and focus are diffed only when their corresponding owner-thread intent is present.
- Events contain provider/generation, kind, node identity and optional announcement kind only. Terminal bytes, notification bodies, titles, CWD and document text never enter an event.
- Repeated keys coalesce to the newest generation in deterministic first-key order. Bell and notification metadata coalesce separately.
- Pending state is cycle-local: beginning a cycle drops undrained or post-drain leftovers. At most one drain publishes per projection cycle. More than 256 distinct pending events collapses atomically to one document-invalidated event.
- Inactive clients clear pending state and produce zero publications. Close is idempotent, clears state and prevents revival; mutex containment makes close/read races safe while normal mutation remains owner-thread driven.
- Mux dirty/title/CWD/error events are non-semantic; output requests text/caret/selection diffs; geometry requests document diff; focus and topology transitions request only their semantic classes.

## Evidence

Tests cover the event truth table and deterministic ordering, payload-free bell/notification announcements, repaint suppression, text burst coalescing, one publication per cycle, overflow invalidation, disabled/enabled idle cycles, shutdown race, mux payload exclusion and input revision classification. Scheduler stats record zero disabled/idle publications and events.

## Gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go test -race ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go run ./scripts/check-maturity-gates.go
git diff --check
```
