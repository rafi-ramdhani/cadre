//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/proc"
)

// stubClaude replaces the test's stub with one that records its
// arguments, environment and folder, then waits for a release file.
func stubClaude(t *testing.T, home string) {
	t.Helper()
	os.WriteFile(home+"/bin/claude", []byte("#!/bin/sh\n"+stubAnswers+"{ printf '%s\\n' \"$@\"; env; pwd; } > \""+home+"/orch-ran\"\n"+
		"i=0; while [ ! -e \""+home+"/release\" ] && [ $i -lt 100 ]; do sleep 0.05; i=$((i+1)); done\n"), 0o755)
}

func TestPlainCadreOpensTheOrchestratorInThisTerminal(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	must(t, "init", "work")
	c := home + "/.cadre/work"
	os.WriteFile(home+"/release", nil, 0o644)
	out := must(t)
	if !strings.Contains(out, "Opening your default cadre work (~/.cadre/work)") {
		t.Errorf("no opening note: %q", out)
	}
	ran := readFile(t, home+"/orch-ran")
	for _, want := range []string{"--name\nwork-orchestrator\n", "--permission-mode\ndefault\n", "--append-system-prompt-file\n" + c + "/.claude/build/orchestrator.md\n",
		"CADRE_HOME=" + c + "\n", "CADRE_ORCHESTRATOR=1\n", "\n" + c + "\n"} {
		if !strings.Contains(ran, want) {
			t.Errorf("the orchestrator ran without %q:\n%s", want, ran)
		}
	}
	if strings.Contains(ran, "--settings") || strings.Contains(ran, "CADRE_PERSONA=") {
		t.Errorf("the orchestrator got persona settings or a persona name:\n%s", ran)
	}
	if p := readFile(t, c+"/.claude/build/orchestrator.md"); !strings.Contains(p, "This session is the cadre orchestrator") || !strings.Contains(p, "You are the orchestrator of cadre `work`") {
		t.Errorf("prompt %q", p)
	}
	if _, err := os.Stat(c + "/.claude/build/orchestrator.lock"); err == nil {
		t.Error("the lock outlived the orchestrator")
	}
}

func TestOneOrchestratorAtATime(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	must(t, "init", "work")
	lock := home + "/.cadre/work/.claude/build/orchestrator.lock"
	done := make(chan int)
	go func() { code, _, _ := call(); done <- code }()
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(lock); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if out := must(t, "ls"); !strings.Contains(out, "orchestrator: open in a terminal since") {
		t.Errorf("ls while it runs:\n%s", out)
	}
	if code, _, errOut := call(); code != 1 || !strings.Contains(errOut, "is already open in another terminal") {
		t.Errorf("a second orchestrator: %d %q", code, errOut)
	}
	os.WriteFile(home+"/release", nil, 0o644)
	if code := <-done; code != 0 {
		t.Errorf("the orchestrator exited %d", code)
	}
	if out := must(t, "ls"); !strings.Contains(out, "orchestrator: not running") {
		t.Errorf("ls after:\n%s", out)
	}
	// A lock left by a process that is gone is stale.
	os.WriteFile(lock, []byte(`{"pid": 999999, "start": "1", "command": "claude", "mode": "terminal"}`), 0o600)
	if code, _, errOut := call(); code != 0 {
		t.Errorf("a stale lock blocked cadre: %d %q", code, errOut)
	}
}

