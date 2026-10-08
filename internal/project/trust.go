// Package project adds, clones, links and trusts a cadre's projects.
package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/jsonx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
)

// ClaudeConfig is Claude Code's config file, where folder trust is kept.
func ClaudeConfig() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = paths.Home()
	}
	return filepath.Join(dir, ".claude.json")
}

// TrustRefusal says why folder dir (physical) must not be trusted, or ""
// when it may be. Only the top folder of a project's git repository is
// trusted: never /, the home folder, ~/.cadre or a folder containing it,
// a cadre's folder or anything in it (team folders included), or a folder
// containing an outside cadre (N.7).
func TrustRefusal(dir string) string {
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
	case paths.Within(dir, filepath.Join(home, ".claude")):
		return "it is inside ~/.claude"
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

// TrustResult is what happened to one project's trust.
type TrustResult struct {
	Name   string
	State  string // "trusted", "already", "refused" or "skipped"
	Reason string // why it was refused
}

// Folder is a project to trust: its name and its folder as cadre spells
// it (the registry's path, expanded).
type Folder struct {
	Name string
	Dir  string
}

// Trust marks the folders as trusted in Claude Code's config, in one write
// with one backup. Each folder is keyed by its physical path and, when it
// differs, by the path as cadre spells it (section C). It returns a result
// per project and, when the config was left alone, a note for the user.
func Trust(folders []Folder) ([]TrustResult, string) {
	cfg := ClaudeConfig()
	var results []TrustResult
	var entries []jsonx.TrustEntry
	for _, f := range folders {
		phys := paths.Real(f.Dir)
		if why := TrustRefusal(phys); why != "" {
			results = append(results, TrustResult{f.Name, "refused", why})
			continue
		}
		dirs := []string{phys}
		if abs, err := filepath.Abs(f.Dir); err == nil && filepath.Clean(abs) != phys {
			dirs = append(dirs, filepath.Clean(abs))
		}
		entries = append(entries, jsonx.TrustEntry{Name: f.Name, Dirs: dirs})
	}
	if len(entries) == 0 {
		return results, ""
	}
	code, lines := jsonx.Edit(cfg, jsonx.Options{Backup: cfg + ".bak-cadre"}, jsonx.Trust(entries))
	var note string
	switch code {
	case jsonx.Changed, jsonx.Unchanged:
		for _, l := range lines {
			name, state, _ := strings.Cut(l, "\t")
			results = append(results, TrustResult{Name: name, State: state})
		}
		return results, ""
	case jsonx.Missing:
		note = "Claude Code has not created its config yet; it will ask to trust the folder on first launch"
	case jsonx.KeptChanged:
		note = "warning: " + cfg + " kept changing (a running Claude Code session?), so it was left unchanged; Claude Code will ask to trust the folder on first launch"
	case jsonx.WriteFailed:
		note = "warning: could not write next to " + cfg + " (folder not writable, or disk full?), so it was left unchanged; Claude Code will ask to trust the folder on first launch"
	default:
		note = "warning: " + cfg + " is not a file cadre can safely edit (unreadable, not valid JSON, an unexpected shape, or owned by another user), so it was left unchanged; Claude Code will ask to trust the folder on first launch"
	}
	for _, e := range entries {
		results = append(results, TrustResult{Name: e.Name, State: "skipped"})
	}
	return results, note
}

// Line is how cadre reports one trust result.
func (r TrustResult) Line() string {
	switch r.State {
	case "trusted":
		return "  " + r.Name + ": trusted in Claude Code"
	case "already":
		return "  " + r.Name + ": already trusted in Claude Code"
	case "refused":
		return "  " + r.Name + ": not trusted, " + r.Reason
	}
	return "  " + r.Name + ": not trusted (see below)"
}
