//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/conf"
	"github.com/rafi-ramdhani/cadrei/internal/orchestrator"
	"github.com/rafi-ramdhani/cadrei/internal/registry"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	"github.com/rafi-ramdhani/cadrei/internal/session"
)

// The data of cadrei ls, which --json prints as it is, so the orchestrator
// never reads the human layout. Its shape is versioned: a field is added
// under the same version, and anything else changes the version.

// jsonVersion is the version of ls --json's shape.
const jsonVersion = 1

type cadreiView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	From    string `json:"from,omitempty"`    // how this command found it
	Project string `json:"project,omitempty"` // the linked project it was found from
	Default bool   `json:"default"`
	Missing bool   `json:"missing,omitempty"`
}

type memberView struct {
	Name string `json:"name"` // the session name, the member's messaging address
	Role string `json:"role"`
}

type sessionView struct {
	Session string       `json:"session"` // the tmux session
	Team    string       `json:"team"`
	Project string       `json:"project,omitempty"`
	Legacy  bool         `json:"legacy"` // from before session names carried the cadrei
	Members []memberView `json:"members"`
}

// A project's state on this machine: "present"; "not here" (no folder
// recorded on this machine); "missing" (its folder is gone); "drive" (on a
// drive that is not connected). Missing projects are never unlinked by
// cadrei: the orchestrator offers to clone (project sync), link (project
// link, with found as a suggestion) or unlink.
type projectView struct {
	Name   string `json:"name"`
	Repo   string `json:"repo,omitempty"`
	Team   string `json:"team,omitempty"`
	About  string `json:"about,omitempty"`
	Path   string `json:"path"` // "" when not here
	Cloned bool   `json:"cloned"`
	State  string `json:"state"`
	Found  string `json:"found,omitempty"` // a clone of its repo in the projects folder, for a project not present
}

type otherView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Running int    `json:"running"`
	Missing bool   `json:"missing,omitempty"`
}

type orchestratorView struct {
	Running bool       `json:"running"`
	Mode    string     `json:"mode,omitempty"`    // "terminal" or "tmux"
	Session string     `json:"session,omitempty"` // its tmux session, in tmux mode
	Since   *time.Time `json:"since,omitempty"`   // when cadrei opened it
}

type cadreiStatus struct {
	Version      int                     `json:"version,omitempty"` // set on the top-level object
	Cadrei       cadreiView              `json:"cadrei"`
	Orchestrator orchestratorView        `json:"orchestrator"`
	Running      []sessionView           `json:"running"`
	Projects     []projectView           `json:"projects"`
	Teams        map[string][]memberView `json:"teams"` // every member, running or not
	Others       []otherView             `json:"other_cadreis,omitempty"`
	Problems     []string                `json:"problems"` // what keeps members from starting, and registry entries left out
}

type allStatus struct {
	Version int            `json:"version"`
	Cadreis []cadreiStatus `json:"cadreis"`
	Legacy  []sessionView  `json:"legacy"`
	Unknown []sessionView  `json:"unknown"`
}

// problemsOf says what keeps every member of a cadrei from starting: the
// runtime is missing, or cannot run the cadrei's permission mode.
func problemsOf(c cadreis.Cadrei) []string {
	out := append([]string{}, skippedEntries(c)...)
	out = append(out, unstartable(c)...)
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return append(out, err.Error())
	}
	if _, err := rt.Detect(); err != nil {
		return append(out, err.Error())
	}
	raw, _ := os.ReadFile(filepath.Join(c.Path, "cadrei.conf"))
	values, _ := conf.Parse(string(raw))
	if err := runtime.Usable(rt, modeOf(values)); err != nil {
		return append(out, err.Error())
	}
	return out
}

// skippedEntries names the registry entries cadrei leaves out because
// their name is not a project name (one that climbs out of the projects
// folder, such as ../x, or nests, such as a/b).
func skippedEntries(c cadreis.Cadrei) []string {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range reg.Skipped() {
		out = append(out, fmt.Sprintf("projects.yaml has an entry named %q, which is not a project name (letters, digits, ., - and _); it is left out until it is renamed or removed", name))
	}
	return out
}

func viewOf(t session.Tmux, i session.Info) sessionView {
	v := sessionView{Session: i.Name, Team: i.Team, Project: i.Project, Legacy: i.Legacy(), Members: []memberView{}}
	if v.Team == "" {
		// A 0.1.x session recorded no team: show its key.
		v.Team = session.Bare(i.Name)
	}
	for _, w := range t.Windows(i.Name) {
		v.Members = append(v.Members, memberView{Name: session.Bare(i.Name) + "-" + w, Role: w})
	}
	return v
}

