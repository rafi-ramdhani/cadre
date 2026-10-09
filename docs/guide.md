# Cadrei guide

You can use cadrei without reading this: run `cadrei`, then talk to the orchestrator. This guide is for when you want to know what happens underneath, shape your cadrei by hand, or fix something.

## Concepts

| Term | What it is |
|---|---|
| **Cadrei** | Your folder, `~/.cadrei/<name>`, and a git repository. It holds the members, the playbook, the projects list and the grants. |
| **Orchestrator** | The Claude Code session you talk to. `cadrei` opens it. It follows the cadrei skill and your playbook, and hands work to members instead of doing it itself. |
| **Member** | A long-lived Claude Code session with one role, started from `members/<team>/<role>.md`. |
| **Team** | A folder of members, started together or one at a time. A new cadrei has one: `dev`, with an `engineer` and a `reviewer`. |
| **Project** | A git repository the cadrei works on. The cadrei records its repository; each machine records where its folder is. |

## Where things live

```
~/.cadrei/
  <name>/                     a cadrei (a git repository)
    playbook.md               teams, pipelines and routing rules
    projects.yaml             each project's repo, team and about (no local paths)
    cadrei.conf                settings (KEY=VALUE lines)
    protocol.md               optional house rules for every member
    members/<team>/<role>.md  one member per file
    teams/<team>/             working folders of teams without a project (tracked)
    .claude/
      member-settings.json    the grants every member starts with (tracked)
      member-settings.once    which grants are one-time (tracked)
      build/                  generated: prompts, settings copies, conversation records, locks (ignored)
  config/                     this machine only, in no repository
    default                   the default cadrei's name
    projects-dir              where new clones go
    places/<name>.json        where each of that cadrei's projects is on this machine
    member-settings.sha256    fingerprints of the grants files
    state.json                the health check's state
  framework/                  the skill and prompts the program writes out, with a VERSION file
```

`config` and `framework` are reserved names; a cadrei's name may use letters, digits, `-` and `_`.

## Writing a member

Ask the orchestrator ("add an SRE to an ops team"), or write the file yourself. A good member file is short and concrete:

```markdown
# Member: Site Reliability Engineer (ops team)

You are the SRE. You keep the services up and explain what broke.

- Read the logs and metrics before you form a theory.
- Change production only when the orchestrator's task says so.
- In your reply: what you found, what you changed, how you verified it.
```

- **One role, plainly stated.** What the member owns and what it does not do.
- **How to work**, in a few bullets. Concrete habits beat adjectives.
- **What the reply contains.** The orchestrator relays it, so make it easy to relay.
- Leave out how to receive work and reply: the shared protocol covers that.

Team and member names may use letters, digits, `-` and `_`. Add the team to `playbook.md` so the orchestrator knows when to use it, and commit the change (the orchestrator does both when it adds a member).

### Working folders

- A member started for a project (`cadrei up dev my-app`) works in that project's folder.
- A team without a project works in `teams/<team>/`, unless `members/<team>/.workdir` names another folder.
- `members/<team>/<role>.workdir` pins one member to its own folder, for example a tutor per study track.

A `.workdir` file holds one path; `~` is expanded.

`teams/` is tracked in the cadrei's git, so the work members keep there is backed up with the cadrei. The cadrei's `.gitignore` leaves out dependency folders, logs and `.env` files (their `.env.example`-style templates stay).

## The playbook

`playbook.md` is read by the orchestrator at the start of every session and wins over the generic skill where they differ. Put there:

- **Teams**: who exists and what each team is for.
- **Pipelines**: multi-step flows, for example *engineer builds, reviewer reviews, engineer fixes*.
- **Routing rules**: which requests go where, and anything the orchestrator must never do itself.
- **Project notes** that do not fit elsewhere, such as how deploys work.

Rules that belong to one project go in that project's own `CLAUDE.md` instead, so every member working there follows them.

## House rules

`protocol.md` in your cadrei (optional) is added to the shared member protocol for every member. Use it for rules that apply to all members, for example a writing style.

## Projects

`projects.yaml` is portable: it holds no local paths, so the same cadrei works on any machine.

