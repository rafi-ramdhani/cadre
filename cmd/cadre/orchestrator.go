//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	cadre "github.com/rafi-ramdhani/cadre"
	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/orchestrator"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/session"
	"golang.org/x/term"
)

// runPlain is plain cadre: open this cadre's orchestrator in this
// terminal, or in tmux with --tmux.
func runPlain(e *env) int {
	useTmux, detach := false, false
	for _, a := range e.args {
		switch a {
		case "--tmux":
			useTmux = true
		case "--detach":
			detach = true
		default:
			return e.fail("usage: cadre [--tmux [--detach]]")
		}
	}
	if detach && !useTmux {
		return e.fail("--detach goes with --tmux")
	}
	if e.member("start the orchestrator") {
		return 1
	}
	rt, err := runtime.Get(runtimeName())
	if err != nil {
		return e.fail("%s", err)
	}
	first := false
	if list, _ := cadres.List(); len(list) == 0 {
		if !e.firstRun(rt) {
			return 1
		}
		first = true
	}
	r, ok := e.resolve()
	if !ok {
		return 1
	}
	if !first {
		e.openingNotes(r)
	}
	if _, fatal := e.health(rt, first); fatal {
		return 1
	}
	if e.eof {
		return e.fail("input ended before the orchestrator opened; run cadre again to open it")
	}
	if err := runtime.CanOrchestrate(rt); err != nil {
		return e.fail("%s", err)
	}
	mode := e.mode(e.conf(r))
	if err := runtime.Usable(rt, mode); err != nil {
		return e.fail("%s", err)
	}
	if first {
		e.say("%s", greeting)
	} else {
		e.missingProjects(r)
	}
	if useTmux && !e.tmuxReady() {
		return 1
	}
	build := rt.BuildDir(r.Path)
	if err := session.EnsureBuild(build); err != nil {
		return e.fail("%s", err)
	}
	// One cadre run at a time reads the lock, starts and writes the lock.
	guard, err := orchestrator.Guard(build)
	if err != nil {
		return e.fail("%s", err)
	}
	released := false
	release := func() {
		if !released {
			guard.Release()
			released = true
		}
	}
	defer release()
	t := session.Default()
	lockPath := orchestrator.LockPath(build)
	orchestrator.ClearStale(lockPath)
	if l := openOrchestrator(t, r.Name, r.Path, lockPath, true); l != nil {
		if l.Mode == orchestrator.Tmux {
			release()
			e.say("the orchestrator of %s is already running", r.Name)
			return e.attachTo(l.Session, detach)
		}
		where := "since " + l.Since.Format("15:04")
		if l.TTY != "" {
			where = l.TTY + ", " + where
		}
		return e.fail("the orchestrator of %s is already open in another terminal (%s); use that one, or close it first", r.Name, where)
	}
	// One in tmux that cadre could not record in the lock still counts.
	if name := orchestrator.SessionName(r.Name); !useTmux && haveTmux() && orchestratorSession(t, name, r.Path, 0) {
		release()
		e.say("the orchestrator of %s is already running", r.Name)
		return e.attachTo(name, detach)
	}
	text, _ := cadre.Assets.ReadFile("orchestrator.md")
	from := ""
	if r.Project != "" {
		from, _ = projectDir(r, r.Project)
	}
	prompt := filepath.Join(build, "orchestrator.md")
	if err := fsx.WriteFile(prompt, orchestrator.Prompt(text, r.Name, r.Path, r.Project, from), 0o644); err != nil {
		return e.fail("%s", err)
	}
	cmd, err := rt.Launch(runtime.LaunchSpec{Role: runtime.Orchestrator, Name: orchestrator.Name(r.Name), Cadre: r.Path,
		WorkDir: r.Path, Mode: mode, PromptFile: prompt})
	if err != nil {
		return e.fail("%s", err)
	}
	// Pinned to this cadre, and marked as the orchestrator, so the hook
	// adds nothing more (K.3).
	cmd.Env = append(cmd.Env, "CADRE_HOME="+r.Path, "CADRE_ORCHESTRATOR=1")
	if useTmux {
		return e.startTmux(r, cmd, lockPath, detach, release)
	}
	return e.runTerminal(cmd, lockPath, release)
}

func haveTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// orchestratorSession reports whether the tmux session name is this
// cadre's orchestrator: marked as such for home, and, when pid is set,
// running pid in its pane.
func orchestratorSession(t session.Tmux, name, home string, pid int) bool {
	if !t.Has(name) || t.Option(name, "@cadre_role") != "orchestrator" || t.Option(name, "@cadre_home") != home {
		return false
	}
	if pid > 0 {
		p, err := t.PanePID(name, "orchestrator")
		return err == nil && p == pid
	}
	return true
}

// openOrchestrator returns the cadre's open orchestrator from its lock, or
// nil. A tmux lock counts only when it names this cadre's orchestrator
// session running the recorded process; otherwise the lock was not
// cadre's (a member's shell can write it), so cadre never attaches the
// user to a session that only claims to be the orchestrator. A caller
// holding the start guard sets clear, and such a lock is removed.
func openOrchestrator(t session.Tmux, cadre, home, lockPath string, clear bool) *orchestrator.Lock {
	l := orchestrator.ReadLock(lockPath)
	if l == nil || l.Mode != orchestrator.Tmux {
		return l
	}
	if l.Session != orchestrator.SessionName(cadre) || !orchestratorSession(t, l.Session, home, l.PID) {
		if clear {
			orchestrator.RemoveLock(lockPath, l)
		}
		return nil
	}
	return l
}

// openingNotes says, in one line each, which cadre opens and why, before
// Claude Code takes the terminal.
func (e *env) openingNotes(r *cadres.Resolved) {
	if r.From != "default" {
		return
	}
	e.say("Opening your default cadre %s (%s). To open another, run cadre in one of its projects.", r.Name, display(r.Path))
	wd, err := paths.Getwd()
	if err != nil {
		return
	}
	if look := lookalike(wd); look != "" {
		e.say("%s looks like a cadre from before 0.2.0: move it into ~/.cadre with cadre migrate.", look)
		return
	}
	if top, err := exec.Command("git", "-C", wd, "rev-parse", "--show-toplevel").Output(); err == nil && len(top) > 0 {
		e.say("This folder is not linked to a cadre; ask the orchestrator to link it.")
	}
}

// lookalike returns the folder at or above dir that looks like a 0.1.x
// cadre (personas/ and projects.yaml) but is not a known one, or "".
// Nothing in it is read.
func lookalike(dir string) string {
	known := map[string]bool{}
	list, _ := cadres.List()
	for _, c := range list {
		known[c.Path] = true
	}
	for d := dir; ; d = filepath.Dir(d) {
		// 0.1.x cadres keep their members in personas/.
		st, err := os.Stat(filepath.Join(d, "personas"))
		_, err2 := os.Stat(filepath.Join(d, "projects.yaml"))
		if err == nil && st.IsDir() && err2 == nil && !known[d] && !cadres.Inside(d) {
			return d
		}
		if d == "/" || d == "." {
			return ""
		}
	}
}

// runTerminal runs the orchestrator as a child in this terminal, holding
// the lock while it runs, and returns its exit code. release ends the
// start guard once the lock is written.
func (e *env) runTerminal(c runtime.Command, lockPath string, release func()) int {
	cmd := exec.Command(c.Argv[0], c.Argv[1:]...)
	cmd.Dir = c.Dir
	cmd.Env = append(withoutMember(os.Environ()), c.Env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = e.stdin, e.stdout, e.stderr
	// The terminal comes back as it was, even if the child dies raw.
	if f, ok := e.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if saved, err := term.GetState(int(f.Fd())); err == nil {
			defer term.Restore(int(f.Fd()), saved)
		}
	}
	// Ctrl-C and the like are for Claude Code: cadre catches them and only
	// waits. Caught, not ignored: an ignored signal stays ignored in the
	// child across exec.
	quiet := make(chan os.Signal, 1)
	signal.Notify(quiet, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		for range quiet {
		}
	}()
	defer func() {
		signal.Stop(quiet)
		close(quiet)
	}()
	if err := cmd.Start(); err != nil {
		return e.fail("could not start the orchestrator: %s", err)
	}
	tty := terminalName()
	lock, lockErr := orchestrator.WriteLock(lockPath, cmd.Process.Pid, orchestrator.Terminal, tty, "")
	release()
	if lockErr == nil {
		// Removed under the start guard, so a run starting now never
		// loses the lock it just wrote.
		defer func() {
			if g, err := orchestrator.Guard(filepath.Dir(lockPath)); err == nil {
				orchestrator.RemoveLock(lockPath, lock)
				g.Release()
			}
		}()
	}
	// A closed terminal or a kill ends the orchestrator; the lock goes with it.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		for s := range stop {
			cmd.Process.Signal(s)
		}
	}()
	err := cmd.Wait()
	// Said once the orchestrator is gone: while it runs, the terminal is
	// its own, and its output is still being copied.
	if lockErr != nil {
		fmt.Fprintf(e.stderr, "warning: could not record the open orchestrator: %s\n", lockErr)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	if err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// withoutMember drops CADRE_MEMBER (and 0.1.x's CADRE_PERSONA): the
// orchestrator is the user's own session, never a member's.
func withoutMember(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "CADRE_MEMBER=") && !strings.HasPrefix(kv, "CADRE_PERSONA=") {
			out = append(out, kv)
		}
	}
	return out
}

