#!/usr/bin/env bash
# End-to-end smoke test of the Go cadre binary. Runs everything in a
# throwaway HOME with a stub agent and a private tmux server, so it never
# touches a real setup. It is the parity contract of the rewrite (spec O.6):
# sections are ported from tests/smoke-bash.sh as each step lands, with
# section N's layout and section M's command names.
#
#   tests/smoke.sh
#
# Not yet ported (their steps come later): plain cadre and the health
# check (O-T5), update, uninstall and install (O-T6), resume, ctx and
# compact (O-T7).

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
# Physical, as cadre keeps the paths of cadres (macOS: /var is /private/var).
T=$(cd "$(mktemp -d)" && pwd -P)
# The build uses the real Go caches, not the throwaway HOME's.
GOMODCACHE=$(go env GOMODCACHE) GOCACHE=$(go env GOCACHE) GOPATH=$(go env GOPATH)
export GOMODCACHE GOCACHE GOPATH
export HOME="$T/home" CADRE_TMUX_SOCKET="cadre-test-$$"
unset TMUX CADRE_HOME CADRE_PERSONA CADRE_OFF CLAUDE_CONFIG_DIR XDG_CACHE_HOME
export GIT_CONFIG_GLOBAL="$T/gitconfig"
git config --global user.name "Cadre Test"
git config --global user.email "test@example.com"
git config --global init.defaultBranch main
mkdir -p "$HOME" "$T/bin" "$T/rel"
trap 'command tmux -L "$CADRE_TMUX_SOCKET" kill-server 2>/dev/null || true; rm -rf "$T"' EXIT

# The binary under test, built with the test-only parts (the fake runtime,
# the JSON race hook), and a release build for the checks that need one.
(cd "$ROOT" && go build -tags cadretest -o "$T/bin/cadre" ./cmd/cadre && go build -o "$T/rel/cadre" ./cmd/cadre)

# A stand-in for Claude Code that records its arguments and stays alive
# like a session would.
cat > "$T/bin/claude" <<EOF
#!/bin/sh
printf '%s\\n' "\$@" > "$T/args-\$CADRE_PERSONA"
printf '%s\\n' "\$CADRE_HOME" > "$T/home-\$CADRE_PERSONA"
exec sleep 300
EOF
chmod +x "$T/bin/claude"
export PATH="$T/bin:$PATH"
# Run from a folder outside every cadre.
cd "$T"

pass=0
ok() { pass=$((pass + 1)); printf '  ok  %s\n' "$1"; }
fail() { printf '  FAIL %s\n' "$1" >&2; exit 1; }
check() { local name=$1; shift; if "$@" >/dev/null 2>&1; then ok "$name"; else fail "$name"; fi; }
tm() { command tmux -L "$CADRE_TMUX_SOCKET" "$@"; }
running() { tm has-session -t "=$1" 2>/dev/null; }
# py <script> [args]: a check written in python, exit status is the result.
py() { python3 -I -c "$@"; }
mode() { py 'import os,sys; print(oct(os.stat(sys.argv[1]).st_mode & 0o777))' "$1"; }
args_of() { for _ in $(seq 50); do [ -s "$T/args-$1" ] && break; sleep 0.1; done; cat "$T/args-$1" 2>/dev/null || true; }
# Checks run in bash -c, which sees exported functions only.
export -f tm running py mode args_of
export T

# A remote to clone projects from.
git init -q "$T/src" && echo app > "$T/src/README" && git -C "$T/src" add -A && git -C "$T/src" commit -qm first
git clone -q --bare "$T/src" "$T/remote.git"

echo "layout (N.1)"
out=$(cadre init demo)
C="$HOME/.cadre/demo"
check "init makes ~/.cadre/<name>" test -f "$C/playbook.md" -a -d "$C/.git"
check "no projects/ in it" test ! -e "$C/projects"
check "the first cadre is the default" grep -q "demo is the default cadre" <<<"$out"
check "default recorded by name" test "$(cat "$HOME/.cadre/config/default")" = demo
check "reserved names refused" bash -c "! cadre init config 2>/dev/null && ! cadre init framework 2>/dev/null"
check "bad names refused" bash -c "! cadre init 'bad name' 2>/dev/null && ! cadre init .x 2>/dev/null"
out=$(cadre init life)
check "a second cadre leaves the default" grep -q "default cadre stays demo" <<<"$out"
check "a name in another case refused" bash -c "! cadre init DEMO 2>/dev/null"
check "cadre.conf is never run" bash -c "echo 'touch $T/marker' >> '$C/cadre.conf'; cd '$C' && cadre ls >/dev/null 2>&1; test ! -e '$T/marker'"
check "and its command line is named" bash -c "cd '$C' && cadre ls 2>&1 | grep -q 'ignored (only KEY=VALUE'"
git -C "$C" checkout -q -- cadre.conf

