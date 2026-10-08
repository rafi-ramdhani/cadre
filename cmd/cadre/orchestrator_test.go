//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// stubClaude replaces the test's stub with one that records its
// arguments, environment and folder, then waits for a release file.
func stubClaude(t *testing.T, home string) {
	t.Helper()
	os.WriteFile(home+"/bin/claude", []byte("#!/bin/sh\n{ printf '%s\\n' \"$@\"; env; pwd; } > \""+home+"/orch-ran\"\n"+
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
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte("app:\n  repo: me/app\n  path: ~/Developer/app\n"), 0o644)
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
	if out := must(t); !strings.Contains(out, home+"/old/demo looks like a cadre that cadre does not know") {
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
	// With one in tmux, plain cadre goes to it (M.3).
	if out := must(t, "--no-tmux"); !strings.Contains(out, "the orchestrator of work is already running") {
		t.Errorf("--no-tmux with one in tmux: %q", out)
	}
	refused(t, "needs a terminal", "attach")
	// stop leaves the orchestrator alone unless asked.
	must(t, "up", "dev/engineer")
	must(t, "stop", "--yes")
	if tmuxIn(socket, "has-session", "-t", "=cadre-work") != "" {
		t.Error("stop stopped the orchestrator")
	}
	out = must(t, "stop", "--with-orchestrator", "--yes")
	if !strings.Contains(out, "cadre-work stopped") {
		t.Errorf("stop --with-orchestrator: %q", out)
	}
	refused(t, "not running in tmux", "attach")
}

func TestOrchestratorRuntimeRules(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	for conf, want := range map[string]string{"ORCHESTRATOR_RUNTIME=codex\n": "runtime codex is not supported yet",
		"ORCHESTRATOR_PERMISSION_MODE=yolo\n": "has no permission mode yolo"} {
		os.WriteFile(home+"/.cadre/work/cadre.conf", []byte(conf), 0o644)
		if code, _, errOut := call(); code != 1 || !strings.Contains(errOut, want) {
			t.Errorf("%s: %d %q", conf, code, errOut)
		}
	}
	os.WriteFile(home+"/.cadre/work/cadre.conf", []byte("ORCHESTRATOR_TMUX=yes\n"), 0o644)
	if out := must(t); !strings.Contains(out, "started the orchestrator of work") {
		t.Errorf("ORCHESTRATOR_TMUX=yes: %q", out)
	}
}
