# Cadrei

**A team of Claude Code helpers you lead from one chat.**

You run `cadrei` and get one Claude Code chat to talk to: the **orchestrator**. It passes your work on to **members**, other copies of Claude Code that each do one job and pick up right where they left off after a restart. A member is just a Markdown file, so if you can describe a team, your cadrei (your whole crew) can hold it:

- **Dev**, one team per project: a PM, an engineer, a reviewer and a designer build a feature on a branch and review it.
- **Research**: a researcher, a skeptic, a writer and an editor answer your question, with sources.
- **Job search**: a recruiter, a resume coach and a mock interviewer.
- **Study**: a tutor per track that keeps tabs on your progress (and your mistakes).
- **Ops**: one member that looks after your server.

You start with a dev team of an engineer and a reviewer. Want more? Ask the orchestrator. `cadrei` is the only command you need to learn.

## Install

```bash
brew install rafi-ramdhani/cadrei/cadrei
cadrei
```

No Homebrew? This downloads the program, makes sure it's the exact file that was released, and puts it in `~/.local/bin/cadrei`:

```bash
curl -fsSL --proto '=https' https://raw.githubusercontent.com/rafi-ramdhani/cadrei/main/install.sh | sh
```

You'll need macOS or Linux, [Claude Code](https://claude.com/claude-code) (installed and logged in), git, and tmux 3.2 or newer (tmux keeps your members running in the background). Homebrew brings tmux along. Missing something? `cadrei` checks when it starts and tells you how to fix it.

The first `cadrei` asks for a name and a yes or two, then opens the orchestrator. Tell it which repo you're on and what you want done:

```text
work on github.com/you/app
add CSV export
```

## Your first run

Here's the first run, in a folder that isn't a git repo:

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

The two yes/no questions: the first makes every Claude Code chat you open start as the orchestrator (handy, but optional). The second hooks up the cadrei skill, the instructions that teach Claude Code how to run your team, so say yes to that one. Run it inside a git repo and it also offers to add that folder as a project.

Then Claude Code opens as the orchestrator and you just talk. It's Claude, so the wording changes every time, but a first task looks roughly like this:

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

Anything past `cadrei` is just you talking to the orchestrator. A few to try:

