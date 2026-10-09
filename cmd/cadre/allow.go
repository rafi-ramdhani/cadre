//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
)

// ago writes how long ago t was: "3d ago", "2h ago", "5m ago", "just now".
func ago(t time.Time) string {
	s := int(time.Since(t).Seconds())
	for _, u := range []struct {
		unit string
		n    int
	}{{"d", 86400}, {"h", 3600}, {"m", 60}} {
		if s >= u.n {
			return fmt.Sprintf("%d%s ago", s/u.n, u.unit)
		}
	}
	return "just now"
}

func runAllowList(e *env) int {
	if len(e.args) > 1 || (len(e.args) == 1 && e.args[0] != "list") {
		return e.fail("usage: cadre allow [list]")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	perms := rt.Permissions()
	file := perms.GrantsFile(r.Path)
	if _, err := os.Stat(file); err != nil {
		e.say("Grants for members: none yet (%s is created by the first cadre up or cadre allow add)", file)
		return 0
	}
	store, err := perms.Open(r.Path, false)
	if err != nil {
		fmt.Fprintf(e.stderr, "warning: members start without %s because %s\n", file, err)
		return 1
	}
	if !perms.Unchanged(r.Path) {
		fmt.Fprintf(e.stderr, "warning: %s was changed outside cadre allow (see git -C %s diff and log -p)\n", file, r.Path)
	}
	e.say("Grants for members (%s):", file)
	list := store.List()
	if len(list) == 0 {
		e.say("  none yet")
	}
	for i, g := range list {
		var marks []string
		if g.Once {
			marks = append(marks, "once, added "+ago(g.Added))
		}
		if g.Wide {
			marks = append(marks, "wide: contains *")
		}
		mark := ""
		if len(marks) > 0 {
			mark = "   [" + strings.Join(marks, "; ") + "]"
		}
		e.say("  %d. %-4s  %s%s", i+1, g.Kind, g.Entry, mark)
	}
	if b := perms.BuiltIn(); b != "" {
		e.say("%s", b)
	}
	return 0
}

// allowLock serializes cadre allow changes in one cadre (read, change,
// write, commit). The lock file stays in the build folder, which git
// ignores, so the cadre's repository stays clean.
func allowLock(build, file string) (*fsx.Lock, error) {
	if err := session.EnsureBuild(build); err != nil {
		return nil, err
	}
	l, err := fsx.Acquire(filepath.Join(build, "allow.lock"), 10*time.Second)
	if errors.Is(err, fsx.ErrBusy) {
		return nil, fmt.Errorf("another cadre allow is changing %s; try again", file)
	}
	return l, err
}

func runAllowAdd(e *env) int    { return allowChange(e, "add") }
func runAllowRemove(e *env) int { return allowChange(e, "remove") }

func allowChange(e *env, op string) int {
	if inMember() {
		return e.fail("refused for members: members cannot change permissions; report the blocked action to the orchestrator instead")
	}
	kind, once := "rule", false
	var args []string
	for _, a := range e.args {
		switch a {
		case "--once":
			once = true
		case "--auto":
			kind = "auto"
		default:
			if strings.HasPrefix(a, "--") {
				return e.fail("unknown option %s (see cadre help advanced)", a)
			}
			args = append(args, a)
		}
	}
	switch {
	case op == "add" && len(args) != 1:
		return e.fail("usage: cadre allow add [--once] <rule> | cadre allow add [--once] --auto \"<sentence>\"")
	case op == "remove" && (kind == "auto" || (once && len(args) != 0) || (!once && len(args) != 1)):
		return e.fail("usage: cadre allow remove <rule | number> | cadre allow remove --once")
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	rt, ok := e.cadreRuntime(r)
	if !ok {
		return 1
	}
	perms := rt.Permissions()
	file := perms.GrantsFile(r.Path)
	_, existed := os.Stat(file)
	// Create the file if needed; it is read again under the lock below.
	if _, err := perms.Open(r.Path, true); err != nil {
		return e.fail("%s cannot be changed because %s; restore it with git -C %s checkout -- %s", file, err, r.Path, relTo(r.Path, file))
	}
	if existed != nil {
		e.say("  created %s (grants for members; change it with cadre allow)", file)
	}
	lock, err := allowLock(rt.BuildDir(r.Path), file)
	if err != nil {
		return e.fail("%s", err)
	}
	defer lock.Release()
	// Read under the lock: another cadre allow may have just written.
	store, err := perms.Open(r.Path, false)
	if err != nil {
		return e.fail("%s cannot be changed because %s", file, err)
	}
	if !perms.Unchanged(r.Path) {
		fmt.Fprintf(e.stderr, "warning: %s was changed outside cadre allow (see git -C %s diff and log -p); this change keeps those edits\n", file, r.Path)
	}
	var msg string
	if op == "add" {
		entry := args[0]
		warning, err := perms.Validate(r.Path, knownPaths(), entry, kind == "auto")
		if err != nil {
			return e.fail("%s", err)
		}
		switch err := store.Add(kind, entry, once); {
		case errors.Is(err, runtime.ErrGranted):
			e.say("  %s is already granted; nothing changed", entry)
			return 0
		case err != nil:
			return e.fail("%s", err)
		}
		msg = "Allow for members: " + entry
		if once {
			msg = "Allow for members once: " + entry
		}
		if warning != "" {
			e.say("%s", warning)
		}
		what := "rule"
		if kind == "auto" {
			what = "--auto entry"
		}
		if once {
			what = "one-time " + what
		}
		e.say("  added %s: %s", what, entry)
	} else {
		target := "--once"
		if !once {
			target = args[0]
		}
		removed, stale, err := store.Remove(target)
		switch {
		case errors.Is(err, runtime.ErrNoOnce):
			e.say("  there are no one-time grants; nothing changed")
			return 0
		case err != nil:
			return e.fail("%s", err)
		}
		switch {
		case len(removed) == 0:
			msg = "Drop stale one-time grant records"
		case once:
			msg = "Remove one-time grants for members"
		default:
			msg = "Remove grant for members: " + removed[0]
		}
		for _, x := range removed {
			e.say("  removed: %s", x)
		}
		for _, x := range stale {
			e.say("  already gone: %s", x)
		}
	}
	perms.Record(r.Path)
	if note, _ := cadres.Commit(r.Path, msg, store.Files()...); note != "" {
		e.say("%s", note)
	}
	e.restartNote(r)
	return 0
}

// relTo writes path relative to dir, for a git command run in dir.
func relTo(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return path
	}
	return rel
}

// knownPaths lists every known cadre's folder.
func knownPaths() []string {
	var out []string
	list, _ := cadres.List()
	for _, c := range list {
		out = append(out, c.Path)
	}
	return out
}

// restartNote says which running members still have the old grants, with
// the commands that restart each (the design assumes a running session
// does not reload its grants).
func (e *env) restartNote(r *cadres.Resolved) {
	s := scope(r)
	list := s.Running()
	if len(list) == 0 {
		e.say("Members started from now on get this change.")
		return
	}
	e.say("Running members will not see this change until restarted (each has the copy it started with):")
	e.showSessions(s.T, list, nil)
	// The commands carry the cadre, since the user may run them anywhere.
	pin := "CADRE_HOME=" + session.Quote(r.Path) + " "
	e.say("Restart one with:")
	generic := false
	for _, i := range list {
		if i.Team == "" {
			generic = true
			continue
		}
		for _, role := range s.T.Windows(i.Name) {
			target := ""
			if i.Target != "" {
				target = " " + session.Quote(i.Target)
			}
			project := ""
			if i.Project != "" {
				project = " " + i.Project
			}
			e.say("  %scadre stop %s/%s%s && %scadre up %s/%s%s", pin, i.Team, role, project, pin, i.Team, role, target)
		}
	}
	if generic {
		e.say("  %scadre stop <team> [project] && %scadre up <team> [project]", pin, pin)
		e.say("  (a 0.1.x session cadre-<team>-<project> is team <team>, project <project>)")
	}
}
