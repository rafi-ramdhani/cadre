package main

import (
	"bytes"
	"strings"
	"testing"
)

func call(args ...string) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(""), &o, &e)
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
