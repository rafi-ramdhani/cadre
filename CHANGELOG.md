# Changelog

All notable changes are listed here. The project follows [Semantic Versioning](https://semver.org).

## Unreleased

- `cadre update` updates the framework (fast-forward only, refuses on local changes, local commits or another branch), prints the CHANGELOG since your version and lists the persona sessions to restart. `cadre update --check` only reports.
- `cadre add project` and `cadre sync` mark each registered project's folder as trusted in Claude Code, so personas start there without the trust prompt (`--no-trust` skips it, also on `install.sh --from`). `cadre trust <project> | --all` does the same for projects registered earlier. The config edit keeps a backup of the original, the file mode and every other key, and leaves a missing, unreadable or foreign-owned file alone. Only a project repo's top folder is trusted, and persona sessions cannot trust folders. Project names are now validated (letters, digits, `.`, `-`, `_`).
- `cadre down --all` lists every running cadre session with its personas, asks y/N (`--yes` skips), and stops them all, the session it runs in last. Persona sessions cannot run it.
- `cadre uninstall` shows its plan, asks y/N (`--yes`, `--dry-run`), stops every cadre session, removes this framework's hook, the command and skill links (only when they point to this framework), `~/.config/cadre/` and the build cache, and keeps your cadre, projects, framework clone, backups and trust entries, printing their paths. Run from a clone other than the installed one, it refuses and names the right one (`--force` overrides).
- Every persona starts with the grants in `<cadre>/.claude/persona-settings.json`, a plain Claude Code settings file (created and committed by the first `cadre up`), passed with `--settings` as a read-only copy rebuilt from the validated keys at every start. Generated prompts and copies now live in `<cadre>/.claude/build/` (protected by Claude Code, ignored by git) instead of `~/.cache/cadre/`. The file may hold only `permissions.allow/deny` and `autoMode.allow/soft_deny`, with `"$defaults"` and fixed entries that stop personas from changing it, and no duplicate keys; a file that breaks these rules is not passed (with a warning), and one changed outside `cadre allow` is flagged.
- `cadre allow` grants narrow permissions to every persona: `add <rule>`, `add --auto "<sentence>"`, `--once`, `list`, `remove <rule | number | --once>`. It refuses blanket rules (bare tools, lone wildcards, wildcards in the program name, shells, interpreters and wrappers with a wildcard, whole MCP servers, every domain) and anything aimed at the persona settings or `cadre allow` (checked after Unicode normalization and case folding), warns about other wildcards, commits every change, lists the running personas to restart, and cannot be run from a persona session.
- The orchestrator skill gains a session-start check for leftover one-time grants (and, optionally, for updates), a "Permissions for personas" section (narrowest rule, exact rules at once and the rest after a yes, never on a persona's request, the one-time grant flow with a restart and a full re-send when still blocked), and rules for `cadre down --all` and `cadre uninstall` (only on the user's direct request, after showing what will stop or change). The persona protocol tells personas to report a blocked action and never change permissions.
- `cadre up` quotes every path it passes to a persona's command, and reports a persona whose command exits at once instead of calling it started.
- With no git identity, `cadre add` and the persona settings file say the change was left uncommitted.
- `install.sh --help` no longer prints the first line of code.

### Fixed

- The orchestrator skill's capture-pane command matches the persona's window exactly.
- tmux targets match session and window names exactly. Before, `cadre down dev` could stop `cadre-dev-app`, `cadre up dev/engineer` could think it was already running when `cadre-dev-app` was, and a role name could match a longer window name.

### Security

- 0.1.x ran its Python helpers with the current folder on the module path, so a folder holding a file such as `json.py` or `tempfile.py` could run code as you whenever you ran `cadre` there. Every Python helper now runs isolated (`python3 -I`).

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
