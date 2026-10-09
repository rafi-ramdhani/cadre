// Package framework writes out the files the cadre binary carries to
// ~/.cadre/framework, where tools that read files find them (the
// orchestrator skill), and says which binary cadre names in hooks.
package framework

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// Current reports whether the framework folder matches this binary.
func Current(version string) bool {
	if Version() != version {
		return false
	}
	_, err := os.Stat(filepath.Join(SkillDir(), "SKILL.md"))
	return err == nil
}

// Write writes the skill from the embedded assets into the framework
// folder, then VERSION last, so a folder with the right VERSION is whole.
func Write(assets fs.FS, version string) error {
	if err := os.MkdirAll(SkillDir(), 0o755); err != nil {
		return err
	}
	err := fs.WalkDir(assets, "skills/cadre", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, p)
		if err != nil {
			return err
		}
		to := filepath.Join(Dir(), p)
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return err
		}
		return fsx.WriteFile(to, data, 0o644)
	})
	if err != nil {
		return err
	}
	return fsx.WriteFile(filepath.Join(Dir(), "VERSION"), []byte(version+"\n"), 0o644)
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
