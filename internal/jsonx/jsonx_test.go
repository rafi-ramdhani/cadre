package jsonx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A config as Claude Code writes it (JSON.stringify(x, null, 2)), with what
// encoding/json would damage: key order, a lone surrogate, HTML characters,
// number spellings and unicode escapes.
const claudeJSON = `{
  "numStartups": 3,
  "zeta": "z",
  "alpha": {
    "nested": [
      1,
      2.50,
      1e3
    ],
    "empty": {},
    "none": []
  },
  "history": "pasted \ud83d broken <b>&amp;</b> é",
  "projects": {
    "/elsewhere": {
      "allowedTools": [],
      "hasTrustDialogAccepted": false
    }
  }
}
`

func TestRoundTripIsByteIdentical(t *testing.T) {
	for _, doc := range []string{claudeJSON, `{"a":1,"b":[true,null,"x"]}`, "{\n\t\"tab\": 1\n}\n", `[]`, `"s"`} {
		v, err := Parse([]byte(doc))
		if err != nil {
			t.Fatalf("Parse(%q): %v", doc, err)
		}
		if got := string(Format(v, Indent([]byte(doc)))); got != doc {
			t.Errorf("round trip changed the document:\n%s\nwant:\n%s", got, doc)
		}
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	for _, doc := range []string{``, `{`, `{"a":1}{}`, `{'a':1}`, `[1,]`} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("Parse(%q) succeeded", doc)
		}
	}
}

func TestDuplicate(t *testing.T) {
	v, _ := Parse([]byte(`{"a":{"b":1,"b":2}}`))
	if k, ok := v.Duplicate(); !ok || k != "b" {
		t.Errorf("Duplicate() = %q, %v", k, ok)
	}
	v, _ = Parse([]byte(`{"a":{"b":1},"c":[{"b":2}]}`))
	if _, ok := v.Duplicate(); ok {
		t.Error("a key in two objects counted as a duplicate")
	}
	v, _ = Parse([]byte(`{"a":1,"a":2}`))
	if string(v.Get("a").Raw) != "2" {
		t.Error("Get does not return the last of a repeated key")
	}
}

