# Changelog

All notable changes are listed here. The project follows [Semantic Versioning](https://semver.org).

## Unreleased

- `cadre update` updates the framework (fast-forward only, refuses on local changes, local commits or another branch), prints the CHANGELOG since your version and lists the persona sessions to restart. `cadre update --check` only reports.
- `cadre add project` and `cadre sync` mark each registered project's folder as trusted in Claude Code, so personas start there without the trust prompt (`--no-trust` skips it, also on `install.sh --from`). `cadre trust <project> | --all` does the same for projects registered earlier. The config edit keeps a backup of the original, the file mode and every other key, and leaves a missing, unreadable or foreign-owned file alone. Only a project repo's top folder is trusted, and persona sessions cannot trust folders. Project names are now validated (letters, digits, `.`, `-`, `_`).
- `install.sh --help` no longer prints the first line of code.

### Fixed

- tmux targets match session and window names exactly. Before, `cadre down dev` could stop `cadre-dev-app`, `cadre up dev/engineer` could think it was already running when `cadre-dev-app` was, and a role name could match a longer window name.

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
