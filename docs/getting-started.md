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

## Experimental Kitty graphics opt-in

CervTerm has deliberately narrow Kitty, Sixel, and iTerm inline-image subsets for controlled testing. They are experimental, disabled by default, restart-scoped, independently composable, rendered only by the GLFW/OpenGL frontend, and do not establish a support claim. Enable only the protocols you need in an explicit v2 configuration, then restart CervTerm:

```lua
return {
  config_version = 2,
  graphics = {
    -- Set only the protocol(s) being tested to true.
    kitty = { enabled = false },
    sixel = { enabled = false },
    iterm = { enabled = false },
    limits = {
      encoded_bytes_per_pane = 8388608,
      decoded_bytes_per_pane = 67108864,
      image_count_per_pane = 256,
      placement_count_per_pane = 1024,
      gpu_bytes_per_context = 268435456,
    },
  },
}
```

The values shown are the built-in caps shared by every enabled protocol in the process; GPU limits apply independently to each OpenGL context. Every value must be positive; configuration may lower but not raise a cap. Changing any graphics enable flag or limit requires a restart. Enabling any one protocol creates the shared image model, process budget, pane-local stores, FIFO decode scheduler, and one cache per GL context, but only the enabled adapters. With all three disabled, these resources, image draw work, deadlines, and idle wakes remain absent.

The accepted Sixel surface is exact:

- 7-bit `ESC P q`, `ESC P 0q`, `ESC P 0;0q`, or `ESC P 0;0;0q`, terminated only by ST;
- exactly one `"1;1;W;H` raster declaration before pixel output;
- data `?`–`~`, repeat `!N<char>` (`N=1..4096`), palette select/define `#N` and `#N;2;R;G;B`, carriage return `$`, and next six-pixel band `-`;
- register `0..255`, RGB percentages `0..100`, dimensions at most 4096, at most 16,777,216 pixels/64 MiB RGBA, at most 4,194,304 operations, and a checked `1..256`-cell span per axis; and
- image-local palette definitions seeded from the pane's detached 256-color palette; unset pixels are transparent.

The accepted iTerm surface is exact: 7-bit `OSC 1337;File=<fields>:<strict padded base64>` terminated by BEL or ST. It requires exactly one lexical `inline=1`, one positive decimal `size`, non-empty standard padded base64, and exactly one PNG whose decoded byte count equals `size` with no trailing bytes. It optionally accepts one cell dimension, `width=N` xor `height=N` for `N=1..256`, plus absent or exact `preserveAspectRatio=1`; the other axis is derived with checked cell-aspect arithmetic.

The Kitty surface remains direct data only (`t=d` or default), actions `t`/`T`/`p`/`d`/`q`, RGB24/RGBA32/PNG, and raw-only `o=z`. Kitty replies are fixed and value-free. Sixel and iTerm are cursor-neutral, emit no protocol reply, and reserve no reply slot.

Shared hard caps include a 256 KiB selected logical control-string frame and 16 KiB borrowed chunks; 4,096 chunks and a 10-second lifetime per transfer; 8 pending transfers/8 MiB encoded per pane and 32/32 MiB process-wide; 64 MiB decoded per image and pane, 256 MiB process-wide; 256 images/1,024 placements per pane and 1,024/4,096 process-wide; one outstanding decode per pane, two process workers, FIFO queue capacity 32, and a queue-inclusive 250 ms acceptance deadline; plus 512 textures/256 MiB per OpenGL context.

Sixel/iTerm use internal image and placement IDs in `0x80000000..0xffffffff`; Kitty wire IDs remain in `1..0x7fffffff`. Each Sixel/iTerm success is one ephemeral resource plus placement. The owner retires the resource when its final placement is removed by editing, erase, scroll/history eviction, ED3, reflow, alternate-screen exit, explicit delete, reset, or close. Parser-coupled mux `PaneOutput` redaction removes only selected envelopes for enabled image protocols before Lua output callbacks; disabled and unselected control strings remain public.

Rejected forms commit nothing. C1 DCS/OSC, HLS, duplicate/unknown metadata, iTerm `name`, auto/pixel/percent/two-axis/stretch/multipart forms, JPEG/GIF, animation, Sixel scrolling/DECSDM, cursor effects, and every file/path/URL/temporary-file/shared-memory/download/write or other external-I/O mode are excluded. Phase 14 leaf packages have no filesystem/network/process/unsafe dependency.

Operational rollback is independent: set only the affected `graphics.kitty.enabled`, `graphics.sixel.enabled`, or `graphics.iterm.enabled` to `false` and restart. `--doctor` reports configured intent and limits from a separate one-shot process; it does not prove live frontend activation or manual qualification. See [`manual-verification.md`](manual-verification.md#phase-14-experimental-sixel-and-iterm-images--real-gui-qualification), [`validation/phase-14-qualification.md`](validation/phase-14-qualification.md), and [`validation/phase-14-closeout.md`](validation/phase-14-closeout.md). Every Phase 14 real-GUI row is currently UNRUN, so full/broad conformance and platform support remain unclaimed.

## Known beta limitations

- Windows binaries are currently unsigned unless a release explicitly says otherwise.
- The portable zip is the primary install path for beta releases.
- GUI visual regression coverage is still being expanded.
- Daily-driver TUI coverage is not complete yet.

## Troubleshooting

See [`troubleshooting.md`](troubleshooting.md) for common failure modes and support checklist.
