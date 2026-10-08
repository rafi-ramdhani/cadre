//go:build windows

// Command cadre needs tmux, which Windows does not have; this build only
// says so. On Windows, run cadre in WSL 2.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cadre: Windows is not supported (cadre needs tmux); use WSL 2")
	os.Exit(1)
}
