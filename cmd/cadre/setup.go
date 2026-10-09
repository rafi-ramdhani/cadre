//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/framework"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/project"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// greeting is what a new user reads before the orchestrator opens.
const greeting = "Your cadre is ready. Tell me which repo to work on, for example: work on github.com/you/app."

// firstRun sets up the first cadre on this machine, asking before it
// creates anything: a new cadre (with the starter team, and this folder
// linked when it is a git repository and the user says yes), or a restore
// from GitHub. Without a terminal it says what to run instead. It reports
// whether plain cadre goes on to open the orchestrator.
func (e *env) firstRun(rt runtime.Runtime) bool {
	if !e.interactive() {
		e.fail("no cadre yet; run cadre in a terminal to set one up, or create one with cadre init <name>")
		return false
	}
	e.say("Welcome to cadre: a team of %s sessions that you lead from one conversation.", rt.Title())
	switch strings.ToLower(e.ask("Start a new cadre, or restore one from GitHub? [new/restore] ")) {
	case "", "n", "new":
	case "r", "restore":
		if !e.restoreCadre(rt) {
			return false
		}
		e.offerHook(rt)
		return true
	default:
		e.fail("answer new or restore; nothing was changed")
		return false
	}
	c, ok := e.newCadre()
	if !ok {
		return false
	}
	e.say("Created your cadre %s, with a dev team (an engineer and a reviewer).", c.Name)
	e.offerLink()
	e.offerHook(rt)
	return true
}

// suggestName is the name offered for a first cadre: the user's login when
// it makes a valid name, else "main".
func suggestName() string {
	if u, err := user.Current(); err == nil {
		if n := strings.ToLower(u.Username); cadres.CheckName(n) == nil {
			return n
		}
	}
	return "main"
}

// newCadre asks for a name and creates the cadre as the default.
func (e *env) newCadre() (cadres.Cadre, bool) {
	suggest := suggestName()
	for tries := 0; tries < 3; tries++ {
		name := e.ask(fmt.Sprintf("Name for your cadre? [%s] ", suggest))
		if name == "" {
			name = suggest
		}
		c, note, err := cadres.Create(name, cadre.Assets)
		if err != nil {
			fmt.Fprintf(e.stderr, "%s\n", err)
			continue
		}
		if note != "" {
			e.say("%s", note)
		}
		e.guard(c)
		if err := cadres.SetDefault(c.Name); err != nil {
			e.fail("%s", err)
			return c, false
		}
		return c, true
	}
	e.fail("no cadre was created")
	return cadres.Cadre{}, false
}

// offerLink offers to link the git repository the user is in to the new
// cadre, as a project of the dev team.
func (e *env) offerLink() {
	wd, err := paths.Getwd()
	if err != nil || cadres.Inside(wd) {
		return
	}
	out, err := exec.Command("git", "-C", wd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return
	}
	top := paths.Real(strings.TrimSpace(string(out)))
	name := filepath.Base(top)
	if project.CheckName(name) != nil || !e.yes(fmt.Sprintf("Link this folder (%s) to your cadre?", name), true) {
		return
	}
	sub := &env{args: []string{name, "--path", top}, stdin: e.stdin, stdout: e.stdout, stderr: e.stderr, lines: e.lines}
	runProjectAdd(sub)
}

// offerHook offers the hook that makes every new session of the runtime the
// orchestrator, when there is none yet.
func (e *env) offerHook(rt runtime.Runtime) {
	hooks := rt.Hooks()
	if progs, err := hooks.Find(); err != nil || len(progs) > 0 {
		return
	}
	if !e.yes(fmt.Sprintf("Make every new %s session the orchestrator?", rt.Title()), false) {
		return
	}
	if _, err := hooks.Set(framework.Binary()); err != nil {
		fmt.Fprintf(e.stderr, "could not add the hook: %s\n", err)
		return
	}
	e.say("Added the hook to %s.", display(hooks.File()))
}

// restoreName is the name a restored cadre gets on this machine: the
// repository's name, without the cadre- a backup's name starts with.
func restoreName(repo string) string {
	name := strings.TrimSuffix(filepath.Base(strings.TrimRight(repo, "/")), ".git")
	if n := strings.TrimPrefix(name, "cadre-"); n != "" {
		name = n
	}
	return name
}

// restoreCadre restores a cadre from its backup repository: it clones it
// into ~/.cadre/<name>, makes it the default, asks where projects go when
// some must be cloned, then clones and trusts them (project sync).
// Projects without a repo stay missing until linked.
func (e *env) restoreCadre(rt runtime.Runtime) bool {
	repo := ""
	for tries := 0; tries < 3 && repo == ""; tries++ {
		repo = e.ask("Which repository holds your cadre? (owner/repo, or its URL) ")
		if strings.HasPrefix(repo, "-") {
			fmt.Fprintln(e.stderr, "a repository cannot start with -")
			repo = ""
		}
	}
	if repo == "" {
		e.fail("no repository given; nothing was changed")
		return false
	}
	var c cadres.Cadre
	suggest := restoreName(repo)
	for tries := 0; ; tries++ {
		if tries == 3 {
			e.fail("no cadre was restored")
			return false
		}
		name := e.ask(fmt.Sprintf("Name for this cadre on this machine? [%s] ", suggest))
		if name == "" {
			name = suggest
		}
		if err := cadres.CheckName(name); err != nil {
			fmt.Fprintln(e.stderr, err)
			continue
		}
		dest := filepath.Join(cadres.Root(), name)
		if _, err := os.Lstat(dest); err == nil {
			fmt.Fprintf(e.stderr, "%s already exists; choose another name\n", dest)
			continue
		}
		if other, ok := cadres.Clash(name, dest); ok {
			fmt.Fprintf(e.stderr, "a cadre named %s is already at %s; choose another name\n", other.Name, other.Path)
			continue
		}
		c = cadres.Cadre{Name: name, Path: dest}
		break
	}
	e.say("Cloning %s into %s...", repo, display(c.Path))
	if err := project.Clone(repo, c.Path); err != nil {
		e.fail("%s", err)
		return false
	}
	c.Path = paths.Real(c.Path)
	if !c.Present() {
		// Only what this command just cloned is removed.
		os.RemoveAll(c.Path)
		e.fail("%s is not a cadre (it has no personas/ folder); nothing was kept", repo)
		return false
	}
	e.guard(c)
	if err := cadres.SetDefault(c.Name); err != nil {
		e.fail("%s", err)
		return false
	}
	e.say("Restored your cadre %s.", c.Name)
	sub := &env{stdin: e.stdin, stdout: e.stdout, stderr: e.stderr, lines: e.lines}
	runProjectSync(sub)
	e.lines = sub.lines
	return true
}
