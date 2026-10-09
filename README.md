# Cadrei

**A team of Claude Code helpers you lead from one chat.**

Run `cadrei` and you get one Claude Code chat to talk to: the **orchestrator**. It hands your work to **members**, other copies of Claude Code that each do one job. Members remember where they left off. Each one is just a short text file (Markdown), so you can build any team you can describe:

- **Dev**, one team per project: a PM, an engineer, a reviewer and a designer build a feature on a branch and review it.
- **Research**: a researcher, a skeptic, a writer and an editor answer your question, with sources.
- **Job search**: a recruiter, a resume coach and a mock interviewer.
- **Study**: a tutor per track that keeps tabs on your progress (and your mistakes).
- **Ops**: one member that looks after your server.

Your members, projects and settings together are your **cadrei**. You start with an engineer and a reviewer. Want more? Just ask.

## Install

```bash
brew install rafi-ramdhani/cadrei/cadrei
cadrei
```

No Homebrew? Use this instead:

```bash
curl -fsSL --proto '=https' https://raw.githubusercontent.com/rafi-ramdhani/cadrei/main/install.sh | sh
```

You'll need macOS or Linux, [Claude Code](https://claude.com/claude-code) (logged in), git, and tmux 3.2 or newer (a terminal tool that keeps your members running in the background). Homebrew installs tmux for you.

The first `cadrei` asks a few quick questions, then opens the orchestrator. Tell it your repo and what you want:

```text
work on github.com/you/app
add CSV export
```

## Your first run

```text
$ cadrei
Welcome to cadrei: a team of Claude Code helpers you lead from one chat.
Start a new cadrei, or restore one from GitHub? [new/restore] new
Name for your cadrei? [you]
Created your cadrei you, with a dev team (an engineer and a reviewer).
...
Your cadrei is ready. Tell me which repo to work on, for example: work on github.com/you/app.
```

Then you just talk. The wording changes every time, but it goes something like this:

```text
> add CSV export
  Started the dev team on app. The engineer is building it on a branch.

  Done: CSV export is on the branch csv-export. The reviewer found one bug
  and the engineer fixed it. Want me to merge it?
```

## Just ask

Everything after `cadrei` is a chat with the orchestrator. Try:

- "add a designer to the dev team"
- "allow the members to run `npm test`"
- "back up my cadrei to GitHub"
- "what is running?"
- "stop the dev team"

New machine? Run `cadrei` and pick "restore".

Want to watch a member, or answer a question it's asking you? `cadrei attach <team> <project>` opens the team's windows, like `cadrei attach dev app`. `cadrei ls` shows what's running, and `cadrei help` lists the few other commands.

## How it works

- **Your cadrei** is a folder, `~/.cadrei/<name>`, kept in git. It holds your members, your projects list and the permissions you've given.
- **Members** run in tmux, one window each, and pick up where they left off.
- **Your projects stay where they are.** Members work right in their folders.
- **Outside that folder**, cadrei adds a link in `~/.claude/skills` so Claude Code can find its instructions. If you say yes on the first run, every new Claude Code chat also starts as the orchestrator (`CADREI_OFF=1 claude` gets you a plain one). [Here's everything it touches](docs/guide.md#where-things-live).

## Cost and safety

- **Each member is its own Claude Code**, so three members use about three times as much of your Claude plan as one. The orchestrator only starts the ones a task needs.
- **You stay in charge.** Members ask you before doing anything risky. To let them do something, tell the orchestrator. It refuses catch-all permissions.
- **Projects you add are marked as trusted** in Claude Code, so members skip the "do you trust this folder?" question. It also means the repo's own Claude Code settings and hooks run, so only add repos you trust.

## Why not subagents?

Subagents live inside one chat and end with it. Members stick around: open one, talk to it, and it's still on the job tomorrow.

## Coming from 0.1.x

0.2.0 is a fresh start: Cadre is now Cadrei, and personas are now members. Install it, run `cadrei`, then say "bring in my old cadre from <path>". Your old cadre folder is only read, never changed. See the [CHANGELOG](CHANGELOG.md) for details.

## Uninstall

`cadrei uninstall` shows what it will remove and asks first. Your cadreis and projects stay. [The guide](docs/guide.md#uninstalling) has the details.

## More

- [Guide](docs/guide.md): members, projects, permissions, backups, troubleshooting, and everything cadrei puts on your machine
- [Security](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Contributing](CONTRIBUTING.md)

[MIT license](LICENSE). Cadrei (say it CAD-ray) is an independent project, not affiliated with or endorsed by Anthropic.

[![CI](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadrei/actions/workflows/ci.yml)
