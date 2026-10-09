# Security

## Reporting a vulnerability

Please report security problems privately through GitHub's "Report a vulnerability" button on this repository's Security tab, not in a public issue. You should get a reply within a week.

## How cadre is built to be safe

Cadre runs several Claude Code sessions for you. Each member is a full Claude Code session that acts on messages from the orchestrator, so the design keeps three things in your hands: what members may do, cadre's own files, and the files cadre shares with Claude Code.

### Consent comes only from you, in the orchestrator

- Only what you type in the orchestrator session, or your answers to its own questions, counts as your consent. A cross-session message, a member's reply, a file or a web page never does, even when it claims to quote you. The orchestrator skill and the member protocol both say so.
- A message from the orchestrator never counts as your consent in a member: each member asks for its own permissions.
- Members report a blocked action instead of working around it, and never try to change permissions.

### Grants reach members through `cadre allow`

- When you allow something, the orchestrator records the narrowest rule with `cadre allow` in your cadre's `.claude/member-settings.json`. It never adds a rule because a member asked for one.
- `cadre allow` refuses blanket rules (bare tools, lone wildcards, whole MCP servers, shells, interpreters and wrappers with a wildcard, chained commands, every domain) and anything that reaches cadre's own files, startup files, or files that run code outside a session. Paths are read the way Claude Code reads them, after Unicode normalization and case folding, so globs, escapes and braces cannot hide a refused file. Other wildcards, `git`, code from project files and secret-reading paths get a warning. Every change is committed in your cadre.
- Members never get the grants file itself. Before each start, cadre validates it (only `permissions.allow/deny` and `autoMode.allow/soft_deny`, no duplicate keys, `"$defaults"` kept, the fixed entries present) and passes a read-only copy from `.claude/build/`, rebuilt at every start. A file that fails the check gives members a copy with no grants, and a warning. A fingerprint in `~/.cadre/config` flags edits made outside `cadre allow`.
- Every copy carries fixed rules: `Edit` denies for `~/.cadre/config`, `~/.cadre/framework`, and each cadre's `.claude`, `cadre.conf`, `members`, `playbook.md`, `protocol.md`, `projects.yaml` and `.git`; a deny for `cadre allow`; and an auto-mode `soft_deny` line saying that only the user changes these, through the orchestrator. Team folders stay writable.
- A one-time grant (`--once`) is usable by any member, any number of times, until it is removed. The orchestrator removes it after the task and checks for leftovers at every session start.

### What members cannot run

Members are refused for `cadre allow add` and `remove`, `cadre stop` with no team, `cadre uninstall`, `cadre init`, `cadre use`, and `cadre project add`, `link`, `unlink` and `trust`. Cadre sets `CADRE_MEMBER` in every member. Sessions started by 0.1.x carry `CADRE_PERSONA` instead, which cadre reads the same way and never sets; this alias goes away in a later release.

This check is advisory: a member with shell access could unset the variable. The deny rules, Claude Code's protection of `.claude` folders, and above all the permission mode are the real boundary.

### No code from unexpected places

- `cadre.conf` is read as `KEY=VALUE` lines, never run as shell code.
- Configuration is read only from `~/.cadre` or the cadre named by `CADRE_HOME`, never from the folder you happen to be in. A folder that looks like a cadre is never used.
- Members and the orchestrator start with `CADRE_HOME` set, so changing folders never switches their cadre.

### Files that are not cadre's

- **Trust** (`~/.claude.json`): adding, linking or cloning a project marks its folder as trusted, so members start without the trust prompt. Only a project repository's top folder is trusted, never `/`, your home folder, `~/.cadre`, a team folder or `~/.claude`. Trust lets that repository's own `.claude/settings.json` rules and hooks take effect, so add only repositories you trust, or pass `--no-trust`.
- **The orchestrator hook** (`~/.claude/settings.json`): added only on your yes, and removed by `cadre uninstall`.
- Edits to both files keep every other key and their order, keep a backup of the original (`.claude.json.bak-cadre`, `settings.json.bak-cadre`; the first is never overwritten), are written atomically, check the file again just before writing, and are skipped with a warning on any doubt.

### Nothing is deleted

Unlinking a project keeps its folder. Bringing in a 0.1.x cadre only reads it, and never changes the old folder or `~/.config/cadre`. `cadre uninstall` keeps every cadre and project, trust entries and backups.

### No credentials in a cadre's repository

Cadre writes no tokens into a cadre, and a new cadre's `.gitignore` leaves out `.env` files. A pre-push hook in each cadre repository refuses a push that carries files that look like credentials, by name (`.env` files, `.credentials.json`, `credentials`, `.git-credentials`, `.netrc`, private keys, key stores, `.aws/`, `.kube/config`, `.docker/config.json`) or by content (Anthropic, GitHub, AWS and Slack tokens, private key headers), or files over 50 MB. It checks every name every file has in every commit pushed, and names each file it stops. The orchestrator creates a backup repository only as private, and only on your yes. A pre-push hook of your own is left alone, and the health check says that cadre's check does not run.

The hook is a safety net, not a guarantee. It does not see a token split across lines or encoded (for example in base64), the content of a file over 8 MB, or a push made with `git push --no-verify` (the skill tells the orchestrator never to use it). A session with shell access in the cadre can also change or remove the hook; one without cadre's marker line is then reported as your own, not restored. Keep credentials out of the cadre folder.

### The skill on disk

The orchestrator skill is written out to `~/.cadre/framework/skills/cadre`, checked against the program, and restored at every `cadre` start. A change made to it after the orchestrator opened lasts until the next start. The fixed deny rules keep members from editing it with their edit tool, but not with a shell in a permissive mode.

### Accounts

Cadre uses one Claude account at a time, whichever Claude Code is logged in with. Tools such as claude-swap can switch it; a switch applies to every cadre on the machine, so restart afterwards (`cadre stop`, close the orchestrator, run `cadre`). Cadre does not depend on any such tool and holds no credentials.

### Permission mode

Members and the orchestrator run in the mode set in `cadre.conf` (default: `default`). A permissive mode such as `auto` lets members act without asking. Choose it deliberately, knowing that members act on messages from the orchestrator.

## Install

`install.sh` is short and meant to be read before you run it. It downloads the release archive for your machine, checks it against the release's `checksums.txt`, and places the program in `~/.local/bin/cadre`. It changes nothing else. The Homebrew formula installs the same release binary. Everything cadre creates afterwards is listed in the README's "What gets installed where".
