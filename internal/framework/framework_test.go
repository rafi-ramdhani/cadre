package framework

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestWrite(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("HOME", home)
	assets := fstest.MapFS{"skills/cadre/SKILL.md": {Data: []byte("skill")}, "protocol.md": {Data: []byte("p")}}
	if Current("1.0.0") {
		t.Error("an empty folder is current")
	}
	if err := Write(assets, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(SkillDir(), "SKILL.md")); string(b) != "skill" || Version() != "1.0.0" || !Current("1.0.0") {
		t.Errorf("after Write: %q %q", b, Version())
	}
	if Current("1.0.1") {
		t.Error("another version is current")
	}
	os.Remove(filepath.Join(SkillDir(), "SKILL.md"))
	if Current("1.0.0") {
		t.Error("a folder without the skill is current")
	}
}

func TestCellarPathBecomesOpt(t *testing.T) {
	if m := cellar.FindStringSubmatch("/opt/homebrew/Cellar/cadre/0.2.0/bin/cadre"); m == nil || m[1]+"/opt/cadre/bin/cadre" != "/opt/homebrew/opt/cadre/bin/cadre" {
		t.Errorf("match %v", m)
	}
	if cellar.MatchString("/usr/local/bin/cadre") {
		t.Error("a plain path matched")
	}
}
