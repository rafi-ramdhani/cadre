package session

import (
	"os"
	"syscall"
)

// execProcess replaces this process with path, as a shell's exec does.
func execProcess(path string, args []string) error {
	return syscall.Exec(path, args, os.Environ())
}