```yaml
my-app:
  repo: you/my-app        # owner/repo, a git URL, or empty for a folder with no remote
  team: dev               # the team that usually handles it
  about: The main product # one line for the orchestrator
```

Where each project is on this machine is kept in `~/.cadrei/config/places/<cadrei>.json`. Projects stay wherever you keep them; nothing is cloned into a cadrei. The orchestrator runs these commands when you ask in plain words ("work on github.com/you/app", "my blog is in ~/src/blog"):

| Command | What it does |
|---|---|
| `cadrei project add <name> <repo> [team] [about]` | clone into your projects folder, record the place, trust the folder |
| `cadrei project add <name> --path <dir> [--repo <repo>]` | register a git folder you already have (its repo is taken from `origin`) |
| `cadrei project link <name> <dir>` | set or change where a project's folder is on this machine |
| `cadrei project unlink <name> [--untrust]` | remove a project from the cadrei; **the folder is never touched** |
| `cadrei project sync` | clone every project that has a repo but no folder on this machine |
| `cadrei project trust <name> \| --all` | trust project folders in Claude Code |
| `cadrei project path <name>` | print a project's folder |
| `cadrei project dir [<dir>]` | show or set the projects folder (asked once per machine; `~/Developer` is suggested) |

Project names may use letters, digits, `.`, `-` and `_`. One project may belong to several cadreis.

**Refused folders.** Add, link and trust refuse `/`, your home folder, anything inside or containing `~/.cadrei`, a cadrei's team folder, `~/.claude`, and the top folder of a 0.1.x cadre (its `projects/<name>` folders can be linked). Trust also needs the top folder of a git repository.

**Missing projects.** When a project's folder is gone, cadrei never unlinks it by itself. `cadrei ls` marks it, and the orchestrator offers to clone it again, point to its new folder (suggesting a repository with the same `origin` in your projects folder), or unlink it. A folder on a drive that is not connected says so. `cadrei up` and `cadrei attach` refuse a missing project.

**Trust.** Adding, linking and cloning a project marks its folder as trusted in Claude Code (`~/.claude.json`, or `$CLAUDE_CONFIG_DIR/.claude.json`), so members start there without the trust prompt. Cadrei records the physical path and, when different, the path as you wrote it. The edit keeps every other key and their order, keeps a backup of the original at `.claude.json.bak-cadrei` (the first one is never overwritten), writes atomically, checks again just before writing (a running session may rewrite the file), and skips with a warning on any doubt. Trust also lets that repository's own `.claude/settings.json` rules and hooks take effect, so add only repositories you trust, or pass `--no-trust`. The orchestrator's folder, `~/.cadrei/<name>`, is not trusted in advance: Claude Code asks once.

## Sessions

| Command | What it does |
|---|---|
| `cadrei` | the orchestrator in this terminal (`--fresh` starts a new conversation) |
| `cadrei --tmux [--detach]` | the orchestrator in tmux, which keeps running when you close the terminal and can be reached over SSH |
| `cadrei up <team>[/<role>] [project] [--fresh]` | start a team or one member (the orchestrator does this) |
| `cadrei attach [team] [project]` | watch or talk to a team; with no team, the orchestrator in tmux |
| `cadrei stop <team>[/<role>] [project]` | stop a team or one member |
| `cadrei stop [--all] [--yes]` | with no team, every member of this cadrei (`--all`: of every cadrei), after a confirmation; never the orchestrator |
| `cadrei ls [--all] [--json]` | the cadrei, the orchestrator, running teams, projects (missing ones marked) and other cadreis |

**Names.** A team runs in tmux session `cadrei-<cadrei>-<team>` (or `cadrei-<cadrei>-<team>-<project>`), one window per role. Each member's Claude Code session is named `<cadrei>-<team>[-<project>]-<role>`, which is the address the orchestrator sends work to. In tmux session names a `.` becomes `_`, as older tmux versions do it themselves: project `my.app` runs in `cadrei-w-dev-my_app`, while its members keep the dot (`w-dev-my.app-engineer`). `cadrei up` refuses when another cadrei already uses the name. Cadrei sessions show the key that takes you back to your terminal in their status line (your tmux prefix, then `d`).

