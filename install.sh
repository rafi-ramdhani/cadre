#!/usr/bin/env bash
# Cadre installer: one command from nothing to a working cadre.
#
#   curl -fsSL https://raw.githubusercontent.com/rafi-ramdhani/cadre/main/install.sh | bash -s -- <name>
#   ./install.sh <name>                  same, from a clone of this repo
#
# It checks the tools cadre needs, generates your cadre (<parent>/<name>) from
# the template, places the framework inside it at projects/cadre, links the
# `cadre` command and the orchestrator skill, and optionally adds the hook that
# makes every new Claude Code session the orchestrator.
#
# Options:
#   --dir <parent>          where the cadre folder goes (default: ~/Documents)
#   --from <owner/repo>     clone an existing cadre instead of generating one
#                           (a new machine), then clone its projects
#   --orchestrator-default  add the orchestrator hook without asking
#   --no-hook               skip the hook without asking
#   --yes                   accept defaults, ask nothing
#   --link-only             only relink the command, skill and hook from
#                           this clone (after moving it)
#
# CADRE_REPO overrides where the framework is cloned from.

set -euo pipefail

CADRE_REPO=${CADRE_REPO:-rafi-ramdhani/cadre}
NAME="" PARENT="$HOME/Documents" FROM="" HOOK="ask" YES="" LINK_ONLY=""

say() { printf '%s\n' "$*"; }
die() { printf 'cadre install: %s\n' "$*" >&2; exit 1; }
usage() { sed -n '2,24p' "${BASH_SOURCE[0]:-$0}" 2>/dev/null | sed 's/^# \{0,1\}//'; }

while [ $# -gt 0 ]; do
  case $1 in
    --dir) PARENT=${2:?--dir needs a folder}; shift ;;
    --from) FROM=${2:?--from needs owner/repo}; shift ;;
    --orchestrator-default) HOOK=yes ;;
    --no-hook) HOOK=no ;;
    --yes|-y) YES=1 ;;
    --link-only) LINK_ONLY=1 ;;
    -h|--help) usage; exit 0 ;;
    -*) die "unknown option $1 (see --help)" ;;
    *) NAME=$1 ;;
  esac
  shift
done

# Questions go to the terminal even when the script itself arrives on stdin.
ask() {
  local prompt=$1 default=$2 reply=""
  if [ -n "$YES" ] || ! { true </dev/tty; } 2>/dev/null; then echo "$default"; return; fi
  read -r -p "$prompt" reply </dev/tty || true
  echo "${reply:-$default}"
}

