package proc

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestOf(t *testing.T) {
	me, err := Of(os.Getpid())
	if err != nil || me.Start == "" || me.Command == "" {
		t.Fatalf("Of(self) = %+v, %v", me, err)
	}
	if !Same(os.Getpid(), me) {
		t.Error("Same(self) is false")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	child, err := Of(cmd.Process.Pid)
	if err != nil || child.Command != "sleep" {
		t.Errorf("Of(child) = %+v, %v", child, err)
	}
	if Same(cmd.Process.Pid, Info{Start: "0", Command: "sleep"}) {
		t.Error("a different start time matched: a reused pid would pass")
	}
	cmd.Process.Kill()
	cmd.Wait()
	time.Sleep(50 * time.Millisecond)
	if Same(cmd.Process.Pid, child) {
		t.Error("a process that ended is still the same")
	}
}
