//go:build !windows

package main

import (
	"bytes"
	"strings"
	"testing"
)

func call(args ...string) (code int, out, errOut string) { return callIn("", args...) }

// callIn runs cadre with stdin as its input.
func callIn(stdin string, args ...string) (code int, out, errOut string) {
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
	want := []string{"open", "ls", "attach", "stop", "help", "--version", "uninstall"}
	// Plain cadre's line starts with its summary, so its second field is "open".
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("help lists %v, want %v", got, want)
	}
	if !strings.Contains(out, "cadre help advanced") {
		t.Error("help does not point to cadre help advanced")
	}
}

func TestHelpAdvancedListsEveryCommandAndOldName(t *testing.T) {
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
		if !strings.Contains(out, "cadre "+o.name) {
			t.Errorf("help advanced lacks the old name cadre %s", o.name)
		}
	}
	if strings.Contains(out, "hook") {
		t.Error("help advanced shows a hidden hook command")
	}
}

func TestOldNamesRunTheirNewCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
		rest string
	}{
		{[]string{"down", "dev"}, "stop", "dev"},
		{[]string{"down", "--all", "--yes"}, "stop", "--yes"},
		{[]string{"path", "app"}, "project path", "app"},
		{[]string{"sync"}, "project sync", ""},
		{[]string{"add", "project", "app", "me/app"}, "project add", "app me/app"},
		{[]string{"add", "team", "ops"}, "team add", "ops"},
		{[]string{"add", "persona", "ops/sre"}, "persona add", "ops/sre"},
		{[]string{"version"}, "--version", ""},
		{[]string{"up", "dev"}, "up", "dev"},
		{[]string{"ls", "--json"}, "ls --json", ""},
		{[]string{"ls", "--all", "--json"}, "ls", "--all --json"},
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

func TestRenamedUnreleasedNamesPointToTheNewName(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"which"}, "cadre ls"},
		{[]string{"cadres"}, "cadre ls"},
		{[]string{"cadres", "list"}, "cadre ls"},
		{[]string{"ctx"}, "cadre ls"},
		{[]string{"start"}, "cadre --tmux"},
		{[]string{"trust", "app"}, "cadre project trust"},
		{[]string{"remove", "project", "app"}, "cadre project unlink"},
		{[]string{"export", "project", "app", "/tmp/x"}, "cadre project export"},
		{[]string{"down", "--all", "--all-cadres"}, "cadre stop --all"},
	} {
		code, _, errOut := call(tc.args...)
		if code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: exit %d, %q; want a refusal naming %q", tc.args, code, errOut, tc.want)
		}
	}
	if c, _, err := lookup([]string{"cadres", "add", "/x"}); err != nil || c.name != "cadres add" {
		t.Errorf("cadres add is still a command: %v %v", c.name, err)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := call("bogus")
	if code != 1 || !strings.Contains(errOut, "unknown command 'bogus'") {
		t.Errorf("exit %d, %q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	for _, a := range []string{"--version", "version"} {
		code, out, _ := call(a)
		if code != 0 || out != "cadre dev\n" {
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

func TestEveryOldNameResolves(t *testing.T) {
	for _, o := range oldNames {
		if _, _, err := lookup(strings.Fields(o.name)); err != nil {
			t.Errorf("old name cadre %s: %v", o.name, err)
		}
	}
	for _, a := range []string{"-h", "--help"} {
		if c, _, err := lookup([]string{a}); err != nil || c.name != "help" {
			t.Errorf("%s runs %q (%v), want help", a, c.name, err)
		}
	}
	if c, _, err := lookup([]string{"-v"}); err != nil || c.name != "--version" {
		t.Errorf("-v runs %q (%v), want --version", c.name, err)
	}
	if c, _, err := lookup([]string{"projects"}); err != nil || c.name != "projects" {
		t.Errorf("projects runs %q (%v)", c.name, err)
	}
}

func TestEveryRenamedNameIsRefused(t *testing.T) {
	for _, r := range renames {
		if _, _, err := lookup(strings.Fields(r.name)); err == nil || !strings.Contains(err.Error(), r.use) {
			t.Errorf("cadre %s: %v, want a refusal naming %q", r.name, err, r.use)
		}
	}
}

func TestUnreleasedFormsOfOldNamesAreRefused(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"add", "project", "x", "--path", "/d"}, "cadre project add <name> --path <dir>"},
		{[]string{"ls", "--all-cadres"}, "cadre ls --all"},
		{[]string{"down", "--all-cadres"}, "cadre stop --all"},
		{[]string{"project", "remove", "x"}, "cadre project unlink"},
	} {
		if _, _, err := lookup(tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: %v, want a refusal naming %q", tc.args, err, tc.want)
		}
	}
}

func TestUnknownSubcommandIsNamed(t *testing.T) {
	_, _, err := lookup([]string{"cadres", "foo"})
	if err == nil || !strings.Contains(err.Error(), "unknown command 'cadres foo'") {
		t.Errorf("cadres foo: %v", err)
	}
	_, _, err = lookup([]string{"project", "bogus", "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown command 'project bogus'") {
		t.Errorf("project bogus: %v", err)
	}
}

func TestVersionTakesNoArguments(t *testing.T) {
	if code, _, _ := call("--version", "x"); code != 1 {
		t.Errorf("--version x exited %d", code)
	}
}
