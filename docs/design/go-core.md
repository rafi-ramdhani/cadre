# Cadre 0.2.0 in Go: package layout and core design

Status: draft, written before Go was installed. Section N is read and
folded in; parts marked **(O)** wait for section O.

## Goals

- One static binary, `cadre`, with no Python, jq or shell helpers at run time.
- The behaviour the bash 0.2.0 work settled after nine review rounds, kept
  exactly where the spec does not change it. The bug classes those reviews
  kept finding (quoting into tmux and shells, `python3 -I`, escapes, bash 3.2,
  `set -e`, tab-separated parsing) should be impossible by construction.
- `tests/smoke.sh` stays the black-box acceptance suite; Go unit tests cover
  the logic beneath it.

## Layout

```
cmd/cadre/main.go          flags, dispatch, exit codes; nothing else
internal/
  cli/         command table (visible, advanced, old names), help text
  paths/       HOME, config and cache folders, physical paths (realpath)
  fsx/         atomic write, mkdir lock with stale takeover, mode and owner checks
  jsonx/       ordered JSON tree with raw leaves; strict decode (no duplicate keys)
  jsonedit/    safe edits of ~/.claude.json and settings.json (trust, hook, unhook)
  psettings/   persona settings: validate, export, fingerprints
  allow/       rule checks: tables, command checks, path checks, --auto checks
  shellwords/  POSIX word splitting and operator detection for Bash(...) rules
  glob/        gitignore-style segment matcher and reaches()
  tmux/        exact targets, options, sessions and windows, argv commands only
  sessions/    names, attribution (@cadre_home), legacy, start, stop, ls data
  conf/        cadre.conf parsed as KEY=VALUE, never sourced (N.1)
  home/        the ~/.cadre layout: config/, framework/, <name>/, external
  cadres/      the cadre list, reserved names, resolution (N.3), the
               ~/.config/cadre copy (N.1), migration (N.6)
  registry/    projects.yaml: projects linked by path, the projects folder
  orchestrator/ plain cadre, its lock, --tmux                      (M-T2)
  health/      fast and full checks                                (M-T3)
  update/, uninstall/, install/   by install kind, Homebrew         (M-T4)
  hooks/       SessionStart and statusLine as cadre subcommands    (O)
  assets/      go:embed of skill, protocol.md and template/         (O)
testdata/
  allow/       regression inputs from every review (see Testing)
  jsonedit/    config fixtures (surrogates, symlinks, odd shapes)
tests/smoke.sh               acceptance suite, unchanged harness
```

Only `cmd/cadre` and `internal/cli` know about argv. Every other package
takes typed values and returns errors with the user-facing message, so the
tests call them directly.

Dependencies, kept small and vendored:
- `golang.org/x/text` for NFKC (`unicode/norm`) and case folding (`cases.Fold`).
- `mvdan.cc/sh/v3/syntax` is an option for `shellwords` (see below).
- No CLI framework: a hand-written dispatch table, so the advanced commands
  and old names stay one table that `help` and `help advanced` print from.

## Data model (section N)

```
~/.cadre/config/{default, projects-dir, persona-settings.sha256, external, state.json}
~/.cadre/framework/            install.sh installs only
~/.cadre/<name>/               one cadre, its own git repository
```

- `home.Root()` is `~/.cadre`. Every path below it is built by one function
  each (`home.Config("default")`, `home.Cadre(name)`, `home.Build(name)`), so
  no package concatenates paths by hand.
- **The cadre list** is the directory listing of `~/.cadre` (entries with
  `personas/`, minus the reserved `config`, `framework` and dot names) plus
  the physical paths in `config/external`. A cadre's name is its folder's
  basename, compared case-insensitively for clashes (I-T1 review).
- **Resolution** (`cadres.Resolve(cwd)`) returns the cadre, how it was found
  and the default:
  1. `CADRE_HOME`;
  2. cwd inside `~/.cadre/<name>/` or an external cadre;
  3. cwd inside a linked project (deepest match, by physical path); several
     linking cadres means a prompt with a terminal and a refusal without one;
  4. the default from `config/default`.

  Configuration is only ever read from `~/.cadre/`, `external` or
  `CADRE_HOME`, never from a folder the user is in.
- **`~/.config/cadre` copy** (N.1): runs once, under the config lock, before
  any command reads config. It copies `home` as a name into `default`, the
  old cadres list into `external` for cadres not under `~/.cadre`, and the
  fingerprints (re-recorded for moved paths), then removes the old folder.
  Until then, reads fall back to the old folder.
