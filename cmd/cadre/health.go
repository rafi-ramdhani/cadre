//go:build !windows

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/backup"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/framework"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// A finding of the health check: a problem, and, when cadre can fix it, the
// question it asks first. Nothing is fixed without a yes.
type finding struct {
	runtime.Problem
	ask      string       // the question for the fix, or ""
	yes      bool         // the answer when the user just presses Enter
	fix      func() error // makes the fix
	declined func()       // remembers a no, when it should not be asked again
}

// ensureFramework writes the skill out to ~/.cadre/framework when this
// binary's version differs from the one written there (a build from source
// writes it every time, since its version does not change).
func (e *env) ensureFramework() {
	if version != "dev" && framework.Current(version) {
		return
	}
	if err := framework.Write(cadre.Assets, version); err != nil {
		fmt.Fprintf(e.stderr, "warning: could not write cadre's skill to %s: %s\n", framework.Dir(), err)
	}
}

// findings runs the health check: fast checks only look at files and PATH;
// full ones also run programs.
func findings(rt runtime.Runtime, full bool) []finding {
	var out []finding
	var dirs []string
	if list, err := cadres.List(); err == nil {
		for _, c := range list {
			dirs = append(dirs, c.Path)
		}
	}
	for _, p := range rt.Health(full, dirs) {
		out = append(out, finding{Problem: p})
	}
	if _, err := exec.LookPath("git"); err != nil {
		out = append(out, finding{Problem: runtime.Problem{What: "git is not installed", Fatal: true,
			Fix: "on macOS: xcode-select --install; on Linux: install git with your package manager"}})
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		out = append(out, finding{Problem: runtime.Problem{What: "tmux is not installed, so personas cannot start",
			Fix: "brew install tmux"}})
	} else if full {
		major, minor, err := session.Default().Version()
		if err == nil && (major < 3 || (major == 3 && minor < 2)) {
			out = append(out, finding{Problem: runtime.Problem{What: fmt.Sprintf("tmux %d.%d is too old for personas; cadre needs 3.2 or newer", major, minor),
				Fix: "brew upgrade tmux"}})
		}
	}
	out = append(out, guardFindings()...)
	out = append(out, skillFindings(rt)...)
	out = append(out, hookFindings(rt)...)
	if f, ok := pathFinding(); ok {
		out = append(out, f)
	}
	return out
}

// guardFindings keeps cadre's pre-push guard in every cadre repository,
// which is cadre's own setup, so it needs no question. A pre-push hook of
// the user's own is left alone and reported.
func guardFindings() []finding {
	var out []finding
	list, _ := cadres.List()
	for _, c := range list {
		if _, err := backup.Install(c.Path, framework.Binary()); errors.Is(err, backup.ErrForeign) {
			out = append(out, finding{Problem: runtime.Problem{
				What: "the cadre " + c.Name + " has a pre-push git hook of its own, so cadre's check for credentials before a backup push does not run",
				Fix:  "add this line to " + display(c.Path) + "/.git/hooks/pre-push: " + framework.Binary() + ` hook pre-push "$@" || exit 1`}})
		}
	}
	return out
}

// skillFindings checks the link that gives the runtime cadre's skill.
func skillFindings(rt runtime.Runtime) []finding {
	ops := rt.Instructions()
	want := framework.SkillDir()
	target, err := ops.Target()
	link := func() error { return ops.Link(want) }
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return []finding{{Problem: runtime.Problem{What: "the cadre skill is not linked into " + rt.Title() + " (" + display(ops.Path()) + ")",
			Fix: "run cadre in a terminal and answer yes"}, ask: "Link the cadre skill into " + rt.Title() + "?", yes: true, fix: link}}
	case errors.Is(err, runtime.ErrNotLink):
		return []finding{{Problem: runtime.Problem{What: display(ops.Path()) + " is not a link to cadre's skill",
			Fix: "move it aside, then run cadre again"}}}
	case err != nil:
		return []finding{{Problem: runtime.Problem{What: "cannot read " + display(ops.Path()) + ": " + err.Error(), Fix: "check the folder's permissions"}}}
	case paths.Real(target) != paths.Real(want):
		return []finding{{Problem: runtime.Problem{What: "the cadre skill links to " + display(target) + ", not to this cadre's (" + display(want) + ")",
			Fix: "run cadre in a terminal and answer yes"}, ask: "Link the skill to this cadre?", yes: true, fix: link}}
	}
	return nil
}

