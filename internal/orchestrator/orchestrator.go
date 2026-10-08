// Package orchestrator holds what cadre keeps about a cadre's orchestrator:
// its prompt and the lock that says one is open (section M.3).
package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rafi-ramdhani/cadre/internal/fsx"
	"github.com/rafi-ramdhani/cadre/internal/proc"
)

// Modes an orchestrator runs in.
const (
	Terminal = "terminal" // plain cadre: a child process in the user's terminal
	Tmux     = "tmux"     // cadre --tmux: a tmux session
)

// SessionName is the orchestrator's tmux session (K-Q6).
func SessionName(cadre string) string { return "cadre-" + cadre }

// Name is the orchestrator's session name, its messaging address.
func Name(cadre string) string { return cadre + "-orchestrator" }

// Lock records an open orchestrator: the process cadre started, so a
// reused pid is never taken for it.
type Lock struct {
	PID     int       `json:"pid"`
	Start   string    `json:"start"`   // the process's start time
	Command string    `json:"command"` // its command name
	TTY     string    `json:"tty,omitempty"`
	Mode    string    `json:"mode"`
	Session string    `json:"session,omitempty"` // the tmux session, in tmux mode
	Since   time.Time `json:"since"`
}

// LockPath is the lock's file in a cadre's build folder.
func LockPath(build string) string { return filepath.Join(build, "orchestrator.lock") }

// ReadLock returns the open orchestrator, or nil. A lock whose process is
// gone, restarted under the same pid, or another command, is stale and
// removed.
func ReadLock(path string) *Lock {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var l Lock
	if err != nil || json.Unmarshal(raw, &l) != nil || l.PID <= 0 || !proc.Same(l.PID, proc.Info{Start: l.Start, Command: l.Command}) {
		os.Remove(path)
		return nil
	}
	return &l
}

// WriteLock records the orchestrator running as pid.
func WriteLock(path string, pid int, mode, tty, session string) error {
	info, err := proc.Of(pid)
	if err != nil {
		return err
	}
	l := Lock{PID: pid, Start: info.Start, Command: info.Command, TTY: tty, Mode: mode, Session: session, Since: time.Now()}
	raw, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fsx.WriteFile(path, append(raw, '\n'), 0o600)
}

// Prompt is the orchestrator's prompt: the framework's orchestrator text,
// which cadre it leads, and, when cadre was opened from a linked project,
// that project (N.4).
func Prompt(text []byte, cadre, path, project, projectDir string) []byte {
	out := append([]byte{}, text...)
	out = append(out, fmt.Sprintf("\nYou are the orchestrator of cadre `%s` (`%s`).\n", cadre, path)...)
	if project != "" {
		out = append(out, fmt.Sprintf("The user opened cadre from the project `%s` (`%s`).\n", project, projectDir)...)
	}
	return out
}
