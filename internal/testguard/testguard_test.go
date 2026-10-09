package testguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) { Main(m) }

func TestTheGuardedEnvironment(t *testing.T) {
	if err := Unsafe(); err != nil {
		t.Fatalf("Main left an unsafe environment: %s", err)
	}
	for _, v := range cleared {
		if _, set := os.LookupEnv(v); set {
			t.Errorf("%s is still set", v)
		}
	}
	t.Setenv("HOME", realHome)
	if Unsafe() == nil {
		t.Error("the real HOME passed")
	}
	t.Setenv("HOME", filepath.Join(realHome, "."))
	if Unsafe() == nil {
		t.Error("the real HOME, spelled another way, passed")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CADRE_TMUX_SOCKET", "")
	if Unsafe() == nil {
		t.Error("no private tmux socket passed")
	}
	t.Setenv("CADRE_TMUX_SOCKET", "x")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(realHome, ".claude"))
	if Unsafe() == nil {
		t.Error("a CLAUDE_CONFIG_DIR in the real home passed")
	}
}
