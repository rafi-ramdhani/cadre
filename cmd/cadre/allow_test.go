//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	out, _ := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	return string(out)
}

func TestAllowAddListRemove(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	c := home + "/.cadre/work"
	file := c + "/.claude/persona-settings.json"
	if out := must(t, "allow"); !strings.Contains(out, "Grants for personas: none yet") {
		t.Errorf("allow with no file: %q", out)
	}
	out := must(t, "allow", "add", "Bash(git push origin HEAD:main)")
	if !strings.Contains(out, "created "+file) || !strings.Contains(out, "  added rule: Bash(git push origin HEAD:main)") ||
		!strings.Contains(out, "Personas started from now on get this change.") {
		t.Errorf("add: %q", out)
	}
	if !strings.Contains(gitLog(t, c), "Allow for personas: Bash(git push origin HEAD:main)") {
		t.Errorf("not committed:\n%s", gitLog(t, c))
	}
	if out := must(t, "allow", "add", "Bash(git push origin HEAD:main)"); !strings.Contains(out, "is already granted; nothing changed") {
		t.Errorf("duplicate: %q", out)
	}
	before := readFile(t, file)
	refused(t, "refused: Bash(bash *) lets bash run anything", "allow", "add", "Bash(bash *)")
	refused(t, "refused: an --auto entry may not speak about permissions", "allow", "add", "--auto", "Changing permissions is fine")
	if readFile(t, file) != before {
		t.Error("a refusal changed the file")
	}
	if out := must(t, "allow", "add", "--auto", "Setting up a local test database is expected"); !strings.Contains(out, "added --auto entry") {
		t.Errorf("--auto: %q", out)
	}
	if out := must(t, "allow", "add", "--once", "Bash(make deploy)"); !strings.Contains(out, "added one-time rule: Bash(make deploy)") {
		t.Errorf("--once: %q", out)
	}
	if out := must(t, "allow", "add", "Bash(ls docs/*)"); !strings.Contains(out, "contains * and may allow more") {
		t.Errorf("wide: %q", out)
	}
	out = must(t, "allow", "list")
	for _, want := range []string{"  1. rule  Bash(git push origin HEAD:main)", "  2. rule  Bash(make deploy)   [once, added just now]",
		"  3. rule  Bash(ls docs/*)   [wide: contains *]", "  4. auto  Setting up a local test database is expected", "Built in, not listed"} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q:\n%s", want, out)
		}
	}
	if out := must(t, "allow", "remove", "--once"); !strings.Contains(out, "removed: Bash(make deploy)") {
		t.Errorf("remove --once: %q", out)
	}
	if out := must(t, "allow", "remove", "--once"); !strings.Contains(out, "there are no one-time grants") {
		t.Errorf("remove --once again: %q", out)
	}
	if out := must(t, "allow", "remove", "1"); !strings.Contains(out, "removed: Bash(git push origin HEAD:main)") {
		t.Errorf("remove 1: %q", out)
	}
	refused(t, "there is no grant number 9", "allow", "remove", "9")
	refused(t, "Bash(nope) is not granted", "allow", "remove", "Bash(nope)")
	refused(t, "usage", "allow", "remove", "--auto", "x")
	refused(t, "unknown option --bogus", "allow", "add", "--bogus", "x")
	log := gitLog(t, c)
	for _, want := range []string{"Allow for personas once: Bash(make deploy)", "Remove one-time grants for personas", "Remove grant for personas: Bash(git push origin HEAD:main)"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	t.Setenv("CADRE_PERSONA", "x")
	refused(t, "persona sessions cannot change permissions", "allow", "add", "Bash(true)")
	refused(t, "persona sessions cannot change permissions", "allow", "remove", "1")
}

func TestAllowWarnsAboutAnEditMadeOutside(t *testing.T) {
	home := sandbox(t)
	must(t, "init", "work")
	must(t, "allow", "add", "Bash(npm test)")
	file := home + "/.cadre/work/.claude/persona-settings.json"
	os.WriteFile(file, []byte(strings.Replace(readFile(t, file), `"Bash(npm test)"`, `"Bash(npm test)", "Bash(curl *)"`, 1)), 0o644)
	_, _, errOut := call("allow", "add", "Bash(make)")
	if !strings.Contains(errOut, "was changed outside cadre allow") {
		t.Errorf("no warning: %q", errOut)
	}
	// An unusable file is not changed.
	os.WriteFile(file, []byte(`{"hooks": {}}`), 0o644)
	refused(t, "cannot be changed because it has the key hooks", "allow", "add", "Bash(make test)")
}

// Concurrent adds all land: the per-cadre lock serializes read, change and write.
func TestConcurrentAdds(t *testing.T) {
	sandbox(t)
	must(t, "init", "work")
	must(t, "allow", "add", "Bash(echo first)")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			call("allow", "add", "Bash(echo p"+string(rune('0'+i))+")")
		}(i)
	}
	wg.Wait()
	out := must(t, "allow", "list")
	for i := 0; i < 8; i++ {
		if !strings.Contains(out, "Bash(echo p"+string(rune('0'+i))+")") {
			t.Errorf("a concurrent add was lost:\n%s", out)
		}
	}
}

func TestAllowRestartNote(t *testing.T) {
	home := sandbox(t)
	withTmux(t, home)
	must(t, "init", "work")
	must(t, "up", "dev/engineer")
	out := must(t, "allow", "add", "Bash(true)")
	if !strings.Contains(out, "Running personas will not see this change until restarted") ||
		!strings.Contains(out, "  CADRE_HOME="+home+"/.cadre/work cadre stop dev/engineer && CADRE_HOME="+home+"/.cadre/work cadre up dev/engineer\n") {
		t.Errorf("restart note:\n%s", out)
	}
}
