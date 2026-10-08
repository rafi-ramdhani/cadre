package session

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// private gives a test its own tmux server, stopped at the end.
func private(t *testing.T) Tmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	tm := Tmux{Socket: fmt.Sprintf("cadre-gotest-%d-%d", os.Getpid(), time.Now().UnixNano())}
	t.Cleanup(func() { tm.command("kill-server").Run() })
	return tm
}

// cadreDir makes a cadre folder with a team dev (pm, engineer) and returns
// it, and a stub claude that records its arguments and environment.
func cadreDir(t *testing.T, name string) (string, string) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	c := filepath.Join(root, name)
	for _, r := range []string{"pm", "engineer"} {
		os.MkdirAll(filepath.Join(c, "personas", "dev"), 0o755)
		os.WriteFile(filepath.Join(c, "personas", "dev", r+".md"), []byte("# "+r+"\n"), 0o644)
	}
	stub := filepath.Join(root, "claude")
	os.WriteFile(stub, []byte("#!/bin/sh\n{ printf '%s\\n' \"$@\"; echo \"HOME=$CADRE_HOME\"; echo \"PERSONA=$CADRE_PERSONA\"; } > \"$0.$CADRE_PERSONA\"\nexec sleep 300\n"), 0o755)
	return c, stub
}

// stubRuntime runs a stub program in place of an agent CLI.
type stubRuntime struct{ bin string }

func (s stubRuntime) Name() string                       { return "stub" }
func (s stubRuntime) Title() string                      { return "Stub" }
func (s stubRuntime) Caps() runtime.Capabilities         { return runtime.Capabilities{} }
func (s stubRuntime) Detect() (runtime.Install, error)   { return runtime.Install{Path: s.bin}, nil }
func (s stubRuntime) BuildDir(cadre string) string       { return filepath.Join(cadre, ".build") }
func (s stubRuntime) Permissions() runtime.PermissionOps { return nil }
func (s stubRuntime) Trust() runtime.TrustOps            { return nil }
func (s stubRuntime) Launch(l runtime.LaunchSpec) (runtime.Command, error) {
	return runtime.Command{Argv: []string{s.bin, "--name", l.Name, "--mode", l.Mode, "--prompt", l.PromptFile}}, nil
}

func pick(bin string) func(string) (runtime.Runtime, string, error) {
	return func(string) (runtime.Runtime, string, error) { return stubRuntime{bin}, "", nil }
}

func up(tm Tmux, cadre, stub, team, role, project string) (string, bool) {
	u := Up{Scope: Scope{Name: filepath.Base(cadre), Path: cadre, T: tm}, Team: team, Role: role, Project: project,
		Dir: DefaultDir(cadre, team), Protocol: []byte("PROTOCOL\n"), Mode: "default", Pick: pick(stub), Wait: 300 * time.Millisecond}
	var out bytes.Buffer
	failed := u.Start(&out)
	return out.String(), failed
}

func TestNames(t *testing.T) {
	if SessionName("work", Key("dev", "app")) != "cadre-work-dev-app" || PersonaName("work", Key("dev", ""), "pm") != "work-dev-pm" || LegacyName("dev-app") != "cadre-dev-app" {
		t.Error("names")
	}
	if Quote("it's a dir") != `'it'\''s a dir'` || Quote("/x/y.md") != "/x/y.md" || Quote("") != "''" {
		t.Error("Quote")
	}
}

