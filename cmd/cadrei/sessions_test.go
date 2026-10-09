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

	"github.com/rafi-ramdhani/cadrei/internal/runtime"
)

// withTmux gives the test a private tmux server and a stub claude that
// records its arguments and stays alive, like a session would.
func withTmux(t *testing.T, home string) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	socket := fmt.Sprintf("cadrei-gotest-%d-%d", os.Getpid(), time.Now().UnixNano())
	t.Setenv("CADREI_TMUX_SOCKET", socket)
	t.Cleanup(func() { exec.Command("tmux", "-L", socket, "kill-server").Run() })
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"+stubAnswers+"printf '%s\\n' \"$@\" > \""+home+"/args-$CADREI_MEMBER\"\nexec sleep 300\n"), 0o755)
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
	t.Chdir(home + "/.cadrei/work")
	out := must(t, "up", "dev/engineer")
	if !strings.Contains(out, "work-dev-engineer started in "+home+"/.cadrei/work/teams/dev") {
		t.Fatalf("up: %q", out)
	}
	if !strings.Contains(out, "created "+home+"/.cadrei/work/.claude/member-settings.json") {
		t.Errorf("no settings file was made: %q", out)
	}
	var args string
	for i := 0; i < 100 && !strings.Contains(args, "--settings"); i++ {
		time.Sleep(50 * time.Millisecond)
		args = readFile(t, home+"/args-work-dev-engineer")
	}
	if !strings.Contains(args, "--settings\n"+home+"/.cadrei/work/.claude/build/member-settings.") {
		t.Errorf("the member got no settings copy:\n%s", args)
	}
	out = must(t, "ls")
	for _, want := range []string{"cadrei work  (~/.cadrei/work, from this folder; default)", "running:", "  dev              engineer", "projects: none", "other cadreis: life (none running)"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls lacks %q:\n%s", want, out)
		}
	}
	var st cadreiStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if st.Cadrei.Name != "work" || len(st.Running) != 1 || st.Running[0].Members[0].Name != "work-dev-engineer" ||
		len(st.Teams["dev"]) == 0 || len(st.Problems) != 0 {
		t.Errorf("ls --json: %+v", st)
	}
	// The other cadrei has its own sessions with the same team.
	t.Chdir(home + "/.cadrei/life")
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "life-dev-engineer started") {
		t.Errorf("up in life: %q", out)
	}
	if out := must(t, "ls"); strings.Contains(out, "work-dev") || !strings.Contains(out, "work (1 running)") {
		t.Errorf("ls in life:\n%s", out)
	}
	var all allStatus
	json.Unmarshal([]byte(must(t, "ls", "--all", "--json")), &all)
	if len(all.Cadreis) != 2 {
		t.Errorf("ls --all --json: %+v", all)
	}
	// stop with no team asks; without a terminal it says how to confirm.
	code, _, errOut := call("stop")
	if code != 1 || !strings.Contains(errOut, "run with --yes to confirm") {
		t.Errorf("stop without a terminal: %d %q", code, errOut)
	}
	out = must(t, "stop", "--yes")
	if !strings.Contains(out, "cadrei-life-dev stopped") || !strings.Contains(out, "stopped every session of cadrei life") {
		t.Errorf("stop --yes: %q", out)
	}
	if tmuxIn(socket, "has-session", "-t", "=cadrei-work-dev") != "" {
		t.Error("stop in life stopped work's session")
	}
	t.Setenv("CADREI_MEMBER", "x")
	refused(t, "refused for members: members cannot stop the whole cadrei", "stop", "--all", "--yes")
	t.Setenv("CADREI_MEMBER", "")
	out = must(t, "stop", "--all", "--yes")
	if !strings.Contains(out, "cadrei work:") || !strings.Contains(out, "stopped every cadrei session") {
		t.Errorf("stop --all: %q", out)
	}
	if out := must(t, "stop", "--all"); !strings.Contains(out, "no cadrei sessions running") {
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
	if out := must(t, "stop", "dev"); strings.TrimSpace(out) != "cadrei-work-dev stopped" {
		t.Errorf("stop a team: %q", out)
	}
	if out := must(t, "stop", "dev"); strings.TrimSpace(out) != "cadrei-work-dev not running" {
		t.Errorf("stop again: %q", out)
	}
	refused(t, "no team 'nope'", "stop", "nope")
}

func TestUpRefusesACollision(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadrei-work-dev", "sleep", "60")
	tmuxIn(socket, "set-option", "-t", "=cadrei-work-dev:", "@cadrei_home", "/elsewhere/work")
	refused(t, "belongs to cadrei work (/elsewhere/work)", "up", "dev/reviewer")
}

