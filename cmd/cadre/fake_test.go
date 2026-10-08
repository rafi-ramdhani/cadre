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

// fakeCadre makes a cadre whose dev/engineer runs on the fake runtime, and
// a stub for the fake to launch that records its arguments and environment.
func fakeCadre(t *testing.T) string {
	t.Helper()
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadre/work/personas/dev/engineer.runtime", []byte("fake\n"), 0o644)
	stub := filepath.Join(home, "fake-agent")
	os.WriteFile(stub, []byte("#!/bin/sh\n{ printf '%s\\n' \"$@\"; env; } > \""+home+"/fake-ran\"\nexec sleep 300\n"), 0o755)
	t.Setenv("CADRE_FAKE_BIN", stub)
	t.Cleanup(func() { fake.Off = map[string]bool{} })
	return home
}

// AC-P6: what runs in tmux is what the runtime's Launch built.
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
		"CADRE_FAKE_LAUNCHED=work-dev-engineer", "CADRE_HOME=" + home + "/.cadre/work", "CADRE_PERSONA=work-dev-engineer"} {
		if !strings.Contains(ran, want) {
			t.Errorf("the fake ran without %q:\n%s", want, ran)
		}
	}
	if _, err := os.Stat(home + "/.cadre/work/.claude/persona-settings.json"); err == nil {
		t.Error("a persona on the fake runtime made Claude Code's settings file")
	}
}

// AC-P5: a runtime that cannot enforce the fixed denies, has no messaging,
// or lacks the permission mode is refused, with a message.
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

// AC-P3: ls --json shows each persona's runtime.
func TestLsShowsEachPersonasRuntime(t *testing.T) {
	fakeCadre(t)
	var st cadreStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range st.Teams["dev"] {
		got[p.Role] = p.Runtime
	}
	if got["engineer"] != "fake" || got["pm"] != "claude" {
		t.Errorf("runtimes %v", got)
	}
}