- **`cadre.conf`** (`conf.Parse`) reads `KEY=VALUE` lines, `#` comments and
  optional single or double quotes, with no expansion. Any other line is
  ignored with a warning naming it. Only known keys are used
  (`PERMISSION_MODE`, then `ORCHESTRATOR_PERMISSION_MODE`, `RESUME` and the
  others from M), so a stray key never changes behaviour.
- **The registry** (`projects.yaml`) keeps its flat format (written by cadre,
  read by the orchestrator). The bash reader's rules are ported, with `path`
  stored as `~/...` under home and absolute otherwise. An entry without `path`
  means `<projects-dir>/<name>` for a cadre in `~/.cadre`, and
  `<cadre>/projects/<name>` for an external one.
- **Migration** (N.6) lives in `cadres` and is built on `fsx`: copy, verify
  (file list, sizes, symlink targets, `git rev-parse HEAD` and
  `git status --porcelain`), rewrite paths, commit, then move the old files
  aside into `cadre-before-0.2/`. It never deletes, and it refuses while
  sessions run.

## Core primitives

### Physical paths (`paths`)

- `Real(p)` resolves symlinks in the longest existing prefix and joins the
  rest, the way Python's `os.path.realpath` does. `filepath.EvalSymlinks`
  fails on missing paths, which `cadre allow` and `cadres check` both need.
- Every stored path is physical (I-T1 review): the known-cadres list, the
  default pointer, `@cadre_home` and `CADRE_HOME` for personas.
- The macOS `/System/Volumes/Data` firmlink prefix is stripped before
  comparing.

### Atomic writes and locks (`fsx`)

- `WriteAtomic(target, data, mode)`: a temp file in the target's own (real)
  folder, `fchmod`, write, `fsync`, `rename`. Shared by every writer.
