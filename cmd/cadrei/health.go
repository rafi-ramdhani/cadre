//go:build !windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	cadrei "github.com/rafi-ramdhani/cadrei"
	"github.com/rafi-ramdhani/cadrei/internal/backup"
	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/framework"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	"github.com/rafi-ramdhani/cadrei/internal/session"
)

// A finding of the health check: a problem, and, when cadrei can fix it, the
// question it asks first. Nothing is fixed without a yes.
type finding struct {
	runtime.Problem
	ask      string       // the question for the fix, or ""
	yes      bool         // the answer when the user just presses Enter
	fix      func() error // makes the fix
	declined func()       // remembers a no, when it should not be asked again
}

// ensureFramework keeps ~/.cadrei/framework exactly as this binary
// carries it, on every run: the skill holds the orchestrator's consent
// rules, and a member's shell could change it. A change made outside cadrei
// is undone and reported.
func (e *env) ensureFramework() []finding {
	restored, err := framework.Sync(cadrei.Assets, version)
	if err != nil {
		return []finding{{Problem: runtime.Problem{What: "could not write cadrei's skill to " + display(framework.Dir()) + ": " + err.Error(),
			Fix: "check that folder's permissions"}}}
	}
	if len(restored) > 0 {
		return []finding{{Problem: runtime.Problem{What: "the cadrei skill was changed outside cadrei (" + strings.Join(restored, ", ") + "); cadrei restored it",
			Fix: "nothing to do; if a member's session made the change, look at what it was doing"}}}
	}
	return nil
}

// findings runs the health check: fast checks only look at files and PATH;
// full ones also run programs.
func (e *env) findings(rt runtime.Runtime, full bool) []finding {
	var out []finding
	var dirs []string
	if list, err := cadreis.List(); err == nil {
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
		out = append(out, finding{Problem: runtime.Problem{What: "tmux is not installed, so members cannot start",
			Fix: "brew install tmux"}})
	} else if full {
		major, minor, err := session.Default().Version()
		if err == nil && (major < 3 || (major == 3 && minor < 2)) {
			out = append(out, finding{Problem: runtime.Problem{What: fmt.Sprintf("tmux %d.%d is too old for members; cadrei needs 3.2 or newer", major, minor),
				Fix: "brew upgrade tmux"}})
		}
	}
	if err := e.hookPlaced(framework.Binary()); err != nil {
		out = append(out, finding{Problem: runtime.Problem{
			What: "members' conversations are resumed from their start only, not after /clear or /compact: the session hook needs cadrei in a safe place, and " + err.Error(),
			Fix:  "install cadrei with Homebrew or install.sh and run it from there"}})
	}
	out = append(out, e.guardFindings()...)
	out = append(out, skillFindings(rt)...)
	out = append(out, oldLinkFindings(rt)...)
	out = append(out, e.hookFindings(rt)...)
	if f, ok := pathFinding(); ok {
		out = append(out, f)
	}
	return out
}

// guardFindings keeps cadrei's pre-push guard in every cadrei repository,
// which is cadrei's own setup, so it needs no question. A pre-push hook of
// the user's own is left alone and reported.
// It names this binary only when it is safely placed, as the orchestrator
// hook does; otherwise a hook already there is left as it is, and a cadrei
// with none is reported.
func (e *env) guardFindings() []finding {
	var out []finding
	list, _ := cadreis.List()
	placed := e.hookPlaced(framework.Binary())
	for _, c := range list {
		if placed != nil {
			if !backup.Installed(c.Path) {
				out = append(out, finding{Problem: runtime.Problem{
					What: "the cadrei " + c.Name + " has no check for credentials before a backup push: " + placed.Error(),
					Fix:  "install cadrei with Homebrew or install.sh and run it from there"}})
			}
			continue
		}
		if _, err := backup.Install(c.Path, framework.Binary()); errors.Is(err, backup.ErrForeign) {
			out = append(out, finding{Problem: runtime.Problem{
				What: "the cadrei " + c.Name + " has a pre-push git hook of its own, so cadrei's check for credentials before a backup push does not run",
				Fix:  "add this line to " + display(c.Path) + "/.git/hooks/pre-push: " + framework.Binary() + ` hook pre-push "$@" || exit 1`}})
		}
	}
	return out
}