func TestPlainCadreFromAProjectAndNotes(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	os.MkdirAll(home+"/Developer/app", 0o755)
	register(t, home, "work", "app:\n  repo: me/app\n  path: ~/Developer/app\n")
	t.Chdir(home + "/Developer/app")
	out := must(t)
	if strings.Contains(out, "Opening your default") {
		t.Errorf("a note from inside a linked project: %q", out)
	}
	if p := readFile(t, home+"/.cadre/work/.claude/build/orchestrator.md"); !strings.Contains(p, "The user opened cadre from the project `app` (`"+home+"/Developer/app`)") {
		t.Errorf("prompt %q", p)
	}
	// An unlinked git repository, and a folder that looks like a cadre.
	os.MkdirAll(home+"/src/repo", 0o755)
	exec.Command("git", "-C", home+"/src/repo", "init", "-q").Run()
	t.Chdir(home + "/src/repo")
	if out := must(t); !strings.Contains(out, "This folder is not linked to a cadre") {
		t.Errorf("unlinked repo: %q", out)
	}
	os.MkdirAll(home+"/old/demo/personas", 0o755)
	os.WriteFile(home+"/old/demo/projects.yaml", nil, 0o644)
	os.WriteFile(home+"/old/demo/cadre.conf", []byte("touch "+home+"/marker\n"), 0o644)
	t.Chdir(home + "/old/demo")
	if out := must(t); !strings.Contains(out, home+"/old/demo looks like a cadre from before 0.2.0: move it into ~/.cadre with cadre migrate") {
		t.Errorf("look-alike: %q", out)
	}
	if _, err := os.Stat(home + "/marker"); err == nil {
		t.Error("a look-alike's cadre.conf was run")
	}
	t.Setenv("CADRE_PERSONA", "x")
	if code, _, errOut := call(); code != 1 || !strings.Contains(errOut, "persona sessions cannot start the orchestrator") {
		t.Errorf("in a persona: %d %q", code, errOut)
	}
}

func TestOrchestratorInTmux(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	out := must(t, "--tmux")
	if !strings.Contains(out, "started the orchestrator of work; attach with: cadre attach") {
		t.Fatalf("--tmux: %q", out)
	}
	if tmuxIn(socket, "show-options", "-qv", "-t", "=cadre-work:", "@cadre_role") != "orchestrator" {
		t.Error("the session is not marked as the orchestrator")
	}
	if out := must(t, "--tmux"); !strings.Contains(out, "already running") {
		t.Errorf("second --tmux: %q", out)
	}
	if out := must(t, "ls"); !strings.Contains(out, "orchestrator: running in tmux (cadre-work)") {
		t.Errorf("ls:\n%s", out)
	}
	// With one in tmux, plain cadre goes to it.
	if out := must(t); !strings.Contains(out, "the orchestrator of work is already running") {
		t.Errorf("plain cadre with one in tmux: %q", out)
	}
	refused(t, "needs a terminal", "attach")
	// stop leaves the orchestrator alone unless asked.
	must(t, "up", "dev/engineer")
	must(t, "stop", "--yes")
	if tmuxIn(socket, "has-session", "-t", "=cadre-work") != "" {
		t.Error("stop stopped the orchestrator")
	}
	must(t, "stop", "--all", "--yes")
	if tmuxIn(socket, "has-session", "-t", "=cadre-work") != "" {
		t.Error("stop --all stopped the orchestrator")
	}
}

// The orchestrator runs with the cadre's PERMISSION_MODE, and a mode the
// runtime lacks is refused before it starts.
func TestOrchestratorPermissionMode(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadre/work/cadre.conf", []byte("PERMISSION_MODE=yolo\n"), 0o644)
	if code, _, errOut := call(); code != 1 || !strings.Contains(errOut, "has no permission mode yolo") {
		t.Errorf("yolo: %d %q", code, errOut)
	}
	os.WriteFile(home+"/.cadre/work/cadre.conf", []byte("PERMISSION_MODE=plan\n"), 0o644)
	must(t)
	if ran := readFile(t, home+"/orch-ran"); !strings.Contains(ran, "--permission-mode\nplan\n") {
		t.Errorf("the orchestrator ran without the cadre's mode:\n%s", ran)
	}
}

