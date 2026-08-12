# macOS packaging re-verification evidence — 2026-08-12

Raw evidence backing PR #232's claims, captured after the Codex
adversarial-review and Opus-adjudication round on this branch (see
`docs/release-packaging.md`'s "macOS app bundle" section for the narrative).

**Environment:** macOS 15.7.7 (24G720), Intel x86_64, Go 1.26.5, commit `9fa1ca1`.

| File | What it shows |
|---|---|
| [`00-environment-and-build.txt`](00-environment-and-build.txt) | `go build ./...` and `go vet` both clean on this branch |
| [`01-package-build.txt`](01-package-build.txt) | `package-macos.go` with default ad-hoc signing: bundle assembly succeeds, `Info.plist` is valid (`plutil -lint` OK) with the fixed version normalization (`v0.9.1-macos-evidence` → `0.9.1` in the plist, full string preserved in `--build-info`), the executable launches correctly from inside the bundle — but signing itself still fails on this specific machine with the previously-documented `com.apple.provenance` block, exactly as described in `docs/release-packaging.md`. Not a new problem, and not silently glossed over. |
| [`02-unsigned-build-and-gatekeeper.txt`](02-unsigned-build-and-gatekeeper.txt) | Same version with `-adhoc-sign=false`: full pipeline completes, 284-file zip (`unzip -t` reports no errors), and a direct Gatekeeper check on a simulated-download-quarantined copy of the bundle returns `rejected / source=no usable signature` — the expected state for an unsigned `.app`. |
