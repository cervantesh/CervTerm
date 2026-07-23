# Configuration Migration Guide

CervTerm configuration migration is explicit and read-only. Loading or diagnosing a configuration never rewrites it, changes its file mode, or creates a converted neighbor. Keep a backup, copy the relevant values into a new v2 file, run `cervterm --doctor --config <path>`, and replace your active file only after reviewing the result.

## CervTerm v1 to v2

An unversioned file remains on the permissive v1 compatibility path. To opt into strict v2, add `config_version = 2` and correct every field reported by doctor. Unknown or mistyped values that v1 historically ignored become actionable v2 errors.

Before:

```lua
return {
  window = { opacity = 0.92 },
  font = { family = "JetBrainsMono Nerd Font", size = 12.5 },
  clipboard = { osc52 = "off" },
}
```

After:

```lua
return {
  config_version = 2,
  window = { opacity = 0.92 },
  font = { family = "JetBrainsMono Nerd Font", size = 12.5 },
  clipboard = { osc52 = "off" },
}
```

The repository corpus proves effective equivalence for the complete supported daily-driver example and proves that the loader leaves source bytes, mode, and adjacent files unchanged.

## WezTerm to CervTerm v2

A WezTerm file is a manual translation, not an executable CervTerm migration. Copy only supported intent into a fresh strict-v2 file. The sanitized daily-driver pair demonstrates mappings for padding, opacity, font fallback, line height, cursor, tabs, scrollback, scrollbar, bell, initial grid, and FPS.

The following source behavior is deliberately not translated automatically:

- renderer selection, which is excluded from the parity roadmap;
- child-process or clipboard-image callbacks;
- update checks and custom status callbacks;
- mouse behavior whose side effects require an explicit CervTerm binding review.

No WezTerm file is executed by the CervTerm migration tests.

## Sanitized real-user corpus

| Case | Source | Result |
|---|---|---|
| `cervterm-v1-daily-driver` | Owner-provided CervTerm v1 daily-driver configuration | Exact effective equivalence to paired v2; source immutability enforced. |
| `wezterm-daily-driver` | Owner-provided WezTerm daily-driver configuration | Supported intent translated; unsupported/excluded surfaces listed in its manifest. |

Fixtures live under `internal/config/testdata/user-migration/`. Personal paths, external script locations, usernames, tokens, and payloads are removed. Manifests distinguish exact CervTerm equivalence from cross-terminal translation.

## Verification

```sh
go test ./internal/config -run '^TestUserMigrationCorpus$' -count=1
go test -race ./internal/config ./internal/script -count=1
cervterm --doctor --config path/to/new-v2.lua
```

A failed v2 load or reload keeps the previous valid runtime. See [Configuration Compatibility and Deprecation Policy](config-compatibility-policy.md).
