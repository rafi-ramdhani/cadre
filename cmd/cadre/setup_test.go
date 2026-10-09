//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/framework"
)

// firstMachine is a machine where cadre never ran: no cadre, no skill link,
// no health check done. Answers come from stdin as from a terminal.
func firstMachine(t *testing.T) string {
	t.Helper()
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	os.Remove(home + "/.claude/skills/cadre")
	os.Remove(cadres.Config("state.json"))
	ttyForTests = true
	t.Cleanup(func() { ttyForTests = false })
	return home
}

func TestFirstRunWithoutATerminalSaysWhatToRun(t *testing.T) {
	home := firstMachine(t)
	ttyForTests = false
	code, _, errOut := call()
	if code != 1 || !strings.Contains(errOut, "run cadre in a terminal to set one up, or create one with cadre init <name>") {
		t.Errorf("exit %d, %q", code, errOut)
	}
	if list, _ := cadres.List(); len(list) != 0 {
		t.Error("a cadre was created without asking")
	}
	if _, err := os.Stat(home + "/orch-ran"); err == nil {
		t.Error("the orchestrator opened")
	}
}

func TestFirstRunNewCadre(t *testing.T) {
	home := firstMachine(t)
	// A git repository the user starts in is offered as a project.
	os.MkdirAll(home+"/src/app", 0o755)
	exec.Command("git", "-C", home+"/src/app", "init", "-q").Run()
	exec.Command("git", "-C", home+"/src/app", "remote", "add", "origin", "https://github.com/me/app.git").Run()
	t.Chdir(home + "/src/app")
	// new, the name, link this folder, the hook, the skill link.
	code, out, errOut := callIn("new\nmine\ny\ny\ny\n")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	c := home + "/.cadre/mine"
	if cadres.Default() != "mine" {
		t.Errorf("default %q", cadres.Default())
	}
	if roles, _ := filepath.Glob(c + "/personas/*/*.md"); len(roles) != 2 {
		t.Errorf("starter team %v", roles)
	}
	if reg := readFile(t, c+"/projects.yaml"); !strings.Contains(reg, "app:\n") || cadres.Place(cadres.Cadre{Name: "mine", Path: c}, "app") != home+"/src/app" {
		t.Errorf("this folder was not linked:\n%s", reg)
	}
	if to, _ := os.Readlink(home + "/.claude/skills/cadre"); to != framework.SkillDir() {
		t.Errorf("skill link %q", to)
	}
	if _, err := os.Stat(framework.SkillDir() + "/SKILL.md"); err != nil {
		t.Error("the skill was not written out")
	}
	settings := readFile(t, home+"/.claude/settings.json")
	if !strings.Contains(settings, "hook orchestrator") || !strings.Contains(settings, "SessionStart") {
		t.Errorf("hook not added:\n%s", settings)
	}
	if !strings.Contains(out, greeting) {
		t.Errorf("no greeting:\n%s", out)
	}
	if ran := readFile(t, home+"/orch-ran"); !strings.Contains(ran, "--name\nmine-orchestrator\n") {
		t.Errorf("the orchestrator did not open:\n%s", ran)
	}
	if cadres.GetState(cadres.CheckedVersion) != version {
		t.Error("the full health check was not recorded")
	}
}

func TestFirstRunAsksBeforeEachChange(t *testing.T) {
	home := firstMachine(t)
	// new with the suggested name, no hook, no skill link.
	code, out, errOut := callIn("\n\nn\nn\n")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if _, err := os.Lstat(home + "/.claude/skills/cadre"); err == nil {
		t.Error("the skill was linked after a no")
	}
	if _, err := os.Stat(home + "/.claude/settings.json"); err == nil {
		t.Error("the hook was added after a no")
	}
	if !strings.Contains(errOut, "problem: the cadre skill is not linked") {
		t.Errorf("the missing link was not reported:\n%s", errOut)
	}
}

func TestFirstRunRestoreIsNotBuiltYet(t *testing.T) {
	firstMachine(t)
	code, _, errOut := callIn("restore\n")
	if code != 1 || !strings.Contains(errOut, "not built yet") {
		t.Errorf("exit %d, %q", code, errOut)
	}
	if list, _ := cadres.List(); len(list) != 0 {
		t.Error("restore created a cadre")
	}
}

