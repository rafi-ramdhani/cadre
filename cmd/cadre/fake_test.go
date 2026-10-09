//go:build !windows && cadretest

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/runtime/fake"
)

// fakeCadre makes a cadre whose sessions run on the fake runtime (a test
// build's CADRE_TEST_RUNTIME), and a stub for the fake to launch that
// records its arguments and environment.
func fakeCadre(t *testing.T) string {
	t.Helper()
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	t.Setenv("CADRE_TEST_RUNTIME", "fake")
	stub := filepath.Join(home, "fake-agent")
	os.WriteFile(stub, []byte("#!/bin/sh\n{ printf '%s\\n' \"$@\"; env; } > \""+home+"/fake-ran\"\nexec sleep 300\n"), 0o755)
	t.Setenv("CADRE_FAKE_BIN", stub)
	t.Cleanup(func() { fake.Off = map[string]bool{} })
	return home
}

// What runs in tmux is what the runtime's Launch built.
func TestUpRunsWhatLaunchBuilt(t *testing.T) {
	home := fakeCadre(t)
	out := must(t, "up", "dev/engineer")
	if !strings.Contains(out, "work-dev-engineer started") {
		t.Fatalf("up: %q", out)
	}
	var ran string
	for i := 0; i < 100 && !strings.Contains(ran, "CADRE_FAKE_LAUNCHED"); i++ {
		time.Sleep(50 * time.Millisecond)
		b, _ := os.ReadFile(home + "/fake-ran")
		ran = string(b)
	}
	for _, want := range []string{"--fake-name\nwork-dev-engineer\n", "--fake-mode\ndefault\n",
		"--fake-prompt\n" + home + "/.cadre/work/.fake/build/dev-engineer.md\n", "--fake-grants\nfake-grants\n",
		"CADRE_FAKE_LAUNCHED=work-dev-engineer", "CADRE_HOME=" + home + "/.cadre/work", "CADRE_MEMBER=work-dev-engineer"} {
		if !strings.Contains(ran, want) {
			t.Errorf("the fake ran without %q:\n%s", want, ran)
		}
	}
	if _, err := os.Stat(home + "/.cadre/work/.claude/member-settings.json"); err == nil {
		t.Error("a member on the fake runtime made Claude Code's settings file")
	}
}

// A runtime that cannot enforce the fixed denies, has no messaging, or
// lacks the permission mode is refused, with a message.
func TestUpRefusesAnUnfitRuntime(t *testing.T) {
	fakeCadre(t)
	for _, tc := range []struct{ off, want string }{
		{"FixedDenies", "cannot enforce cadre's fixed denies"},
		{"Messaging", "has no way to message the orchestrator"},
	} {
		fake.Off = map[string]bool{tc.off: true}
		code, out, _ := call("up", "dev/engineer")
		if code == 0 || !strings.Contains(out, "work-dev-engineer not started: runtime fake "+tc.want) {
			t.Errorf("%s off: %d %q", tc.off, code, out)
		}
	}
	fake.Off = map[string]bool{}
	t.Setenv("CADRE_PERMISSION_MODE", "auto")
	if code, out, _ := call("up", "dev/engineer"); code == 0 || !strings.Contains(out, "runtime fake has no permission mode auto") {
		t.Errorf("an unknown mode: %d %q", code, out)
	}
	// Capabilities can also come from the environment, for the smoke test.
	t.Setenv("CADRE_PERMISSION_MODE", "")
	t.Setenv("CADRE_FAKE_OFF", "Messaging")
	if code, out, _ := call("up", "dev/engineer"); code == 0 || !strings.Contains(out, "has no way to message") {
		t.Errorf("CADRE_FAKE_OFF: %d %q", code, out)
	}
}

// ls says what keeps every member from starting.
func TestLsListsWhatKeepsMembersFromStarting(t *testing.T) {
	fakeCadre(t)
	var st cadreStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Problems) != 0 {
		t.Errorf("problems with a fit runtime: %v", st.Problems)
	}
	fake.Off = map[string]bool{"Messaging": true}
	if out := must(t, "ls"); !strings.Contains(out, "problem: runtime fake has no way to message the orchestrator") {
		t.Errorf("ls with messaging off:\n%s", out)
	}
}

// A member resumes its last conversation on the next start, and starts a
// new one, saying why, when asked or when it cannot resume.
func TestUpResumesConversations(t *testing.T) {
	home := fakeCadre(t)
	ran := func() string {
		for i := 0; i < 100; i++ {
			if b, _ := os.ReadFile(home + "/fake-ran"); len(b) > 0 {
				return string(b)
			}
			time.Sleep(50 * time.Millisecond)
		}
		return ""
	}
	restart := func(args ...string) string {
		call("stop", "dev/engineer")
		os.Remove(home + "/fake-ran")
		return must(t, append([]string{"up", "dev/engineer"}, args...)...)
	}
	out := must(t, "up", "dev/engineer")
	if !strings.Contains(out, "(a new conversation)") || !strings.Contains(ran(), "--fake-session\nfake-session\n") {
		t.Errorf("first start: %q", out)
	}
	record := home + "/.cadre/work/.fake/build/sessions/work-dev-engineer.json"
	if !strings.Contains(readFile(t, record), `"id":"fake-session"`) {
		t.Errorf("record %q", readFile(t, record))
	}
	if out := restart(); !strings.Contains(out, "(resumed its conversation)") || !strings.Contains(ran(), "--fake-resume\nfake-session\n") {
		t.Errorf("second start: %q", out)
	}
	t.Setenv("CADRE_FAKE_GONE", "fake-session")
	if out := restart(); !strings.Contains(out, "(a new conversation: the last one is gone)") {
		t.Errorf("a lost conversation: %q", out)
	}
	t.Setenv("CADRE_FAKE_GONE", "")
	if out := restart("--fresh"); !strings.Contains(out, "(a new conversation, as asked)") {
		t.Errorf("--fresh: %q", out)
	}
	call("stop", "dev/engineer", "--fresh")
	if _, err := os.Stat(record); err == nil {
		t.Error("stop --fresh kept the record")
	}
	os.Remove(home + "/fake-ran")
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "(a new conversation)") {
		t.Errorf("after stop --fresh: %q", out)
	}
}

// The orchestrator resumes too; when the runtime will not resume, a new
// conversation starts at once.
func TestTheOrchestratorResumes(t *testing.T) {
	home := fakeCadre(t)
	// It exits at once: with an error when asked to resume, as a runtime
	// that lost the conversation does.
	os.WriteFile(home+"/fake-agent", []byte("#!/bin/sh\necho \"$@\" >> \""+home+"/orch-runs\"\ncase \"$*\" in *'--fake-resume fake-session'*) exit 1 ;; esac\nexit 0\n"), 0o755)
	out := must(t)
	if !strings.Contains(out, "The orchestrator: a new conversation.") {
		t.Errorf("first: %q", out)
	}
	out = must(t)
	if !strings.Contains(out, "The orchestrator: resumed its conversation.") || !strings.Contains(out, "The orchestrator: a new conversation: resuming failed.") {
		t.Errorf("second: %q", out)
	}
	if runs := readFile(t, home+"/orch-runs"); strings.Count(runs, "--fake-name work-orchestrator") != 3 {
		t.Errorf("runs:\n%s", runs)
	}
}
