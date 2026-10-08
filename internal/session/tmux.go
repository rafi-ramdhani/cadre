// Package session runs persona sessions: Claude Code sessions in tmux,
// one tmux session per team (and project), one window per role. It names
// them, records which cadre each belongs to, and starts, lists and stops
// them.
package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Tmux runs tmux on the user's server, or on a private one when
// CADRE_TMUX_SOCKET names a socket (tests use that). Commands are always
// run with their arguments as they are, never through a shell, and every
// target is exact (=name), since tmux otherwise matches prefixes and
// cadre down dev once stopped cadre-dev-app.
type Tmux struct{ Socket string }

// Default is the tmux cadre uses.
func Default() Tmux { return Tmux{Socket: os.Getenv("CADRE_TMUX_SOCKET")} }

// command runs a tmux client for cadre to read. -u marks the client as
// UTF-8 whatever the locale, so tmux does not turn non-ASCII characters in
// names and paths into "_" (a C locale, as on CI machines).
func (t Tmux) command(args ...string) *exec.Cmd {
	if t.Socket != "" {
		args = append([]string{"-L", t.Socket}, args...)
	}
	return exec.Command("tmux", append([]string{"-u"}, args...)...)
}

// run runs tmux and returns its output, trimmed.
func (t Tmux) run(args ...string) (string, error) {
	out, err := t.command(args...).CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

// Has reports whether a session called name runs.
func (t Tmux) Has(name string) bool {
	_, err := t.run("has-session", "-t", "="+name)
	return err == nil
}

// Option reads a session's user option ("" when unset or no session).
func (t Tmux) Option(session, name string) string {
	out, err := t.run("show-options", "-qv", "-t", "="+session+":", name)
	if err != nil {
		return ""
	}
	return out
}

// recordable refuses an option value that tmux could not give back as it
// is: Sessions and Personas read one line per session with fields split by
// sep, and tmux escapes control characters and bytes that are not UTF-8 in
// its output.
func recordable(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("cannot record %s in tmux: %q is not valid UTF-8", name, value)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("cannot record %s in tmux: %q holds a tab, a line break or another control character", name, value)
		}
	}
	return nil
}

// startDir passes a start folder to tmux's -c, which tmux expands as a
// format: "##" is a literal "#", so a folder holding "#{" stays itself.
func startDir(dir string) string { return strings.ReplaceAll(dir, "#", "##") }

// SetOption sets a session's user option.
func (t Tmux) SetOption(session, name, value string) error {
	if err := recordable(name, value); err != nil {
		return err
	}
	_, err := t.run("set-option", "-q", "-t", "="+session+":", name, value)
	return err
}

// SetWindowOption sets a window's user option.
func (t Tmux) SetWindowOption(session, window, name, value string) error {
	if err := recordable(name, value); err != nil {
		return err
	}
	_, err := t.run("set-option", "-w", "-q", "-t", "="+session+":="+window, name, value)
	return err
}

// Windows lists a session's window names.
func (t Tmux) Windows(session string) []string {
	out, err := t.run("list-windows", "-t", "="+session, "-F", "#W")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// HasWindow reports whether session has a window called name.
func (t Tmux) HasWindow(session, name string) bool {
	for _, w := range t.Windows(session) {
		if w == name {
			return true
		}
	}
	return false
}

// PaneDead reports whether a window's command has ended (a dead pane is
// kept when the user's tmux sets remain-on-exit).
func (t Tmux) PaneDead(session, window string) bool {
	out, err := t.run("list-panes", "-t", "="+session+":="+window, "-F", "#{pane_dead}")
	return err == nil && strings.HasPrefix(out, "1")
}

// Info is what cadre records on a session.
type Info struct {
	Name    string // the tmux session
	Home    string // @cadre_home: its cadre's physical path; "" for a legacy session
	Team    string // @cadre_team
	Project string // @cadre_project
	Role    string // @cadre_role: "orchestrator", or "" for a team's session
	Target  string // @cadre_target: the project or folder it was started for
}

// sep splits the fields of tmux's -F output. tmux writes command output
// through vis(3) with VIS_OCTAL|VIS_CSTYLE (3.4 does; later versions may
// not), which turns control characters into escapes such as \037, except
// tab and newline. So the separator is a tab, and recordable refuses tabs,
// line breaks and every other control character in recorded values.
const sep = "\t"

// Sessions lists the cadre sessions on the server, with their options.
func (t Tmux) Sessions() []Info {
	format := strings.Join([]string{"#S", "#{@cadre_home}", "#{@cadre_team}", "#{@cadre_project}", "#{@cadre_role}", "#{@cadre_target}"}, sep)
	out, err := t.run("list-sessions", "-F", format)
	if err != nil || out == "" {
		return nil
	}
	var list []Info
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, sep)
		if len(f) != 6 || !strings.HasPrefix(f[0], "cadre-") {
			continue
		}
		list = append(list, Info{Name: f[0], Home: f[1], Team: f[2], Project: f[3], Role: f[4], Target: f[5]})
	}
	return list
}

// Persona is a window and the Claude session name recorded on it.
type Persona struct{ Session, Window, Name string }

// Personas lists every window of every cadre session with its
// @cadre_persona (empty for windows from before it was recorded).
func (t Tmux) Personas() []Persona {
	out, err := t.run("list-windows", "-a", "-F", strings.Join([]string{"#S", "#W", "#{@cadre_persona}"}, sep))
	if err != nil || out == "" {
		return nil
	}
	var list []Persona
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, sep)
		if len(f) == 3 && strings.HasPrefix(f[0], "cadre-") {
			list = append(list, Persona{f[0], f[1], f[2]})
		}
	}
	return list
}

