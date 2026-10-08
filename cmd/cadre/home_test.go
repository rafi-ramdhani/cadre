//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox gives a test its own HOME, git identity and working folder.
func sandbox(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	for _, v := range []string{"CADRE_HOME", "CADRE_PERSONA", "CADRE_TEST_TTY", "XDG_CACHE_HOME"} {
		t.Setenv(v, "")
	}
	cfg := filepath.Join(home, ".gitconfig-test")
	os.WriteFile(cfg, []byte("[user]\n\tname = T\n\temail = t@example.com\n[init]\n\tdefaultBranch = main\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Chdir(home)
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
	if b, _ := os.ReadFile(home + "/.cadre/work/playbook.md"); !strings.Contains(string(b), "work") {
		t.Error("the template was not written from the embedded assets")
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
	t.Setenv("CADRE_PERSONA", "x")
	for _, args := range [][]string{{"init", "x"}, {"use", "work"}, {"cadres", "add", "/x"}, {"cadres", "remove", "x"}} {
		refused(t, "persona sessions cannot register or switch cadres", args...)
	}
}

func TestInitOutsideAndOldNames(t *testing.T) {
	home := sandbox(t)
	parent := filepath.Join(home, "Documents")
	os.MkdirAll(parent, 0o755)
	must(t, "init", "visible", parent)
	if _, err := os.Stat(parent + "/visible/projects"); err != nil {
		t.Error("the 0.1.x form made no projects/")
	}
	if b, _ := os.ReadFile(home + "/.cadre/config/external"); strings.TrimSpace(string(b)) != parent+"/visible" {
		t.Errorf("external %q", b)
	}
	// 0.1.x's use <dir> still works, for a cadre anywhere.
	os.MkdirAll(home+"/elsewhere/other/personas", 0o755)
	out := must(t, "use", home+"/elsewhere/other")
	if !strings.Contains(out, "default cadre: other") {
		t.Errorf("use <dir>: %q", out)
	}
}

func TestCadresAddRemove(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.MkdirAll(home+"/x/side/personas", 0o755)
	os.MkdirAll(home+"/x/WORK/personas", 0o755)
	must(t, "cadres", "add", home+"/x/side")
	refused(t, "already at", "cadres", "add", home+"/x/WORK")
	refused(t, "not a cadre", "cadres", "add", home+"/x")
	refused(t, "found without adding it", "cadres", "add", home+"/.cadre/work")
	refused(t, "lives in ~/.cadre", "cadres", "remove", "work")
	must(t, "use", "side")
	refused(t, "is the default cadre", "cadres", "remove", "side")
	must(t, "use", "work")
	out := must(t, "cadres", "remove", "side")
	if !strings.Contains(out, "its folder is untouched") {
		t.Errorf("remove: %q", out)
	}
	if _, err := os.Stat(home + "/x/side/personas"); err != nil {
		t.Error("the folder was touched")
	}
}

func TestTeamPersonaAndProjects(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "team", "add", "ops")
	refused(t, "letters, digits", "team", "add", "../x")
	out := must(t, "persona", "add", "ops/sre")
	f := home + "/.cadre/work/personas/ops/sre.md"
	if !strings.Contains(out, "created "+f) {
		t.Errorf("persona add: %q", out)
	}
	if log, _ := exec.Command("git", "-C", home+"/.cadre/work", "log", "-1", "--format=%s").Output(); strings.TrimSpace(string(log)) != "Add persona ops/sre" {
		t.Errorf("commit %q", log)
	}
	refused(t, "already exists", "persona", "add", "ops/sre")
	refused(t, "usage", "persona", "add", "ops")

	os.MkdirAll(home+"/Developer/app", 0o755)
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte("app:\n  repo: me/app\n  team: dev\n  about: the app\n  path: ~/Developer/app\ngone:\n  repo: me/gone\n  team: dev\n  about: missing\n  path: ~/Developer/gone\n"), 0o644)
	if out := must(t, "project", "path", "app"); strings.TrimSpace(out) != home+"/Developer/app" {
		t.Errorf("project path: %q", out)
	}
	if out := must(t, "path", "app"); strings.TrimSpace(out) != home+"/Developer/app" {
		t.Errorf("old name path: %q", out)
	}
	refused(t, "is not at", "project", "path", "gone")
	refused(t, "neither a registry project nor a folder", "project", "path", "nope")
	out = must(t, "projects")
	if !strings.Contains(out, "  app            dev      the app") || !strings.Contains(out, "? gone") {
		t.Errorf("projects: %q", out)
	}
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
	code, _, errOut := call("projects")
	if code != 0 || !strings.Contains(errOut, "moved cadre's settings") {
		t.Errorf("first command: %d %q", code, errOut)
	}
	if _, err := os.Stat(home + "/.config/cadre"); err == nil {
		t.Error("~/.config/cadre is still there")
	}
	_, _, errOut = call("projects")
	if strings.Contains(errOut, "moved") {
		t.Error("the move was announced twice")
	}
}