func writeConfig(t *testing.T, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func trustOp(dirs ...string) Op { return Trust([]TrustEntry{{Name: "app", Dirs: dirs}}) }

func TestTrustKeepsEverythingElse(t *testing.T) {
	p := writeConfig(t, claudeJSON, 0o644)
	code, lines := Edit(p, Options{Backup: p + ".bak-cadre"}, trustOp("/w/app", "/typed/app"))
	if code != Changed || strings.Join(lines, ",") != "app\ttrusted" {
		t.Fatalf("Edit = %d %q", code, lines)
	}
	got, _ := os.ReadFile(p)
	want := strings.Replace(claudeJSON, `      "hasTrustDialogAccepted": false
    }
  }`, `      "hasTrustDialogAccepted": false
    },
    "/w/app": {
      "hasTrustDialogAccepted": true
    },
    "/typed/app": {
      "hasTrustDialogAccepted": true
    }
  }`, 1)
	if string(got) != want {
		t.Errorf("config after trust:\n%s\nwant:\n%s", got, want)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o644 {
		t.Errorf("mode changed to %v", st.Mode().Perm())
	}
	if bak, _ := os.ReadFile(p + ".bak-cadre"); string(bak) != claudeJSON {
		t.Error("the backup is not the original")
	}
	// Trusting again changes nothing and writes nothing.
	st1, _ := os.Stat(p)
	code, lines = Edit(p, Options{Backup: p + ".bak-cadre"}, trustOp("/w/app", "/typed/app"))
	st2, _ := os.Stat(p)
	if code != Unchanged || lines[0] != "app\talready" || !st1.ModTime().Equal(st2.ModTime()) {
		t.Errorf("second trust: %d %q, mtime changed %v", code, lines, !st1.ModTime().Equal(st2.ModTime()))
	}
	// The first backup is never overwritten.
	Edit(p, Options{Backup: p + ".bak-cadre"}, trustOp("/w/other"))
	if bak, _ := os.ReadFile(p + ".bak-cadre"); string(bak) != claudeJSON {
		t.Error("a later edit overwrote the first backup")
	}
}

func TestEditRefusals(t *testing.T) {
	dir := t.TempDir()
	if code, _ := Edit(filepath.Join(dir, "missing.json"), Options{}, trustOp("/x")); code != Missing {
		t.Errorf("missing file: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("the missing file was created")
	}
	for _, body := range []string{`not json`, `[1, 2]`, `{"projects": []}`, `{"projects": {"/x": 3}}`, ``} {
		p := writeConfig(t, body, 0o600)
		code, _ := Edit(p, Options{Backup: p + ".bak"}, trustOp("/x"))
		got, _ := os.ReadFile(p)
		if code != Unusable || string(got) != body {
			t.Errorf("%q: code %d, file now %q", body, code, got)
		}
		if _, err := os.Stat(p + ".bak"); err == nil {
			t.Errorf("%q: a backup was written for a file left alone", body)
		}
	}
	if os.Geteuid() != 0 {
		p := writeConfig(t, `{}`, 0o000)
		if code, _ := Edit(p, Options{}, trustOp("/x")); code != Unusable {
			t.Errorf("unreadable file: %d", code)
		}
	}
}

func TestEditFollowsASymlink(t *testing.T) {
	real := writeConfig(t, "{}\n", 0o600)
	link := filepath.Join(t.TempDir(), "link.json")
	os.Symlink(real, link)
	if code, _ := Edit(link, Options{}, trustOp("/x")); code != Changed {
		t.Fatalf("code %d", code)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a file")
	}
	if got, _ := os.ReadFile(real); !bytes.Contains(got, []byte(`"/x"`)) {
		t.Errorf("the target was not edited: %s", got)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	p := writeConfig(t, "{}", 0o600)
	code, _ := Edit(p, Options{Backup: p + ".bak", DryRun: true}, trustOp("/x"))
	got, _ := os.ReadFile(p)
	if code != Changed || string(got) != "{}" {
		t.Errorf("dry run: %d, file %q", code, got)
	}
	if _, err := os.Stat(p + ".bak"); err == nil {
		t.Error("a dry run wrote a backup")
	}
}

// raceHook writes a script that rewrites the config while Edit is between
// writing its new file and checking the old one: on the first attempt only,
// or on every attempt.
func raceHook(t *testing.T, always bool) string {
	t.Helper()
	cond := `[ "$1" = 1 ]`
	if always {
		cond = "true"
	}
	script := filepath.Join(t.TempDir(), "race.sh")
	body := "#!/bin/sh\nif " + cond + "; then printf '{\"writer\": %s}' \"$1\" > \"$2\"; fi\n"
	os.WriteFile(script, []byte(body), 0o755)
	return script
}

func TestEditRetriesWhenTheFileChanges(t *testing.T) {
	p := writeConfig(t, "{}", 0o600)
	t.Setenv("CADRE_TEST_JSON_EDIT_HOOK", raceHook(t, false))
	code, _ := Edit(p, Options{}, trustOp("/x"))
	got, _ := os.ReadFile(p)
	if code != Changed || !bytes.Contains(got, []byte(`"writer":1`)) || !bytes.Contains(got, []byte(`"/x"`)) {
		t.Errorf("code %d, file %s; want the other writer's change kept and the trust added", code, got)
	}
}

func TestEditGivesUpWhenTheFileKeepsChanging(t *testing.T) {
	p := writeConfig(t, "{}", 0o600)
	t.Setenv("CADRE_TEST_JSON_EDIT_HOOK", raceHook(t, true))
	code, _ := Edit(p, Options{}, trustOp("/x"))
	got, _ := os.ReadFile(p)
	if code != KeptChanged || string(got) != `{"writer": 3}` {
		t.Errorf("code %d, file %s; want 6 and the other writer's file", code, got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".cadre-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestUntrustRemovesOnlyTheKey(t *testing.T) {
	p := writeConfig(t, `{"projects": {"/a": {"allowedTools": ["x"], "hasTrustDialogAccepted": true}, "/b": {}}}`, 0o600)
	code, lines := Edit(p, Options{Backup: p + ".bak"}, Untrust([]TrustEntry{{Name: "a", Dirs: []string{"/a", "/missing"}}, {Name: "b", Dirs: []string{"/b"}}}))
	got, _ := os.ReadFile(p)
	if code != Changed || string(got) != `{"projects":{"/a":{"allowedTools":["x"]},"/b":{}}}` {
		t.Errorf("code %d, file %s", code, got)
	}
	if strings.Join(lines, ",") != "a\tuntrusted,b\tnot trusted" {
		t.Errorf("lines %q", lines)
	}
}

func isOrch(c string) bool { _, ok := OrchestratorHook(c); return ok }

func TestHooks(t *testing.T) {
	settings := `{
  "model": "x",
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "command": "bash /old/cadre/bin/orchestrator-hook.sh"}]},
      {"hooks": [{"type": "command", "command": "echo mine"}]}
    ],
    "Stop": []
  }
}`
	p := writeConfig(t, settings, 0o600)
	add := AddHook("/opt/bin/cadre hook orchestrator", isOrch)
	if code, _ := Edit(p, Options{Backup: p + ".bak"}, add); code != Changed {
		t.Fatalf("add: %d", code)
	}
	got, _ := os.ReadFile(p)
	root, _ := Parse(got)
	groups := root.Get("hooks").Get("SessionStart").Items
	if len(groups) != 2 {
		t.Fatalf("groups after add: %s", got)
	}
	if c, _ := groups[0].Get("hooks").Items[0].Get("command").Text(); c != "echo mine" {
		t.Errorf("the user's own hook was not kept first: %s", got)
	}
	if c, _ := groups[1].Get("hooks").Items[0].Get("command").Text(); c != "/opt/bin/cadre hook orchestrator" {
		t.Errorf("the new hook is %q", c)
	}
	if code, _ := Edit(p, Options{}, add); code != Unchanged {
		t.Errorf("adding again: %d", code)
	}

	ours := func(c string) bool { return c == "/opt/bin/cadre hook orchestrator" }
	code, lines := Edit(p, Options{Backup: p + ".bak-uninstall", FreshBackup: true}, Unhook(ours, isOrch))
	if code != Changed || strings.Join(lines, ",") != "removed\t/opt/bin/cadre hook orchestrator" {
		t.Errorf("unhook: %d %q", code, lines)
	}
	got, _ = os.ReadFile(p)
	if bytes.Contains(got, []byte("orchestrator")) || !bytes.Contains(got, []byte("echo mine")) || !bytes.Contains(got, []byte(`"Stop": []`)) {
		t.Errorf("after unhook: %s", got)
	}
}

func TestUnhookReportsAnotherFrameworksHooks(t *testing.T) {
	p := writeConfig(t, `{"hooks": {"SessionStart": [{"hooks": [
		{"command": "bash /mine/bin/orchestrator-hook.sh"},
		{"command": "bash /elsewhere/my\\ cadre/bin/orchestrator-hook.sh"}]}]}}`, 0o600)
	ours := func(c string) bool { s, _ := OrchestratorHook(c); return s == "/mine/bin/orchestrator-hook.sh" }
	code, lines := Edit(p, Options{}, Unhook(ours, isOrch))
	want := "removed\tbash /mine/bin/orchestrator-hook.sh,kept\tbash /elsewhere/my\\ cadre/bin/orchestrator-hook.sh"
	if code != Changed || strings.Join(lines, ",") != want {
		t.Errorf("code %d, lines %q", code, lines)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Contains(got, []byte("elsewhere")) {
		t.Errorf("another framework's hook was removed: %s", got)
	}
	// Only hooks that are not ours, or none at all: nothing to do.
	p = writeConfig(t, `{"model": "x"}`, 0o600)
	if code, _ := Edit(p, Options{}, Unhook(ours, isOrch)); code != Unchanged {
		t.Errorf("no hooks: %d", code)
	}
	p = writeConfig(t, `{"hooks": {"SessionStart": {}}}`, 0o600)
	if code, _ := Edit(p, Options{}, Unhook(ours, isOrch)); code != Unusable {
		t.Errorf("SessionStart not a list: %d", code)
	}
}

func TestOrchestratorHook(t *testing.T) {
	for _, tc := range []struct {
		cmd, want string
		ok        bool
	}{
		{"bash /x/bin/orchestrator-hook.sh", "/x/bin/orchestrator-hook.sh", true},
		{`bash /x/my\ cadre/bin/orchestrator-hook.sh`, "/x/my cadre/bin/orchestrator-hook.sh", true},
		{`bash '/x/it''s/orchestrator-hook.sh'`, "/x/its/orchestrator-hook.sh", true},
		{"/opt/homebrew/opt/cadre/bin/cadre hook orchestrator", "/opt/homebrew/opt/cadre/bin/cadre", true},
		{"bash /x/my-orchestrator-hook.sh", "", false},
		{"/x/cadre hook statusline", "", false},
		{"sh /x/orchestrator-hook.sh", "", false},
		{"bash 'unclosed", "", false},
	} {
		got, ok := OrchestratorHook(tc.cmd)
		if got != tc.want || ok != tc.ok {
			t.Errorf("OrchestratorHook(%q) = %q, %v", tc.cmd, got, ok)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	f.Add([]byte(claudeJSON))
	f.Add([]byte(`{"a":[1,{"b":"\ud800"}]}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		v, err := Parse(data)
		if err != nil {
			return
		}
		out := Format(v, Indent(data))
		w, err := Parse(out)
		if err != nil {
			t.Fatalf("Format wrote invalid JSON: %q", out)
		}
		if !Equal(v, w) {
			t.Fatalf("a round trip changed the document: %q -> %q", data, out)
		}
	})
}
