// Package claude is cadre's adapter for Claude Code, the only runtime in
// 0.2.0. Everything cadre knows about Claude Code lives here: its command
// and flags, its settings format and rule grammar (the allow checker), its
// .claude folders, ~/.claude.json trust and its hooks.
package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rafi-ramdhani/cadre/internal/cadres"
	"github.com/rafi-ramdhani/cadre/internal/jsonx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
	"github.com/rafi-ramdhani/cadre/internal/runtime/claude/allow"
	"github.com/rafi-ramdhani/cadre/internal/runtime/claude/settings"
)

func init() { runtime.RegisterDefault(Claude{}) }

// Claude is the Claude Code adapter.
type Claude struct{}

func (Claude) Name() string  { return "claude" }
func (Claude) Title() string { return "Claude Code" }

func (Claude) Caps() runtime.Capabilities {
	return runtime.Capabilities{
		FixedDenies:     true,
		PermissionModes: []string{"default", "acceptEdits", "plan", "auto", "dontAsk", "bypassPermissions"},
		Resume:          true, AssignSessionID: true,
		Trust: true, Instructions: true, OrchestratorHook: true, Messaging: runtime.Native,
	}
}

// Detect finds the claude command.
func (Claude) Detect() (runtime.Install, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return runtime.Install{}, errors.New("Claude Code (claude) is not on your PATH; install it from https://claude.com/claude-code")
	}
	return runtime.Install{Path: path}, nil
}

// Launch builds the claude command for a session. The prompt file is
// appended to Claude Code's system prompt, and a member gets its
// validated settings copy.
func (c Claude) Launch(s runtime.LaunchSpec) (runtime.Command, error) {
	in, err := c.Detect()
	if err != nil {
		return runtime.Command{}, err
	}
	argv := []string{in.Path, "--name", s.Name, "--permission-mode", s.Mode}
	if s.PromptFile != "" {
		argv = append(argv, "--append-system-prompt-file", s.PromptFile)
	}
	if s.Grants != "" {
		argv = append(argv, "--settings", s.Grants)
	}
	return runtime.Command{Argv: argv, Dir: s.WorkDir}, nil
}

// BuildDir is <cadre>/.claude/build: inside a .claude folder, which Claude
// Code protects, and denied to members (N.7).
func (Claude) BuildDir(cadre string) string { return filepath.Join(cadre, ".claude", "build") }

func (Claude) Permissions() runtime.PermissionOps { return permissions{} }
func (Claude) Trust() runtime.TrustOps            { return trust{} }

type permissions struct{}

func (permissions) GrantsFile(cadre string) string { return filepath.Join(cadre, settings.Rel) }

func (permissions) Validate(cadre string, known []string, rule string, auto bool) (string, error) {
	c := allow.New(cadre, known)
	if auto {
		return c.Auto(rule)
	}
	return c.Rule(rule)
}

// hashFile keeps the settings fingerprints, one file for the machine.
func hashFile() string { return cadres.Config("member-settings.sha256") }

// Prepare makes sure the cadre has its settings file and writes the
// validated copy a member starts with; a file that cannot be used gives no
// copy and a warning, and an edit made outside cadre allow a warning.
func (p permissions) Prepare(cadre string, places runtime.Places) runtime.Prepared {
	var out runtime.Prepared
	file := p.GrantsFile(cadre)
	if _, err := os.Stat(file); err != nil {
		os.MkdirAll(filepath.Dir(file), 0o755)
		if err := settings.Create(file); err != nil {
			out.Warnings = append(out.Warnings, "warning: could not create "+file+": "+err.Error())
			return out
		}
		settings.Record(file, hashFile())
		if note, _ := cadres.Commit(cadre, "Add the member settings file", settings.Rel); note != "" {
			out.Notes = append(out.Notes, note)
		}
		out.Notes = append(out.Notes, "  created "+file+" (grants for members; change it with cadre allow)")
	}
	build := Claude{}.BuildDir(cadre)
	sp := settings.Places{Root: places.Root, Cadres: places.Cadres}
	copyPath, err := settings.Export(file, build, sp)
	if err != nil {
		// The member still gets every deny rule, and no grants.
		out.Warnings = append(out.Warnings, fmt.Sprintf("warning: members start with no grants, only cadre's deny rules, because %s cannot be used: %s; fix it or restore it with git -C %s checkout -- %s", file, err, cadre, settings.Rel))
		if out.Grants, err = settings.ExportDenyOnly(build, sp); err != nil {
			out.Warnings = append(out.Warnings, "warning: could not write the deny-only copy: "+err.Error())
		}
		return out
	}
	out.Grants = copyPath
	if g, err := settings.OpenGrants(file); err == nil && g.HasOnce() {
		out.Notes = append(out.Notes, "note: one-time grants are still in place; see cadre allow list and remove each when its task is done")
	}
	if settings.ChangedOutside(cadre, file, hashFile()) {
		out.Warnings = append(out.Warnings, fmt.Sprintf("warning: %s was changed outside cadre allow (see git -C %s diff and log -p -- %s); members still get it because it is valid", file, cadre, settings.Rel))
	}
	return out
}

// Config is Claude Code's config file, where folder trust is kept.
func Config() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = paths.Home()
	}
	return filepath.Join(dir, ".claude.json")
}

type trust struct{}

// Protected: Claude Code loads skills, commands, hooks and settings from
// ~/.claude, so no project may live there.
func (trust) Protected() []string { return []string{filepath.Join(paths.Home(), ".claude")} }

// projectFiles are what Claude Code reads from a session's folder once the
// folder is trusted: settings (with hooks and permissions), commands,
// agents, skills, output styles, MCP servers and instructions.
var projectFiles = []string{
	".claude/settings.json", ".claude/settings.local.json", ".claude/commands", ".claude/agents",
	".claude/skills", ".claude/output-styles", ".mcp.json", "CLAUDE.md", "CLAUDE.local.md",
}

func (trust) Loaded(dir string) []string {
	var out []string
	for _, f := range projectFiles {
		if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func (trust) Mark(folders []runtime.Folder) ([]runtime.TrustResult, string) {
	return edit(folders, func(e []TrustEntry) jsonx.Op { return Trust(e) })
}

func (trust) Unmark(folders []runtime.Folder) ([]runtime.TrustResult, string) {
	return edit(folders, func(e []TrustEntry) jsonx.Op { return Untrust(e) })
}

// edit changes trust for folders in one write with one backup. Each folder
// is keyed by its physical path and, when it differs, by the path as cadre
// spells it (section C).
func edit(folders []runtime.Folder, op func([]TrustEntry) jsonx.Op) ([]runtime.TrustResult, string) {
	cfg := Config()
	var entries []TrustEntry
	for _, f := range folders {
		phys := paths.Real(f.Dir)
		dirs := []string{phys}
		if abs, err := filepath.Abs(f.Dir); err == nil && filepath.Clean(abs) != phys {
			dirs = append(dirs, filepath.Clean(abs))
		}
		entries = append(entries, TrustEntry{Name: f.Name, Dirs: dirs})
	}
	if len(entries) == 0 {
		return nil, ""
	}
	code, lines := jsonx.Edit(cfg, jsonx.Options{Backup: cfg + ".bak-cadre"}, op(entries))
	var results []runtime.TrustResult
	var note string
	switch code {
	case jsonx.Changed, jsonx.Unchanged:
		for _, l := range lines {
			name, state, _ := strings.Cut(l, "\t")
			results = append(results, runtime.TrustResult{Name: name, State: state})
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
		results = append(results, runtime.TrustResult{Name: e.Name, State: "skipped"})
	}
	return results, note
}
