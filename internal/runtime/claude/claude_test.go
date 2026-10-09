package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadrei/internal/runtime"
)

func TestLaunch(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	cmd, err := Claude{}.Launch(runtime.LaunchSpec{Name: "w-dev-pm", Mode: "auto", PromptFile: "/p.md", Grants: "/g.json", WorkDir: "/w"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(bin, "claude") + " --name w-dev-pm --permission-mode auto --append-system-prompt-file /p.md --settings /g.json"
	if strings.Join(cmd.Argv, " ") != want || cmd.Dir != "/w" || len(cmd.Env) != 0 {
		t.Errorf("Launch: %+v", cmd)
	}
	cmd, _ = Claude{}.Launch(runtime.LaunchSpec{Name: "x", Mode: "default"})
	if strings.Contains(strings.Join(cmd.Argv, " "), "--settings") || strings.Contains(strings.Join(cmd.Argv, " "), "--append-system-prompt-file") {
		t.Errorf("no grants and no prompt: %+v", cmd)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Claude{}).Launch(runtime.LaunchSpec{}); err == nil || !strings.Contains(err.Error(), "not on your PATH") {
		t.Errorf("no claude: %v", err)
	}
}

func TestSessions(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\n"), 0o755)
	t.Setenv("PATH", bin)
	cmd, _ := Claude{}.Launch(runtime.LaunchSpec{Name: "x", Mode: "default", SessionID: "11111111-2222-4333-8444-555555555555"})
	if !strings.HasSuffix(strings.Join(cmd.Argv, " "), "--session-id 11111111-2222-4333-8444-555555555555") {
		t.Errorf("a new conversation: %v", cmd.Argv)
	}
	cmd, _ = Claude{}.Launch(runtime.LaunchSpec{Name: "x", Mode: "default", Resume: "11111111-2222-4333-8444-555555555555"})
	if !strings.HasSuffix(strings.Join(cmd.Argv, " "), "--resume 11111111-2222-4333-8444-555555555555") || strings.Contains(strings.Join(cmd.Argv, " "), "--session-id") {
		t.Errorf("a resumed conversation: %v", cmd.Argv)
	}
	ops := Claude{}.Sessions()
	id := ops.NewID()
	if !uuidRule.MatchString(id) || id[14] != '4' || ops.NewID() == id {
		t.Errorf("NewID %q", id)
	}
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	if ops.Exists(id, "/Users/me/app") {
		t.Error("a conversation with no transcript exists")
	}
	// Only the transcript in the session's own folder counts, as
	// claude --resume looks there: /Users/me/.cadrei/w is -Users-me--cadrei-w.
	os.MkdirAll(filepath.Join(cfg, "projects", "-Users-me-app"), 0o755)
	os.WriteFile(filepath.Join(cfg, "projects", "-Users-me-app", id+".jsonl"), []byte("{}\n"), 0o600)
	if !ops.Exists(id, "/Users/me/app") || ops.Exists(id, "/Users/me/.cadrei/w") || ops.Exists("../../etc/passwd", "/Users/me/app") {
		t.Error("Exists")
	}
	os.MkdirAll(filepath.Join(cfg, "projects", "-Users-me--cadrei-w"), 0o755)
	os.WriteFile(filepath.Join(cfg, "projects", "-Users-me--cadrei-w", id+".jsonl"), []byte("{}\n"), 0o600)
	if !ops.Exists(id, "/Users/me/.cadrei/w") {
		t.Error("a folder with a dot")
	}
	if _, size, ok := ops.Transcript(id, "/Users/me/.cadrei/w"); !ok || size != 3 {
		t.Errorf("Transcript: %v %d", ok, size)
	}
	if _, _, ok := ops.Transcript(id, "/Users/me/other"); ok {
		t.Error("Transcript in another folder")
	}
	if got := ops.FromHook(strings.NewReader(`{"session_id": "` + id + `", "source": "clear"}`)); got != id {
		t.Errorf("FromHook %q", got)
	}
	if got := ops.FromHook(strings.NewReader(`{"session_id": "../x"}`)); got != "" {
		t.Errorf("a bad id from the hook: %q", got)
	}
}