// A cadrei- session without markers is not a 0.1.x one: up, stop, attach,
// ls, ls --all and uninstall say it has no markers, never that it is legacy
// or not running.
func TestASessionWithoutMarkersIsNotLegacy(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadrei-work-dev", "-n", "engineer", "sleep", "60")
	if out := must(t, "ls"); strings.Contains(out, "legacy") || !strings.Contains(out, "running: nothing") {
		t.Errorf("ls shows it as the cadrei's:\n%s", out)
	}
	out := must(t, "ls", "--all")
	if strings.Contains(out, "legacy") || !strings.Contains(out, "sessions without cadrei's markers") {
		t.Errorf("ls --all:\n%s", out)
	}
	var st allStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--all", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Legacy) != 0 || len(st.Unknown) != 1 || st.Unknown[0].Session != "cadrei-work-dev" || st.Unknown[0].Legacy {
		t.Errorf("ls --all --json: legacy %v, unknown %v", st.Legacy, st.Unknown)
	}
	// up, stop and attach say the same, and only ls --all lists it.
	want := "a session named cadrei-work-dev exists without cadrei's markers; stop it with tmux kill-session -t cadrei-work-dev"
	refused(t, want, "up", "dev/engineer")
	refused(t, want, "attach", "dev")
	for _, target := range []string{"dev", "dev/engineer"} {
		if out := must(t, "stop", target); !strings.Contains(out, want) || strings.Contains(out, "not running") {
			t.Errorf("stop %s: %q", target, out)
		}
	}
	if tmuxIn(socket, "has-session", "-t", "=cadrei-work-dev:") != "" {
		t.Error("stop stopped it")
	}
	if out := must(t, "ls"); !strings.Contains(out, "1 session without cadrei's markers; see cadrei ls --all") {
		t.Errorf("ls has no pointer:\n%s", out)
	}
	tmuxIn(socket, "new-session", "-d", "-s", "cadrei-other-qa", "sleep", "60")
	if out := must(t, "ls"); !strings.Contains(out, "2 sessions without cadrei's markers; see cadrei ls --all") {
		t.Errorf("ls with two:\n%s", out)
	}
	if out := must(t, "ls", "--json"); strings.Contains(out, "markers") {
		t.Errorf("ls --json carries the pointer:\n%s", out)
	}
	tmuxIn(socket, "kill-session", "-t", "=cadrei-other-qa:")
	out = must(t, "uninstall", "--dry-run")
	if !strings.Contains(out, "sessions without cadrei's markers, left running: cadrei-work-dev") || strings.Contains(out, "0.1.x, left running") {
		t.Errorf("uninstall plan:\n%s", out)
	}
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
// cadrei.conf are not read, and a release build ignores the test variable.
func TestTheRuntimeIsNotAChoice(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	os.WriteFile(home+"/.cadrei/work/members/dev/engineer.runtime", []byte("codex\n"), 0o644)
	os.WriteFile(home+"/.cadrei/work/cadrei.conf", []byte("RUNTIME=codex\n"), 0o644)
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "work-dev-engineer started") {
		t.Errorf("up: %q", out)
	}
	if _, err := runtime.Get("fake"); err == nil {
		return // a -tags cadreitest build has the fake runtime
	}
	t.Setenv("CADREI_TEST_RUNTIME", "fake")
	if out := must(t, "up", "dev/reviewer"); !strings.Contains(out, "work-dev-reviewer started") {
		t.Errorf("CADREI_TEST_RUNTIME in a release build: %q", out)
	}
}

// up starts only names that can be part of a session name: . and .. are no
// teams, and a dot (allowed in 0.1.x, as in ml.ops) is refused with what
// to rename. Nothing starts.
func TestUpRefusesNamesItCannotStart(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	c := home + "/.cadrei/work"
	os.MkdirAll(c+"/members/ml.ops", 0o755)
	os.WriteFile(c+"/members/ml.ops/sre.md", []byte("# sre\n"), 0o644)
	os.WriteFile(c+"/members/dev/qa.lead.md", []byte("# qa\n"), 0o644)
	refused(t, "no team '..'", "up", "..")
	refused(t, "no team '.'", "up", ".")
	refused(t, "no team '..'", "up", "../dev")
	refused(t, "cannot start team 'ml.ops': team and member names may use letters, digits, - and _; rename its folder, ~/.cadrei/work/members/ml.ops", "up", "ml.ops")
	refused(t, "cannot start member 'qa.lead': team and member names may use letters, digits, - and _; rename its file, ~/.cadrei/work/members/dev/qa.lead.md", "up", "dev")
	refused(t, "cannot start member 'qa.lead'", "up", "dev/qa.lead")
	if out := tmuxIn(socket, "list-sessions", "-F", "#S"); strings.Contains(out, "cadrei-work") {
		t.Errorf("a session started: %s", out)
	}
	// The other members of the team still start one by one.
	if out := must(t, "up", "dev/engineer"); !strings.Contains(out, "work-dev-engineer started") {
		t.Errorf("up dev/engineer: %q", out)
	}
}

