# Contributing

Thanks for helping. Cadre is small on purpose: one program that starts and stops sessions, guards permissions and trust, and keeps its own files consistent. Everything else is the orchestrator's job, guided by the skill. Changes that keep it that way are the easiest to accept.

## Before you start

- **Bugs:** open an issue with what you ran, what you expected and what happened, plus your OS and `cadre --version`.
- **Features:** open an issue first to talk it through. If the orchestrator can do something on request, it needs no code: many ideas fit better in the skill, or in a personal cadre (a member, a playbook rule), than in the program.

## Making a change

1. Fork and branch from `main`.
2. **Go version:** `go.mod` declares Go 1.26.0. CI tests with it and with the latest stable release, and releases build with the latest stable.
3. **Dependencies:** the standard library, plus `golang.org/x/term`, `golang.org/x/sys` and `golang.org/x/text` (Unicode normalization for `cadre allow`). Nothing else without an issue first.
4. Run the checks:
   ```bash
   gofmt -l .
   go vet ./...
   go run honnef.co/go/tools/cmd/staticcheck@2026.2.1 ./...
   go test -race ./...
   go test -race -tags cadretest ./...
   tests/smoke.sh
   tests/bridge.sh
   ```
   The tests and the smoke test run in a throwaway HOME with a stub `claude` and a private tmux server, so they need no Claude account and never touch your own setup. The `cadretest` build tag holds the parts only tests use (a fake runtime and test switches); release binaries leave them out. `tests/bridge.sh` installs cadre 0.1.1 from the `v0.1.1` tag and checks that a `git pull` to your last commit keeps that install quiet (see below); it needs `jq` and `python3`, as 0.1.1 did, and tests your last commit, so commit first.
5. Update `CHANGELOG.md` under "Unreleased", and the docs if behaviour changes.
6. Open a pull request describing the problem and the change.

**Releases:** `.goreleaser.yaml` builds static binaries for macOS and Linux (amd64, arm64) with `checksums.txt`, and `packaging/homebrew` makes the tap's formula from them. Check a change to the goreleaser file with `go run github.com/goreleaser/goreleaser/v2@v2.18.2 check`.

## What belongs where

| Change | Place |
|---|---|
| Commands, their help and messages | `cmd/cadre` |
| Cadres, projects, sessions, grants, backups | `internal/...` |
| Anything specific to Claude Code (flags, settings format, hooks, trust, skill location) | `internal/runtime/claude`; a test keeps it out of the rest |
| How members receive and answer work | `protocol.md` |
| How the orchestrator behaves | `skills/cadre/SKILL.md`, and `orchestrator.md` for its opening prompt |
| What a new cadre starts with | `template/` |
| Installation | `install.sh`, `packaging/homebrew` |
| The bridge for 0.1.x installs | `bin/cadre`, `bin/orchestrator-hook.sh` (see below) |

Personal workflows, project-specific members and house rules belong in a user's own cadre, not here.

## The bridge for 0.1.x installs

Cadre 0.1.x installed itself as a clone of this repository: its `cadre` command and orchestrator skill are links into the clone, and its SessionStart hook runs `bash <clone>/bin/orchestrator-hook.sh` in every Claude Code session. So `main` keeps three things at those paths through 0.x:

- `bin/orchestrator-hook.sh` always exits 0 and prints nothing, so a `git pull` never breaks anyone's Claude Code sessions;
- `bin/cadre` prints one line, the install command for the new program, and exits 1;
- `skills/cadre/` stays where it is (it is also the skill the program embeds).

Keep them that way: no pass-through to the new program and no install offer. `tests/bridge.sh` checks them.

## Writing style

Plain words and short sentences in messages and docs. No em dashes anywhere, in code, comments or docs.
