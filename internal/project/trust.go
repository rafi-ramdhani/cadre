// Package project adds, clones, links and trusts a cadre's projects.
package project

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// TrustRefusal says why folder dir (physical) must not be trusted, or ""
// when it may be. Only the top folder of a project's git repository is
// trusted: never /, the home folder, ~/.cadre or a folder containing it,
// a cadre's folder or anything in it (team folders included), a folder
// containing a cadre, or anything inside a folder the runtime loads code
// from (protected).
func TrustRefusal(dir string, protected []string) string {
	home := paths.Home()
	root := paths.Real(cadres.Root())
	switch {
	case dir == "/":
		return "it is the root folder"
	case dir == home:
		return "it is your home folder"
	case paths.Within(root, dir):
		return "it is ~/.cadre or contains it"
	case paths.Within(dir, root):
		return "it is inside ~/.cadre, where cadre keeps its own files"
	}
	for _, p := range protected {
		if paths.Within(dir, p) {
			return "it is inside " + cadres.Tilde(p)
		}
	}
	list, _ := cadres.List()
	for _, c := range list {
		if paths.Within(c.Path, dir) {
			return "it is the cadre folder of " + c.Name + " or contains it"
		}
		if paths.Within(dir, filepath.Join(c.Path, "teams")) {
			return "it is a team folder"
		}
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil || paths.Real(strings.TrimSpace(string(out))) != dir {
		return "it is not the top folder of a git repo"
	}
	return ""
}

// Trust marks folders as trusted in the runtime, refusing the ones
// TrustRefusal names. It returns a result per folder and, when the
// runtime's config was left alone, a note for the user.
func Trust(rt runtime.Runtime, folders []runtime.Folder) ([]runtime.TrustResult, string) {
	var results []runtime.TrustResult
	var ok []runtime.Folder
	protected := rt.Trust().Protected()
	for _, f := range folders {
		if why := TrustRefusal(paths.Real(f.Dir), protected); why != "" {
			results = append(results, runtime.TrustResult{Name: f.Name, State: "refused", Reason: why})
			continue
		}
		ok = append(ok, f)
	}
	marked, note := rt.Trust().Mark(ok)
	return append(results, marked...), note
}

// Line is how cadre reports one trust result in runtime title's settings.
func Line(r runtime.TrustResult, title string) string {
	switch r.State {
	case "trusted":
		return "  " + r.Name + ": trusted in " + title
	case "already":
		return "  " + r.Name + ": already trusted in " + title
	case "refused":
		return "  " + r.Name + ": not trusted, " + r.Reason
	}
	return "  " + r.Name + ": not trusted (see below)"
}
