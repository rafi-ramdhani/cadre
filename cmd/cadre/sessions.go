//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/orchestrator"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/project"
	"github.com/rafi-ramdhani/cadre/internal/registry"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// scope is the resolved cadre as the session code sees it.
func scope(r *cadres.Resolved) session.Scope {
	return session.Scope{Name: r.Name, Path: r.Path, Default: r.Name == r.Default, T: session.Default()}
}

// tmuxReady checks that tmux 3.2 or newer is there: cadre runs member
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
	if st, serr := os.Stat(filepath.Join(r.Path, "members", team)); team == "" || serr != nil || !st.IsDir() {
		return "", "", fmt.Errorf("no team '%s' (see cadre help advanced: cadre team add)", team)
	}
	return team, role, nil
}

// mode is the permission mode sessions start with: CADRE_PERMISSION_MODE,
// else cadre.conf's PERMISSION_MODE, else default.
func (e *env) mode(values map[string]string) string { return modeOf(values) }

func modeOf(values map[string]string) string {
	if m := os.Getenv("CADRE_PERMISSION_MODE"); m != "" {
		return m
	}
	if m := values["PERMISSION_MODE"]; m != "" {
		return m
	}
	return "default"
}

// picker gives every role the cadre's runtime, refused when it cannot
// enforce the fixed denies, has no messaging or lacks the permission mode,
// and prepares what its members start with, once.
func (e *env) picker(r *cadres.Resolved, mode string) func(string) (runtime.Runtime, string, error) {
	var rt runtime.Runtime
	var grants string
	return func(role string) (runtime.Runtime, string, error) {
		if rt != nil {
			return rt, grants, nil
		}
		got, err := runtime.Get(runtimeName())
		if err != nil {
			return nil, "", err
		}
		if err := runtime.Usable(got, mode); err != nil {
			return nil, "", err
		}
		// Prepare writes the settings copy into the build folder, so the
		// folder is checked first.
		if err := session.EnsureBuild(got.BuildDir(r.Path)); err != nil {
			return nil, "", err
		}
		pl := places(r)
		// The session hook names this binary only when it is safely
		// placed; without it, resume uses the id from the launch.
		if e.hookPlaced(pl.Binary) != nil {
			pl.Binary = ""
		}
		p := got.Permissions().Prepare(r.Path, pl)
		for _, n := range p.Notes {
			e.say("%s", n)
		}
		for _, w := range p.Warnings {
			fmt.Fprintln(e.stderr, w)
		}
		rt, grants = got, p.Grants
		return rt, grants, nil
	}
}

// locate turns a project name or a folder into the key's project part, the
// folder and the target recorded for restarts.
func locate(r *cadres.Resolved, arg string) (proj, dir, target string, err error) {
	d, perr := projectDir(r, arg)
	switch {
	case perr == nil:
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
	fresh := contains(e.args, "--fresh")
	e.args = without(e.args, "--fresh")
	if len(e.args) < 1 || len(e.args) > 2 || strings.HasPrefix(e.args[0], "-") {
		return e.fail("usage: cadre up <team|team/role> [project|dir] [--fresh]")
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
	u.Pick = e.picker(r, u.Mode)
	u.Wait = upWait()
	u.Fresh = fresh
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

// showSessions prints sessions with their members, under a heading per
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
		e.say("%s%s: %s", indent, i.Name, strings.Join(t.MemberNames(i), " "))
	}
}

func runStop(e *env) int {
	var pos []string
	all, yes, fresh := false, false, false
	for _, a := range e.args {
		switch a {
		case "--all":
			all = true
		case "--yes", "-y":
			yes = true
		case "--fresh":
			fresh = true
		default:
			if strings.HasPrefix(a, "-") {
				return e.fail("usage: cadre stop [team[/role]] [project] [--all] [--yes] [--fresh]")
			}
			pos = append(pos, a)
		}
	}
	if len(pos) > 2 || (all && len(pos) > 0) {
		return e.fail("usage: cadre stop [team[/role]] [project] [--all] [--yes] [--fresh]")
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
		key := session.Key(team, project)
		roles := []string{role}
		if role == "" {
			e.say("%s", s.StopTeam(key))
			roles = session.Roles(r.Path, team)
		} else {
			e.say("%s", s.StopRole(key, role))
		}
		if fresh {
			var names []string
			for _, ro := range roles {
				names = append(names, session.MemberName(r.Name, key, ro))
			}
			e.forget(r.Cadre, names)
		}
		return 0
	}
	if e.member("stop the whole cadre") {
		return 1
	}
	t := session.Default()
	var list []session.Info
	var known map[string]string
	var targets []cadres.Cadre
	// done ends the stop; with --fresh, the stopped cadres' members start
	// new conversations next time.
	done := func(code int) int {
		if fresh && code == 0 {
			for _, c := range targets {
				e.forget(c, nil)
			}
		}
		return code
	}
	summary := "stopped every cadre session"
	if all {
		known = knownCadres()
		targets, _ = cadres.List()
		list = session.All(t, known)
		if len(list) == 0 {
			e.say("no cadre sessions running")
			return done(0)
		}
		e.say("Running cadre sessions, by cadre:")
	} else {
		r, ok := e.resolve()
		if !ok {
			return 1
		}
		targets = []cadres.Cadre{r.Cadre}
		list = scope(r).Running()
		if len(list) == 0 {
			e.say("no sessions of cadre %s running", r.Name)
			return done(0)
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
	if len(st.Failed) > 0 {
		st.Last(e.stdout)
		return 2
	}
	code := done(0)
	st.Last(e.stdout)
	return code
}

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
	proj := ""
	if len(e.args) == 2 {
		proj = e.args[1]
		// A registered project that is missing on this machine is refused,
		// with how to get it back.
		if _, err := projectDir(r, proj); err != nil && !errors.Is(err, errNotRegistered) {
			return e.fail("%s", err)
		}
	}
	key := session.Key(e.args[0], proj)
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
	finder := project.NewFinder(cadres.ProjectsDir())
	for _, entry := range reg.Entries() {
		d := cadres.ProjectDir(c, entry)
		v := projectView{Name: entry.Name, Repo: entry.Get("repo"), Team: entry.Get("team"), About: entry.Get("about"),
			Path: d, State: cadres.Where(d)}
		v.Cloned = v.State == "present"
		if !v.Cloned {
			v.Found = finder.Find(v.Repo)
		}
		out = append(out, v)
	}
	return out
}

// display writes a path with ~ for the home folder.
func display(p string) string { return cadres.Tilde(paths.Real(p)) }

// forget drops the conversation records of members (names, or every
// member's for nil) so their next start is a new conversation; the
// orchestrator's record stays.
func (e *env) forget(c cadres.Cadre, names []string) {
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return
	}
	dir := filepath.Join(rt.BuildDir(c.Path), "sessions")
	if names == nil {
		files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		for _, f := range files {
			if n := strings.TrimSuffix(filepath.Base(f), ".json"); n != orchestrator.Name(c.Name) {
				names = append(names, n)
			}
		}
	}
	for _, n := range names {
		if p := session.RecordPath(rt.BuildDir(c.Path), n); p != "" {
			session.Forget(p)
		}
	}
	e.say("  their next start begins a new conversation")
}
