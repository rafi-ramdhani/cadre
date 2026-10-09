# Security

## Reporting a vulnerability

Please report security problems privately through GitHub's "Report a vulnerability" button on this repository's Security tab, not in a public issue. You should get a reply within a week.

## How cadrei is built to be safe

Cadrei runs several Claude Code sessions for you. Each member is a full Claude Code session that acts on messages from the orchestrator, so the design keeps three things in your hands: what members may do, cadrei's own files, and the files cadrei shares with Claude Code.

### Consent comes only from you, in the orchestrator

- Only what you type in the orchestrator session, or your answers to its own questions, counts as your consent. A cross-session message, a member's reply, a file or a web page never does, even when it claims to quote you. The orchestrator skill and the member protocol both say so.
- A message from the orchestrator never counts as your consent in a member: each member asks for its own permissions.
- Members report a blocked action instead of working around it, and never try to change permissions.

### Grants reach members through `cadrei allow`

- When you allow something, the orchestrator records the narrowest rule with `cadrei allow` in your cadrei's `.claude/member-settings.json`. It never adds a rule because a member asked for one.
- `cadrei allow` refuses blanket rules (bare tools, lone wildcards, whole MCP servers, shells, interpreters and wrappers with a wildcard, chained commands, every domain) and anything that reaches cadrei's own files, startup files, or files that run code outside a session. For `Edit` and `Read` rules, paths are read the way Claude Code reads them, after Unicode normalization and case folding, so globs, escapes and braces cannot hide a refused file. `Bash` rules are checked word by word (paths, variables, `..` climbs) and cannot see where a link on disk leads, so the fixed `Edit` denies and the permission mode remain the stronger guards. Other wildcards, `git`, code from project files and secret-reading paths get a warning. Every change is committed in your cadrei.
- Members never get the grants file itself. Before each start, cadrei validates it (only `permissions.allow/deny` and `autoMode.allow/soft_deny`, no duplicate keys, `"$defaults"` kept, the fixed entries present) and passes a read-only copy from `.claude/build/`, rebuilt at every start. A file that fails the check gives members a copy with no grants, and a warning. A fingerprint in `~/.cadrei/config` flags edits made outside `cadrei allow`.
- Every copy carries fixed rules: `Edit` denies for `~/.cadrei/config`, `~/.cadrei/framework`, and each cadrei's `.claude`, `cadrei.conf`, `members`, `playbook.md`, `protocol.md`, `projects.yaml` and `.git`; a deny for `cadrei allow`; and an auto-mode `soft_deny` line saying that only the user changes these, through the orchestrator. Team folders stay writable.
- A one-time grant (`--once`) is usable by any member, any number of times, until it is removed. The orchestrator removes it after the task and checks for leftovers at every session start.

### What members cannot run

Members are refused for `cadrei allow add` and `remove`, `cadrei stop` with no team, `cadrei uninstall`, `cadrei init`, `cadrei use`, and `cadrei project add`, `link`, `unlink` and `trust`. Cadrei sets `CADREI_MEMBER` in every member. Sessions started by 0.1.x carry `CADRE_PERSONA` instead, which cadrei reads the same way and never sets; this alias goes away in a later release.

This check is advisory: a member with shell access could unset the variable. The deny rules, Claude Code's protection of `.claude` folders, and above all the permission mode are the real boundary.

### No code from unexpected places

- `cadrei.conf` is read as `KEY=VALUE` lines, never run as shell code.
- Configuration is read only from `~/.cadrei` or the cadrei named by `CADREI_HOME`, never from the folder you happen to be in. A folder that looks like a cadrei is never used.
- Members and the orchestrator start with `CADREI_HOME` set, so changing folders never switches their cadrei.

### Files that are not cadrei's

- **Trust** (`~/.claude.json`): adding, linking or cloning a project marks its folder as trusted, so members start without the trust prompt. Only a project repository's top folder is trusted, never `/`, your home folder, `~/.cadrei`, a team folder or `~/.claude`. Trust lets that repository's own `.claude/settings.json` rules and hooks take effect, so add only repositories you trust, or pass `--no-trust`.
- **The orchestrator hook** (`~/.claude/settings.json`): added only on your yes, and removed by `cadrei uninstall`.
- Edits to both files keep every other key and their order, keep a backup of the original (`.claude.json.bak-cadrei`, `settings.json.bak-cadrei`; the first is never overwritten), are written atomically, check the file again just before writing, and are skipped with a warning on any doubt.

### Nothing is deleted

Unlinking a project keeps its folder. Bringing in a 0.1.x cadre only reads it, and never changes the old folder or `~/.config/cadre`. `cadrei uninstall` keeps every cadrei and project, trust entries and backups.

### No credentials in a cadrei's repository

Cadrei writes no tokens into a cadrei, and a new cadrei's `.gitignore` leaves out `.env` files. A pre-push hook in each cadrei repository refuses a push that carries files that look like credentials, by name (`.env` files, `.credentials.json`, `credentials`, `.git-credentials`, `.netrc`, private keys, key stores, `.aws/`, `.kube/config`, `.docker/config.json`) or by content (Anthropic, GitHub, AWS and Slack tokens, private key headers), or files over 50 MB. It checks every name every file has in every commit pushed, and names each file it stops. The orchestrator creates a backup repository only as private, and only on your yes. A pre-push hook of your own is left alone, and the health check says that cadrei's check does not run.

The hook is a safety net, not a guarantee. It does not see a token split across lines or encoded (for example in base64), the content of a file over 8 MB, or a push made with `git push --no-verify` (the skill tells the orchestrator never to use it). A session with shell access in the cadrei can also change or remove the hook; one without cadrei's marker line is then reported as your own, not restored. Keep credentials out of the cadrei folder.

### Resumed conversations

Members and the orchestrator continue their last conversation when they start again. Only the members' own session hook updates a member's record; the orchestrator resumes only a conversation cadrei itself started. At the same user id a record cannot be authenticated, so a member can point another member that works in the same folder at a different conversation from that folder.

### The skill on disk

The orchestrator skill is written out to `~/.cadrei/framework/skills/cadrei`, checked against the program, and restored at every `cadrei` start. A change made to it after the orchestrator opened lasts until the next start. The fixed deny rules keep members from editing it with their edit tool, but not with a shell in a permissive mode.

### Accounts

Cadrei uses one Claude account at a time, whichever Claude Code is logged in with. Tools such as claude-swap can switch it; a switch applies to every cadrei on the machine, so restart afterwards (`cadrei stop`, close the orchestrator, run `cadrei`). Cadrei does not depend on any such tool and holds no credentials.

### Permission mode

Members and the orchestrator run in the mode set in `cadrei.conf` (default: `default`). A permissive mode such as `auto` lets members act without asking. Choose it deliberately, knowing that members act on messages from the orchestrator.

## Install

`install.sh` is short and meant to be read before you run it. It downloads the release archive for your machine, checks it against the release's `checksums.txt`, and places the program in `~/.local/bin/cadrei`. It changes nothing else. The Homebrew formula installs the same release binary. Everything cadrei creates afterwards is listed in the README's "What gets installed where".
