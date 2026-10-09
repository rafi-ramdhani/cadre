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

	"github.com/rafi-ramdhani/cadrei/internal/cadreis"
	"github.com/rafi-ramdhani/cadrei/internal/testguard"
)

// sandbox gives a test its own HOME, git identity and working folder.
func sandbox(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// Test binaries live in temporary folders; the hooks may name them.
	hookAnywhereForTests = true
	t.Cleanup(func() { hookAnywhereForTests = false })
	// Nothing may point cadrei or Claude Code at the user's own setup: a
	// set CLAUDE_CONFIG_DIR would send trust edits to the real config.
	for _, v := range []string{"CADREI_HOME", "CADREI_MEMBER", "CADRE_PERSONA", "CADREI_TEST_TTY", "XDG_CACHE_HOME", "CLAUDE_CONFIG_DIR", "TMUX", "TMUX_PANE"} {
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
	socket := fmt.Sprintf("cadrei-gotest-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Setenv("CADREI_TMUX_SOCKET", socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })
	// A machine set up and checked: the skill linked, this version's full
	// health check done. Tests of the first run and the health check undo
	// this.
	os.MkdirAll(home+"/.claude/skills", 0o755)
	os.Symlink(home+"/.cadrei/framework/skills/cadrei", home+"/.claude/skills/cadrei")
	cadreis.SetState(cadreis.CheckedVersion, version)
	testguard.Check(t)
	return home
}

// must runs cadrei and fails the test unless it exits 0.
func must(t *testing.T, args ...string) string {
	t.Helper()
	code, out, errOut := call(args...)
	if code != 0 {
		t.Fatalf("cadrei %v: exit %d\n%s%s", args, code, out, errOut)
	}
	return out
}

func refused(t *testing.T, want string, args ...string) {
	t.Helper()
	code, out, errOut := call(args...)
	if code == 0 || !strings.Contains(errOut, want) {
		t.Errorf("cadrei %v: exit %d, %q %q; want a refusal with %q", args, code, out, errOut, want)
	}
}

func TestInitAndUse(t *testing.T) {
	home := sandbox(t)
	out := must(t, "init", "work")
	if !strings.Contains(out, "created "+home+"/.cadrei/work") || !strings.Contains(out, "work is the default cadrei") {
		t.Errorf("init work: %q", out)
	}
	if b, _ := os.ReadFile(home + "/.cadrei/work/README.md"); !strings.HasPrefix(string(b), "# work\n") {
		t.Error("the template was not written from the embedded assets")
	}
	// The starter team: dev, with an engineer and a reviewer.
	roles, _ := filepath.Glob(home + "/.cadrei/work/members/*/*.md")
	if len(roles) != 2 || filepath.Base(roles[0]) != "engineer.md" || filepath.Base(roles[1]) != "reviewer.md" {
		t.Errorf("starter team %v", roles)
	}
	out = must(t, "init", "life")
	if !strings.Contains(out, "default cadrei stays work") {
		t.Errorf("init life: %q", out)
	}
	refused(t, "reserved", "init", "framework")
	refused(t, "already exists", "init", "work")
	must(t, "use", "life")
	if b, _ := os.ReadFile(home + "/.cadrei/config/default"); strings.TrimSpace(string(b)) != "life" {
		t.Errorf("default %q", b)
	}
	refused(t, "no cadrei named nope", "use", "nope")
	t.Setenv("CADREI_MEMBER", "x")
	for _, args := range [][]string{{"init", "x"}, {"use", "work"}} {
		refused(t, "refused for members: members cannot create or switch cadreis", args...)
	}
}

// Cadreis live only in ~/.cadrei: the 0.1.x forms with a folder are gone.
func TestInitAndUseTakeOnlyAName(t *testing.T) {
	home := sandbox(t)
	parent := filepath.Join(home, "Documents")
	os.MkdirAll(parent, 0o755)
	refused(t, "usage: cadrei init <name>", "init", "visible", parent)
	if _, err := os.Stat(parent + "/visible"); err == nil {
		t.Error("a cadrei was made outside ~/.cadrei")
	}
	os.MkdirAll(home+"/elsewhere/other/members", 0o755)
	refused(t, "no cadrei named", "use", home+"/elsewhere/other")
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
	refused(t, "is not a registered project or a folder", "project", "path", "nope")
	// A project of the cadrei resolves the cadrei from inside it.
	t.Chdir(home + "/Developer/app")
	if out := must(t, "project", "path", "app"); strings.TrimSpace(out) != home+"/Developer/app" {
		t.Errorf("from inside the project: %q", out)
	}
}

// snapshot is every file under dir with its content, to compare.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			raw, _ := os.ReadFile(p)
			fmt.Fprintf(&b, "%s %o %q\n", p, info.Mode(), raw)
		}
		return nil
	})
	return b.String()
}

