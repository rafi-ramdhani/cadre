//go:build !windows

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/testguard"
)

func call(args ...string) (code int, out, errOut string) { return callIn("", args...) }

// callIn runs cadre with stdin as its input.
func callIn(stdin string, args ...string) (code int, out, errOut string) {
	testguard.MustBeSafe()
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func TestHelpListsExactlyTheVisibleCommands(t *testing.T) {
	code, out, _ := call("help")
	if code != 0 {
		t.Fatalf("help exited %d", code)
	}
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "  cadre") {
			got = append(got, strings.Fields(l)[1])
		}
	}
	want := []string{"[--fresh]", "--tmux", "ls", "attach", "stop", "help", "--version", "uninstall"}
	// Plain cadre's line has no name, so its second field is its usage.
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("help lists %v, want %v", got, want)
	}
	if !strings.Contains(out, "cadre help advanced") {
		t.Error("help does not point to cadre help advanced")
	}
}

func TestHelpAdvancedListsEveryCommand(t *testing.T) {
	_, out, _ := call("help", "advanced")
	for _, c := range commands {
		if c.group == "" || c.hidden {
			continue
		}
		if !strings.Contains(out, "cadre "+c.name) {
			t.Errorf("help advanced lacks cadre %s", c.name)
		}
	}
	for _, o := range oldNames {
		if strings.Contains(out, "cadre "+o.name+" ") {
			t.Errorf("help advanced lists the old name cadre %s", o.name)
		}
	}
	if strings.Contains(out, "hook") {
		t.Error("help advanced shows a hidden hook command")
	}
}

// Old 0.1.x names are not aliases: each says what to do now and exits 1.
func TestOldNamesPointToTheNewWay(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"down", "dev"}, "use cadre stop"},
		{[]string{"down", "--all"}, "use cadre stop"},
		{[]string{"projects"}, "use cadre ls"},
		{[]string{"path", "app"}, "use cadre project path"},
		{[]string{"sync"}, "use cadre project sync"},
		{[]string{"add", "project", "app", "me/app"}, "ask the orchestrator, or use cadre project add"},
		{[]string{"add", "team", "ops"}, "ask the orchestrator"},
		{[]string{"add", "persona", "ops/sre"}, "ask the orchestrator"},
		{[]string{"version"}, "use cadre --version"},
		{[]string{"trust", "app"}, "use cadre project trust"},
		{[]string{"update", "--check"}, "brew upgrade cadre"},
	} {
		code, out, errOut := call(tc.args...)
		if code != 1 || out != "" || strings.Count(errOut, "\n") != 1 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: exit %d, %q %q; want one line naming %q", tc.args, code, out, errOut, tc.want)
		}
	}
	for _, o := range oldNames {
		if _, _, err := lookup(strings.Fields(o.name)); err == nil {
			t.Errorf("old name cadre %s runs a command", o.name)
		}
	}
}

func TestCommandsStillNamedAsBefore(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		rest string
	}{
		{[]string{"up", "dev"}, "up", "dev"},
		{[]string{"init", "x"}, "init", "x"},
		{[]string{"use", "x"}, "use", "x"},
		{[]string{"attach", "dev"}, "attach", "dev"},
		{[]string{"ls", "--json"}, "ls --json", ""},
		{[]string{"ls", "--all", "--json"}, "ls", "--all --json"},
		{[]string{"-h"}, "help", ""},
		{[]string{"--help", "advanced"}, "help", "advanced"},
		{[]string{"-v"}, "--version", ""},
	} {
		c, rest, err := lookup(tc.args)
		if err != nil {
			t.Errorf("%v: %v", tc.args, err)
			continue
		}
		if c.name != tc.want || strings.Join(rest, " ") != tc.rest {
			t.Errorf("%v runs %q with %q, want %q with %q", tc.args, c.name, rest, tc.want, tc.rest)
		}
	}
}

// Commands of features that were cut are gone, not stubs.
func TestCutCommandsAreUnknown(t *testing.T) {
	for _, args := range [][]string{
		{"compact", "dev/pm"}, {"project", "export", "a", "/x"}, {"project", "move", "a", "/x"},
		{"project", "restore", "a"}, {"project", "relink", "a", "/x"}, {"cadres", "add", "/x"}, {"cadres", "remove", "x"},
		{"team", "add", "ops"}, {"member", "add", "ops/sre"}, {"--no-tmux"}, {"hook", "statusline"}, {"hook", "state", "x"},
		{"which"}, {"ctx"}, {"start"},
	} {
		if code, _, errOut := call(args...); code != 1 || !strings.Contains(errOut, "unknown command") {
			t.Errorf("%v: exit %d, %q", args, code, errOut)
		}
	}
	if code, _, errOut := call("stop", "--with-orchestrator"); code != 1 || !strings.Contains(errOut, "usage") {
		t.Errorf("stop --with-orchestrator: exit %d, %q", code, errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := call("bogus")
	if code != 1 || !strings.Contains(errOut, "unknown command 'bogus'") {
		t.Errorf("exit %d, %q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	for _, a := range []string{"--version", "-v"} {
		code, out, _ := call(a)
		if code != 0 || out != "cadre dev (dev)\n" {
			t.Errorf("%s: exit %d, %q", a, code, out)
		}
	}
}

func TestEveryCommandHasARunner(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range commands {
		if c.run == nil {
			t.Errorf("cadre %s has no runner", c.name)
		}
		if seen[c.name] {
			t.Errorf("cadre %s is listed twice", c.name)
		}
		seen[c.name] = true
		if !c.hidden && c.summary == "" {
			t.Errorf("cadre %s has no summary", c.name)
		}
	}
}

func TestUnknownSubcommandIsNamed(t *testing.T) {
	_, _, err := lookup([]string{"project", "bogus", "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown command 'project bogus'") {
		t.Errorf("project bogus: %v", err)
	}
}

func TestVersionTakesNoArguments(t *testing.T) {
	if code, _, _ := call("--version", "x"); code != 1 {
		t.Errorf("--version x exited %d", code)
	}
}