func TestUpStartsPersonasWithTheirCadre(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "work")
	out, failed := up(tm, c, stub, "dev", "", "")
	if failed || !strings.Contains(out, "work-dev-engineer started in "+c+"/teams/dev") || !strings.Contains(out, "work-dev-pm started") {
		t.Fatalf("up: %v %q", failed, out)
	}
	s := "cadre-work-dev"
	if tm.Option(s, "@cadre_home") != c || tm.Option(s, "@cadre_team") != "dev" || tm.Option(s, "@cadre_project") != "" {
		t.Errorf("options: %q %q", tm.Option(s, "@cadre_home"), tm.Option(s, "@cadre_team"))
	}
	args := waitFile(t, stub+".work-dev-pm")
	for _, want := range []string{"--name\nwork-dev-pm\n", "--mode\ndefault\n", "HOME=" + c + "\n", "PERSONA=work-dev-pm\n"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("the persona was started without %q:\n%s", want, args)
		}
	}
	prompt, _ := os.ReadFile(filepath.Join(c, ".build", "dev-pm.md"))
	if string(prompt) != "PROTOCOL\n\n# pm\n" {
		t.Errorf("prompt %q", prompt)
	}
	if out, _ := up(tm, c, stub, "dev", "pm", ""); !strings.Contains(out, "work-dev-pm already running") {
		t.Errorf("second up: %q", out)
	}
	scope := Scope{Name: "work", Path: c, T: tm}
	if r := scope.Running(); len(r) != 1 || r[0].Name != s {
		t.Errorf("Running: %+v", r)
	}
	if names := tm.PersonaNames(scope.Running()[0]); strings.Join(names, ",") != "work-dev-engineer,work-dev-pm" {
		t.Errorf("PersonaNames %v", names)
	}
	if line := scope.StopRole("dev", "pm"); line != "  work-dev-pm stopped" {
		t.Errorf("StopRole: %q", line)
	}
	if line := scope.StopTeam("dev"); line != "  cadre-work-dev stopped" {
		t.Errorf("StopTeam: %q", line)
	}
	if line := scope.StopTeam("dev"); line != "  cadre-work-dev not running" {
		t.Errorf("StopTeam again: %q", line)
	}
}

func TestAFailedStartIsReported(t *testing.T) {
	tm := private(t)
	c, _ := cadreDir(t, "work")
	bad := filepath.Join(filepath.Dir(c), "bad")
	os.WriteFile(bad, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	tm.command("new-session", "-d", "-s", "keepalive", "sleep", "60").Run()
	u := Up{Scope: Scope{Name: "work", Path: c, T: tm}, Team: "dev", Role: "pm", Dir: c, Mode: "default", Pick: pick(bad), Wait: 2 * time.Second}
	var buf bytes.Buffer
	failed := u.Start(&buf)
	out := buf.String()
	if !failed || !strings.Contains(out, "work-dev-pm failed to start") || !strings.Contains(out, "CADRE_PERSONA=work-dev-pm") {
		t.Errorf("failed start: %v %q", failed, out)
	}
}

func TestSessionCollisions(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "b")
	other := filepath.Join(filepath.Dir(c), "a")
	scope := Scope{Name: "b", Path: c, T: tm}
	// Another cadre holds the name.
	tm.command("new-session", "-d", "-s", "cadre-b-dev", "sleep", "60").Run()
	tm.SetOption("cadre-b-dev", "@cadre_home", other)
	if err := scope.CheckSession("dev", ""); err == nil || !strings.Contains(err.Error(), "belongs to cadre a ("+other+"), not cadre b") {
		t.Errorf("another cadre: %v", err)
	}
	// A legacy session holds it.
	tm.SetOption("cadre-b-dev", "@cadre_home", "")
	tm.command("set-option", "-u", "-t", "=cadre-b-dev:", "@cadre_home").Run()
	if err := scope.CheckSession("dev", ""); err == nil || !strings.Contains(err.Error(), "legacy session") {
		t.Errorf("legacy: %v", err)
	}
	tm.KillSession("cadre-b-dev")
	// Team dev with project x-y and team dev-x with project y join the same way.
	up(tm, c, stub, "dev", "pm", "x-y")
	if err := scope.CheckSession("dev-x", "y"); err == nil || !strings.Contains(err.Error(), "join to the same session name") {
		t.Errorf("joined names: %v", err)
	}
	if err := scope.CheckSession("dev", "x-y"); err != nil {
		t.Errorf("its own session: %v", err)
	}
}