- `Lock(dir)`: `mkdir` lock (macOS has no `flock(1)`, and mkdir works on
  every filesystem cadre meets).
  - A lock older than 60 s is taken over by renaming it to a unique name
    (`.stale-<pid>-<ns>`) and retrying the acquire; it is never renamed back
    (PR #10 review).
  - A lock that vanishes between `mkdir` and `stat` is retried, not an error.
  - Release ignores a missing lock.
- Used for: the known-cadres list, the fingerprint file (machine-wide, I.5),
  `cadre allow` (per cadre), the orchestrator lock (M.3, which adds pid,
  start time and command checks).

### JSON that is not ours (`jsonx`, `jsonedit`)

Go's `encoding/json` is not safe for `~/.claude.json` as it stands:
- maps lose key order, so every edit would reorder Claude Code's file;
- a lone surrogate (`"\ud83d"`) decodes to U+FFFD, so a round trip changes
  user data. The bash version hit this as a crash; Go would corrupt instead;
- `<`, `>` and `&` are HTML-escaped unless `SetEscapeHTML(false)`.

Design: `jsonx.Decode` walks `json.Decoder.Token()` into an ordered tree
(`Object` is a slice of key/value pairs), keeping every scalar as its raw
bytes (`json.RawMessage`). Only objects that an edit touches are rebuilt.
Untouched strings are written back byte for byte, so surrogates, escapes and
number formats survive. Duplicate keys are an error for persona settings
(security) and kept as they are for Claude Code's files (not ours to judge).

`jsonedit` keeps the bash rules exactly:
- the file is never created (exit 4 when missing);
- unreadable, invalid, an unexpected shape, or owned by another user: left
  untouched (exit 5);
- nothing to change: exit 3, no write and no backup;
- a backup of the original before the first change, never overwritten
  (`.bak-cadre`); unhook writes its own (`.bak-cadre-uninstall`);
- the file's mode is kept, and its owner is checked;
- size, mtime and sha256 are re-checked just before the rename; on change,
  retry up to 3 times, then exit 6 with the file untouched;
- a write failure is exit 7 with no temp file left;
- the test hook `CADRE_TEST_JSON_EDIT_HOOK` runs between the write and the
  re-check, as now;
- a symlinked config: the target is edited and the link kept.

Hook detection for unhook parses the hook `command` with `shellwords`, never
`strings.Fields` (the quoted-path bug from PR #4). **(O)** hooks become
`cadre hook session-start` and friends, so detection matches the command by
the cadre binary path instead of `bash .../orchestrator-hook.sh`; old entries
are still recognised for upgrades.

### Persona settings (`psettings`)

FIXED_DENY gains the N.7 entries, as `Edit` rules only (Claude Code ignores
`Write(path)` rules and warns about them, PR #5 review, so N.7's "the same for
`Write`" is left out):
`Edit(//**/.cadre/config/**)`, `Edit(//**/.cadre/framework/**)`,
`Edit(//**/.cadre/*/.claude/**)`, `Edit(//**/.cadre/*/cadre.conf)`,
`Edit(//**/.cadre/*/personas/**)`, `Edit(//**/.cadre/*/playbook.md)`,
`Edit(//**/.cadre/*/protocol.md)`, `Edit(//**/.cadre/*/projects.yaml)` and
`Edit(//**/.cadre/*/.git/**)`. The soft_deny line from N.7 is added the same
way. As with today's newer entries, a file must hold only the `PROTECT`
entries; the rest are added to every copy.

Otherwise it is a straight port of today's rules: only `permissions.{allow,deny}` and
`autoMode.{allow,soft_deny}`, `"$defaults"` required, `PROTECT` entries
required, `FIXED_DENY` added to every copy, strict decode with duplicate keys
refused, the export written fresh at every start (mode 0400, named by hash),
fingerprints keyed by real path under a machine-wide lock, and the
"changed outside cadre allow" check against `git show HEAD:` with cadre's
commit subjects. Git runs as `exec.Command("git", ...)` with argv.

### tmux (`tmux`)

- Every call is `exec.Command("tmux", args...)` with `-L <socket>` when
  `CADRE_TMUX_SOCKET` is set. No shell is ever involved on cadre's side.
- Targets are always exact: `=session`, `=session:=window`, and `=session:` for
  options. Helpers take a session and a window, never a target string.
- Personas start with the command as separate arguments
  (`new-session -d -s S -n R -c DIR -e CADRE_HOME=... -e CADRE_PERSONA=... -- claude --name ...`).
  With more than one argument, tmux 3.0 and newer execs the command directly
  with no shell, and `-e` sets the environment, so paths with quotes, spaces
  or `$` need no quoting at all. That removes the whole `sq()` class.
  - Cost: the user's shell no longer starts first, so a `PATH` set only in
    shell startup files is not applied. cadre resolves `claude` with
    `exec.LookPath` in its own environment and passes the absolute path.
  - Requires tmux 3.0 (2019). The health check (M-T3) reports an older tmux.
- A start is confirmed by the window existing and its pane not dead, after a
  short wait, as now.

### Bash rule parsing (`shellwords`)

What `cadre allow` needs from a `Bash(...)` specifier:
- the program word after leading `NAME=value` words, and the words after it;
- whether there is an operator outside quotes (`&& || ; | |& &` and
  newlines), reading escapes in pairs (`\;` is an argument, `\\;` is a
  backslash and then a real `;`);
- whether the program word is computed (`$`, backticks, `(`, `<`, `>`).

Two options:
1. A small hand-written POSIX lexer (single quotes, double quotes with their
   own escapes, backslash escapes outside quotes). It is about 150 lines and
   is fuzz-tested against the regression inputs.
2. `mvdan.cc/sh/v3/syntax`, a full shell parser. It is exact about
   operators, substitutions and redirections, but a parse error must count as
   a refusal, and the dependency is large.

Leaning to (1), because the checks need only the three facts above. Whichever
is chosen, the regression tables decide.

### Glob reading (`glob`)

A port of `seg_regex`, `segments`, `spells` and `reaches` from PR #9:
- `[...]`, `[!...]` and `[^...]` classes, backslash escapes, and `{a,b}`
  read as alternatives;
- `\/` is read as `/` before splitting, and escapes are read in pairs;
- refused before matching: a trailing unpaired backslash, `[:class:]`, nested
  braces, or a `/` inside braces;
- `reaches(rule, target)` matches the target, a folder holding it, or (for a
  folder target ending in `/`) anything inside it, with `**` crossing
  segments, all case-insensitive;
- both forms are checked: the rule as written and with its folders before the
  first wildcard resolved, against each target and its real path.

Go's `regexp` (RE2) has no backreferences or lookbehind; neither is needed
here. `seg_regex` builds RE2 patterns, and the lookbehind in `spells`
becomes a scan for unescaped `*` and `?`.

### The allow checker (`allow`)

Tables move verbatim into `allow/tables.go`: KNOWN, BARE, RUNNERS,
SUBRUNNERS, FROM_FILES, PROTECTED, OUTSIDE, SECRETS, VERSIONED, BLANKET and
AUTO_REFUSED. The checks keep their order and their messages, since smoke and
the skill match on message text. Text is normalised as now (NFKC, format
characters dropped, dashes and spaces made plain, case folded). `--auto` stays
ASCII-only.

Targets from section N (N.7):
- refused, as OUTSIDE entries:
  - `~/.cadre/config/` and `~/.cadre/framework/`;
  - for every cadre (under `~/.cadre` and in `external`): `.claude/`,
    `cadre.conf`, `personas/`, `playbook.md`, `protocol.md`, `projects.yaml`
    and `.git/`;
- warned: another cadre's `teams/`;
- allowed: the cadre's own `teams/`.

`cadre.conf` is no longer shell code (N.1), but the refusal stays as a
second layer, as N.7 says.

## Testing

- **Unit tests, table-driven.** `testdata/allow/*.txt` holds every rule and
  `--auto` input from the reviews and from smoke.sh, one per line:
  `<expect>\t<input>[\t<message fragment>]`, where expect is `refuse`,
  `accept`, `warn` or `secret-warn`. The reviewer's rules*.txt, narrow.txt,
  extra.txt and autos*.txt are imported as they are, with the expectation
  each review recorded.
  - Symlink cases (stow-style `~/.config`, a symlink into the cadre) are
    built in `t.TempDir()` with a fake HOME.
- **Fuzz tests** (`go test -fuzz`) for `glob`, `shellwords` and `jsonx`:
  - never panic;
  - `jsonx` round trip of an untouched document is byte-identical;
  - a rule that passes never `reaches` a protected target in a brute-force
    check over small generated paths.
- **jsonedit** keeps the race tests through the test hook: one change then
  success, a change on every attempt giving exit 6, plus surrogates, a
  symlinked config, mode 0644 kept, another owner, and an unreadable file.
- **smoke.sh** runs against the built binary from the sessions step on. Its
  harness (fake HOME, stub `claude`, private tmux socket, physical temp root,
  private bare clone) does not change. Sections are updated for the new
  command names and N's layout as each step lands.
- CI: `go vet`, `staticcheck`, `go test -race ./...`, and smoke.sh on macOS
  and Ubuntu.

## Porting order

1. Core: `paths`, `fsx`, `jsonx`, `jsonedit`, `psettings`, `shellwords`,
   `glob`, `allow`, with the full allow tables and the regression tests. No
   CLI yet beyond `cadre allow` for testing.
2. Section N's data model and resolution (N-T1, N-T2), with M-T1's command
   surface: dispatch, help, `ls --json` and `stop` start here with their
   final names.
3. Sessions: up, stop, ls (`--json`), attach, names, legacy, collisions.
   smoke.sh becomes the acceptance suite from here.
4. Plain `cadre`, the orchestrator lock, the health check (M-T2, M-T3).
5. update, uninstall, Homebrew and the install route (M-T4).
6. Resume, ctx and compact (M-T7, J).
7. Docs (M-T6) and the release.

## Known pitfalls carried from the bash reviews

Open in the last bash PR (#11) when it was frozen; the Go code must get them right:
- **Restart commands for another cadre** (`update`, `allow`) must carry that
  cadre: `CADRE_HOME=<path> cadre stop ... && CADRE_HOME=<path> cadre up ...`,
  or a form that names the cadre, since the user runs them from anywhere.
- **Claude session names can collide through hyphens.** Team `dev` with
  project `x` and role `y-z`, and team `dev-x` with role `y-z`, both give
  `<cadre>-dev-x-y-z`. Attribution uses tmux options, never name parsing, and
  `up` refuses when the Claude name it would use is already taken by another
  (cadre, team, project, role), not only when the tmux session name is.
- **Stale-lock takeover race.** Renaming a stale lock aside can still take
  over a lock another process has just re-created between its `stat` and the
  `rename`. Go can do better: the lock folder holds an owner file (pid,
  process start time and a random token). A stale lock is taken over only if
  its owner is dead, judged by pid and start time rather than by age alone,
  and the renamed-aside folder's token is checked before it is removed.
- **`#` is never a comment** in a `Bash(...)` specifier. The bash checker's
  shlex treats `#` as a comment even mid-word, so `Bash(echo x#; bash *)`
  passed the chaining check (fixed on main separately).

## Open points

- N.7 lists `Write(...)` deny entries; this design leaves them out (see
  Persona settings). The PM should confirm.
- Whether hooks and the status line run as `cadre hook ...` (O), and how the
  SessionStart hook finds the binary after `brew upgrade` (opt path, M.1).
- The minimum tmux version (3.0, for argv commands and `-e`).
- Whether `mvdan.cc/sh` is acceptable as a dependency.
