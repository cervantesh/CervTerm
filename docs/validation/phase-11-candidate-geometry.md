# Phase 11.5 Validation — Candidate Geometry and Invalidation

## Contract

- Candidate geometry derives from the exact rendered preedit caret after grapheme and BiDi visual mapping. Terminal callers supply the focused pane’s pixel origin and pane-local zoom metrics; search/modal callers supply their retained input-chrome anchor.
- Framebuffer caret rectangles project to native client coordinates with independently checked `windowWidth/framebufferWidth` and `windowHeight/framebufferHeight` ratios. Content scale is not used as a coordinate substitute.
- Projection rejects zero/negative dimensions, NaN/Inf, overflowing edges and empty clamped results. Partially out-of-bounds rectangles clamp to the native client area with half-open extents.
- One projection-local publisher caches the last successful rectangle, suppresses unchanged calls, retries failed publication only on later externally requested frames, supports explicit cache invalidation, and clears visibility exactly once when composition ends. Persistent sink failures never notify or self-schedule. Replacing a visible sink must clear the old sink successfully first.
- Composition mutations, cursor output, pane focus/layout/resize, zoom/font metrics, framebuffer and native window resize, DPI/content-scale callbacks, viewport movement and modal/search caret changes converge on the on-demand redraw path. Scale/window/framebuffer/layout/zoom/viewport transitions explicitly invalidate the cache; otherwise geometry is recomputed and published only when the projected rectangle changed. Failed clear is retried on the already-requested inactive frame, and an active composition that produces no valid presenter in a frame clears any stale candidate rectangle.
- The sink remains fakeable and unset in production. No IMM/WndProc/native candidate API, timer cadence, configuration, persistence, mux/core mutation or PTY side effect is introduced.

## Evidence

Tests cover asymmetric X/Y ratios, floor/ceil edge projection, partial clamping, zero/NaN/Inf/outside rejection, RTL visual caret placement, inactive presentation, changed-only publication, failed-publication retry, explicit invalidation, idempotent hide, old-sink replacement safety, composition-end clear retry/redraw, and active-without-presenter stale clearing.

## Required gates

```text
go test ./... -count=1
go test -tags glfw ./... -count=1
go vet -unsafeptr=false ./...
go vet -unsafeptr=false -tags glfw ./...
go test -race ./... -count=1
go run ./scripts/check-maturity-gates.go
git diff --check
```

## Rollback

Remove candidate geometry projection/publication files, the App publisher field, composition-end clear hook and draw-site publication calls. Preedit rendering and lifecycle cancellation remain unchanged; no native or persisted state requires migration.
