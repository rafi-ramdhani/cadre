#!/usr/bin/env bash
# The bridge for 0.1.x installs: installs cadre 0.1.x the way its README
# said (install.sh on stdin, with the orchestrator hook), from tag v0.1.1
# and from main as it is before the release, then pulls a new main that
# merges this commit, as a 0.1.x user's git pull would. The old hook
# must then exit 0 and print nothing, the old cadre command must print the
# Cadrei line and exit 1, and the old skill link must reach the bridge
# skill. Everything runs in a throwaway HOME, from a local copy of this
# repository; nothing is fetched. It tests the last commit, so commit
# before running it.
#
#   tests/bridge.sh

set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
T=$(cd "$(mktemp -d)" && pwd -P)
trap 'rm -rf "$T"' EXIT
export HOME="$T/home"
mkdir -p "$HOME" "$T/bin"
unset CADRE_HOME CADRE_MEMBER CADRE_PERSONA CADRE_OFF CADRE_ORCHESTRATOR CADREI_HOME CADREI_MEMBER CADREI_OFF CADREI_ORCHESTRATOR CLAUDE_CONFIG_DIR XDG_CONFIG_HOME XDG_CACHE_HOME
export GIT_CONFIG_GLOBAL="$T/gitconfig"
git config --global user.name "Cadre Test"
git config --global user.email "test@example.com"
git config --global init.defaultBranch main
# 0.1.1's installer checks that claude is installed; nothing runs it here.
printf '#!/bin/sh\nexit 0\n' > "$T/bin/claude"
chmod +x "$T/bin/claude"
export PATH="$T/bin:$PATH"

pass=0
fail() { printf '  FAIL %s\n' "$1"; exit 1; }
ok() { pass=$((pass + 1)); printf '  ok  %s\n' "$1"; }

for tool in git tmux python3 jq; do
  command -v "$tool" >/dev/null || fail "$tool is needed to install 0.1.1"
done
git -C "$ROOT" rev-parse -q --verify 'v0.1.1^{commit}' >/dev/null || fail "the v0.1.1 tag is not here (fetch tags)"

# bridge <ref>: a 0.1.x install from <ref> (its main when the user
# installed), then a pull of the new main: the merge of this commit into
# that main, with this commit's files, as the release merge makes it.
bridge() {
  local ref=$1 d="$T/$2" old new fw hook out code args
  mkdir -p "$d"
  old=$(git -C "$ROOT" rev-parse "$ref^{commit}")
  git init -q --bare "$d/cadre.git"
  git -C "$ROOT" push -q "$d/cadre.git" "$old:refs/heads/main"
  git -C "$ROOT" show "$old:install.sh" > "$d/install.sh"

  echo "install 0.1.x from $ref"
  rm -rf "${T:?}/home"; mkdir -p "$T/home"
  CADRE_REPO="$d/cadre.git" bash -s -- demo --dir "$d/docs" --yes --orchestrator-default < "$d/install.sh" > "$d/install.log" 2>&1 ||
    { cat "$d/install.log"; fail "$ref installs"; }
  fw="$d/docs/demo/projects/cadre"
  hook=$(jq -r '.hooks.SessionStart[].hooks[].command' "$HOME/.claude/settings.json")
  [ "$hook" = "bash $fw/bin/orchestrator-hook.sh" ] || fail "$ref added its hook (got: $hook)"
  out=$(sh -c "$hook" 2>&1) || true
  case $out in *orchestrator*) ok "before the pull, the old hook speaks" ;; *) fail "the old hook's output: $out" ;; esac

  echo "pull the new main"
  git -C "$ROOT" push -q "$d/cadre.git" HEAD:refs/heads/rewrite
  new=$(git -C "$d/cadre.git" commit-tree -p main -p rewrite -m "Merge rewrite/0.2.0" 'rewrite^{tree}')
  git -C "$d/cadre.git" update-ref refs/heads/main "$new"
  git -C "$fw" pull -q --ff-only || fail "the install fast-forwards to the new main"
  ok "the install fast-forwards to the new main"

  code=0; out=$(sh -c "$hook" 2>&1) || code=$?
  if [ "$code" != 0 ] || [ -n "$out" ]; then fail "the hook exits 0 and prints nothing (exit $code, output: $out)"; fi
  ok "the hook exits 0 and prints nothing"
  code=0; out=$(CADRE_PERSONA=x CADRE_OFF=1 sh -c "$hook" 2>&1) || code=$?
  if [ "$code" != 0 ] || [ -n "$out" ]; then fail "the hook is as quiet in a member session (exit $code)"; fi
  ok "the hook is as quiet in a member session"

  for args in "" "ls" "up dev my-app" "version"; do
    code=0
    # shellcheck disable=SC2086 # each word is an argument
    out=$("$HOME/.local/bin/cadre" $args 2>&1) || code=$?
    if [ "$code" != 1 ] || [ "$out" != "$want" ]; then fail "cadre $args prints the Cadrei line and exits 1 (exit $code, output: $out)"; fi
  done
  ok "the old cadre command prints the Cadrei line and exits 1, whatever it is given"

  skill="$HOME/.claude/skills/cadre/SKILL.md"
  if ! grep -qx "name: cadre" "$skill" || ! grep -q "brew install rafi-ramdhani/cadrei/cadrei" "$skill"; then
    fail "the old skill link resolves to the bridge skill"
  fi
  ok "the old skill link resolves to the bridge skill"
}

want="Cadre is now Cadrei: brew install rafi-ramdhani/cadrei/cadrei"
bridge v0.1.1 tag
# main as it is before the release merge, when this clone has it (CI does).
if git -C "$ROOT" rev-parse -q --verify 'refs/remotes/origin/main^{commit}' >/dev/null; then
  bridge refs/remotes/origin/main main
fi

echo "$pass checks passed"