// plantLock writes a lock as a persona's shell could: a live pid with its
// real start time, pointing at a session of the planter's choosing.
func plantLock(t *testing.T, path string, pid int, session string) {
	t.Helper()
	info, err := proc.Of(pid)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"pid": pid, "start": info.Start, "command": info.Command, "mode": "tmux", "session": session, "since": time.Now()})
	os.WriteFile(path, raw, 0o600)
}

func TestATmuxLockIsCheckedBeforeAttaching(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	c := home + "/.cadre/work"
	lock := c + "/.claude/build/orchestrator.lock"
	os.MkdirAll(c+"/.claude/build", 0o755)
	pane := func(target string) int {
		pid, _ := strconv.Atoi(tmuxIn(socket, "display-message", "-p", "-t", target, "#{pane_pid}"))
		return pid
	}
	// Another session, a session named like the orchestrator but not
	// marked as one, and the real orchestrator session with another
	// process's pid: none is attached to.
	tmuxIn(socket, "new-session", "-d", "-s", "evil", "-n", "orchestrator", "sleep", "60")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-work", "-n", "orchestrator", "sleep", "60")
	for _, plant := range []struct{ name, session string }{{"another session", "evil"}, {"an unmarked session", "cadre-work"}} {
		plantLock(t, lock, pane("="+plant.session+":"), plant.session)
		out := must(t)
		if strings.Contains(out, "already running") {
			t.Errorf("%s was taken for the orchestrator: %q", plant.name, out)
		}
	}
	tmuxIn(socket, "set-option", "-t", "=cadre-work:", "@cadre_role", "orchestrator")
	tmuxIn(socket, "set-option", "-t", "=cadre-work:", "@cadre_home", c)
	// The marked session with another process's pid: the lock is not
	// trusted, and removed (cadre then finds the session itself).
	plantLock(t, lock, pane("=evil:"), "cadre-work")
	must(t)
	if _, err := os.Stat(lock); err == nil {
		t.Error("a lock naming another process was left")
	}
}

func TestAnOrchestratorInTmuxWithoutItsLockStillCounts(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	must(t, "init", "work")
	must(t, "--tmux")
	os.Remove(home + "/.cadre/work/.claude/build/orchestrator.lock")
	if out := must(t); !strings.Contains(out, "the orchestrator of work is already running") {
		t.Errorf("a second orchestrator started: %q", out)
	}
	if _, err := os.Stat(home + "/orch-ran"); err != nil {
		t.Fatal("the tmux orchestrator did not run")
	}
}

func TestTwoRunsStartOneOrchestrator(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	os.WriteFile(home+"/bin/claude", []byte("#!/bin/sh\necho x >> \""+home+"/starts\"\n"+
		"i=0; while [ ! -e \""+home+"/release\" ] && [ $i -lt 100 ]; do sleep 0.05; i=$((i+1)); done\n"), 0o755)
	var wg sync.WaitGroup
	codes := make([]int, 4)
	for i := range codes {
		wg.Add(1)
		go func() { defer wg.Done(); codes[i], _, _ = call() }()
	}
	time.Sleep(1500 * time.Millisecond)
	os.WriteFile(home+"/release", nil, 0o644)
	wg.Wait()
	if n := strings.Count(readFile(t, home+"/starts"), "x"); n != 1 {
		t.Errorf("%d orchestrators started (exit codes %v)", n, codes)
	}
	if _, err := os.Stat(home + "/.cadre/work/.claude/build/orchestrator.lock"); err == nil {
		t.Error("the lock outlived the orchestrator")
	}
}

// cadre waits through Ctrl-C, but the orchestrator gets the default
// disposition: an ignored signal would stay ignored across exec.
func TestTheOrchestratorKeepsDefaultSignals(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	os.WriteFile(home+"/bin/claude", []byte("#!/bin/sh\nkill -INT $$\nsleep 0.2\necho survived > \""+home+"/survived\"\n"), 0o755)
	call()
	if _, err := os.Stat(home + "/survived"); err == nil {
		t.Error("SIGINT was ignored in the orchestrator")
	}
}
