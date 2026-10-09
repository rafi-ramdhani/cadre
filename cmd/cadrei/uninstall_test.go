//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadrei/internal/backup"
	"github.com/rafi-ramdhani/cadrei/internal/framework"
)

func TestUninstall(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	must(t, "--check")                  // the framework is written
	must(t, "up", "dev/engineer")       // a member runs
	must(t, "project", "dir", "~/Code") // config holds a projects folder
	c := home + "/.cadrei/work"
	os.MkdirAll(home+"/Code/app/.git", 0o755)
	// The orchestrator hook runs this binary (through a link named cadrei,
	// as an installed binary is); another hook and another cadrei's hook
	// stay.
	os.MkdirAll(home+"/inst", 0o755)
	os.Symlink(framework.Binary(), home+"/inst/cadrei")
	settings := home + "/.claude/settings.json"
	os.WriteFile(settings, []byte(`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "`+home+`/inst/cadrei hook orchestrator"}]}, {"hooks": [{"type": "command", "command": "/other/cadrei hook orchestrator"}]}], "PreToolUse": [{"hooks": [{"type": "command", "command": "mine"}]}]}}`), 0o644)
	if !backup.Installed(c) {
		t.Fatal("init did not install the pre-push check")
	}

	t.Setenv("CADREI_MEMBER", "work-dev-engineer")
	refused(t, "refused for members", "uninstall", "--yes")
	t.Setenv("CADREI_MEMBER", "")

	out := must(t, "uninstall", "--dry-run")
	for _, want := range []string{"stop 1 member sessions: cadrei-work-dev", "remove the skill link ~/.claude/skills/cadrei",
		"remove the orchestrator hook from ~/.claude/settings.json (other hooks stay)", "remove cadrei's pre-push check from ~/.cadrei/work",
		"remove ~/.cadrei/config", "remove ~/.cadrei/framework", "every cadrei (work, in ~/.cadrei) and every project",
		"orchestrator hooks that run another cadrei program", "Then: delete the cadrei you built"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan lacks %q:\n%s", want, out)
		}
	}
	if code, _, errOut := call("uninstall"); code != 1 || !strings.Contains(errOut, "run with --yes") {
		t.Errorf("without a terminal: %d %q", code, errOut)
	}
	if _, err := os.Stat(framework.Dir()); err != nil || tmuxIn(socket, "has-session", "-t", "=cadrei-work-dev") != "" {
		t.Fatal("the dry run or the refusal changed something")
	}

	out = must(t, "uninstall", "--yes")
	if !strings.Contains(out, "Uninstalled. Your cadreis and projects are kept.") {
		t.Errorf("uninstall: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadrei-work-dev") == "" {
		t.Error("the member still runs")
	}
	if _, err := os.Lstat(home + "/.claude/skills/cadrei"); err == nil {
		t.Error("the skill link is still there")
	}
	s := readFile(t, settings)
	if strings.Contains(s, home+"/inst/cadrei") || !strings.Contains(s, "/other/cadrei hook orchestrator") || !strings.Contains(s, `"mine"`) {
		t.Errorf("settings after uninstall:\n%s", s)
	}
	if backup.Installed(c) {
		t.Error("the pre-push check is still there")
	}
	for _, d := range []string{home + "/.cadrei/config", home + "/.cadrei/framework"} {
		if _, err := os.Stat(d); err == nil {
			t.Errorf("%s is still there", d)
		}
	}
	if _, err := os.Stat(c + "/members/dev/engineer.md"); err != nil {
		t.Error("the cadrei was touched")
	}
	if _, err := os.Stat(home + "/Code/app/.git"); err != nil {
		t.Error("a project was touched")
	}
	if log, _ := exec.Command("git", "-C", c, "status", "--porcelain").Output(); len(log) != 0 {
		t.Errorf("the cadrei's repository changed: %s", log)
	}
}

// A skill link that points at another cadrei is kept.
func TestUninstallKeepsWhatIsNotItsOwn(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	os.Remove(home + "/.claude/skills/cadrei")
	os.Symlink("/elsewhere/skills/cadrei", home+"/.claude/skills/cadrei")
	out := must(t, "uninstall", "--yes")
	if !strings.Contains(out, "the skill link ~/.claude/skills/cadrei, which points at another cadrei program") {
		t.Errorf("plan: %q", out)
	}
	if to, _ := os.Readlink(home + "/.claude/skills/cadrei"); to != "/elsewhere/skills/cadrei" {
		t.Error("another cadrei's skill link was removed")
	}
}

// Sessions started by cadre 0.1.x are the user's, not this cadrei's: they
// are named and left running. A config folder that is a link is removed
// as a link, and the plan says its folder stays.
func TestUninstallLeavesLegacySessionsAndLinkedFolders(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-dev", "-n", "engineer", "sleep", "300")
	os.Rename(home+"/.cadrei/config", home+"/realconfig")
	os.Symlink(home+"/realconfig", home+"/.cadrei/config")
	out := must(t, "uninstall", "--dry-run")
	for _, want := range []string{"sessions started by cadre 0.1.x, left running: cadre-dev",
		"remove the link ~/.cadrei/config (its folder ~/realconfig is kept)"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "stop 1 member sessions: cadre-dev") {
		t.Errorf("the plan stops the 0.1.x session:\n%s", out)
	}
	out = must(t, "uninstall", "--yes")
	if !strings.Contains(out, "removed the link ~/.cadrei/config") {
		t.Errorf("uninstall: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadre-dev:") != "" {
		t.Error("the 0.1.x session was stopped")
	}
	if _, err := os.Lstat(home + "/.cadrei/config"); err == nil {
		t.Error("the link is still there")
	}
	if _, err := os.Stat(home + "/realconfig/default"); err != nil {
		t.Error("the linked folder was removed")
	}
}
