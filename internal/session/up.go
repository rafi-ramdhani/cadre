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
	Protocol []byte // the member protocol every prompt starts with
	Mode     string // the permission mode
	// Pick returns a role's runtime and the grants artifact it starts with
	// ("" for none), or why the role cannot start.
	Pick func(role string) (runtime.Runtime, string, error)
	Wait time.Duration
	// Fresh starts new conversations instead of resuming recorded ones.
	Fresh bool
}

// Roles lists a team's roles: the .md files in members/<team>.
func Roles(cadre, team string) []string {
	files, _ := filepath.Glob(filepath.Join(cadre, "members", team, "*.md"))
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
	if d := workdir(filepath.Join(cadre, "members", team, ".workdir")); d != "" {
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
	return failed
}

func (u Up) start(out io.Writer, role string) error {
	key := Key(u.Team, u.Project)
	session := SessionName(u.Name, key)
	name := MemberName(u.Name, key, role)
	memberFile := filepath.Join(u.Path, "members", u.Team, role+".md")
	if _, err := os.Stat(memberFile); err != nil {
		return fmt.Errorf("  no member %s/%s", u.Team, role)
	}
	if u.T.HasWindow(session, role) && u.mine(u.Team, u.Project) == session {
		fmt.Fprintf(out, "  %s already running\n", name)
		return nil
	}
	if l := u.legacy(key); l != "" && u.T.HasWindow(l, role) {
		fmt.Fprintf(out, "  %s-%s already running (legacy session %s; it keeps its old name until restarted)\n", key, role, l)
		return nil
	}
	if err := u.CheckMember(session, role, name); err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	rt, grants, err := u.Pick(role)
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	dir := u.Dir
	if !u.Explicit {
		// A <role>.workdir file pins one member to its own folder.
		if d := workdir(filepath.Join(u.Path, "members", u.Team, role+".workdir")); d != "" {
			dir = d
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	prompt, err := u.writePrompt(rt.BuildDir(u.Path), key, role, memberFile)
	if err != nil {
		return fmt.Errorf("  %s not started: %s", name, err)
	}
	record := RecordPath(rt.BuildDir(u.Path), name)
	conv := Plan(rt, record, dir, u.Fresh)
	started, err := u.run(rt, session, role, name, dir, prompt, grants, conv)
	if err != nil && conv.Resume != "" {
		// The runtime would not resume it: start a new conversation.
		u.T.KillWindow(session, role)
		conv = Conversation{SessionID: rt.Sessions().NewID(), Note: "a new conversation: resuming failed"}
		started, err = u.run(rt, session, role, name, dir, prompt, grants, conv)
	}
	if err != nil {
		return err
	}
	if conv.SessionID != "" {
		WriteRecord(record, Record{ID: conv.SessionID, Dir: started, Since: time.Now()})
	}
	if conv.Note != "" {
		fmt.Fprintf(out, "  %s started in %s (%s)\n", name, started, conv.Note)
	} else {
		fmt.Fprintf(out, "  %s started in %s\n", name, started)
	}
	return nil
}

// run starts one member's window with the conversation conv, and checks
// that it is still there a moment later. It returns the folder it runs in.
func (u Up) run(rt runtime.Runtime, session, role, name, dir, prompt, grants string, conv Conversation) (string, error) {
	cmd, err := rt.Launch(runtime.LaunchSpec{Role: runtime.Member, Name: name, Cadre: u.Path, WorkDir: dir,
		Mode: u.Mode, PromptFile: prompt, Grants: grants, SessionID: conv.SessionID, Resume: conv.Resume})
	if err != nil {
		return "", fmt.Errorf("  %s not started: %s", name, err)
	}
	if cmd.Dir != "" {
		dir = cmd.Dir
	}
	argv := cmd.Argv
	// CADRE_HOME pins the member to this cadre wherever it works.
	env := append([]string{"CADRE_HOME=" + u.Path, "CADRE_MEMBER=" + name}, cmd.Env...)
	hint, hooks := u.T.HintOptions()
	err = u.T.Start(StartSpec{Session: session, Window: role, Dir: dir, Env: env, Argv: argv,
		// The target too, recorded with the rest, so a cadre up whose
		// output pipe closes early (cadre up | head) still records it.
		SessionOptions: append([]Option{{"@cadre_home", u.Path}, {"@cadre_team", u.Team}, {"@cadre_project", u.Project}, {"@cadre_target", u.Target}}, hint...),
		SessionHooks:   hooks,
		WindowOptions:  []Option{{"@cadre_member", name}}})
	if err != nil {
		return "", fmt.Errorf("  %s not started: %s", name, err)
	}
	// A command that cannot run ends at once: its window closes, or stays
	// with a dead pane when the user's tmux sets remain-on-exit.
	wait := u.Wait
	if wait == 0 {
		wait = 500 * time.Millisecond
	}
	time.Sleep(wait)
	if !u.T.HasWindow(session, role) || u.T.PaneDead(session, role) {
		return "", fmt.Errorf("  %s failed to start: its command exited at once; run it by hand in %s to see why:\n    %s",
			name, dir, Line(append(env, argv...)))
	}
	return dir, nil
}

// writePrompt builds the member's prompt, rebuilt and swapped in whole at
// every start: the protocol, the cadre's own protocol.md, then the member.
func (u Up) writePrompt(build, key, role, memberFile string) (string, error) {
	var b strings.Builder
	b.Write(u.Protocol)
	if own, err := os.ReadFile(filepath.Join(u.Path, "protocol.md")); err == nil {
		b.WriteString("\n")
		b.Write(own)
	}
	member, err := os.ReadFile(memberFile)
	if err != nil {
		return "", err
	}
	b.WriteString("\n")
	b.Write(member)
	if err := EnsureBuild(build); err != nil {
		return "", err
	}
	path := filepath.Join(build, key+"-"+role+".md")
	return path, fsx.WriteFile(path, []byte(b.String()), 0o644)
}

// EnsureBuild makes the folder for generated prompts and settings copies,
// ignored by the cadre's git through its own .gitignore.
func EnsureBuild(build string) error {
	// A cloned cadre can carry a committed link here; cadre writes its
	// prompts, settings copies and locks only into a folder of its own.
	for _, d := range []string{filepath.Dir(build), build} {
		if st, err := os.Lstat(d); err == nil && !st.IsDir() {
			return fmt.Errorf("%s is a link or a file, not a folder; cadre writes its generated files only into a folder of its own, so remove it and run again", d)
		}
	}
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

// Line writes words for a POSIX shell, each single-quoted when it needs
// it, for a user to paste.
func Line(words []string) string {
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
