//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/registry"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// scope is the resolved cadre as the session code sees it.
func scope(r *cadres.Resolved) session.Scope {
	return session.Scope{Name: r.Name, Path: r.Path, Default: r.Name == r.Default, T: session.Default()}
}

// tmuxReady checks that tmux 3.2 or newer is there: cadre runs persona
// commands without a shell, with their environment given by new-session
// -e, which tmux 3.2 added.
func (e *env) tmuxReady() bool {
	major, minor, err := session.Default().Version()
	if err != nil {
		e.fail("%s (on macOS: brew install tmux)", err)
		return false
	}
	if major < 3 || (major == 3 && minor < 2) {
		e.fail("tmux %d.%d is too old; cadre needs tmux 3.2 or newer", major, minor)
		return false
	}
	return true
}

// splitTarget reads team or team/role, and checks the team exists.
func splitTarget(r *cadres.Resolved, target string) (team, role string, err error) {
	team, role, _ = strings.Cut(target, "/")
	if st, serr := os.Stat(filepath.Join(r.Path, "personas", team)); team == "" || serr != nil || !st.IsDir() {
		return "", "", fmt.Errorf("no team '%s' (see cadre help advanced: cadre team add)", team)
	}
	return team, role, nil
}

// mode is the persona permission mode: CADRE_PERMISSION_MODE, else
// cadre.conf's PERMISSION_MODE, else default.
func (e *env) mode(values map[string]string) string {
	if m := os.Getenv("CADRE_PERMISSION_MODE"); m != "" {
		return m
	}
	if m := values["PERMISSION_MODE"]; m != "" {
		return m
	}
	return "default"
}

// picker chooses each role's runtime (P.3) and prepares what its personas
// start with, once per runtime. A runtime that is not supported, cannot
// enforce the fixed denies, has no messaging or lacks the permission mode
// is refused for that role.
func (e *env) picker(r *cadres.Resolved, team, mode string, values map[string]string) func(string) (runtime.Runtime, string, error) {
	prepared := map[string]string{}
	return func(role string) (runtime.Runtime, string, error) {
		rt, err := runtime.Get(runtime.For(r.Path, team, role, values["RUNTIME"]))
		if err != nil {
			return nil, "", err
		}
		if err := runtime.Usable(rt, mode); err != nil {
			return nil, "", err
		}
		grants, done := prepared[rt.Name()]
		if !done {
			p := rt.Permissions().Prepare(r.Path, places(r))
			for _, n := range p.Notes {
				e.say("%s", n)
			}
			for _, w := range p.Warnings {
				fmt.Fprintln(e.stderr, w)
			}
			grants = p.Grants
			prepared[rt.Name()] = grants
		}
		return rt, grants, nil
	}
}

// locate turns a project name or a folder into the key's project part, the
// folder and the target recorded for restarts.
func locate(r *cadres.Resolved, arg string) (project, dir, target string, err error) {
	d, perr := projectDir(r, arg)
	switch {
	case perr == nil:
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			return "", "", "", fmt.Errorf("project '%s' is not at %s; clone it again with cadre project sync, or ask the orchestrator to relink or unlink it", arg, d)
		}
		return arg, d, arg, nil
	case perr != errNotRegistered:
		return "", "", "", perr
	}
	st, serr := os.Stat(arg)
	if serr != nil || !st.IsDir() {
		return "", "", "", fmt.Errorf("'%s' is neither a registry project nor a folder", arg)
	}
	abs, _ := filepath.Abs(arg)
	name := strings.Map(func(c rune) rune {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return c
		}
		return '-'
	}, filepath.Base(abs))
	return name, abs, abs, nil
}

func runUp(e *env) int {
	if len(e.args) < 1 || len(e.args) > 2 {
		return e.fail("usage: cadre up <team|team/role> [project|dir]")
	}
	r, ok := e.resolve()
	if !ok || !e.tmuxReady() {
		return 1
	}
	if err := cadres.CheckName(r.Name); err != nil {
		return e.fail("this cadre's folder name %s does not work in session names; rename the folder to use letters, digits, - and _", r.Name)
	}
	team, role, err := splitTarget(r, e.args[0])
	if err != nil {
		return e.fail("%s", err)
	}
	u := session.Up{Scope: scope(r), Team: team, Role: role, Dir: session.DefaultDir(r.Path, team)}
	if len(e.args) == 2 {
		if u.Project, u.Dir, u.Target, err = locate(r, e.args[1]); err != nil {
			return e.fail("%s", err)
		}
		u.Explicit = true
	}
	if err := u.CheckSession(team, u.Project); err != nil {
		return e.fail("%s", err)
	}
	values := e.conf(r)
	u.Mode = e.mode(values)
	u.Pick = e.picker(r, team, u.Mode, values)
	protocol, _ := cadre.Assets.ReadFile("protocol.md")
	u.Protocol = protocol
	if u.Start(e.stdout) {
		return 1
	}
	return 0
}

// confirm asks question, unless --yes was given. Without a terminal it
// says how to confirm and fails.
func (e *env) confirm(question string, yes bool) bool {
	if yes {
		return true
	}
	if !e.interactive() {
		fmt.Fprintln(e.stderr, "run with --yes to confirm")
		return false
	}
	switch strings.ToLower(e.ask(question + " [y/N] ")) {
	case "y", "yes":
		return true
	}
	e.say("nothing changed")
	return false
}

