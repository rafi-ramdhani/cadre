# Cadre

[![CI](https://github.com/rafi-ramdhani/cadre/actions/workflows/ci.yml/badge.svg)](https://github.com/rafi-ramdhani/cadre/actions/workflows/ci.yml) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**A team of Claude Code sessions that you lead from one conversation.**

Cadre turns Claude Code into a small standing team. You talk to one session, the orchestrator. It hands work to persona sessions (a PM, an engineer, a reviewer, a researcher, a tutor, whatever you define), each a full Claude Code session in its own tmux window with its own role and working folder. It passes results between them and reports back to you. When the work is interactive, you attach to a persona's window and talk to it directly.

```
you ── orchestrator ──┬── dev-my-app-pm         writes the spec
                      ├── dev-my-app-engineer   builds it on a branch
                      ├── dev-my-app-reviewer   reviews the branch
                      └── research-researcher   gathers sources
```

Cadre is a starting point, not a fixed product. `install.sh` generates **your own cadre**, a folder you name, with starter personas and a playbook. From there it grows with you: new teams, new personas, new projects, your own routing rules.

> Cadre is an independent project. It is not affiliated with or endorsed by Anthropic.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | bash -s -- my-cadre
```

One command checks your tools, generates `~/Documents/my-cadre` from the template, places the framework inside it, links the `cadre` command and the orchestrator skill, and asks whether every new Claude Code session should start as the orchestrator. Then:

```bash
cd ~/Documents/my-cadre && claude           # talk to the orchestrator
cadre add project my-app you/my-app          # register a repo and clone it into projects/
```

Ask the orchestrator for something like *"add CSV export to my-app"* and it runs the dev pipeline: spec, build, review, fix.

**On a new machine**, restore your cadre and every project in it:

```bash
curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | bash -s -- --from you/my-cadre
```

Installer options: `--dir <parent>` for another location, `--orchestrator-default` or `--no-hook` to answer the hook question up front, `--yes` to ask nothing, `--link-only` to relink after moving the framework. `install.sh --help` lists them all.

### Requirements

- [Claude Code](https://claude.com/claude-code) with cross-session messaging (`SendMessage`, `ListAgents`) and the `--name` and `--append-system-prompt-file` flags
- macOS or Linux with bash, git, tmux and python3
- jq, for the optional orchestrator hook
- gh (optional), for cloning private repos

## What you get

```
my-cadre/                     your cadre: its own git repo, yours to grow
├── playbook.md               your teams, pipelines and routing rules
├── projects.yaml             the project registry
├── personas/<team>/<role>.md one persona per file
├── teams/<team>/             notes and outputs of teams without a project
├── cadre.conf                settings, such as the persona permission mode
└── projects/                 your project repos, each still its own git repo
    ├── my-app/
    └── cadre/                the framework itself
```

- **Everything you work on lives in one folder.** Projects are cloned into `projects/` but stay independent repos; your cadre's git ignores that folder and records only the links in `projects.yaml`. `cadre sync` re-clones them anywhere.
- **The playbook is yours.** It tells the orchestrator which team handles what and in which order. The starter version has a dev team (pm, engineer, reviewer, designer) and a research team (researcher, skeptic, writer, editor).
- **The framework stays generic.** It lives in `projects/cadre` and updates with `cadre update`; your personas and playbook are never touched.

## Commands

```bash
cadre ls                        # teams, roles and what is running
cadre projects                  # the registry
cadre up dev my-app             # the dev team, working inside projects/my-app
cadre up dev/engineer my-app    # one persona
cadre up research               # a team without a project
cadre attach dev my-app         # watch a team or talk to it
cadre down dev my-app           # stop it
cadre down --all                # stop every cadre session (asks first; --yes skips)
cadre add project <name> <repo> [team] [about]   # also trusts its folder (--no-trust skips)
cadre trust <project> | --all   # trust registered projects added before 0.2.0
cadre allow add '<rule>'        # grant a narrow permission to every persona (list, remove, --once, --auto)
cadre add team <team>
cadre add persona <team>/<role>
cadre sync                      # clone registry projects missing on this machine, and trust them
cadre init <name>               # generate another cadre from the template
cadre use <dir>                 # switch the active cadre
cadre update                    # update the framework (--check only looks)
cadre version
cadre uninstall                 # undo the install (asks first; --dry-run shows the plan)
```

The orchestrator runs these for you; they are there when you want to drive by hand.

## How it works

- **Personas** are Markdown files. At launch, the framework's shared protocol (how to receive work and reply), your cadre's house rules and the persona are joined into one prompt and passed to `claude --append-system-prompt-file`, so each persona keeps the full Claude Code toolset and your `CLAUDE.md`.
- **Sessions** run in tmux: `cadre-<team>` or `cadre-<team>-<project>`, one window per role, each named `<team>[-<project>]-<role>` so the orchestrator can address it with `SendMessage`.
- **The orchestrator** is any Claude Code session using the `cadre` skill. With the optional SessionStart hook, every new session starts as the orchestrator; persona sessions and sessions started with `CADRE_OFF=1 claude` are skipped.
- **Project rules** belong in each project's own `CLAUDE.md`, so every persona working there follows them.

See [docs/guide.md](docs/guide.md) for writing personas, shaping the playbook, the registry format, and troubleshooting.

## Costs and permissions

**Every persona is a full Claude Code session.** Five running personas use roughly five times the usage of one. The orchestrator starts only the personas a task needs and offers to stop them afterwards.

**Permission mode is your call.** Personas run in the mode set in `cadre.conf` (default: `default`). Cross-session messages are delivered without approval only when the orchestrator and the persona run in the same permission-mode class; otherwise they wait for you in the persona's window. Choose `auto` or another mode deliberately, knowing the personas act on messages from the orchestrator.

**Grants reach personas through `cadre allow`.** When you tell the orchestrator "allow the personas to push to main in my-app", consent stays in the orchestrator: a message from it never counts as your consent in a persona. `cadre allow add 'Bash(git push origin HEAD:main)'` records the rule in your cadre's persona settings file, which every persona starts with. It refuses blanket rules (`*`, a bare `Bash`, `Edit`, `Write`, `Read`, `WebFetch`, `NotebookEdit` or `PowerShell`, a lone wildcard, a whole MCP server, a wildcard in the program name, a shell, interpreter or wrapper with a wildcard) and anything that targets the settings or `cadre allow` itself, warns about other wildcards, and commits every change. Personas cannot run it. `--once` marks a grant for removal after the task, and `--auto` adds a plain-English allowance for auto mode.

**Registered projects are trusted.** `cadre add project` and `cadre sync` mark each project's folder as trusted in Claude Code, so personas start there without the trust prompt. Trust also lets that repo's own `.claude/settings.json` rules and hooks take effect, so register only repos you trust, or pass `--no-trust`.

## Upgrading from 0.1.x

0.1.x has no `cadre update`, so the first upgrade is one command by hand (your cadre's path is the first line of `~/.config/cadre/home`):

```bash
git -C "$(head -1 ~/.config/cadre/home)/projects/cadre" pull --ff-only
cadre version    # cadre 0.2.0
```

If the pull fails because you changed files in the framework folder, keep them on a branch first (`git switch -c my-changes && git commit -am "My local changes" && git switch main`), then pull. Then restart your running sessions (`cadre down <team> [project]` and `cadre up <team> [project]`, and a new orchestrator session), and optionally run `cadre trust --all` to trust the projects you registered before. The [CHANGELOG](CHANGELOG.md) has the full steps. From then on, `cadre update` does it.

## Uninstall

`cadre uninstall` shows its plan, asks, then stops every cadre session and removes what the installer wired in: the `cadre` command and skill links (only when they point to this framework), the orchestrator hook in `~/.claude/settings.json` (backup: `settings.json.bak-cadre-uninstall`), `~/.config/cadre/` and the build cache. It keeps your cadre folder, your projects, the framework clone, backups and Claude Code trust entries, and prints their paths; delete those by hand if you want them gone. `--dry-run` shows the plan only, `--yes` skips the question.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Run `tests/smoke.sh` before sending a change; it needs no Claude account and never touches your own setup.

## License

[MIT](LICENSE)
