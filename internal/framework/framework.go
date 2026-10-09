// Package framework writes out the files the cadre binary carries to
// ~/.cadre/framework, where tools that read files find them (the
// orchestrator skill), and says which binary cadre names in hooks.
package framework

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// Dir is ~/.cadre/framework.
func Dir() string { return filepath.Join(cadres.Root(), "framework") }

// SkillDir is the written-out orchestrator skill.
func SkillDir() string { return filepath.Join(Dir(), "skills", "cadre") }

// Version is the version the framework folder was written for, or "".
func Version() string {
	raw, err := os.ReadFile(filepath.Join(Dir(), "VERSION"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// Sync makes the framework folder hold exactly the embedded skill and this
// version: a file that differs from the embedded one is rewritten, a file
// that is not cadre's is removed, and a link where a folder or file belongs
// is replaced. It runs on every plain cadre, since the skill carries the
// orchestrator's consent rules and members can write files with their
// shell. It returns what it restored when VERSION already named this
// version (a change made outside cadre), and nothing for an upgrade.
func Sync(assets fs.FS, version string) (restored []string, err error) {
	current := Version() == version
	want := map[string][]byte{}
	err = fs.WalkDir(assets, "skills/cadre", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, p)
		want[p] = data
		return err
	})
	if err != nil {
		return nil, err
	}
	// The folders themselves are cadre's: a link in their place goes.
	for _, d := range []string{Dir(), filepath.Join(Dir(), "skills"), SkillDir()} {
		if st, err := os.Lstat(d); err == nil && !st.IsDir() {
			os.Remove(d)
			if current {
				restored = append(restored, d+" (not a folder)")
			}
		}
	}
	if err := os.MkdirAll(SkillDir(), 0o755); err != nil {
		return nil, err
	}
	// Anything in the skill folder that is not cadre's goes.
	filepath.WalkDir(SkillDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(Dir(), p)
		if _, ok := want[filepath.ToSlash(rel)]; !ok {
			os.Remove(p)
			if current {
				restored = append(restored, filepath.ToSlash(rel)+" (not cadre's)")
			}
		}
		return nil
	})
	for rel, data := range want {
		to := filepath.Join(Dir(), rel)
		if st, err := os.Lstat(to); err == nil && st.Mode().IsRegular() {
			if have, err := os.ReadFile(to); err == nil && string(have) == string(data) {
				continue
			}
		}
		if current {
			restored = append(restored, rel)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return restored, err
		}
		// A new file renamed over the old one, which replaces a link
		// instead of writing through it.
		if err := fsx.WriteFile(to, data, 0o644); err != nil {
			return restored, err
		}
	}
	if !current {
		if err := fsx.WriteFile(filepath.Join(Dir(), "VERSION"), []byte(version+"\n"), 0o644); err != nil {
			return restored, err
		}
	}
	sort.Strings(restored)
	return restored, nil
}

// cellar matches a Homebrew keg's binary: <prefix>/Cellar/cadre/<version>/bin/cadre.
var cellar = regexp.MustCompile(`^(.*)/Cellar/cadre/[^/]+/bin/cadre$`)

// Binary is the path cadre persists wherever it names itself (hooks,
// restart commands): for a Homebrew install, <prefix>/opt/cadre/bin/cadre,
// which survives brew upgrade, never the Cellar path; otherwise the
// running binary, symlinks resolved.
func Binary() string {
	exe, err := os.Executable()
	if err != nil {
		return "cadre"
	}
	real := paths.Real(exe)
	if m := cellar.FindStringSubmatch(real); m != nil {
		return m[1] + "/opt/cadre/bin/cadre"
	}
	return real
}

// Placed says why binary must not be named in a hook that every session
// of the runtime runs, or nil: it must not be in a temporary folder, and
// it and every folder above it must belong to the user or root and be
// writable by no one else, so no other user can put a program there.
func Placed(binary string) error {
	p := paths.Real(binary)
	for _, tmp := range []string{os.TempDir(), "/tmp", "/var/tmp", "/var/folders"} {
		if paths.Within(p, paths.Real(tmp)) {
			return fmt.Errorf("%s is in a temporary folder", binary)
		}
	}
	return owned(p)
}

// owned says why the file at p (physical) could be replaced by another
// user, or nil.
func owned(p string) error {
	uid := os.Getuid()
	for d := p; ; d = filepath.Dir(d) {
		st, err := os.Stat(d)
		if err != nil {
			return err
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && int(sys.Uid) != uid && sys.Uid != 0 {
			return fmt.Errorf("%s belongs to another user", d)
		}
		if st.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("other users can change %s", d)
		}
		if d == "/" || d == "." {
			return nil
		}
	}
}

// Kind says how this binary was installed: "homebrew" (in a Homebrew
// keg), "dev" (built from source: version dev, or one with a commit or
// -dirty suffix), or "release" (a release binary, from install.sh or by
// hand).
func Kind(version string) string {
	exe, _ := os.Executable()
	switch {
	case cellar.MatchString(paths.Real(exe)):
		return "homebrew"
	case version == "dev" || strings.Contains(version, "-"):
		return "dev"
	}
	return "release"
}
