package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/jsonx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// ConfigDir is Claude Code's config folder: CLAUDE_CONFIG_DIR, else
// ~/.claude. User skills and settings.json live in it.
func ConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(paths.Home(), ".claude")
}

// authStatusSince is the first version whose docs list `claude auth
// status` (with configDirectory). Older versions are not asked: an
// unknown subcommand could be read as a prompt.
var authStatusSince = [3]int{2, 1, 268}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func parseVersion(s string) ([3]int, bool) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

func atLeast(v, floor [3]int) bool {
	for i := range v {
		if v[i] != floor[i] {
			return v[i] > floor[i]
		}
	}
	return true
}

// output runs claude with args, no input and a time limit.
func output(bin string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = nil
	return cmd.Output()
}

func (c Claude) Health(full bool, cadreDirs []string) []runtime.Problem {
	var out []runtime.Problem
	in, err := c.Detect()
	if err != nil {
		return []runtime.Problem{{What: "Claude Code (claude) is not on your PATH", Fatal: true,
			Fix: "brew install --cask claude-code, or see https://claude.com/claude-code"}}
	}
	if full {
		raw, err := output(in.Path, "--version")
		v, ok := parseVersion(string(raw))
		switch {
		case err != nil || !ok:
			out = append(out, runtime.Problem{What: fmt.Sprintf("%s --version does not run", in.Path), Fatal: true,
				Fix: "reinstall Claude Code (brew reinstall --cask claude-code, or see https://claude.com/claude-code)"})
		case !atLeast(v, authStatusSince):
			out = append(out, runtime.Problem{What: "could not check the login",
				Fix: "if Claude Code asks you to log in, do so"})
		default:
			raw, err := output(in.Path, "auth", "status")
			var exit *exec.ExitError
			isJSON := json.Valid(bytes.TrimSpace(raw))
			switch {
			case err == nil && isJSON:
			case errors.As(err, &exit) && exit.ExitCode() == 1 && isJSON:
				out = append(out, runtime.Problem{What: "Claude Code is not logged in", Fix: "run claude and log in"})
			default:
				out = append(out, runtime.Problem{What: "could not check the login",
					Fix: "if Claude Code asks you to log in, do so"})
			}
		}
	}
	// Once a cadre folder is trusted, Claude Code loads these for its
	// orchestrator; cadre never writes them (N.7).
	for _, d := range cadreDirs {
		for _, f := range []string{"settings.json", "settings.local.json"} {
			p := filepath.Join(d, ".claude", f)
			if _, err := os.Lstat(p); err == nil {
				out = append(out, runtime.Problem{
					What: p + " exists: Claude Code loads it for this cadre's orchestrator, and cadre never writes it",
					Fix:  "look at what it holds, and remove it unless you added it yourself"})
			}
		}
	}
	return out
}

func (Claude) Instructions() runtime.InstructionOps { return instructions{} }
func (Claude) Hooks() runtime.HookOps               { return hooks{} }

type instructions struct{}

// Path is the user skill folder Claude Code reads cadre's skill from.
func (instructions) Path() string { return filepath.Join(ConfigDir(), "skills", "cadre") }

func (i instructions) Target() (string, error) {
	p := i.Path()
	st, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if st.Mode()&fs.ModeSymlink == 0 {
		return "", fmt.Errorf("%s: %w", p, runtime.ErrNotLink)
	}
	to, err := os.Readlink(p)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(to) {
		to = filepath.Join(filepath.Dir(p), to)
	}
	return filepath.Clean(to), nil
}

func (i instructions) Link(dir string) error {
	p := i.Path()
	if _, err := i.Target(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// A new link renamed over the old one, so the skill is never missing.
	tmp := fmt.Sprintf("%s.cadre-%d", p, os.Getpid())
	os.Remove(tmp)
	if err := os.Symlink(dir, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (i instructions) Unlink(dir string) (bool, error) {
	target, err := i.Target()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if paths.Real(target) != paths.Real(dir) {
		return false, nil
	}
	return true, os.Remove(i.Path())
}

type hooks struct{}

func (hooks) File() string { return filepath.Join(ConfigDir(), "settings.json") }

func isOrchestratorHook(command string) bool {
	_, ok := OrchestratorHook(command)
	return ok
}

func (h hooks) Find() ([]string, error) {
	raw, err := os.ReadFile(h.File())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	root, err := jsonx.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", h.File(), err)
	}
	_, groups, err := sessionStart(root, false)
	if err != nil || groups == nil {
		return nil, err
	}
	var found []string
	for _, g := range groups.Items {
		for _, hk := range g.Get("hooks").Items {
			if c, ok := commandOf(hk); ok {
				if prog, ok := OrchestratorHook(c); ok {
					found = append(found, prog)
				}
			}
		}
	}
	return found, nil
}

// HookCommand is the hook command for a cadre binary.
func HookCommand(binary string) string { return quote(binary) + " hook orchestrator" }

func quote(w string) string {
	if w != "" && strings.Trim(w, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:@%+,") == "" {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

func (h hooks) Set(binary string) (bool, error) {
	file := h.File()
	if _, err := os.Lstat(file); errors.Is(err, fs.ErrNotExist) {
		// Claude Code has no user settings yet: start them empty.
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			return false, err
		}
		f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, err = f.WriteString("{}\n")
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return false, err
		}
	}
	code, _ := jsonx.Edit(file, jsonx.Options{Backup: file + ".bak-cadre"}, AddHook(HookCommand(binary), isOrchestratorHook))
	switch code {
	case jsonx.Changed:
		return true, nil
	case jsonx.Unchanged:
		return false, nil
	case jsonx.KeptChanged:
		return false, fmt.Errorf("%s kept changing (a running Claude Code session?), so it was left unchanged; try again", file)
	case jsonx.WriteFailed:
		return false, fmt.Errorf("could not write next to %s, so it was left unchanged", file)
	}
	return false, fmt.Errorf("%s is not a file cadre can safely edit (unreadable, not valid JSON, an unexpected shape, or owned by another user), so it was left unchanged", file)
}

func (h hooks) Remove(binary string) (bool, error) {
	file := h.File()
	if _, err := os.Lstat(file); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	bin := paths.Real(binary)
	isOurs := func(c string) bool {
		prog, ok := OrchestratorHook(c)
		return ok && paths.Real(prog) == bin
	}
	code, _ := jsonx.Edit(file, jsonx.Options{Backup: file + ".bak-cadre"}, Unhook(isOurs, isOrchestratorHook))
	switch code {
	case jsonx.Changed:
		return true, nil
	case jsonx.Unchanged, jsonx.Missing:
		return false, nil
	case jsonx.KeptChanged:
		return false, fmt.Errorf("%s kept changing (a running Claude Code session?), so it was left unchanged; try again", file)
	case jsonx.WriteFailed:
		return false, fmt.Errorf("could not write next to %s, so it was left unchanged", file)
	}
	return false, fmt.Errorf("%s is not a file cadre can safely edit, so it was left unchanged", file)
}

// Output is a SessionStart hook's answer: text added to the new session's
// context.
func (hooks) Output(text string) []byte {
	type specific struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	}
	b, _ := json.Marshal(struct {
		HookSpecificOutput specific `json:"hookSpecificOutput"`
	}{specific{"SessionStart", text}})
	return append(b, '\n')
}
