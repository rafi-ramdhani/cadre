//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/backup"
	"github.com/rafi-ramdhani/cadre/internal/framework"
)

func TestUninstall(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	must(t, "--check")                  // the framework is written
	must(t, "up", "dev/engineer")       // a member runs
	must(t, "project", "dir", "~/Code") // config holds a projects folder
	c := home + "/.cadre/work"
	os.MkdirAll(home+"/Code/app/.git", 0o755)
	// The orchestrator hook runs this binary (through a link named cadre,
	// as an installed binary is); another hook and another cadre's hook
	// stay.
	os.MkdirAll(home+"/inst", 0o755)
	os.Symlink(framework.Binary(), home+"/inst/cadre")
	settings := home + "/.claude/settings.json"
	os.WriteFile(settings, []byte(`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "`+home+`/inst/cadre hook orchestrator"}]}, {"hooks": [{"type": "command", "command": "/other/cadre hook orchestrator"}]}], "PreToolUse": [{"hooks": [{"type": "command", "command": "mine"}]}]}}`), 0o644)
	if !backup.Installed(c) {
		t.Fatal("init did not install the pre-push check")
	}

	t.Setenv("CADRE_MEMBER", "work-dev-engineer")
	refused(t, "refused for members", "uninstall", "--yes")
	t.Setenv("CADRE_MEMBER", "")

	out := must(t, "uninstall", "--dry-run")
	for _, want := range []string{"stop 1 member sessions: cadre-work-dev", "remove the skill link ~/.claude/skills/cadre",
		"remove the orchestrator hook from ~/.claude/settings.json (other hooks stay)", "remove cadre's pre-push check from ~/.cadre/work",
		"remove ~/.cadre/config", "remove ~/.cadre/framework", "every cadre (work, in ~/.cadre) and every project",
		"orchestrator hooks that run another cadre", "Then: delete the cadre you built"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan lacks %q:\n%s", want, out)
		}
	}
	if code, _, errOut := call("uninstall"); code != 1 || !strings.Contains(errOut, "run with --yes") {
		t.Errorf("without a terminal: %d %q", code, errOut)
	}
	if _, err := os.Stat(framework.Dir()); err != nil || tmuxIn(socket, "has-session", "-t", "=cadre-work-dev") != "" {
		t.Fatal("the dry run or the refusal changed something")
	}

	out = must(t, "uninstall", "--yes")
	if !strings.Contains(out, "Uninstalled. Your cadres and projects are kept.") {
		t.Errorf("uninstall: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadre-work-dev") == "" {
		t.Error("the member still runs")
	}
	if _, err := os.Lstat(home + "/.claude/skills/cadre"); err == nil {
		t.Error("the skill link is still there")
	}
	s := readFile(t, settings)
	if strings.Contains(s, home+"/inst/cadre") || !strings.Contains(s, "/other/cadre hook orchestrator") || !strings.Contains(s, `"mine"`) {
		t.Errorf("settings after uninstall:\n%s", s)
	}
	if backup.Installed(c) {
		t.Error("the pre-push check is still there")
	}
	for _, d := range []string{home + "/.cadre/config", home + "/.cadre/framework"} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s is still there", d)
		}
	}
	if _, err := os.Stat(c + "/members/dev/engineer.md"); err != nil {
		t.Error("the cadre was touched")
	}
	if _, err := os.Stat(home + "/Code/app/.git"); err != nil {
		t.Error("a project was touched")
	}
	if log, _ := exec.Command("git", "-C", c, "status", "--porcelain").Output(); len(log) != 0 {
		t.Errorf("the cadre's repository changed: %s", log)
	}
}

// A skill link that points at another cadre is kept.
func TestUninstallKeepsWhatIsNotItsOwn(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.Remove(home + "/.claude/skills/cadre")
	os.Symlink("/elsewhere/skills/cadre", home+"/.claude/skills/cadre")
	out := must(t, "uninstall", "--yes")
	if !strings.Contains(out, "the skill link ~/.claude/skills/cadre, which points at another cadre") {
		t.Errorf("plan: %q", out)
	}
	if to, _ := os.Readlink(home + "/.claude/skills/cadre"); to != "/elsewhere/skills/cadre" {
		t.Error("another cadre's skill link was removed")
	}
}

// Sessions started by cadre 0.1.x are the user's, not this cadre's: they
// are named and left running. A config folder that is a link is removed
// as a link, and the plan says its folder stays.
func TestUninstallLeavesLegacySessionsAndLinkedFolders(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-dev", "-n", "engineer", "sleep", "300")
	os.Rename(home+"/.cadre/config", home+"/realconfig")
	os.Symlink(home+"/realconfig", home+"/.cadre/config")
	out := must(t, "uninstall", "--dry-run")
	for _, want := range []string{"sessions started by cadre 0.1.x, left running: cadre-dev",
		"remove the link ~/.cadre/config (its folder ~/realconfig is kept)"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "stop 1 member sessions: cadre-dev") {
		t.Errorf("the plan stops the 0.1.x session:\n%s", out)
	}
	out = must(t, "uninstall", "--yes")
	if !strings.Contains(out, "removed the link ~/.cadre/config") {
		t.Errorf("uninstall: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadre-dev:") != "" {
		t.Error("the 0.1.x session was stopped")
	}
	if _, err := os.Lstat(home + "/.cadre/config"); err == nil {
		t.Error("the link is still there")
	}
	if _, err := os.Stat(home + "/realconfig/default"); err != nil {
		t.Error("the linked folder was removed")
	}
}
