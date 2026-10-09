package runtime

import (
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
func (t testRuntime) Health(bool, []string) []Problem    { return nil }
func (t testRuntime) Instructions() InstructionOps       { return nil }
func (t testRuntime) Hooks() HookOps                     { return nil }

func TestTheDefaultRuntime(t *testing.T) {
	RegisterDefault(testRuntime{})
	defer func() { delete(registry, "test"); defaultName = "" }()
	if Default() != "test" || len(Supported()) != 1 {
		t.Errorf("default %q, supported %v", Default(), Supported())
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
