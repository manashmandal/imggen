# Self-Update & Version Check

## Overview

Add automatic version checking on startup (cached, 24h TTL) and an `imggen update` command that downloads the latest release binary from GitHub and replaces the current executable.

## Architecture

New package: `internal/update/`

### check.go — Version Check

- `CheckForUpdate(currentVersion string) (*UpdateInfo, error)` — main entry point
- Fetches `GET https://api.github.com/repos/manashmandal/imggen/releases/latest`
- Caches result in `~/.imggen/update-check.json` with 24h TTL
- Uses `golang.org/x/mod/semver` for version comparison
- Returns nil if no update available or version is "dev"

Cache format:
```json
{"last_check": "2026-03-28T19:00:00Z", "latest_version": "0.3.0"}
```

### update.go — Self-Update

- `SelfUpdate(currentVersion string) error` — download and replace binary
- Determines correct asset: `imggen_{version}_{goos}_{goarch}.tar.gz` (`.zip` for Windows)
- Downloads to temp dir, extracts binary, replaces current executable via rename
- Handles permission errors with clear messaging

### CLI Integration (cmd/imggen/main.go)

- `imggen update` — downloads and replaces binary
- `imggen update --check` — just checks, prints result
- Startup: background goroutine in `run()` calls CheckForUpdate, prints yellow notice to stderr if update available

### Edge Cases

- `version == "dev"` — skip checks, print error on explicit update
- Permission denied — suggest sudo or manual download
- No network — silent fail on startup, error on explicit update
- GitHub rate limiting — 60 req/hr unauthed, 24h cache prevents issues

## Dependencies

- `golang.org/x/mod/semver` (Go extended stdlib)