echo "the old ~/.config/cadre is moved once (N.1)"
mkdir -p "$T/old/visible/personas" "$HOME/.config/cadre"
echo "$T/old/visible" > "$HOME/.config/cadre/home"
mv "$HOME/.cadre/config/default" "$T/default.saved"
err=$(cadre ls 2>&1 >/dev/null || true)
check "moved, with a note" grep -q "moved cadre's settings" <<<"$err"
check "the old folder is kept aside" test -f "$HOME/.config/cadre.moved-to-0.2.0/home" -a ! -e "$HOME/.config/cadre"
check "the visible cadre is outside" grep -qx "$T/old/visible" "$HOME/.cadre/config/external"
mv "$T/default.saved" "$HOME/.cadre/config/default"
cadre cadres remove visible >/dev/null

echo "grow"
cd "$C"
cadre team add ops >/dev/null
cadre persona add ops/sre >/dev/null
check "persona created" test -f "$C/personas/ops/sre.md"
check "team and role names that leave personas/ refused" bash -c "! cadre team add ../x 2>/dev/null && ! cadre persona add dev/../../pw 2>/dev/null"
check "no projects folder: refused without a terminal" bash -c "cadre project add app '$T/remote.git' 2>&1 | grep -q 'the projects folder is not set'"
cadre project dir "$HOME/Developer" >/dev/null
out=$(cadre project add app "$T/remote.git")
check "project cloned into the projects folder" test -d "$HOME/Developer/app/.git"
check "its path stored with ~" grep -qx "  path: ~/Developer/app" "$C/projects.yaml"
check "no config: explained" grep -q "has not created its config yet" <<<"$out"
check "project listed" bash -c "cadre projects | grep -q app"
check "project path" test "$(cadre project path app)" = "$HOME/Developer/app"
check "old name path" test "$(cadre path app)" = "$HOME/Developer/app"
check "duplicate project refused" bash -c "! cadre project add app '$T/remote.git' 2>/dev/null"
git clone -q "$T/remote.git" "$HOME/src/mine"
check "a folder is linked with --path" bash -c "cadre project add mine --path '$HOME/src/mine' --no-trust | grep -q 'mine added, linked at'"
check "cadre's own folders cannot be linked" bash -c "! cadre project add x --path '$HOME/.cadre/demo' 2>/dev/null"

echo "trust"
CFG="$HOME/.claude.json"
trusted() { py 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if d["projects"][sys.argv[2]]["hasTrustDialogAccepted"] is True else 1)' "$1" "$2"; }
printf '{"numStartups": 3, "oauthAccount": {"x": 1}, "projects": {"/elsewhere": {"allowedTools": [], "hasTrustDialogAccepted": false}}}' > "$CFG"
chmod 600 "$CFG"
cp "$CFG" "$T/cfg.orig"
out=$(cadre project add t1 "$T/remote.git")
check "add project trusts" grep -q "t1 added, cloned to $HOME/Developer/t1 and trusted in Claude Code" <<<"$out"
check "the folder is trusted" trusted "$CFG" "$HOME/Developer/t1"
check "only the trust keys changed" py '
import json, sys
new, old = json.load(open(sys.argv[1])), json.load(open(sys.argv[2]))
assert new["projects"].pop(sys.argv[3]) == {"hasTrustDialogAccepted": True}
assert new == old' "$CFG" "$T/cfg.orig" "$HOME/Developer/t1"
check "the original keeps its last byte (no newline added)" test "$(tail -c 1 "$CFG")" = "}"
check "backup written" cmp -s "$CFG.bak-cadre" "$T/cfg.orig"
check "mode kept" test "$(mode "$CFG")" = 0o600
check "already trusted reported" bash -c "cadre project trust t1 | grep -q 'already trusted'"
cp "$CFG" "$T/cfg.before"
cadre project add t2 "$T/remote.git" --no-trust >/dev/null
check "--no-trust leaves the config alone" cmp -s "$CFG" "$T/cfg.before"
check "persona sessions cannot trust" bash -c "! CADRE_PERSONA=x cadre project trust t2 2>/dev/null"
# A writer that changes the config while cadre writes: once (cadre retries
# and keeps the change), then on every attempt (cadre gives up).
cat > "$T/race-once.sh" <<'EOF'
#!/bin/sh
if [ "$1" = 1 ]; then python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); d["writer"]=1; json.dump(d, open(sys.argv[1], "w"))' "$2"; fi
EOF
cat > "$T/race-always.sh" <<'EOF'
#!/bin/sh
python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); d["writer"]=int(sys.argv[2]); json.dump(d, open(sys.argv[1], "w"))' "$2" "$1"
EOF
chmod +x "$T/race-once.sh" "$T/race-always.sh"
CADRE_TEST_JSON_EDIT_HOOK="$T/race-once.sh" cadre project trust t2 >/dev/null
check "a change while writing: retried, both kept" py 'import json,sys; d=json.load(open(sys.argv[1])); sys.exit(0 if d.get("writer")==1 and d["projects"][sys.argv[2]]["hasTrustDialogAccepted"] else 1)' "$CFG" "$HOME/Developer/t2"
cadre project add t3 "$T/remote.git" --no-trust >/dev/null
out=$(CADRE_TEST_JSON_EDIT_HOOK="$T/race-always.sh" cadre project trust t3)
check "a file that keeps changing is left alone" bash -c "grep -q 'kept changing' <<<'$out' && ! grep -q 'Developer/t3' '$CFG'"
check "the release binary has no race hook" bash -c "! grep -a -q CADRE_TEST_ '$T/rel/cadre'"

