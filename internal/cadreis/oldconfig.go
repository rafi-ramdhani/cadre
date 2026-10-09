package cadreis

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadrei/internal/paths"
)

// Cadrei 0.2.0 does not convert a 0.1.x cadre by itself: it never changes
// one or ~/.config/cadre, and the orchestrator brings an old cadre in on
// the user's request (the skill's guidance). What cadrei knows of 0.1.x is
// read only: where the old default cadre is, for a note, and what an old
// cadre looks like, so it is never linked or trusted as a project.

// OldHome is the folder ~/.config/cadre/home names, 0.1.x's default cadre,
// or "" when there is none.
func OldHome() string {
	h := expandHome(firstLine(filepath.Join(paths.Home(), ".config", "cadre", "home")))
	if !filepath.IsAbs(h) {
		return ""
	}
	if st, err := os.Stat(h); err != nil || !st.IsDir() {
		return ""
	}
	return paths.Real(h)
}

// OldCadre reports whether dir (physical) is the top folder of a 0.1.x
// cadre: it has personas/ and projects.yaml, and it is not a cadrei this
// program knows. Nothing in it is read.
func OldCadre(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, "personas"))
	if err != nil || !st.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "projects.yaml")); err != nil {
		return false
	}
	if Inside(dir) {
		return false
	}
	list, _ := List()
	for _, c := range list {
		if c.Path == dir {
			return false
		}
	}
	return true
}

func firstLine(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	return strings.TrimSpace(line)
}

// expandHome turns a leading ~ into the home folder.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return paths.Home() + p[1:]
	}
	return p
}
