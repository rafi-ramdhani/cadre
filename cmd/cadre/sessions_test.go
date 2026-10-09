//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// withTmux gives the test a private tmux server and a stub claude that
// records its arguments and stays alive, like a session would.
func withTmux(t *testing.T, home string) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	socket := fmt.Sprintf("cadre-gotest-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Setenv("CADRE_TMUX_SOCKET", socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"+stubAnswers+"printf '%s\\n' \"$@\" > \""+home+"/args-$CADRE_MEMBER\"\nexec sleep 300\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return socket
}

// stubAnswers makes a stub claude answer the health check's questions as
// a current, logged-in install does.
const stubAnswers = "case \"$1 $2\" in\n" +
	"  '--version '*) echo '2.1.300 (Claude Code)'; exit 0 ;;\n" +
	"  'auth status') echo '{\"loggedIn\": true}'; exit 0 ;;\n" +
	"esac\n"

func tmuxIn(socket string, args ...string) string {
	out, _ := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

func TestUpLsStop(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	must(t, "init", "life")
	t.Chdir(home + "/.cadre/work")
	out := must(t, "up", "dev/engineer")
	if !strings.Contains(out, "work-dev-engineer started in "+home+"/.cadre/work/teams/dev") {
		t.Fatalf("up: %q", out)
	}
	if !strings.Contains(out, "created "+home+"/.cadre/work/.claude/member-settings.json") {
		t.Errorf("no settings file was made: %q", out)
	}
	var args string
	for i := 0; i < 100 && !strings.Contains(args, "--settings"); i++ {
		time.Sleep(50 * time.Millisecond)
		args = readFile(t, home+"/args-work-dev-engineer")
	}
	if !strings.Contains(args, "--settings\n"+home+"/.cadre/work/.claude/build/member-settings.") {
		t.Errorf("the member got no settings copy:\n%s", args)
	}
	out = must(t, "ls")
	for _, want := range []string{"cadre work  (~/.cadre/work, from this folder; default)", "running:", "  dev              engineer", "projects: none", "other cadres: life (none running)"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls lacks %q:\n%s", want, out)
		}
	}
	var st cadreStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if st.Cadre.Name != "work" || len(st.Running) != 1 || st.Running[0].Members[0].Name != "work-dev-engineer" ||
		len(st.Teams["dev"]) == 0 || len(st.Problems) != 0 {
		t.Errorf("ls --json: %+v", st)
	}
	// The other cadre has its own sessions with the same team.
	t.Chdir(home + "/.cadre/life")
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "life-dev-engineer started") {
		t.Errorf("up in life: %q", out)
	}
	if out := must(t, "ls"); strings.Contains(out, "work-dev") || !strings.Contains(out, "work (1 running)") {
		t.Errorf("ls in life:\n%s", out)
	}
	var all allStatus
	json.Unmarshal([]byte(must(t, "ls", "--all", "--json")), &all)
	if len(all.Cadres) != 2 {
		t.Errorf("ls --all --json: %+v", all)
	}
	// stop with no team asks; without a terminal it says how to confirm.
	code, _, errOut := call("stop")
	if code != 1 || !strings.Contains(errOut, "run with --yes to confirm") {
		t.Errorf("stop without a terminal: %d %q", code, errOut)
	}
	out = must(t, "stop", "--yes")
	if !strings.Contains(out, "cadre-life-dev stopped") || !strings.Contains(out, "stopped every session of cadre life") {
		t.Errorf("stop --yes: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadre-work-dev") != "" {
		t.Error("stop in life stopped work's session")
	}
	t.Setenv("CADRE_MEMBER", "x")
	refused(t, "refused for members: members cannot stop the whole cadre", "stop", "--all", "--yes")
	t.Setenv("CADRE_MEMBER", "")
	out = must(t, "stop", "--all", "--yes")
	if !strings.Contains(out, "cadre work:") || !strings.Contains(out, "stopped every cadre session") {
		t.Errorf("stop --all: %q", out)
	}
	if out := must(t, "stop", "--all"); !strings.Contains(out, "no cadre sessions running") {
		t.Errorf("stop --all with nothing running: %q", out)
	}
	refused(t, "usage", "stop", "dev", "--all")
	refused(t, "is not running", "attach", "dev")
}

