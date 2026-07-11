# WiX MSI template

`CervTerm.wxs` is a WiX v4 starter template kept for possible future installer work. It is intentionally deferred: current beta releases use unsigned portable zips with SHA256 checksums and GitHub provenance attestations, not MSI publishing.

Build sketch after a release directory exists:

```powershell
winget install WiXToolset.WiXToolset
wix build packaging/wix/CervTerm.wxs `
  -d SourceDir=dist/cervterm-v0.2.0-beta.1-windows `
  -d ProductVersion=0.2.0 `
  -o dist/CervTerm-0.2.0.msi
```

The template currently installs:

- `cervterm.exe`
- `cervterm.lua`
- `README.md`
- `CHANGELOG.md`
- a Start Menu shortcut

If MSI publishing is revisited later, decide:

1. Whether installs should be per-machine or per-user.
2. Whether `cervterm.lua` should be installed beside the exe or generated in the user config directory on first run.
3. Whether the MSI should modify PATH or rely on shortcuts/winget aliases.
4. Whether paid Authenticode signing is available for both `cervterm.exe` and the `.msi`.
5. Whether upgrades should preserve user config and logs.
