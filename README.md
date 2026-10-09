# Cadre

[![CI](https://github.com/rafi-ramdhani/cadre/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadre/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Plug-and-play orchestration for Claude Code.**

Run `cadre`, then talk to it. Cadre opens Claude Code as an orchestrator that runs a small team for you: an engineer that builds on a branch, a reviewer that checks the work, and any other member you ask for. Each member is its own Claude Code session in tmux. The orchestrator hands out the work, passes results between members and reports back to you.

<!-- The demo GIF is recorded from docs/demo.tape; see the comments at its top. -->
<!-- ![cadre in a terminal](docs/demo.gif) -->

**Why not subagents?** Members are full, long-lived Claude Code sessions with their own context: you can open one and talk to it directly, and they keep working across days.

> Cadre is an independent project. It is not affiliated with or endorsed by Anthropic.

## First steps

```bash
brew install rafi-ramdhani/cadre/cadre
cadre                     # set up your cadre and open the orchestrator, then talk to it
cadre ls                  # what runs, and your projects
cadre attach dev my-app   # watch a team at work, or talk to it
cadre stop                # stop the members (asks first)
```

The first `cadre` asks a name and a yes or two, then opens the orchestrator with a ready team. Tell it which repo to work on ("work on github.com/you/app"), then what to do ("add CSV export"). Everything else is a request in plain words:

- "add a designer to the team"
- "allow the members to run `npm test`"
- "back up my cadre to GitHub"
- "stop the dev team"

**Without Homebrew**, download the release binary to `~/.local/bin/cadre` (it checks the checksum, and running it again upgrades):

```bash
curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | sh
```

**On a new machine**, run `cadre` and choose "restore": it clones your cadre from its GitHub backup and clones its projects.

### Requirements

- macOS or Linux
- [Claude Code](https://claude.com/claude-code), installed and logged in
- tmux 3.2 or newer, and git (Homebrew installs tmux with cadre, and git too on Linux)
- gh (optional), to back up your cadre to GitHub

`cadre` checks these when it starts and says how to fix anything missing.

## Commands

```bash
cadre                       # the orchestrator, in this terminal (--fresh starts a new conversation)
cadre --tmux [--detach]     # the orchestrator in tmux, to come back to later (also over SSH)
cadre ls [--all]            # what runs, your projects, and your other cadres
cadre attach [team] [project]   # watch or talk to a team; with no team, the orchestrator in tmux
cadre stop [team[/role]] [project]   # stop a team; with no team, every member of this cadre (asks first)
cadre help [advanced]       # the commands; advanced lists the ones the orchestrator runs
cadre --version
cadre uninstall             # remove what cadre set up (asks first; keeps your cadres and projects)
```

The orchestrator runs the advanced commands for you (projects, starting members, permissions). `cadre help advanced` lists them, and [docs/guide.md](docs/guide.md) explains them.

## How it works

- **Your cadre** is a folder, `~/.cadre/<name>`, and a git repository. It holds the members (`members/<team>/<role>.md`, plain Markdown), the playbook the orchestrator follows, the list of projects, and the grants members get. The orchestrator commits every change, and pushes when you have a backup.
- **Projects stay where you keep them.** The cadre records each project's repository; each machine records where the folder is. New clones go to a projects folder you choose once (`~/Developer` is suggested).
- **Members** run in tmux, one window each, in their project's folder. Each starts with the shared protocol (how to receive work and reply), the member's file, and the grants you gave. They keep their conversations: a member started again resumes where it was.
- **The orchestrator** is a Claude Code session with the cadre skill. `cadre` opens it in your cadre's folder. With the optional hook, every new Claude Code session starts as the orchestrator (`CADRE_OFF=1 claude` starts a plain one).
- **Project rules** belong in each project's own `CLAUDE.md`, so every member working there follows them.

## Costs, permissions and accounts

**Every member is a full Claude Code session.** Three running members use roughly three times the usage of one. The orchestrator starts only the members a task needs and offers to stop them afterwards.

**Permissions stay with you.** A message from the orchestrator never counts as your consent in a member. When you tell the orchestrator "allow the members to push to main in my-app", it records a narrow rule with `cadre allow`, which every member starts with. Blanket rules, and rules that reach cadre's own files, are refused; members cannot grant themselves anything. Members run in the permission mode set in your cadre's `cadre.conf` (default: `default`); choose `auto` or another mode deliberately.

**Registered projects are trusted** in Claude Code, so members start there without the trust prompt. Trust also lets a repo's own `.claude/settings.json` take effect, so add only repos you trust (or pass `--no-trust`).

**One Claude account at a time.** Cadre uses the account Claude Code is logged in with. Tools such as claude-swap can switch it; a switch applies to every cadre on the machine. After a switch, restart: `cadre stop`, close the orchestrator, then run `cadre` again. Conversations resume.

[SECURITY.md](SECURITY.md) has the details.

## Coming from 0.1.x

0.2.0 is a **breaking upgrade**: cadre is now one program instead of a git clone, and cadres live in `~/.cadre`. Nothing is migrated by code, and your old cadre is read, never changed.

1. Stop your 0.1.x teams (`cadre down --all` in 0.1.x, or `cadre stop` after the upgrade, which also stops sessions 0.1.x started).
2. Install 0.2.0: `brew install rafi-ramdhani/cadre/cadre` (or the `install.sh` above).
3. Run `cadre`. It creates a new cadre with the starter team, and offers to point the skill link and the orchestrator hook at the new program.
4. Tell the orchestrator: **"bring in my old cadre from <path>"**. It reads the old members, playbook, projects and grants, shows you one plan, and copies them on your yes. Projects stay where they are.
5. Start teams again as you need them.

The [CHANGELOG](CHANGELOG.md) lists every change to know about.

## What gets installed where

Cadre needs no runtime besides Claude Code, tmux and git. Everything it creates is listed here. `$CLAUDE_CONFIG_DIR` replaces `~/.claude` (and holds `.claude.json`) when it is set.

| What | Where | Created by | `cadre uninstall` |
|---|---|---|---|
| The `cadre` program | Homebrew's `bin`, or `~/.local/bin/cadre` | `brew install`, or `install.sh` | Kept: it ends by printing the command that removes it |
| Your cadres: members, playbook, projects list, `cadre.conf`, grants (`.claude/member-settings.json`) and `teams/` | `~/.cadre/<name>/`, each its own git repository | the first run, or the orchestrator | Kept |
| Generated files: prompts, settings copies, conversation records, locks | `~/.cadre/<name>/.claude/build/`, ignored by the cadre's git | every start | Kept, with the cadre |
| The pre-push check that keeps credentials and large files out of backups | `~/.cadre/<name>/.git/hooks/pre-push` | the first run, and put back by every `cadre` | Removed |
| This machine's settings: the default cadre, the projects folder, where each project is, the grants fingerprint, the health check's state | `~/.cadre/config/` | `cadre` | Removed |
| The orchestrator skill and prompts | `~/.cadre/framework/`, rewritten when they differ from the program's | every `cadre` | Removed |
| The skill link | `~/.claude/skills/cadre`, pointing at `~/.cadre/framework/skills/cadre` | the first run, on your yes | Removed, if it points at this cadre |
| The orchestrator hook (optional) | one SessionStart entry in `~/.claude/settings.json`; backup `settings.json.bak-cadre` | the first run, on your yes | Removed, if it runs this cadre (other hooks stay) |
| Workspace trust for your projects | entries in `~/.claude.json`; backup `.claude.json.bak-cadre` | adding, linking or cloning a project | Kept: the entries are shared with your own sessions |
| Your projects | wherever you keep them; new clones in your projects folder | you, or the orchestrator | Kept |
| Members' conversations | Claude Code's own folder | Claude Code | Kept |

Backups are written before the first change and never overwritten.

## Uninstall

`cadre uninstall --dry-run` shows the plan. `cadre uninstall` shows it, asks, stops the members, and removes the skill link and the hook (only where they point at this cadre), each cadre's pre-push check, `~/.cadre/config` and `~/.cadre/framework`. It keeps every cadre and project, and ends with the command that removes the program itself (`brew uninstall cadre`, or the `rm` for `install.sh`). To remove a cadre too, delete its folder in `~/.cadre`.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
