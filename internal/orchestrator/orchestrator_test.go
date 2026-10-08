package orchestrator

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLock(t *testing.T) {
	path := LockPath(t.TempDir())
	if ReadLock(path) != nil {
		t.Error("a lock without a file")
	}
	cmd := exec.Command("sleep", "30")
	cmd.Start()
	defer cmd.Process.Kill()
	if err := WriteLock(path, cmd.Process.Pid, Terminal, "/dev/ttys001", ""); err != nil {
		t.Fatal(err)
	}
	l := ReadLock(path)
	if l == nil || l.PID != cmd.Process.Pid || l.Mode != Terminal || l.TTY != "/dev/ttys001" || l.Command != "sleep" {
		t.Fatalf("lock %+v", l)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	// Another start time under the same pid (a reused pid) is stale.
	os.WriteFile(path, []byte(strings.Replace(mustRead(t, path), l.Start, "1.000000", 1)), 0o600)
	if ReadLock(path) != nil {
		t.Error("a reused pid was taken for the orchestrator")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a stale lock was not removed")
	}
	// A dead process is stale.
	WriteLock(path, cmd.Process.Pid, Tmux, "", "cadre-work")
	cmd.Process.Kill()
	cmd.Wait()
	if ReadLock(path) != nil {
		t.Error("a dead process's lock is live")
	}
	os.WriteFile(path, []byte("not json"), 0o600)
	if ReadLock(path) != nil {
		t.Error("an unreadable lock is live")
	}
}

func mustRead(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPrompt(t *testing.T) {
	p := string(Prompt([]byte("TEXT\n"), "work", "/h/.cadre/work", "", ""))
	if p != "TEXT\n\nYou are the orchestrator of cadre `work` (`/h/.cadre/work`).\n" {
		t.Errorf("%q", p)
	}
	p = string(Prompt([]byte("TEXT\n"), "work", "/w", "app", "/d/app"))
	if !strings.HasSuffix(p, "The user opened cadre from the project `app` (`/d/app`).\n") {
		t.Errorf("%q", p)
	}
}
