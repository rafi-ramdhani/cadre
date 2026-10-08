// Package paths resolves physical paths and the folders cadre uses.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// maxLinks bounds symlink resolution, so a loop ends.
const maxLinks = 255

// Real returns the physical form of p: every symlink along the way is
// resolved and "." and ".." are applied to the resolved path, as Python's
// os.path.realpath does. Unlike filepath.EvalSymlinks it works for paths
// that do not exist: from the first missing part on, the rest is kept as
// written. A relative p is taken from the current folder.
func Real(p string) string {
	if !filepath.IsAbs(p) {
		wd, err := os.Getwd()
		if err != nil {
			wd = "/"
		}
		p = wd + "/" + p
	}
	rest := strings.Split(p, "/")
	cur, links := "/", 0
	for len(rest) > 0 {
		name := rest[0]
		rest = rest[1:]
		switch name {
		case "", ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		next := filepath.Join(cur, name)
		fi, err := os.Lstat(next)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 || links >= maxLinks {
			cur = next
			continue
		}
		target, err := os.Readlink(next)
		if err != nil {
			cur = next
			continue
		}
		links++
		if filepath.IsAbs(target) {
			cur = "/"
		}
		rest = append(strings.Split(target, "/"), rest...)
	}
	return cur
}

// Home is the user's home folder, physical. HOME wins, so tests can use a
// fake one.
func Home() string {
	h := os.Getenv("HOME")
	if h == "" {
		h, _ = os.UserHomeDir()
	}
	return Real(h)
}

// Within reports whether path is dir or inside it. Both should be physical.
func Within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}
