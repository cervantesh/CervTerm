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
- default config and documented Lua config overrides.

Not yet guaranteed:

- stable daily-driver compatibility for all TUIs,
- signed Windows installer UX,
- MSI or stable winget install path,
- full cross-platform GUI behavior.

## Security or supply-chain concerns

For release trust details, see [`docs/release-trust.md`](docs/release-trust.md). Do not paste secrets, private keys, tokens, or proprietary terminal captures into public issues.