// A build folder that is a link is refused before anything is written
// into it: the member settings copy included.
func TestUpWritesNothingThroughALinkedBuildFolder(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	scratch := t.TempDir()
	os.RemoveAll(home + "/.cadrei/work/.claude/build")
	os.MkdirAll(home+"/.cadrei/work/.claude", 0o755)
	os.Symlink(scratch, home+"/.cadrei/work/.claude/build")
	if code, out, _ := call("up", "dev/engineer"); code == 0 || !strings.Contains(out, "work-dev-engineer not started") || !strings.Contains(out, "is a link or a file, not a folder") {
		t.Errorf("up: %d %q", code, out)
	}
	if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
		t.Errorf("written through the link: %v", entries)
	}
	if out := tmuxIn(socket, "list-sessions", "-F", "#S"); strings.Contains(out, "cadrei-work") {
		t.Errorf("a session started: %s", out)
	}
}

// my.app and my_app share a tmux session name: stop and attach act only on
// the one asked for, and name the other when it is the one running.
func TestStopAndAttachMatchTheProject(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	for _, p := range []string{"my.app", "my_app"} {
		exec.Command("git", "init", "-q", home+"/src/"+p).Run()
		must(t, "project", "add", p, "--path", home+"/src/"+p, "--no-trust")
	}
	must(t, "up", "dev/engineer", "my.app")
	session := "cadrei-work-dev-my_app"
	if out := must(t, "stop", "dev", "my_app"); !strings.Contains(out, session+" not running (dev my.app runs under that name, and was left as it is)") {
		t.Errorf("stop the other project: %q", out)
	}
	if out := must(t, "stop", "dev/engineer", "my_app"); !strings.Contains(out, "not running (dev my.app runs under that name") {
		t.Errorf("stop a role of the other project: %q", out)
	}
	refused(t, "dev my_app is not running; dev my.app runs under that session name", "attach", "dev", "my_app")
	if tmuxIn(socket, "has-session", "-t", "="+session+":") != "" {
		t.Fatal("my.app's members were stopped")
	}
	if out := must(t, "stop", "dev", "my.app"); !strings.Contains(out, session+" stopped") {
		t.Errorf("stop the project that runs: %q", out)
	}
}

// ls names the teams and members that cannot start, so the orchestrator
// does not offer them.
func TestLsNamesWhatCannotStart(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	c := home + "/.cadrei/work"
	os.MkdirAll(c+"/members/ml.ops", 0o755)
	os.WriteFile(c+"/members/ml.ops/sre.md", []byte("# sre\n"), 0o644)
	os.WriteFile(c+"/members/dev/qa.lead.md", []byte("# qa\n"), 0o644)
	var st cadreiStatus
	if err := json.Unmarshal([]byte(must(t, "ls", "--json")), &st); err != nil {
		t.Fatal(err)
	}
	all := strings.Join(st.Problems, "\n")
	for _, want := range []string{"team ml.ops cannot start: team and member names may use letters, digits, - and _; rename its folder, members/ml.ops",
		"member qa.lead of team dev cannot start: team and member names may use letters, digits, - and _; rename its file, members/dev/qa.lead.md"} {
		if !strings.Contains(all, want) {
			t.Errorf("problems lack %q:\n%s", want, all)
		}
	}
}

// A session with this cadrei's home but no team markers (a start that died
// early, or one made by hand): up, attach and stop say how to stop it, and
// no member starts in it.
func TestUpRefusesASessionWithoutMarkers(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	tmuxIn(socket, "new-session", "-d", "-s", "cadrei-work-dev", "-n", "x", "sleep", "60")
	tmuxIn(socket, "set-option", "-q", "-t", "=cadrei-work-dev:", "@cadrei_home", home+"/.cadrei/work")
	want := "a session named cadrei-work-dev exists without cadrei's markers; stop it with tmux kill-session -t cadrei-work-dev, or cadrei stop --yes"
	refused(t, want, "up", "dev/engineer")
	refused(t, want, "attach", "dev")
	if out := must(t, "stop", "dev"); !strings.Contains(out, want) {
		t.Errorf("stop: %q", out)
	}
	if out := tmuxIn(socket, "list-windows", "-t", "=cadrei-work-dev:", "-F", "#W"); out != "x" {
		t.Errorf("a member started in it: %q", out)
	}
}
