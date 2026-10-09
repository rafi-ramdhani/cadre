//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	cadrei "github.com/rafi-ramdhani/cadrei"
	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/framework"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/project"
	"github.com/rafi-ramdhani/cadrei/internal/registry"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
)

// greeting is what a new user reads before the orchestrator opens.
const greeting = "Your cadrei is ready. Tell me which repo to work on, for example: work on github.com/you/app."

// firstRun sets up the first cadrei on this machine, asking before it
// creates anything: a new cadrei (with the starter team, and this folder
// linked when it is a git repository and the user says yes), or a restore
// from GitHub. Without a terminal it says what to run instead. It reports
// whether plain cadrei goes on to open the orchestrator.
func (e *env) firstRun(rt runtime.Runtime) bool {
	if !e.interactive() {
		e.fail("no cadrei yet; run cadrei in a terminal to set one up, or create one with cadrei init <name>")
		return false
	}
	e.say("Welcome to cadrei: a team of %s helpers you lead from one chat.", rt.Title())
	choice, ok := e.answer("Start a new cadrei, or restore one from GitHub? [new/restore] ")
	if !ok {
		e.fail("input ended; nothing was changed")
		return false
	}
	switch strings.ToLower(choice) {
	case "", "n", "new":
	case "r", "restore":
		if !e.restoreCadrei(rt) {
			return false
		}
		e.offerHook(rt)
		if e.eof {
			e.fail("input ended; the cadrei is restored, run cadrei to finish")
			return false
		}
		return true
	default:
		e.fail("answer new or restore; nothing was changed")
		return false
	}
	c, ok := e.newCadrei()
	if !ok {
		return false
	}
	e.say("Created your cadrei %s, with a dev team (an engineer and a reviewer).", c.Name)
	e.offerLink(rt)
	e.offerHook(rt)
	if e.eof {
		e.fail("input ended; your cadrei is created, run cadrei to finish")
		return false
	}
	return true
}

// suggestName is the name offered for a first cadrei: the user's login when
// it makes a valid name, else "main".
func suggestName() string {
	if u, err := user.Current(); err == nil {
		if n := strings.ToLower(u.Username); cadreis.CheckName(n) == nil {
			return n
		}
	}
	return "main"
}

// newCadrei asks for a name and creates the cadrei as the default.
func (e *env) newCadrei() (cadreis.Cadrei, bool) {
	suggest := suggestName()
	for tries := 0; tries < 3; tries++ {
		name, ok := e.answer(fmt.Sprintf("Name for your cadrei? [%s] ", suggest))
		if !ok {
			e.fail("input ended; nothing was changed")
			return cadreis.Cadrei{}, false
		}
		if name == "" {
			name = suggest
		}
		c, note, err := cadreis.Create(name, cadrei.Assets)
		if err != nil {
			fmt.Fprintf(e.stderr, "%s\n", err)
			continue
		}
		if note != "" {
			e.say("%s", note)
		}
		e.guard(c)
		if err := cadreis.SetDefault(c.Name); err != nil {
			e.fail("%s", err)
			return c, false
		}
		return c, true
	}
	e.fail("no cadrei was created")
	return cadreis.Cadrei{}, false
}

// offerLink offers to link the git repository the user is in to the new
// cadrei, as a project of the dev team. Linking trusts the folder in the
// runtime, so the question says so, and the answer defaults to no: a user
// may run cadrei first in a repository they just cloned.
func (e *env) offerLink(rt runtime.Runtime) {
	wd, err := paths.Getwd()
	if err != nil || cadreis.Inside(wd) {
		return
	}
	out, err := exec.Command("git", "-C", wd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return
	}
	top := paths.Real(strings.TrimSpace(string(out)))
	name := filepath.Base(top)
	if project.CheckName(name) != nil || !e.yes(fmt.Sprintf("Link this folder (%s) to your cadrei and trust it in %s?", name, rt.Title()), false) {
		return
	}
	sub := &env{args: []string{name, "--path", top}, stdin: e.stdin, stdout: e.stdout, stderr: e.stderr, lines: e.lines, eof: e.eof}
	runProjectAdd(sub)
	e.lines, e.eof = sub.lines, sub.eof
}

// offerHook offers the hook that makes every new session of the runtime the
// orchestrator, when there is none yet.
func (e *env) offerHook(rt runtime.Runtime) {
	hooks := rt.Hooks()
	if progs, err := hooks.Find(); err != nil || len(progs) > 0 {
		return
	}
	bin := framework.Binary()
	if err := e.hookPlaced(bin); err != nil {
		e.say("Not offering the orchestrator hook: %s. Install cadrei (with Homebrew or install.sh) and run it from there to add it.", err)
		return
	}
	if !e.yes(fmt.Sprintf("Make every new %s session the orchestrator?", rt.Title()), false) {
		return
	}
	if _, err := hooks.Set(bin); err != nil {
		fmt.Fprintf(e.stderr, "could not add the hook: %s\n", err)
		return
	}
	e.say("Added the hook to %s.", display(hooks.File()))
}

// restoreName is the name a restored cadrei gets on this machine: the
// repository's name, without the cadrei- a backup's name starts with.
func restoreName(repo string) string {
	name := strings.TrimSuffix(filepath.Base(strings.TrimRight(repo, "/")), ".git")
	if n := strings.TrimPrefix(name, "cadrei-"); n != "" {
		name = n
	}
	return name
}