func TestHealthCheck(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	if code, out, _ := call("--check"); code != 0 || !strings.Contains(out, "everything is in order") {
		t.Errorf("--check on a sound machine: %d %q", code, out)
	}
	// A broken link and a 0.1.x hook, without a terminal: reported with
	// their fixes, the orchestrator still opens.
	os.Remove(home + "/.claude/skills/cadre")
	os.Symlink(home+"/old/skills/cadre", home+"/.claude/skills/cadre")
	os.WriteFile(home+"/.claude/settings.json", []byte(`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "bash /old/bin/orchestrator-hook.sh"}]}]}}`), 0o644)
	os.MkdirAll(home+"/.cadre/work/.claude", 0o755)
	os.WriteFile(home+"/.cadre/work/.claude/settings.json", []byte("{}"), 0o644)
	code, _, errOut := call()
	for _, want := range []string{"problem: the cadre skill links to ~/old/skills/cadre", "problem: the orchestrator hook runs /old/bin/orchestrator-hook.sh",
		".cadre/work/.claude/settings.json exists", "fix: "} {
		if !strings.Contains(errOut, want) {
			t.Errorf("no %q in:\n%s", want, errOut)
		}
	}
	if code != 0 {
		t.Errorf("a problem that is not fatal stopped cadre: %d", code)
	}
	os.Remove(home + "/.cadre/work/.claude/settings.json")
	// With a terminal: fix the link, keep the hook; the kept hook is not
	// asked about again.
	ttyForTests = true
	defer func() { ttyForTests = false }()
	code, _, errOut = callIn("y\nn\n", "--check")
	if to, _ := os.Readlink(home + "/.claude/skills/cadre"); to != framework.SkillDir() || code != 0 {
		t.Errorf("relink: %q %d\n%s", to, code, errOut)
	}
	if !strings.Contains(readFile(t, home+"/.claude/settings.json"), "orchestrator-hook.sh") {
		t.Error("the hook changed after a no")
	}
	if _, out, _ := call("--check"); !strings.Contains(out, "everything is in order") {
		t.Errorf("the kept hook was asked about again: %q", out)
	}
	// A yes points the hook at this binary.
	cadres.SetState(cadres.HookKept, "")
	callIn("y\n", "--check")
	var s map[string]any
	json.Unmarshal([]byte(readFile(t, home+"/.claude/settings.json")), &s)
	if raw, _ := json.Marshal(s); !strings.Contains(string(raw), framework.Binary()+" hook orchestrator") || strings.Contains(string(raw), "orchestrator-hook.sh") {
		t.Errorf("hook after a yes: %s", raw)
	}
}

func TestHealthStopsWithoutTheRuntime(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	only := filepath.Join(home, "only")
	os.MkdirAll(only, 0o755)
	for _, tool := range []string{"git", "tmux"} {
		p, _ := exec.LookPath(tool)
		os.Symlink(p, filepath.Join(only, tool))
	}
	t.Setenv("PATH", only)
	code, _, errOut := call()
	if code != 1 || !strings.Contains(errOut, "problem: Claude Code (claude) is not on your PATH") || !strings.Contains(errOut, "fix: brew install --cask claude-code") {
		t.Errorf("exit %d, %q", code, errOut)
	}
}

func TestAVersionChangeRunsTheFullCheck(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	cadres.SetState(cadres.CheckedVersion, "0.0.1")
	must(t)
	if cadres.GetState(cadres.CheckedVersion) != version {
		t.Error("the full check after a version change was not recorded")
	}
	if b, _ := os.ReadFile(framework.Dir() + "/VERSION"); strings.TrimSpace(string(b)) != version {
		t.Errorf("framework VERSION %q", b)
	}
}

func TestMissingProjectsAreMentioned(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	stubClaude(t, home)
	os.WriteFile(home+"/release", nil, 0o644)
	must(t, "init", "work")
	register(t, home, "work", "app:\n  repo: me/app\n  path: ~/Developer/app\n")
	if out := must(t); !strings.Contains(out, "Not on this machine yet: app") {
		t.Errorf("no note: %q", out)
	}
}

func TestHookOrchestrator(t *testing.T) {
	sandbox(t)
	code, out, _ := call("hook", "orchestrator")
	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.HookSpecificOutput.HookEventName != "SessionStart" ||
		!strings.Contains(got.HookSpecificOutput.AdditionalContext, "This session is the cadre orchestrator") {
		t.Errorf("hook: %d %q", code, out)
	}
	for _, v := range []string{"CADRE_PERSONA", "CADRE_OFF", "CADRE_ORCHESTRATOR"} {
		t.Setenv(v, "1")
		if code, out, errOut := call("hook", "orchestrator"); code != 0 || out != "" || errOut != "" {
			t.Errorf("with %s: %d %q %q", v, code, out, errOut)
		}
		t.Setenv(v, "")
	}
}
