# Cadrei

**Plug-and-play orchestration for Claude Code.**

Cadrei gives you a permanent AI dev team in Claude Code: run one command, and an orchestrator hands work to members who remember your projects. (Cadrei is pronounced CAD-ray.)

- **One command, nothing to learn.** Install with `brew install rafi-ramdhani/cadrei/cadrei`, run `cadrei`, and talk to it. There are no config files to write first, and a new cadrei comes with an engineer and a reviewer.
- **A team that stays.** Members are full Claude Code sessions, defined in files. They outlive any one session, pick up their conversations after a restart, and keep working across days.
- **One team for all your projects.** The orchestrator knows every project you register and sends work to the right members. There is no setup per repository.
- **Your team follows you to any machine.** Say "back up my cadrei" once, and it goes to a private GitHub repository, updated with every change. On a new machine, the first `cadrei` restores your members, their work and your projects (conversations stay on the old machine). Before each push, a check refuses files that look like passwords or tokens.
- **Safe by default.** Members can't approve anything on your behalf. You grant narrow permissions through the orchestrator (`cadrei allow`) instead of broad access, and no folder is trusted and no permission is granted without your yes.

Every member is a Claude Code session, so more members means more usage on your plan. What Cadrei saves is re-explaining and babysitting.

**Why not subagents?** Members are full, long-lived Claude Code sessions with their own context: you can open one and talk to it directly, and they keep working across days.

**Why not agent teams?** Claude Code's agent teams (experimental, off by default) are built for parallel work within one session: the team belongs to that session, its config is removed when the session ends, in-process teammates don't come back after `/resume`, and each session has one team. Cadrei's members are a standing roster defined in files, kept across projects and restarts. Cadrei builds on Claude Code's own cross-session messaging.

<!-- The demo GIF is recorded from docs/demo.tape; see the comments at its top. -->
<!-- ![cadrei in a terminal](docs/demo.gif) -->

