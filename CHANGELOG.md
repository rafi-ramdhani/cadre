# Changelog

All notable changes are listed here. The project follows [Semantic Versioning](https://semver.org).

## Unreleased

## 0.2.0 - Unreleased

Cadre is now one program, written in Go, that you install with Homebrew or a small download script. Run `cadre`, then talk to the orchestrator. **This is a breaking upgrade**: read "Upgrading from 0.1.x" below before you install.

### Breaking changes

| Change | What to know |
|---|---|
| Cadre is a program, not a git clone | `git pull` in `projects/cadre` no longer upgrades. Upgrade with `brew upgrade cadre`, or run `install.sh` again. |
| Cadres live in `~/.cadre/<name>` | 0.2.0 does not open a 0.1.x cadre. The orchestrator brings it in when you ask ("bring in my old cadre from <path>"). Projects stay where they are, and the old folder and `~/.config/cadre` are left as they are. |
| Personas are now members | `personas/` becomes `members/` and `persona-settings.json` becomes `member-settings.json` (bringing a cadre in writes the new names). `CADRE_PERSONA` becomes `CADRE_MEMBER`. `cadre ls --json` lists `members`. |
| Team and member names | They may use letters, digits, `-` and `_`. A team or member with a dot in its name (allowed in 0.1.x, for example `ml.ops`) cannot be started; rename its folder (the orchestrator does it on request, and does it when it brings in an old cadre). |
| Old command names | Each prints one line pointing to the new way and exits 1: `cadre down` says to use `cadre stop`, `cadre add project` to ask the orchestrator or use `cadre project add`, and so on. |
| `cadre ls` output | A new status screen. Scripts use `cadre ls --json`. |
| `cadre.conf` | Read as `KEY=VALUE` lines. Shell code in it is ignored with a warning. |
| Projects | `projects.yaml` holds each project's repo, team and about, and no local paths. Where each project is on a machine is kept in `~/.cadre/config/places/`. New clones go to a projects folder you choose (`~/Developer` is suggested), not into the cadre. |
| Session names | They gain the cadre's name. Sessions started by 0.1.x show as legacy until restarted. |
| The orchestrator hook | It runs `<cadre program> hook orchestrator`. The first run offers to replace the 0.1.x hook. |
| Python and jq | No longer needed. |

### New

- **Plain `cadre`** opens the orchestrator in your terminal, in your cadre's folder, with the cadre's permission mode. `cadre --tmux [--detach]` runs it in tmux instead, to come back to later or over SSH. One orchestrator runs per cadre: a second `cadre` says where it is open, or attaches to it in tmux.
- **First run**: a new cadre with a starter `dev` team (an engineer and a reviewer), or a restore from GitHub; offers to add the folder you are in as a project, link the skill and add the hook, each on your yes; then a short greeting and the orchestrator.
- **Health check** on every `cadre`, silent unless something is wrong; `cadre --check` runs the full one. It recognizes 0.1.x leftovers and offers to point them at the new program.
- **Projects across machines**: `cadre project add | link | unlink | sync | trust | path | dir`. Missing projects are marked in `cadre ls`, and the orchestrator offers to clone them again, link their new folder or unlink them. Cadre never unlinks a project by itself, and unlinking never touches the folder.
- **Backup and restore**: ask the orchestrator to back up your cadre to a private GitHub repository; the first run on a new machine restores it, shows what it brings in, and clones its projects after a yes. `teams/` is tracked, so members' work there travels too. A pre-push hook refuses to push files that look like credentials, and files over 50 MB.
- **Conversations resume**: members and the orchestrator continue their last conversation when started again. `--fresh` on `cadre`, `cadre up` and `cadre stop` starts a new one.
- **`cadre stop`** stops a team or a member; with no team, every member of this cadre after a confirmation (`--all`: every cadre). It never stops the orchestrator.
- **`cadre allow`** grants narrow permissions to every member, the channel for consent you give in the orchestrator: `add <rule>`, `add --auto "<sentence>"`, `--once`, `list`, `remove`. It refuses blanket rules and anything that reaches cadre's own files, warns about other wildcards, and commits every change. Members start with a validated, read-only copy of the grants, plus fixed rules that keep them off cadre's own files.
- **Trust**: registered project folders are marked trusted in Claude Code, so members start there without the trust prompt (`--no-trust` skips it).
- **Several cadres**: `cadre init <name>` and `cadre use <name>`. A command acts on the cadre whose folder or project you are in, else the default. Configuration is read only from `~/.cadre` or `CADRE_HOME`.
- **`cadre uninstall`** shows its plan and asks. It removes the skill link, the hook, each cadre's pre-push hook, `~/.cadre/config` and `~/.cadre/framework`, keeps every cadre and project, and ends with the command that removes the program.
- **Install**: `brew install rafi-ramdhani/cadre/cadre`, or `install.sh`, which downloads the release binary, checks it against `checksums.txt` and places it in `~/.local/bin`.

### Security

- Members cannot grant permissions, stop the whole cadre, uninstall, create or switch cadres, or add, link, unlink or trust projects. Sessions started by 0.1.x carry `CADRE_PERSONA`, which counts the same way; this alias goes away in a later release.
- `cadre.conf` is parsed, never run, and no configuration is read from the folder you happen to be in.
- Edits to `~/.claude.json` and `~/.claude/settings.json` keep every other key and their order, keep a backup, are written atomically, and are skipped on any doubt.
- 0.1.x ran its Python helpers with the current folder on the module path, so a folder holding a file such as `json.py` could run code as you whenever you ran `cadre` there. 0.2.0 has no Python helpers.

### Upgrading from 0.1.x

Nothing is migrated by code, and your old cadre is only read.

1. **Stop your teams.** In 0.1.x: `cadre down --all`. Or after the upgrade: `cadre stop`, which also stops sessions 0.1.x started.
2. **Install 0.2.0**: `brew install rafi-ramdhani/cadre/cadre`, or `curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | sh`. If the new `cadre` is not the one your shell runs, the health check says which `cadre` comes first on your `PATH`.
3. **Run `cadre`.** It creates a new cadre with the starter team and offers to point the skill link and the orchestrator hook at the new program. When it finds your 0.1.x cadre, it prints its path.
4. **Tell the orchestrator: "bring in my old cadre from <path>".** It reads the old members, playbook, house rules, projects, permission mode and lasting grants, shows you one plan, and copies them on your yes. Projects are linked where they are. Grants go through `cadre allow`, so a rule it refuses is reported, not forced. One-time grants are not carried over.
5. **Start teams again** as you need them. Members start new conversations under the new names.

The 0.1.x clone stays where it was. Running `git pull` in it is harmless: its `cadre` command then points you to the new install, and its hook does nothing.

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
