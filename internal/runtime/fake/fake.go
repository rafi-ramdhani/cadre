//go:build cadretest

// Package fake is a runtime for tests only (section P.8): it is compiled
// only with -tags cadretest, never into a release binary. Its capabilities
// can be switched off per test, or by CADRE_FAKE_OFF (a comma-separated
// list of capability names) for the smoke test, and it launches
// CADRE_FAKE_BIN, a stub that records its arguments and environment.
package fake

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

func init() { runtime.Register(Fake{}) }

// Off lists capabilities switched off, for unit tests.
var Off = map[string]bool{}

// Fake is the test runtime.
type Fake struct{}

func (Fake) Name() string  { return "fake" }
func (Fake) Title() string { return "the fake runtime" }

func off(name string) bool {
	if Off[name] {
		return true
	}
	for _, n := range strings.Split(os.Getenv("CADRE_FAKE_OFF"), ",") {
		if strings.TrimSpace(n) == name {
			return true
		}
	}
	return false
}

func (Fake) Caps() runtime.Capabilities {
	c := runtime.Capabilities{
		Grants: !off("Grants"), AutoModeText: !off("AutoModeText"), FixedDenies: !off("FixedDenies"),
		PermissionModes: []string{"default", "fake-mode"},
		ContextUsage:    !off("ContextUsage"), Compact: !off("Compact"), StateSignals: !off("StateSignals"),
		Resume: !off("Resume"), AssignSessionID: !off("AssignSessionID"), Trust: !off("Trust"),
		Instructions: !off("Instructions"), OrchestratorHook: !off("OrchestratorHook"), Messaging: runtime.Native,
	}
	if off("Messaging") {
		c.Messaging = runtime.NoMessaging
	}
	return c
}

func (Fake) Detect() (runtime.Install, error) {
	return runtime.Install{Path: os.Getenv("CADRE_FAKE_BIN"), Version: "0.0.0-fake"}, nil
}

// Launch returns a command whose every part comes from the spec, so a test
// can check that what runs in tmux is what Launch built.
func (f Fake) Launch(s runtime.LaunchSpec) (runtime.Command, error) {
	in, _ := f.Detect()
	return runtime.Command{
		Argv: []string{in.Path, "--fake-name", s.Name, "--fake-mode", s.Mode, "--fake-prompt", s.PromptFile, "--fake-grants", s.Grants},
		Env:  []string{"CADRE_FAKE_LAUNCHED=" + s.Name},
		Dir:  s.WorkDir,
	}, nil
}

func (Fake) BuildDir(cadre string) string       { return filepath.Join(cadre, ".fake", "build") }
func (Fake) Permissions() runtime.PermissionOps { return permissions{} }
func (Fake) Trust() runtime.TrustOps            { return trust{} }

type permissions struct{}

func (permissions) GrantsFile(cadre string) string { return filepath.Join(cadre, ".fake", "grants") }
func (permissions) Validate(cadre string, known []string, rule string, auto bool) (string, error) {
	return "", nil
}
func (permissions) Prepare(cadre string, places runtime.Places) runtime.Prepared {
	return runtime.Prepared{Grants: "fake-grants"}
}

type trust struct{}

func (trust) Mark(f []runtime.Folder) ([]runtime.TrustResult, string)   { return nil, "" }
func (trust) Unmark(f []runtime.Folder) ([]runtime.TrustResult, string) { return nil, "" }
func (trust) Protected() []string                                       { return nil }