echo "resolution (N.3)"
cd "$T"
check "outside every cadre: the default" bash -c "cadre ls | grep -q '^cadre demo  (~/.cadre/demo, the default cadre)'"
check "in a cadre's folder" bash -c "cd '$HOME/.cadre/life' && cadre ls | grep -q '^cadre life  (~/.cadre/life, from this folder)'"
check "in a linked project" bash -c "cd '$HOME/Developer/app' && cadre ls | grep -q 'from the project app'"
cat >> "$HOME/.cadre/life/projects.yaml" <<EOF

app:
  repo: $T/remote.git
  path: ~/Developer/app
EOF
check "a project two cadres link: refused without a terminal" bash -c "cd '$HOME/Developer/app' && cadre ls 2>&1 | grep -q 'app is linked by demo and life'"
check "CADRE_HOME wins there" bash -c "cd '$HOME/Developer/app' && CADRE_HOME='$HOME/.cadre/life' cadre ls | grep -q '^cadre life'"
git -C "$HOME/.cadre/life" checkout -q -- projects.yaml

echo "sessions"
cd "$C"
out=$(cadre up dev/engineer app)
check "up starts the persona" grep -q "demo-dev-app-engineer started in $HOME/Developer/app" <<<"$out"
check "the persona settings file is made" test -f "$C/.claude/persona-settings.json"
check "tmux session named with the cadre" running cadre-demo-dev-app
check "it records its cadre" test "$(tm show-options -qv -t =cadre-demo-dev-app: @cadre_home)" = "$C"
check "the persona works in the project" test "$(tm display -p -t =cadre-demo-dev-app:=engineer '#{pane_current_path}')" = "$HOME/Developer/app"
args=$(args_of demo-dev-app-engineer)
check "named for messaging" grep -A1 -x -- --name <<<"$args"
check "CADRE_HOME pinned" test "$(cat "$T/home-demo-dev-app-engineer")" = "$C"
copy=$(grep -A1 -x -- --settings <<<"$args" | tail -1)
check "a validated copy, not the file" bash -c "case '$copy' in '$C/.claude/build/persona-settings.'*.json) exit 0 ;; *) exit 1 ;; esac"
check "the copy is read-only" test "$(mode "$copy")" = 0o400
check "the copy denies cadre's own files" grep -q '//\*\*/.cadre/\*/personas/\*\*' "$copy"
check "the copy names this cadre by its path" grep -qF "Edit(/$C/personas/**)" "$copy"
check "ls shows it" bash -c "cadre ls | grep -q '^  dev app *engineer'"
check "ls --json names its runtime" bash -c "cadre ls --json | grep -q '\"runtime\": \"claude\"'"
check "a second up: already running" bash -c "cadre up dev/engineer app | grep -q 'already running'"
cd "$HOME/.cadre/life"
cadre up dev/engineer >/dev/null
check "another cadre's team runs apart" running cadre-life-dev
check "and its ls does not show demo's" bash -c "! cadre ls | grep -q 'dev app'"
tm new-session -d -s cadre-dev -n pm "sleep 300"
check "a legacy session shows in the default cadre" bash -c "cd '$C' && cadre ls | grep -q 'dev (legacy)'"
check "and not in another" bash -c "! cadre ls | grep -q legacy"
tm new-session -d -s cadre-life-research "sleep 300"
tm set-option -t =cadre-life-research: @cadre_home /elsewhere/life
check "up refuses a session name another cadre holds" bash -c "cadre up research/writer 2>&1 | grep -q 'belongs to cadre life (/elsewhere/life)'"
tm kill-session -t =cadre-life-research
check "stop without a terminal asks for --yes" bash -c "! cadre stop </dev/null 2>/dev/null && cadre stop </dev/null 2>&1 | grep -q 'run with --yes'"
check "persona sessions cannot stop a whole cadre" bash -c "! CADRE_PERSONA=x cadre stop --all --yes 2>/dev/null"
out=$(cadre stop --yes)
check "stop stops this cadre only" bash -c "grep -q 'stopped every session of cadre life' <<<'$out' && running cadre-demo-dev-app"
tm new-session -d -s mywork "sleep 300"
tm new-session -d -s cadre-self "cadre stop --all --yes > '$T/stop.out' 2>&1"
for _ in $(seq 50); do running cadre-self || break; sleep 0.2; done
check "stop --all stops every cadre session" bash -c "! tm ls -F '#S' | grep -q '^cadre-'"
check "its own session last, after the summary" bash -c "grep -A1 'stopped every cadre session' '$T/stop.out' | grep -q 'stopping cadre-self last'"
check "other tmux sessions are left" running mywork
tm kill-session -t =mywork

