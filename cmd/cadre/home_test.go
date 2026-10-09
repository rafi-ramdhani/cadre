//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/testguard"
)

// sandbox gives a test its own HOME, git identity and working folder.
func sandbox(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// Nothing may point cadre or Claude Code at the user's own setup: a
	// set CLAUDE_CONFIG_DIR would send trust edits to the real config.
	for _, v := range []string{"CADRE_HOME", "CADRE_MEMBER", "CADRE_PERSONA", "CADRE_TEST_TTY", "XDG_CACHE_HOME", "CLAUDE_CONFIG_DIR", "TMUX", "TMUX_PANE"} {
		t.Setenv(v, "")
		os.Unsetenv(v)
	}
	cfg := filepath.Join(home, ".gitconfig-test")
	os.WriteFile(cfg, []byte("[user]\n\tname = T\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(home)
	// Every test gets a private tmux server, so no test can ever see or
	// stop the user's own sessions.
	socket := fmt.Sprintf("cadre-gotest-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Setenv("CADRE_TMUX_SOCKET", socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })
	// A machine set up and checked: the skill linked, this version's full
	// health check done. Tests of the first run and the health check undo
	// this.
	os.MkdirAll(home+"/.claude/skills", 0o755)
	os.Symlink(home+"/.cadre/framework/skills/cadre", home+"/.claude/skills/cadre")
	cadres.SetState(cadres.CheckedVersion, version)
	testguard.Check(t)
	return home
}

// must runs cadre and fails the test unless it exits 0.
func must(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := call(args...)
	if code != 0 {
		t.Fatalf("cadre %v: exit %d\n%s%s", args, code, out, errOut)
	}
	return out
}

func refused(t *testing.T, want string, args ...string) {
	t.Helper()
	code, out, errOut := call(args...)
	if code == 0 || !strings.Contains(errOut, want) {
		t.Errorf("cadre %v: exit %d, %q %q; want a refusal with %q", args, code, out, errOut, want)
	}
}

func TestInitAndUse(t *testing.T) {
	home := sandbox(t)
	out := must(t, "init", "work")
	if !strings.Contains(out, "created "+home+"/.cadre/work") || !strings.Contains(out, "work is the default cadre") {
		t.Errorf("init work: %q", out)
	}
	if b, _ := os.ReadFile(home + "/.cadre/work/README.md"); !strings.HasPrefix(string(b), "# work\n") {
		t.Error("the template was not written from the embedded assets")
	}
	// The starter team: dev, with an engineer and a reviewer.
	roles, _ := filepath.Glob(home + "/.cadre/work/members/*/*.md")
	if len(roles) != 2 || filepath.Base(roles[0]) != "engineer.md" || filepath.Base(roles[1]) != "reviewer.md" {
		t.Errorf("starter team %v", roles)
	}
	out = must(t, "init", "life")
	if !strings.Contains(out, "default cadre stays work") {
		t.Errorf("init life: %q", out)
	}
	refused(t, "reserved", "init", "framework")
	refused(t, "already exists", "init", "work")
	must(t, "use", "life")
	if b, _ := os.ReadFile(home + "/.cadre/config/default"); strings.TrimSpace(string(b)) != "life" {
		t.Errorf("default %q", b)
	}
	refused(t, "no cadre named nope", "use", "nope")
	t.Setenv("CADRE_MEMBER", "x")
	for _, args := range [][]string{{"init", "x"}, {"use", "work"}} {
		refused(t, "refused for members: members cannot create or switch cadres", args...)
	}
}

// Cadres live only in ~/.cadre: the 0.1.x forms with a folder are gone.
func TestInitAndUseTakeOnlyAName(t *testing.T) {
	home := sandbox(t)
	parent := filepath.Join(home, "Documents")
	os.MkdirAll(parent, 0o755)
	refused(t, "usage: cadre init <name>", "init", "visible", parent)
	if _, err := os.Stat(parent + "/visible"); err == nil {
		t.Error("a cadre was made outside ~/.cadre")
	}
	os.MkdirAll(home+"/elsewhere/other/members", 0o755)
	refused(t, "no cadre named", "use", home+"/elsewhere/other")
}

func TestProjectPath(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.MkdirAll(home+"/Developer/app", 0o755)
	register(t, home, "work", "app:\n  repo: me/app\n  team: dev\n  about: the app\n  path: ~/Developer/app\ngone:\n  repo: me/gone\n  team: dev\n  about: missing\n  path: ~/Developer/gone\n")
	if out := must(t, "project", "path", "app"); strings.TrimSpace(out) != home+"/Developer/app" {
		t.Errorf("project path: %q", out)
	}
	refused(t, "is missing: ~/Developer/gone is gone", "project", "path", "gone")
	refused(t, "neither a registry project nor a folder", "project", "path", "nope")
	// A project of the cadre resolves the cadre from inside it.
	t.Chdir(home + "/Developer/app")
	if out := must(t, "project", "path", "app"); strings.TrimSpace(out) != home+"/Developer/app" {
		t.Errorf("from inside the project: %q", out)
	}
}

func TestOldConfigIsMovedOnce(t *testing.T) {
	home := sandbox(t)
	os.MkdirAll(home+"/Documents/demo/personas", 0o755)
	os.MkdirAll(home+"/.config/cadre", 0o755)
	os.WriteFile(home+"/.config/cadre/home", []byte(home+"/Documents/demo\n"), 0o644)
	code, _, errOut := call("ls", "--all")
	if code != 0 || !strings.Contains(errOut, "moved cadre's settings") {
		t.Errorf("first command: %d %q", code, errOut)
	}
	if _, err := os.Stat(home + "/.config/cadre"); err == nil {
		t.Error("~/.config/cadre is still there")
	}
	if _, err := os.Stat(home + "/.config/cadre.moved-to-0.2.0/home"); err != nil {
		t.Error("the old folder was not kept aside")
	}
	_, _, errOut = call("ls", "--all")
	if strings.Contains(errOut, "moved") {
		t.Error("the move was announced twice")
	}
}

func TestCutConfigKeysAreNamed(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadre/work/cadre.conf", []byte("PERMISSION_MODE=default\nRUNTIME=codex\nORCHESTRATOR_TMUX=yes\n"), 0o644)
	_, _, errOut := call("ls")
	for _, want := range []string{"cadre.conf sets RUNTIME, which cadre no longer reads: there is one runtime",
		"cadre.conf sets ORCHESTRATOR_TMUX, which cadre no longer reads: open the orchestrator in tmux with cadre --tmux"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("no %q in %q", want, errOut)
		}
	}
}

// A session started by 0.1.x carries CADRE_PERSONA: it is read exactly
// like CADRE_MEMBER, and never set.
func TestTheOldMemberVariableStillCounts(t *testing.T) {
	sandbox(t)
	must(t, "init", "work")
	t.Setenv("CADRE_PERSONA", "work-dev-engineer")
	refused(t, "refused for members", "allow", "add", "Bash(true)")
	refused(t, "refused for members", "init", "other")
	refused(t, "refused for members", "stop", "--all", "--yes")
	if code, out, _ := call("hook", "orchestrator"); code != 0 || out != "" {
		t.Errorf("the hook spoke in a 0.1.x member session: %q", out)
	}
	if env := withoutMember([]string{"A=1", "CADRE_PERSONA=x", "CADRE_MEMBER=y"}); len(env) != 1 || env[0] != "A=1" {
		t.Errorf("the orchestrator's environment keeps %v", env)
	}
}