func TestStopOneTeamOrRole(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	must(t, "up", "dev/engineer")
	must(t, "up", "dev/reviewer")
	if out := must(t, "stop", "dev/reviewer"); strings.TrimSpace(out) != "work-dev-reviewer stopped" {
		t.Errorf("stop a role: %q", out)
	}
	if out := must(t, "stop", "dev"); strings.TrimSpace(out) != "cadre-work-dev stopped" {
		t.Errorf("stop a team: %q", out)
	}
	if out := must(t, "stop", "dev"); strings.TrimSpace(out) != "cadre-work-dev not running" {
		t.Errorf("stop again: %q", out)
	}
	refused(t, "no team 'nope'", "stop", "nope")
}

func TestUpRefusesACollision(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-work-dev", "sleep", "60")
	tmuxIn(socket, "set-option", "-t", "=cadre-work-dev:", "@cadre_home", "/elsewhere/work")
	refused(t, "belongs to cadre work (/elsewhere/work)", "up", "dev/reviewer")
}

func TestLegacySessionsInLs(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-dev", "-n", "engineer", "sleep", "60")
	out := must(t, "ls")
	if !strings.Contains(out, "dev (legacy)") || !strings.Contains(out, "legacy sessions keep their old names") {
		t.Errorf("ls with a legacy session:\n%s", out)
	}
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "already running (legacy session cadre-dev") {
		t.Errorf("up: %q", out)
	}
	out = must(t, "ls", "--all")
	if !strings.Contains(out, "legacy sessions (started by cadre 0.1.x") || !strings.Contains(out, "cadre-dev: dev-engineer") {
		t.Errorf("ls --all:\n%s", out)
	}
}

// AC-P4: a runtime other than claude is refused by a release build, at up
// and in ls.
// Claude Code is the only runtime: .runtime files and RUNTIME in
// cadre.conf are not read, and a release build ignores the test variable.
func TestTheRuntimeIsNotAChoice(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadre/work/members/dev/engineer.runtime", []byte("codex\n"), 0o644)
	os.WriteFile(home+"/.cadre/work/cadre.conf", []byte("RUNTIME=codex\n"), 0o644)
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "work-dev-engineer started") {
		t.Errorf("up: %q", out)
	}
	if _, err := runtime.Get("fake"); err == nil {
		return // a -tags cadretest build has the fake runtime
	}
	t.Setenv("CADRE_TEST_RUNTIME", "fake")
	if out := must(t, "up", "dev/reviewer"); !strings.Contains(out, "work-dev-reviewer started") {
		t.Errorf("CADRE_TEST_RUNTIME in a release build: %q", out)
	}
}

// up starts only names that can be part of a session name: . and .. are no
// teams, and a dot (allowed in 0.1.x, as in ml.ops) is refused with what
// to rename. Nothing starts.
func TestUpRefusesNamesItCannotStart(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	c := home + "/.cadre/work"
	os.MkdirAll(c+"/members/ml.ops", 0o755)
	os.WriteFile(c+"/members/ml.ops/sre.md", []byte("# sre\n"), 0o644)
	os.WriteFile(c+"/members/dev/qa.lead.md", []byte("# qa\n"), 0o644)
	refused(t, "no team '..'", "up", "..")
	refused(t, "no team '.'", "up", ".")
	refused(t, "no team '..'", "up", "../dev")
	refused(t, "cannot start team 'ml.ops': team and member names may use letters, digits, - and _; rename its folder, ~/.cadre/work/members/ml.ops", "up", "ml.ops")
	refused(t, "cannot start member 'qa.lead': team and member names may use letters, digits, - and _; rename its file, ~/.cadre/work/members/dev/qa.lead.md", "up", "dev")
	refused(t, "cannot start member 'qa.lead'", "up", "dev/qa.lead")
	if out := tmuxIn(socket, "list-sessions", "-F", "#S"); strings.Contains(out, "cadre-work") {
		t.Errorf("a session started: %s", out)
	}
	// The other members of the team still start one by one.
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "work-dev-engineer started") {
		t.Errorf("up dev/engineer: %q", out)
	}
}
