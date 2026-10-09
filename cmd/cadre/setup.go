//go:build !windows

package main

import (
	"fmt"
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
		e.fail("restoring a cadre from GitHub is not built yet in this version; start a new one for now")
		return false
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
