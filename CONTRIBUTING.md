# Contributing

Thanks for helping. Cadre is small on purpose: a bash launcher, a protocol, a skill and a template. Changes that keep it that way are the easiest to accept.

## Before you start

- **Bugs:** open an issue with what you ran, what you expected and what happened, plus your OS and `cadre version`.
- **Features:** open an issue first to talk it through. Many ideas fit better in a personal cadre (a persona, a playbook rule) than in the framework.

## Making a change

1. Fork and branch from `main`.
2. Keep the scripts portable: bash 3.2 (macOS default) and GNU tools on Linux. Avoid `sed -i`, `readlink -f` and other flags that differ between the two.
3. Run the checks:
   ```bash
   shellcheck -x bin/cadre bin/orchestrator-hook.sh install.sh tests/smoke.sh
   tests/smoke.sh
   ```
   The smoke test runs in a throwaway HOME with a stub `claude` and a private tmux server, so it needs no account and never touches your own cadre. It installs from your last commit, so commit before running it.
4. Update `CHANGELOG.md` under "Unreleased" and the docs if behaviour changes.
5. Open a pull request describing the problem and the change.

## The Go rewrite (0.2.0)

Cadre 0.2.0 is being rewritten in Go on the `rewrite/0.2.0` branch (spec section O). Until it reaches parity, the bash launcher stays next to it.

- **Go version:** `go.mod` declares Go 1.26.0, one minor behind the latest stable release when the module was created (Go 1.27.2, October 2026). CI tests with both, and releases build with the latest stable.
- **Dependencies:** the standard library, plus `golang.org/x/term`, `golang.org/x/sys` and `golang.org/x/text` (Unicode normalization for `cadre allow`). Nothing else without an issue first.
- **Checks:**
  ```bash
  gofmt -l .
  go vet ./...
  go run honnef.co/go/tools/cmd/staticcheck@2026.2.1 ./...
  go test -race ./...
  ```
- **Releases:** `.goreleaser.yaml` builds static binaries for macOS and Linux (amd64, arm64). Check a change to it with `go run github.com/goreleaser/goreleaser/v2@v2.18.2 check`.

## What belongs where

| Change | Place |
|---|---|
| How personas receive and answer work | `protocol.md` |
| How the orchestrator behaves | `skills/cadre/SKILL.md` |
| What a new cadre starts with | `template/` |
| Commands | `bin/cadre` |
| Installation | `install.sh` |

Personal workflows, project-specific personas and house rules belong in a user's own cadre, not here.
