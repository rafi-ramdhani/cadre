package backup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.email", "t@example.com")
	git(t, dir, "config", "user.name", "T")
	return dir
}

func commit(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	for p, data := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		os.WriteFile(filepath.Join(dir, p), []byte(data), 0o644)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "x")
	return git(t, dir, "rev-parse", "HEAD")
}

func TestInstall(t *testing.T) {
	dir := repo(t)
	if wrote, err := Install(dir, "/opt/cadre/bin/cadre"); err != nil || !wrote {
		t.Fatalf("install: %v %v", wrote, err)
	}
	hook := filepath.Join(dir, ".git", "hooks", "pre-push")
	raw, _ := os.ReadFile(hook)
	if !strings.Contains(string(raw), "exec '/opt/cadre/bin/cadre' hook pre-push \"$@\"") {
		t.Errorf("hook %q", raw)
	}
	if st, _ := os.Stat(hook); st.Mode().Perm()&0o100 == 0 {
		t.Error("the hook is not executable")
	}
	if wrote, _ := Install(dir, "/opt/cadre/bin/cadre"); wrote {
		t.Error("a current hook was written again")
	}
	if wrote, _ := Install(dir, "/new/cadre"); !wrote {
		t.Error("cadre's hook for another binary was not updated")
	}
	os.WriteFile(hook, []byte("#!/bin/sh\nmy own hook\n"), 0o755)
	if _, err := Install(dir, "/new/cadre"); err == nil || !strings.Contains(err.Error(), "not cadre's") {
		t.Errorf("a hook of the user's: %v", err)
	}
	if raw, _ := os.ReadFile(hook); string(raw) != "#!/bin/sh\nmy own hook\n" {
		t.Error("the user's hook was changed")
	}
}

func TestScan(t *testing.T) {
	MaxSize = 1 << 10
	defer func() { MaxSize = 50 << 20 }()
	dir := repo(t)
	// A first commit already on the remote is not checked again.
	commit(t, dir, map[string]string{"playbook.md": "# Playbook\n", ".env": "OLD=1\n"})
	remote := t.TempDir()
	git(t, remote, "init", "-q", "--bare")
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "-q", "origin", "main")
	// A secret added then removed is still in the history that is pushed.
	commit(t, dir, map[string]string{"teams/dev/notes.md": "key: sk-ant-api03-AbCdEfGhIjKlMnOpQrSt\n"})
	os.Remove(filepath.Join(dir, "teams/dev/notes.md"))
	head := commit(t, dir, map[string]string{
		"teams/dev/.env.local":   "X=1\n",
		"teams/dev/.env.example": "X=\n",
		"teams/dev/big.bin":      strings.Repeat("x", 2<<10),
		"teams/dev/id_ed25519":   "key\n",
		"teams/dev/ok.md":        "fine, and ghp_ is only a prefix here\n",
		"teams/dev/gh.txt":       "token ghp_" + strings.Repeat("a", 36) + "\n",
		"teams/dev/key.txt":      "-----BEGIN OPENSSH PRIVATE KEY-----\n",
	})
	got, err := Scan(dir, []Update{{"refs/heads/main", head, "refs/heads/main", "0000"}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"teams/dev/.env.local": "environment file", "teams/dev/big.bin": "over the", "teams/dev/gh.txt": "GitHub token",
		"teams/dev/id_ed25519": "private key", "teams/dev/key.txt": "private key", "teams/dev/notes.md": "Anthropic API key",
	}
	if len(got) != len(want) {
		t.Errorf("findings %+v", got)
	}
	for _, f := range got {
		if w, ok := want[f.Path]; !ok || !strings.Contains(f.Why, w) {
			t.Errorf("finding %+v, want %q", f, w)
		}
	}
	// Deleting a branch carries nothing.
	if got, _ := Scan(dir, []Update{{"(delete)", "0000000000000000000000000000000000000000", "refs/heads/x", head}}); len(got) != 0 {
		t.Errorf("a delete: %+v", got)
	}
}

func TestReadUpdates(t *testing.T) {
	u := ReadUpdates(strings.NewReader("refs/heads/main abc refs/heads/main def\nbad line\n"))
	if len(u) != 1 || u[0].LocalSHA != "abc" || u[0].RemoteSHA != "def" {
		t.Errorf("%+v", u)
	}
}

func TestRemove(t *testing.T) {
	dir := repo(t)
	Install(dir, "/opt/cadre/bin/cadre")
	if removed, err := Remove(dir); !removed || err != nil {
		t.Errorf("remove: %v %v", removed, err)
	}
	hook := filepath.Join(dir, ".git", "hooks", "pre-push")
	if _, err := os.Stat(hook); err == nil {
		t.Error("cadre's hook is still there")
	}
	os.WriteFile(hook, []byte("#!/bin/sh\nmine\n"), 0o755)
	if removed, _ := Remove(dir); removed {
		t.Error("the user's hook was removed")
	}
	if removed, _ := Remove(t.TempDir()); removed {
		t.Error("a folder that is not a repository")
	}
}
