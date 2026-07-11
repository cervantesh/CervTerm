# Windows Package Manager templates

These files are templates for a future `winget-pkgs` submission for the portable Windows zip release.

Release flow:

1. Push a tag such as `v0.2.0-beta.1` and wait for `.github/workflows/release.yml` to publish assets.
2. Download `SHA256SUMS.txt` from the GitHub release.
3. Copy this directory to the `winget-pkgs` tree under `manifests/t/T50Systems/CervTerm/<version>/`.
4. Replace `<version>` with the tag without the leading `v` (for example `0.2.0-beta.1`).
5. Replace `<sha256>` in `T50Systems.CervTerm.installer.yaml` with the Windows zip hash from `SHA256SUMS.txt`.
6. Validate with `winget validate` before submitting.

The template uses:

- `InstallerType: zip`
- `NestedInstallerType: portable`
- `PortableCommandAlias: cervterm`

A full MSI/WiX installer can supersede this later if CervTerm needs Start Menu shortcuts, install-location control, file associations, auto-update hooks, or per-machine installation.