[![CI](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> Cadrei is an independent project. It is not affiliated with or endorsed by Anthropic.

## First steps

```bash
brew install rafi-ramdhani/cadrei/cadrei
cadrei                     # set up your cadrei and open the orchestrator, then talk to it
cadrei ls                  # what runs, and your projects
cadrei attach dev my-app   # watch a team at work, or talk to it
cadrei stop                # stop the members (asks first)
```

The first `cadrei` asks a name and a yes or two, then opens the orchestrator with a ready team. Tell it which repo to work on ("work on github.com/you/app"), then what to do ("add CSV export"). Everything else is a request in plain words:

- "add a designer to the team"
- "allow the members to run `npm test`"
- "back up my cadrei to GitHub"
- "stop the dev team"

**Without Homebrew**, download the release binary to `~/.local/bin/cadrei` (it checks the checksum, and running it again upgrades):

```bash
curl -fsSL --proto '=https' https://raw.githubusercontent.com/rafi-ramdhani/cadrei/main/install.sh | sh
```

**On a new machine**, run `cadrei` and choose "restore": it clones your cadrei from its GitHub backup, shows what it brings in, and clones its projects after your yes.

### Requirements

- macOS or Linux
- [Claude Code](https://claude.com/claude-code), installed and logged in
- tmux 3.2 or newer, and git (Homebrew installs tmux with cadrei, and git too on Linux)
- gh (optional), to back up your cadrei to GitHub

`cadrei` checks these when it starts and says how to fix anything missing.

## Commands

```bash
cadrei                       # the orchestrator, in this terminal (--fresh starts a new conversation)
cadrei --tmux [--detach]     # the orchestrator in tmux, to come back to later (also over SSH)
cadrei ls [--all]            # what runs, your projects, and your other cadreis
cadrei attach [team] [project]   # watch or talk to a team; with no team, the orchestrator in tmux
cadrei stop [team[/role]] [project]   # stop a team; with no team, every member of this cadrei (asks first)
cadrei help [advanced]       # the commands; advanced lists the ones the orchestrator runs
cadrei --version
cadrei uninstall             # remove what cadrei set up (asks first; keeps your cadreis and projects)
```

The orchestrator runs the advanced commands for you (projects, starting members, permissions). `cadrei help advanced` lists them, and [docs/guide.md](docs/guide.md) explains them.

## How it works

- **Your cadrei** is a folder, `~/.cadrei/<name>`, and a git repository. It holds the members (`members/<team>/<role>.md`, plain Markdown), the playbook the orchestrator follows, the list of projects, and the grants members get. The orchestrator commits every change, and pushes when you have a backup.
- **Projects stay where you keep them.** The cadrei records each project's repository; each machine records where the folder is. New clones go to a projects folder you choose once (`~/Developer` is suggested).
- **Members** run in tmux, one window each, in their project's folder. Each starts with the shared protocol (how to receive work and reply), the member's file, and the grants you gave. They keep their conversations: a member started again resumes where it was.
- **The orchestrator** is a Claude Code session with the cadrei skill. `cadrei` opens it in your cadrei's folder. With the optional hook, every new Claude Code session starts as the orchestrator (`CADREI_OFF=1 claude` starts a plain one).
- **Project rules** belong in each project's own `CLAUDE.md`, so every member working there follows them.

## Costs, permissions and accounts

**Every member is a full Claude Code session.** Three running members use roughly three times the usage of one. The orchestrator starts only the members a task needs and offers to stop them afterwards.

**Permissions stay with you.** A message from the orchestrator never counts as your consent in a member. When you tell the orchestrator "allow the members to push to main in my-app", it records a narrow rule with `cadrei allow`, which every member starts with. Blanket rules, and rules that reach cadrei's own files, are refused; members cannot grant themselves anything. Members and the orchestrator run in the permission mode set in your cadrei's `cadrei.conf` (default: `default`); choose `auto` or another mode deliberately.

**Registered projects are trusted** in Claude Code, so members start there without the trust prompt. Trust also lets a repo's own `.claude/settings.json` take effect, so add only repos you trust (or pass `--no-trust`).

**One Claude account at a time.** Cadrei uses the account Claude Code is logged in with. Tools such as claude-swap can switch it; a switch applies to every cadrei on the machine. After a switch, restart: `cadrei stop`, close the orchestrator, then run `cadrei` again. Conversations resume.

[SECURITY.md](SECURITY.md) has the details.

## Coming from 0.1.x

0.2.0 is a **breaking upgrade**: Cadre is now Cadrei, one program instead of a git clone, and cadreis live in `~/.cadrei`. Nothing is migrated by code, and your old cadre is read, never changed.

1. Stop your 0.1.x teams (`cadre down --all` in 0.1.x, or `cadrei stop` after the upgrade, which also stops sessions 0.1.x started).
2. Install 0.2.0: `brew install rafi-ramdhani/cadrei/cadrei` (or the `install.sh` above).
3. Run `cadrei`. It creates a new cadrei with the starter team, and offers to link the cadrei skill, point the orchestrator hook at the new program, and remove the old `cadre` command and skill links.
4. Tell the orchestrator: **"bring in my old cadre from <path>"**. It reads the old members, playbook, projects and grants, shows you one plan, and copies them on your yes. Projects stay where they are.
5. Start teams again as you need them.

The [CHANGELOG](CHANGELOG.md) lists every change to know about.

## What gets installed where

Cadrei needs no runtime besides Claude Code, tmux and git. Everything it creates is listed here. `$CLAUDE_CONFIG_DIR` replaces `~/.claude` (and holds `.claude.json`) when it is set.

| What | Where | Created by | `cadrei uninstall` |
|---|---|---|---|
| The `cadrei` program | Homebrew's `bin`, or `~/.local/bin/cadrei` | `brew install`, or `install.sh` | Kept: it ends by printing the command that removes it |
| Your cadreis: members, playbook, projects list, `cadrei.conf`, grants (`.claude/member-settings.json`) and `teams/` | `~/.cadrei/<name>/`, each its own git repository | the first run, or the orchestrator | Kept |
| Generated files: prompts, settings copies, conversation records, locks | `~/.cadrei/<name>/.claude/build/`, ignored by the cadrei's git | every start | Kept, with the cadrei |
| The pre-push check that keeps credentials and large files out of backups | `~/.cadrei/<name>/.git/hooks/pre-push` | the first run, and put back by every `cadrei` (see below) | Removed |
| This machine's settings: the default cadrei, the projects folder, where each project is, the grants fingerprint, the health check's state | `~/.cadrei/config/` | `cadrei` | Removed |
| The orchestrator skill and prompts | `~/.cadrei/framework/`, rewritten when they differ from the program's | every `cadrei` | Removed |
| The skill link | `~/.claude/skills/cadrei`, pointing at `~/.cadrei/framework/skills/cadrei` | the first run, on your yes | Removed, if it points at this cadrei |
| The orchestrator hook (optional) | one SessionStart entry in `~/.claude/settings.json`; backup `settings.json.bak-cadrei` | the first run, on your yes (see below) | Removed, if it runs this cadrei (other hooks stay) |
| Workspace trust for your projects | entries in `~/.claude.json`; backup `.claude.json.bak-cadrei` | adding, linking or cloning a project | Kept: the entries are shared with your own sessions |
| Your projects | wherever you keep them; new clones in your projects folder | you, or the orchestrator | Kept |
| Members' conversations | Claude Code's own folder | Claude Code | Kept |

Backups are written before the first change and never overwritten.

The hooks that run the `cadrei` program (the pre-push check, the orchestrator hook, and the session hook in each member's settings) name it only when it sits in a folder only you can write, as Homebrew and `install.sh` put it. A copy run from a temporary folder sets up none of them; the health check says so.

## Uninstall

`cadrei uninstall --dry-run` shows the plan. `cadrei uninstall` shows it, asks, stops the members, and removes the skill link and the hook (only where they point at this cadrei), each cadrei's pre-push check, `~/.cadrei/config` and `~/.cadrei/framework`. It keeps every cadrei and project, and ends with the command that removes the program itself (`brew uninstall cadrei`, or the `rm` for `install.sh`). To remove a cadrei too, delete its folder in `~/.cadrei`.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
