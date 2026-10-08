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
git -C "$T/seed" -c user.name=t -c user.email=t@t commit -q --allow-empty -m init
git -C "$T/seed" push -q origin HEAD 2>/dev/null
cadre add project app "$T/remote.git" dev "A test app" >/dev/null
check "project cloned into projects/" test -d "$C/projects/app/.git"
check "project listed" bash -c "cadre projects | grep -q app"
check "path resolves" test "$(cadre path app)" = "$C/projects/app"
cadre add team ops >/dev/null
cadre add persona ops/sre >/dev/null
check "persona created" test -f "$C/personas/ops/sre.md"
check "duplicate project refused" bash -c "! cadre add project app '$T/remote.git'"

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

echo "restore on a new machine"
rm -rf "$HOME/.local" "$HOME/.config"
CADRE_REPO="$ROOT" bash "$ROOT/install.sh" --from "$C" --dir "$T/machine2" --yes --no-hook >/dev/null
check "cadre cloned" test -f "$T/machine2/demo/playbook.md"
check "projects cloned by sync" test -d "$T/machine2/demo/projects/app/.git"
check "active cadre switched" grep -qx "$T/machine2/demo" "$HOME/.config/cadre/home"

echo "$pass checks passed"
