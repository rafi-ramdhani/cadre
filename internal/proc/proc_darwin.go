package proc

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// Of reads a process's start time and command from the kernel.
func Of(pid int) (Info, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) {
		return Info{}, ErrGone
	}
	start := kp.Proc.P_starttime
	comm := kp.Proc.P_comm[:]
	n := 0
	for n < len(comm) && comm[n] != 0 {
		n++
	}
	return Info{Start: fmt.Sprintf("%d.%06d", start.Sec, start.Usec), Command: string(comm[:n])}, nil
}