// The reviewer's exp32: names that join the same way in different tmux
// sessions. The tmux session check cannot see these; the Claude name check
// does.
func TestClaudeNamesCannotCollide(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "c")
	for _, r := range []string{"x-y"} {
		os.WriteFile(filepath.Join(c, "personas", "dev", r+".md"), []byte("# "+r+"\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(c, "personas", "dev-x"), 0o755)
	os.WriteFile(filepath.Join(c, "personas", "dev-x", "y.md"), []byte("# y\n"), 0o644)
	if out, failed := up(tm, c, stub, "dev", "x-y", ""); failed {
		t.Fatalf("first: %q", out)
	}
	out, failed := up(tm, c, stub, "dev-x", "y", "")
	if !failed || !strings.Contains(out, "the Claude session name c-dev-x-y is already used by window x-y of tmux session cadre-c-dev") {
		t.Errorf("one cadre, two teams: %v %q", failed, out)
	}
	// Across cadres: a with team x and role y-z, a-x with team y and role z.
	a, stubA := cadreDir(t, "a")
	os.MkdirAll(filepath.Join(a, "personas", "x"), 0o755)
	os.WriteFile(filepath.Join(a, "personas", "x", "y-z.md"), []byte("# y-z\n"), 0o644)
	ax, stubAX := cadreDir(t, "a-x")
	os.MkdirAll(filepath.Join(ax, "personas", "y"), 0o755)
	os.WriteFile(filepath.Join(ax, "personas", "y", "z.md"), []byte("# z\n"), 0o644)
	if out, failed := up(tm, a, stubA, "x", "y-z", ""); failed {
		t.Fatalf("a: %q", out)
	}
	out, failed = up(tm, ax, stubAX, "y", "z", "")
	if !failed || !strings.Contains(out, "a-x-y-z is already used") {
		t.Errorf("two cadres: %v %q", failed, out)
	}
}

func TestLegacySessions(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "work")
	tm.command("new-session", "-d", "-s", "cadre-dev", "-n", "pm", "sleep", "60").Run()
	def := Scope{Name: "work", Path: c, T: tm, Default: true}
	other := Scope{Name: "work", Path: c, T: tm}
	if def.Live("dev") != "cadre-dev" || other.Live("dev") != "" {
		t.Errorf("Live: default %q, other %q", def.Live("dev"), other.Live("dev"))
	}
	if len(def.Running()) != 1 || len(other.Running()) != 0 {
		t.Errorf("Running: default %d, other %d", len(def.Running()), len(other.Running()))
	}
	u := Up{Scope: def, Team: "dev", Role: "pm", Dir: c, Protocol: nil, Mode: "default", Pick: pick(stub), Wait: 300 * time.Millisecond}
	var out bytes.Buffer
	u.Start(&out)
	if !strings.Contains(out.String(), "dev-pm already running (legacy session cadre-dev") {
		t.Errorf("up with a legacy session: %q", out.String())
	}
	if line := def.StopTeam("dev"); line != "  cadre-dev stopped" {
		t.Errorf("stop a legacy team: %q", line)
	}
}

func TestAnOrchestratorSessionIsNotAPersonaSession(t *testing.T) {
	tm := private(t)
	c, _ := cadreDir(t, "work")
	tm.command("new-session", "-d", "-s", "cadre-work", "sleep", "60").Run()
	tm.SetOption("cadre-work", "@cadre_home", c)
	tm.SetOption("cadre-work", "@cadre_role", "orchestrator")
	if r := (Scope{Name: "work", Path: c, T: tm}).Running(); len(r) != 0 {
		t.Errorf("Running lists the orchestrator: %+v", r)
	}
	if len(All(tm, nil)) != 0 {
		t.Error("All lists the orchestrator")
	}
}

func TestVersion(t *testing.T) {
	tm := private(t)
	major, _, err := tm.Version()
	if err != nil || major < 3 {
		t.Errorf("tmux version %d: %v", major, err)
	}
}

// waitFile waits for a stub to write its file, under load too.
func waitFile(t *testing.T, path string) string {
	t.Helper()
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 && bytes.Contains(b, []byte("PERSONA=")) {
			return string(b)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s was not written", path)
	return ""
}
