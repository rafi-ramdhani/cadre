//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/project"
	"github.com/rafi-ramdhani/cadre/internal/registry"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// projectsDir returns where new clones go. It is asked once and remembered
// (N.2): on a terminal cadre asks; without one it refuses, so the
// orchestrator asks the user in the chat instead.
func (e *env) projectsDir(protected []string) (string, bool) {
	if d := cadres.ProjectsDir(); d != "" {
		return d, true
	}
	suggest := cadres.Tilde(cadres.SuggestProjectsDir())
	if !e.interactive() {
		e.fail("the projects folder is not set; ask the user and run cadre project dir <folder> (suggested: %s)", suggest)
		return "", false
	}
	answer := e.ask(fmt.Sprintf("Where do you keep your projects? [%s] ", suggest))
	if answer == "" {
		answer = suggest
	}
	dir, ok := e.setProjectsDir(answer, protected)
	return dir, ok
}

func (e *env) setProjectsDir(answer string, protected []string) (string, bool) {
	dir := answer
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		dir = paths.Home() + dir[1:]
	}
	if !filepath.IsAbs(dir) {
		dir, _ = filepath.Abs(dir)
	}
	if why := project.DestRefusal(paths.Real(dir), protected); why != "" {
		e.fail("%s cannot hold projects: %s", dir, why)
		return "", false
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		if !paths.Within(paths.Real(dir), paths.Home()) {
			e.fail("%s does not exist; create it first, or choose a folder under your home folder", dir)
			return "", false
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			e.fail("%s", err)
			return "", false
		}
	}
	if err := cadres.SetProjectsDir(dir); err != nil {
		e.fail("%s", err)
		return "", false
	}
	e.say("new clones go to %s (change it with cadre project dir <folder>)", cadres.Tilde(paths.Real(dir)))
	return paths.Real(dir), true
}

func runProjectDir(e *env) int {
	switch len(e.args) {
	case 0:
		if d := cadres.ProjectsDir(); d != "" {
			e.say("%s", cadres.Tilde(d))
		} else {
			e.say("not set (suggested: %s); set it with cadre project dir <folder>", cadres.Tilde(cadres.SuggestProjectsDir()))
		}
		return 0
	case 1:
		if e.persona("change where projects are cloned") {
			return 1
		}
		// Every runtime's protected folders; in 0.2.0, Claude Code's.
		rt, err := runtime.Get(runtime.Default())
		if err != nil {
			return e.fail("%s", err)
		}
		if _, ok := e.setProjectsDir(e.args[0], rt.Trust().Protected()); !ok {
			return 1
		}
		return 0
	}
	return e.fail("usage: cadre project dir [<folder>]")
}

const addUsage = "usage: cadre project add <name> <owner/repo> [team] [about] [--no-trust] | cadre project add <name> --path <dir> [--repo <repo>] [team] [about] [--no-trust]"

