package cadres

import (
	"fmt"
	"os/exec"
	"strings"
)

// CommittedLinks lists the symbolic links committed in a cadre repository
// where cadre would follow them: in its member and team folders, in the
// runtime's folder (runtimeDir, relative, holding the build folder), and
// at its top level (playbook.md, projects.yaml and the like). Git checks
// out a committed link as a link, so a cloned cadre could point cadre at
// files outside it.
func CommittedLinks(dir, runtimeDir string) ([]string, error) {
	linkedDirs := []string{"members/", "teams/", runtimeDir + "/"}
	out, err := exec.Command("git", "-C", dir, "ls-files", "-s", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("could not list the files of %s: %v", dir, err)
	}
	var links []string
	for _, entry := range strings.Split(string(out), "\x00") {
		// <mode> <object> <stage>\t<path>
		meta, p, ok := strings.Cut(entry, "\t")
		if !ok || !strings.HasPrefix(meta, "120000 ") {
			continue
		}
		if !strings.Contains(p, "/") {
			links = append(links, p)
			continue
		}
		for _, d := range linkedDirs {
			if strings.HasPrefix(p, d) {
				links = append(links, p)
				break
			}
		}
	}
	return links, nil
}
