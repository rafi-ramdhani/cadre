package fsx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestWriteFile(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "f")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(p, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	st, _ := os.Stat(p)
	if string(got) != "new" || st.Mode().Perm() != 0o600 {
		t.Errorf("got %q mode %v", got, st.Mode().Perm())
	}
	entries, _ := os.ReadDir(d)
	if len(entries) != 1 {
		t.Errorf("a temporary file was left: %v", entries)
	}
	if err := WriteFile(filepath.Join(d, "missing", "f"), nil, 0o600); err == nil {
		t.Error("writing into a missing folder succeeded")
	}
}

func TestLockIsExclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	l, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(p, 200*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Acquire: %v, want ErrBusy", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	l2.Release()
}

// A lock held by a process that dies is free at once: the kernel drops a
// flock with its holder.
func TestLockIsFreedWhenItsHolderDies(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperHoldLock")
	cmd.Env = append(os.Environ(), "CADRE_TEST_HOLD_LOCK="+p)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := out.Read(buf); err != nil {
		t.Fatalf("helper did not report holding the lock: %v", err)
	}
	if _, err := Acquire(p, 100*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("Acquire while the helper holds it: %v, want ErrBusy", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
	l, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("Acquire after the holder died: %v", err)
	}
	l.Release()
}

func TestHelperHoldLock(t *testing.T) {
	p := os.Getenv("CADRE_TEST_HOLD_LOCK")
	if p == "" {
		t.Skip("helper for TestLockIsFreedWhenItsHolderDies")
	}
	if _, err := Acquire(p, time.Second); err != nil {
		os.Exit(1)
	}
	os.Stdout.WriteString("held")
	time.Sleep(time.Minute)
}

func TestDirLockFallback(t *testing.T) {
	forceDirLock = true
	defer func() { forceDirLock = false }()
	p := filepath.Join(t.TempDir(), "x.lock")
	l, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(p, 200*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Acquire: %v, want ErrBusy", err)
	}
	l.Release()
	if _, err := os.Stat(p + ".d"); !errors.Is(err, os.ErrNotExist) {
		t.Error("Release left the lock folder")
	}
	// A lock older than a minute is from a run that died: taken over.
	if err := os.Mkdir(p+".d", 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(p+".d", old, old)
	l, err = Acquire(p, time.Second)
	if err != nil {
		t.Fatalf("stale lock not taken over: %v", err)
	}
	l.Release()
	if matches, _ := filepath.Glob(p + ".d.stale-*"); len(matches) != 0 {
		t.Errorf("the stale lock was left aside: %v", matches)
	}
	// Releasing a lock that is already gone is not an error.
	l, _ = Acquire(p, time.Second)
	os.Remove(p + ".d")
	if err := l.Release(); err != nil {
		t.Errorf("Release of a vanished lock: %v", err)
	}
}

func TestCopyTreeAndVerify(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	write := func(rel, body string, perm os.FileMode) {
		t.Helper()
		p := filepath.Join(src, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), perm); err != nil {
			t.Fatal(err)
		}
	}
	write("playbook.md", "play", 0o644)
	write(".claude/member-settings.json", "{}", 0o600)
	write("teams/dev/notes.md", "n", 0o644)
	write("projects/app/README", "skip me", 0o644)
	os.MkdirAll(filepath.Join(src, "empty"), 0o755)
	os.Symlink("../elsewhere", filepath.Join(src, "link"))
	skip := func(rel string) bool { return rel == "projects" }

	dst := filepath.Join(t.TempDir(), "dst")
	if err := CopyTree(src, dst, skip); err != nil {
		t.Fatal(err)
	}
	if err := VerifyTree(src, dst, skip); err != nil {
		t.Fatalf("VerifyTree after CopyTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "projects")); err == nil {
		t.Error("a skipped folder was copied")
	}
	if got, _ := os.Readlink(filepath.Join(dst, "link")); got != "../elsewhere" {
		t.Errorf("symlink target %q", got)
	}
	if st, _ := os.Stat(filepath.Join(dst, ".claude/member-settings.json")); st.Mode().Perm() != 0o600 {
		t.Errorf("mode not kept: %v", st.Mode().Perm())
	}
	if err := CopyTree(src, dst, skip); err == nil {
		t.Error("CopyTree over an existing folder succeeded")
	}

	os.WriteFile(filepath.Join(dst, "playbook.md"), []byte("changed!"), 0o644)
	if err := VerifyTree(src, dst, skip); err == nil || !strings.Contains(err.Error(), "playbook.md") {
		t.Errorf("a changed size was not found: %v", err)
	}
	os.WriteFile(filepath.Join(dst, "playbook.md"), []byte("play"), 0o644)
	os.WriteFile(filepath.Join(dst, "extra"), nil, 0o644)
	if err := VerifyTree(src, dst, skip); err == nil || !strings.Contains(err.Error(), "extra") {
		t.Errorf("an extra file was not found: %v", err)
	}
	os.Remove(filepath.Join(dst, "extra"))
	os.Remove(filepath.Join(dst, "teams/dev/notes.md"))
	if err := VerifyTree(src, dst, skip); err == nil || !strings.Contains(err.Error(), "lacks") {
		t.Errorf("a missing file was not found: %v", err)
	}
}

// A lock path planted as a symlink or a hard link to another file must not
// get that file changed: Acquire refuses, and the file stays as it was.
func TestLockRefusesLinks(t *testing.T) {
	d := t.TempDir()
	victim := filepath.Join(d, "victim.txt")
	os.WriteFile(victim, []byte("precious user data"), 0o600)
	sym := filepath.Join(d, "sym.lock")
	os.Symlink(victim, sym)
	if _, err := Acquire(sym, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("a symlinked lock: %v", err)
	}
	dangling := filepath.Join(d, "dangling.lock")
	os.Symlink(filepath.Join(d, "would-be-created"), dangling)
	if _, err := Acquire(dangling, 100*time.Millisecond); err == nil {
		t.Error("a dangling symlink was followed")
	}
	if _, err := os.Stat(filepath.Join(d, "would-be-created")); err == nil {
		t.Error("a dangling symlink's target was created")
	}
	hard := filepath.Join(d, "hard.lock")
	os.Link(victim, hard)
	if _, err := Acquire(hard, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "hard link") {
		t.Errorf("a hard-linked lock: %v", err)
	}
	fifo := filepath.Join(d, "fifo.lock")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := Acquire(fifo, 100*time.Millisecond); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "not a plain file") {
			t.Errorf("a FIFO lock: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Acquire hung on a FIFO")
	}
	os.Mkdir(filepath.Join(d, "dir.lock"), 0o700)
	if _, err := Acquire(filepath.Join(d, "dir.lock"), 100*time.Millisecond); err == nil {
		t.Error("a folder was taken as a lock")
	}
	if got, _ := os.ReadFile(victim); string(got) != "precious user data" {
		t.Errorf("the victim changed: %q", got)
	}
	// A lock leaves its file empty and unchanged.
	p := filepath.Join(d, "x.lock")
	l, err := Acquire(p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	if st, _ := os.Stat(p); st.Size() != 0 {
		t.Errorf("the lock file was written: %d bytes", st.Size())
	}
}
