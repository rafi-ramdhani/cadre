# The README's demo: from a fresh machine to a team at work, in about a
# minute of screen time. Recorded with VHS (https://github.com/charmbracelet/vhs):
#
#   vhs docs/demo.tape        # writes docs/demo.gif
#
# Before recording:
# - Use a throwaway HOME or a separate user account. The first run creates
#   ~/.cadre and offers the skill link and the hook in ~/.claude, so a
#   recording must never run against a setup someone uses.
# - In that HOME: cadre 0.2.0 on PATH, tmux, git, and Claude Code installed
#   and logged in (run claude once there and log in). Claude Code asks once
#   to trust the new cadre's folder; the tape accepts it on screen.
# - A small git repository at ~/src/notes-app with an origin remote, for
#   the request to work on. Its name and the request below can change; keep
#   the request small so the team finishes within the hidden wait.
# - Afterwards, cadre stop --yes in that HOME stops the members.
# - The Sleep lengths depend on the machine and on Claude Code's speed;
#   tune them after a first take.

Output docs/demo.gif

Require cadre
Require claude
Require tmux
Require git

Set Shell "bash"
Set FontSize 18
Set Width 1280
Set Height 760
Set Padding 24
Set TypingSpeed 45ms
Set Theme "Catppuccin Mocha"

Hide
Type "cd ~/src/notes-app && export PS1='$ ' && clear"
Enter
Show

# The first run: a new cadre, named demo.
Type "cadre"
Sleep 600ms
Enter
Sleep 1.5s
Type "new"
Enter
Sleep 800ms
Type "demo"
Enter
Sleep 1.2s

# Link this folder to the cadre (and trust it): yes.
Type "y"
Enter
Sleep 1.2s
# Make every new Claude Code session the orchestrator: no, for the demo.
Type "n"
Enter
Sleep 1s
# Link the cadre skill into Claude Code: yes.
Type "y"
Enter

# The greeting, then the orchestrator opens; trust its folder once.
Sleep 6s
Enter
Sleep 4s

Type "Add a --version flag to notes-app, then have the reviewer check it."
Sleep 800ms
Enter
Sleep 20s

# The team works; skip the wait.
Hide
Sleep 180s
Show

# What runs, from inside the orchestrator.
Type "! cadre ls"
Sleep 500ms
Enter
Sleep 5s

# The orchestrator's report, then leave.
Sleep 6s
Type "/exit"
Enter
Sleep 1.5s

# Watch the team directly, then detach (the tmux prefix, then d).
Type "cadre attach dev notes-app"
Enter
Sleep 6s
Ctrl+B
Type "d"
Sleep 2s