echo "persona settings"
cd "$C"
PS="$C/.claude/persona-settings.json"
relaunch() { cadre stop dev/engineer app >/dev/null; rm -f "$T/args-demo-dev-app-engineer"; cadre up dev/engineer app 2>&1; }
cp "$PS" "$T/ps.good"
printf '{"hooks": {}}' > "$PS"
out=$(relaunch)
check "an unusable file: warned, with the reason" grep -q "personas start with no grants, only cadre's deny rules, because $PS cannot be used: it has the key hooks" <<<"$out"
args=$(args_of demo-dev-app-engineer)
copy=$(grep -A1 -x -- --settings <<<"$args" | tail -1)
check "and the persona still gets every deny rule" bash -c "test -n '$copy' && grep -q 'cadre allow:\*' '$copy' && grep -qF 'Edit(/$C/personas/**)' '$copy' && grep -q '\"allow\": \[\]' '$copy'"
cp "$T/ps.good" "$PS"
py 'import json,sys; d=json.load(open(sys.argv[1])); d["permissions"]["allow"].append("Bash(curl *)"); json.dump(d, open(sys.argv[1], "w"), indent=2)' "$PS"
check "an edit outside cadre allow is warned about" bash -c "cadre stop dev/engineer app >/dev/null; cadre up dev/engineer app 2>&1 | grep -q 'changed outside cadre allow'"
git -C "$C" checkout -q -- .claude/persona-settings.json
cadre stop dev app >/dev/null

echo "allow"
cp "$PS" "$T/ps.before"
for rule in 'Bash(bash *)' 'Bash(npm test && bash *)' 'Bash(echo x#; bash *)' 'Bash(npm test ;>x bash *)' 'Edit(~/.zshrc)' \
    'Edit(~/.ss[h]/config)' "Edit(//$C/cadre.conf)" "Edit(//$C/personas/**)" 'Edit(~/.cadre/config/default)' \
    'Edit(~/.local\/bin/cadre)' 'Bash(cadre allow add x)' 'WebFetch(domain:*.com)' '*'; do
  if err=$(cadre allow add "$rule" 2>&1); then fail "refused: $rule"; fi
  grep -q "refused" <<<"$err" || fail "refused with a reason: $rule"
  cmp -s "$PS" "$T/ps.before" || fail "refused leaves the file: $rule"
done
ok "bypass rules refused, file unchanged"
for rule in "Bash(grep -E 'a|b' src/x.txt)" 'Bash(git commit -m "fix; typo")' 'Bash(npm test 2>&1)' "Bash(echo ';;' x)" \
    'Edit(docs/**/*.md)' 'Edit(src/app/[id]/**)' "Edit(//$HOME/.cadre/demo/teams/**)" 'Read(~/Documents/notes/**)'; do
  cadre allow add "$rule" >/dev/null || fail "accepted: $rule"
  cadre allow remove "$rule" >/dev/null
