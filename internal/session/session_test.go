package session

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/testguard"
)

// private gives a test its own tmux server, stopped at the end.
func private(t *testing.T) Tmux {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	testguard.Check(t)
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
		os.MkdirAll(filepath.Join(c, "members", "dev"), 0o755)
		os.WriteFile(filepath.Join(c, "members", "dev", r+".md"), []byte("# "+r+"\n"), 0o644)
	}
	stub := filepath.Join(root, "claude")
	os.WriteFile(stub, []byte("#!/bin/sh\n{ printf '%s\\n' \"$@\"; echo \"HOME=$CADRE_HOME\"; echo \"MEMBER=$CADRE_MEMBER\"; } > \"$0.$CADRE_MEMBER\"\nexec sleep 300\n"), 0o755)
	return c, stub
}

// stubRuntime runs a stub program in place of an agent CLI.
type stubRuntime struct{ bin string }

func (s stubRuntime) Name() string                            { return "stub" }
func (s stubRuntime) Title() string                           { return "Stub" }
func (s stubRuntime) Caps() runtime.Capabilities              { return runtime.Capabilities{} }
func (s stubRuntime) Detect() (runtime.Install, error)        { return runtime.Install{Path: s.bin}, nil }
func (s stubRuntime) BuildDir(cadre string) string            { return filepath.Join(cadre, ".build") }
func (s stubRuntime) Permissions() runtime.PermissionOps      { return nil }
func (s stubRuntime) Trust() runtime.TrustOps                 { return nil }
func (s stubRuntime) Health(bool, []string) []runtime.Problem { return nil }
func (s stubRuntime) Instructions() runtime.InstructionOps    { return nil }
func (s stubRuntime) Hooks() runtime.HookOps                  { return nil }
func (s stubRuntime) Sessions() runtime.SessionOps            { return nil }
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
	if SessionName("work", Key("dev", "app")) != "cadre-work-dev-app" || MemberName("work", Key("dev", ""), "pm") != "work-dev-pm" || LegacyName("dev-app") != "cadre-dev-app" {
		t.Error("names")
	}
	if Quote("it's a dir") != `'it'\''s a dir'` || Quote("/x/y.md") != "/x/y.md" || Quote("") != "''" {
		t.Error("Quote")
	}
}

