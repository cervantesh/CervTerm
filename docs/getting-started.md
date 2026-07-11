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

Run with an explicit config:

```cmd
.\cervterm.exe --config .\cervterm.lua
```

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

## Known beta limitations

- Windows binaries are currently unsigned unless a release explicitly says otherwise.
- The portable zip is the primary install path for beta releases.
- GUI visual regression coverage is still being expanded.
- Daily-driver TUI coverage is not complete yet.

## Troubleshooting

See [`troubleshooting.md`](troubleshooting.md) for common failure modes and support checklist.
