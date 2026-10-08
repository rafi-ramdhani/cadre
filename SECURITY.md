# Security

## Reporting a vulnerability

Please report security problems privately through GitHub's "Report a vulnerability" button on this repository's Security tab, not in a public issue. You should get a reply within a week.

## Things to know

- Persona sessions are full Claude Code sessions. They act on messages from the orchestrator with the permissions of the mode set in `cadre.conf`. A permissive mode such as `auto` lets them act without asking; choose it deliberately.
- The optional orchestrator hook edits `~/.claude/settings.json` (a backup is kept as `settings.json.bak-cadre`).
- `cadre add project`, `cadre sync` and `cadre trust` mark registered project folders as trusted in Claude Code's `~/.claude.json` (a backup is kept as `.claude.json.bak-cadre`). Trust lets a repo's own `.claude/settings.json` take effect, so register only repos you trust; `--no-trust` skips it.
- `install.sh` is meant to be read before you pipe it to bash. It writes only to the cadre folder you name, `~/.local/bin/cadre`, `~/.claude/skills/cadre`, `~/.config/cadre/` and, with the hook, `~/.claude/settings.json`.
