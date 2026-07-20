# Phase 2 Transactional Teal Publication Validation

## Scope

Candidate-only publication of already-generated, composed, and caller-validated Teal outputs. Public configuration loading and bundle activation remain unchanged.

## Verified contracts

- Absent outputs are created; unmarked legacy outputs are adopted only when staged bytes match exactly.
- Existing ownership markers must be valid and name the same canonical Teal source.
- Foreign/unowned differing, malformed-marker, nonregular, hardlinked, duplicate-destination, explicit-module-collision, and commit-time changed paths reject before overwrite.
- Every output and marker is prepared beside its destination before mutation; stale CervTerm temp files older than 24 hours are removed while fresh files remain.
- Identity, complete mode, and bytes are rechecked globally and immediately around marker/output replacement.
- Ownership publishes before output so interruption leaves an absent, previously-owned, or byte-identical adoptable path recoverable.
- Unix replacement and rollback deletion sync the parent directory; Windows uses write-through atomic replacement.
- Marker/output and multi-output injected failures restore prior bytes/modes or remove newly-created files in reverse order, with temp cleanup.
- The explicit rollback contract does not promise ACL/xattr/timestamp/ownership/hardlink identity preservation; pre-existing hardlinks are rejected.

## Evidence

```text
go test ./... -count=1                                      PASS
go test -tags headless ./... -count=1                       PASS
go test -tags glfw ./... -count=1                           PASS
go test -race ./internal/config -count=1                    PASS
go vet ./internal/config                                    PASS
GOOS=windows GOARCH=amd64 go test ./internal/config -run ^$ PASS
```

Independent review findings for directory durability, commit-time identity checks, hardlinks, duplicate targets, stale temps, and marker ordering were addressed before merge.

## Activation gate

No active loader invokes publication yet. It becomes a preparation step only when the atomic candidate bundle owns configuration, Lua runtime, graph, bindings/events, provenance, and generated-output transfer together.
