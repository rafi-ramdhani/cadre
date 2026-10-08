package session

import (
	"fmt"
	"io"
)

// StopTeam stops the session that runs key for this cadre (its own, else
// a legacy one) and returns the line to print.
func (s Scope) StopTeam(key string) string {
	if live := s.Live(key); live != "" && s.T.KillSession(live) == nil {
		return "  " + live + " stopped"
	}
	return "  " + SessionName(s.Name, key) + " not running"
}

// StopRole stops one persona: its window in this cadre's session, or in a
// legacy one.
func (s Scope) StopRole(key, role string) string {
	for _, session := range []string{s.mine(key), s.legacy(key)} {
		if session != "" && s.T.KillWindow(session, role) == nil {
			return "  " + session[len("cadre-"):] + "-" + role + " stopped"
		}
	}
	return "  " + PersonaName(s.Name, key, role) + " not running"
}

// Stopping stops a list of sessions, leaving the one this command runs in
// for Last, so the caller can print its summary first.
type Stopping struct {
	T      Tmux
	own    string
	Failed []string
}

// Stop stops every session in names except the command's own, printing a
// line each.
func (st *Stopping) Stop(out io.Writer, names []string) {
	st.own = ""
	self := st.T.Own()
	for _, n := range names {
		if n == self {
			st.own = n
			continue
		}
		if st.T.KillSession(n) == nil {
			fmt.Fprintf(out, "  %s stopped\n", n)
		} else {
			st.Failed = append(st.Failed, n)
			fmt.Fprintf(out, "  %s could not be stopped\n", n)
		}
	}
}

// Last stops the command's own session, when it was in the list.
func (st *Stopping) Last(out io.Writer) {
	if st.own == "" {
		return
	}
	fmt.Fprintf(out, "  stopping %s last (this command runs in it)\n", st.own)
	st.T.KillSession(st.own)
}