// Option is a tmux user option to set.
type Option struct{ Name, Value string }

// StartSpec is a window to start.
type StartSpec struct {
	Session, Window, Dir string
	Env                  []string // KEY=value
	Argv                 []string // at least the program and one argument
	SessionOptions       []Option // set on a new session, in the same tmux command
	WindowOptions        []Option
}

// word passes one argument to tmux as it is. tmux splits its command line
// at an argument that is or ends with ";", and reads a trailing "\;" as a
// literal ";", so a trailing ";" is escaped; an argument that already ends
// in "\;" is refused, since tmux would change it.
func word(w string) (string, error) {
	switch {
	case strings.HasSuffix(w, `\;`):
		return "", fmt.Errorf("cannot pass %q to tmux: it ends in a backslash and a semicolon", w)
	case strings.HasSuffix(w, ";"):
		return w[:len(w)-1] + `\;`, nil
	}
	return w, nil
}

// Start runs argv in a new window of the session (a new session when it
// does not run yet), in dir, with env added, and sets the options in the
// same tmux command, so no other cadre command sees the session without
// them. The command runs as it is: with several arguments, tmux execs it
// without a shell (and -e on new-session needs tmux 3.2), so no value
// needs quoting.
func (t Tmux) Start(s StartSpec) error {
	if len(s.Argv) < 2 {
		// With one argument, tmux hands it to a shell.
		return fmt.Errorf("tmux: a command needs at least two arguments to run without a shell")
	}
	for _, o := range append(append([]Option{}, s.SessionOptions...), s.WindowOptions...) {
		if err := recordable(o.Name, o.Value); err != nil {
			return err
		}
	}
	var args []string
	var bad error
	// add escapes each word as it goes in; only the separators added below
	// are bare ";".
	add := func(words ...string) {
		for _, w := range words {
			e, err := word(w)
			if err != nil && bad == nil {
				bad = err
			}
			args = append(args, e)
		}
	}
	isNew := !t.Has(s.Session)
	if isNew {
		add("new-session", "-d", "-s", s.Session, "-n", s.Window, "-c", startDir(s.Dir))
	} else {
		add("new-window", "-d", "-t", "="+s.Session+":", "-n", s.Window, "-c", startDir(s.Dir))
	}
	for _, e := range s.Env {
		add("-e", e)
	}
	add("--")
	add(s.Argv...)
	set := func(flag, target string, o Option) {
		args = append(args, ";")
		add("set-option", flag, "-t", target, o.Name, o.Value)
	}
	if isNew {
		for _, o := range s.SessionOptions {
			set("-q", "="+s.Session+":", o)
		}
	}
	for _, o := range s.WindowOptions {
		set("-wq", "="+s.Session+":="+s.Window, o)
	}
	if bad != nil {
		return bad
	}
	if out, err := t.run(args...); err != nil && t.Has(s.Session) {
		return fmt.Errorf("tmux: %s", out)
	}
	// A command that ended at once took its session with it before the
	// options were set; the caller's start check reports that.
	return nil
}

// KillSession stops a session.
func (t Tmux) KillSession(name string) error {
	_, err := t.run("kill-session", "-t", "="+name)
	return err
}

// KillWindow stops one window of a session.
func (t Tmux) KillWindow(session, window string) error {
	_, err := t.run("kill-window", "-t", "="+session+":="+window)
	return err
}

// Version returns tmux's major and minor version, or an error when tmux
// is missing.
func (t Tmux) Version() (int, int, error) {
	out, err := exec.Command("tmux", "-V").Output()
	if err != nil {
		return 0, 0, errors.New("tmux is not installed")
	}
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "tmux"))
	v = strings.TrimPrefix(v, "next-")
	majorText, rest, _ := strings.Cut(v, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return 0, 0, fmt.Errorf("cannot read the tmux version %q", out)
	}
	minor := 0
	for i := 0; i < len(rest) && rest[i] >= '0' && rest[i] <= '9'; i++ {
		minor = minor*10 + int(rest[i]-'0')
	}
	return major, minor, nil
}

// Own returns the cadre session this process runs in, if any: $TMUX names
// a session on the same tmux server.
func (t Tmux) Own() string {
	tmuxEnv, pane := os.Getenv("TMUX"), os.Getenv("TMUX_PANE")
	if tmuxEnv == "" || pane == "" {
		return ""
	}
	socket, _, _ := strings.Cut(tmuxEnv, ",")
	if path, err := t.run("display", "-p", "-t", pane, "#{socket_path}"); err != nil || path != socket {
		return ""
	}
	name, err := t.run("display", "-p", "-t", pane, "#S")
	if err != nil || !strings.HasPrefix(name, "cadre-") {
		return ""
	}
	return name
}

// Attach replaces this process with tmux attached to session (or switches
// the client when already inside tmux).
func (t Tmux) Attach(session string) error {
	path, err := exec.LookPath("tmux")
	if err != nil {
		return errors.New("tmux is not installed")
	}
	args := []string{"tmux"}
	if t.Socket != "" {
		args = append(args, "-L", t.Socket)
	}
	if os.Getenv("TMUX") != "" {
		args = append(args, "switch-client", "-t", "="+session)
	} else {
		args = append(args, "attach", "-t", "="+session)
	}
	return execProcess(path, args)
}
