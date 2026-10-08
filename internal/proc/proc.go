// Package proc tells whether a process is still the one cadre started:
// alive, with the same start time, so a reused pid is never taken for it.
package proc

import "errors"

// Info identifies a running process.
type Info struct {
	Start   string // the process's start time, as the system reports it
	Command string // its command name (for example node, or claude)
}

// ErrGone is returned for a process that no longer runs.
var ErrGone = errors.New("no such process")

// Same reports whether pid is still the process described by want. Only
// the start time is compared: a process keeps it across exec, while its
// command name changes when a wrapper execs the real program (a
// #!/usr/bin/env node script, a version manager's shim).
func Same(pid int, want Info) bool {
	got, err := Of(pid)
	return err == nil && got.Start == want.Start
}