# Clone a repo given as owner/repo (via gh when available), a URL or a path.
clone() {
  mkdir -p "$(dirname "$2")"
  case $1 in
    */*/*|/*|.*|*:*) git clone -q "$1" "$2" ;;
    *) if command -v gh >/dev/null && gh auth status >/dev/null 2>&1; then gh repo clone "$1" "$2" -- -q
       else git clone -q "https://github.com/$1.git" "$2"; fi ;;
  esac
}

link() {
  local root=$1
  mkdir -p "$HOME/.local/bin" "$HOME/.claude/skills"
  ln -sfn "$root/bin/cadre" "$HOME/.local/bin/cadre"
  ln -sfn "$root/skills/cadre" "$HOME/.claude/skills/cadre"
  say "  linked the cadre command and skill from $root"
}

add_hook() {
  local root=$1 settings="$HOME/.claude/settings.json" cmd tmp
  cmd="bash $root/bin/orchestrator-hook.sh"
  command -v jq >/dev/null || { say "  skipped the hook: jq is not installed"; return; }
  [ -f "$settings" ] || echo '{}' > "$settings"
  if ! jq -e . "$settings" >/dev/null 2>&1; then
    say "  skipped the hook: $settings is not plain JSON; left it unchanged"; return
  fi
  if jq -e --arg cmd "$cmd" '[.hooks.SessionStart[]?.hooks[]?.command // "" | select(test("orchestrator-hook\\.sh"))] == [$cmd]' "$settings" >/dev/null 2>&1; then
    say "  the orchestrator hook is already in $settings"; return
  fi
  cp "$settings" "$settings.bak-cadre"
  # A copy keeps the file's mode; the original is replaced only when jq succeeds.
  tmp="$settings.tmp-cadre"
  cp -p "$settings" "$tmp"
  if jq --arg cmd "$cmd" '
    .hooks.SessionStart = (
      [ (.hooks.SessionStart // [])[]
        | select(all(.hooks[]; (.command // "") | test("orchestrator-hook\\.sh") | not)) ]
      + [ { "hooks": [ { "type": "command", "command": $cmd, "timeout": 10 } ] } ]
    )' "$settings.bak-cadre" > "$tmp"; then
    mv "$tmp" "$settings"
    say "  added the orchestrator hook to $settings (backup: $settings.bak-cadre)"
  else
    rm -f "$tmp"
    say "  skipped the hook: could not edit $settings; left it unchanged"
  fi
}

hook_installed() { grep -q 'orchestrator-hook\.sh' "$HOME/.claude/settings.json" 2>/dev/null; }

# Where this script lives, when it runs from a clone rather than from curl.
HERE=""
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/bin/cadre" ]; then
  HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
fi

if [ -n "$LINK_ONLY" ]; then
  [ -n "$HERE" ] || die "--link-only runs from a clone of the framework"
  link "$HERE"
  if [ "$HOOK" = yes ] || { [ "$HOOK" = ask ] && hook_installed; }; then add_hook "$HERE"; fi
  exit 0
fi

say "Checking tools"
missing=""
for tool in git tmux python3 claude; do command -v "$tool" >/dev/null || missing="$missing $tool"; done
[ -z "$missing" ] || die "missing:$missing (on macOS: brew install tmux; Claude Code: https://claude.com/claude-code)"
command -v jq >/dev/null || say "  note: jq is not installed; the orchestrator hook needs it (brew install jq)"

if [ -z "$NAME" ]; then
  if [ -n "$FROM" ]; then NAME=$(basename "$FROM" .git); else NAME=$(ask "Name for your cadre (a folder with this name is created): " ""); fi
fi
[ -n "$NAME" ] || die "give your cadre a name: install.sh <name>"
[[ $NAME =~ ^[A-Za-z0-9][A-Za-z0-9_-]*$ ]] || die "the name may use letters, digits, - and _"
mkdir -p "$PARENT"
PARENT=$(cd "$PARENT" && pwd)
DEST="$PARENT/$NAME"
[ -e "$DEST" ] && die "$DEST already exists"

# The framework runs from a temporary copy until the cadre exists, then moves
# into the cadre's projects/ folder like any other project.
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if [ -n "$HERE" ]; then FRAMEWORK_SRC=$HERE; else
  say "Fetching the framework"
  clone "$CADRE_REPO" "$TMP/cadre"
  FRAMEWORK_SRC="$TMP/cadre"
fi

if [ -n "$FROM" ]; then
  say "Cloning your cadre $FROM"
  clone "$FROM" "$DEST"
  [ -d "$DEST/personas" ] || die "$FROM does not look like a cadre (no personas/ folder)"
  CADRE_HOME="$DEST" "$FRAMEWORK_SRC/bin/cadre" use "$DEST" >/dev/null
else
  say "Generating $DEST"
  (cd "$PARENT" && "$FRAMEWORK_SRC/bin/cadre" init "$NAME" >/dev/null)
fi

FRAMEWORK="$DEST/projects/cadre"
mkdir -p "$DEST/projects"
if [ ! -d "$FRAMEWORK" ]; then
  if [ -n "$HERE" ]; then clone "$HERE" "$FRAMEWORK"
    git -C "$FRAMEWORK" remote set-url origin "$(git -C "$HERE" remote get-url origin 2>/dev/null || echo "$CADRE_REPO")" 2>/dev/null || true
  else mv "$TMP/cadre" "$FRAMEWORK"; fi
fi
if ! grep -q '^cadre:' "$DEST/projects.yaml"; then
  printf '\ncadre:\n  repo: %s\n  team: dev\n  about: The cadre framework this cadre is built on\n' "$CADRE_REPO" >> "$DEST/projects.yaml"
  if git -C "$DEST" config user.email >/dev/null; then git -C "$DEST" commit -qam "Register the cadre framework as a project"; fi
fi

say "Linking"
link "$FRAMEWORK"

if [ "$HOOK" = ask ]; then
  case $(ask "Make every new Claude Code session the orchestrator? [y/N] " "n") in [yY]*) HOOK=yes ;; *) HOOK=no ;; esac
fi
[ "$HOOK" = yes ] && add_hook "$FRAMEWORK"

if [ -n "$FROM" ]; then say "Cloning projects"; "$HOME/.local/bin/cadre" sync; fi

say ""
say "Done. Your cadre is $DEST"
case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) say "Add ~/.local/bin to your PATH to use the cadre command." ;; esac
say "Next: cd $DEST && claude, then ask for anything. Add projects with: cadre add project <name> <owner/repo>"
