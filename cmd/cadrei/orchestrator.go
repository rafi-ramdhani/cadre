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

	cadrei "github.com/rafi-ramdhani/cadrei"
	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/fsx"
	"github.com/rafi-ramdhani/cadrei/internal/orchestrator"
	"github.com/rafi-ramdhani/cadrei/internal/paths"
	"github.com/rafi-ramdhani/cadrei/internal/runtime"
	"github.com/rafi-ramdhani/cadrei/internal/session"
	"golang.org/x/term"
)

// runPlain is plain cadrei: open this cadrei's orchestrator in this
// terminal, or in tmux with --tmux.
func runPlain(e *env) int {
	useTmux, detach, fresh := false, false, false
	for _, a := range e.args {
		switch a {
		case "--tmux":
			useTmux = true
		case "--detach":
			detach = true
		case "--fresh":
			fresh = true
		default:
			return e.fail("usage: cadrei [--tmux [--detach]] [--fresh]")
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
	if list, _ := cadreis.List(); len(list) == 0 {
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
		return e.fail("input ended before the orchestrator opened; run cadrei again to open it")
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
		if old := cadreis.OldHome(); old != "" {
			e.say("You have a cadre from 0.1.x at %s. To bring it in, tell the orchestrator: bring in my old cadre from %s.", display(old), display(old))
		}
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
	// One cadrei run at a time reads the lock, starts and writes the lock.
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
	// One in tmux that cadrei could not record in the lock still counts.
	if name := orchestrator.SessionName(r.Name); !useTmux && haveTmux() && orchestratorSession(t, name, r.Path, 0) {
		release()
		e.say("the orchestrator of %s is already running", r.Name)
		return e.attachTo(name, detach)
	}
	text, _ := cadrei.Assets.ReadFile("orchestrator.md")
	from := ""
	if r.Project != "" {
		from, _ = projectDir(r, r.Project)
	}
	prompt := filepath.Join(build, "orchestrator.md")
	if err := fsx.WriteFile(prompt, orchestrator.Prompt(text, r.Name, r.Path, r.Project, from), 0o644); err != nil {
		return e.fail("%s", err)
	}
	// The orchestrator resumes its last conversation, as members do.
	record := session.RecordPath(build, orchestrator.Name(r.Name))
	launch := func(conv session.Conversation) (runtime.Command, error) {
		if conv.Note != "" {
			e.say("The orchestrator: %s.", conv.Note)
		}
		if conv.SessionID != "" {
			session.WriteRecord(record, session.Record{ID: conv.SessionID, Dir: r.Path, Since: time.Now()})
			cadreis.SetState(cadreis.OrchestratorID(r.Name), conv.SessionID)
		}
		cmd, err := rt.Launch(runtime.LaunchSpec{Role: runtime.Orchestrator, Name: orchestrator.Name(r.Name), Cadrei: r.Path,
			WorkDir: r.Path, Mode: mode, PromptFile: prompt, SessionID: conv.SessionID, Resume: conv.Resume})
		// Pinned to this cadrei, and marked as the orchestrator, so the
		// hook adds nothing more (K.3).
		cmd.Env = append(cmd.Env, "CADREI_HOME="+r.Path, "CADREI_ORCHESTRATOR=1")
		return cmd, err
	}
	conv := session.Plan(rt, record, r.Path, fresh)
	// Only a conversation cadrei itself started for this orchestrator is
	// resumed: a record written by anything else is not.
	if conv.Resume != "" && conv.Resume != cadreis.GetState(cadreis.OrchestratorID(r.Name)) {
		conv = session.Conversation{SessionID: rt.Sessions().NewID(), Note: "a new conversation: its record is not one cadrei wrote"}
	}
	cmd, err := launch(conv)
	if err != nil {
		return e.fail("%s", err)
	}
	// When the runtime will not resume the conversation, a new one.
	var renew func() (runtime.Command, error)
	if conv.Resume != "" {
		renew = func() (runtime.Command, error) {
			return launch(session.Conversation{SessionID: rt.Sessions().NewID(), Note: "a new conversation: resuming failed"})
		}
	}
	if useTmux {
		return e.startTmux(r, cmd, lockPath, detach, release, renew)
	}
	start := time.Now()
	var mod0 time.Time
	var size0 int64
	if renew != nil {
		mod0, size0, _ = rt.Sessions().Transcript(conv.Resume, r.Path)
	}
	code := e.runTerminal(cmd, lockPath, release)
	// The run keeps the real terminal, so its output is not read: a resume
	// failed when the run ended at once, with an error, and wrote nothing
	// to the conversation. A quick quit after it got going is not that.
	if renew != nil && code != 0 && time.Since(start) < 5*time.Second && untouched(rt, conv.Resume, r.Path, mod0, size0) {
		// Under the start guard again, so no other cadrei run opens a
		// second orchestrator meanwhile.
		g, err := orchestrator.Guard(build)
		if err != nil {
			return e.fail("%s", err)
		}
		orchestrator.ClearStale(lockPath)
		if l := openOrchestrator(t, r.Name, r.Path, lockPath, true); l != nil {
			g.Release()
			return e.fail("the orchestrator of %s was opened elsewhere meanwhile; use that one", r.Name)
		}
		if cmd, err = renew(); err != nil {
			g.Release()
			return e.fail("%s", err)
		}
		code = e.runTerminal(cmd, lockPath, func() { g.Release() })
	}
	return code
}

func haveTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// orchestratorSession reports whether the tmux session name is this
// cadrei's orchestrator: marked as such for home, and, when pid is set,
// running pid in its pane.
func orchestratorSession(t session.Tmux, name, home string, pid int) bool {
	if !t.Has(name) || t.Option(name, "@cadrei_role") != "orchestrator" || t.Option(name, "@cadrei_home") != home {
		return false
	}
	if pid > 0 {
		p, err := t.PanePID(name, "orchestrator")
		return err == nil && p == pid
	}
	return true
}

// openOrchestrator returns the cadrei's open orchestrator from its lock, or
// nil. A tmux lock counts only when it names this cadrei's orchestrator
// session running the recorded process; otherwise the lock was not
// cadrei's (a member's shell can write it), so cadrei never attaches the
// user to a session that only claims to be the orchestrator. A caller
// holding the start guard sets clear, and such a lock is removed.
func openOrchestrator(t session.Tmux, cadrei, home, lockPath string, clear bool) *orchestrator.Lock {
	l := orchestrator.ReadLock(lockPath)
	if l == nil || l.Mode != orchestrator.Tmux {
		return l
	}
	if l.Session != orchestrator.SessionName(cadrei) || !orchestratorSession(t, l.Session, home, l.PID) {
		if clear {
			orchestrator.RemoveLock(lockPath, l)
		}
		return nil
	}
	return l
}

// openingNotes says, in one line each, which cadrei opens and why, before
// Claude Code takes the terminal.
func (e *env) openingNotes(r *cadreis.Resolved) {
	if r.From != "default" {
		return
	}
	e.say("Opening your default cadrei %s (%s). To open another, run cadrei in one of its projects.", r.Name, display(r.Path))
	wd, err := paths.Getwd()
	if err != nil {
		return
	}
	if look := lookalike(wd); look != "" {
		e.say("%s looks like a cadre from 0.1.x. To bring it in, tell the orchestrator: bring in my old cadre from %s.", look, look)
		return
	}
	if top, err := exec.Command("git", "-C", wd, "rev-parse", "--show-toplevel").Output(); err == nil && len(top) > 0 {
		e.say("This folder is not linked to a cadrei; ask the orchestrator to link it.")
	}
}

// lookalike returns the folder at or above dir that looks like a 0.1.x
// cadrei, or "". Nothing in it is read.
func lookalike(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if cadreis.OldCadre(d) {
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
	// Ctrl-C and the like are for Claude Code: cadrei catches them and only
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

// withoutMember drops CADREI_MEMBER (and 0.1.x's CADRE_PERSONA): the
// orchestrator is the user's own session, never a member's.
func withoutMember(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "CADREI_MEMBER=") && !strings.HasPrefix(kv, "CADRE_PERSONA=") {
			out = append(out, kv)
		}
	}
	return out
}

// startTmux starts the orchestrator in its tmux session (K.1), or finds
// the running one, and attaches unless detach is set.
func (e *env) startTmux(r *cadreis.Resolved, c runtime.Command, lockPath string, detach bool, release func(), renew func() (runtime.Command, error)) int {
	t := session.Default()
	name := orchestrator.SessionName(r.Name)
	if t.Has(name) {
		if t.Option(name, "@cadrei_role") == "orchestrator" && t.Option(name, "@cadrei_home") == r.Path {
			release()
			e.say("the orchestrator of %s is already running", r.Name)
			return e.attachTo(name, detach)
		}
		return e.fail("tmux session %s exists and is not this cadrei's orchestrator; rename it (tmux rename-session) or keep using it by hand", name)
	}
	hint, hooks := t.HintOptions()
	start := func(c runtime.Command) error {
		err := t.Start(session.StartSpec{Session: name, Window: "orchestrator", Dir: c.Dir, Env: c.Env, Argv: c.Argv,
			SessionOptions: append([]session.Option{{Name: "@cadrei_home", Value: r.Path}, {Name: "@cadrei_role", Value: "orchestrator"}}, hint...),
			SessionHooks:   hooks})
		if err != nil {
			return err
		}
		time.Sleep(upWaitOr(500 * time.Millisecond))
		if !t.HasWindow(name, "orchestrator") || t.PaneDead(name, "orchestrator") {
			return fmt.Errorf("the orchestrator failed to start: its command exited at once; run it by hand in %s to see why:\n    %s",
				c.Dir, session.Line(append(c.Env, c.Argv...)))
		}
		return nil
	}
	err := start(c)
	if err != nil && renew != nil {
		// The runtime would not resume it: start a new conversation.
		t.KillSession(name)
		if c, err = renew(); err == nil {
			err = start(c)
		}
	}
	if err != nil {
		return e.fail("%s", err)
	}
	// Without the lock, cadrei still finds it by its session (runPlain).
	pid, err := t.PanePID(name, "orchestrator")
	if err == nil {
		_, err = orchestrator.WriteLock(lockPath, pid, orchestrator.Tmux, "", name)
	}
	if err != nil {
		fmt.Fprintf(e.stderr, "warning: could not record the orchestrator in %s (%s); cadrei finds it by its tmux session\n", lockPath, err)
	}
	release()
	if detach || !e.interactive() {
		e.say("started the orchestrator of %s; attach with: cadrei attach", r.Name)
		return 0
	}
	return e.attachTo(name, false)
}

// attachTo attaches to a session, or only says how when detach is set or
// there is no terminal.
func (e *env) attachTo(name string, detach bool) int {
	if detach || !e.interactive() {
		e.say("attach with: cadrei attach")
		return 0
	}
	if err := session.Default().Attach(name); err != nil {
		return e.fail("%s", err)
	}
	return 0
}

// attachOrchestrator is cadrei attach with no team (K.2).
func (e *env) attachOrchestrator(r *cadreis.Resolved) int {
	t := session.Default()
	name := orchestrator.SessionName(r.Name)
	if !t.Has(name) || t.Option(name, "@cadrei_role") != "orchestrator" || t.Option(name, "@cadrei_home") != r.Path {
		return e.fail("the orchestrator of %s is not running in tmux; start it with cadrei --tmux", r.Name)
	}
	if !e.interactive() {
		return e.fail("cadrei attach needs a terminal")
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

// upWaitOr is how long a start is given before it is checked: the test
// build's setting, else d.
func upWaitOr(d time.Duration) time.Duration {
	if w := upWait(); w > 0 {
		return w
	}
	return d
}

// untouched reports whether a conversation's transcript is missing, or
// has the time and size it had before the run: the size too, since on
// filesystems with 1 or 2 second times a write in the same tick keeps
// the time.
func untouched(rt runtime.Runtime, id, dir string, mod0 time.Time, size0 int64) bool {
	mod, size, ok := rt.Sessions().Transcript(id, dir)
	return !ok || (mod.Equal(mod0) && size == size0)
}
