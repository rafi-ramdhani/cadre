// Package proc tells whether a process is still the one cadre started:
// alive, with the same start time and command, so a reused pid is never
// taken for it.
package proc

import "errors"

// Info identifies a running process.
type Info struct {
	Start   string // the process's start time, as the system reports it
	Command string // its command name (for example node, or claude)
}

// ErrGone is returned for a process that no longer runs.
var ErrGone = errors.New("no such process")

// Same reports whether pid is still the process described by want.
func Same(pid int, want Info) bool {
	got, err := Of(pid)
	return err == nil && got == want
}
