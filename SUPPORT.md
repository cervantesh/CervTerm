# CervTerm Support

CervTerm is beta software. The project currently supports developer testing, controlled beta releases, and technically comfortable early users.

## Before opening an issue

Please run:

```cmd
cervterm --doctor
cervterm --build-info
```

If the issue is visual or terminal-behavior related, also collect:

- screenshot or short screen recording,
- diagnostics log path from `--doctor`,
- config file if customized,
- raw VT capture when possible,
- exact shell/program that reproduces the issue.

See [`docs/getting-started.md`](docs/getting-started.md) and [`docs/troubleshooting.md`](docs/troubleshooting.md).

## Supported beta environment

Best-effort support currently targets:

- Windows portable zip releases from GitHub Releases,
- the GLFW/OpenGL frontend on Windows,
- headless Linux verification artifacts,
- default config and documented Lua config overrides,
- OSC 8 hyperlinks and OSC 133/633 shell semantic metadata/actions under the documented trust policy,
- default-off terminal notification requests; the native notification adapter is currently Windows-only.
- controlled testing of the restart-scoped, visible-only, default-off Windows UI Automation adapter.

Not yet guaranteed:

- stable daily-driver compatibility for all TUIs,
- signed Windows installer UX,
- MSI or stable winget install path,
- full cross-platform GUI behavior,
- native bell/notification effects on macOS or Linux GUI builds.
- Narrator/NVDA support, default-on accessibility, macOS NSAccessibility, Linux AT-SPI, or Windows 386 accessibility.

## Security or supply-chain concerns

For release trust details, see [`docs/release-trust.md`](docs/release-trust.md). Do not paste secrets, private keys, tokens, or proprietary terminal captures into public issues.