**One orchestrator per cadrei.** `cadrei` records the orchestrator in `.claude/build/orchestrator.lock`. When it is already open in another terminal, `cadrei` says so and exits; when it runs in tmux, `cadrei` attaches to it. A lock left by a session that ended is cleared. An orchestrator you started by hand (through the hook) is not detected.

**Conversations resume.** Every start records the member's Claude Code session in `.claude/build/sessions/`, kept current across `/clear` and `/compact`. The next `cadrei up` (and the next `cadrei`) resumes that conversation. It starts a new one, and says why, when there is no record, the working folder changed, the conversation is gone, or resuming fails. `--fresh` on `cadrei`, `cadrei up` and `cadrei stop` starts clean.

**Sessions started by 0.1.x** show as legacy in the default cadrei's `cadrei ls`, and `cadrei stop` stops them.

## The orchestrator hook

The first run offers a SessionStart hook in `~/.claude/settings.json` that makes every new Claude Code session an orchestrator. Its `cadrei` commands act on the cadrei for its folder (your default cadrei outside every cadrei and project). It runs `<cadrei program> hook orchestrator`, and stays silent in member sessions, in an orchestrator `cadrei` opened, and in sessions started with `CADREI_OFF=1 claude`. Without the hook, run `cadrei`, or ask any session to use the cadrei skill.

The hook is added with a backup of the file (`settings.json.bak-cadrei`), keeping every other setting. `cadrei uninstall` removes it.

## Permissions for members

Each member is its own Claude Code session and asks for its own permissions. A message from the orchestrator never counts as your consent there. To grant something to every member, tell the orchestrator ("allow the members to run npm test"), which runs `cadrei allow`:

```bash
cadrei allow add 'Bash(git push origin HEAD:main)'      # an exact command
cadrei allow add --once 'Bash(npm publish)'             # for one task; removed afterwards
cadrei allow add --auto "Merging a reviewed branch into main in my projects is expected"
cadrei allow list                                       # numbered, with kind, once and wildcard marks
cadrei allow remove 'Bash(npm publish)'                 # by its exact text, or its list number
cadrei allow remove --once                              # every one-time grant
```

- A rule is a Claude Code permission rule: a tool name with an optional specifier, such as `Bash(npm test)`, `Read(./docs/**)` or `mcp__github__create_issue`.
- `--auto` adds a sentence (one line, at most 300 characters, plain ASCII letters) to the auto-mode classifier's allow list, after `"$defaults"`, so the built-in rules stay. Describe the work that is expected; a sentence about permissions, settings or grants, or one claiming your approval, is refused.
- **Refused**: blanket rules (`*`, a bare `Bash`, `Edit`, `Write`, `Read`, `WebFetch`, `NotebookEdit` or `PowerShell`, a specifier that is only a wildcard), whole MCP servers, wildcards in the program name, shells, interpreters and wrappers with a wildcard, programs that run whatever follows a subcommand (`docker run`, `npm exec`, `go run` and the like) with a wildcard, chained or backgrounded commands, commands built with shell syntax, `WebFetch` for every domain, `Edit` and `Read` paths that climb with `..`, startup files and files that run code outside a session (shell and git config, `~/.ssh`, launch agents, `~/.local/bin` and the like), and anything that reaches cadrei's own files: the grants file, `cadrei allow`, `cadrei.conf`, `~/.cadrei/config`. For `Edit` and `Read` rules, paths are read the way Claude Code reads them, so glob classes, escapes and braces cannot hide a refused file. `Bash` rules are checked word by word: cadrei reads the paths, variables such as `$CADREI_HOME` and `..` climbs in them, but it cannot see where a link on disk leads, so a shell rule is a weaker guard than an `Edit` rule. The full tables are in `internal/runtime/claude/allow`.
- **Warned**: other wildcards, `git` with a wildcard (it can run other programs), `make`, `npm run`, `pip install` and `./script` with a wildcard (they run code from files a member can change), and `Read` rules that reach secrets such as `~/.ssh` or `~/.aws`. Accepted wildcards are marked in `list`.
- Every change is committed in your cadrei (`git log -- .claude/` is the record) and travels with it. One-time grants are listed in `.claude/member-settings.once` with the time they were added.
- Members cannot add or remove grants.
- A running member keeps the grants it started with. `cadrei allow` lists the members to restart; with resume, a restart keeps the conversation.

