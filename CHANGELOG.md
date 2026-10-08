# Changelog

All notable changes are listed here. The project follows [Semantic Versioning](https://semver.org).

## Unreleased

## 0.1.1 - 2026-10-08

- `cadre init`, `cadre add` and the installer work on a machine without a git identity: the change is left uncommitted with a note instead of failing.
- Clearer control flow in `bin/cadre` (no `A && B || C`), lint-clean at shellcheck's strictest level.
- The smoke test uses its own git config.

## 0.1.0 - 2026-10-08

First public release.

- `install.sh`: one command from nothing to a working cadre; `--from` restores an existing cadre and its projects on a new machine.
- `cadre` command: `init`, `use`, `add project|team|persona`, `up`, `down`, `attach`, `ls`, `projects`, `path`, `sync`, `version`.
- Per-project team sessions (`cadre up dev my-app` gives `dev-my-app-<role>`).
- Project registry (`projects.yaml`) with projects cloned into `projects/`.
- Shared persona protocol, orchestrator skill, optional SessionStart orchestrator hook.
- Starter template with a dev team and a research team.
- End-to-end smoke test and CI on macOS and Linux.
