package cadre

import (
	"io/fs"
	"testing"
)

func TestAssetsHoldSkillProtocolAndTemplate(t *testing.T) {
	for _, name := range []string{
		"skills/cadre/SKILL.md", "protocol.md",
		"template/playbook.md", "template/projects.yaml", "template/cadre.conf",
		"template/.gitignore", "template/teams/.gitkeep", "template/personas/dev/engineer.md",
	} {
		if _, err := fs.Stat(Assets, name); err != nil {
			t.Errorf("%s is not embedded: %v", name, err)
		}
	}
}