### The grants file

Every member starts with the grants in `.claude/member-settings.json`, a Claude Code settings file. Members never get the file itself: before each start, cadrei validates it and writes a read-only copy to `.claude/build/`. The file may hold only `permissions.allow` and `deny` and `autoMode.allow` and `soft_deny`, with no duplicate keys, `"$defaults"` in every `autoMode` list, and the fixed entries. A file that breaks these rules gives members a copy with no grants, and a warning.

Every copy also gets fixed entries that keep members off cadrei's own files: `Edit` denies for `~/.cadrei/config`, `~/.cadrei/framework`, and each cadrei's `.claude`, `cadrei.conf`, `members`, `playbook.md`, `protocol.md`, `projects.yaml` and `.git`; a deny for `cadrei allow`; and a `soft_deny` line telling the auto-mode classifier that only the user changes these, through the orchestrator. Team folders stay writable.

A fingerprint in `~/.cadrei/config/member-settings.sha256` flags a file changed outside `cadrei allow`. A change that arrives as a commit `cadrei allow` made (for example pulled from another machine) is accepted. To undo a hand edit, run `git -C ~/.cadrei/<name> checkout -- .claude/member-settings.json`.

## Backup and restore

Your cadrei is a git repository, and the orchestrator commits every change to it. Say "back up my cadrei to GitHub": the orchestrator shows the repository it will create (`<your GitHub user>/cadrei-<name>`, private) and, on your yes, creates it with `gh` and pushes. From then on it pushes after each commit.

On a new machine, run `cadrei` and choose "restore". Cadrei clones the cadrei into `~/.cadrei/<name>` and then asks before anything takes effect:

- A backup with a link where cadrei reads its files or runs sessions (members, `teams/`, `.claude/`, the top folder) is not kept: cadrei would follow the link out of the cadrei.
- Files that Claude Code loads into the orchestrator once you trust the cadrei's folder (`.claude/settings.json` and `settings.local.json`, `.claude/commands`, `agents`, `skills` and `output-styles`, `.mcp.json`, `CLAUDE.md`, `CLAUDE.local.md`) are listed, and the orchestrator opens only after your yes. Cadrei never writes these itself; on a no, look at them and remove any you did not put there.
- The projects are listed with their repositories, and cloned and trusted only after your yes ("Clone these N projects and trust them in Claude Code?"). On a no they stay "not on this machine", for the orchestrator to offer later.

Projects with no repo are listed as missing until you link them.

| Travels with the cadrei | Stays on each machine |
|---|---|
| members, playbook, `projects.yaml`, `cadrei.conf`, `protocol.md`, grants, `teams/` | project folders and where they are, `~/.cadrei/config`, `.claude/build` |

Claude Code's conversations and memory live in Claude Code's own folder, not in the cadrei, so on a new machine members start new conversations with the same setup. Grants that name a local path travel as written and apply only where that path exists.

**No credentials in a backup.** Cadrei writes no tokens into a cadrei. A pre-push hook in each cadrei repository (`.git/hooks/pre-push`, running `cadrei hook pre-push`) refuses a push that carries files that look like credentials (`.credentials.json`, `.env` files, private keys, and tokens such as `sk-ant-`, `ghp_` or `github_pat_`) or files over 50 MB, and names each one. A pre-push hook of your own is left alone, and the health check says so.

## Several cadreis

Most people need one cadrei. When you have more (`cadrei init <name>` creates one; `cadrei use <name>` makes it the default), a command acts on:

1. the cadrei in `CADREI_HOME`, when that is set;
2. otherwise the cadrei whose folder you are in (`~/.cadrei/<name>`);
3. otherwise the cadrei of the project you are in (when several cadreis link it, cadrei asks; without a terminal it refuses and says how to choose);
4. otherwise the default cadrei.