// hookFindings checks that an orchestrator hook, when there is one, runs
// this binary. A no is remembered for that pair of programs.
func hookFindings(rt runtime.Runtime) []finding {
	hooks := rt.Hooks()
	progs, err := hooks.Find()
	if err != nil {
		return []finding{{Problem: runtime.Problem{What: "cannot read the orchestrator hook in " + display(hooks.File()) + ": " + err.Error(),
			Fix: "fix the file, or remove cadre's hook from it"}}}
	}
	bin := framework.Binary()
	var out []finding
	for _, p := range progs {
		if paths.Real(p) == paths.Real(bin) {
			continue
		}
		pair := p + " -> " + bin
		if cadres.GetState(cadres.HookKept) == pair {
			continue
		}
		out = append(out, finding{Problem: runtime.Problem{What: "the orchestrator hook runs " + display(p) + ", not this cadre (" + display(bin) + ")",
			Fix: "run cadre in a terminal and answer yes"},
			ask:      "Point the hook at this cadre?",
			fix:      func() error { _, err := hooks.Set(bin); return err },
			declined: func() { cadres.SetState(cadres.HookKept, pair) }})
	}
	return out
}

// pathFinding checks that the cadre first on PATH is this one. A binary not
// named cadre (a test binary) is not the command users type.
func pathFinding() (finding, bool) {
	exe, err := os.Executable()
	if err != nil || filepath.Base(exe) != "cadre" {
		return finding{}, false
	}
	first, err := exec.LookPath("cadre")
	if err != nil || paths.Real(first) == paths.Real(exe) {
		return finding{}, false
	}
	return finding{Problem: runtime.Problem{What: "the cadre first on your PATH is " + display(first) + ", not this one (" + display(exe) + ")",
		Fix: "remove " + display(first) + ", or put " + display(filepath.Dir(exe)) + " before it in PATH"}}, true
}

// health runs the health check, silent unless something is wrong: fast
// checks every time, full ones when forced, after a version change, or
// when a fast check finds something. It offers each fix it can make, and
// reports how many problems it found and whether cadre must stop.
func (e *env) health(rt runtime.Runtime, force bool) (found int, fatal bool) {
	e.ensureFramework()
	full := force || cadres.GetState(cadres.CheckedVersion) != version
	list := findings(rt, full)
	if len(list) > 0 && !full {
		full = true
		list = findings(rt, true)
	}
	for _, f := range list {
		fmt.Fprintf(e.stderr, "problem: %s\n", f.What)
		if f.ask != "" && e.interactive() {
			if e.yes(f.ask, f.yes) {
				if err := f.fix(); err != nil {
					fmt.Fprintf(e.stderr, "  could not fix it: %s\n", err)
				} else {
					fmt.Fprintln(e.stderr, "  fixed")
				}
			} else if f.declined != nil {
				f.declined()
			}
			continue
		}
		fmt.Fprintf(e.stderr, "  fix: %s\n", f.Fix)
		fatal = fatal || f.Fatal
	}
	if full && !fatal {
		cadres.SetState(cadres.CheckedVersion, version)
	}
	return len(list), fatal
}

// yes asks a yes-or-no question; Enter gives def.
func (e *env) yes(question string, def bool) bool {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	switch strings.ToLower(e.ask(question + hint)) {
	case "":
		return def
	case "y", "yes":
		return true
	}
	return false
}

// missingProjects says, as information, which of the cadre's projects have
// no folder on this machine.
func (e *env) missingProjects(r *cadres.Resolved) {
	var names []string
	for _, p := range projectsOf(r.Cadre) {
		if !p.Cloned {
			names = append(names, p.Name)
		}
	}
	if len(names) > 0 {
		e.say("Not on this machine yet: %s (ask the orchestrator to clone or link them).", strings.Join(names, ", "))
	}
}

// runCheck is cadre --check: the full health check, with its fixes.
func runCheck(e *env) int {
	if len(e.args) > 0 {
		return e.fail("usage: cadre --check")
	}
	if !e.home() {
		return 1
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return e.fail("%s", err)
	}
	found, fatal := e.health(rt, true)
	if found == 0 {
		e.say("cadre %s: everything is in order", version)
	}
	if fatal {
		return 1
	}
	return 0
}
