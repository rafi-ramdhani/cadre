package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/runtime/claude/settings"
)

// A broken settings file (one shell write does it) must never leave a
// member without the deny rules: it gets a deny-only copy.
func TestPrepareWithABrokenFileStillDenies(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	cadre := filepath.Join(home, ".cadre", "work")
	os.MkdirAll(filepath.Join(cadre, "members"), 0o755)
	places := runtime.Places{Root: home + "/.cadre", Cadres: []string{cadre, "/Docs/outside"}}
	p := permissions{}
	if got := p.Prepare(cadre, places); got.Grants == "" || len(got.Warnings) != 0 {
		t.Fatalf("a new file: %+v", got)
	}
	file := p.GrantsFile(cadre)
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("}")
	f.Close()
	got := p.Prepare(cadre, places)
	if got.Grants == "" {
		t.Fatal("a broken file gave no copy: the member would start without any deny rule")
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "no grants, only cadre's deny rules") {
		t.Errorf("warnings %q", got.Warnings)
	}
	raw, _ := os.ReadFile(got.Grants)
	text := string(raw)
	for _, d := range append(append([]string{}, settings.FixedDeny...), "Edit(/"+cadre+"/members/**)", "Edit(//Docs/outside/cadre.conf)", "Edit(/"+home+"/.cadre/config/**)") {
		if !strings.Contains(text, `"`+d+`"`) {
			t.Errorf("the deny-only copy lacks %s", d)
		}
	}
	if !strings.Contains(text, settings.FixedSoft) || !strings.Contains(text, settings.CadreSoft) {
		t.Error("the deny-only copy lacks a soft_deny line")
	}
	if !strings.Contains(text, `"allow": []`) {
		t.Errorf("the deny-only copy grants something:\n%s", text)
	}
}