// restoreCadrei restores a cadrei from its backup repository: it clones it
// into ~/.cadrei/<name>, makes it the default, asks where projects go when
// some must be cloned, then clones and trusts them (project sync).
// Projects without a repo stay missing until linked.
func (e *env) restoreCadrei(rt runtime.Runtime) bool {
	repo := ""
	for tries := 0; tries < 3 && repo == ""; tries++ {
		var ok bool
		if repo, ok = e.answer("Which repository holds your cadrei? (owner/repo, or its URL) "); !ok {
			e.fail("input ended; nothing was changed")
			return false
		}
		if strings.HasPrefix(repo, "-") {
			fmt.Fprintln(e.stderr, "a repository cannot start with -")
			repo = ""
		}
	}
	if repo == "" {
		e.fail("no repository given; nothing was changed")
		return false
	}
	var c cadreis.Cadrei
	suggest := restoreName(repo)
	for tries := 0; ; tries++ {
		if tries == 3 {
			e.fail("no cadrei was restored")
			return false
		}
		name, ok := e.answer(fmt.Sprintf("Name for this cadrei on this machine? [%s] ", suggest))
		if !ok {
			e.fail("input ended; nothing was changed")
			return false
		}
		if name == "" {
			name = suggest
		}
		if err := cadreis.CheckName(name); err != nil {
			fmt.Fprintln(e.stderr, err)
			continue
		}
		dest := filepath.Join(cadreis.Root(), name)
		if _, err := os.Lstat(dest); err == nil {
			fmt.Fprintf(e.stderr, "%s already exists; choose another name\n", dest)
			continue
		}
		if other, ok := cadreis.Clash(name, dest); ok {
			fmt.Fprintf(e.stderr, "a cadrei named %s is already at %s; choose another name\n", other.Name, other.Path)
			continue
		}
		c = cadreis.Cadrei{Name: name, Path: dest}
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
		e.fail("%s is not a cadrei (it has no members/ folder); nothing was kept", repo)
		return false
	}
	// Git checks out a committed link as a link, which cadrei would follow
	// out of the cadrei when it reads members or the playbook, writes its
	// generated files, or starts a team in its folder.
	rtDir, _ := filepath.Rel(c.Path, filepath.Dir(rt.BuildDir(c.Path)))
	links, err := cadreis.CommittedLinks(c.Path, rtDir)
	if err != nil || len(links) > 0 {
		os.RemoveAll(c.Path)
		if err != nil {
			e.fail("%s; nothing was kept", err)
		} else {
			e.fail("%s has links where cadrei reads its files or runs sessions (%s), so nothing was kept; put the files themselves in the backup, then restore again", repo, strings.Join(links, ", "))
		}
		return false
	}
	e.guard(c)
	if err := cadreis.SetDefault(c.Name); err != nil {
		e.fail("%s", err)
		return false
	}
	e.say("Restored your cadrei %s.", c.Name)
	// What the backup brings into the orchestrator once its folder is
	// trusted is shown, and the orchestrator opens only on a yes.
	if loaded := rt.Trust().Loaded(c.Path); len(loaded) > 0 {
		e.say("It holds files that %s loads into the orchestrator once you trust the cadrei's folder (settings and hooks, commands, instructions):", rt.Title())
		for _, f := range loaded {
			e.say("  %s", cadreis.Tilde(filepath.Join(c.Path, f)))
		}
		if !e.yes("Open the orchestrator with them?", false) {
			e.say("The orchestrator was not opened. Look at these files and remove any you did not put there (git -C %s rm <file>, then commit), then run cadrei.", cadreis.Tilde(c.Path))
			return false
		}
	}
	e.offerProjects(rt, c)
	return true
}

// offerProjects lists the restored cadrei's projects that can be cloned,
// with their repositories, and clones and trusts them only on the user's
// yes. Otherwise they stay "not on this machine" for the orchestrator to
// offer later.
func (e *env) offerProjects(rt runtime.Runtime, c cadreis.Cadrei) {
	reg, err := registry.Load(c.Registry())
	if err != nil {
		return
	}
	var lines []string
	for _, entry := range reg.Entries() {
		if entry.Get("repo") != "" && cadreis.Where(cadreis.ProjectDir(c, entry)) != "present" {
			lines = append(lines, fmt.Sprintf("  %-20s %s", entry.Name, entry.Get("repo")))
		}
	}
	if len(lines) == 0 {
		return
	}
	e.say("Its projects:")
	for _, l := range lines {
		e.say("%s", l)
	}
	question := fmt.Sprintf("Clone these %d projects and trust them in %s?", len(lines), rt.Title())
	if len(lines) == 1 {
		question = fmt.Sprintf("Clone this project and trust it in %s?", rt.Title())
	}
	if !e.yes(question, false) {
		e.say("They stay not on this machine; ask the orchestrator to clone them when you want them.")
		return
	}
	sub := &env{stdin: e.stdin, stdout: e.stdout, stderr: e.stderr, lines: e.lines, eof: e.eof}
	runProjectSync(sub)
	e.lines, e.eof = sub.lines, sub.eof
}
