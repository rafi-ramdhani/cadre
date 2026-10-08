package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/paths"
	"github.com/rafi-ramdhani/cadre/internal/runtime"
)

// Up starts a team, or one of its roles, for this cadre.
type Up struct {
	Scope
	Team     string
	Role     string // "" for every role of the team
	Project  string // the project part of the key, or ""
	Target   string // @cadre_target: the registry name or folder it was started for
	Dir      string // the working folder
	Explicit bool   // Dir was given (a project or folder): a role's .workdir pin does not apply
	Protocol []byte // the persona protocol every prompt starts with
	Mode     string // the permission mode
	// Pick returns a role's runtime and the grants artifact it starts with
	// ("" for none), or why the role cannot start.
	Pick func(role string) (runtime.Runtime, string, error)
	Wait time.Duration
}

// Roles lists a team's roles: the .md files in personas/<team>.
func Roles(cadre, team string) []string {
	files, _ := filepath.Glob(filepath.Join(cadre, "personas", team, "*.md"))
	var out []string
	for _, f := range files {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".md"))
	}
	sort.Strings(out)
	return out
}

// workdir reads the first line of a .workdir file, with ~ expanded.
func workdir(file string) string {
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(raw), "\n")
	line = strings.TrimSpace(line)
	if line == "~" || strings.HasPrefix(line, "~/") {
		line = paths.Home() + line[1:]
	}
	return line
}

// DefaultDir is a team's working folder: its .workdir file, or
// <cadre>/teams/<team>.
func DefaultDir(cadre, team string) string {
	if d := workdir(filepath.Join(cadre, "personas", team, ".workdir")); d != "" {
		return d
	}
	return filepath.Join(cadre, "teams", team)
}

// Start starts the roles and prints a line each. It reports whether any
// failed to start.
func (u Up) Start(out io.Writer) bool {
	roles := []string{u.Role}
	if u.Role == "" {
		roles = Roles(u.Path, u.Team)
	}
	failed := false
	for _, r := range roles {
		if err := u.start(out, r); err != nil {
			fmt.Fprintln(out, err)
			failed = true
		}
	}
	key := Key(u.Team, u.Project)
	if s := u.mine(key); s != "" {
		// Recorded so the restart commands can name the exact target.
		u.T.SetOption(s, "@cadre_target", u.Target)
	}
	return failed
}

func (u Up) start(out io.Writer, role string) error {
	key := Key(u.Team, u.Project)
	session := SessionName(u.Name, key)
	name := PersonaName(u.Name, key, role)
	personaFile := filepath.Join(u.Path, "personas", u.Team, role+".md")
	if _, err := os.Stat(personaFile); err != nil {
		return fmt.Errorf("  no persona %s/%s", u.Team, role)
	}
	if u.T.HasWindow(session, role) && u.mine(key) == session {
		fmt.Fprintf(out, "  %s already running\n", name)
		return nil
	}
	if l := u.legacy(key); l != "" && u.T.HasWindow(l, role) {
		fmt.Fprintf(out, "  %s-%s already running (legacy session %s; it keeps its old name until restarted)\n", key, role, l)
		return nil
	}
	if err := u.CheckPersona(session, role, name); err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	rt, grants, err := u.Pick(role)
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	dir := u.Dir
	if !u.Explicit {
		// A <role>.workdir file pins one persona to its own folder.
		if d := workdir(filepath.Join(u.Path, "personas", u.Team, role+".workdir")); d != "" {
			dir = d
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	prompt, err := u.writePrompt(rt.BuildDir(u.Path), key, role, personaFile)
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	cmd, err := rt.Launch(runtime.LaunchSpec{Role: runtime.Persona, Name: name, Cadre: u.Path, WorkDir: dir,
		Mode: u.Mode, PromptFile: prompt, Grants: grants})
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	if cmd.Dir != "" {
		dir = cmd.Dir
	}
	argv := cmd.Argv
	// CADRE_HOME pins the persona to this cadre wherever it works.
	env := append([]string{"CADRE_HOME=" + u.Path, "CADRE_PERSONA=" + name}, cmd.Env...)
	err = u.T.Start(StartSpec{Session: session, Window: role, Dir: dir, Env: env, Argv: argv,
		SessionOptions: []Option{{"@cadre_home", u.Path}, {"@cadre_team", u.Team}, {"@cadre_project", u.Project}},
		WindowOptions:  []Option{{"@cadre_persona", name}}})
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	// A command that cannot run ends at once: its window closes, or stays
	// with a dead pane when the user's tmux sets remain-on-exit.
	wait := u.Wait
	if wait == 0 {
		wait = 500 * time.Millisecond
	}
	time.Sleep(wait)
	if !u.T.HasWindow(session, role) || u.T.PaneDead(session, role) {
		return fmt.Errorf("  %s failed to start: its command exited at once; run it by hand in %s to see why:\n    %s",
			name, dir, shellLine(append(env, argv...)))
	}
	fmt.Fprintf(out, "  %s started in %s\n", name, dir)
	return nil
}

// writePrompt builds the persona's prompt, rebuilt and swapped in whole at
// every start: the protocol, the cadre's own protocol.md, then the persona.
func (u Up) writePrompt(build, key, role, personaFile string) (string, error) {
	var b strings.Builder
	b.Write(u.Protocol)
	if own, err := os.ReadFile(filepath.Join(u.Path, "protocol.md")); err == nil {
		b.WriteString("\n")
		b.Write(own)
	}
	persona, err := os.ReadFile(personaFile)
	if err != nil {
		return "", err
	}
	b.WriteString("\n")
	b.Write(persona)
	if err := EnsureBuild(build); err != nil {
		return "", err
	}
	path := filepath.Join(build, key+"-"+role+".md")
	return path, fsx.WriteFile(path, []byte(b.String()), 0o644)
}

// EnsureBuild makes the folder for generated prompts and settings copies,
// ignored by the cadre's git through its own .gitignore.
func EnsureBuild(build string) error {
	if err := os.MkdirAll(build, 0o755); err != nil {
		return err
	}
	ignore := filepath.Join(build, ".gitignore")
	if st, err := os.Lstat(ignore); err == nil && st.Mode().IsRegular() {
		return nil
	}
	// Written through a new file and a rename, which replaces a link
	// planted here instead of following it.
	return fsx.WriteFile(ignore, []byte("*\n"), 0o644)
}

// shellLine writes words for a POSIX shell, each single-quoted when it
// needs it, for a user to paste.
func shellLine(words []string) string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = Quote(w)
	}
	return strings.Join(out, " ")
}

// Quote quotes a word for a POSIX shell when it needs it.
func Quote(w string) string {
	if w != "" && strings.Trim(w, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:@%+,") == "" {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}
