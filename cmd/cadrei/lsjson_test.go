//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// testdata is found before a test changes the current folder.
var testdata, _ = filepath.Abs("testdata")

// shape describes a JSON value by its keys and types, not its values:
// objects as {key: shape}, arrays by the shape of their elements.
func shape(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if m, ok := e.(map[string]any); ok && k == "teams" {
				// Keyed by team names, which are data: the shape is its values'.
				var vals []any
				for _, v := range m {
					vals = append(vals, v)
				}
				out[k] = "map of " + shape(vals).(string)
				continue
			}
			out[k] = shape(e)
		}
		return out
	case []any:
		var shapes []string
		seen := map[string]bool{}
		for _, e := range x {
			b, _ := json.Marshal(shape(e))
			if !seen[string(b)] {
				seen[string(b)] = true
				shapes = append(shapes, string(b))
			}
		}
		sort.Strings(shapes)
		return "array of " + strings.Join(shapes, " | ")
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return "?"
}

func checkShape(t *testing.T, name, out string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	got, _ := json.MarshalIndent(shape(v), "", "  ")
	file := filepath.Join(testdata, name)
	if os.Getenv("UPDATE_SHAPE") == "1" {
		os.WriteFile(file, append(got, '\n'), 0o644)
	}
	want, _ := os.ReadFile(file)
	if strings.TrimSpace(string(got)) != strings.TrimSpace(string(want)) {
		t.Errorf("the shape of %s changed (version it, or run with UPDATE_SHAPE=1 for an added field):\n%s", name, got)
	}
}

// The ls --json shape is what the orchestrator reads: a change must be
// deliberate.
func TestLsJSONShape(t *testing.T) {
	home := sandbox(t)
	socket := withTmux(t, home)
	must(t, "init", "work")
	must(t, "init", "life")
	t.Chdir(home + "/.cadrei/work")
	os.MkdirAll(home+"/Developer/app", 0o755)
	register(t, home, "work", "app:\n  repo: me/app\n  team: dev\n  about: the app\n  path: ~/Developer/app\n")
	must(t, "up", "dev/engineer")
	tmuxIn(socket, "new-session", "-d", "-s", "cadre-dev", "-n", "pm", "sleep", "60")
	// A mode the runtime lacks, so problems has an element.
	os.WriteFile(home+"/.cadrei/work/cadrei.conf", []byte("PERMISSION_MODE=yolo\n"), 0o644)
	checkShape(t, "ls-shape.json", must(t, "ls", "--json"))
	checkShape(t, "ls-all-shape.json", must(t, "ls", "--all", "--json"))
	if out := must(t, "ls", "--json"); !strings.Contains(out, `"runtime claude has no permission mode yolo`) {
		t.Errorf("no problem listed: %s", out)
	}
	if out := must(t, "ls", "--json"); !strings.HasPrefix(out, "{\n  \"version\": 1,") {
		t.Errorf("version first: %s", out[:40])
	}
}