// skillFindings checks the link that gives the runtime cadrei's skill.
func skillFindings(rt runtime.Runtime) []finding {
	ops := rt.Instructions()
	want := framework.SkillDir()
	target, err := ops.Target()
	link := func() error { return ops.Link(want) }
	kept := func(state string) func() { return func() { cadreis.SetState(cadreis.SkillKept, state) } }
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if cadreis.GetState(cadreis.SkillKept) == "missing" {
			return nil
		}
		return []finding{{Problem: runtime.Problem{What: "the cadrei skill is not linked into " + rt.Title() + " (" + display(ops.Path()) + ")",
			Fix: "run cadrei in a terminal and answer yes"}, ask: "Link the cadrei skill into " + rt.Title() + "?", yes: true, fix: link, declined: kept("missing")}}
	case errors.Is(err, runtime.ErrNotLink):
		return []finding{{Problem: runtime.Problem{What: display(ops.Path()) + " is not a link to cadrei's skill",
			Fix: "move it aside, then run cadrei again"}}}
	case err != nil:
		return []finding{{Problem: runtime.Problem{What: "cannot read " + display(ops.Path()) + ": " + err.Error(), Fix: "check the folder's permissions"}}}
	case paths.Real(target) != paths.Real(want):
		if cadreis.GetState(cadreis.SkillKept) == target {
			return nil
		}
		return []finding{{Problem: runtime.Problem{What: "the cadrei skill links to " + display(target) + ", not to this cadrei program's (" + display(want) + ")",
			Fix: "run cadrei in a terminal and answer yes"}, ask: "Link the skill to this cadrei program?", yes: true, fix: link, declined: kept(target)}}
	}
	return nil
}

// oldLinkFindings offers to remove the links a 0.1.x install made into its
// clone: the cadre command in ~/.local/bin and the cadre skill beside
// cadrei's. After a pull they lead only to the bridge files. The question
// defaults to no, since a link is something the user may have made, and a
// no is remembered for that link.
func oldLinkFindings(rt runtime.Runtime) []finding {
	var out []finding
	for _, l := range []struct{ link, inClone string }{
		{filepath.Join(paths.Home(), ".local", "bin", "cadre"), "bin/cadre"},
		{filepath.Join(filepath.Dir(rt.Instructions().Path()), "cadre"), "skills/cadre"},
	} {
		clone := oldClone(rt, l.link, l.inClone)
		if clone == "" || cadreis.GetState(cadreis.OldLinkKept(l.link)) != "" {
			continue
		}
		link := l.link
		out = append(out, finding{Problem: runtime.Problem{What: cadreis.Tilde(link) + " is a cadre 0.1.x link into its clone at " + display(clone),
			Fix: "remove the link (the clone stays): rm " + cadreis.Tilde(link)},
			ask:      "Remove the link " + cadreis.Tilde(link) + "? The clone stays.",
			fix:      func() error { return os.Remove(link) },
			declined: func() { cadreis.SetState(cadreis.OldLinkKept(link), "yes") }})
	}
	return out
}

// oldClone returns the 0.1.x clone link points into, as <clone>/inClone,
// or "" when link is not such a link (see runtime.InstructionOps.OldClone).
func oldClone(rt runtime.Runtime, link, inClone string) string {
	if st, err := os.Lstat(link); err != nil || st.Mode()&fs.ModeSymlink == 0 {
		return ""
	}
	to, err := os.Readlink(link)
	if err != nil {
		return ""
	}
	if !filepath.IsAbs(to) {
		to = filepath.Join(filepath.Dir(link), to)
	}
	clone, ok := strings.CutSuffix(filepath.Clean(to), "/"+inClone)
	if !ok || !rt.Instructions().OldClone(clone) {
		return ""
	}
	return clone
}

// hookPlaced says why the hook must not name bin, or nil (see
// framework.Placed).
func (e *env) hookPlaced(bin string) error {
	if hookAnywhere() {
		return nil
	}
	return framework.Placed(bin)
}

// hookFindings checks that an orchestrator hook, when there is one, runs
// this binary. A no is remembered for that pair of programs. The hook is
// pointed here only when this binary is safely placed.
func (e *env) hookFindings(rt runtime.Runtime) []finding {
	hooks := rt.Hooks()
	progs, err := hooks.Find()
	if err != nil {
		return []finding{{Problem: runtime.Problem{What: "cannot read the orchestrator hook in " + display(hooks.File()) + ": " + err.Error(),
			Fix: "fix the file, or remove cadrei's hook from it"}}}
	}
	bin := framework.Binary()
	placed := e.hookPlaced(bin)
	var out []finding
	for _, p := range progs {
		if paths.Real(p) == paths.Real(bin) {
			continue
		}
		pair := p + " -> " + bin
		if cadreis.GetState(cadreis.HookKept) == pair {
			continue
		}
		f := finding{Problem: runtime.Problem{What: "the orchestrator hook runs " + display(p) + ", not this cadrei program (" + cadreis.Tilde(bin) + ")",
			Fix: "run cadrei in a terminal and answer yes"}}
		if placed != nil {
			f.Fix = "install cadrei (with Homebrew or install.sh) and run it from there; " + placed.Error()
		} else {
			f.ask = "Point the hook at this cadrei program?"
			f.fix = func() error { _, err := hooks.Set(bin); return err }
			f.declined = func() { cadreis.SetState(cadreis.HookKept, pair) }
		}
		out = append(out, f)
	}
	return out
}