// showSessions prints sessions with their personas, under a heading per
// group when known is set (a listing of every cadre).
func (e *env) showSessions(t session.Tmux, list []session.Info, known map[string]string) {
	last := ""
	for _, i := range list {
		indent := "  "
		if known != nil {
			if g := session.Group(i, known); g != last {
				e.say("  %s:", g)
				last = g
			}
			indent = "    "
		}
		e.say("%s%s: %s", indent, i.Name, strings.Join(t.PersonaNames(i), " "))
	}
}

func runStop(e *env) int {
	var pos []string
	all, yes, withOrch := false, false, false
	for _, a := range e.args {
		switch a {
		case "--all":
			all = true
		case "--yes", "-y":
			yes = true
		case "--with-orchestrator":
			withOrch = true
		default:
			if strings.HasPrefix(a, "-") {
				return e.fail("usage: cadre stop [team[/role]] [project] [--all] [--yes]")
			}
			pos = append(pos, a)
		}
	}
	if len(pos) > 2 || (all && len(pos) > 0) {
		return e.fail("usage: cadre stop [team[/role]] [project] [--all] [--yes]")
	}
	if len(pos) > 0 {
		r, ok := e.resolve()
		if !ok {
			return 1
		}
		team, role, err := splitTarget(r, pos[0])
		if err != nil {
			return e.fail("%s", err)
		}
		project := ""
		if len(pos) == 2 {
			project = pos[1]
		}
		s := scope(r)
		if role == "" {
			e.say("%s", s.StopTeam(session.Key(team, project)))
		} else {
			e.say("%s", s.StopRole(session.Key(team, project), role))
		}
		return 0
	}
	if e.persona("stop the whole cadre") {
		return 1
	}
	t := session.Default()
	var list []session.Info
	var known map[string]string
	summary := "stopped every cadre session"
	if all {
		known = knownCadres()
		list = session.All(t, known)
		if withOrch {
			list = append(list, orchestrators(t, "")...)
		}
		if len(list) == 0 {
			e.say("no cadre sessions running")
			return 0
		}
		e.say("Running cadre sessions, by cadre:")
	} else {
		r, ok := e.resolve()
		if !ok {
			return 1
		}
		list = scope(r).Running()
		if withOrch {
			list = append(list, orchestrators(t, r.Path)...)
		}
		if len(list) == 0 {
			e.say("no sessions of cadre %s running", r.Name)
			return 0
		}
		e.say("Running sessions of cadre %s:", r.Name)
		summary = "stopped every session of cadre " + r.Name
	}
	e.showSessions(t, list, known)
	if !e.confirm("Stop these sessions?", yes) {
		return 1
	}
	var names []string
	for _, i := range list {
		names = append(names, i.Name)
	}
	st := &session.Stopping{T: t}
	st.Stop(e.stdout, names)
	if len(st.Failed) > 0 {
		fmt.Fprintf(e.stderr, "cadre: could not stop: %s\n", strings.Join(st.Failed, " "))
	} else {
		e.say("%s", summary)
	}
	st.Last(e.stdout)
	if len(st.Failed) > 0 {
		return 2
	}
	return 0
}

// orchestrators lists the orchestrator sessions in tmux, of the cadre at
// home, or of every cadre for "". They come after the personas in a stop
// list, so they stop last (K.4).
func orchestrators(t session.Tmux, home string) []session.Info {
	var out []session.Info
	for _, i := range t.Sessions() {
		if i.Role == "orchestrator" && (home == "" || i.Home == home) {
			out = append(out, i)
		}
	}
	return out
}

// knownCadres maps each known cadre's path to its name.
func knownCadres() map[string]string {
	known := map[string]string{}
	list, _ := cadres.List()
	for _, c := range list {
		known[c.Path] = c.Name
	}
	return known
}

func runAttach(e *env) int {
	if len(e.args) > 2 {
		return e.fail("usage: cadre attach [team] [project]")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	if len(e.args) == 0 {
		return e.attachOrchestrator(r)
	}
	project := ""
	if len(e.args) == 2 {
		project = e.args[1]
	}
	key := session.Key(e.args[0], project)
	s := scope(r)
	live := s.Live(key)
	if live == "" {
		return e.fail("%s is not running", session.SessionName(r.Name, key))
	}
	if !e.interactive() {
		return e.fail("cadre attach needs a terminal")
	}
	if err := s.T.Attach(live); err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// projectsOf lists a cadre's projects with whether each is on this machine.
func projectsOf(c cadres.Cadre) []projectView {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return nil
	}
	var out []projectView
	for _, entry := range reg.Entries() {
		d := cadres.ProjectDir(c, entry)
		st, err := os.Stat(d)
		out = append(out, projectView{Name: entry.Name, Team: entry.Get("team"), About: entry.Get("about"),
			Path: d, Cloned: d != "" && err == nil && st.IsDir()})
	}
	return out
}

// display writes a path with ~ for the home folder.
func display(p string) string { return cadres.Tilde(paths.Real(p)) }
