//go:build !windows

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rafi-ramdhani/cadrei/internal/backup"
	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/framework"
	"github.com/rafi-ramdhani/cadrei/internal/orchestrator"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	"github.com/rafi-ramdhani/cadrei/internal/session"
)

// runUninstall is cadrei uninstall: it removes what this cadrei made outside
// the user's cadreis and projects, after showing the plan and getting a
// yes. It stops cadrei's sessions; removes the skill link and the
// orchestrator hook only where they point at this cadrei, each cadrei's
// pre-push guard (which would stop every push once cadrei is gone),
// ~/.cadrei/config and ~/.cadrei/framework; and keeps every cadrei and
// project. The program itself is removed last, by the user.
func runUninstall(e *env) int {
	yes, dry := false, false
	for _, a := range e.args {
		switch a {
		case "--yes", "-y":
			yes = true
		case "--dry-run":
			dry = true
		default:
			return e.fail("usage: cadrei uninstall [--yes | --dry-run]")
		}
	}
	if e.member("uninstall cadrei") {
		return 1
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return e.fail("%s", err)
	}
	t := session.Default()
	known := knownCadreis()
	list, _ := cadreis.List()

	// The plan: only what this cadrei made, and where it still points here.
	// Sessions started by 0.1.x are not this cadrei's to stop: on a machine
	// that runs 0.1.x too, they are the user's live sessions.
	// Nor are sessions without cadrei's markers, which cadrei did not make
	// in full.
	var members, orchestrators, legacy, unmarked []string
	for _, i := range session.All(t, known) {
		if i.Legacy() {
			legacy = append(legacy, i.Name)
			continue
		}
		if i.Home == "" {
			unmarked = append(unmarked, i.Name)
			continue
		}
		members = append(members, i.Name)
	}
	for _, i := range t.Sessions() {
		if i.Role == "orchestrator" {
			orchestrators = append(orchestrators, i.Name)
		}
	}
	var terminals []string
	for _, c := range list {
		if l := orchestrator.ReadLock(orchestrator.LockPath(rt.BuildDir(c.Path))); l != nil && l.Mode == orchestrator.Terminal {
			terminals = append(terminals, c.Name)
		}
	}
	skills := rt.Instructions()
	skillOurs := false
	if target, err := skills.Target(); err == nil && paths.Real(target) == paths.Real(framework.SkillDir()) {
		skillOurs = true
	}
	bin := framework.Binary()
	hookOurs, hookOthers := false, false
	if progs, err := rt.Hooks().Find(); err == nil {
		for _, p := range progs {
			if paths.Real(p) == paths.Real(bin) {
				hookOurs = true
			} else {
				hookOthers = true
			}
		}
	}
	var guarded []cadreis.Cadrei
	for _, c := range list {
		if backup.Installed(c.Path) {
			guarded = append(guarded, c)
		}
	}
	var folders []string
	for _, d := range []string{cadreis.ConfigDir(), framework.Dir()} {
		if _, err := os.Lstat(d); err == nil {
			folders = append(folders, d)
		}
	}

	e.say("cadrei uninstall will:")
	steps := 0
	step := func(format string, a ...any) {
		steps++
		e.say("  "+format, a...)
	}
	if len(members) > 0 {
		step("stop %d member sessions: %s", len(members), joinNames(members))
	}
	if len(orchestrators) > 0 {
		step("stop the orchestrator sessions in tmux, last: %s", joinNames(orchestrators))
	}
	if skillOurs {
		step("remove the skill link %s", cadreis.Tilde(skills.Path()))
	}
	if hookOurs {
		step("remove the orchestrator hook from %s (other hooks stay)", cadreis.Tilde(rt.Hooks().File()))
	}
	for _, c := range guarded {
		step("remove cadrei's pre-push check from %s", display(c.Path))
	}
	for _, d := range folders {
		// A link is removed as a link; the folder it points to stays.
		if st, err := os.Lstat(d); err == nil && st.Mode()&os.ModeSymlink != 0 {
			step("remove the link %s (its folder %s is kept)", cadreis.Tilde(d), display(d))
			continue
		}
		step("remove %s", display(d))
	}
	if steps == 0 {
		e.say("  nothing: cadrei has nothing set up here")
	}
	e.say("It keeps:")
	if len(list) > 0 {
		var names []string
		for _, c := range list {
			names = append(names, c.Name)
		}
		e.say("  every cadrei (%s, in %s) and every project", joinNames(names), display(cadreis.Root()))
	} else {
		e.say("  every project")
	}
	if _, err := skills.Target(); err == nil && !skillOurs {
		e.say("  the skill link %s, which points at another cadrei program", cadreis.Tilde(skills.Path()))
	}
	if hookOthers {
		e.say("  orchestrator hooks that run another cadrei program")
	}
	if len(legacy) > 0 {
		e.say("  sessions started by cadre 0.1.x, left running: %s", joinNames(legacy))
	}
	if len(unmarked) > 0 {
		e.say("  sessions without cadrei's markers, left running: %s", joinNames(unmarked))
	}
	for _, name := range terminals {
		e.say("  the orchestrator of %s open in a terminal: close it yourself", name)
	}
	finish := finalStep(bin)
	if dry {
		e.say("Then: %s", finish)
		return 0
	}
	if steps == 0 {
		e.say("Then: %s", finish)
		return 0
	}
	if !e.confirm("Uninstall cadrei?", yes) {
		return 1
	}

	code := 0
	warn := func(what string, err error) {
		fmt.Fprintf(e.stderr, "cadrei: could not %s: %s\n", what, err)
		code = 2
	}
	st := &session.Stopping{T: t}
	st.Stop(e.stdout, members)
	st.Stop(e.stdout, orchestrators)
	if len(st.Failed) > 0 {
		warn("stop "+joinNames(st.Failed), fmt.Errorf("see tmux"))
	}
	if skillOurs {
		if _, err := skills.Unlink(framework.SkillDir()); err != nil {
			warn("remove the skill link", err)
		} else {
			e.say("  removed the skill link")
		}
	}
	if hookOurs {
		if _, err := rt.Hooks().Remove(bin); err != nil {
			warn("remove the orchestrator hook", err)
		} else {
			e.say("  removed the orchestrator hook")
		}
	}
	for _, c := range guarded {
		if _, err := backup.Remove(c.Path); err != nil {
			warn("remove the pre-push check from "+c.Name, err)
		}
	}
	for _, d := range folders {
		what := display(d)
		if st, err := os.Lstat(d); err == nil && st.Mode()&os.ModeSymlink != 0 {
			what = "the link " + cadreis.Tilde(d)
		}
		if err := os.RemoveAll(d); err != nil {
			warn("remove "+what, err)
		} else {
			e.say("  removed %s", what)
		}
	}
	e.say("Uninstalled. Your cadreis and projects are kept. To finish: %s", finish)
	st.Last(e.stdout)
	return code
}

// finalStep is how the user removes the program itself, by install kind.
func finalStep(bin string) string {
	switch framework.Kind(version) {
	case "homebrew":
		return "brew uninstall cadrei"
	case "dev":
		return "delete the cadrei you built (" + display(bin) + ")"
	}
	return "rm " + display(bin)
}

func joinNames(names []string) string { return strings.Join(names, ", ") }
