package session

import (
	"fmt"
	"io"
)

// StopTeam stops the session that runs key for this cadre (its own, else
// a legacy one) and returns the line to print.
func (s Scope) StopTeam(team, project string) string {
	if live := s.Live(team, project); live != "" && s.T.KillSession(live) == nil {
		return "  " + live + " stopped"
	}
	return "  " + SessionName(s.Name, Key(team, project)) + " not running" + s.insteadNote(team, project)
}

// insteadNote names the team that runs under the session name instead.
func (s Scope) insteadNote(team, project string) string {
	if other := s.Instead(team, project); other != "" {
		return " (" + other + " runs under that name, and was left as it is)"
	}
	return ""
}

// StopRole stops one member: its window in this cadre's session, or in a
// legacy one.
func (s Scope) StopRole(team, project, role string) string {
	key := Key(team, project)
	for _, session := range []string{s.mine(team, project), s.legacy(key)} {
		if session != "" && s.T.KillWindow(session, role) == nil {
			return "  " + session[len("cadre-"):] + "-" + role + " stopped"
		}
	}
	return "  " + MemberName(s.Name, key, role) + " not running" + s.insteadNote(team, project)
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