// statusOf gathers one cadrei's status.
func statusOf(c cadreis.Cadrei, def string, t session.Tmux) cadreiStatus {
	s := cadreiStatus{Cadrei: cadreiView{Name: c.Name, Path: c.Path, Default: c.Name == def, Missing: !c.Present()},
		Running: []sessionView{}, Projects: []projectView{}, Teams: map[string][]memberView{}, Problems: problemsOf(c)}
	scope := session.Scope{Name: c.Name, Path: c.Path, Default: c.Name == def, T: t}
	s.Orchestrator = orchestratorOf(c, t)
	for _, i := range scope.Running() {
		s.Running = append(s.Running, viewOf(t, i))
	}
	if p := projectsOf(c); p != nil {
		s.Projects = p
	}
	teams, _ := filepath.Glob(filepath.Join(c.Path, "members", "*"))
	for _, d := range teams {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			team := filepath.Base(d)
			s.Teams[team] = []memberView{}
			for _, role := range session.Roles(c.Path, team) {
				s.Teams[team] = append(s.Teams[team], memberView{Name: session.MemberName(c.Name, team, role), Role: role})
			}
		}
	}
	return s
}

// orchestratorOf says whether a cadrei's orchestrator is open: its lock
// (plain cadrei or cadrei --tmux), or its tmux session.
func orchestratorOf(c cadreis.Cadrei, t session.Tmux) orchestratorView {
	if rt, err := runtime.Get(runtimeName()); err == nil {
		if l := openOrchestrator(t, c.Name, c.Path, orchestrator.LockPath(rt.BuildDir(c.Path)), false); l != nil {
			since := l.Since
			return orchestratorView{Running: true, Mode: l.Mode, Session: l.Session, Since: &since}
		}
	}
	name := orchestrator.SessionName(c.Name)
	if t.Has(name) && t.Option(name, "@cadrei_role") == "orchestrator" && t.Option(name, "@cadrei_home") == c.Path {
		return orchestratorView{Running: true, Mode: orchestrator.Tmux, Session: name}
	}
	return orchestratorView{}
}

