package proc

import (
	"os"
	"strconv"
	"strings"
)

// Of reads a process's start time (in clock ticks since boot) and command
// from /proc.
func Of(pid int) (Info, error) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return Info{}, ErrGone
	}
	s := string(raw)
	// The command is in parentheses and may hold spaces or parentheses.
	open, close := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || close < open {
		return Info{}, ErrGone
	}
	fields := strings.Fields(s[close+1:])
	// After the command: state is field 3; starttime is field 22.
	if len(fields) < 20 || fields[0] == "Z" {
		return Info{}, ErrGone
	}
	return Info{Start: fields[19], Command: s[open+1 : close]}, nil
}