// pathFinding checks that the cadrei first on PATH is this one. A binary not
// named cadrei (a test binary) is not the command users type.
func pathFinding() (finding, bool) {
	exe, err := os.Executable()
	if err != nil || filepath.Base(exe) != "cadrei" {
		return finding{}, false
	}
	first, err := exec.LookPath("cadrei")
	if err != nil || paths.Real(first) == paths.Real(exe) {
		return finding{}, false
	}
	return finding{Problem: runtime.Problem{What: "the cadrei first on your PATH is " + display(first) + ", not this one (" + display(exe) + ")",
		Fix: "remove " + display(first) + ", or put " + display(filepath.Dir(exe)) + " before it in PATH"}}, true
}

// health runs the health check, silent unless something is wrong: fast
// checks every time, full ones when forced, after a version change, or
// when a fast check finds something new (a lasting finding does not slow
// every start). It offers each fix it can make, and reports how many
// problems it found and whether cadrei must stop.
func (e *env) health(rt runtime.Runtime, force bool) (found int, fatal bool) {
	list := e.ensureFramework()
	fast := e.findings(rt, false)
	key := findingsKey(fast)
	full := force || cadreis.GetState(cadreis.CheckedVersion) != version || (len(fast) > 0 && cadreis.GetState(cadreis.LastFindings) != key)
	if full {
		list = append(list, e.findings(rt, true)...)
	} else {
		list = append(list, fast...)
	}
	for _, f := range list {
		fmt.Fprintf(e.stderr, "problem: %s\n", f.What)
		// Once the input has ended, nothing more is asked, and the end is
		// not remembered as a no: the fix is printed instead.
		if f.ask != "" && e.interactive() && !e.eof {
			if e.yes(f.ask, f.yes) {
				if err := f.fix(); err != nil {
					fmt.Fprintf(e.stderr, "  could not fix it: %s\n", err)
				} else {
					fmt.Fprintln(e.stderr, "  fixed")
				}
				continue
			}
			if !e.eof {
				if f.declined != nil {
					f.declined()
				}
				continue
			}
		}
		fmt.Fprintf(e.stderr, "  fix: %s\n", f.Fix)
		fatal = fatal || f.Fatal
	}
	cadreis.SetState(cadreis.LastFindings, key)
	if full && !fatal {
		cadreis.SetState(cadreis.CheckedVersion, version)
	}
	return len(list), fatal
}

// findingsKey identifies a set of findings, to tell a new one from those
// already reported.
func findingsKey(list []finding) string {
	if len(list) == 0 {
		return ""
	}
	var whats []string
	for _, f := range list {
		whats = append(whats, f.What)
	}
	sort.Strings(whats)
	sum := sha256.Sum256([]byte(strings.Join(whats, "\n")))
	return hex.EncodeToString(sum[:8])
}

// yes asks a yes-or-no question; Enter gives def.
func (e *env) yes(question string, def bool) bool {
	hint := " [y/N] "
	if def {
		hint = " [Y/n] "
	}
	answer, ok := e.answer(question + hint)
	if !ok {
		return false // the end of the input is never a yes
	}
	switch strings.ToLower(answer) {
	case "":
		return def
	case "y", "yes":
		return true
	}
	return false
}

// missingProjects says, as information, which of the cadrei's projects have
// no folder on this machine.
func (e *env) missingProjects(r *cadreis.Resolved) {
	var names []string
	for _, p := range projectsOf(r.Cadrei) {
		if !p.Cloned {
			names = append(names, p.Name)
		}
	}
	if len(names) > 0 {
		e.say("Not on this machine yet: %s (ask the orchestrator to clone or link them).", strings.Join(names, ", "))
	}
}

// runCheck is cadrei --check: the full health check, with its fixes.
func runCheck(e *env) int {
	if len(e.args) > 0 {
		return e.fail("usage: cadrei --check")
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return e.fail("%s", err)
	}
	found, fatal := e.health(rt, true)
	if found == 0 {
		e.say("cadrei %s: everything is in order", version)
	}
	if fatal {
		return 1
	}
	return 0
}
