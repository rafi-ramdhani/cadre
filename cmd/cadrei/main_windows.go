//go:build windows

// Command cadrei needs tmux, which Windows does not have; this build only
// says so. On Windows, run cadrei in WSL 2.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cadrei: Windows is not supported (cadrei needs tmux); use WSL 2")
	os.Exit(1)
}
