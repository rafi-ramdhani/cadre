package session

import "strings"

// Hint is the way back out of tmux, with the user's actual prefix:
// "Ctrl-b then d: back to your terminal" (M.3).
func (t Tmux) Hint() string {
	prefix, err := t.run("show-options", "-gv", "prefix")
	if err != nil || strings.TrimSpace(prefix) == "" {
		prefix = "C-b"
	}
	key := strings.TrimSpace(prefix)
	key = strings.Replace(key, "C-", "Ctrl-", 1)
	key = strings.Replace(key, "M-", "Alt-", 1)
	return key + " then d: back to your terminal"
}

// HintOptions are the session-level settings that show the hint: in the
// status line, and as a message when a client attaches. The user's global
// tmux settings are not changed.
func (t Tmux) HintOptions() ([]Option, []Option) {
	h := t.Hint()
	return []Option{{"status-right", h}},
		[]Option{{"client-attached", `display-message -d 4000 "` + h + `"`}}
}
