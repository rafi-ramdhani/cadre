# Cadrei

**A team of Claude Code sessions you lead from one conversation.**

Run `cadrei` and you talk to an orchestrator. It hands your work to members, separate Claude Code sessions that each play one role and pick up where they left off after a restart. A member is one Markdown file, so a cadrei can hold any team you can describe:

- **Dev**, one team per project: a PM, an engineer, a reviewer and a designer build a feature on a branch and review it.
- **Research**: a researcher, a skeptic, a writer and an editor answer a question with sources.
- **Job search**: a recruiter, a resume coach and a mock interviewer.
- **Study**: a tutor per track that keeps track of your progress and your mistakes.
- **Ops**: one member that looks after your server.

A new cadrei starts with a dev team of an engineer and a reviewer; ask the orchestrator to add the rest. `cadrei` is the only command to learn: for everything else, you ask.

## Install

```bash
brew install rafi-ramdhani/cadrei/cadrei
cadrei
```

Without Homebrew, this puts the release binary in `~/.local/bin/cadrei` after checking its checksum:

```bash
curl -fsSL --proto '=https' https://raw.githubusercontent.com/rafi-ramdhani/cadrei/main/install.sh | sh
```

You need macOS or Linux, [Claude Code](https://claude.com/claude-code) (installed and logged in), tmux 3.2 or newer, and git. Homebrew installs tmux for you. `cadrei` checks all of this when it starts and tells you how to fix anything missing.

The first `cadrei` asks for a name and a yes or two, then opens the orchestrator. Tell it which repo to work on and what to do:

```text
work on github.com/you/app
add CSV export
```

## A first session

The first run, in a folder that is not a git repository:

```text
$ cadrei
Welcome to cadrei: a team of Claude Code sessions you lead from one conversation.
Start a new cadrei, or restore one from GitHub? [new/restore] new
Name for your cadrei? [you]
Created your cadrei you, with a dev team (an engineer and a reviewer).
Make every new Claude Code session the orchestrator? [y/N] n
problem: the cadrei skill is not linked into Claude Code (~/.claude/skills/cadrei)
Link the cadrei skill into Claude Code? [Y/n] y
  fixed
Your cadrei is ready. Tell me which repo to work on, for example: work on github.com/you/app.
The orchestrator: a new conversation.
```

Run it inside a git repository and it also offers to add that folder as a project. Then Claude Code opens as the orchestrator, and you talk. Its words vary, but a first task goes something like this:

```text
> work on github.com/you/app
  Cloned you/app into ~/Developer/app and added it for the dev team.

> add CSV export
  Started the dev team on app. The engineer is building it on a branch,
  and the reviewer will check it when the engineer reports back.

  Done: CSV export is on the branch csv-export. The reviewer found one bug
  (an empty file when there are no rows) and the engineer fixed it.
  Want me to merge it?
```

## Just ask

Everything after `cadrei` is a request in plain words to the orchestrator. Some to start with:

- "start the dev team on my-app"
- "add a designer to the dev team"
- "make a research team with a researcher, a skeptic, a writer and an editor"
- "allow the members to run `npm test`"
- "back up my cadrei to GitHub" (needs [gh](https://cli.github.com), signed in)
- "what is running?"
- "stop the dev team"

On a new machine, run `cadrei` and choose "restore": it clones your cadrei from its GitHub backup, shows what it brings in, and clones its projects after your yes.

If you would rather type, `cadrei help` lists the few commands, such as `cadrei ls` (what runs, and your projects), `cadrei attach` (watch a team or talk to it) and `cadrei stop`.

## How it works

- **Your cadrei** is a folder and a git repository, `~/.cadrei/<name>`. It holds the members, the playbook the orchestrator follows, the list of projects and the grants members get. The orchestrator commits every change, and pushes when you have a backup.
- **Members** are Markdown files, `members/<team>/<role>.md`. Each runs as a Claude Code session in tmux, one window per member, and resumes its conversation when started again.
- **Projects stay where you keep them.** The cadrei records each project's repository, and each machine records where its folder is. A member started for a project works in its folder. A team without a project works in `teams/<team>/`, which is backed up with the cadrei.
- **The orchestrator** is a Claude Code session with the cadrei skill, opened in your cadrei's folder. With the optional hook, every new Claude Code session starts as the orchestrator (`CADREI_OFF=1 claude` starts a plain one).
- **Project rules** go in each project's own `CLAUDE.md`, so every member working there follows them.

## Cost and safety

**Every member is a full Claude Code session.** Three running members use roughly three times the usage of one. The orchestrator starts only the members a task needs and offers to stop idle ones.

**Permissions stay with you.** A message from the orchestrator never counts as your consent in a member. When you tell the orchestrator "allow the members to push to main in my-app", it records that one narrow rule, and every member starts with it. Blanket rules, and rules that reach cadrei's own files, are refused, and members cannot grant themselves anything.

**The permission mode is yours to pick.** Members and the orchestrator run in the mode set in your cadrei's `cadrei.conf` (default: `default`). Choose `auto` or another mode deliberately.

**Registered projects are trusted** in Claude Code, so members start there without the trust prompt. Trust also lets a repo's own `.claude/settings.json` take effect, so add only repos you trust (or ask for `--no-trust`).

## Why not subagents or agent teams?

Subagents work inside one session and end with it. Members are full, long-lived Claude Code sessions that you can open and talk to, and they keep working across days.

Claude Code's agent teams (experimental) belong to the session that made them and go away with it. A cadrei is a standing roster in files, kept across projects and restarts.

## Coming from 0.1.x

0.2.0 is a breaking upgrade: Cadre is now Cadrei, one program instead of a git clone, and personas are now members. Install it, run `cadrei`, then tell the orchestrator "bring in my old cadre from <path>"; your old cadre is only read, never changed. The [CHANGELOG](CHANGELOG.md) has the full steps and every breaking change.

## What cadrei changes on your machine

| What | Where | `cadrei uninstall` |
|---|---|---|
| The `cadrei` program | Homebrew's `bin`, or `~/.local/bin/cadrei` | Kept: it prints the command that removes it |
| Your cadreis (members, playbook, projects list, grants, `teams/`) | `~/.cadrei/<name>/`, each a git repository | Kept |
| This machine's settings, and the skill and prompts the program writes out | `~/.cadrei/config/` and `~/.cadrei/framework/` | Removed |
| A pre-push check that keeps credentials and large files out of backups | `~/.cadrei/<name>/.git/hooks/pre-push` | Removed |
| The skill link (on your yes) | `~/.claude/skills/cadrei` | Removed, if it points at this program |
| The orchestrator hook (optional, on your yes) | one SessionStart entry in `~/.claude/settings.json` | Removed, if it runs this program |
| Trust for your projects | entries in `~/.claude.json` | Kept: shared with your own sessions |

Before its first change to `~/.claude/settings.json` or `~/.claude.json`, cadrei saves a backup beside it (ending in `.bak-cadrei`) and never overwrites it. With `CLAUDE_CONFIG_DIR` set, that folder takes the place of `~/.claude`. The hooks name the `cadrei` program only when it sits in a folder only you can write, as Homebrew and `install.sh` put it.

## Uninstall

`cadrei uninstall --dry-run` shows the plan, and `cadrei uninstall` shows it and asks first. It keeps every cadrei and project, and ends with the command that removes the program itself. To remove a cadrei too, delete its folder in `~/.cadrei`.

## More

- [docs/guide.md](docs/guide.md): writing members, the playbook, projects across machines, permissions, backup and restore, troubleshooting
- [SECURITY.md](SECURITY.md): what members can and cannot do, and how to report a problem
- [CHANGELOG.md](CHANGELOG.md): every change, and how to upgrade

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)

[![CI](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Cadrei (pronounced CAD-ray) is an independent project. It is not affiliated with or endorsed by Anthropic.
