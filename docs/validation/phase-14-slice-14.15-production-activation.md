# Phase 14 Slice 14.15 — Transactional Production Activation

Date: 2026-07-22
Base: `bd7de9b`

Activated the independently configured Kitty, Sixel and iTerm bounded subsets through one `imagesEnabled` predicate. Any enabled combination receives one shared limits/process budget/scheduler, one pane-local store, only its enabled adapters, and one distinct texture cache per OpenGL context. With all three disabled, image limits, budget, stores, scheduler, adapters, pending maps, caches, deadlines, retry/draw work and wake sources remain literal nil/absent.

Initial, child and restored projections retain the existing prepared-resource transaction. Renderer/capability/cache/mux/bind/attach/commit failures close provisional resources with the owning context current. Restore publication now aborts the later-acquired mux candidate before unbinding and closing earlier native bundles/caches, preserving strict reverse-acquisition rollback. Pending restart-scoped graphics changes are replaced with the effective graphics configuration when deriving child/restored projections, so they cannot activate or disable protocols before restart.

Validation passed:

- focused activation/mux/restore tests and focused race;
- full tagged/untagged tests and vet;
- full race plus tagged focused race;
- Phase 13/14 import, maturity and diff gates;
- adversarial slice and lifecycle/GL-ownership reviews. The restore rollback-order violation and support-matrix heading-anchor drift were fixed and re-reviewed with Decisions/Cross-slice/Research all OK.

Coverage includes all eight protocol masks, exact adapter/pending-map presence, Sixel-only/iTerm-only renderer and factory failures, initial commit rollback, runtime child prepare/bind/attach/focus seams, restored prepare/bind/commit rollback, per-context cache identity and teardown, effective-config inheritance across pending reload, shared limits and literal-nil all-disabled state.

A standalone ten-sample all-disabled frame run measured median 7.721 ns/op, 0 B/op and 0 allocs/op. A contemporaneous clean-base `bd7de9b` run measured 7.557 ns/op; the +2.17% paired difference remains inside the 3% gate with no allocation or wake regression.

The protocols remain experimental, default-off and restart-scoped. Manual Windows/OpenGL qualification and final support/drift documentation are deferred to Slice 14.16; no broader conformance claim is made here.