func runProjectAdd(e *env) int {
	var pos []string
	var path, repo string
	trust := true
	for i := 0; i < len(e.args); i++ {
		switch a := e.args[i]; {
		case a == "--no-trust":
			trust = false
		case (a == "--path" || a == "--repo") && i+1 < len(e.args):
			if a == "--path" {
				path = e.args[i+1]
			} else {
				repo = e.args[i+1]
			}
			i++
		case strings.HasPrefix(a, "-"):
			return e.fail("unknown option %s (%s)", a, addUsage)
		default:
			pos = append(pos, a)
		}
	}
	linking := path != ""
	s := project.Spec{Team: "dev"}
	switch {
	case linking && len(pos) >= 1 && len(pos) <= 3:
		s.Name, s.Repo = pos[0], repo
		pos = pos[1:]
	case !linking && repo == "" && len(pos) >= 2 && len(pos) <= 4:
		s.Name, s.Repo = pos[0], pos[1]
		pos = pos[2:]
	default:
		return e.fail("%s", addUsage)
	}
	if len(pos) > 0 {
		s.Team = pos[0]
	}
	if len(pos) > 1 {
		s.About = pos[1]
	}
	// Only the user, through the orchestrator, adds projects: a project's
	// folder is trusted and personas work in it.
	if e.persona("add projects") {
		return 1
	}
	if err := project.CheckName(s.Name); err != nil {
		return e.fail("%s", err)
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	protected := rt.Trust().Protected()
	var added project.Added
	var err error
	if linking {
		added, err = project.Link(r.Cadre, s, path, protected)
	} else {
		dir, ok := e.projectsDir(protected)
		if !ok {
			return 1
		}
		added, err = project.Add(r.Cadre, s, dir, protected)
	}
	if err != nil {
		return e.fail("%s", err)
	}
	for _, n := range added.Notes {
		e.say("%s", n)
	}
	head := fmt.Sprintf("  %s added, %s", s.Name, added.Where)
	switch {
	case !trust:
		e.say("%s", head)
	default:
		results, note := project.Trust(rt, []runtime.Folder{{Name: s.Name, Dir: added.Dir}})
		res := results[0]
		switch res.State {
		case "trusted":
			e.say("%s and trusted in %s (registered projects are trusted; use --no-trust to skip)", head, rt.Title())
		case "already":
			e.say("%s; its folder was already trusted in %s", head, rt.Title())
		case "refused":
			e.say("%s; not trusted in %s, %s", head, rt.Title(), res.Reason)
		default:
			e.say("%s", head)
			e.say("%s", strings.Replace(note, "the folder", added.Dir, 1))
		}
	}
	return 0
}

func runProjectSync(e *env) int {
	trust := true
	switch {
	case len(e.args) == 1 && e.args[0] == "--no-trust":
		trust = false
	case len(e.args) != 0:
		return e.fail("usage: cadre project sync [--no-trust]")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	dir := cadres.ProjectsDir()
	if dir == "" && needsProjectsDir(r) {
		if dir, ok = e.projectsDir(rt.Trust().Protected()); !ok {
			return 1
		}
	}
	results, err := project.Sync(r.Cadre, dir, rt.Trust().Protected())
	if err != nil {
		return e.fail("%s", err)
	}
	var cloned []runtime.Folder
	code := 0
	for _, res := range results {
		switch res.State {
		case "present":
			e.say("  %s: present", res.Name)
		case "cloned":
			e.say("  %s: cloned to %s", res.Name, res.Dir)
			cloned = append(cloned, runtime.Folder{Name: res.Name, Dir: res.Dir})
		case "found":
			e.say("  %s: already at %s", res.Name, res.Dir)
			cloned = append(cloned, runtime.Folder{Name: res.Name, Dir: res.Dir})
		case "drive":
			e.say("  %s: on a drive that is not connected (%s); connect the drive", res.Name, display(res.Dir))
		case "no repo":
			e.say("  %s: not on this machine, and no repo to clone; link its folder with cadre project link %s <dir>", res.Name, res.Name)
		case "no folder":
			e.say("  %s: no folder (set the projects folder with cadre project dir <folder>)", res.Name)
		default:
			e.say("  %s: not cloned: %s", res.Name, res.Err)
			code = 1
		}
	}
	if trust && len(cloned) > 0 {
		e.trustReport(rt, cloned)
	}
	return code
}

// needsProjectsDir reports whether some project with a repo has no place
// on this machine, so sync would clone it into the projects folder.
func needsProjectsDir(r *cadres.Resolved) bool {
	reg, err := registry.Load(r.Registry())
	if err != nil {
		return false
	}
	for _, entry := range reg.Entries() {
		if entry.Get("repo") != "" && cadres.ProjectDir(r.Cadre, entry) == "" {
			return true
		}
	}
	return false
}

// runProjectLink is cadre project link <name> <dir>: where a project's
// folder is on this machine, for a folder that moved or a clone made
// elsewhere. The folder is trusted, as with project add.
func runProjectLink(e *env) int {
	trust := true
	var pos []string
	for _, a := range e.args {
		switch {
		case a == "--no-trust":
			trust = false
		case strings.HasPrefix(a, "-"):
			return e.fail("usage: cadre project link <name> <dir> [--no-trust]")
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 2 {
		return e.fail("usage: cadre project link <name> <dir> [--no-trust]")
	}
	if e.persona("link folders to the cadre") {
		return 1
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	dir, err := project.Relink(r.Cadre, pos[0], pos[1], rt.Trust().Protected())
	if err != nil {
		return e.fail("%s", err)
	}
	e.say("  %s is at %s on this machine", pos[0], dir)
	if trust {
		e.trustReport(rt, []runtime.Folder{{Name: pos[0], Dir: dir}})
	}
	return 0
}

// runProjectUnlink is cadre project unlink <name> [--untrust]: the project
// leaves the registry and this machine's places; its folder is kept.
func runProjectUnlink(e *env) int {
	untrust := false
	var pos []string
	for _, a := range e.args {
		switch {
		case a == "--untrust":
			untrust = true
		case strings.HasPrefix(a, "-"):
			return e.fail("usage: cadre project unlink <name> [--untrust]")
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) != 1 {
		return e.fail("usage: cadre project unlink <name> [--untrust]")
	}
	if e.persona("unlink projects") {
		return 1
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	dir, notes, err := project.Unlink(r.Cadre, pos[0])
	if err != nil {
		return e.fail("%s", err)
	}
	for _, n := range notes {
		e.say("%s", n)
	}
	if dir != "" {
		e.say("  %s unlinked; its folder %s is kept", pos[0], display(dir))
	} else {
		e.say("  %s unlinked", pos[0])
	}
	if untrust && dir != "" {
		results, note := rt.Trust().Unmark([]runtime.Folder{{Name: pos[0], Dir: dir}})
		for _, res := range results {
			e.say("  %s: %s in %s", res.Name, res.State, rt.Title())
		}
		if note != "" {
			e.say("%s", note)
		}
	}
	return 0
}

// trustReport trusts folders and prints a line each, then any note.
func (e *env) trustReport(rt runtime.Runtime, folders []runtime.Folder) {
	if os.Getenv("CADRE_PERSONA") != "" {
		var names []string
		for _, f := range folders {
			e.say("  %s: not trusted (see below)", f.Name)
			names = append(names, f.Name)
		}
		e.say("persona sessions cannot trust folders in %s; run cadre project trust %s from the orchestrator", rt.Title(), strings.Join(names, " "))
		return
	}
	results, note := project.Trust(rt, folders)
	for _, f := range folders {
		for _, res := range results {
			if res.Name == f.Name {
				e.say("%s", project.Line(res, rt.Title()))
			}
		}
	}
	if note != "" {
		e.say("%s", note)
	}
}

func runProjectTrust(e *env) int {
	if e.persona("trust project folders") {
		return 1
	}
	if len(e.args) != 1 || (strings.HasPrefix(e.args[0], "-") && e.args[0] != "--all") {
		return e.fail("usage: cadre project trust <name> | --all")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	reg, err := registry.Load(r.Registry())
	if err != nil {
		return e.fail("%s", err)
	}
	var folders []runtime.Folder
	if e.args[0] == "--all" {
		for _, entry := range reg.Entries() {
			d := cadres.ProjectDir(r.Cadre, entry)
			if st, err := os.Stat(d); d == "" || err != nil || !st.IsDir() {
				e.say("  %s: missing locally, skipped (run cadre project sync)", entry.Name)
				continue
			}
			folders = append(folders, runtime.Folder{Name: entry.Name, Dir: d})
		}
	} else {
		entry := reg.Get(e.args[0])
		if entry == nil {
			return e.fail("'%s' is not a registered project (only registry projects are trusted; see cadre ls)", e.args[0])
		}
		d := cadres.ProjectDir(r.Cadre, entry)
		if st, err := os.Stat(d); d == "" || err != nil || !st.IsDir() {
			return e.fail("project '%s' is not at %s (run cadre project sync)", e.args[0], d)
		}
		folders = []runtime.Folder{{Name: entry.Name, Dir: d}}
	}
	if len(folders) > 0 {
		e.trustReport(rt, folders)
	}
	return 0
}