- "start the dev team on my-app"
- "add a designer to the dev team"
- "make a research team with a researcher, a skeptic, a writer and an editor"
- "allow the members to run `npm test`"
- "back up my cadrei to GitHub" (you'll need GitHub's command-line tool, [gh](https://cli.github.com), signed in)
- "what is running?"
- "stop the dev team"

New machine? Run `cadrei` and pick "restore". It downloads your cadrei from its GitHub backup, shows you what's coming in, and downloads your projects once you say yes.

Rather type than talk? `cadrei help` lists the handful of commands, like `cadrei ls` (what's running, and your projects), `cadrei attach` (watch a team work, or jump in) and `cadrei stop`.

## How it works

- **Your cadrei** is a folder, `~/.cadrei/<name>`, kept in git. It holds your members, the playbook (your notes for the orchestrator on who handles what), your projects list, and the permissions you've given members. The orchestrator saves every change in git, and sends it to GitHub too once you've set up a backup.
- **Members** are Markdown files in `members/<team>/<role>.md`. Each one runs in its own tmux window and picks its conversation back up when it starts again.
- **Your projects stay put.** The cadrei remembers each project's repo, and each machine remembers where the folder lives. A member working on a project works in that folder. A team without a project works in `teams/<team>/`, which gets backed up with the cadrei.
- **The orchestrator** is Claude Code plus cadrei's instructions, opened in your cadrei's folder. If you said yes to that first question, every new Claude Code chat starts as the orchestrator (`CADREI_OFF=1 claude` gets you a normal one).
- **Project rules** go in each project's own `CLAUDE.md`, so every member working there follows them.

## Cost and safety

**Every member is its own Claude Code.** Three members running use roughly three times what one would. The orchestrator only starts the members a task needs, and offers to stop the ones sitting idle.

**You hold the keys.** Members ask you, not the orchestrator, before doing anything that needs your OK, and a message from the orchestrator never counts as your yes. Say "allow the members to push to main in my-app" and the orchestrator records that one specific permission, which every member then starts with. Catch-all permissions (like "run any command") and anything that would touch cadrei's own files get refused, and members can't give themselves permissions.

**You pick how often Claude asks.** Members and the orchestrator use the Claude Code permission mode set in your cadrei's `cadrei.conf` (default: `default`, which asks before edits and commands). Switch to `auto` or another mode on purpose, not by accident.

**Projects you add are marked as trusted** in Claude Code, so members don't get stuck on the "do you trust this folder?" question. That also means the repo's own `.claude/settings.json` gets applied, so only add repos you trust (or ask for `--no-trust`).

## Why not subagents or agent teams?

Subagents (the helpers Claude Code spins up for one task) live inside one chat and end with it. Members are full copies of Claude Code that stick around: you can open one, talk to it, and come back tomorrow to find it still on the job.

Claude Code's agent teams (an experimental feature) belong to the chat that made them and go away with it. Your cadrei's team lives in files and sticks around across projects and restarts.

## Coming from 0.1.x

Heads up, 0.2.0 doesn't upgrade your old setup in place: Cadre is now Cadrei, it's one program instead of a git clone, and personas are now members. Install it, run `cadrei`, then tell the orchestrator "bring in my old cadre from <path>". Your old cadre is only read, never changed. The [CHANGELOG](CHANGELOG.md) has the full steps and every change you need to know about.

## What cadrei changes on your machine

No surprises, here's everything:

| What | Where | `cadrei uninstall` |
|---|---|---|
| The `cadrei` program | Homebrew's `bin`, or `~/.local/bin/cadrei` | Kept: it prints the command that removes it |
| Your cadreis (members, playbook, projects list, permissions, `teams/`) | `~/.cadrei/<name>/`, each one a git repo | Kept |
| This machine's settings, and cadrei's instructions for Claude Code | `~/.cadrei/config/` and `~/.cadrei/framework/` | Removed |
| A check that stops passwords, keys and huge files from going into your backup | `~/.cadrei/<name>/.git/hooks/pre-push` | Removed |
| The link that gives Claude Code cadrei's instructions (on your yes) | `~/.claude/skills/cadrei` | Removed, if it points at this program |
| "Start every chat as the orchestrator" (optional, on your yes) | one startup entry in `~/.claude/settings.json` | Removed, if it runs this program |
| The "trusted folder" marks for your projects | entries in `~/.claude.json` | Kept: your own Claude Code uses them too |

Before cadrei first touches `~/.claude/settings.json` or `~/.claude.json`, it saves a backup next to it (ending in `.bak-cadrei`) and never overwrites that backup. If you've moved Claude Code's settings folder with `CLAUDE_CONFIG_DIR`, read that folder wherever this says `~/.claude`. The backup check and the startup entry only run the `cadrei` program when it sits in a folder only you can write to, which is where Homebrew and `install.sh` put it.

## Uninstall

`cadrei uninstall --dry-run` shows you the plan. `cadrei uninstall` shows it too, then asks before doing anything. Your cadreis and projects stay, and it finishes by printing the command that removes the program itself. Want a cadrei gone as well? Delete its folder in `~/.cadrei`.

## More

- [docs/guide.md](docs/guide.md): writing members, the playbook, projects across machines, permissions, backup and restore, troubleshooting
- [SECURITY.md](SECURITY.md): what members can and can't do, and how to report a problem
- [CHANGELOG.md](CHANGELOG.md): every change, and how to upgrade

## Contributing

Issues and pull requests are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)

[![CI](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Cadrei (say it CAD-ray) is an independent project. It's not affiliated with or endorsed by Anthropic.