func TestUpStartsMembersWithTheirCadre(t *testing.T) {
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
	for _, want := range []string{"--name\nwork-dev-pm\n", "--mode\ndefault\n", "HOME=" + c + "\n", "MEMBER=work-dev-pm\n"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("the member was started without %q:\n%s", want, args)
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
	if names := tm.MemberNames(scope.Running()[0]); strings.Join(names, ",") != "work-dev-engineer,work-dev-pm" {
		t.Errorf("MemberNames %v", names)
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
	if !failed || !strings.Contains(out, "work-dev-pm failed to start") || !strings.Contains(out, "CADRE_MEMBER=work-dev-pm") {
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
		os.WriteFile(filepath.Join(c, "members", "dev", r+".md"), []byte("# "+r+"\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(c, "members", "dev-x"), 0o755)
	os.WriteFile(filepath.Join(c, "members", "dev-x", "y.md"), []byte("# y\n"), 0o644)
	if out, failed := up(tm, c, stub, "dev", "x-y", ""); failed {
		t.Fatalf("first: %q", out)
	}
	out, failed := up(tm, c, stub, "dev-x", "y", "")
	if !failed || !strings.Contains(out, "the session name c-dev-x-y is already used by window x-y of tmux session cadre-c-dev") {
		t.Errorf("one cadre, two teams: %v %q", failed, out)
	}
	// Across cadres: a with team x and role y-z, a-x with team y and role z.
	a, stubA := cadreDir(t, "a")
	os.MkdirAll(filepath.Join(a, "members", "x"), 0o755)
	os.WriteFile(filepath.Join(a, "members", "x", "y-z.md"), []byte("# y-z\n"), 0o644)
	ax, stubAX := cadreDir(t, "a-x")
	os.MkdirAll(filepath.Join(ax, "members", "y"), 0o755)
	os.WriteFile(filepath.Join(ax, "members", "y", "z.md"), []byte("# z\n"), 0o644)
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

func TestAnOrchestratorSessionIsNotAMemberSession(t *testing.T) {
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
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 && bytes.Contains(b, []byte("MEMBER=")) {
			return string(b)
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s was not written", path)
	return ""
}

func TestStartPassesEveryArgumentAsItIs(t *testing.T) {
	tm := private(t)
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe")
	os.WriteFile(probe, []byte("#!/bin/sh\nout=$1; shift\nprintf '%s|' \"$@\" > \"$out\"\nexec sleep 30\n"), 0o755)
	out := filepath.Join(dir, "out")
	// tmux splits at an argument that is or ends with ";".
	err := tm.Start(StartSpec{Session: "cadre-x", Window: "w", Dir: dir, Argv: []string{probe, out, "a;", ";", "x;y", "$HOME", "it's"},
		SessionOptions: []Option{{"@cadre_home", "/path;"}}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if b, _ := os.ReadFile(out); len(b) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if b, _ := os.ReadFile(out); string(b) != "a;|;|x;y|$HOME|it's|" {
		t.Errorf("the command got %q", b)
	}
	if tm.Option("cadre-x", "@cadre_home") != "/path;" {
		t.Errorf("option %q", tm.Option("cadre-x", "@cadre_home"))
	}
	if err := tm.Start(StartSpec{Session: "cadre-y", Window: "w", Dir: dir, Argv: []string{probe, `a\;`}}); err == nil {
		t.Error("an argument ending in a backslash and a semicolon was passed")
	}
	if err := tm.Start(StartSpec{Session: "cadre-z", Window: "w", Dir: dir, Argv: []string{"sleep"}}); err == nil {
		t.Error("a one-word command, which tmux runs through a shell, was started")
	}
}

func TestTeamSessionsCarryTheHint(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "work")
	up(tm, c, stub, "dev", "pm", "")
	if got := tm.Option("cadre-work-dev", "status-right"); got != "Ctrl-b then d: back to your terminal" {
		t.Errorf("status-right %q", got)
	}
	if out, _ := tm.run("show-hooks", "-t", "=cadre-work-dev:"); !strings.Contains(out, "client-attached") || !strings.Contains(out, "back to your terminal") {
		t.Errorf("hooks %q", out)
	}
	if out, _ := tm.run("show-options", "-gv", "status-right"); strings.Contains(out, "back to your terminal") {
		t.Error("the global status-right changed")
	}
	tm.run("set-option", "-g", "prefix", "C-a")
	if tm.Hint() != "Ctrl-a then d: back to your terminal" {
		t.Errorf("another prefix: %q", tm.Hint())
	}
}

func TestStartFolderIsNotAFormat(t *testing.T) {
	tm := private(t)
	base, _ := filepath.EvalSymlinks(t.TempDir())
	// tmux expands -c as a format; this folder must stay itself, for a
	// new session and for a new window in it.
	dir := filepath.Join(base, "a#{session_name}#(echo hi)##b")
	os.MkdirAll(dir, 0o755)
	for _, w := range []string{"one", "two"} {
		if err := tm.Start(StartSpec{Session: "cadre-hash", Window: w, Dir: dir, Argv: []string{"sleep", "30"}}); err != nil {
			t.Fatal(err)
		}
		got, _ := tm.run("display-message", "-p", "-t", "=cadre-hash:="+w, "#{pane_current_path}")
		if got != dir {
			t.Errorf("window %s started in %q, not %q", w, got, dir)
		}
	}
}

func TestRecordedValuesComeBackAsTheyAre(t *testing.T) {
	tm := private(t)
	dir := t.TempDir()
	for _, v := range []string{"/a\nb", "/a\rb", "/a\tb", "/a\x1fb", "/a\x7fb", "/a\xffb", `/a\$HOME`, `/a\${x}`, `/a\$é`} {
		err := tm.Start(StartSpec{Session: "cadre-bad", Window: "w", Dir: dir, Argv: []string{"sleep", "30"},
			SessionOptions: []Option{{"@cadre_home", v}}})
		if err == nil || tm.Has("cadre-bad") {
			t.Errorf("%q was recorded", v)
		}
		if err := tm.Start(StartSpec{Session: "cadre-bad", Window: "w", Dir: dir, Argv: []string{"sleep", "30"},
			WindowOptions: []Option{{"@cadre_member", v}}}); err == nil {
			t.Errorf("%q was recorded on a window", v)
		}
		tm.KillSession("cadre-bad")
	}
	// What may be recorded is read back exactly, even in a C locale, where
	// tmux would turn non-ASCII characters into "_" for a client it does
	// not know to be UTF-8.
	for _, v := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		t.Setenv(v, "C")
	}
	home := `/Users/josé/my cadre;\ "x" 'y' $HOME #{z} 日本`
	err := tm.Start(StartSpec{Session: "cadre-ok", Window: "w-1", Dir: dir, Argv: []string{"sleep", "30"},
		SessionOptions: []Option{{"@cadre_home", home}, {"@cadre_team", "dev"}, {"@cadre_target", "ü"}},
		WindowOptions:  []Option{{"@cadre_member", "ok-dev-w-1"}}})
	if err != nil {
		t.Fatal(err)
	}
	list := tm.Sessions()
	if len(list) != 1 || list[0].Home != home || list[0].Team != "dev" || list[0].Target != "ü" {
		t.Errorf("Sessions read back %+v", list)
	}
	if p := tm.Members(); len(p) != 1 || p[0] != (Member{"cadre-ok", "w-1", "ok-dev-w-1"}) {
		t.Errorf("Members read back %+v", p)
	}
	if tm.Option("cadre-ok", "@cadre_home") != home {
		t.Errorf("Option read back %q", tm.Option("cadre-ok", "@cadre_home"))
	}
	if tm.SetOption("cadre-ok", "@cadre_target", "x\ny") == nil || tm.SetWindowOption("cadre-ok", "w-1", "@cadre_member", "x\ty") == nil {
		t.Error("a control character was set")
	}
}

// tmux 3.4 writes command output through vis(3) with VIS_OCTAL|VIS_CSTYLE,
// which escapes every control character but tab and newline: a separator
// of any other control character (the old \x1f) never reaches cadre.
func TestTheSeparatorSurvivesTmuxOutput(t *testing.T) {
	for _, b := range []byte(sep) {
		if b != '\t' && (b < 0x20 || b >= 0x7f) {
			t.Errorf("separator byte %#x is escaped by tmux 3.4", b)
		}
	}
	if recordable("x", "a"+sep+"b") == nil {
		t.Error("a value holding the separator can be recorded")
	}
}

// tmux 3.4 writes "$" before a letter, "_" or "{" as "\$" (utf8_strvis);
// later versions write it as it is. Both read back as the value.
func TestDollarsReadBackFromEitherTmux(t *testing.T) {
	for _, v := range []string{`$HOME/x`, `a$_b`, `${x}`, `$1 $ $`, `a\b`, `\$1`, `x$`, `\`, `/a/$é`, `$日本`, `€$€`} {
		if err := recordable("v", v); err != nil {
			t.Fatalf("%q refused: %v", v, err)
		}
		var escaped strings.Builder
		for i := 0; i < len(v); i++ {
			if v[i] == '$' && i+1 < len(v) && dollarEscaped(v[i+1]) {
				escaped.WriteByte('\\')
			}
			escaped.WriteByte(v[i])
		}
		if got := unescape(escaped.String()); got != v {
			t.Errorf("from tmux 3.4: %q read back as %q", v, got)
		}
		if got := unescape(v); got != v {
			t.Errorf("from a later tmux: %q read back as %q", v, got)
		}
	}
}

// resumable is a runtime that can resume; gone names a lost conversation.
type resumable struct {
	stubRuntime
	gone string
}

func (r resumable) Caps() runtime.Capabilities {
	return runtime.Capabilities{Resume: true, AssignSessionID: true}
}
func (r resumable) Sessions() runtime.SessionOps { return resumableOps{r.gone} }

type resumableOps struct{ gone string }

func (o resumableOps) NewID() string                      { return "new-id" }
func (o resumableOps) Exists(id, dir string) bool         { return id != o.gone }
func (o resumableOps) LastWrite(string, string) time.Time { return time.Time{} }
func (o resumableOps) FromHook(io.Reader) string          { return "" }

func TestPlan(t *testing.T) {
	dir := t.TempDir()
	record := RecordPath(dir, "work-dev-engineer")
	rt := resumable{}
	if c := Plan(rt, record, dir, false); c.SessionID != "new-id" || c.Note != "a new conversation" {
		t.Errorf("no record: %+v", c)
	}
	WriteRecord(record, Record{ID: "old-id", Dir: dir, Since: time.Now()})
	if c := Plan(rt, record, dir, false); c.Resume != "old-id" || c.SessionID != "" || c.Note != "resumed its conversation" {
		t.Errorf("a record: %+v", c)
	}
	if c := Plan(rt, record, dir, true); c.Resume != "" || c.Note != "a new conversation, as asked" {
		t.Errorf("fresh: %+v", c)
	}
	if c := Plan(rt, record, t.TempDir(), false); c.Resume != "" || !strings.Contains(c.Note, "another folder") {
		t.Errorf("another folder: %+v", c)
	}
	if c := Plan(resumable{gone: "old-id"}, record, dir, false); c.Resume != "" || !strings.Contains(c.Note, "the last one is gone") {
		t.Errorf("gone: %+v", c)
	}
	if c := Plan(stubRuntime{}, record, dir, false); c != (Conversation{}) {
		t.Errorf("a runtime that cannot resume: %+v", c)
	}
	if RecordPath(dir, "../x") != "" || RecordPath(dir, "a/b") != "" {
		t.Error("a name cadre does not make got a record")
	}
	Forget(record)
	if ReadRecord(record) != nil {
		t.Error("Forget")
	}
}

// tmux reads a dot in a target as the start of a pane, so a project such
// as my.app is named with the colon that ends the session part: it can be
// found, listed, attached to and stopped.
func TestADottedProjectIsFoundStoppedAndAttached(t *testing.T) {
	tm := private(t)
	c, stub := cadreDir(t, "work")
	if out, failed := up(tm, c, stub, "dev", "pm", "my.app"); failed {
		t.Fatalf("up: %s", out)
	}
	// tmux 3.4 turns the dot into "_"; cadre names it so on every version.
	name := "cadre-work-dev-my_app"
	if !tm.Has(name) || tm.Has("cadre-work-dev-my") || SessionName("work", "dev-my.app") != name {
		t.Fatalf("Has: %v", tm.Has(name))
	}
	if w := tm.Windows(name); len(w) != 1 || w[0] != "pm" {
		t.Errorf("Windows: %q", w)
	}
	// Without a terminal, tmux finds the session and then cannot open the
	// terminal; a target it cannot read fails before that.
	out, _ := tm.command(attachArgs(name, false)...).CombinedOutput()
	if !strings.Contains(string(out), "not a terminal") {
		t.Errorf("attach: %s", out)
	}
	if args := attachArgs(name, true); args[0] != "switch-client" || args[2] != "="+name+":" {
		t.Errorf("switch-client: %q", args)
	}
	s := Scope{Name: "work", Path: c, T: tm}
	if line := s.StopTeam("dev-my.app"); line != "  "+name+" stopped" || tm.Has(name) {
		t.Errorf("stop: %q", line)
	}
}

// A build folder, or the .claude folder holding it, that is a link (as a
// cloned cadre can carry) is refused: nothing is written through it.
func TestEnsureBuildRefusesALink(t *testing.T) {
	for _, linked := range []string{".claude/build", ".claude"} {
		c, scratch := t.TempDir(), t.TempDir()
		os.MkdirAll(filepath.Join(c, filepath.Dir(linked)), 0o755)
		os.Symlink(scratch, filepath.Join(c, linked))
		if err := EnsureBuild(filepath.Join(c, ".claude", "build")); err == nil || !strings.Contains(err.Error(), "is a link or a file, not a folder") {
			t.Errorf("%s: %v", linked, err)
		}
		if entries, _ := os.ReadDir(scratch); len(entries) != 0 {
			t.Errorf("%s: wrote through the link: %v", linked, entries)
		}
	}
	c := t.TempDir()
	if err := EnsureBuild(filepath.Join(c, ".claude", "build")); err != nil {
		t.Errorf("a new cadre: %v", err)
	}
}
