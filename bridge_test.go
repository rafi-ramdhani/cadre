package cadrei

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The bridge for 0.1.x installs: the old command prints the Cadrei line
// and exits 1, and the old skill only points to it (tests/bridge.sh runs
// them from a real 0.1.1 install).
func TestTheBridgePointsToCadrei(t *testing.T) {
	want := "Cadre is now Cadrei: brew install rafi-ramdhani/cadrei/cadrei\n"
	out, err := exec.Command("sh", "bin/cadre", "ls").CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 || string(out) != want {
		t.Errorf("bin/cadre: %v %q", err, out)
	}
	skill, err := os.ReadFile("skills/cadre/SKILL.md")
	if err != nil || !strings.Contains(string(skill), "name: cadre\n") || !strings.Contains(string(skill), "brew install rafi-ramdhani/cadrei/cadrei") {
		t.Errorf("skills/cadre/SKILL.md: %v\n%s", err, skill)
	}
}
