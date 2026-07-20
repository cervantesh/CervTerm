# Getting Started with CervTerm

CervTerm is currently a beta GPU terminal emulator. It is suitable for developer testing and controlled beta use, but not yet a guaranteed daily-driver terminal for every workflow.

## Install

### Windows portable zip

1. Download `cervterm-<tag>-windows.zip` from the GitHub release.
2. Download `SHA256SUMS.txt` from the same release.
3. Verify the zip hash against `SHA256SUMS.txt`.
4. Extract the zip to a user-writable directory.
5. Run `cervterm.exe`.

For checksum, attestation, and unsigned beta details, see [`release-trust.md`](release-trust.md).

### Linux headless zip

The Linux artifact is currently headless and intended for non-GUI verification surfaces such as build info, default config generation, and VT capture workflows.

## First run diagnostics

Before filing an issue, run:

```cmd
.\cervterm.exe --doctor
```

Useful related commands:

```cmd
.\cervterm.exe --version
.\cervterm.exe --build-info
.\cervterm.exe --print-default-config
.\cervterm.exe --log-file .\cervterm.log
```

`--doctor` prints:

- CervTerm version and Go runtime.
- OS and CPU architecture.
- executable and working directory.
- config discovery result and candidate paths.
- diagnostics log path.
- selected environment hints.
- emoji font warnings when using the GLFW build.

## Configuration

CervTerm looks for config files beside the executable first, then in the user config directory.

Typical Windows candidates:

```text
<directory containing cervterm.exe>\cervterm.tl
<directory containing cervterm.exe>\cervterm.lua
%APPDATA%\cervterm\cervterm.tl
%APPDATA%\cervterm\cervterm.lua
```

Typical non-Windows candidates:

```text
<directory containing cervterm>\cervterm.tl
<directory containing cervterm>\cervterm.lua
$XDG_CONFIG_HOME/cervterm/cervterm.tl
$XDG_CONFIG_HOME/cervterm/cervterm.lua
~/.config/cervterm/cervterm.tl
~/.config/cervterm/cervterm.lua
```

Generate a default config:

```cmd
.\cervterm.exe --print-default-config > cervterm.lua
```

The generated template declares `config_version = 2`, which enables strict field/type diagnostics, includes, named environments/profiles, and typed process overrides. Existing unversioned configurations remain v1-compatible. Explicit `--environment`, `--profile`, or `--config-override` inputs require a v2 source; ambient selection variables are ignored by v1 for compatibility.

Run with an explicit config:

```cmd
.\cervterm.exe --config .\cervterm.lua
```

Select named v2 layers. Flags win over the corresponding process variables; configured defaults and an exact host-OS environment are lower-priority fallbacks:

```cmd
set CERVTERM_ENV=windows
set CERVTERM_PROFILE=work
.\cervterm.exe --config .\cervterm.lua --environment windows --profile work
```

Apply typed v2 overrides after the selected profile. Repeat the flag to apply values left-to-right; the last occurrence for a path wins. Values use JSON syntax except schema-known strings may be unquoted:

```cmd
.\cervterm.exe --config .\cervterm.lua ^
  --config-override window.opacity=0.85 ^
  --config-override scrolling.history=4000 ^
  --config-override shell.args="[\"pwsh\",\"-NoLogo\"]"
```

Unknown, unsupported, invalid, and sensitive paths such as `shell.env` are rejected. Raw override values are not written to diagnostics or provenance. The startup selection and ordered overrides are snapshotted and reused unchanged by automatic reload.

Inspect the fully resolved v2 configuration without opening GLFW, creating a window, publishing Teal output, or starting a PTY:

```cmd
.\cervterm.exe --config .\cervterm.lua --profile work --explain-config
.\cervterm.exe --config .\cervterm.lua --explain-config-field window.opacity --explain-config-field shell.env
```

Explanation output has a versioned, deterministic text format with selection, source graph, dependencies, application scopes, and low-to-high provenance. Sensitive values are shown as `<redacted>`. Explanation requires an authored v2 source; unknown field filters exit with status 2. `--doctor` uses the same read-only composed loader for v2, reports explicit v1/no-source boundaries, and exits nonzero when configuration loading fails. Pending changes and the last reload failure are reported as unavailable because diagnostic mode is a separate one-shot process.

## Logs

By default, CervTerm writes diagnostics to a user cache path such as:

```text
%LOCALAPPDATA%\cervterm\cervterm.log
```

Override the log file:

```cmd
.\cervterm.exe --log-file .\cervterm.log
```

Disable file logging and use stderr only:

```cmd
.\cervterm.exe --log-file -
```

## Capturing terminal output for bugs

For PTY/rendering bugs, capture raw terminal output:

```cmd
.\cervterm.exe --capture-vt .\issue.vt --capture-program cmd.exe --capture-arg /c --capture-arg "echo cervterm-capture"
```

Attach the `.vt` file, diagnostics log, `--doctor` output, and a screenshot to the issue when possible.

## Experimental Windows IME opt-in

Native Windows preedit/candidate integration is available for controlled testing but remains disabled by default because the full Japanese/Chinese/Korean qualification matrix is incomplete. Enable it only in a v2 configuration and restart CervTerm:

```lua
return {
  config_version = 2,
  ime = { enabled = true },
}
```

If installation is unavailable or fails, CervTerm keeps the existing GLFW text-input path and displays a bounded fallback notice. Run `cervterm.exe --doctor --config <path>` to confirm configured intent and platform eligibility. See [`manual-verification.md`](manual-verification.md) before making a support claim.

## Experimental Windows accessibility opt-in

Windows UI Automation exposure is experimental, visible-only, restart-scoped, and disabled by default. Enable it only in a v2 configuration and restart CervTerm:

```lua
return {
  config_version = 2,
  accessibility = { enabled = true, scope = "visible" },
}
```

Only the active workspace/window, active tab, visible pane viewports, active modal/search UI, cursor, selection, and IME preedit are published. Scrollback outside the viewport, inactive tabs/workspaces, OSC 8 URIs, shell commands, process arguments, environment values, native handles, and provider tokens are not exposed. If native registration or a later publication fails, CervTerm disables accessibility for that window, keeps terminal input/rendering alive, and displays a bounded fallback notice.

Run `cervterm.exe --doctor --config <path>` to confirm configured intent and platform eligibility. A passing automated suite is not a Narrator/NVDA support claim; complete the matrix in [`manual-verification.md`](manual-verification.md) and record evidence in `docs/validation/phase-12-accessibility-qualification.md`.

## Known beta limitations

- Windows binaries are currently unsigned unless a release explicitly says otherwise.
- The portable zip is the primary install path for beta releases.
- GUI visual regression coverage is still being expanded.
- Daily-driver TUI coverage is not complete yet.

## Troubleshooting

See [`troubleshooting.md`](troubleshooting.md) for common failure modes and support checklist.
