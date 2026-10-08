package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafi-ramdhani/cadre/internal/runtime"
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
	cmd, _ = Claude{}.Launch(runtime.LaunchSpec{Name: "x", Mode: "default", ConfigDir: "/profiles/work"})
	if strings.Join(cmd.Env, " ") != "CLAUDE_CONFIG_DIR=/profiles/work" || strings.Contains(strings.Join(cmd.Argv, " "), "--settings") {
		t.Errorf("a cadre's own config folder: %+v", cmd)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Claude{}).Launch(runtime.LaunchSpec{}); err == nil || !strings.Contains(err.Error(), "not on your PATH") {
		t.Errorf("no claude: %v", err)
	}
}
