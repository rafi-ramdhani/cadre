//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bareRepo makes a local repository with one commit and returns its path,
// to clone from without a network.
func bareRepo(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), name+"-src")
	bare := filepath.Join(t.TempDir(), name+".git")
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	os.MkdirAll(src, 0o755)
	run(src, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(src, "README"), []byte(name), 0o644)
	run(src, "add", "-A")
	run(src, "commit", "-qm", "first")
	if out, err := exec.Command("git", "clone", "-q", "--bare", src, bare).CombinedOutput(); err != nil {
		t.Fatalf("bare clone: %v %s", err, out)
	}
	return bare
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, _ := os.ReadFile(p)
	return string(b)
}

func TestProjectsFolderIsAskedOnce(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	repo := bareRepo(t, "app")
	// Without a terminal: refused, with the suggestion.
	refused(t, "the projects folder is not set; ask the user and run cadre project dir <folder> (suggested: ~/Developer)", "project", "add", "app", repo)
	if strings.Contains(readFile(t, home+"/.cadre/work/projects.yaml"), "app") {
		t.Error("a refused add was registered")
	}
	// On a terminal: asked, answered with the default, remembered.
	t.Setenv("CADRE_TEST_TTY", "1")
	code, out, errOut := callIn("\n", "project", "add", "app", repo)
	if code != 0 || !strings.Contains(errOut, "Where do you keep your projects? [~/Developer]") {
		t.Fatalf("add on a terminal: %d %q %q", code, out, errOut)
	}
	if !strings.Contains(out, "app added, cloned to "+home+"/Developer/app") {
		t.Errorf("add: %q", out)
	}
	if got := strings.TrimSpace(readFile(t, home+"/.cadre/config/projects-dir")); got != "~/Developer" {
		t.Errorf("projects-dir %q", got)
	}
	reg := readFile(t, home+"/.cadre/work/projects.yaml")
	if !strings.Contains(reg, "app:\n  repo: "+repo+"\n  team: dev\n  about:\n  path: ~/Developer/app\n") {
		t.Errorf("registry:\n%s", reg)
	}
	if _, err := os.Stat(home + "/.cadre/work/projects"); err == nil {
		t.Error("the add made projects/ inside the cadre")
	}
	t.Setenv("CADRE_TEST_TTY", "")
	if out := must(t, "project", "dir"); strings.TrimSpace(out) != "~/Developer" {
		t.Errorf("project dir: %q", out)
	}
}

func TestProjectAddTrustsAndLinks(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	cfg := home + "/.claude.json"
	os.WriteFile(cfg, []byte("{\n  \"numStartups\": 1\n}\n"), 0o600)
	repo := bareRepo(t, "app")
	out := must(t, "project", "add", "app", repo, "dev", "the app")
	if !strings.Contains(out, "app added, cloned to "+home+"/Code/app and trusted in Claude Code") {
		t.Errorf("add: %q", out)
	}
	if !strings.Contains(readFile(t, cfg), `"`+home+`/Code/app": {`) || readFile(t, cfg+".bak-cadre") != "{\n  \"numStartups\": 1\n}\n" {
		t.Errorf("config %s", readFile(t, cfg))
	}
	refused(t, "already in projects.yaml", "project", "add", "app", repo)
	refused(t, "letters, digits", "project", "add", "../x", repo)
	// An existing folder that is the same repository is linked, another is refused.
	os.MkdirAll(home+"/Code/other", 0o755)
	refused(t, "is not a clone of", "project", "add", "other", repo)

	// Linking a folder the user already has.
	mine := home + "/src/mine"
	exec.Command("git", "clone", "-q", repo, mine).Run()
	out = must(t, "project", "add", "mine", "--path", mine, "research", "--no-trust")
	if !strings.Contains(out, "mine added, linked at "+mine) {
		t.Errorf("link: %q", out)
	}
	if reg := readFile(t, home+"/.cadre/work/projects.yaml"); !strings.Contains(reg, "mine:\n  repo: "+repo+"\n  team: research\n  about:\n  path: ~/src/mine\n") {
		t.Errorf("registry:\n%s", reg)
	}
	refused(t, "already linked as mine", "project", "add", "again", "--path", mine)
	refused(t, "not the top folder of a git repository", "project", "add", "sub", "--path", home+"/src")
	refused(t, "inside ~/.cadre", "project", "add", "x", "--path", home+"/.cadre/work")
	refused(t, "your home folder", "project", "add", "x", "--path", home)
	t.Setenv("CADRE_PERSONA", "x")
	refused(t, "persona sessions cannot link folders", "project", "add", "y", "--path", mine)
	refused(t, "persona sessions cannot trust", "project", "trust", "app")
}

func TestProjectSyncAndTrust(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "project", "dir", "~/Code")
	os.WriteFile(home+"/.claude.json", []byte("{}\n"), 0o600)
	repo := bareRepo(t, "app")
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte("app:\n  repo: "+repo+"\n  team: dev\n  about: a\nlocal:\n  team: dev\n  about: no repo\n  path: ~/nowhere/local\n"), 0o644)
	out := must(t, "project", "sync")
	for _, want := range []string{"app: cloned to " + home + "/Code/app", "local: missing, and no repo to clone", "app: trusted in Claude Code"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync lacks %q:\n%s", want, out)
		}
	}
	if out := must(t, "project", "sync"); !strings.Contains(out, "app: present") {
		t.Errorf("second sync: %q", out)
	}
	out = must(t, "project", "trust", "--all")
	if !strings.Contains(out, "app: already trusted") || !strings.Contains(out, "local: missing locally, skipped") {
		t.Errorf("trust --all: %q", out)
	}
	refused(t, "not a registered project", "project", "trust", "nope")
}

func TestTrustRefusals(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.WriteFile(home+"/.claude.json", []byte("{}\n"), 0o600)
	os.WriteFile(home+"/.cadre/work/projects.yaml", []byte(
		"me:\n  path: ~\nteam:\n  path: ~/.cadre/work/teams/dev\nnotgit:\n  path: ~/plain\n"), 0o644)
	os.MkdirAll(home+"/.cadre/work/teams/dev", 0o755)
	os.MkdirAll(home+"/plain", 0o755)
	out := must(t, "project", "trust", "--all")
	for _, want := range []string{"me: not trusted, it is your home folder", "team: not trusted, it is inside ~/.cadre", "notgit: not trusted, it is not the top folder of a git repo"} {
		if !strings.Contains(out, want) {
			t.Errorf("trust --all lacks %q:\n%s", want, out)
		}
	}
	if readFile(t, home+"/.claude.json") != "{}\n" {
		t.Error("a refused trust changed the config")
	}
}
