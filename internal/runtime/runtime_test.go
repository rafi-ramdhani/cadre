package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type testRuntime struct{ caps Capabilities }

func (t testRuntime) Name() string                       { return "test" }
func (t testRuntime) Title() string                      { return "Test" }
func (t testRuntime) Caps() Capabilities                 { return t.caps }
func (t testRuntime) Detect() (Install, error)           { return Install{}, nil }
func (t testRuntime) Launch(LaunchSpec) (Command, error) { return Command{}, nil }
func (t testRuntime) BuildDir(string) string             { return "" }
func (t testRuntime) Permissions() PermissionOps         { return nil }
func (t testRuntime) Trust() TrustOps                    { return nil }

func TestForFollowsTheLayers(t *testing.T) {
	c := t.TempDir()
	team := filepath.Join(c, "personas", "dev")
	os.MkdirAll(team, 0o755)
	RegisterDefault(testRuntime{})
	defer func() { delete(registry, "test"); defaultName = "" }()
	if For(c, "dev", "pm", "") != "test" {
		t.Error("the default runtime is not used")
	}
	if For(c, "dev", "pm", "codex") != "codex" {
		t.Error("RUNTIME in cadre.conf is not used")
	}
	os.WriteFile(filepath.Join(team, ".runtime"), []byte("agy\n"), 0o644)
	if For(c, "dev", "pm", "codex") != "agy" {
		t.Error("the team's .runtime does not win over RUNTIME")
	}
	os.WriteFile(filepath.Join(team, "pm.runtime"), []byte("  fake  \nignored\n"), 0o644)
	if For(c, "dev", "pm", "codex") != "fake" || For(c, "dev", "eng", "codex") != "agy" {
		t.Error("the role's .runtime does not win, or leaks to another role")
	}
}

func TestGetRefusesAnUnknownRuntime(t *testing.T) {
	Register(testRuntime{})
	defer delete(registry, "test")
	if _, err := Get("test"); err != nil {
		t.Fatal(err)
	}
	_, err := Get("codex")
	if err == nil || !strings.HasPrefix(err.Error(), "runtime codex is not supported yet (supported: ") {
		t.Errorf("Get(codex): %v", err)
	}
}

func TestUsable(t *testing.T) {
	ok := Capabilities{FixedDenies: true, Messaging: Native, PermissionModes: []string{"default"}}
	if err := Usable(testRuntime{ok}, "default"); err != nil {
		t.Errorf("a usable runtime: %v", err)
	}
	noDeny := ok
	noDeny.FixedDenies = false
	if err := Usable(testRuntime{noDeny}, "default"); err == nil || !strings.Contains(err.Error(), "fixed denies") {
		t.Errorf("no fixed denies: %v", err)
	}
	noMsg := ok
	noMsg.Messaging = NoMessaging
	if err := Usable(testRuntime{noMsg}, "default"); err == nil || !strings.Contains(err.Error(), "message the orchestrator") {
		t.Errorf("no messaging: %v", err)
	}
	if err := Usable(testRuntime{ok}, "yolo"); err == nil || !strings.Contains(err.Error(), "no permission mode yolo") {
		t.Errorf("an unknown mode: %v", err)
	}
}