func runLs(e *env) int {
	all, asJSON := false, false
	for _, a := range e.args {
		switch a {
		case "--all":
			all = true
		case "--json":
			asJSON = true
		default:
			return e.fail("usage: cadrei ls [--all] [--json]")
		}
	}
	t := session.Default()
	def := cadreis.Default()
	list, _ := cadreis.List()
	if all {
		st := allStatus{Version: jsonVersion, Cadreis: []cadreiStatus{}, Legacy: []sessionView{}, Unknown: []sessionView{}}
		known := map[string]bool{}
		for _, c := range list {
			known[c.Path] = true
			if c.Present() {
				st.Cadreis = append(st.Cadreis, statusOf(c, def, t))
			}
		}
		// A cadrei- session without markers (made by hand, or a start that
		// died early) is not legacy: it is listed with the unknown ones.
		unmarked := map[string]bool{}
		for _, i := range session.All(t, nil) {
			switch {
			case i.Legacy():
				st.Legacy = append(st.Legacy, viewOf(t, i))
			case i.Home == "":
				unmarked[i.Name] = true
				st.Unknown = append(st.Unknown, viewOf(t, i))
			case !known[i.Home]:
				st.Unknown = append(st.Unknown, viewOf(t, i))
			}
		}
		if asJSON {
			return e.printJSON(st)
		}
		for n, c := range st.Cadreis {
			if n > 0 {
				e.say("")
			}
			e.printStatus(c)
		}
		e.printSessions("legacy sessions (started by cadre 0.1.x; they belong to the default cadrei)", st.Legacy)
		for _, u := range st.Unknown {
			heading := "sessions of a cadrei no longer known"
			if unmarked[u.Session] {
				heading = "sessions without cadrei's markers"
			}
			e.printSessions(heading, []sessionView{u})
		}
		return 0
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	e.conf(r) // a status screen says when cadrei.conf has lines it ignores
	st := statusOf(r.Cadrei, r.Default, t)
	st.Version = jsonVersion
	st.Cadrei.From, st.Cadrei.Project = r.From, r.Project
	for _, c := range list {
		if c.Path == r.Path {
			continue
		}
		o := otherView{Name: c.Name, Path: c.Path, Missing: !c.Present()}
		o.Running = len(session.Scope{Name: c.Name, Path: c.Path, Default: c.Name == r.Default, T: t}.Running())
		st.Others = append(st.Others, o)
	}
	if asJSON {
		return e.printJSON(st)
	}
	e.printStatus(st)
	if len(st.Others) > 0 {
		var parts []string
		for _, o := range st.Others {
			switch {
			case o.Missing:
				parts = append(parts, o.Name+" (folder missing)")
			case o.Running == 0:
				parts = append(parts, o.Name+" (none running)")
			default:
				parts = append(parts, fmt.Sprintf("%s (%d running)", o.Name, o.Running))
			}
		}
		e.say("other cadreis: %s", strings.Join(parts, ", "))
	}
	return 0
}

func (e *env) printJSON(v any) int {
	enc := json.NewEncoder(e.stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// printStatus is one cadrei's block of the status screen.
func (e *env) printStatus(s cadreiStatus) {
	where := display(s.Cadrei.Path)
	switch {
	case s.Cadrei.From == "default":
		where += ", the default cadrei"
	case s.Cadrei.From != "":
		where += ", " + strings.Replace(s.Cadrei.From, "from the current folder", "from this folder", 1)
		if s.Cadrei.Default {
			where += "; default"
		}
	case s.Cadrei.Default:
		where += ", default"
	}
	e.say("cadrei %s  (%s)", s.Cadrei.Name, where)
	switch o := s.Orchestrator; {
	case !o.Running:
		e.say("orchestrator: not running")
	case o.Mode == orchestrator.Terminal && o.Since != nil:
		e.say("orchestrator: open in a terminal since %s", o.Since.Format("15:04"))
	default:
		e.say("orchestrator: running in tmux (%s)", o.Session)
	}
	if len(s.Running) == 0 {
		e.say("running: nothing")
	} else {
		e.say("running:")
		legacy := false
		for _, v := range s.Running {
			label := v.Team
			if v.Project != "" {
				label += " " + v.Project
			}
			if v.Legacy {
				label += " (legacy)"
				legacy = true
			}
			var roles []string
			for _, p := range v.Members {
				roles = append(roles, p.Role)
			}
			e.say("  %-16s %s", label, strings.Join(roles, "  "))
		}
		if legacy {
			e.say("  legacy sessions keep their old names until restarted")
		}
	}
	defer func() {
		for _, p := range s.Problems {
			e.say("problem: %s", p)
		}
	}()
	if len(s.Projects) == 0 {
		e.say("projects: none")
		return
	}
	var names, gone, absent, drives []string
	for _, p := range s.Projects {
		names = append(names, p.Name)
		switch p.State {
		case "missing":
			gone = append(gone, p.Name)
		case "not here":
			absent = append(absent, p.Name)
		case "drive":
			drives = append(drives, p.Name)
		}
	}
	line := "projects: " + strings.Join(names, ", ")
	var notes []string
	if len(absent) > 0 {
		notes = append(notes, "not on this machine: "+strings.Join(absent, ", "))
	}
	if len(gone) > 0 {
		notes = append(notes, "missing: "+strings.Join(gone, ", "))
	}
	if len(drives) > 0 {
		notes = append(notes, "on a drive that is not connected: "+strings.Join(drives, ", "))
	}
	if len(notes) > 0 {
		line += " (" + strings.Join(notes, "; ") + ")"
	}
	e.say("%s", line)
}

func (e *env) printSessions(heading string, list []sessionView) {
	if len(list) == 0 {
		return
	}
	e.say("")
	e.say("%s", heading)
	sort.Slice(list, func(a, b int) bool { return list[a].Session < list[b].Session })
	for _, v := range list {
		var names []string
		for _, p := range v.Members {
			names = append(names, p.Name)
		}
		e.say("  %s: %s", v.Session, strings.Join(names, " "))
	}
}

// unstartable names the teams and members whose names cannot be part of a
// session name (0.1.x allowed dots, as in ml.ops): cadrei up refuses them,
// so the orchestrator should not offer to start them.
func unstartable(c cadreis.Cadrei) []string {
	var out []string
	teams, _ := os.ReadDir(filepath.Join(c.Path, "members"))
	for _, t := range teams {
		if !t.IsDir() || strings.HasPrefix(t.Name(), ".") {
			continue
		}
		if !session.CheckName(t.Name()) {
			out = append(out, fmt.Sprintf("team %s cannot start: %s; rename its folder, members/%s", t.Name(), session.NameRule, t.Name()))
			continue
		}
		for _, role := range session.Roles(c.Path, t.Name()) {
			if !session.CheckName(role) {
				out = append(out, fmt.Sprintf("member %s of team %s cannot start: %s; rename its file, members/%s/%s.md", role, t.Name(), session.NameRule, t.Name(), role))
			}
		}
	}
	return out
}
