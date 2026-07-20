# Phase 11.2 Validation — Stable Text Targets and Exactly-Once Router

## Contract

- Retained modal and search controllers expose monotonic activation-instance IDs distinct from mutable content revisions.
- Whole-text append validates one activation once, rejects stale/control/overflow input atomically, mutates the query once, refilters once and redraws once.
- Every GLFW character enters one router. Binding/native-echo suppression executes before modal, search or terminal routing.
- Existing binding suppression still consumes exactly the next produced character and is cleared before any later press/repeat if GLFW emitted no character. The dormant IME echo mode is bounded to the full validated commit sequence and 100 ms, and clears on completion, mismatch, expiry, non-echo key input or teardown.
- Terminal text captures one focused pane plus projection-local activation and performs one pane-addressed UTF-8 mux write. A newer capture, intervening focus transition (including away-and-back), missing pane or invalid target fails without fallback.
- Search/modal routes require the same opening pane and activation even when content revisions change. Replace/close/reopen invalidates the old target.
- Existing single-character Quick Select semantics remain intact. Consuming a label character succeeds independently of the selected action; action failure remains retained and visible in the modal, preventing retry ambiguity. Multi-rune modal text updates the retained query atomically and does not partially route later runes to another destination.
- Empty, invalid UTF-8, control-bearing or over-bound text reaches no target. Legacy GLFW character callbacks remain authoritative; no native host or preedit is wired.

## Focused evidence

Tests cover modal/search activation stability and exhaustion, stale/atomic whole-text behavior, one-call multi-rune pane writes, newer-capture/focus-cycle/missing-pane invalidation, modal revision tolerance/replacement rejection, search reopen rejection, invalid input, Quick Select action-error consumption, pre-routing binding/IME echo suppression, stale-binding/full-sequence mismatch/deadline/clear paths, and existing key/search/modal compatibility.

## Required gates

```text
go test ./internal/modal -count=1
go test -tags glfw ./internal/frontend/glfwgl -run 'TestCommittedText|TestCharSuppression|TestBindingSuppression|TestIMEEcho|TestSearchActivation|TestCoordinator|TestKeyPipeline' -count=1
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

## Rollback

Restore the prior one-bit binding suppression and direct GLFW character callback, then remove activation IDs/whole-text helpers. No native adapter, configuration or persisted state exists in Slice 11.2.