Cadrei reads configuration only from `~/.cadrei` or `CADREI_HOME`, never from the folder you happen to be in. Members and the orchestrator start with `CADREI_HOME` set, so changing folders never switches their cadrei. Session names carry the cadrei's name, so two cadreis can each run a `dev` team. `cadrei ls --all` and `cadrei stop --all` cover every cadrei.

## Configuration

`cadrei.conf` is read as `KEY=VALUE` lines, never run. Other lines are ignored with a warning.

| Setting | Default | Meaning |
|---|---|---|
| `PERMISSION_MODE` | `default` | the permission mode of the members and the orchestrator |

Cross-session messages reach a member without your approval only when the orchestrator and the member run in the same permission-mode class; otherwise they wait for you in the member's window.

Environment variables: `CADREI_HOME` (use another cadrei for one command), `CADREI_PERMISSION_MODE` (override the mode), `CADREI_OFF=1` (a plain Claude Code session, not an orchestrator), `CADREI_TMUX_SOCKET` (run sessions on a separate tmux server), and Claude Code's own `CLAUDE_CONFIG_DIR`.

## The health check

Every `cadrei` checks the setup and stays silent unless something is wrong. Quick checks run every time; full checks run on the first run, after an upgrade, when a quick check fails, and on `cadrei --check`. They cover Claude Code (installed and logged in), tmux and git, the skill link and the hook pointing at this program, the `cadrei` first on your `PATH` being this one, a stray `settings.json` in a cadrei's `.claude/`, each cadrei's pre-push hook, the skill files (put back when something changed them), and missing projects (as information). Leftovers from 0.1.x (the old hook, a skill link and a `cadrei` link into the old clone) are recognized. Each problem comes with its fix, and nothing outside cadrei's own files is changed without your yes.

## Upgrading

`brew upgrade cadrei`, or run `install.sh` again. The next `cadrei` runs the full health check and rewrites `~/.cadrei/framework`. Running members keep the old protocol until they restart (`cadrei stop`, then let the orchestrator start them again; conversations resume). Read the [CHANGELOG](../CHANGELOG.md) for anything to know.

## Uninstalling

`cadrei uninstall --dry-run` prints the plan. `cadrei uninstall` prints it, asks, then:

1. stops every cadrei member session, and the orchestrator sessions in tmux last;
2. removes the skill link and the orchestrator hook, only where they point at this program (other hooks and settings stay; the backup made before cadrei first changed the settings file, `settings.json.bak-cadrei`, is kept, and no new one is written);
3. removes each cadrei's pre-push hook;
4. removes `~/.cadrei/config` and `~/.cadrei/framework`.

It keeps every cadrei and project, trust entries and backups, and ends with the command that removes the program (`brew uninstall cadrei`, or the `rm` for an `install.sh` install). An orchestrator open in a terminal is named so you can close it. Members cannot run it.

## Troubleshooting

- **A member never replies.** `cadrei attach <team> [project]` and look at its window. A new folder may show Claude Code's trust prompt; `cadrei project trust <name>` avoids it next time.
- **The trust prompt still shows for a project.** Run `cadrei project trust <name>`. A Claude Code session running during the change may have written its own copy of `~/.claude.json` over it; accept the prompt once, or run the command again with no sessions running. If you set `CLAUDE_CONFIG_DIR`, members use the value the tmux server started with.
- **A member reports a blocked action.** Decide whether to allow it. If yes, tell the orchestrator to allow that exact action; it restarts the member when needed.
- **Messages wait for approval.** The member runs in a different permission-mode class from the orchestrator. Set `PERMISSION_MODE` in `cadrei.conf` to the mode you want for both.
- **"The orchestrator is already open".** It runs in another terminal; switch to it, or close it first.
- **A project shows as missing.** Ask the orchestrator to clone it again or point to its new folder, or run `cadrei project sync` or `cadrei project link <name> <dir>`.
- **After switching Claude accounts.** Run `cadrei stop`, close the orchestrator, and run `cadrei` again. Conversations resume.
