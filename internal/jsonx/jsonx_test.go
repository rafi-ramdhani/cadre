package jsonx

import (
	"bytes"
	"fmt"
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

// trustOp stands in for a real edit in these tests of Edit itself: it adds
// projects[<dir>] = {"ok": true}, with the shape checks a real edit has.
func trustOp(dirs ...string) Op {
	return func(root *Value) ([]string, bool, error) {
		if root.Kind != Object {
			return nil, false, &ShapeError{What: "the top level is not an object"}
		}
		projects := root.Get("projects")
		if projects == nil {
			projects = NewObject()
			root.Set("projects", projects)
		}
		if projects.Kind != Object {
			return nil, false, &ShapeError{What: "projects is not an object"}
		}
		changed := false
		for _, d := range dirs {
			entry := projects.Get(d)
			if entry == nil {
				entry = NewObject()
				projects.Set(d, entry)
			}
			if entry.Kind != Object {
				return nil, false, &ShapeError{What: "an entry is not an object"}
			}
			if !entry.Get("ok").IsTrue() {
				entry.Set("ok", Literal("true"))
				changed = true
			}
		}
		return []string{"app\tdone"}, changed, nil
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

// race makes Edit see another writer rewrite the config between writing
// its new file and checking the old one: on the first attempt only, or on
// every attempt.
func race(t *testing.T, always bool) {
	t.Helper()
	raceHook = func(n int, path string) {
		if always || n == 1 {
			os.WriteFile(path, []byte(fmt.Sprintf(`{"writer": %d}`, n)), 0o600)
		}
	}
	t.Cleanup(func() { raceHook = nil })
}

func TestEditRetriesWhenTheFileChanges(t *testing.T) {
	p := writeConfig(t, "{}", 0o600)
	race(t, false)
	code, _ := Edit(p, Options{}, trustOp("/x"))
	got, _ := os.ReadFile(p)
	if code != Changed || !bytes.Contains(got, []byte(`"writer":1`)) || !bytes.Contains(got, []byte(`"/x"`)) {
		t.Errorf("code %d, file %s; want the other writer's change kept and the trust added", code, got)
	}
}

func TestEditGivesUpWhenTheFileKeepsChanging(t *testing.T) {
	p := writeConfig(t, "{}", 0o600)
	race(t, true)
	code, _ := Edit(p, Options{}, trustOp("/x"))
	got, _ := os.ReadFile(p)
	if code != KeptChanged || string(got) != `{"writer": 3}` {
		t.Errorf("code %d, file %s; want 6 and the other writer's file", code, got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".cadrei-*")); len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
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

// Claude Code writes ~/.claude.json with JSON.stringify, which ends without
// a newline; an edit keeps that, and keeps a newline when there is one.
func TestEditKeepsTheFinalNewlineState(t *testing.T) {
	for _, body := range []string{"{\n  \"a\": 1\n}", "{\n  \"a\": 1\n}\n", `{"a":1}`} {
		p := writeConfig(t, body, 0o600)
		if code, _ := Edit(p, Options{}, trustOp("/x")); code != Changed {
			t.Fatalf("%q: %d", body, code)
		}
		got, _ := os.ReadFile(p)
		if strings.HasSuffix(string(got), "\n") != strings.HasSuffix(body, "\n") {
			t.Errorf("%q became %q", body, got)
		}
	}
}