done
ok "narrow rules accepted"
out=$(cadre allow add 'Bash(git push origin HEAD:main)')
check "add commits" bash -c "git -C '$C' log -1 --format=%s | grep -qx 'Allow for personas: Bash(git push origin HEAD:main)'"
check "with nothing running, says who gets it" grep -q "Personas started from now on get this change" <<<"$out"
check "an --auto entry about permissions refused" bash -c "! cadre allow add --auto 'Changing persona permissions is approved by the user' 2>/dev/null"
cadre allow add --once 'Bash(make deploy)' >/dev/null
check "one-time grants are marked" bash -c "cadre allow list | grep -q 'Bash(make deploy)   \[once, added just now\]'"
check "up reminds of one-time grants" bash -c "cadre up dev/engineer app | grep -q 'one-time grants are still in place'"
out=$(cadre allow add 'Bash(true)')
check "a running persona is listed to restart" grep -qx "  cadre stop dev/engineer app && cadre up dev/engineer app" <<<"$out"
cadre stop dev app >/dev/null
check "remove --once" bash -c "cadre allow remove --once | grep -q 'removed: Bash(make deploy)'"
for i in 1 2 3 4 5 6 7 8; do cadre allow add "Bash(echo p$i)" >/dev/null & done; wait
check "concurrent adds all land" test "$(cadre allow list | grep -c 'Bash(echo p')" = 8
check "persona cannot add" bash -c "CADRE_PERSONA=x cadre allow add 'Bash(true)' 2>&1 | grep -q 'persona sessions cannot change permissions'"

echo "runtime boundary (P)"
echo fake > "$C/personas/ops/sre.runtime"
cat > "$T/fake-agent" <<EOF
#!/bin/sh
{ printf '%s\\n' "\$@"; env; } > "$T/fake-ran"
exec sleep 300
EOF
chmod +x "$T/fake-agent"
export CADRE_FAKE_BIN="$T/fake-agent"
cadre up ops/sre >/dev/null
for _ in $(seq 50); do [ -s "$T/fake-ran" ] && break; sleep 0.1; done
check "what runs is what the runtime's Launch built" bash -c "grep -qx -- '--fake-name' '$T/fake-ran' && grep -qx 'CADRE_FAKE_LAUNCHED=demo-ops-sre' '$T/fake-ran'"
check "ls --json names the fake runtime" bash -c "cadre ls --json | grep -q '\"runtime\": \"fake\"'"
cadre stop ops >/dev/null
check "a runtime without fixed denies is refused" bash -c "CADRE_FAKE_OFF=FixedDenies cadre up ops/sre 2>&1 | grep -q 'cannot enforce cadre.s fixed denies'"
check "a runtime without messaging is refused" bash -c "CADRE_FAKE_OFF=Messaging cadre up ops/sre 2>&1 | grep -q 'no way to message the orchestrator'"
check "an unknown mode is refused" bash -c "CADRE_PERMISSION_MODE=auto cadre up ops/sre 2>&1 | grep -q 'has no permission mode auto'"
check "a release build refuses the fake runtime" bash -c "'$T/rel/cadre' up ops/sre 2>&1 | grep -q 'runtime fake is not supported yet (supported: claude)'"
check "and ls names the problem" bash -c "'$T/rel/cadre' ls | grep -q 'problem: ops/sre: runtime fake is not supported yet'"
rm "$C/personas/ops/sre.runtime"
mkdir -p "$T/nopy"
for tool in tmux git; do ln -s "$(command -v "$tool")" "$T/nopy/$tool"; done
check "no python needed: cadre runs with only tmux and git on PATH" bash -c "! PATH='$T/nopy' command -v python3 && PATH='$T/nopy' '$T/bin/cadre' ls >/dev/null"

echo "help"
check "help lists the visible commands" bash -c "cadre help | grep -q 'cadre stop' && ! cadre help | grep -q 'cadre allow'"
check "help advanced lists the rest" bash -c "cadre help advanced | grep -q 'cadre allow add'"
check "renamed names point to the new one" bash -c "cadre which 2>&1 | grep -q 'use cadre ls'"
check "old names keep working" bash -c "cadre version | grep -q '^cadre ' && cadre down dev | grep -q 'not running'"

echo "$pass checks passed"
