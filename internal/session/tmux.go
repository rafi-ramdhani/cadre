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
)

// Tmux runs tmux on the user's server, or on a private one when
// CADRE_TMUX_SOCKET names a socket (tests use that). Commands are always
// run with their arguments as they are, never through a shell, and every
// target is exact (=name), since tmux otherwise matches prefixes and
// cadre down dev once stopped cadre-dev-app.
type Tmux struct{ Socket string }

// Default is the tmux cadre uses.
func Default() Tmux { return Tmux{Socket: os.Getenv("CADRE_TMUX_SOCKET")} }

func (t Tmux) command(args ...string) *exec.Cmd {
	if t.Socket != "" {
		args = append([]string{"-L", t.Socket}, args...)
	}
	return exec.Command("tmux", args...)
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

// SetOption sets a session's user option.
func (t Tmux) SetOption(session, name, value string) error {
	_, err := t.run("set-option", "-q", "-t", "="+session+":", name, value)
	return err
}

// SetWindowOption sets a window's user option.
func (t Tmux) SetWindowOption(session, window, name, value string) error {
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

const sep = "\x1f" // a separator no name or path holds

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

// Start runs argv in a new window of session (a new session when it does
// not run yet), in dir, with env added. The command runs as it is: tmux
// 3.0 and newer exec a command given as several arguments without a shell,
// so no value needs quoting.
func (t Tmux) Start(session, window, dir string, env []string, argv []string) error {
	var args []string
	if t.Has(session) {
		args = []string{"new-window", "-d", "-t", "=" + session + ":", "-n", window, "-c", dir}
	} else {
		args = []string{"new-session", "-d", "-s", session, "-n", window, "-c", dir}
	}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(append(args, "--"), argv...)
	if out, err := t.run(args...); err != nil {
		return fmt.Errorf("tmux: %s", out)
	}
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
