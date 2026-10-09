package session

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Names (section I.3): a team's tmux session is cadrei-<cadrei>-<team>, or
// cadrei-<cadrei>-<team>-<project> for a project; its windows are roles; a
// member's Claude session name is <cadrei>-<team>[-<project>]-<role>. Names
// are addresses, never parsed: a session's cadrei, team and project are its
// tmux options.

// partRule is a team or role name cadrei starts. The name becomes part of
// tmux and Claude session names and of a path under members/, so it may
// use letters, digits, - and _; that keeps out . and .. too.
var partRule = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NameRule says what a team or role name may use.
const NameRule = "team and member names may use letters, digits, - and _"

// CheckName reports whether a team or role name can be started.
func CheckName(name string) bool { return partRule.MatchString(name) }

// Key is a team, or a team and its project.
func Key(team, project string) string {
	if project == "" {
		return team
	}
	return team + "-" + project
}

// SessionName is a team's tmux session.
func SessionName(cadrei, key string) string { return tmuxName(prefix + cadrei + "-" + key) }

// tmuxName is a session name as tmux keeps it: tmux 3.4 and earlier turn
// a "." into "_" (a project such as my.app), later ones keep it, so cadrei
// writes "_" for every version. Two projects whose names differ only
// there are told apart by the session's @cadrei_project.
func tmuxName(name string) string { return strings.ReplaceAll(name, ".", "_") }

// MemberName is a member's Claude session name.
func MemberName(cadrei, key, role string) string { return cadrei + "-" + key + "-" + role }

// LegacyName is the tmux session 0.1.x gave a team: cadre-<key>, with no
// cadrei name in it.
func LegacyName(key string) string { return tmuxName(legacyPrefix + key) }

// The start of every session name: this program's, and 0.1.x's.
const (
	prefix       = "cadrei-"
	legacyPrefix = "cadre-"
)

// Started reports whether a tmux session name is one this program or
// 0.1.x starts.
func Started(name string) bool {
	return strings.HasPrefix(name, prefix) || strings.HasPrefix(name, legacyPrefix)
}

// Bare is a session name without its cadrei- (or 0.1.x cadre-) start.
func Bare(name string) string {
	if strings.HasPrefix(name, prefix) {
		return name[len(prefix):]
	}
	return strings.TrimPrefix(name, legacyPrefix)
}

// Scope is the cadrei a command acts on.
type Scope struct {
	Name    string // the cadrei's name
	Path    string // physical
	Default bool   // it is the default cadrei, which legacy sessions belong to
	T       Tmux
}

// Ours reports whether a session belongs to this cadrei: its own, or a
// legacy session (no @cadrei_home) when this is the default cadrei.
func (s Scope) Ours(i Info) bool { return i.Home == s.Path || (i.Home == "" && s.Default) }

// Running lists this cadrei's member sessions (not its orchestrator),
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

// mine returns this cadrei's own session for team and project, when it
// runs. The session name alone is not enough: my.app and my_app share
// one, so the session's team and project must match too.
func (s Scope) mine(team, project string) string {
	name := SessionName(s.Name, Key(team, project))
	if s.T.Has(name) && s.T.Option(name, "@cadrei_home") == s.Path &&
		s.T.Option(name, "@cadrei_team") == team && s.T.Option(name, "@cadrei_project") == project {
		return name
	}
	return ""
}

// Instead says which of this cadrei's teams runs under the session name
// that team and project would have, when it is another one ("dev my.app"
// for dev my_app), or "".
func (s Scope) Instead(team, project string) string {
	name := SessionName(s.Name, Key(team, project))
	if !s.T.Has(name) || s.T.Option(name, "@cadrei_home") != s.Path {
		return ""
	}
	t, p := s.T.Option(name, "@cadrei_team"), s.T.Option(name, "@cadrei_project")
	if t == team && p == project {
		return ""
	}
	return strings.TrimSpace(t + " " + p)
}

// legacy returns, in the default cadrei, the legacy session for key, when it
// runs.
func (s Scope) legacy(key string) string {
	if !s.Default {
		return ""
	}
	// tmux 3.5 and later kept a dot in a 0.1.x session's name.
	for _, name := range []string{LegacyName(key), legacyPrefix + key} {
		if s.T.Has(name) && s.T.Option(name, "@cadrei_home") == "" {
			return name
		}
	}
	return ""
}

// Live returns the session that runs key for this cadrei: its own, else a
// legacy one.
func (s Scope) Live(team, project string) string {
	if m := s.mine(team, project); m != "" {
		return m
	}
	return s.legacy(Key(team, project))
}

// CheckSession refuses when this cadrei's session name for team and project
// is taken by another cadrei, a legacy session, or another team and
// project whose names join to the same key.
func (s Scope) CheckSession(team, project string) error {
	name := SessionName(s.Name, Key(team, project))
	if !s.T.Has(name) {
		return nil
	}
	home := s.T.Option(name, "@cadrei_home")
	switch {
	case home == "":
		// 0.1.x sessions are named cadre-<key>, so one without a home here
		// was made by hand.
		return fmt.Errorf("a session named %s exists without cadrei's markers; stop it with tmux kill-session -t %s", name, name)
	case home != s.Path:
		return fmt.Errorf("tmux session %s belongs to cadrei %s (%s), not cadrei %s (%s); rename a team or one of the cadrei folders", name, filepath.Base(home), home, s.Name, s.Path)
	}
	t, p := s.T.Option(name, "@cadrei_team"), s.T.Option(name, "@cadrei_project")
	// A session with this cadrei's home but no team (a start that died
	// before its options were set, or one made by hand) is not one stop
	// and attach match, so up does not start members in it either.
	if t == "" {
		return fmt.Errorf("a session named %s exists without cadrei's markers; stop it with tmux kill-session -t %s, or cadrei stop --yes", name, name)
	}
	if t != team || p != project {
		held := t
		if p != "" {
			held += " for project " + p
		}
		return fmt.Errorf("tmux session %s already holds team %s, which gives the same session name; rename a team or project", name, held)
	}
	return nil
}

// CheckMember refuses when the session name a member would get (its
// messaging address) is already used by a window of another session
// (another cadrei, team, project or role whose names join the same way),
// since a name must reach one session only.
func (s Scope) CheckMember(session, window, member string) error {
	for _, p := range s.T.Members() {
		if p.Name == member && !(p.Session == session && p.Window == window) {
			return fmt.Errorf("the session name %s is already used by window %s of tmux session %s; rename a team, role or cadrei folder so the names differ", member, p.Window, p.Session)
		}
	}
	return nil
}

// Group is the label of a session in a listing of every cadrei: its cadrei's
// name, "legacy" or "unknown".
func Group(i Info, known map[string]string) string {
	switch {
	case i.Home == "":
		return "legacy sessions (started by cadre 0.1.x)"
	case known[i.Home] != "":
		return "cadrei " + known[i.Home]
	}
	return "unknown cadrei at " + i.Home
}

// All lists every member session on the server, sorted by group and name,
// with known mapping a cadrei's path to its name.
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
// <session without its cadrei- start>-<role>.
func (t Tmux) MemberNames(i Info) []string {
	var out []string
	for _, w := range t.Windows(i.Name) {
		out = append(out, Bare(i.Name)+"-"+w)
	}
	return out
}
