#!/usr/bin/env bash
# End-to-end smoke test. Runs everything in a throwaway HOME with a stub
# `claude` and a private tmux server, so it never touches a real setup.
#
#   tests/smoke.sh

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d)
export HOME="$T/home" CADRE_TMUX_SOCKET="cadre-test-$$"
unset TMUX CADRE_HOME CADRE_PERSONA CADRE_OFF
# The throwaway HOME has no git identity; give it one so commits work.
export GIT_CONFIG_GLOBAL="$T/gitconfig"
git config --global user.name "Cadre Test"
git config --global user.email "test@example.com"
git config --global init.defaultBranch main
mkdir -p "$HOME/.claude" "$T/bin"
trap 'command tmux -L "$CADRE_TMUX_SOCKET" kill-server 2>/dev/null || true; rm -rf "$T"' EXIT

# A stand-in for Claude Code that just stays alive like a session would.
printf '#!/bin/sh\nsleep 300\n' > "$T/bin/claude"
chmod +x "$T/bin/claude"
export PATH="$T/bin:$HOME/.local/bin:$PATH"

pass=0
ok() { pass=$((pass + 1)); printf '  ok  %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1" >&2; exit 1; }
check() { local name=$1; shift; if "$@" >/dev/null 2>&1; then ok "$name"; else fail "$name"; fi; }

echo "install"
CADRE_REPO="$ROOT" bash "$ROOT/install.sh" demo --dir "$T/work" --yes --orchestrator-default >/dev/null
C="$T/work/demo"
check "cadre generated" test -f "$C/playbook.md"
check "framework placed inside" test -x "$C/projects/cadre/bin/cadre"
check "command linked" test -L "$HOME/.local/bin/cadre"
check "skill linked" test -L "$HOME/.claude/skills/cadre"
check "active cadre recorded" grep -qx "$C" "$HOME/.config/cadre/home"
check "hook added" grep -q orchestrator-hook.sh "$HOME/.claude/settings.json"
check "framework registered" grep -q '^cadre:' "$C/projects.yaml"
check "cadre repo is clean" test -z "$(git -C "$C" status --porcelain)"
check "existing folder refused" bash -c "! bash '$ROOT/install.sh' demo --dir '$T/work' --yes"
check "bad name refused" bash -c "! bash '$ROOT/install.sh' 'bad name' --dir '$T/work' --yes"

echo "hook"
out=$(echo '{}' | bash "$ROOT/bin/orchestrator-hook.sh")
check "hook speaks in a normal session" grep -q SessionStart <<<"$out"
check "hook silent in a persona" test -z "$(echo '{}' | CADRE_PERSONA=x bash "$ROOT/bin/orchestrator-hook.sh")"
check "hook silent with CADRE_OFF" test -z "$(echo '{}' | CADRE_OFF=1 bash "$ROOT/bin/orchestrator-hook.sh")"

echo "grow"
git init -q --bare "$T/remote.git"
git -C "$T" clone -q "$T/remote.git" seed 2>/dev/null
git -C "$T/seed" commit -q --allow-empty -m init
git -C "$T/seed" push -q origin HEAD 2>/dev/null
out=$(cadre add project app "$T/remote.git" dev "A test app")
check "project cloned into projects/" test -d "$C/projects/app/.git"
check "project listed" bash -c "cadre projects | grep -q app"
check "path resolves" test "$(cadre path app)" = "$C/projects/app"
cadre add team ops >/dev/null
cadre add persona ops/sre >/dev/null
check "persona created" test -f "$C/personas/ops/sre.md"
check "duplicate project refused" bash -c "! cadre add project app '$T/remote.git'"

echo "trust"
CFG="$HOME/.claude.json"
check "no config: project still added" test -d "$C/projects/app/.git"
check "no config: none created" test ! -e "$CFG"
check "no config: explained" grep -q "has not created its config yet" <<<"$out"
# py <script> [args]: runs a check written in python, exit status is the result.
py() { python3 -c "$@"; }
trusted() { py 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if d["projects"][sys.argv[2]]["hasTrustDialogAccepted"] is True else 1)' "$1" "$2"; }
mtime() { py 'import os,sys; print(os.stat(sys.argv[1]).st_mtime_ns)' "$1"; }
mode() { py 'import os,sys; print(oct(os.stat(sys.argv[1]).st_mode & 0o777))' "$1"; }
phys() { (cd "$1" && pwd -P); }
printf '{"numStartups": 3, "oauthAccount": {"x": 1}, "projects": {"/elsewhere": {"allowedTools": [], "hasTrustDialogAccepted": false}}}' > "$CFG"
chmod 600 "$CFG"
cp "$CFG" "$T/cfg.orig"
out=$(cadre add project t1 "$T/remote.git")
check "add project trusts" grep -q "t1 added, cloned to $C/projects/t1 and trusted in Claude Code" <<<"$out"
check "physical path trusted" trusted "$CFG" "$(phys "$C/projects/t1")"
check "path as typed trusted" trusted "$CFG" "$C/projects/t1"
check "only the trust keys changed" py '
import json, sys
new, old = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
for k in set(sys.argv[3:]):
    assert new["projects"].pop(k) == {"hasTrustDialogAccepted": True}
assert new == old' "$CFG" "$T/cfg.orig" "$C/projects/t1" "$(phys "$C/projects/t1")"
check "backup written" cmp -s "$CFG.bak-cadre" "$T/cfg.orig"
check "mode kept" test "$(mode "$CFG")" = 0o600
rm "$CFG.bak-cadre"; m=$(mtime "$CFG")
check "already trusted reported" bash -c "cadre trust t1 | grep -q 'already trusted'"
check "already trusted: no rewrite" test "$(mtime "$CFG")" = "$m" -a ! -e "$CFG.bak-cadre"
cp "$CFG" "$T/cfg.before"
cadre add project t2 "$T/remote.git" --no-trust >/dev/null
check "--no-trust leaves the config alone" cmp -s "$CFG" "$T/cfg.before"
check "trust one project" bash -c "cadre trust t2 | grep -q 't2: trusted'"
check "trust one project: written" trusted "$CFG" "$(phys "$C/projects/t2")"
check "non-registry name refused" bash -c "! cadre trust nope"
check "plain folder refused" bash -c "! cadre trust '$C/teams'"
cp "$CFG" "$T/cfg.good"
printf '{not json' > "$CFG"; cp "$CFG" "$T/cfg.bad"
out=$(cadre add project t3 "$T/remote.git") || fail "invalid config: add failed"
check "invalid config: unchanged" cmp -s "$CFG" "$T/cfg.bad"
check "invalid config: warning names it" grep -q "warning: $CFG" <<<"$out"
printf '{"projects": []}' > "$CFG"; cp "$CFG" "$T/cfg.bad"
check "projects not an object: exit 0" cadre trust t3
check "projects not an object: unchanged" cmp -s "$CFG" "$T/cfg.bad"
mkdir -p "$T/ccd"; printf '{}' > "$T/ccd/.claude.json"
cp "$T/cfg.good" "$CFG"
CLAUDE_CONFIG_DIR="$T/ccd" cadre trust t3 >/dev/null
check "CLAUDE_CONFIG_DIR honoured" trusted "$T/ccd/.claude.json" "$(phys "$C/projects/t3")"
check "CLAUDE_CONFIG_DIR: home config untouched" cmp -s "$CFG" "$T/cfg.good"
cadre add project t4 "$T/remote.git" --no-trust >/dev/null
rm -rf "$C/projects/t4"
cp "$CFG" "$T/cfg.before"
out=$(cadre trust --all)
check "--all: already trusted" grep -q "t1: already trusted" <<<"$out"
check "--all: trusted" grep -q "t3: trusted" <<<"$out"
check "--all: missing locally" grep -q "t4: missing locally" <<<"$out"
check "--all: one backup of the original" cmp -s "$CFG.bak-cadre" "$T/cfg.before"
m=$(mtime "$CFG")
cadre trust --all >/dev/null
check "--all again changes nothing" test "$(mtime "$CFG")" = "$m"
out=$(cadre sync)
check "sync clones and trusts" grep -q "t4: trusted" <<<"$out"
check "sync: trust written" trusted "$CFG" "$(phys "$C/projects/t4")"
check "sync: present projects left alone" bash -c "! grep -q 'app: .*trusted' <<<'$out'"
rm -rf "$C/projects/t4"
py 'import json,sys; d=json.load(open(sys.argv[1])); [d["projects"].pop(k) for k in list(d["projects"]) if k.endswith("/t4")]; json.dump(d, open(sys.argv[1], "w"))' "$CFG"
cp "$CFG" "$T/cfg.before"
cadre sync --no-trust >/dev/null
check "sync --no-trust clones" test -d "$C/projects/t4/.git"
check "sync --no-trust leaves the config alone" cmp -s "$CFG" "$T/cfg.before"

echo "sessions"
cadre up dev/engineer app >/dev/null
check "project persona running" bash -c "cadre ls | grep -q '\[running\] dev-app-engineer'"
check "persona works in the project" test "$(command tmux -L "$CADRE_TMUX_SOCKET" display -p -t cadre-dev-app:engineer '#{pane_current_path}')" = "$(cd "$C/projects/app" && pwd -P)"
check "prompt built" grep -q "Persona" "${XDG_CACHE_HOME:-$HOME/.cache}/cadre/build/dev-app-engineer.md"
cadre up ops >/dev/null
check "team without project running" bash -c "cadre ls | grep -q '\[running\] ops-sre'"
cadre down dev app >/dev/null
cadre down ops >/dev/null
check "sessions stopped" bash -c "! cadre ls | grep -q running"

echo "update"
# The installed framework tracks main on a local bare remote; a seed clone
# pushes fake releases to it.
F="$C/projects/cadre"
git -C "$F" switch -q -C main
git clone -q --bare "$F" "$T/fw.git"
git -C "$F" remote set-url origin "$T/fw.git"
git -C "$F" fetch -q origin
git -C "$F" remote set-head origin main >/dev/null
git -C "$F" branch -q -u origin/main
git clone -q "$T/fw.git" "$T/fwseed"
git clone -q "$T/fw.git" "$T/clone2"
head_of() { git -C "$F" rev-parse HEAD; }
# Hashes of every file in the cadre except the framework, registry projects included.
snap() { python3 -c '
import hashlib, os, sys
root, out = sys.argv[1], []
for d, dirs, files in os.walk(root):
    if d == os.path.join(root, "projects"):
        dirs[:] = [x for x in dirs if x != "cadre"]
    for f in files:
        p = os.path.join(d, f)
        out.append(os.path.relpath(p, root) + " " + hashlib.sha256(open(p, "rb").read()).hexdigest())
print("\n".join(sorted(out)))
' "$C"; }
# release <message> <python>: commits a change made by python (cwd: the seed) and pushes it.
release() { (cd "$T/fwseed" && python3 -c "$2") && git -C "$T/fwseed" add -A && git -C "$T/fwseed" commit -qm "$1" && git -C "$T/fwseed" push -q origin HEAD:main 2>/dev/null; }
v0=$(cadre version)
h0=$(head_of)
check "up to date" bash -c "cadre update | grep -qx '${v0} is up to date'"
check "check without update exits 0" cadre update --check
check "check when up to date changes nothing" test "$(head_of)" = "$h0" -a -z "$(git -C "$F" status --porcelain)"
check "unknown option refused" bash -c "! cadre update --bogus"
check "persona cannot update" bash -c "! CADRE_PERSONA=x cadre update"
check "persona can check" env CADRE_PERSONA=x cadre update --check
mkdir -p "$T/fwcopy" && cp -R "$F/bin" "$F/install.sh" "$T/fwcopy/"
check "not a git checkout refused" bash -c "'$T/fwcopy/bin/cadre' update 2>&1 | grep -q 'not a git checkout'"
cp -R "$T/fwcopy" "$C/teams/fwcopy"
check "framework copy inside the cadre repo refused" bash -c "! '$C/teams/fwcopy/bin/cadre' update"
rm -rf "$C/teams/fwcopy"

release "Release 9.9.9" '
s = open("bin/cadre").read().replace("CADRE_VERSION=", "CADRE_VERSION=9.9.9\nOLD_VERSION=", 1)
filler = "".join("# filler line %d that moves every later byte of the script\n" % i for i in range(400))
open("bin/cadre", "w").write(s.replace("set -euo pipefail\n", "set -euo pipefail\n" + filler, 1))
s = open("CHANGELOG.md").read()
open("CHANGELOG.md", "w").write(s.replace("## Unreleased\n", "## Unreleased\n\n## 9.9.9 - 2026-10-09\n\n- Test release.\n\n### Upgrading\n\n- Nothing to do by hand.\n", 1))'

code=0; out=$(cadre update --check) || code=$?
check "check finds the update" test "$code" = 3
check "check names both versions" grep -qx "update available: ${v0#cadre } -> 9.9.9" <<<"$out"
check "check changes nothing" test "$(head_of)" = "$h0" -a -z "$(git -C "$F" status --porcelain)"

refused() { local name=$1 want=$2; shift 2; local o; if o=$(cadre update 2>&1); then fail "$name"; fi; grep -q "$want" <<<"$o" || fail "$name: $o"; test "$(head_of)" = "$h0" || fail "$name: HEAD moved"; ok "$name"; }
echo "# local edit" >> "$F/README.md"
refused "dirty tree refused" "$F has local changes"
git -C "$F" checkout -q -- README.md
git -C "$F" switch -q -c other
refused "other branch refused" "on branch other, not main"
git -C "$F" switch -q main
git -C "$F" switch -q --detach
refused "detached HEAD refused" "detached HEAD"
git -C "$F" switch -q main
git -C "$F" commit -q --allow-empty -m "local work"
h0=$(head_of)
refused "local commit refused" "local commit"
git -C "$F" reset -q --hard HEAD~1
h0=$(head_of)
git -C "$F" remote set-url origin "$T/missing.git"
refused "failing fetch reported" "could not fetch"
git -C "$F" remote set-url origin "$T/fw.git"

# A second clone on main updates itself but must not take over the links.
cp "$HOME/.claude/settings.json" "$T/settings.before"
out=$("$T/clone2/bin/cadre" update)
check "other clone updated" test "$("$T/clone2/bin/cadre" version)" = "cadre 9.9.9"
check "other clone leaves the command link" test "$(readlink "$HOME/.local/bin/cadre")" = "$F/bin/cadre"
check "other clone leaves the skill link" test "$(readlink "$HOME/.claude/skills/cadre")" = "$F/skills/cadre"
check "other clone leaves the hook" cmp -s "$HOME/.claude/settings.json" "$T/settings.before"
check "other clone says why" grep -q "points at another framework folder" <<<"$out"

# The update must never read bin/cadre again once the merge has run. git
# writes the new file as a new inode, which alone would hide the hazard, so a
# post-merge hook writes the new script into the old inode too.
ln "$F/bin/cadre" "$T/old-inode"
printf '#!/bin/sh\ncat bin/cadre > "%s"\n' "$T/old-inode" > "$F/.git/hooks/post-merge"
chmod +x "$F/.git/hooks/post-merge"
cp "$HOME/.claude/settings.json.bak-cadre" "$T/settings.bak.before"
cadre up dev/engineer app >/dev/null
before=$(snap)
mv "$HOME/.config/cadre/home" "$T/home.saved"
code=0; out=$(cadre update 2>"$T/update.err") || code=$?
mv "$T/home.saved" "$HOME/.config/cadre/home"
rm "$F/.git/hooks/post-merge" "$T/old-inode"
check "update exits 0" test "$code" = 0
check "update applied" test "$(cadre version)" = "cadre 9.9.9"
check "update prints the versions" grep -qx "updated cadre ${v0#cadre } -> 9.9.9" <<<"$out"
check "update prints the changelog" grep -q "Test release" <<<"$out"
check "update prints the upgrading notes" grep -qx "### Upgrading" <<<"$out"
check "old script ran without errors" test ! -s "$T/update.err"
check "command still linked here" test "$(readlink "$HOME/.local/bin/cadre")" = "$F/bin/cadre"
check "skill still linked here" test "$(readlink "$HOME/.claude/skills/cadre")" = "$F/skills/cadre"
check "hook kept" grep -q orchestrator-hook.sh "$HOME/.claude/settings.json"
check "hook already right: settings not rewritten" cmp -s "$HOME/.claude/settings.json" "$T/settings.before"
check "hook already right: backup kept" cmp -s "$HOME/.claude/settings.json.bak-cadre" "$T/settings.bak.before"
check "running persona listed" grep -q "cadre-dev-app: dev-app-engineer" <<<"$out"
check "restart command printed" grep -qx "  cadre down dev/engineer app && cadre up dev/engineer app" <<<"$out"
check "running persona not stopped" bash -c "cadre ls | grep -q '\[running\] dev-app-engineer'"
check "cadre and projects unchanged" test "$(snap)" = "$before"
check "cadre repo still clean" test -z "$(git -C "$C" status --porcelain)"
cadre down dev app >/dev/null
mv "$HOME/.config/cadre/home" "$T/home.saved"
check "works without an active cadre" bash -c "cadre update | grep -q 'is up to date'"
mv "$T/home.saved" "$HOME/.config/cadre/home"

release "Unreleased change" '
s = open("CHANGELOG.md").read()
open("CHANGELOG.md", "w").write(s.replace("## Unreleased\n", "## Unreleased\n\n- An unreleased change.\n", 1))'
printf '{ "a": 1, // bash orchestrator-hook.sh\n}\n' > "$HOME/.claude/settings.json"
cp "$HOME/.claude/settings.json" "$T/settings.bad"
code=0; out=$(cadre update 2>&1) || code=$?
check "update between releases succeeds" test "$code" = 0
check "update between releases shows Unreleased" grep -q "An unreleased change" <<<"$out"
check "settings that are not plain JSON are left alone" cmp -s "$HOME/.claude/settings.json" "$T/settings.bad"
check "and the update says so" grep -q "skipped the hook" <<<"$out"
cp "$T/settings.before" "$HOME/.claude/settings.json"

release "Drop the changelog" 'import os; os.remove("CHANGELOG.md")'
out=$(cadre update)
check "no changelog: no pointer to a missing file" bash -c "! grep -q 'CHANGELOG' <<<'$out'"

release "Remove bin/cadre" 'import os; os.remove("bin/cadre")'
code=0; out=$(cadre update --check 2>&1) || code=$?
check "origin without bin/cadre: check exits 1" test "$code" = 1
check "origin without bin/cadre: explained" grep -q "no bin/cadre" <<<"$out"
git -C "$T/fwseed" reset -q --hard HEAD~1
git -C "$T/fwseed" push -q -f origin HEAD:main 2>/dev/null
check "back to up to date" bash -c "cadre update | grep -q 'is up to date'"

echo "restore on a new machine"
rm -rf "$HOME/.local" "$HOME/.config"
cp "$CFG" "$T/cfg.before"
CADRE_REPO="$ROOT" bash "$ROOT/install.sh" --from "$C" --dir "$T/machine2" --yes --no-hook --no-trust >/dev/null
check "cadre cloned" test -f "$T/machine2/demo/playbook.md"
check "projects cloned by sync" test -d "$T/machine2/demo/projects/app/.git"
check "active cadre switched" grep -qx "$T/machine2/demo" "$HOME/.config/cadre/home"
check "--from --no-trust leaves the config alone" cmp -s "$CFG" "$T/cfg.before"

echo "$pass checks passed"