// A machine with a 0.1.x cadre: cadrei changes neither it nor
// ~/.config/cadre, refuses to link or trust its top folder, accepts its
// projects, and points to the bring-in.
func TestA01CadreIsLeftAsItIs(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	old := home + "/Documents/demo"
	os.MkdirAll(old+"/personas/dev", 0o755)
	os.WriteFile(old+"/personas/dev/pm.md", []byte("# pm\n"), 0o644)
	os.WriteFile(old+"/projects.yaml", []byte("app:\n  repo: me/app\n"), 0o644)
	os.WriteFile(old+"/cadrei.conf", []byte("PERMISSION_MODE=auto\n"), 0o644)
	repo := bareRepo(t, "app")
	exec.Command("git", "init", "-q", old).Run()
	exec.Command("git", "clone", "-q", repo, old+"/projects/app").Run()
	os.MkdirAll(home+"/.config/cadre", 0o755)
	os.WriteFile(home+"/.config/cadre/home", []byte(old+"\n"), 0o644)
	os.WriteFile(home+"/.config/cadre/cadreis", []byte(old+"\n"), 0o644)
	before := snapshot(t, old) + snapshot(t, home+"/.config/cadre")
	// The first run points to the bring-in.
	ttyForTests = true
	code, out, errOut := callIn("new\nwork\nn\nn\n")
	ttyForTests = false
	if code != 0 || !strings.Contains(out, "You have a cadre from 0.1.x at ~/Documents/demo. To bring it in, tell the orchestrator: bring in my old cadre from ~/Documents/demo.") {
		t.Errorf("first run: %d\n%s%s", code, out, errOut)
	}
	if cadreis.Default() != "work" {
		t.Errorf("default %q", cadreis.Default())
	}
	for _, args := range [][]string{{"ls"}, {"ls", "--all"}, {"--check"}, {"project", "dir", "~/Code"}} {
		call(args...)
	}
	hint := "it is a cadre from 0.1.x; to bring it in, tell the orchestrator: bring in my old cadre from ~/Documents/demo"
	refused(t, hint, "project", "add", "demo", "--path", old)
	must(t, "project", "add", "app", "--path", old+"/projects/app", "--no-trust")
	refused(t, hint, "project", "link", "app", old)
	register(t, home, "work", "app:\n  repo: "+repo+"\n  path: "+old+"\n")
	if out := must(t, "project", "trust", "app"); !strings.Contains(out, "app: not trusted, "+hint) {
		t.Errorf("trust: %q", out)
	}
	if after := snapshot(t, old) + snapshot(t, home+"/.config/cadre"); after != before {
		t.Error("the 0.1.x cadre or ~/.config/cadre changed")
	}
	if code, _, errOut := call("migrate"); code != 1 || !strings.Contains(errOut, "unknown command 'migrate'") {
		t.Errorf("migrate: %d %q", code, errOut)
	}
}

// A 0.1.x ~/.config/cadre is never changed, and a command without a
// terminal creates nothing before asking.
func TestTheOldConfigIsLeftAlone(t *testing.T) {
	home := sandbox(t)
	os.RemoveAll(home + "/.cadrei") // the sandbox's own state, not cadrei's
	os.MkdirAll(home+"/Documents/demo/personas", 0o755)
	os.MkdirAll(home+"/.config/cadre", 0o755)
	os.WriteFile(home+"/.config/cadre/home", []byte(home+"/Documents/demo\n"), 0o644)
	os.WriteFile(home+"/.config/cadre/persona-settings.sha256", []byte("abc x\n"), 0o600)
	before, _ := exec.Command("ls", "-lnR", home+"/.config").Output()
	if code, _, _ := call(); code != 1 {
		t.Errorf("plain cadrei with no cadrei and no terminal exited %d", code)
	}
	if _, err := os.Stat(home + "/.cadrei"); err == nil {
		t.Error("~/.cadrei was created before asking")
	}
	for _, args := range [][]string{{"ls", "--all"}, {"--check"}, {"init", "work"}, {"ls"}} {
		call(args...)
	}
	after, _ := exec.Command("ls", "-lnR", home+"/.config").Output()
	if string(after) != string(before) || readFile(t, home+"/.config/cadre/home") != home+"/Documents/demo\n" {
		t.Errorf("~/.config/cadre changed:\n%s\n%s", before, after)
	}
}

func TestCutConfigKeysAreNamed(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadrei/work/cadrei.conf", []byte("PERMISSION_MODE=default\nRUNTIME=codex\nORCHESTRATOR_TMUX=yes\n"), 0o644)
	_, _, errOut := call("ls")
	for _, want := range []string{"cadrei.conf sets RUNTIME, which cadrei no longer reads: there is one runtime",
		"cadrei.conf sets ORCHESTRATOR_TMUX, which cadrei no longer reads: open the orchestrator in tmux with cadrei --tmux"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("no %q in %q", want, errOut)
		}
	}
}

// A session started by 0.1.x carries CADRE_PERSONA: it is read exactly
// like CADREI_MEMBER, and never set.
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
	if env := withoutMember([]string{"A=1", "CADRE_PERSONA=x", "CADREI_MEMBER=y"}); len(env) != 1 || env[0] != "A=1" {
		t.Errorf("the orchestrator's environment keeps %v", env)
	}
}
