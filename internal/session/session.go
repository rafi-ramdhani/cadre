package session

import (
	"fmt"
	"path/filepath"
	"sort"
)

// Names (section I.3): a team's tmux session is cadre-<cadre>-<team>, or
// cadre-<cadre>-<team>-<project> for a project; its windows are roles; a
// member's Claude session name is <cadre>-<team>[-<project>]-<role>. Names
// are addresses, never parsed: a session's cadre, team and project are its
// tmux options.

// Key is a team, or a team and its project.
func Key(team, project string) string {
	if project == "" {
		return team
	}
	return team + "-" + project
}

// SessionName is a team's tmux session.
func SessionName(cadre, key string) string { return "cadre-" + cadre + "-" + key }

// MemberName is a member's Claude session name.
func MemberName(cadre, key, role string) string { return cadre + "-" + key + "-" + role }

// LegacyName is the tmux session 0.1.x gave a team: no cadre in it.
func LegacyName(key string) string { return "cadre-" + key }

// Scope is the cadre a command acts on.
type Scope struct {
	Name    string // the cadre's name
	Path    string // physical
	Default bool   // it is the default cadre, which legacy sessions belong to
	T       Tmux
}

// Ours reports whether a session belongs to this cadre: its own, or a
// legacy session (no @cadre_home) when this is the default cadre.
func (s Scope) Ours(i Info) bool { return i.Home == s.Path || (i.Home == "" && s.Default) }

// Running lists this cadre's member sessions (not its orchestrator),
// sorted by name.
func (s Scope) Running() []Info {
	var out []Info
	for _, i := range s.T.Sessions() {
		if s.Ours(i) && i.Role != "orchestrator" {
			out = append(out, i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// mine returns this cadre's own session for key, when it runs.
func (s Scope) mine(key string) string {
	name := SessionName(s.Name, key)
	if s.T.Has(name) && s.T.Option(name, "@cadre_home") == s.Path {
		return name
	}
	return ""
}

// legacy returns, in the default cadre, the legacy session for key, when it
// runs.
func (s Scope) legacy(key string) string {
	name := LegacyName(key)
	if s.Default && s.T.Has(name) && s.T.Option(name, "@cadre_home") == "" {
		return name
	}
	return ""
}

// Live returns the session that runs key for this cadre: its own, else a
// legacy one.
func (s Scope) Live(key string) string {
	if m := s.mine(key); m != "" {
		return m
	}
	return s.legacy(key)
}

// CheckSession refuses when this cadre's session name for team and project
// is taken by another cadre, a legacy session, or another team and
// project whose names join to the same key.
func (s Scope) CheckSession(team, project string) error {
	name := SessionName(s.Name, Key(team, project))
	if !s.T.Has(name) {
		return nil
	}
	home := s.T.Option(name, "@cadre_home")
	switch {
	case home == "":
		return fmt.Errorf("tmux session %s is a legacy session from before cadre names, not cadre %s's; stop it, or rename a team or this cadre's folder", name, s.Name)
	case home != s.Path:
		return fmt.Errorf("tmux session %s belongs to cadre %s (%s), not cadre %s (%s); rename a team or one of the cadre folders", name, filepath.Base(home), home, s.Name, s.Path)
	}
	t, p := s.T.Option(name, "@cadre_team"), s.T.Option(name, "@cadre_project")
	if t != "" && (t != team || p != project) {
		held := t
		if p != "" {
			held += " for project " + p
		}
		return fmt.Errorf("tmux session %s holds team %s, whose names join to the same session name; rename a team or project", name, held)
	}
	return nil
}

// CheckMember refuses when the session name a member would get (its
// messaging address) is already used by a window of another session
// (another cadre, team, project or role whose names join the same way),
// since a name must reach one session only.
func (s Scope) CheckMember(session, window, member string) error {
	for _, p := range s.T.Members() {
		if p.Name == member && !(p.Session == session && p.Window == window) {
			return fmt.Errorf("the session name %s is already used by window %s of tmux session %s; rename a team, role or cadre folder so the names differ", member, p.Window, p.Session)
		}
	}
	return nil
}

// Group is the label of a session in a listing of every cadre: its cadre's
// name, "legacy" or "unknown".
func Group(i Info, known map[string]string) string {
	switch {
	case i.Home == "":
		return "legacy sessions (from before cadre names)"
	case known[i.Home] != "":
		return "cadre " + known[i.Home]
	}
	return "unknown cadre at " + i.Home
}

// All lists every member session on the server, sorted by group and name,
// with known mapping a cadre's path to its name.
func All(t Tmux, known map[string]string) []Info {
	var out []Info
	for _, i := range t.Sessions() {
		if i.Role != "orchestrator" {
			out = append(out, i)
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		ga, gb := Group(out[a], known), Group(out[b], known)
		if ga != gb {
			return ga < gb
		}
		return out[a].Name < out[b].Name
	})
	return out
}

// Members returns the member names of a session: its windows, named as
// <session without "cadre-">-<role>.
func (t Tmux) MemberNames(i Info) []string {
	var out []string
	for _, w := range t.Windows(i.Name) {
		out = append(out, i.Name[len("cadre-"):]+"-"+w)
	}
	return out
}
