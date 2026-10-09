package testguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	t.Setenv("TMUX_TMPDIR", "")
	if Unsafe() == nil {
		t.Error("tmux sockets in the user's tmux folder passed")
	}
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(realHome, ".claude-that-does-not-exist", "x"))
	if Unsafe() == nil {
		t.Error("a CLAUDE_CONFIG_DIR in the real home that does not exist yet passed")
	}
}

func TestRealResolvesTheExistingPart(t *testing.T) {
	dir := t.TempDir()
	os.Symlink(dir, dir+"-link")
	want, _ := filepath.EvalSymlinks(dir)
	if got := real(dir + "-link/missing/x"); got != want+"/missing/x" {
		t.Errorf("real = %q, want %q", got, want+"/missing/x")
	}
}

// Every package with tests runs them through Main, so none can reach the
// user's setup.
func TestEveryTestPackageIsGuarded(t *testing.T) {
	root := filepath.Join("..", "..")
	guarded := regexp.MustCompile(`func TestMain\(m \*testing\.M\) \{ (testguard\.)?Main\(m\) \}`)
	tested := map[string]bool{}
	mains := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dist", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		tested[dir] = true
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if guarded.Match(raw) {
			mains[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tested) < 10 {
		t.Fatalf("found only %d test packages; is the walk in the module?", len(tested))
	}
	for dir := range tested {
		if !mains[dir] {
			rel, _ := filepath.Rel(root, dir)
			t.Errorf("%s has tests but no TestMain that runs testguard.Main", rel)
		}
	}
}