// startTmux starts the orchestrator in its tmux session (K.1), or finds
// the running one, and attaches unless detach is set.
func (e *env) startTmux(r *cadres.Resolved, c runtime.Command, lockPath string, detach bool, release func()) int {
	t := session.Default()
	name := orchestrator.SessionName(r.Name)
	if t.Has(name) {
		if t.Option(name, "@cadre_role") == "orchestrator" && t.Option(name, "@cadre_home") == r.Path {
			release()
			e.say("the orchestrator of %s is already running", r.Name)
			return e.attachTo(name, detach)
		}
		return e.fail("tmux session %s exists and is not this cadre's orchestrator; rename it (tmux rename-session) or keep using it by hand", name)
	}
	hint, hooks := t.HintOptions()
	err := t.Start(session.StartSpec{Session: name, Window: "orchestrator", Dir: c.Dir, Env: c.Env, Argv: c.Argv,
		SessionOptions: append([]session.Option{{Name: "@cadre_home", Value: r.Path}, {Name: "@cadre_role", Value: "orchestrator"}}, hint...),
		SessionHooks:   hooks})
	if err != nil {
		return e.fail("%s", err)
	}
	time.Sleep(500 * time.Millisecond)
	if !t.HasWindow(name, "orchestrator") || t.PaneDead(name, "orchestrator") {
		return e.fail("the orchestrator failed to start: its command exited at once; run it by hand in %s to see why:\n    %s",
			c.Dir, session.Line(append(c.Env, c.Argv...)))
	}
	// Without the lock, cadre still finds it by its session (runPlain).
	pid, err := t.PanePID(name, "orchestrator")
	if err == nil {
		_, err = orchestrator.WriteLock(lockPath, pid, orchestrator.Tmux, "", name)
	}
	if err != nil {
		fmt.Fprintf(e.stderr, "warning: could not record the orchestrator in %s (%s); cadre finds it by its tmux session\n", lockPath, err)
	}
	release()
	if detach || !e.interactive() {
		e.say("started the orchestrator of %s; attach with: cadre attach", r.Name)
		return 0
	}
	return e.attachTo(name, false)
}

// attachTo attaches to a session, or only says how when detach is set or
// there is no terminal.
func (e *env) attachTo(name string, detach bool) int {
	if detach || !e.interactive() {
		e.say("attach with: cadre attach")
		return 0
	}
	if err := session.Default().Attach(name); err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// attachOrchestrator is cadre attach with no team (K.2).
func (e *env) attachOrchestrator(r *cadres.Resolved) int {
	t := session.Default()
	name := orchestrator.SessionName(r.Name)
	if !t.Has(name) || t.Option(name, "@cadre_role") != "orchestrator" || t.Option(name, "@cadre_home") != r.Path {
		return e.fail("the orchestrator of %s is not running in tmux; start it with cadre --tmux", r.Name)
	}
	if !e.interactive() {
		return e.fail("cadre attach needs a terminal")
	}
	if err := t.Attach(name); err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// terminalName is this process's controlling terminal as /dev/<name>, or
// "" when it has none (a pipe on stdin is not a terminal).
func terminalName() string {
	out, err := exec.Command("ps", "-o", "tty=", "-p", strconv.Itoa(os.Getpid())).Output()
	name := strings.TrimSpace(string(out))
	if err != nil || strings.Trim(name, "?") == "" {
		return ""
	}
	return orchestrator.CleanTTY("/dev/" + name)
}
