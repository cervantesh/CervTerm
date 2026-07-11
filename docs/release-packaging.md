# CervTerm Release Packaging Notes

## Current artifacts

- Windows GLFW beta zip: built by CI as `cervterm-windows-zip` with a `goversioninfo`-generated embedded Windows resource, `cervterm.exe`, generated `cervterm.lua`, README, CHANGELOG, docs, and packaging metadata/assets.
- Linux headless beta zip: built by CI as `cervterm-linux-headless-zip` with the headless binary, generated `cervterm.lua`, README, CHANGELOG, and docs to keep Unix PTY compilation covered before a Linux GUI frontend is packaged.

## Windows metadata scaffolding

The repository carries packaging metadata under `packaging/windows/` and source artwork under `packaging/assets/`:

- `cervterm.manifest`: asInvoker, per-monitor DPI awareness, and long-path awareness.
- `versioninfo.json`: product/file metadata consumed by `goversioninfo` for `.syso` generation.
- `cervterm.rc`: Windows resource script retained as a readable/reference equivalent for icon, manifest, and version metadata.
- `packaging/assets/cervterm.svg`: source icon artwork for future PNG/ICO regeneration.
- `cervterm.ico`: committed Windows icon generated from the CervTerm icon concept and referenced by `versioninfo.json`.

## Next packaging steps

1. Generate `cmd/cervterm/resource_windows_amd64.syso` with `goversioninfo` so `cervterm.ico`, `cervterm.manifest`, and version metadata are embedded in `cervterm.exe`:

   ```powershell
   go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
   powershell -NoProfile -ExecutionPolicy Bypass -File scripts/generate-windows-resource.ps1 -Arch amd64
   # or: go generate ./cmd/cervterm
   ```

2. Publish tagged releases with `.github/workflows/release.yml` by pushing a `v*` tag; the workflow builds Windows and Linux headless zips and uploads them to the GitHub release.
3. Publish a portable winget package after a release is published by filling in the templates under `packaging/winget/` with the tag version and Windows zip SHA256.
4. Promote the WiX v4 template under `packaging/wix/` into CI when per-machine/per-user policy, config installation behavior, PATH/shortcut choices, and MSI signing policy are finalized.
5. Configure Authenticode signing secrets when a code-signing certificate exists. The release workflow already calls `scripts/sign-windows-exe.ps1` before zipping when `WINDOWS_CODESIGN_PFX_BASE64` and `WINDOWS_CODESIGN_PASSWORD` are configured; otherwise it skips signing and still publishes SHA256 checksums plus GitHub build provenance attestations.

## Tagged release workflow

Push a tag such as `v0.2.0-beta.1` to run the release workflow. It generates release notes from GitHub, uploads `cervterm-<tag>-windows.zip`, uploads `cervterm-<tag>-linux-headless-amd64.zip`, publishes `SHA256SUMS.txt`, and requests GitHub artifact provenance attestations for those assets. Windows release builds run `goversioninfo` before `go build` so icon, manifest, and version metadata are embedded when the tool succeeds.

Packaged binaries support diagnostics logging with `--log-file <path>` or `CERVTERM_LOG_FILE`; panic recovery writes a stack trace through the standard diagnostics logger before process exit. Use `--log-file -` for stderr-only logging in scripted smoke tests.

For the release trust model, checksum verification, provenance, and unsigned beta status, see [`docs/release-trust.md`](release-trust.md). For user diagnostics and bug-report capture workflow, see [`docs/troubleshooting.md`](troubleshooting.md).

## Release preflight

Run the local preflight before cutting or publishing a release:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release-preflight.ps1 -Version <tag> -OutDir dist
```

By default, Authenticode signing and MSI/WiX publishing are treated as intentionally deferred because the free beta release path is the portable zip plus SHA256 checksums and GitHub attestations. The Windows release workflow runs this non-strict preflight after creating the zip. Add `-RequireSigning`, `-RequireVttest`, and/or `-RequireWix` only when preparing a stricter local gate that must fail until those optional dependencies are present.

## Deferred Authenticode signing

Authenticode signing is not required for the free beta release path. If paid signing is added later, add these repository secrets:

- `WINDOWS_CODESIGN_PFX_BASE64`: base64-encoded `.pfx` certificate.
- `WINDOWS_CODESIGN_PASSWORD`: password for the `.pfx`.

The workflow decodes the PFX into the runner temp directory, runs `signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256`, verifies the signature, and removes the temporary PFX.

## Winget portable package

`packaging/winget/` contains Windows Package Manager templates for the release zip. After a tagged GitHub release is published, replace `<version>` and `<sha256>`, validate with `winget validate`, and submit the generated manifests to `winget-pkgs`. This gives CervTerm an installable package path without committing to MSI/WiX yet.

## MSI/WiX installer template

`packaging/wix/CervTerm.wxs` is a WiX v4 starter template for a possible future full MSI installer. It installs the executable, generated default config, README, CHANGELOG, and a Start Menu shortcut. It is intentionally not enabled in CI; portable zip releases are the current distribution target.

## Verification commands

```sh
go test ./...
go test -tags glfw ./internal/fontglyph ./internal/frontend/glfwgl ./cmd/cervterm -count=1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/generate-windows-resource.ps1 -Arch amd64
go build -tags glfw -o dist/cervterm.exe ./cmd/cervterm
./dist/cervterm.exe --build-info
./dist/cervterm.exe --print-default-config > dist/cervterm.lua
./dist/cervterm.exe --log-file ./dist/cervterm.log
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/release-preflight.ps1 -Version <tag> -OutDir dist
```
