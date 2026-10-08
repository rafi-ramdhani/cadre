//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/conf"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// The data of cadre ls, which --json prints as it is, so the orchestrator
// never reads the human layout (M.2). Its shape is versioned: a field is
// added under the same version, and anything else changes the version.
// A legacy session's personas have runtime "": they started before cadre
// recorded which runtime ran them.

// jsonVersion is the version of ls --json's shape.
const jsonVersion = 1

type cadreView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	From    string `json:"from,omitempty"`    // how this command found it
	Project string `json:"project,omitempty"` // the linked project it was found from
	Default bool   `json:"default"`
	Missing bool   `json:"missing,omitempty"`
}

type personaView struct {
	Name    string `json:"name"` // the session name, the persona's messaging address
	Role    string `json:"role"`
	Runtime string `json:"runtime"`
}

type sessionView struct {
	Session  string        `json:"session"` // the tmux session
	Team     string        `json:"team"`
	Project  string        `json:"project,omitempty"`
	Legacy   bool          `json:"legacy"` // from before session names carried the cadre
	Personas []personaView `json:"personas"`
}

type projectView struct {
	Name   string `json:"name"`
	Team   string `json:"team,omitempty"`
	About  string `json:"about,omitempty"`
	Path   string `json:"path"`
	Cloned bool   `json:"cloned"`
}

type otherView struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Running int    `json:"running"`
	Missing bool   `json:"missing,omitempty"`
}

type cadreStatus struct {
	Version  int                      `json:"version,omitempty"` // set on the top-level object
	Cadre    cadreView                `json:"cadre"`
	Running  []sessionView            `json:"running"`
	Projects []projectView            `json:"projects"`
	Teams    map[string][]personaView `json:"teams"` // every persona, running or not
	Others   []otherView              `json:"other_cadres,omitempty"`
	Problems []string                 `json:"problems"` // what keeps personas from starting
}

type allStatus struct {
	Version int           `json:"version"`
	Cadres  []cadreStatus `json:"cadres"`
	Legacy  []sessionView `json:"legacy"`
	Unknown []sessionView `json:"unknown"`
}

// runtimeOf names a persona's runtime (P.3).
func runtimeOf(cadre, team, role string) string {
	raw, _ := os.ReadFile(filepath.Join(cadre, "cadre.conf"))
	values, _ := conf.Parse(string(raw))
	return runtime.For(cadre, team, role, values["RUNTIME"])
}

func viewOf(t session.Tmux, i session.Info) sessionView {
	v := sessionView{Session: i.Name, Team: i.Team, Project: i.Project, Legacy: i.Home == "", Personas: []personaView{}}
	if v.Team == "" {
		// A 0.1.x session recorded no team: show its key.
		v.Team = strings.TrimPrefix(i.Name, "cadre-")
	}
	cadre := i.Home
	for _, w := range t.Windows(i.Name) {
		p := personaView{Name: strings.TrimPrefix(i.Name, "cadre-") + "-" + w, Role: w}
		if cadre != "" && i.Team != "" {
			p.Runtime = runtimeOf(cadre, i.Team, w)
		}
		v.Personas = append(v.Personas, p)
	}
	return v
}

// statusOf gathers one cadre's status.
func statusOf(c cadres.Cadre, def string, t session.Tmux) cadreStatus {
	s := cadreStatus{Cadre: cadreView{Name: c.Name, Path: c.Path, Default: c.Name == def, Missing: !c.Present()},
		Running: []sessionView{}, Projects: []projectView{}, Teams: map[string][]personaView{}, Problems: []string{}}
	scope := session.Scope{Name: c.Name, Path: c.Path, Default: c.Name == def, T: t}
	for _, i := range scope.Running() {
		s.Running = append(s.Running, viewOf(t, i))
	}
	if p := projectsOf(c); p != nil {
		s.Projects = p
	}
	teams, _ := filepath.Glob(filepath.Join(c.Path, "personas", "*"))
	for _, d := range teams {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			team := filepath.Base(d)
			s.Teams[team] = []personaView{}
			for _, role := range session.Roles(c.Path, team) {
				rt := runtimeOf(c.Path, team, role)
				s.Teams[team] = append(s.Teams[team], personaView{Name: session.PersonaName(c.Name, team, role), Role: role, Runtime: rt})
				if _, err := runtime.Get(rt); err != nil {
					s.Problems = append(s.Problems, fmt.Sprintf("%s/%s: %s", team, role, err))
				}
			}
		}
	}
	return s
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
			return e.fail("usage: cadre ls [--all] [--json]")
		}
	}
	if !e.home() {
		return 1
	}
	t := session.Default()
	def := cadres.Default()
	list, _ := cadres.List()
	if all {
		st := allStatus{Version: jsonVersion, Cadres: []cadreStatus{}, Legacy: []sessionView{}, Unknown: []sessionView{}}
		known := map[string]bool{}
		for _, c := range list {
			known[c.Path] = true
			if c.Present() {
				st.Cadres = append(st.Cadres, statusOf(c, def, t))
			}
		}
		for _, i := range session.All(t, nil) {
			switch {
			case i.Home == "":
				st.Legacy = append(st.Legacy, viewOf(t, i))
			case !known[i.Home]:
				st.Unknown = append(st.Unknown, viewOf(t, i))
			}
		}
		if asJSON {
			return e.printJSON(st)
		}
		for n, c := range st.Cadres {
			if n > 0 {
				e.say("")
			}
			e.printStatus(c)
		}
		e.printSessions("legacy sessions (from before cadre names; they belong to the default cadre)", st.Legacy)
		for _, u := range st.Unknown {
			e.printSessions("sessions of a cadre no longer known", []sessionView{u})
		}
		return 0
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	e.conf(r) // a status screen says when cadre.conf has lines it ignores
	st := statusOf(r.Cadre, r.Default, t)
	st.Version = jsonVersion
	st.Cadre.From, st.Cadre.Project = r.From, r.Project
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
		e.say("other cadres: %s", strings.Join(parts, ", "))
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

// printStatus is one cadre's block of the status screen.
func (e *env) printStatus(s cadreStatus) {
	where := display(s.Cadre.Path)
	switch {
	case s.Cadre.From == "default":
		where += ", the default cadre"
	case s.Cadre.From != "":
		where += ", " + strings.Replace(s.Cadre.From, "from the current folder", "from this folder", 1)
		if s.Cadre.Default {
			where += "; default"
		}
	case s.Cadre.Default:
		where += ", default"
	}
	e.say("cadre %s  (%s)", s.Cadre.Name, where)
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
			for _, p := range v.Personas {
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
	var names, missing []string
	for _, p := range s.Projects {
		names = append(names, p.Name)
		if !p.Cloned {
			missing = append(missing, p.Name)
		}
	}
	line := "projects: " + strings.Join(names, ", ")
	if len(missing) > 0 {
		line += " (? not cloned: " + strings.Join(missing, ", ") + ")"
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
		for _, p := range v.Personas {
			names = append(names, p.Name)
		}
		e.say("  %s: %s", v.Session, strings.Join(names, " "))
	}
}
